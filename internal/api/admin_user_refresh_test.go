package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

// TestAdminRefreshUserStatusSyncsTelegramAndEmby 验证强制刷新接口会：
//   - 用 getChat 拉取并回写最新的 Telegram 用户名；
//   - 对「Web 已禁用但 Emby 仍启用」的越权漂移强制关停远端 Emby；
//   - 在响应 refresh 摘要里如实反映这两处同步。
func TestAdminRefreshUserStatusSyncsTelegramAndEmby(t *testing.T) {
	app := newTestApp(t)
	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")

	user, err := app.store().CreateUser(store.User{
		Username:         "target",
		Role:             store.RoleNormal,
		Active:           false, // 已禁用 → Emby 应被强制关停
		TelegramID:       7777,
		TelegramUsername: "old",
		EmbyID:           "emby-rf",
		EmbyUsername:     "embyrf",
	})
	if err != nil {
		t.Fatal(err)
	}
	// CreateUser 的 ensure() 会把 Active 兜底成 true、ExpiredAt 兜成永久，这里显式
	// 把账号改成「已禁用」才能验证刷新会强制关停 Emby。
	if _, err := app.store().UpdateUser(user.UID, func(u *store.User) error { u.Active = false; return nil }); err != nil {
		t.Fatal(err)
	}

	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123:ABC/getChat" {
			t.Fatalf("unexpected telegram path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":7777,"type":"private","username":"newname"}}`))
	}))
	defer tg.Close()
	app.cfg().TelegramAPIURL = tg.URL

	embyDisabled := false
	app.cfg().EmbyToken = "emby-token"
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users/emby-rf":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Id":"emby-rf","Name":"embyrf","Policy":{"IsDisabled":false}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/Users/emby-rf/Policy":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["IsDisabled"] == true {
				embyDisabled = true
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected Emby request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	headers := map[string]string{"X-Twilight-Client": "webui"}
	path := fmt.Sprintf("/api/v1/admin/users/%d/refresh-status", user.UID)
	resp := doJSONWithHeaders(app, http.MethodPost, path, "", adminCookies, headers)
	if resp.Code != http.StatusOK {
		t.Fatalf("refresh-status status=%d body=%s", resp.Code, resp.Body.String())
	}

	updated, _ := app.store().User(user.UID)
	if updated.TelegramUsername != "newname" {
		t.Fatalf("telegram username = %q, want newname", updated.TelegramUsername)
	}
	if !embyDisabled {
		t.Fatal("expected remote Emby account to be disabled for inactive user")
	}

	var env struct {
		Data struct {
			Refresh struct {
				TelegramUpdated bool `json:"telegram_username_updated"`
				EmbyDisabled    bool `json:"emby_disabled_synced"`
			} `json:"refresh"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.Data.Refresh.TelegramUpdated {
		t.Fatal("refresh.telegram_username_updated should be true")
	}
	if !env.Data.Refresh.EmbyDisabled {
		t.Fatal("refresh.emby_disabled_synced should be true")
	}
}

// TestAdminBulkExpireDisablesEmby 验证批量改到期时间到「过去」时，被影响用户的 Emby
// 账号会被同步关停——堵住「Web 已过期、Emby 仍可登录」的越权窗口。
func TestAdminBulkExpireDisablesEmby(t *testing.T) {
	app := newTestApp(t)
	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")

	user, err := app.store().CreateUser(store.User{
		Username:     "expireme",
		Role:         store.RoleNormal,
		Active:       true,
		ExpiredAt:    permanentExpiryUnix, // 先永久，bulk-expire 改成过去
		EmbyID:       "emby-be",
		EmbyUsername: "embybe",
	})
	if err != nil {
		t.Fatal(err)
	}

	embyDisabled := false
	app.cfg().EmbyToken = "emby-token"
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users/emby-be":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Id":"emby-be","Name":"embybe","Policy":{"IsDisabled":false}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/Users/emby-be/Policy":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["IsDisabled"] == true {
				embyDisabled = true
			}
			// 真实 Emby 对 Policy 写入回 204 空 body：回归 doJSONRequestWithTimeout
			// 对空 body 的容忍（曾导致禁用 Emby 误报 unexpected end of JSON input）。
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected Emby request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	headers := map[string]string{"X-Twilight-Client": "webui"}
	payload := fmt.Sprintf(`{"confirm":"BULK_EXPIRE_OK","expired_at":1000000000,"filter":{"uids":[%d]}}`, user.UID)
	resp := doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/users/bulk-expire", payload, adminCookies, headers)
	if resp.Code != http.StatusOK {
		t.Fatalf("bulk-expire status=%d body=%s", resp.Code, resp.Body.String())
	}
	if !embyDisabled {
		t.Fatal("bulk-expire to a past date must disable the Emby account")
	}

	var env struct {
		Data struct {
			EmbyDisabled int `json:"emby_disabled"`
			Updated      int `json:"updated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.Updated != 1 {
		t.Fatalf("updated=%d, want 1", env.Data.Updated)
	}
	if env.Data.EmbyDisabled != 1 {
		t.Fatalf("emby_disabled=%d, want 1", env.Data.EmbyDisabled)
	}
}

// TestAdminToggleEmbyOnly 验证「单独禁用 Emby（保留 Web）」：Emby 被关停、Web 账号
// 仍 active。Policy 回 204 空 body，顺带回归 unexpected end of JSON input 的修复。
func TestAdminToggleEmbyOnly(t *testing.T) {
	app := newTestApp(t)
	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")

	user, err := app.store().CreateUser(store.User{
		Username:     "embytoggle",
		Role:         store.RoleNormal,
		Active:       true,
		EmbyID:       "emby-tg",
		EmbyUsername: "embytoggle",
	})
	if err != nil {
		t.Fatal(err)
	}

	var lastDisabled *bool
	app.cfg().EmbyToken = "emby-token"
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users/emby-tg":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Id":"emby-tg","Name":"embytoggle","Policy":{"IsDisabled":false}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/Users/emby-tg/Policy":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			v := body["IsDisabled"] == true
			lastDisabled = &v
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected Emby request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	headers := map[string]string{"X-Twilight-Client": "webui"}
	resp := doJSONWithHeaders(app, http.MethodPost, fmt.Sprintf("/api/v1/admin/users/%d/emby/disable", user.UID), "", adminCookies, headers)
	if resp.Code != http.StatusOK {
		t.Fatalf("emby disable status=%d body=%s", resp.Code, resp.Body.String())
	}
	if lastDisabled == nil || !*lastDisabled {
		t.Fatal("expected Emby IsDisabled=true")
	}
	updated, _ := app.store().User(user.UID)
	if !updated.Active {
		t.Fatal("web account should remain active after Emby-only disable")
	}
	// 本地镜像必须反映「Emby 已被单独禁用」，让用户列表无需查远端即可展示。
	if !updated.EmbyDisabled {
		t.Fatal("EmbyDisabled mirror should be true after Emby-only disable")
	}
}

// TestAdminToggleEmbyEnableBlockedForInactiveWeb 验证启用方向不绕过有效期：Web 账号
// 已禁用时单独启用 Emby 必须在校验阶段被拒，且根本不应发起任何 Emby 请求。
func TestAdminToggleEmbyEnableBlockedForInactiveWeb(t *testing.T) {
	app := newTestApp(t)
	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")
	user, err := app.store().CreateUser(store.User{Username: "embyblk", Role: store.RoleNormal, Active: true, EmbyID: "emby-blk", EmbyUsername: "embyblk"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().UpdateUser(user.UID, func(u *store.User) error { u.Active = false; return nil }); err != nil {
		t.Fatal(err)
	}
	app.cfg().EmbyToken = "emby-token"
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("Emby must not be called when enable is blocked: %s %s", r.Method, r.URL.Path)
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	headers := map[string]string{"X-Twilight-Client": "webui"}
	resp := doJSONWithHeaders(app, http.MethodPost, fmt.Sprintf("/api/v1/admin/users/%d/emby/enable", user.UID), "", adminCookies, headers)
	if resp.Code != http.StatusConflict {
		t.Fatalf("enable for inactive web should be 409, got %d body=%s", resp.Code, resp.Body.String())
	}
}
