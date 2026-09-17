package api

import "net/http"

// Migration remains a streamed/ multipart admin resource. These adapters keep
// the archive validator, feature gate, password handling and transactional
// import logic in the canonical handlers.
func (a *App) handleV2MigrationStatus(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleMigrationStatus(w, r, p)
}

func (a *App) handleV2MigrationExport(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleMigrationExport(w, r, p)
}

func (a *App) handleV2MigrationImport(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleMigrationImport(w, r, p)
}
