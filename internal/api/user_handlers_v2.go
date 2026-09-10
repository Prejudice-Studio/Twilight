package api

import (
	"net/http"
)

func (a *App) handleV2UpdateUsername(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleUpdateUsername(w, r, p)
}

func (a *App) handleV2GeneratePassword(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleGeneratedPassword(w, r, p)
}

func (a *App) handleV2Renew(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleRenew(w, r, p)
}

func (a *App) handleV2UseCode(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleUseCode(w, r, p)
}

func (a *App) handleV2Sessions(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleSessions(w, r, p)
}

func (a *App) handleV2RebindComplete(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleRebindComplete(w, r, p)
}

func (a *App) handleV2UserBindCode(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleUserBindCode(w, r, p)
}

func (a *App) handleV2UserBindCodeStatus(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleUserBindCodeStatus(w, r, p)
}

func (a *App) handleV2QueueStatus(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleQueueStatus(w, r, p)
}

func (a *App) handleV2PublicConfig(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	a.handlePublicConfig(w, r, p)
}

func (a *App) handleV2AdminConfig(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleAdminConfig(w, r, p)
}

func (a *App) handleV2EmbyStatus(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleEmbyStatus(w, r, p)
}

func (a *App) handleV2EmbySearch(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleMediaSearch(w, r, p)
}

func (a *App) handleV2EmbyLatest(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleEmbyLatest(w, r, p)
}

func (a *App) handleV2EmbySessionCount(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleSessionCount(w, r, p)
}

func (a *App) handleV2EmbyItemImage(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleEmbyItemImage(w, r, p)
}

func (a *App) handleV2ServerIcon(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleServerIcon(w, r, p)
}

func (a *App) handleV2AuthBackground(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAuthBackground(w, r, p)
}

func (a *App) handleV2RuntimeLogStream(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleRuntimeLogStream(w, r, p)
}

func (a *App) handleV2SystemUpdate(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleSystemUpdate(w, r, p)
}

func (a *App) handleV2UploadServerIcon(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleUploadServerIcon(w, r, p)
}

func (a *App) handleV2InviteConfig(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	a.handleInviteConfig(w, r, p)
}

func (a *App) handleV2InviteCheck(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleInviteCheck(w, r, p)
}

func (a *App) handleV2InviteUse(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleInviteUse(w, r, p)
}

func (a *App) handleV2SigninConfig(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	a.handleSigninConfig(w, r, p)
}

func (a *App) handleV2SigninHistory(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleSigninHistory(w, r, p)
}

func (a *App) handleV2UserAsset(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAsset(w, r, p)
}

func (a *App) handleV2UserBackground(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleGetBackground(w, r, p)
}

func (a *App) handleV2UserAvatar(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleGetAvatar(w, r, p)
}

func (a *App) handleV2AdminEmbyActivity(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleEmbyActivity(w, r, p)
}

func (a *App) handleV2EmbyURLProbe(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleEmbyURLProbe(w, r, p)
}

func (a *App) handleV2MediaDetail2(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleMediaDetail(w, r, p)
}

func (a *App) handleV2MediaInventorySearch(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleInventorySearch(w, r, p)
}

func (a *App) handleV2MediaRequestByID(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleMediaRequestByID(w, r, p)
}

func (a *App) handleV2ExternalMediaUpdate(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleExternalMediaUpdate(w, r, p)
}

func (a *App) handleV2RegcodeCheck(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleRegcodeCheck(w, r, p)
}

func (a *App) handleV2BindCodeStatus(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleBindCodeStatus(w, r, p)
}

func (a *App) handleV2BindConfirmSecure(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleBindConfirmSecure(w, r, p)
}

func (a *App) handleV2AdminBulkExpire(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleAdminBulkExpire(w, r, p)
}

func (a *App) handleV2AdminBulkEnableDisabled(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleAdminBulkEnableDisabled(w, r, p)
}

func (a *App) handleV2AdminCleanupInvalid(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleAdminCleanupInvalid(w, r, p)
}

func (a *App) handleV2AdminClearStalePendingEmby(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleAdminClearStalePendingEmby(w, r, p)
}

func (a *App) handleV2AdminClearUserEmails(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleAdminClearUserEmails(w, r, p)
}

func (a *App) handleV2AdminBindUserEmail(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleAdminBindUserEmail(w, r, p)
}

func (a *App) handleV2AdminSetUserEmailVerified(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleAdminSetUserEmailVerified(w, r, p)
}

func (a *App) handleV2AdminKickNoEmby(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleAdminKickNoEmby(w, r, p)
}

func (a *App) handleV2AdminWhitelist(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleWhitelist(w, r, p)
}

func (a *App) handleV2BatchEmbyUnbindLock(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleBatchLockEmbyUnbind(w, r, p)
}

func (a *App) handleV2BatchEmbyGrantClear(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleBatchClearEmbyGrantUnbound(w, r, p)
}

func (a *App) handleV2BatchGrantAllLibraries(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleBatchGrantAllLibraries(w, r, p)
}

func (a *App) handleV2ExpiringUsers(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleExpiringUsers(w, r, p)
}

func (a *App) handleV2SendReminders(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleSendReminders(w, r, p)
}

func (a *App) handleV2TelegramRejoinedEnable(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleTelegramRejoinedEnable(w, r, p)
}

func (a *App) handleV2TelegramKickUnbound(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleTelegramKickUnbound(w, r, p)
}

func (a *App) handleV2SecurityBlockDevice(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleBlockDevice(w, r, p)
}

func (a *App) handleV2AdminLoginHistoryByUID(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleLoginHistory(w, r, p)
}

func (a *App) handleV2DeveloperModeActivate(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "no-store")
	a.handleDeveloperModeActivate(w, r, p)
}

func (a *App) handleV2BangumiWebhook(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleBangumiWebhook(w, r, p)
}
