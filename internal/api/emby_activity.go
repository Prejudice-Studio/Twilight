package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

func (a *App) handleEmbyActivityLogs(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.embyConfigured() {
		failWithCode(w, http.StatusBadGateway, ErrEmbyNotConfigured, "Emby 未配置")
		return
	}
	limit := clamp(queryInt(r, "limit", 50), 1, 200)
	refresh := r.URL.Query().Get("refresh") == "1"

	if refresh {
		count, err := a.fetchAndStoreEmbyActivityLogs(r.Context())
		if err != nil {
			zap.L().Warn("fetch emby activity logs failed", zap.Error(err))
		} else {
			zap.L().Info("fetched emby activity logs", zap.Int("count", count))
		}
	}

	logs := a.store().ListEmbyActivityLogs(0, limit)
	ok(w, "OK", map[string]any{"entries": logs, "total": len(logs)})
}

func (a *App) fetchAndStoreEmbyActivityLogs(ctx context.Context) (int, error) {
	type logEntry struct {
		Items []map[string]any `json:"Items"`
	}
	var resp logEntry
	if err := a.embyGet(ctx, "/System/ActivityLog/Entries?startIndex=0&limit=100&hasUserId=true", &resp); err != nil {
		return 0, fmt.Errorf("emby activity log fetch: %w", err)
	}
	entries := make([]store.EmbyActivityLog, 0, len(resp.Items))
	for _, item := range resp.Items {
		entries = append(entries, store.EmbyActivityLog{
			EmbyLogID: int64(numeric(item["Id"])),
			Type:      asString(item["Type"]),
			Name:      asString(item["Name"]),
			UserID:    asString(item["UserId"]),
			UserName:  asString(item["UserName"]),
			Overview:  asString(item["ShortOverview"]),
			Date:      a.parseEmbyDate(asString(item["Date"])),
		})
	}
	return a.store().SyncEmbyActivityLogs(entries)
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
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}

func (a *App) handleEmbyPlaybackStats(w http.ResponseWriter, r *http.Request, params Params) {
	if !a.cfg().EmbyPlaybackStatsEnabled {
		failWithCode(w, http.StatusForbidden, ErrForbidden, "播放统计功能未启用")
		return
	}
	if !a.embyConfigured() {
		failWithCode(w, http.StatusBadGateway, ErrEmbyNotConfigured, "Emby 未配置")
		return
	}
	uid := int64(0)
	if uidStr, ok := params["uid"]; ok {
		uid, _ = strconv.ParseInt(uidStr, 10, 64)
	}
	days := queryInt(r, "days", 30)
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()

	logs := a.store().ListEmbyActivityLogs(uid, 5000)
	type userStats struct {
		UID      int64  `json:"uid"`
		Username string `json:"username"`
		Plays    int    `json:"plays"`
		Duration int64  `json:"duration"`
	}
	userMap := map[int64]*userStats{}
	playCount := map[string]int{}
	totalPlays := 0

	for _, log := range logs {
		if log.Date < since { continue }
		if log.Type != "VideoPlayback" && log.Type != "VideoPlaybackComplete" { continue }
		totalPlays++
		playCount[log.Name]++

		if log.UserID != "" {
			u, ok := a.store().FindUserByEmbyID(log.UserID)
			if ok {
				if userMap[u.UID] == nil {
					userMap[u.UID] = &userStats{UID: u.UID, Username: u.Username}
				}
				userMap[u.UID].Plays++
			}
		}
	}

	rankings := make([]userStats, 0, len(userMap))
	for _, v := range userMap {
		rankings = append(rankings, *v)
	}
	sort.Slice(rankings, func(i, j int) bool { return rankings[i].Plays > rankings[j].Plays })

	ok(w, "OK", map[string]any{
		"total_plays":   totalPlays,
		"unique_items":  len(playCount),
		"days":          days,
		"user_rankings": rankings,
	})
}
