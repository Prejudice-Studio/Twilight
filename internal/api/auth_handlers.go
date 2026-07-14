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
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/prejudice-studio/twilight/internal/config"
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
	email := stringValue(payload, "email")
	password := stringValue(payload, "password")
	if (username == "" && email == "") || password == "" {
		failWithCode(w, http.StatusBadRequest, ErrAuthCredentialsEmpty, "用户名/邮箱和密码不能为空")
		return
	}
	// 支持邮箱登录：email 字段优先于 username，或 username 本身常 @ 时视为邮箱
	loginByEmail := email != "" || strings.Contains(username, "@")
	var u store.User
	var okUser bool
	if loginByEmail {
		lookupEmail := email
		if lookupEmail == "" {
			lookupEmail = username
		}
		u, okUser = a.store().FindUserByEmail(lookupEmail)
	} else {
		u, okUser = a.store().FindUserByUsername(username)
	}
	// 常量代价校验：用户名不存在时也对占位哈希跑一次等代价 PBKDF2，抹平
	// "不存在(快) vs 存在但密码错(慢 ~150ms)"的时序差，避免用户名枚举旁路。
	// verifyPasswordThrottled 还把并发哈希数压到 GOMAXPROCS-1，防 CPU 饿死。
	encoded := dummyPasswordHash()
	if okUser {
		encoded = u.PasswordHash
	}
	valid := verifyPasswordThrottled(password, encoded)
	if !okUser || !valid {
		// 每用户名桶（10 次 / 5 分钟）只在「认证失败」时计数。
		// 旧实现在认证前就消耗该桶，任何人都能用垃圾请求把受害者（尤其是已知
		// 用户名的管理员）的桶打满，造成定向账号锁定 DoS。改为仅对失败计数后：
		//   - 攻击者的垃圾尝试只会节流攻击者自己（撞库防护保留：分布式攻击同样
		//     按用户名累计失败，10 次/5min 后该用户名被 429）；
		//   - 持有正确密码的受害者认证成功、不触碰该桶，永不被锁定。
		// 计数在常量代价校验之后进行，不影响上面的时序均一性。
		if a.cfg().RateLimitLoginUserPer5m > 0 {
			userKey := strings.ToLower(strings.TrimSpace(username))
			if loginByEmail && userKey == "" {
				userKey = strings.ToLower(strings.TrimSpace(email))
			}
			if userKey != "" && !a.allowRate(r.Context(), rateKey("login:user:", userKey), a.cfg().RateLimitLoginUserPer5m, 5*time.Minute) {
				failWithCode(w, http.StatusTooManyRequests, ErrLoginRateLimited, "登录过于频繁，请稍后再试")
				return
			}
		}
		failWithCode(w, http.StatusUnauthorized, ErrLoginInvalid, "用户名或密码错误")
		return
	}
	if !u.Active {
		// 优先走 ErrAccountExpired，让 webui 把"账号到期需续费"和"管理员
		// 主动禁用"两条 CTA 分开。check_expired 调度对非邀请用户会同时
		// Active=false + ExpiredAt<now，单看 Active 分不出原因；这里以
		// "ExpiredAt 落在过去"为信号区分。
		if userExpiredOnly(u) {
			failWithCode(w, http.StatusForbidden, ErrAccountExpired, "账号有效期已到期，请续费后再登录")
			return
		}
		failWithCode(w, http.StatusForbidden, ErrAccountDisabled, "账号已被禁用")
		return
	}
	// 登录成功后透明升级陈旧哈希（legacy Python salt$sha256，或迭代数低于当前门槛
	// 的 PBKDF2）。尽力而为：UpdateUser 失败不阻断本次登录。放在 VerifyPassword
	// 成功之后，损坏哈希不可能走到这里（那会先让校验失败）。
	if security.NeedsRehash(u.PasswordHash) {
		if h, hErr := security.HashPassword(password); hErr == nil {
			_, _ = a.store().UpdateUser(u.UID, func(uu *store.User) error { uu.PasswordHash = h; return nil })
		}
	}
	token, expires, err := a.sessions().Create(r.Context(), u.UID)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrSessionCreateFailed, "创建会话失败")
		return
	}
	a.issueSessionCookies(w, token, expires)
	deviceID := firstNonEmpty(r.Header.Get("X-Twilight-Device"), r.UserAgent(), a.clientIP(r))
	ua := firstNonEmpty(r.UserAgent(), "unknown")
	ip := a.clientIP(r)
	now := time.Now().Unix()
	// 用 UpdateDevice 做读改写：保留既有 FirstSeen / Trusted / Blocked，只刷新本次
	// 的 UA / IP / 最近登录时间。此前用 UpsertDevice 直接整条覆盖，会把每次登录的
	// FirstSeen 重置、并把受信任 / 已封禁标记清掉（被封设备再次登录即被静默解封）。
	_ = a.store().UpdateDevice(u.UID, deviceID, func(d *store.Device) {
		d.DeviceName = ua
		d.Client = "web"
		d.LastIP = ip
		d.LastSeen = now
	})
	_ = a.store().AddLoginLog(store.LoginLog{UID: u.UID, IP: ip, DeviceID: deviceID, DeviceName: ua, Client: "web", Time: now})
	// 登录是 AuthPublic 接口，此时请求上下文尚无 principal（会话 Cookie 在响应里下发），
	// 因此必须用 auditWithUser 显式传入已认证的用户身份，避免审计日志 uid=0/username=""。
	a.auditWithUser(r, u.UID, u.Username, "login", "user", 0, map[string]any{"ip": ip, "device": deviceID})
	// 登录通知：如果用户启用了 Telegram/邮箱登录通知，发送通知。
	loginTime := time.Now().Format("2006-01-02 15:04:05")
	notifValues := map[string]string{
		"{username}":    u.Username,
		"{time}":        loginTime,
		"{ip}":          ip,
		"{device}":      ua,
		"{server_name}": a.cfg().AppName,
	}
	if a.telegramAvailable() && u.TelegramID != 0 && u.NotifyOnLoginTelegram {
		tmpl := a.cfg().LoginNotifyTelegramTemplate
		if tmpl == "" {
			tmpl = config.DefaultLoginNotifyTelegramTemplate
		}
		text := replaceNotifPlaceholders(tmpl, notifValues)
		a.telegramSendMessage(r.Context(), u.TelegramID, text)
	}
	if emailConfigured(a.cfg()) && u.Email != "" && u.EmailVerified && u.NotifyOnLoginEmail {
		subjectTmpl := a.cfg().LoginNotifyEmailSubjectTemplate
		if subjectTmpl == "" {
			subjectTmpl = config.DefaultLoginNotifyEmailSubjectTemplate
		}
		bodyTmpl := a.cfg().LoginNotifyEmailBodyTemplate
		if bodyTmpl == "" {
			bodyTmpl = config.DefaultLoginNotifyEmailBodyTemplate
		}
		subject := replaceNotifPlaceholders(subjectTmpl, notifValues)
		body := replaceNotifPlaceholders(bodyTmpl, notifValues)
		smtpDeliver(r.Context(), *a.cfg(), u.Email, subject, body)
	}
	// 设备数限制为可选（默认关闭）：开启后淘汰超额的未受信任旧设备，绝不踢掉本次
	// 登录设备或受信任设备，避免把用户锁在门外。
	if cfg := a.cfg(); cfg.DeviceLimitEnabled && cfg.MaxDevices > 0 {
		_ = a.store().EnforceDeviceLimit(u.UID, cfg.MaxDevices)
	}
	ok(w, "登录成功", map[string]any{"token": token, "user": publicUser(u)})
}

func (a *App) handleLoginByAPIKey(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.allowRate(r.Context(), rateKey("login:apikey:ip:", a.clientIP(r)), a.cfg().RateLimitLoginPerMinute, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrLoginRateLimited, "登录过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	key := stringValue(payload, "apikey")
	if key == "" {
		failWithCode(w, http.StatusBadRequest, ErrAPIKeyEmpty, "API Key 不能为空")
		return
	}
	keyHash := hashAPIKey(key)
	if a.cfg().RateLimitLoginUserPer5m > 0 && !a.allowRate(r.Context(), rateKey("login:apikey:key:", keyHash), a.cfg().RateLimitLoginUserPer5m, 5*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrLoginRateLimited, "登录过于频繁，请稍后再试")
		return
	}
	_, u, okKey := a.store().FindAPIKeyByHash(keyHash)
	if !okKey {
		failWithCode(w, http.StatusUnauthorized, ErrAPIKeyInvalid, "API Key 无效")
		return
	}
	// 与 handleLogin 对齐：禁用账号不能凭 API Key 重新拿到 session。
	// 旧路径只查 API Key 命中即建会话，导致管理员把账号 Active=false 后
	// 该用户仍可继续访问；handleLogin 走的是 password 路径有 u.Active 守卫，
	// 这里属于同一身份链路必须共享同一不变量。
	// 同样区分 ExpiredAt-触发 vs admin 禁用，让前端按 ErrAccountExpired
	// 把 API key login 失败也引导到续费流。
	if !u.Active {
		if userExpiredOnly(u) {
			failWithCode(w, http.StatusForbidden, ErrAccountExpired, "账号有效期已到期，请续费后再登录")
			return
		}
		failWithCode(w, http.StatusForbidden, ErrAccountDisabled, "账号已被禁用")
		return
	}
	token, expires, err := a.sessions().Create(r.Context(), u.UID)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrSessionCreateFailed, "创建会话失败")
		return
	}
	a.issueSessionCookies(w, token, expires)
	ok(w, "登录成功", map[string]any{"token": token, "user": publicUser(u)})
}

func (a *App) handleDirectLoginUnavailable(w http.ResponseWriter, r *http.Request, _ Params) {
	failWithCode(w, http.StatusForbidden, ErrDirectLoginDisabled, "直接登录未启用")
}

func (a *App) handleForgotPassword(w http.ResponseWriter, r *http.Request, _ Params) {
	cfg := a.cfg()
	if !cfg.ForgotPasswordEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrForgotPasswordDisabled, "找回密码功能已关闭")
		return
	}
	if !cfg.ForgotPasswordEmbyEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrForgotPasswordDisabled, "通过 Emby 找回密码已关闭")
		return
	}
	ip := a.clientIP(r)
	if !a.allowRate(r.Context(), rateKey("forgot-password:ip:", ip), cfg.RateLimitForgotPasswordIPPer10m, 10*time.Minute) {
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
	if !a.allowRate(r.Context(), rateKey("forgot-password:user:", strings.ToLower(embyUsername)), cfg.RateLimitForgotPasswordUserPer30m, 30*time.Minute) {
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
		if userExpiredOnly(u) {
			failWithCode(w, http.StatusForbidden, ErrAccountExpired, "账号有效期已到期，请续费后再重置密码")
			return
		}
		failWithCode(w, http.StatusForbidden, ErrAccountDisabled, "账号已被禁用")
		return
	}
	// R62-7：账号 Active=true 但 entitlement 已过期（ExpiredAt < now）时不再
	// 重置密码并往 emby 写新密码。两条原因：
	//   1. embyShouldEnableUser 在过期态会返回 false，下面那条
	//      embySetUserEnabled 会立即把账号 disable 掉——发出去的"new_password"
	//      用户拿去登录会立刻被 emby 拒，UX 是"我刚改了密码就登不上"；
	//   2. 攻击者只要凭 emby 密码就能换出一份"理论可用"的面板凭据，把已经
	//      软冻结的账号当成绕开续费的入口。
	// 这里返回 ErrAccountExpired 与 !u.Active && expired 分支同口径——前端
	// 已经按这条错误码引导到"账号到期，请续费"，对用户最不困惑。
	if !userEntitlementOK(u) {
		failWithCode(w, http.StatusForbidden, ErrAccountExpired, "账号有效期已到期，请先续期再重置密码")
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
	a.issueSessionCookies(w, token, expires)
	ok(w, "刷新成功", map[string]any{"token": token, "user": publicUser(p.User)})
}

func (a *App) handleCurrentUser(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", publicUser(current(r).User))
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.allowRate(r.Context(), rateKey("register:", a.clientIP(r)), a.cfg().RateLimitRegisterPer10m, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRegisterRateLimited, "注册过于频繁，请稍后再试")
		return
	}
	currentUsers := a.store().UserCount()
	if !a.cfg().RegisterEnabled && currentUsers > 0 {
		failWithCode(w, http.StatusForbidden, ErrRegisterDisabled, "系统注册未开启")
		return
	}
	payload := decodeMap(r)
	username := stringValue(payload, "username")
	password := stringValue(payload, "password")
	regCode := firstNonEmpty(stringValue(payload, "reg_code"), stringValue(payload, "code"))
	telegramBindCode := strings.ToUpper(strings.TrimSpace(stringValue(payload, "telegram_bind_code")))
	email := strings.TrimSpace(stringValue(payload, "email"))
	if err := validate.ValidateUsername(username); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrUsernameInvalid, err.Error())
		return
	}
	if _, exists := a.store().FindUserByUsername(username); exists {
		failWithCode(w, http.StatusConflict, ErrUsernameTaken, "用户名已被占用，请换一个用户名")
		return
	}
	// bootstrapMode：空数据库首次注册。它只用来放宽"必须填注册码 / 必须已开放
	// 注册"这类首启 UX 限制（见下方 RegisterCodeLimit 判断），不再授予任何管理员
	// 身份——管理员只能由配置文件 admin_uids / admin_usernames 列表在创建后提升。
	bootstrapMode := currentUsers == 0
	if err := validate.ValidatePasswordStrength(password); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrPasswordWeak, err.Error())
		return
	}
	if email != "" {
		if err := validate.ValidateEmailFormat(email); err != nil {
			failWithCode(w, http.StatusBadRequest, ErrEmailInvalid, err.Error())
			return
		}
		if len(a.cfg().EmailBlacklist) > 0 && validate.CheckEmailBlacklist(email, a.cfg().EmailBlacklist) {
			failWithCode(w, http.StatusBadRequest, ErrEmailInvalid, "该邮箱域名不在允许范围内")
			return
		}
		if len(a.cfg().EmailWhitelist) > 0 && !validate.CheckEmailWhitelist(email, a.cfg().EmailWhitelist) {
			failWithCode(w, http.StatusBadRequest, ErrEmailInvalid, "该邮箱域名不在允许范围内")
			return
		}
		if a.store().EmailAlreadyUsed(email) {
			failWithCode(w, http.StatusConflict, ErrEmailConflict, "该邮箱已被其他账号使用")
			return
		}
	}
	if reached, current, limit := a.systemUserLimitReached(); reached {
		failWithCode(w, http.StatusConflict, ErrUserLimitReached, fmt.Sprintf("系统用户数量已达上限 %d/%d", current, limit))
		return
	}
	var registerReg store.RegCode
	if a.cfg().RegisterCodeLimit && !bootstrapMode {
		if regCode == "" {
			failWithCode(w, http.StatusBadRequest, ErrCodeEmpty, "注册需要提供注册码")
			return
		}
		if !a.allowRate(r.Context(), rateKey("register:regcode:", a.clientIP(r)), 10, time.Minute) {
			failWithCode(w, http.StatusTooManyRequests, ErrRegisterRateLimited, "注册码注册尝试过于频繁，请稍后再试")
			return
		}
		reg, okReg := a.store().RegCode(regCode)
		if !okReg || reg.IsDecoy || reg.Type != 1 || regcodeStatus(reg) != "available" {
			failWithCode(w, http.StatusBadRequest, ErrRegcodeInvalid, "注册码无效、已用完或已过期")
			return
		}
		registerReg = reg
	}
	var telegramID int64
	var telegramUsername string
	if a.cfg().ForceBindTelegram || telegramBindCode != "" {
		if telegramBindCode == "" {
			failWithCode(w, http.StatusBadRequest, ErrTGBindRequired, "需要先完成 Telegram 绑定")
			return
		}
		bindState := a.telegramBindCodeState(telegramBindCode, 0, "register", time.Now().Unix(), true)
		if bindState.Status != "confirmed" {
			status := bindState.HTTPStatus
			if status == 0 {
				status = http.StatusBadRequest
			}
			failWithCode(w, status, bindState.ErrorCode, bindState.Message)
			return
		}
		telegramID = bindState.TelegramID
		telegramUsername = bindState.TelegramUsername
	}
	if registerReg.Code != "" && regcodeTargetMismatchReason(registerReg, store.User{Username: username, TelegramID: telegramID, TelegramUsername: telegramUsername}) != "" {
		failWithCode(w, http.StatusBadRequest, ErrRegcodeInvalid, "注册码无效、已用完或已过期")
		return
	}
	if registerReg.Code != "" {
		if reached, current, limit := a.embyCapacityReachedExcluding(0, regCode, ""); reached {
			failWithCode(w, http.StatusConflict, ErrEmbyCapacityReached, fmt.Sprintf("Emby 用户数量已达上限 %d/%d", current, limit))
			return
		}
	}
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrPasswordHashFailed, "密码处理失败")
		return
	}
	// 管理员身份只能来自配置文件的 admin_uids / admin_usernames 列表（见下方
	// configuredAdminMatch）。已移除旧的"空数据库首注册者无条件成为 Admin"通道：
	// 它是一个抢注风险——生产部署后、运维注册前的窗口内，任何访问者抢先 POST
	// /register 即可拿到 Admin。现在首注册者只是普通用户，除非其 UID/用户名命中
	// 配置列表才会在创建后被提升。默认不配置列表 = 无人是管理员。
	role := store.RoleNormal
	newUser := store.User{Username: username, Email: email, PasswordHash: passwordHash, Role: role, TelegramID: telegramID, TelegramUsername: telegramUsername}
	now := time.Now().Unix()
	applyRegistrationGrant := func(user *store.User, consumed store.RegCode, _ store.BindCode) error {
		if consumed.Code != "" {
			days := normalizeRegCodeDays(consumed.Days)
			user.PendingEmby = true
			user.PendingEmbyDays = &days
			user.EmbyUsername = username
			markRegistrationGrant(user, registrationSourceRegCode, consumed.Code)
		}
		return nil
	}
	createUser := func(base store.User) (store.User, store.RegCode, error) {
		if registerReg.Code != "" {
			user, consumed, _, createErr := a.store().CreateUserForRegistration(base, registerReg.Code, "", now, applyRegistrationGrant)
			return user, consumed, createErr
		}
		user, createErr := a.store().CreateUser(base)
		return user, store.RegCode{}, createErr
	}
	var u store.User
	if telegramBindCode != "" {
		var consumed store.RegCode
		var consumedBind store.BindCode
		u, consumed, consumedBind, err = a.consumeConfirmedRegisterBindCode(telegramBindCode, now, func(bind store.BindCode) (store.User, store.RegCode, error) {
			base := newUser
			base.TelegramID = bind.TelegramID
			base.TelegramUsername = bind.TelegramUsername
			return createUser(base)
		})
		if err != nil {
			if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrExpired) {
				if consumedBind.Code != "" {
					failWithCode(w, http.StatusBadRequest, ErrRegcodeInvalid, "注册码无效、已用完或已过期")
					return
				}
				failWithCode(w, http.StatusBadRequest, ErrTGBindCodeExpired, "绑定码无效或已过期")
				return
			}
			if errors.Is(err, store.ErrConflict) {
				if _, exists := a.store().FindUserByUsername(username); exists {
					failWithCode(w, http.StatusConflict, ErrUsernameTaken, "用户名已被占用，请换一个用户名")
					return
				}
				if newUser.Email != "" && a.store().EmailAlreadyUsed(newUser.Email) {
					failWithCode(w, http.StatusConflict, ErrEmailConflict, "该邮箱已被其他账号使用")
					return
				}
				if consumedBind.TelegramID != 0 {
					if _, exists := a.store().FindUserByTelegramID(consumedBind.TelegramID); exists {
						failWithCode(w, http.StatusConflict, ErrTGBindTargetTaken, "该 Telegram 已绑定到其他账号或绑定码状态已变化")
						return
					}
				}
				if registerReg.Code != "" {
					failWithCode(w, http.StatusBadRequest, ErrRegcodeInvalid, "注册码无效、已用完或已过期")
					return
				}
				failWithCode(w, http.StatusConflict, ErrTGBindTargetTaken, "该 Telegram 已绑定到其他账号或绑定码状态已变化")
				return
			}
			if statusFromError(w, err) {
				return
			}
		}
		registerReg = consumed
	} else if registerReg.Code != "" {
		var consumed store.RegCode
		u, consumed, _, err = a.store().CreateUserForRegistration(newUser, registerReg.Code, "", now, func(user *store.User, consumed store.RegCode, _ store.BindCode) error {
			if consumed.Code != "" {
				days := normalizeRegCodeDays(consumed.Days)
				user.PendingEmby = true
				user.PendingEmbyDays = &days
				user.EmbyUsername = username
				markRegistrationGrant(user, registrationSourceRegCode, consumed.Code)
			}
			return nil
		})
		if err != nil {
			if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrExpired) {
				failWithCode(w, http.StatusBadRequest, ErrRegcodeInvalid, "注册码无效、已用完或已过期")
				return
			}
			if errors.Is(err, store.ErrConflict) {
				if _, exists := a.store().FindUserByUsername(username); exists {
					failWithCode(w, http.StatusConflict, ErrUsernameTaken, "用户名已被占用，请换一个用户名")
					return
				}
				if telegramBindCode != "" {
					failWithCode(w, http.StatusConflict, ErrTGBindTargetTaken, "该 Telegram 已绑定到其他账号或绑定码状态已变化")
					return
				}
				failWithCode(w, http.StatusBadRequest, ErrRegcodeInvalid, "注册码无效、已用完或已过期")
				return
			}
			if statusFromError(w, err) {
				return
			}
		}
		registerReg = consumed
	} else {
		u, _, err = createUser(newUser)
	}
	if errors.Is(err, store.ErrConflict) {
		if _, exists := a.store().FindUserByUsername(username); exists {
			failWithCode(w, http.StatusConflict, ErrUsernameTaken, "用户名已被占用，请换一个用户名")
			return
		}
		if telegramBindCode != "" {
			failWithCode(w, http.StatusConflict, ErrTGBindTargetTaken, "该 Telegram 已绑定到其他账号或绑定码状态已变化")
			return
		}
	}
	if statusFromError(w, err) {
		return
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
	created(w, "注册成功", map[string]any{"user": publicUser(u), "first_admin": role == store.RoleAdmin, "reg_code_used": registerReg.Code, "email_verification_sent": sendRegistrationEmailVerification(a, r, u, email)})
}

func sendRegistrationEmailVerification(a *App, r *http.Request, u store.User, email string) string {
	if email == "" || !emailConfigured(a.cfg()) {
		return ""
	}
	vid, s, _, msg := a.issueEmailCode(r.Context(), a.clientIP(r), "bind", email, u.UID)
	if s != http.StatusOK {
		zap.L().Warn("register email auto-send failed",
			zap.Int64("uid", u.UID),
			zap.String("message", msg),
		)
		return ""
	}
	return vid
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
			message = "用户名已被占用，请换一个用户名"
		}
	}
	currentUsers := a.store().UserCount()
	canRegister := a.cfg().RegisterEnabled || currentUsers == 0
	if a.cfg().UserLimit > 0 && currentUsers >= a.cfg().UserLimit {
		canRegister = false
		available = false
		message = fmt.Sprintf("系统用户数量已达上限 %d/%d", currentUsers, a.cfg().UserLimit)
	}
	embyBoundUsers := 0
	for _, u := range a.store().ListUsers() {
		if u.EmbyID != "" {
			embyBoundUsers++
		}
	}
	directDays := a.cfg().EmbyDirectRegisterDays
	if directDays == 0 {
		directDays = 30
	}
	ok(w, "OK", map[string]any{
		"enabled":                      a.cfg().RegisterEnabled,
		"register_mode":                a.cfg().RegisterEnabled,
		"can_register":                 canRegister,
		"requires_reg_code":            a.cfg().RegisterCodeLimit,
		"available":                    available,
		"message":                      message,
		"current_users":                currentUsers,
		"max_users":                    a.cfg().UserLimit,
		"allow_pending_register":       a.cfg().AllowPendingRegister,
		"emby_direct_register_enabled": a.cfg().EmbyDirectRegisterEnabled,
		"emby_direct_register_days":    directDays,
		"emby_user_limit":              a.cfg().EmbyUserLimit,
		"emby_bound_users":             embyBoundUsers,
	})
}

// replaceNotifPlaceholders 替换通知模板中的占位符。
func replaceNotifPlaceholders(tmpl string, values map[string]string) string {
	pairs := make([]string, 0, len(values)*2)
	for k, v := range values {
		pairs = append(pairs, k, v)
	}
	return strings.NewReplacer(pairs...).Replace(tmpl)
}
