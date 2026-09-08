package api

import (
	"net/http"
)

// V2 authentication resources keep the SSR transport contract separate from
// legacy HTTP handlers. Session reads and lifecycle operations are implemented
// here on top of shared application services and audited compatibility rules.
func (a *App) handleV2Login(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleLogin(w, r, p)
}

func (a *App) handleV2LoginByAPIKey(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleLoginByAPIKey(w, r, p)
}

func (a *App) handleV2TelegramLogin(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleDirectLoginUnavailable(w, r, p)
}

func (a *App) handleV2CurrentUser(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	ok(w, "OK", publicUser(current(r).User))
}

func (a *App) handleV2Logout(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.revokeSession(r.Context(), current(r).Token)
	a.clearSessionCookie(w)
	ok(w, "logged out", nil)
}

func (a *App) handleV2LogoutAll(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	principal := current(r)
	a.revokeAllSessions(r.Context(), principal.User.UID)
	a.clearSessionCookie(w)
	ok(w, "all sessions logged out", nil)
}

func (a *App) handleV2RefreshSession(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	principal := current(r)
	token, expires, err := a.refreshSession(r.Context(), principal.Token, principal.User.UID)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrAuthSessionRefreshFailed, "刷新会话失败")
		return
	}
	a.issueSessionCookies(w, token, expires)
	ok(w, "刷新成功", map[string]any{"token": token, "user": publicUser(principal.User)})
}

func (a *App) handleV2ForgotPasswordByEmby(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleForgotPassword(w, r, p)
}

func (a *App) handleV2EmailPasswordResetRequest(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleForgotPasswordEmailRequest(w, r, p)
}

func (a *App) handleV2EmailPasswordReset(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleForgotPasswordEmailReset(w, r, p)
}

func (a *App) handleV2Register(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleRegistration(w, r)
}

func (a *App) handleV2RegistrationAvailability(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleRegisterAvailability(w, r, p)
}

func (a *App) handleV2CreateRegistrationBindCode(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleRegisterBindCode(w, r, p)
}
