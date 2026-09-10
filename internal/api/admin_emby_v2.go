package api

import "net/http"

// V2 Emby management routes keep the SSR resource namespace separate from the
// legacy compatibility inventory. The underlying handlers remain single
// sourced so permission checks, audit entries, manual-refresh behavior and
// upstream error sanitization cannot drift during the migration.
func (a *App) handleV2AdminEmbyUsers(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminEmbyUsersV2(w, r, p)
}

func (a *App) handleV2AdminEmbyDeviceAudit(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminEmbyDeviceAudit(w, r, p)
}

func (a *App) handleV2AdminEmbyActivityLogs(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleEmbyActivityLogs(w, r, p)
}

// Activity synchronization is a state-changing operation. Keep it on a POST
// resource so SSR form actions and intermediaries cannot accidentally replay a
// write through a cached or prefetched GET URL.
func (a *App) handleV2AdminEmbyActivityLogSync(w http.ResponseWriter, r *http.Request, p Params) {
	query := r.URL.Query()
	query.Set("refresh", "1")
	if query.Get("since_hours") == "" {
		query.Set("since_hours", "24")
	}
	r.URL.RawQuery = query.Encode()
	a.handleEmbyActivityLogs(w, r, p)
}

func (a *App) handleV2AdminEmbyConnectivityTest(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleEmbyConnectivityTest(w, r, p)
}

func (a *App) handleV2AdminEmbyBroadcast(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleEmbyBroadcast(w, r, p)
}

func (a *App) handleV2AdminEmbySync(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleEmbySyncV2(w, r, p)
}

func (a *App) handleV2AdminEmbyImportUsers(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleEmbyImportUsers(w, r, p)
}

func (a *App) handleV2AdminEmbyDeleteUnlinked(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleEmbyDeleteUnlinked(w, r, p)
}

func (a *App) handleV2AdminEmbyCleanupOrphans(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleEmbyCleanupOrphans(w, r, p)
}

func (a *App) handleV2AdminEmbyResetBindings(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleEmbyResetBindings(w, r, p)
}

func (a *App) handleV2AdminEmbyCreateStandalone(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleCreateStandaloneEmbyV2(w, r, p)
}

func (a *App) handleV2AdminEmbyForceSetPassword(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminEmbyForceSetPassword(w, r, p)
}

func (a *App) handleV2AdminEmbyUserToggle(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminEmbyUserToggle(w, r, p)
}

func (a *App) handleV2AdminEmbyUserKick(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminEmbyUserKick(w, r, p)
}
