package api

import (
	"net/http"
)

// handleV2SystemInfo exposes only the public system projection needed by the
// SSR setup and shell pages. Configuration secrets, upstream addresses and
// dependency diagnostics stay behind their dedicated admin resources.
func (a *App) handleV2SystemInfo(w http.ResponseWriter, _ *http.Request, _ Params) {
	cfg := a.cfg()
	features := map[string]bool{
		"register":                      cfg.RegisterEnabled,
		"emby_direct_register":          cfg.EmbyDirectRegisterEnabled,
		"telegram":                      cfg.TelegramMode,
		"force_bind_telegram":           cfg.ForceBindTelegram,
		"force_bind_group":              cfg.TelegramForceBindGroup,
		"force_bind_channel":            cfg.TelegramForceBindChannel,
		"bangumi_sync":                  cfg.BangumiEnabled,
		"bangumi_manage":                cfg.BangumiManageEnabled,
		"media_request":                 cfg.MediaRequestEnabled,
		"signin":                        cfg.SigninEnabled,
		"invite":                        cfg.InviteEnabled,
		"email_enabled":                 emailConfigured(cfg),
		"force_bind_email":              cfg.EmailForceBind,
		"forgot_password_enabled":       cfg.ForgotPasswordEnabled,
		"forgot_password_emby_enabled":  cfg.ForgotPasswordEmbyEnabled,
		"forgot_password_email_enabled": cfg.ForgotPasswordEmailEnabled,
		"ticket_system":                 cfg.TicketSystemEnabled,
		"developer_mode":                a.store().DeveloperModeEnabled(),
		"activity_logs":                 true,
		"viewing_stats":                 false,
	}
	limits := map[string]any{
		"user_limit":             zeroNil(int64(cfg.UserLimit)),
		"stream_limit":           cfg.MaxStreams,
		"ticket_image_max_size":  cfg.TicketImageMaxSize,
		"ticket_image_max_count": cfg.TicketImageMaxCount,
	}
	ok(w, "OK", map[string]any{
		"name":        cfg.AppName,
		"icon":        a.publicServerIconURL(),
		"version":     cfg.Version,
		"api_version": "v2",
		"features":    features,
		"limits":      limits,
		"setup":       a.setupStatusData(),
	})
}

func markV2AdminHealth(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
}

// Each health resource intentionally owns one probe. Do not combine these
// calls: the status page must be able to distinguish API, database and Emby
// failures and render successful probes when another dependency is down.
func (a *App) handleV2AdminHealthAPI(w http.ResponseWriter, _ *http.Request, _ Params) {
	markV2AdminHealth(w)
	ok(w, "OK", a.apiHealth())
}

func (a *App) handleV2AdminHealthDatabase(w http.ResponseWriter, r *http.Request, _ Params) {
	markV2AdminHealth(w)
	ok(w, "OK", a.databaseHealth(r.Context()))
}

func (a *App) handleV2AdminHealthEmby(w http.ResponseWriter, r *http.Request, _ Params) {
	markV2AdminHealth(w)
	ok(w, "OK", a.embyStatusSnapshot(r.Context(), true))
}

func (a *App) handleV2AdminStats(w http.ResponseWriter, _ *http.Request, _ Params) {
	markV2AdminHealth(w)
	ok(w, "OK", a.systemStatsData())
}
