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
	"net/http"
	"strings"
	"time"
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
	a.handleLoginResource(w, r)
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
	a.handleForgotPasswordResource(w, r)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	a.revokeSession(r.Context(), p.Token)
	a.clearSessionCookie(w)
	ok(w, "logged out", nil)
}

func (a *App) handleLogoutAll(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	a.revokeAllSessions(r.Context(), p.User.UID)
	a.clearSessionCookie(w)
	ok(w, "all sessions logged out", nil)
}

func (a *App) handleRefresh(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	token, expires, err := a.refreshSession(r.Context(), p.Token, p.User.UID)
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
	a.handleRegistration(w, r)
}

func (a *App) handleRegisterAvailability(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleRegistrationAvailability(w, r)
}

// replaceNotifPlaceholders 替换通知模板中的占位符。
func replaceNotifPlaceholders(tmpl string, values map[string]string) string {
	pairs := make([]string, 0, len(values)*2)
	for k, v := range values {
		pairs = append(pairs, k, v)
	}
	return strings.NewReplacer(pairs...).Replace(tmpl)
}
