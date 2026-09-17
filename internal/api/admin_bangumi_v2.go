package api

import "net/http"

// V2 Bangumi admin resources keep the V2 namespace explicit while reusing
// the existing Store, feature-gate, sync timeout and audit rules.
func (a *App) handleV2AdminBangumiUsers(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleAdminBangumiUsers(w, r, p)
}

func (a *App) handleV2AdminBangumiRecords(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleAdminBangumiRecords(w, r, p)
}

func (a *App) handleV2AdminBangumiSync(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleAdminBangumiSyncUser(w, r, p)
}

func (a *App) handleV2AdminBangumiLogs(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleAdminBangumiSyncLogs(w, r, p)
}

func (a *App) handleV2AdminBangumiClearLogs(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleAdminBangumiClearLogs(w, r, p)
}
