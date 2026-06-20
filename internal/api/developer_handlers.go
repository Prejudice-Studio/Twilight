package api

import (
	"fmt"
	"net/http"
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
	a.audit(r, "developer_mode_activate", "admin", p.User.UID, map[string]any{"entry": "dashboard_code"})
	ok(w, "developer mode enabled", map[string]any{
		"enabled": true,
		"scope":   "browser_session",
		"features": []string{
			"telegram_js_command_docs",
			"telegram_js_sandbox_preview",
		},
	})
}

func (a *App) handleDeveloperJSSandbox(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	code := stringValue(payload, "code")
	result := validateDeveloperJSCommand(code)
	if ok, _ := result["ok"].(bool); ok {
		output, logs, err := a.telegramRunJSCustomCommand(code, telegramCommandCtx{
			ChatID:   0,
			FromID:   current(r).User.TelegramID,
			Username: current(r).User.Username,
			Args:     []string{"preview"},
		}, true)
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

func (a *App) handleDeveloperJSPresets(w http.ResponseWriter, r *http.Request, _ Params) {
	presets := a.store().ListDeveloperJSPresets()
	ok(w, "OK", map[string]any{"presets": presets, "total": len(presets)})
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
		"Allowed APIs are limited to ctx, args, user, constants, reply(text), log(text), auth(role), config(key), and env(key).",
		"config(key) and env(key) are read-only allowlists; sensitive values always return an empty string.",
	}
	if trimmed == "" {
		return map[string]any{"ok": false, "errors": []string{"code is empty"}, "warnings": warnings}
	}
	if len(trimmed) > 8000 {
		return map[string]any{"ok": false, "errors": []string{"code exceeds 8000 bytes"}, "warnings": warnings}
	}
	lower := strings.ToLower(trimmed)
	blocked := []string{
		"fetch(", "xmlhttprequest", "websocket", "eval(", "function(", "new function",
		"import(", "require(", "process.", "globalthis", "window.", "document.",
		"localstorage", "sessionstorage", "cookie", "constructor.constructor",
	}
	errors := []string{}
	for _, token := range blocked {
		if strings.Contains(lower, token) {
			errors = append(errors, "blocked token: "+token)
		}
	}
	return map[string]any{
		"ok":       len(errors) == 0,
		"errors":   errors,
		"warnings": warnings,
		"example":  "reply('Hello ' + (user.username || 'user'));",
		"bindings": []string{"ctx", "args", "user", "constants", "reply(text)", "log(text)", "auth(role)", "config(key)", "env(key)"},
	}
}
