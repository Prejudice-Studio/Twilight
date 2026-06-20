package api

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
)

const telegramJSPrefix = "js:"

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

	text, logs, err := a.telegramRunJSCustomCommand(strings.TrimSpace(trimmed[len(telegramJSPrefix):]), c, privateChat)
	user, _ := a.store().FindUserByTelegramID(c.FromID)
	detail := map[string]any{"command": telegramCommand(command), "ok": err == nil, "private_chat": privateChat}
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

func (a *App) telegramRunJSCustomCommand(code string, c telegramCommandCtx, privateChat bool) (string, []string, error) {
	result := validateDeveloperJSCommand(code)
	if ok, _ := result["ok"].(bool); !ok {
		return "", nil, fmt.Errorf("developer js command rejected: %v", result["errors"])
	}

	user, _ := a.store().FindUserByTelegramID(c.FromID)
	vm := goja.New()
	replies := make([]string, 0, 4)
	logs := make([]string, 0, 8)
	_ = vm.Set("ctx", map[string]any{
		"private_chat": privateChat,
		"command_time": time.Now().Unix(),
	})
	_ = vm.Set("args", c.Args)
	_ = vm.Set("user", map[string]any{
		"uid":      user.UID,
		"username": user.Username,
		"role":     user.Role,
		"active":   user.Active,
		"has_emby": strings.TrimSpace(user.EmbyID) != "",
	})
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
	_ = vm.Set("reply", func(call goja.FunctionCall) goja.Value {
		if len(replies) < 4 {
			replies = append(replies, call.Argument(0).String())
		}
		return goja.Undefined()
	})
	_ = vm.Set("log", func(call goja.FunctionCall) goja.Value {
		if len(logs) < 8 {
			logs = append(logs, call.Argument(0).String())
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
	if _, err := vm.RunString(code); err != nil {
		return "", logs, err
	}
	return strings.Join(replies, "\n"), logs, nil
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
