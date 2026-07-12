package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
)

func TestDeveloperJSDocsEndpointRequiresAdminAndDescribesGoja(t *testing.T) {
	app := newTestApp(t)

	unauth := doJSON(app, http.MethodGet, "/api/v1/admin/developer/js-docs", "", nil)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated docs status = %d body=%s", unauth.Code, unauth.Body.String())
	}

	cookies := registerAndLogin(t, app, "admin", "Password123!")
	if err := app.store().SetDeveloperModeEnabled(true); err != nil {
		t.Fatal(err)
	}
	resp := doJSON(app, http.MethodGet, "/api/v1/admin/developer/js-docs", "", cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("docs status = %d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"name":"Goja"`) {
		t.Fatalf("docs response does not describe Goja engine: %s", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"users.setLoginNotify(options)"`) {
		t.Fatalf("docs response does not include users.setLoginNotify: %s", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"users.search(query, limit)"`) ||
		!strings.Contains(resp.Body.String(), `"system.info()"`) ||
		!strings.Contains(resp.Body.String(), `"input"`) ||
		!strings.Contains(resp.Body.String(), `"exit(text)"`) ||
		!strings.Contains(resp.Body.String(), `"assert(condition, text)"`) ||
		!strings.Contains(resp.Body.String(), `"id":"exit-and-assert"`) ||
		!strings.Contains(resp.Body.String(), `"format.user(user)"`) ||
		!strings.Contains(resp.Body.String(), `"admin.searchUsers(query, limit)"`) {
		t.Fatalf("docs response does not include expanded JS APIs: %s", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"regcodes.generate(options)"`) ||
		!strings.Contains(resp.Body.String(), `"regcodes.get(code)"`) ||
		!strings.Contains(resp.Body.String(), `"invites.generate(options)"`) ||
		!strings.Contains(resp.Body.String(), `"announcements.create(options)"`) ||
		!strings.Contains(resp.Body.String(), `"admin.generateRegcode(options)"`) ||
		!strings.Contains(resp.Body.String(), `"id":"admin-generate-regcode"`) {
		t.Fatalf("docs response does not include generator JS APIs: %s", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"users.enable(uid)"`) ||
		!strings.Contains(resp.Body.String(), `"users.disable(uid)"`) ||
		!strings.Contains(resp.Body.String(), `"users.extend(uid, days)"`) ||
		!strings.Contains(resp.Body.String(), `"users.find(query, limit)"`) ||
		!strings.Contains(resp.Body.String(), `"users.exists(uid)"`) ||
		!strings.Contains(resp.Body.String(), `"regcodes.quick(days, count, type)"`) ||
		!strings.Contains(resp.Body.String(), `"invites.quick(days)"`) ||
		!strings.Contains(resp.Body.String(), `"announcements.post(title, content, level)"`) {
		t.Fatalf("docs response does not include simplified convenience JS APIs: %s", resp.Body.String())
	}
}

func TestAdminRoutesRequireAdminAuth(t *testing.T) {
	app := newTestApp(t)
	for _, route := range app.routes {
		if routeShouldRequireAdmin(route.Pattern) {
			if route.Auth != AuthAdmin {
				t.Fatalf("admin route %s %s has auth level %d", route.Method, route.Pattern, route.Auth)
			}
		}
	}
}

func routeShouldRequireAdmin(pattern string) bool {
	if strings.Contains(pattern, "/system/admin/") || strings.Contains(pattern, "/admin/") {
		return true
	}
	if pattern == "/api/v1/system/stats" {
		return true
	}
	if pattern == "/api/v1/media/request/pending" || pattern == "/api/v1/media/request/:request_id/status" {
		return true
	}
	if strings.HasPrefix(pattern, "/api/v1/security/") {
		return pattern == "/api/v1/security/login-history/:uid" ||
			strings.HasPrefix(pattern, "/api/v1/security/ip/") ||
			pattern == "/api/v1/security/suspicious" ||
			strings.HasPrefix(pattern, "/api/v1/security/users/")
	}
	if strings.HasPrefix(pattern, "/api/v1/batch/") {
		return true
	}
	return false
}

func TestDeveloperJSUsersAPISanitizesCurrentUser(t *testing.T) {
	app := newTestApp(t)
	user, err := app.store().CreateUser(store.User{
		Username:              "tg-user",
		Email:                 "secret@example.com",
		EmailVerified:         true,
		Role:                  store.RoleNormal,
		TelegramID:            424242,
		EmbyID:                "emby-sensitive-id",
		PasswordHash:          "unused",
		NotifyOnLoginEmail:    true,
		NotifyOnLoginTelegram: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	output, logs, err := app.telegramRunJSCustomCommand(`reply(JSON.stringify(users.current()));`, telegramCommandCtx{FromID: user.TelegramID}, true)
	if err != nil {
		t.Fatalf("run js: %v logs=%v", err, logs)
	}
	if !strings.Contains(output, "secret@example.com") {
		t.Fatalf("user output should include email after expanded JS user snapshot: %s", output)
	}
	if strings.Contains(output, "emby-sensitive-id") || strings.Contains(output, "unused") {
		t.Fatalf("sanitized user output leaked sensitive fields: %s", output)
	}
	if !strings.Contains(output, `"username":"tg-user"`) || !strings.Contains(output, `"email_verified":true`) || !strings.Contains(output, `"email_masked":"se***@example.com"`) {
		t.Fatalf("sanitized user output missing expected safe fields: %s", output)
	}
}

func TestDeveloperJSGetUserByUIDIsSanitizedAndAdminScoped(t *testing.T) {
	app := newTestApp(t)
	admin, err := app.store().CreateUser(store.User{
		Username:     "tg-admin",
		Role:         store.RoleAdmin,
		TelegramID:   111222,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := app.store().CreateUser(store.User{
		Username:      "lookup-target",
		Email:         "target-secret@example.com",
		EmailVerified: true,
		Role:          store.RoleNormal,
		TelegramID:    333444,
		EmbyID:        "emby-sensitive-target-id",
		PasswordHash:  "unused",
	})
	if err != nil {
		t.Fatal(err)
	}

	output, logs, err := app.telegramRunJSCustomCommand(
		`const target = getUser(`+fmt.Sprint(target.UID)+`); reply(JSON.stringify(target));`,
		telegramCommandCtx{FromID: admin.TelegramID},
		true,
	)
	if err != nil {
		t.Fatalf("admin getUser js: %v logs=%v", err, logs)
	}
	if !strings.Contains(output, `"username":"lookup-target"`) || !strings.Contains(output, `"email_verified":true`) {
		t.Fatalf("admin getUser output missing safe fields: %s", output)
	}
	if !strings.Contains(output, "target-secret@example.com") || !strings.Contains(output, "333444") {
		t.Fatalf("admin getUser output should include expanded user contact fields: %s", output)
	}
	if strings.Contains(output, "emby-sensitive-target-id") || strings.Contains(output, "unused") {
		t.Fatalf("admin getUser leaked sensitive fields: %s", output)
	}

	denied, logs, err := app.telegramRunJSCustomCommand(
		`reply(String(getUser(`+fmt.Sprint(admin.UID)+`) === null));`,
		telegramCommandCtx{FromID: target.TelegramID},
		true,
	)
	if err != nil {
		t.Fatalf("non-admin getUser js: %v logs=%v", err, logs)
	}
	if strings.TrimSpace(denied) != "true" {
		t.Fatalf("non-admin should not read another user, output=%s logs=%v", denied, logs)
	}

	self, logs, err := app.telegramRunJSCustomCommand(
		`reply(users.get(user.uid).username);`,
		telegramCommandCtx{FromID: target.TelegramID},
		true,
	)
	if err != nil {
		t.Fatalf("self users.get js: %v logs=%v", err, logs)
	}
	if strings.TrimSpace(self) != "lookup-target" {
		t.Fatalf("user should read self by UID, output=%s", self)
	}
}

func TestDeveloperJSAdminUserSearchAndControlledUpdate(t *testing.T) {
	app := newTestApp(t)
	app.cfg().AuditLogEnabled = true
	admin, err := app.store().CreateUser(store.User{
		Username:     "js-admin",
		Role:         store.RoleAdmin,
		TelegramID:   900001,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := app.store().CreateUser(store.User{
		Username:         "search-target",
		Email:            "target@example.com",
		Role:             store.RoleNormal,
		Active:           false,
		TelegramID:       900002,
		TelegramUsername: "target_tg",
		EmbyID:           "emby-secret-id",
		EmbyUsername:     "emby-target",
		BGMToken:         "bgm-secret-token",
		PasswordHash:     "password-secret-hash",
	})
	if err != nil {
		t.Fatal(err)
	}

	code := fmt.Sprintf(`
const rows = users.search("target@example.com", 3);
const result = users.update(%d, { active: true, notify_on_login_email: true });
reply(rows[0].email + "|" + rows[0].telegram_id + "|ok=" + result.ok + "|email_enabled=" + system.info().features.email_enabled);
`, target.UID)
	output, logs, err := app.telegramRunJSCustomCommand(code, telegramCommandCtx{FromID: admin.TelegramID}, true)
	if err != nil {
		t.Fatalf("admin js failed: %v logs=%v", err, logs)
	}
	if !strings.Contains(output, "target@example.com") || !strings.Contains(output, "ok=true") || !strings.Contains(output, "900002") {
		t.Fatalf("expanded admin APIs missing expected output: %s", output)
	}
	if strings.Contains(output, "emby-secret-id") || strings.Contains(output, "bgm-secret-token") || strings.Contains(output, "password-secret-hash") {
		t.Fatalf("expanded admin APIs leaked sensitive fields: %s", output)
	}
	updated, _ := app.store().User(target.UID)
	if !updated.Active || !updated.NotifyOnLoginEmail {
		t.Fatalf("users.update did not mutate allowed fields: %+v", updated)
	}
	if !hasAuditAction(app, "telegram_js_admin_user_update") {
		t.Fatalf("missing audit log for admin JS user update, audits=%+v", app.store().ListAuditLogs())
	}
}

func TestDeveloperJSSimplifiedUserHelpers(t *testing.T) {
	app := newTestApp(t)
	app.cfg().AuditLogEnabled = true
	admin, err := app.store().CreateUser(store.User{
		Username:     "js-admin-2",
		Role:         store.RoleAdmin,
		TelegramID:   910001,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := app.store().CreateUser(store.User{
		Username:     "helper-target",
		Email:        "helper@example.com",
		Role:         store.RoleNormal,
		Active:       false,
		ExpiredAt:    100, // 远早于现在，extend 应以 now 作为基准
		TelegramID:   910002,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}

	code := fmt.Sprintf(`
const found = users.find("helper@example.com", 3);
const exists = users.exists(%d);
const missing = users.exists(99999999);
const enabled = users.enable(%d);
const extended = users.extend(%d, 7);
reply([
  "found=" + found.length,
  "exists=" + exists,
  "missing=" + missing,
  "enabled=" + enabled.ok,
  "extended=" + extended.ok
].join("|"));
`, target.UID, target.UID, target.UID)
	output, logs, err := app.telegramRunJSCustomCommand(code, telegramCommandCtx{FromID: admin.TelegramID}, true)
	if err != nil {
		t.Fatalf("simplified helpers js failed: %v logs=%v", err, logs)
	}
	for _, want := range []string{"found=1", "exists=true", "missing=false", "enabled=true", "extended=true"} {
		if !strings.Contains(output, want) {
			t.Fatalf("simplified helpers output missing %q: %s", want, output)
		}
	}
	updated, _ := app.store().User(target.UID)
	if !updated.Active {
		t.Fatalf("users.enable did not activate target: %+v", updated)
	}
	now := time.Now().Unix()
	if updated.ExpiredAt < now+6*86400 || updated.ExpiredAt > now+8*86400 {
		t.Fatalf("users.extend did not add ~7 days from now: expired_at=%d now=%d", updated.ExpiredAt, now)
	}

	// 非管理员不得跨用户写或探测他人是否存在。
	normal, err := app.store().CreateUser(store.User{
		Username:     "normal-user",
		Role:         store.RoleNormal,
		TelegramID:   910003,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	denyCode := fmt.Sprintf(`reply("enable=" + users.enable(%d).ok + "|exists=" + users.exists(%d));`, target.UID, target.UID)
	deny, logs, err := app.telegramRunJSCustomCommand(denyCode, telegramCommandCtx{FromID: normal.TelegramID}, true)
	if err != nil {
		t.Fatalf("non-admin js failed: %v logs=%v", err, logs)
	}
	if !strings.Contains(deny, "enable=false") || !strings.Contains(deny, "exists=false") {
		t.Fatalf("non-admin should not enable or probe other users: %s", deny)
	}
}

func TestDeveloperJSConvenienceObjectsAndHelpers(t *testing.T) {
	app := newTestApp(t)
	admin, err := app.store().CreateUser(store.User{
		Username:     "helper-admin",
		Email:        "helper@example.com",
		Role:         store.RoleAdmin,
		Active:       true,
		TelegramID:   910001,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := app.store().CreateUser(store.User{
		Username:     "helper-target",
		Email:        "target-helper@example.com",
		Role:         store.RoleWhitelist,
		Active:       true,
		TelegramID:   910002,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}

	code := `
if (!admin.ensure()) {
  reply("admin denied");
  return;
}
const q = input.named("q", input.first);
const rows = admin.searchUsers(q, 2);
const picked = rows[0];
reply([
  "me=" + me.email,
  "arg=" + input.arg(0),
  "flag=" + input.flag("force"),
  "named=" + q,
  "role=" + format.role(roles.whitelist),
  "user=" + format.user(picked),
  "email=" + text.maskEmail(picked.email),
  "array=" + arrays.join(arrays.sortStrings(["b", "a"]), ","),
  "stats=" + system.stats().users.total
].join("\n"));
`
	output, logs, err := app.telegramRunJSCustomCommand(code, telegramCommandCtx{
		FromID:  admin.TelegramID,
		Command: "/lookup",
		Args:    []string{"--q", target.Email, "--force"},
	}, true)
	if err != nil {
		t.Fatalf("convenience js failed: %v logs=%v", err, logs)
	}
	for _, want := range []string{
		"me=helper@example.com",
		"arg=--q",
		"flag=true",
		"named=target-helper@example.com",
		"user=#2 helper-target",
		"email=ta***@example.com",
		"array=a,b",
		"stats=2",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("convenience output missing %q: %s", want, output)
		}
	}
}

func TestDeveloperJSExitAndAssertStopWithoutRuntimeError(t *testing.T) {
	app := newTestApp(t)
	user, err := app.store().CreateUser(store.User{
		Username:     "exit-user",
		Role:         store.RoleNormal,
		Active:       true,
		TelegramID:   920001,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}

	output, logs, err := app.telegramRunJSCustomCommand(`reply("before"); exit("stopped"); reply("after");`, telegramCommandCtx{FromID: user.TelegramID}, true)
	if err != nil {
		t.Fatalf("exit should stop without runtime error: %v logs=%v", err, logs)
	}
	if strings.TrimSpace(output) != "before\nstopped" {
		t.Fatalf("exit output mismatch: %q logs=%v", output, logs)
	}
	if len(logs) == 0 || logs[len(logs)-1] != "exit called" {
		t.Fatalf("exit should append audit-safe runtime log, logs=%v", logs)
	}

	output, logs, err = app.telegramRunJSCustomCommand(`assert(false, "bad input"); reply("after");`, telegramCommandCtx{FromID: user.TelegramID}, true)
	if err != nil {
		t.Fatalf("assert(false) should stop without runtime error: %v logs=%v", err, logs)
	}
	if strings.TrimSpace(output) != "bad input" {
		t.Fatalf("assert(false) output mismatch: %q logs=%v", output, logs)
	}

	output, logs, err = app.telegramRunJSCustomCommand(`assert(true, "unused"); reply("continued");`, telegramCommandCtx{FromID: user.TelegramID}, true)
	if err != nil {
		t.Fatalf("assert(true) should continue: %v logs=%v", err, logs)
	}
	if strings.TrimSpace(output) != "continued" {
		t.Fatalf("assert(true) output mismatch: %q logs=%v", output, logs)
	}
}

func TestDeveloperJSSetLoginNotifyDryRunAndRuntimeMutation(t *testing.T) {
	app := newTestApp(t)
	app.cfg().AuditLogEnabled = true
	user, err := app.store().CreateUser(store.User{
		Username:     "notify-user",
		Role:         store.RoleNormal,
		TelegramID:   555777,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	code := `const result = users.setLoginNotify({ telegram: true, email: true }); reply(JSON.stringify(result));`

	preview, logs, err := app.telegramRunJSCustomCommandWithOptions(code, telegramCommandCtx{FromID: user.TelegramID}, true, developerJSRunOptions{Preview: true})
	if err != nil {
		t.Fatalf("preview js: %v logs=%v", err, logs)
	}
	if !strings.Contains(preview, `"dry_run":true`) {
		t.Fatalf("preview did not report dry_run: %s", preview)
	}
	unchanged, _ := app.store().User(user.UID)
	if unchanged.NotifyOnLoginTelegram || unchanged.NotifyOnLoginEmail {
		t.Fatalf("preview mutated user notify flags: %+v", unchanged)
	}

	output, logs, err := app.telegramRunJSCustomCommand(code, telegramCommandCtx{FromID: user.TelegramID}, true)
	if err != nil {
		t.Fatalf("runtime js: %v logs=%v", err, logs)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("runtime output is not json: %v output=%s", err, output)
	}
	if parsed["ok"] != true {
		t.Fatalf("runtime did not return ok: %s", output)
	}
	updated, _ := app.store().User(user.UID)
	if !updated.NotifyOnLoginTelegram || !updated.NotifyOnLoginEmail {
		t.Fatalf("runtime did not update notify flags: %+v", updated)
	}
	audits := app.store().ListAuditLogs()
	found := false
	for _, entry := range audits {
		if entry.Action == "telegram_js_user_notify_update" && entry.TargetUID == user.UID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing audit log for users.setLoginNotify, audits=%+v", audits)
	}
}

func TestDeveloperJSInlineCallbackOwnerAndEdit(t *testing.T) {
	app := newTestApp(t)
	if err := app.store().SetDeveloperModeEnabled(true); err != nil {
		t.Fatal(err)
	}
	app.cfg().AuditLogEnabled = true
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	requests := []map[string]any{}
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		body["_path"] = r.URL.Path
		requests = append(requests, body)
		switch r.URL.Path {
		case "/bot123:ABC/sendMessage":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":101}}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		}
	}))
	defer tg.Close()
	app.cfg().TelegramAPIURL = tg.URL

	code := `interactions.inline("Choose", [{ text: "OK", answer: "Ack", edit: "Done" }]);`
	output, logs, err := app.telegramRunJSCustomCommandWithContext(context.Background(), code, telegramCommandCtx{ChatID: 900, FromID: 700}, true)
	if err != nil {
		t.Fatalf("inline js failed: %v logs=%v output=%s", err, logs, output)
	}
	if len(requests) != 1 || requests[0]["_path"] != "/bot123:ABC/sendMessage" {
		t.Fatalf("expected one sendMessage request, got %#v", requests)
	}
	markup, _ := requests[0]["reply_markup"].(map[string]any)
	keyboard, _ := markup["inline_keyboard"].([]any)
	row, _ := keyboard[0].([]any)
	button, _ := row[0].(map[string]any)
	callbackData := asString(button["callback_data"])
	if !strings.HasPrefix(callbackData, "djs:") {
		t.Fatalf("unexpected callback data: %#v", button)
	}

	app.handleTelegramUpdate(context.Background(), map[string]any{"callback_query": map[string]any{
		"id":   "cb-denied",
		"data": callbackData,
		"from": map[string]any{"id": float64(701)},
		"message": map[string]any{
			"message_id": float64(101),
			"chat":       map[string]any{"id": float64(900)},
		},
	}})
	if len(requests) != 2 || requests[1]["_path"] != "/bot123:ABC/answerCallbackQuery" || boolish(requests[1]["show_alert"]) != true {
		t.Fatalf("expected denied callback answer, got %#v", requests)
	}

	app.handleTelegramUpdate(context.Background(), map[string]any{"callback_query": map[string]any{
		"id":   "cb-ok",
		"data": callbackData,
		"from": map[string]any{"id": float64(700)},
		"message": map[string]any{
			"message_id": float64(101),
			"chat":       map[string]any{"id": float64(900)},
		},
	}})
	if len(requests) != 4 || requests[2]["_path"] != "/bot123:ABC/answerCallbackQuery" || requests[3]["_path"] != "/bot123:ABC/editMessageText" {
		t.Fatalf("expected callback answer + edit, got %#v", requests)
	}
	if asString(requests[3]["text"]) != "Done" {
		t.Fatalf("unexpected edit text: %#v", requests[3])
	}
	if !hasAuditAction(app, "telegram_js_interaction_callback") {
		t.Fatalf("missing audit log for developer js callback, audits=%+v", app.store().ListAuditLogs())
	}
}

func TestDeveloperJSWaitTextConsumesSameUserPlainText(t *testing.T) {
	app := newTestApp(t)
	if err := app.store().SetDeveloperModeEnabled(true); err != nil {
		t.Fatal(err)
	}
	app.cfg().AuditLogEnabled = true
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	requests := []map[string]any{}
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		body["_path"] = r.URL.Path
		requests = append(requests, body)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":102}}`))
	}))
	defer tg.Close()
	app.cfg().TelegramAPIURL = tg.URL

	code := `interactions.waitText({ seconds: 30, prompt: "Send text", reply_prefix: "Received:", max_chars: 10, numbered: true });`
	output, logs, err := app.telegramRunJSCustomCommandWithContext(context.Background(), code, telegramCommandCtx{ChatID: 901, FromID: 701}, true)
	if err != nil {
		t.Fatalf("waitText js failed: %v logs=%v output=%s", err, logs, output)
	}
	if len(requests) != 1 || asString(requests[0]["text"]) != "Send text" {
		t.Fatalf("expected prompt send, got %#v", requests)
	}

	app.handleTelegramUpdate(context.Background(), map[string]any{"message": map[string]any{
		"text": "alpha beta gamma",
		"from": map[string]any{"id": float64(702)},
		"chat": map[string]any{"id": float64(901), "type": "private"},
	}})
	if len(requests) != 1 {
		t.Fatalf("waiter consumed wrong user message: %#v", requests)
	}

	app.handleTelegramUpdate(context.Background(), map[string]any{"message": map[string]any{
		"text": "alpha beta gamma",
		"from": map[string]any{"id": float64(701)},
		"chat": map[string]any{"id": float64(901), "type": "private"},
	}})
	if len(requests) != 2 {
		t.Fatalf("expected waiter reply, got %#v", requests)
	}
	if asString(requests[1]["text"]) != "Received:\n1. alpha\n2. beta" {
		t.Fatalf("unexpected waiter reply: %#v", requests[1])
	}
	if !hasAuditAction(app, "telegram_js_interaction_wait_text") {
		t.Fatalf("missing audit log for developer js waitText, audits=%+v", app.store().ListAuditLogs())
	}
}

func TestDeveloperJSRiskTokensWarnButDoNotReject(t *testing.T) {
	result := validateDeveloperJSCommand(`function test(){ return eval("1+1"); } globalThis.x = fetch; setTimeout(function(){ reply(String(test())); }, 1);`)
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("expected risky compatibility tokens to pass validation: %#v", result)
	}
	hits, _ := result["risk_tokens"].([]string)
	if len(hits) == 0 {
		t.Fatalf("expected risk token hits: %#v", result)
	}
	blocked := validateDeveloperJSCommand(`reply(process.env.SECRET);`)
	if ok, _ := blocked["ok"].(bool); ok {
		t.Fatalf("expected process access to remain blocked: %#v", blocked)
	}
}

// TestDeveloperJSInteractionSurvivesSlowNetwork guards against the regression
// where the script execution timeout (formerly 200ms) was shorter than the
// time a real network send needs, which interrupted every command that touched
// the network with "execution timeout". The Telegram stub here responds after a
// delay longer than the old cap but well under the current budget.
func TestDeveloperJSInteractionSurvivesSlowNetwork(t *testing.T) {
	app := newTestApp(t)
	if err := app.store().SetDeveloperModeEnabled(true); err != nil {
		t.Fatal(err)
	}
	app.cfg().AuditLogEnabled = true
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	requests := []map[string]any{}
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Respond slower than the old 200ms cap to simulate a real network hop.
		time.Sleep(400 * time.Millisecond)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		body["_path"] = r.URL.Path
		requests = append(requests, body)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":120}}`))
	}))
	defer tg.Close()
	app.cfg().TelegramAPIURL = tg.URL

	code := `interactions.inline("Choose", [{ text: "OK", answer: "Ack" }]);`
	output, logs, err := app.telegramRunJSCustomCommandWithContext(context.Background(), code, telegramCommandCtx{ChatID: 950, FromID: 750}, true)
	if err != nil {
		t.Fatalf("slow-network inline js failed: %v logs=%v output=%s", err, logs, output)
	}
	if len(requests) != 1 || requests[0]["_path"] != "/bot123:ABC/sendMessage" {
		t.Fatalf("expected one sendMessage request after slow network, got %#v", requests)
	}
}

func TestDeveloperJSPresetReferenceUsesLatestPresetCode(t *testing.T) {
	app := newTestApp(t)
	if err := app.store().SetDeveloperModeEnabled(true); err != nil {
		t.Fatal(err)
	}
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	requests := []map[string]any{}
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		body["_path"] = r.URL.Path
		requests = append(requests, body)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":301}}`))
	}))
	defer tg.Close()
	app.cfg().TelegramAPIURL = tg.URL

	preset, err := app.store().UpsertDeveloperJSPreset(store.DeveloperJSPreset{Name: "hello", Code: `reply("old");`})
	if err != nil {
		t.Fatal(err)
	}
	app.cfg().TelegramCustomCommands = []config.TelegramCommandReply{{Command: "/hello", Reply: fmt.Sprintf("js:preset:%d", preset.ID)}}
	if !app.telegramHandleCustomCommand(context.Background(), "/hello", telegramCommandCtx{ChatID: 9001, FromID: 42, Command: "/hello"}, true) {
		t.Fatal("custom command was not handled")
	}
	if len(requests) == 0 || asString(requests[len(requests)-1]["text"]) != "old" {
		t.Fatalf("expected old preset output, got %#v", requests)
	}

	preset.Code = `reply("new");`
	if _, err := app.store().UpsertDeveloperJSPreset(preset); err != nil {
		t.Fatal(err)
	}
	if !app.telegramHandleCustomCommand(context.Background(), "/hello", telegramCommandCtx{ChatID: 9001, FromID: 42, Command: "/hello"}, true) {
		t.Fatal("custom command was not handled after update")
	}
	if asString(requests[len(requests)-1]["text"]) != "new" {
		t.Fatalf("expected updated preset output, got %#v", requests[len(requests)-1])
	}
}

func TestDeveloperModeDisabledBlocksJSButKeepsPlainTextCommands(t *testing.T) {
	app := newTestApp(t)
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	app.cfg().TelegramCustomCommands = []config.TelegramCommandReply{
		{Command: "/js", Reply: `js:reply("blocked");`},
		{Command: "/text", Reply: `plain ok`},
	}
	requests := []map[string]any{}
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, body)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":302}}`))
	}))
	defer tg.Close()
	app.cfg().TelegramAPIURL = tg.URL

	if !app.telegramHandleCustomCommand(context.Background(), "/js", telegramCommandCtx{ChatID: 9002, FromID: 43, Command: "/js"}, true) {
		t.Fatal("js command was not handled")
	}
	if len(requests) != 1 || asString(requests[0]["text"]) == "blocked" {
		t.Fatalf("developer mode disabled should block JS output, got %#v", requests)
	}
	if !app.telegramHandleCustomCommand(context.Background(), "/text", telegramCommandCtx{ChatID: 9002, FromID: 43, Command: "/text"}, true) {
		t.Fatal("text command was not handled")
	}
	if len(requests) != 2 || asString(requests[1]["text"]) != "plain ok" {
		t.Fatalf("plain command should still work, got %#v", requests)
	}
}

func hasAuditAction(app *App, action string) bool {
	for _, entry := range app.store().ListAuditLogs() {
		if entry.Action == action {
			return true
		}
	}
	return false
}

// TestDeveloperJSPrivateIPBlocksSSRFTargets 锁定 fetch 沙箱的 IP 黑名单，
// 防止开发者 JS 通过 DNS rebinding / IPv4-mapped IPv6 等手段访问内网、
// 回环、链路本地（云元数据 169.254.169.254）等敏感目标。
func TestDeveloperJSPrivateIPBlocksSSRFTargets(t *testing.T) {
	blocked := []string{
		"127.0.0.1",        // loopback
		"::1",              // IPv6 loopback
		"10.0.0.5",         // RFC1918
		"172.16.3.4",       // RFC1918
		"192.168.1.1",      // RFC1918
		"169.254.169.254",  // 链路本地 / 云元数据
		"fe80::1",          // IPv6 链路本地
		"fc00::1",          // IPv6 ULA
		"fd00::1",          // IPv6 ULA
		"0.0.0.0",          // unspecified
		"::",               // IPv6 unspecified
		"255.255.255.255",  // 广播
		"224.0.0.1",        // multicast
		"::ffff:127.0.0.1", // IPv4-mapped 回环
		"::ffff:10.0.0.1",  // IPv4-mapped 私网
	}
	for _, s := range blocked {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("test setup: cannot parse %q", s)
		}
		if !developerJSPrivateIP(ip) {
			t.Errorf("expected %q to be blocked as private/internal", s)
		}
	}
	if developerJSPrivateIP(nil) != true {
		t.Errorf("nil IP must be treated as blocked")
	}

	allowed := []string{
		"8.8.8.8",
		"1.1.1.1",
		"93.184.216.34", // example.com
		"2606:4700:4700::1111",
	}
	for _, s := range allowed {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("test setup: cannot parse %q", s)
		}
		if developerJSPrivateIP(ip) {
			t.Errorf("expected public IP %q to be allowed", s)
		}
	}
}

func TestDeveloperJSGenerateRegcodeRequiresAdmin(t *testing.T) {
	app := newTestApp(t)
	user, err := app.store().CreateUser(store.User{
		Username:     "js-plain",
		Role:         store.RoleNormal,
		TelegramID:   920001,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	output, logs, err := app.telegramRunJSCustomCommand(
		`const r = regcodes.generate({ count: 1, days: 30 }); reply(String(r.ok) + "|" + (r.error || ""));`,
		telegramCommandCtx{FromID: user.TelegramID},
		false,
	)
	if err != nil {
		t.Fatalf("run js: %v logs=%v", err, logs)
	}
	if !strings.Contains(output, "false") || !strings.Contains(output, "admin_required") {
		t.Fatalf("non-admin regcode generate should be denied: %s", output)
	}
}

func TestDeveloperJSGenerateRegcodePreviewDoesNotWrite(t *testing.T) {
	app := newTestApp(t)
	admin, err := app.store().CreateUser(store.User{
		Username:     "js-gen-admin",
		Role:         store.RoleAdmin,
		TelegramID:   920002,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	before := len(app.store().ListRegCodes())
	output, logs, err := app.telegramRunJSCustomCommandWithOptions(
		`const r = regcodes.generate({ count: 2, days: 30 }); reply(String(r.ok) + "|" + String(r.dry_run) + "|" + r.count);`,
		telegramCommandCtx{FromID: admin.TelegramID},
		true,
		developerJSRunOptions{Preview: true},
	)
	if err != nil {
		t.Fatalf("run js: %v logs=%v", err, logs)
	}
	if !strings.Contains(output, "true|true|2") {
		t.Fatalf("preview should be dry-run with count: %s", output)
	}
	if after := len(app.store().ListRegCodes()); after != before {
		t.Fatalf("preview must not write regcodes: before=%d after=%d", before, after)
	}
}

func TestDeveloperJSGenerateRegcodeCreatesRetrievableCode(t *testing.T) {
	app := newTestApp(t)
	app.cfg().AuditLogEnabled = true
	admin, err := app.store().CreateUser(store.User{
		Username:     "js-gen-admin2",
		Role:         store.RoleAdmin,
		TelegramID:   920003,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	output, logs, err := app.telegramRunJSCustomCommand(
		`const r = regcodes.generate({ count: 1, type: 1, days: 30, use_count_limit: 1 }); reply(r.ok ? r.codes[0] : ("err:" + r.error));`,
		telegramCommandCtx{FromID: admin.TelegramID},
		false,
	)
	if err != nil {
		t.Fatalf("run js: %v logs=%v", err, logs)
	}
	code := strings.TrimSpace(output)
	if code == "" || strings.HasPrefix(code, "err:") {
		t.Fatalf("expected a generated code, got %q", output)
	}
	if _, ok := app.store().RegCode(code); !ok {
		t.Fatalf("generated code %q not found in store", code)
	}
	if !hasAuditAction(app, "telegram_js_regcode_generate") {
		t.Fatalf("missing audit log for regcode generate")
	}
}

func TestDeveloperJSGenerateInviteCodeRespectsFeatureGate(t *testing.T) {
	app := newTestApp(t)
	admin, err := app.store().CreateUser(store.User{
		Username:     "js-invite-admin",
		Role:         store.RoleAdmin,
		TelegramID:   920004,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}

	app.cfg().InviteEnabled = false
	disabled, logs, err := app.telegramRunJSCustomCommand(
		`const r = invites.generate({ days: 30 }); reply(String(r.ok) + "|" + (r.error || ""));`,
		telegramCommandCtx{FromID: admin.TelegramID},
		false,
	)
	if err != nil {
		t.Fatalf("run js: %v logs=%v", err, logs)
	}
	if !strings.Contains(disabled, "false") || !strings.Contains(disabled, "invite_disabled") {
		t.Fatalf("invite generate should be gated when feature disabled: %s", disabled)
	}

	app.cfg().InviteEnabled = true
	output, logs, err := app.telegramRunJSCustomCommand(
		`const r = invites.generate({ days: 30 }); reply(r.ok ? r.code : ("err:" + r.error));`,
		telegramCommandCtx{FromID: admin.TelegramID},
		false,
	)
	if err != nil {
		t.Fatalf("run js: %v logs=%v", err, logs)
	}
	code := strings.TrimSpace(output)
	if code == "" || strings.HasPrefix(code, "err:") {
		t.Fatalf("expected an invite code, got %q", output)
	}
	if _, ok := app.store().InviteCode(code); !ok {
		t.Fatalf("generated invite %q not found in store", code)
	}
}

func TestDeveloperJSCreateAnnouncement(t *testing.T) {
	app := newTestApp(t)
	admin, err := app.store().CreateUser(store.User{
		Username:     "js-ann-admin",
		Role:         store.RoleAdmin,
		TelegramID:   920005,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	before := len(app.store().ListAnnouncements(true))

	preview, logs, err := app.telegramRunJSCustomCommandWithOptions(
		`const r = announcements.create({ title: "JS Notice", content: "hello", level: "info" }); reply(String(r.ok) + "|" + String(r.dry_run));`,
		telegramCommandCtx{FromID: admin.TelegramID},
		true,
		developerJSRunOptions{Preview: true},
	)
	if err != nil {
		t.Fatalf("run preview js: %v logs=%v", err, logs)
	}
	if !strings.Contains(preview, "true|true") {
		t.Fatalf("announcement preview should be dry-run: %s", preview)
	}
	if after := len(app.store().ListAnnouncements(true)); after != before {
		t.Fatalf("preview must not write announcements: before=%d after=%d", before, after)
	}

	output, logs, err := app.telegramRunJSCustomCommand(
		`const r = announcements.create({ title: "JS Notice", content: "hello", level: "info", render_mode: "markdown" }); reply(r.ok ? "ok" : ("err:" + r.error));`,
		telegramCommandCtx{FromID: admin.TelegramID},
		false,
	)
	if err != nil {
		t.Fatalf("run js: %v logs=%v", err, logs)
	}
	if strings.TrimSpace(output) != "ok" {
		t.Fatalf("expected announcement creation ok, got %q", output)
	}
	if after := len(app.store().ListAnnouncements(true)); after != before+1 {
		t.Fatalf("announcement not created: before=%d after=%d", before, after)
	}
}

func TestDeveloperJSQuickGeneratorsMatchOptionForm(t *testing.T) {
	app := newTestApp(t)
	app.cfg().AuditLogEnabled = true
	app.cfg().InviteEnabled = true
	admin, err := app.store().CreateUser(store.User{
		Username:     "js-quick-admin",
		Role:         store.RoleAdmin,
		TelegramID:   930001,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}

	annBefore := len(app.store().ListAnnouncements(true))
	code := `
const reg = regcodes.quick(30, 2);
const inv = invites.quick(15);
const ann = announcements.post("Quick", "body", "info");
reply([
  "reg=" + reg.ok + ":" + reg.count,
  "inv=" + inv.ok,
  "ann=" + ann.ok
].join("|"));
`
	output, logs, err := app.telegramRunJSCustomCommand(code, telegramCommandCtx{FromID: admin.TelegramID}, false)
	if err != nil {
		t.Fatalf("quick generators js failed: %v logs=%v", err, logs)
	}
	for _, want := range []string{"reg=true:2", "inv=true", "ann=true"} {
		if !strings.Contains(output, want) {
			t.Fatalf("quick generators output missing %q: %s", want, output)
		}
	}
	if after := len(app.store().ListAnnouncements(true)); after != annBefore+1 {
		t.Fatalf("announcements.post did not create announcement: before=%d after=%d", annBefore, after)
	}

	// 非管理员不得使用快捷生成器。
	normal, err := app.store().CreateUser(store.User{
		Username:     "js-quick-normal",
		Role:         store.RoleNormal,
		TelegramID:   930002,
		PasswordHash: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	deny, logs, err := app.telegramRunJSCustomCommand(
		`reply("reg=" + regcodes.quick(30).ok + "|inv=" + invites.quick(15).ok + "|ann=" + announcements.post("x", "y").ok);`,
		telegramCommandCtx{FromID: normal.TelegramID},
		false,
	)
	if err != nil {
		t.Fatalf("non-admin quick js failed: %v logs=%v", err, logs)
	}
	if !strings.Contains(deny, "reg=false") || !strings.Contains(deny, "inv=false") || !strings.Contains(deny, "ann=false") {
		t.Fatalf("non-admin should not use quick generators: %s", deny)
	}
}
