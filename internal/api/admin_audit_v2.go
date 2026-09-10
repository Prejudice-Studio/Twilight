package api

import "net/http"

// V2 audit resources keep the independent PostgreSQL audit table behind the
// SSR API boundary. Filtering, sanitization, confirmation phrases and SQL
// parameterization remain single-sourced in the existing handlers.
func (a *App) handleV2ListAuditLogs(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleListAuditLogs(w, r, p)
}

func (a *App) handleV2DeleteAuditLog(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleDeleteAuditLog(w, r, p)
}

func (a *App) handleV2ClearAuditLogs(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleClearAuditLogs(w, r, p)
}

func (a *App) handleV2PruneAuditLogs(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handlePruneAuditLogs(w, r, p)
}
