package api

import "net/http"

func (a *App) handleInviteTree(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", a.inviteForest())
}
func (a *App) handleAdminInviteCodes(w http.ResponseWriter, r *http.Request, _ Params) {
	codes := a.store().ListAllInviteCodes()
	items := make([]map[string]any, 0, len(codes))
	for _, code := range codes {
		items = append(items, a.inviteCodeDTO(code))
	}
	ok(w, "OK", map[string]any{"codes": items, "total": len(items)})
}
