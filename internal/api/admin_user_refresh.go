package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

// handleAdminRefreshUserStatus 给管理员一个「强制刷新单个用户外部状态」的入口：
// 主动从 Telegram / Emby 拉取该用户当前的真实状态并回写本地，再返回刷新后的用户。
// 列表里的 telegram_username / emby_username / Emby 启停只在注册、绑定或用户主动
// 与 Bot 交互时被动更新，管理员核对时拿到的可能是旧值——这里补一个主动同步动作。
func (a *App) handleAdminRefreshUserStatus(w http.ResponseWriter, r *http.Request, params Params) {
	u, okUser := a.userFromPath(w, params, "uid")
	if !okUser {
		return
	}
	scope := normalizeRefreshScope(stringValue(decodeMap(r), "scope"))
	refresh := a.refreshUserExternalStatus(r.Context(), u, scope)
	// 刷新过程中可能写入了 TelegramUsername / EmbyUsername / Emby 启停，重新读取最新值
	// 让响应与列表展示一致。
	if latest, found := a.store().User(u.UID); found {
		u = latest
	}
	data := publicUser(u)
	data["refresh"] = refresh
	ok(w, "用户状态已刷新", data)
}

// normalizeRefreshScope 归一化刷新范围：telegram 仅刷新 TG 用户名、emby 仅核对 Emby
// 状态、其余（含空）默认 both 两者都刷。让 TG 同步与 Emby 同步可以分开触发。
func normalizeRefreshScope(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "telegram", "tg":
		return "telegram"
	case "emby":
		return "emby"
	default:
		return "both"
	}
}

// refreshUserExternalStatus 主动核对并回写用户在 Telegram 与 Emby 的当前状态：
//   - Telegram：用 getChat 拉最新 @username（被动刷新只在用户给 Bot 发消息时触发，
//     这里给管理员一个强制拉取入口）；取不到用户名时保留旧值不清空，避免破坏指名码匹配。
//   - Emby：核对远端账号是否仍存在、用户名是否变化，并把「Web 已禁用/过期但 Emby 仍
//     启用」的越权漂移强制关停。只收紧、不在此自动放开，避免绕过有效期重新启用 Emby。
//
// 任一外部系统失败都降级为结果字段（telegram_error / emby_error，已脱敏），不阻断另一侧、
// 也不阻断整体响应。返回的 map 直接挂到响应的 refresh 字段，供前端提示同步差异。
func (a *App) refreshUserExternalStatus(ctx context.Context, u store.User, scope string) map[string]any {
	out := map[string]any{}
	if scope != "emby" {
		a.refreshTelegramUsernameForUser(ctx, u, out)
	}
	// Emby 段可能在 Telegram 段之后写过库，但两段写的是不同字段（用户名互不影响 Emby
	// 判定），用入参 u 的副本即可；Emby 自身的判定只依赖 Active / ExpiredAt / EmbyID。
	if scope != "telegram" {
		a.refreshEmbyStatusForUser(ctx, u, out)
	}
	return out
}

// refreshTelegramUsernameForUser 拉取并回写单个用户的最新 Telegram 用户名。
func (a *App) refreshTelegramUsernameForUser(ctx context.Context, u store.User, out map[string]any) {
	if u.TelegramID == 0 {
		return
	}
	out["telegram_checked"] = true
	if strings.TrimSpace(a.cfg().TelegramBotToken) == "" {
		out["telegram_error"] = "Telegram Bot Token 未配置"
		return
	}
	var chat map[string]any
	if err := a.telegramPost(ctx, "getChat", map[string]any{"chat_id": u.TelegramID}, &chat); err != nil {
		out["telegram_error"] = a.telegramSanitizeError(err)
		return
	}
	username := strings.TrimPrefix(strings.TrimSpace(asString(chat["username"])), "@")
	out["telegram_username"] = username
	if username == "" || username == u.TelegramUsername {
		return
	}
	if _, err := a.store().UpdateUser(u.UID, func(cur *store.User) error {
		cur.TelegramUsername = username
		return nil
	}); err == nil {
		out["telegram_username_updated"] = true
	}
}

// refreshEmbyStatusForUser 核对并回写单个用户的 Emby 用户名与启停状态。
func (a *App) refreshEmbyStatusForUser(ctx context.Context, u store.User, out map[string]any) {
	if u.EmbyID == "" {
		return
	}
	out["emby_checked"] = true
	if !a.embyConfigured() {
		out["emby_error"] = "Emby URL 或 API Token 未配置"
		return
	}
	remote, found, err := a.embyUserByID(ctx, u.EmbyID)
	if err != nil {
		out["emby_error"] = redactSensitiveText(err.Error())
		return
	}
	if !found {
		out["emby_missing"] = true
		return
	}
	// 远端用户名变化时同步本地记录（如管理员在 Emby 后台改了名）。
	if name := asString(remote["Name"]); name != "" && name != u.EmbyUsername {
		if updated, err := a.store().UpdateUser(u.UID, func(cur *store.User) error {
			cur.EmbyUsername = name
			return nil
		}); err == nil {
			u = updated
			out["emby_username_updated"] = true
		}
	}
	out["emby_username"] = u.EmbyUsername
	remoteDisabled := false
	if policy, okPolicy := remote["Policy"].(map[string]any); okPolicy {
		remoteDisabled = boolish(policy["IsDisabled"])
	}
	out["emby_remote_disabled"] = remoteDisabled
	// 刷新即「以远端为准」校正本地 EmbyDisabled 镜像，修掉任何历史漂移，让列表展示真值。
	a.mirrorEmbyDisabled(u.UID, remoteDisabled)
	// 只收紧：Web 账号已禁用或已过期、而 Emby 仍处于启用态时强制关停，消除越权窗口。
	if embyShouldDisableForWebState(u) && !remoteDisabled {
		if changed, err := a.disableRemoteEmbyForWebState(ctx, u); err != nil {
			out["emby_error"] = redactSensitiveText(err.Error())
		} else if changed {
			out["emby_disabled_synced"] = true
		}
	}
}
