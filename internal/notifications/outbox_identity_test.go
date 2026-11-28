package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"food-service/internal/orders"
)

// seedOrderWithOutboxAt inserts an order and one due outbox row. An explicit
// outboxID models SQLite reusing the numeric id of a deleted row.
func seedOrderWithOutboxAt(
	t *testing.T,
	db *sql.DB,
	tokenHash string,
	outboxID int64,
	recipientID int64,
	nextAttempt string,
) int64 {
	t.Helper()

	result, err := db.Exec(
		`INSERT INTO orders (token_hash, token, customer_name, customer_phone, comment)
		 VALUES (?, ?, 'Илья', '+70000000000', '')`,
		tokenHash,
		"token-"+tokenHash,
	)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}
	orderID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get order ID: %v", err)
	}

	if outboxID > 0 {
		_, err = db.Exec(
			`INSERT INTO notification_outbox (id, order_id, recipient_id, next_attempt_at)
			 VALUES (?, ?, ?, ?)`,
			outboxID,
			orderID,
			recipientID,
			nextAttempt,
		)
	} else {
		_, err = db.Exec(
			`INSERT INTO notification_outbox (order_id, recipient_id, next_attempt_at)
			 VALUES (?, ?, ?)`,
			orderID,
			recipientID,
			nextAttempt,
		)
	}
	if err != nil {
		t.Fatalf("insert outbox row: %v", err)
	}

	return orderID
}

func readOutboxJob(t *testing.T, db *sql.DB, orderID int64) outboxJob {
	t.Helper()

	var job outboxJob
	if err := db.QueryRow(
		`SELECT id, job_token, order_id, recipient_id, attempts
		 FROM notification_outbox
		 WHERE order_id = ?`,
		orderID,
	).Scan(&job.ID, &job.JobToken, &job.OrderID, &job.RecipientID, &job.Attempts); err != nil {
		t.Fatalf("read outbox job for order %d: %v", orderID, err)
	}
	if job.JobToken == "" {
		t.Fatal("outbox row has an empty job_token")
	}

	return job
}

func readOutboxStateByID(
	t *testing.T,
	db *sql.DB,
	id int64,
) (string, int, sql.NullString, string) {
	t.Helper()

	var (
		status    string
		attempts  int
		sentAt    sql.NullString
		lastError string
	)
	if err := db.QueryRow(
		`SELECT status, attempts, sent_at, last_error
		 FROM notification_outbox
		 WHERE id = ?`,
		id,
	).Scan(&status, &attempts, &sentAt, &lastError); err != nil {
		t.Fatalf("read outbox row %d: %v", id, err)
	}

	return status, attempts, sentAt, lastError
}

// gatedTelegram blocks the first sendMessage request until the test releases
// it, so the stale-handler interleaving is fully deterministic.
type gatedTelegram struct {
	started     chan struct{}
	release     chan struct{}
	startOnce   sync.Once
	releaseOnce sync.Once
	statusCode  int
}

func newGatedTelegram(t *testing.T, statusCode int) (*gatedTelegram, *httptest.Server) {
	t.Helper()

	fake := &gatedTelegram{
		started:    make(chan struct{}),
		release:    make(chan struct{}),
		statusCode: statusCode,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			http.NotFound(w, r)
			return
		}

		fake.startOnce.Do(func() { close(fake.started) })
		<-fake.release

		if fake.statusCode == http.StatusOK {
			_ = json.NewEncoder(w).Encode(apiResponse{OK: true})
			return
		}

		w.WriteHeader(fake.statusCode)
		_ = json.NewEncoder(w).Encode(apiResponse{
			OK:          false,
			ErrorCode:   fake.statusCode,
			Description: "injected failure",
		})
	}))
	t.Cleanup(server.Close)

	return fake, server
}

func (f *gatedTelegram) waitStarted(t *testing.T) {
	t.Helper()

	select {
	case <-f.started:
	case <-time.After(5 * time.Second):
		t.Fatal("notifier did not start the Telegram request")
	}
}

func (f *gatedTelegram) allowResponse() {
	f.releaseOnce.Do(func() { close(f.release) })
}

func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("notifier did not finish")
	}
}

func TestStaleOutboxJobCannotMarkReusedIDAsSent(t *testing.T) {
	db := newTestDB(t)
	orderID := seedOrderWithOutboxAt(t, db, "stale-sent", 0, 101, "2000-01-01 00:00:00")
	oldJob := readOutboxJob(t, db, orderID)

	fake, server := newGatedTelegram(t, http.StatusOK)
	notifier := newTestNotifier(t, db, server.URL, []int64{101})

	done := make(chan struct{})
	go func() {
		notifier.process(context.Background())
		close(done)
	}()
	fake.waitStarted(t)

	// The old request is in flight. Remove the original order (and its outbox
	// row through ON DELETE CASCADE), then create a new notification that
	// reuses the same numeric id.
	if err := (&orders.Service{DB: db}).DeleteOrder(orderID); err != nil {
		t.Fatalf("delete original order: %v", err)
	}
	newOrderID := seedOrderWithOutboxAt(
		t,
		db,
		"stale-sent-new",
		oldJob.ID,
		101,
		"2999-01-01 00:00:00",
	)

	fake.allowResponse()
	waitDone(t, done)

	status, attempts, sentAt, lastError := readOutboxStateByID(t, db, oldJob.ID)
	if status != "pending" || attempts != 0 || sentAt.Valid || lastError != "" {
		t.Fatalf(
			"stale success changed the reused row: status=%q attempts=%d sent_at=%v last_error=%q",
			status,
			attempts,
			sentAt,
			lastError,
		)
	}

	var storedOrderID int64
	if err := db.QueryRow(
		`SELECT order_id FROM notification_outbox WHERE id = ?`,
		oldJob.ID,
	).Scan(&storedOrderID); err != nil {
		t.Fatalf("read reused outbox row: %v", err)
	}
	if storedOrderID != newOrderID {
		t.Fatalf("outbox row order_id = %d, want %d", storedOrderID, newOrderID)
	}
}

func TestStaleOutboxJobCannotRecordFailureOnReusedID(t *testing.T) {
	db := newTestDB(t)
	orderID := seedOrderWithOutboxAt(t, db, "stale-failure", 0, 101, "2000-01-01 00:00:00")
	oldJob := readOutboxJob(t, db, orderID)

	fake, server := newGatedTelegram(t, http.StatusInternalServerError)
	notifier := newTestNotifier(t, db, server.URL, []int64{101})

	done := make(chan struct{})
	go func() {
		notifier.process(context.Background())
		close(done)
	}()
	fake.waitStarted(t)

	if err := (&orders.Service{DB: db}).DeleteOrder(orderID); err != nil {
		t.Fatalf("delete original order: %v", err)
	}
	seedOrderWithOutboxAt(
		t,
		db,
		"stale-failure-new",
		oldJob.ID,
		101,
		"2999-01-01 00:00:00",
	)

	fake.allowResponse()
	waitDone(t, done)

	status, attempts, sentAt, lastError := readOutboxStateByID(t, db, oldJob.ID)
	if status != "pending" || attempts != 0 || sentAt.Valid || lastError != "" {
		t.Fatalf(
			"stale failure changed the reused row: status=%q attempts=%d sent_at=%v last_error=%q",
			status,
			attempts,
			sentAt,
			lastError,
		)
	}
}

func TestDeletingOrderDoesNotBlockLaterNotifications(t *testing.T) {
	db := newTestDB(t)
	fake, server := newFakeServer(t)

	firstOrderID := seedOrderWithOutboxAt(t, db, "delete-first", 0, 101, "2000-01-01 00:00:00")
	if err := (&orders.Service{DB: db}).DeleteOrder(firstOrderID); err != nil {
		t.Fatalf("delete first order: %v", err)
	}

	secondOrderID := seedOrderWithOutboxAt(t, db, "delete-second", 0, 101, "2000-01-01 00:00:00")
	notifier := newTestNotifier(t, db, server.URL, []int64{101})
	notifier.process(context.Background())

	status, attempts, sentAt := outboxState(t, db, 101)
	if status != "sent" || attempts != 1 || !sentAt.Valid {
		t.Fatalf("later notification = (%q, %d, valid=%v), want (sent, 1, true)", status, attempts, sentAt.Valid)
	}

	messages := fake.messagesFor(101)
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	if !strings.Contains(messages[0].Text, "Новый заказ #"+strconv.FormatInt(secondOrderID, 10)) {
		t.Fatalf("later notification does not reference order #%d: %q", secondOrderID, messages[0].Text)
	}
}

func TestNotificationsContinueAfterOutboxClearedByRestore(t *testing.T) {
	db := newTestDB(t)
	fake, server := newFakeServer(t)

	seedOrderWithOutboxAt(t, db, "before-restore", 0, 101, "2000-01-01 00:00:00")

	// ApplyRestore clears notification_outbox before committing the restored
	// data. New orders enqueue rows afterwards into the same table.
	if _, err := db.Exec(`DELETE FROM notification_outbox`); err != nil {
		t.Fatalf("clear outbox: %v", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM notification_outbox`); got != 0 {
		t.Fatalf("outbox rows after clear = %d, want 0", got)
	}

	orderID := seedOrderWithOutboxAt(t, db, "after-restore", 0, 101, "2000-01-01 00:00:00")
	notifier := newTestNotifier(t, db, server.URL, []int64{101})
	notifier.process(context.Background())

	status, attempts, sentAt := outboxState(t, db, 101)
	if status != "sent" || attempts != 1 || !sentAt.Valid {
		t.Fatalf("notification after restore = (%q, %d, valid=%v), want (sent, 1, true)", status, attempts, sentAt.Valid)
	}

	messages := fake.messagesFor(101)
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	if !strings.Contains(messages[0].Text, "Новый заказ #"+strconv.FormatInt(orderID, 10)) {
		t.Fatalf("notification after restore does not reference order #%d: %q", orderID, messages[0].Text)
	}
}

func TestStaleHandlerLeavesReusedRowAvailableForRetry(t *testing.T) {
	db := newTestDB(t)
	fake, server := newFakeServer(t)

	oldOrderID := seedOrderWithOutboxAt(t, db, "retry-old", 0, 101, "2000-01-01 00:00:00")
	oldJob := readOutboxJob(t, db, oldOrderID)

	if err := (&orders.Service{DB: db}).DeleteOrder(oldOrderID); err != nil {
		t.Fatalf("delete original order: %v", err)
	}
	newOrderID := seedOrderWithOutboxAt(t, db, "retry-new", oldJob.ID, 101, "2000-01-01 00:00:00")

	notifier := newTestNotifier(t, db, server.URL, []int64{101})

	// The old handler finishes after its row was deleted and a new row reused
	// the id. Its success must not consume the new job.
	notifier.markSent(context.Background(), oldJob)

	status, attempts, sentAt, _ := readOutboxStateByID(t, db, oldJob.ID)
	if status != "pending" || attempts != 0 || sentAt.Valid {
		t.Fatalf("new row after stale success = (%q, %d, valid=%v), want (pending, 0, false)", status, attempts, sentAt.Valid)
	}

	notifier.process(context.Background())

	status, attempts, sentAt = outboxState(t, db, 101)
	if status != "sent" || attempts != 1 || !sentAt.Valid {
		t.Fatalf("new row after retry = (%q, %d, valid=%v), want (sent, 1, true)", status, attempts, sentAt.Valid)
	}

	messages := fake.messagesFor(101)
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	if !strings.Contains(messages[0].Text, "Новый заказ #"+strconv.FormatInt(newOrderID, 10)) {
		t.Fatalf("retry notification does not reference order #%d: %q", newOrderID, messages[0].Text)
	}
}
