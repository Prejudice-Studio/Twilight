package api

import "net/http"

// V2 user API Key resources keep the browser on the SSR/form-action boundary.
// Key ownership, masking, one-time plaintext creation, validation, persistence
// and audit remain in the existing API Key handlers.
func (a *App) handleV2ListAPIKeys(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleListAPIKeys(w, r, p)
}

func (a *App) handleV2CreateAPIKey(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleCreateAPIKey(w, r, p)
}

func (a *App) handleV2UpdateAPIKey(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleUpdateAPIKey(w, r, p)
}

func (a *App) handleV2DeleteAPIKey(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleDeleteAPIKey(w, r, p)
}
