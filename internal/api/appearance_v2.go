package api

import "net/http"

// V2 appearance resources keep the WebUI on one private read and
// explicit form-action writes. The existing upload/background handlers remain
// the only business implementation for validation, rate limits, safe paths,
// persistence and asset URLs.
func (a *App) handleV2UserAppearance(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	u := current(r).User
	ok(w, "OK", map[string]any{
		"avatar":     u.Avatar,
		"background": u.Background,
	})
}

func (a *App) handleV2UpdateUserBackground(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleUpdateBackground(w, r, p)
}

func (a *App) handleV2DeleteUserBackground(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleDeleteBackground(w, r, p)
}

func (a *App) handleV2UploadUserBackground(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleUploadBackground(w, r, p)
}

func (a *App) handleV2UploadUserAvatar(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleUploadAvatar(w, r, p)
}

func (a *App) handleV2DeleteUserAvatar(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleDeleteAvatar(w, r, p)
}
