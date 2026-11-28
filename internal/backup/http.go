package backup

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const maxJSONBodyBytes = 1 << 20

func (s *Service) MaxArchiveBytes() int64 {
	return s.cfg.MaxArchiveBytes
}

func (s *Service) HandlePrepareBackup(w http.ResponseWriter, r *http.Request) {
	if s.rejectCrossSite(w, r) {
		return
	}

	summary, err := s.PrepareBackup()
	if err != nil {
		writeBackupError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, summary)
}

func (s *Service) HandleBackupInfo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	summary, err := s.BackupInfo(id)
	if err != nil {
		writeBackupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Service) HandleConfirmBackup(w http.ResponseWriter, r *http.Request) {
	if s.rejectCrossSite(w, r) {
		return
	}

	id := r.PathValue("id")
	summary, err := s.ConfirmBackup(id)
	if err != nil {
		writeBackupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Service) HandleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	download, err := s.BuildDownload(r.Context(), id)
	if err != nil {
		writeBackupError(w, err)
		return
	}
	defer download.Close()

	filename := "food-service-backup-" + download.session.CreatedAt.UTC().Format("20060102-150405") + "Z.zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	if err := download.Write(w); err != nil {
		// Headers are already sent. The client receives a truncated download;
		// log the real error and let the administrator retry.
		log.Printf("backup: stream archive %s: %v", id, err)
	}
}

func (s *Service) HandleValidateRestore(w http.ResponseWriter, r *http.Request) {
	if s.rejectCrossSite(w, r) {
		return
	}

	limit := s.MaxArchiveBytes()
	r.Body = http.MaxBytesReader(w, r.Body, limit+1)
	summary, err := s.ValidateRestore(r.Context(), io.Reader(r.Body), r.ContentLength)
	if err != nil {
		writeBackupError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

func (s *Service) HandleApplyRestore(w http.ResponseWriter, r *http.Request) {
	if s.rejectCrossSite(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	var request applyRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Некорректные данные подтверждения.", http.StatusBadRequest)
		return
	}

	summary, err := s.ApplyRestore(r.Context(), r.PathValue("id"), request.Confirmation, request.SafetyBackupID)
	if err != nil {
		writeBackupError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":    "Данные успешно восстановлены.",
		"restore_id": summary.ID,
		"counts":     summary.Counts,
		"photos":     summary.PhotoCount,
	})
}

func (s *Service) HandleCancelRestore(w http.ResponseWriter, r *http.Request) {
	if s.rejectCrossSite(w, r) {
		return
	}

	id := r.PathValue("id")
	s.opMu.Lock()
	s.mu.Lock()
	session := s.restores[id]
	delete(s.restores, id)
	s.mu.Unlock()

	if session != nil {
		_ = removeAll(session.TempRoot)
		_ = removeAll(session.PhotosRoot)
	}
	s.opMu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

func removeAll(path string) error {
	if path == "" {
		return nil
	}
	return os.RemoveAll(path)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("backup: encode response: %v", err)
	}
}

func writeBackupError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "Внутренняя ошибка операции с резервной копией."

	switch {
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
		message = err.Error()
	case errors.Is(err, ErrConflict):
		status = http.StatusConflict
		message = err.Error()
	case errors.Is(err, ErrArchiveTooBig):
		status = http.StatusRequestEntityTooLarge
		message = "Архив слишком большой. Уменьшите размер резервной копии."
	case errors.Is(err, ErrBadPhrase):
		status = http.StatusBadRequest
		message = "Введите точную фразу: ЗАМЕНИТЬ ДАННЫЕ"
	case errors.Is(err, ErrNotDownloaded):
		status = http.StatusConflict
		message = "Сначала скачайте текущую резервную копию."
	case errors.Is(err, ErrNotConfirmed):
		status = http.StatusConflict
		message = "Подтвердите, что страховочная копия сохранена на компьютере."
	case errors.Is(err, ErrIncompatible):
		status = http.StatusConflict
		message = err.Error()
	case errors.Is(err, ErrBadArchive), errors.Is(err, ErrInvalidData):
		status = http.StatusBadRequest
		message = err.Error()
	default:
		log.Printf("backup API error: %v", err)
	}

	http.Error(w, message, status)
}

func (s *Service) rejectCrossSite(w http.ResponseWriter, r *http.Request) bool {
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")), "cross-site") {
		rejectRequest(w)
		return true
	}

	rawOrigin := strings.TrimSpace(r.Header.Get("Origin"))
	if rawOrigin == "" {
		return false
	}

	origin, err := url.Parse(rawOrigin)
	if err != nil {
		rejectRequest(w)
		return true
	}
	originScheme, originHostPort, originOK := normalizeOrigin(origin)
	expectedScheme, expectedHostPort, expectedOK := s.expectedOrigin(r)
	if !originOK || !expectedOK ||
		originScheme != expectedScheme ||
		originHostPort != expectedHostPort {
		rejectRequest(w)
		return true
	}

	return false
}

func rejectRequest(w http.ResponseWriter) {
	http.Error(w, "Запрос отклонён.", http.StatusForbidden)
}

// expectedOrigin prefers the configured public URL: it does not depend on
// client-supplied headers. When it is not configured (local development), the
// expected scheme and authority are derived from the request, including the
// scheme reported by the reverse proxy in X-Forwarded-Proto.
func (s *Service) expectedOrigin(r *http.Request) (string, string, bool) {
	if s.cfg.PublicBaseURL != "" {
		parsed, err := url.Parse(s.cfg.PublicBaseURL)
		if err != nil || parsed.User != nil {
			return "", "", false
		}

		scheme := strings.ToLower(parsed.Scheme)
		if scheme != "http" && scheme != "https" {
			return "", "", false
		}
		hostPort, ok := normalizeHostPort(parsed.Host, scheme)
		if !ok {
			return "", "", false
		}
		return scheme, hostPort, true
	}

	scheme := requestScheme(r)
	if scheme == "" {
		return "", "", false
	}
	hostPort, ok := normalizeHostPort(r.Host, scheme)
	if !ok {
		return "", "", false
	}
	return scheme, hostPort, true
}

func normalizeOrigin(origin *url.URL) (string, string, bool) {
	scheme := strings.ToLower(origin.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", "", false
	}
	if origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return "", "", false
	}
	hostPort, ok := normalizeHostPort(origin.Host, scheme)
	if !ok {
		return "", "", false
	}
	return scheme, hostPort, true
}

func normalizeHostPort(authority, scheme string) (string, bool) {
	authority = strings.TrimSpace(authority)
	if authority == "" {
		return "", false
	}

	parsed, err := url.Parse("//" + authority)
	if err != nil || parsed.Host == "" || parsed.User != nil ||
		parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", false
	}
	port := parsed.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return net.JoinHostPort(host, port), true
}

func requestScheme(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		first := strings.ToLower(strings.TrimSpace(strings.Split(forwarded, ",")[0]))
		if first == "http" || first == "https" {
			return first
		}
		return ""
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
