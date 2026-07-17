package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dop251/goja"
	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
)

const (
	telegramJSPrefix       = "js:"
	telegramJSPresetPrefix = "preset:"
	// developerJSExecutionTimeout caps wall-clock execution of a single custom
	// command so a runaway loop cannot spin a goroutine forever. It must stay
	// comfortably larger than the synchronous network budgets the sandbox APIs
	// allow (fetch: 1500ms HTTP + 500ms DNS validation; interactions send real
	// Telegram messages). A 200ms cap previously interrupted every command that
	// touched the network, which broke developer-mode JS commands entirely.
	developerJSExecutionTimeout = 8 * time.Second
)

type developerJSRunOptions struct {
	Preview     bool
	PrivateChat bool
	Context     context.Context
}

type developerJSExitSignal struct{}

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
			"preset_id":    zeroNil(presetID),
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
	program, err := goja.Compile("telegram_custom_command.js", "(function(){\n"+code+"\n})();", false)
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
	userSnapshot := developerJSUserSnapshot(user)
	roles := map[string]int{
		"admin":     int(store.RoleAdmin),
		"user":      int(store.RoleNormal),
		"whitelist": int(store.RoleWhitelist),
	}
	_ = vm.Set("user", userSnapshot)
	_ = vm.Set("me", userSnapshot)
	_ = vm.Set("constants", map[string]any{
		"roles": roles,
		"limits": map[string]int{
			"max_replies": 4,
			"max_logs":    8,
		},
	})
	_ = vm.Set("roles", roles)
	opts.PrivateChat = privateChat
	_ = vm.Set("db", a.developerJSDBAPI(vm, &user, opts, &logs))
	_ = vm.Set("users", a.developerJSUsersAPI(vm, &user, opts, &logs))
	_ = vm.Set("admin", a.developerJSAdminAPI(vm, &user, opts, &logs))
	_ = vm.Set("regcodes", a.developerJSRegcodesAPI(vm, &user, opts, &logs))
	_ = vm.Set("invites", a.developerJSInvitesAPI(vm, &user, opts, &logs))
	_ = vm.Set("announcements", a.developerJSAnnouncementsAPI(vm, &user, opts, &logs))
	_ = vm.Set("system", a.developerJSSystemAPI(vm, &user))
	_ = vm.Set("input", developerJSInputAPI(vm, c.Args, commandName, privateChat, opts.Preview))
	_ = vm.Set("getUser", func(call goja.FunctionCall) goja.Value {
		return a.developerJSGetUserByUID(vm, &user, call.Argument(0), &logs)
	})
	_ = vm.Set("text", developerJSTextAPI(vm))
	_ = vm.Set("arrays", developerJSArraysAPI(vm))
	_ = vm.Set("time", developerJSTimeAPI(vm))
	_ = vm.Set("format", developerJSFormatAPI(vm))
	_ = vm.Set("interactions", a.developerJSInteractionsAPI(vm, c, opts, &logs))
	_ = vm.Set("sendMessage", func(call goja.FunctionCall) goja.Value {
		chatID := call.Argument(0).Export()
		msgText := developerJSLimitText(call.Argument(1).String(), 3900)
		if strings.TrimSpace(msgText) == "" {
			return vm.ToValue(false)
		}
		pm := ""
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
			p := strings.TrimSpace(call.Argument(2).String())
			if p == "Markdown" || p == "MarkdownV2" || p == "HTML" {
				pm = p
			}
		}
		body := map[string]any{"chat_id": chatID, "text": msgText, "disable_web_page_preview": true}
		if pm != "" {
			body["parse_mode"] = pm
		}
		var result map[string]any
		if err := a.telegramPost(opts.Context, "sendMessage", body, &result); err != nil {
			if len(logs) < 8 {
				logs = append(logs, "sendMessage error: "+truncateString(err.Error(), 200))
			}
			return vm.ToValue(false)
		}
		return vm.ToValue(numeric(result["message_id"]))
	})
	_ = vm.Set("reply", func(call goja.FunctionCall) goja.Value {
		if len(replies) < 4 {
			replies = append(replies, developerJSLimitText(call.Argument(0).String(), 1200))
		}
		return goja.Undefined()
	})
	exitWithMessage := func(message string) {
		if strings.TrimSpace(message) != "" && len(replies) < 4 {
			replies = append(replies, developerJSLimitText(message, 1200))
		}
		if len(logs) < 8 {
			logs = append(logs, "exit called")
		}
		vm.Interrupt(developerJSExitSignal{})
	}
	_ = vm.Set("exit", func(call goja.FunctionCall) goja.Value {
		message := ""
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			message = call.Argument(0).String()
		}
		exitWithMessage(message)
		return goja.Undefined()
	})
	_ = vm.Set("assert", func(call goja.FunctionCall) goja.Value {
		if boolish(call.Argument(0).Export()) {
			return vm.ToValue(true)
		}
		message := "assertion failed"
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			message = call.Argument(1).String()
		}
		exitWithMessage(message)
		return vm.ToValue(false)
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

	timer := time.AfterFunc(developerJSExecutionTimeout, func() {
		vm.Interrupt("execution timeout")
	})
	defer timer.Stop()
	// Also interrupt promptly if the caller's context is cancelled (e.g. the
	// Telegram update was abandoned) so we never keep the VM spinning after the
	// request is gone.
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-opts.Context.Done():
			vm.Interrupt("context cancelled")
		case <-watchDone:
		}
	}()
	if _, err := vm.RunProgram(program); err != nil {
		if developerJSWasExit(err) {
			return strings.Join(replies, "\n"), logs, nil
		}
		return "", logs, developerJSSafeError(err)
	}
	return strings.Join(replies, "\n"), logs, nil
}

func developerJSWasExit(err error) bool {
	var interrupted *goja.InterruptedError
	if !errors.As(err, &interrupted) {
		return false
	}
	_, ok := interrupted.Value().(developerJSExitSignal)
	return ok
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
	hasEmail := strings.TrimSpace(user.Email) != ""
	return map[string]any{
		"uid":                      user.UID,
		"username":                 user.Username,
		"email":                    user.Email,
		"email_masked":             maskEmail(user.Email),
		"has_email":                hasEmail,
		"role":                     user.Role,
		"role_name":                roleName(user.Role),
		"active":                   user.Active,
		"expired_at":               zeroNil(user.ExpiredAt),
		"expire_status":            expireStatus(user.ExpiredAt),
		"created_at":               zeroNil(user.CreatedAt),
		"register_time":            zeroNil(user.RegisterTime),
		"has_emby":                 strings.TrimSpace(user.EmbyID) != "",
		"emby_username":            user.EmbyUsername,
		"emby_disabled":            user.EmbyDisabled,
		"avatar":                   user.Avatar,
		"background":               user.Background,
		"bgm_mode":                 user.BGMMode,
		"bgm_token_set":            strings.TrimSpace(user.BGMToken) != "",
		"emby_grant_locked":        user.EmbyGrantLocked,
		"registration_source":      user.RegistrationSource,
		"pending_emby":             user.PendingEmby,
		"pending_emby_days":        user.PendingEmbyDays,
		"email_verified":           user.EmailVerified,
		"email_verified_at":        zeroNil(user.EmailVerifiedAt),
		"telegram_bound":           user.TelegramID != 0,
		"telegram_id":              zeroNil(user.TelegramID),
		"telegram_username":        user.TelegramUsername,
		"notify_on_login_telegram": user.NotifyOnLoginTelegram,
		"notify_on_login_email":    user.NotifyOnLoginEmail,
		"legacy_api_key_enabled":   user.LegacyAPIKeyStatus,
		"rebinding_in_progress":    user.RebindingInProgress,
		"rebinding_since":          zeroNil(user.RebindingSince),
	}
}

func developerJSInputAPI(vm *goja.Runtime, args []string, commandName string, privateChat bool, preview bool) map[string]any {
	textValue := strings.Join(args, " ")
	first := ""
	if len(args) > 0 {
		first = args[0]
	}
	rest := []string{}
	if len(args) > 1 {
		rest = append(rest, args[1:]...)
	}
	return map[string]any{
		"command":      commandName,
		"args":         args,
		"text":         textValue,
		"count":        len(args),
		"first":        first,
		"rest":         rest,
		"private_chat": privateChat,
		"preview":      preview,
		"arg": func(call goja.FunctionCall) goja.Value {
			index := int(call.Argument(0).ToInteger())
			fallback := ""
			if len(call.Arguments) > 1 {
				fallback = call.Argument(1).String()
			}
			if index < 0 || index >= len(args) {
				return vm.ToValue(fallback)
			}
			return vm.ToValue(args[index])
		},
		"has": func(call goja.FunctionCall) goja.Value {
			index := int(call.Argument(0).ToInteger())
			return vm.ToValue(index >= 0 && index < len(args) && strings.TrimSpace(args[index]) != "")
		},
		"flag": func(call goja.FunctionCall) goja.Value {
			name := strings.TrimLeft(strings.ToLower(strings.TrimSpace(call.Argument(0).String())), "-")
			if name == "" {
				return vm.ToValue(false)
			}
			for _, arg := range args {
				trimmed := strings.TrimSpace(arg)
				if strings.EqualFold(trimmed, "--"+name) || strings.EqualFold(trimmed, "-"+name) {
					return vm.ToValue(true)
				}
			}
			return vm.ToValue(false)
		},
		"named": func(call goja.FunctionCall) goja.Value {
			name := strings.TrimLeft(strings.ToLower(strings.TrimSpace(call.Argument(0).String())), "-")
			fallback := ""
			if len(call.Arguments) > 1 {
				fallback = call.Argument(1).String()
			}
			if name == "" {
				return vm.ToValue(fallback)
			}
			for i, arg := range args {
				trimmed := strings.TrimSpace(arg)
				lower := strings.ToLower(trimmed)
				for _, prefix := range []string{"--" + name + "=", "-" + name + "="} {
					if strings.HasPrefix(lower, prefix) {
						return vm.ToValue(trimmed[len(prefix):])
					}
				}
				if (strings.EqualFold(trimmed, "--"+name) || strings.EqualFold(trimmed, "-"+name)) && i+1 < len(args) {
					return vm.ToValue(args[i+1])
				}
			}
			return vm.ToValue(fallback)
		},
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
		"search": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSSearchUsers(user, call.Argument(0).String(), int(call.Argument(1).ToInteger()), logs))
		},
		"list": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSListUsers(user, call.Argument(0).Export(), logs))
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
		"setActive": func(call goja.FunctionCall) goja.Value {
			return a.developerJSSetUserActive(vm, user, opts, logs, call.Argument(0), call.Argument(1).Export())
		},
		// enable(uid) / disable(uid) 是 setActive 的布尔简化别名，少传一个参数。
		"enable": func(call goja.FunctionCall) goja.Value {
			return a.developerJSSetUserActive(vm, user, opts, logs, call.Argument(0), true)
		},
		"disable": func(call goja.FunctionCall) goja.Value {
			return a.developerJSSetUserActive(vm, user, opts, logs, call.Argument(0), false)
		},
		"setRole": func(call goja.FunctionCall) goja.Value {
			return a.developerJSSetUserRole(vm, user, opts, logs, call.Argument(0), call.Argument(1))
		},
		"setExpiry": func(call goja.FunctionCall) goja.Value {
			return a.developerJSSetUserExpiry(vm, user, opts, logs, call.Argument(0), call.Argument(1))
		},
		// extend(uid, days) 在用户当前到期时间（或当前时间，取较晚者）基础上顺延 days 天，
		// 是续期场景的便捷封装；负数或 0 天会被拒绝。复用 admin 校验、预览 dry-run 与审计。
		"extend": func(call goja.FunctionCall) goja.Value {
			return a.developerJSExtendUserExpiry(vm, user, opts, logs, call.Argument(0), call.Argument(1))
		},
		// find(query, limit?) 是 search 的简化别名（语义一致，名字更直观）。
		"find": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSSearchUsers(user, call.Argument(0).String(), int(call.Argument(1).ToInteger()), logs))
		},
		// exists(uid) 返回该 UID 是否存在；仅管理员或查询自身可用，遵循 getUser 的同口径鉴权。
		"exists": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSUserExists(user, call.Argument(0), logs))
		},
		"update": func(call goja.FunctionCall) goja.Value {
			return a.developerJSUpdateUser(vm, user, opts, logs, call.Argument(0), call.Argument(1).Export())
		},
	}
}

func (a *App) developerJSAdminAPI(vm *goja.Runtime, user *store.User, opts developerJSRunOptions, logs *[]string) map[string]any {
	return map[string]any{
		"ok": func(goja.FunctionCall) goja.Value {
			return vm.ToValue(user != nil && user.Role == store.RoleAdmin)
		},
		"ensure": func(goja.FunctionCall) goja.Value {
			return vm.ToValue(developerJSAdminUser(user, logs, "admin.ensure"))
		},
		"searchUsers": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSSearchUsers(user, call.Argument(0).String(), int(call.Argument(1).ToInteger()), logs))
		},
		"listUsers": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSListUsers(user, call.Argument(0).Export(), logs))
		},
		"updateUser": func(call goja.FunctionCall) goja.Value {
			return a.developerJSUpdateUser(vm, user, opts, logs, call.Argument(0), call.Argument(1).Export())
		},
		"setActive": func(call goja.FunctionCall) goja.Value {
			return a.developerJSSetUserActive(vm, user, opts, logs, call.Argument(0), call.Argument(1).Export())
		},
		"setRole": func(call goja.FunctionCall) goja.Value {
			return a.developerJSSetUserRole(vm, user, opts, logs, call.Argument(0), call.Argument(1))
		},
		"setExpiry": func(call goja.FunctionCall) goja.Value {
			return a.developerJSSetUserExpiry(vm, user, opts, logs, call.Argument(0), call.Argument(1))
		},
		"generateRegcode": func(call goja.FunctionCall) goja.Value {
			return a.developerJSGenerateRegcode(vm, user, opts, logs, call.Argument(0).Export())
		},
		"generateInviteCode": func(call goja.FunctionCall) goja.Value {
			return a.developerJSGenerateInviteCode(vm, user, opts, logs, call.Argument(0).Export())
		},
		"createAnnouncement": func(call goja.FunctionCall) goja.Value {
			return a.developerJSCreateAnnouncement(vm, user, opts, logs, call.Argument(0).Export())
		},
		"stats": func(goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSSystemStats(user))
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
		"findUsers": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSSearchUsers(user, call.Argument(0).String(), int(call.Argument(1).ToInteger()), logs))
		},
		"listUsers": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSListUsers(user, call.Argument(0).Export(), logs))
		},
		"listRegcodes": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSListRegcodes(user, call.Argument(0).Export(), logs))
		},
		"listInviteCodes": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSListInviteCodes(user, call.Argument(0).Export(), logs))
		},
		"listMediaRequests": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSListMediaRequests(user, call.Argument(0).Export(), logs))
		},
		"listAnnouncements": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSListAnnouncements(call.Argument(0).Export()))
		},
		"listTickets": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSListTickets(user, call.Argument(0).Export(), logs))
		},
		"listPresets": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSListPresets(user, call.Argument(0).Export(), logs))
		},
		"updateCurrentUser": func(call goja.FunctionCall) goja.Value {
			return a.developerJSUpdateCurrentUser(vm, user, opts, logs, call.Argument(0).Export())
		},
		"updateUser": func(call goja.FunctionCall) goja.Value {
			return a.developerJSUpdateUser(vm, user, opts, logs, call.Argument(0), call.Argument(1).Export())
		},
	}
}

func developerJSDBSchema() map[string]any {
	userFields := []string{
		"uid", "username", "role", "active", "expired_at", "created_at", "register_time",
		"email", "email_masked", "has_email", "role_name", "expire_status", "has_emby",
		"emby_username", "emby_disabled", "avatar", "background", "bgm_mode", "bgm_token_set",
		"emby_grant_locked", "registration_source", "pending_emby", "pending_emby_days",
		"email_verified", "email_verified_at", "telegram_bound", "telegram_id", "telegram_username",
		"notify_on_login_telegram", "notify_on_login_email", "legacy_api_key_enabled",
		"rebinding_in_progress", "rebinding_since",
	}
	return map[string]any{
		"users": map[string]any{
			"read":   "current user, self lookup, admin exact UID lookup, admin list/search",
			"write":  "current user notification preferences; admin controlled active/role/expiry/notification updates",
			"fields": userFields,
		},
		"regcodes": map[string]any{
			"read":   "admin count and admin list (db.listRegcodes / regcodes.list / regcodes.get)",
			"write":  "admin only: regcodes.generate / admin.generateRegcode create new registration/renewal codes (preview = dry-run, audited)",
			"fields": []string{"code", "type", "type_name", "days", "validity_time", "use_count", "use_count_limit", "active", "is_decoy", "source", "target_username", "created_at", "expired_at", "paused_seconds"},
		},
		"invite_codes": map[string]any{
			"read":   "admin list (db.listInviteCodes / invites.list); current user owned codes for non-admin",
			"write":  "admin only: invites.generate / admin.generateInviteCode create new invite codes when invite feature is enabled (preview = dry-run, audited)",
			"fields": []string{"code", "uid", "inviter_uid", "days", "use_count", "use_count_limit", "active", "used", "note", "target_username", "created_at", "expired_at"},
		},
		"media_requests": map[string]any{
			"read":   "current user owned count/list; admin count/list (db.listMediaRequests)",
			"write":  "not available",
			"fields": []string{"id", "uid", "username", "title", "original_title", "source", "media_id", "media_type", "season", "year", "status", "created_at", "updated_at"},
		},
		"announcements": map[string]any{
			"read":   "visible count and visible list (db.listAnnouncements / announcements.list)",
			"write":  "admin only: announcements.create / admin.createAnnouncement publish a new announcement (preview = dry-run, audited)",
			"fields": []string{"id", "title", "level", "render_mode", "pinned", "visible", "created_by_uid", "created_at", "updated_at", "expired_at"},
		},
		"tickets": map[string]any{
			"read":   "current user owned count/list; admin count/list (db.listTickets)",
			"write":  "not available",
			"fields": []string{"id", "uid", "username", "title", "type", "status", "priority", "created_at", "updated_at", "resolved_at", "closed_at"},
		},
		"developer_js_presets": map[string]any{
			"read":   "admin count and admin list (db.listPresets)",
			"write":  "manage through developer preset APIs",
			"fields": []string{"id", "name", "description", "creator_uid", "code_length", "created_at", "updated_at"},
		},
	}
}

func (a *App) developerJSDBCount(name string, user *store.User) (int, bool) {
	admin := user != nil && user.Role == store.RoleAdmin
	switch name {
	case "users":
		if !admin {
			return 0, false
		}
		return a.store().UserCount(), true
	case "regcodes":
		if !admin {
			return 0, false
		}
		return a.store().CountRegCodes(), true
	case "invite_codes":
		if !admin {
			return 0, false
		}
		return a.store().CountInviteCodes(), true
	case "media_requests":
		if admin {
			return a.store().CountMediaRequests(0, true), true
		}
		if user == nil || user.UID == 0 {
			return 0, false
		}
		return a.store().CountMediaRequests(user.UID, false), true
	case "announcements":
		return a.store().CountAnnouncements(false), true
	case "tickets":
		if admin {
			return a.store().CountTickets(store.TicketFilter{}), true
		}
		if user == nil || user.UID == 0 {
			return 0, false
		}
		return a.store().CountTickets(store.TicketFilter{UID: user.UID}), true
	case "developer_js_presets":
		if !admin {
			return 0, false
		}
		return a.store().CountDeveloperJSPresets(), true
	default:
		return 0, false
	}
}

// developerJSListOptions parses { limit, offset } from an exported JS options object.
// limit is clamped to [1, max] with the given fallback; offset is clamped to >= 0.
func developerJSListOptions(options any, fallback int, max int) (limit int, offset int) {
	limit = developerJSLimit(0, fallback, max)
	values, _ := options.(map[string]any)
	if values == nil {
		return limit, 0
	}
	if raw, ok := values["limit"]; ok {
		limit = developerJSLimit(int(numeric(raw)), fallback, max)
	}
	if raw, ok := values["offset"]; ok {
		offset = int(numeric(raw))
		if offset < 0 {
			offset = 0
		}
	}
	return limit, offset
}

func developerJSRegcodeSnapshot(c store.RegCode) map[string]any {
	return map[string]any{
		"code":            c.Code,
		"type":            c.Type,
		"type_name":       regcodeTypeName(c.Type),
		"days":            c.Days,
		"validity_time":   c.ValidityTime,
		"use_count":       c.UseCount,
		"use_count_limit": c.UseCountLimit,
		"active":          c.Active,
		"is_decoy":        c.IsDecoy,
		"source":          c.Source,
		"target_username": c.TargetUsername,
		"created_at":      c.CreatedAt,
		"expired_at":      c.ExpiredAt,
		"paused_seconds":  c.PausedSeconds,
	}
}

func developerJSInviteSnapshot(c store.InviteCode) map[string]any {
	return map[string]any{
		"code":            c.Code,
		"uid":             c.UID,
		"inviter_uid":     c.InviterUID,
		"days":            c.Days,
		"use_count":       c.UseCount,
		"use_count_limit": c.UseCountLimit,
		"active":          c.Active,
		"used":            c.Used,
		"note":            c.Note,
		"target_username": c.TargetUsername,
		"created_at":      c.CreatedAt,
		"expired_at":      c.ExpiredAt,
	}
}

func developerJSMediaRequestSnapshot(m store.MediaRequest) map[string]any {
	return map[string]any{
		"id":             m.ID,
		"uid":            m.UID,
		"username":       m.Username,
		"title":          m.Title,
		"original_title": m.OriginalTitle,
		"source":         m.Source,
		"media_id":       m.MediaID,
		"media_type":     m.MediaType,
		"season":         m.Season,
		"year":           m.Year,
		"status":         m.Status,
		"created_at":     m.CreatedAt,
		"updated_at":     m.UpdatedAt,
	}
}

func developerJSAnnouncementSnapshot(an store.Announcement) map[string]any {
	return map[string]any{
		"id":             an.ID,
		"title":          an.Title,
		"level":          an.Level,
		"render_mode":    an.RenderMode,
		"pinned":         an.Pinned,
		"visible":        an.Visible,
		"created_by_uid": an.CreatedByUID,
		"created_at":     an.CreatedAt,
		"updated_at":     an.UpdatedAt,
		"expired_at":     an.ExpiredAt,
	}
}

func developerJSTicketSnapshot(t store.Ticket) map[string]any {
	return map[string]any{
		"id":          t.ID,
		"uid":         t.UID,
		"username":    t.Username,
		"title":       t.Title,
		"type":        t.Type,
		"status":      t.Status,
		"priority":    t.Priority,
		"created_at":  t.CreatedAt,
		"updated_at":  t.UpdatedAt,
		"resolved_at": t.ResolvedAt,
		"closed_at":   t.ClosedAt,
	}
}

func developerJSPresetSnapshot(p store.DeveloperJSPreset) map[string]any {
	return map[string]any{
		"id":          p.ID,
		"name":        p.Name,
		"description": p.Description,
		"creator_uid": p.CreatorUID,
		"code_length": len(p.Code),
		"created_at":  p.CreatedAt,
		"updated_at":  p.UpdatedAt,
	}
}

// developerJSListRegcodes returns masked registration code snapshots. Admin-only.
func (a *App) developerJSListRegcodes(user *store.User, options any, logs *[]string) []map[string]any {
	if !developerJSAdminUser(user, logs, "db.listRegcodes") {
		return nil
	}
	limit, offset := developerJSListOptions(options, 20, 50)
	codes := a.store().ListRegCodes()
	out := make([]map[string]any, 0, limit)
	for i := offset; i < len(codes) && len(out) < limit; i++ {
		out = append(out, developerJSRegcodeSnapshot(codes[i]))
	}
	return out
}

// developerJSListInviteCodes returns masked invite code snapshots. Admins see all
// codes; non-admin users only receive codes they own.
func (a *App) developerJSListInviteCodes(user *store.User, options any, logs *[]string) []map[string]any {
	if user == nil || user.UID == 0 {
		if logs != nil && len(*logs) < 8 {
			*logs = append(*logs, "db.listInviteCodes denied: no bound user")
		}
		return nil
	}
	limit, offset := developerJSListOptions(options, 20, 50)
	var codes []store.InviteCode
	if user.Role == store.RoleAdmin {
		codes = a.store().ListAllInviteCodes()
	} else {
		codes = a.store().ListInviteCodes(user.UID)
	}
	out := make([]map[string]any, 0, limit)
	for i := offset; i < len(codes) && len(out) < limit; i++ {
		out = append(out, developerJSInviteSnapshot(codes[i]))
	}
	return out
}

// developerJSListMediaRequests returns media request snapshots. Admins see all
// requests; non-admin users only receive their own.
func (a *App) developerJSListMediaRequests(user *store.User, options any, logs *[]string) []map[string]any {
	if user == nil || user.UID == 0 {
		if logs != nil && len(*logs) < 8 {
			*logs = append(*logs, "db.listMediaRequests denied: no bound user")
		}
		return nil
	}
	limit, offset := developerJSListOptions(options, 20, 50)
	admin := user.Role == store.RoleAdmin
	var requests []store.MediaRequest
	if admin {
		requests = a.store().ListMediaRequests(0, true)
	} else {
		requests = a.store().ListMediaRequests(user.UID, false)
	}
	out := make([]map[string]any, 0, limit)
	for i := offset; i < len(requests) && len(out) < limit; i++ {
		out = append(out, developerJSMediaRequestSnapshot(requests[i]))
	}
	return out
}

// developerJSListAnnouncements returns visible announcement snapshots for any user.
func (a *App) developerJSListAnnouncements(options any) []map[string]any {
	limit, offset := developerJSListOptions(options, 20, 50)
	items := a.store().ListAnnouncements(false)
	out := make([]map[string]any, 0, limit)
	for i := offset; i < len(items) && len(out) < limit; i++ {
		out = append(out, developerJSAnnouncementSnapshot(items[i]))
	}
	return out
}

// developerJSListTickets returns ticket snapshots. Admins see all tickets;
// non-admin users only receive their own.
func (a *App) developerJSListTickets(user *store.User, options any, logs *[]string) []map[string]any {
	if user == nil || user.UID == 0 {
		if logs != nil && len(*logs) < 8 {
			*logs = append(*logs, "db.listTickets denied: no bound user")
		}
		return nil
	}
	limit, offset := developerJSListOptions(options, 20, 50)
	var filter store.TicketFilter
	if user.Role != store.RoleAdmin {
		filter.UID = user.UID
	}
	tickets := a.store().ListTickets(filter)
	out := make([]map[string]any, 0, limit)
	for i := offset; i < len(tickets) && len(out) < limit; i++ {
		out = append(out, developerJSTicketSnapshot(tickets[i]))
	}
	return out
}

// developerJSListPresets returns developer JS preset metadata (no code bodies). Admin-only.
func (a *App) developerJSListPresets(user *store.User, options any, logs *[]string) []map[string]any {
	if !developerJSAdminUser(user, logs, "db.listPresets") {
		return nil
	}
	limit, offset := developerJSListOptions(options, 20, 50)
	presets := a.store().ListDeveloperJSPresets()
	out := make([]map[string]any, 0, limit)
	for i := offset; i < len(presets) && len(out) < limit; i++ {
		out = append(out, developerJSPresetSnapshot(presets[i]))
	}
	return out
}

func developerJSAdminUser(user *store.User, logs *[]string, action string) bool {
	if user == nil || user.UID == 0 {
		if logs != nil && len(*logs) < 8 {
			*logs = append(*logs, action+" denied: no bound user")
		}
		return false
	}
	if user.Role != store.RoleAdmin {
		if logs != nil && len(*logs) < 8 {
			*logs = append(*logs, action+" denied: admin role required")
		}
		return false
	}
	return true
}

func developerJSUserMatches(u store.User, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return false
	}
	if strconv.FormatInt(u.UID, 10) == query {
		return true
	}
	fields := []string{u.Username, u.Email, u.TelegramUsername, u.EmbyUsername}
	if u.TelegramID != 0 {
		fields = append(fields, strconv.FormatInt(u.TelegramID, 10))
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(strings.TrimSpace(field)), query) {
			return true
		}
	}
	return false
}

func developerJSLimit(limit int, fallback int, max int) int {
	if limit <= 0 {
		limit = fallback
	}
	if limit > max {
		limit = max
	}
	return limit
}

func (a *App) developerJSSearchUsers(user *store.User, query string, limit int, logs *[]string) []map[string]any {
	if !developerJSAdminUser(user, logs, "users.search") {
		return nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return []map[string]any{}
	}
	limit = developerJSLimit(limit, 10, 50)
	users := a.store().ListUsers()
	sort.Slice(users, func(i, j int) bool { return users[i].UID < users[j].UID })
	out := make([]map[string]any, 0, limit)
	for _, u := range users {
		if !developerJSUserMatches(u, query) {
			continue
		}
		out = append(out, developerJSUserSnapshot(u))
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (a *App) developerJSListUsers(user *store.User, options any, logs *[]string) []map[string]any {
	if user == nil || user.UID == 0 {
		if logs != nil && len(*logs) < 8 {
			*logs = append(*logs, "users.list denied: no bound user")
		}
		return nil
	}
	if user.Role != store.RoleAdmin {
		return []map[string]any{developerJSUserSnapshot(*user)}
	}
	values, _ := options.(map[string]any)
	limit := 20
	offset := 0
	roleFilter := ""
	activeFilter := ""
	if values != nil {
		limit = int(numeric(values["limit"]))
		offset = int(numeric(values["offset"]))
		if raw, ok := values["role"]; ok {
			roleFilter = strings.TrimSpace(fmt.Sprint(raw))
		}
		if raw, ok := values["active"]; ok {
			activeFilter = strings.ToLower(strings.TrimSpace(fmt.Sprint(raw)))
		}
	}
	limit = developerJSLimit(limit, 20, 50)
	if offset < 0 {
		offset = 0
	}
	users := a.store().ListUsers()
	sort.Slice(users, func(i, j int) bool { return users[i].UID < users[j].UID })
	out := make([]map[string]any, 0, limit)
	skipped := 0
	for _, u := range users {
		if roleFilter != "" && strconv.Itoa(u.Role) != roleFilter && strings.ToLower(roleName(u.Role)) != strings.ToLower(roleFilter) {
			continue
		}
		if activeFilter != "" && strconv.FormatBool(u.Active) != activeFilter {
			continue
		}
		if skipped < offset {
			skipped++
			continue
		}
		out = append(out, developerJSUserSnapshot(u))
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (a *App) developerJSSetUserActive(vm *goja.Runtime, actor *store.User, opts developerJSRunOptions, logs *[]string, uidValue goja.Value, activeValue any) goja.Value {
	result := map[string]any{"ok": false}
	if !developerJSAdminUser(actor, logs, "users.setActive") {
		result["error"] = "admin_required"
		return vm.ToValue(result)
	}
	uid := uidValue.ToInteger()
	if uid <= 0 {
		result["error"] = "invalid_uid"
		return vm.ToValue(result)
	}
	active, ok := activeValue.(bool)
	if !ok {
		result["error"] = "invalid_active"
		return vm.ToValue(result)
	}
	result["uid"] = uid
	result["active"] = active
	if opts.Preview {
		result["dry_run"] = true
		result["ok"] = true
		return vm.ToValue(result)
	}
	updated, err := a.store().SetUserActiveAtomic(uid, active)
	if err != nil {
		result["error"] = err.Error()
		return vm.ToValue(result)
	}
	result["ok"] = true
	result["user"] = developerJSUserSnapshot(updated)
	if logs != nil && len(*logs) < 8 {
		*logs = append(*logs, "users.setActive updated user")
	}
	a.auditEntryIP("telegram", actor.UID, actor.Username, "telegram_js_admin_user_active_update", "admin", updated.UID, map[string]any{
		"active":       active,
		"script_api":   "users.setActive",
		"private_chat": opts.PrivateChat,
	})
	return vm.ToValue(result)
}

func (a *App) developerJSSetUserRole(vm *goja.Runtime, actor *store.User, opts developerJSRunOptions, logs *[]string, uidValue goja.Value, roleValue goja.Value) goja.Value {
	result := map[string]any{"ok": false}
	if !developerJSAdminUser(actor, logs, "users.setRole") {
		result["error"] = "admin_required"
		return vm.ToValue(result)
	}
	uid := uidValue.ToInteger()
	role := int(roleValue.ToInteger())
	if uid <= 0 || (role != store.RoleAdmin && role != store.RoleNormal && role != store.RoleWhitelist) {
		result["error"] = "invalid_payload"
		return vm.ToValue(result)
	}
	result["uid"] = uid
	result["role"] = role
	if opts.Preview {
		result["dry_run"] = true
		result["ok"] = true
		return vm.ToValue(result)
	}
	updated, err := a.store().SetUserRoleAtomic(uid, role)
	if err != nil {
		result["error"] = err.Error()
		return vm.ToValue(result)
	}
	result["ok"] = true
	result["user"] = developerJSUserSnapshot(updated)
	if logs != nil && len(*logs) < 8 {
		*logs = append(*logs, "users.setRole updated user")
	}
	a.auditEntryIP("telegram", actor.UID, actor.Username, "telegram_js_admin_user_role_update", "admin", updated.UID, map[string]any{
		"role":         role,
		"script_api":   "users.setRole",
		"private_chat": opts.PrivateChat,
	})
	return vm.ToValue(result)
}

func (a *App) developerJSSetUserExpiry(vm *goja.Runtime, actor *store.User, opts developerJSRunOptions, logs *[]string, uidValue goja.Value, expiryValue goja.Value) goja.Value {
	result := map[string]any{"ok": false}
	if !developerJSAdminUser(actor, logs, "users.setExpiry") {
		result["error"] = "admin_required"
		return vm.ToValue(result)
	}
	uid := uidValue.ToInteger()
	expiredAt := expiryValue.ToInteger()
	if uid <= 0 || expiredAt < -1 {
		result["error"] = "invalid_payload"
		return vm.ToValue(result)
	}
	if expiryIsPermanent(expiredAt) {
		expiredAt = permanentExpiryUnix
	}
	result["uid"] = uid
	result["expired_at"] = publicExpiryUnix(expiredAt)
	if opts.Preview {
		result["dry_run"] = true
		result["ok"] = true
		return vm.ToValue(result)
	}
	updated, err := a.store().UpdateUser(uid, func(u *store.User) error {
		u.ExpiredAt = expiredAt
		if expiredAt == permanentExpiryUnix || expiredAt > time.Now().Unix() {
			u.Active = true
		}
		return nil
	})
	if err != nil {
		result["error"] = err.Error()
		return vm.ToValue(result)
	}
	result["ok"] = true
	result["user"] = developerJSUserSnapshot(updated)
	if logs != nil && len(*logs) < 8 {
		*logs = append(*logs, "users.setExpiry updated user")
	}
	a.auditEntryIP("telegram", actor.UID, actor.Username, "telegram_js_admin_user_expiry_update", "admin", updated.UID, map[string]any{
		"expired_at":   publicExpiryUnix(expiredAt),
		"script_api":   "users.setExpiry",
		"private_chat": opts.PrivateChat,
	})
	return vm.ToValue(result)
}

// developerJSExtendUserExpiry adds the given number of days on top of the user's
// current expiry (or now, whichever is later), then delegates the actual write to
// developerJSSetUserExpiry so all admin-gating, preview and audit logic is shared.
func (a *App) developerJSExtendUserExpiry(vm *goja.Runtime, actor *store.User, opts developerJSRunOptions, logs *[]string, uidValue goja.Value, daysValue goja.Value) goja.Value {
	result := map[string]any{"ok": false}
	if !developerJSAdminUser(actor, logs, "users.extend") {
		result["error"] = "admin_required"
		return vm.ToValue(result)
	}
	uid := uidValue.ToInteger()
	days := daysValue.ToInteger()
	if uid <= 0 || days <= 0 {
		result["error"] = "invalid_payload"
		return vm.ToValue(result)
	}
	target, ok := a.store().User(uid)
	if !ok {
		result["error"] = "user_not_found"
		return vm.ToValue(result)
	}
	now := time.Now().Unix()
	base := target.ExpiredAt
	if expiryIsPermanent(base) {
		// 永久用户无需顺延，直接返回当前状态。
		result["ok"] = true
		result["uid"] = uid
		result["expired_at"] = publicExpiryUnix(base)
		result["note"] = "already_permanent"
		return vm.ToValue(result)
	}
	if base < now {
		base = now
	}
	newExpiry := base + days*86400
	return a.developerJSSetUserExpiry(vm, actor, opts, logs, uidValue, vm.ToValue(newExpiry))
}

// developerJSUserExists reports whether a user with the given UID exists. It reuses
// the getUser visibility rule: admins may probe any UID, normal users only their own.
func (a *App) developerJSUserExists(actor *store.User, uidValue goja.Value, logs *[]string) bool {
	uid := uidValue.ToInteger()
	if uid <= 0 {
		return false
	}
	if actor == nil || actor.UID == 0 {
		if logs != nil && len(*logs) < 8 {
			*logs = append(*logs, "users.exists denied: no bound user")
		}
		return false
	}
	if actor.Role != store.RoleAdmin && actor.UID != uid {
		if logs != nil && len(*logs) < 8 {
			*logs = append(*logs, "users.exists denied: cross-user lookup requires admin")
		}
		return false
	}
	_, ok := a.store().User(uid)
	return ok
}

func (a *App) developerJSUpdateUser(vm *goja.Runtime, actor *store.User, opts developerJSRunOptions, logs *[]string, uidValue goja.Value, patch any) goja.Value {
	result := map[string]any{"ok": false}
	if !developerJSAdminUser(actor, logs, "users.update") {
		result["error"] = "admin_required"
		return vm.ToValue(result)
	}
	uid := uidValue.ToInteger()
	values, ok := patch.(map[string]any)
	if uid <= 0 || !ok {
		result["error"] = "invalid_payload"
		return vm.ToValue(result)
	}
	allowed := map[string]any{}
	active, hasActive := developerJSBoolOption(values, "active")
	roleRaw, hasRole := values["role"]
	expiryRaw, hasExpiry := values["expired_at"]
	telegram, hasTelegram := developerJSBoolOption(values, "notify_on_login_telegram")
	if !hasTelegram {
		telegram, hasTelegram = developerJSBoolOption(values, "telegram")
	}
	email, hasEmail := developerJSBoolOption(values, "notify_on_login_email")
	if !hasEmail {
		email, hasEmail = developerJSBoolOption(values, "email")
	}
	if hasActive {
		allowed["active"] = active
	}
	if hasRole {
		role := int(numeric(roleRaw))
		if role != store.RoleAdmin && role != store.RoleNormal && role != store.RoleWhitelist {
			result["error"] = "invalid_role"
			return vm.ToValue(result)
		}
		allowed["role"] = role
	}
	if hasExpiry {
		expiredAt := int64(numeric(expiryRaw))
		if expiredAt < -1 {
			result["error"] = "invalid_expired_at"
			return vm.ToValue(result)
		}
		allowed["expired_at"] = expiredAt
	}
	if hasTelegram {
		allowed["notify_on_login_telegram"] = telegram
	}
	if hasEmail {
		allowed["notify_on_login_email"] = email
	}
	if len(allowed) == 0 {
		result["error"] = "empty_patch"
		return vm.ToValue(result)
	}
	result["uid"] = uid
	result["patch"] = allowed
	if opts.Preview {
		result["dry_run"] = true
		result["ok"] = true
		return vm.ToValue(result)
	}
	var (
		updated store.User
		err     error
	)
	if hasActive {
		updated, err = a.store().SetUserActiveAtomic(uid, active)
		if err != nil {
			result["error"] = err.Error()
			return vm.ToValue(result)
		}
	}
	if hasRole {
		role := int(numeric(roleRaw))
		updated, err = a.store().SetUserRoleAtomic(uid, role)
		if err != nil {
			result["error"] = err.Error()
			return vm.ToValue(result)
		}
	}
	if hasExpiry || hasTelegram || hasEmail {
		updated, err = a.store().UpdateUser(uid, func(u *store.User) error {
			if hasExpiry {
				expiredAt := int64(numeric(expiryRaw))
				if expiredAt < -1 {
					return fmt.Errorf("invalid_expired_at")
				}
				if expiryIsPermanent(expiredAt) {
					expiredAt = permanentExpiryUnix
				}
				u.ExpiredAt = expiredAt
				if expiredAt == permanentExpiryUnix || expiredAt > time.Now().Unix() {
					u.Active = true
				}
			}
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
	}
	result["ok"] = true
	result["user"] = developerJSUserSnapshot(updated)
	if logs != nil && len(*logs) < 8 {
		*logs = append(*logs, "users.update updated user")
	}
	a.auditEntryIP("telegram", actor.UID, actor.Username, "telegram_js_admin_user_update", "admin", updated.UID, map[string]any{
		"patch":        allowed,
		"script_api":   "users.update",
		"private_chat": opts.PrivateChat,
	})
	return vm.ToValue(result)
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

func (a *App) developerJSSystemAPI(vm *goja.Runtime, user *store.User) map[string]any {
	return map[string]any{
		"info": func(goja.FunctionCall) goja.Value {
			cfg := a.cfg()
			return vm.ToValue(map[string]any{
				"name":    cfg.AppName,
				"version": cfg.Version,
				"features": map[string]any{
					"telegram_mode":       cfg.TelegramMode,
					"telegram_force_bind": cfg.ForceBindTelegram,
					"telegram_panel":      cfg.TelegramEnablePanel,
					"invite_enabled":      cfg.InviteEnabled,
					"email_enabled":       cfg.EmailEnabled,
					"email_force_bind":    cfg.EmailForceBind,
					"media_request":       cfg.MediaRequestEnabled,
					"signin_enabled":      cfg.SigninEnabled,
					"ticket_enabled":      cfg.TicketSystemEnabled,
					"developer_mode":      a.store().DeveloperModeEnabled(),
				},
				"limits": map[string]any{
					"user":              cfg.UserLimit,
					"emby_user":         cfg.EmbyUserLimit,
					"invite_depth":      cfg.InviteMaxDepth,
					"invite_limit":      cfg.InviteLimit,
					"reply_segments":    4,
					"log_lines":         8,
					"script_timeout_ms": int(developerJSExecutionTimeout / time.Millisecond),
				},
			})
		},
		"feature": func(call goja.FunctionCall) goja.Value {
			key := strings.ToLower(strings.TrimSpace(call.Argument(0).String()))
			cfg := a.cfg()
			values := map[string]bool{
				"telegram_mode":       cfg.TelegramMode,
				"telegram_force_bind": cfg.ForceBindTelegram,
				"telegram_panel":      cfg.TelegramEnablePanel,
				"invite_enabled":      cfg.InviteEnabled,
				"email_enabled":       cfg.EmailEnabled,
				"email_force_bind":    cfg.EmailForceBind,
				"media_request":       cfg.MediaRequestEnabled,
				"signin_enabled":      cfg.SigninEnabled,
				"ticket_enabled":      cfg.TicketSystemEnabled,
				"developer_mode":      a.store().DeveloperModeEnabled(),
			}
			return vm.ToValue(values[key])
		},
		"stats": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSSystemStats(user))
		},
	}
}

func (a *App) developerJSSystemStats(user *store.User) map[string]any {
	admin := user != nil && user.Role == store.RoleAdmin
	users := a.store().ListUsers()
	active := 0
	admins := 0
	telegramBound := 0
	embyBound := 0
	emailBound := 0
	emailVerified := 0
	for _, u := range users {
		if u.Active {
			active++
		}
		if u.Role == store.RoleAdmin {
			admins++
		}
		if u.TelegramID != 0 {
			telegramBound++
		}
		if strings.TrimSpace(u.EmbyID) != "" {
			embyBound++
		}
		if strings.TrimSpace(u.Email) != "" {
			emailBound++
		}
		if u.EmailVerified {
			emailVerified++
		}
	}
	result := map[string]any{
		"users": map[string]any{
			"total":             len(users),
			"active":            active,
			"admins":            admins,
			"telegram_bound":    telegramBound,
			"emby_bound":        embyBound,
			"email_bound":       emailBound,
			"email_verified":    emailVerified,
			"admin_detail_view": admin,
		},
		"visible_announcements": a.store().CountAnnouncements(false),
		"developer_mode":        a.store().DeveloperModeEnabled(),
	}
	if admin {
		result["admin_counts"] = map[string]any{
			"regcodes":             a.store().CountRegCodes(),
			"invite_codes":         a.store().CountInviteCodes(),
			"media_requests":       a.store().CountMediaRequests(0, true),
			"tickets":              a.store().CountTickets(store.TicketFilter{}),
			"developer_js_presets": a.store().CountDeveloperJSPresets(),
		}
	}
	return result
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
		"trim": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(strings.TrimSpace(call.Argument(0).String()))
		},
		"lower": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(strings.ToLower(call.Argument(0).String()))
		},
		"upper": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(strings.ToUpper(call.Argument(0).String()))
		},
		"contains": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(strings.Contains(strings.ToLower(call.Argument(0).String()), strings.ToLower(call.Argument(1).String())))
		},
		"split": func(call goja.FunctionCall) goja.Value {
			sep := call.Argument(1).String()
			if sep == "" {
				sep = " "
			}
			return vm.ToValue(strings.Split(call.Argument(0).String(), sep))
		},
		"maskEmail": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(maskEmail(call.Argument(0).String()))
		},
		"template": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(developerJSTemplate(call.Argument(0).String(), call.Argument(1).Export()))
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
		"last": func(call goja.FunctionCall) goja.Value {
			items := developerJSAnySlice(call.Argument(0).Export())
			if len(items) == 0 {
				return goja.Undefined()
			}
			return vm.ToValue(items[len(items)-1])
		},
		"join": func(call goja.FunctionCall) goja.Value {
			sep := call.Argument(1).String()
			return vm.ToValue(strings.Join(developerJSStringSlice(call.Argument(0).Export()), sep))
		},
		"includes": func(call goja.FunctionCall) goja.Value {
			needle := call.Argument(1).String()
			for _, item := range developerJSStringSlice(call.Argument(0).Export()) {
				if item == needle {
					return vm.ToValue(true)
				}
			}
			return vm.ToValue(false)
		},
		"sortStrings": func(call goja.FunctionCall) goja.Value {
			items := developerJSStringSlice(call.Argument(0).Export())
			sort.Strings(items)
			return vm.ToValue(items)
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
		"fromNow": func(call goja.FunctionCall) goja.Value {
			seconds := call.Argument(0).ToInteger()
			return vm.ToValue(time.Now().Add(time.Duration(seconds) * time.Second).Unix())
		},
		"addDays": func(call goja.FunctionCall) goja.Value {
			base := call.Argument(0).ToInteger()
			if base <= 0 {
				base = time.Now().Unix()
			}
			days := int(call.Argument(1).ToInteger())
			return vm.ToValue(time.Unix(base, 0).AddDate(0, 0, days).Unix())
		},
		"duration": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(formatSeconds(call.Argument(0).ToInteger()))
		},
	}
}

func developerJSFormatAPI(vm *goja.Runtime) map[string]any {
	return map[string]any{
		"bool": func(call goja.FunctionCall) goja.Value {
			yes := "yes"
			no := "no"
			if len(call.Arguments) > 1 {
				yes = call.Argument(1).String()
			}
			if len(call.Arguments) > 2 {
				no = call.Argument(2).String()
			}
			return vm.ToValue(map[bool]string{true: yes, false: no}[boolish(call.Argument(0).Export())])
		},
		"role": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(roleName(int(call.Argument(0).ToInteger())))
		},
		"date": func(call goja.FunctionCall) goja.Value {
			ts := call.Argument(0).ToInteger()
			if ts <= 0 {
				return vm.ToValue("")
			}
			if expiryIsPermanent(ts) {
				return vm.ToValue(expireStatus(ts))
			}
			return vm.ToValue(time.Unix(ts, 0).UTC().Format(time.RFC3339))
		},
		"expiry": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(expireStatus(call.Argument(0).ToInteger()))
		},
		"duration": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(formatSeconds(call.Argument(0).ToInteger()))
		},
		"user": func(call goja.FunctionCall) goja.Value {
			values, _ := call.Argument(0).Export().(map[string]any)
			if values == nil {
				return vm.ToValue("")
			}
			uid := int64(numeric(values["uid"]))
			username := strings.TrimSpace(fmt.Sprint(values["username"]))
			role := fmt.Sprint(values["role_name"])
			if strings.TrimSpace(role) == "" || role == "<nil>" {
				role = roleName(int(numeric(values["role"])))
			}
			active := boolish(values["active"])
			expiry := fmt.Sprint(values["expire_status"])
			if strings.TrimSpace(expiry) == "" || expiry == "<nil>" {
				expiry = expireStatus(int64(numeric(values["expired_at"])))
			}
			return vm.ToValue(fmt.Sprintf("#%d %s / %s / %s / %s", uid, username, role, map[bool]string{true: "active", false: "disabled"}[active], expiry))
		},
		"json": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(developerJSLimitText(fmt.Sprint(call.Argument(0).String()), 1200))
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
			// 关键 SSRF 防护：在拨号阶段对实际连接到的 IP 再校验一次，
			// 阻断 DNS rebinding（校验时返回公网 IP、连接时返回内网/链路本地 IP）。
			// 这里不复用 sharedHTTPTransport，避免开发者沙箱 fetch 与系统调用串用连接池。
			Transport: &http.Transport{
				Proxy: nil,
				DialContext: (&net.Dialer{
					Timeout:   1500 * time.Millisecond,
					KeepAlive: -1,
					Control: func(network, address string, _ syscall.RawConn) error {
						host, _, splitErr := net.SplitHostPort(address)
						if splitErr != nil {
							host = address
						}
						ip := net.ParseIP(host)
						if ip == nil || developerJSPrivateIP(ip) {
							return fmt.Errorf("host_not_allowed")
						}
						return nil
					},
				}).DialContext,
				DisableKeepAlives:     true,
				TLSHandshakeTimeout:   1500 * time.Millisecond,
				ExpectContinueTimeout: 500 * time.Millisecond,
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
	if ip == nil {
		return true
	}
	// 归一化 IPv4-mapped IPv6（如 ::ffff:127.0.0.1），避免绕过私网判断。
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsInterfaceLocalMulticast() {
		return true
	}
	// IPv4 广播地址。
	if ip.Equal(net.IPv4bcast) {
		return true
	}
	return false
}

func developerJSAnySlice(input any) []any {
	switch values := input.(type) {
	case []any:
		return values
	case []string:
		out := make([]any, 0, len(values))
		for _, item := range values {
			out = append(out, item)
		}
		return out
	case []int:
		out := make([]any, 0, len(values))
		for _, item := range values {
			out = append(out, item)
		}
		return out
	case []int64:
		out := make([]any, 0, len(values))
		for _, item := range values {
			out = append(out, item)
		}
		return out
	case []float64:
		out := make([]any, 0, len(values))
		for _, item := range values {
			out = append(out, item)
		}
		return out
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

func developerJSTemplate(template string, data any) string {
	values, ok := data.(map[string]any)
	if !ok || strings.TrimSpace(template) == "" {
		return developerJSLimitText(template, 1200)
	}
	out := template
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		replacement := developerJSLimitText(fmt.Sprint(value), 240)
		out = strings.ReplaceAll(out, "{"+key+"}", replacement)
	}
	return developerJSLimitText(out, 1200)
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
