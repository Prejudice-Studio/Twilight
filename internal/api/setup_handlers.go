package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/security"
	"github.com/prejudice-studio/twilight/internal/store"
	"github.com/prejudice-studio/twilight/internal/validate"
	"go.uber.org/zap"
)

func (a *App) handleSetupStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", a.setupStatusData())
}

func (a *App) handleSetupComplete(w http.ResponseWriter, r *http.Request, _ Params) {
	if !requireWebUIIntent(w, r, twilightIntentCompleteSetup) {
		return
	}
	limiter := a.limiter()
	if limiter != nil && !limiter.Allow(r.Context(), rateKey("setup:", a.clientIP(r)), 5, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "初始化尝试过于频繁，请稍后再试")
		return
	}

	a.setupMu.Lock()
	defer a.setupMu.Unlock()

	status := a.setupStatusData()
	if available, _ := status["available"].(bool); !available {
		failWithCode(w, http.StatusForbidden, ErrForbidden, "初始化向导已关闭或当前系统已存在用户/管理员配置")
		return
	}

	payload := decodeMap(r)
	admin := setupObject(payload, "admin")
	username := stringValue(admin, "username")
	password := stringValue(admin, "password")
	email := stringValue(admin, "email")
	if err := validate.ValidateUsername(username); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrUsernameInvalid, err.Error())
		return
	}
	if _, exists := a.store().FindUserByUsername(username); exists {
		failWithCode(w, http.StatusConflict, ErrUsernameTaken, "用户名已被占用，请换一个用户名")
		return
	}
	if err := validate.ValidatePasswordStrength(password); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrPasswordWeak, err.Error())
		return
	}
	if email != "" {
		if err := validate.ValidateEmailFormat(email); err != nil {
			failWithCode(w, http.StatusBadRequest, ErrEmailInvalid, err.Error())
			return
		}
		if a.store().EmailAlreadyUsed(email) {
			failWithCode(w, http.StatusConflict, ErrEmailConflict, "该邮箱已被其他账号使用")
			return
		}
	}

	values, err := a.setupConfigValues(payload)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, err.Error())
		return
	}
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrPasswordHashFailed, "密码处理失败")
		return
	}

	now := time.Now().Unix()
	newUser := store.User{
		Username:        username,
		Email:           email,
		EmailVerified:   email != "",
		EmailVerifiedAt: 0,
		PasswordHash:    passwordHash,
		Role:            store.RoleAdmin,
		Active:          true,
		ExpiredAt:       -1,
	}
	if email != "" {
		newUser.EmailVerifiedAt = now
	}
	u, err := a.store().CreateUser(newUser)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			failWithCode(w, http.StatusConflict, ErrUsernameTaken, "用户名已被占用，请换一个用户名")
			return
		}
		if statusFromError(w, err) {
			return
		}
		failWithCode(w, http.StatusInternalServerError, ErrInternal, "创建初始化管理员失败")
		return
	}

	info, saveStatus, message := a.saveInitialSetupConfigContent(renderConfigTOML(values), u.Username)
	if saveStatus != http.StatusOK {
		if err := a.deleteLocalUser(r.Context(), u); err != nil {
			zap.L().Error("rollback setup admin user failed", zap.Int64("uid", u.UID), zap.Error(err))
		}
		failWithCode(w, saveStatus, ErrInvalidPayload, message)
		return
	}

	token, expires, err := a.sessions().Create(r.Context(), u.UID)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrSessionCreateFailed, "初始化已完成，但自动登录会话创建失败，请返回登录页手动登录")
		return
	}
	a.issueSessionCookies(w, token, expires)
	deviceID := firstNonEmpty(r.Header.Get("X-Twilight-Device"), r.UserAgent(), a.clientIP(r))
	ua := firstNonEmpty(r.UserAgent(), "unknown")
	ip := a.clientIP(r)
	_ = a.store().UpdateDevice(u.UID, deviceID, func(d *store.Device) {
		d.DeviceName = ua
		d.Client = "web"
		d.LastIP = ip
		d.LastSeen = now
	})
	_ = a.store().AddLoginLog(store.LoginLog{UID: u.UID, IP: ip, DeviceID: deviceID, DeviceName: ua, Client: "web", Time: now})
	a.auditWithUser(r, u.UID, u.Username, "complete_setup_wizard", "system", u.UID, map[string]any{
		"configured_sections": setupConfiguredSections(payload),
		"ip":                  ip,
		"device":              deviceID,
	})

	created(w, "初始化完成", map[string]any{
		"user":            publicUser(u),
		"setup_completed": true,
		"config":          info,
	})
}

func (a *App) setupStatusData() map[string]any {
	reasons := []string{}
	userCount := 0
	if a.store() != nil {
		userCount = a.store().UserCount()
		if userCount > 0 {
			reasons = append(reasons, "users_exist")
		}
	}
	cfg := a.cfg()
	if !cfg.SetupMode {
		reasons = append(reasons, "setup_mode_disabled")
	}
	if len(cfg.AdminUIDs) > 0 {
		reasons = append(reasons, "admin_uids_configured")
	}
	if len(cfg.AdminUsernames) > 0 {
		reasons = append(reasons, "admin_usernames_configured")
	}
	available := cfg.SetupMode && userCount == 0 && len(cfg.AdminUIDs) == 0 && len(cfg.AdminUsernames) == 0
	_, statErr := os.Stat(a.configFilePath())
	return map[string]any{
		"available":          available,
		"setup_mode":         cfg.SetupMode,
		"reasons":            reasons,
		"user_count":         userCount,
		"config_file_exists": statErr == nil,
	}
}

func (a *App) setupConfigValues(payload map[string]any) (map[string]map[string]any, error) {
	values := configValues(*a.cfg())

	global := setupObject(payload, "global")
	if name := stringValue(global, "server_name"); name != "" {
		if len([]rune(name)) > 64 {
			return nil, fmt.Errorf("站点名称不能超过 64 个字符")
		}
		values["Global"]["server_name"] = name
	}

	emby := setupObject(payload, "emby")
	if embyURL := stringValue(emby, "emby_url"); embyURL != "" {
		if err := validateSetupHTTPURL(embyURL, "Emby 地址"); err != nil {
			return nil, err
		}
		values["Emby"]["emby_url"] = embyURL
	}
	if token := stringValue(emby, "emby_token"); token != "" {
		values["Emby"]["emby_token"] = token
	}
	if lines, err := setupLineList(emby["emby_url_list"]); err != nil {
		return nil, err
	} else if lines != nil {
		values["Emby"]["emby_url_list"] = lines
	}

	telegram := setupObject(payload, "telegram")
	if _, ok := telegram["enabled"]; ok {
		values["Global"]["telegram_mode"] = boolValue(telegram, "enabled", false)
	}
	if token := stringValue(telegram, "bot_token"); token != "" {
		values["Telegram"]["bot_token"] = token
	}
	if adminIDs := setupList(telegram["admin_id"]); adminIDs != nil {
		values["Telegram"]["admin_id"] = adminIDs
	}

	email := setupObject(payload, "email")
	if _, ok := email["enabled"]; ok {
		values["Email"]["enabled"] = boolValue(email, "enabled", false)
	}
	for _, key := range []string{"smtp_host", "smtp_username", "smtp_password", "smtp_from_address", "smtp_from_name", "smtp_encryption"} {
		if value := stringValue(email, key); value != "" {
			values["Email"][key] = value
		}
	}
	if _, ok := email["smtp_port"]; ok {
		port := intValue(email, "smtp_port", 0)
		if port <= 0 || port > 65535 {
			return nil, fmt.Errorf("SMTP 端口必须在 1-65535 之间")
		}
		values["Email"]["smtp_port"] = port
	}

	policy := setupObject(payload, "policy")
	for _, key := range []string{"register_mode", "register_code_limit", "allow_pending_register"} {
		if _, ok := policy[key]; ok {
			values["SAR"][key] = boolValue(policy, key, false)
		}
	}
	ensureTicketDefaults(values)
	return values, nil
}

func setupObject(payload map[string]any, key string) map[string]any {
	if raw, ok := payload[key].(map[string]any); ok {
		return raw
	}
	return map[string]any{}
}

func setupList(raw any) []any {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(fmt.Sprint(item))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func setupLineList(raw any) ([]any, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, nil
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		var name, lineURL string
		switch typed := item.(type) {
		case string:
			lineURL = strings.TrimSpace(typed)
		case map[string]any:
			name = stringValue(typed, "name")
			lineURL = stringValue(typed, "url")
		default:
			lineURL = strings.TrimSpace(fmt.Sprint(item))
		}
		if lineURL == "" {
			continue
		}
		if err := validateSetupHTTPURL(lineURL, "Emby 线路地址"); err != nil {
			return nil, err
		}
		if name != "" {
			out = append(out, name+" : "+lineURL)
		} else {
			out = append(out, lineURL)
		}
	}
	return out, nil
}

func validateSetupHTTPURL(value, label string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s格式不正确", label)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s不能包含用户名或密码", label)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%s仅支持 http 或 https", label)
	}
	return nil
}

func setupConfiguredSections(payload map[string]any) []string {
	sections := []string{"admin"}
	for _, key := range []string{"global", "emby", "telegram", "email", "policy"} {
		section := setupObject(payload, key)
		if len(section) == 0 {
			continue
		}
		hasValue := false
		for field, value := range section {
			if strings.Contains(strings.ToLower(field), "password") || strings.Contains(strings.ToLower(field), "token") {
				if strings.TrimSpace(fmt.Sprint(value)) != "" {
					hasValue = true
				}
				continue
			}
			switch typed := value.(type) {
			case string:
				hasValue = strings.TrimSpace(typed) != ""
			case []any:
				hasValue = len(typed) > 0
			default:
				hasValue = true
			}
			if hasValue {
				break
			}
		}
		if hasValue {
			sections = append(sections, key)
		}
	}
	return sections
}
