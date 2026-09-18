package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"strings"
	"testing"
)

func TestV2ConfigResourcesDoNotExposeFilesystemPaths(t *testing.T) {
	app := newTestApp(t)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	configPath := filepath.Join(t.TempDir(), "config.toml")
	app.cfg().ConfigFile = configPath
	// 首个请求会按文件 signature 触发 reloadConfigIfChanged：如果这里写一个只有
	// server_name 的残缺配置，热重载会把 databases_dir / backup_dir 冲成默认值，
	// configBackupDir() 随之改址，下面预置的 sample.toml 就找不到了。因此磁盘
	// 配置必须把路径与管理员名单一并写全（与 TestConfigAdminBackupRestoreAndDelete
	// 同口径）。
	content := "[Global]\ndatabases_dir = " + strconv.Quote(app.cfg().DatabaseDir) + "\n\n" +
		"[Admin]\nadmin_usernames = \"admin\"\n\n" +
		"[Database]\ndriver = " + strconv.Quote(app.cfg().DatabaseDriver) + "\nbackup_dir = " + strconv.Quote(app.cfg().DatabaseBackupDir) + "\nstate_file = " + strconv.Quote(app.cfg().StateFile) + "\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {

		t.Fatal(err)
	}
	backupDir := app.configBackupDir()
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(backupDir, "sample.toml")
	if err := os.WriteFile(backupPath, []byte("[Global]\nserver_name = \"backup\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v2/admin/config/toml"},
		{http.MethodGet, "/api/v2/admin/config/backups"},
		{http.MethodGet, "/api/v2/admin/config/backups/sample.toml"},
	}
	for _, tc := range cases {
		response := doJSON(app, tc.method, tc.path, "", admin)
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, response.Code, response.Body.String())
		}
		body := response.Body.String()
		if strings.Contains(body, configPath) || strings.Contains(body, backupDir) || strings.Contains(body, backupPath) {
			t.Fatalf("%s %s exposed filesystem path: %s", tc.method, tc.path, body)
		}
	}
}

func TestV2ConfigResourcesRequireAdmin(t *testing.T) {
	app := newTestApp(t)
	user := registerAndLogin(t, app, "config-user", "User123456")
	response := doJSON(app, http.MethodGet, "/api/v2/admin/config/schema", "", user)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-admin config access status=%d body=%s", response.Code, response.Body.String())
	}
}
