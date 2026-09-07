package api

import "net/http"

// V2 user announcement resources provide one private SSR payload for visible
// announcements and force-read state. The shared handlers remain the single
// source for visibility, acknowledgement persistence, and response shape so
// the V1 compatibility endpoints cannot drift from the SSR path.
func (a *App) handleV2Announcements(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleAnnouncementsMe(w, r, p)
}

func (a *App) handleV2AckAnnouncements(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleAckAnnouncements(w, r, p)
}
