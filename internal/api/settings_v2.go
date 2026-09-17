package api

import "net/http"

// V2 settings resources are the WebUI-facing account-management boundary. The
// underlying handlers remain the single source of truth for validation,
// feature gates, session rotation, Emby side effects, persistence and audit.
// These adapters only give the new frontend explicit resource names and a
// consistent private cache policy.
func (a *App) handleV2UserSettings(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleUserSettings(w, r, p)
}

func (a *App) handleV2UpdateUserSettings(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleUpdateMe(w, r, p)
}

func (a *App) handleV2SendEmailCode(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleSendEmailCode(w, r, p)
}

func (a *App) handleV2VerifyEmailCode(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleVerifyEmailCode(w, r, p)
}

func (a *App) handleV2ChangePassword(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleChangePassword(w, r, p)
}

func (a *App) handleV2ChangeEmbyPassword(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleChangeEmbyPassword(w, r, p)
}

func (a *App) handleV2BindEmby(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleBindEmby(w, r, p)
}

func (a *App) handleV2RegisterEmby(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleRegisterEmby(w, r, p)
}

func (a *App) handleV2UnbindEmby(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleUnbindEmby(w, r, p)
}
