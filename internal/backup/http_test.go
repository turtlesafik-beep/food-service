package backup

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPBackupAndRestoreFlow(t *testing.T) {
	env := newTestEnv(t, 0)
	dishID := seedData(t, env, "http.jpg")
	writePhoto(t, env.uploadDir, "http.jpg", []byte("jpeg-data"))

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/admin/backups", env.service.HandlePrepareBackup)
	mux.HandleFunc("GET /api/admin/backups/{id}/download", env.service.HandleDownloadBackup)
	mux.HandleFunc("POST /api/admin/backups/{id}/confirm", env.service.HandleConfirmBackup)
	mux.HandleFunc("POST /api/admin/restores/validate", env.service.HandleValidateRestore)
	mux.HandleFunc("POST /api/admin/restores/{id}/apply", env.service.HandleApplyRestore)
	server := httptest.NewServer(mux)
	defer server.Close()

	response, err := http.Post(server.URL+"/api/admin/backups", "application/json", nil)
	if err != nil {
		t.Fatalf("prepare via HTTP: %v", err)
	}
	var prepared backupSummary
	if err := json.NewDecoder(response.Body).Decode(&prepared); err != nil {
		t.Fatalf("decode backup summary: %v", err)
	}
	response.Body.Close()

	response, err = http.Get(server.URL + "/api/admin/backups/" + prepared.ID + "/download")
	if err != nil {
		t.Fatalf("download via HTTP: %v", err)
	}
	archive, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if response.StatusCode != http.StatusOK || len(archive) == 0 {
		t.Fatalf("download failed: status=%d size=%d", response.StatusCode, len(archive))
	}

	response, err = http.Post(server.URL+"/api/admin/restores/validate", "application/zip", bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("validate via HTTP: %v", err)
	}
	var validated restoreSummary
	if err := json.NewDecoder(response.Body).Decode(&validated); err != nil {
		t.Fatalf("decode restore summary: %v", err)
	}
	response.Body.Close()

	response, err = http.Post(server.URL+"/api/admin/backups", "application/json", nil)
	if err != nil {
		t.Fatalf("prepare safety backup: %v", err)
	}
	var safety backupSummary
	if err := json.NewDecoder(response.Body).Decode(&safety); err != nil {
		t.Fatalf("decode safety summary: %v", err)
	}
	response.Body.Close()

	response, err = http.Get(server.URL + "/api/admin/backups/" + safety.ID + "/download")
	if err != nil {
		t.Fatalf("download safety backup: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()

	response, err = http.Post(server.URL+"/api/admin/backups/"+safety.ID+"/confirm", "application/json", nil)
	if err != nil {
		t.Fatalf("confirm safety backup: %v", err)
	}
	response.Body.Close()

	payload, _ := json.Marshal(applyRequest{
		Confirmation:   "ЗАМЕНИТЬ ДАННЫЕ",
		SafetyBackupID: safety.ID,
	})
	response, err = http.Post(
		server.URL+"/api/admin/restores/"+validated.ID+"/apply",
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("apply via HTTP: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("apply failed: status=%d body=%s", response.StatusCode, body)
	}

	var count int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM dishes WHERE id = ?`, dishID).Scan(&count); err != nil {
		t.Fatalf("count restored dish: %v", err)
	}
	if count != 1 {
		t.Fatalf("dish was not restored")
	}
}
