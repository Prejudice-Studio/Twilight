package api

import "net/http"

// registerV2Routes keeps the native V2 resource contract separate from the
// legacy compatibility inventory. V1 remains registered below for external
// clients and the NEXT_PUBLIC_USE_V1_COMPAT rollback; the WebUI must prefer
// this collection as it grows.
func (a *App) registerV2Routes() {
	a.add(http.MethodGet, "/api/v2/system/health", AuthPublic, a.handleV2Health)
	a.add(http.MethodGet, "/api/v2/system/capabilities", AuthPublic, a.handleV2Capabilities)
	a.add(http.MethodGet, "/api/v2/openapi.json", AuthPublic, a.handleV2OpenAPI)
	a.add(http.MethodGet, "/api/v2/admin/docs/routes", AuthAdmin, a.handleV2AdminAPIRoutes)
	a.add(http.MethodGet, "/api/v2/system/info", AuthPublic, a.handleV2SystemInfo)
	a.add(http.MethodGet, "/api/v2/setup/status", AuthPublic, a.handleV2SetupStatus)
	a.add(http.MethodPost, "/api/v2/setup/complete", AuthPublic, a.handleV2SetupComplete)
	a.add(http.MethodGet, "/api/v2/admin/health/api", AuthAdmin, a.handleV2AdminHealthAPI)
	a.add(http.MethodGet, "/api/v2/admin/health/database", AuthAdmin, a.handleV2AdminHealthDatabase)
	a.add(http.MethodGet, "/api/v2/admin/health/emby", AuthAdmin, a.handleV2AdminHealthEmby)
	a.add(http.MethodGet, "/api/v2/admin/stats", AuthAdmin, a.handleV2AdminStats)
	a.add(http.MethodGet, "/api/v2/admin/playback/summary", AuthAdmin, a.handleGetPlaybackSummary)
	a.add(http.MethodGet, "/api/v2/admin/config/toml", AuthAdmin, a.handleV2ConfigTOMLGet)
	a.add(http.MethodPut, "/api/v2/admin/config/toml", AuthAdmin, a.handleV2ConfigTOMLUpdate)
	a.add(http.MethodGet, "/api/v2/admin/config/schema", AuthAdmin, a.handleV2ConfigSchema)
	a.add(http.MethodPut, "/api/v2/admin/config/schema", AuthAdmin, a.handleV2ConfigSchemaUpdate)
	a.add(http.MethodGet, "/api/v2/admin/config/backups", AuthAdmin, a.handleV2ConfigBackups)
	a.add(http.MethodPost, "/api/v2/admin/config/backup", AuthAdmin, a.handleV2ConfigBackup)
	a.add(http.MethodGet, "/api/v2/admin/config/backups/:name", AuthAdmin, a.handleV2ConfigBackupInspect)
	a.add(http.MethodDelete, "/api/v2/admin/config/backups/:name", AuthAdmin, a.handleV2ConfigBackupDelete)
	a.add(http.MethodPost, "/api/v2/admin/config/restore", AuthAdmin, a.handleV2ConfigRestore)
	a.add(http.MethodPost, "/api/v2/admin/config/sweep", AuthAdmin, a.handleV2ConfigSweep)
	a.add(http.MethodPost, "/api/v2/admin/config/upload-auth-background", AuthAdmin, a.handleV2ConfigBackgroundUpload)
	a.add(http.MethodGet, "/api/v2/admin/database/status", AuthAdmin, a.handleV2DatabaseStatus)
	a.add(http.MethodGet, "/api/v2/admin/database/backups", AuthAdmin, a.handleV2DatabaseBackups)
	a.add(http.MethodGet, "/api/v2/admin/database/backups/:name", AuthAdmin, a.handleV2DatabaseBackupInspect)
	a.add(http.MethodGet, "/api/v2/admin/migration/status", AuthAdmin, a.handleV2MigrationStatus)
	a.add(http.MethodPost, "/api/v2/admin/migration/export", AuthAdmin, a.handleV2MigrationExport)
	a.add(http.MethodPost, "/api/v2/admin/migration/import", AuthAdmin, a.handleV2MigrationImport)
	a.add(http.MethodDelete, "/api/v2/admin/database/backups/:name", AuthAdmin, a.handleV2DatabaseBackupDelete)
	a.add(http.MethodPost, "/api/v2/admin/database/backup", AuthAdmin, a.handleV2DatabaseBackup)
	a.add(http.MethodPost, "/api/v2/admin/database/restore", AuthAdmin, a.handleV2DatabaseRestore)
	a.add(http.MethodPost, "/api/v2/admin/database/migrate", AuthAdmin, a.handleV2DatabaseMigrate)
	a.add(http.MethodGet, "/api/v2/admin/runtime/status", AuthAdmin, a.handleV2RuntimeStatus)
	a.add(http.MethodGet, "/api/v2/admin/runtime/logs", AuthAdmin, a.handleV2RuntimeLogs)
	a.add(http.MethodGet, "/api/v2/admin/scheduler/jobs", AuthAdmin, a.handleV2SchedulerJobs)
	a.add(http.MethodPost, "/api/v2/admin/scheduler/jobs/:job_id/run", AuthAdmin, a.handleV2SchedulerRun)
	a.add(http.MethodPost, "/api/v2/admin/scheduler/jobs/:job_id/terminate", AuthAdmin, a.handleV2SchedulerTerminate)
	a.add(http.MethodGet, "/api/v2/admin/scheduler/jobs/:job_id/last-run", AuthAdmin, a.handleV2SchedulerLastRun)
	a.add(http.MethodGet, "/api/v2/admin/scheduler/jobs/:job_id/history", AuthAdmin, a.handleV2SchedulerHistory)
	a.add(http.MethodPut, "/api/v2/admin/scheduler/jobs/:job_id/schedule", AuthAdmin, a.handleV2SchedulerSchedule)
	a.add(http.MethodDelete, "/api/v2/admin/scheduler/jobs/:job_id/schedule", AuthAdmin, a.handleV2SchedulerSchedule)
	a.add(http.MethodGet, "/api/v2/tickets", AuthUser, a.handleV2UserTickets)
	a.add(http.MethodPost, "/api/v2/tickets", AuthUser, a.handleV2CreateTicket)
	a.add(http.MethodGet, "/api/v2/tickets/:ticket_id", AuthUser, a.handleV2UserTicket)
	a.add(http.MethodPost, "/api/v2/tickets/:ticket_id/close", AuthUser, a.handleV2CloseUserTicket)
	a.add(http.MethodPost, "/api/v2/tickets/:ticket_id/reopen", AuthUser, a.handleV2ReopenUserTicket)
	a.add(http.MethodPut, "/api/v2/tickets/:ticket_id/notify-telegram", AuthUser, a.handleV2ToggleUserTicketNotify)
	a.add(http.MethodPost, "/api/v2/tickets/:ticket_id/replies", AuthUser, a.handleV2UserTicketReply)
	a.add(http.MethodPost, "/api/v2/tickets/:ticket_id/attachments", AuthUser, a.handleV2UserTicketAttachmentUpload)
	a.add(http.MethodGet, "/api/v2/tickets/:ticket_id/attachments/:filename", AuthUser, a.handleV2UserTicketAttachment)
	a.add(http.MethodDelete, "/api/v2/tickets/:ticket_id/attachments/:filename", AuthUser, a.handleV2UserTicketAttachmentDelete)
	a.add(http.MethodPost, "/api/v2/auth/login", AuthPublic, a.handleV2Login)
	a.add(http.MethodPost, "/api/v2/auth/login/apikey", AuthPublic, a.handleV2LoginByAPIKey)
	a.add(http.MethodPost, "/api/v2/auth/login/telegram", AuthPublic, a.handleV2TelegramLogin)
	a.add(http.MethodGet, "/api/v2/auth/me", AuthUser, a.handleV2CurrentUser)
	a.add(http.MethodPost, "/api/v2/auth/logout", AuthUser, a.handleV2Logout)
	a.add(http.MethodPost, "/api/v2/auth/logout/all", AuthUser, a.handleV2LogoutAll)
	a.add(http.MethodPost, "/api/v2/auth/refresh", AuthUser, a.handleV2RefreshSession)
	a.add(http.MethodPost, "/api/v2/auth/password/emby", AuthPublic, a.handleV2ForgotPasswordByEmby)
	a.add(http.MethodPost, "/api/v2/auth/password/email/request", AuthPublic, a.handleV2EmailPasswordResetRequest)
	a.add(http.MethodPost, "/api/v2/auth/password/email/reset", AuthPublic, a.handleV2EmailPasswordReset)
	a.add(http.MethodPost, "/api/v2/registration", AuthPublic, a.handleV2Register)
	a.add(http.MethodGet, "/api/v2/registration/availability", AuthPublic, a.handleV2RegistrationAvailability)
	// The product client posts the registration code instead of using a query
	// string, so both verbs must resolve to the same bounded check.
	a.add(http.MethodPost, "/api/v2/registration/availability", AuthPublic, a.handleV2RegistrationAvailability)
	a.add(http.MethodPost, "/api/v2/registration/telegram/bind-code", AuthPublic, a.handleV2CreateRegistrationBindCode)
	a.add(http.MethodGet, "/api/v2/dashboard/summary", AuthUser, a.handleV2DashboardSummary)
	a.add(http.MethodGet, "/api/v2/settings", AuthUser, a.handleV2UserSettings)
	a.add(http.MethodPut, "/api/v2/settings/preferences", AuthUser, a.handleV2UpdateUserSettings)
	a.add(http.MethodGet, "/api/v2/settings/appearance", AuthUser, a.handleV2UserAppearance)
	a.add(http.MethodPut, "/api/v2/settings/appearance/background", AuthUser, a.handleV2UpdateUserBackground)
	a.add(http.MethodDelete, "/api/v2/settings/appearance/background", AuthUser, a.handleV2DeleteUserBackground)
	a.add(http.MethodPost, "/api/v2/settings/appearance/background/upload", AuthUser, a.handleV2UploadUserBackground)
	a.add(http.MethodPost, "/api/v2/settings/appearance/avatar/upload", AuthUser, a.handleV2UploadUserAvatar)
	a.add(http.MethodDelete, "/api/v2/settings/appearance/avatar", AuthUser, a.handleV2DeleteUserAvatar)
	a.add(http.MethodPost, "/api/v2/settings/email/send-code", AuthUser, a.handleV2SendEmailCode)
	a.add(http.MethodPost, "/api/v2/settings/email/verify", AuthUser, a.handleV2VerifyEmailCode)
	a.add(http.MethodPost, "/api/v2/settings/password/system", AuthUser, a.handleV2ChangePassword)
	a.add(http.MethodPost, "/api/v2/settings/password/emby", AuthUser, a.handleV2ChangeEmbyPassword)
	a.add(http.MethodPost, "/api/v2/settings/emby/bind", AuthUser, a.handleV2BindEmby)
	a.add(http.MethodPost, "/api/v2/settings/emby/register", AuthUser, a.handleV2RegisterEmby)
	a.add(http.MethodPost, "/api/v2/settings/emby/unbind", AuthUser, a.handleV2UnbindEmby)
	a.add(http.MethodGet, "/api/v2/settings/apikeys", AuthUser, a.handleV2ListAPIKeys)
	a.add(http.MethodPost, "/api/v2/settings/apikeys", AuthUser, a.handleV2CreateAPIKey)
	a.add(http.MethodPut, "/api/v2/settings/apikeys/:key_id", AuthUser, a.handleV2UpdateAPIKey)
	a.add(http.MethodDelete, "/api/v2/settings/apikeys/:key_id", AuthUser, a.handleV2DeleteAPIKey)
	a.add(http.MethodGet, "/api/v2/users/:uid/playback/history", AuthUser, a.handleGetUserPlaybackHistory)
	a.add(http.MethodGet, "/api/v2/users/:uid/playback/sessions", AuthUser, a.handleGetUserPlaybackSessions)
	a.add(http.MethodGet, "/api/v2/announcements", AuthPublic, a.handleV2Announcements)
	a.add(http.MethodPost, "/api/v2/announcements/ack", AuthUser, a.handleV2AckAnnouncements)
	a.add(http.MethodGet, "/api/v2/signin/summary", AuthUser, a.handleV2SigninSummary)
	a.add(http.MethodPost, "/api/v2/signin", AuthUser, a.handleV2Signin)
	a.add(http.MethodPost, "/api/v2/signin/renew", AuthUser, a.handleV2SigninRenew)
	a.add(http.MethodPut, "/api/v2/signin/preferences", AuthUser, a.handleV2SigninPreferences)
	a.add(http.MethodGet, "/api/v2/invite/summary", AuthUser, a.handleV2InviteSummary)
	a.add(http.MethodPost, "/api/v2/invite/codes", AuthUser, a.handleV2CreateInviteCode)
	a.add(http.MethodPost, "/api/v2/invite/renew-codes", AuthUser, a.handleV2CreateInviteCode)
	a.add(http.MethodDelete, "/api/v2/invite/codes/:code", AuthUser, a.handleV2DeleteInviteCode)
	a.add(http.MethodPost, "/api/v2/invite/children/:uid/detach-expired", AuthUser, a.handleV2DetachExpiredInviteChild)
	a.add(http.MethodPost, "/api/v2/invite/me/detach-expired", AuthUser, a.handleV2DetachMyExpiredInvite)
	a.add(http.MethodGet, "/api/v2/bangumi/summary", AuthUser, a.handleV2BangumiSummary)
	a.add(http.MethodPost, "/api/v2/bangumi/sync", AuthUser, a.handleV2BangumiSync)
	a.add(http.MethodDelete, "/api/v2/bangumi/sync/history", AuthUser, a.handleV2BangumiClearHistory)
	a.add(http.MethodPut, "/api/v2/bangumi/preferences", AuthUser, a.handleV2BangumiPreferences)
	a.add(http.MethodGet, "/api/v2/bangumi/collections", AuthUser, a.handleV2BangumiCollections)
	a.add(http.MethodPatch, "/api/v2/bangumi/collections/:subject_id", AuthUser, a.handleV2UpdateBangumiCollection)
	a.add(http.MethodGet, "/api/v2/bangumi/covers/:subject_id", AuthPublic, a.handleV2BangumiCover)
	a.add(http.MethodGet, "/api/v2/media/search", AuthUser, a.handleV2MediaSearch)
	a.add(http.MethodGet, "/api/v2/media/search/:source", AuthUser, a.handleV2MediaSearch)
	a.add(http.MethodGet, "/api/v2/media/detail", AuthUser, a.handleV2MediaDetail)
	a.add(http.MethodPost, "/api/v2/media/inventory/check", AuthUser, a.handleV2MediaInventoryCheck)
	a.add(http.MethodGet, "/api/v2/media/requests", AuthUser, a.handleV2MediaRequests)
	a.add(http.MethodPost, "/api/v2/media/requests", AuthUser, a.handleV2CreateMediaRequest)
	a.add(http.MethodGet, "/api/v2/media/requests/by-key/:require_key", AuthUser, a.handleV2MediaRequestByKey)
	a.add(http.MethodDelete, "/api/v2/media/requests/by-key/:require_key", AuthUser, a.handleV2DeleteMediaRequestByKey)
	a.add(http.MethodGet, "/api/v2/admin/bangumi/users", AuthAdmin, a.handleV2AdminBangumiUsers)
	a.add(http.MethodGet, "/api/v2/admin/bangumi/users/:uid/records", AuthAdmin, a.handleV2AdminBangumiRecords)
	a.add(http.MethodPost, "/api/v2/admin/bangumi/users/:uid/sync", AuthAdmin, a.handleV2AdminBangumiSync)
	a.add(http.MethodGet, "/api/v2/admin/bangumi/users/:uid/logs", AuthAdmin, a.handleV2AdminBangumiLogs)
	a.add(http.MethodDelete, "/api/v2/admin/bangumi/users/:uid/logs", AuthAdmin, a.handleV2AdminBangumiClearLogs)
	a.add(http.MethodGet, "/api/v2/admin/announcements", AuthAdmin, a.handleV2AdminAnnouncements)
	a.add(http.MethodPost, "/api/v2/admin/announcements", AuthAdmin, a.handleV2CreateAnnouncement)
	a.add(http.MethodPut, "/api/v2/admin/announcements/:announcement_id", AuthAdmin, a.handleV2UpdateAnnouncement)
	a.add(http.MethodDelete, "/api/v2/admin/announcements/:announcement_id", AuthAdmin, a.handleV2DeleteAnnouncement)
	a.add(http.MethodGet, "/api/v2/admin/audit-logs", AuthAdmin, a.handleV2ListAuditLogs)
	a.add(http.MethodDelete, "/api/v2/admin/audit-logs/:log_id", AuthAdmin, a.handleV2DeleteAuditLog)
	a.add(http.MethodPost, "/api/v2/admin/audit-logs/clear", AuthAdmin, a.handleV2ClearAuditLogs)
	a.add(http.MethodPost, "/api/v2/admin/audit-logs/prune", AuthAdmin, a.handleV2PruneAuditLogs)
	a.add(http.MethodGet, "/api/v2/admin/violations", AuthAdmin, a.handleV2ListViolations)
	a.add(http.MethodDelete, "/api/v2/admin/violations/:violation_id", AuthAdmin, a.handleV2DeleteViolation)
	a.add(http.MethodPost, "/api/v2/admin/violations/clear", AuthAdmin, a.handleV2ClearViolations)
	a.add(http.MethodPost, "/api/v2/admin/email/test", AuthAdmin, a.handleV2AdminEmailTest)
	a.add(http.MethodGet, "/api/v2/admin/email/verifications", AuthAdmin, a.handleV2AdminEmailVerifications)
	a.add(http.MethodPost, "/api/v2/admin/email/verifications/cleanup", AuthAdmin, a.handleV2AdminCleanupEmailVerifications)
	a.add(http.MethodPost, "/api/v2/admin/email/verifications/clear-unverified", AuthAdmin, a.handleV2AdminClearUnverifiedEmails)
	a.add(http.MethodDelete, "/api/v2/admin/email/verifications/:id", AuthAdmin, a.handleV2AdminDeleteEmailVerification)
	a.add(http.MethodGet, "/api/v2/admin/telegram/commands/catalog", AuthAdmin, a.handleV2AdminTelegramCommandCatalog)
	a.add(http.MethodGet, "/api/v2/admin/telegram/roster/stats", AuthAdmin, a.handleV2AdminTelegramRosterStats)
	a.add(http.MethodPost, "/api/v2/admin/telegram/test", AuthAdmin, a.handleV2AdminTelegramBotTest)
	a.add(http.MethodPost, "/api/v2/admin/developer/js-sandbox", AuthAdmin, a.handleV2DeveloperJSSandbox)
	a.add(http.MethodGet, "/api/v2/admin/developer/js-docs", AuthAdmin, a.handleV2DeveloperJSDocs)
	a.add(http.MethodGet, "/api/v2/admin/developer/js-presets", AuthAdmin, a.handleV2DeveloperJSPresets)
	a.add(http.MethodPost, "/api/v2/admin/developer/js-presets", AuthAdmin, a.handleV2CreateDeveloperJSPreset)
	a.add(http.MethodPut, "/api/v2/admin/developer/js-presets/:preset_id", AuthAdmin, a.handleV2UpdateDeveloperJSPreset)
	a.add(http.MethodDelete, "/api/v2/admin/developer/js-presets/:preset_id", AuthAdmin, a.handleV2DeleteDeveloperJSPreset)
	a.add(http.MethodGet, "/api/v2/admin/telegram/rebind-requests", AuthAdmin, a.handleV2ListRebindRequests)
	a.add(http.MethodPost, "/api/v2/admin/telegram/rebind-requests/:request_id/approve", AuthAdmin, a.handleV2ReviewRebindRequest)
	a.add(http.MethodPost, "/api/v2/admin/telegram/rebind-requests/:request_id/reject", AuthAdmin, a.handleV2ReviewRebindRequest)
	a.add(http.MethodPost, "/api/v2/admin/telegram/rebind-requests/batch", AuthAdmin, a.handleV2BatchReviewRebindRequests)
	a.add(http.MethodPost, "/api/v2/admin/telegram/rebind-requests/revoke-approved", AuthAdmin, a.handleV2RevokeAllRebindApprovals)
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
	a.add(http.MethodGet, "/api/v2/admin/media-requests", AuthAdmin, a.handleV2AdminMediaRequests)
	a.add(http.MethodPut, "/api/v2/admin/media-requests/:request_id", AuthAdmin, a.handleV2UpdateMediaRequestStatus)
	a.add(http.MethodDelete, "/api/v2/admin/media-requests/:request_id", AuthAdmin, a.handleV2DeleteMediaRequest)
	a.add(http.MethodPut, "/api/v2/admin/media-requests/by-key/:require_key", AuthAdmin, a.handleV2UpdateMediaRequestByKey)
	a.add(http.MethodPut, "/api/v2/admin/media-requests/batch", AuthAdmin, a.handleV2UpdateMediaRequestsByKey)
	a.add(http.MethodPut, "/api/v2/admin/media-requests/batch/by-key", AuthAdmin, a.handleV2UpdateMediaRequestsByKey)
	a.add(http.MethodDelete, "/api/v2/admin/media-requests/by-key/:require_key", AuthAdmin, a.handleV2DeleteMediaRequestByKey)

	// Admin ticket resources back the WebUI queue and conversation page. Keep
	// attachments inside the same protected resource family so rendered URLs
	// cannot accidentally fall back to a legacy browser-side request.
	a.add(http.MethodGet, "/api/v2/admin/tickets", AuthAdmin, a.handleV2AdminTickets)
	a.add(http.MethodGet, "/api/v2/admin/tickets/:ticket_id", AuthAdmin, a.handleV2AdminTicket)
	a.add(http.MethodPatch, "/api/v2/admin/tickets/:ticket_id", AuthAdmin, a.handleAdminUpdateTicket)
	a.add(http.MethodPost, "/api/v2/admin/tickets/:ticket_id/replies", AuthAdmin, a.handleV2AdminReplyTicket)
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

	// Administrator user resources. V1 remains available for rollback and
	// integrations; V2 uses an explicit items/pagination collection and keeps
	// all mutations behind the same audited handlers.
	a.add(http.MethodGet, "/api/v2/admin/users", AuthAdmin, a.handleV2AdminUsers)
	a.add(http.MethodPost, "/api/v2/admin/users", AuthAdmin, a.handleV2AdminCreateUser)
	a.add(http.MethodGet, "/api/v2/admin/users/:uid", AuthAdmin, a.handleV2AdminUser)
	a.add(http.MethodPut, "/api/v2/admin/users/:uid", AuthAdmin, a.handleV2AdminUpdateUser)
	a.add(http.MethodDelete, "/api/v2/admin/users/:uid", AuthAdmin, a.handleV2AdminDeleteUser)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/delete", AuthAdmin, a.handleV2AdminDeleteUser)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/disable", AuthAdmin, a.handleV2AdminToggleUser)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/enable", AuthAdmin, a.handleV2AdminToggleUser)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/emby/enable", AuthAdmin, a.handleV2AdminToggleEmby)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/emby/disable", AuthAdmin, a.handleV2AdminToggleEmby)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/force-unbind", AuthAdmin, a.handleV2AdminForceUnbind)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/registration-queue/clear", AuthAdmin, a.handleV2AdminRegistrationQueueClear)
	a.add(http.MethodPost, "/api/v2/admin/users/registration-queue/clear", AuthAdmin, a.handleV2AdminRegistrationQueueClear)
	a.add(http.MethodPost, "/api/v2/admin/users/registration-queue/grant-entitlement-and-clear", AuthAdmin, a.handleRegistrationEntitlementBulk)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/registration-entitlement", AuthAdmin, a.handleV2AdminRegistrationEntitlement)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/registration-entitlement/dequeue", AuthAdmin, a.handleV2AdminRegistrationEntitlement)
	a.add(http.MethodPost, "/api/v2/admin/users/sync-bindings", AuthAdmin, a.handleSyncBindings)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/refresh-status", AuthAdmin, a.handleV2AdminRefreshUserStatus)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/renew", AuthAdmin, a.handleV2AdminRenewUser)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/cancel-permanent", AuthAdmin, a.handleV2AdminSetUserExpiry)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/set-expiry", AuthAdmin, a.handleV2AdminSetUserExpiry)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/reset-password", AuthAdmin, a.handleV2AdminResetPassword)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/kick", AuthAdmin, a.handleV2AdminKickUser)
	a.add(http.MethodPut, "/api/v2/admin/users/:uid/admin", AuthAdmin, a.handleV2AdminSetRole)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/unbind-telegram", AuthAdmin, a.handleV2AdminUnbindTelegram)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/bind-telegram", AuthAdmin, a.handleV2AdminBindTelegram)
	a.add(http.MethodGet, "/api/v2/admin/users/by-telegram/:telegram_id", AuthAdmin, a.handleUserByTelegram)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/bind-emby", AuthAdmin, a.handleV2AdminBindEmby)
	a.add(http.MethodPost, "/api/v2/admin/users/batch/disable", AuthAdmin, a.handleV2BatchDisableUsers)
	a.add(http.MethodPost, "/api/v2/admin/users/batch/enable", AuthAdmin, a.handleV2BatchEnableUsers)
	a.add(http.MethodPost, "/api/v2/admin/users/batch/renew", AuthAdmin, a.handleV2BatchRenewUsers)
	a.add(http.MethodPost, "/api/v2/admin/users/batch/delete", AuthAdmin, a.handleV2BatchDeleteUsers)
	a.add(http.MethodPost, "/api/v2/admin/users/batch/emby-enable", AuthAdmin, a.handleV2BatchEmbyEnable)
	a.add(http.MethodPost, "/api/v2/admin/users/batch/emby-disable", AuthAdmin, a.handleV2BatchEmbyDisable)
	a.add(http.MethodPost, "/api/v2/admin/users/batch/refresh-status", AuthAdmin, a.handleV2BatchRefreshStatus)

	// Security management resources: devices, login history, IP blacklist
	a.add(http.MethodGet, "/api/v2/security/devices", AuthUser, a.handleV2UserDevices)
	a.add(http.MethodPost, "/api/v2/security/devices/:device_id/trust", AuthUser, a.handleV2TrustDevice)
	a.add(http.MethodDelete, "/api/v2/security/devices/:device_id", AuthUser, a.handleV2DeleteDevice)
	a.add(http.MethodGet, "/api/v2/security/login-history", AuthUser, a.handleV2UserLoginHistory)
	a.add(http.MethodGet, "/api/v2/admin/security/users/:uid/devices", AuthAdmin, a.handleV2AdminUserDevices)
	a.add(http.MethodPost, "/api/v2/admin/security/users/:uid/devices/:device_id/block", AuthAdmin, a.handleV2BlockDevice)
	a.add(http.MethodGet, "/api/v2/admin/security/users/:uid/login-history", AuthAdmin, a.handleV2AdminLoginHistory)
	a.add(http.MethodGet, "/api/v2/admin/security/ip-blacklist", AuthAdmin, a.handleV2IPBlacklist)
	a.add(http.MethodPost, "/api/v2/admin/security/ip-blacklist", AuthAdmin, a.handleV2AddIPBlacklist)
	a.add(http.MethodDelete, "/api/v2/admin/security/ip-blacklist", AuthAdmin, a.handleV2DeleteIPBlacklist)
	a.add(http.MethodGet, "/api/v2/admin/security/suspicious", AuthAdmin, a.handleV2Suspicious)

	// Emby management resources. Reads remain bounded and manual-refresh only;
	// mutations reuse the audited legacy handlers during the V2 migration.
	a.add(http.MethodGet, "/api/v2/admin/emby/users", AuthAdmin, a.handleV2AdminEmbyUsers)
	a.add(http.MethodGet, "/api/v2/admin/emby/device-audit", AuthAdmin, a.handleV2AdminEmbyDeviceAudit)
	a.add(http.MethodGet, "/api/v2/admin/emby/activity-logs", AuthAdmin, a.handleV2AdminEmbyActivityLogs)
	a.add(http.MethodPost, "/api/v2/admin/emby/activity-logs/sync", AuthAdmin, a.handleV2AdminEmbyActivityLogSync)
	a.add(http.MethodPost, "/api/v2/admin/emby/test", AuthAdmin, a.handleV2AdminEmbyConnectivityTest)
	a.add(http.MethodPost, "/api/v2/admin/emby/broadcast", AuthAdmin, a.handleV2AdminEmbyBroadcast)
	a.add(http.MethodPost, "/api/v2/admin/emby/sync", AuthAdmin, a.handleV2AdminEmbySync)
	a.add(http.MethodPost, "/api/v2/admin/emby/import-users", AuthAdmin, a.handleV2AdminEmbyImportUsers)
	a.add(http.MethodPost, "/api/v2/admin/emby/delete-unlinked", AuthAdmin, a.handleV2AdminEmbyDeleteUnlinked)
	a.add(http.MethodPost, "/api/v2/admin/emby/cleanup-orphans", AuthAdmin, a.handleV2AdminEmbyCleanupOrphans)
	a.add(http.MethodPost, "/api/v2/admin/emby/reset-bindings", AuthAdmin, a.handleV2AdminEmbyResetBindings)
	a.add(http.MethodPost, "/api/v2/admin/emby/create-standalone", AuthAdmin, a.handleV2AdminEmbyCreateStandalone)
	a.add(http.MethodPost, "/api/v2/admin/emby/force-set-password", AuthAdmin, a.handleV2AdminEmbyForceSetPassword)
	a.add(http.MethodPost, "/api/v2/admin/emby/users/:embyId/enable", AuthAdmin, a.handleV2AdminEmbyUserToggle)
	a.add(http.MethodPost, "/api/v2/admin/emby/users/:embyId/disable", AuthAdmin, a.handleV2AdminEmbyUserToggle)
	a.add(http.MethodPost, "/api/v2/admin/emby/users/:embyId/kick", AuthAdmin, a.handleV2AdminEmbyUserKick)

	// Emby extension resources: stats, online status, now playing, sessions, URLs
	// Emby 媒体库与人数的读取面与 V1 的 AuthUser 口径保持一致：不允许匿名访问，
	// 匿名调用者不应拿到站点媒体规模或在线人数。
	a.add(http.MethodGet, "/api/v2/emby/stats", AuthUser, a.handleV2EmbyStats)
	a.add(http.MethodGet, "/api/v2/emby/viewer-count", AuthUser, a.handleV2EmbyViewerCount)
	// 播放排行榜只对已登录账号开放（AuthUser）：无账号访客没有任何入口，
	// 普通用户能否查看由 PlayRankUserVisible 在 handler 内判断。
	a.add(http.MethodGet, "/api/v2/emby/play-rank", AuthUser, a.handleV2PlayRank)
	a.add(http.MethodGet, "/api/v2/admin/emby/play-rank", AuthAdmin, a.handleV2AdminPlayRank)
	// 注意：不存在 /api/v2/emby/now-playing 普通用户路由。观看明细属于隐私数据，
	// 任何角色都不能通过普通用户路由拉取"谁在看什么"。
	a.add(http.MethodGet, "/api/v2/admin/emby/now-playing", AuthAdmin, a.handleV2AdminEmbyNowPlaying)
	a.add(http.MethodGet, "/api/v2/emby/online", AuthUser, a.handleV2EmbyOnline)
	a.add(http.MethodGet, "/api/v2/emby/urls", AuthUser, a.handleV2EmbyURLs)
	a.add(http.MethodGet, "/api/v2/admin/emby/urls", AuthAdmin, a.handleV2AdminEmbyURLs)
	a.add(http.MethodGet, "/api/v2/emby/sessions", AuthUser, a.handleV2EmbyUserSessions)
	a.add(http.MethodGet, "/api/v2/admin/emby/sessions", AuthAdmin, a.handleV2EmbySessionsAdmin)

	// Telegram extension resources: status, unbind, rebind request, command catalog, roster stats
	a.add(http.MethodGet, "/api/v2/telegram/status", AuthUser, a.handleV2TelegramStatus)
	a.add(http.MethodPost, "/api/v2/telegram/unbind", AuthUser, a.handleV2TelegramUnbind)
	a.add(http.MethodPost, "/api/v2/telegram/rebind-request", AuthUser, a.handleV2TelegramRebindRequest)
	a.add(http.MethodGet, "/api/v2/telegram/commands", AuthPublic, a.handleV2TelegramCommandCatalog)
	a.add(http.MethodGet, "/api/v2/admin/telegram/roster/stats", AuthAdmin, a.handleV2TelegramRosterStats)

	// Export resources: users CSV export
	a.add(http.MethodGet, "/api/v2/admin/export/users", AuthAdmin, a.handleV2ExportUsers)

	// User self-service resources: username, password generation, renewal, code use, sessions
	a.add(http.MethodPut, "/api/v2/settings/username", AuthUser, a.handleV2UpdateUsername)
	a.add(http.MethodPut, "/api/v2/settings/password/generate", AuthUser, a.handleV2GeneratePassword)
	a.add(http.MethodPost, "/api/v2/me/renew", AuthUser, a.handleV2Renew)
	a.add(http.MethodPost, "/api/v2/me/use-code", AuthUser, a.handleV2UseCode)
	a.add(http.MethodGet, "/api/v2/me/use-code/status", AuthUser, a.handleV2QueueStatus)
	a.add(http.MethodGet, "/api/v2/me/sessions", AuthUser, a.handleV2Sessions)
	a.add(http.MethodPost, "/api/v2/me/telegram/rebind-complete", AuthUser, a.handleV2RebindComplete)
	a.add(http.MethodGet, "/api/v2/me/telegram/bind-code", AuthUser, a.handleV2UserBindCode)
	a.add(http.MethodGet, "/api/v2/me/telegram/bind-code/status", AuthUser, a.handleV2UserBindCodeStatus)

	// Registration flow polling endpoints
	a.add(http.MethodGet, "/api/v2/registration/regcode/check", AuthPublic, a.handleV2RegcodeCheck)
	a.add(http.MethodGet, "/api/v2/registration/telegram/bind-code/status", AuthPublic, a.handleV2BindCodeStatus)
	a.add(http.MethodPost, "/api/v2/registration/telegram/bind-confirm", AuthPublic, a.handleV2BindConfirmSecure)
	a.add(http.MethodGet, "/api/v2/registration/emby/queue-status", AuthPublic, a.handleV2QueueStatus)

	// Public system resources: config, icons, backgrounds, emby-urls
	a.add(http.MethodGet, "/api/v2/system/config", AuthUser, a.handleV2PublicConfig)
	a.add(http.MethodGet, "/api/v2/system/server-icon", AuthPublic, a.handleV2ServerIcon)
	a.add(http.MethodGet, "/api/v2/system/auth-background", AuthPublic, a.handleV2AuthBackground)
	a.add(http.MethodGet, "/api/v2/system/emby-urls", AuthUser, a.handleV2EmbyURLs)
	a.add(http.MethodPost, "/api/v2/system/emby-urls/probe", AuthUser, a.handleV2EmbyURLProbe)
	a.add(http.MethodGet, "/api/v2/admin/config", AuthAdmin, a.handleV2AdminConfig)
	a.add(http.MethodGet, "/api/v2/admin/runtime/logs/stream", AuthAdmin, a.handleV2RuntimeLogStream)
	a.add(http.MethodPost, "/api/v2/admin/system/update", AuthAdmin, a.handleV2SystemUpdate)
	a.add(http.MethodPost, "/api/v2/admin/system/server-icon/upload", AuthAdmin, a.handleV2UploadServerIcon)
	a.add(http.MethodPost, "/api/v2/admin/developer-mode/activate", AuthAdmin, a.handleV2DeveloperModeActivate)

	// Emby extension resources: status, search, latest, sessions/count, images
	a.add(http.MethodGet, "/api/v2/emby/status", AuthUser, a.handleV2EmbyStatus)
	a.add(http.MethodGet, "/api/v2/emby/search", AuthUser, a.handleV2EmbySearch)
	a.add(http.MethodGet, "/api/v2/emby/latest", AuthUser, a.handleV2EmbyLatest)
	a.add(http.MethodGet, "/api/v2/emby/sessions/count", AuthUser, a.handleV2EmbySessionCount)
	a.add(http.MethodGet, "/api/v2/emby/items/:item_id/image", AuthUser, a.handleV2EmbyItemImage)
	a.add(http.MethodPost, "/api/v2/emby/urls/probe", AuthUser, a.handleV2EmbyURLProbe)
	a.add(http.MethodPost, "/api/v2/emby/bangumi/webhook", AuthPublic, a.handleV2BangumiWebhook)
	a.add(http.MethodGet, "/api/v2/admin/emby/activity", AuthAdmin, a.handleV2AdminEmbyActivity)

	// Media extended endpoints
	a.add(http.MethodGet, "/api/v2/media/detail/:source_type/:media_id", AuthUser, a.handleV2MediaDetail2)
	a.add(http.MethodGet, "/api/v2/media/inventory/search", AuthUser, a.handleV2MediaInventorySearch)
	a.add(http.MethodGet, "/api/v2/media/requests/:request_id", AuthUser, a.handleV2MediaRequestByID)
	a.add(http.MethodDelete, "/api/v2/media/requests/:request_id", AuthUser, a.handleV2DeleteMediaRequestByKey)
	a.add(http.MethodPost, "/api/v2/media/requests/external/update", AuthPublic, a.handleV2ExternalMediaUpdate)

	// Invite/signin extended endpoints
	a.add(http.MethodGet, "/api/v2/invite/config", AuthPublic, a.handleV2InviteConfig)
	a.add(http.MethodGet, "/api/v2/invite/check", AuthPublic, a.handleV2InviteCheck)
	a.add(http.MethodPost, "/api/v2/invite/use", AuthUser, a.handleV2InviteUse)
	a.add(http.MethodGet, "/api/v2/signin/config", AuthPublic, a.handleV2SigninConfig)
	a.add(http.MethodGet, "/api/v2/signin/history", AuthUser, a.handleV2SigninHistory)

	// User appearance resources (public access)
	a.add(http.MethodGet, "/api/v2/users/:uid/background", AuthUser, a.handleV2UserBackground)
	a.add(http.MethodGet, "/api/v2/users/:uid/avatar", AuthUser, a.handleV2UserAvatar)
	a.add(http.MethodGet, "/api/v2/users/assets/:kind/:filename", AuthUser, a.handleV2UserAsset)

	// Admin extended user operations
	a.add(http.MethodPost, "/api/v2/admin/users/bulk-expire", AuthAdmin, a.handleV2AdminBulkExpire)
	a.add(http.MethodPost, "/api/v2/admin/users/bulk-enable-disabled", AuthAdmin, a.handleV2AdminBulkEnableDisabled)
	a.add(http.MethodPost, "/api/v2/admin/users/cleanup-invalid", AuthAdmin, a.handleV2AdminCleanupInvalid)
	a.add(http.MethodPost, "/api/v2/admin/users/clear-stale-pending-emby", AuthAdmin, a.handleV2AdminClearStalePendingEmby)
	a.add(http.MethodPost, "/api/v2/admin/users/clear-emails", AuthAdmin, a.handleV2AdminClearUserEmails)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/bind-email", AuthAdmin, a.handleV2AdminBindUserEmail)
	a.add(http.MethodPost, "/api/v2/admin/users/:uid/email/verified", AuthAdmin, a.handleV2AdminSetUserEmailVerified)
	a.add(http.MethodPost, "/api/v2/admin/users/kick-no-emby", AuthAdmin, a.handleV2AdminKickNoEmby)
	a.add(http.MethodPost, "/api/v2/admin/whitelist", AuthAdmin, a.handleV2AdminWhitelist)

	// Batch extended operations
	a.add(http.MethodPost, "/api/v2/admin/users/batch/emby-unbind-lock", AuthAdmin, a.handleV2BatchEmbyUnbindLock)
	a.add(http.MethodPost, "/api/v2/admin/users/batch/emby-grant-clear", AuthAdmin, a.handleV2BatchEmbyGrantClear)
	a.add(http.MethodPost, "/api/v2/admin/users/batch/emby-grant-all-libraries", AuthAdmin, a.handleV2BatchGrantAllLibraries)
	a.add(http.MethodGet, "/api/v2/admin/users/expiring", AuthAdmin, a.handleV2ExpiringUsers)
	a.add(http.MethodPost, "/api/v2/admin/users/send-reminders", AuthAdmin, a.handleV2SendReminders)

	// Telegram admin extended
	a.add(http.MethodPost, "/api/v2/admin/telegram/rejoined-users/enable", AuthAdmin, a.handleV2TelegramRejoinedEnable)
	a.add(http.MethodPost, "/api/v2/admin/telegram/kick-unbound", AuthAdmin, a.handleV2TelegramKickUnbound)

	// Security extended
	a.add(http.MethodPost, "/api/v2/security/devices/:device_id/block", AuthUser, a.handleV2SecurityBlockDevice)

	a.registerV2CompletionRoutes()
}

// registerV2CompletionRoutes closes the remaining V1-only gaps so the product
// frontend can run entirely on /api/v2/*. Handlers are the audited V1
// implementations: the goal of this block is contract parity, not behaviour
// change, and the shared middleware chain already applies CORS, body limits,
// rate limits, authentication and fallback auditing to every entry below.
func (a *App) registerV2CompletionRoutes() {
	// API key integration surface. V1 kept this behind AuthAPIKey only; V2
	// mirrors it with the same permission wrappers so external clients can
	// move off /api/v1 without weakening scopes.
	a.add(http.MethodGet, "/api/v2/apikey/info", AuthAPIKey, a.withAPIKeyPermission(apiKeyPermissionAccountRead, a.handleAPIKeyInfo))
	a.add(http.MethodGet, "/api/v2/apikey/status", AuthAPIKey, a.withAPIKeyPermission(apiKeyPermissionAccountRead, a.handleAPIKeyStatus))
	a.add(http.MethodPost, "/api/v2/apikey/enable", AuthAPIKey, a.withAPIKeyPermission(apiKeyPermissionAccountWrite, a.handleAPIKeyEnableAccount))
	a.add(http.MethodPost, "/api/v2/apikey/disable", AuthAPIKey, a.withAPIKeyPermission(apiKeyPermissionAccountWrite, a.handleAPIKeyDisableAccount))
	a.add(http.MethodPost, "/api/v2/apikey/renew", AuthAPIKey, a.withAPIKeyPermission(apiKeyPermissionAccountWrite, a.handleAPIKeyRenew))
	a.add(http.MethodPost, "/api/v2/apikey/key/refresh", AuthAPIKey, a.withAPIKeyPermission(apiKeyPermissionAccountWrite, a.handleLegacyAPIKeyGenerate))
	a.add(http.MethodGet, "/api/v2/apikey/permissions", AuthAPIKey, a.handleAPIKeyPermissions)
	a.add(http.MethodPut, "/api/v2/apikey/permissions", AuthAPIKey, a.handleForbiddenSelfPermission)
	a.add(http.MethodPost, "/api/v2/apikey/key/disable", AuthAPIKey, a.handleAPIKeyDisableKey)
	a.add(http.MethodPost, "/api/v2/apikey/key/enable", AuthAPIKey, a.handleAPIKeyEnableKey)
	a.add(http.MethodGet, "/api/v2/apikey/emby/status", AuthAPIKey, a.withAPIKeyPermission(apiKeyPermissionEmbyRead, a.handleEmbyStatus))
	a.add(http.MethodPost, "/api/v2/apikey/emby/kick", AuthAPIKey, a.withAPIKeyPermission(apiKeyPermissionEmbyWrite, a.handleAPIKeyEmbyKick))
	a.add(http.MethodPost, "/api/v2/apikey/use-code", AuthAPIKey, a.withAPIKeyPermission(apiKeyPermissionAccountWrite, a.handleUseCode))

	// Legacy per-user API key management kept under the auth resource family.
	a.add(http.MethodGet, "/api/v2/auth/apikey", AuthUser, a.handleLegacyAPIKeyStatus)
	a.add(http.MethodPost, "/api/v2/auth/apikey", AuthUser, a.handleLegacyAPIKeyGenerate)
	a.add(http.MethodDelete, "/api/v2/auth/apikey", AuthUser, a.handleLegacyAPIKeyDelete)
	a.add(http.MethodPost, "/api/v2/auth/apikey/enable", AuthUser, a.handleLegacyAPIKeyEnable)
	a.add(http.MethodGet, "/api/v2/auth/apikey/permissions", AuthUser, a.handleLegacyAPIKeyPermissions)
	a.add(http.MethodPut, "/api/v2/auth/apikey/permissions", AuthUser, a.handleLegacyAPIKeyPermissionsUpdate)

	// Telegram bind-code websocket transports. V1 exposes them for both the
	// public registration flow and the authenticated account page.
	a.add(http.MethodGet, "/api/v2/users/telegram/register/bind-code/ws", AuthPublic, a.handleBindCodeStatusWS)
	a.add(http.MethodGet, "/api/v2/me/telegram/bind-code/ws", AuthUser, a.handleUserBindCodeStatusWS)

	// Personalised announcement feed. The public /api/v2/announcements route
	// is anonymous and cannot carry per-user force-read state.
	a.add(http.MethodGet, "/api/v2/me/announcements", AuthUser, a.handleAnnouncementsMe)

	// Invite code listing for the account page.
	a.add(http.MethodGet, "/api/v2/invite/codes", AuthUser, a.handleInviteCodes)
	a.add(http.MethodGet, "/api/v2/invite/me", AuthUser, a.handleInviteMe)

	// Bangumi read surface parity.
	a.add(http.MethodGet, "/api/v2/bangumi/me", AuthUser, a.handleBangumiMe)
	a.add(http.MethodGet, "/api/v2/bangumi/sync/status", AuthUser, a.handleBangumiSyncStatus)
	a.add(http.MethodGet, "/api/v2/bangumi/sync/history", AuthUser, a.handleBangumiSyncHistory)

	// Media detail aliases used by existing deep links.
	a.add(http.MethodGet, "/api/v2/media/tmdb/:tmdb_id", AuthUser, a.handleMediaDetail)
	a.add(http.MethodGet, "/api/v2/media/bangumi/:bgm_id", AuthUser, a.handleMediaDetail)

	// Administrator self-service and Emby unbind parity. V1 accepted PUT on
	// regcodes and tickets; V2 registered PATCH only, so V1 clients and any
	// tooling still issuing PUT must keep working.
	a.add(http.MethodPut, "/api/v2/admin/me/update", AuthAdmin, a.handleUpdateMe)
	a.add(http.MethodDelete, "/api/v2/admin/users/:uid/emby", AuthAdmin, a.handleAdminUnbindEmby)
	a.add(http.MethodPut, "/api/v2/admin/regcodes/:code", AuthAdmin, a.handleUpdateRegcode)
	a.add(http.MethodPut, "/api/v2/admin/tickets/:ticket_id", AuthAdmin, a.handleAdminUpdateTicket)

	// System statistics and documentation parity.
	a.add(http.MethodPost, "/api/v2/settings/password/change", AuthUser, a.handleChangePassword)

	// System statistics and documentation parity.
	a.add(http.MethodGet, "/api/v2/system/stats", AuthAdmin, a.handleSystemStats)
	a.add(http.MethodGet, "/api/v2/system/emby-stats", AuthUser, a.handleEmbyStats)
	a.add(http.MethodGet, "/api/v2/system/emby-viewers", AuthUser, a.handleEmbyViewerCount)
	a.add(http.MethodGet, "/api/v2/system/health/api", AuthAdmin, a.handleHealthAPI)
	a.add(http.MethodGet, "/api/v2/system/health/database", AuthAdmin, a.handleHealthDatabase)
	a.add(http.MethodGet, "/api/v2/system/health/emby", AuthAdmin, a.handleHealthEmby)
	a.add(http.MethodGet, "/api/v2/docs", AuthPublic, a.handleDocs)
}
