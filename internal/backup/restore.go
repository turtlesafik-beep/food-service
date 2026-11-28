package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"food-service/internal/database"
)

const journalVersion = 1

var (
	errRestoreCommitUnknown  = errors.New("restore commit result is unknown")
	errInvalidRestoreJournal = errors.New("restore journal is invalid")
)

type restoreJournal struct {
	Version     int       `json:"version"`
	OpID        string    `json:"operation_id"`
	CreatedAt   time.Time `json:"created_at"`
	LiveDir     string    `json:"live_dir"`
	RollbackDir string    `json:"rollback_dir"`
	StagedDir   string    `json:"staged_dir"`
	TempRoot    string    `json:"temp_root"`
	PhotosRoot  string    `json:"photos_root"`
	MarkerName  string    `json:"marker_name"`
	LiveExisted bool      `json:"live_existed"`
}

func (s *Service) journalPath() string {
	return filepath.Join(filepath.Dir(s.cfg.DBPath), "restore-journal.json")
}

// validateRestoreJournal accepts only journals whose paths exactly match the
// directories this process would create for the recorded operation. The
// journal is a plain file on disk, so startup recovery must never trust its
// paths for RemoveAll or Rename.
func (s *Service) validateRestoreJournal(journal restoreJournal) error {
	fail := func(format string, args ...any) error {
		return fmt.Errorf(
			"%w: %s; автоматическое восстановление остановлено, данные сохранены для ручного восстановления",
			errInvalidRestoreJournal,
			fmt.Sprintf(format, args...),
		)
	}

	if journal.Version != journalVersion {
		return fail("unsupported version %d", journal.Version)
	}
	if !validOperationID(journal.OpID) {
		return fail("invalid operation id %q", journal.OpID)
	}

	photoParent := filepath.Dir(s.cfg.UploadDir)
	photoRoot := filepath.Join(photoParent, ".restore-"+journal.OpID)
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"live_dir", journal.LiveDir, s.cfg.UploadDir},
		{"rollback_dir", journal.RollbackDir, filepath.Join(photoParent, ".rollback-"+journal.OpID)},
		{"staged_dir", journal.StagedDir, filepath.Join(photoRoot, "dishes")},
		{"temp_root", journal.TempRoot, filepath.Join(s.cfg.TempDir, "restore-"+journal.OpID)},
		{"photos_root", journal.PhotosRoot, photoRoot},
		{"marker_name", journal.MarkerName, ".restore-complete-" + journal.OpID},
	}

	for _, check := range checks {
		if check.name == "marker_name" {
			if check.got != check.want {
				return fail("unexpected marker name %q", check.got)
			}
			continue
		}
		if check.got == "" || !samePath(check.got, check.want) {
			return fail("unexpected %s %q", check.name, check.got)
		}
	}

	return nil
}

func samePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func validOperationID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, char := range id {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

// ApplyRestore performs the only operation that changes user data. Validation
// was completed earlier; this method briefly stops all requests and swaps both
// SQLite contents and the photo directory.
func (s *Service) ApplyRestore(ctx context.Context, restoreID, confirmation, safetyBackupID string) (restoreSummary, error) {
	if confirmation != "ЗАМЕНИТЬ ДАННЫЕ" {
		return restoreSummary{}, ErrBadPhrase
	}

	session, err := s.restoreSession(restoreID)
	if err != nil {
		return restoreSummary{}, err
	}
	backupSession, err := s.backupSession(safetyBackupID)
	if err != nil {
		return restoreSummary{}, fmt.Errorf("%w: страховочная копия не найдена", ErrNotFound)
	}

	s.mu.Lock()
	confirmed := !backupSession.ConfirmedAt.IsZero()
	backupCreatedAt := backupSession.CreatedAt
	s.mu.Unlock()

	if !confirmed {
		return restoreSummary{}, ErrNotConfirmed
	}
	if backupCreatedAt.Before(session.CreatedAt) {
		return restoreSummary{}, fmt.Errorf("сначала скачайте свежую страховочную копию текущих данных")
	}

	restoreFinished := false
	defer func() {
		if !restoreFinished {
			s.discardRestore(session)
		}
	}()

	s.opMu.Lock()
	defer s.opMu.Unlock()

	// Another apply may have consumed this session while we waited for the
	// operation lock.
	current, err := s.restoreSession(restoreID)
	if err != nil || current != session {
		return restoreSummary{}, ErrNotFound
	}

	// The full gate is held from the first photo rename until the SQLite
	// COMMIT. All API and media requests receive 503 during this window.
	// If the COMMIT result is genuinely unknown we deliberately keep the gate
	// closed and require a restart, where RecoverOnStartup reconciles state.
	s.gate.Lock()
	gateHeld := true
	defer func() {
		if gateHeld {
			s.gate.Unlock()
		}
	}()

	journal := restoreJournal{
		Version:     journalVersion,
		OpID:        session.ID,
		CreatedAt:   time.Now().UTC(),
		LiveDir:     s.cfg.UploadDir,
		RollbackDir: filepath.Join(filepath.Dir(s.cfg.UploadDir), ".rollback-"+session.ID),
		StagedDir:   session.PhotosDir,
		TempRoot:    session.TempRoot,
		PhotosRoot:  session.PhotosRoot,
	}

	if _, err := os.Stat(journal.LiveDir); err == nil {
		journal.LiveExisted = true
	} else if !os.IsNotExist(err) {
		return restoreSummary{}, fmt.Errorf("check photo directory: %w", err)
	}

	if _, err := os.Stat(journal.RollbackDir); err == nil {
		return restoreSummary{}, fmt.Errorf("временный каталог восстановления уже существует")
	} else if !os.IsNotExist(err) {
		return restoreSummary{}, fmt.Errorf("check rollback directory: %w", err)
	}

	// The marker lives inside the staged photo directory. After COMMIT,
	// recovery can verify that the live directory is the new one, not just
	// that some directory exists at the same path.
	journal.MarkerName = ".restore-complete-" + session.ID
	if err := writeMarker(filepath.Join(journal.StagedDir, journal.MarkerName), session.ID); err != nil {
		return restoreSummary{}, err
	}

	if err := writeJournal(s.journalPath(), journal); err != nil {
		return restoreSummary{}, err
	}

	abort := func(cause error) error {
		if rollbackErr := s.rollbackPhotos(journal, cause); rollbackErr != nil {
			gateHeld = false
			restoreFinished = true
			log.Printf("backup: restore %s failed and photo rollback also failed; keeping maintenance mode until restart: %v", session.ID, rollbackErr)
			return fmt.Errorf("восстановление не удалось, откат фотографий тоже не удался; перезапустите приложение для согласования данных: %w", rollbackErr)
		}
		return cause
	}

	if journal.LiveExisted {
		if err := os.Rename(journal.LiveDir, journal.RollbackDir); err != nil {
			_ = removeJournal(s.journalPath())
			return restoreSummary{}, fmt.Errorf("move current photos: %w", err)
		}
		if err := syncDir(filepath.Dir(journal.LiveDir)); err != nil {
			return restoreSummary{}, abort(fmt.Errorf("sync photo directory: %w", err))
		}
	}

	if err := os.Rename(journal.StagedDir, journal.LiveDir); err != nil {
		return restoreSummary{}, abort(fmt.Errorf("install restored photos: %w", err))
	}
	if err := syncDir(filepath.Dir(journal.LiveDir)); err != nil {
		return restoreSummary{}, abort(fmt.Errorf("sync photo directory: %w", err))
	}

	if err := s.replaceDatabase(ctx, session.DBPath, session.ID); err != nil {
		if errors.Is(err, errRestoreCommitUnknown) {
			gateHeld = false
			restoreFinished = true
			log.Printf("backup: restore %s commit result unknown; keeping maintenance mode until restart: %v", session.ID, err)
			return restoreSummary{}, fmt.Errorf("не удалось определить результат восстановления; перезапустите приложение, данные и фотографии будут согласованы при старте: %w", err)
		}
		return restoreSummary{}, abort(err)
	}

	// COMMIT succeeded. The restore ID is already durable in SQLite. Cleanup
	// errors must not turn a successful restore into a reported failure.
	if err := os.RemoveAll(journal.RollbackDir); err != nil {
		log.Printf("backup: remove rollback photos: %v", err)
	}
	if err := os.RemoveAll(journal.StagedDir); err != nil {
		log.Printf("backup: remove staged photos: %v", err)
	}
	if err := os.RemoveAll(journal.PhotosRoot); err != nil {
		log.Printf("backup: remove photo restore root: %v", err)
	}
	if err := os.RemoveAll(journal.TempRoot); err != nil {
		log.Printf("backup: remove restore temp root: %v", err)
	}
	journalRemoved := true
	if err := removeJournal(s.journalPath()); err != nil {
		journalRemoved = false
		log.Printf("backup: remove restore journal: %v", err)
	}
	if journalRemoved && journal.MarkerName != "" {
		if err := os.Remove(filepath.Join(journal.LiveDir, journal.MarkerName)); err != nil && !os.IsNotExist(err) {
			log.Printf("backup: remove restore marker: %v", err)
		}
	}

	s.mu.Lock()
	delete(s.restores, session.ID)
	s.mu.Unlock()
	restoreFinished = true

	return session.summary(), nil
}

func (s *Service) discardRestore(session *restoreSession) {
	if session == nil {
		return
	}
	s.mu.Lock()
	delete(s.restores, session.ID)
	s.mu.Unlock()
	_ = os.RemoveAll(session.TempRoot)
	_ = os.RemoveAll(session.PhotosRoot)
}

func (s *Service) rollbackPhotos(journal restoreJournal, cause error) error {
	if err := restorePhotoDirectory(journal); err != nil {
		return fmt.Errorf("%w; additionally failed to roll back photos: %v", cause, err)
	}
	if err := removeJournal(s.journalPath()); err != nil {
		// The photos are back in place. A stale journal is safe: startup
		// recovery sees that SQLite did not commit and cleans it up.
		log.Printf("backup: remove journal after failed restore: %v", err)
	}
	return nil
}

func restorePhotoDirectory(journal restoreJournal) error {
	parent := filepath.Dir(journal.LiveDir)

	if _, err := os.Stat(journal.RollbackDir); err == nil {
		if err := os.RemoveAll(journal.LiveDir); err != nil {
			return fmt.Errorf("remove new photo directory: %w", err)
		}
		if err := os.Rename(journal.RollbackDir, journal.LiveDir); err != nil {
			return fmt.Errorf("restore old photo directory: %w", err)
		}
		return syncDir(parent)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check rollback directory: %w", err)
	}

	// No rollback directory: either the old directory was never renamed, or
	// the live path did not exist before the operation.
	if !journal.LiveExisted {
		if err := os.RemoveAll(journal.LiveDir); err != nil {
			return fmt.Errorf("remove new photo directory: %w", err)
		}
	} else if _, err := os.Stat(journal.LiveDir); err != nil {
		return fmt.Errorf("old photo directory disappeared during restore: %w", err)
	}
	return syncDir(parent)
}

func (s *Service) replaceDatabase(ctx context.Context, sourcePath, operationID string) error {
	db, err := sql.Open("sqlite", database.DSN(s.cfg.DBPath))
	if err != nil {
		return fmt.Errorf("open database for restore: %w", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("get restore connection: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `PRAGMA busy_timeout = 15000`); err != nil {
		return fmt.Errorf("set restore busy timeout: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA synchronous = FULL`); err != nil {
		return fmt.Errorf("set synchronous mode: %w", err)
	}

	escapedPath := strings.ReplaceAll(sourcePath, "'", "''")
	if _, err := conn.ExecContext(ctx, "ATTACH DATABASE '"+escapedPath+"' AS src"); err != nil {
		return fmt.Errorf("attach restored database: %w", err)
	}
	defer func() {
		if _, err := conn.ExecContext(context.Background(), `DETACH DATABASE src`); err != nil {
			log.Printf("backup: detach restored database: %v", err)
		}
	}()

	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return fmt.Errorf("begin restore transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if _, err := conn.ExecContext(context.Background(), `ROLLBACK`); err != nil && !strings.Contains(err.Error(), "no transaction is active") {
				log.Printf("backup: rollback restore transaction: %v", err)
			}
		}
	}()

	if _, err := conn.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		return fmt.Errorf("defer foreign keys: %w", err)
	}

	for i := len(businessTables) - 1; i >= 0; i-- {
		table := businessTables[i]
		query := "DELETE FROM main." + quoteIdent(table.name)
		if _, err := conn.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("clear %s: %w", table.name, err)
		}
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM main.notification_outbox`); err != nil {
		return fmt.Errorf("clear notification outbox: %w", err)
	}

	for _, table := range businessTables {
		columns := strings.Join(quoteAll(table.columns), ", ")
		query := fmt.Sprintf(
			"INSERT INTO main.%s (%s) SELECT %s FROM src.%s",
			quoteIdent(table.name), columns, columns, quoteIdent(table.name),
		)
		if _, err := conn.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("restore %s: %w", table.name, err)
		}
	}

	if _, err := conn.ExecContext(ctx, `
		INSERT INTO main.restore_state (id, restore_id, applied_at)
		VALUES (1, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			restore_id = excluded.restore_id,
			applied_at = excluded.applied_at
	`, operationID); err != nil {
		return fmt.Errorf("write restore state: %w", err)
	}

	if err := verifyRestoredCounts(ctx, conn); err != nil {
		return err
	}
	if err := checkForeignKeysConn(ctx, conn); err != nil {
		return err
	}
	if err := checkIntegrityConn(ctx, conn); err != nil {
		return err
	}

	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		commitErr := err
		// A driver-level COMMIT error is rare. Determine the real outcome from
		// restore_state before deciding whether photos must be rolled back.
		_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		applied, checkErr := restoreStateApplied(context.Background(), s.cfg.DBPath, operationID)
		if checkErr != nil {
			return fmt.Errorf("%w: %v (commit error: %v)", errRestoreCommitUnknown, checkErr, commitErr)
		}
		if applied {
			committed = true
			return nil
		}
		return fmt.Errorf("commit restored data: %w", commitErr)
	}
	committed = true

	return nil
}

func restoreStateApplied(ctx context.Context, dbPath, operationID string) (bool, error) {
	db, err := sql.Open("sqlite", database.DSN(dbPath))
	if err != nil {
		return false, err
	}
	defer db.Close()

	var applied string
	err = db.QueryRowContext(ctx, `SELECT restore_id FROM restore_state WHERE id = 1`).Scan(&applied)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return applied == operationID, nil
}

func quoteAll(columns []string) []string {
	result := make([]string, len(columns))
	for i, column := range columns {
		result[i] = quoteIdent(column)
	}
	return result
}

func verifyRestoredCounts(ctx context.Context, conn *sql.Conn) error {
	for _, table := range businessTables {
		var restored int64
		if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM main."+quoteIdent(table.name)).Scan(&restored); err != nil {
			return fmt.Errorf("count restored %s: %w", table.name, err)
		}
		var source int64
		if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM src."+quoteIdent(table.name)).Scan(&source); err != nil {
			return fmt.Errorf("count source %s: %w", table.name, err)
		}
		if restored != source {
			return fmt.Errorf("restored row count mismatch in %s: got %d, expected %d", table.name, restored, source)
		}
	}

	var outbox int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM main.notification_outbox`).Scan(&outbox); err != nil {
		return fmt.Errorf("count notification outbox: %w", err)
	}
	if outbox != 0 {
		return fmt.Errorf("notification_outbox was not cleared")
	}
	return nil
}

func checkForeignKeysConn(ctx context.Context, conn *sql.Conn) error {
	rows, err := conn.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		return errors.New("foreign_key_check found violations")
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	return nil
}

func checkIntegrityConn(ctx context.Context, conn *sql.Conn) error {
	var result string
	if err := conn.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return fmt.Errorf("integrity_check: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(result), "ok") {
		return fmt.Errorf("integrity_check failed: %s", result)
	}
	return nil
}

// RecoverOnStartup completes or rolls back an interrupted restore. SQLite is
// the source of truth: restore_state.restore_id is written in the same
// transaction as the data. The photo journal tells us which directories are
// involved, but never overrides a committed database.
func (s *Service) RecoverOnStartup(ctx context.Context) error {
	journalPath := s.journalPath()
	journal, err := readJournal(journalPath)
	if err != nil {
		if os.IsNotExist(err) {
			applied, stateErr := s.currentRestoreID(ctx)
			if stateErr != nil {
				// The rollback directory may still be needed. Do not remove
				// anything that could be recovery data.
				log.Printf("backup: cannot read restore state; keeping rollback directories: %v", stateErr)
				s.cleanupOrphans(ctx, "", false)
				return nil
			}
			s.cleanupOrphans(ctx, applied, true)
			return nil
		}
		return err
	}

	if err := s.validateRestoreJournal(journal); err != nil {
		return err
	}

	applied, err := s.currentRestoreID(ctx)
	if err != nil {
		return fmt.Errorf("read restore state: %w", err)
	}

	if applied == journal.OpID {
		// SQLite COMMIT succeeded. Photos must be the new set. This is the
		// critical case where the process died after COMMIT but before the
		// journal was removed.
		if _, statErr := os.Stat(journal.LiveDir); statErr != nil {
			return fmt.Errorf("restore %s committed but photo directory is missing: %w", journal.OpID, statErr)
		}
		if journal.MarkerName != "" {
			if _, statErr := os.Stat(filepath.Join(journal.LiveDir, journal.MarkerName)); statErr != nil {
				return fmt.Errorf("restore %s committed but live photos do not match the new version: %w", journal.OpID, statErr)
			}
		}
		if err := os.RemoveAll(journal.RollbackDir); err != nil {
			log.Printf("backup: recovery remove rollback photos: %v", err)
		}
		if err := os.RemoveAll(journal.StagedDir); err != nil {
			log.Printf("backup: recovery remove staged photos: %v", err)
		}
	} else {
		// The database did not commit this operation. Put the old photos back.
		if err := restorePhotoDirectory(journal); err != nil {
			return fmt.Errorf("roll back interrupted restore: %w", err)
		}
		if err := os.RemoveAll(journal.StagedDir); err != nil {
			log.Printf("backup: recovery remove staged photos: %v", err)
		}
	}

	if err := os.RemoveAll(journal.TempRoot); err != nil {
		log.Printf("backup: recovery remove restore temp: %v", err)
	}
	if err := os.RemoveAll(journal.PhotosRoot); err != nil {
		log.Printf("backup: recovery remove restore photo root: %v", err)
	}
	if err := removeJournal(journalPath); err != nil {
		return fmt.Errorf("remove restore journal: %w", err)
	}

	s.cleanupOrphans(ctx, applied, true)
	return nil
}

// currentRestoreID returns the last successfully committed restore operation.
// An empty string means no restore has ever completed. Any error must be
// treated as "unknown": rollback data must not be removed in that case.
func (s *Service) currentRestoreID(ctx context.Context) (string, error) {
	var applied string
	err := s.cfg.DB.QueryRowContext(ctx, `SELECT restore_id FROM restore_state WHERE id = 1`).Scan(&applied)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return applied, nil
}

func (s *Service) cleanupOrphans(ctx context.Context, applied string, allowRollbackCleanup bool) {
	_ = ctx

	// A crash during an atomic journal write leaves only this temporary file.
	_ = os.Remove(s.journalPath() + ".tmp")

	if entries, err := os.ReadDir(s.cfg.TempDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if _, ok := operationIDAfter(name, "backup-"); ok {
				_ = removeDirWithin(s.cfg.TempDir, name)
				continue
			}
			if _, ok := operationIDAfter(name, "restore-"); ok {
				_ = removeDirWithin(s.cfg.TempDir, name)
			}
		}
	}

	// Markers are only needed while the journal exists. Remove leftovers from
	// a crash after journal cleanup, but only files with an expected name.
	if entries, err := os.ReadDir(s.cfg.UploadDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if _, ok := operationIDAfter(entry.Name(), ".restore-complete-"); !ok {
				continue
			}
			path := filepath.Join(s.cfg.UploadDir, entry.Name())
			if pathWithin(s.cfg.UploadDir, path) {
				_ = os.Remove(path)
			}
		}
	}

	parent := filepath.Dir(s.cfg.UploadDir)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		switch {
		case strings.HasPrefix(name, ".backup-"):
			if _, ok := operationIDAfter(name, ".backup-"); ok {
				_ = removeDirWithin(parent, name)
			}
		case strings.HasPrefix(name, ".restore-"):
			if _, ok := operationIDAfter(name, ".restore-"); ok {
				_ = removeDirWithin(parent, name)
			}
		case strings.HasPrefix(name, ".rollback-"):
			opID, ok := operationIDAfter(name, ".rollback-")
			if !ok || !allowRollbackCleanup || !validOperationID(applied) {
				continue
			}
			if opID == applied {
				if err := removeDirWithin(parent, name); err != nil {
					log.Printf("backup: cleanup applied rollback directory: %v", err)
				}
			}
		}
	}
}

func operationIDAfter(name, prefix string) (string, bool) {
	if !strings.HasPrefix(name, prefix) {
		return "", false
	}
	id := strings.TrimPrefix(name, prefix)
	if !validOperationID(id) {
		return "", false
	}
	return id, true
}

func pathWithin(parent, candidate string) bool {
	rel, err := filepath.Rel(parent, candidate)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(rel)
}

func removeDirWithin(parent, name string) error {
	path := filepath.Join(parent, name)
	if !pathWithin(parent, path) {
		return fmt.Errorf("refusing to remove directory outside %s: %s", parent, path)
	}
	return os.RemoveAll(path)
}

func writeMarker(path, operationID string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create restore marker: %w", err)
	}
	if _, err := file.WriteString(operationID + "\n"); err != nil {
		file.Close()
		return fmt.Errorf("write restore marker: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync restore marker: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close restore marker: %w", err)
	}
	return syncDir(filepath.Dir(path))
}

func writeJournal(path string, value restoreJournal) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode restore journal: %w", err)
	}

	tempPath := path + ".tmp"
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create restore journal: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("write restore journal: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("sync restore journal: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close restore journal: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("publish restore journal: %w", err)
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync restore journal directory: %w", err)
	}
	return nil
}

func readJournal(path string) (restoreJournal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return restoreJournal{}, err
	}
	var value restoreJournal
	if err := json.Unmarshal(data, &value); err != nil {
		return restoreJournal{}, fmt.Errorf("decode restore journal: %w", err)
	}
	return value, nil
}

func removeJournal(path string) error {
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return syncDir(filepath.Dir(path))
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
