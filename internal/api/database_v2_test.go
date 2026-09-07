package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestV2DatabaseResourcesHidePathsAndTopology(t *testing.T) {
	app := newTestApp(t)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	backupDir := app.cfg().DatabaseBackupDir
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(backupDir, "sample.json")
	if err := os.WriteFile(backupPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v2/admin/database/status"},
		{http.MethodGet, "/api/v2/admin/database/backups"},
	}
	for _, tc := range cases {
		response := doJSON(app, tc.method, tc.path, "", admin)
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, response.Code, response.Body.String())
		}
		body := response.Body.String()
		if strings.Contains(body, backupDir) || strings.Contains(body, backupPath) || strings.Contains(body, "state_file") || strings.Contains(body, "backup_dir") {
			t.Fatalf("%s %s exposed database implementation detail: %s", tc.method, tc.path, body)
		}
	}
}

func TestV2DatabaseResourcesRequireAdmin(t *testing.T) {
	app := newTestApp(t)
	user := registerAndLogin(t, app, "database-user", "User123456")
	response := doJSON(app, http.MethodGet, "/api/v2/admin/database/status", "", user)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-admin database access status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestV2DatabaseBackupMutationHidesPath(t *testing.T) {
	app := newTestApp(t)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	response := doJSON(app, http.MethodPost, "/api/v2/admin/database/backup", `{"note":"v2 test"}`, admin)
	if response.Code != http.StatusOK {
		t.Fatalf("database backup status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), app.cfg().DatabaseBackupDir) || strings.Contains(response.Body.String(), `"path"`) {
		t.Fatalf("database backup response exposed path: %s", response.Body.String())
	}
}
