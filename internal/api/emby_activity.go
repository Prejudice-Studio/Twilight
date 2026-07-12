package api

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

const (
	embyActivityFetchPageSize          = 500
	embyActivityFetchLimit             = 20000
	maxActivityPlaybackDurationSeconds = int64(12 * time.Hour / time.Second)
)

type embyActivityPlaybackEvent struct {
	UserKey  string
	UserID   string
	UserName string
	ItemID   string
	ItemName string
	PlayedAt int64
	Duration int64
}

type embyActivityPlaybackStart struct {
	UserKey  string
	UserID   string
	UserName string
	ItemID   string
	ItemName string
	Started  int64
}

func (a *App) handleEmbyActivityLogs(w http.ResponseWriter, r *http.Request, _ Params) {
	limit := clamp(queryInt(r, "limit", 100), 1, 200)
	refreshed := false
	newEntries := 0

	if r.URL.Query().Get("refresh") == "1" {
		if !a.embyConfigured() {
			failWithCode(w, http.StatusBadGateway, ErrEmbyNotConfigured, "Emby not configured")
			return
		}
		sinceHours := clamp(queryInt(r, "since_hours", 24), 1, 720)
		count, err := a.fetchAndStoreEmbyActivityLogsSince(r.Context(), time.Now().Add(-time.Duration(sinceHours)*time.Hour))
		if err != nil {
			zap.L().Warn("fetch emby activity logs failed", zap.Error(err))
			failWithCode(w, http.StatusBadGateway, ErrInternal, "failed to fetch Emby activity logs")
			return
		}
		refreshed = true
		newEntries = count
	}

	logs := a.store().ListEmbyActivityLogs(0, limit)
	ok(w, "OK", map[string]any{
		"entries":     logs,
		"total":       len(logs),
		"refreshed":   refreshed,
		"new_entries": newEntries,
	})
}

// fetchAndStoreEmbyActivityLogs is the scheduler's incremental entry point.
func (a *App) fetchAndStoreEmbyActivityLogs(ctx context.Context) (int, error) {
	return a.fetchAndStoreEmbyActivityLogsSince(ctx, time.Now().Add(-24*time.Hour))
}

func (a *App) fetchAndStoreEmbyActivityLogsSince(ctx context.Context, since time.Time) (int, error) {
	type logEntry struct {
		Items []struct {
			ID            any    `json:"Id"`
			Type          string `json:"Type"`
			Name          string `json:"Name"`
			ItemID        string `json:"ItemId"`
			UserID        string `json:"UserId"`
			UserName      string `json:"UserName"`
			ShortOverview string `json:"ShortOverview"`
			Date          string `json:"Date"`
		} `json:"Items"`
		TotalRecordCount int `json:"TotalRecordCount"`
	}

	entries := make([]store.EmbyActivityLog, 0, embyActivityFetchPageSize)
	for startIndex := 0; startIndex < embyActivityFetchLimit; startIndex += embyActivityFetchPageSize {
		var resp logEntry
		path := fmt.Sprintf("/System/ActivityLog/Entries?StartIndex=%d&Limit=%d&HasUserId=true", startIndex, embyActivityFetchPageSize)
		if err := a.embyGet(ctx, path, &resp); err != nil {
			return 0, fmt.Errorf("emby activity log fetch: %w", err)
		}
		if len(resp.Items) == 0 {
			break
		}
		reachedWindowStart := false
		for _, item := range resp.Items {
			parsedDate := a.parseEmbyDate(item.Date)
			embyLogID := int64(numeric(item.ID))
			if embyLogID <= 0 {
				continue
			}
			itemID := firstNonEmpty(item.ItemID, embyActivityDerivedItemID(item.Type, item.Name))
			entries = append(entries, store.EmbyActivityLog{
				EmbyLogID: embyLogID,
				Type:      item.Type,
				Name:      item.Name,
				ItemID:    itemID,
				UserID:    item.UserID,
				UserName:  item.UserName,
				Overview:  item.ShortOverview,
				Date:      parsedDate,
			})
			if !since.IsZero() && parsedDate > 0 && parsedDate < since.Unix() {
				reachedWindowStart = true
			}
		}
		if reachedWindowStart || len(entries) >= embyActivityFetchLimit || startIndex+len(resp.Items) >= resp.TotalRecordCount {
			break
		}
	}
	if len(entries) > embyActivityFetchLimit {
		entries = entries[:embyActivityFetchLimit]
	}
	added, err := a.store().SyncEmbyActivityLogs(entries)
	if err != nil {
		return added, err
	}
	if persisted, err := a.persistEmbyPlaybackRecordsFromActivity(ctx, since); err != nil {
		zap.L().Warn("failed to persist Emby playback records from activity logs", zap.Error(err))
	} else if persisted > 0 {
		zap.L().Info("persisted Emby playback records", zap.Int("records", persisted))
	}
	return added, nil
}

func (a *App) parseEmbyDate(s string) int64 {
	if s == "" {
		return 0
	}
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.0000000",
		"2006-01-02T15:04:05.0000000Z",
	}
	for _, format := range formats {
		if parsed, err := time.Parse(format, s); err == nil {
			return parsed.Unix()
		}
	}
	return 0
}

func (a *App) persistEmbyPlaybackRecordsFromActivity(ctx context.Context, since time.Time) (int, error) {
	until := time.Now().Add(time.Hour).Unix()
	sinceUnix := since.Add(-12 * time.Hour).Unix()
	if sinceUnix < 0 {
		sinceUnix = 0
	}
	logs := a.store().ListEmbyActivityLogs(0, embyActivityFetchLimit)
	events := embyActivityPlaybackEventsFromLogs(logs, sinceUnix, until)
	if len(events) == 0 {
		return 0, nil
	}
	usersByKey := embyActivityUsersByKey(a.store().ListUsers())
	itemIDs := make([]string, 0, len(events))
	for _, event := range events {
		if validEmbyItemID(event.ItemID) {
			itemIDs = append(itemIDs, event.ItemID)
		}
	}
	metadata := a.embyItemMetadata(ctx, itemIDs)
	inserted := 0
	for _, event := range events {
		user := usersByKey[normalizeEmbyActivityUserKey(event.UserID)]
		if user.UID == 0 {
			user = usersByKey[normalizeEmbyActivityUserKey(event.UserName)]
		}
		if user.UID == 0 {
			user = usersByKey[normalizeEmbyActivityUserKey(event.UserKey)]
		}
		if user.UID == 0 {
			continue
		}
		meta := metadata[event.ItemID]
		itemID := firstNonEmpty(meta.ID, event.ItemID)
		title := firstNonEmpty(meta.Name, event.ItemName, event.ItemID)
		seriesName := strings.TrimSpace(meta.SeriesName)
		mediaType := strings.ToLower(strings.TrimSpace(meta.Type))
		if mediaType == "episode" || mediaType == "series" {
			title = firstNonEmpty(meta.Name, event.ItemID)
			seriesName = firstNonEmpty(meta.SeriesName, meta.Name)
			mediaType = "episode"
		} else if mediaType == "" {
			mediaType = "unknown"
		}
		ok, err := a.store().AddPlaybackRecordIdempotent(store.PlaybackRecord{
			UID:        user.UID,
			ItemID:     itemID,
			Title:      title,
			SeriesName: seriesName,
			MediaType:  mediaType,
			Duration:   clampActivityPlaybackDuration(event.Duration),
			PlayedAt:   event.PlayedAt,
		})
		if err != nil {
			return inserted, err
		}
		if ok {
			inserted++
		}
	}
	return inserted, nil
}

func embyActivityPlaybackEventsFromLogs(logs []store.EmbyActivityLog, since, until int64) []embyActivityPlaybackEvent {
	sortedLogs := append([]store.EmbyActivityLog(nil), logs...)
	sort.Slice(sortedLogs, func(i, j int) bool {
		if sortedLogs[i].Date != sortedLogs[j].Date {
			return sortedLogs[i].Date < sortedLogs[j].Date
		}
		return sortedLogs[i].EmbyLogID < sortedLogs[j].EmbyLogID
	})
	starts := map[string][]embyActivityPlaybackStart{}
	events := make([]embyActivityPlaybackEvent, 0)
	for _, log := range sortedLogs {
		if log.Date <= 0 || strings.TrimSpace(log.ItemID) == "" {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(log.Type))
		userKey := firstNonEmpty(strings.TrimSpace(log.UserID), strings.TrimSpace(log.UserName))
		if userKey == "" {
			continue
		}
		itemName := embyActivityPlaybackTitle(log.Type, log.Name)
		key := normalizeEmbyActivityUserKey(userKey) + "|" + strings.TrimSpace(log.ItemID)
		switch kind {
		case "playback.start", "videoplayback":
			starts[key] = append(starts[key], embyActivityPlaybackStart{UserKey: userKey, UserID: log.UserID, UserName: log.UserName, ItemID: log.ItemID, ItemName: itemName, Started: log.Date})
		case "playback.stop", "videoplaybackcomplete", "videoplaybackstopped":
			pending := starts[key]
			if len(pending) == 0 || log.Date < since || log.Date >= until {
				continue
			}
			start := pending[len(pending)-1]
			starts[key] = pending[:len(pending)-1]
			startedAt := start.Started
			if startedAt < since {
				startedAt = since
			}
			if log.Date <= startedAt {
				continue
			}
			events = append(events, embyActivityPlaybackEvent{
				UserKey:  firstNonEmpty(start.UserKey, userKey),
				UserID:   firstNonEmpty(start.UserID, log.UserID),
				UserName: firstNonEmpty(start.UserName, log.UserName),
				ItemID:   log.ItemID,
				ItemName: firstNonEmpty(start.ItemName, itemName),
				PlayedAt: log.Date,
				Duration: clampActivityPlaybackDuration(log.Date - startedAt),
			})
		}
	}
	return events
}

func clampActivityPlaybackDuration(seconds int64) int64 {
	if seconds < 0 {
		return 0
	}
	if seconds > maxActivityPlaybackDurationSeconds {
		return maxActivityPlaybackDurationSeconds
	}
	return seconds
}

func embyActivityUsersByKey(users []store.User) map[string]store.User {
	out := make(map[string]store.User, len(users)*3)
	for _, user := range users {
		for _, key := range []string{user.EmbyID, user.EmbyUsername, user.Username} {
			if normalized := normalizeEmbyActivityUserKey(key); normalized != "" {
				out[normalized] = user
			}
		}
	}
	return out
}

func normalizeEmbyActivityUserKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func embyActivityDerivedItemID(kind string, name string) string {
	title := embyActivityPlaybackTitle(kind, name)
	if title == "" {
		return ""
	}
	sum := sha1.Sum([]byte(strings.ToLower(title)))
	return "activity:" + hex.EncodeToString(sum[:8])
}

func embyActivityPlaybackTitle(kind string, name string) string {
	if !embyActivityIsPlaybackKind(kind) {
		return ""
	}
	text := strings.TrimSpace(name)
	if text == "" {
		return ""
	}
	markers := []string{
		"开始播放 ",
		"停止播放 ",
		"正在播放 ",
		"started playing ",
		"stopped playing ",
		"is playing ",
	}
	lower := strings.ToLower(text)
	for _, marker := range markers {
		search := strings.ToLower(marker)
		if idx := strings.LastIndex(lower, search); idx >= 0 {
			return strings.TrimSpace(text[idx+len(marker):])
		}
	}
	return text
}

func embyActivityIsPlaybackKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "playback.start", "playback.stop", "videoplayback", "videoplaybackcomplete", "videoplaybackstopped":
		return true
	default:
		return false
	}
}
