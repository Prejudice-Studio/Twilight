package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/migration"
)

func TestTwoFactorFileKeyEnrollmentLoginRestoreAndMigration(t *testing.T) {
	t.Setenv("TWILIGHT_TWO_FACTOR_KEY", "")
	app := newTestApp(t)
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("f", 32)))
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("TWILIGHT_CONFIG_LOCAL_FILE", path+".local")
	if err := os.WriteFile(path, []byte("[Security]\ntwo_factor_enrollment_enabled = true\ntwo_factor_key = \""+key+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	app.cfg().ConfigFile = path
	if _, err := app.reloadConfig(); err != nil {
		t.Fatal(err)
	}
	u, _, codes, _ := factorEnrollWithCurrentKey(t, app)
	request := factorChallenge(t, app, u)
	login := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": request, "code": codes[0], "recovery": true}, nil)
	factorData(t, login)
	status := factorData(t, factorRequest(app, "GET", "/api/v2/settings/two-factor", nil, login.Result().Cookies()))
	if status["key_ready"] != true || status["enabled"] != true {
		t.Fatal("file key unavailable after enrollment")
	}
	backup, err := app.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().LoadSnapshot(backup, "invalid"); err == nil {
		t.Fatal("restore accepted invalid file key")
	}
	if err := app.store().LoadSnapshot(backup, cfg.TwoFactorKey); err != nil {
		t.Fatal(err)
	}
	files, err := app.store().ExportMigrationFiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	archive := migration.Archive{Files: map[string][]byte{}}
	for _, file := range files {
		archive.Files[file.Path] = file.Data
	}
	if _, err := app.store().ImportMigrationArchive(context.Background(), archive, cfg.TwoFactorKey); err != nil {
		t.Fatal(err)
	}
	request = factorChallenge(t, app, u)
	factorData(t, factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": request, "code": codes[1], "recovery": true}, nil))
}

func TestTwoFactorFileKeyMaskedPreservedAndNotExported(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("m", 32)))
	for _, content := range []string{
		"[Security]\ntwo_factor_key = \"" + key + "\"\n",
		"Security.two_factor_key = \"" + key + "\"\n",
		"Security = { two_factor_key = \"" + key + "\" }\n",
		"[security]\nTWO_FACTOR_KEY = \"" + key + "\"\n",
	} {
		masked, err := maskTOMLSecrets(content)
		if err != nil || strings.Contains(masked, key) || !strings.Contains(masked, secretMaskValue) {
			t.Fatal("server key was not masked", err)
		}
		restored, err := restoreTOMLSecrets(masked, content, nil)
		if err != nil || restored != content {
			t.Fatal("masked save lost server key", err)
		}
		if err := preserveTwoFactorConfigKey(restored, content); err != nil {
			t.Fatal(err)
		}
		for _, candidate := range []string{"[Security]\n", strings.ReplaceAll(content, key, "changed")} {
			if err := preserveTwoFactorConfigKey(candidate, content); err == nil {
				t.Fatal("web edit changed or deleted the deployment key")
			}
		}
	}
	if err := preserveTwoFactorConfigKey("[Security]\ntwo_factor_key = \"added\"\n", ""); err == nil {
		t.Fatal("web edit added a deployment key")
	}
	if _, err := twoFactorConfigKeyValue("[Security]\ntwo_factor_key = \"one\"\nTWO_FACTOR_KEY = \"two\"\n"); err == nil {
		t.Fatal("ambiguous deployment keys accepted")
	}
	for _, encrypted := range []bool{false, true} {
		files, err := collectMigrationConfig(config.Config{TwoFactorKey: key}, encrypted)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.Contains(string(f.Data), key) || strings.Contains(string(f.Data), "two_factor_key") {
				t.Fatal("deployment key included in migration")
			}
		}
	}
}

func TestTwoFactorFileKeySurvivesInitialSetup(t *testing.T) {
	t.Setenv("TWILIGHT_TWO_FACTOR_KEY", "")
	app := newSetupTestApp(t)
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("i", 32)))
	if err := os.WriteFile(app.cfg().ConfigFile, []byte("SetupMode = true\n[Security]\ntwo_factor_key = \""+key+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	response := doJSONWithHeaders(app, http.MethodPost, "/api/v2/setup/complete", `{"admin":{"username":"owner","password":"Owner123456"}}`, nil, setupIntentHeaders())
	if response.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", response.Code, response.Body.String())
	}
	file, err := config.LoadFileOnly(app.cfg().ConfigFile)
	if err != nil || file.TwoFactorKey != key || app.cfg().TwoFactorKey != key {
		t.Fatal("setup lost the deployment key", err)
	}
	if file.SetupMode || strings.Contains(response.Body.String(), key) {
		t.Fatal("setup stayed open or exposed the key")
	}
}

func TestTwoFactorConfigKeyLogRedaction(t *testing.T) {
	const secret = "test-deployment-key-value"
	for _, name := range []string{"two_factor_key", "TWILIGHT_TWO_FACTOR_KEY", "TwoFactorKey", "two-factor-key"} {
		if !sensitiveLogKey(name) {
			t.Fatal("deployment key not considered sensitive")
		}
		for _, value := range []string{secret, `"` + secret + ` with spaces"`} {
			if strings.Contains(redactSensitiveText(name+"="+value), secret) {
				t.Fatal("deployment key exposed in log text")
			}
		}
	}
}

func TestTwoFactorFileKeySurvivesConfigSave(t *testing.T) {
	t.Setenv("TWILIGHT_TWO_FACTOR_KEY", "")
	app := newTestApp(t)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	path := filepath.Join(t.TempDir(), "config.toml")
	app.cfg().ConfigFile = path
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 32)))
	content := fmt.Sprintf("[Security]\ntwo_factor_key = %q\n[Admin]\nusernames = [\"admin\"]\n", key)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, status, message := app.patchConfigSections("", map[string]any{"Security": map[string]any{"two_factor_enrollment_enabled": true}})
	if status != http.StatusOK {
		t.Fatalf("schema save: %d %s", status, message)
	}
	if app.cfg().TwoFactorKey != key {
		t.Fatal("schema save or hot reload lost key")
	}
	masked, err := maskTOMLSecrets(content)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"content": masked})
	if err != nil {
		t.Fatal(err)
	}
	if response := doJSON(app, http.MethodPut, "/api/v2/admin/config/toml", string(payload), admin); response.Code != http.StatusOK {
		t.Fatalf("masked TOML save: %d %s", response.Code, response.Body.String())
	}
	for _, candidate := range []string{"[Security]\ntwo_factor_enrollment_enabled=true\n", strings.ReplaceAll(content, key, "changed")} {
		if _, status, _ := app.saveConfigContent(candidate); status != http.StatusBadRequest {
			t.Fatal("web save replaced key", status)
		}
	}
}
