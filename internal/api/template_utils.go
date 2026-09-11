package api

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

// TemplateParams 生成模板渲染的完整参数集合
type TemplateParams struct {
	app  *App
	ctx  context.Context
	user store.User
}

// NewTemplateParams 创建模板参数生成器
func (a *App) NewTemplateParams(ctx context.Context, user store.User) *TemplateParams {
	return &TemplateParams{
		app:  a,
		ctx:  ctx,
		user: user,
	}
}

// BuildAll 构建包含所有可用参数的 map，供各类模板使用
func (tp *TemplateParams) BuildAll() map[string]string {
	u := tp.user
	params := make(map[string]string, 60)

	// 基础信息
	params["server_name"] = tp.app.cfg().AppName
	params["username"] = u.Username
	params["uid"] = strconv.FormatInt(u.UID, 10)

	// 角色和权限
	params["role"] = roleName(u.Role)
	params["role_id"] = strconv.Itoa(u.Role)
	params["role_name"] = roleName(u.Role)
	params["is_admin"] = boolLabel(u.Role == store.RoleAdmin, "是", "否")
	params["is_whitelist"] = boolLabel(u.Role == store.RoleWhitelist, "是", "否")
	params["is_protected"] = boolLabel(tp.app.telegramProtectedTarget(u), "是", "否")

	// 账号状态
	params["web_status"] = activeLabel(u.Active)
	params["web_active"] = boolLabel(u.Active, "是", "否")
	params["account_enabled"] = boolLabel(u.Active, "已启用", "已禁用")
	params["account_disabled"] = boolLabel(!u.Active, "是", "否")

	// 到期信息
	params["expire_status"] = expireStatus(u.ExpiredAt)
	params["expired_at"] = expiryTimeLabel(u.ExpiredAt)
	params["expiry_time"] = expiryTimeLabel(u.ExpiredAt)
	params["days_until_expiry"] = daysUntilExpiry(u.ExpiredAt)
	params["is_expired"] = boolLabel(u.ExpiredAt > 0 && u.ExpiredAt < time.Now().Unix(), "是", "否")
	params["is_permanent"] = boolLabel(u.ExpiredAt == 0, "是", "否")

	// 注册信息
	params["register_time"] = unixTimeLabel(firstNonZeroInt64(u.RegisterTime, u.CreatedAt))
	params["created_at"] = unixTimeLabel(u.CreatedAt)
	params["registration_source"] = registrationSourceLabel(u.RegistrationSource)
	params["registration_code"] = firstNonEmpty(u.RegistrationCode, "-")

	// Emby 绑定状态
	embyUsername := strings.TrimSpace(u.EmbyUsername)
	if embyUsername == "" {
		embyUsername = "-"
	}
	params["emby_status"] = localEmbyLabel(u)
	params["emby_bound"] = boolLabel(u.EmbyID != "", "是", "否")
	params["emby_bound_status"] = localEmbyBindingStatusLabel(u)
	params["emby_username"] = embyUsername
	params["emby_id"] = firstNonEmpty(u.EmbyID, "-")
	params["emby_enabled"] = boolLabel(!u.EmbyDisabled && u.EmbyID != "", "是", "否")
	params["emby_disabled"] = boolLabel(u.EmbyDisabled, "是", "否")
	params["emby_enabled_status"] = embyEnabledStatusLabel(u)
	params["emby_disabled_reason"] = embyDisabledReason(u)
	params["pending_emby"] = boolLabel(u.PendingEmby, "是", "否")
	params["pending_emby_days"] = pendingEmbyDaysLabel(u.PendingEmbyDays)
	params["emby_grant_locked"] = boolLabel(u.EmbyGrantLocked, "是", "否")

	// Telegram 绑定状态
	telegramUsername := strings.TrimPrefix(strings.TrimSpace(u.TelegramUsername), "@")
	if telegramUsername == "" {
		telegramUsername = "-"
	} else {
		telegramUsername = "@" + telegramUsername
	}
	telegramUserID := "-"
	if u.TelegramID != 0 {
		telegramUserID = strconv.FormatInt(u.TelegramID, 10)
	}
	params["telegram_status"] = telegramBindingLabel(u)
	params["telegram_bound"] = boolLabel(u.TelegramID != 0, "是", "否")
	params["telegram_username"] = telegramUsername
	params["telegram_userid"] = telegramUserID
	params["telegram_id"] = telegramUserID
	params["rebinding_in_progress"] = boolLabel(u.RebindingInProgress, "是", "否")

	// 邮箱信息
	email := strings.TrimSpace(u.Email)
	if email == "" {
		email = "-"
	}
	params["email"] = email
	params["email_bound"] = boolLabel(u.Email != "", "是", "否")
	params["email_verified"] = boolLabel(u.EmailVerified, "是", "否")
	params["email_verified_status"] = boolLabel(u.EmailVerified, "已验证", "未验证")
	params["email_verified_at"] = unixTimeLabel(u.EmailVerifiedAt)

	// Bangumi 同步
	params["bgm_mode"] = enabledLabel(u.BGMMode)
	params["bgm_token_status"] = configuredLabel(u.BGMToken != "")
	bgmSyncStatus := "未启用"
	if u.BGMMode && u.BGMToken == "" {
		bgmSyncStatus = "缺少个人 Token"
	} else if u.BGMMode {
		bgmSyncStatus = "可同步"
	}
	params["bgm_sync_status"] = bgmSyncStatus

	// API Key
	params["api_key_status"] = enabledLabel(u.LegacyAPIKeyStatus)
	params["api_key_enabled"] = boolLabel(u.LegacyAPIKeyStatus, "是", "否")

	// 通知设置
	params["notify_login_telegram"] = boolLabel(u.NotifyOnLoginTelegram, "是", "否")
	params["notify_login_email"] = boolLabel(u.NotifyOnLoginEmail, "是", "否")
	params["notify_ticket_telegram"] = boolLabel(u.NotifyOnTicketTelegram, "是", "否")

	return params
}

// BuildWithExtra 构建参数集合并合并额外参数（如登录通知的 time、ip、device 等）
func (tp *TemplateParams) BuildWithExtra(extra map[string]string) map[string]string {
	params := tp.BuildAll()
	for k, v := range extra {
		params[k] = v
	}
	return params
}

// RenderTemplate 使用参数渲染模板字符串
func RenderTemplate(template string, params map[string]string) string {
	return telegramRenderPanelTemplate(template, params)
}

// 辅助函数：布尔值转换为标签
func boolLabel(value bool, trueLabel, falseLabel string) string {
	if value {
		return trueLabel
	}
	return falseLabel
}

// 辅助函数：到期剩余天数
func daysUntilExpiry(expiredAt int64) string {
	if expiredAt == 0 {
		return "永久"
	}
	now := time.Now().Unix()
	if expiredAt < now {
		return "已过期"
	}
	days := (expiredAt - now) / 86400
	if days == 0 {
		return "不足1天"
	}
	return strconv.FormatInt(days, 10) + "天"
}

// 辅助函数：Emby 禁用原因
func embyDisabledReason(u store.User) string {
	if u.EmbyID == "" {
		return "未绑定"
	}
	if !u.EmbyDisabled {
		return "正常"
	}
	if !u.Active {
		return "Web账号被禁用"
	}
	if u.ExpiredAt > 0 && u.ExpiredAt < time.Now().Unix() {
		return "账号已过期"
	}
	return "已禁用"
}

// 辅助函数：本地 Emby 标签
func localEmbyLabel(u store.User) string {
	if u.EmbyID == "" {
		return "未绑定"
	}
	if u.EmbyDisabled {
		return "已禁用"
	}
	return "已绑定"
}

// 辅助函数：本地 Emby 绑定状态标签
func localEmbyBindingStatusLabel(u store.User) string {
	if u.EmbyID == "" {
		if u.PendingEmby {
			return "等待开通"
		}
		return "未绑定"
	}
	return "已绑定"
}

// 辅助函数：Emby 启用状态标签
func embyEnabledStatusLabel(u store.User) string {
	if u.EmbyID == "" {
		return "-"
	}
	if u.EmbyDisabled {
		return "已禁用"
	}
	return "已启用"
}

// 辅助函数：Telegram 绑定标签
func telegramBindingLabel(u store.User) string {
	if u.TelegramID == 0 {
		return "未绑定"
	}
	if u.RebindingInProgress {
		return "换绑中"
	}
	return "已绑定"
}

// 辅助函数：Pending Emby 天数标签
func pendingEmbyDaysLabel(days *int) string {
	if days == nil {
		return "-"
	}
	if *days == 0 {
		return "永久"
	}
	return strconv.Itoa(*days) + "天"
}

// 辅助函数：活跃状态标签
func activeLabel(active bool) string {
	if active {
		return "正常"
	}
	return "已禁用"
}

// 辅助函数：启用状态标签
func enabledLabel(enabled bool) string {
	if enabled {
		return "已启用"
	}
	return "未启用"
}

// 辅助函数：配置状态标签
func configuredLabel(configured bool) string {
	if configured {
		return "已配置"
	}
	return "未配置"
}

// 辅助函数：时间标签
func unixTimeLabel(timestamp int64) string {
	if timestamp == 0 {
		return "-"
	}
	return time.Unix(timestamp, 0).Format("2006-01-02 15:04:05")
}

// 辅助函数：到期时间标签
func expiryTimeLabel(expiredAt int64) string {
	if expiredAt == 0 {
		return "永久"
	}
	return time.Unix(expiredAt, 0).Format("2006-01-02 15:04:05")
}
