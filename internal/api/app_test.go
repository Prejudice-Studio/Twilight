package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"log/slog"
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
		MediaRequestEnabled:          true,
		MaxConcurrentRequestsPerUser: 3,
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
		InstallRuntimeLogger(io.Discard, slog.LevelInfo)
	})

	var out bytes.Buffer
	InstallRuntimeLogger(&out, slog.LevelWarn)
	ConfigureRuntimeLogging(slog.LevelWarn, 20)

	slog.Info("runtime info should be filtered")
	slog.Warn("runtime warn should be captured", "token", "secret-value")
	log.Print("runtime standard log should be captured")

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
		InstallRuntimeLogger(io.Discard, slog.LevelInfo)
	})
	InstallRuntimeLogger(io.Discard, slog.LevelError)
	ConfigureRuntimeLogging(slog.LevelError, 20)
	app.cfg.LogLevel = "error"
	writeRuntimeConfig("error", 20)
	app.configSignature = configFileSignature(app.cfg.ConfigFile)

	time.Sleep(5 * time.Millisecond)
	writeRuntimeConfig("debug", 21)
	app.reloadConfigIfChanged()

	if app.cfg.LogLevel != "debug" || app.cfg.RuntimeLogLimit != 21 {
		t.Fatalf("config was not hot reloaded: level=%q limit=%d", app.cfg.LogLevel, app.cfg.RuntimeLogLimit)
	}
	slog.Debug("debug after hot reload")
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
