package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(config.Config{
		AppName:                      "Twilight Test",
		Version:                      "test",
		Host:                         "127.0.0.1",
		Port:                         0,
		DatabaseDir:                  dir,
		DatabaseDriver:               store.BackendJSON,
		DatabaseBackupDir:            filepath.Join(dir, "backups"),
		StateFile:                    filepath.Join(dir, "state.json"),
		UploadDir:                    filepath.Join(dir, "uploads"),
		MaxUploadSize:                1024 * 1024,
		CORSOrigins:                  []string{"http://localhost:3000"},
		AllowCredential:              true,
		SessionCookie:                "twilight_session",
		SessionTTL:                   time.Hour,
		CookieSameSite:               "lax",
		RegisterEnabled:              true,
		MediaRequestEnabled:          true,
		MaxConcurrentRequestsPerUser: 3,
		SigninEnabled:                true,
		SigninCurrencyName:           "积分",
		SigninDailyMin:               1,
		SigninDailyMax:               1,
		SigninResetAfterMiss:         true,
		InviteEnabled:                true,
		InviteMaxDepth:               3,
		InviteLimit:                  10,
		InviteRootUserLimit:          -1,
		InviteDefaultDays:            30,
		PermanentInviteMaxDays:       365,
		MaxDevices:                   5,
		MaxStreams:                   2,
	}, st)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func TestRegcodeWritesBlockedWhenRuntimeDatabaseMismatchesConfig(t *testing.T) {
	app := newTestApp(t)
	app.cfg.DatabaseDriver = store.BackendPostgres

	rec := httptest.NewRecorder()
	if !app.rejectRegcodeWriteIfStorageMismatch(rec) {
		t.Fatal("expected regcode writes to be blocked when configured database differs from active store")
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected conflict status, got %d", rec.Code)
	}

	app.cfg.DatabaseDriver = store.BackendJSON
	rec = httptest.NewRecorder()
	if app.rejectRegcodeWriteIfStorageMismatch(rec) {
		t.Fatal("regcode writes were blocked even though active and configured databases match")
	}
}

func TestAuthFlowAndCSRFMitigation(t *testing.T) {
	app := newTestApp(t)

	register := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	if register.Code != http.StatusCreated {
		t.Fatalf("register status = %d body=%s", register.Code, register.Body.String())
	}

	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", login.Code, login.Body.String())
	}
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	if cookie == nil || !cookie.HttpOnly {
		t.Fatalf("expected httponly session cookie, got %#v", cookie)
	}

	me := doJSON(app, http.MethodGet, "/api/v1/users/me", ``, []*http.Cookie{cookie})
	if me.Code != http.StatusOK {
		t.Fatalf("me status = %d body=%s", me.Code, me.Body.String())
	}
	if me.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing security header")
	}

	blocked := doJSON(app, http.MethodPut, "/api/v1/users/me", `{"email":"a@example.com"}`, []*http.Cookie{cookie})
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("csrf status = %d body=%s", blocked.Code, blocked.Body.String())
	}

	allowed := doJSONWithHeaders(app, http.MethodPut, "/api/v1/users/me", `{"email":"a@example.com"}`, []*http.Cookie{cookie}, map[string]string{"X-Twilight-Client": "webui"})
	if allowed.Code != http.StatusOK {
		t.Fatalf("update status = %d body=%s", allowed.Code, allowed.Body.String())
	}
}

func TestCredentialedCORSRequiresExplicitOrigin(t *testing.T) {
	app := newTestApp(t)
	app.cfg.CORSOrigins = []string{"*"}

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/users/me", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", "PUT")
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)

	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("wildcard CORS origin was allowed: %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}

	app.cfg.CORSOrigins = []string{"https://panel.example/"}
	req = httptest.NewRequest(http.MethodOptions, "/api/v1/users/me", nil)
	req.Header.Set("Origin", "https://panel.example")
	req.Header.Set("Access-Control-Request-Method", "PUT")
	rr = httptest.NewRecorder()
	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("explicit CORS preflight status = %d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "https://panel.example" {
		t.Fatalf("explicit CORS origin not allowed: %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}

	app.cfg.CORSOrigins = []string{"https://panel.example/app"}
	req = httptest.NewRequest(http.MethodOptions, "/api/v1/users/me", nil)
	req.Header.Set("Origin", "https://panel.example")
	req.Header.Set("Access-Control-Request-Method", "PUT")
	rr = httptest.NewRecorder()
	app.ServeHTTP(rr, req)
	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("path-bearing CORS origin was allowed: %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestAPIKeyFlow(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")

	created := doJSONWithHeaders(app, http.MethodPost, "/api/v1/users/me/apikeys", `{"name":"ci","rate_limit":50}`, []*http.Cookie{cookie}, map[string]string{"X-Twilight-Client": "webui"})
	if created.Code != http.StatusOK {
		t.Fatalf("create key status = %d body=%s", created.Code, created.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(created.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	data := env.Data.(map[string]any)
	key, _ := data["key"].(string)
	if !strings.HasPrefix(key, "key-") {
		t.Fatalf("expected plaintext key once, got %q", key)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apikey/info", nil)
	req.Header.Set("X-API-Key", key)
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("apikey info status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFrontendRouteCompatibilityDoesNot404(t *testing.T) {
	app := newTestApp(t)
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/system/info"},
		{http.MethodGet, "/api/v1/system/health"},
		{http.MethodPost, "/api/v1/auth/login"},
		{http.MethodGet, "/api/v1/users/me"},
		{http.MethodGet, "/api/v1/admin/users"},
		{http.MethodGet, "/api/v1/media/search?q=x&source=tmdb"},
		{http.MethodPost, "/api/v1/media/request"},
		{http.MethodGet, "/api/v1/admin/scheduler/jobs"},
		{http.MethodGet, "/api/v1/announcements"},
		{http.MethodGet, "/api/v1/invite/config"},
		{http.MethodGet, "/api/v1/signin/config"},
		{http.MethodGet, "/api/v1/demo/bootstrap"},
		{http.MethodGet, "/api/v1/apikey/info"},
	}
	for _, route := range routes {
		req := httptest.NewRequest(route.method, route.path, nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		if rr.Code == http.StatusNotFound || rr.Code == http.StatusMethodNotAllowed {
			t.Fatalf("%s %s returned %d", route.method, route.path, rr.Code)
		}
	}
}

func TestUploadRejectsNonImage(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("not an image"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/avatar/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Twilight-Client", "webui")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("upload status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminServerIconUploadUpdatesConfig(t *testing.T) {
	app := newTestApp(t)
	app.cfg.ConfigFile = filepath.Join(app.cfg.DatabaseDir, "config.toml")
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "icon.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0})
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/admin/server-icon/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Twilight-Client", "webui")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("server icon upload status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.HasPrefix(app.cfg.ServerIcon, "server-icon/") || !strings.HasSuffix(app.cfg.ServerIcon, ".png") {
		t.Fatalf("server_icon was not updated to uploaded asset: %q", app.cfg.ServerIcon)
	}
	if _, _, ok := app.configuredServerIconPath(); !ok {
		t.Fatalf("uploaded server icon is not readable from configured path: %q", app.cfg.ServerIcon)
	}
}

func TestUploadImageExtensionWhitelist(t *testing.T) {
	allowed := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/gif":  ".gif",
		"image/webp": ".webp",
		"image/bmp":  ".bmp",
	}
	for contentType, expectedExt := range allowed {
		ext, ok := uploadImageExtension(contentType)
		if !ok || ext != expectedExt {
			t.Fatalf("uploadImageExtension(%q) = %q, %v; want %q, true", contentType, ext, ok, expectedExt)
		}
	}

	blocked := []string{"image/svg+xml", "text/html", "application/octet-stream", ""}
	for _, contentType := range blocked {
		if ext, ok := uploadImageExtension(contentType); ok || ext != "" {
			t.Fatalf("uploadImageExtension(%q) = %q, %v; want empty, false", contentType, ext, ok)
		}
	}
}

func TestUploadAssetPathAndFilenameSafety(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")

	validName := "0123456789abcdef.png"
	avatarDir := filepath.Join(app.cfg.UploadDir, "avatar")
	if err := os.MkdirAll(avatarDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(avatarDir, validName), []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, 0o600); err != nil {
		t.Fatal(err)
	}

	valid := doJSON(app, http.MethodGet, "/api/v1/users/assets/avatar/"+validName, ``, []*http.Cookie{cookie})
	if valid.Code != http.StatusOK {
		t.Fatalf("valid asset status=%d body=%s", valid.Code, valid.Body.String())
	}

	invalids := []string{
		"/api/v1/users/assets/avatar/0123456789abcdef.svg",
		"/api/v1/users/assets/avatar/0123456789abcdeg.png",
		"/api/v1/users/assets/avatar/%2e%2e",
		"/api/v1/users/assets/profile/" + validName,
	}
	for _, path := range invalids {
		resp := doJSON(app, http.MethodGet, path, ``, []*http.Cookie{cookie})
		if resp.Code != http.StatusNotFound {
			t.Fatalf("invalid asset %s status=%d body=%s", path, resp.Code, resp.Body.String())
		}
	}
}

func TestProtectedAdminConfigHiddenPreservedAndApplied(t *testing.T) {
	app := newTestApp(t)
	app.cfg.ConfigFile = filepath.Join(app.cfg.DatabaseDir, "config.toml")
	databaseConfig := "[Database]\n" +
		"driver = \"json\"\n" +
		"state_file = " + strconv.Quote(app.cfg.StateFile) + "\n" +
		"backup_dir = " + strconv.Quote(app.cfg.DatabaseBackupDir) + "\n"
	existing := "[Global]\nserver_name = \"old\"\n\n" + databaseConfig + "\nadmin_uids = \"2\"\nadmin_usernames = \"alice\"\n"
	if err := os.WriteFile(app.cfg.ConfigFile, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	if shown := stripProtectedAdminConfig(existing); strings.Contains(shown, "admin_uids") || strings.Contains(shown, "admin_usernames") {
		t.Fatalf("protected admin config leaked: %s", shown)
	}
	info, status, message := app.saveConfigContent("[Global]\nserver_name = \"new\"\n\n" + databaseConfig + "\n[Admin]\nadmin_uids = \"999\"\nadmin_usernames = \"mallory\"\n")
	if status != http.StatusOK {
		t.Fatalf("saveConfigContent status=%d message=%s info=%v", status, message, info)
	}
	data, err := os.ReadFile(app.cfg.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "999") || strings.Contains(content, "mallory") {
		t.Fatalf("submitted protected admin config was not stripped: %s", content)
	}
	if !strings.Contains(content, `admin_uids = "2"`) || !strings.Contains(content, `admin_usernames = "alice"`) {
		t.Fatalf("existing protected admin config was not preserved: %s", content)
	}

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"owner","password":"owner123456"}`, nil)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"alice","password":"alice123456"}`, nil)
	alice, ok := app.store.FindUserByUsername("alice")
	if !ok || alice.Role != store.RoleAdmin || !alice.Active {
		t.Fatalf("configured admin username was not applied on registration: %#v", alice)
	}
}

func TestBackgroundConfigIsSanitized(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	headers := map[string]string{"X-Twilight-Client": "webui"}

	valid := doJSONWithHeaders(app, http.MethodPut, "/api/v1/users/me/background", `{"lightBg":"linear-gradient(135deg, #111 0%, #222 100%)","lightBgImage":"url('/api/v1/users/assets/background/0123456789abcdef.png')","lightBlur":99,"lightOpacity":1}`, []*http.Cookie{cookie}, headers)
	if valid.Code != http.StatusOK {
		t.Fatalf("valid background status=%d body=%s", valid.Code, valid.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(valid.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	background := env.Data.(map[string]any)["background"].(string)
	if !strings.Contains(background, `"lightBlur":30`) || !strings.Contains(background, `"lightOpacity":10`) {
		t.Fatalf("background bounds were not enforced: %s", background)
	}
	blockedURL := doJSONWithHeaders(app, http.MethodPut, "/api/v1/users/me/background", `{"lightBgImage":"http://127.0.0.1/private.png"}`, []*http.Cookie{cookie}, headers)
	if blockedURL.Code != http.StatusBadRequest {
		t.Fatalf("external background URL status=%d body=%s", blockedURL.Code, blockedURL.Body.String())
	}
	blockedCSS := doJSONWithHeaders(app, http.MethodPut, "/api/v1/users/me/background", `{"lightBg":"linear-gradient(red, blue);background:url(http://127.0.0.1/x)"}`, []*http.Cookie{cookie}, headers)
	if blockedCSS.Code != http.StatusBadRequest {
		t.Fatalf("unsafe background CSS status=%d body=%s", blockedCSS.Code, blockedCSS.Body.String())
	}
}

func TestRegcodeInviteMediaAndSecurityFlows(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	adminLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	adminCookie := findCookie(adminLogin.Result().Cookies(), "twilight_session")

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"user","password":"user123456"}`, nil)
	userLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"user","password":"user123456"}`, nil)
	userCookie := findCookie(userLogin.Result().Cookies(), "twilight_session")
	user, _ := app.store.FindUserByUsername("user")
	_, _ = app.store.UpdateUser(user.UID, func(u *store.User) error { u.TelegramID = 12345; return nil })

	createdCodes := doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/regcodes", `{"type":2,"days":15,"count":1,"random_algorithm":"hex20"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if createdCodes.Code != http.StatusOK {
		t.Fatalf("create regcode status=%d body=%s", createdCodes.Code, createdCodes.Body.String())
	}
	var codeEnv envelope
	if err := json.Unmarshal(createdCodes.Body.Bytes(), &codeEnv); err != nil {
		t.Fatal(err)
	}
	code := codeEnv.Data.(map[string]any)["codes"].([]any)[0].(string)

	preview := doJSONWithHeaders(app, http.MethodPost, "/api/v1/users/me/use-code", `{"reg_code":"`+code+`","check_only":true}`, []*http.Cookie{userCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), "续期") {
		t.Fatalf("preview status=%d body=%s", preview.Code, preview.Body.String())
	}
	used := doJSONWithHeaders(app, http.MethodPost, "/api/v1/users/me/use-code", `{"reg_code":"`+code+`"}`, []*http.Cookie{userCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if used.Code != http.StatusOK {
		t.Fatalf("use code status=%d body=%s", used.Code, used.Body.String())
	}
	batchCodes := doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/regcodes", `{"type":1,"days":3,"count":2,"random_algorithm":"hex20"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if batchCodes.Code != http.StatusOK {
		t.Fatalf("create batch regcodes status=%d body=%s", batchCodes.Code, batchCodes.Body.String())
	}
	var batchEnv envelope
	if err := json.Unmarshal(batchCodes.Body.Bytes(), &batchEnv); err != nil {
		t.Fatal(err)
	}
	rawBatchCodes := batchEnv.Data.(map[string]any)["codes"].([]any)
	deletePayload := `{"codes":["` + rawBatchCodes[0].(string) + `","` + rawBatchCodes[1].(string) + `","missing-code"]}`
	batchDelete := doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/regcodes/batch-delete", deletePayload, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if batchDelete.Code != http.StatusOK || !strings.Contains(batchDelete.Body.String(), `"deleted":2`) || !strings.Contains(batchDelete.Body.String(), `"missing":1`) {
		t.Fatalf("batch delete regcodes status=%d body=%s", batchDelete.Code, batchDelete.Body.String())
	}

	invite := doJSONWithHeaders(app, http.MethodPost, "/api/v1/invite/codes", `{"days":7}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if invite.Code != http.StatusCreated {
		t.Fatalf("invite status=%d body=%s", invite.Code, invite.Body.String())
	}
	forest := doJSONWithHeaders(app, http.MethodGet, "/api/v1/admin/invite/tree", ``, []*http.Cookie{adminCookie}, nil)
	if forest.Code != http.StatusOK || !strings.Contains(forest.Body.String(), "nodes") {
		t.Fatalf("forest status=%d body=%s", forest.Code, forest.Body.String())
	}

	media := doJSONWithHeaders(app, http.MethodPost, "/api/v1/media/request", `{"source":"tmdb","media_id":550,"title":"Fight Club","media_type":"movie"}`, []*http.Cookie{userCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if media.Code != http.StatusCreated || !strings.Contains(media.Body.String(), "require_key") {
		t.Fatalf("media status=%d body=%s", media.Code, media.Body.String())
	}
	userRequests := app.store.ListMediaRequests(user.UID, false)
	if len(userRequests) != 1 {
		t.Fatalf("expected one media request, got %d", len(userRequests))
	}
	userStatusUpdate := doJSONWithHeaders(app, http.MethodPut, "/api/v1/media/request/"+strconv.FormatInt(userRequests[0].ID, 10)+"/status", `{"status":"accepted"}`, []*http.Cookie{userCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if userStatusUpdate.Code != http.StatusForbidden {
		t.Fatalf("user status update should be forbidden, status=%d body=%s", userStatusUpdate.Code, userStatusUpdate.Body.String())
	}
	adminStatusUpdate := doJSONWithHeaders(app, http.MethodPut, "/api/v1/admin/media-requests/"+strconv.FormatInt(userRequests[0].ID, 10), `{"status":"accepted"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if adminStatusUpdate.Code != http.StatusOK || !strings.Contains(adminStatusUpdate.Body.String(), `"status":"accepted"`) {
		t.Fatalf("admin status update status=%d body=%s", adminStatusUpdate.Code, adminStatusUpdate.Body.String())
	}
	adminReqs := doJSONWithHeaders(app, http.MethodGet, "/api/v1/admin/media-requests?status=all", ``, []*http.Cookie{adminCookie}, nil)
	if adminReqs.Code != http.StatusOK || !strings.Contains(adminReqs.Body.String(), "Fight Club") {
		t.Fatalf("admin reqs status=%d body=%s", adminReqs.Code, adminReqs.Body.String())
	}

	blocked := doJSONWithHeaders(app, http.MethodPost, "/api/v1/security/ip/blacklist", `{"ip":"203.0.113.9","reason":"test"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if blocked.Code != http.StatusOK {
		t.Fatalf("blacklist status=%d body=%s", blocked.Code, blocked.Body.String())
	}
}

func TestInviteMeReturnsStableTreeShape(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")

	resp := doJSON(app, http.MethodGet, "/api/v1/invite/me", ``, []*http.Cookie{cookie})
	if resp.Code != http.StatusOK {
		t.Fatalf("invite/me status=%d body=%s", resp.Code, resp.Body.String())
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	tree, ok := env.Data["tree"].(map[string]any)
	if !ok {
		t.Fatalf("tree is not an object: %#v", env.Data["tree"])
	}
	self, ok := tree["self"].(map[string]any)
	if !ok {
		t.Fatalf("tree.self missing: %#v", tree)
	}
	if depth, _ := self["depth"].(float64); depth != 1 {
		t.Fatalf("unexpected self depth: %#v", self["depth"])
	}
	if _, ok := tree["descendants"].([]any); !ok {
		t.Fatalf("tree.descendants missing: %#v", tree)
	}
}

func TestSigninResponsesMatchFrontendContract(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")

	first := doJSONWithHeaders(app, http.MethodPost, "/api/v1/signin", `{}`, []*http.Cookie{cookie}, map[string]string{"X-Twilight-Client": "webui"})
	if first.Code != http.StatusOK {
		t.Fatalf("signin status=%d body=%s", first.Code, first.Body.String())
	}
	var firstEnv struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstEnv); err != nil {
		t.Fatal(err)
	}
	if firstEnv.Data["currency_name"] != "积分" || firstEnv.Data["total_today"].(float64) != 1 || firstEnv.Data["current_points"].(float64) != 1 {
		t.Fatalf("unexpected first signin payload: %#v", firstEnv.Data)
	}

	second := doJSONWithHeaders(app, http.MethodPost, "/api/v1/signin", `{}`, []*http.Cookie{cookie}, map[string]string{"X-Twilight-Client": "webui"})
	if second.Code != http.StatusOK {
		t.Fatalf("second signin status=%d body=%s", second.Code, second.Body.String())
	}
	var secondEnv struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondEnv); err != nil {
		t.Fatal(err)
	}
	if secondEnv.Data["created"].(bool) || secondEnv.Data["total_today"].(float64) != 0 || secondEnv.Data["today_signed"] != true {
		t.Fatalf("unexpected duplicate signin payload: %#v", secondEnv.Data)
	}

	history := doJSON(app, http.MethodGet, "/api/v1/signin/history?limit=30", ``, []*http.Cookie{cookie})
	if history.Code != http.StatusOK {
		t.Fatalf("history status=%d body=%s", history.Code, history.Body.String())
	}
	if !strings.Contains(history.Body.String(), `"daily_points":1`) || !strings.Contains(history.Body.String(), `"total":1`) {
		t.Fatalf("history did not include frontend fields: %s", history.Body.String())
	}
}

func TestDisabledFeatureFlagsAreExposedAndEnforced(t *testing.T) {
	app := newTestApp(t)
	app.cfg.MediaRequestEnabled = false
	app.cfg.SigninEnabled = false
	app.cfg.InviteEnabled = false

	info := doJSON(app, http.MethodGet, "/api/v1/system/info", ``, nil)
	if info.Code != http.StatusOK {
		t.Fatalf("system info status=%d body=%s", info.Code, info.Body.String())
	}
	var infoEnv struct {
		Data struct {
			Features map[string]bool `json:"features"`
		} `json:"data"`
	}
	if err := json.Unmarshal(info.Body.Bytes(), &infoEnv); err != nil {
		t.Fatal(err)
	}
	if infoEnv.Data.Features["media_request"] || infoEnv.Data.Features["signin"] || infoEnv.Data.Features["invite"] {
		t.Fatalf("disabled feature flags were not exposed: %#v", infoEnv.Data.Features)
	}

	configResp := doJSON(app, http.MethodGet, "/api/v1/signin/config", ``, nil)
	if configResp.Code != http.StatusOK || !strings.Contains(configResp.Body.String(), `"enabled":false`) {
		t.Fatalf("signin config did not expose disabled state: status=%d body=%s", configResp.Code, configResp.Body.String())
	}

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	signin := doJSONWithHeaders(app, http.MethodPost, "/api/v1/signin", `{}`, []*http.Cookie{cookie}, map[string]string{"X-Twilight-Client": "webui"})
	if signin.Code != http.StatusForbidden {
		t.Fatalf("disabled signin status=%d body=%s", signin.Code, signin.Body.String())
	}
	inviteMe := doJSONWithHeaders(app, http.MethodGet, "/api/v1/invite/me", ``, []*http.Cookie{cookie}, map[string]string{"X-Twilight-Client": "webui"})
	if inviteMe.Code != http.StatusForbidden {
		t.Fatalf("disabled invite/me status=%d body=%s", inviteMe.Code, inviteMe.Body.String())
	}
	inviteTree := doJSONWithHeaders(app, http.MethodGet, "/api/v1/admin/invite/tree", ``, []*http.Cookie{cookie}, map[string]string{"X-Twilight-Client": "webui"})
	if inviteTree.Code != http.StatusForbidden {
		t.Fatalf("disabled admin invite tree status=%d body=%s", inviteTree.Code, inviteTree.Body.String())
	}
}

func TestInventoryCheckUsesEmbyProviderAndSeasons(t *testing.T) {
	app := newTestApp(t)
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		if q.Get("AnyProviderIdEquals") == "Tmdb.42" {
			_, _ = w.Write([]byte(`{"Items":[{"Id":"series1","Name":"Example Show","Type":"Series","ProductionYear":2024,"ProviderIds":{"Tmdb":"42"}}],"TotalRecordCount":1}`))
			return
		}
		if q.Get("ParentId") == "series1" {
			_, _ = w.Write([]byte(`{"Items":[{"Id":"s1","Name":"Season 1","Type":"Season","IndexNumber":1},{"Id":"s2","Name":"Season 2","Type":"Season","IndexNumber":2}],"TotalRecordCount":2}`))
			return
		}
		_, _ = w.Write([]byte(`{"Items":[],"TotalRecordCount":0}`))
	}))
	defer emby.Close()
	app.cfg.EmbyURL = emby.URL

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")

	missingSeason := doJSONWithHeaders(app, http.MethodPost, "/api/v1/media/inventory/check", `{"source":"tmdb","media_id":42,"media_type":"tv","season":3}`, []*http.Cookie{cookie}, map[string]string{"X-Twilight-Client": "webui"})
	if missingSeason.Code != http.StatusOK || !strings.Contains(missingSeason.Body.String(), `"exists":false`) || !strings.Contains(missingSeason.Body.String(), `"seasons_available":[1,2]`) {
		t.Fatalf("missing season status=%d body=%s", missingSeason.Code, missingSeason.Body.String())
	}
	existingSeason := doJSONWithHeaders(app, http.MethodPost, "/api/v1/media/inventory/check", `{"source":"tmdb","media_id":42,"media_type":"tv","season":2}`, []*http.Cookie{cookie}, map[string]string{"X-Twilight-Client": "webui"})
	if existingSeason.Code != http.StatusOK || !strings.Contains(existingSeason.Body.String(), `"exists":true`) || !strings.Contains(existingSeason.Body.String(), `"season_requested":2`) {
		t.Fatalf("existing season status=%d body=%s", existingSeason.Code, existingSeason.Body.String())
	}
}

func TestEmbyURLsDoNotFallbackToInternalServerURL(t *testing.T) {
	app := newTestApp(t)
	app.cfg.EmbyURL = "http://127.0.0.1:8096"
	app.cfg.EmbyURLList = nil
	app.cfg.EmbyPublicURL = ""
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	admin, _ := app.store.FindUserByUsername("admin")
	_, _ = app.store.UpdateUser(admin.UID, func(u *store.User) error { u.EmbyID = "emby-admin"; return nil })
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	resp := doJSONWithHeaders(app, http.MethodGet, "/api/v1/system/emby-urls", ``, []*http.Cookie{cookie}, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("emby urls status=%d body=%s", resp.Code, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), app.cfg.EmbyURL) {
		t.Fatalf("internal Emby URL leaked in user route response: %s", resp.Body.String())
	}
}

func TestBangumiSearchUsesV0EndpointAndReturnsResults(t *testing.T) {
	app := newTestApp(t)
	bgm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v0/search/subjects" {
			t.Fatalf("unexpected Bangumi request %s %s", r.Method, r.URL.String())
		}
		if got := r.URL.Query().Get("limit"); got != "20" {
			t.Fatalf("limit query = %q", got)
		}
		if got := r.URL.Query().Get("offset"); got != "0" {
			t.Fatalf("offset query = %q", got)
		}
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("content-type = %q", ct)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["keyword"] != "葬送的芙莉莲" || body["sort"] != "match" {
			t.Fatalf("unexpected Bangumi body %#v", body)
		}
		filter, _ := body["filter"].(map[string]any)
		if filter == nil || filter["nsfw"] != true {
			t.Fatalf("missing Bangumi filter %#v", body["filter"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":400602,"type":2,"name":"Sousou no Frieren","name_cn":"葬送的芙莉莲","date":"2023-09-29","images":{"common":"https://example.test/frieren.jpg"},"rating":{"score":8.8,"rank":10},"eps":28,"tags":[{"name":"漫画改"}]}]}`))
	}))
	defer bgm.Close()
	app.cfg.BangumiAPIURL = bgm.URL

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	resp := doJSON(app, http.MethodGet, "/api/v1/media/search?q=%E8%91%AC%E9%80%81%E7%9A%84%E8%8A%99%E8%8E%89%E8%8E%B2&source=bangumi", ``, []*http.Cookie{cookie})
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"source":"bangumi"`) || !strings.Contains(resp.Body.String(), "葬送的芙莉莲") {
		t.Fatalf("bangumi search status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestBangumiSearchErrorIsVisibleForBangumiSource(t *testing.T) {
	app := newTestApp(t)
	bgm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"description":"bad bangumi request"}`, http.StatusBadGateway)
	}))
	defer bgm.Close()
	app.cfg.BangumiAPIURL = bgm.URL

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	resp := doJSON(app, http.MethodGet, "/api/v1/media/search?q=test&source=bangumi", ``, []*http.Cookie{cookie})
	if resp.Code != http.StatusBadGateway || !strings.Contains(resp.Body.String(), "Bangumi 搜索失败") {
		t.Fatalf("bangumi failure status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestSystemUpdateRejectsUnsafeRepoURL(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	resp := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/update", `{"repo_url":"https://user:pass@example.com/repo.git","branch":"main"}`, []*http.Cookie{cookie}, map[string]string{"X-Twilight-Client": "webui"})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unsafe update URL status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestRuntimeLogsRequireAdminAndRedactSecrets(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	adminLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	adminCookie := findCookie(adminLogin.Result().Cookies(), "twilight_session")
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"user","password":"user123456"}`, nil)
	userLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"user","password":"user123456"}`, nil)
	userCookie := findCookie(userLogin.Result().Cookies(), "twilight_session")

	unauth := doJSON(app, http.MethodGet, "/api/v1/system/admin/runtime/logs", ``, nil)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("runtime logs unauth = %d body=%s", unauth.Code, unauth.Body.String())
	}
	forbidden := doJSON(app, http.MethodGet, "/api/v1/system/admin/runtime/status", ``, []*http.Cookie{userCookie})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("runtime status user = %d body=%s", forbidden.Code, forbidden.Body.String())
	}
	adminStatus := doJSON(app, http.MethodGet, "/api/v1/system/admin/runtime/status", ``, []*http.Cookie{adminCookie})
	if adminStatus.Code != http.StatusOK || !strings.Contains(adminStatus.Body.String(), `"goroutines"`) {
		t.Fatalf("runtime status admin = %d body=%s", adminStatus.Code, adminStatus.Body.String())
	}
	redacted := redactSensitiveText("Authorization: Bearer abcdefghijklmnopqrstuvwxyz api_key=123456789012345")
	if strings.Contains(redacted, "abcdefghijklmnopqrstuvwxyz") || strings.Contains(redacted, "123456789012345") {
		t.Fatalf("secret was not redacted: %s", redacted)
	}
	for _, key := range []string{"apiKey", "api-key", "authorization", "postgres_dsn", "bot.token"} {
		if !sensitiveLogKey(key) {
			t.Fatalf("sensitive log key was not detected: %s", key)
		}
	}
}

func TestRuntimeLoggerAppliesLevelAndCapturesStdLog(t *testing.T) {
	runtimeLogs = newRuntimeLogSink(20)
	t.Cleanup(func() {
		runtimeLogs = newRuntimeLogSink(5000)
		InstallRuntimeLogger(io.Discard, zapcore.InfoLevel)
	})

	var out bytes.Buffer
	InstallRuntimeLogger(&out, zapcore.WarnLevel)
	ConfigureRuntimeLogging(zapcore.WarnLevel, 20)

	zap.L().Info("runtime info should be filtered")
	zap.L().Warn("runtime warn should be captured", zap.String("token", "secret-value"))
	log.Print("runtime standard log should be captured")
	time.Sleep(10 * time.Millisecond)

	entries, _ := runtimeLogs.snapshot(20, 0)
	joined := ""
	for _, entry := range entries {
		joined += entry.Level + ":" + entry.Message + "\n"
	}
	if strings.Contains(joined, "runtime info should be filtered") {
		t.Fatalf("info log passed warn level filter: %s", joined)
	}
	if !strings.Contains(joined, "runtime warn should be captured") || !strings.Contains(joined, "runtime standard log should be captured") {
		t.Fatalf("expected slog and std log entries, got: %s", joined)
	}
	if strings.Contains(out.String(), "secret-value") {
		t.Fatalf("sensitive attribute leaked to runtime log output: %s", out.String())
	}
}

func TestConfigFileChangeHotReloadsRuntimeLogLevel(t *testing.T) {
	app := newTestApp(t)
	app.cfg.ConfigFile = filepath.Join(app.cfg.DatabaseDir, "config.toml")
	writeRuntimeConfig := func(level string, limit int) {
		t.Helper()
		content := "[Global]\n" +
			"databases_dir = " + strconv.Quote(app.cfg.DatabaseDir) + "\n" +
			"log_level = " + strconv.Quote(level) + "\n" +
			"runtime_log_limit = " + strconv.Itoa(limit) + "\n\n" +
			"[Database]\n" +
			"driver = " + strconv.Quote(app.cfg.DatabaseDriver) + "\n" +
			"backup_dir = " + strconv.Quote(app.cfg.DatabaseBackupDir) + "\n" +
			"state_file = " + strconv.Quote(app.cfg.StateFile) + "\n"
		if err := os.WriteFile(app.cfg.ConfigFile, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	runtimeLogs = newRuntimeLogSink(20)
	t.Cleanup(func() {
		runtimeLogs = newRuntimeLogSink(5000)
		InstallRuntimeLogger(io.Discard, zapcore.InfoLevel)
	})
	InstallRuntimeLogger(io.Discard, zapcore.ErrorLevel)
	ConfigureRuntimeLogging(zapcore.ErrorLevel, 20)
	app.cfg.LogLevel = "error"
	writeRuntimeConfig("error", 20)
	app.configSignature = configFileSignature(app.cfg.ConfigFile)

	time.Sleep(5 * time.Millisecond)
	writeRuntimeConfig("debug", 21)
	app.reloadConfigIfChanged()

	if app.cfg.LogLevel != "debug" || app.cfg.RuntimeLogLimit != 21 {
		t.Fatalf("config was not hot reloaded: level=%q limit=%d", app.cfg.LogLevel, app.cfg.RuntimeLogLimit)
	}
	zap.L().Debug("debug after hot reload")
	entries, _ := runtimeLogs.snapshot(20, 0)
	found := false
	for _, entry := range entries {
		if strings.Contains(entry.Message, "debug after hot reload") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("debug log was not captured after hot reload: %#v", entries)
	}
}

func TestConfigSchemaRendersTelegramNewlines(t *testing.T) {
	values := configValues(config.Config{
		TelegramBotStartText: "第一行\n第二行",
		TelegramCustomCommands: []config.TelegramCommandReply{
			{Command: "/hello", Reply: "第一行\n第二行"},
		},
	})
	content := renderConfigTOML(values)
	if !strings.Contains(content, `bot_start_text = "第一行\n第二行"`) {
		t.Fatalf("telegram newline was not TOML escaped correctly:\n%s", content)
	}
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TelegramBotStartText != "第一行\n第二行" {
		t.Fatalf("telegram newline did not round-trip: %q", cfg.TelegramBotStartText)
	}
	if len(cfg.TelegramCustomCommands) != 1 || cfg.TelegramCustomCommands[0].Reply != "第一行\n第二行" {
		t.Fatalf("telegram custom command did not round-trip: %#v", cfg.TelegramCustomCommands)
	}
}

func TestDemoEndpointsAreReadonlyAndValidateActions(t *testing.T) {
	app := newTestApp(t)
	media := doJSON(app, http.MethodGet, "/api/v1/demo/media/search?q=dune", ``, nil)
	if media.Code != http.StatusOK || !strings.Contains(media.Body.String(), "Dune") || !strings.Contains(media.Body.String(), `"readonly":true`) {
		t.Fatalf("demo media status=%d body=%s", media.Code, media.Body.String())
	}
	if media.Header().Get("Cache-Control") != "no-store" || media.Header().Get("X-Twilight-Demo") != "true" {
		t.Fatalf("demo headers missing: cache=%q demo=%q", media.Header().Get("Cache-Control"), media.Header().Get("X-Twilight-Demo"))
	}
	valid := doJSON(app, http.MethodPost, "/api/v1/demo/action/media-request", ``, nil)
	if valid.Code != http.StatusOK || !strings.Contains(valid.Body.String(), `"mutated":false`) {
		t.Fatalf("demo action status=%d body=%s", valid.Code, valid.Body.String())
	}
	invalid := doJSON(app, http.MethodPost, "/api/v1/demo/action/bad%0Aname", ``, nil)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid demo action status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestSystemUpdateValidationHelpers(t *testing.T) {
	if _, err := validateUpdateBranch("../main"); err == nil {
		t.Fatal("expected traversal branch to be rejected")
	}
	if _, err := validateUpdateRepoURL("https://user:pass@example.com/repo.git"); err == nil {
		t.Fatal("expected credentialed repo URL to be rejected")
	}
	if _, err := validateUpdateRepoURL("https://example.com/repo.git?token=secret"); err == nil {
		t.Fatal("expected query-bearing repo URL to be rejected")
	}
	if !systemdServicePattern.MatchString("twilight-scheduler") || systemdServicePattern.MatchString("twilight;reboot") {
		t.Fatal("systemd service name validator is too loose or too strict")
	}
}

func TestBindCodesPersistAcrossStoreReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if err := st.UpsertBindCode(store.BindCode{Code: "PERSIST12345", Scene: "register", Confirmed: true, TelegramID: 12345, TelegramUsername: "persist", CreatedAt: now, ExpiresAt: now + 600}); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	reopened, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	bind, ok := reopened.BindCode("PERSIST12345")
	if !ok || bind.TelegramID != 12345 || !bind.Confirmed || bind.TelegramUsername != "persist" {
		t.Fatalf("bind code did not persist correctly: ok=%v bind=%#v", ok, bind)
	}
}

func TestCleanupExpiredBindCodesKeepsValidCodes(t *testing.T) {
	app := newTestApp(t)
	now := time.Now().Unix()
	if err := app.store.UpsertBindCode(store.BindCode{Code: "EXPIRED12345", Scene: "register", CreatedAt: now - 700, ExpiresAt: now - 1}); err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertBindCode(store.BindCode{Code: "VALID1234567", Scene: "register", CreatedAt: now, ExpiresAt: now + 600}); err != nil {
		t.Fatal(err)
	}
	deleted, err := app.store.CleanupExpiredBindCodes(now)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1", deleted)
	}
	if _, ok := app.store.BindCode("EXPIRED12345"); ok {
		t.Fatal("expired bind code was not deleted")
	}
	if _, ok := app.store.BindCode("VALID1234567"); !ok {
		t.Fatal("valid bind code was deleted")
	}
}

func TestSchedulerCleanupBindCodesJob(t *testing.T) {
	app := newTestApp(t)
	now := time.Now().Unix()
	if err := app.store.UpsertBindCode(store.BindCode{Code: "OLD123456789", Scene: "register", CreatedAt: now - 800, ExpiresAt: now - 1}); err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertRegCode(store.RegCode{Code: "OLD-REGCODE", Type: 1, Days: 30, ValidityTime: 1, UseCountLimit: 1, Active: true, CreatedAt: now - 7200}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/scheduler", nil)
	summary, logs, err := app.runSchedulerJob(req, "cleanup_bind_codes")
	if err != nil {
		t.Fatal(err)
	}
	if int(numeric(summary["deleted"])) != 1 || len(logs) == 0 {
		t.Fatalf("unexpected cleanup summary=%v logs=%v", summary, logs)
	}
	if _, ok := app.store.BindCode("OLD123456789"); ok {
		t.Fatal("scheduler did not delete expired bind code")
	}
	if _, ok := app.store.RegCode("OLD-REGCODE"); !ok {
		t.Fatal("bind-code cleanup deleted a regcode")
	}
}

func TestSchedulerCleanupNoEmbySkipsPendingEntitlements(t *testing.T) {
	app := newTestApp(t)
	app.cfg.AutoCleanupNoEmby = true
	app.cfg.AutoCleanupNoEmbyDays = 1
	old := time.Now().AddDate(0, 0, -3).Unix()
	plain, err := app.store.CreateUser(store.User{Username: "plain-no-emby", Role: store.RoleNormal, Active: true, RegisterTime: old, CreatedAt: old})
	if err != nil {
		t.Fatal(err)
	}
	days := 30
	pending, err := app.store.CreateUser(store.User{Username: "pending-emby", Role: store.RoleNormal, Active: true, PendingEmby: true, PendingEmbyDays: &days, RegisterTime: old, CreatedAt: old})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/scheduler", nil)
	summary, _, err := app.runSchedulerJob(req, "cleanup_no_emby")
	if err != nil {
		t.Fatal(err)
	}
	if int(numeric(summary["deleted"])) != 1 || int(numeric(summary["skipped_pending_emby"])) != 1 {
		t.Fatalf("unexpected cleanup summary: %#v", summary)
	}
	if _, ok := app.store.User(plain.UID); ok {
		t.Fatal("plain no-Emby web user was not deleted")
	}
	if u, ok := app.store.User(pending.UID); !ok || !u.PendingEmby {
		t.Fatalf("pending Emby entitlement user was not preserved: ok=%v user=%#v", ok, u)
	}
}

func TestSchedulerCleanupPendingEmbyEntitlementsKeepsWebAccount(t *testing.T) {
	app := newTestApp(t)
	app.cfg.AutoCleanupPendingEmby = true
	app.cfg.AutoCleanupPendingEmbyDays = 1
	old := time.Now().AddDate(0, 0, -3).Unix()
	recent := time.Now().Unix()
	days := 30
	user, err := app.store.CreateUser(store.User{Username: "pending-clear", Role: store.RoleNormal, Active: true, PendingEmby: true, PendingEmbyDays: &days, RegisterTime: old, CreatedAt: old})
	if err != nil {
		t.Fatal(err)
	}
	recentUser, err := app.store.CreateUser(store.User{Username: "pending-recent-clear", Role: store.RoleNormal, Active: true, PendingEmby: true, PendingEmbyDays: &days, RegisterTime: recent, CreatedAt: recent})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/scheduler", nil)
	summary, _, err := app.runSchedulerJob(req, "cleanup_pending_emby_entitlements")
	if err != nil {
		t.Fatal(err)
	}
	if int(numeric(summary["cleared"])) != 2 || int(numeric(summary["deleted"])) != 0 || asString(summary["scope"]) != "all" {
		t.Fatalf("unexpected entitlement cleanup summary: %#v", summary)
	}
	updated, ok := app.store.User(user.UID)
	if !ok {
		t.Fatal("web account was deleted while clearing pending Emby entitlement")
	}
	if updated.PendingEmby || updated.PendingEmbyDays != nil || !updated.Active {
		t.Fatalf("pending entitlement was not cleared cleanly: %#v", updated)
	}
	updatedRecent, ok := app.store.User(recentUser.UID)
	if !ok || updatedRecent.PendingEmby || updatedRecent.PendingEmbyDays != nil || !updatedRecent.Active {
		t.Fatalf("recent pending entitlement was not cleared cleanly: ok=%v user=%#v", ok, updatedRecent)
	}
}

func TestSchedulerEmbySyncRepairsPlaceholderAndMissingIDs(t *testing.T) {
	app := newTestApp(t)
	app.cfg.EmbyToken = "token"
	remoteUsers := []map[string]any{
		{"Id": "real-alpha", "Name": "alpha", "Policy": map[string]any{}},
		{"Id": "real-beta", "Name": "beta", "Policy": map[string]any{}},
	}
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users":
			_ = json.NewEncoder(w).Encode(remoteUsers)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/Users/"):
			id := strings.TrimPrefix(r.URL.Path, "/Users/")
			for _, user := range remoteUsers {
				if asString(user["Id"]) == id {
					_ = json.NewEncoder(w).Encode(user)
					return
				}
			}
			http.NotFound(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/Policy"):
			_, _ = io.Copy(io.Discard, r.Body)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected Emby request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer emby.Close()
	app.cfg.EmbyURL = emby.URL

	alpha, err := app.store.CreateUser(store.User{Username: "alpha", Role: store.RoleNormal, Active: true, PendingEmby: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.store.UpdateUser(alpha.UID, func(u *store.User) error {
		u.EmbyID = fmt.Sprintf("Emby_%d", alpha.UID)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := app.store.CreateUser(store.User{Username: "local-beta", EmbyUsername: "beta", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/scheduler", nil)
	summary, _, err := app.runSchedulerJob(req, "emby_sync")
	if err != nil {
		t.Fatal(err)
	}
	if int(numeric(summary["filled_emby_ids"])) != 2 || int(numeric(summary["repaired_placeholders"])) != 1 {
		t.Fatalf("unexpected emby sync summary: %#v", summary)
	}
	updatedAlpha, _ := app.store.User(alpha.UID)
	if updatedAlpha.EmbyID != "real-alpha" || updatedAlpha.EmbyUsername != "alpha" || updatedAlpha.PendingEmby {
		t.Fatalf("placeholder Emby ID was not repaired: %#v", updatedAlpha)
	}
	updatedBeta, _ := app.store.User(beta.UID)
	if updatedBeta.EmbyID != "real-beta" || updatedBeta.EmbyUsername != "beta" {
		t.Fatalf("missing Emby ID was not filled by username: %#v", updatedBeta)
	}
}

func TestTelegramMembershipRejoinManualReviewAndAutoEnable(t *testing.T) {
	app := newTestApp(t)
	app.cfg.TelegramMode = true
	app.cfg.TelegramBotToken = "123:ABC"
	app.cfg.TelegramRequireMembership = true
	app.cfg.TelegramGroupIDs = []string{"-1001"}
	user, err := app.store.CreateUser(store.User{Username: "rejoined", Role: store.RoleNormal, Active: false, TelegramID: 4242, EmbyID: "emby-rejoined"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.UpdateUser(user.UID, func(u *store.User) error { u.Active = false; return nil }); err != nil {
		t.Fatal(err)
	}
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123:ABC/getChatMember" {
			t.Fatalf("unexpected telegram path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"status":"member","user":{"id":4242,"is_bot":false}}}`))
	}))
	defer tg.Close()
	app.cfg.TelegramAPIURL = tg.URL

	app.cfg.TelegramAutoEnableRejoined = false
	summary, _, err := app.enforceTelegramMembership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if int(numeric(summary["rejoined_pending_review"])) != 1 || int(numeric(summary["rejoin_candidates"])) != 1 {
		t.Fatalf("manual review rejoin was not reported: %#v", summary)
	}
	if updated, _ := app.store.User(user.UID); updated.Active {
		t.Fatal("manual review mode should not auto-enable the web account")
	}

	app.cfg.TelegramAutoEnableRejoined = true
	summary, _, err = app.enforceTelegramMembership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if int(numeric(summary["rejoined_enabled"])) != 1 {
		t.Fatalf("auto rejoin did not enable user: %#v", summary)
	}
	if updated, _ := app.store.User(user.UID); !updated.Active {
		t.Fatal("auto rejoin did not enable the web account")
	}
}

func TestRegisterConsumesConfirmedTelegramBindCode(t *testing.T) {
	app := newTestApp(t)
	app.cfg.BotInternalSecret = "test-secret"
	app.cfg.ForceBindTelegram = true

	codeResp := doJSON(app, http.MethodPost, "/api/v1/users/telegram/register/bind-code", `{}`, nil)
	if codeResp.Code != http.StatusOK {
		t.Fatalf("bind-code status=%d body=%s", codeResp.Code, codeResp.Body.String())
	}
	var codeBody struct {
		Data struct {
			BindCode string `json:"bind_code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(codeResp.Body.Bytes(), &codeBody); err != nil {
		t.Fatal(err)
	}
	code := codeBody.Data.BindCode
	if code == "" {
		t.Fatalf("missing bind code in response: %s", codeResp.Body.String())
	}

	confirmPayload := fmt.Sprintf(`{"code":%q,"telegram_id":424242,"telegram_username":"alice_tg"}`, code)
	confirmed := doJSONWithHeaders(app, http.MethodPost, "/api/v1/users/me/telegram/bind-confirm", confirmPayload, nil, map[string]string{"X-Internal-Secret": "test-secret"})
	if confirmed.Code != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", confirmed.Code, confirmed.Body.String())
	}
	registered := doJSON(app, http.MethodPost, "/api/v1/users/register", fmt.Sprintf(`{"username":"alice","password":"alice123456","telegram_bind_code":%q}`, code), nil)
	if registered.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", registered.Code, registered.Body.String())
	}
	u, ok := app.store.FindUserByUsername("alice")
	if !ok || u.TelegramID != 424242 || u.TelegramUsername != "alice_tg" {
		t.Fatalf("telegram bind not applied to registered user: ok=%v user=%#v", ok, u)
	}
	if _, ok := app.store.BindCode(code); ok {
		t.Fatal("confirmed register bind code was not consumed")
	}
}

func TestRegisterRejectsUserSceneTelegramBindCode(t *testing.T) {
	app := newTestApp(t)
	app.cfg.ForceBindTelegram = true
	now := time.Now().Unix()
	if err := app.store.UpsertBindCode(store.BindCode{Code: "USERBIND1234", Scene: "user", UID: 99, Confirmed: true, TelegramID: 999, CreatedAt: now, ExpiresAt: now + 600}); err != nil {
		t.Fatal(err)
	}
	resp := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"bob","password":"bob123456","telegram_bind_code":"USERBIND1234"}`, nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("register with user-scene bind code status=%d body=%s", resp.Code, resp.Body.String())
	}
	if _, ok := app.store.FindUserByUsername("bob"); ok {
		t.Fatal("user was registered with a user-scene bind code")
	}
}

func TestRegisterRejectsMalformedTelegramBindCodeBeforeLookup(t *testing.T) {
	app := newTestApp(t)
	app.cfg.ForceBindTelegram = true
	resp := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"badbind","password":"badbind123A","telegram_bind_code":"../../bad"}`, nil)
	if resp.Code != http.StatusBadRequest || !strings.Contains(resp.Body.String(), "invalid telegram bind code format") {
		t.Fatalf("malformed bind code status=%d body=%s", resp.Code, resp.Body.String())
	}

	rr := doJSON(app, http.MethodGet, "/api/v1/users/telegram/register/bind-code/status?code=../../bad", ``, nil)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"invalid":true`) || !strings.Contains(rr.Body.String(), `"terminal":true`) {
		t.Fatalf("malformed bind code status endpoint response=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestTelegramBindConfirmRequiresInternalSecret(t *testing.T) {
	app := newTestApp(t)
	app.cfg.BotInternalSecret = "test-secret"
	now := time.Now().Unix()
	if err := app.store.UpsertBindCode(store.BindCode{Code: "ABCDEFGH", Scene: "register", CreatedAt: now, ExpiresAt: now + 60}); err != nil {
		t.Fatal(err)
	}
	blocked := doJSON(app, http.MethodPost, "/api/v1/users/me/telegram/bind-confirm", `{"code":"ABCDEFGH","telegram_id":42}`, nil)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("bind confirm without secret = %d body=%s", blocked.Code, blocked.Body.String())
	}
	allowed := doJSONWithHeaders(app, http.MethodPost, "/api/v1/users/me/telegram/bind-confirm", `{"code":"ABCDEFGH","telegram_id":42}`, nil, map[string]string{"X-Internal-Secret": "test-secret"})
	if allowed.Code != http.StatusOK {
		t.Fatalf("bind confirm with secret = %d body=%s", allowed.Code, allowed.Body.String())
	}
}

func TestTelegramEndpointAcceptsCommonBaseURLs(t *testing.T) {
	app := newTestApp(t)
	app.cfg.TelegramMode = true
	app.cfg.TelegramBotToken = "123:ABC"
	cases := map[string]string{
		"https://api.telegram.org":            "https://api.telegram.org/bot123:ABC/getMe",
		"https://api.telegram.org/bot":        "https://api.telegram.org/bot123:ABC/getMe",
		"https://api.telegram.org/bot123:ABC": "https://api.telegram.org/bot123:ABC/getMe",
	}
	for base, want := range cases {
		app.cfg.TelegramAPIURL = base
		if got := app.telegramEndpoint("getMe"); got != want {
			t.Fatalf("telegramEndpoint(%q) = %q, want %q", base, got, want)
		}
	}
}

func TestTelegramErrorRedactsBotToken(t *testing.T) {
	app := newTestApp(t)
	app.cfg.TelegramBotToken = "123:SECRET"
	raw := `Post "https://api.telegram.org/bot123:SECRET/getUpdates": context deadline exceeded 123:SECRET`
	got := app.telegramSanitizeError(errors.New(raw))
	if strings.Contains(got, "123:SECRET") || !strings.Contains(got, "/bot<redacted>/getUpdates") {
		t.Fatalf("telegram error was not redacted: %s", got)
	}
	app.setTelegramRuntimeStatus(true, errors.New(raw))
	status := app.telegramRuntimeStatus()
	if strings.Contains(asString(status["last_error"]), "123:SECRET") {
		t.Fatalf("runtime status leaked token: %#v", status)
	}
}

func TestTelegramGetUpdatesAllowsCallbacks(t *testing.T) {
	app := newTestApp(t)
	app.cfg.TelegramMode = true
	app.cfg.TelegramBotToken = "123:ABC"
	var body map[string]any
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123:ABC/getUpdates" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer tg.Close()
	app.cfg.TelegramAPIURL = tg.URL
	if _, err := app.telegramGetUpdates(context.Background(), 42); err != nil {
		t.Fatal(err)
	}
	updates, _ := body["allowed_updates"].([]any)
	foundCallback := false
	for _, update := range updates {
		if update == "callback_query" {
			foundCallback = true
		}
	}
	if !foundCallback || numeric(body["timeout"]) != 30 || numeric(body["offset"]) != 42 {
		t.Fatalf("unexpected getUpdates body: %#v", body)
	}
}

func TestTelegramBindRequirementSplitsGroupAndChannel(t *testing.T) {
	app := newTestApp(t)
	app.cfg.TelegramMode = true
	app.cfg.TelegramBotToken = "123:ABC"
	app.cfg.TelegramGroupIDs = []string{"-1001"}
	app.cfg.TelegramChannelIDs = []string{"@RequiredChannel"}
	app.cfg.TelegramForceBindGroup = true
	app.cfg.TelegramForceBindChannel = false
	requests := []map[string]any{}
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		body["_path"] = r.URL.Path
		requests = append(requests, body)
		switch r.URL.Path {
		case "/bot123:ABC/getChatMember":
			if asString(body["chat_id"]) == "@RequiredChannel" {
				_, _ = w.Write([]byte(`{"ok":true,"result":{"status":"left","user":{"id":42,"is_bot":false}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"status":"member","user":{"id":42,"is_bot":false}}}`))
		case "/bot123:ABC/sendMessage":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
		default:
			t.Fatalf("unexpected telegram path: %s", r.URL.Path)
		}
	}))
	defer tg.Close()
	app.cfg.TelegramAPIURL = tg.URL

	now := time.Now().Unix()
	if err := app.store.UpsertBindCode(store.BindCode{Code: "GROUP1", Scene: "register", CreatedAt: now, ExpiresAt: now + 600}); err != nil {
		t.Fatal(err)
	}
	app.telegramConfirmBindCode(context.Background(), 42, 42, "tguser", "GROUP1")
	bind, ok := app.store.BindCode("GROUP1")
	if !ok || !bind.Confirmed {
		t.Fatalf("group-only requirement should confirm bind code: %#v", bind)
	}
	for _, req := range requests {
		if req["_path"] == "/bot123:ABC/getChatMember" && asString(req["chat_id"]) == "@RequiredChannel" {
			t.Fatalf("channel was checked even though force_bind_channel=false: %#v", requests)
		}
	}

	app.cfg.TelegramForceBindChannel = true
	if err := app.store.UpsertBindCode(store.BindCode{Code: "CHAN01", Scene: "register", CreatedAt: now, ExpiresAt: now + 600}); err != nil {
		t.Fatal(err)
	}
	app.telegramConfirmBindCode(context.Background(), 42, 43, "tguser2", "CHAN01")
	channelBind, ok := app.store.BindCode("CHAN01")
	if !ok || channelBind.Confirmed {
		t.Fatalf("channel requirement should reject user missing required channel: %#v", channelBind)
	}
}

func TestTelegramAnonymousGroupUserRequiresInlineAuth(t *testing.T) {
	app := newTestApp(t)
	app.cfg.TelegramMode = true
	app.cfg.TelegramBotToken = "123:ABC"
	user := store.User{UID: 1001, Username: "target", Role: store.RoleNormal, Active: true, TelegramID: 888, CreatedAt: time.Now().Unix(), RegisterTime: time.Now().Unix()}
	if _, err := app.store.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	requests := []map[string]any{}
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		body["_path"] = r.URL.Path
		requests = append(requests, body)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":9001}}`))
	}))
	defer tg.Close()
	app.cfg.TelegramAPIURL = tg.URL

	app.handleTelegramUpdate(context.Background(), map[string]any{
		"message": map[string]any{
			"message_id": 77,
			"text":       "/twguser target",
			"chat":       map[string]any{"id": -1001, "type": "supergroup"},
			"from":       map[string]any{"id": 1087968824, "is_bot": true},
			"sender_chat": map[string]any{
				"id": -1001,
			},
		},
	})
	if len(requests) != 1 {
		t.Fatalf("expected one Telegram request, got %#v", requests)
	}
	if requests[0]["_path"] != "/bot123:ABC/sendMessage" {
		t.Fatalf("expected sendMessage, got %#v", requests[0])
	}
	markup, _ := requests[0]["reply_markup"].(map[string]any)
	keyboard, _ := markup["inline_keyboard"].([]any)
	if len(keyboard) == 0 || !strings.Contains(asString(requests[0]["text"]), "验证") {
		t.Fatalf("anonymous command did not create auth panel: %#v", requests[0])
	}
}

func TestSchedulerManualRunUpdatesSingleHistoryEntry(t *testing.T) {
	app := newTestApp(t)
	run, okRun := app.startManualSchedulerJob(context.Background(), "daily_stats", nil)
	if !okRun {
		t.Fatal("manual scheduler job did not start")
	}
	deadline := time.Now().Add(2 * time.Second)
	for app.schedulerJobRunning("daily_stats") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if app.schedulerJobRunning("daily_stats") {
		t.Fatal("manual scheduler job did not finish")
	}
	runs := app.store.SchedulerRuns("daily_stats", 10)
	if len(runs) != 1 {
		t.Fatalf("manual run should update one history entry, got %d: %#v", len(runs), runs)
	}
	if runs[0].ID != run.ID || runs[0].Status != "success" || runs[0].Type != "manual" || runs[0].FinishedAt == 0 {
		t.Fatalf("unexpected manual run history: %#v", runs[0])
	}
}

func TestSchedulerManualTriggerSpecDisablesAutoRun(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/scheduler/jobs/daily_stats/schedule", strings.NewReader(`{"type":"manual"}`))
	rr := httptest.NewRecorder()
	app.handleSchedulerSchedule(rr, req, Params{"job_id": "daily_stats"})
	if rr.Code != http.StatusOK {
		t.Fatalf("schedule manual status=%d body=%s", rr.Code, rr.Body.String())
	}
	spec := app.schedulerTriggerSpec("daily_stats")
	if asString(spec["type"]) != "manual" || !schedulerTriggerDisabled(spec) {
		t.Fatalf("manual trigger spec not persisted: %#v", spec)
	}
	if next := app.schedulerNextRunAt("daily_stats", spec, time.Now()); next != 0 {
		t.Fatalf("manual trigger should not have next run, got %d", next)
	}
}

func TestSchedulerJobsReconcileStaleRunningHistory(t *testing.T) {
	app := newTestApp(t)
	run, err := app.store.AddSchedulerRunReturning(store.SchedulerRun{JobID: "enforce_group_membership", Type: "auto", Trigger: "scheduler", Status: "running", Message: "running", StartedAt: time.Now().Add(-time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/jobs", nil)
	rr := httptest.NewRecorder()
	app.handleSchedulerJobs(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", rr.Code, rr.Body.String())
	}
	runs := app.store.SchedulerRuns("enforce_group_membership", 1)
	if len(runs) != 1 || runs[0].ID != run.ID || runs[0].Status == "running" || runs[0].FinishedAt == 0 {
		t.Fatalf("stale running history was not reconciled: %#v", runs)
	}
	if !boolish(runs[0].Summary["interrupted"]) {
		t.Fatalf("reconciled run should be marked interrupted: %#v", runs[0])
	}
}

func TestSchedulerTerminateIsIdempotentWhenJobAlreadyStopped(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scheduler/jobs/enforce_group_membership/terminate", nil)
	rr := httptest.NewRecorder()
	app.handleSchedulerTerminate(rr, req, Params{"job_id": "enforce_group_membership"})
	if rr.Code != http.StatusOK {
		t.Fatalf("terminate should be idempotent, status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"already_stopped":true`) {
		t.Fatalf("terminate response did not mark already_stopped: %s", rr.Body.String())
	}
}

func TestSchedulerRuntimeParamsPersistInStoreAndDriveRunner(t *testing.T) {
	app := newTestApp(t)
	app.cfg.AutoCleanupPendingEmby = true
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/scheduler/jobs/cleanup_pending_emby_entitlements/schedule", strings.NewReader(`{"type":"interval","seconds":120,"runtime_params":{"enabled":false}}`))
	rr := httptest.NewRecorder()
	app.handleSchedulerSchedule(rr, req, Params{"job_id": "cleanup_pending_emby_entitlements"})
	if rr.Code != http.StatusOK {
		t.Fatalf("schedule status=%d body=%s", rr.Code, rr.Body.String())
	}
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/jobs", nil)
	listRR := httptest.NewRecorder()
	app.handleSchedulerJobs(listRR, listReq, nil)
	if listRR.Code != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	var body struct {
		Data struct {
			Jobs []map[string]any `json:"jobs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, job := range body.Data.Jobs {
		if asString(job["id"]) != "cleanup_pending_emby_entitlements" {
			continue
		}
		found = true
		params, _ := job["runtime_params"].(map[string]any)
		if boolish(params["enabled"]) || asString(params["scope"]) != "all" {
			t.Fatalf("runtime params did not come from backend store: %#v", params)
		}
	}
	if !found {
		t.Fatal("cleanup_pending_emby_entitlements job not returned")
	}
	days := 30
	if _, err := app.store.CreateUser(store.User{Username: "pending-db-disabled", Role: store.RoleNormal, Active: true, PendingEmby: true, PendingEmbyDays: &days}); err != nil {
		t.Fatal(err)
	}
	summary, _, err := app.runSchedulerJob(httptest.NewRequest(http.MethodPost, "/scheduler", nil), "cleanup_pending_emby_entitlements")
	if err != nil {
		t.Fatal(err)
	}
	if boolish(summary["enabled"]) || int(numeric(summary["cleared"])) != 0 {
		t.Fatalf("DB runtime params should disable automatic cleanup: %#v", summary)
	}
}

func TestBangumiWebhookRequiresSecretWhenEnabled(t *testing.T) {
	app := newTestApp(t)
	app.cfg.BangumiEnabled = true
	blocked := doJSON(app, http.MethodPost, "/api/v1/emby/bangumi/webhook", `{"Event":"PlaybackStopped"}`, nil)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("webhook without configured secret = %d body=%s", blocked.Code, blocked.Body.String())
	}
	app.cfg.BangumiWebhookSecret = "webhook-secret"
	allowed := doJSON(app, http.MethodPost, "/api/v1/emby/bangumi/webhook?token=webhook-secret", `{"Event":"PlaybackStopped"}`, nil)
	if allowed.Code != http.StatusOK {
		t.Fatalf("webhook with secret = %d body=%s", allowed.Code, allowed.Body.String())
	}
}

func TestDatabaseAdminBackupRestoreAndAuth(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	adminLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	adminCookie := findCookie(adminLogin.Result().Cookies(), "twilight_session")
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"user","password":"user123456"}`, nil)
	userLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"user","password":"user123456"}`, nil)
	userCookie := findCookie(userLogin.Result().Cookies(), "twilight_session")

	unauth := doJSON(app, http.MethodGet, "/api/v1/system/admin/database/status", ``, nil)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("database status unauth = %d", unauth.Code)
	}
	forbidden := doJSON(app, http.MethodGet, "/api/v1/system/admin/database/status", ``, []*http.Cookie{userCookie})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("database status user = %d body=%s", forbidden.Code, forbidden.Body.String())
	}
	if err := os.WriteFile(filepath.Join(app.cfg.DatabaseDir, "users.db"), []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	status := doJSON(app, http.MethodGet, "/api/v1/system/admin/database/status", ``, []*http.Cookie{adminCookie})
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"legacy_sqlite_detected":true`) {
		t.Fatalf("database status did not report legacy sqlite status=%d body=%s", status.Code, status.Body.String())
	}
	backup := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/backup", `{"note":"before restore test"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if backup.Code != http.StatusOK {
		t.Fatalf("backup status=%d body=%s", backup.Code, backup.Body.String())
	}
	if !strings.Contains(backup.Body.String(), `"legacy_sqlite_backup"`) {
		t.Fatalf("backup did not include legacy sqlite files body=%s", backup.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(backup.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	backupData := env.Data.(map[string]any)["backup"].(map[string]any)
	backupName := backupData["name"].(string)
	if backupData["note"] != "before restore test" {
		t.Fatalf("backup note was not persisted: %#v", backupData["note"])
	}
	backupInspect := doJSONWithHeaders(app, http.MethodGet, "/api/v1/system/admin/database/backups/"+backupName, ``, []*http.Cookie{adminCookie}, nil)
	if backupInspect.Code != http.StatusOK || !strings.Contains(backupInspect.Body.String(), `"counts"`) || !strings.Contains(backupInspect.Body.String(), `"note":"before restore test"`) {
		t.Fatalf("backup inspect status=%d body=%s", backupInspect.Code, backupInspect.Body.String())
	}
	backupList := doJSONWithHeaders(app, http.MethodGet, "/api/v1/system/admin/database/backups", ``, []*http.Cookie{adminCookie}, nil)
	if backupList.Code != http.StatusOK || strings.Contains(backupList.Body.String(), backupName+".meta.json") {
		t.Fatalf("backup list exposed metadata file status=%d body=%s", backupList.Code, backupList.Body.String())
	}
	backupMetaInspect := doJSONWithHeaders(app, http.MethodGet, "/api/v1/system/admin/database/backups/"+backupName+".meta.json", ``, []*http.Cookie{adminCookie}, nil)
	if backupMetaInspect.Code != http.StatusBadRequest {
		t.Fatalf("backup metadata inspect status=%d body=%s", backupMetaInspect.Code, backupMetaInspect.Body.String())
	}

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"extra","password":"extra123456"}`, nil)
	if _, ok := app.store.FindUserByUsername("extra"); !ok {
		t.Fatal("expected extra user before restore")
	}
	restorePreview := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/restore", `{"name":"`+backupName+`"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if restorePreview.Code != http.StatusOK || !strings.Contains(restorePreview.Body.String(), `"requires_confirmation":true`) {
		t.Fatalf("restore preview status=%d body=%s", restorePreview.Code, restorePreview.Body.String())
	}
	if _, ok := app.store.FindUserByUsername("extra"); !ok {
		t.Fatal("restore preview mutated state")
	}
	restore := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/restore", `{"name":"`+backupName+`","confirm":"RESTORE_DATABASE_BACKUP"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if restore.Code != http.StatusOK {
		t.Fatalf("restore status=%d body=%s", restore.Code, restore.Body.String())
	}
	if !strings.Contains(restore.Body.String(), `"pre_operation_backup"`) {
		t.Fatalf("restore did not report pre-operation backup body=%s", restore.Body.String())
	}
	if _, ok := app.store.FindUserByUsername("extra"); ok {
		t.Fatal("restore did not replace state")
	}
	traversal := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/restore", `{"name":"../state.json"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if traversal.Code != http.StatusBadRequest {
		t.Fatalf("restore traversal status=%d body=%s", traversal.Code, traversal.Body.String())
	}
	migrateDisabled := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/migrate", `{"target_driver":"json","dry_run":true}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if migrateDisabled.Code != http.StatusForbidden {
		t.Fatalf("migrate disabled status=%d body=%s", migrateDisabled.Code, migrateDisabled.Body.String())
	}
	app.cfg.DatabaseMigrationPanelEnabled = true
	migrate := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/migrate", `{"target_driver":"json","dry_run":true}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if migrate.Code != http.StatusOK || !strings.Contains(migrate.Body.String(), `"dry_run":true`) {
		t.Fatalf("migrate dry-run status=%d body=%s", migrate.Code, migrate.Body.String())
	}
	migrateNoConfirm := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/migrate", `{"target_driver":"json","state_file":"migrated.json"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if migrateNoConfirm.Code != http.StatusOK || !strings.Contains(migrateNoConfirm.Body.String(), `"requires_confirmation":true`) || !strings.Contains(migrateNoConfirm.Body.String(), `"dry_run":true`) {
		t.Fatalf("migrate without confirm status=%d body=%s", migrateNoConfirm.Code, migrateNoConfirm.Body.String())
	}
	if _, err := os.Stat(filepath.Join(app.cfg.DatabaseDir, "migrated.json")); err == nil {
		t.Fatal("migrate without confirm wrote target file")
	}
	migrateExecute := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/migrate", `{"target_driver":"json","state_file":"migrated.json","confirm":"MIGRATE_DATABASE"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if migrateExecute.Code != http.StatusOK || !strings.Contains(migrateExecute.Body.String(), `"pre_operation_backup"`) {
		t.Fatalf("migrate execute status=%d body=%s", migrateExecute.Code, migrateExecute.Body.String())
	}
	if _, err := os.Stat(filepath.Join(app.cfg.DatabaseDir, "migrated.json")); err != nil {
		t.Fatalf("migrate with confirm did not write target file: %v", err)
	}
	migrateTraversal := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/migrate", `{"target_driver":"json","state_file":"../outside.json","dry_run":true}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if migrateTraversal.Code != http.StatusBadRequest {
		t.Fatalf("migrate traversal status=%d body=%s", migrateTraversal.Code, migrateTraversal.Body.String())
	}
	migrateWrongType := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/migrate", `{"target_driver":"json","state_file":"state.txt","dry_run":true}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if migrateWrongType.Code != http.StatusBadRequest {
		t.Fatalf("migrate wrong type status=%d body=%s", migrateWrongType.Code, migrateWrongType.Body.String())
	}
	deleteBackup := doJSONWithHeaders(app, http.MethodDelete, "/api/v1/system/admin/database/backups/"+backupName, ``, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if deleteBackup.Code != http.StatusOK {
		t.Fatalf("delete backup status=%d body=%s", deleteBackup.Code, deleteBackup.Body.String())
	}
}

func TestConfigAdminBackupRestoreAndDelete(t *testing.T) {
	app := newTestApp(t)
	app.cfg.ConfigFile = filepath.Join(app.cfg.DatabaseDir, "config.toml")
	original := "[Global]\ndatabases_dir = " + strconv.Quote(app.cfg.DatabaseDir) + "\n\n[Database]\ndriver = " + strconv.Quote(app.cfg.DatabaseDriver) + "\nbackup_dir = " + strconv.Quote(app.cfg.DatabaseBackupDir) + "\nstate_file = " + strconv.Quote(app.cfg.StateFile) + "\n\n[API]\nhost = \"127.0.0.1\"\nport = 5010\n"
	changed := strings.Replace(original, "port = 5010", "port = 5011", 1)
	if err := os.WriteFile(app.cfg.ConfigFile, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	adminLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	adminCookie := findCookie(adminLogin.Result().Cookies(), "twilight_session")

	backup := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/config/backup", `{}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if backup.Code != http.StatusOK {
		t.Fatalf("config backup status=%d body=%s", backup.Code, backup.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(backup.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	backupData := env.Data.(map[string]any)["backup"].(map[string]any)
	backupName := backupData["name"].(string)

	list := doJSONWithHeaders(app, http.MethodGet, "/api/v1/system/admin/config/backups", ``, []*http.Cookie{adminCookie}, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), backupName) {
		t.Fatalf("config backup list status=%d body=%s", list.Code, list.Body.String())
	}
	inspect := doJSONWithHeaders(app, http.MethodGet, "/api/v1/system/admin/config/backups/"+backupName, ``, []*http.Cookie{adminCookie}, nil)
	if inspect.Code != http.StatusOK || !strings.Contains(inspect.Body.String(), "port = 5010") {
		t.Fatalf("config backup inspect status=%d body=%s", inspect.Code, inspect.Body.String())
	}

	if err := os.WriteFile(app.cfg.ConfigFile, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	restorePreview := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/config/restore", `{"name":"`+backupName+`"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if restorePreview.Code != http.StatusOK || !strings.Contains(restorePreview.Body.String(), `"requires_confirmation":true`) {
		t.Fatalf("config restore preview status=%d body=%s", restorePreview.Code, restorePreview.Body.String())
	}
	if data, _ := os.ReadFile(app.cfg.ConfigFile); !strings.Contains(string(data), "port = 5011") {
		t.Fatal("config restore preview mutated file")
	}
	restore := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/config/restore", `{"name":"`+backupName+`","confirm":"RESTORE_CONFIG_BACKUP"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if restore.Code != http.StatusOK || !strings.Contains(restore.Body.String(), `"pre_operation_backup"`) {
		t.Fatalf("config restore status=%d body=%s", restore.Code, restore.Body.String())
	}
	if data, _ := os.ReadFile(app.cfg.ConfigFile); !strings.Contains(string(data), "port = 5010") {
		t.Fatalf("config restore did not restore original content: %s", string(data))
	}

	adminLogin = doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	adminCookie = findCookie(adminLogin.Result().Cookies(), "twilight_session")
	deleteBackup := doJSONWithHeaders(app, http.MethodDelete, "/api/v1/system/admin/config/backups/"+backupName, ``, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if deleteBackup.Code != http.StatusOK {
		t.Fatalf("config backup delete status=%d body=%s", deleteBackup.Code, deleteBackup.Body.String())
	}
}

func TestConfigTOMLGetReturnsCompletedConfig(t *testing.T) {
	app := newTestApp(t)
	app.cfg.ConfigFile = filepath.Join(app.cfg.DatabaseDir, "config.toml")
	minimal := "[Global]\ndatabases_dir = " + strconv.Quote(app.cfg.DatabaseDir) + "\n\n[Database]\ndriver = " + strconv.Quote(app.cfg.DatabaseDriver) + "\nstate_file = " + strconv.Quote(app.cfg.StateFile) + "\nbackup_dir = " + strconv.Quote(app.cfg.DatabaseBackupDir) + "\n\n[API]\nhost = \"127.0.0.1\"\nport = 5010\n\n[Admin]\nusernames = [\"root\"]\n"
	if err := os.WriteFile(app.cfg.ConfigFile, []byte(minimal), 0o600); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	app.handleConfigTOMLGet(rr, httptest.NewRequest(http.MethodGet, "/api/v1/system/admin/config/toml", nil), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("config toml status=%d body=%s", rr.Code, rr.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	data := env.Data.(map[string]any)
	content := data["content"].(string)
	if data["completed"] != true {
		t.Fatalf("expected completed config marker, got %#v", data["completed"])
	}
	if !strings.Contains(content, "[Signin]") || !strings.Contains(content, "daily_min") {
		t.Fatalf("completed config missing signin section: %s", content)
	}
	if strings.Contains(content, "[Admin]") || strings.Contains(content, "usernames =") {
		t.Fatalf("protected admin config leaked: %s", content)
	}
}

func TestConfigSaveMigratesLegacyTelegramForceSubscribe(t *testing.T) {
	app := newTestApp(t)
	app.cfg.ConfigFile = filepath.Join(app.cfg.DatabaseDir, "config.toml")
	existing := "[Global]\nserver_name = \"old\"\n\n[Admin]\nusernames = [\"root\"]\n"
	if err := os.WriteFile(app.cfg.ConfigFile, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy := `[Global]
server_name = "new"
databases_dir = "` + strings.ReplaceAll(app.cfg.DatabaseDir, `\`, `\\`) + `"

[Database]
driver = "json"
state_file = "` + strings.ReplaceAll(app.cfg.StateFile, `\`, `\\`) + `"
backup_dir = "` + strings.ReplaceAll(app.cfg.DatabaseBackupDir, `\`, `\\`) + `"

[Telegram]
group_id = ["@group"]
channel_id = ["@channel"]
force_subscribe = true
`
	info, status, message := app.saveConfigContent(legacy)
	if status != http.StatusOK {
		t.Fatalf("saveConfigContent status=%d message=%s info=%v", status, message, info)
	}
	data, err := os.ReadFile(app.cfg.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "force_subscribe") {
		t.Fatalf("legacy force_subscribe was not removed: %s", content)
	}
	if !strings.Contains(content, "force_bind_group = true") || !strings.Contains(content, "force_bind_channel = true") {
		t.Fatalf("legacy force_subscribe was not migrated to split true values: %s", content)
	}
	if !strings.Contains(content, "[Admin]") || !strings.Contains(content, "root") {
		t.Fatalf("protected admin config was not preserved: %s", content)
	}
}

func TestEmbyCapacityCountsPendingEntitlementsSeparatelyFromSystemLimit(t *testing.T) {
	app := newTestApp(t)
	app.cfg.UserLimit = 100
	app.cfg.EmbyUserLimit = 3
	now := time.Now().Unix()
	if _, err := app.store.CreateUser(store.User{Username: "emby", Role: store.RoleNormal, Active: true, EmbyID: "emby-1"}); err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertInviteCode(store.InviteCode{Code: "INV-A", UID: 1, InviterUID: 1, Days: 30, UseCountLimit: 1, Active: true, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertRegCode(store.RegCode{Code: "REG-A", Type: 1, Days: 30, ValidityTime: -1, UseCountLimit: 1, Active: true, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertRegCode(store.RegCode{Code: "REG-RENEW", Type: 2, Days: 30, ValidityTime: -1, UseCountLimit: 100, Active: true, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	if reached, current, limit := app.systemUserLimitReached(); reached || current != 1 || limit != 100 {
		t.Fatalf("system limit should only count local users, got reached=%v current=%d limit=%d", reached, current, limit)
	}
	if reached, current, limit := app.embyCapacityReached(0); !reached || current != 3 || limit != 3 {
		t.Fatalf("emby capacity should count existing users and pending code slots, got reached=%v current=%d limit=%d", reached, current, limit)
	}
}

func TestMediaRequestInventoryIssueBypassesDuplicateGuard(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	base := store.MediaRequest{UID: 1, Title: "Movie", Source: "tmdb", MediaID: 42, MediaType: "movie", MediaInfo: map[string]any{"title": "Movie"}}
	if _, err := st.CreateMediaRequest(base); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateMediaRequest(base); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("expected duplicate conflict, got %v", err)
	}
	issue := base
	issue.MediaInfo = map[string]any{"title": "Movie", "inventory_issue": true}
	issue.Note = "字幕不同步"
	if _, err := st.CreateMediaRequest(issue); err != nil {
		t.Fatalf("inventory issue report should bypass duplicate guard: %v", err)
	}
}

func TestRegCodeRandomAlgorithmsAndFormatFallback(t *testing.T) {
	code := generateRegCode("TW-{type}", 1, "symbols-24", 30, 1, -1, 1)
	if !strings.HasPrefix(code, "TW-REG-") {
		t.Fatalf("format without random should append random part, got %q", code)
	}
	mixedCase := generateRegCode("Twilight-{type}-Vip-{random}", 1, "digits-12", 30, 1, -1, 1)
	if !strings.HasPrefix(mixedCase, "Twilight-REG-Vip-") {
		t.Fatalf("custom format text casing was not preserved: %q", mixedCase)
	}
	seenSpecial := false
	for i := 0; i < 10; i++ {
		symbolCode := generateRegCode("TW-{random}", 1, "symbols-24", 30, i+1, -1, 1)
		if strings.ContainsAny(symbolCode, "!@$%^*_-+=.:") {
			seenSpecial = true
			break
		}
	}
	if !seenSpecial {
		t.Fatal("symbols algorithm did not produce any special characters in repeated samples")
	}
	uuidCode := generateRegCode("{random}", 1, "uuid", 30, 1, -1, 1)
	if parts := strings.Split(uuidCode, "-"); len(parts) != 5 || len(uuidCode) != 36 {
		t.Fatalf("uuid algorithm returned invalid shape: %q", uuidCode)
	}
}

func TestTargetedRegcodesAreCreatedListedAndEnforced(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	adminLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	adminCookie := findCookie(adminLogin.Result().Cookies(), "twilight_session")
	created := doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/regcodes", `{"type":2,"days":10,"count":1,"target_username":"alpha","format":"TGT-{random}","random_algorithm":"digits-12"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if created.Code != http.StatusOK {
		t.Fatalf("create targeted regcode status=%d body=%s", created.Code, created.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(created.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	code := env.Data.(map[string]any)["codes"].([]any)[0].(string)
	reg, ok := app.store.RegCode(code)
	if !ok || reg.TargetUsername != "alpha" {
		t.Fatalf("target username was not saved: %#v", reg)
	}
	list := doJSONWithHeaders(app, http.MethodGet, "/api/v1/admin/regcodes?search=alpha", ``, []*http.Cookie{adminCookie}, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"target_username":"alpha"`) {
		t.Fatalf("target username was not searchable/listed, status=%d body=%s", list.Code, list.Body.String())
	}

	alpha, err := app.store.CreateUser(store.User{Username: "alpha", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := app.store.CreateUser(store.User{Username: "beta", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/use-code", strings.NewReader(`{"reg_code":"`+code+`"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: beta}))
	rr := httptest.NewRecorder()
	app.handleUseCode(rr, req, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("non-target user should not use targeted regcode, status=%d body=%s", rr.Code, rr.Body.String())
	}
	reg, _ = app.store.RegCode(code)
	if reg.UseCount != 0 {
		t.Fatalf("target mismatch consumed regcode, use_count=%d", reg.UseCount)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/me/use-code", strings.NewReader(`{"reg_code":"`+code+`"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: alpha}))
	rr = httptest.NewRecorder()
	app.handleUseCode(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("target user should use targeted regcode, status=%d body=%s", rr.Code, rr.Body.String())
	}

	if err := app.store.UpsertRegCode(store.RegCode{Code: "RENEW-ALPHA", Type: 2, Days: 5, ValidityTime: -1, UseCountLimit: 1, Active: true, TargetUsername: "alpha"}); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/me/renew", strings.NewReader(`{"reg_code":"RENEW-ALPHA"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: beta}))
	rr = httptest.NewRecorder()
	app.handleRenew(rr, req, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("renew endpoint should reject non-target user, status=%d body=%s", rr.Code, rr.Body.String())
	}
	reg, _ = app.store.RegCode("RENEW-ALPHA")
	if reg.UseCount != 0 {
		t.Fatalf("target mismatch consumed renewal code, use_count=%d", reg.UseCount)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/me/renew", strings.NewReader(`{"reg_code":"RENEW-ALPHA"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: alpha}))
	rr = httptest.NewRecorder()
	app.handleRenew(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("target user should renew with targeted code, status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestInviteParentCanDetachExpiredChildAndKeepWebAccountActive(t *testing.T) {
	app := newTestApp(t)
	parent, err := app.store.CreateUser(store.User{Username: "parent", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.store.CreateUser(store.User{Username: "child", Role: store.RoleNormal, Active: true, ExpiredAt: time.Now().AddDate(0, 0, -1).Unix(), EmbyID: "emby-child", EmbyUsername: "child"})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertInviteCode(store.InviteCode{Code: "INV-CHILD", UID: parent.UID, InviterUID: parent.UID, Days: 30, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.ConsumeInviteCode("INV-CHILD", child.UID); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invite/children/2/detach-expired", nil)
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: parent}))
	rr := httptest.NewRecorder()
	app.handleDetachExpiredInviteChild(rr, req, Params{"uid": strconv.FormatInt(child.UID, 10)})
	if rr.Code != http.StatusOK {
		t.Fatalf("detach status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, ok := app.store.ParentOf(child.UID); ok {
		t.Fatal("child still has invite parent")
	}
	updated, ok := app.store.User(child.UID)
	if !ok || !updated.Active || updated.EmbyID != "" || updated.EmbyUsername != "" || updated.PendingEmby {
		t.Fatalf("child web account was not preserved while Emby was cleared: %#v", updated)
	}
}

func TestRegcodeDTOAndUsersHideLegacyUsedBy(t *testing.T) {
	app := newTestApp(t)
	user, err := app.store.CreateUser(store.User{Username: "legacy-user", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	reg := store.RegCode{Code: "LEGACY-USED", Type: 2, Days: 30, ValidityTime: -1, UseCountLimit: 5, UseCount: 1, UsedBy: user.UID, Active: true}
	dto := regcodeDTO(reg)
	uids, _ := dto["used_by_uids"].([]int64)
	if len(uids) != 0 || asString(dto["used_by"]) != "" {
		t.Fatalf("legacy used_by should be hidden in dto: %#v", dto)
	}
	if err := app.store.UpsertRegCode(reg); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/regcodes/LEGACY-USED/users", nil)
	rr := httptest.NewRecorder()
	app.handleRegcodeUsers(rr, req, Params{"code": "LEGACY-USED"})
	if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), `"username":"legacy-user"`) {
		t.Fatalf("legacy used_by user should not be listed, status=%d body=%s", rr.Code, rr.Body.String())
	}
	reg.UsedByUIDs = []int64{user.UID}
	dto = regcodeDTO(reg)
	uids, _ = dto["used_by_uids"].([]int64)
	if len(uids) != 1 || uids[0] != user.UID || asString(dto["used_by"]) != strconv.FormatInt(user.UID, 10) {
		t.Fatalf("new used_by_uids should be exposed in dto: %#v", dto)
	}
}

func TestPendingEmbyUserCanReplaceEntitlementWithRegisterCode(t *testing.T) {
	app := newTestApp(t)
	app.cfg.EmbyUserLimit = 1
	oldDays := 90
	user, err := app.store.CreateUser(store.User{Username: "pending-replace", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	updatedUser, err := app.store.UpdateUser(user.UID, func(u *store.User) error {
		u.PendingEmby = true
		u.PendingEmbyDays = &oldDays
		u.EmbyUsername = "old-name"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	user = updatedUser
	if err := app.store.UpsertRegCode(store.RegCode{Code: "REG-REPLACE", Type: 1, Days: 7, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/use-code", strings.NewReader(`{"reg_code":"REG-REPLACE","emby_username":"new-name"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: user}))
	rr := httptest.NewRecorder()
	app.handleUseCode(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("pending entitlement replacement should use register code, status=%d body=%s", rr.Code, rr.Body.String())
	}
	updated, _ := app.store.User(user.UID)
	if !updated.PendingEmby || updated.PendingEmbyDays == nil || *updated.PendingEmbyDays != 7 || updated.EmbyUsername != "new-name" {
		t.Fatalf("register code did not replace pending entitlement cleanly: %#v days=%#v", updated, updated.PendingEmbyDays)
	}
	reg, _ := app.store.RegCode("REG-REPLACE")
	if reg.UseCount != 1 || reg.UsedBy != user.UID {
		t.Fatalf("register code usage was not recorded: %#v", reg)
	}
}

func TestRegisterCodeCapacityExcludesCodeBeingConsumedAndRejectsBoundUser(t *testing.T) {
	app := newTestApp(t)
	app.cfg.EmbyUserLimit = 1
	user, err := app.store.CreateUser(store.User{Username: "capacity-user", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertRegCode(store.RegCode{Code: "REG-CAPACITY", Type: 1, Days: 7, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/use-code", strings.NewReader(`{"reg_code":"REG-CAPACITY"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: user}))
	rr := httptest.NewRecorder()
	app.handleUseCode(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("register code should consume its own reserved capacity slot, status=%d body=%s", rr.Code, rr.Body.String())
	}

	bound, err := app.store.CreateUser(store.User{Username: "bound-user", Role: store.RoleNormal, Active: true, EmbyID: "emby-bound"})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertRegCode(store.RegCode{Code: "REG-BOUND", Type: 1, Days: 7, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/me/use-code", strings.NewReader(`{"reg_code":"REG-BOUND","check_only":true}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: bound}))
	rr = httptest.NewRecorder()
	app.handleUseCode(rr, req, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Emby-bound user should not preview register code as usable, status=%d body=%s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/me/use-code", strings.NewReader(`{"reg_code":"REG-BOUND"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: bound}))
	rr = httptest.NewRecorder()
	app.handleUseCode(rr, req, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Emby-bound user should not use register code as renewal, status=%d body=%s", rr.Code, rr.Body.String())
	}
	reg, _ := app.store.RegCode("REG-BOUND")
	if reg.UseCount != 0 {
		t.Fatalf("rejected register code was consumed: %#v", reg)
	}

	if err := app.store.UpsertRegCode(store.RegCode{Code: "VIP-BOUND", Type: 3, Days: -1, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/me/use-code", strings.NewReader(`{"reg_code":"VIP-BOUND"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: bound}))
	rr = httptest.NewRecorder()
	app.handleUseCode(rr, req, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Emby-bound user should not use whitelist code as registration entitlement, status=%d body=%s", rr.Code, rr.Body.String())
	}
	reg, _ = app.store.RegCode("VIP-BOUND")
	if reg.UseCount != 0 {
		t.Fatalf("rejected whitelist code was consumed: %#v", reg)
	}

	parent, err := app.store.CreateUser(store.User{Username: "invite-parent-bound-check", Role: store.RoleNormal, Active: true, EmbyID: "emby-parent", ExpiredAt: time.Now().AddDate(0, 0, 30).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertInviteCode(store.InviteCode{Code: "INV-BOUND-CHECK", UID: parent.UID, InviterUID: parent.UID, Days: 7, UseCountLimit: 1, Active: true, CreatedAt: time.Now().Unix(), ExpiredAt: time.Now().Add(time.Hour).Unix()}); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/me/use-code", strings.NewReader(`{"reg_code":"INV-BOUND-CHECK","check_only":true}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: bound}))
	rr = httptest.NewRecorder()
	app.handleUseCode(rr, req, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Emby-bound user should not preview invite code as usable, status=%d body=%s", rr.Code, rr.Body.String())
	}
	invite, _ := app.store.InviteCode("INV-BOUND-CHECK")
	if invite.UseCount != 0 {
		t.Fatalf("rejected invite code was consumed: %#v", invite)
	}

	if err := app.store.UpsertRegCode(store.RegCode{Code: "RENEW-BOUND", Type: 2, Days: 7, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/me/use-code", strings.NewReader(`{"reg_code":"RENEW-BOUND"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: bound}))
	rr = httptest.NewRecorder()
	app.handleUseCode(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("Emby-bound user should still use renewal code, status=%d body=%s", rr.Code, rr.Body.String())
	}
	reg, _ = app.store.RegCode("RENEW-BOUND")
	if reg.UseCount != 1 {
		t.Fatalf("renewal code was not consumed: %#v", reg)
	}
}

func TestExpiredInviteCannotBeUsedOrConsumed(t *testing.T) {
	app := newTestApp(t)
	now := time.Now()
	parent, err := app.store.CreateUser(store.User{Username: "expired-invite-parent", Role: store.RoleNormal, Active: true, EmbyID: "emby-parent", ExpiredAt: now.AddDate(0, 0, 30).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.store.CreateUser(store.User{Username: "expired-invite-child", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertInviteCode(store.InviteCode{Code: "INV-EXPIRED-CODE", UID: parent.UID, InviterUID: parent.UID, Days: 7, UseCountLimit: 1, Active: true, CreatedAt: now.Add(-time.Hour).Unix(), ExpiredAt: now.Add(-time.Second).Unix()}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invite/use", strings.NewReader(`{"code":"INV-EXPIRED-CODE"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: child}))
	rr := httptest.NewRecorder()
	app.handleInviteUse(rr, req, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expired invite should be rejected, status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := app.store.ConsumeInviteCode("INV-EXPIRED-CODE", child.UID); !errors.Is(err, store.ErrExpired) {
		t.Fatalf("store should reject expired invite consumption, got %v", err)
	}
	invite, _ := app.store.InviteCode("INV-EXPIRED-CODE")
	if invite.UseCount != 0 || invite.UsedByUID != 0 {
		t.Fatalf("expired invite was consumed: %#v", invite)
	}
}

func TestInviteRenewCodeCreatesTargetedRegCode(t *testing.T) {
	app := newTestApp(t)
	now := time.Now()
	parent, err := app.store.CreateUser(store.User{Username: "renew-parent", Role: store.RoleNormal, Active: true, EmbyID: "emby-parent", ExpiredAt: now.AddDate(0, 0, 30).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.store.CreateUser(store.User{Username: "renew-child", Role: store.RoleNormal, Active: true, ExpiredAt: now.AddDate(0, 0, 1).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := app.store.CreateUser(store.User{Username: "renew-outsider", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertInviteCode(store.InviteCode{Code: "INV-RENEW-PARENT", UID: parent.UID, InviterUID: parent.UID, Days: 7, UseCountLimit: 1, Active: true, CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.ConsumeInviteCode("INV-RENEW-PARENT", child.UID); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invite/renew-codes", strings.NewReader(fmt.Sprintf(`{"target_uid":%d,"days":5,"validity_hours":24}`, child.UID)))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: parent}))
	rr := httptest.NewRecorder()
	app.handleCreateInviteCode(rr, req, nil)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create renew code status=%d body=%s", rr.Code, rr.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	code := env.Data.(map[string]any)["code"].(string)
	reg, ok := app.store.RegCode(code)
	if !ok || reg.Type != 2 || reg.TargetUsername != child.Username || reg.ValidityTime != 24 {
		t.Fatalf("renew code was not stored as targeted regcode: %#v", reg)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/me/renew", strings.NewReader(`{"reg_code":"`+code+`"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: outsider}))
	rr = httptest.NewRecorder()
	app.handleRenew(rr, req, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("outsider should not use targeted renew code, status=%d body=%s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/me/renew", strings.NewReader(`{"reg_code":"`+code+`"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: child}))
	rr = httptest.NewRecorder()
	app.handleRenew(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("target child should use renew code, status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestPublicRegcodeCheckHidesTargetedCodes(t *testing.T) {
	app := newTestApp(t)
	if err := app.store.UpsertRegCode(store.RegCode{Code: "TARGET-SECRET", Type: 2, Days: 5, ValidityTime: -1, UseCountLimit: 1, Active: true, TargetUsername: "alpha"}); err != nil {
		t.Fatal(err)
	}
	resp := doJSON(app, http.MethodPost, "/api/v1/users/regcode/check", `{"reg_code":"TARGET-SECRET"}`, nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("public regcode check should hide targeted codes, status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestInviteUseRevalidatesTreeAndBoundsExpiry(t *testing.T) {
	app := newTestApp(t)
	now := time.Now()
	parentExpiry := now.AddDate(0, 0, 5).Unix()
	parent, err := app.store.CreateUser(store.User{Username: "parent", Role: store.RoleNormal, Active: true, EmbyID: "emby-parent", ExpiredAt: parentExpiry})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.store.CreateUser(store.User{Username: "child", Role: store.RoleNormal, Active: true, ExpiredAt: now.AddDate(0, 0, 60).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertInviteCode(store.InviteCode{Code: "INV-BOUNDS", UID: parent.UID, InviterUID: parent.UID, Days: 30, UseCountLimit: 1, Active: true, CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invite/use", strings.NewReader(`{"code":"INV-BOUNDS"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: child}))
	rr := httptest.NewRecorder()
	app.handleInviteUse(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("invite use status=%d body=%s", rr.Code, rr.Body.String())
	}
	updated, _ := app.store.User(child.UID)
	if updated.ExpiredAt > parentExpiry {
		t.Fatalf("child expiry exceeded parent expiry: child=%d parent=%d", updated.ExpiredAt, parentExpiry)
	}
	if updated.PendingEmbyDays == nil || *updated.PendingEmbyDays > 5 {
		t.Fatalf("pending emby days were not clamped to inviter expiry: %#v", updated.PendingEmbyDays)
	}

	otherParent, err := app.store.CreateUser(store.User{Username: "other-parent", Role: store.RoleNormal, Active: true, EmbyID: "emby-other", ExpiredAt: now.AddDate(0, 0, 30).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertInviteCode(store.InviteCode{Code: "INV-SECOND", UID: otherParent.UID, InviterUID: otherParent.UID, Days: 7, UseCountLimit: 1, Active: true, CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/invite/use", strings.NewReader(`{"code":"INV-SECOND"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: child}))
	rr = httptest.NewRecorder()
	app.handleInviteUse(rr, req, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("second parent should be rejected, status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestInviteUseRejectsExpiredInviterAndRootLimit(t *testing.T) {
	app := newTestApp(t)
	now := time.Now()
	expiredParent, err := app.store.CreateUser(store.User{Username: "expired-parent", Role: store.RoleNormal, Active: true, EmbyID: "emby-expired", ExpiredAt: now.AddDate(0, 0, -1).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := app.store.CreateUser(store.User{Username: "candidate", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertInviteCode(store.InviteCode{Code: "INV-EXPIRED-PARENT", UID: expiredParent.UID, InviterUID: expiredParent.UID, Days: 30, UseCountLimit: 1, Active: true, CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invite/use", strings.NewReader(`{"code":"INV-EXPIRED-PARENT"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: candidate}))
	rr := httptest.NewRecorder()
	app.handleInviteUse(rr, req, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expired inviter should be rejected, status=%d body=%s", rr.Code, rr.Body.String())
	}

	app.cfg.InviteRootUserLimit = 1
	root, err := app.store.CreateUser(store.User{Username: "root", Role: store.RoleNormal, Active: true, EmbyID: "emby-root", ExpiredAt: now.AddDate(0, 0, 30).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.store.CreateUser(store.User{Username: "child2", Role: store.RoleNormal, Active: true, EmbyID: "emby-child2", ExpiredAt: now.AddDate(0, 0, 20).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertInviteCode(store.InviteCode{Code: "INV-ROOT-CHILD", UID: root.UID, InviterUID: root.UID, Days: 10, UseCountLimit: 1, Active: true, CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.ConsumeInviteCode("INV-ROOT-CHILD", child.UID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := app.canInvite(child); ok {
		t.Fatal("child should not be allowed to create more invites after root tree reaches limit")
	}
	grandchild, err := app.store.CreateUser(store.User{Username: "grandchild", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertInviteCode(store.InviteCode{Code: "INV-GRANDCHILD", UID: child.UID, InviterUID: child.UID, Days: 10, UseCountLimit: 1, Active: true, CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/invite/use", strings.NewReader(`{"code":"INV-GRANDCHILD"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: grandchild}))
	rr = httptest.NewRecorder()
	app.handleInviteUse(rr, req, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("root limit should reject grandchild, status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestTelegramGrantRegisterActionSetsPendingEntitlement(t *testing.T) {
	app := newTestApp(t)
	app.cfg.InviteDefaultDays = 12
	target, err := app.store.CreateUser(store.User{Username: "tg-target", Role: store.RoleUnrecognized, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	panel := telegramPanelContext{Token: "grant", TargetUID: target.UID, ExpiresAt: time.Now().Add(time.Minute).Unix()}
	app.telegramSavePanel(panel)
	app.telegramApplyPanelAction(context.Background(), panel, "grant_register")
	updated, _ := app.store.User(target.UID)
	if !updated.PendingEmby || updated.PendingEmbyDays == nil || *updated.PendingEmbyDays != 12 || updated.Role != store.RoleNormal {
		t.Fatalf("grant_register did not create pending entitlement: %#v days=%#v", updated, updated.PendingEmbyDays)
	}

	admin, err := app.store.CreateUser(store.User{Username: "protected-admin", Role: store.RoleAdmin, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	adminPanel := telegramPanelContext{Token: "admin-grant", TargetUID: admin.UID, ExpiresAt: time.Now().Add(time.Minute).Unix()}
	app.telegramSavePanel(adminPanel)
	app.telegramApplyPanelAction(context.Background(), adminPanel, "grant_register")
	protected, _ := app.store.User(admin.UID)
	if protected.PendingEmby {
		t.Fatal("grant_register should not mutate protected admin accounts")
	}
}

func TestTelegramRosterStatsUsesObservedMembers(t *testing.T) {
	app := newTestApp(t)
	app.cfg.TelegramGroupIDs = []string{"-1001"}
	if err := app.store.UpsertTelegramRoster("-1001", 100, "member", false); err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertTelegramRoster("-1001", 200, "member", false); err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpsertTelegramRoster("-1001", 300, "member", true); err != nil {
		t.Fatal(err)
	}
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"admin123456"}`, nil)
	admin, _ := app.store.FindUserByUsername("admin")
	_, _ = app.store.UpdateUser(admin.UID, func(u *store.User) error { u.TelegramID = 100; return nil })
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	resp := doJSONWithHeaders(app, http.MethodGet, "/api/v1/admin/telegram/roster/stats", ``, []*http.Cookie{cookie}, nil)
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"bound":1`) || !strings.Contains(resp.Body.String(), `"unbound":1`) || !strings.Contains(resp.Body.String(), `"bots":1`) {
		t.Fatalf("roster stats status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func doJSON(app *App, method, path, body string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	return doJSONWithHeaders(app, method, path, body, cookies, nil)
}

func doJSONWithHeaders(app *App, method, path, body string, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)
	return rr
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
