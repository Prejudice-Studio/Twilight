package api

import (
	"net/http"
)

func (a *App) handleV2EmbyStats(w http.ResponseWriter, r *http.Request, _ Params) {
	result, err := a.emby().stats(r.Context())
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "获取 Emby 统计失败: "+err.Error())
		return
	}
	ok(w, "OK", result)
}

func (a *App) handleV2EmbyViewerCount(w http.ResponseWriter, r *http.Request, _ Params) {
	count, _ := a.emby().viewerCount(r.Context())
	ok(w, "OK", map[string]any{"viewers": count})
}

func (a *App) handleV2EmbyNowPlaying(w http.ResponseWriter, r *http.Request, _ Params) {
	result, _ := a.emby().nowPlaying(r.Context())
	ok(w, "OK", result)
}

func (a *App) handleV2EmbyOnline(w http.ResponseWriter, r *http.Request, _ Params) {
	result, _ := a.emby().online(r.Context())
	ok(w, "OK", result)
}

func (a *App) handleV2EmbySessionsAdmin(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.embyConfigured() {
		ok(w, "OK", []any{})
		return
	}
	remote, err := a.embySessionsSnapshot(r.Context(), false)
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyRemoteSessionsFail, "failed to read Emby sessions")
		return
	}
	items := []map[string]any{}
	for _, session := range remote {
		nowPlaying := any(nil)
		if item, ok := session["NowPlayingItem"].(map[string]any); ok {
			nowPlaying = map[string]any{"id": item["Id"], "name": item["Name"], "type": item["Type"]}
		}
		local := any(nil)
		if u, okUser := a.store().FindUserByEmbyID(asString(session["UserId"])); okUser {
			local = map[string]any{"uid": u.UID, "username": u.Username, "telegram_id": nullableInt(u.TelegramID)}
		}
		items = append(items, map[string]any{
			"session_id":          asString(session["Id"]),
			"user_id":             asString(session["UserId"]),
			"user_name":           asString(session["UserName"]),
			"client":              asString(session["Client"]),
			"device_name":         asString(session["DeviceName"]),
			"device_id":           asString(session["DeviceId"]),
			"remote_endpoint":     asString(session["RemoteEndPoint"]),
			"application_version": asString(session["ApplicationVersion"]),
			"is_active":           boolish(session["IsActive"]),
			"now_playing":         nowPlaying,
			"local_user":          local,
		})
	}
	ok(w, "OK", items)
}

func (a *App) handleV2EmbyUserSessions(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.embyConfigured() {
		ok(w, "OK", []any{})
		return
	}
	remote, err := a.embySessionsSnapshot(r.Context(), false)
	if err != nil {
		ok(w, "OK", []any{})
		return
	}
	user := current(r).User
	items := []map[string]any{}
	for _, session := range remote {
		if asString(session["UserId"]) != user.EmbyID {
			continue
		}
		nowPlaying := any(nil)
		if item, ok := session["NowPlayingItem"].(map[string]any); ok {
			nowPlaying = map[string]any{"id": item["Id"], "name": item["Name"], "type": item["Type"]}
		}
		items = append(items, map[string]any{
			"session_id":          asString(session["Id"]),
			"client":              asString(session["Client"]),
			"device_name":         asString(session["DeviceName"]),
			"device_id":           asString(session["DeviceId"]),
			"remote_endpoint":     asString(session["RemoteEndPoint"]),
			"application_version": asString(session["ApplicationVersion"]),
			"is_active":           boolish(session["IsActive"]),
			"now_playing":         nowPlaying,
		})
	}
	ok(w, "OK", items)
}

func (a *App) handleV2EmbyURLs(w http.ResponseWriter, r *http.Request, _ Params) {
	cfg := a.cfg()
	lines := []map[string]string{}
	for _, line := range cfg.EmbyURLList {
		lines = append(lines, map[string]string{"name": line.Name, "url": line.URL})
	}
	if cfg.EmbyPublicURL != "" {
		lines = append(lines, map[string]string{"name": "默认线路", "url": cfg.EmbyPublicURL})
	}
	ok(w, "OK", map[string]any{"lines": lines})
}
