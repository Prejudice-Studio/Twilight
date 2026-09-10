package api

import (
	"context"
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

type setupResult struct {
	User               store.User
	Config             map[string]any
	ConfiguredSections []string
}

type setupFailure struct {
	Status  int
	Code    ErrCode
	Message string
}

func (e *setupFailure) Error() string { return e.Message }

func setupFail(status int, code ErrCode, message string) error {
	return &setupFailure{Status: status, Code: code, Message: message}
}

// completeSetup owns the one-time initialization state transition. The lock
// also serializes normal registration so a failed config write cannot leave a
// concurrent registration account behind while the first admin is rolled back.
// The Store-level CreateInitialAdmin remains the cross-process atomic empty
// system guard.
func (a *App) completeSetup(ctx context.Context, payload map[string]any) (setupResult, error) {
	a.setupMu.Lock()
	defer a.setupMu.Unlock()

	status := a.setupStatusData()
	if available, _ := status["available"].(bool); !available {
		return setupResult{}, setupFail(http.StatusForbidden, ErrForbidden, "初始化向导已关闭或当前系统已存在用户/管理员配置")
	}

	admin := setupObject(payload, "admin")
	username := stringValue(admin, "username")
	password := stringValue(admin, "password")
	email := stringValue(admin, "email")
	if err := validate.ValidateUsername(username); err != nil {
		return setupResult{}, setupFail(http.StatusBadRequest, ErrUsernameInvalid, err.Error())
	}
	if err := validate.ValidatePasswordStrength(password); err != nil {
		return setupResult{}, setupFail(http.StatusBadRequest, ErrPasswordWeak, err.Error())
	}
	if email != "" {
		if err := validate.ValidateEmailFormat(email); err != nil {
			return setupResult{}, setupFail(http.StatusBadRequest, ErrEmailInvalid, err.Error())
		}
		if a.store().EmailAlreadyUsed(email) {
			return setupResult{}, setupFail(http.StatusConflict, ErrEmailConflict, "该邮箱已被其他账号使用")
		}
	}

	values, err := a.setupConfigValues(payload)
	if err != nil {
		return setupResult{}, setupFail(http.StatusBadRequest, ErrInvalidPayload, err.Error())
	}
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		return setupResult{}, setupFail(http.StatusInternalServerError, ErrPasswordHashFailed, "密码处理失败")
	}

	now := time.Now().Unix()
	newUser := store.User{
		Username:        username,
		Email:           email,
		EmailVerified:   email != "",
		EmailVerifiedAt: now,
		PasswordHash:    passwordHash,
		Role:            store.RoleAdmin,
		Active:          true,
		ExpiredAt:       -1,
	}
	if email == "" {
		newUser.EmailVerifiedAt = 0
	}
	u, err := a.store().CreateInitialAdmin(newUser)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrSetupUnavailable):
			return setupResult{}, setupFail(http.StatusForbidden, ErrForbidden, "初始化向导已关闭或当前系统已存在用户/管理员配置")
		case errors.Is(err, store.ErrConflict):
			return setupResult{}, setupFail(http.StatusConflict, ErrUsernameTaken, "用户名或邮箱已被占用，请换一个")
		default:
			return setupResult{}, err
		}
	}

	info, saveStatus, _ := a.saveInitialSetupConfigContent(renderConfigTOML(values), u.Username)
	if saveStatus != http.StatusOK {
		if rollbackErr := a.deleteLocalUser(ctx, u); rollbackErr != nil {
			zap.L().Error("rollback setup admin user failed", zap.Int64("uid", u.UID), zap.Error(rollbackErr))
		}
		// Do not return filesystem/config parser details to a public setup client.
		return setupResult{}, setupFail(saveStatus, ErrInvalidPayload, "初始化配置保存失败，请检查填写内容后重试")
	}

	return setupResult{
		User:               u,
		Config:             info,
		ConfiguredSections: setupConfiguredSections(payload),
	}, nil
}

func (a *App) setupStatusData() map[string]any {
	reasons := []string{}
	userCount := 0
	if rt := a.runtimeSnapshot(); rt != nil && rt.store != nil {
		userCount = rt.store.UserCount()
		if userCount > 0 {
			reasons = append(reasons, "users_exist")
		}
		cfg := rt.cfg
		if !cfg.SetupMode {
			reasons = append(reasons, "setup_mode_disabled")
		}
		if len(cfg.AdminUIDs) > 0 {
			reasons = append(reasons, "admin_uids_configured")
		}
		if len(cfg.AdminUsernames) > 0 {
			reasons = append(reasons, "admin_usernames_configured")
		}
		_, statErr := os.Stat(cfg.ConfigFile)
		return map[string]any{
			"available":          cfg.SetupMode && userCount == 0 && len(cfg.AdminUIDs) == 0 && len(cfg.AdminUsernames) == 0,
			"setup_mode":         cfg.SetupMode,
			"reasons":            reasons,
			"user_count":         userCount,
			"config_file_exists": statErr == nil,
		}
	}
	return map[string]any{"available": false, "setup_mode": false, "reasons": []string{"runtime_unavailable"}, "user_count": 0, "config_file_exists": false}
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
