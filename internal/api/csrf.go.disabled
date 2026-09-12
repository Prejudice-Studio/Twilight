package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"time"
)

// generateCSRFToken 生成一个加密安全的随机 CSRF token。
// 使用 32 字节随机数据，base64 URL 编码后约 43 字符。
func generateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// issueCSRFCookie 在用户登录或会话刷新时颁发 CSRF token cookie。
// 该 cookie 可被前端 JavaScript 读取，用于在请求头中发送。
func (a *App) issueCSRFCookie(w http.ResponseWriter, token string, expires time.Time) {
	cfg := a.cfg()
	http.SetCookie(w, &http.Cookie{
		Name:     "twilight_csrf",
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: false, // 前端需要读取此 cookie
		Secure:   cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// verifyCSRF 验证 CSRF token。采用 Double Submit Cookie 方案：
// 1. 读取 Cookie 中的 token
// 2. 读取 Header (X-CSRF-Token) 或 Form 字段 (csrf_token) 中的 token
// 3. 常量时间比较两者是否相同
//
// 返回 true 表示验证通过，false 表示验证失败。
func (a *App) verifyCSRF(r *http.Request) bool {
	// 1. 读取 Cookie 中的 token
	cookie, err := r.Cookie("twilight_csrf")
	if err != nil || cookie.Value == "" {
		return false
	}
	cookieToken := cookie.Value

	// 2. 读取请求中的 token（优先 Header，其次 Form）
	requestToken := r.Header.Get("X-CSRF-Token")
	if requestToken == "" {
		// SSR form action 可能通过 hidden input 提交
		if err := r.ParseForm(); err == nil {
			requestToken = r.FormValue("csrf_token")
		}
	}
	if requestToken == "" {
		return false
	}

	// 3. 常量时间比较（防止时序攻击）
	return subtle.ConstantTimeCompare([]byte(cookieToken), []byte(requestToken)) == 1
}

// requireCSRF 是一个中间件，要求所有状态变更方法（POST/PUT/DELETE/PATCH）
// 必须通过 CSRF 验证。只有安全方法（GET/HEAD/OPTIONS）可以跳过验证。
//
// 调用方式：在 ServeHTTP 中，路由匹配前调用。
func (a *App) requireCSRF(w http.ResponseWriter, r *http.Request) bool {
	// 安全方法不需要 CSRF 保护
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}

	// 验证 CSRF token
	if !a.verifyCSRF(r) {
		failWithCode(w, http.StatusForbidden, ErrCSRFTokenInvalid, "CSRF 验证失败，请刷新页面后重试")
		return false
	}

	return true
}
