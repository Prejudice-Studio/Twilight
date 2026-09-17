package api

import "net/http"

// Developer resources expose the existing Goja validation, execution,
// capability gate, audit and preset persistence through the V2 namespace.
// The adapter intentionally does not duplicate sandbox policy.
func (a *App) handleV2DeveloperJSSandbox(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleDeveloperJSSandbox(w, r, p)
}

func (a *App) handleV2DeveloperJSDocs(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleDeveloperJSDocs(w, r, p)
}

func (a *App) handleV2DeveloperJSPresets(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleDeveloperJSPresets(w, r, p)
}

func (a *App) handleV2CreateDeveloperJSPreset(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleCreateDeveloperJSPreset(w, r, p)
}

func (a *App) handleV2UpdateDeveloperJSPreset(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleUpdateDeveloperJSPreset(w, r, p)
}

func (a *App) handleV2DeleteDeveloperJSPreset(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleDeleteDeveloperJSPreset(w, r, p)
}
