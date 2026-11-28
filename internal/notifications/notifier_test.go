package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"food-service/internal/database"
	"food-service/internal/orders"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open(
		"sqlite",
		"file:"+filepath.Join(t.TempDir(), "test.db")+
			"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)",
	)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})

	if err := database.InitSchema(db); err != nil {
		t.Fatalf("init test schema: %v", err)
	}

	return db
}

func seedDish(t *testing.T, db *sql.DB) int64 {
	t.Helper()

	result, err := db.Exec(
		`INSERT INTO dishes (name, description, weight_grams, price, is_available, menu_type)
		 VALUES ('Борщ', 'Тестовый борщ', 250, 300, 1, 'home')`,
	)
	if err != nil {
		t.Fatalf("insert dish: %v", err)
	}
	dishID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get dish ID: %v", err)
	}

	result, err = db.Exec(
		`INSERT INTO ingredients (name, name_key, unit) VALUES ('Свёкла', 'свёкла', 'g')`,
	)
	if err != nil {
		t.Fatalf("insert ingredient: %v", err)
	}
	ingredientID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get ingredient ID: %v", err)
	}

	if _, err := db.Exec(
		`INSERT INTO dish_ingredients (dish_id, ingredient_id, quantity) VALUES (?, ?, ?)`,
		dishID,
		ingredientID,
		100,
	); err != nil {
		t.Fatalf("insert dish ingredient: %v", err)
	}

	return dishID
}

func testOrderRequest(dishID int64) orders.CreateOrderRequest {
	return orders.CreateOrderRequest{
		CustomerName:  "Илья",
		CustomerPhone: "+70000000000",
		Comment:       "",
		Items: []orders.OrderItemInput{
			{DishID: dishID, Quantity: 2},
		},
	}
}

func seedOrderWithOutbox(t *testing.T, db *sql.DB, recipients []int64) int64 {
	t.Helper()

	result, err := db.Exec(
		`INSERT INTO orders (token_hash, token, customer_name, customer_phone, comment)
		 VALUES ('hash', 'token', 'Илья', '+70000000000', 'Без лука')`,
	)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}
	orderID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get order ID: %v", err)
	}

	result, err = db.Exec(
		`INSERT INTO order_items (order_id, dish_name, weight_grams, quantity, unit_price)
		 VALUES (?, 'Борщ', 250, 2, 300)`,
		orderID,
	)
	if err != nil {
		t.Fatalf("insert order item: %v", err)
	}
	itemID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get order item ID: %v", err)
	}

	if _, err := db.Exec(
		`INSERT INTO order_ingredients (order_item_id, ingredient_name, unit, quantity)
		 VALUES (?, 'Свёкла', 'g', 200)`,
		itemID,
	); err != nil {
		t.Fatalf("insert order ingredient: %v", err)
	}

	for _, recipientID := range recipients {
		if _, err := db.Exec(
			`INSERT INTO notification_outbox (order_id, recipient_id) VALUES (?, ?)`,
			orderID,
			recipientID,
		); err != nil {
			t.Fatalf("insert outbox row: %v", err)
		}
	}

	return orderID
}

type sentMessage struct {
	ChatID int64
	Text   string
}

type fakeTelegram struct {
	mu         sync.Mutex
	sends      []sentMessage
	failures   map[int64]int
	failCode   int
	retryAfter int
}

func (f *fakeTelegram) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/sendMessage"):
		var request sendMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}

		f.mu.Lock()
		f.sends = append(f.sends, sentMessage{ChatID: request.ChatID, Text: request.Text})
		remaining := f.failures[request.ChatID]
		if remaining > 0 {
			f.failures[request.ChatID] = remaining - 1
		}
		code := f.failCode
		if code == 0 {
			code = http.StatusInternalServerError
		}
		retryAfter := f.retryAfter
		f.mu.Unlock()

		if remaining > 0 {
			w.WriteHeader(code)
			response := apiResponse{
				OK:          false,
				ErrorCode:   code,
				Description: "injected failure",
			}
			if retryAfter > 0 {
				response.Parameters = &struct {
					RetryAfter int `json:"retry_after"`
				}{RetryAfter: retryAfter}
			}
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		_ = json.NewEncoder(w).Encode(apiResponse{OK: true})

	case strings.HasSuffix(r.URL.Path, "/getMe"):
		_ = json.NewEncoder(w).Encode(apiResponse{
			OK:     true,
			Result: json.RawMessage(`{"username":"test_bot"}`),
		})

	case strings.HasSuffix(r.URL.Path, "/getUpdates"):
		_ = json.NewEncoder(w).Encode(apiResponse{
			OK:     true,
			Result: json.RawMessage(`[]`),
		})

	default:
		http.NotFound(w, r)
	}
}

func (f *fakeTelegram) messagesFor(chatID int64) []sentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()

	var messages []sentMessage
	for _, message := range f.sends {
		if message.ChatID == chatID {
			messages = append(messages, message)
		}
	}

	return messages
}

func newFakeServer(t *testing.T) (*fakeTelegram, *httptest.Server) {
	t.Helper()

	fake := &fakeTelegram{failures: make(map[int64]int)}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	return fake, server
}

func newTestNotifier(
	t *testing.T,
	db *sql.DB,
	serverURL string,
	recipients []int64,
) *Notifier {
	t.Helper()

	return &Notifier{
		db:            db,
		orders:        &orders.Service{DB: db},
		client:        &Client{token: "TEST_TOKEN", baseURL: serverURL, httpClient: &http.Client{}},
		recipients:    recipients,
		publicBaseURL: "https://example.test",
		logger:        log.New(io.Discard, "", 0),
		wake:          make(chan struct{}, 1),
		interval:      time.Second,
		now:           time.Now,
	}
}

func outboxState(t *testing.T, db *sql.DB, recipientID int64) (string, int, sql.NullString) {
	t.Helper()

	var (
		status   string
		attempts int
		sentAt   sql.NullString
	)
	if err := db.QueryRow(
		`SELECT status, attempts, sent_at FROM notification_outbox WHERE recipient_id = ?`,
		recipientID,
	).Scan(&status, &attempts, &sentAt); err != nil {
		t.Fatalf("read outbox row for %d: %v", recipientID, err)
	}

	return status, attempts, sentAt
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()

	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}

	return count
}

type failingNotifications struct{}

func (failingNotifications) EnqueueInTx(*sql.Tx, int64) error {
	return errors.New("injected outbox failure")
}

func (failingNotifications) Wake() {}

func TestEnqueueInTxRollsBackWithTransaction(t *testing.T) {
	db := newTestDB(t)
	_, server := newFakeServer(t)
	orderID := seedOrderWithOutbox(t, db, nil)

	notifier := newTestNotifier(t, db, server.URL, []int64{101})

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	if err := notifier.EnqueueInTx(tx, orderID); err != nil {
		t.Fatalf("enqueue in transaction: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback transaction: %v", err)
	}

	if got := countRows(t, db, `SELECT COUNT(*) FROM notification_outbox`); got != 0 {
		t.Fatalf("outbox rows after rollback = %d, want 0", got)
	}
}

func TestCreateOrderEnqueuesOutboxInTransaction(t *testing.T) {
	db := newTestDB(t)
	dishID := seedDish(t, db)

	fake, server := newFakeServer(t)
	notifier := newTestNotifier(t, db, server.URL, []int64{101, 202})
	service := &orders.Service{DB: db, Notifications: notifier}

	result, err := service.CreateOrder(testOrderRequest(dishID))
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	if got := countRows(t, db, `SELECT COUNT(*) FROM notification_outbox WHERE order_id = ?`, result.OrderID); got != 2 {
		t.Fatalf("outbox rows = %d, want 2", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM notification_outbox WHERE order_id = ? AND status = 'pending'`, result.OrderID); got != 2 {
		t.Fatalf("pending outbox rows = %d, want 2", got)
	}
	if len(fake.messagesFor(101)) != 0 || len(fake.messagesFor(202)) != 0 {
		t.Fatal("CreateOrder must not send Telegram messages synchronously")
	}
}

func TestCreateOrderRollsBackWhenOutboxFails(t *testing.T) {
	db := newTestDB(t)
	dishID := seedDish(t, db)

	service := &orders.Service{DB: db, Notifications: failingNotifications{}}

	if _, err := service.CreateOrder(testOrderRequest(dishID)); err == nil {
		t.Fatal("expected CreateOrder to fail when outbox insert fails")
	}

	if got := countRows(t, db, `SELECT COUNT(*) FROM orders`); got != 0 {
		t.Fatalf("orders = %d, want 0", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM order_items`); got != 0 {
		t.Fatalf("order_items = %d, want 0", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM order_ingredients`); got != 0 {
		t.Fatalf("order_ingredients = %d, want 0", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM notification_outbox`); got != 0 {
		t.Fatalf("notification_outbox = %d, want 0", got)
	}
}

func TestNotifierProcessesPendingTasksAfterRestart(t *testing.T) {
	db := newTestDB(t)
	fake, server := newFakeServer(t)
	orderID := seedOrderWithOutbox(t, db, []int64{101})

	// A new Notifier represents a fresh process start: it must pick up rows
	// that were committed before the restart.
	notifier := newTestNotifier(t, db, server.URL, []int64{101})
	notifier.process(context.Background())

	status, attempts, sentAt := outboxState(t, db, 101)
	if status != "sent" {
		t.Fatalf("status = %q, want sent", status)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if !sentAt.Valid {
		t.Fatal("sent_at must be set")
	}

	messages := fake.messagesFor(101)
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	if !strings.Contains(messages[0].Text, "Новый заказ #"+strconv.FormatInt(orderID, 10)) {
		t.Fatalf("message does not contain order number: %q", messages[0].Text)
	}
}

func TestNotifierKeepsRecipientsIndependent(t *testing.T) {
	db := newTestDB(t)
	fake, server := newFakeServer(t)
	seedOrderWithOutbox(t, db, []int64{101, 202})

	clock := time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)
	fake.failures[101] = 1

	notifier := newTestNotifier(t, db, server.URL, []int64{101, 202})
	notifier.now = func() time.Time { return clock }
	notifier.process(context.Background())

	status, attempts, _ := outboxState(t, db, 101)
	if status != "pending" || attempts != 1 {
		t.Fatalf("recipient 101 = (%q, %d), want (pending, 1)", status, attempts)
	}
	status, attempts, _ = outboxState(t, db, 202)
	if status != "sent" || attempts != 1 {
		t.Fatalf("recipient 202 = (%q, %d), want (sent, 1)", status, attempts)
	}

	clock = clock.Add(time.Hour)
	notifier.process(context.Background())

	status, attempts, _ = outboxState(t, db, 101)
	if status != "sent" || attempts != 2 {
		t.Fatalf("recipient 101 after retry = (%q, %d), want (sent, 2)", status, attempts)
	}
	if got := len(fake.messagesFor(101)); got != 2 {
		t.Fatalf("recipient 101 messages = %d, want 2", got)
	}
	if got := len(fake.messagesFor(202)); got != 1 {
		t.Fatalf("recipient 202 messages = %d, want 1 (must not resend)", got)
	}
}

func TestNotifierRetriesTemporaryFailure(t *testing.T) {
	db := newTestDB(t)
	fake, server := newFakeServer(t)
	seedOrderWithOutbox(t, db, []int64{101})
	fake.failures[101] = 1

	clock := time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)
	notifier := newTestNotifier(t, db, server.URL, []int64{101})
	notifier.now = func() time.Time { return clock }

	notifier.process(context.Background())

	status, attempts, _ := outboxState(t, db, 101)
	if status != "pending" || attempts != 1 {
		t.Fatalf("after failure = (%q, %d), want (pending, 1)", status, attempts)
	}

	clock = clock.Add(31 * time.Second)
	notifier.process(context.Background())

	status, attempts, sentAt := outboxState(t, db, 101)
	if status != "sent" || attempts != 2 || !sentAt.Valid {
		t.Fatalf("after retry = (%q, %d, valid=%v), want (sent, 2, true)", status, attempts, sentAt.Valid)
	}
}

func TestNotifierHonorsRetryAfter(t *testing.T) {
	db := newTestDB(t)
	fake, server := newFakeServer(t)
	seedOrderWithOutbox(t, db, []int64{101})
	fake.failures[101] = 1
	fake.failCode = http.StatusTooManyRequests
	fake.retryAfter = 120

	clock := time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)
	notifier := newTestNotifier(t, db, server.URL, []int64{101})
	notifier.now = func() time.Time { return clock }

	notifier.process(context.Background())

	status, attempts, _ := outboxState(t, db, 101)
	if status != "pending" || attempts != 1 {
		t.Fatalf("after 429 = (%q, %d), want (pending, 1)", status, attempts)
	}

	clock = clock.Add(90 * time.Second)
	notifier.process(context.Background())
	if got := len(fake.messagesFor(101)); got != 1 {
		t.Fatalf("messages at +90s = %d, want 1 (retry_after not respected)", got)
	}

	clock = clock.Add(31 * time.Second)
	notifier.process(context.Background())

	status, attempts, _ = outboxState(t, db, 101)
	if status != "sent" || attempts != 2 {
		t.Fatalf("after retry_after = (%q, %d), want (sent, 2)", status, attempts)
	}
}

func TestNotifierPermanentFailureStopsRetrying(t *testing.T) {
	db := newTestDB(t)
	fake, server := newFakeServer(t)
	seedOrderWithOutbox(t, db, []int64{101})
	fake.failures[101] = 10
	fake.failCode = http.StatusBadRequest

	clock := time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)
	notifier := newTestNotifier(t, db, server.URL, []int64{101})
	notifier.now = func() time.Time { return clock }

	notifier.process(context.Background())

	status, attempts, _ := outboxState(t, db, 101)
	if status != "failed" || attempts != 1 {
		t.Fatalf("after 400 = (%q, %d), want (failed, 1)", status, attempts)
	}

	clock = clock.Add(24 * time.Hour)
	notifier.process(context.Background())

	if got := len(fake.messagesFor(101)); got != 1 {
		t.Fatalf("messages after permanent failure = %d, want 1", got)
	}
}

func TestDeleteOrderRemovesPendingNotifications(t *testing.T) {
	db := newTestDB(t)
	orderID := seedOrderWithOutbox(t, db, []int64{101, 202})

	service := &orders.Service{DB: db}
	if err := service.DeleteOrder(orderID); err != nil {
		t.Fatalf("delete order: %v", err)
	}

	if got := countRows(t, db, `SELECT COUNT(*) FROM notification_outbox`); got != 0 {
		t.Fatalf("outbox rows after delete = %d, want 0", got)
	}
}
