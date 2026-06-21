package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
)

const (
	telegramJSPrefix       = "js:"
	telegramJSPresetPrefix = "preset:"
)

type developerJSRunOptions struct {
	Preview     bool
	PrivateChat bool
	Context     context.Context
}

func (a *App) telegramHandleCustomCommand(ctx context.Context, command string, c telegramCommandCtx, privateChat bool) bool {
	reply, ok := a.telegramCustomCommandReply(command)
	if !ok {
		return false
	}
	trimmed := strings.TrimSpace(reply)
	if !strings.HasPrefix(strings.ToLower(trimmed), telegramJSPrefix) {
		_ = a.telegramSendMessage(ctx, c.ChatID, a.telegramRenderText(reply))
		return true
	}

	user, _ := a.store().FindUserByTelegramID(c.FromID)
	if !a.store().DeveloperModeEnabled() {
		a.auditEntryIP("telegram", user.UID, user.Username, "telegram_js_command_blocked", "system", user.UID, map[string]any{
			"command":      telegramCommand(command),
			"reason":       "developer_mode_disabled",
			"private_chat": privateChat,
		})
		_ = a.telegramSendMessage(ctx, c.ChatID, "Developer mode is disabled. Custom JS commands are blocked.")
		return true
	}
	script, presetID, err := a.telegramResolveJSCustomCommand(strings.TrimSpace(trimmed[len(telegramJSPrefix):]))
	if err != nil {
		a.auditEntryIP("telegram", user.UID, user.Username, "telegram_js_command_blocked", "system", user.UID, map[string]any{
			"command":      telegramCommand(command),
			"reason":       err.Error(),
			"preset_id":    valueOrNilInt64(presetID),
			"private_chat": privateChat,
		})
		_ = a.telegramSendMessage(ctx, c.ChatID, "Custom JS command is not available. Please contact an administrator.")
		return true
	}
	c.Command = telegramCommand(command)
	text, logs, err := a.telegramRunJSCustomCommandWithContext(ctx, script, c, privateChat)
	detail := map[string]any{"command": telegramCommand(command), "ok": err == nil, "private_chat": privateChat}
	if presetID > 0 {
		detail["preset_id"] = presetID
	}
	if len(logs) > 0 {
		detail["logs"] = logs
	}
	a.auditEntryIP("telegram", user.UID, user.Username, "telegram_js_command_execute", "system", user.UID, detail)
	if err != nil {
		_ = a.telegramSendMessage(ctx, c.ChatID, "自定义指令执行失败，请联系管理员查看安全审计。")
		return true
	}
	if strings.TrimSpace(text) == "" {
		text = "自定义指令已执行。"
	}
	_ = a.telegramSendMessage(ctx, c.ChatID, a.telegramRenderText(text))
	return true
}

func (a *App) telegramResolveJSCustomCommand(script string) (string, int64, error) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(script)), telegramJSPresetPrefix) {
		return strings.TrimSpace(script), 0, nil
	}
	rawID := strings.TrimSpace(script[len(telegramJSPresetPrefix):])
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		return "", 0, fmt.Errorf("invalid_preset_reference")
	}
	preset, ok := a.store().DeveloperJSPreset(id)
	if !ok || strings.TrimSpace(preset.Code) == "" {
		return "", id, fmt.Errorf("preset_not_found")
	}
	return strings.TrimSpace(preset.Code), id, nil
}

func valueOrNilInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func (a *App) telegramRunJSCustomCommand(code string, c telegramCommandCtx, privateChat bool) (string, []string, error) {
	return a.telegramRunJSCustomCommandWithOptions(code, c, privateChat, developerJSRunOptions{})
}

func (a *App) telegramRunJSCustomCommandWithContext(ctx context.Context, code string, c telegramCommandCtx, privateChat bool) (string, []string, error) {
	return a.telegramRunJSCustomCommandWithOptions(code, c, privateChat, developerJSRunOptions{Context: ctx})
}

func (a *App) telegramRunJSCustomCommandWithOptions(code string, c telegramCommandCtx, privateChat bool, opts developerJSRunOptions) (output string, logs []string, runErr error) {
	defer func() {
		if r := recover(); r != nil {
			runErr = fmt.Errorf("developer js runtime panic: %s", truncateString(redactSensitiveText(fmt.Sprint(r)), 160))
		}
	}()
	result := validateDeveloperJSCommand(code)
	if ok, _ := result["ok"].(bool); !ok {
		return "", nil, fmt.Errorf("developer js command rejected: %v", result["errors"])
	}
	program, err := goja.Compile("telegram_custom_command.js", code, false)
	if err != nil {
		return "", nil, developerJSSafeError(err)
	}
	if opts.Context == nil {
		opts.Context = context.Background()
	}

	user, _ := a.store().FindUserByTelegramID(c.FromID)
	vm := goja.New()
	replies := make([]string, 0, 4)
	logs = make([]string, 0, 8)
	commandName := telegramCommand(c.Command)
	_ = vm.Set("ctx", map[string]any{
		"private_chat": privateChat,
		"command_time": time.Now().Unix(),
		"preview":      opts.Preview,
		"command":      commandName,
	})
	_ = vm.Set("command", map[string]any{
		"name":         commandName,
		"args":         c.Args,
		"text":         strings.Join(c.Args, " "),
		"private_chat": privateChat,
		"preview":      opts.Preview,
		"from_id":      c.FromID != 0,
	})
	_ = vm.Set("args", c.Args)
	_ = vm.Set("globalThis", vm.GlobalObject())
	_ = vm.Set("user", developerJSUserSnapshot(user))
	_ = vm.Set("constants", map[string]any{
		"roles": map[string]int{
			"admin":     int(store.RoleAdmin),
			"user":      int(store.RoleNormal),
			"whitelist": int(store.RoleWhitelist),
		},
		"limits": map[string]int{
			"max_replies": 4,
			"max_logs":    8,
		},
	})
	opts.PrivateChat = privateChat
	_ = vm.Set("db", a.developerJSDBAPI(vm, &user, opts, &logs))
	_ = vm.Set("users", a.developerJSUsersAPI(vm, &user, opts, &logs))
	_ = vm.Set("getUser", func(call goja.FunctionCall) goja.Value {
		return a.developerJSGetUserByUID(vm, &user, call.Argument(0), &logs)
	})
	_ = vm.Set("text", developerJSTextAPI(vm))
	_ = vm.Set("arrays", developerJSArraysAPI(vm))
	_ = vm.Set("time", developerJSTimeAPI(vm))
	_ = vm.Set("interactions", a.developerJSInteractionsAPI(vm, c, opts, &logs))
	_ = vm.Set("reply", func(call goja.FunctionCall) goja.Value {
		if len(replies) < 4 {
			replies = append(replies, developerJSLimitText(call.Argument(0).String(), 1200))
		}
		return goja.Undefined()
	})
	_ = vm.Set("log", func(call goja.FunctionCall) goja.Value {
		if len(logs) < 8 {
			logs = append(logs, developerJSLimitText(call.Argument(0).String(), 240))
		}
		return goja.Undefined()
	})
	_ = vm.Set("auth", func(call goja.FunctionCall) goja.Value {
		role := strings.ToLower(strings.TrimSpace(call.Argument(0).String()))
		allowed := false
		switch role {
		case "admin", "0":
			allowed = user.Role == store.RoleAdmin
		case "whitelist", "2":
			allowed = user.Role == store.RoleAdmin || user.Role == store.RoleWhitelist
		case "user", "1":
			allowed = user.Role == store.RoleAdmin || user.Role == store.RoleWhitelist || user.Role == store.RoleNormal
		default:
			allowed = false
		}
		return vm.ToValue(allowed)
	})
	_ = vm.Set("authAdmin", func(goja.FunctionCall) goja.Value {
		return vm.ToValue(user.Role == store.RoleAdmin)
	})
	_ = vm.Set("fetch", a.developerJSFetchAPI(vm, opts, &logs))
	_ = vm.Set("setTimeout", developerJSTimerAPI(vm, &logs, "setTimeout"))
	_ = vm.Set("setInterval", developerJSTimerAPI(vm, &logs, "setInterval"))
	_ = vm.Set("clearTimeout", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = vm.Set("clearInterval", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = vm.Set("config", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		value, ok := developerJSConfigValue(a.cfg(), key)
		if !ok && len(logs) < 8 {
			logs = append(logs, "config denied: "+strings.TrimSpace(key))
		}
		return vm.ToValue(value)
	})
	_ = vm.Set("env", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		value, ok := developerJSEnvValue(key)
		if !ok && len(logs) < 8 {
			logs = append(logs, "env denied: "+strings.TrimSpace(key))
		}
		return vm.ToValue(value)
	})

	timer := time.AfterFunc(200*time.Millisecond, func() {
		vm.Interrupt("execution timeout")
	})
	defer timer.Stop()
	if _, err := vm.RunProgram(program); err != nil {
		return "", logs, developerJSSafeError(err)
	}
	return strings.Join(replies, "\n"), logs, nil
}

func developerJSSafeError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", truncateString(redactSensitiveText(err.Error()), 300))
}

func developerJSLimitText(value string, limit int) string {
	if limit <= 0 {
		limit = 240
	}
	return truncateString(redactSensitiveText(strings.TrimSpace(value)), limit)
}

func developerJSUserSnapshot(user store.User) map[string]any {
	return map[string]any{
		"uid":                      user.UID,
		"username":                 user.Username,
		"role":                     user.Role,
		"active":                   user.Active,
		"expired_at":               zeroNil(user.ExpiredAt),
		"created_at":               zeroNil(user.CreatedAt),
		"register_time":            zeroNil(user.RegisterTime),
		"has_emby":                 strings.TrimSpace(user.EmbyID) != "",
		"emby_disabled":            user.EmbyDisabled,
		"email_verified":           user.EmailVerified,
		"email_verified_at":        zeroNil(user.EmailVerifiedAt),
		"telegram_bound":           user.TelegramID != 0,
		"notify_on_login_telegram": user.NotifyOnLoginTelegram,
		"notify_on_login_email":    user.NotifyOnLoginEmail,
	}
}

func (a *App) developerJSGetUserByUID(vm *goja.Runtime, current *store.User, uidValue goja.Value, logs *[]string) goja.Value {
	uid := uidValue.ToInteger()
	if uid <= 0 {
		return goja.Null()
	}
	if current == nil || current.UID == 0 {
		if len(*logs) < 8 {
			*logs = append(*logs, "getUser denied: no bound user")
		}
		return goja.Null()
	}
	if current.UID != uid && current.Role != store.RoleAdmin {
		if len(*logs) < 8 {
			*logs = append(*logs, "getUser denied: admin role required for other users")
		}
		return goja.Null()
	}
	target, ok := a.store().User(uid)
	if !ok {
		return goja.Null()
	}
	return vm.ToValue(developerJSUserSnapshot(target))
}

func (a *App) developerJSUsersAPI(vm *goja.Runtime, user *store.User, opts developerJSRunOptions, logs *[]string) map[string]any {
	hasRole := func(role string) bool {
		switch strings.ToLower(strings.TrimSpace(role)) {
		case "admin", "0":
			return user.Role == store.RoleAdmin
		case "whitelist", "2":
			return user.Role == store.RoleAdmin || user.Role == store.RoleWhitelist
		case "user", "1":
			return user.Role == store.RoleAdmin || user.Role == store.RoleWhitelist || user.Role == store.RoleNormal
		default:
			return false
		}
	}
	return map[string]any{
		"current": func(goja.FunctionCall) goja.Value {
			return vm.ToValue(developerJSUserSnapshot(*user))
		},
		"describe": func(goja.FunctionCall) goja.Value {
			return vm.ToValue(developerJSUserSnapshot(*user))
		},
		"get": func(call goja.FunctionCall) goja.Value {
			return a.developerJSGetUserByUID(vm, user, call.Argument(0), logs)
		},
		"byUID": func(call goja.FunctionCall) goja.Value {
			return a.developerJSGetUserByUID(vm, user, call.Argument(0), logs)
		},
		"hasRole": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(hasRole(call.Argument(0).String()))
		},
		"requireActive": func(goja.FunctionCall) goja.Value {
			return vm.ToValue(user.UID != 0 && user.Active)
		},
		"setLoginNotify": func(call goja.FunctionCall) goja.Value {
			result := map[string]any{"ok": false}
			telegram, hasTelegram := developerJSBoolOption(call.Argument(0).Export(), "telegram")
			email, hasEmail := developerJSBoolOption(call.Argument(0).Export(), "email")
			if !hasTelegram && !hasEmail {
				result["error"] = "invalid_options"
				return vm.ToValue(result)
			}
			if user.UID == 0 {
				result["error"] = "no_bound_user"
				return vm.ToValue(result)
			}
			result["uid"] = user.UID
			if hasTelegram {
				result["telegram"] = telegram
			}
			if hasEmail {
				result["email"] = email
			}
			if opts.Preview {
				result["dry_run"] = true
				result["ok"] = true
				return vm.ToValue(result)
			}
			updated, err := a.store().UpdateUser(user.UID, func(u *store.User) error {
				if hasTelegram {
					u.NotifyOnLoginTelegram = telegram
				}
				if hasEmail {
					u.NotifyOnLoginEmail = email
				}
				return nil
			})
			if err != nil {
				result["error"] = err.Error()
				return vm.ToValue(result)
			}
			*user = updated
			result["ok"] = true
			if len(*logs) < 8 {
				*logs = append(*logs, "users.setLoginNotify updated current user")
			}
			a.auditEntryIP("telegram", updated.UID, updated.Username, "telegram_js_user_notify_update", "user", updated.UID, map[string]any{
				"telegram":     valueOrNil(hasTelegram, telegram),
				"email":        valueOrNil(hasEmail, email),
				"script_api":   "users.setLoginNotify",
				"private_chat": opts.PrivateChat,
			})
			return vm.ToValue(result)
		},
	}
}

func (a *App) developerJSDBAPI(vm *goja.Runtime, user *store.User, opts developerJSRunOptions, logs *[]string) map[string]any {
	return map[string]any{
		"schema": func(goja.FunctionCall) goja.Value {
			return vm.ToValue(developerJSDBSchema())
		},
		"collections": func(goja.FunctionCall) goja.Value {
			return vm.ToValue([]string{"users", "regcodes", "invite_codes", "media_requests", "announcements", "tickets", "developer_js_presets"})
		},
		"count": func(call goja.FunctionCall) goja.Value {
			name := strings.ToLower(strings.TrimSpace(call.Argument(0).String()))
			count, ok := a.developerJSDBCount(name, user)
			if !ok {
				if len(*logs) < 8 {
					*logs = append(*logs, "db.count denied: "+name)
				}
				return vm.ToValue(-1)
			}
			return vm.ToValue(count)
		},
		"currentUser": func(goja.FunctionCall) goja.Value {
			return vm.ToValue(developerJSUserSnapshot(*user))
		},
		"getUser": func(call goja.FunctionCall) goja.Value {
			return a.developerJSGetUserByUID(vm, user, call.Argument(0), logs)
		},
		"updateCurrentUser": func(call goja.FunctionCall) goja.Value {
			return a.developerJSUpdateCurrentUser(vm, user, opts, logs, call.Argument(0).Export())
		},
	}
}

func developerJSDBSchema() map[string]any {
	userFields := []string{
		"uid", "username", "role", "active", "expired_at", "created_at", "register_time",
		"has_emby", "emby_disabled", "email_verified", "email_verified_at", "telegram_bound",
		"notify_on_login_telegram", "notify_on_login_email",
	}
	return map[string]any{
		"users": map[string]any{
			"read":   "current user, or exact UID lookup when current user is admin",
			"write":  "current user notification preferences only",
			"fields": userFields,
		},
		"regcodes":             map[string]any{"read": "admin count only", "write": "not available"},
		"invite_codes":         map[string]any{"read": "admin count only", "write": "not available"},
		"media_requests":       map[string]any{"read": "current user count; admin count", "write": "not available"},
		"announcements":        map[string]any{"read": "visible count", "write": "not available"},
		"tickets":              map[string]any{"read": "current user count; admin count", "write": "not available"},
		"developer_js_presets": map[string]any{"read": "admin count only", "write": "manage through developer preset APIs"},
	}
}

func (a *App) developerJSDBCount(name string, user *store.User) (int, bool) {
	admin := user != nil && user.Role == store.RoleAdmin
	switch name {
	case "users":
		if !admin {
			return 0, false
		}
		return len(a.store().ListUsers()), true
	case "regcodes":
		if !admin {
			return 0, false
		}
		return len(a.store().ListRegCodes()), true
	case "invite_codes":
		if !admin {
			return 0, false
		}
		return a.store().CountInviteCodes(), true
	case "media_requests":
		if admin {
			return len(a.store().ListMediaRequests(0, true)), true
		}
		if user == nil || user.UID == 0 {
			return 0, false
		}
		return len(a.store().ListMediaRequests(user.UID, false)), true
	case "announcements":
		return len(a.store().ListAnnouncements(false)), true
	case "tickets":
		if admin {
			return len(a.store().ListTickets(store.TicketFilter{})), true
		}
		if user == nil || user.UID == 0 {
			return 0, false
		}
		return len(a.store().ListTickets(store.TicketFilter{UID: user.UID})), true
	case "developer_js_presets":
		if !admin {
			return 0, false
		}
		return len(a.store().ListDeveloperJSPresets()), true
	default:
		return 0, false
	}
}

func (a *App) developerJSUpdateCurrentUser(vm *goja.Runtime, user *store.User, opts developerJSRunOptions, logs *[]string, patch any) goja.Value {
	result := map[string]any{"ok": false}
	if user == nil || user.UID == 0 {
		result["error"] = "no_bound_user"
		return vm.ToValue(result)
	}
	telegram, hasTelegram := developerJSBoolOption(patch, "notify_on_login_telegram")
	if !hasTelegram {
		telegram, hasTelegram = developerJSBoolOption(patch, "telegram")
	}
	email, hasEmail := developerJSBoolOption(patch, "notify_on_login_email")
	if !hasEmail {
		email, hasEmail = developerJSBoolOption(patch, "email")
	}
	if !hasTelegram && !hasEmail {
		result["error"] = "invalid_patch"
		return vm.ToValue(result)
	}
	result["uid"] = user.UID
	if hasTelegram {
		result["notify_on_login_telegram"] = telegram
	}
	if hasEmail {
		result["notify_on_login_email"] = email
	}
	if opts.Preview {
		result["dry_run"] = true
		result["ok"] = true
		return vm.ToValue(result)
	}
	updated, err := a.store().UpdateUser(user.UID, func(u *store.User) error {
		if hasTelegram {
			u.NotifyOnLoginTelegram = telegram
		}
		if hasEmail {
			u.NotifyOnLoginEmail = email
		}
		return nil
	})
	if err != nil {
		result["error"] = err.Error()
		return vm.ToValue(result)
	}
	*user = updated
	result["ok"] = true
	if len(*logs) < 8 {
		*logs = append(*logs, "db.updateCurrentUser updated current user")
	}
	a.auditEntryIP("telegram", updated.UID, updated.Username, "telegram_js_db_user_update", "user", updated.UID, map[string]any{
		"telegram":     valueOrNil(hasTelegram, telegram),
		"email":        valueOrNil(hasEmail, email),
		"script_api":   "db.updateCurrentUser",
		"private_chat": opts.PrivateChat,
	})
	return vm.ToValue(result)
}

func developerJSBoolOption(input any, key string) (bool, bool) {
	values, ok := input.(map[string]any)
	if !ok {
		return false, false
	}
	value, ok := values[key]
	if !ok {
		return false, false
	}
	typed, ok := value.(bool)
	return typed, ok
}

func valueOrNil(ok bool, value bool) any {
	if !ok {
		return nil
	}
	return value
}

func developerJSTextAPI(vm *goja.Runtime) map[string]any {
	return map[string]any{
		"truncate": func(call goja.FunctionCall) goja.Value {
			value := call.Argument(0).String()
			limit := int(call.Argument(1).ToInteger())
			if limit <= 0 {
				limit = 80
			}
			return vm.ToValue(truncateString(value, limit))
		},
		"joinLines": func(call goja.FunctionCall) goja.Value {
			items := developerJSStringSlice(call.Argument(0).Export())
			return vm.ToValue(strings.Join(items, "\n"))
		},
		"escape": func(call goja.FunctionCall) goja.Value {
			value := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(call.Argument(0).String())
			return vm.ToValue(value)
		},
		"numberLines": func(call goja.FunctionCall) goja.Value {
			items := developerJSStringSlice(call.Argument(0).Export())
			lines := make([]string, 0, len(items))
			for i, item := range items {
				lines = append(lines, fmt.Sprintf("%d. %s", i+1, item))
			}
			return vm.ToValue(strings.Join(lines, "\n"))
		},
	}
}

func developerJSArraysAPI(vm *goja.Runtime) map[string]any {
	return map[string]any{
		"first": func(call goja.FunctionCall) goja.Value {
			items := developerJSAnySlice(call.Argument(0).Export())
			if len(items) == 0 {
				return goja.Undefined()
			}
			return vm.ToValue(items[0])
		},
		"compact": func(call goja.FunctionCall) goja.Value {
			items := developerJSAnySlice(call.Argument(0).Export())
			out := make([]any, 0, len(items))
			for _, item := range items {
				if item == nil || item == "" {
					continue
				}
				out = append(out, item)
			}
			return vm.ToValue(out)
		},
		"unique": func(call goja.FunctionCall) goja.Value {
			items := developerJSStringSlice(call.Argument(0).Export())
			seen := map[string]bool{}
			out := make([]string, 0, len(items))
			for _, item := range items {
				if seen[item] {
					continue
				}
				seen[item] = true
				out = append(out, item)
			}
			return vm.ToValue(out)
		},
		"take": func(call goja.FunctionCall) goja.Value {
			items := developerJSAnySlice(call.Argument(0).Export())
			limit := int(call.Argument(1).ToInteger())
			if limit < 0 {
				limit = 0
			}
			if limit > len(items) {
				limit = len(items)
			}
			return vm.ToValue(items[:limit])
		},
	}
}

func (a *App) developerJSInteractionsAPI(vm *goja.Runtime, c telegramCommandCtx, opts developerJSRunOptions, logs *[]string) map[string]any {
	return map[string]any{
		"inline": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSInline(opts.Context, c, opts, call.Argument(0).String(), call.Argument(1).Export(), logs))
		},
		"waitText": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSWaitText(opts.Context, c, opts, call.Argument(0).Export(), logs))
		},
	}
}

func (a *App) developerJSInline(ctx context.Context, c telegramCommandCtx, opts developerJSRunOptions, text string, rawActions any, logs *[]string) map[string]any {
	result := map[string]any{"ok": false}
	actions := developerJSCallbackActions(rawActions)
	if len(actions) == 0 {
		result["error"] = "no_actions"
		return result
	}
	if len(actions) > developerJSMaxInlineButtons {
		actions = actions[:developerJSMaxInlineButtons]
	}
	text = developerJSLimitText(text, developerJSMaxInteractionChars)
	if text == "" {
		result["error"] = "empty_text"
		return result
	}
	if opts.Preview || c.ChatID == 0 {
		result["ok"] = true
		result["dry_run"] = true
		result["actions"] = len(actions)
		return result
	}
	token := telegramRandomToken()
	rows := make([][]telegramInlineButton, 0, len(actions))
	for i, action := range actions {
		rows = append(rows, []telegramInlineButton{{Text: action.Text, Data: fmt.Sprintf("djs:%s:%d", token, i)}})
	}
	messageID, err := a.telegramSendMessageWithMarkup(ctx, c.ChatID, text, telegramInlineKeyboard(rows))
	if err != nil {
		result["error"] = developerJSSafeError(err).Error()
		return result
	}
	a.saveDeveloperJSCallback(developerJSCallbackContext{
		Token:           token,
		ChatID:          c.ChatID,
		MessageID:       messageID,
		OwnerTelegramID: c.FromID,
		ExpiresAt:       time.Now().Add(developerJSInteractionTTL).Unix(),
		Actions:         actions,
	})
	result["ok"] = true
	result["message_id"] = messageID
	result["actions"] = len(actions)
	if len(*logs) < 8 {
		*logs = append(*logs, "interactions.inline sent")
	}
	return result
}

func developerJSCallbackActions(input any) []developerJSCallbackAction {
	values := developerJSAnySlice(input)
	out := make([]developerJSCallbackAction, 0, len(values))
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		text := developerJSLimitText(fmt.Sprint(item["text"]), 40)
		if text == "" {
			continue
		}
		out = append(out, developerJSCallbackAction{
			Text:   text,
			Answer: developerJSLimitText(developerJSMapString(item, "answer"), 190),
			Edit:   developerJSLimitText(developerJSMapString(item, "edit"), developerJSMaxInteractionChars),
			Reply:  developerJSLimitText(developerJSMapString(item, "reply"), developerJSMaxInteractionChars),
		})
	}
	return out
}

func developerJSMapString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func (a *App) developerJSWaitText(ctx context.Context, c telegramCommandCtx, opts developerJSRunOptions, rawOptions any, logs *[]string) map[string]any {
	result := map[string]any{"ok": false}
	values, _ := rawOptions.(map[string]any)
	seconds := int64(30)
	if values != nil {
		if raw, ok := values["seconds"]; ok {
			seconds = int64(numeric(raw))
		}
	}
	if seconds <= 0 {
		seconds = 30
	}
	if seconds > developerJSWaitMaxSeconds {
		seconds = developerJSWaitMaxSeconds
	}
	if opts.Preview || c.ChatID == 0 || c.FromID == 0 {
		result["ok"] = true
		result["dry_run"] = true
		result["seconds"] = seconds
		return result
	}
	item := developerJSMessageWaiter{
		Key:          developerJSWaiterKey(c.ChatID, c.FromID),
		ChatID:       c.ChatID,
		FromID:       c.FromID,
		ExpiresAt:    time.Now().Add(time.Duration(seconds) * time.Second).Unix(),
		ReplyPrefix:  developerJSLimitText(developerJSMapString(values, "reply_prefix"), 240),
		TimeoutReply: developerJSLimitText(developerJSMapString(values, "timeout_reply"), 240),
		MaxChars:     int(numeric(values["max_chars"])),
		Numbered:     boolish(values["numbered"]),
	}
	a.saveDeveloperJSWaiter(item)
	if prompt := developerJSLimitText(developerJSMapString(values, "prompt"), 600); prompt != "" {
		_ = a.telegramSendMessage(ctx, c.ChatID, prompt)
	}
	result["ok"] = true
	result["seconds"] = seconds
	if len(*logs) < 8 {
		*logs = append(*logs, "interactions.waitText armed")
	}
	return result
}

func developerJSTimeAPI(vm *goja.Runtime) map[string]any {
	return map[string]any{
		"now": func(goja.FunctionCall) goja.Value {
			return vm.ToValue(time.Now().Unix())
		},
		"formatUnix": func(call goja.FunctionCall) goja.Value {
			ts := call.Argument(0).ToInteger()
			if ts <= 0 {
				return vm.ToValue("")
			}
			return vm.ToValue(time.Unix(ts, 0).UTC().Format(time.RFC3339))
		},
	}
}

func developerJSTimerAPI(vm *goja.Runtime, logs *[]string, name string) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			return vm.ToValue(0)
		}
		delay := call.Argument(1).ToInteger()
		if len(*logs) < 8 {
			*logs = append(*logs, fmt.Sprintf("%s executed synchronously; requested delay=%dms", name, delay))
		}
		_, _ = fn(goja.Undefined())
		return vm.ToValue(1)
	}
}

func (a *App) developerJSFetchAPI(vm *goja.Runtime, opts developerJSRunOptions, logs *[]string) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		rawURL := strings.TrimSpace(call.Argument(0).String())
		if rawURL == "" {
			return vm.ToValue(map[string]any{"ok": false, "blocked": true, "error": "empty_url"})
		}
		if len(*logs) < 8 {
			*logs = append(*logs, "fetch called")
		}
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Hostname() == "" {
			return vm.ToValue(map[string]any{"ok": false, "blocked": true, "error": "invalid_url"})
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return vm.ToValue(map[string]any{"ok": false, "blocked": true, "error": "scheme_not_allowed"})
		}
		if err := developerJSValidateFetchHost(opts.Context, parsed.Hostname()); err != nil {
			return vm.ToValue(map[string]any{"ok": false, "blocked": true, "error": err.Error()})
		}
		method := "GET"
		if options, ok := call.Argument(1).Export().(map[string]any); ok {
			if rawMethod, ok := options["method"]; ok {
				method = strings.ToUpper(strings.TrimSpace(fmt.Sprint(rawMethod)))
			}
		}
		if method == "" {
			method = "GET"
		}
		if method != "GET" && method != "POST" && method != "HEAD" {
			return vm.ToValue(map[string]any{"ok": false, "blocked": true, "error": "method_not_allowed"})
		}
		ctx := opts.Context
		if ctx == nil {
			ctx = context.Background()
		}
		reqCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		defer cancel()
		req, err := http.NewRequestWithContext(reqCtx, method, parsed.String(), nil)
		if err != nil {
			return vm.ToValue(map[string]any{"ok": false, "blocked": true, "error": "request_failed"})
		}
		req.Header.Set("User-Agent", "TwilightDeveloperJS/1.0")
		client := http.Client{
			Timeout: 1500 * time.Millisecond,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			return vm.ToValue(map[string]any{"ok": false, "error": developerJSSafeError(err).Error()})
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return vm.ToValue(map[string]any{
			"ok":         resp.StatusCode >= 200 && resp.StatusCode < 300,
			"status":     resp.StatusCode,
			"statusText": resp.Status,
			"text":       developerJSLimitText(string(body), 4096),
			"truncated":  len(body) >= 8192,
		})
	}
}

func developerJSValidateFetchHost(ctx context.Context, host string) error {
	lower := strings.ToLower(strings.TrimSpace(host))
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return fmt.Errorf("host_not_allowed")
	}
	if ip := net.ParseIP(lower); ip != nil {
		if developerJSPrivateIP(ip) {
			return fmt.Errorf("host_not_allowed")
		}
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("host_lookup_failed")
	}
	for _, addr := range ips {
		if developerJSPrivateIP(addr.IP) {
			return fmt.Errorf("host_not_allowed")
		}
	}
	return nil
}

func developerJSPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

func developerJSAnySlice(input any) []any {
	values, ok := input.([]any)
	if ok {
		return values
	}
	return nil
}

func developerJSStringSlice(input any) []string {
	values := developerJSAnySlice(input)
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, fmt.Sprint(value))
	}
	return out
}

func developerJSConfigValue(cfg *config.Config, key string) (any, bool) {
	if cfg == nil {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "app.name", "site.name", "global.server_name":
		return cfg.AppName, true
	case "app.version":
		return cfg.Version, true
	case "telegram.enabled", "global.telegram_mode":
		return cfg.TelegramMode, true
	case "telegram.force_bind", "global.force_bind_telegram":
		return cfg.ForceBindTelegram, true
	case "telegram.require_membership":
		return cfg.TelegramRequireMembership, true
	case "telegram.panel_enabled":
		return cfg.TelegramEnablePanel, true
	case "telegram.ban_on_leave":
		return cfg.TelegramBanOnLeave, true
	case "invite.enabled":
		return cfg.InviteEnabled, true
	case "invite.max_depth":
		return cfg.InviteMaxDepth, true
	case "invite.limit":
		return cfg.InviteLimit, true
	case "invite.root_user_limit":
		return cfg.InviteRootUserLimit, true
	case "email.enabled":
		return cfg.EmailEnabled, true
	case "email.force_bind":
		return cfg.EmailForceBind, true
	case "media_request.enabled":
		return cfg.MediaRequestEnabled, true
	case "signin.enabled":
		return cfg.SigninEnabled, true
	case "ticket.enabled":
		return cfg.TicketSystemEnabled, true
	case "limits.user":
		return cfg.UserLimit, true
	case "limits.emby_user":
		return cfg.EmbyUserLimit, true
	default:
		return "", false
	}
}

func developerJSEnvValue(key string) (string, bool) {
	normalized := strings.ToUpper(strings.TrimSpace(key))
	switch normalized {
	case "TWILIGHT_APP_NAME",
		"TWILIGHT_SERVER_NAME",
		"TWILIGHT_HOST",
		"TWILIGHT_PORT",
		"TWILIGHT_BASE_URL",
		"TWILIGHT_DATABASE_DRIVER",
		"TWILIGHT_EMAIL_ENABLED",
		"TWILIGHT_TELEGRAM_REQUIRE_GROUP_MEMBERSHIP",
		"TWILIGHT_TELEGRAM_BAN_ON_LEAVE",
		"TWILIGHT_INVITE_ENABLED",
		"TWILIGHT_MEDIA_REQUEST_ENABLED":
		return os.Getenv(normalized), true
	default:
		return "", false
	}
}
