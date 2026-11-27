package orders

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"food-service/internal/database"
)

func newTestService(t *testing.T) *Service {
	t.Helper()

	db, err := sql.Open(
		"sqlite",
		"file:"+filepath.Join(t.TempDir(), "test.db")+"?_pragma=foreign_keys(1)",
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

	return &Service{DB: db}
}

func seedOrder(t *testing.T, service *Service, name string) (int64, int64) {
	t.Helper()

	result, err := service.DB.Exec(
		`INSERT INTO orders (token_hash, token, customer_name, customer_phone)
		 VALUES (?, ?, ?, ?)`,
		name, name, name, "+70000000000",
	)
	if err != nil {
		t.Fatalf("insert order %q: %v", name, err)
	}

	orderID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get order ID: %v", err)
	}

	result, err = service.DB.Exec(
		`INSERT INTO order_items (order_id, dish_name, quantity, unit_price)
		 VALUES (?, ?, ?, ?)`,
		orderID, "Test dish", 1, 100,
	)
	if err != nil {
		t.Fatalf("insert order item: %v", err)
	}

	itemID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get order item ID: %v", err)
	}

	if _, err := service.DB.Exec(
		`INSERT INTO order_ingredients (order_item_id, ingredient_name, unit, quantity)
		 VALUES (?, ?, ?, ?)`,
		itemID, "Test ingredient", "g", 10,
	); err != nil {
		t.Fatalf("insert order ingredient: %v", err)
	}

	return orderID, itemID
}

func countRows(t *testing.T, service *Service, query string, args ...any) int {
	t.Helper()

	var count int
	if err := service.DB.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}

	return count
}

func TestDeleteOrderRemovesOrderAndRelatedRows(t *testing.T) {
	service := newTestService(t)

	orderID, itemID := seedOrder(t, service, "delete-me")
	otherOrderID, otherItemID := seedOrder(t, service, "keep-me")

	if err := service.DeleteOrder(orderID); err != nil {
		t.Fatalf("delete order: %v", err)
	}

	if got := countRows(t, service, `SELECT COUNT(*) FROM orders WHERE id = ?`, orderID); got != 0 {
		t.Errorf("deleted order rows = %d, want 0", got)
	}
	if got := countRows(t, service, `SELECT COUNT(*) FROM order_items WHERE id = ?`, itemID); got != 0 {
		t.Errorf("deleted order item rows = %d, want 0", got)
	}
	if got := countRows(t, service, `SELECT COUNT(*) FROM order_ingredients WHERE order_item_id = ?`, itemID); got != 0 {
		t.Errorf("deleted order ingredient rows = %d, want 0", got)
	}

	if got := countRows(t, service, `SELECT COUNT(*) FROM orders WHERE id = ?`, otherOrderID); got != 1 {
		t.Errorf("other order rows = %d, want 1", got)
	}
	if got := countRows(t, service, `SELECT COUNT(*) FROM order_items WHERE id = ?`, otherItemID); got != 1 {
		t.Errorf("other order item rows = %d, want 1", got)
	}
	if got := countRows(t, service, `SELECT COUNT(*) FROM order_ingredients WHERE order_item_id = ?`, otherItemID); got != 1 {
		t.Errorf("other order ingredient rows = %d, want 1", got)
	}
}

func TestDeleteOrderNotFound(t *testing.T) {
	service := newTestService(t)

	err := service.DeleteOrder(9999)
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("delete missing order error = %v, want ErrOrderNotFound", err)
	}
}
