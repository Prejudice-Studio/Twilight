package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	if pattern == "/api/v1/system/stats" || pattern == "/api/v1/stats/user/:uid" {
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
		return pattern != "/api/v1/batch/watch-stats"
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
	if strings.Contains(output, "secret@example.com") || strings.Contains(output, "emby-sensitive-id") {
		t.Fatalf("sanitized user output leaked sensitive fields: %s", output)
	}
	if !strings.Contains(output, `"username":"tg-user"`) || !strings.Contains(output, `"email_verified":true`) {
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
	if strings.Contains(output, "target-secret@example.com") || strings.Contains(output, "emby-sensitive-target-id") || strings.Contains(output, "333444") {
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
