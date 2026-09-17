package api

import "net/http"

// V2 violation resources keep the existing audited Store operations as the
// single implementation. The adapter only gives the WebUI a versioned
// namespace and an explicit private cache policy.
func (a *App) handleV2ListViolations(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleListViolations(w, r, p)
}

func (a *App) handleV2DeleteViolation(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleDeleteViolation(w, r, p)
}

func (a *App) handleV2ClearViolations(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleClearViolations(w, r, p)
}
