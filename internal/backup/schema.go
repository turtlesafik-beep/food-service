package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	photoURLPrefix = "/media/dishes/"

	manifestFormat        = "food-service-backup"
	manifestFormatVersion = 1
)

// tableSpec is the allowlist of business tables copied by backup and restore.
// notification_outbox is deliberately absent: it is technical delivery state
// and must never be restored from an old archive. restore_state is service
// state and is also never copied.
type tableSpec struct {
	name    string
	columns []string
}

// businessTables is in parent -> child order, which is also the INSERT order.
var businessTables = []tableSpec{
	{
		name: "dishes",
		columns: []string{
			"id", "name", "description", "weight_grams", "price",
			"is_available", "menu_type", "previous_menu_type", "emoji", "photo_url",
		},
	},
	{
		name:    "categories",
		columns: []string{"id", "name", "name_key", "menu_type"},
	},
	{
		name:    "ingredients",
		columns: []string{"id", "name", "name_key", "unit"},
	},
	{
		name:    "dish_categories",
		columns: []string{"id", "dish_id", "category_id"},
	},
	{
		name:    "dish_ingredients",
		columns: []string{"id", "dish_id", "ingredient_id", "quantity"},
	},
	{
		name: "orders",
		columns: []string{
			"id", "token_hash", "token", "customer_name",
			"customer_phone", "comment", "created_at",
		},
	},
	{
		name: "order_items",
		columns: []string{
			"id", "order_id", "dish_id", "dish_name",
			"weight_grams", "quantity", "unit_price",
		},
	},
	{
		name:    "order_ingredients",
		columns: []string{"id", "order_item_id", "ingredient_name", "unit", "quantity"},
	},
}

func businessTableNames() []string {
	names := make([]string, 0, len(businessTables))
	for _, table := range businessTables {
		names = append(names, table.name)
	}
	return names
}

func tableByName(name string) (tableSpec, bool) {
	for _, table := range businessTables {
		if table.name == name {
			return table, true
		}
	}
	return tableSpec{}, false
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// schemaFingerprint describes every user-data table except restore_state.
// notification_outbox is technical delivery state and is excluded from the
// archive entirely, so it is excluded from the fingerprint too. The archive is
// accepted only when this fingerprint exactly matches the running application.
func schemaFingerprint(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) (string, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT name
		FROM sqlite_master
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
		  AND name <> 'restore_state'
		  AND name <> 'notification_outbox'
		ORDER BY name
	`)
	if err != nil {
		return "", fmt.Errorf("read schema tables: %w", err)
	}

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return "", fmt.Errorf("scan schema table: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", fmt.Errorf("iterate schema tables: %w", err)
	}
	if err := rows.Close(); err != nil {
		return "", fmt.Errorf("close schema tables: %w", err)
	}

	var builder strings.Builder
	for _, table := range tables {
		columns, err := tableColumns(ctx, q, table)
		if err != nil {
			return "", err
		}

		builder.WriteString(table)
		builder.WriteByte('(')
		for i, column := range columns {
			if i > 0 {
				builder.WriteByte(',')
			}
			builder.WriteString(column.name)
			builder.WriteByte(':')
			builder.WriteString(strings.ToUpper(column.typ))
		}
		builder.WriteString(")\n")
	}

	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:]), nil
}

type columnInfo struct {
	name string
	typ  string
}

func tableColumns(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, table string) ([]columnInfo, error) {
	rows, err := q.QueryContext(ctx, "PRAGMA table_info("+quoteIdent(table)+")")
	if err != nil {
		return nil, fmt.Errorf("read columns of %s: %w", table, err)
	}
	defer rows.Close()

	var columns []columnInfo
	for rows.Next() {
		var (
			cid        int
			name       string
			typ        string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultVal, &primaryKey); err != nil {
			return nil, fmt.Errorf("scan column of %s: %w", table, err)
		}
		columns = append(columns, columnInfo{name: name, typ: typ})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate columns of %s: %w", table, err)
	}

	return columns, nil
}

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func countRows(ctx context.Context, q rowQuerier, table string) (int64, error) {
	var count int64
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteIdent(table)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	return count, nil
}

func businessCounts(ctx context.Context, q rowQuerier) (map[string]int64, error) {
	counts := make(map[string]int64, len(businessTables))
	for _, table := range businessTables {
		count, err := countRows(ctx, q, table.name)
		if err != nil {
			return nil, err
		}
		counts[table.name] = count
	}
	return counts, nil
}

func tableExists(ctx context.Context, q rowQuerier, table string) (bool, error) {
	var count int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		table,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check table %s: %w", table, err)
	}
	return count > 0, nil
}

func photoName(photoURL string) (string, bool) {
	if !strings.HasPrefix(photoURL, photoURLPrefix) {
		return "", false
	}

	name := strings.TrimPrefix(photoURL, photoURLPrefix)
	if !validPhotoName(name) {
		return "", false
	}
	return name, true
}

func validPhotoName(name string) bool {
	if name == "" || name != filepath.Base(name) {
		return false
	}
	if strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return false
	}
	if name == "." || name == ".." {
		return false
	}

	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	default:
		return false
	}
}
