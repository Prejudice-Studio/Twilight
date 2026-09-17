package api

import "net/http"

// V2 announcement resources are the WebUI-facing namespace. Validation,
// rendering-mode normalization, Store writes and audit entries remain in the
// shared handlers so V1 compatibility cannot drift from the new frontend.
func (a *App) handleV2AdminAnnouncements(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleAdminAnnouncements(w, r, p)
}

func (a *App) handleV2CreateAnnouncement(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleCreateAnnouncement(w, r, p)
}

func (a *App) handleV2UpdateAnnouncement(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleUpdateAnnouncement(w, r, p)
}

func (a *App) handleV2DeleteAnnouncement(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleDeleteAnnouncement(w, r, p)
}
