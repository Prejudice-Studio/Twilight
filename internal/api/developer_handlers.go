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
		"Allowed APIs are limited to the documented ctx, command, input, args, user, me, constants, roles, db, users, admin, system, text, arrays, time, format, interactions, getUser(uid), reply(text), exit(text), assert(condition, text), log(text), auth(role), authAdmin(), fetch(url), config(key), and env(key) bindings.",
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
	Name        string                `json:"name"`
	Category    string                `json:"category"`
	Type        string                `json:"type,omitempty"`
	Description string                `json:"description"`
	Example     string                `json:"example,omitempty"`
	Mutates     bool                  `json:"mutates,omitempty"`
	Scope       string                `json:"scope,omitempty"`
	Fields      []string              `json:"fields,omitempty"`
	Params      []developerJSDocParam `json:"params,omitempty"`
	Returns     string                `json:"returns,omitempty"`
}

type developerJSDocParam struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description"`
	Default     string `json:"default,omitempty"`
}

func jsDocParam(name, typ, description string, required bool) developerJSDocParam {
	return developerJSDocParam{Name: name, Type: typ, Description: description, Required: required}
}

func jsDocParamDefault(name, typ, description, fallback string) developerJSDocParam {
	return developerJSDocParam{Name: name, Type: typ, Description: description, Default: fallback}
}

func developerJSBindingNames() []string {
	return []string{
		"ctx", "command", "input", "args", "user", "me", "constants", "roles", "db", "users", "admin", "system", "text", "arrays", "time", "format", "interactions",
		"getUser(uid)", "reply(text)", "exit(text)", "assert(condition, text)", "log(text)", "auth(role)", "authAdmin()", "fetch(url)", "config(key)", "env(key)",
	}
}

func developerJSDocs() map[string]any {
	examples := []map[string]string{
		{
			"id":          "command-context",
			"title":       "Command input context",
			"description": "Show values available when a Telegram user triggers this command. Secrets, password hashes, tokens, API keys, and Emby internal IDs are never injected.",
			"code":        "const me = users.current();\nconst lines = [\n  'command=' + command.name,\n  'command_text=' + command.text,\n  'private_chat=' + ctx.private_chat,\n  'preview=' + ctx.preview,\n  'command_time=' + time.formatUnix(ctx.command_time),\n  'args=' + JSON.stringify(args),\n  'uid=' + me.uid,\n  'username=' + (me.username || 'unbound'),\n  'email=' + (me.email || 'none'),\n  'email_masked=' + (me.email_masked || 'none'),\n  'role=' + me.role + '/' + me.role_name,\n  'active=' + me.active,\n  'expire_status=' + me.expire_status,\n  'has_emby=' + me.has_emby,\n  'emby_username=' + (me.emby_username || 'none'),\n  'email_verified=' + me.email_verified,\n  'telegram_bound=' + me.telegram_bound,\n  'telegram_username=' + (me.telegram_username || 'none'),\n  'notify_tg=' + me.notify_on_login_telegram,\n  'notify_email=' + me.notify_on_login_email\n];\nreply(text.truncate(text.joinLines(lines), 1200));",
		},
		{
			"id":          "current-user",
			"title":       "Current user summary",
			"description": "Return a sanitized summary for the Telegram-bound Twilight user.",
			"code":        "const me = users.current();\nreply('User: ' + (me.username || 'unbound') + '\\nActive: ' + me.active);",
		},
		{
			"id":          "exit-and-assert",
			"title":       "Early exit and assertion guard",
			"description": "Stop a script without turning it into an error. Use assert for compact input validation.",
			"code":        "assert(input.has(0), 'Usage: /lookup <uid>');\nconst uid = Number(input.arg(0));\nif (!uid) {\n  exit('UID must be a number');\n}\nconst target = getUser(uid);\nif (!target) {\n  exit('User not found or permission denied');\n}\nreply(format.user(target));",
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
			"id":          "admin-search-users",
			"title":       "Admin user search",
			"description": "Search users by UID, username, email, Telegram username/ID, or Emby username. Requires administrator role.",
			"code":        "if (!authAdmin()) {\n  reply('Admin only');\n  return;\n}\nconst rows = users.search(args.join(' '), 5);\nreply(text.numberLines(rows.map(function(u) {\n  return '#' + u.uid + ' ' + u.username + ' / ' + (u.email || 'no email') + ' / ' + u.role_name;\n})));",
		},
		{
			"id":          "admin-update-user",
			"title":       "Admin controlled user update",
			"description": "Update allowed user fields from JS. Preview returns dry_run; runtime writes audit logs.",
			"code":        "if (!authAdmin()) {\n  reply('Admin only');\n  return;\n}\nconst uid = Number(args[0] || 0);\nconst result = users.update(uid, { active: true, notify_on_login_email: true });\nreply(JSON.stringify(result));",
		},
		{
			"id":          "system-info",
			"title":       "System feature flags",
			"description": "Read safe system metadata and feature flags without touching raw config secrets.",
			"code":        "const info = system.info();\nreply([\n  'name=' + info.name,\n  'version=' + info.version,\n  'email=' + info.features.email_enabled,\n  'invite=' + info.features.invite_enabled,\n  'developer=' + info.features.developer_mode\n].join('\\n'));",
		},
		{
			"id":          "convenience-admin-lookup",
			"title":       "Convenience admin lookup",
			"description": "Use input parsing, admin shortcuts, and format helpers to build a compact command.",
			"code":        "if (!admin.ensure()) return;\nconst query = input.named('q', input.text);\nconst rows = admin.searchUsers(query, 5);\nif (!rows.length) {\n  reply('No users matched: ' + query);\n  return;\n}\nreply(text.joinLines(rows.map(function(u) { return format.user(u); })));",
		},
		{
			"id":          "echo-arguments",
			"title":       "Echo arguments and flags",
			"description": "Parse positional arguments, boolean flags, and named options from a command.",
			"code":        "const lines = [\n  'command=' + input.command,\n  'first=' + input.arg(0, 'none'),\n  'has_second=' + input.has(1),\n  'force=' + input.flag('force'),\n  'uid=' + input.named('uid', 'missing'),\n  'text=' + input.text\n];\nreply(text.joinLines(lines));",
		},
		{
			"id":          "template-self-summary",
			"title":       "Template self summary",
			"description": "Build a readable reply with simple placeholders and sanitized user fields.",
			"code":        "const tpl = 'Hi {name}\\nUID: {uid}\\nEmail: {email}\\nRole: {role}\\nExpiry: {expiry}';\nreply(text.template(tpl, {\n  name: user.username || 'unbound',\n  uid: user.uid,\n  email: user.email_masked || 'none',\n  role: user.role_name,\n  expiry: user.expire_status\n}));",
		},
		{
			"id":          "self-email-status",
			"title":       "Current user email status",
			"description": "Show email, verification, and login-notification status available on the user object.",
			"code":        "const me = users.current();\nreply([\n  'email=' + (me.email_masked || 'none'),\n  'verified=' + format.bool(me.email_verified, 'yes', 'no'),\n  'notify_email=' + format.bool(me.notify_on_login_email, 'on', 'off'),\n  'notify_tg=' + format.bool(me.notify_on_login_telegram, 'on', 'off')\n].join('\\n'));",
		},
		{
			"id":          "system-stats-summary",
			"title":       "System stats summary",
			"description": "Format safe aggregate counts. Admin users receive extra admin-only counters.",
			"code":        "const stats = system.stats();\nconst lines = [\n  'users=' + stats.users.total,\n  'active=' + stats.users.active,\n  'email_bound=' + stats.users.email_bound,\n  'telegram_bound=' + stats.users.telegram_bound,\n  'developer_mode=' + stats.developer_mode\n];\nif (stats.admin_counts) {\n  lines.push('regcodes=' + stats.admin_counts.regcodes);\n  lines.push('invite_codes=' + stats.admin_counts.invite_codes);\n}\nreply(text.joinLines(lines));",
		},
		{
			"id":          "toggle-login-notify-by-flag",
			"title":       "Toggle login notify by flag",
			"description": "Use command flags to update the current user's notification preferences.",
			"code":        "const enable = input.flag('on') || text.lower(input.first) === 'on';\nconst disable = input.flag('off') || text.lower(input.first) === 'off';\nif (!enable && !disable) {\n  reply('Usage: /notify on|off');\n  return;\n}\nconst result = users.setLoginNotify({ telegram: enable, email: enable });\nreply(result.dry_run ? 'Preview only' : ('Notifications ' + (enable ? 'enabled' : 'disabled')));",
		},
		{
			"id":          "admin-list-filtered-users",
			"title":       "Admin filtered user list",
			"description": "List active normal users in pages and format the first results.",
			"code":        "if (!admin.ensure()) return;\nconst rows = admin.listUsers({ limit: 10, offset: Number(input.named('offset', 0)), role: roles.user, active: true });\nif (!rows.length) {\n  reply('No users');\n  return;\n}\nreply(text.numberLines(rows.map(function(u) { return format.user(u); })));",
		},
		{
			"id":          "admin-set-expiry-days",
			"title":       "Admin set expiry by days",
			"description": "Set a target user's expiry to N days from now using named arguments.",
			"code":        "if (!admin.ensure()) return;\nconst uid = Number(input.named('uid', 0));\nconst days = Number(input.named('days', 7));\nif (!uid || days < 1 || days > 3650) {\n  reply('Usage: /setexp --uid 10001 --days 30');\n  return;\n}\nconst result = admin.setExpiry(uid, time.addDays(time.now(), days));\nreply(result.ok ? ('New expiry: ' + format.expiry(result.user.expired_at)) : ('Failed: ' + result.error));",
		},
		{
			"id":          "admin-disable-user-with-confirm-flag",
			"title":       "Admin disable user with confirm flag",
			"description": "Require an explicit --confirm flag before a state-changing admin action.",
			"code":        "if (!admin.ensure()) return;\nconst uid = Number(input.named('uid', 0));\nif (!uid) {\n  reply('Usage: /disable --uid 10001 --confirm');\n  return;\n}\nif (!input.flag('confirm')) {\n  const target = users.get(uid);\n  reply('Preview: would disable ' + (target ? format.user(target) : ('#' + uid)) + '\\nAdd --confirm to execute.');\n  return;\n}\nconst result = admin.setActive(uid, false);\nreply(result.ok ? 'Disabled #' + uid : 'Failed: ' + result.error);",
		},
		{
			"id":          "wait-text-note",
			"title":       "Wait for note text",
			"description": "Prompt the same Telegram user for one follow-up message and echo it with a prefix.",
			"code":        "interactions.waitText({\n  seconds: 45,\n  prompt: 'Send the note text within 45 seconds.',\n  reply_prefix: 'Saved note:',\n  timeout_reply: 'Timed out; no note saved.',\n  max_chars: 200\n});",
		},
		{
			"id":          "inline-user-menu",
			"title":       "Inline user menu",
			"description": "Create a static inline menu for status/help actions without rerunning JavaScript on callback.",
			"code":        "const me = users.current();\ninteractions.inline('Account menu for ' + (me.username || 'user'), [\n  { text: 'Status', answer: 'Status checked', edit: format.user(me) },\n  { text: 'Email', reply: 'Email: ' + (me.email_masked || 'none') },\n  { text: 'Help', reply: 'Use /help for built-in commands.' }\n]);",
		},
		{
			"id":          "fetch-json-status",
			"title":       "Fetch external JSON status",
			"description": "Fetch public JSON, parse it safely, and handle blocked or failed requests.",
			"code":        "const res = fetch('https://example.com/status.json');\nif (!res.ok) {\n  reply('fetch failed: ' + (res.error || res.status));\n  return;\n}\ntry {\n  const data = JSON.parse(res.text);\n  reply('status=' + (data.status || 'unknown'));\n} catch (e) {\n  reply('invalid json: ' + text.truncate(res.text, 120));\n}",
		},
		{
			"id":          "complex-admin-audit-summary",
			"title":       "Complex admin audit-style summary",
			"description": "Combine search, formatting, counters, and bounded output into one admin command.",
			"code":        "if (!admin.ensure()) return;\nconst query = input.named('q', input.text);\nconst rows = admin.searchUsers(query, 20);\nconst active = rows.filter(function(u) { return u.active; }).length;\nconst withEmail = rows.filter(function(u) { return u.has_email; }).length;\nconst preview = arrays.take(rows.map(function(u) {\n  return '#' + u.uid + ' ' + (u.username || 'unknown') + ' ' + format.bool(u.active, 'on', 'off') + ' ' + (u.email_masked || 'no-email');\n}), 10);\nreply(text.truncate(text.joinLines([\n  'query=' + query,\n  'matched=' + rows.length,\n  'active=' + active,\n  'email_bound=' + withEmail,\n  '---',\n  text.numberLines(preview)\n]), 1200));",
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
				"Scripts are executed inside a function scope, so top-level return or exit(message) can be used to stop a command early.",
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
			{Name: "input", Category: "context", Type: "object", Description: "Convenience command input object with parsed argument helpers.", Fields: []string{"command", "args", "text", "count", "first", "rest", "private_chat", "preview", "arg(index, fallback)", "has(index)", "flag(name)", "named(name, fallback)"}},
			{Name: "args", Category: "context", Type: "string[]", Description: "Command arguments excluding the command name.", Example: "const action = (args[0] || 'help').toLowerCase();"},
			{Name: "user", Category: "user", Type: "object", Description: "Snapshot of the Telegram-bound Twilight user. Includes account metadata such as email and Telegram username/ID, but never password hashes, tokens, API keys, BGM token values, or Emby internal IDs.", Fields: []string{"uid", "username", "email", "email_masked", "has_email", "role", "role_name", "active", "expired_at", "expire_status", "created_at", "register_time", "has_emby", "emby_username", "emby_disabled", "avatar", "background", "bgm_mode", "bgm_token_set", "emby_grant_locked", "registration_source", "pending_emby", "pending_emby_days", "email_verified", "email_verified_at", "telegram_bound", "telegram_id", "telegram_username", "notify_on_login_telegram", "notify_on_login_email", "legacy_api_key_enabled", "rebinding_in_progress", "rebinding_since"}},
			{Name: "me", Category: "user", Type: "object", Description: "Alias of user for shorter scripts.", Example: "reply(me.username)"},
			{Name: "constants.roles", Category: "constants", Type: "object", Description: "Role constants: admin=0, user=1, whitelist=2."},
			{Name: "roles", Category: "constants", Type: "object", Description: "Shortcut alias for constants.roles.", Fields: []string{"admin", "user", "whitelist"}},
			{Name: "constants.limits", Category: "constants", Type: "object", Description: "Runtime collection limits for reply and log calls."},
		},
		"functions": []developerJSDocEntry{
			{Name: "reply(text)", Category: "output", Type: "function", Description: "Append one reply segment. At most four segments are collected and joined with newlines.", Example: "reply('hello')", Params: []developerJSDocParam{jsDocParam("text", "string", "Reply text. It is truncated and sensitive fragments are redacted before sending.", true)}, Returns: "void"},
			{Name: "exit(text)", Category: "control", Type: "function", Description: "Stop the current script normally. If text is provided, append it as a final reply segment before stopping. This is not treated as a sandbox error.", Example: "if (!input.has(0)) exit('Usage: /lookup <uid>');", Params: []developerJSDocParam{jsDocParamDefault("text", "string", "Optional reply text sent before stopping.", "")}, Returns: "never"},
			{Name: "assert(condition, text)", Category: "control", Type: "function", Description: "Continue when condition is truthy; otherwise append text and stop the script normally. Useful for compact guards.", Example: "assert(authAdmin(), 'Admin only');\nreply('allowed');", Params: []developerJSDocParam{jsDocParam("condition", "any", "Truthy value required to continue.", true), jsDocParamDefault("text", "string", "Reply text used when the assertion fails.", "assertion failed")}, Returns: "boolean|never"},
			{Name: "log(text)", Category: "output", Type: "function", Description: "Append one audit/debug log line for this execution. At most eight lines are collected.", Example: "log('branch=help')", Params: []developerJSDocParam{jsDocParam("text", "string", "Internal execution note. Do not write secrets.", true)}, Returns: "void"},
			{Name: "auth(role)", Category: "auth", Type: "function", Description: "Check the current user role. Accepts admin, whitelist, user, or numeric role strings.", Example: "if (!auth('admin')) return;", Params: []developerJSDocParam{jsDocParam("role", "string|number", "Role name or role id: admin/0, whitelist/2, user/1.", true)}, Returns: "boolean"},
			{Name: "authAdmin()", Category: "auth", Type: "function", Description: "Shortcut that returns true when the current Telegram-bound user is an administrator.", Example: "if (!authAdmin()) return;", Returns: "boolean"},
			{Name: "getUser(uid)", Category: "users", Type: "function", Description: "Global shortcut for exact UID lookup. Returns a sanitized snapshot or null. Other-user lookup requires administrator role; non-admin users can only read themselves.", Example: "const u = getUser(10001); if (u) reply(u.username);", Params: []developerJSDocParam{jsDocParam("uid", "number|string", "Exact Twilight UID. Non-admin callers may only pass their own UID.", true)}, Returns: "UserSnapshot|null"},
			{Name: "fetch(url, options)", Category: "network", Type: "function", Description: "Risky synchronous compatibility helper. Supports GET/POST/HEAD, blocks localhost/private/link-local targets, does not send credentials, disables redirects, times out quickly, and returns { ok, status, statusText, text, truncated, error, blocked }.", Example: "const res = fetch('https://example.com', { method: 'GET' });\nif (res.ok) reply(text.truncate(res.text, 200));", Params: []developerJSDocParam{jsDocParam("url", "string", "Public http/https URL. Localhost, private IP, link-local, and non-HTTP schemes are blocked.", true), jsDocParamDefault("options.method", "string", "HTTP method. Only GET, POST, and HEAD are accepted.", "GET")}, Returns: "{ ok:boolean, status?:number, statusText?:string, text?:string, truncated?:boolean, error?:string, blocked?:boolean }"},
			{Name: "setTimeout(fn, ms)", Category: "runtime", Type: "function", Description: "Compatibility helper. Executes fn synchronously and records a log warning; it does not schedule async work.", Example: "setTimeout(function(){ reply('done'); }, 1);", Params: []developerJSDocParam{jsDocParam("fn", "function", "Callback executed immediately inside the same sandbox run.", true), jsDocParamDefault("ms", "number", "Requested delay in milliseconds. It is logged but not actually scheduled.", "0")}, Returns: "number"},
			{Name: "setInterval(fn, ms)", Category: "runtime", Type: "function", Description: "Compatibility helper. Executes fn once synchronously and records a log warning; it does not schedule repeated async work.", Example: "setInterval(function(){ log('tick once'); }, 1000);", Params: []developerJSDocParam{jsDocParam("fn", "function", "Callback executed once immediately inside the same sandbox run.", true), jsDocParamDefault("ms", "number", "Requested interval in milliseconds. It is logged but not actually scheduled.", "0")}, Returns: "number"},
			{Name: "config(key)", Category: "config", Type: "function", Description: "Read one non-sensitive allowlisted config value. Denied keys return an empty string.", Example: "reply('invite=' + config('invite.enabled'));", Params: []developerJSDocParam{jsDocParam("key", "string", "Allowlisted config key, such as invite.enabled or site.name.", true)}, Returns: "string|number|boolean"},
			{Name: "env(key)", Category: "config", Type: "function", Description: "Read one non-sensitive allowlisted TWILIGHT_* environment value. Denied keys return an empty string.", Example: "reply('host=' + env('TWILIGHT_HOST'));", Params: []developerJSDocParam{jsDocParam("key", "string", "Allowlisted TWILIGHT_* environment key.", true)}, Returns: "string"},
		},
		"namespaces": []developerJSDocEntry{
			{Name: "input.arg(index, fallback)", Category: "input", Type: "function", Description: "Read one argument by zero-based index with an optional fallback.", Example: "const action = input.arg(0, 'help');\nreply('action=' + action);", Params: []developerJSDocParam{jsDocParam("index", "number", "Zero-based argument index.", true), jsDocParamDefault("fallback", "string", "Value returned when the index is missing.", "")}, Returns: "string"},
			{Name: "input.has(index)", Category: "input", Type: "function", Description: "Check whether an argument exists and is not blank.", Example: "if (!input.has(0)) reply('missing argument');", Params: []developerJSDocParam{jsDocParam("index", "number", "Zero-based argument index.", true)}, Returns: "boolean"},
			{Name: "input.flag(name)", Category: "input", Type: "function", Description: "Check whether a flag is present. Both --name and -name are accepted.", Example: "if (input.flag('force')) reply('force mode');", Params: []developerJSDocParam{jsDocParam("name", "string", "Flag name with or without leading dashes.", true)}, Returns: "boolean"},
			{Name: "input.named(name, fallback)", Category: "input", Type: "function", Description: "Read a named option from --name=value, -name=value, --name value, or -name value.", Example: "const uid = Number(input.named('uid', 0));\nreply('uid=' + uid);", Params: []developerJSDocParam{jsDocParam("name", "string", "Option name with or without leading dashes.", true), jsDocParamDefault("fallback", "string", "Value returned when the option is missing.", "")}, Returns: "string"},
			{Name: "db.schema()", Category: "db", Type: "function", Description: "Return safe database collection metadata and allowed field names. This does not expose raw state.", Example: "const schema = db.schema(); reply(schema.users.fields.join(', '));", Returns: "object"},
			{Name: "db.collections()", Category: "db", Type: "function", Description: "Return the controlled collection names available to the JS sandbox.", Example: "reply(db.collections().join(', '));", Returns: "string[]"},
			{Name: "db.count(name)", Category: "db", Type: "function", Description: "Return an allowed collection count. Admin-only collections return -1 for non-admin users.", Example: "reply('announcements=' + db.count('announcements'));", Params: []developerJSDocParam{jsDocParam("name", "string", "Collection name from db.collections().", true)}, Returns: "number"},
			{Name: "db.currentUser()", Category: "db", Type: "function", Description: "Return the same sanitized snapshot as users.current().", Example: "reply(db.currentUser().username || 'unbound');", Returns: "UserSnapshot"},
			{Name: "db.getUser(uid)", Category: "db", Type: "function", Description: "Exact UID lookup with the same permission rules and sanitized fields as getUser(uid).", Example: "const u = db.getUser(user.uid); reply(format.user(u));", Params: []developerJSDocParam{jsDocParam("uid", "number|string", "Exact Twilight UID.", true)}, Returns: "UserSnapshot|null"},
			{Name: "db.findUsers(query, limit)", Category: "db", Type: "function", Description: "Alias of users.search(query, limit). Admin-only.", Example: "reply(text.numberLines(db.findUsers('alice', 5).map(format.user)));", Params: []developerJSDocParam{jsDocParam("query", "string", "Search text. Admin-only; matches UID, username, email, Telegram username/ID, or Emby username.", true), jsDocParamDefault("limit", "number", "Maximum returned rows. Capped at 50.", "20")}, Returns: "UserSnapshot[]"},
			{Name: "db.listUsers(options)", Category: "db", Type: "function", Description: "Alias of users.list(options). Non-admin users only receive themselves; admins can page/filter.", Example: "const rows = db.listUsers({ limit: 20, role: 1 });\nreply(text.numberLines(rows.map(format.user)));", Params: []developerJSDocParam{jsDocParamDefault("options.limit", "number", "Maximum returned rows. Capped at 50.", "20"), jsDocParamDefault("options.offset", "number", "Number of matching rows to skip.", "0"), jsDocParamDefault("options.role", "string|number", "Role filter: 0/admin, 1/user, 2/whitelist.", ""), jsDocParamDefault("options.active", "boolean|string", "Active-state filter.", "")}, Returns: "UserSnapshot[]"},
			{Name: "db.updateCurrentUser(patch)", Category: "db", Type: "function", Description: "Controlled write for the current user only. Accepted fields: notify_on_login_telegram / notify_on_login_email, or telegram / email aliases.", Example: "const r = db.updateCurrentUser({ notify_on_login_telegram: true });\nreply(r.dry_run ? 'preview' : 'saved');", Params: []developerJSDocParam{jsDocParam("patch", "object", "Allowed booleans: notify_on_login_telegram, notify_on_login_email, telegram, email.", true)}, Returns: "{ ok:boolean, dry_run?:boolean, user?:UserSnapshot, error?:string }", Mutates: true, Scope: "current_user_only"},
			{Name: "db.updateUser(uid, patch)", Category: "db", Type: "function", Description: "Alias of users.update(uid, patch). Admin-only controlled write with audit logging.", Example: "const r = db.updateUser(10001, { active: true });\nreply(r.ok ? 'saved' : r.error);", Params: []developerJSDocParam{jsDocParam("uid", "number|string", "Target Twilight UID.", true), jsDocParam("patch", "object", "Allowed fields: active, role, expired_at, notify_on_login_telegram, notify_on_login_email, telegram, email.", true)}, Returns: "{ ok:boolean, dry_run?:boolean, user?:UserSnapshot, error?:string }", Mutates: true, Scope: "admin_only"},
			{Name: "users.current()", Category: "users", Type: "function", Description: "Return the sanitized current Telegram-bound user snapshot.", Example: "const me = users.current(); reply(me.username || 'unbound');", Returns: "UserSnapshot"},
			{Name: "users.describe()", Category: "users", Type: "function", Description: "Alias of users.current() for readable scripts.", Example: "reply(JSON.stringify(users.describe()));", Returns: "UserSnapshot"},
			{Name: "users.get(uid)", Category: "users", Type: "function", Description: "Exact UID lookup returning the same sanitized snapshot as getUser(uid). Other-user lookup requires administrator role.", Example: "const target = users.get(10001);\nif (target) reply(format.user(target));", Params: []developerJSDocParam{jsDocParam("uid", "number|string", "Exact Twilight UID.", true)}, Returns: "UserSnapshot|null"},
			{Name: "users.byUID(uid)", Category: "users", Type: "function", Description: "Alias of users.get(uid).", Example: "reply(format.user(users.byUID(user.uid)));", Params: []developerJSDocParam{jsDocParam("uid", "number|string", "Exact Twilight UID.", true)}, Returns: "UserSnapshot|null"},
			{Name: "users.search(query, limit)", Category: "users", Type: "function", Description: "Admin-only user search by UID, username, email, Telegram username/ID, or Emby username. Returns sanitized user snapshots, limited to 50 rows.", Example: "const rows = users.search('alice', 5);\nreply(text.numberLines(rows.map(format.user)));", Params: []developerJSDocParam{jsDocParam("query", "string", "Search text.", true), jsDocParamDefault("limit", "number", "Maximum returned rows. Capped at 50.", "20")}, Returns: "UserSnapshot[]"},
			{Name: "users.list(options)", Category: "users", Type: "function", Description: "List users. Non-admin users receive only their own snapshot. Admins can pass { limit, offset, role, active }; limit is capped at 50.", Example: "const rows = users.list({ limit: 10, active: true });\nreply(text.numberLines(rows.map(format.user)));", Params: []developerJSDocParam{jsDocParamDefault("options.limit", "number", "Maximum returned rows. Capped at 50.", "20"), jsDocParamDefault("options.offset", "number", "Number of matching rows to skip.", "0"), jsDocParamDefault("options.role", "string|number", "Role filter: 0/admin, 1/user, 2/whitelist.", ""), jsDocParamDefault("options.active", "boolean|string", "Active-state filter.", "")}, Returns: "UserSnapshot[]"},
			{Name: "users.hasRole(role)", Category: "users", Type: "function", Description: "Role check under the users namespace; same role semantics as auth(role).", Example: "if (users.hasRole('whitelist')) reply('allowed');", Params: []developerJSDocParam{jsDocParam("role", "string|number", "Role name or role id.", true)}, Returns: "boolean"},
			{Name: "users.requireActive()", Category: "users", Type: "function", Description: "Return true only when the command is bound to an enabled local user.", Example: "if (!users.requireActive()) {\n  reply('Account inactive');\n  return;\n}", Returns: "boolean"},
			{Name: "users.setLoginNotify(options)", Category: "users", Type: "function", Description: "Update the current bound user's login notification preferences. Only telegram/email boolean fields are accepted.", Example: "const r = users.setLoginNotify({ telegram: true, email: false });\nreply(r.ok ? 'saved' : r.error);", Params: []developerJSDocParam{jsDocParam("options", "object", "Allowed booleans: telegram and email.", true)}, Returns: "{ ok:boolean, dry_run?:boolean, user?:UserSnapshot, error?:string }", Mutates: true, Scope: "current_user_only"},
			{Name: "users.setActive(uid, active)", Category: "users", Type: "function", Description: "Admin-only controlled write for Web account enabled/disabled state. Last active admin protection is enforced.", Example: "const r = users.setActive(10001, false);\nreply(r.ok ? 'disabled' : r.error);", Params: []developerJSDocParam{jsDocParam("uid", "number|string", "Target Twilight UID.", true), jsDocParam("active", "boolean", "New Web account enabled state.", true)}, Returns: "{ ok:boolean, dry_run?:boolean, user?:UserSnapshot, error?:string }", Mutates: true, Scope: "admin_only"},
			{Name: "users.setRole(uid, role)", Category: "users", Type: "function", Description: "Admin-only controlled role update. Accepts constants.roles.admin/user/whitelist or numeric roles; last admin protection is enforced.", Example: "const r = users.setRole(10001, constants.roles.whitelist);\nreply(r.ok ? 'role updated' : r.error);", Params: []developerJSDocParam{jsDocParam("uid", "number|string", "Target Twilight UID.", true), jsDocParam("role", "number", "Role id: constants.roles.admin/user/whitelist.", true)}, Returns: "{ ok:boolean, dry_run?:boolean, user?:UserSnapshot, error?:string }", Mutates: true, Scope: "admin_only"},
			{Name: "users.setExpiry(uid, expiredAt)", Category: "users", Type: "function", Description: "Admin-only controlled expiry update using Unix seconds; -1 means permanent. Runtime writes an audit log.", Example: "const r = users.setExpiry(10001, time.addDays(time.now(), 7));\nreply(r.ok ? format.expiry(r.user.expired_at) : r.error);", Params: []developerJSDocParam{jsDocParam("uid", "number|string", "Target Twilight UID.", true), jsDocParam("expiredAt", "number", "Unix seconds. Use -1 for permanent.", true)}, Returns: "{ ok:boolean, dry_run?:boolean, user?:UserSnapshot, error?:string }", Mutates: true, Scope: "admin_only"},
			{Name: "users.update(uid, patch)", Category: "users", Type: "function", Description: "Admin-only combined controlled update. Accepted patch fields: active, role, expired_at, notify_on_login_telegram / notify_on_login_email, telegram / email aliases.", Example: "const r = users.update(10001, { active: true, notify_on_login_email: true });\nreply(r.ok ? 'saved' : r.error);", Params: []developerJSDocParam{jsDocParam("uid", "number|string", "Target Twilight UID.", true), jsDocParam("patch", "object", "Allowed fields: active, role, expired_at, notify_on_login_telegram, notify_on_login_email, telegram, email.", true)}, Returns: "{ ok:boolean, dry_run?:boolean, user?:UserSnapshot, error?:string }", Mutates: true, Scope: "admin_only"},
			{Name: "admin.ok()", Category: "admin", Type: "function", Description: "Return true when the current Telegram-bound user is an administrator.", Example: "if (!admin.ok()) return;", Returns: "boolean"},
			{Name: "admin.ensure()", Category: "admin", Type: "function", Description: "Admin guard helper. Returns true for administrators; otherwise records a sandbox log line and returns false.", Example: "if (!admin.ensure()) return;", Returns: "boolean"},
			{Name: "admin.searchUsers(query, limit)", Category: "admin", Type: "function", Description: "Shortcut for users.search(query, limit). Admin-only.", Example: "const rows = admin.searchUsers(input.text, 5);\nreply(text.numberLines(rows.map(format.user)));", Params: []developerJSDocParam{jsDocParam("query", "string", "Search text.", true), jsDocParamDefault("limit", "number", "Maximum returned rows. Capped at 50.", "20")}, Returns: "UserSnapshot[]"},
			{Name: "admin.listUsers(options)", Category: "admin", Type: "function", Description: "Shortcut for users.list(options). Admin-only for cross-user results.", Example: "reply(text.numberLines(admin.listUsers({ limit: 10 }).map(format.user)));", Params: []developerJSDocParam{jsDocParamDefault("options.limit", "number", "Maximum returned rows. Capped at 50.", "20"), jsDocParamDefault("options.offset", "number", "Number of matching rows to skip.", "0"), jsDocParamDefault("options.role", "string|number", "Role filter.", ""), jsDocParamDefault("options.active", "boolean|string", "Active-state filter.", "")}, Returns: "UserSnapshot[]"},
			{Name: "admin.updateUser(uid, patch)", Category: "admin", Type: "function", Description: "Shortcut for users.update(uid, patch). Admin-only controlled write with audit logging.", Example: "const uid = Number(input.named('uid', 0));\nreply(admin.updateUser(uid, { active: true }).ok ? 'saved' : 'failed');", Params: []developerJSDocParam{jsDocParam("uid", "number|string", "Target Twilight UID.", true), jsDocParam("patch", "object", "Allowed user patch fields.", true)}, Returns: "{ ok:boolean, dry_run?:boolean, user?:UserSnapshot, error?:string }", Mutates: true, Scope: "admin_only"},
			{Name: "admin.setActive(uid, active)", Category: "admin", Type: "function", Description: "Shortcut for users.setActive(uid, active).", Example: "const r = admin.setActive(10001, true);\nreply(r.ok ? 'enabled' : r.error);", Params: []developerJSDocParam{jsDocParam("uid", "number|string", "Target Twilight UID.", true), jsDocParam("active", "boolean", "New Web account enabled state.", true)}, Returns: "{ ok:boolean, dry_run?:boolean, user?:UserSnapshot, error?:string }", Mutates: true, Scope: "admin_only"},
			{Name: "admin.setRole(uid, role)", Category: "admin", Type: "function", Description: "Shortcut for users.setRole(uid, role).", Example: "const r = admin.setRole(10001, roles.user);\nreply(r.ok ? 'role updated' : r.error);", Params: []developerJSDocParam{jsDocParam("uid", "number|string", "Target Twilight UID.", true), jsDocParam("role", "number", "Role id.", true)}, Returns: "{ ok:boolean, dry_run?:boolean, user?:UserSnapshot, error?:string }", Mutates: true, Scope: "admin_only"},
			{Name: "admin.setExpiry(uid, expiredAt)", Category: "admin", Type: "function", Description: "Shortcut for users.setExpiry(uid, expiredAt).", Example: "const r = admin.setExpiry(10001, time.fromNow(86400));\nreply(r.ok ? 'expiry updated' : r.error);", Params: []developerJSDocParam{jsDocParam("uid", "number|string", "Target Twilight UID.", true), jsDocParam("expiredAt", "number", "Unix seconds. Use -1 for permanent.", true)}, Returns: "{ ok:boolean, dry_run?:boolean, user?:UserSnapshot, error?:string }", Mutates: true, Scope: "admin_only"},
			{Name: "admin.stats()", Category: "admin", Type: "function", Description: "Return the same summary as system.stats(), including admin-only counts when authorized.", Example: "const stats = admin.stats();\nreply('regcodes=' + stats.admin_counts.regcodes);", Returns: "object"},
			{Name: "system.info()", Category: "system", Type: "function", Description: "Return safe system metadata, feature flags, and limits. Does not expose raw config secrets.", Example: "const info = system.info(); reply(info.name);", Returns: "object"},
			{Name: "system.feature(key)", Category: "system", Type: "function", Description: "Read one safe boolean feature flag from the system namespace.", Example: "if (system.feature('email_enabled')) reply('email ready');", Params: []developerJSDocParam{jsDocParam("key", "string", "Feature key from system.info().features.", true)}, Returns: "boolean"},
			{Name: "system.stats()", Category: "system", Type: "function", Description: "Return safe aggregate counts. Admin callers also receive admin-only collection counts.", Example: "const stats = system.stats();\nreply('users=' + stats.users.total);", Returns: "object"},
			{Name: "text.truncate(value, max)", Category: "text", Type: "function", Description: "Trim a string to max characters using the backend truncation helper.", Example: "reply(text.truncate(args.join(' '), 80));", Params: []developerJSDocParam{jsDocParam("value", "any", "Text-like value.", true), jsDocParamDefault("max", "number", "Maximum characters.", "80")}, Returns: "string"},
			{Name: "text.joinLines(values)", Category: "text", Type: "function", Description: "Join an array into newline-separated text.", Example: "reply(text.joinLines(['a', 'b']));", Params: []developerJSDocParam{jsDocParam("values", "any[]", "Items converted to strings.", true)}, Returns: "string"},
			{Name: "text.escape(value)", Category: "text", Type: "function", Description: "Escape basic HTML-sensitive characters for plain text output.", Example: "reply(text.escape('<tag>'));", Params: []developerJSDocParam{jsDocParam("value", "any", "Text-like value.", true)}, Returns: "string"},
			{Name: "text.numberLines(values)", Category: "text", Type: "function", Description: "Convert an array to numbered lines.", Example: "reply(text.numberLines(['a', 'b']));", Params: []developerJSDocParam{jsDocParam("values", "any[]", "Items converted to strings.", true)}, Returns: "string"},
			{Name: "text.trim(value)", Category: "text", Type: "function", Description: "Trim leading and trailing whitespace.", Example: "const q = text.trim(input.text);", Params: []developerJSDocParam{jsDocParam("value", "any", "Text-like value.", true)}, Returns: "string"},
			{Name: "text.lower(value)", Category: "text", Type: "function", Description: "Lowercase text.", Example: "const action = text.lower(input.first);", Params: []developerJSDocParam{jsDocParam("value", "any", "Text-like value.", true)}, Returns: "string"},
			{Name: "text.upper(value)", Category: "text", Type: "function", Description: "Uppercase text.", Example: "reply(text.upper(input.first));", Params: []developerJSDocParam{jsDocParam("value", "any", "Text-like value.", true)}, Returns: "string"},
			{Name: "text.contains(value, needle)", Category: "text", Type: "function", Description: "Case-insensitive contains check.", Example: "if (text.contains(input.text, 'help')) reply('help requested');", Params: []developerJSDocParam{jsDocParam("value", "any", "Text-like value.", true), jsDocParam("needle", "any", "Case-insensitive search text.", true)}, Returns: "boolean"},
			{Name: "text.split(value, separator)", Category: "text", Type: "function", Description: "Split text by a separator; default separator is a space.", Example: "const tags = text.split(input.text, ',');", Params: []developerJSDocParam{jsDocParam("value", "any", "Text-like value.", true), jsDocParamDefault("separator", "string", "Separator string.", "space")}, Returns: "string[]"},
			{Name: "text.maskEmail(email)", Category: "text", Type: "function", Description: "Mask an email address using the same backend display helper.", Example: "reply(text.maskEmail(user.email));", Params: []developerJSDocParam{jsDocParam("email", "string", "Email address. Empty input returns an empty string.", true)}, Returns: "string"},
			{Name: "text.template(template, data)", Category: "text", Type: "function", Description: "Replace {key} placeholders from a simple object and truncate/redact the result.", Example: "reply(text.template('Hi {name}', { name: user.username }));", Params: []developerJSDocParam{jsDocParam("template", "string", "Template containing {key} placeholders.", true), jsDocParam("data", "object", "Flat object used for placeholder replacement.", true)}, Returns: "string"},
			{Name: "arrays.first(values)", Category: "arrays", Type: "function", Description: "Return the first array item or undefined.", Example: "reply(String(arrays.first(args) || 'none'));", Params: []developerJSDocParam{jsDocParam("values", "any[]", "Array-like input.", true)}, Returns: "any|undefined"},
			{Name: "arrays.last(values)", Category: "arrays", Type: "function", Description: "Return the last array item or undefined.", Example: "reply(String(arrays.last(args) || 'none'));", Params: []developerJSDocParam{jsDocParam("values", "any[]", "Array-like input.", true)}, Returns: "any|undefined"},
			{Name: "arrays.compact(values)", Category: "arrays", Type: "function", Description: "Remove null and empty-string values from an array.", Example: "const clean = arrays.compact(args);", Params: []developerJSDocParam{jsDocParam("values", "any[]", "Array-like input.", true)}, Returns: "any[]"},
			{Name: "arrays.unique(values)", Category: "arrays", Type: "function", Description: "Return unique string values while preserving first-seen order.", Example: "reply(arrays.unique(args).join(', '));", Params: []developerJSDocParam{jsDocParam("values", "any[]", "Array-like input converted to strings.", true)}, Returns: "string[]"},
			{Name: "arrays.take(values, count)", Category: "arrays", Type: "function", Description: "Return the first count array items.", Example: "reply(arrays.take(args, 3).join(', '));", Params: []developerJSDocParam{jsDocParam("values", "any[]", "Array-like input.", true), jsDocParam("count", "number", "Maximum item count. Negative values return an empty array.", true)}, Returns: "any[]"},
			{Name: "arrays.join(values, separator)", Category: "arrays", Type: "function", Description: "Join array values as strings.", Example: "reply(arrays.join(args, ', '));", Params: []developerJSDocParam{jsDocParam("values", "any[]", "Array-like input converted to strings.", true), jsDocParamDefault("separator", "string", "Separator string.", "")}, Returns: "string"},
			{Name: "arrays.includes(values, value)", Category: "arrays", Type: "function", Description: "Check whether an array contains an exact string value.", Example: "if (arrays.includes(args, 'force')) reply('forced');", Params: []developerJSDocParam{jsDocParam("values", "any[]", "Array-like input converted to strings.", true), jsDocParam("value", "any", "Exact string value to find.", true)}, Returns: "boolean"},
			{Name: "arrays.sortStrings(values)", Category: "arrays", Type: "function", Description: "Return a sorted copy of stringified array values.", Example: "reply(arrays.sortStrings(args).join(', '));", Params: []developerJSDocParam{jsDocParam("values", "any[]", "Array-like input converted to strings.", true)}, Returns: "string[]"},
			{Name: "time.now()", Category: "time", Type: "function", Description: "Return the current Unix timestamp in seconds.", Example: "reply(String(time.now()));", Returns: "number"},
			{Name: "time.formatUnix(ts)", Category: "time", Type: "function", Description: "Format a Unix timestamp as UTC RFC3339 text.", Example: "reply(time.formatUnix(ctx.command_time));", Params: []developerJSDocParam{jsDocParam("ts", "number", "Unix timestamp in seconds.", true)}, Returns: "string"},
			{Name: "time.fromNow(seconds)", Category: "time", Type: "function", Description: "Return a Unix timestamp seconds from now.", Example: "const tomorrow = time.fromNow(86400);", Params: []developerJSDocParam{jsDocParam("seconds", "number", "Offset seconds from current time. Negative values are allowed.", true)}, Returns: "number"},
			{Name: "time.addDays(ts, days)", Category: "time", Type: "function", Description: "Return a Unix timestamp after adding days to ts; ts<=0 uses current time.", Example: "const nextWeek = time.addDays(time.now(), 7);", Params: []developerJSDocParam{jsDocParam("ts", "number", "Base Unix timestamp. Values <=0 use current time.", true), jsDocParam("days", "number", "Days to add. Negative values subtract days.", true)}, Returns: "number"},
			{Name: "time.duration(seconds)", Category: "time", Type: "function", Description: "Format seconds using the backend duration helper.", Example: "reply(time.duration(3600));", Params: []developerJSDocParam{jsDocParam("seconds", "number", "Duration in seconds.", true)}, Returns: "string"},
			{Name: "format.bool(value, yes, no)", Category: "format", Type: "function", Description: "Format a boolean with custom labels.", Example: "reply(format.bool(user.active, 'enabled', 'disabled'));", Params: []developerJSDocParam{jsDocParam("value", "any", "Boolean-like value.", true), jsDocParamDefault("yes", "string", "Text for true.", "yes"), jsDocParamDefault("no", "string", "Text for false.", "no")}, Returns: "string"},
			{Name: "format.role(role)", Category: "format", Type: "function", Description: "Format numeric role as a localized role name.", Example: "reply(format.role(user.role));", Params: []developerJSDocParam{jsDocParam("role", "number", "Twilight role id.", true)}, Returns: "string"},
			{Name: "format.date(ts)", Category: "format", Type: "function", Description: "Format a Unix timestamp as UTC RFC3339 text; permanent expiry uses the expiry label.", Example: "reply(format.date(user.expired_at));", Params: []developerJSDocParam{jsDocParam("ts", "number", "Unix timestamp in seconds.", true)}, Returns: "string"},
			{Name: "format.expiry(expiredAt)", Category: "format", Type: "function", Description: "Format user expiry using the same backend expiry status helper.", Example: "reply(format.expiry(user.expired_at));", Params: []developerJSDocParam{jsDocParam("expiredAt", "number", "User expiry Unix seconds. Permanent values are supported.", true)}, Returns: "string"},
			{Name: "format.duration(seconds)", Category: "format", Type: "function", Description: "Format seconds using the backend duration helper.", Example: "reply(format.duration(7200));", Params: []developerJSDocParam{jsDocParam("seconds", "number", "Duration in seconds.", true)}, Returns: "string"},
			{Name: "format.user(user)", Category: "format", Type: "function", Description: "Format a sanitized user snapshot as a compact one-line summary.", Example: "reply(format.user(user));", Params: []developerJSDocParam{jsDocParam("user", "UserSnapshot", "Sanitized user object from users.current/list/search/get.", true)}, Returns: "string"},
			{Name: "format.json(value)", Category: "format", Type: "function", Description: "Return a bounded string representation. For structured JSON prefer native JSON.stringify(value).", Example: "reply(format.json(input.text));", Params: []developerJSDocParam{jsDocParam("value", "any", "Value converted to a bounded string.", true)}, Returns: "string"},
			{Name: "interactions.inline(text, actions)", Category: "interactions", Type: "function", Description: "Send a Telegram inline keyboard for the current command. Actions are static text objects with text plus optional answer/edit/reply fields.", Example: "interactions.inline('Choose', [{ text: 'OK', edit: 'Done' }]);", Params: []developerJSDocParam{jsDocParam("text", "string", "Message text, truncated to interaction limits.", true), jsDocParam("actions", "Array<{ text:string, answer?:string, edit?:string, reply?:string }>", "Button definitions. The sandbox stores static callback actions; it does not rerun JS on click.", true)}, Returns: "{ ok:boolean, dry_run?:boolean, message_id?:number, actions?:number, error?:string }", Mutates: true, Scope: "current_chat_owner_only"},
			{Name: "interactions.waitText(options)", Category: "interactions", Type: "function", Description: "Wait for the same Telegram user to send one non-command text message within 1-60 seconds, then reply with bounded text. Options: seconds, prompt, reply_prefix, timeout_reply, max_chars, numbered.", Example: "interactions.waitText({ seconds: 30, prompt: 'Send text', reply_prefix: 'Got:' });", Params: []developerJSDocParam{jsDocParamDefault("options.seconds", "number", "Wait window. Values are clamped to 1-60 seconds.", "30"), jsDocParamDefault("options.prompt", "string", "Optional prompt sent immediately.", ""), jsDocParamDefault("options.reply_prefix", "string", "Prefix used when replying to received text.", ""), jsDocParamDefault("options.timeout_reply", "string", "Optional timeout text.", ""), jsDocParamDefault("options.max_chars", "number", "Maximum captured text length.", "backend default"), jsDocParamDefault("options.numbered", "boolean", "Whether to prefix captured output with an item number.", "false")}, Returns: "{ ok:boolean, dry_run?:boolean, seconds?:number, error?:string }", Mutates: true, Scope: "current_chat_owner_only"},
		},
		"native_objects": []developerJSDocEntry{
			{Name: "Object", Category: "native", Type: "constructor", Description: "Native JavaScript object support from Goja.", Example: "const data = { uid: user.uid, name: user.username };"},
			{Name: "Array", Category: "native", Type: "constructor", Description: "Native JavaScript arrays. Prefer arrays.* helpers for common command output operations.", Example: "const rows = [user.username, user.role_name];"},
			{Name: "JSON", Category: "native", Type: "object", Description: "Native JSON parse/stringify support.", Example: "JSON.stringify(users.current())"},
			{Name: "Math", Category: "native", Type: "object", Description: "Native Math helpers.", Example: "const page = Math.max(1, Number(input.arg(0, 1)));"},
			{Name: "Date", Category: "native", Type: "constructor", Description: "Native Date object support. Prefer time.now/time.formatUnix for stable command output.", Example: "const iso = new Date(time.now() * 1000).toISOString();"},
			{Name: "Function / eval", Category: "native", Type: "runtime", Description: "Available through Goja for compatibility. Risky; use only in administrator-reviewed presets.", Example: "const plus = Function('a', 'b', 'return a + b');\nreply(String(plus(1, 2)));"},
			{Name: "globalThis", Category: "native", Type: "object", Description: "Bound to the Goja global object for compatibility. Does not provide browser or Node.js globals.", Example: "globalThis.tmp = 'ok';\nreply(globalThis.tmp);"},
			{Name: "String / Number / Boolean", Category: "native", Type: "constructors", Description: "Native primitive wrappers and prototype methods supported by Goja.", Example: "const uid = Number(input.named('uid', user.uid));"},
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
