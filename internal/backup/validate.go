package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"food-service/internal/database"
)

const (
	maxManifestBytes = 4 << 20
	maxZipEntries    = 20000
)

var zipNamePattern = regexp.MustCompile(`^[\pL\pN._ /-]+$`)

func (s *Service) ValidateRestore(ctx context.Context, body io.Reader, declaredSize int64) (restoreSummary, error) {
	s.cleanupExpired()

	id, err := newOperationID()
	if err != nil {
		return restoreSummary{}, err
	}

	tempRoot := filepath.Join(s.cfg.TempDir, "restore-"+id)
	photosRoot := filepath.Join(filepath.Dir(s.cfg.UploadDir), ".restore-"+id)
	photosDir := filepath.Join(photosRoot, "dishes")

	cleanup := func() {
		_ = os.RemoveAll(tempRoot)
		_ = os.RemoveAll(photosRoot)
	}
	success := false
	defer func() {
		if !success {
			cleanup()
		}
	}()

	if err := os.MkdirAll(tempRoot, 0o700); err != nil {
		return restoreSummary{}, fmt.Errorf("create restore temp directory: %w", err)
	}
	if err := os.MkdirAll(photosDir, 0o755); err != nil {
		return restoreSummary{}, fmt.Errorf("create restore photo directory: %w", err)
	}

	zipPath := filepath.Join(tempRoot, "upload.zip")
	archiveSize, err := receiveArchive(body, zipPath, s.cfg.MaxArchiveBytes, declaredSize)
	if err != nil {
		return restoreSummary{}, err
	}
	defer os.Remove(zipPath)

	session, err := s.extractAndValidate(ctx, id, zipPath, archiveSize, tempRoot, photosRoot, photosDir)
	if err != nil {
		return restoreSummary{}, err
	}

	s.mu.Lock()
	s.restores[id] = session
	s.mu.Unlock()

	success = true
	return session.summary(), nil
}

func receiveArchive(body io.Reader, destination string, limit, declaredSize int64) (int64, error) {
	if limit <= 0 {
		limit = defaultMaxArchiveBytes
	}
	if declaredSize > limit {
		return 0, ErrArchiveTooBig
	}

	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, fmt.Errorf("create uploaded archive: %w", err)
	}

	written, copyErr := io.Copy(file, io.LimitReader(body, limit+1))
	closeErr := file.Close()
	if copyErr != nil {
		var maxErr *http.MaxBytesError
		if errors.As(copyErr, &maxErr) {
			return 0, ErrArchiveTooBig
		}
		return 0, fmt.Errorf("save uploaded archive: %w", copyErr)
	}
	if closeErr != nil {
		return 0, fmt.Errorf("save uploaded archive: %w", closeErr)
	}
	if written > limit {
		return 0, ErrArchiveTooBig
	}
	if written == 0 {
		return 0, fmt.Errorf("%w: пустой файл", ErrBadArchive)
	}
	return written, nil
}

func (s *Service) extractAndValidate(
	ctx context.Context,
	id, zipPath string,
	archiveSize int64,
	tempRoot, photosRoot, photosDir string,
) (*restoreSession, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadArchive, err)
	}
	defer reader.Close()

	if len(reader.File) == 0 {
		return nil, fmt.Errorf("%w: архив пуст", ErrBadArchive)
	}
	if len(reader.File) > maxZipEntries {
		return nil, fmt.Errorf("%w: слишком много файлов", ErrBadArchive)
	}

	maxUncompressed := s.cfg.MaxArchiveBytes * 4
	if maxUncompressed < 64<<20 {
		maxUncompressed = 64 << 20
	}

	seen := make(map[string]struct{})
	var manifestData []byte
	var dbInfo *fileDigest
	var dbSize int64
	photos := make(map[string]photoInfo)
	var totalUncompressed int64

	dbPath := filepath.Join(tempRoot, "orders.db")

	for _, file := range reader.File {
		name := file.Name
		if err := validateArchiveName(name); err != nil {
			return nil, err
		}

		if file.FileInfo().IsDir() {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 || !file.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: недопустимый тип файла в архиве", ErrBadArchive)
		}
		if file.Flags&0x1 != 0 {
			return nil, fmt.Errorf("%w: зашифрованные ZIP-файлы не поддерживаются", ErrBadArchive)
		}
		if file.Method != zip.Store && file.Method != zip.Deflate {
			return nil, fmt.Errorf("%w: неподдерживаемый метод сжатия", ErrBadArchive)
		}
		if file.UncompressedSize64 > uint64(maxUncompressed) {
			return nil, fmt.Errorf("%w: слишком большой распакованный файл", ErrBadArchive)
		}
		if file.UncompressedSize64 > 1<<20 && file.CompressedSize64 > 0 &&
			file.UncompressedSize64/file.CompressedSize64 > 2000 {
			return nil, fmt.Errorf("%w: подозрительная степень сжатия", ErrBadArchive)
		}
		totalUncompressed += int64(file.UncompressedSize64)
		if totalUncompressed > maxUncompressed {
			return nil, fmt.Errorf("%w: слишком большой распакованный объём", ErrBadArchive)
		}

		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("%w: дублирующийся файл %s", ErrBadArchive, name)
		}
		seen[name] = struct{}{}

		switch {
		case name == "manifest.json":
			data, err := readZipEntry(file, maxManifestBytes)
			if err != nil {
				return nil, err
			}
			manifestData = data
		case name == "database/orders.db":
			digest, size, err := extractFile(file, dbPath, 0o600)
			if err != nil {
				return nil, err
			}
			dbInfo = &digest
			dbSize = size
		case strings.HasPrefix(name, "uploads/dishes/"):
			photoName := strings.TrimPrefix(name, "uploads/dishes/")
			if !validPhotoName(photoName) {
				return nil, fmt.Errorf("%w: недопустимое имя фотографии", ErrBadArchive)
			}
			destination := filepath.Join(photosDir, photoName)
			digest, size, err := extractFile(file, destination, 0o644)
			if err != nil {
				return nil, err
			}
			photos[photoName] = photoInfo{
				Name:   photoName,
				Size:   size,
				SHA256: digest.hash,
			}
		default:
			return nil, fmt.Errorf("%w: недопустимый файл в архиве: %s", ErrBadArchive, name)
		}
	}

	if manifestData == nil {
		return nil, fmt.Errorf("%w: отсутствует manifest.json", ErrBadArchive)
	}
	if dbInfo == nil {
		return nil, fmt.Errorf("%w: отсутствует database/orders.db", ErrBadArchive)
	}

	var archiveManifest manifest
	if err := json.Unmarshal(manifestData, &archiveManifest); err != nil {
		return nil, fmt.Errorf("%w: manifest.json: %v", ErrBadArchive, err)
	}
	if err := validateManifest(archiveManifest); err != nil {
		return nil, err
	}
	if archiveManifest.Database.Size != dbSize || !strings.EqualFold(archiveManifest.Database.SHA256, dbInfo.hash) {
		return nil, fmt.Errorf("%w: контрольная сумма базы не совпадает", ErrBadArchive)
	}
	if err := comparePhotos(archiveManifest.Photos, photos); err != nil {
		return nil, err
	}

	if err := s.validateDatabaseFile(ctx, dbPath, photos, archiveManifest); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	return &restoreSession{
		ID:           id,
		CreatedAt:    now,
		ExpiresAt:    now.Add(s.cfg.RestoreTTL),
		ArchiveSize:  archiveSize,
		DatabaseSize: dbSize,
		TempRoot:     tempRoot,
		PhotosRoot:   photosRoot,
		PhotosDir:    photosDir,
		DBPath:       dbPath,
		Manifest:     archiveManifest,
	}, nil
}

func validateArchiveName(name string) error {
	if name == "" || strings.ContainsRune(name, 0) || strings.Contains(name, `\`) {
		return fmt.Errorf("%w: недопустимое имя файла", ErrBadArchive)
	}
	if !zipNamePattern.MatchString(name) {
		return fmt.Errorf("%w: недопустимые символы в имени файла", ErrBadArchive)
	}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") {
		return fmt.Errorf("%w: path traversal", ErrBadArchive)
	}

	clean := path.Clean(name)
	if clean != name || clean == "." || clean == ".." {
		return fmt.Errorf("%w: path traversal", ErrBadArchive)
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("%w: path traversal", ErrBadArchive)
		}
	}
	return nil
}

func readZipEntry(file *zip.File, limit int64) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: не удалось открыть %s", ErrBadArchive, file.Name)
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: не удалось прочитать %s", ErrBadArchive, file.Name)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: %s слишком большой", ErrBadArchive, file.Name)
	}
	return data, nil
}

func extractFile(file *zip.File, destination string, mode os.FileMode) (fileDigest, int64, error) {
	reader, err := file.Open()
	if err != nil {
		return fileDigest{}, 0, fmt.Errorf("%w: не удалось открыть %s", ErrBadArchive, file.Name)
	}
	defer reader.Close()

	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return fileDigest{}, 0, fmt.Errorf("create extracted file: %w", err)
	}

	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(out, hash), io.LimitReader(reader, int64(file.UncompressedSize64)+1))
	if err != nil {
		out.Close()
		_ = os.Remove(destination)
		return fileDigest{}, 0, fmt.Errorf("%w: не удалось распаковать %s", ErrBadArchive, file.Name)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(destination)
		return fileDigest{}, 0, fmt.Errorf("close extracted file: %w", err)
	}
	if size != int64(file.UncompressedSize64) {
		_ = os.Remove(destination)
		return fileDigest{}, 0, fmt.Errorf("%w: неверный размер %s", ErrBadArchive, file.Name)
	}

	return fileDigest{size: size, hash: hex.EncodeToString(hash.Sum(nil))}, size, nil
}

func validateManifest(value manifest) error {
	if value.Format != manifestFormat {
		return fmt.Errorf("%w: неизвестный формат", ErrBadArchive)
	}
	if value.FormatVersion != manifestFormatVersion {
		return fmt.Errorf("%w: версия формата %d не поддерживается", ErrIncompatible, value.FormatVersion)
	}
	if value.SchemaVersion != database.SchemaVersion {
		return fmt.Errorf("%w: версия схемы %d, ожидается %d", ErrIncompatible, value.SchemaVersion, database.SchemaVersion)
	}
	if value.CreatedAt.IsZero() {
		return fmt.Errorf("%w: не указана дата создания", ErrBadArchive)
	}
	if value.SchemaFingerprint == "" {
		return fmt.Errorf("%w: отсутствует fingerprint схемы", ErrIncompatible)
	}
	if value.Database.Path != "database/orders.db" {
		return fmt.Errorf("%w: неверный путь к базе в манифесте", ErrBadArchive)
	}
	if value.Database.Size <= 0 || value.Database.SHA256 == "" {
		return fmt.Errorf("%w: неверные метаданные базы", ErrBadArchive)
	}
	for _, table := range businessTableNames() {
		if value.Counts[table] < 0 {
			return fmt.Errorf("%w: неверное количество строк", ErrBadArchive)
		}
	}
	return nil
}

func comparePhotos(expected []photoInfo, actual map[string]photoInfo) error {
	if len(expected) != len(actual) {
		return fmt.Errorf("%w: количество фотографий не совпадает", ErrBadArchive)
	}

	seen := make(map[string]struct{}, len(expected))
	for _, photo := range expected {
		if !validPhotoName(photo.Name) {
			return fmt.Errorf("%w: недопустимое имя фотографии", ErrBadArchive)
		}
		if _, exists := seen[photo.Name]; exists {
			return fmt.Errorf("%w: дублирующаяся фотография", ErrBadArchive)
		}
		seen[photo.Name] = struct{}{}

		found, exists := actual[photo.Name]
		if !exists {
			return fmt.Errorf("%w: в архиве отсутствует фотография %s", ErrBadArchive, photo.Name)
		}
		if found.Size != photo.Size || !strings.EqualFold(found.SHA256, photo.SHA256) {
			return fmt.Errorf("%w: контрольная сумма фотографии %s не совпадает", ErrBadArchive, photo.Name)
		}
	}
	return nil
}

func (s *Service) validateDatabaseFile(
	ctx context.Context,
	dbPath string,
	photos map[string]photoInfo,
	archiveManifest manifest,
) error {
	db, err := openSQLite(dbPath, false)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadArchive, err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("%w: не удалось прочитать версию схемы", ErrBadArchive)
	}
	if version != database.SchemaVersion {
		return fmt.Errorf("%w: версия схемы %d, ожидается %d", ErrIncompatible, version, database.SchemaVersion)
	}

	if err := checkIntegrity(ctx, db); err != nil {
		return err
	}
	if err := checkForeignKeys(ctx, db); err != nil {
		return err
	}

	fingerprint, err := schemaFingerprint(ctx, db)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadArchive, err)
	}
	if !strings.EqualFold(fingerprint, archiveManifest.SchemaFingerprint) {
		return fmt.Errorf("%w: схема базы не совпадает с манифестом", ErrIncompatible)
	}
	currentFingerprint, err := schemaFingerprint(ctx, s.cfg.DB)
	if err != nil {
		return fmt.Errorf("read current schema: %w", err)
	}
	if !strings.EqualFold(fingerprint, currentFingerprint) {
		return fmt.Errorf("%w: схема архива не совпадает с текущей схемой приложения", ErrIncompatible)
	}

	counts, err := businessCounts(ctx, db)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadArchive, err)
	}
	for table, expected := range archiveManifest.Counts {
		if table == "photos" {
			continue
		}
		if _, ok := tableByName(table); !ok {
			continue
		}
		if counts[table] != expected {
			return fmt.Errorf("%w: количество строк в %s не совпадает", ErrBadArchive, table)
		}
	}
	for _, table := range businessTables {
		if _, ok := archiveManifest.Counts[table.name]; !ok {
			return fmt.Errorf("%w: в манифесте нет количества для %s", ErrBadArchive, table.name)
		}
	}

	if exists, err := tableExists(ctx, db, "notification_outbox"); err != nil {
		return err
	} else if exists {
		outboxCount, err := countRows(ctx, db, "notification_outbox")
		if err != nil {
			return fmt.Errorf("%w: %v", ErrBadArchive, err)
		}
		if outboxCount != 0 {
			return fmt.Errorf("%w: notification_outbox не должна входить в архив", ErrInvalidData)
		}
	}
	if exists, err := tableExists(ctx, db, "restore_state"); err != nil {
		return err
	} else if exists {
		count, err := countRows(ctx, db, "restore_state")
		if err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("%w: служебная таблица не должна входить в архив", ErrInvalidData)
		}
	}

	if err := validatePhotoReferences(ctx, db, photos); err != nil {
		return err
	}

	return nil
}

func validatePhotoReferences(ctx context.Context, db *sql.DB, photos map[string]photoInfo) error {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT photo_url FROM dishes WHERE photo_url <> ''`)
	if err != nil {
		return fmt.Errorf("%w: не удалось проверить фотографии: %v", ErrBadArchive, err)
	}
	defer rows.Close()

	for rows.Next() {
		var photoURL string
		if err := rows.Scan(&photoURL); err != nil {
			return fmt.Errorf("%w: не удалось прочитать photo_url", ErrBadArchive)
		}
		name, ok := photoName(photoURL)
		if !ok {
			return fmt.Errorf("%w: некорректная ссылка на фотографию в базе", ErrInvalidData)
		}
		if _, exists := photos[name]; !exists {
			return fmt.Errorf("%w: в архиве отсутствует фотография %s", ErrInvalidData, name)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: не удалось проверить фотографии: %v", ErrBadArchive, err)
	}
	return nil
}

func (session *restoreSession) summary() restoreSummary {
	counts := cloneCounts(session.Manifest.Counts)
	if counts == nil {
		counts = make(map[string]int64)
	}
	counts["photos"] = int64(len(session.Manifest.Photos))

	return restoreSummary{
		ID:            session.ID,
		CreatedAt:     session.Manifest.CreatedAt,
		ExpiresAt:     session.ExpiresAt,
		ArchiveSize:   session.ArchiveSize,
		DatabaseSize:  session.DatabaseSize,
		SchemaVersion: session.Manifest.SchemaVersion,
		Counts:        counts,
		PhotoCount:    len(session.Manifest.Photos),
	}
}
