package api

import "net/http"

// Setup is a public, one-time resource. The canonical handler remains
// responsible for availability, intent headers, rate limits, validation,
// rollback, session issuance and configuration persistence.
func (a *App) handleV2SetupStatus(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleSetupStatus(w, r, p)
}

func (a *App) handleV2SetupComplete(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleSetupComplete(w, r, p)
}
