package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const defaultPath = "./data/orders.db"

// Path returns the SQLite database path. DB_PATH is optional and is useful for
// tests and for running the service outside the default working directory.
func Path() string {
	if path := os.Getenv("DB_PATH"); path != "" {
		return path
	}

	return defaultPath
}

func DSN(path string) string {
	return "file:" + path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

func Connect() (*sql.DB, error) {
	return ConnectAt(Path())
}

func ConnectAt(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create data directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", DSN(path))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}
