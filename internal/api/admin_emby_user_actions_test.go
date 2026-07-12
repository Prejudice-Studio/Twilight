package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

// TestAdminEmbyUserToggleByIdLinked 验证设备审查页按 emby_user_id 单独禁用 Emby：
// 已关联本地用户时 Emby 被关停、本地 EmbyDisabled 镜像置真、Web 账号不变；Policy 回
// 204 空 body 也能成功（回归 unexpected end of JSON input 修复）。
func TestAdminEmbyUserToggleByIdLinked(t *testing.T) {
	app := newTestApp(t)
	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")

	user, err := app.store().CreateUser(store.User{
		Username:     "linkedemby",
		Role:         store.RoleNormal,
		Active:       true,
		EmbyID:       "emby-linked",
		EmbyUsername: "linkedemby",
	})
	if err != nil {
		t.Fatal(err)
	}

	disabled := false
	app.cfg().EmbyToken = "emby-token"
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users/emby-linked":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Id":"emby-linked","Name":"linkedemby","Policy":{"IsDisabled":false,"IsAdministrator":false}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/Users/emby-linked/Policy":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["IsDisabled"] == true {
				disabled = true
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected Emby request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	headers := map[string]string{"X-Twilight-Client": "webui"}
	resp := doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/emby/users/emby-linked/disable", "", adminCookies, headers)
	if resp.Code != http.StatusOK {
		t.Fatalf("emby-id disable status=%d body=%s", resp.Code, resp.Body.String())
	}
	if !disabled {
		t.Fatal("expected remote Emby IsDisabled=true")
	}
	updated, _ := app.store().User(user.UID)
	if !updated.Active {
		t.Fatal("web account must stay active")
	}
	if !updated.EmbyDisabled {
		t.Fatal("EmbyDisabled mirror should be true for linked user")
	}
}

func TestSchedulerCleanupEmbyDevicesUsesDocumentedDeviceAPI(t *testing.T) {
	app := newTestApp(t)
	_ = registerAndLogin(t, app, "admin", "Admin123456")
	if _, err := app.store().CreateUser(store.User{
		Username:     "protected",
		Role:         store.RoleAdmin,
		Active:       true,
		EmbyID:       "emby-protected",
		EmbyUsername: "protected",
	}); err != nil {
		t.Fatal(err)
	}

	deleted := []string{}
	var deletedMu sync.Mutex
	app.cfg().EmbyToken = "emby-token"
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Devices":
			_, _ = w.Write([]byte(`{"Items":[
				{"Id":"device-playing","Name":"TV","AppName":"Emby Theater","LastUserId":"emby-user","LastUserName":"normal"},
				{"ReportedId":"reported-device","Name":"Phone","AppName":"CineDock","LastUserId":"emby-user","LastUserName":"normal"},
				{"Id":"twilight-client","Name":"Twilight","AppName":"Twilight","LastUserId":"emby-user","LastUserName":"normal"},
				{"Id":"protected-device","Name":"Admin Phone","AppName":"Emby","LastUserId":"emby-protected","LastUserName":"protected"}
			]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/Devices":
			deletedMu.Lock()
			deleted = append(deleted, r.URL.Query().Get("Id"))
			deletedMu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected Emby request: %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	req := httptest.NewRequest(http.MethodPost, "/scheduler", strings.NewReader(`{"runtime_params":{"dry_run":false,"max_workers":2}}`))
	summary, _, err := app.runSchedulerJob(req, "cleanup_emby_devices")
	if err != nil {
		t.Fatalf("cleanup_emby_devices returned err: %v", err)
	}
	if int(numeric(summary["deleted"])) != 2 || int(numeric(summary["skipped_twilight"])) != 1 || int(numeric(summary["skipped_protected"])) != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	deletedMu.Lock()
	got := strings.Join(deleted, ",")
	deletedMu.Unlock()
	if !strings.Contains(got, "device-playing") || !strings.Contains(got, "reported-device") || strings.Contains(got, "twilight-client") || strings.Contains(got, "protected-device") {
		t.Fatalf("deleted device ids = %#v", deleted)
	}
}

// TestAdminEmbyUserToggleByIdBlocksEmbyAdmin 验证按 emby_user_id 操作时，远端是 Emby
// 管理员的账号会被拒绝（403），且不会发出任何 Policy 写入。
func TestAdminEmbyUserToggleByIdBlocksEmbyAdmin(t *testing.T) {
	app := newTestApp(t)
	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")

	app.cfg().EmbyToken = "emby-token"
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users/emby-admin":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Id":"emby-admin","Name":"server-admin","Policy":{"IsDisabled":false,"IsAdministrator":true}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/Users/emby-admin/Policy":
			t.Fatalf("must not write policy for an Emby admin account")
		default:
			t.Fatalf("unexpected Emby request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	headers := map[string]string{"X-Twilight-Client": "webui"}
	resp := doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/emby/users/emby-admin/disable", "", adminCookies, headers)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("disabling an Emby admin should be 403, got %d body=%s", resp.Code, resp.Body.String())
	}
}

// TestAdminRefreshScopeTelegramSkipsEmby 验证 scope=telegram 时只刷新 Telegram 用户名，
// 完全不触碰 Emby（Emby 端任何请求都视为错误）。
func TestAdminRefreshScopeTelegramSkipsEmby(t *testing.T) {
	app := newTestApp(t)
	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")

	user, err := app.store().CreateUser(store.User{
		Username:         "scopetg",
		Role:             store.RoleNormal,
		Active:           false,
		TelegramID:       8888,
		TelegramUsername: "old",
		EmbyID:           "emby-scope",
		EmbyUsername:     "scopetg",
	})
	if err != nil {
		t.Fatal(err)
	}

	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123:ABC/getChat" {
			t.Fatalf("unexpected telegram path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":8888,"type":"private","username":"newtg"}}`))
	}))
	defer tg.Close()
	app.cfg().TelegramAPIURL = tg.URL

	app.cfg().EmbyToken = "emby-token"
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("Emby must not be called for scope=telegram: %s %s", r.Method, r.URL.Path)
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	headers := map[string]string{"X-Twilight-Client": "webui"}
	path := fmt.Sprintf("/api/v1/admin/users/%d/refresh-status", user.UID)
	resp := doJSONWithHeaders(app, http.MethodPost, path, `{"scope":"telegram"}`, adminCookies, headers)
	if resp.Code != http.StatusOK {
		t.Fatalf("scope telegram status=%d body=%s", resp.Code, resp.Body.String())
	}
	if updated, _ := app.store().User(user.UID); updated.TelegramUsername != "newtg" {
		t.Fatalf("telegram username = %q, want newtg", updated.TelegramUsername)
	}
}
