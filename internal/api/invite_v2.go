package api

import "net/http"

type v2InviteSummary struct {
	Config map[string]any `json:"config"`
	Invite map[string]any `json:"invite"`
}

// handleV2InviteSummary aggregates the authenticated invite page's first-view
// data into one request. The payload contains only the current user's
// relationship projection and non-secret invite configuration; all eligibility
// and mutation decisions remain in the Go invite handlers/store.
func (a *App) handleV2InviteSummary(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", v2InviteSummary{
		Config: a.inviteConfigPayload(),
		Invite: a.inviteMePayload(current(r).User),
	})
}

func (a *App) inviteConfigPayload() map[string]any {
	cfg := a.cfg()
	return map[string]any{
		"enabled":                   cfg.InviteEnabled,
		"max_depth":                 cfg.InviteMaxDepth,
		"invite_limit":              cfg.InviteLimit,
		"invite_root_user_limit":    cfg.InviteRootUserLimit,
		"require_emby":              cfg.InviteRequireEmby,
		"default_days":              cfg.InviteDefaultDays,
		"code_format":               a.inviteCodeFormat(""),
		"permanent_invite_max_days": cfg.PermanentInviteMaxDays,
	}
}
