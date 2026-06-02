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
	"sync/atomic"
	"testing"
	"time"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/security"
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

// registerAndLogin 注册并登录指定账户，返回 session cookie 切片。
func registerAndLogin(t *testing.T, app *App, username, password string) []*http.Cookie {
	t.Helper()
	register := doJSON(app, http.MethodPost, "/api/v1/users/register", fmt.Sprintf(`{"username":%q,"password":%q}`, username, password), nil)
	if register.Code != http.StatusCreated {
		t.Fatalf("register %s status = %d body=%s", username, register.Code, register.Body.String())
	}
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", fmt.Sprintf(`{"username":%q,"password":%q}`, username, password), nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login %s status = %d body=%s", username, login.Code, login.Body.String())
	}
	session := findCookie(login.Result().Cookies(), "twilight_session")
	if session == nil {
		t.Fatalf("missing session cookie for %s", username)
	}
	return []*http.Cookie{session}
}

// loginCookies 从已存在的账户登录，返回 session cookie 切片。
// 仅用于 inline 登录流程（不走注册 helper）。
func loginCookies(t *testing.T, app *App, username, password string) []*http.Cookie {
	t.Helper()
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", fmt.Sprintf(`{"username":%q,"password":%q}`, username, password), nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login %s status=%d body=%s", username, login.Code, login.Body.String())
	}
	all := login.Result().Cookies()
	session := findCookie(all, "twilight_session")
	if session == nil {
		t.Fatalf("login %s missing session cookie", username)
	}
	return []*http.Cookie{session}
}

func TestRegcodeWritesBlockedWhenRuntimeDatabaseMismatchesConfig(t *testing.T) {
	app := newTestApp(t)
	app.cfg().DatabaseDriver = store.BackendPostgres

	rec := httptest.NewRecorder()
	if !app.rejectRegcodeWriteIfStorageMismatch(rec) {
		t.Fatal("expected regcode writes to be blocked when configured database differs from active store")
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected conflict status, got %d", rec.Code)
	}

	app.cfg().DatabaseDriver = store.BackendJSON
	rec = httptest.NewRecorder()
	if app.rejectRegcodeWriteIfStorageMismatch(rec) {
		t.Fatal("regcode writes were blocked even though active and configured databases match")
	}
}

func TestAuthFlowWithoutCSRF(t *testing.T) {
	app := newTestApp(t)

	register := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	if register.Code != http.StatusCreated {
		t.Fatalf("register status = %d body=%s", register.Code, register.Body.String())
	}

	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", login.Code, login.Body.String())
	}
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	if cookie == nil || !cookie.HttpOnly {
		t.Fatalf("expected httponly session cookie, got %#v", cookie)
	}
	if csrfCookie := findCookie(login.Result().Cookies(), "twilight_session_csrf"); csrfCookie != nil {
		t.Fatalf("csrf cookie should not be issued, got %#v", csrfCookie)
	}

	me := doJSON(app, http.MethodGet, "/api/v1/users/me", ``, []*http.Cookie{cookie})
	if me.Code != http.StatusOK {
		t.Fatalf("me status = %d body=%s", me.Code, me.Body.String())
	}
	if me.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing security header")
	}

	allowed := doJSON(app, http.MethodPut, "/api/v1/users/me", `{"email":"a@example.com"}`, []*http.Cookie{cookie})
	if allowed.Code != http.StatusOK {
		t.Fatalf("expected 200 without csrf token, got status=%d body=%s", allowed.Code, allowed.Body.String())
	}
}

func TestCredentialedCORSRequiresExplicitOrigin(t *testing.T) {
	app := newTestApp(t)
	app.cfg().CORSOrigins = []string{"*"}

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/users/me", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", "PUT")
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)

	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("wildcard CORS origin was allowed: %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}

	app.cfg().CORSOrigins = []string{"https://panel.example/"}
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

	app.cfg().CORSOrigins = []string{"https://panel.example/app"}
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
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
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

func TestAPIKeyLoginIsRateLimited(t *testing.T) {
	app := newTestApp(t)
	app.cfg().RateLimitEnabled = true
	app.cfg().RateLimitLoginPerMinute = 1
	app.cfg().RateLimitLoginUserPer5m = 1

	first := doJSON(app, http.MethodPost, "/api/v1/auth/login/apikey", `{"apikey":"key-invalid-one"}`, nil)
	if first.Code != http.StatusUnauthorized || !strings.Contains(first.Body.String(), `"error_code":"AUTH_APIKEY_INVALID"`) {
		t.Fatalf("first invalid API key login status=%d body=%s", first.Code, first.Body.String())
	}
	second := doJSON(app, http.MethodPost, "/api/v1/auth/login/apikey", `{"apikey":"key-invalid-two"}`, nil)
	if second.Code != http.StatusTooManyRequests || !strings.Contains(second.Body.String(), `"error_code":"AUTH_LOGIN_RATE_LIMITED"`) {
		t.Fatalf("second invalid API key login status=%d body=%s", second.Code, second.Body.String())
	}
}

// TestAPIKeyAuthSources 锁定 authenticateAPIKey 的三条入口（X-API-Key 头 /
// Authorization Bearer/ApiKey / ?apikey= 查询串）以及 AllowQuery 门禁的回归
// 行为：query 路径在 AllowQuery=false 时必须 401（避免 referer / log 中泄露
// 的 key 被重放），同时携带 header 与 query 时 header 优先（保留旧客户端兼
// 容性，又不会被一个开放的 query 串旁路掉 AllowQuery 限制）。
func TestAPIKeyAuthSources(t *testing.T) {
	app := newTestApp(t)
	cookies := registerAndLogin(t, app, "alice", "Password123456")

	createKey := func(name string, allowQuery bool) string {
		body := fmt.Sprintf(`{"name":%q,"rate_limit":120,"allow_query":%t}`, name, allowQuery)
		rec := doJSONWithHeaders(app, http.MethodPost, "/api/v1/users/me/apikeys", body, cookies, map[string]string{"X-Twilight-Client": "webui"})
		if rec.Code != http.StatusOK {
			t.Fatalf("create key %s status=%d body=%s", name, rec.Code, rec.Body.String())
		}
		var env envelope
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode created key %s: %v", name, err)
		}
		key, _ := env.Data.(map[string]any)["key"].(string)
		if !strings.HasPrefix(key, "key-") {
			t.Fatalf("expected plaintext key prefix for %s, got %q", name, key)
		}
		return key
	}

	queryKey := createKey("ci-query", true)
	headerOnly := createKey("ci-header-only", false)

	call := func(req *http.Request) int {
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		return rr.Code
	}

	// (a) ?apikey= 查询路径配 AllowQuery=true 必须通过
	r := httptest.NewRequest(http.MethodGet, "/api/v1/apikey/info?apikey="+queryKey, nil)
	if got := call(r); got != http.StatusOK {
		t.Fatalf("query+AllowQuery=true expected 200, got %d", got)
	}

	// (b) ?apikey= 查询路径配 AllowQuery=false 必须 401
	r = httptest.NewRequest(http.MethodGet, "/api/v1/apikey/info?apikey="+headerOnly, nil)
	if got := call(r); got != http.StatusUnauthorized {
		t.Fatalf("query+AllowQuery=false expected 401, got %d", got)
	}

	// (c1) Authorization: Bearer 必须通过（与 X-API-Key 等价）
	r = httptest.NewRequest(http.MethodGet, "/api/v1/apikey/info", nil)
	r.Header.Set("Authorization", "Bearer "+headerOnly)
	if got := call(r); got != http.StatusOK {
		t.Fatalf("Authorization Bearer expected 200, got %d", got)
	}

	// (c2) Authorization: ApiKey 必须通过（同上，区分大小写不敏感）
	r = httptest.NewRequest(http.MethodGet, "/api/v1/apikey/info", nil)
	r.Header.Set("Authorization", "ApiKey "+headerOnly)
	if got := call(r); got != http.StatusOK {
		t.Fatalf("Authorization ApiKey expected 200, got %d", got)
	}

	// (d) header 与 query 同时存在时 header 优先：用 header-only key 走 header，
	// 同时附一个不存在的 ?apikey=garbage —— 既不应当 401（不要被 query 干扰），
	// 也不应当因为 query 路径而触发 AllowQuery 检查。
	r = httptest.NewRequest(http.MethodGet, "/api/v1/apikey/info?apikey=key-garbage", nil)
	r.Header.Set("X-API-Key", headerOnly)
	if got := call(r); got != http.StatusOK {
		t.Fatalf("header overrides query expected 200, got %d", got)
	}

	// (e) 错误 query key 必须 401（防御性：错 key 不能被当成无 key 兜底匿名通过）
	r = httptest.NewRequest(http.MethodGet, "/api/v1/apikey/info?apikey=key-bogus", nil)
	if got := call(r); got != http.StatusUnauthorized {
		t.Fatalf("invalid query key expected 401, got %d", got)
	}
}

// TestAPIKeyDisableAccountKillsSessions 锁定 disable-account 的两个不变量：
// 1) cookie session 立即失效（不依赖 SessionTTL 自然过期）；2) 同账号下既
// 存的 API key 被 authenticateAPIKey:!u.Active 立即拒绝。任何后续把 Active
// 改成软删除标志或把 DeleteUser 拆成异步任务的重构都会触发这个测试。
func TestAPIKeyDisableAccountKillsSessions(t *testing.T) {
	app := newTestApp(t)
	cookies := registerAndLogin(t, app, "bob", "Password123456")

	created := doJSONWithHeaders(app, http.MethodPost, "/api/v1/users/me/apikeys", `{"name":"kill-me","rate_limit":50}`, cookies, map[string]string{"X-Twilight-Client": "webui"})
	if created.Code != http.StatusOK {
		t.Fatalf("create key status=%d body=%s", created.Code, created.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(created.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	key, _ := env.Data.(map[string]any)["key"].(string)
	if !strings.HasPrefix(key, "key-") {
		t.Fatalf("expected plaintext key prefix, got %q", key)
	}

	// 主动 disable 自己（走 API key 路径，与 webui 调用一致）
	disableReq := httptest.NewRequest(http.MethodPost, "/api/v1/apikey/disable", nil)
	disableReq.Header.Set("X-API-Key", key)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, disableReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 1) cookie session 必须立刻被踢
	meRec := doJSONWithHeaders(app, http.MethodGet, "/api/v1/users/me", "", cookies, nil)
	if meRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected cookie 401 after disable, got %d body=%s", meRec.Code, meRec.Body.String())
	}

	// 2) 同账号下的 API key 也必须立刻 401（!u.Active 兜底）
	apiReq := httptest.NewRequest(http.MethodGet, "/api/v1/apikey/info", nil)
	apiReq.Header.Set("X-API-Key", key)
	apiRec := httptest.NewRecorder()
	app.ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected api key 401 after disable, got %d body=%s", apiRec.Code, apiRec.Body.String())
	}

	// 3) 状态层面校验 User.Active=false（防止"看起来 401 但其实是别的兜底"）
	u, ok := app.store().FindUserByUsername("bob")
	if !ok {
		t.Fatalf("user bob disappeared from store after disable")
	}
	if u.Active {
		t.Fatalf("expected User.Active=false after disable, got true")
	}
}

// TestCheckExpiredKillsInvitedUserSessions 锁定 R60-1 整改后的不变量：
// check_expired job 处理 invited 用户时，即便保留 Active=true 让用户能重
// 新登录续期，已经过期的时刻必须立刻让现有 cookie session 失效。否则
// stale token 在 SessionTTL 内仍能访问受保护接口，与 R51-2 锁定的
// "disable-account 必须立即踢 session" 语义不一致。non-invited 分支沿用
// 原本就有的 sessions().DeleteUser，保留断言以防误把整段都拆掉。
func TestCheckExpiredKillsInvitedUserSessions(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()

	// invited user (有 InviteRelation)
	invited, err := app.store().CreateUser(store.User{Username: "invitee", Role: store.RoleNormal, Active: true, ExpiredAt: time.Now().Add(-time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	// 直接构造 invite code 并 ConsumeInviteCode 走真实路径，让 ParentOf 命中
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "INVR60", InviterUID: 999, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().ConsumeInviteCode("INVR60", invited.UID); err != nil {
		t.Fatal(err)
	}
	if _, ok := app.store().ParentOf(invited.UID); !ok {
		t.Fatalf("ParentOf %d should hit invite relation after ConsumeInviteCode", invited.UID)
	}

	// non-invited user
	standalone, err := app.store().CreateUser(store.User{Username: "standalone", Role: store.RoleNormal, Active: true, ExpiredAt: time.Now().Add(-time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}

	invitedToken, _, err := app.sessions().Create(ctx, invited.UID)
	if err != nil {
		t.Fatal(err)
	}
	standaloneToken, _, err := app.sessions().Create(ctx, standalone.UID)
	if err != nil {
		t.Fatal(err)
	}

	// pre-condition: 两个 session 都活
	if _, ok := app.sessions().Get(ctx, invitedToken); !ok {
		t.Fatalf("invited session must exist before check_expired")
	}
	if _, ok := app.sessions().Get(ctx, standaloneToken); !ok {
		t.Fatalf("standalone session must exist before check_expired")
	}

	req := httptest.NewRequest(http.MethodPost, "/scheduler/internal", nil)
	if _, _, err := app.runSchedulerJob(req, "check_expired"); err != nil {
		t.Fatalf("runSchedulerJob check_expired returned err: %v", err)
	}

	// invited: Active 保留为 true（走续期路径），但 session 必须已被踢
	if u, ok := app.store().User(invited.UID); !ok {
		t.Fatalf("invited user disappeared")
	} else if !u.Active {
		t.Fatalf("invited user Active should remain true (renewal path), got false")
	}
	if _, ok := app.sessions().Get(ctx, invitedToken); ok {
		t.Fatalf("invited user session should be killed after expiry, but still alive")
	}

	// standalone: Active=false 且 session 也被踢（兜底确保原来逻辑没回退）
	if u, ok := app.store().User(standalone.UID); !ok {
		t.Fatalf("standalone user disappeared")
	} else if u.Active {
		t.Fatalf("standalone user Active should be false after check_expired")
	}
	if _, ok := app.sessions().Get(ctx, standaloneToken); ok {
		t.Fatalf("standalone user session should be killed after expiry, but still alive")
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
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
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
	app.cfg().ConfigFile = filepath.Join(app.cfg().DatabaseDir, "config.toml")
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
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
	if !strings.HasPrefix(app.cfg().ServerIcon, "server-icon/") || !strings.HasSuffix(app.cfg().ServerIcon, ".png") {
		t.Fatalf("server_icon was not updated to uploaded asset: %q", app.cfg().ServerIcon)
	}
	if _, _, ok := app.configuredServerIconPath(); !ok {
		t.Fatalf("uploaded server icon is not readable from configured path: %q", app.cfg().ServerIcon)
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
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")

	validName := "0123456789abcdef.png"
	avatarDir := filepath.Join(app.cfg().UploadDir, "avatar")
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
	app.cfg().ConfigFile = filepath.Join(app.cfg().DatabaseDir, "config.toml")
	databaseConfig := "[Database]\n" +
		"driver = \"json\"\n" +
		"state_file = " + strconv.Quote(app.cfg().StateFile) + "\n" +
		"backup_dir = " + strconv.Quote(app.cfg().DatabaseBackupDir) + "\n"
	existing := "[Global]\nserver_name = \"old\"\n\n" + databaseConfig + "\nadmin_uids = \"2\"\nadmin_usernames = \"alice\"\n"
	if err := os.WriteFile(app.cfg().ConfigFile, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	if shown := stripProtectedAdminConfig(existing); strings.Contains(shown, "admin_uids") || strings.Contains(shown, "admin_usernames") {
		t.Fatalf("protected admin config leaked: %s", shown)
	}
	info, status, message := app.saveConfigContent("[Global]\nserver_name = \"new\"\n\n" + databaseConfig + "\n[Admin]\nadmin_uids = \"999\"\nadmin_usernames = \"mallory\"\n")
	if status != http.StatusOK {
		t.Fatalf("saveConfigContent status=%d message=%s info=%v", status, message, info)
	}
	data, err := os.ReadFile(app.cfg().ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	// 必须用完整 key=value 形式断言，避免 Windows 临时目录里随机数字（典型如
	// `...Temp\TestProtected...1493999686\001\state.json`）巧合包含 "999"
	// 子串触发误报。"mallory" 同理保持显式 key=value 检查。
	if strings.Contains(content, `admin_uids = "999"`) || strings.Contains(content, `admin_usernames = "mallory"`) {
		t.Fatalf("submitted protected admin config was not stripped: %s", content)
	}
	if !strings.Contains(content, `admin_uids = "2"`) || !strings.Contains(content, `admin_usernames = "alice"`) {
		t.Fatalf("existing protected admin config was not preserved: %s", content)
	}

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"owner","password":"Owner123456"}`, nil)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"alice","password":"Alice123456"}`, nil)
	alice, ok := app.store().FindUserByUsername("alice")
	if !ok || alice.Role != store.RoleAdmin || !alice.Active {
		t.Fatalf("configured admin username was not applied on registration: %#v", alice)
	}
}

func TestBackgroundConfigIsSanitized(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
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
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	adminLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
	adminCookie := findCookie(adminLogin.Result().Cookies(), "twilight_session")

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"user","password":"User123456"}`, nil)
	userLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"user","password":"User123456"}`, nil)
	userCookie := findCookie(userLogin.Result().Cookies(), "twilight_session")
	user, _ := app.store().FindUserByUsername("user")
	_, _ = app.store().UpdateUser(user.UID, func(u *store.User) error { u.TelegramID = 12345; return nil })

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
	missingConfirmDelete := doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/regcodes/batch-delete", `{"codes":["`+rawBatchCodes[0].(string)+`"]}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if missingConfirmDelete.Code != http.StatusBadRequest || !strings.Contains(missingConfirmDelete.Body.String(), confirmBatchDeleteRegcodes) || !strings.Contains(missingConfirmDelete.Body.String(), `"error_code":"REGCODE_BATCH_CONFIRM_REQUIRED"`) {
		t.Fatalf("batch delete regcodes missing confirm status=%d body=%s", missingConfirmDelete.Code, missingConfirmDelete.Body.String())
	}
	deletePayload := `{"codes":["` + code + `","` + rawBatchCodes[0].(string) + `","` + rawBatchCodes[1].(string) + `","missing-code"],"confirm":"` + confirmBatchDeleteRegcodes + `"}`
	batchDelete := doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/regcodes/batch-delete", deletePayload, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if batchDelete.Code != http.StatusOK || !strings.Contains(batchDelete.Body.String(), `"deleted":3`) || !strings.Contains(batchDelete.Body.String(), `"missing":1`) {
		t.Fatalf("batch delete regcodes status=%d body=%s", batchDelete.Code, batchDelete.Body.String())
	}
	if _, ok := app.store().RegCode(code); ok {
		t.Fatal("used regcode should be physically deleted by batch delete")
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
	userRequests := app.store().ListMediaRequests(user.UID, false)
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
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
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
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
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
	app.cfg().MediaRequestEnabled = false
	app.cfg().SigninEnabled = false
	app.cfg().InviteEnabled = false

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

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	signin := doJSONWithHeaders(app, http.MethodPost, "/api/v1/signin", `{}`, []*http.Cookie{cookie}, map[string]string{"X-Twilight-Client": "webui"})
	if signin.Code != http.StatusForbidden {
		t.Fatalf("disabled signin status=%d body=%s", signin.Code, signin.Body.String())
	}
	inviteMe := doJSONWithHeaders(app, http.MethodGet, "/api/v1/invite/me", ``, []*http.Cookie{cookie}, map[string]string{"X-Twilight-Client": "webui"})
	if inviteMe.Code != http.StatusOK || !strings.Contains(inviteMe.Body.String(), `"enabled":false`) || !strings.Contains(inviteMe.Body.String(), `"can_invite":false`) {
		t.Fatalf("disabled invite/me status=%d body=%s", inviteMe.Code, inviteMe.Body.String())
	}
	adminRequests := doJSONWithHeaders(app, http.MethodGet, "/api/v1/admin/media-requests", ``, []*http.Cookie{cookie}, map[string]string{"X-Twilight-Client": "webui"})
	if adminRequests.Code != http.StatusOK {
		t.Fatalf("disabled media admin requests status=%d body=%s", adminRequests.Code, adminRequests.Body.String())
	}
	inviteTree := doJSONWithHeaders(app, http.MethodGet, "/api/v1/admin/invite/tree", ``, []*http.Cookie{cookie}, map[string]string{"X-Twilight-Client": "webui"})
	if inviteTree.Code != http.StatusOK || !strings.Contains(inviteTree.Body.String(), `"enabled":false`) {
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
	app.cfg().EmbyURL = emby.URL
	app.cfg().EmbyToken = "test-token"

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
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

func TestMediaRequestIgnoresUserSkipInventoryCheck(t *testing.T) {
	app := newTestApp(t)
	app.cfg().EmbyToken = "test-token"
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("AnyProviderIdEquals") == "Tmdb.550" {
			_, _ = w.Write([]byte(`{"Items":[{"Id":"movie1","Name":"Existing Movie","Type":"Movie","ProductionYear":1999,"ProviderIds":{"Tmdb":"550"}}],"TotalRecordCount":1}`))
			return
		}
		_, _ = w.Write([]byte(`{"Items":[],"TotalRecordCount":0}`))
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	_ = registerAndLogin(t, app, "admin", "Admin123456")
	userCookies := registerAndLogin(t, app, "requester", "User123456")
	user, ok := app.store().FindUserByUsername("requester")
	if !ok {
		t.Fatal("requester not found")
	}
	if _, err := app.store().UpdateUser(user.UID, func(u *store.User) error { u.TelegramID = 12345; return nil }); err != nil {
		t.Fatal(err)
	}

	resp := doJSONWithHeaders(app, http.MethodPost, "/api/v1/media/request", `{"source":"tmdb","media_id":550,"media_type":"movie","title":"Existing Movie","skip_inventory_check":true}`, userCookies, map[string]string{"X-Twilight-Client": "webui"})
	if resp.Code != http.StatusBadRequest || !strings.Contains(resp.Body.String(), `"error_code":"MEDIA_REQUEST_ALREADY_EXISTS"`) {
		t.Fatalf("media request status=%d body=%s", resp.Code, resp.Body.String())
	}
	if requests := app.store().ListMediaRequests(user.UID, false); len(requests) != 0 {
		t.Fatalf("user bypassed inventory check and created requests: %#v", requests)
	}
}

func TestMediaRequestStatusUpdatesRequireExplicitStatus(t *testing.T) {
	app := newTestApp(t)
	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")
	created, err := app.store().CreateMediaRequest(store.MediaRequest{UID: 1, TelegramID: 99, Username: "admin", Title: "Pending", Source: "tmdb", MediaID: 42, MediaType: "movie"})
	if err != nil {
		t.Fatal(err)
	}

	adminResp := doJSONWithHeaders(app, http.MethodPut, "/api/v1/admin/media-requests/"+strconv.FormatInt(created.ID, 10), `{}`, adminCookies, map[string]string{"X-Twilight-Client": "webui"})
	if adminResp.Code != http.StatusBadRequest || !strings.Contains(adminResp.Body.String(), `"error_code":"MEDIA_REQUEST_STATUS_INVALID"`) {
		t.Fatalf("admin empty status update status=%d body=%s", adminResp.Code, adminResp.Body.String())
	}
	updated, _ := app.store().MediaRequest(created.ID)
	if updated.Status != "UNHANDLED" {
		t.Fatalf("empty admin status update changed status to %q", updated.Status)
	}

	app.cfg().BotInternalSecret = "secret"
	externalResp := doJSONWithHeaders(app, http.MethodPost, "/api/v1/media/request/external/update", fmt.Sprintf(`{"key":%q}`, created.RequireKey), nil, map[string]string{"X-Internal-Secret": "secret"})
	if externalResp.Code != http.StatusBadRequest || !strings.Contains(externalResp.Body.String(), `"error_code":"MEDIA_REQUEST_STATUS_INVALID"`) {
		t.Fatalf("external empty status update status=%d body=%s", externalResp.Code, externalResp.Body.String())
	}
	updated, _ = app.store().MediaRequest(created.ID)
	if updated.Status != "UNHANDLED" {
		t.Fatalf("empty external status update changed status to %q", updated.Status)
	}
}

func TestEmbyURLsDoNotFallbackToInternalServerURL(t *testing.T) {
	app := newTestApp(t)
	app.cfg().EmbyURL = "http://127.0.0.1:8096"
	app.cfg().EmbyURLList = nil
	app.cfg().EmbyPublicURL = ""
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	admin, _ := app.store().FindUserByUsername("admin")
	_, _ = app.store().UpdateUser(admin.UID, func(u *store.User) error { u.EmbyID = "emby-admin"; return nil })
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	resp := doJSONWithHeaders(app, http.MethodGet, "/api/v1/system/emby-urls", ``, []*http.Cookie{cookie}, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("emby urls status=%d body=%s", resp.Code, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), app.cfg().EmbyURL) {
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
	app.cfg().BangumiAPIURL = bgm.URL

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
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
	app.cfg().BangumiAPIURL = bgm.URL

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	resp := doJSON(app, http.MethodGet, "/api/v1/media/search?q=test&source=bangumi", ``, []*http.Cookie{cookie})
	if resp.Code != http.StatusBadGateway || !strings.Contains(resp.Body.String(), "Bangumi 搜索失败") {
		t.Fatalf("bangumi failure status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestMediaDetailSanitizesPathPartsBeforeUpstreamCall(t *testing.T) {
	app := newTestApp(t)
	app.cfg().TMDBAPIKey = "tmdb-key"
	var gotPath string
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if gotPath != "/movie/123" {
			t.Fatalf("unexpected TMDB detail path %q", gotPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":123,"title":"Safe Movie","media_type":"movie"}`))
	}))
	defer tmdb.Close()
	app.cfg().TMDBAPIURL = tmdb.URL

	cookies := registerAndLogin(t, app, "admin", "Admin123456")
	resp := doJSON(app, http.MethodGet, "/api/v1/media/detail?source=tmdb&media_id=123&media_type=movie%2F..%2F..%2Fauthentication", ``, cookies)
	if resp.Code != http.StatusOK || gotPath != "/movie/123" || strings.Contains(resp.Body.String(), "authentication") {
		t.Fatalf("media detail status=%d path=%q body=%s", resp.Code, gotPath, resp.Body.String())
	}

	badID := doJSON(app, http.MethodGet, "/api/v1/media/detail?source=tmdb&media_id=123%2F..%2Fsecret&media_type=movie", ``, cookies)
	if badID.Code != http.StatusBadRequest || !strings.Contains(badID.Body.String(), `"error_code":"MEDIA_REQUEST_PAYLOAD_EMPTY"`) {
		t.Fatalf("bad media id status=%d body=%s", badID.Code, badID.Body.String())
	}
}

func TestSystemUpdateRejectsUnsafeRepoURL(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	resp := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/update", `{"repo_url":"https://user:pass@example.com/repo.git","branch":"main"}`, []*http.Cookie{cookie}, map[string]string{"X-Twilight-Client": "webui"})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unsafe update URL status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestRuntimeLogsRequireAdminAndRedactSecrets(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	adminLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
	adminCookie := findCookie(adminLogin.Result().Cookies(), "twilight_session")
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"user","password":"User123456"}`, nil)
	userLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"user","password":"User123456"}`, nil)
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
	// Emby / MediaBrowser / Session 变体必须被脱敏
	for _, sample := range []string{
		`emby_token=secret-emby-token-XYZ123`,
		`X-Emby-Token: super-secret-emby-12345`,
		`MediaBrowser Client="Twilight", Token="emby-token-deadbeef-9876", DeviceId="dev"`,
		`session_id=sess-abcdef-1234567890`,
	} {
		out := redactSensitiveText(sample)
		// 任何 12+ 长度连续 alnum/-/_ 的真实值都不应残留
		for _, leak := range []string{"secret-emby-token-XYZ123", "super-secret-emby-12345", "emby-token-deadbeef-9876", "sess-abcdef-1234567890"} {
			if strings.Contains(out, leak) {
				t.Fatalf("variant secret was not redacted: input=%q output=%q leak=%q", sample, out, leak)
			}
		}
	}
	for _, key := range []string{"apiKey", "api-key", "authorization", "postgres_dsn", "bot.token", "X-Emby-Token", "emby_authorization", "MediaBrowserToken", "session_id"} {
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
	app.cfg().ConfigFile = filepath.Join(app.cfg().DatabaseDir, "config.toml")
	writeRuntimeConfig := func(level string, limit int) {
		t.Helper()
		content := "[Global]\n" +
			"databases_dir = " + strconv.Quote(app.cfg().DatabaseDir) + "\n" +
			"log_level = " + strconv.Quote(level) + "\n" +
			"runtime_log_limit = " + strconv.Itoa(limit) + "\n\n" +
			"[Database]\n" +
			"driver = " + strconv.Quote(app.cfg().DatabaseDriver) + "\n" +
			"backup_dir = " + strconv.Quote(app.cfg().DatabaseBackupDir) + "\n" +
			"state_file = " + strconv.Quote(app.cfg().StateFile) + "\n"
		if err := os.WriteFile(app.cfg().ConfigFile, []byte(content), 0o600); err != nil {
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
	app.cfg().LogLevel = "error"
	writeRuntimeConfig("error", 20)
	app.configSignature = configFileSignature(app.cfg().ConfigFile)

	time.Sleep(5 * time.Millisecond)
	writeRuntimeConfig("debug", 21)
	app.reloadConfigIfChanged()

	if app.cfg().LogLevel != "debug" || app.cfg().RuntimeLogLimit != 21 {
		t.Fatalf("config was not hot reloaded: level=%q limit=%d", app.cfg().LogLevel, app.cfg().RuntimeLogLimit)
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

// TestReloadConfigSurfacesLiveAppliedFields 锁定 reload 响应里的
// `live_applied` 列表必须列出本次实际生效的 cookie 配置（session_cookie /
// cookie_secure / cookie_samesite / cookie_domain），并且写出来的 cookie header 立刻反映
// 新配置。这是 R55-2 整改的可观察性合同：cookie 字段不会"沉默成功"也不会
// "沉默失败"——值真的变了就出现在 live_applied，下一次 setSessionCookie
// 就用新值。
func TestReloadConfigSurfacesLiveAppliedFields(t *testing.T) {
	app := newTestApp(t)
	app.cfg().ConfigFile = filepath.Join(app.cfg().DatabaseDir, "config.toml")

	writeCfg := func(cookieName, sameSite string, secure bool, domain string) {
		t.Helper()
		content := "[Global]\n" +
			"databases_dir = " + strconv.Quote(app.cfg().DatabaseDir) + "\n\n" +
			"[Database]\n" +
			"driver = " + strconv.Quote(app.cfg().DatabaseDriver) + "\n" +
			"backup_dir = " + strconv.Quote(app.cfg().DatabaseBackupDir) + "\n" +
			"state_file = " + strconv.Quote(app.cfg().StateFile) + "\n\n" +
			"[Security]\n" +
			"session_cookie_name = " + strconv.Quote(cookieName) + "\n" +
			"session_cookie_samesite = " + strconv.Quote(sameSite) + "\n" +
			"session_cookie_secure = " + strconv.FormatBool(secure) + "\n" +
			"session_cookie_domain = " + strconv.Quote(domain) + "\n"
		if err := os.WriteFile(app.cfg().ConfigFile, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// 起手把进程内 cfg 也对齐到 writeCfg("twilight_session","lax",false)，
	// 否则 defaults() 里 CookieSecure=true 会让 previous→next 看不到差异。
	app.cfg().SessionCookie = "twilight_session"
	app.cfg().CookieSecure = false
	app.cfg().CookieSameSite = "lax"
	app.cfg().CookieDomain = ""
	writeCfg("twilight_session", "lax", false, "")
	app.configSignature = configFileSignature(app.cfg().ConfigFile)

	// 注册一个账户，使后续 login 能成功（reload 之后才测 cookie，避免 reload
	// 期间替换 store 把 user 抹掉的歧义）。
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"reloadops","password":"Password123456"}`, nil)

	// 改 cookie 三件套并触发 reload
	writeCfg("twilight_secure_session", "strict", true, ".example.com")
	info, err := app.reloadConfig()
	if err != nil {
		t.Fatalf("reload returned err: %v", err)
	}
	live, _ := info["live_applied"].([]string)
	expected := map[string]bool{"session_cookie_name": false, "session_cookie_secure": false, "session_cookie_samesite": false, "session_cookie_domain": false}
	for _, name := range live {
		if _, ok := expected[name]; ok {
			expected[name] = true
		}
	}
	for name, seen := range expected {
		if !seen {
			t.Fatalf("live_applied missing %q, got %v", name, live)
		}
	}

	// 走一次登录，断言 Set-Cookie 用的就是新名字 + Secure + SameSite=Strict
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"reloadops","password":"Password123456"}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login after reload status=%d body=%s", login.Code, login.Body.String())
	}
	cookies := login.Result().Cookies()
	sess := findCookie(cookies, "twilight_secure_session")
	if sess == nil {
		t.Fatalf("expected new session cookie name, got %#v", cookies)
	}
	if !sess.Secure {
		t.Fatalf("expected Secure=true after reload, got %#v", sess)
	}
	if sess.SameSite != http.SameSiteStrictMode {
		t.Fatalf("expected SameSite=Strict after reload, got %v", sess.SameSite)
	}
	if sess.Domain != "example.com" {
		t.Fatalf("expected Domain=example.com after reload, got %q", sess.Domain)
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
	if err := app.store().UpsertBindCode(store.BindCode{Code: "EXPIRED12345", Scene: "register", CreatedAt: now - 700, ExpiresAt: now - 1}); err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertBindCode(store.BindCode{Code: "VALID1234567", Scene: "register", CreatedAt: now, ExpiresAt: now + 600}); err != nil {
		t.Fatal(err)
	}
	deleted, err := app.store().CleanupExpiredBindCodes(now)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1", deleted)
	}
	if _, ok := app.store().BindCode("EXPIRED12345"); ok {
		t.Fatal("expired bind code was not deleted")
	}
	if _, ok := app.store().BindCode("VALID1234567"); !ok {
		t.Fatal("valid bind code was deleted")
	}
}

func TestSchedulerCleanupBindCodesJob(t *testing.T) {
	app := newTestApp(t)
	now := time.Now().Unix()
	if err := app.store().UpsertBindCode(store.BindCode{Code: "OLD123456789", Scene: "register", CreatedAt: now - 800, ExpiresAt: now - 1}); err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertRegCode(store.RegCode{Code: "OLD-REGCODE", Type: 1, Days: 30, ValidityTime: 1, UseCountLimit: 1, Active: true, CreatedAt: now - 7200}); err != nil {
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
	if _, ok := app.store().BindCode("OLD123456789"); ok {
		t.Fatal("scheduler did not delete expired bind code")
	}
	if _, ok := app.store().RegCode("OLD-REGCODE"); !ok {
		t.Fatal("bind-code cleanup deleted a regcode")
	}
}

func TestSchedulerCleanupNoEmbySkipsPendingEntitlements(t *testing.T) {
	app := newTestApp(t)
	app.cfg().AutoCleanupNoEmby = true
	app.cfg().AutoCleanupNoEmbyDays = 1
	app.cfg().AdminUsernames = []string{"configured-no-emby"}
	old := time.Now().AddDate(0, 0, -3).Unix()
	plain, err := app.store().CreateUser(store.User{Username: "plain-no-emby", Role: store.RoleNormal, Active: true, RegisterTime: old, CreatedAt: old})
	if err != nil {
		t.Fatal(err)
	}
	days := 30
	pending, err := app.store().CreateUser(store.User{Username: "pending-emby", Role: store.RoleNormal, Active: true, PendingEmby: true, PendingEmbyDays: &days, RegisterTime: old, CreatedAt: old})
	if err != nil {
		t.Fatal(err)
	}
	configuredAdmin, err := app.store().CreateUser(store.User{Username: "configured-no-emby", Role: store.RoleNormal, Active: true, RegisterTime: old, CreatedAt: old})
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
	if _, ok := app.store().User(plain.UID); ok {
		t.Fatal("plain no-Emby web user was not deleted")
	}
	if u, ok := app.store().User(pending.UID); !ok || !u.PendingEmby {
		t.Fatalf("pending Emby entitlement user was not preserved: ok=%v user=%#v", ok, u)
	}
	if _, ok := app.store().User(configuredAdmin.UID); !ok {
		t.Fatal("configured admin no-Emby user was deleted")
	}
}

func TestSchedulerCleanupPendingEmbyEntitlementsKeepsWebAccount(t *testing.T) {
	app := newTestApp(t)
	app.cfg().AutoCleanupPendingEmby = true
	app.cfg().AutoCleanupPendingEmbyDays = 1
	old := time.Now().AddDate(0, 0, -3).Unix()
	recent := time.Now().Unix()
	days := 30
	user, err := app.store().CreateUser(store.User{Username: "pending-clear", Role: store.RoleNormal, Active: true, PendingEmby: true, PendingEmbyDays: &days, RegisterTime: old, CreatedAt: old})
	if err != nil {
		t.Fatal(err)
	}
	recentUser, err := app.store().CreateUser(store.User{Username: "pending-recent-clear", Role: store.RoleNormal, Active: true, PendingEmby: true, PendingEmbyDays: &days, RegisterTime: recent, CreatedAt: recent})
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
	updated, ok := app.store().User(user.UID)
	if !ok {
		t.Fatal("web account was deleted while clearing pending Emby entitlement")
	}
	if updated.PendingEmby || updated.PendingEmbyDays != nil || !updated.Active {
		t.Fatalf("pending entitlement was not cleared cleanly: %#v", updated)
	}
	updatedRecent, ok := app.store().User(recentUser.UID)
	if !ok || updatedRecent.PendingEmby || updatedRecent.PendingEmbyDays != nil || !updatedRecent.Active {
		t.Fatalf("recent pending entitlement was not cleared cleanly: ok=%v user=%#v", ok, updatedRecent)
	}
}

func TestSchedulerEmbySyncRepairsPlaceholderAndMissingIDs(t *testing.T) {
	app := newTestApp(t)
	app.cfg().EmbyToken = "token"
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
	app.cfg().EmbyURL = emby.URL

	alpha, err := app.store().CreateUser(store.User{Username: "alpha", Role: store.RoleNormal, Active: true, PendingEmby: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.store().UpdateUser(alpha.UID, func(u *store.User) error {
		u.EmbyID = fmt.Sprintf("Emby_%d", alpha.UID)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := app.store().CreateUser(store.User{Username: "local-beta", EmbyUsername: "beta", Role: store.RoleNormal, Active: true})
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
	updatedAlpha, _ := app.store().User(alpha.UID)
	if updatedAlpha.EmbyID != "real-alpha" || updatedAlpha.EmbyUsername != "alpha" || updatedAlpha.PendingEmby {
		t.Fatalf("placeholder Emby ID was not repaired: %#v", updatedAlpha)
	}
	updatedBeta, _ := app.store().User(beta.UID)
	if updatedBeta.EmbyID != "real-beta" || updatedBeta.EmbyUsername != "beta" {
		t.Fatalf("missing Emby ID was not filled by username: %#v", updatedBeta)
	}
}

// TestSchedulerEmbySyncRetriesTransient5xx 验证 R61-5 的核心承诺：emby 反代偶发
// 502 不应让 emby_sync 把用户记成 failed_sync。第一次拉 /Users 返回 502，第二次
// 才返回正常 JSON——重试 helper 必须把第一次失败吃掉，让 sync 像无错一样完成。
//
// 同时校验 4xx 不会被重试：Policy POST 一旦返回 400，调用立刻 fail，重试只会
// 复读相同错误并刷日志。
func TestSchedulerEmbySyncRetriesTransient5xx(t *testing.T) {
	app := newTestApp(t)
	app.cfg().EmbyToken = "token"
	remoteUsers := []map[string]any{
		{"Id": "real-alpha", "Name": "alpha", "Policy": map[string]any{}},
	}
	usersAttempts := 0
	policyAttempts := 0
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users":
			usersAttempts++
			if usersAttempts == 1 {
				// 模拟反代瞬时 502（典型 nginx upstream timeout）。
				http.Error(w, "bad gateway", http.StatusBadGateway)
				return
			}
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
			policyAttempts++
			_, _ = io.Copy(io.Discard, r.Body)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected Emby request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	alpha, err := app.store().CreateUser(store.User{Username: "alpha", EmbyUsername: "alpha", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().UpdateUser(alpha.UID, func(u *store.User) error {
		u.EmbyID = "real-alpha"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/scheduler", nil)
	summary, _, err := app.runSchedulerJob(req, "emby_sync")
	if err != nil {
		t.Fatalf("emby_sync should have recovered from a transient 502, got err: %v", err)
	}
	if !boolish(summary["success"]) {
		t.Fatalf("emby_sync did not succeed after retry: %#v", summary)
	}
	if usersAttempts < 2 {
		t.Fatalf("expected /Users to be retried at least once on 502, got attempts=%d", usersAttempts)
	}
	// 用户列表只有 1 条，policy 同步应当走通——证明 retry 包装没有让幂等
	// 写路径"重试地狱"地反复 POST。
	if int(numeric(summary["synced_state"])) != 1 {
		t.Fatalf("expected 1 user state synced after retry, summary=%#v", summary)
	}
	if policyAttempts != 1 {
		t.Fatalf("expected exactly 1 policy POST after recovery, got %d", policyAttempts)
	}
}

func TestSchedulerEmbySyncSkipsPolicyUpdateWhenRemoteStateMatches(t *testing.T) {
	app := newTestApp(t)
	app.cfg().EmbyToken = "token"
	remoteUsers := []map[string]any{
		{"Id": "real-alpha", "Name": "alpha", "Policy": map[string]any{"IsDisabled": false}},
	}
	policyRequests := 0
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users":
			_ = json.NewEncoder(w).Encode(remoteUsers)
		case strings.HasPrefix(r.URL.Path, "/Users/"):
			policyRequests++
			http.Error(w, "policy endpoint should not be called when state is already known", http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected Emby request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	if _, err := app.store().CreateUser(store.User{Username: "alpha", EmbyUsername: "alpha", EmbyID: "real-alpha", Role: store.RoleNormal, Active: true}); err != nil {
		t.Fatal(err)
	}

	summary, _, err := app.runSchedulerJob(httptest.NewRequest(http.MethodPost, "/scheduler", nil), "emby_sync")
	if err != nil {
		t.Fatal(err)
	}
	if int(numeric(summary["synced_state"])) != 1 || int(numeric(summary["state_unchanged"])) != 1 {
		t.Fatalf("expected already-synced remote state to be counted without policy writes: %#v", summary)
	}
	if policyRequests != 0 {
		t.Fatalf("policy endpoint was called despite matching remote state: %d", policyRequests)
	}
}

// TestEmbyRetryOn5xxSkips4xx 直接覆盖 retry helper 的"4xx 不重试"分支，
// 避免被错误的"什么错都重试"实现悄悄回归。
func TestEmbyRetryOn5xxSkips4xx(t *testing.T) {
	attempts := 0
	err := embyRetryOn5xx(context.Background(), func(ctx context.Context) error {
		attempts++
		return fmt.Errorf("remote status 404: not found")
	})
	if err == nil {
		t.Fatalf("expected 4xx error to bubble up unchanged")
	}
	if attempts != 1 {
		t.Fatalf("4xx must not be retried, got attempts=%d", attempts)
	}
}

func TestTelegramMembershipRejoinManualReviewAndAutoEnable(t *testing.T) {
	app := newTestApp(t)
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	app.cfg().TelegramRequireMembership = true
	app.cfg().TelegramGroupIDs = []string{"-1001"}
	user, err := app.store().CreateUser(store.User{Username: "rejoined", Role: store.RoleNormal, Active: false, TelegramID: 4242, EmbyID: "emby-rejoined"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().UpdateUser(user.UID, func(u *store.User) error { u.Active = false; return nil }); err != nil {
		t.Fatal(err)
	}
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123:ABC/getChatMember" {
			t.Fatalf("unexpected telegram path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"status":"member","user":{"id":4242,"is_bot":false}}}`))
	}))
	defer tg.Close()
	app.cfg().TelegramAPIURL = tg.URL

	app.cfg().TelegramAutoEnableRejoined = false
	summary, _, err := app.enforceTelegramMembership(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if int(numeric(summary["rejoined_pending_review"])) != 1 || int(numeric(summary["rejoin_candidates"])) != 1 {
		t.Fatalf("manual review rejoin was not reported: %#v", summary)
	}
	if updated, _ := app.store().User(user.UID); updated.Active {
		t.Fatal("manual review mode should not auto-enable the web account")
	}

	app.cfg().TelegramAutoEnableRejoined = true
	summary, _, err = app.enforceTelegramMembership(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if int(numeric(summary["rejoined_enabled"])) != 1 {
		t.Fatalf("auto rejoin did not enable user: %#v", summary)
	}
	if updated, _ := app.store().User(user.UID); !updated.Active {
		t.Fatal("auto rejoin did not enable the web account")
	}
}

func TestRegisterConsumesConfirmedTelegramBindCode(t *testing.T) {
	app := newTestApp(t)
	app.cfg().BotInternalSecret = "test-secret"
	app.cfg().ForceBindTelegram = true

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
	registered := doJSON(app, http.MethodPost, "/api/v1/users/register", fmt.Sprintf(`{"username":"alice","password":"Alice123456","telegram_bind_code":%q}`, code), nil)
	if registered.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", registered.Code, registered.Body.String())
	}
	u, ok := app.store().FindUserByUsername("alice")
	if !ok || u.TelegramID != 424242 || u.TelegramUsername != "alice_tg" {
		t.Fatalf("telegram bind not applied to registered user: ok=%v user=%#v", ok, u)
	}
	if _, ok := app.store().BindCode(code); ok {
		t.Fatal("confirmed register bind code was not consumed")
	}
}

func TestRegisterWithTelegramBindCodeAllowsBootstrapAdminUIDOnlyConfig(t *testing.T) {
	app := newTestApp(t)
	app.cfg().BotInternalSecret = "test-secret"
	app.cfg().ForceBindTelegram = true
	app.cfg().AdminUIDs = []int64{1}
	app.cfg().AdminUsernames = nil

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
	confirmPayload := fmt.Sprintf(`{"code":%q,"telegram_id":777,"telegram_username":"admin_tg"}`, code)
	confirmed := doJSONWithHeaders(app, http.MethodPost, "/api/v1/users/me/telegram/bind-confirm", confirmPayload, nil, map[string]string{"X-Internal-Secret": "test-secret"})
	if confirmed.Code != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", confirmed.Code, confirmed.Body.String())
	}

	registered := doJSON(app, http.MethodPost, "/api/v1/users/register", fmt.Sprintf(`{"username":"bootstrap","password":"Admin123456","telegram_bind_code":%q}`, code), nil)
	if registered.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", registered.Code, registered.Body.String())
	}
	u, ok := app.store().FindUserByUsername("bootstrap")
	if !ok || u.UID != 1 || u.Role != store.RoleAdmin || u.TelegramID != 777 {
		t.Fatalf("bootstrap admin uid-only config not applied: ok=%v user=%#v", ok, u)
	}
}

func TestRegisterRejectsUserSceneTelegramBindCode(t *testing.T) {
	app := newTestApp(t)
	app.cfg().ForceBindTelegram = true
	now := time.Now().Unix()
	if err := app.store().UpsertBindCode(store.BindCode{Code: "USERBIND1234", Scene: "user", UID: 99, Confirmed: true, TelegramID: 999, CreatedAt: now, ExpiresAt: now + 600}); err != nil {
		t.Fatal(err)
	}
	resp := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"bob","password":"Bob123456","telegram_bind_code":"USERBIND1234"}`, nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("register with user-scene bind code status=%d body=%s", resp.Code, resp.Body.String())
	}
	if _, ok := app.store().FindUserByUsername("bob"); ok {
		t.Fatal("user was registered with a user-scene bind code")
	}
}

func TestRegisterRejectsMalformedTelegramBindCodeBeforeLookup(t *testing.T) {
	app := newTestApp(t)
	app.cfg().ForceBindTelegram = true
	resp := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"badbind","password":"badbind123A","telegram_bind_code":"../../bad"}`, nil)
	if resp.Code != http.StatusBadRequest || !strings.Contains(resp.Body.String(), `"error_code":"TG_BIND_CODE_FORMAT_INVALID"`) {
		t.Fatalf("malformed bind code status=%d body=%s", resp.Code, resp.Body.String())
	}

	rr := doJSON(app, http.MethodGet, "/api/v1/users/telegram/register/bind-code/status?code=../../bad", ``, nil)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"invalid":true`) || !strings.Contains(rr.Body.String(), `"terminal":true`) {
		t.Fatalf("malformed bind code status endpoint response=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestTelegramBindConfirmRequiresInternalSecret(t *testing.T) {
	app := newTestApp(t)
	app.cfg().BotInternalSecret = "test-secret"
	now := time.Now().Unix()
	if err := app.store().UpsertBindCode(store.BindCode{Code: "ABCDEFGH", Scene: "register", CreatedAt: now, ExpiresAt: now + 60}); err != nil {
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

func TestChangeEmbyPasswordAcceptsEmptyEmbyResponses(t *testing.T) {
	app := newTestApp(t)
	app.cfg().EmbyToken = "test-token"

	_ = registerAndLogin(t, app, "admin", "Admin123456")
	userCookies := registerAndLogin(t, app, "embyuser", "User123456")
	user, ok := app.store().FindUserByUsername("embyuser")
	if !ok {
		t.Fatal("created user not found")
	}
	if _, err := app.store().UpdateUser(user.UID, func(u *store.User) error {
		u.EmbyID = "emby-user-id"
		u.EmbyUsername = "embyuser"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	passwordCalls := 0
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users/emby-user-id":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Id":"emby-user-id","Name":"embyuser","Policy":{"IsAdministrator":false}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/Users/emby-user-id/Password":
			passwordCalls++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			switch passwordCalls {
			case 1:
				if body["ResetPassword"] != true {
					t.Fatalf("first password call body=%#v, want ResetPassword=true", body)
				}
			case 2:
				if body["CurrentPw"] != "" || body["NewPw"] != "NewPass123" {
					t.Fatalf("second password call body=%#v", body)
				}
			default:
				t.Fatalf("unexpected extra password call body=%#v", body)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected Emby request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	resp := doJSONWithHeaders(app, http.MethodPost, "/api/v1/users/me/password/emby", `{"new_password":"NewPass123"}`, userCookies, map[string]string{"X-Twilight-Client": "webui"})
	if resp.Code != http.StatusOK {
		t.Fatalf("change Emby password status=%d body=%s", resp.Code, resp.Body.String())
	}
	if passwordCalls != 2 {
		t.Fatalf("passwordCalls=%d, want 2", passwordCalls)
	}
}

func TestTelegramEndpointAcceptsCommonBaseURLs(t *testing.T) {
	app := newTestApp(t)
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	cases := map[string]string{
		"https://api.telegram.org":            "https://api.telegram.org/bot123:ABC/getMe",
		"https://api.telegram.org/bot":        "https://api.telegram.org/bot123:ABC/getMe",
		"https://api.telegram.org/bot123:ABC": "https://api.telegram.org/bot123:ABC/getMe",
	}
	for base, want := range cases {
		app.cfg().TelegramAPIURL = base
		got, err := app.telegramEndpoint("getMe")
		if err != nil {
			t.Fatalf("telegramEndpoint(%q) err=%v", base, err)
		}
		if got != want {
			t.Fatalf("telegramEndpoint(%q) = %q, want %q", base, got, want)
		}
	}
}

// TestTelegramEndpointRejectsUnsafeBaseURLs 验证 telegramEndpoint 现在会
// 否决 link-local / 元数据 IP / 非 http(s) scheme 的 base URL，避免 bot
// token 在拼出 /bot<TOKEN>/method 之前就被发到攻击者控制的目标。
func TestTelegramEndpointRejectsUnsafeBaseURLs(t *testing.T) {
	app := newTestApp(t)
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	cases := []string{
		"http://169.254.169.254",
		"http://100.100.100.200",
		"http://0.0.0.0",
		"javascript:alert(1)",
	}
	for _, base := range cases {
		app.cfg().TelegramAPIURL = base
		if _, err := app.telegramEndpoint("getMe"); err == nil {
			t.Fatalf("telegramEndpoint(%q) should reject but returned nil error", base)
		}
	}
}

func TestTelegramErrorRedactsBotToken(t *testing.T) {
	app := newTestApp(t)
	app.cfg().TelegramBotToken = "123:SECRET"
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

// TestTelegramRetryAfterFromError 锁定 R61-3 不变量：telegramPost 在 OK=false
// + parameters.retry_after>0 时必须把秒数编进错误字符串，
// telegramRetryAfterFromError 反解出来后调用方才能 sleep 真实秒数。
//
// 用 httptest server 模拟 429+retry_after=27 的真实响应。
func TestTelegramRetryAfterFromError(t *testing.T) {
	app := newTestApp(t)
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 27","parameters":{"retry_after":27}}`))
	}))
	defer tg.Close()
	app.cfg().TelegramAPIURL = tg.URL
	err := app.telegramSendMessage(context.Background(), int64(123456), "test")
	if err == nil {
		t.Fatalf("expected 429 error from mock server")
	}
	d, ok := telegramRetryAfterFromError(err)
	if !ok {
		t.Fatalf("retry_after sentinel not found in error: %s", err.Error())
	}
	if d != 27*time.Second {
		t.Fatalf("retry_after parsed = %v, want 27s; err=%s", d, err.Error())
	}

	// 反向：error 没有 sentinel 时返回 false。
	if d, ok := telegramRetryAfterFromError(errors.New("some unrelated error")); ok {
		t.Fatalf("retry_after should be false for unrelated err, got d=%v", d)
	}
}

// TestSendExpiryRemindersAbortsOnConsecutiveRateLimits 锁定 R61-4 不变量：
// 提醒批触发连续 5 次 429 后必须 break、把已发送条数 commit 到 result，并在
// summary 中暴露 aborted=rate_limited——避免 100 用户提醒一次性把全局 quota
// 打爆，下一轮 expiry_reminders 重炸的"提醒雪崩"。
func TestSendExpiryRemindersAbortsOnConsecutiveRateLimits(t *testing.T) {
	app := newTestApp(t)
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	app.cfg().NotificationEnabled = true

	// mock telegram 永远返回 429+retry_after=1（保证测试快）
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":1}}`))
	}))
	defer tg.Close()
	app.cfg().TelegramAPIURL = tg.URL

	// 造 8 个即将到期且绑了 telegram 的用户，确保超过 maxConsecutiveRateLimited=5
	expSoon := time.Now().Add(24 * time.Hour).Unix()
	for i := 0; i < 8; i++ {
		_, err := app.store().CreateUser(store.User{
			Username:   fmt.Sprintf("soon%d", i),
			Role:       store.RoleNormal,
			Active:     true,
			ExpiredAt:  expSoon,
			TelegramID: int64(1000 + i),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// 真实 sleep 60s × 5 太慢——这里我们 ctx 超时 6s 让 retry_after sleep
	// 提前返回；目的是验证 abort 路径，不是验证准确 sleep。
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	result := app.sendExpiryReminders(ctx, 7)

	if result["aborted"] != "rate_limited" && result["aborted"] != "context_canceled" {
		t.Fatalf("expected aborted=rate_limited or context_canceled, got result=%#v", result)
	}
	// 至少几条 failed 被记录（无论是 5 个 rate-limit 还是更早被 ctx 打断）。
	failed, _ := result["failed"].([]map[string]any)
	if len(failed) == 0 {
		t.Fatalf("expected at least one failed item recorded; result=%#v", result)
	}
}

func TestTelegramGetUpdatesAllowsCallbacks(t *testing.T) {
	app := newTestApp(t)
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
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
	app.cfg().TelegramAPIURL = tg.URL
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
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	app.cfg().TelegramGroupIDs = []string{"-1001"}
	app.cfg().TelegramChannelIDs = []string{"@RequiredChannel"}
	app.cfg().TelegramForceBindGroup = true
	app.cfg().TelegramForceBindChannel = false
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
	app.cfg().TelegramAPIURL = tg.URL

	now := time.Now().Unix()
	if err := app.store().UpsertBindCode(store.BindCode{Code: "GROUP1", Scene: "register", CreatedAt: now, ExpiresAt: now + 600}); err != nil {
		t.Fatal(err)
	}
	app.telegramConfirmBindCode(context.Background(), 42, 42, "tguser", "GROUP1")
	bind, ok := app.store().BindCode("GROUP1")
	if !ok || !bind.Confirmed {
		t.Fatalf("group-only requirement should confirm bind code: %#v", bind)
	}
	for _, req := range requests {
		if req["_path"] == "/bot123:ABC/getChatMember" && asString(req["chat_id"]) == "@RequiredChannel" {
			t.Fatalf("channel was checked even though force_bind_channel=false: %#v", requests)
		}
	}

	app.cfg().TelegramForceBindChannel = true
	if err := app.store().UpsertBindCode(store.BindCode{Code: "CHAN01", Scene: "register", CreatedAt: now, ExpiresAt: now + 600}); err != nil {
		t.Fatal(err)
	}
	app.telegramConfirmBindCode(context.Background(), 42, 43, "tguser2", "CHAN01")
	channelBind, ok := app.store().BindCode("CHAN01")
	if !ok || channelBind.Confirmed {
		t.Fatalf("channel requirement should reject user missing required channel: %#v", channelBind)
	}
}

// TestTelegramTouchPanelReusesSingleTimer 锁定 R52-2：多次 touch 同一个
// panel 不应累积 *time.Timer 实例。历史实现每次 touch 调 AfterFunc 新建
// 一个 closure，旧定时器从不取消，admin 高频按钮场景下单 panel 能挂出
// 几十个未释放的 closure；本测试通过对比 timer 指针来锁定"复用同一把"。
func TestTelegramTouchPanelReusesSingleTimer(t *testing.T) {
	app := newTestApp(t)
	panel := telegramPanelContext{
		Token:     "tok-test",
		ChatID:    1001,
		MessageID: 2002,
		TargetUID: 3003,
		ExpiresAt: time.Now().Add(telegramPanelTTL).Unix(),
	}
	app.telegramSavePanel(panel)
	defer app.telegramDeletePanel(panel.Token)

	app.telegramPanelMu.Lock()
	saved, ok := app.telegramPanels[panel.Token]
	app.telegramPanelMu.Unlock()
	if !ok {
		t.Fatalf("panel not stored after save")
	}
	if saved.timer == nil {
		t.Fatalf("expected save to set up a timer")
	}
	firstTimer := saved.timer

	for i := 0; i < 25; i++ {
		_ = app.telegramTouchPanel(saved)
		app.telegramPanelMu.Lock()
		saved = app.telegramPanels[panel.Token]
		app.telegramPanelMu.Unlock()
		if saved.timer != firstTimer {
			t.Fatalf("iteration %d: expected timer pointer to be reused, got fresh AfterFunc closure", i)
		}
	}
}

func TestTelegramAnonymousGroupUserRequiresInlineAuth(t *testing.T) {
	app := newTestApp(t)
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	user := store.User{UID: 1001, Username: "target", Role: store.RoleNormal, Active: true, TelegramID: 888, CreatedAt: time.Now().Unix(), RegisterTime: time.Now().Unix()}
	if _, err := app.store().CreateUser(user); err != nil {
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
	app.cfg().TelegramAPIURL = tg.URL

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

func TestTelegramGroupUserPanelShowsEmbyInfoAndActions(t *testing.T) {
	app := newTestApp(t)
	app.cfg().EmbyToken = "emby-token"
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/Users/emby-user" {
			t.Fatalf("unexpected Emby request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Id":"emby-user","Name":"remote-alpha","Policy":{"IsDisabled":true,"IsHidden":false,"IsAdministrator":false},"DateLastActivity":"2026-01-02T03:04:05Z"}`))
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	user := store.User{UID: 42, Username: "alpha", Role: store.RoleNormal, Active: true, TelegramID: 1001, TelegramUsername: "alpha_tg", EmbyID: "emby-user", EmbyUsername: "alpha-emby", RegisterTime: time.Now().Unix(), LibrarySelfService: true}
	text := app.telegramGroupUserPanelText(context.Background(), user)
	for _, want := range []string{"== Web 账号 ==", "== Emby 远端 ==", "远端用户名: remote-alpha", "远端状态: 禁用", "媒体库自助: 开启"} {
		if !strings.Contains(text, want) {
			t.Fatalf("panel text missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "emby-user") || strings.Contains(text, "emby-token") {
		t.Fatalf("panel leaked sensitive Emby identifier/token:\n%s", text)
	}

	labels := telegramInlineButtonLabels(app.telegramGroupUserPanelMarkup("tok", user, ""))
	for _, want := range []string{"禁用 Emby", "启用 Emby", "删除 Emby", "删除用户"} {
		if !stringSliceContains(labels, want) {
			t.Fatalf("panel buttons missing %q: %#v", want, labels)
		}
	}
}

func TestTelegramPanelEmbyDeleteRequiresConfirmAndClearsLocalBinding(t *testing.T) {
	app := newTestApp(t)
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	tgRequests := []map[string]any{}
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		body["_path"] = r.URL.Path
		tgRequests = append(tgRequests, body)
		if r.URL.Path != "/bot123:ABC/editMessageText" {
			t.Fatalf("unexpected Telegram request: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer tg.Close()
	app.cfg().TelegramAPIURL = tg.URL

	app.cfg().EmbyToken = "emby-token"
	deleteCount := 0
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users/emby-user":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Id":"emby-user","Name":"remote-alpha","Policy":{}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/Users/emby-user":
			deleteCount++
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected Emby request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	user, err := app.store().CreateUser(store.User{Username: "target", Role: store.RoleNormal, Active: true, EmbyID: "emby-user", EmbyUsername: "remote-alpha"})
	if err != nil {
		t.Fatal(err)
	}
	panel := telegramPanelContext{Token: "tok-delete-emby", ChatID: -1001, MessageID: 9001, TargetUID: user.UID, ExpiresAt: time.Now().Add(telegramPanelTTL).Unix()}
	app.telegramSavePanel(panel)
	defer app.telegramDeletePanel(panel.Token)

	app.telegramApplyPanelAction(context.Background(), panel, "emby_delete_confirm")
	if deleteCount != 0 {
		t.Fatalf("Emby was deleted without confirmation")
	}
	current, _ := app.store().User(user.UID)
	if current.EmbyID == "" {
		t.Fatalf("local Emby binding was cleared without confirmation")
	}

	panel.ConfirmAction = "emby_delete"
	panel = app.telegramTouchPanel(panel)
	app.telegramApplyPanelAction(context.Background(), panel, "emby_delete_confirm")
	if deleteCount != 1 {
		t.Fatalf("expected one Emby delete request, got %d", deleteCount)
	}
	current, _ = app.store().User(user.UID)
	if current.EmbyID != "" || current.EmbyUsername != "" || current.PendingEmby || current.PendingEmbyDays != nil {
		t.Fatalf("local Emby binding was not cleared: %#v", current)
	}
	if len(tgRequests) == 0 || !strings.Contains(asString(tgRequests[len(tgRequests)-1]["text"]), "Emby 账号已删除") {
		t.Fatalf("final Telegram edit did not report deletion: %#v", tgRequests)
	}
}

func TestTelegramProtectedTargetUsesUnifiedProtectedUsers(t *testing.T) {
	app := newTestApp(t)
	app.cfg().AdminUsernames = []string{"configured-admin"}
	for _, user := range []store.User{
		{Username: "admin", Role: store.RoleAdmin},
		{Username: "white", Role: store.RoleWhitelist},
		{Username: "configured-admin", Role: store.RoleNormal},
	} {
		if !app.telegramProtectedTarget(user) {
			t.Fatalf("expected protected Telegram target: %#v", user)
		}
		labels := telegramInlineButtonLabels(app.telegramGroupUserPanelMarkup("tok", user, ""))
		if len(labels) != 1 || labels[0] != "刷新" {
			t.Fatalf("protected target should only expose refresh button, got %#v", labels)
		}
	}
}

func telegramInlineButtonLabels(markup any) []string {
	m, _ := markup.(map[string]any)
	rows, _ := m["inline_keyboard"].([][]map[string]string)
	labels := []string{}
	for _, row := range rows {
		for _, button := range row {
			labels = append(labels, button["text"])
		}
	}
	return labels
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
	runs := app.store().SchedulerRuns("daily_stats", 10)
	if len(runs) != 1 {
		t.Fatalf("manual run should update one history entry, got %d: %#v", len(runs), runs)
	}
	if runs[0].ID != run.ID || runs[0].Status != "success" || runs[0].Type != "manual" || runs[0].FinishedAt == 0 {
		t.Fatalf("unexpected manual run history: %#v", runs[0])
	}
}

// TestSchedulerAutoRunRecordVisibleBeforeReturn 复现 R56-1：之前 daemon 用
// `go a.runScheduledJob` 异步起动，AddSchedulerRunReturning 还没落库 daemon
// 主循环就推进到下一轮 tick，schedulerJobDue 看不到新 auto 记录就再判 due
// 一次。改造后 runScheduledJob 在拿到锁 + INSERT 完成之前 *不* 返回，重活
// 仍走内层 goroutine。这条测试在 runScheduledJob 返回的瞬间断言 PG 已经有
// 这条 running 记录——即拿到了"daemon 视角下的 last 已经更新"的可观测保证。
func TestSchedulerAutoRunRecordVisibleBeforeReturn(t *testing.T) {
	app := newTestApp(t)
	before := app.store().SchedulerRuns("daily_stats", 10)
	if len(before) != 0 {
		t.Fatalf("precondition: expected no daily_stats runs, got %d", len(before))
	}
	app.runScheduledJob(context.Background(), "daily_stats")
	// 同步段返回时记录必须已经在 store 里——不论内层 goroutine 是否已经 finish。
	immediate := app.store().SchedulerRuns("daily_stats", 10)
	if len(immediate) != 1 {
		t.Fatalf("auto run record not persisted before runScheduledJob returned: %#v", immediate)
	}
	if immediate[0].Type != "auto" || immediate[0].Trigger != "scheduler" {
		t.Fatalf("unexpected auto run row: %#v", immediate[0])
	}
	deadline := time.Now().Add(2 * time.Second)
	for app.schedulerJobRunning("daily_stats") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if app.schedulerJobRunning("daily_stats") {
		t.Fatal("auto scheduler job did not finish in time")
	}
	final := app.store().SchedulerRuns("daily_stats", 10)
	if len(final) != 1 || final[0].ID != immediate[0].ID || final[0].Status != "success" || final[0].FinishedAt == 0 {
		t.Fatalf("auto run did not converge to success: %#v", final)
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
	run, err := app.store().AddSchedulerRunReturning(store.SchedulerRun{JobID: "enforce_group_membership", Type: "auto", Trigger: "scheduler", Status: "running", Message: "running", StartedAt: time.Now().Add(-time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/jobs", nil)
	rr := httptest.NewRecorder()
	app.handleSchedulerJobs(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", rr.Code, rr.Body.String())
	}
	runs := app.store().SchedulerRuns("enforce_group_membership", 1)
	if len(runs) != 1 || runs[0].ID != run.ID || runs[0].Status == "running" || runs[0].FinishedAt == 0 {
		t.Fatalf("stale running history was not reconciled: %#v", runs)
	}
	if !boolish(runs[0].Summary["interrupted"]) {
		t.Fatalf("reconciled run should be marked interrupted: %#v", runs[0])
	}
}

func TestSchedulerJobsPreservesFreshExternalRunningHistory(t *testing.T) {
	app := newTestApp(t)
	run, err := app.store().AddSchedulerRunReturning(store.SchedulerRun{JobID: "enforce_group_membership", Type: "auto", Trigger: "scheduler", Status: "running", Message: "running", StartedAt: time.Now().Unix()})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/jobs", nil)
	rr := httptest.NewRecorder()
	app.handleSchedulerJobs(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", rr.Code, rr.Body.String())
	}
	runs := app.store().SchedulerRuns("enforce_group_membership", 1)
	if len(runs) != 1 || runs[0].ID != run.ID || runs[0].Status != "running" || runs[0].FinishedAt != 0 {
		t.Fatalf("fresh external running history should stay running: %#v", runs)
	}
	var body struct {
		Data struct {
			Jobs []map[string]any `json:"jobs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, job := range body.Data.Jobs {
		if asString(job["id"]) == "enforce_group_membership" {
			found = true
			if !boolish(job["is_running"]) {
				t.Fatalf("fresh external running job should be reported running: %#v", job)
			}
		}
	}
	if !found {
		t.Fatal("enforce_group_membership job not returned")
	}
}

func TestSchedulerFinishedRunRedactsPersistentFields(t *testing.T) {
	finished := schedulerFinishedRun(
		"system_auto_update",
		"manual",
		"manual",
		1,
		map[string]any{
			"message": "failed with token=abc123456789 and password=SuperSecret123",
			"nested":  []any{map[string]any{"stderr": "Authorization: Bearer abcdefghijklmnopqrstuvwxyz"}},
		},
		[]string{"retry failed: api_key=key-abcdefghijklmnopqrstuvwxyz"},
		fmt.Errorf("remote error: token=abc123456789 password=SuperSecret123"),
	)
	data, err := json.Marshal(finished)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, leaked := range []string{"abc123456789", "SuperSecret123", "abcdefghijklmnopqrstuvwxyz"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("scheduler run leaked sensitive value %q: %s", leaked, out)
		}
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("scheduler run did not include redacted markers: %s", out)
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
	app.cfg().AutoCleanupPendingEmby = true
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
	if _, err := app.store().CreateUser(store.User{Username: "pending-db-disabled", Role: store.RoleNormal, Active: true, PendingEmby: true, PendingEmbyDays: &days}); err != nil {
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
	app.cfg().BangumiEnabled = true
	blocked := doJSON(app, http.MethodPost, "/api/v1/emby/bangumi/webhook", `{"Event":"PlaybackStopped"}`, nil)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("webhook without configured secret = %d body=%s", blocked.Code, blocked.Body.String())
	}
	app.cfg().BangumiWebhookSecret = "webhook-secret"
	allowed := doJSON(app, http.MethodPost, "/api/v1/emby/bangumi/webhook?token=webhook-secret", `{"Event":"PlaybackStopped"}`, nil)
	if allowed.Code != http.StatusOK {
		t.Fatalf("webhook with secret = %d body=%s", allowed.Code, allowed.Body.String())
	}
}

// TestBangumiWebhookRejectsStaleTimestamp 锁定 R58-1 replay window:带
// X-Twilight-Bangumi-Timestamp 但落在 ±300s 之外的请求必须 410 拒绝,即使
// secret 正确。窗口内 / 不带 header 仍应放行(向后兼容路径)。
func TestBangumiWebhookRejectsStaleTimestamp(t *testing.T) {
	app := newTestApp(t)
	app.cfg().BangumiEnabled = true
	app.cfg().BangumiWebhookSecret = "webhook-secret"

	// 落在 1 小时之前——窗口 5 分钟,必拒。
	stale := strconv.FormatInt(time.Now().Unix()-3600, 10)
	rec := doJSONWithHeaders(app, http.MethodPost, "/api/v1/emby/bangumi/webhook", `{"Event":"PlaybackStopped"}`, nil, map[string]string{
		"X-Twilight-Bangumi-Token":     "webhook-secret",
		"X-Twilight-Bangumi-Timestamp": stale,
	})
	if rec.Code != http.StatusGone {
		t.Fatalf("stale timestamp should be rejected with 410, got %d body=%s", rec.Code, rec.Body.String())
	}

	// 当下时间戳——必放行。
	fresh := strconv.FormatInt(time.Now().Unix(), 10)
	freshRec := doJSONWithHeaders(app, http.MethodPost, "/api/v1/emby/bangumi/webhook", `{"Event":"PlaybackStopped"}`, nil, map[string]string{
		"X-Twilight-Bangumi-Token":     "webhook-secret",
		"X-Twilight-Bangumi-Timestamp": fresh,
	})
	if freshRec.Code != http.StatusOK {
		t.Fatalf("fresh timestamp should be accepted, got %d body=%s", freshRec.Code, freshRec.Body.String())
	}

	// 非数字时间戳——必拒。
	badRec := doJSONWithHeaders(app, http.MethodPost, "/api/v1/emby/bangumi/webhook", `{"Event":"PlaybackStopped"}`, nil, map[string]string{
		"X-Twilight-Bangumi-Token":     "webhook-secret",
		"X-Twilight-Bangumi-Timestamp": "not-a-number",
	})
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("non-numeric timestamp should be 400, got %d body=%s", badRec.Code, badRec.Body.String())
	}
}

// TestBangumiWebhookIdempotentReplay 锁定 R58-1 idempotency:同一份合法
// 请求被重放也只产生一条 PlaybackRecord,而不是无限堆积。即使时间戳
// header 通过校验,store 层的 (uid, item_id, played_at) 三元组兜底。
func TestBangumiWebhookIdempotentReplay(t *testing.T) {
	app := newTestApp(t)
	app.cfg().BangumiEnabled = true
	app.cfg().BangumiWebhookSecret = "webhook-secret"
	created, err := app.store().CreateUser(store.User{Username: "viewer", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().UpdateUser(created.UID, func(u *store.User) error {
		u.EmbyID = "emby-replay"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	body := `{"Event":"PlaybackStopped","UserId":"emby-replay","Item":{"Id":"item-replay","Name":"Replay Title","Type":"Episode","RunTimeTicks":600000000}}`
	// 同一份字节重放必须用同一份 timestamp header,否则 PlayedAt 在跨秒边
	// 界会被 time.Now() 拉开,(uid,item_id,played_at) 唯一键失效。这本来
	// 就是合法重放攻击的前提:攻击者抓的是字节,header 当然也一样。
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	for i := 0; i < 5; i++ {
		rec := doJSONWithHeaders(app, http.MethodPost, "/api/v1/emby/bangumi/webhook", body, nil, map[string]string{
			"X-Twilight-Bangumi-Token":     "webhook-secret",
			"X-Twilight-Bangumi-Timestamp": ts,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("replay #%d webhook = %d body=%s", i, rec.Code, rec.Body.String())
		}
	}

	records := app.store().PlaybackRecords(created.UID, 0, 100)
	itemMatches := 0
	for _, rec := range records {
		if rec.ItemID == "item-replay" {
			itemMatches++
		}
	}
	if itemMatches != 1 {
		t.Fatalf("expected exactly 1 PlaybackRecord for replayed webhook, got %d (records=%#v)", itemMatches, records)
	}
}

// TestBangumiWebhookConstantTimeStringEqual 锁定 R58-1 timing oracle 收紧:
// length-mismatch 不能再通过 ConstantTimeCompare 提前 return 触发。直接调
// 用 helper 验证逻辑等价性(timing 行为靠 zero-pad 实现,Go 测试框架不便
// 直接测,这里至少锁住语义)。
func TestBangumiWebhookConstantTimeStringEqual(t *testing.T) {
	cases := []struct {
		got, want string
		expect    bool
	}{
		{"abc", "abc", true},
		{"abc", "xyz", false},
		{"abc", "abcd", false}, // 长度不同
		{"", "", true},
		{"", "abc", false},
		{strings.Repeat("a", 1025), strings.Repeat("a", 1025), false}, // 超过 1024 直接 false
	}
	for _, c := range cases {
		if got := constantTimeStringEqual(c.got, c.want); got != c.expect {
			t.Errorf("constantTimeStringEqual(%q,%q) = %v, want %v", c.got, c.want, got, c.expect)
		}
	}
}

func TestDatabaseAdminBackupRestoreAndAuth(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	adminLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
	adminCookie := findCookie(adminLogin.Result().Cookies(), "twilight_session")
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"user","password":"User123456"}`, nil)
	userLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"user","password":"User123456"}`, nil)
	userCookie := findCookie(userLogin.Result().Cookies(), "twilight_session")

	unauth := doJSON(app, http.MethodGet, "/api/v1/system/admin/database/status", ``, nil)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("database status unauth = %d", unauth.Code)
	}
	forbidden := doJSON(app, http.MethodGet, "/api/v1/system/admin/database/status", ``, []*http.Cookie{userCookie})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("database status user = %d body=%s", forbidden.Code, forbidden.Body.String())
	}
	status := doJSON(app, http.MethodGet, "/api/v1/system/admin/database/status", ``, []*http.Cookie{adminCookie})
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"legacy_sqlite_detected":false`) {
		t.Fatalf("database status did not report sqlite disabled status=%d body=%s", status.Code, status.Body.String())
	}
	backup := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/backup", `{"note":"before restore test"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if backup.Code != http.StatusOK {
		t.Fatalf("backup status=%d body=%s", backup.Code, backup.Body.String())
	}
	if strings.Contains(backup.Body.String(), `"legacy_sqlite_backup"`) {
		t.Fatalf("backup unexpectedly included legacy sqlite files body=%s", backup.Body.String())
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

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"extra","password":"Extra123456"}`, nil)
	if _, ok := app.store().FindUserByUsername("extra"); !ok {
		t.Fatal("expected extra user before restore")
	}
	restorePreview := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/restore", `{"name":"`+backupName+`"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if restorePreview.Code != http.StatusOK || !strings.Contains(restorePreview.Body.String(), `"requires_confirmation":true`) {
		t.Fatalf("restore preview status=%d body=%s", restorePreview.Code, restorePreview.Body.String())
	}
	if _, ok := app.store().FindUserByUsername("extra"); !ok {
		t.Fatal("restore preview mutated state")
	}
	restore := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/restore", `{"name":"`+backupName+`","confirm":"RESTORE_DATABASE_BACKUP"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if restore.Code != http.StatusOK {
		t.Fatalf("restore status=%d body=%s", restore.Code, restore.Body.String())
	}
	if !strings.Contains(restore.Body.String(), `"pre_operation_backup"`) {
		t.Fatalf("restore did not report pre-operation backup body=%s", restore.Body.String())
	}
	if _, ok := app.store().FindUserByUsername("extra"); ok {
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
	app.cfg().DatabaseMigrationPanelEnabled = true
	migrate := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/migrate", `{"target_driver":"json","dry_run":true}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if migrate.Code != http.StatusOK || !strings.Contains(migrate.Body.String(), `"dry_run":true`) {
		t.Fatalf("migrate dry-run status=%d body=%s", migrate.Code, migrate.Body.String())
	}
	migrateNoConfirm := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/migrate", `{"target_driver":"json","state_file":"migrated.json"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if migrateNoConfirm.Code != http.StatusOK || !strings.Contains(migrateNoConfirm.Body.String(), `"requires_confirmation":true`) || !strings.Contains(migrateNoConfirm.Body.String(), `"dry_run":true`) {
		t.Fatalf("migrate without confirm status=%d body=%s", migrateNoConfirm.Code, migrateNoConfirm.Body.String())
	}
	if _, err := os.Stat(filepath.Join(app.cfg().DatabaseDir, "migrated.json")); err == nil {
		t.Fatal("migrate without confirm wrote target file")
	}
	migrateExecute := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/database/migrate", `{"target_driver":"json","state_file":"migrated.json","confirm":"MIGRATE_DATABASE"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if migrateExecute.Code != http.StatusOK || !strings.Contains(migrateExecute.Body.String(), `"pre_operation_backup"`) {
		t.Fatalf("migrate execute status=%d body=%s", migrateExecute.Code, migrateExecute.Body.String())
	}
	if _, err := os.Stat(filepath.Join(app.cfg().DatabaseDir, "migrated.json")); err != nil {
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
	app.cfg().ConfigFile = filepath.Join(app.cfg().DatabaseDir, "config.toml")
	original := "[Global]\ndatabases_dir = " + strconv.Quote(app.cfg().DatabaseDir) + "\n\n[Database]\ndriver = " + strconv.Quote(app.cfg().DatabaseDriver) + "\nbackup_dir = " + strconv.Quote(app.cfg().DatabaseBackupDir) + "\nstate_file = " + strconv.Quote(app.cfg().StateFile) + "\n\n[API]\nhost = \"127.0.0.1\"\nport = 5010\n"
	changed := strings.Replace(original, "port = 5010", "port = 5011", 1)
	if err := os.WriteFile(app.cfg().ConfigFile, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	adminLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
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

	if err := os.WriteFile(app.cfg().ConfigFile, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	restorePreview := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/config/restore", `{"name":"`+backupName+`"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if restorePreview.Code != http.StatusOK || !strings.Contains(restorePreview.Body.String(), `"requires_confirmation":true`) {
		t.Fatalf("config restore preview status=%d body=%s", restorePreview.Code, restorePreview.Body.String())
	}
	if data, _ := os.ReadFile(app.cfg().ConfigFile); !strings.Contains(string(data), "port = 5011") {
		t.Fatal("config restore preview mutated file")
	}
	restore := doJSONWithHeaders(app, http.MethodPost, "/api/v1/system/admin/config/restore", `{"name":"`+backupName+`","confirm":"RESTORE_CONFIG_BACKUP"}`, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if restore.Code != http.StatusOK || !strings.Contains(restore.Body.String(), `"pre_operation_backup"`) {
		t.Fatalf("config restore status=%d body=%s", restore.Code, restore.Body.String())
	}
	if data, _ := os.ReadFile(app.cfg().ConfigFile); !strings.Contains(string(data), "port = 5010") {
		t.Fatalf("config restore did not restore original content: %s", string(data))
	}

	adminLogin = doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
	adminCookie = findCookie(adminLogin.Result().Cookies(), "twilight_session")
	deleteBackup := doJSONWithHeaders(app, http.MethodDelete, "/api/v1/system/admin/config/backups/"+backupName, ``, []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if deleteBackup.Code != http.StatusOK {
		t.Fatalf("config backup delete status=%d body=%s", deleteBackup.Code, deleteBackup.Body.String())
	}
}

func TestConfigTOMLGetReturnsCompletedConfig(t *testing.T) {
	app := newTestApp(t)
	app.cfg().ConfigFile = filepath.Join(app.cfg().DatabaseDir, "config.toml")
	minimal := "[Global]\ndatabases_dir = " + strconv.Quote(app.cfg().DatabaseDir) + "\n\n[Database]\ndriver = " + strconv.Quote(app.cfg().DatabaseDriver) + "\nstate_file = " + strconv.Quote(app.cfg().StateFile) + "\nbackup_dir = " + strconv.Quote(app.cfg().DatabaseBackupDir) + "\n\n[API]\nhost = \"127.0.0.1\"\nport = 5010\n\n[Admin]\nusernames = [\"root\"]\n"
	if err := os.WriteFile(app.cfg().ConfigFile, []byte(minimal), 0o600); err != nil {
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

// TestConfigTOMLGetMasksSecretsAndPUTPreserves 验证：
//   - GET /system/admin/config/toml 不在 content / raw_content 中回传任何
//     真实密钥（Emby Token / Bot Token / BotInternalSecret / Webhook Secret /
//     Postgres 密码 / Redis URL）；非空密钥一律被替换为哨兵 secretMaskValue。
//   - PUT 回传带哨兵的 content 时，写盘前哨兵被还原为内存中的真实值，
//     不会把密钥清成无效字符串（防止"打开配置页再保存就把密钥清空"）。
func TestConfigTOMLGetMasksSecretsAndPUTPreserves(t *testing.T) {
	app := newTestApp(t)
	app.cfg().ConfigFile = filepath.Join(app.cfg().DatabaseDir, "config.toml")
	const embyToken = "EMBY_TOKEN_TOPSECRET_123456"
	const botToken = "8585413194:AAFwzhD0_BOT_TOKEN_SECRET"
	const internalSecret = "tw_internal_secret_value_abc"
	const webhookSecret = "bangumi_webhook_secret_xyz"
	raw := "[Global]\ndatabases_dir = " + strconv.Quote(app.cfg().DatabaseDir) + "\nredis_url = \"\"\n\n" +
		"[Database]\ndriver = " + strconv.Quote(app.cfg().DatabaseDriver) + "\nstate_file = " + strconv.Quote(app.cfg().StateFile) + "\nbackup_dir = " + strconv.Quote(app.cfg().DatabaseBackupDir) + "\npostgres_password = \"PG_PASS_SECRET\"\n\n" +
		"[Emby]\nemby_url = \"http://127.0.0.1:8096/\"\nemby_token = " + strconv.Quote(embyToken) + "\n\n" +
		"[Telegram]\nbot_token = " + strconv.Quote(botToken) + "\n\n" +
		"[Security]\nbot_internal_secret = " + strconv.Quote(internalSecret) + "\n\n" +
		"[BangumiSync]\nwebhook_secret = " + strconv.Quote(webhookSecret) + "\n\n" +
		"[API]\nhost = \"127.0.0.1\"\nport = 5010\n\n[Admin]\nusernames = [\"admin\"]\n"
	if err := os.WriteFile(app.cfg().ConfigFile, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	// 让 reload 把磁盘上的密钥读进内存 cfg，PUT 还原哨兵时需要它。
	if _, err := app.reloadConfig(); err != nil {
		t.Fatalf("reload config: %v", err)
	}

	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	adminLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
	adminCookie := findCookie(adminLogin.Result().Cookies(), "twilight_session")

	get := doJSONWithHeaders(app, http.MethodGet, "/api/v1/system/admin/config/toml", ``, []*http.Cookie{adminCookie}, nil)
	if get.Code != http.StatusOK {
		t.Fatalf("config toml get status=%d body=%s", get.Code, get.Body.String())
	}
	body := get.Body.String()
	for _, secret := range []string{embyToken, botToken, internalSecret, webhookSecret, "PG_PASS_SECRET"} {
		if strings.Contains(body, secret) {
			t.Fatalf("plaintext secret leaked in config toml GET: %q\nbody=%s", secret, body)
		}
	}
	if !strings.Contains(body, secretMaskValue) {
		t.Fatalf("expected masked secret sentinel in GET body, got: %s", body)
	}

	var env envelope
	if err := json.Unmarshal(get.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	data := env.Data.(map[string]any)
	maskedContent := data["content"].(string)

	// PUT 回传遮蔽后的 content（管理员未改密钥），保存后磁盘必须保留真实密钥。
	putBody, _ := json.Marshal(map[string]any{"content": maskedContent})
	put := doJSONWithHeaders(app, http.MethodPut, "/api/v1/system/admin/config/toml", string(putBody), []*http.Cookie{adminCookie}, map[string]string{"X-Twilight-Client": "webui"})
	if put.Code != http.StatusOK {
		t.Fatalf("config toml put status=%d body=%s", put.Code, put.Body.String())
	}
	saved, err := os.ReadFile(app.cfg().ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	savedContent := string(saved)
	if strings.Contains(savedContent, secretMaskValue) {
		t.Fatalf("sentinel was written to disk instead of real secret:\n%s", savedContent)
	}
	for _, secret := range []string{embyToken, botToken, internalSecret, webhookSecret} {
		if !strings.Contains(savedContent, secret) {
			t.Fatalf("real secret %q was lost after masked PUT round-trip:\n%s", secret, savedContent)
		}
	}
}

func TestConfigSaveMigratesLegacyTelegramForceSubscribe(t *testing.T) {
	app := newTestApp(t)
	app.cfg().ConfigFile = filepath.Join(app.cfg().DatabaseDir, "config.toml")
	existing := "[Global]\nserver_name = \"old\"\n\n[Admin]\nusernames = [\"root\"]\n"
	if err := os.WriteFile(app.cfg().ConfigFile, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy := `[Global]
server_name = "new"
databases_dir = "` + strings.ReplaceAll(app.cfg().DatabaseDir, `\`, `\\`) + `"

[Database]
driver = "json"
state_file = "` + strings.ReplaceAll(app.cfg().StateFile, `\`, `\\`) + `"
backup_dir = "` + strings.ReplaceAll(app.cfg().DatabaseBackupDir, `\`, `\\`) + `"

[Telegram]
group_id = ["@group"]
channel_id = ["@channel"]
force_subscribe = true
`
	info, status, message := app.saveConfigContent(legacy)
	if status != http.StatusOK {
		t.Fatalf("saveConfigContent status=%d message=%s info=%v", status, message, info)
	}
	data, err := os.ReadFile(app.cfg().ConfigFile)
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
	app.cfg().UserLimit = 100
	app.cfg().EmbyUserLimit = 3
	now := time.Now().Unix()
	if _, err := app.store().CreateUser(store.User{Username: "emby", Role: store.RoleNormal, Active: true, EmbyID: "emby-1"}); err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "INV-A", UID: 1, InviterUID: 1, Days: 30, UseCountLimit: 1, Active: true, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertRegCode(store.RegCode{Code: "REG-A", Type: 1, Days: 30, ValidityTime: -1, UseCountLimit: 1, Active: true, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertRegCode(store.RegCode{Code: "REG-RENEW", Type: 2, Days: 30, ValidityTime: -1, UseCountLimit: 100, Active: true, CreatedAt: now}); err != nil {
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

func TestRegcodeFormatsAreSeparatedWithLegacyFallback(t *testing.T) {
	app := newTestApp(t)
	app.cfg().RegCodeFormat = "OLD-{type}-{random}"
	app.cfg().RegisterCodeFormat = ""
	app.cfg().RenewCodeFormat = ""

	create := func(codeType int) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/regcodes", strings.NewReader(fmt.Sprintf(`{"type":%d,"days":7,"count":1,"random_algorithm":"digits-12"}`, codeType)))
		rr := httptest.NewRecorder()
		app.handleCreateRegcodes(rr, req, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("create type %d status=%d body=%s", codeType, rr.Code, rr.Body.String())
		}
		var env envelope
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		return env.Data.(map[string]any)["codes"].([]any)[0].(string)
	}

	if code := create(1); !strings.HasPrefix(code, "OLD-REG-") {
		t.Fatalf("register code should fall back to legacy regcode_format, got %q", code)
	}
	if code := create(2); !strings.HasPrefix(code, "OLD-REN-") {
		t.Fatalf("renew code should fall back to legacy regcode_format, got %q", code)
	}

	app.cfg().RegisterCodeFormat = "REGONLY-{random}"
	app.cfg().RenewCodeFormat = "RENONLY-{days}-{random}"
	if code := create(1); !strings.HasPrefix(code, "REGONLY-") {
		t.Fatalf("register code should use register_code_format, got %q", code)
	}
	if code := create(2); !strings.HasPrefix(code, "RENONLY-7-") {
		t.Fatalf("renew code should use renew_code_format, got %q", code)
	}
	if code := create(3); !strings.HasPrefix(code, "OLD-VIP-") {
		t.Fatalf("whitelist code should keep legacy regcode_format fallback, got %q", code)
	}
}

func TestInviteCodeFormatAndDTOUserRecords(t *testing.T) {
	app := newTestApp(t)
	app.cfg().InviteCodeFormat = "JOIN-{days}-{index}-{random}"
	app.cfg().InviteCodeRandomAlgorithm = "digits-12"
	now := time.Now()
	parent, err := app.store().CreateUser(store.User{Username: "invite-format-parent", Role: store.RoleNormal, Active: true, ExpiredAt: now.AddDate(0, 0, 30).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.store().CreateUser(store.User{Username: "invite-format-child", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/invite/codes", strings.NewReader(`{"days":7,"target_username":"invite-format-child"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: parent}))
	rr := httptest.NewRecorder()
	app.handleCreateInviteCode(rr, req, nil)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create invite status=%d body=%s", rr.Code, rr.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	code := env.Data.(map[string]any)["code"].(string)
	if !strings.HasPrefix(code, "JOIN-7-1-") {
		t.Fatalf("invite code should use invite_code_format, got %q", code)
	}
	randomPart := strings.TrimPrefix(code, "JOIN-7-1-")
	if len(randomPart) != 12 || strings.Trim(randomPart, "0123456789") != "" {
		t.Fatalf("invite code should use invite_code_random_algorithm digits-12, got %q", code)
	}
	if _, err := app.store().ConsumeInviteCode(code, child.UID); err != nil {
		t.Fatal(err)
	}
	stored, _ := app.store().InviteCode(code)
	dto := app.inviteCodeDTO(stored)
	if dto["inviter_username"] != parent.Username || dto["used_by_username"] != child.Username || numeric(dto["target_uid"]) != child.UID {
		t.Fatalf("invite dto should include inviter, used-by and target records: %#v", dto)
	}
}

func TestTargetedRegcodesAreCreatedListedAndEnforced(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	adminLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
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
	reg, ok := app.store().RegCode(code)
	if !ok || reg.TargetUsername != "alpha" {
		t.Fatalf("target username was not saved: %#v", reg)
	}
	list := doJSONWithHeaders(app, http.MethodGet, "/api/v1/admin/regcodes?search=alpha", ``, []*http.Cookie{adminCookie}, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"target_username":"alpha"`) {
		t.Fatalf("target username was not searchable/listed, status=%d body=%s", list.Code, list.Body.String())
	}

	alpha, err := app.store().CreateUser(store.User{Username: "alpha", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := app.store().CreateUser(store.User{Username: "beta", Role: store.RoleNormal, Active: true})
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
	reg, _ = app.store().RegCode(code)
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

	if err := app.store().UpsertRegCode(store.RegCode{Code: "RENEW-ALPHA", Type: 2, Days: 5, ValidityTime: -1, UseCountLimit: 1, Active: true, TargetUsername: "alpha"}); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/me/renew", strings.NewReader(`{"reg_code":"RENEW-ALPHA"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: beta}))
	rr = httptest.NewRecorder()
	app.handleRenew(rr, req, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("renew endpoint should reject non-target user, status=%d body=%s", rr.Code, rr.Body.String())
	}
	reg, _ = app.store().RegCode("RENEW-ALPHA")
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

func TestRegisterCodeLimitConsumesRegcodeAtomically(t *testing.T) {
	app := newTestApp(t)
	adminResp := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	if adminResp.Code != http.StatusCreated {
		t.Fatalf("bootstrap register status=%d body=%s", adminResp.Code, adminResp.Body.String())
	}
	app.cfg().RegisterCodeLimit = true

	missing := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"no-code","password":"User123456"}`, nil)
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), `"error_code":"CODE_EMPTY"`) {
		t.Fatalf("register without code should be rejected, status=%d body=%s", missing.Code, missing.Body.String())
	}

	if err := app.store().UpsertRegCode(store.RegCode{Code: "REGISTER-OK", Type: 1, Days: 7, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	created := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"with-code","password":"User123456","reg_code":"REGISTER-OK"}`, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("register with code status=%d body=%s", created.Code, created.Body.String())
	}
	user, ok := app.store().FindUserByUsername("with-code")
	if !ok || !user.PendingEmby || user.PendingEmbyDays == nil || *user.PendingEmbyDays != 7 {
		t.Fatalf("registered user did not receive pending entitlement: %#v days=%#v", user, user.PendingEmbyDays)
	}
	reg, ok := app.store().RegCode("REGISTER-OK")
	if !ok || reg.UseCount != 1 || reg.UsedBy != user.UID || reg.Active {
		t.Fatalf("register code usage was not persisted: %#v", reg)
	}

	reuse := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"reuse-code","password":"User123456","reg_code":"REGISTER-OK"}`, nil)
	if reuse.Code != http.StatusBadRequest {
		t.Fatalf("used register code should be rejected, status=%d body=%s", reuse.Code, reuse.Body.String())
	}

	if err := app.store().UpsertRegCode(store.RegCode{Code: "RENEW-NOT-REGISTER", Type: 2, Days: 30, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	wrongType := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"wrong-type","password":"User123456","reg_code":"RENEW-NOT-REGISTER"}`, nil)
	if wrongType.Code != http.StatusBadRequest {
		t.Fatalf("renew code should not register users, status=%d body=%s", wrongType.Code, wrongType.Body.String())
	}
	reg, _ = app.store().RegCode("RENEW-NOT-REGISTER")
	if reg.UseCount != 0 {
		t.Fatalf("wrong type code was consumed: %#v", reg)
	}
}

func TestRenewEndpointHonorsPermanentRegcode(t *testing.T) {
	app := newTestApp(t)
	user, err := app.store().CreateUser(store.User{Username: "renew-permanent", Role: store.RoleNormal, Active: true, ExpiredAt: time.Now().AddDate(0, 0, 1).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertRegCode(store.RegCode{Code: "PERMANENT-RENEW", Type: 2, Days: -1, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/renew", strings.NewReader(`{"reg_code":"PERMANENT-RENEW"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: user}))
	rr := httptest.NewRecorder()
	app.handleRenew(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("permanent renew status=%d body=%s", rr.Code, rr.Body.String())
	}
	updated, _ := app.store().User(user.UID)
	if updated.ExpiredAt != permanentExpiryUnix {
		t.Fatalf("permanent renew should set permanent expiry, got %d", updated.ExpiredAt)
	}
}

// TestMediaRequestGlobalLimitBlocksNonAdmins 验证 max_concurrent_requests_global
// 上限：达到后普通用户的新求片返回 MEDIA_REQUEST_GLOBAL_LIMIT，admin 仍可继续。
func TestMediaRequestGlobalLimitBlocksNonAdmins(t *testing.T) {
	app := newTestApp(t)
	app.cfg().MaxConcurrentRequestsGlobal = 2
	app.cfg().MaxConcurrentRequestsPerUser = -1
	app.cfg().EmbyToken = "test-token"
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Items":[],"TotalRecordCount":0}`))
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")
	if admin, ok := app.store().FindUserByUsername("admin"); ok {
		_, _ = app.store().UpdateUser(admin.UID, func(u *store.User) error { u.TelegramID = 1; return nil })
	}
	userCookies := registerAndLogin(t, app, "user-one", "User123456")
	user, _ := app.store().FindUserByUsername("user-one")
	_, _ = app.store().UpdateUser(user.UID, func(u *store.User) error { u.TelegramID = 2; return nil })
	userCookies2 := registerAndLogin(t, app, "user-two", "User123456")
	user2, _ := app.store().FindUserByUsername("user-two")
	_, _ = app.store().UpdateUser(user2.UID, func(u *store.User) error { u.TelegramID = 3; return nil })

	makeReq := func(cookies []*http.Cookie, mediaID int) *httptest.ResponseRecorder {
		return doJSONWithHeaders(app, http.MethodPost, "/api/v1/media/request",
			fmt.Sprintf(`{"source":"tmdb","media_id":%d,"title":"M%d","media_type":"movie"}`, mediaID, mediaID),
			cookies, map[string]string{"X-Twilight-Client": "webui"})
	}

	if r := makeReq(userCookies, 100); r.Code != http.StatusCreated {
		t.Fatalf("first user request should succeed, status=%d body=%s", r.Code, r.Body.String())
	}
	if r := makeReq(userCookies2, 101); r.Code != http.StatusCreated {
		t.Fatalf("second user request should succeed, status=%d body=%s", r.Code, r.Body.String())
	}
	// 全局 2 已满，第三个非 admin 求片应该被挡掉
	blocked := makeReq(userCookies, 102)
	if blocked.Code != http.StatusTooManyRequests || !strings.Contains(blocked.Body.String(), `"error_code":"MEDIA_REQUEST_GLOBAL_LIMIT"`) {
		t.Fatalf("third request should hit global limit, status=%d body=%s", blocked.Code, blocked.Body.String())
	}
	// admin 不受全局上限影响
	adminOk := makeReq(adminCookies, 103)
	if adminOk.Code != http.StatusCreated {
		t.Fatalf("admin request should bypass global limit, status=%d body=%s", adminOk.Code, adminOk.Body.String())
	}
}

func TestInviteParentCanDetachExpiredChildAndKeepWebAccountActive(t *testing.T) {
	app := newTestApp(t)
	deletedRemote := false
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/Users/emby-child" {
			t.Fatalf("unexpected Emby request: %s %s", r.Method, r.URL.Path)
		}
		deletedRemote = true
		_, _ = w.Write([]byte(`{}`))
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL
	app.cfg().EmbyToken = "token"
	parent, err := app.store().CreateUser(store.User{Username: "parent", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.store().CreateUser(store.User{Username: "child", Role: store.RoleNormal, Active: true, ExpiredAt: time.Now().AddDate(0, 0, -1).Unix(), EmbyID: "emby-child", EmbyUsername: "child"})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "INV-CHILD", UID: parent.UID, InviterUID: parent.UID, Days: 30, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().ConsumeInviteCode("INV-CHILD", child.UID); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invite/children/2/detach-expired", nil)
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: parent}))
	rr := httptest.NewRecorder()
	app.handleDetachExpiredInviteChild(rr, req, Params{"uid": strconv.FormatInt(child.UID, 10)})
	if rr.Code != http.StatusOK {
		t.Fatalf("detach status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, ok := app.store().ParentOf(child.UID); ok {
		t.Fatal("child still has invite parent")
	}
	if !deletedRemote {
		t.Fatal("remote Emby user was not deleted")
	}
	updated, ok := app.store().User(child.UID)
	if !ok || !updated.Active || updated.EmbyID != "" || updated.EmbyUsername != "" || updated.PendingEmby {
		t.Fatalf("child web account was not preserved while Emby was cleared: %#v", updated)
	}
}

func TestInviteParentCanDetachDisabledChildWithoutReactivatingWeb(t *testing.T) {
	app := newTestApp(t)
	deletedRemote := false
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/Users/emby-disabled-child" {
			t.Fatalf("unexpected Emby request: %s %s", r.Method, r.URL.Path)
		}
		deletedRemote = true
		_, _ = w.Write([]byte(`{}`))
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL
	app.cfg().EmbyToken = "token"
	parent, err := app.store().CreateUser(store.User{Username: "disabled-parent", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.store().CreateUser(store.User{Username: "disabled-child", Role: store.RoleNormal, Active: false, ExpiredAt: time.Now().AddDate(0, 0, 10).Unix(), EmbyID: "emby-disabled-child", EmbyUsername: "disabled-child"})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "INV-DISABLED-CHILD", UID: parent.UID, InviterUID: parent.UID, Days: 30, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().ConsumeInviteCode("INV-DISABLED-CHILD", child.UID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().UpdateUser(child.UID, func(u *store.User) error { u.Active = false; return nil }); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invite/children/2/detach-expired", nil)
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: parent}))
	rr := httptest.NewRecorder()
	app.handleDetachExpiredInviteChild(rr, req, Params{"uid": strconv.FormatInt(child.UID, 10)})
	if rr.Code != http.StatusOK {
		t.Fatalf("detach disabled child status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !deletedRemote {
		t.Fatal("remote Emby user was not deleted")
	}
	if _, ok := app.store().ParentOf(child.UID); ok {
		t.Fatal("disabled child still has invite parent")
	}
	updated, ok := app.store().User(child.UID)
	if !ok || updated.Active || updated.EmbyID != "" || updated.EmbyUsername != "" {
		t.Fatalf("disabled child should stay disabled with Emby cleared: %#v", updated)
	}
}

func TestRegcodeDTOAndUsersExposeLegacyUsedBy(t *testing.T) {
	app := newTestApp(t)
	user, err := app.store().CreateUser(store.User{Username: "legacy-user", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	reg := store.RegCode{Code: "LEGACY-USED", Type: 2, Days: 30, ValidityTime: -1, UseCountLimit: 5, UseCount: 1, UsedBy: user.UID, Active: true}
	dto := regcodeDTO(reg)
	uids, _ := dto["used_by_uids"].([]int64)
	if len(uids) != 1 || uids[0] != user.UID || asString(dto["used_by"]) != strconv.FormatInt(user.UID, 10) {
		t.Fatalf("legacy used_by should be exposed in dto: %#v", dto)
	}
	if err := app.store().UpsertRegCode(reg); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/regcodes/LEGACY-USED/users", nil)
	rr := httptest.NewRecorder()
	app.handleRegcodeUsers(rr, req, Params{"code": "LEGACY-USED"})
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"username":"legacy-user"`) {
		t.Fatalf("legacy used_by user should be listed, status=%d body=%s", rr.Code, rr.Body.String())
	}
	appDTO := app.regcodeDTO(reg)
	usernames, _ := appDTO["used_by_usernames"].([]string)
	if len(usernames) != 1 || usernames[0] != user.Username {
		t.Fatalf("used_by_usernames should include legacy user: %#v", appDTO)
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
	app.cfg().EmbyUserLimit = 1
	oldDays := 90
	user, err := app.store().CreateUser(store.User{Username: "pending-replace", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	updatedUser, err := app.store().UpdateUser(user.UID, func(u *store.User) error {
		u.PendingEmby = true
		u.PendingEmbyDays = &oldDays
		u.EmbyUsername = "old-name"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	user = updatedUser
	if err := app.store().UpsertRegCode(store.RegCode{Code: "REG-REPLACE", Type: 1, Days: 7, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/use-code", strings.NewReader(`{"reg_code":"REG-REPLACE","emby_username":"new-name"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: user}))
	rr := httptest.NewRecorder()
	app.handleUseCode(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("pending entitlement replacement should use register code, status=%d body=%s", rr.Code, rr.Body.String())
	}
	updated, _ := app.store().User(user.UID)
	if !updated.PendingEmby || updated.PendingEmbyDays == nil || *updated.PendingEmbyDays != 7 || updated.EmbyUsername != "new-name" {
		t.Fatalf("register code did not replace pending entitlement cleanly: %#v days=%#v", updated, updated.PendingEmbyDays)
	}
	reg, _ := app.store().RegCode("REG-REPLACE")
	if reg.UseCount != 1 || reg.UsedBy != user.UID {
		t.Fatalf("register code usage was not recorded: %#v", reg)
	}
}

func TestRegisterCodeCapacityExcludesCodeBeingConsumedAndRejectsBoundUser(t *testing.T) {
	app := newTestApp(t)
	app.cfg().EmbyUserLimit = 1
	user, err := app.store().CreateUser(store.User{Username: "capacity-user", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertRegCode(store.RegCode{Code: "REG-CAPACITY", Type: 1, Days: 7, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/use-code", strings.NewReader(`{"reg_code":"REG-CAPACITY"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: user}))
	rr := httptest.NewRecorder()
	app.handleUseCode(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("register code should consume its own reserved capacity slot, status=%d body=%s", rr.Code, rr.Body.String())
	}

	bound, err := app.store().CreateUser(store.User{Username: "bound-user", Role: store.RoleNormal, Active: true, EmbyID: "emby-bound"})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertRegCode(store.RegCode{Code: "REG-BOUND", Type: 1, Days: 7, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
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
	reg, _ := app.store().RegCode("REG-BOUND")
	if reg.UseCount != 0 {
		t.Fatalf("rejected register code was consumed: %#v", reg)
	}

	if err := app.store().UpsertRegCode(store.RegCode{Code: "VIP-BOUND", Type: 3, Days: -1, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/me/use-code", strings.NewReader(`{"reg_code":"VIP-BOUND"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: bound}))
	rr = httptest.NewRecorder()
	app.handleUseCode(rr, req, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Emby-bound user should not use whitelist code as registration entitlement, status=%d body=%s", rr.Code, rr.Body.String())
	}
	reg, _ = app.store().RegCode("VIP-BOUND")
	if reg.UseCount != 0 {
		t.Fatalf("rejected whitelist code was consumed: %#v", reg)
	}

	parent, err := app.store().CreateUser(store.User{Username: "invite-parent-bound-check", Role: store.RoleNormal, Active: true, EmbyID: "emby-parent", ExpiredAt: time.Now().AddDate(0, 0, 30).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "INV-BOUND-CHECK", UID: parent.UID, InviterUID: parent.UID, Days: 7, UseCountLimit: 1, Active: true, CreatedAt: time.Now().Unix(), ExpiredAt: time.Now().Add(time.Hour).Unix()}); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/me/use-code", strings.NewReader(`{"reg_code":"INV-BOUND-CHECK","check_only":true}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: bound}))
	rr = httptest.NewRecorder()
	app.handleUseCode(rr, req, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Emby-bound user should not preview invite code as usable, status=%d body=%s", rr.Code, rr.Body.String())
	}
	invite, _ := app.store().InviteCode("INV-BOUND-CHECK")
	if invite.UseCount != 0 {
		t.Fatalf("rejected invite code was consumed: %#v", invite)
	}

	if err := app.store().UpsertRegCode(store.RegCode{Code: "RENEW-BOUND", Type: 2, Days: 7, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/me/use-code", strings.NewReader(`{"reg_code":"RENEW-BOUND"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: bound}))
	rr = httptest.NewRecorder()
	app.handleUseCode(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("Emby-bound user should still use renewal code, status=%d body=%s", rr.Code, rr.Body.String())
	}
	reg, _ = app.store().RegCode("RENEW-BOUND")
	if reg.UseCount != 1 {
		t.Fatalf("renewal code was not consumed: %#v", reg)
	}
}

func TestExpiredInviteCannotBeUsedOrConsumed(t *testing.T) {
	app := newTestApp(t)
	now := time.Now()
	parent, err := app.store().CreateUser(store.User{Username: "expired-invite-parent", Role: store.RoleNormal, Active: true, EmbyID: "emby-parent", ExpiredAt: now.AddDate(0, 0, 30).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.store().CreateUser(store.User{Username: "expired-invite-child", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "INV-EXPIRED-CODE", UID: parent.UID, InviterUID: parent.UID, Days: 7, UseCountLimit: 1, Active: true, CreatedAt: now.Add(-time.Hour).Unix(), ExpiredAt: now.Add(-time.Second).Unix()}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invite/use", strings.NewReader(`{"code":"INV-EXPIRED-CODE"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: child}))
	rr := httptest.NewRecorder()
	app.handleInviteUse(rr, req, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expired invite should be rejected, status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := app.store().ConsumeInviteCode("INV-EXPIRED-CODE", child.UID); !errors.Is(err, store.ErrExpired) {
		t.Fatalf("store should reject expired invite consumption, got %v", err)
	}
	invite, _ := app.store().InviteCode("INV-EXPIRED-CODE")
	if invite.UseCount != 0 || invite.UsedByUID != 0 {
		t.Fatalf("expired invite was consumed: %#v", invite)
	}
}

func TestInviteRenewCodeCreatesTargetedRegCode(t *testing.T) {
	app := newTestApp(t)
	now := time.Now()
	parent, err := app.store().CreateUser(store.User{Username: "renew-parent", Role: store.RoleNormal, Active: true, EmbyID: "emby-parent", ExpiredAt: now.AddDate(0, 0, 30).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.store().CreateUser(store.User{Username: "renew-child", Role: store.RoleNormal, Active: true, ExpiredAt: now.AddDate(0, 0, 1).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := app.store().CreateUser(store.User{Username: "renew-outsider", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "INV-RENEW-PARENT", UID: parent.UID, InviterUID: parent.UID, Days: 7, UseCountLimit: 1, Active: true, CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().ConsumeInviteCode("INV-RENEW-PARENT", child.UID); err != nil {
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
	reg, ok := app.store().RegCode(code)
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

func TestInviteDisabledStillAllowsExistingChildRenewCodesButBlocksNewInvites(t *testing.T) {
	app := newTestApp(t)
	app.cfg().InviteEnabled = false
	now := time.Now()
	parent, err := app.store().CreateUser(store.User{Username: "disabled-renew-parent", Role: store.RoleNormal, Active: true, EmbyID: "emby-parent", ExpiredAt: now.AddDate(0, 0, 30).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.store().CreateUser(store.User{Username: "disabled-renew-child", Role: store.RoleNormal, Active: true, ExpiredAt: now.AddDate(0, 0, -1).Unix(), EmbyID: "emby-child"})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "INV-DISABLED-RENEW", UID: parent.UID, InviterUID: parent.UID, Days: 7, UseCountLimit: 1, Active: true, CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().ConsumeInviteCode("INV-DISABLED-RENEW", child.UID); err != nil {
		t.Fatal(err)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/invite/me", nil)
	meReq = meReq.WithContext(context.WithValue(meReq.Context(), principalKey, principal{User: parent}))
	meRR := httptest.NewRecorder()
	app.handleInviteMe(meRR, meReq, nil)
	if meRR.Code != http.StatusOK || !strings.Contains(meRR.Body.String(), `"enabled":false`) || !strings.Contains(meRR.Body.String(), `"can_invite":false`) || !strings.Contains(meRR.Body.String(), `"can_generate_renew_code":true`) {
		t.Fatalf("disabled invite/me should expose existing child renewal controls, status=%d body=%s", meRR.Code, meRR.Body.String())
	}

	inviteReq := httptest.NewRequest(http.MethodPost, "/api/v1/invite/codes", strings.NewReader(`{"days":7}`))
	inviteReq = inviteReq.WithContext(context.WithValue(inviteReq.Context(), principalKey, principal{User: parent}))
	inviteRR := httptest.NewRecorder()
	app.handleCreateInviteCode(inviteRR, inviteReq, nil)
	if inviteRR.Code != http.StatusForbidden || !strings.Contains(inviteRR.Body.String(), string(ErrInviteDisabled)) {
		t.Fatalf("disabled invite system should block new invite codes, status=%d body=%s", inviteRR.Code, inviteRR.Body.String())
	}

	renewReq := httptest.NewRequest(http.MethodPost, "/api/v1/invite/renew-codes", strings.NewReader(fmt.Sprintf(`{"target_uid":%d,"days":5,"validity_hours":24}`, child.UID)))
	renewReq = renewReq.WithContext(context.WithValue(renewReq.Context(), principalKey, principal{User: parent}))
	renewRR := httptest.NewRecorder()
	app.handleCreateInviteCode(renewRR, renewReq, nil)
	if renewRR.Code != http.StatusCreated {
		t.Fatalf("disabled invite system should still allow child renew code, status=%d body=%s", renewRR.Code, renewRR.Body.String())
	}
}

func TestPublicRegcodeCheckHidesTargetedCodes(t *testing.T) {
	app := newTestApp(t)
	if err := app.store().UpsertRegCode(store.RegCode{Code: "TARGET-SECRET", Type: 2, Days: 5, ValidityTime: -1, UseCountLimit: 1, Active: true, TargetUsername: "alpha"}); err != nil {
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
	parent, err := app.store().CreateUser(store.User{Username: "parent", Role: store.RoleNormal, Active: true, EmbyID: "emby-parent", ExpiredAt: parentExpiry})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.store().CreateUser(store.User{Username: "child", Role: store.RoleNormal, Active: true, ExpiredAt: now.AddDate(0, 0, 60).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "INV-BOUNDS", UID: parent.UID, InviterUID: parent.UID, Days: 30, UseCountLimit: 1, Active: true, CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invite/use", strings.NewReader(`{"code":"INV-BOUNDS"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: child}))
	rr := httptest.NewRecorder()
	app.handleInviteUse(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("invite use status=%d body=%s", rr.Code, rr.Body.String())
	}
	updated, _ := app.store().User(child.UID)
	if updated.ExpiredAt > parentExpiry {
		t.Fatalf("child expiry exceeded parent expiry: child=%d parent=%d", updated.ExpiredAt, parentExpiry)
	}
	if updated.PendingEmbyDays == nil || *updated.PendingEmbyDays > 5 {
		t.Fatalf("pending emby days were not clamped to inviter expiry: %#v", updated.PendingEmbyDays)
	}

	otherParent, err := app.store().CreateUser(store.User{Username: "other-parent", Role: store.RoleNormal, Active: true, EmbyID: "emby-other", ExpiredAt: now.AddDate(0, 0, 30).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "INV-SECOND", UID: otherParent.UID, InviterUID: otherParent.UID, Days: 7, UseCountLimit: 1, Active: true, CreatedAt: now.Unix()}); err != nil {
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
	expiredParent, err := app.store().CreateUser(store.User{Username: "expired-parent", Role: store.RoleNormal, Active: true, EmbyID: "emby-expired", ExpiredAt: now.AddDate(0, 0, -1).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := app.store().CreateUser(store.User{Username: "candidate", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "INV-EXPIRED-PARENT", UID: expiredParent.UID, InviterUID: expiredParent.UID, Days: 30, UseCountLimit: 1, Active: true, CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invite/use", strings.NewReader(`{"code":"INV-EXPIRED-PARENT"}`))
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal{User: candidate}))
	rr := httptest.NewRecorder()
	app.handleInviteUse(rr, req, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expired inviter should be rejected, status=%d body=%s", rr.Code, rr.Body.String())
	}

	app.cfg().InviteRootUserLimit = 1
	root, err := app.store().CreateUser(store.User{Username: "root", Role: store.RoleNormal, Active: true, EmbyID: "emby-root", ExpiredAt: now.AddDate(0, 0, 30).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.store().CreateUser(store.User{Username: "child2", Role: store.RoleNormal, Active: true, EmbyID: "emby-child2", ExpiredAt: now.AddDate(0, 0, 20).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "INV-ROOT-CHILD", UID: root.UID, InviterUID: root.UID, Days: 10, UseCountLimit: 1, Active: true, CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().ConsumeInviteCode("INV-ROOT-CHILD", child.UID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := app.canInvite(child); ok {
		t.Fatal("child should not be allowed to create more invites after root tree reaches limit")
	}
	grandchild, err := app.store().CreateUser(store.User{Username: "grandchild", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "INV-GRANDCHILD", UID: child.UID, InviterUID: child.UID, Days: 10, UseCountLimit: 1, Active: true, CreatedAt: now.Unix()}); err != nil {
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
	app.cfg().InviteDefaultDays = 12
	target, err := app.store().CreateUser(store.User{Username: "tg-target", Role: store.RoleUnrecognized, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	panel := telegramPanelContext{Token: "grant", TargetUID: target.UID, ExpiresAt: time.Now().Add(time.Minute).Unix()}
	app.telegramSavePanel(panel)
	app.telegramApplyPanelAction(context.Background(), panel, "grant_register")
	updated, _ := app.store().User(target.UID)
	if !updated.PendingEmby || updated.PendingEmbyDays == nil || *updated.PendingEmbyDays != 12 || updated.Role != store.RoleNormal {
		t.Fatalf("grant_register did not create pending entitlement: %#v days=%#v", updated, updated.PendingEmbyDays)
	}

	admin, err := app.store().CreateUser(store.User{Username: "protected-admin", Role: store.RoleAdmin, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	adminPanel := telegramPanelContext{Token: "admin-grant", TargetUID: admin.UID, ExpiresAt: time.Now().Add(time.Minute).Unix()}
	app.telegramSavePanel(adminPanel)
	app.telegramApplyPanelAction(context.Background(), adminPanel, "grant_register")
	protected, _ := app.store().User(admin.UID)
	if protected.PendingEmby {
		t.Fatal("grant_register should not mutate protected admin accounts")
	}
}

func TestTelegramRosterStatsUsesObservedMembers(t *testing.T) {
	app := newTestApp(t)
	app.cfg().TelegramGroupIDs = []string{"-1001"}
	if err := app.store().UpsertTelegramRoster("-1001", 100, "member", false); err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertTelegramRoster("-1001", 200, "member", false); err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertTelegramRoster("-1001", 300, "member", true); err != nil {
		t.Fatal(err)
	}
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	admin, _ := app.store().FindUserByUsername("admin")
	_, _ = app.store().UpdateUser(admin.UID, func(u *store.User) error { u.TelegramID = 100; return nil })
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	resp := doJSONWithHeaders(app, http.MethodGet, "/api/v1/admin/telegram/roster/stats", ``, []*http.Cookie{cookie}, nil)
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"bound":1`) || !strings.Contains(resp.Body.String(), `"unbound":1`) || !strings.Contains(resp.Body.String(), `"bots":1`) {
		t.Fatalf("roster stats status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestUserStatsEndpointRejectsOtherUser(t *testing.T) {
	app := newTestApp(t)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"root","password":"Root123456"}`, nil)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"alpha","password":"Alpha123456"}`, nil)
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"beta","password":"Beta123456"}`, nil)
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"alpha","password":"Alpha123456"}`, nil)
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	beta, ok := app.store().FindUserByUsername("beta")
	if !ok {
		t.Fatal("beta user not found")
	}

	resp := doJSON(app, http.MethodGet, "/api/v1/stats/user/"+strconv.FormatInt(beta.UID, 10), ``, []*http.Cookie{cookie})
	if resp.Code != http.StatusForbidden || !strings.Contains(resp.Body.String(), `"error_code":"WATCH_STATS_FORBIDDEN"`) {
		t.Fatalf("expected forbidden stats response, status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestBatchUserDangerousActionsRequireConfirm(t *testing.T) {
	app := newTestApp(t)
	cookies := registerAndLogin(t, app, "admin", "Admin123456")
	headers := map[string]string{"X-Twilight-Client": "webui"}

	tests := []struct {
		name string
		path string
		body string
		want string
	}{
		{"enable", "/api/v1/batch/users/enable", `{"uids":[1]}`, confirmBatchEnableUsers},
		{"disable", "/api/v1/batch/users/disable", `{"uids":[1]}`, confirmBatchDisableUsers},
		{"delete", "/api/v1/batch/users/delete", `{"uids":[1]}`, confirmBatchDeleteUsers},
		{"renew", "/api/v1/batch/users/renew", `{"uids":[1],"days":30}`, confirmBatchRenewUsers},
		{"library self service", "/api/v1/batch/users/library-self-service", `{"uids":[1],"enabled":true}`, confirmBatchLibrarySelfService},
		{"libraries", "/api/v1/batch/users/libraries", `{"uids":[1],"action":"set","library_ids":[]}`, confirmBatchUserLibraries},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := doJSONWithHeaders(app, http.MethodPost, tt.path, tt.body, cookies, headers)
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
			}
			body := resp.Body.String()
			if !strings.Contains(body, tt.want) || !strings.Contains(body, `"error_code":"BATCH_CONFIRM_REQUIRED"`) {
				t.Fatalf("missing confirm/error contract in body=%s", body)
			}
		})
	}

	resp := doJSONWithHeaders(app, http.MethodPost, "/api/v1/batch/users/enable", fmt.Sprintf(`{"confirm":%q}`, confirmBatchEnableUsers), cookies, headers)
	if resp.Code != http.StatusBadRequest || !strings.Contains(resp.Body.String(), "uids required") || !strings.Contains(resp.Body.String(), `"error_code":"BATCH_UIDS_REQUIRED"`) {
		t.Fatalf("missing uids contract, status=%d body=%s", resp.Code, resp.Body.String())
	}

	tooLarge := doJSONWithHeaders(app, http.MethodPost, "/api/v1/batch/users/renew", fmt.Sprintf(`{"confirm":%q,"uids":[1],"days":36501}`, confirmBatchRenewUsers), cookies, headers)
	if tooLarge.Code != http.StatusBadRequest || !strings.Contains(tooLarge.Body.String(), `"error_code":"BATCH_DAYS_INVALID"`) {
		t.Fatalf("batch renew days cap status=%d body=%s", tooLarge.Code, tooLarge.Body.String())
	}
}

func TestOtherDangerousActionsRequireConfirm(t *testing.T) {
	app := newTestApp(t)
	cookies := registerAndLogin(t, app, "admin", "Admin123456")
	headers := map[string]string{"X-Twilight-Client": "webui"}

	resp := doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/violations/clear", `{}`, cookies, headers)
	if resp.Code != http.StatusBadRequest || !strings.Contains(resp.Body.String(), confirmClearViolations) || !strings.Contains(resp.Body.String(), `"error_code":"VIOLATION_CONFIRM_REQUIRED"`) {
		t.Fatalf("clear violations missing confirm status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestCleanupInvalidUsersDefaultsToDryRunAndRequiresConfirm(t *testing.T) {
	app := newTestApp(t)
	cookies := registerAndLogin(t, app, "admin", "Admin123456")
	headers := map[string]string{"X-Twilight-Client": "webui"}
	old := time.Now().Add(-10 * 24 * time.Hour).Unix()
	ghost, err := app.store().CreateUser(store.User{Username: "ghost", PasswordHash: "x", Role: store.RoleNormal, Active: true, RegisterTime: old, CreatedAt: old})
	if err != nil {
		t.Fatal(err)
	}

	preview := doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/users/cleanup-invalid", `{}`, cookies, headers)
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), `"dry_run":true`) {
		t.Fatalf("cleanup default dry run status=%d body=%s", preview.Code, preview.Body.String())
	}
	if _, ok := app.store().User(ghost.UID); !ok {
		t.Fatal("cleanup default request deleted user")
	}

	missingConfirm := doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/users/cleanup-invalid", `{"dry_run":false}`, cookies, headers)
	if missingConfirm.Code != http.StatusBadRequest || !strings.Contains(missingConfirm.Body.String(), confirmCleanupInvalidUsers) {
		t.Fatalf("cleanup missing confirm status=%d body=%s", missingConfirm.Code, missingConfirm.Body.String())
	}
	if _, ok := app.store().User(ghost.UID); !ok {
		t.Fatal("cleanup without confirm deleted user")
	}

	confirmed := doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/users/cleanup-invalid", fmt.Sprintf(`{"dry_run":false,"confirm":%q}`, confirmCleanupInvalidUsers), cookies, headers)
	if confirmed.Code != http.StatusOK || !strings.Contains(confirmed.Body.String(), `"dry_run":false`) {
		t.Fatalf("cleanup confirmed status=%d body=%s", confirmed.Code, confirmed.Body.String())
	}
	if _, ok := app.store().User(ghost.UID); ok {
		t.Fatal("confirmed cleanup did not delete user")
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
		if cookie == nil {
			continue
		}
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

// TestDecodeMapEnforcesSizeAndDepthLimits 验证 decodeMap 在 256KB / 32 层
// 边界外都返回空 map，避免攻击者通过超大 payload / 深嵌套 JSON 让单次解码
// 吃光内存或拖慢后续遍历。
func TestDecodeMapEnforcesSizeAndDepthLimits(t *testing.T) {
	t.Run("rejects body over size limit", func(t *testing.T) {
		// 构造略大于 maxJSONBodyBytes 的 payload：用一个长字符串字段顶到上限。
		big := make([]byte, maxJSONBodyBytes+1024)
		for i := range big {
			big[i] = 'a'
		}
		body := []byte(`{"data":"`)
		body = append(body, big...)
		body = append(body, []byte(`"}`)...)
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		got := decodeMap(req)
		if len(got) != 0 {
			t.Fatalf("expected empty map for oversized body, got %v", got)
		}
	})
	t.Run("rejects deeply nested payload", func(t *testing.T) {
		var b strings.Builder
		// 深度 = maxJSONNestingDepth + 5：一定越界。
		depth := maxJSONNestingDepth + 5
		for i := 0; i < depth; i++ {
			b.WriteString(`{"a":`)
		}
		b.WriteString(`1`)
		for i := 0; i < depth; i++ {
			b.WriteString(`}`)
		}
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(b.String()))
		req.Header.Set("Content-Type", "application/json")
		got := decodeMap(req)
		if len(got) != 0 {
			t.Fatalf("expected empty map for over-nested body, got %v", got)
		}
	})
	t.Run("accepts shallow payload within limits", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":{"b":{"c":1}}}`))
		req.Header.Set("Content-Type", "application/json")
		got := decodeMap(req)
		if len(got) == 0 {
			t.Fatalf("expected non-empty map for shallow body")
		}
	})
}

// TestSharedHTTPClientRefusesCrossHostRedirect 锁定 sharedHTTPClient 的
// CheckRedirect 策略：跨主机 302 必须被拒绝，调用方拿到 redirect 错误而非
// 让 token / api_key 等自定义凭据被 Go 默认 follow 行为发到第三方。
func TestSharedHTTPClientRefusesCrossHostRedirect(t *testing.T) {
	// attacker 模拟外部恶意目标：如果 Go 真的 follow，会把 X-Emby-Token 这类
	// 自定义头原样转发过来。我们靠 onAttacker 计数器断言 *没有* 命中。
	var hitAttacker int32
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hitAttacker, 1)
		// 把所有自定义头回传，便于断言泄露。
		for k, v := range r.Header {
			for _, vv := range v {
				w.Header().Add("X-Echo-"+k, vv)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer attacker.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, attacker.URL+"/exfil", http.StatusFound)
	}))
	defer upstream.Close()

	headers := map[string]string{
		"X-Emby-Token":  "super-secret-token",
		"Authorization": "Bearer secret",
	}
	err := getJSON(context.Background(), upstream.URL+"/", headers, nil)
	if err == nil {
		t.Fatalf("expected error from cross-host redirect, got nil")
	}
	// 错误信息中应包含 redirect refused / status 3xx 字样
	msg := err.Error()
	if !strings.Contains(msg, "redirect") && !strings.Contains(msg, "302") && !strings.Contains(msg, "Location") {
		t.Fatalf("expected redirect-refused error, got %q", msg)
	}
	if got := atomic.LoadInt32(&hitAttacker); got != 0 {
		t.Fatalf("attacker host received %d requests, expected 0 (token would have leaked)", got)
	}
}

// TestSharedHTTPClientFollowsSameHostRedirect 锁定同主机 302 仍然能正常 follow，
// 避免 CheckRedirect 把合理的 trailing-slash / index 重定向也禁掉。
func TestSharedHTTPClientFollowsSameHostRedirect(t *testing.T) {
	var hits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusFound)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var dst map[string]any
	if err := getJSON(context.Background(), srv.URL+"/redirect", nil, &dst); err != nil {
		t.Fatalf("same-host redirect failed: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected /final to be hit once, got %d", got)
	}
	if dst["ok"] != true {
		t.Fatalf("expected ok:true, got %v", dst)
	}
}

// TestValidateEmbyURLRejectsUnsafeTargets 锁定 Emby URL 校验：
// 链路本地 / 元数据 IP / 非 http(s) scheme / query+fragment 必须被拒；
// loopback / RFC1918 / 域名 + 公网必须被允许（与同机 / 同 VPC 部署兼容）。
func TestValidateEmbyURLRejectsUnsafeTargets(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"loopback ipv4", "http://127.0.0.1:8096", false},
		{"loopback ipv6", "http://[::1]:8096", false},
		{"private rfc1918", "http://10.0.0.5:8096", false},
		{"public host", "https://emby.example.com", false},
		{"trailing slash trimmed", "http://127.0.0.1:8096/", false},
		{"link-local ipv4", "http://169.254.0.10:8096", true},
		{"aws metadata", "http://169.254.169.254/latest/meta-data/", true},
		{"aliyun metadata", "http://100.100.100.200/", true},
		{"link-local ipv6", "http://[fe80::1]:8096", true},
		{"unspecified ipv4", "http://0.0.0.0:8096", true},
		{"file scheme", "file:///etc/passwd", true},
		{"ftp scheme", "ftp://example.com/", true},
		{"missing host", "http:///path", true},
		{"with query", "http://emby.example.com/?token=xxx", true},
		{"with fragment", "http://emby.example.com/#frag", true},
		{"empty", "", true},
		{"whitespace only", "   ", true},
		{"unparseable", "://", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateEmbyURL(strings.TrimSpace(tc.input))
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("validateEmbyURL(%q) err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

// TestValidateOutboundBaseURLForAllServices 锁定 Bangumi / Telegram / TMDB
// 与 Emby 共享同一套 SSRF 否决：scheme 必须 http(s)、host 不能是 link-local
// /unspecified / 阿里云 magic IP，且 base URL 不允许带 query / fragment。
// 这条测试存在的意义是防止以后有人单独修 Emby 校验时漏改其他三家。
func TestValidateOutboundBaseURLForAllServices(t *testing.T) {
	type tc struct {
		input   string
		wantErr bool
	}
	cases := []tc{
		{"https://api.example.com", false},
		{"http://10.0.0.5", false},
		{"http://127.0.0.1:8080", false},
		{"http://169.254.169.254", true},
		{"http://100.100.100.200", true},
		{"http://[fe80::1]", true},
		{"http://0.0.0.0", true},
		{"javascript:alert(1)", true},
		{"http:///path", true},
		{"http://api.example.com/?k=v", true},
		{"http://api.example.com/#x", true},
	}
	for _, svc := range []string{"Bangumi", "Telegram", "TMDB", "Emby"} {
		for _, c := range cases {
			c := c
			t.Run(svc+"_"+c.input, func(t *testing.T) {
				_, err := validateOutboundBaseURL(c.input, svc)
				gotErr := err != nil
				if gotErr != c.wantErr {
					t.Fatalf("validateOutboundBaseURL(%q,%s) err=%v wantErr=%v", c.input, svc, err, c.wantErr)
				}
				if err != nil && !strings.Contains(err.Error(), svc) {
					t.Fatalf("error message must surface service name %q, got %q", svc, err.Error())
				}
			})
		}
	}
}

// TestRenewReactivatesDisabledNonInvitedUser 锁定 R62-2 不变量：
// check_expired 把"非邀请"过期账号设成 Active=false 后，任何续费路径都
// 必须把 Active 同步复位到 true，否则用户续完照样登不上，admin 续完看到
// "成功 200"实际上还得手动 enable 一遍——这是新人最容易踩的坑。
//
// 用 admin renew 路径覆盖这一点（self renew 需要消耗注册码、设置成本高，
// admin renew 路径与 self/batch/regcode 共用同一个 renewExpiryAndReactivate
// 助手，覆盖一条等价于覆盖四条）。
func TestRenewReactivatesDisabledNonInvitedUser(t *testing.T) {
	app := newTestApp(t)

	// 第一个注册的用户自动晋升 admin（newTestApp 行为）。
	_ = doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	adminLogin := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
	if adminLogin.Code != http.StatusOK {
		t.Fatalf("admin login: %d %s", adminLogin.Code, adminLogin.Body.String())
	}
	adminCookies := adminLogin.Result().Cookies()

	// 构造一个已过期非邀请普通用户，模拟 check_expired 走完后的终态。
	target, err := app.store().CreateUser(store.User{
		Username:  "expired-normal",
		Role:      store.RoleNormal,
		Active:    false,
		ExpiredAt: time.Now().Add(-24 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 触发 admin renew，days=30。修复前只 bump ExpiredAt，Active 仍是 false。
	url := fmt.Sprintf("/api/v1/admin/users/%d/renew", target.UID)
	resp := doJSONWithHeaders(app, http.MethodPost, url, `{"days":30}`, adminCookies, map[string]string{"X-Twilight-Client": "webui"})
	if resp.Code != http.StatusOK {
		t.Fatalf("admin renew: %d %s", resp.Code, resp.Body.String())
	}

	updated, ok := app.store().User(target.UID)
	if !ok {
		t.Fatalf("user disappeared after renew")
	}
	if !updated.Active {
		t.Fatalf("R62-2 regression: renew did not reactivate disabled user (Active=%v)", updated.Active)
	}
	if updated.ExpiredAt <= time.Now().Unix() {
		t.Fatalf("renew should advance ExpiredAt past now, got %d", updated.ExpiredAt)
	}
}

// TestCheckExpiredSkipsAdminAndWhitelist 锁定 R62-3 不变量：
// check_expired 调度看到 RoleAdmin / RoleWhitelist 用户即便 ExpiredAt 撞过
// 期限也必须跳过——demote-then-repromote / 手动 SQL / 旧迁移都可能让 admin
// 留下 finite ExpiredAt，一旦命中 check_expired 把 admin 自禁后 panel 就再
// 也登不上。同时锁定 schedulerSummary 中暴露 skipped_protected 计数。
func TestCheckExpiredSkipsAdminAndWhitelist(t *testing.T) {
	app := newTestApp(t)
	app.cfg().AdminUsernames = []string{"exp-config-admin"}

	expiredUnix := time.Now().Add(-2 * time.Hour).Unix()
	admin, err := app.store().CreateUser(store.User{Username: "exp-admin", Role: store.RoleAdmin, Active: true, ExpiredAt: expiredUnix})
	if err != nil {
		t.Fatal(err)
	}
	whitelist, err := app.store().CreateUser(store.User{Username: "exp-wl", Role: store.RoleWhitelist, Active: true, ExpiredAt: expiredUnix})
	if err != nil {
		t.Fatal(err)
	}
	configuredAdmin, err := app.store().CreateUser(store.User{Username: "exp-config-admin", Role: store.RoleNormal, Active: true, ExpiredAt: expiredUnix})
	if err != nil {
		t.Fatal(err)
	}
	normal, err := app.store().CreateUser(store.User{Username: "exp-normal", Role: store.RoleNormal, Active: true, ExpiredAt: expiredUnix})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/scheduler/internal", nil)
	summary, _, err := app.runSchedulerJob(req, "check_expired")
	if err != nil {
		t.Fatalf("runSchedulerJob check_expired: %v", err)
	}

	// 普通用户：仍走原路径被禁。
	if u, _ := app.store().User(normal.UID); u.Active {
		t.Fatalf("non-admin expired user should be disabled, got Active=true")
	}

	// admin / whitelist：必须保留 Active=true。
	if u, _ := app.store().User(admin.UID); !u.Active {
		t.Fatalf("R62-3 regression: admin auto-disabled by check_expired")
	}
	if u, _ := app.store().User(whitelist.UID); !u.Active {
		t.Fatalf("R62-3 regression: whitelist user auto-disabled by check_expired")
	}
	if u, _ := app.store().User(configuredAdmin.UID); !u.Active {
		t.Fatalf("configured admin auto-disabled by check_expired")
	}

	// summary 必须暴露 skipped_protected 计数（admin + whitelist + configured admin = 3）。
	got := int(numeric(summary["skipped_protected"]))
	if got != 3 {
		t.Fatalf("skipped_protected=%d in summary, want 3; summary=%v", got, summary)
	}
}

// TestLoginByAPIKeyRejectsDisabledAccount 锁定与 handleLogin 同一不变量：
// 禁用账号无法用 API Key 重新拿到 session。修复前 /auth/login/apikey 只查
// API Key 命中即建会话，admin 把 Active=false 后这条路径仍可继续访问。
func TestLoginByAPIKeyRejectsDisabledAccount(t *testing.T) {
	app := newTestApp(t)
	cookies := registerAndLogin(t, app, "carol", "Password123456")
	rec := doJSONWithHeaders(app, http.MethodPost, "/api/v1/users/me/apikeys", `{"name":"k","rate_limit":50}`, cookies, map[string]string{"X-Twilight-Client": "webui"})
	if rec.Code != http.StatusOK {
		t.Fatalf("create key status=%d body=%s", rec.Code, rec.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode key: %v", err)
	}
	key, _ := env.Data.(map[string]any)["key"].(string)
	if !strings.HasPrefix(key, "key-") {
		t.Fatalf("expected plaintext key, got %q", key)
	}
	user, ok := app.store().FindUserByUsername("carol")
	if !ok {
		t.Fatalf("user disappeared")
	}
	if _, err := app.store().UpdateUser(user.UID, func(u *store.User) error { u.Active = false; return nil }); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"apikey": key})
	resp := doJSON(app, http.MethodPost, "/api/v1/auth/login/apikey", string(body), nil)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("login/apikey on disabled account: status=%d body=%s, want 403", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), string(ErrAccountDisabled)) {
		t.Fatalf("expected ErrAccountDisabled in body, got %s", resp.Body.String())
	}
}

// TestLoginDistinguishesExpiredFromDisabled 锁定 R62-6 不变量：
// "管理员手动禁用"和"check_expired 触发的到期"两种 Active=false 状态在
// /api/v1/auth/login 必须返回不同的 error_code，让 webui 的 CTA 分别走
// /support 和 /renew。这条测试比单纯 helper 单测更接近 user-visible 契约：
// 任何把 ErrAccountExpired 又静默融回 ErrAccountDisabled 的回归都会被它打断。
func TestLoginDistinguishesExpiredFromDisabled(t *testing.T) {
	app := newTestApp(t)
	hash, err := security.HashPassword("Password123456")
	if err != nil {
		t.Fatal(err)
	}
	// 创建一个占位 admin，让后续两个测试用户不会被 bootstrap 路径强行
	// 提升为 admin + Active=true（CreateUser 在 UserCount==0 时会无视
	// 入参 Active 把首注册用户兜成 admin 启用态，绕开本测试要验证的禁用
	// /到期分支）。
	if _, err := app.store().CreateUser(store.User{Username: "boot-admin", Role: store.RoleAdmin, PasswordHash: hash, Active: true}); err != nil {
		t.Fatal(err)
	}
	disabled, err := app.store().CreateUser(store.User{
		Username:     "manually-disabled",
		Role:         store.RoleNormal,
		PasswordHash: hash,
		Active:       true,
		ExpiredAt:    time.Now().Add(24 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().UpdateUser(disabled.UID, func(u *store.User) error { u.Active = false; return nil }); err != nil {
		t.Fatal(err)
	}
	expired, err := app.store().CreateUser(store.User{
		Username:     "auto-expired",
		Role:         store.RoleNormal,
		PasswordHash: hash,
		Active:       true,
		ExpiredAt:    time.Now().Add(-1 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().UpdateUser(expired.UID, func(u *store.User) error { u.Active = false; return nil }); err != nil {
		t.Fatal(err)
	}

	check := func(username, want string) {
		body := fmt.Sprintf(`{"username":%q,"password":"Password123456"}`, username)
		resp := doJSON(app, http.MethodPost, "/api/v1/auth/login", body, nil)
		if resp.Code != http.StatusForbidden {
			t.Fatalf("login %s: status=%d body=%s, want 403", username, resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), want) {
			t.Fatalf("login %s: expected %s in body, got %s", username, want, resp.Body.String())
		}
	}
	check("manually-disabled", string(ErrAccountDisabled))
	check("auto-expired", string(ErrAccountExpired))
}

// TestUserEntitlementOKExpiredBlocksInviteMint 锁定 R62-4 不变量：
// 一个 Active=true 但 ExpiredAt < now 的用户不允许通过 invite_create / renew
// code 路径继续 mint entitlement。这是新人最容易踩的一类逻辑：
// "Active 字段为真就以为账号还能用任何接口"——R62-4 的本质是把 Active 与
// ExpiredAt 这对 invariants 在 entitlement 决策点合并表达。
//
// 直接覆盖 userEntitlementOK helper（核心不变量），加一条 canInvite 验证
// 提前给出可读 reason 文案。canInvite 已经隐式靠 maxCodeDays 挡住，但显
// 式 gate 是防御纵深。
func TestUserEntitlementOKExpiredBlocksInviteMint(t *testing.T) {
	now := time.Now().Unix()
	cases := []struct {
		name string
		user store.User
		want bool
	}{
		{"active+future_expiry", store.User{Active: true, ExpiredAt: now + 86400}, true},
		{"active+no_expiry", store.User{Active: true, ExpiredAt: 0}, true},
		{"active+permanent_expiry", store.User{Active: true, ExpiredAt: 253402214400}, true},
		{"active+past_expiry_blocks", store.User{Active: true, ExpiredAt: now - 1}, false},
		{"inactive+future_expiry_blocks", store.User{Active: false, ExpiredAt: now + 86400}, false},
		{"inactive+no_expiry_blocks", store.User{Active: false, ExpiredAt: 0}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := userEntitlementOK(tc.user); got != tc.want {
				t.Fatalf("userEntitlementOK(%+v) = %v, want %v", tc.user, got, tc.want)
			}
		})
	}

	// 端到端：构造一个"Active=true 但已过期"的非邀请用户，调用 canInvite 必须
	// 返回 (false, 含"到期"信号的 reason)。这一条防的是未来重构 maxCodeDays
	// 改返回路径时静默放过 entitlement 的回归。
	app := newTestApp(t)
	app.cfg().InviteEnabled = true
	user, err := app.store().CreateUser(store.User{
		Username:  "active-but-expired",
		Role:      store.RoleNormal,
		Active:    true,
		ExpiredAt: time.Now().Add(-1 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	can, reason := app.canInvite(user)
	if can {
		t.Fatalf("R62-4 regression: canInvite returned true for Active=true & expired user")
	}
	if !strings.Contains(reason, "到期") {
		t.Fatalf("expected reason to mention expiry, got %q", reason)
	}
}

// TestForgotPasswordRejectsExpiredAccount 锁定 R62-7 不变量：
// emby 鉴权通过但面板侧账号 entitlement 已过期（Active=true & ExpiredAt<now）
// 时不应进入密码重置 + emby 写新密码流程。否则
//  1. embySetUserEnabled 在过期态会立即把账号 disable，用户拿到的"new
//     _password"登录就失败；
//  2. 攻击者拿到 emby 密码可以反复 mint 面板凭据，把已经被运维软冻结的账号
//     当成绕开续费的入口。
func TestForgotPasswordRejectsExpiredAccount(t *testing.T) {
	app := newTestApp(t)
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/Users/AuthenticateByName" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"User":{"Id":"emby-stale","Name":"stale"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	// Active=true 但 ExpiredAt < now：典型"软冻结但还没被 check_expired 跑过"
	// 状态，或邀请用户过期后保留 Active=true 的设计（参见 scheduler_runner.go
	// 的 invited 分支）。
	user, err := app.store().CreateUser(store.User{
		Username:  "stale",
		Role:      store.RoleNormal,
		Active:    true,
		EmbyID:    "emby-stale",
		ExpiredAt: time.Now().Add(-1 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = user

	body := `{"emby_username":"stale","emby_password":"any-password"}`
	resp := doJSON(app, http.MethodPost, "/api/v1/auth/forgot-password/emby", body, nil)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("forgot-password on expired account: status=%d body=%s, want 403", resp.Code, resp.Body.String())
	}
	// R62-6：到期路径必须返回 ErrAccountExpired，让 webui 把"续费"和
	// "申诉"两条 CTA 区分开。如果未来又把它静默换回 ErrAccountDisabled，
	// 用户会被错误引导到联系管理员而不是续费页。
	if !strings.Contains(resp.Body.String(), string(ErrAccountExpired)) {
		t.Fatalf("expected ErrAccountExpired in body, got %s", resp.Body.String())
	}
	// 确认密码哈希没有被改写：若旧实现漏掉 expired guard，UpdateUser 写哈希后
	// PasswordHash 会变成非空。
	updated, _ := app.store().User(user.UID)
	if updated.PasswordHash != user.PasswordHash {
		t.Fatalf("R62-7 regression: forgot-password modified password despite expired entitlement")
	}
}

// TestSessionCookieRespectsConfiguredDomain 锁住 R62-cookie-domain 修复：
// 双子域部署（webui = twilight.example.com / API = twilightapi.example.com）
// 必须把 session cookie 显式打上注册域 ".example.com"，否则浏览器把
// cookie 锁在 API 子域，webui 这边的 Next middleware 读不到，已登录用户访
// 问 /dashboard 直接被 302 回 /login —— 用户视角即"登录成功但不跳转面板"。
//
// 这里走 runtime.Store 直接替换 cfg，是因为 New() 之后 cfg 副本存在
// runtimeState 里，单纯改 a.cfg() 返回的指针字段会被下次 reload 覆盖。
//
// 关于 ".example.com" vs "example.com"：RFC 6265 已把前缀点视为兼容形式，
// net/http 的 cookie 解析器会把它剥掉再回填到 cookie.Domain；浏览器实际收到
// 的 Set-Cookie 文本里仍是 Domain=example.com，对子域共享语义没影响。这里
// 测试比对的是 Go 解析后的形态，所以用 "example.com"（无前缀点）。
func TestSessionCookieRespectsConfiguredDomain(t *testing.T) {
	app := newTestApp(t)
	rt := app.runtime.Load()
	next := *rt
	next.cfg.CookieDomain = ".example.com"
	app.runtime.Store(&next)

	const wantDomain = "example.com"

	register := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	if register.Code != http.StatusCreated {
		t.Fatalf("register status = %d body=%s", register.Code, register.Body.String())
	}
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", login.Code, login.Body.String())
	}
	session := findCookie(login.Result().Cookies(), "twilight_session")
	if session == nil {
		t.Fatal("missing session cookie")
	}
	if session.Domain != wantDomain {
		t.Fatalf("session cookie Domain = %q, want %q", session.Domain, wantDomain)
	}

	// 登出时 expire cookie 也必须复用同一 Domain，否则浏览器会按 default-domain
	// (= 设置时的请求 host) 寻找另一份同名 cookie，删除失败 → "登出后再访问还是
	// 登录态"幽灵 cookie。
	logout := doJSON(app, http.MethodPost, "/api/v1/auth/logout", "", []*http.Cookie{session})
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status = %d body=%s", logout.Code, logout.Body.String())
	}
	for _, c := range logout.Result().Cookies() {
		if c.Name != "twilight_session" {
			continue
		}
		if c.Domain != wantDomain {
			t.Fatalf("logout %q cookie Domain = %q, want %q", c.Name, c.Domain, wantDomain)
		}
		if c.MaxAge >= 0 {
			t.Fatalf("logout %q cookie MaxAge = %d, want negative (immediate expiry)", c.Name, c.MaxAge)
		}
	}
}

// TestSessionCookieOmitsDomainWhenUnset 守住单 origin 部署的旧行为：未配置
// CookieDomain 时 Set-Cookie 不应写出 Domain= 属性，让浏览器把 cookie 锁回
// 设置时的精确 host —— 这是单 origin 部署里更窄的暴露面。
func TestSessionCookieOmitsDomainWhenUnset(t *testing.T) {
	app := newTestApp(t)
	register := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil)
	if register.Code != http.StatusCreated {
		t.Fatalf("register status = %d body=%s", register.Code, register.Body.String())
	}
	login := doJSON(app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"Admin123456"}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", login.Code, login.Body.String())
	}
	session := findCookie(login.Result().Cookies(), "twilight_session")
	if session == nil {
		t.Fatal("missing session cookie on default config")
	}
	if session.Domain != "" {
		t.Fatalf("session cookie Domain = %q, want empty (host-only)", session.Domain)
	}
}

// TestBatchUserOutcomesCarryStableErrorCodes 锁住 R64-8 的契约：批量响应里
// 每条失败 outcome 都附带 errors[].code（一个稳定的 ErrCode 字符串），前端
// 用它做 UI 分支（"用户不存在" / "受保护账号" / "命中自己" / "未绑定 Emby"），
// 而不是去 grep errors[].error 里漂浮的中文文案 / 拼接后的 fmt.Errorf 字符串。
//
// 这里覆盖的 4 条失败路径恰好是 batch_user_handlers.go 之前裸 fmt.Errorf 的
// 全部位置：
//   - delete 命中 self → BATCH_SELF_TARGET
//   - disable 命中不存在的 uid → USER_NOT_FOUND
//   - libraries 命中无 emby 的用户 → USER_NO_EMBY
//   - delete 命中 protected admin → USER_PROTECTED
func TestBatchUserOutcomesCarryStableErrorCodes(t *testing.T) {
	app := newTestApp(t)
	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")
	headers := map[string]string{"X-Twilight-Client": "webui"}

	// admin 的 UID = 1。再造一个普通 user 留给后面的 disable 用，但故意不
	// 把它放进请求里——下面 disable 走的是不存在的 uid=9999 路径。
	_, err := app.store().CreateUser(store.User{Username: "ghost", PasswordHash: "x", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}

	type errEntry struct {
		UID  int64  `json:"uid"`
		Code string `json:"code"`
	}
	type batchResp struct {
		Data struct {
			Errors []errEntry `json:"errors"`
		} `json:"data"`
	}

	parse := func(t *testing.T, body string) []errEntry {
		t.Helper()
		var br batchResp
		if err := json.Unmarshal([]byte(body), &br); err != nil {
			t.Fatalf("decode batch resp: %v body=%s", err, body)
		}
		return br.Data.Errors
	}

	// 1) delete 命中 self（UID 1） → BATCH_SELF_TARGET
	delBody := fmt.Sprintf(`{"confirm":%q,"uids":[1]}`, confirmBatchDeleteUsers)
	resp := doJSONWithHeaders(app, http.MethodPost, "/api/v1/batch/users/delete", delBody, adminCookies, headers)
	if resp.Code != http.StatusOK {
		t.Fatalf("delete self batch status=%d body=%s", resp.Code, resp.Body.String())
	}
	errs := parse(t, resp.Body.String())
	if len(errs) != 1 || errs[0].UID != 1 || errs[0].Code != string(ErrBatchSelfTarget) {
		t.Fatalf("expected BATCH_SELF_TARGET on uid=1, got %+v", errs)
	}

	// 2) disable 命中不存在 uid=9999 → USER_NOT_FOUND
	disBody := fmt.Sprintf(`{"confirm":%q,"uids":[9999]}`, confirmBatchDisableUsers)
	resp = doJSONWithHeaders(app, http.MethodPost, "/api/v1/batch/users/disable", disBody, adminCookies, headers)
	if resp.Code != http.StatusOK {
		t.Fatalf("disable missing batch status=%d body=%s", resp.Code, resp.Body.String())
	}
	errs = parse(t, resp.Body.String())
	if len(errs) != 1 || errs[0].UID != 9999 || errs[0].Code != string(ErrUserNotFound) {
		t.Fatalf("expected USER_NOT_FOUND on uid=9999, got %+v", errs)
	}

	// 3) libraries 命中 ghost（UID 2，无 emby）→ USER_NO_EMBY
	libBody := fmt.Sprintf(`{"confirm":%q,"uids":[2],"action":"set","library_ids":[]}`, confirmBatchUserLibraries)
	resp = doJSONWithHeaders(app, http.MethodPost, "/api/v1/batch/users/libraries", libBody, adminCookies, headers)
	if resp.Code != http.StatusOK {
		t.Fatalf("libraries no-emby batch status=%d body=%s", resp.Code, resp.Body.String())
	}
	errs = parse(t, resp.Body.String())
	if len(errs) != 1 || errs[0].UID != 2 || errs[0].Code != string(ErrUserHasNoEmby) {
		t.Fatalf("expected USER_NO_EMBY on uid=2, got %+v", errs)
	}
}

// TestUserNotFoundResponsesAreUniform 锁住 R64-6 / R64-7 的不变量：所有结构上
// "按 uid 加载用户失败"的端点必须返回同一个 (HTTP 404, code=USER_NOT_FOUND,
// message="用户不存在") 三元组。原先后端在 9 个 call site 内分裂出 3 种 copy
// （"user not found" / "用户不存在" / "目标用户不存在"），其中 invite renew
// code 还把 not-found 错挂到了 INVITE_RENEW_TARGET_MISSING 上——前端要写两份
// 一模一样的"用户不存在"分支才能覆盖同一种语义。这个回归测试覆盖三条不同
// path（admin GET / admin reset password / kick session）保证未来谁要把消息
// 从 helper 里抽出来 inline 都会立刻炸掉。
func TestUserNotFoundResponsesAreUniform(t *testing.T) {
	app := newTestApp(t)
	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")
	headers := map[string]string{"X-Twilight-Client": "webui"}

	type errResp struct {
		Code    string `json:"error_code"`
		Message string `json:"message"`
	}
	expect := func(t *testing.T, label string, resp *httptest.ResponseRecorder) {
		t.Helper()
		if resp.Code != http.StatusNotFound {
			t.Fatalf("%s: status=%d body=%s", label, resp.Code, resp.Body.String())
		}
		var e errResp
		if err := json.Unmarshal(resp.Body.Bytes(), &e); err != nil {
			t.Fatalf("%s: decode: %v body=%s", label, err, resp.Body.String())
		}
		if e.Code != string(ErrUserNotFound) {
			t.Fatalf("%s: code=%q want USER_NOT_FOUND", label, e.Code)
		}
		if e.Message != "用户不存在" {
			t.Fatalf("%s: message=%q want %q", label, e.Message, "用户不存在")
		}
	}

	// admin user GET 走 handleAdminUser → userFromPath helper
	expect(t, "GET admin user", doJSONWithHeaders(app, http.MethodGet, "/api/v1/admin/users/9999", "", adminCookies, headers))
	// admin password reset 走 handleAdminResetPassword → 直接 store.User 查询
	expect(t, "POST admin password reset", doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/users/9999/reset-password", `{"scope":"system"}`, adminCookies, headers))
	// kick session 走 handleKickUser → userFromPath helper
	expect(t, "POST admin kick", doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/users/9999/kick", "", adminCookies, headers))
}
