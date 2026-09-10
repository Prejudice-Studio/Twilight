package api

import "net/http"

// handleV2SigninSummary 聚合签到页首屏需要的摘要、公开规则和有限历史。
// 资格、余额及自动续期状态仍由 signin_handlers.go 与 Store 统一计算；V2
// 这里只是减少网络往返，不形成第二套积分业务实现。
func (a *App) handleV2SigninSummary(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	ok(w, "OK", a.signinPagePayload(current(r).User, 30))
}

func (a *App) handleV2Signin(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleSignin(w, r, p)
}

func (a *App) handleV2SigninRenew(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleSigninRenew(w, r, p)
}

// Preferences is intentionally narrower than the general user update
// resource. The shared handler still owns strict boolean parsing, feature
// gates, Emby eligibility, protected-account checks, persistence, and audit.
func (a *App) handleV2SigninPreferences(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleUpdateMe(w, r, p)
}
