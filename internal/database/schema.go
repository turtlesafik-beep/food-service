package database

import (
	"database/sql"
	"fmt"
)

func InitSchema(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS dishes (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL,
		weight_grams INTEGER CHECK(weight_grams > 0),
		price INTEGER NOT NULL CHECK(price >= 0),
		is_available INTEGER NOT NULL DEFAULT 1,
		emoji TEXT NOT NULL DEFAULT '🍽️'
	);

	CREATE TABLE IF NOT EXISTS ingredients (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		name_key TEXT NOT NULL UNIQUE,
		unit TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS dish_ingredients (
		id INTEGER PRIMARY KEY,
		dish_id INTEGER NOT NULL,
		ingredient_id INTEGER NOT NULL,
		quantity REAL NOT NULL CHECK(quantity > 0),
		FOREIGN KEY (dish_id) REFERENCES dishes(id),
		FOREIGN KEY (ingredient_id) REFERENCES ingredients(id),
		UNIQUE(dish_id, ingredient_id)
	);

	CREATE TABLE IF NOT EXISTS orders (
		id INTEGER PRIMARY KEY,
		token_hash TEXT NOT NULL UNIQUE,
		token TEXT NOT NULL DEFAULT '',
		customer_name TEXT NOT NULL DEFAULT '',
		customer_phone TEXT NOT NULL DEFAULT '',
		comment TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS order_items (
		id INTEGER PRIMARY KEY,
		order_id INTEGER NOT NULL,
		dish_id INTEGER,
		dish_name TEXT NOT NULL,
		weight_grams INTEGER,
		quantity REAL NOT NULL CHECK(quantity > 0),
		unit_price INTEGER NOT NULL CHECK(unit_price >= 0),
		FOREIGN KEY (order_id) REFERENCES orders(id),
		FOREIGN KEY (dish_id) REFERENCES dishes(id)
	);

	CREATE TABLE IF NOT EXISTS order_ingredients (
		id INTEGER PRIMARY KEY,
		order_item_id INTEGER NOT NULL,
		ingredient_name TEXT NOT NULL,
		unit TEXT NOT NULL,
		quantity REAL NOT NULL CHECK(quantity > 0),
		FOREIGN KEY (order_item_id) REFERENCES order_items(id)
	);

	CREATE INDEX IF NOT EXISTS idx_dish_ingredients_dish_id
		ON dish_ingredients(dish_id);
	CREATE INDEX IF NOT EXISTS idx_dish_ingredients_ingredient_id
		ON dish_ingredients(ingredient_id);
	CREATE INDEX IF NOT EXISTS idx_order_items_order_id
		ON order_items(order_id);
	CREATE INDEX IF NOT EXISTS idx_order_ingredients_order_item_id
		ON order_ingredients(order_item_id);
	CREATE INDEX IF NOT EXISTS idx_orders_customer_phone
		ON orders(customer_phone);
	`

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("initialize database schema: %w", err)
	}

	// CREATE TABLE IF NOT EXISTS не добавляет новые колонки в уже существующую БД.
	if err := ensureColumn(db, "orders", "token", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("add orders.token column: %w", err)
	}

	return nil
}

func ensureColumn(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			typ        string
			notNull    int
			defaultV   sql.NullString
			primaryKey int
		)

		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultV, &primaryKey); err != nil {
			return err
		}

		if name == column {
			return nil
		}
	}

	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}
