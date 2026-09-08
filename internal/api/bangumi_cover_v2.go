package api

import "net/http"

// Covers are intentionally public cacheable image resources. The canonical
// handler owns subject validation, safe local-file access and CDN fallback.
func (a *App) handleV2BangumiCover(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleBangumiCover(w, r, p)
}
