package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/security"
	"github.com/prejudice-studio/twilight/internal/store"
)

type loginInput struct {
	Username  string
	Email     string
	Password  string
	DeviceID  string
	UserAgent string
	IP        string
}

type loginResult struct {
	User   store.User
	Token  string
	Expiry time.Time
}

type loginFailure struct {
	Status  int
	Code    ErrCode
	Message string
}

func (e *loginFailure) Error() string { return e.Message }

func loginFail(status int, code ErrCode, message string) error {
	return &loginFailure{Status: status, Code: code, Message: message}
}

func (a *App) authenticateLogin(input loginInput) (store.User, error) {
	if (input.Username == "" && input.Email == "") || input.Password == "" {
		return store.User{}, loginFail(http.StatusBadRequest, ErrAuthCredentialsEmpty, "用户名/邮箱和密码不能为空")
	}

	loginByEmail := input.Email != "" || strings.Contains(input.Username, "@")
	var user store.User
	var found bool
	if loginByEmail {
		lookupEmail := input.Email
		if lookupEmail == "" {
			lookupEmail = input.Username
		}
		user, found = a.store().FindUserByEmail(lookupEmail)
	} else {
		user, found = a.store().FindUserByUsername(input.Username)
	}

	encoded := dummyPasswordHash()
	if found {
		encoded = user.PasswordHash
	}
	valid := verifyPasswordThrottled(input.Password, encoded)
	if !found || !valid {
		return store.User{}, loginFail(http.StatusUnauthorized, ErrLoginInvalid, "用户名或密码错误")
	}
	if !user.Active {
		if userExpiredOnly(user) {
			return store.User{}, loginFail(http.StatusForbidden, ErrAccountExpired, "账号有效期已到期，请续费后再登录")
		}
		return store.User{}, loginFail(http.StatusForbidden, ErrAccountDisabled, "账号已被禁用")
	}

	if security.NeedsRehash(user.PasswordHash) {
		if hash, err := security.HashPassword(input.Password); err == nil {
			_, _ = a.store().UpdateUser(user.UID, func(existing *store.User) error {
				existing.PasswordHash = hash
				return nil
			})
		}
	}
	return user, nil
}

func (a *App) completeLogin(r *http.Request, input loginInput, user store.User) (loginResult, error) {
	token, expiry, err := a.sessions().Create(r.Context(), user.UID)
	if err != nil {
		return loginResult{}, loginFail(http.StatusInternalServerError, ErrSessionCreateFailed, "创建会话失败")
	}

	now := time.Now().Unix()
	deviceID := firstNonEmpty(input.DeviceID, input.UserAgent, input.IP)
	userAgent := firstNonEmpty(input.UserAgent, "unknown")
	_ = a.store().UpdateDevice(user.UID, deviceID, func(device *store.Device) {
		device.DeviceName = userAgent
		device.Client = "web"
		device.LastIP = input.IP
		device.LastSeen = now
	})
	_ = a.store().AddLoginLog(store.LoginLog{
		UID: user.UID, IP: input.IP, DeviceID: deviceID, DeviceName: userAgent, Client: "web", Time: now,
	})
	a.auditWithUser(r, user.UID, user.Username, "login", "user", 0, map[string]any{"ip": input.IP, "device": deviceID})

	// 使用统一的模板参数系统，支持所有用户状态参数
	templateParams := a.NewTemplateParams(r.Context(), user).BuildWithExtra(map[string]string{
		"time":   time.Now().Format("2006-01-02 15:04:05"),
		"ip":     input.IP,
		"device": userAgent,
	})
	if a.telegramAvailable() && user.TelegramID != 0 && user.NotifyOnLoginTelegram {
		template := a.cfg().LoginNotifyTelegramTemplate
		if template == "" {
			template = config.DefaultLoginNotifyTelegramTemplate
		}
		a.telegramSendMessage(r.Context(), user.TelegramID, RenderTemplate(template, templateParams))
	}
	if emailConfigured(a.cfg()) && user.Email != "" && user.EmailVerified && user.NotifyOnLoginEmail {
		subjectTemplate := a.cfg().LoginNotifyEmailSubjectTemplate
		if subjectTemplate == "" {
			subjectTemplate = config.DefaultLoginNotifyEmailSubjectTemplate
		}
		bodyTemplate := a.cfg().LoginNotifyEmailBodyTemplate
		if bodyTemplate == "" {
			bodyTemplate = config.DefaultLoginNotifyEmailBodyTemplate
		}
		smtpDeliver(r.Context(), *a.cfg(), user.Email,
			RenderTemplate(subjectTemplate, templateParams),
			RenderTemplate(bodyTemplate, templateParams))
	}
	if cfg := a.cfg(); cfg.DeviceLimitEnabled && cfg.MaxDevices > 0 {
		_ = a.store().EnforceDeviceLimit(user.UID, cfg.MaxDevices)
	}
	return loginResult{User: user, Token: token, Expiry: expiry}, nil
}

func (a *App) handleLoginResource(w http.ResponseWriter, r *http.Request) {
	if !a.allowRate(r.Context(), rateKey("login:", a.clientIP(r)), a.cfg().RateLimitLoginPerMinute, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrLoginRateLimited, "登录过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	input := loginInput{
		Username:  stringValue(payload, "username"),
		Email:     stringValue(payload, "email"),
		Password:  stringValue(payload, "password"),
		DeviceID:  r.Header.Get("X-Twilight-Device"),
		UserAgent: r.UserAgent(),
		IP:        a.clientIP(r),
	}
	user, err := a.authenticateLogin(input)
	if err != nil {
		if failure, ok := err.(*loginFailure); ok && failure.Code == ErrLoginInvalid && a.cfg().RateLimitLoginUserPer5m > 0 {
			userKey := strings.ToLower(strings.TrimSpace(input.Username))
			if input.Email != "" && userKey == "" {
				userKey = strings.ToLower(strings.TrimSpace(input.Email))
			}
			if userKey != "" && !a.allowRate(r.Context(), rateKey("login:user:", userKey), a.cfg().RateLimitLoginUserPer5m, 5*time.Minute) {
				failWithCode(w, http.StatusTooManyRequests, ErrLoginRateLimited, "登录过于频繁，请稍后再试")
				return
			}
		}
		if failure, ok := err.(*loginFailure); ok {
			failWithCode(w, failure.Status, failure.Code, failure.Message)
			return
		}
		statusFromError(w, err)
		return
	}
	result, err := a.completeLogin(r, input, user)
	if err != nil {
		if failure, ok := err.(*loginFailure); ok {
			failWithCode(w, failure.Status, failure.Code, failure.Message)
			return
		}
		statusFromError(w, err)
		return
	}
	a.issueSessionCookies(w, result.Token, result.Expiry)
	ok(w, "登录成功", map[string]any{"token": result.Token, "user": publicUser(result.User)})
}
