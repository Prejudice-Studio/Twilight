package api

import (
	"context"
	"strings"
)

type embyService struct {
	app *App
}

type embyStatsResult struct {
	Enabled      bool  `json:"enabled"`
	Configured   bool  `json:"configured"`
	MovieCount   int64 `json:"movie_count"`
	SeriesCount  int64 `json:"series_count"`
	EpisodeCount int64 `json:"episode_count"`
}

type embyOnlineResult struct {
	Online        bool  `json:"online"`
	CurrentOnline int   `json:"current_online"`
	Users         []any `json:"users"`
}

type embyNowPlayingItem struct {
	ItemID       string `json:"item_id"`
	ItemName     string `json:"item_name"`
	SeriesName   string `json:"series_name,omitempty"`
	MediaType    string `json:"media_type"`
	ImageURL     string `json:"image_url,omitempty"`
	UserName     string `json:"user_name"`
	PlayDuration int64  `json:"play_duration"`
	TotalRuntime int64  `json:"total_runtime"`
}

func (s *embyService) stats(ctx context.Context) (embyStatsResult, error) {
	result := embyStatsResult{
		Enabled:    s.app.cfg().EmbyStatsEnabled,
		Configured: s.app.embyConfigured(),
	}

	if !result.Enabled || !result.Configured {
		return result, nil
	}

	var counts map[string]any
	if err := s.app.embyGet(ctx, "/Items/Counts", &counts); err != nil {
		return result, err
	}

	result.MovieCount = int64(numeric(counts["MovieCount"]))
	result.SeriesCount = int64(numeric(counts["SeriesCount"]))
	result.EpisodeCount = int64(numeric(counts["EpisodeCount"]))

	return result, nil
}

func (s *embyService) online(ctx context.Context) (embyOnlineResult, error) {
	result := embyOnlineResult{
		Online: false,
		Users:  []any{},
	}

	if !s.app.embyConfigured() {
		return result, nil
	}

	sessions, err := s.app.embySessionsSnapshot(ctx, false)
	if err != nil {
		return result, nil
	}

	result.Online = true
	for _, session := range sessions {
		if nowPlaying, _ := session["NowPlayingItem"].(map[string]any); nowPlaying != nil {
			result.CurrentOnline++
		}
	}

	return result, nil
}

func (s *embyService) nowPlaying(ctx context.Context) (map[string]any, error) {
	if !s.app.embyConfigured() {
		return map[string]any{"viewers": 0, "items": []any{}}, nil
	}

	sessions, err := s.app.embySessionsSnapshot(ctx, false)
	if err != nil {
		return map[string]any{"viewers": 0, "items": []any{}}, nil
	}

	items := make([]embyNowPlayingItem, 0)
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

		items = append(items, embyNowPlayingItem{
			ItemID:       itemID,
			ItemName:     itemName,
			SeriesName:   seriesName,
			MediaType:    mediaType,
			ImageURL:     embyItemImageURL(imgID),
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
		metadata := s.app.embyItemMetadata(ctx, itemIDs[:maxItems])
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
				mapped["image_url"] = embyItemImageURL(meta.SeriesID)
			}
			enriched = append(enriched, mapped)
		}
		resp["items"] = enriched
	}

	return resp, nil
}

func (s *embyService) viewerCount(ctx context.Context) (int, error) {
	if !s.app.embyConfigured() {
		return 0, nil
	}

	sessions, err := s.app.embySessionsSnapshot(ctx, false)
	if err != nil {
		return 0, nil
	}

	return countEmbyPlayingSessions(sessions), nil
}

func (a *App) emby() *embyService {
	return &embyService{app: a}
}
