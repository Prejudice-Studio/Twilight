package api

// 鉴权 / 注册域 handler。从 handlers.go 抽出来的目的：
//   - handlers.go 一度聚合 9+ 业务域 2888 行，新人接手时无法快速定位"登录这条
//     链路在哪"；
//   - 这里保留 login / login-by-apikey / direct-login-disabled / forgot-password
//     / logout / logout-all / refresh / current-user / register / register-availability
//     共 10 个端点，刚好覆盖"前端身份链路"，并把所有 rate_limit 决策（IP 桶 +
//     username 桶）集中到一处；
//   - 路由注册仍在 routes.go，不需要改动注册器。
//
// 修改时务必保持与原有契约一致：
//   - failWithCode 的 ErrCode 参数集中复用 errcode.go，不在这里临时新增；
//   - clientIP / allowRate / rateKey 走 App 公共方法，避免再写一份限流逻辑；
//   - publicUser、issueSessionCookies、clearSessionCookie 继续在 business.go /
//     app.go 维护；本文件只做"业务流程编排"。

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/security"
	"github.com/prejudice-studio/twilight/internal/store"
	"github.com/prejudice-studio/twilight/internal/validate"
)

// checkAvailableRatePerMin 限制 /api/v1/users/register/availability 的 IP 桶速率。
// 数值要点：
//   - 普通用户在注册表单上反复改用户名 < 10 次/分钟，30 留出宽裕缓冲；
//   - 脚本化用户名枚举攻击通常 60-1000 RPS，30/min 足以让其无法在合理时间
//     完成有意义的字典扫描；
//   - 该端点是 AuthPublic，无 cookie，所以唯一可控维度是 IP；命中后返回
//     RATE_LIMITED，前端走通用"稍后重试"路径，不暴露任何用户名差异。
const checkAvailableRatePerMin = 30

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.allowRate(r.Context(), rateKey("login:", a.clientIP(r)), a.cfg().RateLimitLoginPerMinute, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrLoginRateLimited, "登录过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	username := stringValue(payload, "username")
	password := stringValue(payload, "password")
	if username == "" || password == "" {
		failWithCode(w, http.StatusBadRequest, ErrAuthCredentialsEmpty, "用户名和密码不能为空")
		return
	}
	// 双桶限速：除按 IP 之外，再按用户名维度独立计数。
	// 仅 IP 桶时存在两类绕过：
	//   1) NAT/CGNAT 邻居共享同一公网 IP，正常用户被恶意邻居拖累；
	//   2) 攻击者用代理池 / IPv6 大池子各 ≤60/min 撞同一账号，单账号锁定失效。
	// user 桶预算更紧（10 次 / 5 分钟），且必须在解析到 username 之后才能算 key；
	// 这里把"username 是否存在"判断放在桶之后，避免攻击者通过响应时间差做账号
	// 枚举（无论账号是否存在，都消耗 user 桶配额）。
	if a.cfg().RateLimitLoginUserPer5m > 0 {
		userKey := strings.ToLower(strings.TrimSpace(username))
		if userKey != "" && !a.allowRate(r.Context(), rateKey("login:user:", userKey), a.cfg().RateLimitLoginUserPer5m, 5*time.Minute) {
			failWithCode(w, http.StatusTooManyRequests, ErrLoginRateLimited, "登录过于频繁，请稍后再试")
			return
		}
	}
	u, okUser := a.store().FindUserByUsername(username)
	if !okUser || !security.VerifyPassword(password, u.PasswordHash) {
		failWithCode(w, http.StatusUnauthorized, ErrLoginInvalid, "用户名或密码错误")
		return
	}
	if !u.Active {
		failWithCode(w, http.StatusForbidden, ErrAccountDisabled, "账号已被禁用")
		return
	}
	token, expires, err := a.sessions().Create(r.Context(), u.UID)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrSessionCreateFailed, "创建会话失败")
		return
	}
	csrf, err := a.issueSessionCookies(w, token, expires)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrSessionCreateFailed, "创建 CSRF 令牌失败")
		return
	}
	deviceID := firstNonEmpty(r.Header.Get("X-Twilight-Device"), r.UserAgent(), a.clientIP(r))
	_ = a.store().UpsertDevice(store.Device{UID: u.UID, DeviceID: deviceID, DeviceName: firstNonEmpty(r.UserAgent(), "unknown"), Client: "web", FirstSeen: time.Now().Unix(), LastSeen: time.Now().Unix()})
	_ = a.store().AddLoginLog(store.LoginLog{UID: u.UID, IP: a.clientIP(r), DeviceID: deviceID, DeviceName: firstNonEmpty(r.UserAgent(), "unknown"), Client: "web", Time: time.Now().Unix()})
	ok(w, "登录成功", map[string]any{"token": token, "csrf_token": csrf, "user": publicUser(u)})
}

func (a *App) handleLoginByAPIKey(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	key := stringValue(payload, "apikey")
	if key == "" {
		failWithCode(w, http.StatusBadRequest, ErrAPIKeyEmpty, "API Key 不能为空")
		return
	}
	_, u, okKey := a.store().FindAPIKeyByHash(hashAPIKey(key))
	if !okKey {
		failWithCode(w, http.StatusUnauthorized, ErrAPIKeyInvalid, "API Key 无效")
		return
	}
	token, expires, err := a.sessions().Create(r.Context(), u.UID)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrSessionCreateFailed, "创建会话失败")
		return
	}
	csrf, err := a.issueSessionCookies(w, token, expires)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrSessionCreateFailed, "创建 CSRF 令牌失败")
		return
	}
	ok(w, "登录成功", map[string]any{"token": token, "csrf_token": csrf, "user": publicUser(u)})
}

func (a *App) handleDirectLoginUnavailable(w http.ResponseWriter, r *http.Request, _ Params) {
	failWithCode(w, http.StatusForbidden, ErrDirectLoginDisabled, "直接登录未启用")
}

func (a *App) handleForgotPassword(w http.ResponseWriter, r *http.Request, _ Params) {
	ip := a.clientIP(r)
	if !a.allowRate(r.Context(), rateKey("forgot-password:ip:", ip), a.cfg().RateLimitForgotPasswordIPPer10m, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrPasswordResetTooMany, "重置密码尝试过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	embyUsername := stringValue(payload, "emby_username")
	embyPassword := stringValue(payload, "emby_password")
	if embyUsername == "" || embyPassword == "" {
		failWithCode(w, http.StatusBadRequest, ErrEmbyMissingCreds, "缺少 Emby 用户名或密码")
		return
	}
	if len(embyUsername) > 100 || len(embyPassword) > 200 {
		failWithCode(w, http.StatusBadRequest, ErrEmbyInputTooLong, "输入内容过长")
		return
	}
	if !a.allowRate(r.Context(), rateKey("forgot-password:user:", strings.ToLower(embyUsername)), a.cfg().RateLimitForgotPasswordUserPer30m, 30*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrPasswordResetTooMany, "该账号重置密码尝试过于频繁，请稍后再试")
		return
	}
	embyUser, okAuth, err := a.embyAuthenticateByName(r.Context(), embyUsername, embyPassword)
	if err != nil {
		failWithCode(w, http.StatusUnauthorized, ErrEmbyAuthFailed, "Emby 鉴权失败")
		return
	}
	if !okAuth {
		failWithCode(w, http.StatusUnauthorized, ErrLoginInvalid, "Emby 用户名或密码错误")
		return
	}
	embyID := firstNonEmpty(asString(embyUser["Id"]), asString(embyUser["ID"]), asString(embyUser["id"]))
	u, okUser := a.store().FindUserByEmbyID(embyID)
	if !okUser {
		failWithCode(w, http.StatusNotFound, ErrEmbyAccountUnlinked, "该 Emby 账号未关联面板账号")
		return
	}
	if !u.Active {
		failWithCode(w, http.StatusForbidden, ErrAccountDisabled, "账号已被禁用")
		return
	}
	newPassword := "Twilight-" + randomCode(18)
	hash, err := security.HashPassword(newPassword)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrPasswordHashFailed, "密码处理失败")
		return
	}
	u, err = a.store().UpdateUser(u.UID, func(u *store.User) error { u.PasswordHash = hash; return nil })
	if statusFromError(w, err) {
		return
	}
	if u.EmbyID != "" && a.cfg().EmbyURL != "" {
		_ = a.embySetUserEnabled(r.Context(), u.EmbyID, a.embyShouldEnableUser(u))
	}
	a.sessions().DeleteUser(r.Context(), u.UID)
	ok(w, "密码已重置", map[string]any{"username": u.Username, "new_password": newPassword})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	a.sessions().Delete(r.Context(), p.Token)
	a.clearSessionCookie(w)
	ok(w, "logged out", nil)
}

func (a *App) handleLogoutAll(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	a.sessions().DeleteUser(r.Context(), p.User.UID)
	a.clearSessionCookie(w)
	ok(w, "all sessions logged out", nil)
}

func (a *App) handleRefresh(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	a.sessions().Delete(r.Context(), p.Token)
	token, expires, err := a.sessions().Create(r.Context(), p.User.UID)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrAuthSessionRefreshFailed, "刷新会话失败")
		return
	}
	csrf, err := a.issueSessionCookies(w, token, expires)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrAuthCSRFRefreshFailed, "刷新 CSRF 令牌失败")
		return
	}
	ok(w, "刷新成功", map[string]any{"token": token, "csrf_token": csrf, "user": publicUser(p.User)})
}

func (a *App) handleCurrentUser(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", publicUser(current(r).User))
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.allowRate(r.Context(), rateKey("register:", a.clientIP(r)), a.cfg().RateLimitRegisterPer10m, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRegisterRateLimited, "注册过于频繁，请稍后再试")
		return
	}
	if !a.cfg().RegisterEnabled && a.store().UserCount() > 0 {
		failWithCode(w, http.StatusForbidden, ErrRegisterDisabled, "系统注册未开启")
		return
	}
	payload := decodeMap(r)
	username := stringValue(payload, "username")
	password := stringValue(payload, "password")
	telegramBindCode := strings.ToUpper(strings.TrimSpace(stringValue(payload, "telegram_bind_code")))
	if err := validate.ValidateUsername(username); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrUsernameInvalid, err.Error())
		return
	}
	// 当 ConfiguredAdmins 配置存在时，首注册必须命中（用户名 / UID）才允许在
	// RegisterEnabled=false 状态下走"空 DB bootstrap"通道。否则任何外部用户
	// 都可以在 RegisterEnabled=false 的部署里抢先注册并自动成为 Admin。
	//   - 配置了 AdminUsernames / AdminUIDs，且首注册命中 ⇒ 允许进入下游、role=Admin
	//   - 配置了 AdminUsernames / AdminUIDs，但首注册未命中 ⇒ 直接拒绝
	//   - 未配置 ConfiguredAdmins ⇒ 维持旧"首注册即 Admin"语义（兜底引导路径）
	hasConfiguredAdmins := len(a.cfg().AdminUIDs) > 0 || len(a.cfg().AdminUsernames) > 0
	bootstrapMode := a.store().UserCount() == 0
	if bootstrapMode && hasConfiguredAdmins && !configuredAdminMatchSets(a.configuredAdminUIDSet(), a.configuredAdminUsernameSet(), 0, username) {
		failWithCode(w, http.StatusForbidden, ErrRegisterDisabled, "系统初始管理员已通过配置指定，请使用配置的用户名注册")
		return
	}
	if err := validate.ValidatePasswordStrength(password); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrPasswordWeak, err.Error())
		return
	}
	if reached, current, limit := a.systemUserLimitReached(); reached {
		failWithCode(w, http.StatusConflict, ErrUserLimitReached, fmt.Sprintf("系统用户数量已达上限 %d/%d", current, limit))
		return
	}
	var telegramID int64
	var telegramUsername string
	if a.cfg().ForceBindTelegram || telegramBindCode != "" {
		if telegramBindCode == "" {
			failWithCode(w, http.StatusBadRequest, ErrTGBindRequired, "需要先完成 Telegram 绑定")
			return
		}
		if !telegramBindCodePattern.MatchString(telegramBindCode) {
			failWithCode(w, http.StatusBadRequest, ErrTGBindCodeFormat, "Telegram 绑定码格式不正确")
			return
		}
		bind, okBind := a.store().BindCode(telegramBindCode)
		switch {
		case !okBind || bind.ExpiresAt <= time.Now().Unix():
			if okBind {
				_ = a.store().DeleteBindCode(telegramBindCode)
			}
			failWithCode(w, http.StatusBadRequest, ErrTGBindCodeExpired, "绑定码无效或已过期")
			return
		case bind.Scene != "register" || bind.UID != 0:
			failWithCode(w, http.StatusBadRequest, ErrTGBindCodeSceneBad, "绑定码场景无效")
			return
		case !bind.Confirmed || bind.TelegramID == 0:
			failWithCode(w, http.StatusBadRequest, ErrTGBindCodeNotConfirm, "绑定码尚未在 Telegram 中确认")
			return
		}
		if existing, okUser := a.store().FindUserByTelegramID(bind.TelegramID); okUser {
			failWithCode(w, http.StatusConflict, ErrTGAlreadyBound, "该 Telegram 已绑定到账号 "+existing.Username)
			return
		}
		telegramID = bind.TelegramID
		telegramUsername = bind.TelegramUsername
	}
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrPasswordHashFailed, "密码处理失败")
		return
	}
	role := store.RoleNormal
	if a.store().UserCount() == 0 {
		role = store.RoleAdmin
	}
	u, err := a.store().CreateUser(store.User{Username: username, Email: stringValue(payload, "email"), PasswordHash: passwordHash, Role: role, TelegramID: telegramID, TelegramUsername: telegramUsername})
	if statusFromError(w, err) {
		return
	}
	if telegramBindCode != "" {
		_ = a.store().DeleteBindCode(telegramBindCode)
	}
	if a.configuredAdminMatch(u.UID, u.Username) {
		if promoted, err := a.store().UpdateUser(u.UID, func(user *store.User) error {
			user.Role = store.RoleAdmin
			user.Active = true
			return nil
		}); err == nil {
			u = promoted
			role = store.RoleAdmin
		}
	}
	created(w, "注册成功", map[string]any{"user": publicUser(u), "first_admin": role == store.RoleAdmin})
}

func (a *App) handleRegisterAvailability(w http.ResponseWriter, r *http.Request, _ Params) {
	// 反枚举：未限速时攻击者可遍历常见用户名表来收集账户清单。
	// 这里使用独立桶 register-availability:<ip>，30 次 / 分钟，足够普通用户在
	// 注册表单上反复尝试用户名，但封堵脚本化扫描。命中限速时返回 429 + RATE_LIMITED，
	// 前端按通用 RATE_LIMITED 引导即可。
	if !a.allowRate(r.Context(), rateKey("register-availability:", a.clientIP(r)), checkAvailableRatePerMin, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "请求过于频繁，请稍后再试")
		return
	}
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	available := true
	message := ""
	if username != "" {
		_, found := a.store().FindUserByUsername(username)
		available = !found
		if !available {
			message = "用户名已被使用"
		}
	}
	currentUsers := a.store().UserCount()
	canRegister := a.cfg().RegisterEnabled || currentUsers == 0
	if a.cfg().UserLimit > 0 && currentUsers >= a.cfg().UserLimit {
		canRegister = false
		available = false
		message = fmt.Sprintf("系统用户数量已达上限 %d/%d", currentUsers, a.cfg().UserLimit)
	}
	ok(w, "OK", map[string]any{
		"enabled":           a.cfg().RegisterEnabled,
		"can_register":      canRegister,
		"requires_reg_code": a.cfg().RegisterCodeLimit,
		"available":         available,
		"message":           message,
		"current_users":     currentUsers,
		"max_users":         a.cfg().UserLimit,
		"emby_user_limit":   a.cfg().EmbyUserLimit,
	})
}
