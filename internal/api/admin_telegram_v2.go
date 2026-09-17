package api

import "net/http"

// Telegram management resources keep the existing command catalog, roster
// projection and safe Bot test as the single business implementation while
// giving the WebUI an explicit V2 boundary.
func (a *App) handleV2AdminTelegramCommandCatalog(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleTelegramCommandCatalog(w, r, p)
}

func (a *App) handleV2AdminTelegramRosterStats(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleTelegramRosterStats(w, r, p)
}

func (a *App) handleV2AdminTelegramBotTest(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleBotTest(w, r, p)
}
