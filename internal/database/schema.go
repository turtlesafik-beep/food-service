
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
	`

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("initialize database schema: %w", err)
	}

	return nil
}

