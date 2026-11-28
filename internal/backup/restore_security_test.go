package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validJournalForSession(env *testEnv, session *restoreSession) restoreJournal {
	return restoreJournal{
		Version:     journalVersion,
		OpID:        session.ID,
		CreatedAt:   time.Now().UTC(),
		LiveDir:     env.uploadDir,
		RollbackDir: filepath.Join(filepath.Dir(env.uploadDir), ".rollback-"+session.ID),
		StagedDir:   session.PhotosDir,
		TempRoot:    session.TempRoot,
		PhotosRoot:  session.PhotosRoot,
		MarkerName:  ".restore-complete-" + session.ID,
		LiveExisted: true,
	}
}

func newValidatedSession(t *testing.T, env *testEnv) *restoreSession {
	t.Helper()

	seedData(t, env, "")
	archive := buildArchive(t, env.service)
	summary := validateArchive(t, env.service, archive)
	session, err := env.service.restoreSession(summary.ID)
	if err != nil {
		t.Fatalf("get restore session: %v", err)
	}
	return session
}

func TestValidateRestoreJournalRejectsUnexpectedValues(t *testing.T) {
	env := newTestEnv(t, 0)
	session := newValidatedSession(t, env)
	valid := validJournalForSession(env, session)

	if err := env.service.validateRestoreJournal(valid); err != nil {
		t.Fatalf("valid journal was rejected: %v", err)
	}

	outside := filepath.Join(t.TempDir(), "outside")
	otherID := strings.Repeat("b", 32)

	mutations := []struct {
		name   string
		mutate func(*restoreJournal)
	}{
		{"version", func(j *restoreJournal) { j.Version = journalVersion + 1 }},
		{"empty operation id", func(j *restoreJournal) { j.OpID = "" }},
		{"short operation id", func(j *restoreJournal) { j.OpID = strings.Repeat("a", 31) }},
		{"uppercase operation id", func(j *restoreJournal) { j.OpID = strings.ToUpper(j.OpID) }},
		{"non-hex operation id", func(j *restoreJournal) { j.OpID = strings.Repeat("z", 32) }},
		{"empty marker", func(j *restoreJournal) { j.MarkerName = "" }},
		{"foreign marker", func(j *restoreJournal) { j.MarkerName = ".restore-complete-" + otherID }},
		{"marker path", func(j *restoreJournal) { j.MarkerName = "../restore-complete-" + j.OpID }},
		{"live dir", func(j *restoreJournal) { j.LiveDir = outside }},
		{"rollback dir", func(j *restoreJournal) { j.RollbackDir = outside }},
		{"staged dir", func(j *restoreJournal) { j.StagedDir = outside }},
		{"temp root", func(j *restoreJournal) { j.TempRoot = outside }},
		{"photos root", func(j *restoreJournal) { j.PhotosRoot = outside }},
		{"relative live dir", func(j *restoreJournal) { j.LiveDir = "relative/dishes" }},
		{"empty live dir", func(j *restoreJournal) { j.LiveDir = "" }},
	}

	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			journal := valid
			mutation.mutate(&journal)

			err := env.service.validateRestoreJournal(journal)
			if !errors.Is(err, errInvalidRestoreJournal) {
				t.Fatalf("journal was accepted, got %v", err)
			}
		})
	}
}

func TestRecoverRejectsMaliciousJournalWithoutDeletingData(t *testing.T) {
	env := newTestEnv(t, 0)
	session := newValidatedSession(t, env)
	journal := validJournalForSession(env, session)
	journal.RollbackDir = filepath.Join(t.TempDir(), "precious")

	if err := os.MkdirAll(journal.RollbackDir, 0o755); err != nil {
		t.Fatalf("create precious directory: %v", err)
	}
	sentinel := filepath.Join(journal.RollbackDir, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("must survive"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	if err := writeJournal(env.service.journalPath(), journal); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	if _, err := env.db.Exec(
		`INSERT INTO restore_state (id, restore_id) VALUES (1, ?)`,
		session.ID,
	); err != nil {
		t.Fatalf("write restore state: %v", err)
	}

	err := env.service.RecoverOnStartup(context.Background())
	if !errors.Is(err, errInvalidRestoreJournal) {
		t.Fatalf("expected invalid journal error, got %v", err)
	}

	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("recovery touched a path from a malicious journal: %v", err)
	}
	if _, err := os.Stat(env.service.journalPath()); err != nil {
		t.Fatalf("invalid journal must be preserved for manual recovery: %v", err)
	}
}

func TestRecoverRejectsInvalidJournalIdentity(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*restoreJournal)
	}{
		{"operation id", func(j *restoreJournal) { j.OpID = "not-an-operation-id" }},
		{"marker name", func(j *restoreJournal) {
			j.MarkerName = ".restore-complete-" + strings.Repeat("c", 32)
		}},
	}

	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			env := newTestEnv(t, 0)
			session := newValidatedSession(t, env)
			journal := validJournalForSession(env, session)
			mutation.mutate(&journal)

			if err := writeJournal(env.service.journalPath(), journal); err != nil {
				t.Fatalf("write journal: %v", err)
			}

			err := env.service.RecoverOnStartup(context.Background())
			if !errors.Is(err, errInvalidRestoreJournal) {
				t.Fatalf("expected invalid journal error, got %v", err)
			}
			if _, err := os.Stat(env.service.journalPath()); err != nil {
				t.Fatalf("invalid journal must be preserved: %v", err)
			}
		})
	}
}

func TestStartupCleanupOnlyRemovesExpectedOrphans(t *testing.T) {
	env := newTestEnv(t, 0)
	validBackupID := strings.Repeat("a", 32)
	validRestoreID := strings.Repeat("b", 32)
	validMarkerID := strings.Repeat("c", 32)
	unknownID := strings.Repeat("d", 32)

	tempDir := env.service.cfg.TempDir
	parent := filepath.Dir(env.uploadDir)

	mkdir := func(path string) {
		t.Helper()
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create %s: %v", path, err)
		}
	}

	removed := []string{
		filepath.Join(tempDir, "backup-"+validBackupID),
		filepath.Join(tempDir, "restore-"+validRestoreID),
		filepath.Join(parent, ".backup-"+validBackupID),
		filepath.Join(parent, ".restore-"+validRestoreID),
	}
	kept := []string{
		filepath.Join(tempDir, "backup-not-an-id"),
		filepath.Join(tempDir, "restore-"+strings.Repeat("a", 31)),
		filepath.Join(tempDir, "other-"+unknownID),
		filepath.Join(parent, ".backup-evil"),
		filepath.Join(parent, ".restore-"+strings.Repeat("z", 32)),
		filepath.Join(parent, ".rollback-"+validBackupID),
		filepath.Join(parent, "unknown-"+unknownID),
	}

	for _, path := range append(append([]string{}, removed...), kept...) {
		mkdir(path)
	}

	markerRemoved := filepath.Join(env.uploadDir, ".restore-complete-"+validMarkerID)
	if err := os.WriteFile(markerRemoved, []byte(validMarkerID), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	markerDir := filepath.Join(env.uploadDir, ".restore-complete-"+strings.Repeat("e", 32))
	mkdir(markerDir)
	keptMarkers := []string{
		filepath.Join(env.uploadDir, ".restore-complete-not-valid"),
		filepath.Join(env.uploadDir, ".other-"+validMarkerID),
		filepath.Join(env.uploadDir, "photo-"+validMarkerID+".jpg"),
	}
	for _, path := range keptMarkers {
		if err := os.WriteFile(path, []byte("keep"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	if err := env.service.RecoverOnStartup(context.Background()); err != nil {
		t.Fatalf("recover: %v", err)
	}

	for _, path := range removed {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected orphan %s to be removed, stat err = %v", path, err)
		}
	}
	for _, path := range kept {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("unexpectedly removed %s: %v", path, err)
		}
	}
	if _, err := os.Stat(markerRemoved); !os.IsNotExist(err) {
		t.Fatalf("expected orphan marker to be removed, stat err = %v", err)
	}
	if _, err := os.Stat(markerDir); err != nil {
		t.Fatalf("marker-named directory must not be removed: %v", err)
	}
	for _, path := range keptMarkers {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("unexpectedly removed %s: %v", path, err)
		}
	}
}
