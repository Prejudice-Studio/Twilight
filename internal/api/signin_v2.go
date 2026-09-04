package api

import "net/http"

// handleV2SigninSummary 聚合签到页首屏需要的摘要、公开规则和有限历史。
// 资格、余额及自动续期状态仍由 signin_handlers.go 与 Store 统一计算；V2
// 这里只是减少网络往返，不形成第二套积分业务实现。
func (a *App) handleV2SigninSummary(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", a.signinPagePayload(current(r).User, 30))
}
