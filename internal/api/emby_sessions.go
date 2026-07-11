package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

const embySessionsCacheTTL = 5 * time.Second

func cloneEmbySessions(sessions []map[string]any) []map[string]any {
	out := make([]map[string]any, len(sessions))
	for i, session := range sessions {
		copySession := make(map[string]any, len(session))
		for key, value := range session {
			copySession[key] = value
		}
		out[i] = copySession
	}
	return out
}

// embySessionsSnapshot coalesces the many dashboard/admin polling paths that
// need /Sessions. Holding the mutex across the upstream request intentionally
// provides single-flight behavior for concurrent callers.
func (a *App) embySessionsSnapshot(ctx context.Context, force bool) ([]map[string]any, error) {
	a.embySessionsMu.Lock()
	defer a.embySessionsMu.Unlock()

	if !force && a.embySessionsCache != nil && time.Now().Before(a.embySessionsUntil) {
		return cloneEmbySessions(a.embySessionsCache), nil
	}

	var sessions []map[string]any
	if err := a.embyGet(ctx, "/Sessions", &sessions); err != nil {
		return nil, err
	}
	a.embySessionsCache = cloneEmbySessions(sessions)
	a.embySessionsUntil = time.Now().Add(embySessionsCacheTTL)
	return cloneEmbySessions(sessions), nil
}

func (a *App) invalidateEmbySessionsSnapshot() {
	a.embySessionsMu.Lock()
	a.embySessionsUntil = time.Time{}
	a.embySessionsCache = nil
	a.embySessionsMu.Unlock()
}

func embySessionNowPlaying(session map[string]any) (map[string]any, bool) {
	item, ok := session["NowPlayingItem"].(map[string]any)
	return item, ok && item != nil
}

func countEmbyPlayingSessions(sessions []map[string]any) int {
	count := 0
	for _, session := range sessions {
		if _, playing := embySessionNowPlaying(session); playing {
			count++
		}
	}
	return count
}

func (a *App) handleEmbyNowPlaying(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.embyConfigured() {
		ok(w, "OK", map[string]any{"viewers": 0, "items": []any{}})
		return
	}
	sessions, err := a.embySessionsSnapshot(r.Context(), false)
	if err != nil {
		ok(w, "OK", map[string]any{"viewers": 0, "items": []any{}})
		return
	}
	type nowPlayingItem struct {
		ItemID       string `json:"item_id"`
		ItemName     string `json:"item_name"`
		SeriesName   string `json:"series_name,omitempty"`
		MediaType    string `json:"media_type"`
		ImageURL     string `json:"image_url,omitempty"`
		UserName     string `json:"user_name"`
		PlayDuration int64  `json:"play_duration"`
		TotalRuntime int64  `json:"total_runtime"`
	}
	items := make([]nowPlayingItem, 0)
	itemIDs := make([]string, 0)
	for _, session := range sessions {
		item, playing := embySessionNowPlaying(session)
		if !playing {
			continue
		}
		itemID := firstNonEmpty(asString(item["Id"]), asString(item["ID"]), asString(item["id"]))
		itemName := firstNonEmpty(asString(item["Name"]), asString(item["name"]))
		seriesName := firstNonEmpty(asString(item["SeriesName"]), asString(item["SeriesId"]), asString(item["Album"]))
		mediaType := strings.ToLower(strings.TrimSpace(firstNonEmpty(asString(item["Type"]), asString(item["type"]), asString(item["MediaType"]), "other")))
		userName := firstNonEmpty(asString(session["UserName"]), asString(session["userName"]), asString(session["Client"]), "未知")
		imgID := itemID
		if mediaType == "episode" {
			parentID := firstNonEmpty(asString(item["SeriesId"]), asString(item["ParentId"]))
			if parentID != "" {
				imgID = parentID
			}
		}
		posTicks := int64(numeric(item["PlaybackPositionTicks"]))
		runTicks := int64(numeric(item["RunTimeTicks"]))
		items = append(items, nowPlayingItem{
			ItemID:       itemID,
			ItemName:     itemName,
			SeriesName:   seriesName,
			MediaType:    mediaType,
			ImageURL:     embyPlaybackImageURL(imgID),
			UserName:     userName,
			PlayDuration: posTicks / 10000000,
			TotalRuntime: runTicks / 10000000,
		})
		if itemID != "" {
			itemIDs = append(itemIDs, itemID)
		}
	}
	resp := map[string]any{
		"viewers": len(items),
		"items":   items,
	}
	if len(itemIDs) > 0 {
		maxItems := min(len(itemIDs), 50)
		metadata := a.embyPlaybackMetadata(r.Context(), makeMetadataEvents(itemIDs[:maxItems]))
		enriched := make([]map[string]any, 0, len(items))
		for _, entry := range items {
			meta, ok := metadata[entry.ItemID]
			mapped := map[string]any{
				"item_id":       entry.ItemID,
				"item_name":     entry.ItemName,
				"series_name":   entry.SeriesName,
				"media_type":    entry.MediaType,
				"image_url":     entry.ImageURL,
				"user_name":     entry.UserName,
				"play_duration": entry.PlayDuration,
				"total_runtime": entry.TotalRuntime,
			}
			if ok && meta.SeriesName != "" && mapped["series_name"] == nil {
				mapped["series_name"] = meta.SeriesName
			}
			if ok && meta.Name != "" {
				mapped["item_name"] = meta.Name
			}
			if ok && meta.SeriesID != "" && entry.ImageURL == "" {
				mapped["image_url"] = embyPlaybackImageURL(meta.SeriesID)
			}
			enriched = append(enriched, mapped)
		}
		resp["items"] = enriched
	}
	ok(w, "OK", resp)
}

func makeMetadataEvents(ids []string) []embyPlaybackEvent {
	events := make([]embyPlaybackEvent, len(ids))
	for i, id := range ids {
		events[i] = embyPlaybackEvent{ItemID: id}
	}
	return events
}

func (a *App) handleEmbyOnline(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.embyConfigured() {
		ok(w, "OK", map[string]any{"online": false, "current_online": 0, "users": []any{}})
		return
	}
	u := current(r).User
	canSeeDetails := u.Role == store.RoleAdmin || u.Role == store.RoleWhitelist
	sessions, err := a.embySessionsSnapshot(r.Context(), false)
	if err != nil {
		ok(w, "OK", map[string]any{"online": false, "current_online": 0, "users": []any{}})
		return
	}
	currentOnline := 0
	users := make([]map[string]any, 0, len(sessions))
	for _, session := range sessions {
		nowPlaying, _ := session["NowPlayingItem"].(map[string]any)
		if nowPlaying == nil {
			continue
		}
		currentOnline++
		if !canSeeDetails {
			continue
		}
		users = append(users, map[string]any{
			"username":      firstNonEmpty(asString(session["UserName"]), asString(session["UserId"])),
			"item_name":     firstNonEmpty(asString(nowPlaying["SeriesName"]), asString(nowPlaying["Name"])),
			"media_type":    asString(nowPlaying["Type"]),
			"client":        firstNonEmpty(asString(session["Client"]), asString(session["AppName"])),
			"device_name":   asString(session["DeviceName"]),
			"last_activity": asString(session["LastActivityDate"]),
		})
	}
	ok(w, "OK", map[string]any{
		"online":         true,
		"current_online": currentOnline,
		"users":          users,
	})
}
