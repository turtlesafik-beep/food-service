package backup

import (
	"context"
	"testing"
)

func TestBusinessTableAllowlistCoversSchema(t *testing.T) {
	env := newTestEnv(t, 0)
	ctx := context.Background()

	rows, err := env.db.QueryContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`)
	if err != nil {
		t.Fatalf("read tables: %v", err)
	}
	defer rows.Close()

	known := make(map[string]bool)
	for _, table := range businessTables {
		known[table.name] = true
	}
	known["notification_outbox"] = true
	known["restore_state"] = true

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		if !known[name] {
			t.Fatalf("table %s is missing from backup allowlist", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}
}
