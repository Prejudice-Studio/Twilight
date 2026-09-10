package api

import "net/http"

// V2 rebind resources deliberately share the legacy review implementation so
// request-state transitions and audit records cannot diverge by UI version.
func (a *App) handleV2ListRebindRequests(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleListRebindRequests(w, r, p)
}

func (a *App) handleV2ReviewRebindRequest(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleReviewRebindRequest(w, r, p)
}

func (a *App) handleV2BatchReviewRebindRequests(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleBatchReviewRebindRequests(w, r, p)
}

func (a *App) handleV2RevokeAllRebindApprovals(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleRevokeAllRebindApprovals(w, r, p)
}
