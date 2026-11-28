package backup

import "time"

type manifest struct {
	Format            string           `json:"format"`
	FormatVersion     int              `json:"format_version"`
	CreatedAt         time.Time        `json:"created_at"`
	AppVersion        string           `json:"app_version"`
	SchemaVersion     int              `json:"schema_version"`
	SchemaFingerprint string           `json:"schema_fingerprint"`
	Database          databaseInfo     `json:"database"`
	Counts            map[string]int64 `json:"counts"`
	Photos            []photoInfo      `json:"photos"`
}

type databaseInfo struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type photoInfo struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type backupSummary struct {
	ID            string           `json:"id"`
	CreatedAt     time.Time        `json:"created_at"`
	ExpiresAt     time.Time        `json:"expires_at"`
	DownloadedAt  time.Time        `json:"downloaded_at,omitempty"`
	ConfirmedAt   time.Time        `json:"confirmed_at,omitempty"`
	Size          int64            `json:"size,omitempty"`
	SHA256        string           `json:"sha256,omitempty"`
	SchemaVersion int              `json:"schema_version,omitempty"`
	Counts        map[string]int64 `json:"counts,omitempty"`
	PhotoCount    int              `json:"photo_count,omitempty"`
}

type restoreSummary struct {
	ID            string           `json:"restore_id"`
	CreatedAt     time.Time        `json:"created_at"`
	ExpiresAt     time.Time        `json:"expires_at"`
	ArchiveSize   int64            `json:"archive_size"`
	DatabaseSize  int64            `json:"database_size"`
	SchemaVersion int              `json:"schema_version"`
	Counts        map[string]int64 `json:"counts"`
	PhotoCount    int              `json:"photo_count"`
}

type applyRequest struct {
	Confirmation   string `json:"confirmation"`
	SafetyBackupID string `json:"safety_backup_id"`
}
