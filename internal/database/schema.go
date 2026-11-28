package database

import (
	"database/sql"
	"fmt"
)

// SchemaVersion is stored in PRAGMA user_version. Backup archives carry the
// same value and the table/column fingerprint, so an archive can only be
// restored into a compatible application schema.
const SchemaVersion = 1

func InitSchema(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS dishes (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL,
		weight_grams INTEGER CHECK(weight_grams > 0),
		price INTEGER NOT NULL CHECK(price >= 0),
		is_available INTEGER NOT NULL DEFAULT 1,
		menu_type TEXT NOT NULL DEFAULT 'home'
			CHECK(menu_type IN ('home', 'banquet', 'both', 'hidden')),
		previous_menu_type TEXT NOT NULL DEFAULT '',
		emoji TEXT NOT NULL DEFAULT '🍽️',
		photo_url TEXT NOT NULL DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS categories (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		name_key TEXT NOT NULL,
		menu_type TEXT NOT NULL
			CHECK(menu_type IN ('home', 'banquet')),
		UNIQUE(menu_type, name_key)
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

	CREATE TABLE IF NOT EXISTS dish_categories (
		id INTEGER PRIMARY KEY,
		dish_id INTEGER NOT NULL,
		category_id INTEGER NOT NULL,
		FOREIGN KEY (dish_id) REFERENCES dishes(id) ON DELETE CASCADE,
		FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE CASCADE,
		UNIQUE(dish_id, category_id)
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

	CREATE TABLE IF NOT EXISTS notification_outbox (
		id INTEGER PRIMARY KEY,
		order_id INTEGER NOT NULL,
		recipient_id INTEGER NOT NULL,
		-- Immutable identity of this exact delivery attempt. A numeric id may
		-- be reused by SQLite after a row is deleted, so completion updates
		-- must match both id and job_token.
		job_token TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending'
			CHECK (status IN ('pending', 'sent', 'failed')),
		attempts INTEGER NOT NULL DEFAULT 0,
		next_attempt_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_error TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		sent_at TEXT,
		FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE,
		UNIQUE(order_id, recipient_id)
	);

	-- Service table. The ID of the last successfully applied restore is written
	-- in the same SQLite transaction as the restored data. It is the source of
	-- truth for crash recovery when the process dies between the SQLite COMMIT
	-- and filesystem journal cleanup.
	CREATE TABLE IF NOT EXISTS restore_state (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		restore_id TEXT NOT NULL,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_dish_ingredients_dish_id
		ON dish_ingredients(dish_id);
	CREATE INDEX IF NOT EXISTS idx_dish_ingredients_ingredient_id
		ON dish_ingredients(ingredient_id);
	CREATE INDEX IF NOT EXISTS idx_order_items_order_id
		ON order_items(order_id);
	CREATE INDEX IF NOT EXISTS idx_order_ingredients_order_item_id
		ON order_ingredients(order_item_id);
	CREATE INDEX IF NOT EXISTS idx_notification_outbox_due
		ON notification_outbox(status, next_attempt_at);
	CREATE INDEX IF NOT EXISTS idx_orders_customer_phone
		ON orders(customer_phone);
	CREATE INDEX IF NOT EXISTS idx_categories_menu_type
		ON categories(menu_type);
	CREATE INDEX IF NOT EXISTS idx_dish_categories_dish_id
		ON dish_categories(dish_id);
	CREATE INDEX IF NOT EXISTS idx_dish_categories_category_id
		ON dish_categories(category_id);
	`

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("initialize database schema: %w", err)
	}

	// CREATE TABLE IF NOT EXISTS не добавляет новые колонки в уже существующую БД.
	if err := ensureColumn(db, "orders", "token", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("add orders.token column: %w", err)
	}
	if err := ensureColumn(db, "dishes", "photo_url", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("add dishes.photo_url column: %w", err)
	}
	if err := ensureColumn(
		db,
		"dishes",
		"menu_type",
		"TEXT NOT NULL DEFAULT 'home' CHECK(menu_type IN ('home', 'banquet', 'both', 'hidden'))",
	); err != nil {
		return fmt.Errorf("add dishes.menu_type column: %w", err)
	}
	if err := ensureColumn(
		db,
		"dishes",
		"previous_menu_type",
		"TEXT NOT NULL DEFAULT ''",
	); err != nil {
		return fmt.Errorf("add dishes.previous_menu_type column: %w", err)
	}
	if err := ensureColumn(
		db,
		"notification_outbox",
		"job_token",
		"TEXT NOT NULL DEFAULT ''",
	); err != nil {
		return fmt.Errorf("add notification_outbox.job_token column: %w", err)
	}

	// Backfill rows created before job_token existed. The trigger gives every
	// future row a non-empty identity even when it is inserted without an
	// explicit token (for example from a migration or a test fixture).
	if _, err := db.Exec(`
		UPDATE notification_outbox
		SET job_token = lower(hex(randomblob(16)))
		WHERE job_token = '';

		CREATE TRIGGER IF NOT EXISTS trg_notification_outbox_job_token
		AFTER INSERT ON notification_outbox
		WHEN NEW.job_token = ''
		BEGIN
			UPDATE notification_outbox
			SET job_token = lower(hex(randomblob(16)))
			WHERE id = NEW.id;
		END;
	`); err != nil {
		return fmt.Errorf("initialize notification job identity: %w", err)
	}

	if err := syncDishVisibility(db); err != nil {
		return err
	}

	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion)); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	return nil
}

// syncDishVisibility keeps the two historical hiding mechanisms consistent:
// a hidden dish is never published, and an unpublished dish is always hidden.
// previous_menu_type stores the menu the dish belonged to before it was hidden.
func syncDishVisibility(db *sql.DB) error {
	_, err := db.Exec(`
		UPDATE dishes
		SET previous_menu_type = menu_type
		WHERE menu_type <> 'hidden'
		  AND previous_menu_type NOT IN ('home', 'banquet', 'both');

		UPDATE dishes
		SET menu_type = 'hidden',
		    previous_menu_type = CASE
		        WHEN previous_menu_type IN ('home', 'banquet', 'both') THEN previous_menu_type
		        WHEN menu_type IN ('home', 'banquet', 'both') THEN menu_type
		        ELSE 'home'
		    END
		WHERE is_available = 0
		  AND menu_type <> 'hidden';

		UPDATE dishes
		SET is_available = 0
		WHERE menu_type = 'hidden';

		UPDATE dishes
		SET previous_menu_type = 'home'
		WHERE menu_type = 'hidden'
		  AND previous_menu_type NOT IN ('home', 'banquet', 'both');
	`)
	if err != nil {
		return fmt.Errorf("sync dish visibility: %w", err)
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
