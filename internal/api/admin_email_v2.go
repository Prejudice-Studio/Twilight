package api

import "net/http"

// V2 email administration keeps the existing sanitized DTOs, confirmation
// phrases, SMTP handling and audit writes in one set of handlers.
func (a *App) handleV2AdminEmailVerifications(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleAdminEmailVerifications(w, r, p)
}

func (a *App) handleV2AdminEmailTest(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminEmailTest(w, r, p)
}

func (a *App) handleV2AdminDeleteEmailVerification(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminDeleteEmailVerification(w, r, p)
}

func (a *App) handleV2AdminCleanupEmailVerifications(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminCleanupEmailVerifications(w, r, p)
}

func (a *App) handleV2AdminClearUnverifiedEmails(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminClearUnverifiedEmails(w, r, p)
}
