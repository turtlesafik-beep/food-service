package backup

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"food-service/internal/database"
	"food-service/internal/maintenance"
)

const (
	defaultMaxArchiveBytes = int64(512 << 20)
	defaultSessionTTL      = 2 * time.Hour
	defaultRestoreTTL      = 30 * time.Minute
)

var (
	ErrNotFound      = errors.New("операция не найдена или истекла")
	ErrConflict      = errors.New("уже выполняется другая операция с данными")
	ErrArchiveTooBig = errors.New("архив превышает допустимый размер")
	ErrBadArchive    = errors.New("архив повреждён или имеет неверный формат")
	ErrNotDownloaded = errors.New("сначала скачайте резервную копию")
	ErrNotConfirmed  = errors.New("подтвердите, что резервная копия сохранена")
	ErrBadPhrase     = errors.New("неверная подтверждающая фраза")
	ErrIncompatible  = errors.New("архив несовместим с текущей версией приложения")
	ErrInvalidData   = errors.New("архив содержит некорректные данные")
)

type Config struct {
	DB              *sql.DB
	DBPath          string
	UploadDir       string
	TempDir         string
	PublicBaseURL   string
	MaxArchiveBytes int64
	SessionTTL      time.Duration
	RestoreTTL      time.Duration
}

type Service struct {
	cfg  Config
	gate *maintenance.Gate

	opMu sync.Mutex

	mu       sync.Mutex
	backups  map[string]*backupSession
	restores map[string]*restoreSession
}

type backupSession struct {
	ID           string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	DownloadedAt time.Time
	ConfirmedAt  time.Time
	Size         int64
	SHA256       string
	Counts       map[string]int64
	PhotoCount   int
}

type restoreSession struct {
	ID           string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	ArchiveSize  int64
	DatabaseSize int64
	TempRoot     string
	PhotosRoot   string
	PhotosDir    string
	DBPath       string
	Manifest     manifest
}

type exportArtifacts struct {
	TempRoot   string
	PhotosRoot string
	DBPath     string
	Photos     []photoInfo
	Manifest   manifest
}

func (a *exportArtifacts) cleanup() {
	if a == nil {
		return
	}
	if a.TempRoot != "" {
		_ = os.RemoveAll(a.TempRoot)
	}
	if a.PhotosRoot != "" {
		_ = os.RemoveAll(a.PhotosRoot)
	}
}

func NewService(cfg Config, gate *maintenance.Gate) (*Service, error) {
	if cfg.DB == nil {
		return nil, errors.New("backup: database is required")
	}
	if cfg.DBPath == "" {
		cfg.DBPath = database.Path()
	}
	if cfg.UploadDir == "" {
		cfg.UploadDir = filepath.Join("data", "uploads", "dishes")
	}
	if cfg.TempDir == "" {
		cfg.TempDir = filepath.Join(filepath.Dir(cfg.DBPath), "tmp")
	}
	if cfg.MaxArchiveBytes <= 0 {
		cfg.MaxArchiveBytes = defaultMaxArchiveBytes
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = defaultSessionTTL
	}
	if cfg.RestoreTTL <= 0 {
		cfg.RestoreTTL = defaultRestoreTTL
	}

	cfg.DBPath = absolutePath(cfg.DBPath)
	cfg.UploadDir = absolutePath(cfg.UploadDir)
	cfg.TempDir = absolutePath(cfg.TempDir)
	cfg.PublicBaseURL = strings.TrimSpace(cfg.PublicBaseURL)

	if err := os.MkdirAll(cfg.TempDir, 0o700); err != nil {
		return nil, fmt.Errorf("create backup temp directory: %w", err)
	}

	return &Service{
		cfg:      cfg,
		gate:     gate,
		backups:  make(map[string]*backupSession),
		restores: make(map[string]*restoreSession),
	}, nil
}

func absolutePath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return absolute
}

func newOperationID() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate operation id: %w", err)
	}
	return hex.EncodeToString(data), nil
}

func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupExpired()
		}
	}
}

func (s *Service) cleanupExpired() {
	// Serialize directory removal with apply and archive building. Otherwise an
	// expired session could have its staged photos removed while it is being
	// installed.
	s.opMu.Lock()
	defer s.opMu.Unlock()

	now := time.Now()

	s.mu.Lock()
	var stale []*restoreSession
	for id, session := range s.backups {
		if now.After(session.ExpiresAt) {
			delete(s.backups, id)
		}
	}
	for id, session := range s.restores {
		if now.After(session.ExpiresAt) {
			stale = append(stale, session)
			delete(s.restores, id)
		}
	}
	s.mu.Unlock()

	for _, session := range stale {
		_ = os.RemoveAll(session.TempRoot)
		_ = os.RemoveAll(session.PhotosRoot)
	}
}

func (s *Service) PrepareBackup() (backupSummary, error) {
	s.cleanupExpired()

	id, err := newOperationID()
	if err != nil {
		return backupSummary{}, err
	}

	now := time.Now().UTC()
	session := &backupSession{
		ID:        id,
		CreatedAt: now,
		ExpiresAt: now.Add(s.cfg.SessionTTL),
	}

	s.mu.Lock()
	s.backups[id] = session
	s.mu.Unlock()

	return session.summary(), nil
}

func (s *Service) backupSession(id string) (*backupSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.backups[id]
	if session == nil || time.Now().After(session.ExpiresAt) {
		return nil, ErrNotFound
	}
	return session, nil
}

func (s *Service) restoreSession(id string) (*restoreSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.restores[id]
	if session == nil || time.Now().After(session.ExpiresAt) {
		return nil, ErrNotFound
	}
	return session, nil
}

func (s *Service) ConfirmBackup(id string) (backupSummary, error) {
	session, err := s.backupSession(id)
	if err != nil {
		return backupSummary{}, err
	}

	s.mu.Lock()
	if session.DownloadedAt.IsZero() {
		s.mu.Unlock()
		return backupSummary{}, ErrNotDownloaded
	}
	session.ConfirmedAt = time.Now().UTC()
	summary := session.summary()
	s.mu.Unlock()

	return summary, nil
}

func (s *Service) BackupInfo(id string) (backupSummary, error) {
	session, err := s.backupSession(id)
	if err != nil {
		return backupSummary{}, err
	}

	s.mu.Lock()
	summary := session.summary()
	s.mu.Unlock()
	return summary, nil
}

func (session *backupSession) summary() backupSummary {
	return backupSummary{
		ID:            session.ID,
		CreatedAt:     session.CreatedAt,
		ExpiresAt:     session.ExpiresAt,
		DownloadedAt:  session.DownloadedAt,
		ConfirmedAt:   session.ConfirmedAt,
		Size:          session.Size,
		SHA256:        session.SHA256,
		SchemaVersion: database.SchemaVersion,
		Counts:        cloneCounts(session.Counts),
		PhotoCount:    session.PhotoCount,
	}
}

func cloneCounts(source map[string]int64) map[string]int64 {
	if source == nil {
		return nil
	}
	result := make(map[string]int64, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

// Download is a prepared, consistent archive. It is built before HTTP headers
// are written so errors can still be reported normally.
type Download struct {
	service   *Service
	session   *backupSession
	artifacts *exportArtifacts
}

func (s *Service) BuildDownload(ctx context.Context, id string) (*Download, error) {
	session, err := s.backupSession(id)
	if err != nil {
		return nil, err
	}

	s.opMu.Lock()
	artifacts, err := s.buildExport(ctx, id)
	s.opMu.Unlock()
	if err != nil {
		return nil, err
	}

	return &Download{service: s, session: session, artifacts: artifacts}, nil
}

func (d *Download) Write(w io.Writer) error {
	hash := sha256.New()
	counter := &countingWriter{writer: io.MultiWriter(w, hash)}
	if err := d.service.writeZip(counter, d.artifacts); err != nil {
		return err
	}

	d.service.mu.Lock()
	d.session.DownloadedAt = time.Now().UTC()
	d.session.Size = counter.written
	d.session.SHA256 = hex.EncodeToString(hash.Sum(nil))
	d.session.Counts = cloneCounts(d.artifacts.Manifest.Counts)
	d.session.PhotoCount = len(d.artifacts.Manifest.Photos)
	d.service.mu.Unlock()

	return nil
}

func (d *Download) Close() {
	if d != nil && d.artifacts != nil {
		d.artifacts.cleanup()
	}
}

// WriteArchive is a convenience wrapper used by tests and non-HTTP callers.
func (s *Service) WriteArchive(ctx context.Context, id string, w io.Writer) error {
	download, err := s.BuildDownload(ctx, id)
	if err != nil {
		return err
	}
	defer download.Close()
	return download.Write(w)
}

type countingWriter struct {
	writer  io.Writer
	written int64
}

func (w *countingWriter) Write(data []byte) (int, error) {
	n, err := w.writer.Write(data)
	w.written += int64(n)
	return n, err
}

func (s *Service) buildExport(ctx context.Context, id string) (*exportArtifacts, error) {
	tempRoot := filepath.Join(s.cfg.TempDir, "backup-"+id)
	if err := os.MkdirAll(tempRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create backup temp directory: %w", err)
	}

	artifacts := &exportArtifacts{TempRoot: tempRoot}
	success := false
	defer func() {
		if !success {
			artifacts.cleanup()
		}
	}()

	dbPath := filepath.Join(tempRoot, "orders.db")
	photosRoot := filepath.Join(filepath.Dir(s.cfg.UploadDir), ".backup-"+id)
	photosDir := filepath.Join(photosRoot, "dishes")
	artifacts.PhotosRoot = photosRoot

	// Block changes only while the SQLite snapshot and photo hard links are
	// created. Stable hard links and the snapshot allow the ZIP to be streamed
	// after normal traffic resumes.
	s.gate.Lock()
	err := func() error {
		if err := s.snapshotDatabase(ctx, dbPath); err != nil {
			return err
		}
		if err := stagePhotos(s.cfg.UploadDir, photosDir); err != nil {
			return fmt.Errorf("stage photos: %w", err)
		}
		return nil
	}()
	s.gate.Unlock()
	if err != nil {
		return nil, err
	}

	if err := os.Chmod(dbPath, 0o600); err != nil {
		return nil, fmt.Errorf("protect snapshot: %w", err)
	}

	dbInfo, err := fileInfo(dbPath)
	if err != nil {
		return nil, err
	}

	photos, err := photoInfos(photosDir)
	if err != nil {
		return nil, err
	}

	counts, fingerprint, version, err := s.inspectSnapshot(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	if err := verifySnapshotPhotos(ctx, dbPath, photos); err != nil {
		return nil, err
	}

	artifacts.DBPath = dbPath
	// Hard links point to the original inode, so the paths used by the ZIP can
	// remain the staging directory even after release of the gate.
	artifacts.Photos = photos
	artifacts.Manifest = manifest{
		Format:            manifestFormat,
		FormatVersion:     manifestFormatVersion,
		CreatedAt:         time.Now().UTC(),
		AppVersion:        "dev",
		SchemaVersion:     version,
		SchemaFingerprint: fingerprint,
		Database: databaseInfo{
			Path:   "database/orders.db",
			Size:   dbInfo.Size(),
			SHA256: dbInfo.hash,
		},
		Counts: counts,
		Photos: photos,
	}

	success = true
	return artifacts, nil
}

type fileDigest struct {
	size int64
	hash string
}

func (f fileDigest) Size() int64 { return f.size }

func fileInfo(path string) (fileDigest, error) {
	info, err := os.Stat(path)
	if err != nil {
		return fileDigest{}, fmt.Errorf("stat file: %w", err)
	}
	hash, err := hashFile(path)
	if err != nil {
		return fileDigest{}, err
	}
	return fileDigest{size: info.Size(), hash: hash}, nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file for hash: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash file: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// snapshotDatabase uses VACUUM INTO: SQLite itself creates a transactionally
// consistent single-file snapshot including all committed WAL frames. Copying
// orders.db, -wal and -shm independently would not be safe.
func (s *Service) snapshotDatabase(ctx context.Context, destination string) error {
	_ = os.Remove(destination)
	escaped := strings.ReplaceAll(destination, "'", "''")
	if _, err := s.cfg.DB.ExecContext(ctx, "VACUUM INTO '"+escaped+"'"); err != nil {
		return fmt.Errorf("create SQLite snapshot: %w", err)
	}

	db, err := openSQLite(destination, true)
	if err != nil {
		return err
	}
	defer db.Close()

	// notification_outbox is technical delivery state and is explicitly
	// excluded from archives, including its table definition. restore_state is
	// service state. The VACUUM rewrites the database so deleted rows do not
	// remain in free pages.
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS notification_outbox`); err != nil {
		return fmt.Errorf("remove outbox from snapshot: %w", err)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS restore_state`); err != nil {
		return fmt.Errorf("remove restore state from snapshot: %w", err)
	}
	if _, err := db.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("compact SQLite snapshot: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode = DELETE`); err != nil {
		return fmt.Errorf("finalize SQLite snapshot journal: %w", err)
	}

	return nil
}

func verifySnapshotPhotos(ctx context.Context, dbPath string, photos []photoInfo) error {
	db, err := openSQLite(dbPath, false)
	if err != nil {
		return err
	}
	defer db.Close()

	available := make(map[string]photoInfo, len(photos))
	for _, photo := range photos {
		available[photo.Name] = photo
	}
	return validatePhotoReferences(ctx, db, available)
}

func (s *Service) inspectSnapshot(ctx context.Context, path string) (map[string]int64, string, int, error) {
	db, err := openSQLite(path, true)
	if err != nil {
		return nil, "", 0, err
	}
	defer db.Close()

	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return nil, "", 0, fmt.Errorf("read snapshot schema version: %w", err)
	}
	if version != database.SchemaVersion {
		return nil, "", 0, fmt.Errorf("%w: версия схемы %d, ожидается %d", ErrIncompatible, version, database.SchemaVersion)
	}

	if err := checkIntegrity(ctx, db); err != nil {
		return nil, "", 0, err
	}

	counts, err := businessCounts(ctx, db)
	if err != nil {
		return nil, "", 0, err
	}
	fingerprint, err := schemaFingerprint(ctx, db)
	if err != nil {
		return nil, "", 0, err
	}

	return counts, fingerprint, version, nil
}

func openSQLite(path string, writable bool) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)"
	if !writable {
		dsn = "file:" + path + "?mode=ro&_pragma=busy_timeout(5000)"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open SQLite file: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping SQLite file: %w", err)
	}
	return db, nil
}

func checkIntegrity(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return fmt.Errorf("%w: integrity_check: %v", ErrBadArchive, err)
	}
	defer rows.Close()

	var result string
	if rows.Next() {
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("%w: read integrity_check: %v", ErrBadArchive, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: integrity_check: %v", ErrBadArchive, err)
	}
	if !strings.EqualFold(strings.TrimSpace(result), "ok") {
		return fmt.Errorf("%w: SQLite integrity check failed: %s", ErrBadArchive, result)
	}
	return nil
}

func checkForeignKeys(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("%w: foreign_key_check: %v", ErrBadArchive, err)
	}
	defer rows.Close()

	if rows.Next() {
		return fmt.Errorf("%w: SQLite foreign key check failed", ErrBadArchive)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: foreign_key_check: %v", ErrBadArchive, err)
	}
	return nil
}

func stagePhotos(sourceDir, destinationDir string) error {
	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsupported symlink in photo directory: %s", entry.Name())
		}

		if strings.HasPrefix(entry.Name(), ".restore-complete-") {
			continue
		}
		if !validPhotoName(entry.Name()) {
			log.Printf("backup: skip unsupported file in photo directory: %s", entry.Name())
			continue
		}

		source := filepath.Join(sourceDir, entry.Name())
		destination := filepath.Join(destinationDir, entry.Name())

		if err := os.Link(source, destination); err == nil {
			continue
		}
		if err := copyFile(source, destination); err != nil {
			return err
		}
	}

	return nil
}

func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(destination)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(destination)
		return err
	}
	return nil
}

func photoInfos(directory string) ([]photoInfo, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return []photoInfo{}, nil
		}
		return nil, err
	}

	photos := make([]photoInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !validPhotoName(entry.Name()) {
			return nil, fmt.Errorf("unsupported photo file: %s", entry.Name())
		}

		info, err := fileInfo(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		photos = append(photos, photoInfo{
			Name:   entry.Name(),
			Size:   info.Size(),
			SHA256: info.hash,
		})
	}

	sort.Slice(photos, func(i, j int) bool {
		return photos[i].Name < photos[j].Name
	})
	return photos, nil
}

func (s *Service) writeZip(w io.Writer, artifacts *exportArtifacts) error {
	archive := zip.NewWriter(w)

	manifestData, err := json.MarshalIndent(artifacts.Manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := writeZipBytes(archive, "manifest.json", manifestData); err != nil {
		return err
	}
	if err := writeZipFile(archive, "database/orders.db", artifacts.DBPath); err != nil {
		return err
	}
	for _, photo := range artifacts.Photos {
		if err := writeZipFile(archive, "uploads/dishes/"+photo.Name, filepath.Join(artifacts.PhotosRoot, "dishes", photo.Name)); err != nil {
			return err
		}
	}

	if err := archive.Close(); err != nil {
		return fmt.Errorf("finish ZIP archive: %w", err)
	}
	return nil
}

func writeZipBytes(archive *zip.Writer, name string, data []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o644)
	writer, err := archive.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create ZIP entry %s: %w", name, err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("write ZIP entry %s: %w", name, err)
	}
	return nil
}

func writeZipFile(archive *zip.Writer, name, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat ZIP source: %w", err)
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("ZIP header for %s: %w", name, err)
	}
	header.Name = name
	header.Method = zip.Deflate
	if strings.HasPrefix(name, "uploads/") {
		header.Method = zip.Store
	}
	header.SetMode(0o644)

	writer, err := archive.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create ZIP entry %s: %w", name, err)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open ZIP source: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(writer, file); err != nil {
		return fmt.Errorf("write ZIP entry %s: %w", name, err)
	}
	return nil
}
