package api

import "net/http"

// registerV2Routes keeps the native SSR resource contract separate from the
// legacy compatibility inventory. V1 remains registered below for rollback
// and external clients; V2 pages must prefer this collection as it grows.
func (a *App) registerV2Routes() {
	a.add(http.MethodGet, "/api/v2/system/health", AuthPublic, a.handleV2Health)
	a.add(http.MethodGet, "/api/v2/system/capabilities", AuthPublic, a.handleV2Capabilities)
	a.add(http.MethodGet, "/api/v2/dashboard/summary", AuthUser, a.handleV2DashboardSummary)
	a.add(http.MethodGet, "/api/v2/signin/summary", AuthUser, a.handleV2SigninSummary)
	a.add(http.MethodGet, "/api/v2/invite/summary", AuthUser, a.handleV2InviteSummary)
	a.add(http.MethodGet, "/api/v2/bangumi/summary", AuthUser, a.handleV2BangumiSummary)
	a.add(http.MethodGet, "/api/v2/admin/invite/tree", AuthAdmin, a.handleV2AdminInviteTree)
	a.add(http.MethodGet, "/api/v2/admin/invite/codes", AuthAdmin, a.handleV2AdminInviteCodes)
	a.add(http.MethodPost, "/api/v2/admin/invite/users/:uid/detach", AuthAdmin, a.handleV2AdminInviteDetach)
	a.add(http.MethodPost, "/api/v2/admin/invite/users/:uid/detach-delete-emby", AuthAdmin, a.handleV2AdminInviteDetachDeleteEmby)
	a.add(http.MethodPost, "/api/v2/admin/invite/users/detach-batch", AuthAdmin, a.handleV2AdminInviteDetachBatch)
	a.add(http.MethodPost, "/api/v2/admin/invite/quick-maintenance", AuthAdmin, a.handleV2AdminInviteQuickMaintenance)
	a.add(http.MethodPost, "/api/v2/admin/invite/users/:uid/disable", AuthAdmin, a.handleV2AdminInviteToggleUser)
	a.add(http.MethodPost, "/api/v2/admin/invite/users/:uid/enable", AuthAdmin, a.handleV2AdminInviteToggleUser)
	a.add(http.MethodPost, "/api/v2/admin/invite/users/:uid/delete", AuthAdmin, a.handleV2AdminInviteDeleteUser)
	a.add(http.MethodGet, "/api/v2/admin/invite/config/schema", AuthAdmin, a.handleV2AdminInviteConfigSchema)
	a.add(http.MethodPut, "/api/v2/admin/invite/config/schema", AuthAdmin, a.handleV2AdminInviteConfigSchema)

	// Admin ticket resources back the SSR queue and conversation page. Keep
	// attachments inside the same protected resource family so rendered URLs
	// cannot accidentally fall back to a legacy browser-side request.
	a.add(http.MethodGet, "/api/v2/admin/tickets", AuthAdmin, a.handleV2AdminTickets)
	a.add(http.MethodGet, "/api/v2/admin/tickets/:ticket_id", AuthAdmin, a.handleV2AdminTicket)
	a.add(http.MethodPatch, "/api/v2/admin/tickets/:ticket_id", AuthAdmin, a.handleAdminUpdateTicket)
	a.add(http.MethodPost, "/api/v2/admin/tickets/:ticket_id/replies", AuthAdmin, a.handleAdminReplyTicket)
	a.add(http.MethodDelete, "/api/v2/admin/tickets/:ticket_id", AuthAdmin, a.handleAdminDeleteTicket)
	a.add(http.MethodPost, "/api/v2/admin/tickets/:ticket_id/attachments", AuthAdmin, a.handleUploadTicketImage)
	a.add(http.MethodGet, "/api/v2/admin/tickets/:ticket_id/attachments/:filename", AuthAdmin, a.handleGetTicketImage)
	a.add(http.MethodDelete, "/api/v2/admin/tickets/:ticket_id/attachments/:filename", AuthAdmin, a.handleDeleteTicketImage)
	a.add(http.MethodGet, "/api/v2/admin/ticket-types", AuthAdmin, a.handleV2AdminTicketTypes)
	a.add(http.MethodPost, "/api/v2/admin/ticket-types", AuthAdmin, a.handleAdminAddTicketType)
	a.add(http.MethodPatch, "/api/v2/admin/ticket-types/:ticket_type", AuthAdmin, a.handleV2AdminRenameTicketType)
	a.add(http.MethodDelete, "/api/v2/admin/ticket-types/:ticket_type", AuthAdmin, a.handleV2AdminDeleteTicketType)

	a.add(http.MethodGet, "/api/v2/admin/regcodes", AuthAdmin, a.handleV2AdminRegcodes)
	a.add(http.MethodPost, "/api/v2/admin/regcodes", AuthAdmin, a.handleV2CreateRegcodes)
	a.add(http.MethodPost, "/api/v2/admin/regcodes/batch-delete", AuthAdmin, a.handleV2BatchDeleteRegcodes)
	a.add(http.MethodGet, "/api/v2/admin/regcodes/:code", AuthAdmin, a.handleV2AdminRegcode)
	a.add(http.MethodPatch, "/api/v2/admin/regcodes/:code", AuthAdmin, a.handleV2UpdateRegcode)
	a.add(http.MethodDelete, "/api/v2/admin/regcodes/:code", AuthAdmin, a.handleV2DeleteRegcode)
	a.add(http.MethodGet, "/api/v2/admin/regcodes/:code/usage", AuthAdmin, a.handleV2AdminRegcodeUsage)
	a.add(http.MethodPost, "/api/v2/admin/regcodes/:code/usage/clear", AuthAdmin, a.handleV2ClearRegcodeUsage)
}
