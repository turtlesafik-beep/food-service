package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"food-service/internal/database"
	"food-service/internal/maintenance"
	"food-service/internal/orders"
)

type testEnv struct {
	service   *Service
	db        *sql.DB
	dbPath    string
	uploadDir string
}

func newTestEnv(t *testing.T, maxArchive int64) *testEnv {
	t.Helper()

	root := t.TempDir()
	dbPath := filepath.Join(root, "data", "orders.db")
	db, err := database.ConnectAt(dbPath)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	if err := database.InitSchema(db); err != nil {
		t.Fatalf("init test schema: %v", err)
	}

	uploadDir := filepath.Join(root, "uploads", "dishes")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatalf("create upload dir: %v", err)
	}

	service, err := NewService(Config{
		DB:              db,
		DBPath:          dbPath,
		UploadDir:       uploadDir,
		TempDir:         filepath.Join(root, "data", "tmp"),
		MaxArchiveBytes: maxArchive,
	}, maintenance.New())
	if err != nil {
		t.Fatalf("create backup service: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	return &testEnv{service: service, db: db, dbPath: dbPath, uploadDir: uploadDir}
}

func seedData(t *testing.T, env *testEnv, photoName string) int64 {
	t.Helper()

	ingredientResult, err := env.db.Exec(
		`INSERT INTO ingredients (name, name_key, unit) VALUES ('Свёкла', 'свёкла', 'g')`,
	)
	if err != nil {
		t.Fatalf("insert ingredient: %v", err)
	}
	ingredientID, _ := ingredientResult.LastInsertId()

	photoURL := ""
	if photoName != "" {
		photoURL = photoURLPrefix + photoName
	}
	dishResult, err := env.db.Exec(
		`INSERT INTO dishes (name, description, weight_grams, price, is_available, menu_type, emoji, photo_url)
		 VALUES ('Борщ', 'Тестовый борщ', 250, 300, 1, 'home', '🍲', ?)`,
		photoURL,
	)
	if err != nil {
		t.Fatalf("insert dish: %v", err)
	}
	dishID, _ := dishResult.LastInsertId()

	if _, err := env.db.Exec(
		`INSERT INTO dish_ingredients (dish_id, ingredient_id, quantity) VALUES (?, ?, 100)`,
		dishID, ingredientID,
	); err != nil {
		t.Fatalf("insert recipe: %v", err)
	}

	categoryResult, err := env.db.Exec(
		`INSERT INTO categories (name, name_key, menu_type) VALUES ('Супы', 'супы', 'home')`,
	)
	if err != nil {
		t.Fatalf("insert category: %v", err)
	}
	categoryID, _ := categoryResult.LastInsertId()
	if _, err := env.db.Exec(
		`INSERT INTO dish_categories (dish_id, category_id) VALUES (?, ?)`,
		dishID, categoryID,
	); err != nil {
		t.Fatalf("insert dish category: %v", err)
	}

	orderResult, err := env.db.Exec(
		`INSERT INTO orders (token_hash, token, customer_name, customer_phone, comment)
		 VALUES ('hash-1', 'token-1', 'Иван', '+79990000000', 'тест')`,
	)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}
	orderID, _ := orderResult.LastInsertId()

	itemResult, err := env.db.Exec(
		`INSERT INTO order_items (order_id, dish_id, dish_name, weight_grams, quantity, unit_price)
		 VALUES (?, ?, 'Борщ', 250, 2, 300)`,
		orderID, dishID,
	)
	if err != nil {
		t.Fatalf("insert order item: %v", err)
	}
	itemID, _ := itemResult.LastInsertId()
	if _, err := env.db.Exec(
		`INSERT INTO order_ingredients (order_item_id, ingredient_name, unit, quantity)
		 VALUES (?, 'Свёкла', 'g', 200)`,
		itemID,
	); err != nil {
		t.Fatalf("insert order ingredient: %v", err)
	}

	if _, err := env.db.Exec(
		`INSERT INTO notification_outbox (order_id, recipient_id, status) VALUES (?, 42, 'sent')`,
		orderID,
	); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}

	return dishID
}

func writePhoto(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatalf("write photo: %v", err)
	}
}

func buildArchive(t *testing.T, service *Service) []byte {
	t.Helper()

	summary, err := service.PrepareBackup()
	if err != nil {
		t.Fatalf("prepare backup: %v", err)
	}
	var buffer bytes.Buffer
	if err := service.WriteArchive(context.Background(), summary.ID, &buffer); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	return buffer.Bytes()
}

func validateArchive(t *testing.T, service *Service, data []byte) restoreSummary {
	t.Helper()
	summary, err := service.ValidateRestore(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("validate archive: %v", err)
	}
	return summary
}

func readArchiveEntry(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		handle, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		defer handle.Close()
		result, err := io.ReadAll(handle)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return result
	}
	t.Fatalf("entry %s not found", name)
	return nil
}

func rewriteZip(t *testing.T, data []byte, replacements map[string][]byte, extra map[string][]byte) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range reader.File {
		if _, skip := replacements[file.Name]; skip {
			continue
		}
		header := &zip.FileHeader{Name: file.Name, Method: file.Method}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("create %s: %v", file.Name, err)
		}
		handle, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		if _, err := io.Copy(entry, handle); err != nil {
			t.Fatalf("copy %s: %v", file.Name, err)
		}
		handle.Close()
	}
	for name, content := range replacements {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create replacement %s: %v", name, err)
		}
		entry.Write(content)
	}
	for name, content := range extra {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create extra %s: %v", name, err)
		}
		entry.Write(content)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return buffer.Bytes()
}

func removeZipEntries(t *testing.T, data []byte, names ...string) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	remove := make(map[string]bool)
	for _, name := range names {
		remove[name] = true
	}

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range reader.File {
		if remove[file.Name] {
			continue
		}
		header := &zip.FileHeader{Name: file.Name, Method: file.Method}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("create %s: %v", file.Name, err)
		}
		handle, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		if _, err := io.Copy(entry, handle); err != nil {
			t.Fatalf("copy %s: %v", file.Name, err)
		}
		handle.Close()
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return buffer.Bytes()
}

func TestBackupSnapshotIsConsistentAndExcludesOutbox(t *testing.T) {
	env := newTestEnv(t, 0)
	seedData(t, env, "photo.jpg")
	writePhoto(t, env.uploadDir, "photo.jpg", []byte("jpeg-data"))

	var journalMode string
	if err := env.db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("read journal mode: %v", err)
	}
	if strings.ToLower(journalMode) != "wal" {
		t.Fatalf("expected WAL, got %s", journalMode)
	}

	archive := buildArchive(t, env.service)

	manifestData := readArchiveEntry(t, archive, "manifest.json")
	var value manifest
	if err := json.Unmarshal(manifestData, &value); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if value.Counts["orders"] != 1 || value.Counts["dishes"] != 1 {
		t.Fatalf("unexpected manifest counts: %#v", value.Counts)
	}
	if len(value.Photos) != 1 || value.Photos[0].Name != "photo.jpg" {
		t.Fatalf("unexpected photos: %#v", value.Photos)
	}

	dbData := readArchiveEntry(t, archive, "database/orders.db")
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	if err := os.WriteFile(dbPath, dbData, 0o600); err != nil {
		t.Fatalf("write archive db: %v", err)
	}
	archiveDB, err := openSQLite(dbPath, false)
	if err != nil {
		t.Fatalf("open archive db: %v", err)
	}
	defer archiveDB.Close()

	outboxExists, err := tableExists(context.Background(), archiveDB, "notification_outbox")
	if err != nil {
		t.Fatalf("check outbox table: %v", err)
	}
	if outboxExists {
		t.Fatalf("notification_outbox table leaked into archive")
	}

	var ordersCount int
	if err := archiveDB.QueryRow(`SELECT COUNT(*) FROM orders`).Scan(&ordersCount); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if ordersCount != 1 {
		t.Fatalf("expected 1 order in archive, got %d", ordersCount)
	}

	names := make(map[string]bool)
	reader, _ := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	for _, file := range reader.File {
		names[file.Name] = true
	}
	for _, forbidden := range []string{".env", "docker-compose.yml", "nginx/default.conf"} {
		if names[forbidden] {
			t.Fatalf("forbidden file %s in archive", forbidden)
		}
	}

	// A snapshot is a point-in-time view; later writes must not change it.
	if _, err := env.db.Exec(`INSERT INTO orders (token_hash, token) VALUES ('hash-2', 'token-2')`); err != nil {
		t.Fatalf("insert later order: %v", err)
	}
}

func TestRestoreRoundTripClearsOutboxAndAllowsNewOrders(t *testing.T) {
	env := newTestEnv(t, 0)
	dishID := seedData(t, env, "photo.jpg")
	writePhoto(t, env.uploadDir, "photo.jpg", []byte("jpeg-data"))

	archive := buildArchive(t, env.service)

	// Change current data and photos after the archive was created.
	if _, err := env.db.Exec(`UPDATE dishes SET name = 'Изменённый борщ' WHERE id = ?`, dishID); err != nil {
		t.Fatalf("update dish: %v", err)
	}
	if _, err := env.db.Exec(`INSERT INTO orders (token_hash, token) VALUES ('hash-current', 'token-current')`); err != nil {
		t.Fatalf("insert current order: %v", err)
	}
	writePhoto(t, env.uploadDir, "current.jpg", []byte("current-photo"))

	summary := validateArchive(t, env.service, archive)
	if summary.Counts["orders"] != 1 || summary.Counts["dishes"] != 1 {
		t.Fatalf("unexpected summary: %#v", summary.Counts)
	}
	if summary.PhotoCount != 1 {
		t.Fatalf("unexpected photo count: %d", summary.PhotoCount)
	}

	backupSummary, err := env.service.PrepareBackup()
	if err != nil {
		t.Fatalf("prepare safety backup: %v", err)
	}
	var safety bytes.Buffer
	if err := env.service.WriteArchive(context.Background(), backupSummary.ID, &safety); err != nil {
		t.Fatalf("write safety backup: %v", err)
	}
	if _, err := env.service.ConfirmBackup(backupSummary.ID); err != nil {
		t.Fatalf("confirm safety backup: %v", err)
	}

	if _, err := env.service.ApplyRestore(
		context.Background(), summary.ID, "ЗАМЕНИТЬ ДАННЫЕ", backupSummary.ID,
	); err != nil {
		t.Fatalf("apply restore: %v", err)
	}

	var name string
	if err := env.db.QueryRow(`SELECT name FROM dishes WHERE id = ?`, dishID).Scan(&name); err != nil {
		t.Fatalf("read restored dish: %v", err)
	}
	if name != "Борщ" {
		t.Fatalf("dish was not restored: %q", name)
	}

	var ordersCount, outboxCount int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM orders`).Scan(&ordersCount); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox`).Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if ordersCount != 1 {
		t.Fatalf("expected restored order count 1, got %d", ordersCount)
	}
	if outboxCount != 0 {
		t.Fatalf("expected outbox to be cleared, got %d", outboxCount)
	}

	entries, err := os.ReadDir(env.uploadDir)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "photo.jpg" {
		t.Fatalf("photos were not replaced: %#v", entries)
	}

	// The application must continue to work and create outbox rows for new
	// orders after a restore.
	orderService := &orders.Service{DB: env.db}
	notifier := &testNotifier{db: env.db}
	orderService.Notifications = notifier
	if _, err := orderService.CreateOrder(orders.CreateOrderRequest{
		CustomerName:  "После восстановления",
		CustomerPhone: "+79991112233",
		Items:         []orders.OrderItemInput{{DishID: dishID, Quantity: 1}},
	}); err != nil {
		t.Fatalf("create order after restore: %v", err)
	}
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox`).Scan(&outboxCount); err != nil {
		t.Fatalf("count new outbox: %v", err)
	}
	if outboxCount == 0 {
		t.Fatalf("new order did not create a notification")
	}
}

type testNotifier struct {
	db *sql.DB
}

func (n *testNotifier) EnqueueInTx(tx *sql.Tx, orderID int64) error {
	_, err := tx.Exec(
		`INSERT INTO notification_outbox (order_id, recipient_id) VALUES (?, 42)`,
		orderID,
	)
	return err
}

func (n *testNotifier) Wake() {}

func TestRejectsCorruptArchive(t *testing.T) {
	env := newTestEnv(t, 0)
	_, err := env.service.ValidateRestore(context.Background(), strings.NewReader("not a zip"), 9)
	if !errors.Is(err, ErrBadArchive) {
		t.Fatalf("expected ErrBadArchive, got %v", err)
	}
}

func TestRejectsCorruptSQLite(t *testing.T) {
	env := newTestEnv(t, 0)
	seedData(t, env, "")
	archive := buildArchive(t, env.service)

	corrupt := rewriteZip(t, archive, map[string][]byte{
		"database/orders.db": []byte("definitely not sqlite"),
	}, nil)

	_, err := env.service.ValidateRestore(context.Background(), bytes.NewReader(corrupt), int64(len(corrupt)))
	if err == nil {
		t.Fatal("expected corrupt database to be rejected")
	}
}

func TestRejectsMissingReferencedPhoto(t *testing.T) {
	env := newTestEnv(t, 0)
	seedData(t, env, "missing.jpg")
	writePhoto(t, env.uploadDir, "missing.jpg", []byte("photo"))
	archive := buildArchive(t, env.service)
	archive = removeZipEntries(t, archive, "uploads/dishes/missing.jpg")

	_, err := env.service.ValidateRestore(context.Background(), bytes.NewReader(archive), int64(len(archive)))
	if !errors.Is(err, ErrInvalidData) && !errors.Is(err, ErrBadArchive) {
		t.Fatalf("expected missing photo rejection, got %v", err)
	}
}

func TestRejectsPathTraversalAndOversizedArchive(t *testing.T) {
	env := newTestEnv(t, 0)
	seedData(t, env, "")
	archive := buildArchive(t, env.service)

	traversal := rewriteZip(t, archive, nil, map[string][]byte{
		"../evil.jpg": []byte("evil"),
	})
	if _, err := env.service.ValidateRestore(context.Background(), bytes.NewReader(traversal), int64(len(traversal))); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("expected path traversal rejection, got %v", err)
	}

	env.service.cfg.MaxArchiveBytes = 32
	_, err := env.service.ValidateRestore(context.Background(), bytes.NewReader(archive), int64(len(archive)))
	if !errors.Is(err, ErrArchiveTooBig) {
		t.Fatalf("expected archive size rejection, got %v", err)
	}
}

func TestRequiresPhraseAndConfirmedSafetyBackup(t *testing.T) {
	env := newTestEnv(t, 0)
	seedData(t, env, "")
	archive := buildArchive(t, env.service)
	summary := validateArchive(t, env.service, archive)

	if _, err := env.service.ApplyRestore(context.Background(), summary.ID, "заменить данные", "none"); !errors.Is(err, ErrBadPhrase) {
		t.Fatalf("expected bad phrase rejection, got %v", err)
	}
	if _, err := env.service.ApplyRestore(context.Background(), summary.ID, "ЗАМЕНИТЬ ДАННЫЕ", "none"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected missing backup rejection, got %v", err)
	}

	backupSummary, err := env.service.PrepareBackup()
	if err != nil {
		t.Fatalf("prepare backup: %v", err)
	}
	var buffer bytes.Buffer
	if err := env.service.WriteArchive(context.Background(), backupSummary.ID, &buffer); err != nil {
		t.Fatalf("write backup: %v", err)
	}
	if _, err := env.service.ApplyRestore(context.Background(), summary.ID, "ЗАМЕНИТЬ ДАННЫЕ", backupSummary.ID); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("expected confirmation rejection, got %v", err)
	}
}

func prepareCrashState(t *testing.T, env *testEnv, archive []byte) (*restoreSession, restoreJournal) {
	t.Helper()

	summary := validateArchive(t, env.service, archive)
	session, err := env.service.restoreSession(summary.ID)
	if err != nil {
		t.Fatalf("get restore session: %v", err)
	}

	markerName := ".restore-complete-" + session.ID
	if err := writeMarker(filepath.Join(session.PhotosDir, markerName), session.ID); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	journal := restoreJournal{
		Version:     journalVersion,
		OpID:        session.ID,
		CreatedAt:   time.Now().UTC(),
		LiveDir:     env.uploadDir,
		RollbackDir: filepath.Join(filepath.Dir(env.uploadDir), ".rollback-"+session.ID),
		StagedDir:   session.PhotosDir,
		TempRoot:    session.TempRoot,
		PhotosRoot:  session.PhotosRoot,
		MarkerName:  markerName,
		LiveExisted: true,
	}
	if err := writeJournal(env.service.journalPath(), journal); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	return session, journal
}

func TestRecoverAfterCommitBeforeJournalCleanup(t *testing.T) {
	env := newTestEnv(t, 0)
	seedData(t, env, "new.jpg")
	writePhoto(t, env.uploadDir, "new.jpg", []byte("new-photo"))
	archive := buildArchive(t, env.service)

	// Replace the live photo with an old one before the simulated restore.
	os.Remove(filepath.Join(env.uploadDir, "new.jpg"))
	writePhoto(t, env.uploadDir, "old.jpg", []byte("old-photo"))

	session, journal := prepareCrashState(t, env, archive)
	if err := os.Rename(journal.LiveDir, journal.RollbackDir); err != nil {
		t.Fatalf("rename live: %v", err)
	}
	if err := os.Rename(journal.StagedDir, journal.LiveDir); err != nil {
		t.Fatalf("install staged: %v", err)
	}
	if err := env.service.replaceDatabase(context.Background(), session.DBPath, session.ID); err != nil {
		t.Fatalf("commit database: %v", err)
	}

	// Simulate a crash exactly after COMMIT: journal and rollback still exist.
	if err := env.service.RecoverOnStartup(context.Background()); err != nil {
		t.Fatalf("recover: %v", err)
	}

	entries, err := os.ReadDir(env.uploadDir)
	if err != nil {
		t.Fatalf("read photos: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "new.jpg" {
		t.Fatalf("committed photos were rolled back: %#v", entries)
	}
	if _, err := os.Stat(journal.RollbackDir); !os.IsNotExist(err) {
		t.Fatalf("rollback directory was not cleaned")
	}
	if _, err := os.Stat(env.service.journalPath()); !os.IsNotExist(err) {
		t.Fatalf("journal was not removed")
	}
}

func TestRecoverBeforeCommitRollsBackPhotos(t *testing.T) {
	env := newTestEnv(t, 0)
	seedData(t, env, "new.jpg")
	writePhoto(t, env.uploadDir, "new.jpg", []byte("new-photo"))
	archive := buildArchive(t, env.service)

	os.Remove(filepath.Join(env.uploadDir, "new.jpg"))
	writePhoto(t, env.uploadDir, "old.jpg", []byte("old-photo"))

	_, journal := prepareCrashState(t, env, archive)
	if err := os.Rename(journal.LiveDir, journal.RollbackDir); err != nil {
		t.Fatalf("rename live: %v", err)
	}
	if err := os.Rename(journal.StagedDir, journal.LiveDir); err != nil {
		t.Fatalf("install staged: %v", err)
	}

	// No database COMMIT happens here.
	if err := env.service.RecoverOnStartup(context.Background()); err != nil {
		t.Fatalf("recover: %v", err)
	}

	entries, err := os.ReadDir(env.uploadDir)
	if err != nil {
		t.Fatalf("read photos: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "old.jpg" {
		t.Fatalf("old photos were not restored: %#v", entries)
	}
}

func TestRecoverBeforePhotoRenameKeepsOldPhotos(t *testing.T) {
	env := newTestEnv(t, 0)
	seedData(t, env, "new.jpg")
	writePhoto(t, env.uploadDir, "new.jpg", []byte("new-photo"))
	archive := buildArchive(t, env.service)

	os.Remove(filepath.Join(env.uploadDir, "new.jpg"))
	writePhoto(t, env.uploadDir, "old.jpg", []byte("old-photo"))

	_, journal := prepareCrashState(t, env, archive)
	// Simulate a crash immediately after writing the journal.
	if err := env.service.RecoverOnStartup(context.Background()); err != nil {
		t.Fatalf("recover: %v", err)
	}

	entries, err := os.ReadDir(env.uploadDir)
	if err != nil {
		t.Fatalf("read photos: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "old.jpg" {
		t.Fatalf("old photos were changed: %#v", entries)
	}
	if _, err := os.Stat(journal.StagedDir); !os.IsNotExist(err) {
		t.Fatalf("staged photos were not cleaned")
	}
}

func TestBackupSnapshotWhileWALWritesContinue(t *testing.T) {
	env := newTestEnv(t, 0)
	seedData(t, env, "")

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			_, _ = env.db.Exec(
				`INSERT INTO orders (token_hash, token) VALUES (?, ?)`,
				fmt.Sprintf("concurrent-%d", i),
				fmt.Sprintf("token-%d", i),
			)
		}
	}()

	archive := buildArchive(t, env.service)
	close(stop)
	<-done

	summary := validateArchive(t, env.service, archive)
	if summary.Counts["orders"] < 1 {
		t.Fatalf("snapshot lost the seeded order: %#v", summary.Counts)
	}
}

func TestStartupCleansRollbackForAppliedRestoreWithoutJournal(t *testing.T) {
	env := newTestEnv(t, 0)

	applied := strings.Repeat("a", 32)
	if _, err := env.db.Exec(
		`INSERT INTO restore_state (id, restore_id) VALUES (1, ?)`,
		applied,
	); err != nil {
		t.Fatalf("write restore state: %v", err)
	}

	parent := filepath.Dir(env.uploadDir)
	rollbackDir := filepath.Join(parent, ".rollback-"+applied)
	if err := os.MkdirAll(rollbackDir, 0o755); err != nil {
		t.Fatalf("create rollback dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rollbackDir, "photo.jpg"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write rollback photo: %v", err)
	}

	// No journal: this is the leftover after a successful restore whose
	// rollback cleanup failed.
	if err := env.service.RecoverOnStartup(context.Background()); err != nil {
		t.Fatalf("recover: %v", err)
	}
	if _, err := os.Stat(rollbackDir); !os.IsNotExist(err) {
		t.Fatalf("rollback directory of applied restore was not removed")
	}
}

func TestStartupKeepsRollbackForOtherOrUnknownRestore(t *testing.T) {
	env := newTestEnv(t, 0)

	if _, err := env.db.Exec(
		`INSERT INTO restore_state (id, restore_id) VALUES (1, 'applied-operation')`,
	); err != nil {
		t.Fatalf("write restore state: %v", err)
	}

	parent := filepath.Dir(env.uploadDir)
	otherRollback := filepath.Join(parent, ".rollback-other-operation")
	if err := os.MkdirAll(otherRollback, 0o755); err != nil {
		t.Fatalf("create other rollback dir: %v", err)
	}

	if err := env.service.RecoverOnStartup(context.Background()); err != nil {
		t.Fatalf("recover: %v", err)
	}
	if _, err := os.Stat(otherRollback); err != nil {
		t.Fatalf("rollback of another operation must be kept: %v", err)
	}

	// If restore_state cannot be read, no rollback directory may be removed.
	if _, err := env.db.Exec(`DROP TABLE restore_state`); err != nil {
		t.Fatalf("drop restore state: %v", err)
	}
	unknownRollback := filepath.Join(parent, ".rollback-unknown-operation")
	if err := os.MkdirAll(unknownRollback, 0o755); err != nil {
		t.Fatalf("create unknown rollback dir: %v", err)
	}

	if err := env.service.RecoverOnStartup(context.Background()); err != nil {
		t.Fatalf("recover with unreadable state: %v", err)
	}
	if _, err := os.Stat(unknownRollback); err != nil {
		t.Fatalf("rollback must be kept when restore state is unknown: %v", err)
	}
}
