package api

import "net/http"

// Runtime V2 resources intentionally expose the existing bounded snapshot
// handlers only. The browser never receives the legacy SSE stream.
func (a *App) handleV2RuntimeStatus(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleRuntimeStatus(w, r, p)
}

func (a *App) handleV2RuntimeLogs(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleRuntimeLogs(w, r, p)
}
