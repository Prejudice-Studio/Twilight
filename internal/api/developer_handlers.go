package api

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/prejudice-studio/twilight/internal/security"
	"github.com/prejudice-studio/twilight/internal/store"
)

const developerModeCode = "DEBUGMODE"

func (a *App) handleDeveloperModeActivate(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if p.User.Role != store.RoleAdmin {
		failWithCode(w, http.StatusForbidden, ErrForbidden, "developer mode requires administrator privileges")
		return
	}
	payload := decodeMap(r)
	code := strings.TrimSpace(stringValue(payload, "code"))
	password := stringValue(payload, "password")
	if !strings.EqualFold(code, developerModeCode) || password == "" {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "invalid developer mode confirmation")
		return
	}
	u, okUser := a.store().User(p.User.UID)
	if !okUser || !security.VerifyPassword(password, u.PasswordHash) {
		failWithCode(w, http.StatusUnauthorized, ErrLoginInvalid, "administrator verification failed")
		return
	}
	enabled := !a.store().DeveloperModeEnabled()
	if statusFromError(w, a.store().SetDeveloperModeEnabled(enabled)) {
		return
	}
	action := "developer_mode_activate"
	message := "developer mode enabled"
	if !enabled {
		action = "developer_mode_deactivate"
		message = "developer mode disabled"
	}
	a.audit(r, action, "admin", p.User.UID, map[string]any{"entry": "dashboard_code", "enabled": enabled})
	ok(w, message, map[string]any{
		"enabled": enabled,
		"scope":   "global_server_gate",
		"features": []string{
			"telegram_js_command_docs",
			"telegram_js_sandbox_preview",
			"telegram_js_runtime_gate",
		},
	})
}

func (a *App) handleDeveloperJSSandbox(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.store().DeveloperModeEnabled() {
		failWithCode(w, http.StatusForbidden, ErrForbidden, "developer mode is disabled")
		return
	}
	payload := decodeMap(r)
	code := stringValue(payload, "code")
	result := validateDeveloperJSCommand(code)
	if ok, _ := result["ok"].(bool); ok {
		output, logs, err := a.telegramRunJSCustomCommandWithOptions(code, telegramCommandCtx{
			ChatID:   0,
			FromID:   current(r).User.TelegramID,
			Username: current(r).User.Username,
			Command:  "/preview",
			Args:     []string{"preview"},
		}, true, developerJSRunOptions{Preview: true})
		if err != nil {
			result["ok"] = false
			result["errors"] = appendStringAny(result["errors"], err.Error())
		} else {
			result["output"] = output
			result["logs"] = logs
		}
	}
	a.audit(r, "developer_js_sandbox_preview", "admin", 0, map[string]any{"ok": result["ok"], "bytes": len(code)})
	ok(w, "sandbox preview completed", result)
}

func (a *App) handleDeveloperJSDocs(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", developerJSDocs())
}

func (a *App) handleDeveloperJSPresets(w http.ResponseWriter, r *http.Request, _ Params) {
	presets := a.store().ListDeveloperJSPresets()
	ok(w, "OK", map[string]any{"presets": presets, "total": len(presets), "developer_mode_enabled": a.store().DeveloperModeEnabled()})
}

func (a *App) handleCreateDeveloperJSPreset(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	preset, okPayload := developerJSPresetFromPayload(payload, store.DeveloperJSPreset{
		CreatorUID: current(r).User.UID,
	})
	if !okPayload {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "preset name is required")
		return
	}
	if !validateDeveloperJSPresetCode(w, preset.Code) {
		return
	}
	saved, err := a.store().UpsertDeveloperJSPreset(preset)
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "developer_js_preset_create", "admin", current(r).User.UID, map[string]any{"preset_id": saved.ID, "name": saved.Name, "bytes": len(saved.Code)})
	created(w, "developer js preset created", saved)
}

func (a *App) handleUpdateDeveloperJSPreset(w http.ResponseWriter, r *http.Request, params Params) {
	id, err := int64Param(params, "preset_id")
	if err != nil || id <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "invalid preset id")
		return
	}
	existing, found := a.store().DeveloperJSPreset(id)
	if !found {
		failWithCode(w, http.StatusNotFound, ErrNotFound, "resource not found")
		return
	}
	payload := decodeMap(r)
	preset, okPayload := developerJSPresetFromPayload(payload, existing)
	if !okPayload {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "preset name is required")
		return
	}
	if !validateDeveloperJSPresetCode(w, preset.Code) {
		return
	}
	saved, err := a.store().UpsertDeveloperJSPreset(preset)
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "developer_js_preset_update", "admin", current(r).User.UID, map[string]any{"preset_id": saved.ID, "name": saved.Name, "bytes": len(saved.Code)})
	ok(w, "developer js preset updated", saved)
}

func (a *App) handleDeleteDeveloperJSPreset(w http.ResponseWriter, r *http.Request, params Params) {
	id, err := int64Param(params, "preset_id")
	if err != nil || id <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "invalid preset id")
		return
	}
	existing, found := a.store().DeveloperJSPreset(id)
	if !found {
		failWithCode(w, http.StatusNotFound, ErrNotFound, "resource not found")
		return
	}
	if statusFromError(w, a.store().DeleteDeveloperJSPreset(id)) {
		return
	}
	a.audit(r, "developer_js_preset_delete", "admin", current(r).User.UID, map[string]any{"preset_id": id, "name": existing.Name})
	ok(w, "developer js preset deleted", map[string]any{"id": id})
}

func appendStringAny(value any, item string) []string {
	out := []string{}
	if items, ok := value.([]string); ok {
		out = append(out, items...)
	}
	return append(out, item)
}

func developerJSPresetFromPayload(payload map[string]any, base store.DeveloperJSPreset) (store.DeveloperJSPreset, bool) {
	if _, ok := payload["name"]; ok {
		base.Name = truncateString(strings.TrimSpace(fmt.Sprint(payload["name"])), 80)
	}
	if _, ok := payload["description"]; ok {
		base.Description = truncateString(strings.TrimSpace(fmt.Sprint(payload["description"])), 500)
	}
	if _, ok := payload["code"]; ok {
		base.Code = strings.TrimSpace(fmt.Sprint(payload["code"]))
	}
	return base, base.Name != ""
}

func validateDeveloperJSPresetCode(w http.ResponseWriter, code string) bool {
	if strings.TrimSpace(code) == "" {
		return true
	}
	result := validateDeveloperJSCommand(code)
	if ok, _ := result["ok"].(bool); ok {
		return true
	}
	failWithCodeData(w, http.StatusBadRequest, ErrInvalidPayload, "developer js preset rejected", result)
	return false
}

func validateDeveloperJSCommand(code string) map[string]any {
	trimmed := strings.TrimSpace(code)
	warnings := []string{
		"Preview only: saving to bot_custom_commands is required before production Bot runtime can use this script.",
		"Allowed APIs are limited to the documented ctx, command, args, user, constants, db, users, text, arrays, time, interactions, getUser(uid), reply(text), log(text), auth(role), authAdmin(), fetch(url), config(key), and env(key) bindings.",
		"config(key) and env(key) are read-only allowlists; sensitive values always return an empty string.",
		"Risky JavaScript features such as eval, Function, globalThis, fetch, and timers are available for compatibility and should be used only in administrator-reviewed presets.",
	}
	if trimmed == "" {
		return map[string]any{"ok": false, "errors": []string{"code is empty"}, "warnings": warnings}
	}
	if len(trimmed) > 8000 {
		return map[string]any{"ok": false, "errors": []string{"code exceeds 8000 bytes"}, "warnings": warnings}
	}
	lower := strings.ToLower(trimmed)
	blocked := []string{
		"xmlhttprequest", "websocket", "import(", "require(", "process.", "window.", "document.",
		"localstorage", "sessionstorage", "cookie", "constructor.constructor",
	}
	errors := []string{}
	for _, token := range blocked {
		if strings.Contains(lower, token) {
			errors = append(errors, "blocked token: "+token)
		}
	}
	risky := []string{"fetch(", "eval(", "function", "new function", "globalthis", "settimeout", "setinterval"}
	riskHits := []string{}
	for _, token := range risky {
		if strings.Contains(lower, token) {
			riskHits = append(riskHits, token)
			warnings = append(warnings, "risk token present: "+token)
		}
	}
	return map[string]any{
		"ok":          len(errors) == 0,
		"errors":      errors,
		"warnings":    warnings,
		"risk_tokens": riskHits,
		"example":     "reply('Hello ' + (user.username || 'user'));",
		"bindings":    developerJSBindingNames(),
	}
}

type developerJSDocEntry struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description"`
	Example     string   `json:"example,omitempty"`
	Mutates     bool     `json:"mutates,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	Fields      []string `json:"fields,omitempty"`
}

func developerJSBindingNames() []string {
	return []string{
		"ctx", "command", "args", "user", "constants", "db", "users", "text", "arrays", "time", "interactions",
		"getUser(uid)", "reply(text)", "log(text)", "auth(role)", "authAdmin()", "fetch(url)", "config(key)", "env(key)",
	}
}

func developerJSDocs() map[string]any {
	examples := []map[string]string{
		{
			"id":          "command-context",
			"title":       "Command input context",
			"description": "Show every non-sensitive value available when a Telegram user triggers this command.",
			"code":        "const me = users.current();\nconst lines = [\n  'command=' + command.name,\n  'command_text=' + command.text,\n  'private_chat=' + ctx.private_chat,\n  'preview=' + ctx.preview,\n  'command_time=' + time.formatUnix(ctx.command_time),\n  'args=' + JSON.stringify(args),\n  'uid=' + me.uid,\n  'username=' + (me.username || 'unbound'),\n  'role=' + me.role,\n  'active=' + me.active,\n  'has_emby=' + me.has_emby,\n  'email_verified=' + me.email_verified,\n  'telegram_bound=' + me.telegram_bound,\n  'notify_tg=' + me.notify_on_login_telegram,\n  'notify_email=' + me.notify_on_login_email\n];\nreply(text.truncate(text.joinLines(lines), 1200));",
		},
		{
			"id":          "current-user",
			"title":       "Current user summary",
			"description": "Return a sanitized summary for the Telegram-bound Twilight user.",
			"code":        "const me = users.current();\nreply('User: ' + (me.username || 'unbound') + '\\nActive: ' + me.active);",
		},
		{
			"id":          "db-summary",
			"title":       "Controlled database summary",
			"description": "Use controlled database helpers to inspect safe schema metadata and allowed counts.",
			"code":        "const schema = db.schema();\nconst lines = [\n  'collections=' + db.collections().join(', '),\n  'users=' + db.count('users'),\n  'announcements=' + db.count('announcements'),\n  'user_fields=' + schema.users.fields.join(', ')\n];\nreply(text.truncate(lines.join('\\n'), 1200));",
		},
		{
			"id":          "admin-get-user",
			"title":       "Admin exact UID lookup",
			"description": "Read a sanitized user snapshot by exact UID. Other users require the current Telegram-bound user to be an administrator.",
			"code":        "if (!authAdmin()) {\n  reply('Admin only');\n  return;\n}\nconst target = getUser(Number(args[0] || 0));\nif (!target) {\n  reply('User not found or permission denied');\n  return;\n}\nreply([\n  'UID: ' + target.uid,\n  'Username: ' + target.username,\n  'Active: ' + target.active,\n  'Role: ' + target.role,\n  'Has Emby: ' + target.has_emby,\n  'Email verified: ' + target.email_verified\n].join('\\n'));",
		},
		{
			"id":          "login-notify",
			"title":       "Toggle login notifications",
			"description": "Enable Telegram login notifications for the current bound user. Sandbox preview returns dry_run and does not write state.",
			"code":        "const result = users.setLoginNotify({ telegram: true });\nreply(result.dry_run ? 'Preview only' : 'Telegram login notifications enabled');",
		},
		{
			"id":          "db-update-current-user",
			"title":       "Controlled current-user write",
			"description": "Update only the current bound user's allowed notification fields. Preview returns dry_run.",
			"code":        "const result = db.updateCurrentUser({ notify_on_login_telegram: true, notify_on_login_email: false });\nreply(JSON.stringify(result));",
		},
		{
			"id":          "risk-fetch",
			"title":       "Risky compatibility fetch",
			"description": "Fetch is synchronous, bounded, blocks private hosts, and should be used only in reviewed admin presets.",
			"code":        "const res = fetch('https://example.com');\nif (!res.ok) {\n  reply('fetch failed: ' + (res.error || res.status));\n} else {\n  reply(text.truncate(res.text, 200));\n}",
		},
		{
			"id":          "array-tools",
			"title":       "Array and text helpers",
			"description": "Normalize arguments before replying.",
			"code":        "const values = arrays.unique(arrays.compact(args));\nreply(text.truncate(text.joinLines(values), 120));",
		},
		{
			"id":          "inline-actions",
			"title":       "Inline action message",
			"description": "Send a short inline keyboard whose callbacks use predefined answer/edit/reply text.",
			"code":        "interactions.inline('Choose an action', [\n  { text: 'Status', answer: 'OK', edit: 'Status acknowledged' },\n  { text: 'Help', reply: 'Use /help for commands' }\n]);",
		},
		{
			"id":          "wait-text",
			"title":       "Wait for next text",
			"description": "Wait for the same Telegram user to send one plain text message within a bounded time window.",
			"code":        "interactions.waitText({ seconds: 30, prompt: 'Send one line in 30 seconds', reply_prefix: 'Received:', max_chars: 120 });",
		},
	}
	return map[string]any{
		"engine": map[string]any{
			"name":        "Goja",
			"module":      "github.com/dop251/goja",
			"version":     developerJSGojaVersion(),
			"description": "In-process Go JavaScript engine used by Telegram js: custom commands.",
			"language":    "ECMAScript 5.1-oriented JavaScript with Goja-supported extensions; prefer plain synchronous JavaScript.",
			"timeout_ms":  200,
			"sandbox": []string{
				"No filesystem, process, module loader, browser globals, or broad environment access is injected.",
				"fetch is synchronous and bounded; it blocks localhost/private/link-local targets, redirects, credentials, and large responses.",
				"setTimeout/setInterval are compatibility wrappers that execute callbacks synchronously inside the same 200ms run.",
				"Config and environment access are explicit read-only allowlists; sensitive keys return an empty string.",
				"Sandbox preview is dry-run for state-changing and Telegram interaction helper APIs.",
			},
		},
		"bindings": []developerJSDocEntry{
			{Name: "ctx.private_chat", Category: "context", Type: "boolean", Description: "Whether the command was received in a private chat.", Example: "if (!ctx.private_chat) reply('Please DM me');"},
			{Name: "ctx.command_time", Category: "context", Type: "number", Description: "Unix timestamp in seconds when the command entered the sandbox."},
			{Name: "ctx.preview", Category: "context", Type: "boolean", Description: "True when running from the admin sandbox preview endpoint."},
			{Name: "ctx.command", Category: "context", Type: "string", Description: "Normalized command name, such as /hello."},
			{Name: "command", Category: "context", Type: "object", Description: "Auto-initialized command trigger object.", Fields: []string{"name", "args", "text", "private_chat", "preview", "from_id"}},
			{Name: "args", Category: "context", Type: "string[]", Description: "Command arguments excluding the command name.", Example: "const action = (args[0] || 'help').toLowerCase();"},
			{Name: "user", Category: "user", Type: "object", Description: "Sanitized snapshot of the Telegram-bound Twilight user.", Fields: []string{"uid", "username", "role", "active", "expired_at", "created_at", "register_time", "has_emby", "emby_disabled", "email_verified", "email_verified_at", "telegram_bound", "notify_on_login_telegram", "notify_on_login_email"}},
			{Name: "constants.roles", Category: "constants", Type: "object", Description: "Role constants: admin=0, user=1, whitelist=2."},
			{Name: "constants.limits", Category: "constants", Type: "object", Description: "Runtime collection limits for reply and log calls."},
		},
		"functions": []developerJSDocEntry{
			{Name: "reply(text)", Category: "output", Type: "function", Description: "Append one reply segment. At most four segments are collected and joined with newlines.", Example: "reply('hello')"},
			{Name: "log(text)", Category: "output", Type: "function", Description: "Append one audit/debug log line for this execution. At most eight lines are collected.", Example: "log('branch=help')"},
			{Name: "auth(role)", Category: "auth", Type: "function", Description: "Check the current user role. Accepts admin, whitelist, user, or numeric role strings.", Example: "if (!auth('admin')) return;"},
			{Name: "authAdmin()", Category: "auth", Type: "function", Description: "Shortcut that returns true when the current Telegram-bound user is an administrator.", Example: "if (!authAdmin()) return;"},
			{Name: "getUser(uid)", Category: "users", Type: "function", Description: "Global shortcut for exact UID lookup. Returns a sanitized snapshot or null. Other-user lookup requires administrator role; non-admin users can only read themselves.", Example: "const u = getUser(10001); if (u) reply(u.username);"},
			{Name: "fetch(url, options)", Category: "network", Type: "function", Description: "Risky synchronous compatibility helper. Supports GET/POST/HEAD, blocks localhost/private/link-local targets, does not send credentials, disables redirects, times out quickly, and returns { ok, status, statusText, text, truncated, error, blocked }.", Example: "const res = fetch('https://example.com');"},
			{Name: "setTimeout(fn, ms)", Category: "runtime", Type: "function", Description: "Compatibility helper. Executes fn synchronously and records a log warning; it does not schedule async work.", Example: "setTimeout(function(){ reply('done'); }, 1);"},
			{Name: "setInterval(fn, ms)", Category: "runtime", Type: "function", Description: "Compatibility helper. Executes fn once synchronously and records a log warning; it does not schedule repeated async work."},
			{Name: "config(key)", Category: "config", Type: "function", Description: "Read one non-sensitive allowlisted config value. Denied keys return an empty string.", Example: "config('invite.enabled')"},
			{Name: "env(key)", Category: "config", Type: "function", Description: "Read one non-sensitive allowlisted TWILIGHT_* environment value. Denied keys return an empty string.", Example: "env('TWILIGHT_HOST')"},
		},
		"namespaces": []developerJSDocEntry{
			{Name: "db.schema()", Category: "db", Type: "function", Description: "Return safe database collection metadata and allowed field names. This does not expose raw state.", Example: "const schema = db.schema(); reply(schema.users.fields.join(', '));"},
			{Name: "db.collections()", Category: "db", Type: "function", Description: "Return the controlled collection names available to the JS sandbox.", Example: "db.collections().join(', ')"},
			{Name: "db.count(name)", Category: "db", Type: "function", Description: "Return an allowed collection count. Admin-only collections return -1 for non-admin users.", Example: "db.count('announcements')"},
			{Name: "db.currentUser()", Category: "db", Type: "function", Description: "Return the same sanitized snapshot as users.current().", Example: "db.currentUser().username"},
			{Name: "db.getUser(uid)", Category: "db", Type: "function", Description: "Exact UID lookup with the same permission rules and sanitized fields as getUser(uid).", Example: "db.getUser(10001)"},
			{Name: "db.updateCurrentUser(patch)", Category: "db", Type: "function", Description: "Controlled write for the current user only. Accepted fields: notify_on_login_telegram / notify_on_login_email, or telegram / email aliases.", Example: "db.updateCurrentUser({ notify_on_login_telegram: true })", Mutates: true, Scope: "current_user_only"},
			{Name: "users.current()", Category: "users", Type: "function", Description: "Return the sanitized current Telegram-bound user snapshot.", Example: "const me = users.current(); reply(me.username || 'unbound');"},
			{Name: "users.describe()", Category: "users", Type: "function", Description: "Alias of users.current() for readable scripts.", Example: "JSON.stringify(users.describe())"},
			{Name: "users.get(uid)", Category: "users", Type: "function", Description: "Exact UID lookup returning the same sanitized snapshot as getUser(uid). Other-user lookup requires administrator role.", Example: "const target = users.get(10001);"},
			{Name: "users.byUID(uid)", Category: "users", Type: "function", Description: "Alias of users.get(uid).", Example: "users.byUID(user.uid)"},
			{Name: "users.hasRole(role)", Category: "users", Type: "function", Description: "Role check under the users namespace; same role semantics as auth(role).", Example: "users.hasRole('whitelist')"},
			{Name: "users.requireActive()", Category: "users", Type: "function", Description: "Return true only when the command is bound to an enabled local user.", Example: "if (!users.requireActive()) reply('Account inactive');"},
			{Name: "users.setLoginNotify(options)", Category: "users", Type: "function", Description: "Update the current bound user's login notification preferences. Only telegram/email boolean fields are accepted.", Example: "users.setLoginNotify({ telegram: true, email: false })", Mutates: true, Scope: "current_user_only"},
			{Name: "text.truncate(value, max)", Category: "text", Type: "function", Description: "Trim a string to max characters using the backend truncation helper.", Example: "text.truncate(args.join(' '), 80)"},
			{Name: "text.joinLines(values)", Category: "text", Type: "function", Description: "Join an array into newline-separated text.", Example: "text.joinLines(['a', 'b'])"},
			{Name: "text.escape(value)", Category: "text", Type: "function", Description: "Escape basic HTML-sensitive characters for plain text output.", Example: "text.escape('<tag>')"},
			{Name: "text.numberLines(values)", Category: "text", Type: "function", Description: "Convert an array to numbered lines.", Example: "text.numberLines(['a', 'b'])"},
			{Name: "arrays.first(values)", Category: "arrays", Type: "function", Description: "Return the first array item or undefined.", Example: "arrays.first(args)"},
			{Name: "arrays.compact(values)", Category: "arrays", Type: "function", Description: "Remove null and empty-string values from an array.", Example: "arrays.compact(args)"},
			{Name: "arrays.unique(values)", Category: "arrays", Type: "function", Description: "Return unique string values while preserving first-seen order.", Example: "arrays.unique(args)"},
			{Name: "arrays.take(values, count)", Category: "arrays", Type: "function", Description: "Return the first count array items.", Example: "arrays.take(args, 3)"},
			{Name: "time.now()", Category: "time", Type: "function", Description: "Return the current Unix timestamp in seconds.", Example: "time.now()"},
			{Name: "time.formatUnix(ts)", Category: "time", Type: "function", Description: "Format a Unix timestamp as UTC RFC3339 text.", Example: "time.formatUnix(ctx.command_time)"},
			{Name: "interactions.inline(text, actions)", Category: "interactions", Type: "function", Description: "Send a Telegram inline keyboard for the current command. Actions are static text objects with text plus optional answer/edit/reply fields.", Example: "interactions.inline('Choose', [{ text: 'OK', edit: 'Done' }])", Mutates: true, Scope: "current_chat_owner_only"},
			{Name: "interactions.waitText(options)", Category: "interactions", Type: "function", Description: "Wait for the same Telegram user to send one non-command text message within 1-60 seconds, then reply with bounded text. Options: seconds, prompt, reply_prefix, timeout_reply, max_chars, numbered.", Example: "interactions.waitText({ seconds: 30, prompt: 'Send text', reply_prefix: 'Got:' })", Mutates: true, Scope: "current_chat_owner_only"},
		},
		"native_objects": []developerJSDocEntry{
			{Name: "Object", Category: "native", Type: "constructor", Description: "Native JavaScript object support from Goja."},
			{Name: "Array", Category: "native", Type: "constructor", Description: "Native JavaScript arrays. Prefer arrays.* helpers for common command output operations."},
			{Name: "JSON", Category: "native", Type: "object", Description: "Native JSON parse/stringify support.", Example: "JSON.stringify(users.current())"},
			{Name: "Math", Category: "native", Type: "object", Description: "Native Math helpers."},
			{Name: "Date", Category: "native", Type: "constructor", Description: "Native Date object support. Prefer time.now/time.formatUnix for stable command output."},
			{Name: "Function / eval", Category: "native", Type: "runtime", Description: "Available through Goja for compatibility. Risky; use only in administrator-reviewed presets."},
			{Name: "globalThis", Category: "native", Type: "object", Description: "Bound to the Goja global object for compatibility. Does not provide browser or Node.js globals."},
			{Name: "String / Number / Boolean", Category: "native", Type: "constructors", Description: "Native primitive wrappers and prototype methods supported by Goja."},
		},
		"config_keys": []string{
			"app.name", "site.name", "global.server_name", "app.version",
			"telegram.enabled", "global.telegram_mode", "telegram.force_bind", "global.force_bind_telegram", "telegram.require_membership", "telegram.panel_enabled", "telegram.ban_on_leave",
			"invite.enabled", "invite.max_depth", "invite.limit", "invite.root_user_limit",
			"email.enabled", "email.force_bind", "media_request.enabled", "signin.enabled", "ticket.enabled", "limits.user", "limits.emby_user",
		},
		"env_keys": []string{
			"TWILIGHT_APP_NAME", "TWILIGHT_SERVER_NAME", "TWILIGHT_HOST", "TWILIGHT_PORT", "TWILIGHT_BASE_URL", "TWILIGHT_DATABASE_DRIVER",
			"TWILIGHT_EMAIL_ENABLED", "TWILIGHT_TELEGRAM_REQUIRE_GROUP_MEMBERSHIP", "TWILIGHT_TELEGRAM_BAN_ON_LEAVE", "TWILIGHT_INVITE_ENABLED", "TWILIGHT_MEDIA_REQUEST_ENABLED",
		},
		"examples": examples,
		"blocked_tokens": []string{
			"xmlhttprequest", "websocket", "import(", "require(", "process.", "window.", "document.",
			"localstorage", "sessionstorage", "cookie", "constructor.constructor",
		},
		"risk_tokens": []string{
			"fetch(", "eval(", "function", "new function", "globalthis", "settimeout", "setinterval",
		},
	}
}

func developerJSGojaVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "bundled"
	}
	for _, dep := range info.Deps {
		if dep.Path != "github.com/dop251/goja" {
			continue
		}
		if dep.Replace != nil && dep.Replace.Version != "" {
			return dep.Replace.Version
		}
		if dep.Version != "" {
			return dep.Version
		}
		return "bundled"
	}
	return "bundled"
}
