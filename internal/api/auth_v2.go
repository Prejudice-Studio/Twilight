package api

import "net/http"

// V2 authentication resources are transport adapters for the existing auth
// handlers. They keep session issuance, password verification, rate limits,
// bind-code consumption, persistence and audit behavior single-sourced.
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
	a.handleCurrentUser(w, r, p)
}

func (a *App) handleV2Logout(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleLogout(w, r, p)
}

func (a *App) handleV2LogoutAll(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleLogoutAll(w, r, p)
}

func (a *App) handleV2RefreshSession(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleRefresh(w, r, p)
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
	a.handleRegister(w, r, p)
}

func (a *App) handleV2RegistrationAvailability(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleRegisterAvailability(w, r, p)
}

func (a *App) handleV2CreateRegistrationBindCode(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleRegisterBindCode(w, r, p)
}
