package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

const embyActivityAutoRefreshInterval = 2 * time.Minute

func (a *App) handleEmbyActivityLogs(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.embyConfigured() {
		failWithCode(w, http.StatusBadGateway, ErrEmbyNotConfigured, "Emby not configured")
		return
	}
	limit := clamp(queryInt(r, "limit", 50), 1, 200)
	refresh := r.URL.Query().Get("refresh") == "1"
	autoRefresh := r.URL.Query().Get("auto") != "0"
	refreshed := false
	newEntries := 0

	if refresh || autoRefresh {
		count, didRefresh, err := a.refreshEmbyActivityLogs(r.Context(), refresh)
		refreshed = didRefresh
		newEntries = count
		if err != nil {
			zap.L().Warn("fetch emby activity logs failed", zap.Error(err))
		} else if didRefresh {
			zap.L().Info("fetched emby activity logs", zap.Int("count", count))
		}
	}

	logs := a.store().ListEmbyActivityLogs(0, limit)
	ok(w, "OK", map[string]any{"entries": logs, "total": len(logs), "refreshed": refreshed, "new_entries": newEntries})
}

func (a *App) refreshEmbyActivityLogs(ctx context.Context, force bool) (int, bool, error) {
	if !a.embyConfigured() {
		return 0, false, nil
	}
	now := time.Now()
	if !force {
		a.embyActivityMu.Lock()
		if now.Before(a.embyActivityNextAuto) {
			a.embyActivityMu.Unlock()
			return 0, false, nil
		}
		a.embyActivityNextAuto = now.Add(embyActivityAutoRefreshInterval)
		a.embyActivityMu.Unlock()
	}

	count, err := a.fetchAndStoreEmbyActivityLogs(ctx)
	if err != nil && !force {
		a.embyActivityMu.Lock()
		a.embyActivityNextAuto = now.Add(30 * time.Second)
		a.embyActivityMu.Unlock()
	}
	return count, true, err
}

func (a *App) fetchAndStoreEmbyActivityLogs(ctx context.Context) (int, error) {
	type logEntry struct {
		Items []map[string]any `json:"Items"`
	}
	var resp logEntry
	if err := a.embyGet(ctx, "/System/ActivityLog/Entries?startIndex=0&limit=500&hasUserId=true", &resp); err != nil {
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
		failWithCode(w, http.StatusForbidden, ErrForbidden, "playback stats disabled")
		return
	}
	if !a.embyConfigured() {
		failWithCode(w, http.StatusBadGateway, ErrEmbyNotConfigured, "Emby not configured")
		return
	}

	caller := current(r).User
	isAdmin := caller.Role == store.RoleAdmin
	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	uid := caller.UID
	if strings.HasPrefix(r.URL.Path, "/api/v1/admin/") {
		scope = firstNonEmpty(scope, "global")
		uid = 0
	}
	if uidStr, ok := params["uid"]; ok {
		parsed, err := strconv.ParseInt(uidStr, 10, 64)
		if err != nil || parsed <= 0 {
			failWithCode(w, http.StatusBadRequest, ErrBadRequest, "invalid uid")
			return
		}
		uid = parsed
		scope = "user"
	}
	if queryUID := strings.TrimSpace(r.URL.Query().Get("uid")); queryUID != "" {
		parsed, err := strconv.ParseInt(queryUID, 10, 64)
		if err != nil || parsed <= 0 {
			failWithCode(w, http.StatusBadRequest, ErrBadRequest, "invalid uid")
			return
		}
		uid = parsed
		scope = "user"
	}

	switch scope {
	case "", "self", "me":
		uid = caller.UID
		scope = "self"
	case "global", "all":
		if !isAdmin {
			failWithCode(w, http.StatusForbidden, ErrWatchStatsForbidden, "cannot view global playback stats")
			return
		}
		uid = 0
		scope = "global"
	case "user":
		if uid == 0 {
			uid = caller.UID
		}
	default:
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "invalid scope")
		return
	}
	if uid != caller.UID && !isAdmin {
		failWithCode(w, http.StatusForbidden, ErrWatchStatsForbidden, "cannot view another user's playback stats")
		return
	}

	days := clamp(queryInt(r, "days", 30), 1, 365)
	now := time.Now()
	since := now.Add(-time.Duration(days) * 24 * time.Hour).Unix()
	periodLabel := fmt.Sprintf("%d_days", days)
	if r.URL.Query().Get("today") == "1" {
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		since = startOfDay.Unix()
		days = 1
		periodLabel = "today"
	}
	itemLimit := clamp(queryInt(r, "limit", 20), 1, 100)
	sortBy := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	if sortBy == "" {
		sortBy = "plays"
	}
	if r.URL.Query().Get("refresh") == "1" {
		_, _, _ = a.refreshEmbyActivityLogs(r.Context(), true)
	} else {
		_, _, _ = a.refreshEmbyActivityLogs(r.Context(), false)
	}

	logs := a.store().ListEmbyActivityLogs(uid, 20000)
	type userStats struct {
		UID      int64  `json:"uid"`
		Username string `json:"username"`
		Plays    int    `json:"plays"`
		Duration int64  `json:"duration"`
	}
	userMap := map[int64]*userStats{}
	playCount := map[string]int{}
	dailyMap := map[string]int{}
	totalPlays := 0

	for _, log := range logs {
		if log.Date < since || !isPlaybackActivity(log.Type) {
			continue
		}
		totalPlays++
		playCount[log.Name]++

		dateKey := time.Unix(log.Date, 0).Format("2006-01-02")
		dailyMap[dateKey]++

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
	if len(rankings) > itemLimit {
		rankings = rankings[:itemLimit]
	}

	daily := make([]map[string]any, 0, len(dailyMap))
	for d, c := range dailyMap {
		daily = append(daily, map[string]any{"date": d, "plays": c})
	}
	sort.Slice(daily, func(i, j int) bool { return asString(daily[i]["date"]) < asString(daily[j]["date"]) })

	items := make([]map[string]any, 0, len(playCount))
	for name, count := range playCount {
		items = append(items, map[string]any{"name": name, "plays": count})
	}
	sort.Slice(items, func(i, j int) bool {
		switch sortBy {
		case "name":
			return asString(items[i]["name"]) < asString(items[j]["name"])
		default:
			return numeric(items[i]["plays"]) > numeric(items[j]["plays"])
		}
	})
	topItems := items
	if len(topItems) > itemLimit {
		topItems = topItems[:itemLimit]
	}

	ok(w, "OK", map[string]any{
		"scope":           scope,
		"uid":             uid,
		"period":          periodLabel,
		"total_plays":     totalPlays,
		"total_duration":  int64(0),
		"unique_items":    len(playCount),
		"days":            days,
		"limit":           itemLimit,
		"can_view_global": isAdmin,
		"user_rankings":   rankings,
		"daily_breakdown": daily,
		"top_items":       topItems,
	})
}

func isPlaybackActivity(activityType string) bool {
	switch strings.ToLower(strings.TrimSpace(activityType)) {
	case "videoplayback", "videoplaybackcomplete":
		return true
	default:
		return false
	}
}
