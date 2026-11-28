package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"food-service/internal/maintenance"
)

func TestNotifierRespectsMaintenanceGate(t *testing.T) {
	db := newTestDB(t)
	fake, server := newFakeServer(t)
	orderID := seedOrderWithOutbox(t, db, []int64{101})

	var jobID int64
	var recipientID int64
	if err := db.QueryRow(
		`SELECT id, recipient_id FROM notification_outbox WHERE order_id = ?`,
		orderID,
	).Scan(&jobID, &recipientID); err != nil {
		t.Fatalf("read outbox job: %v", err)
	}

	notifier := newTestNotifier(t, db, server.URL, []int64{101})
	gate := maintenance.New()
	notifier.SetGate(gate)

	gate.Lock()

	if _, err := notifier.fetchDue(context.Background(), outboxBatchSize); !errors.Is(err, errMaintenance) {
		t.Fatalf("fetchDue must stop at maintenance, got %v", err)
	}

	// Loading an order and deleting its outbox rows must not touch SQLite or
	// send a message while maintenance is active.
	notifier.deliver(context.Background(), []outboxJob{{
		ID:          jobID,
		OrderID:     orderID,
		RecipientID: recipientID,
	}})
	notifier.process(context.Background())

	// These are the operations that run after a network call. Entering the
	// gate must fail, leaving the job untouched for the next cycle.
	notifier.markSent(context.Background(), outboxJob{ID: jobID})
	notifier.recordFailure(
		context.Background(),
		outboxJob{ID: jobID, OrderID: orderID, RecipientID: recipientID},
		errors.New("temporary failure"),
		false,
		0,
	)

	status, attempts, _ := outboxState(t, db, recipientID)
	if status != "pending" || attempts != 0 {
		t.Fatalf("outbox changed during maintenance: (%q, %d)", status, attempts)
	}
	if len(fake.messagesFor(recipientID)) != 0 {
		t.Fatalf("notifier sent a message during maintenance")
	}

	gate.Unlock()

	// After maintenance the normal flow must still work.
	notifier.process(context.Background())

	status, attempts, sentAt := outboxState(t, db, recipientID)
	if status != "sent" || attempts != 1 || !sentAt.Valid {
		t.Fatalf("after maintenance = (%q, %d, valid=%v), want (sent, 1, true)", status, attempts, sentAt.Valid)
	}
	if got := len(fake.messagesFor(recipientID)); got != 1 {
		t.Fatalf("messages after maintenance = %d, want 1", got)
	}
}

func TestNotifierStopsSendingPartsAfterMaintenanceStarts(t *testing.T) {
	db := newTestDB(t)
	gate := maintenance.New()

	firstSendStarted := make(chan struct{})
	allowFirstSend := make(chan struct{})
	var once sync.Once
	var releaseOnce sync.Once
	releaseFirstSend := func() {
		releaseOnce.Do(func() { close(allowFirstSend) })
	}
	defer releaseFirstSend()
	var sends atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			http.NotFound(w, r)
			return
		}

		once.Do(func() { close(firstSendStarted) })
		<-allowFirstSend

		sends.Add(1)
		_ = json.NewEncoder(w).Encode(apiResponse{OK: true})
	}))
	defer server.Close()

	notifier := newTestNotifier(t, db, server.URL, []int64{101})
	notifier.SetGate(gate)

	done := make(chan struct{})
	go func() {
		notifier.deliverJob(
			context.Background(),
			outboxJob{ID: 1, OrderID: 1, RecipientID: 101},
			[]string{"part 1", "part 2", "part 3"},
		)
		close(done)
	}()

	select {
	case <-firstSendStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("notifier did not start sending the first part")
	}

	// Maintenance starts while the first part is still in flight. The network
	// request is allowed to finish, but no further parts may be sent.
	gate.Lock()
	releaseFirstSend()
	<-done
	gate.Unlock()

	if got := sends.Load(); got != 1 {
		t.Fatalf("sent parts = %d, want 1 (maintenance must stop later parts)", got)
	}
}

func TestNotifierLeavesJobPendingWhenMaintenanceStartsDuringSend(t *testing.T) {
	db := newTestDB(t)
	orderID := seedOrderWithOutbox(t, db, []int64{101})
	_ = orderID

	gate := maintenance.New()

	sendStarted := make(chan struct{})
	allowResponse := make(chan struct{})
	var once sync.Once
	var sends atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			http.NotFound(w, r)
			return
		}

		once.Do(func() { close(sendStarted) })
		<-allowResponse

		sends.Add(1)
		_ = json.NewEncoder(w).Encode(apiResponse{OK: true})
	}))
	defer server.Close()

	notifier := newTestNotifier(t, db, server.URL, []int64{101})
	notifier.SetGate(gate)

	done := make(chan struct{})
	go func() {
		notifier.process(context.Background())
		close(done)
	}()

	select {
	case <-sendStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("notifier did not start the network send")
	}

	// Maintenance starts while Telegram is still processing the request. The
	// notifier must not hold the gate during SendMessage, so this lock succeeds.
	gate.Lock()
	close(allowResponse)
	<-done

	status, attempts, _ := outboxState(t, db, 101)
	if status != "pending" || attempts != 0 {
		t.Fatalf("outbox updated after maintenance started during send: (%q, %d)", status, attempts)
	}

	gate.Unlock()

	// The pending job is retried normally after maintenance.
	notifier.process(context.Background())
	status, attempts, sentAt := outboxState(t, db, 101)
	if status != "sent" || attempts != 1 || !sentAt.Valid {
		t.Fatalf("after retry = (%q, %d, valid=%v), want (sent, 1, true)", status, attempts, sentAt.Valid)
	}
	if got := sends.Load(); got != 2 {
		t.Fatalf("network sends = %d, want 2 (at-least-once)", got)
	}
}
