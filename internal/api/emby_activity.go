package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

const (
	embyActivityAutoRefreshInterval = 2 * time.Minute
	embyPlaybackStatsCacheTTL       = 30 * time.Second
	embyActivityFetchPageSize       = 500
	embyActivityFetchLimit          = 20000
	maxPlaybackDurationSeconds      = int64(12 * time.Hour / time.Second)
	maxPlaybackStatsAdminDays       = 1825
	maxEmbyPlaybackImageBytes       = int64(10 << 20)
)

type embyPlaybackStatsCacheEntry struct {
	Until time.Time
	Value embyPlaybackStatsResponse
}

type embyPlaybackStatsResponse struct {
	Scope          string                     `json:"scope"`
	UID            int64                      `json:"uid,omitempty"`
	Period         string                     `json:"period"`
	From           string                     `json:"from"`
	To             string                     `json:"to"`
	GroupBy        string                     `json:"group_by"`
	MediaType      string                     `json:"media_type"`
	Query          string                     `json:"query,omitempty"`
	MinDuration    int64                      `json:"min_duration"`
	TotalPlays     int                        `json:"total_plays"`
	TotalDuration  int64                      `json:"total_duration"`
	UniqueItems    int                        `json:"unique_items"`
	Days           int                        `json:"days"`
	Limit          int                        `json:"limit"`
	CanViewGlobal  bool                       `json:"can_view_global"`
	CanViewOthers  bool                       `json:"can_view_others"`
	Source         string                     `json:"source"`
	Policy         embyPlaybackViewerPolicy   `json:"policy"`
	UserRankings   []embyPlaybackUserRanking  `json:"user_rankings"`
	DailyBreakdown []embyPlaybackDailySummary `json:"daily_breakdown"`
	TopItems       []embyPlaybackItemRanking  `json:"top_items"`
}

type embyPlaybackViewerPolicy struct {
	UserEnabled      bool     `json:"user_enabled"`
	SelfOnly         bool     `json:"self_only"`
	CanViewGlobal    bool     `json:"can_view_global"`
	CanViewOthers    bool     `json:"can_view_others"`
	ShowUserRankings bool     `json:"show_user_rankings"`
	ShowItemRankings bool     `json:"show_item_rankings"`
	ShowDailySummary bool     `json:"show_daily_summary"`
	AllowedPeriods   []string `json:"allowed_periods"`
	AllowedGroupings []string `json:"allowed_groupings"`
	MaxDays          int      `json:"max_days"`
}

type embyPlaybackStatsQuery struct {
	Since       time.Time
	Until       time.Time
	From        string
	To          string
	Days        int
	Period      string
	PeriodKind  string
	GroupBy     string
	Limit       int
	SortBy      string
	MediaType   string
	Search      string
	MinDuration int64
	Force       bool
}

type embyPlaybackUserRanking struct {
	UID      int64  `json:"uid"`
	Username string `json:"username"`
	Plays    int    `json:"plays"`
	Duration int64  `json:"duration"`
}

type embyPlaybackDailySummary struct {
	Date     string `json:"date"`
	Plays    int    `json:"plays"`
	Duration int64  `json:"duration"`
}

type embyPlaybackItemRanking struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	MediaType string `json:"media_type,omitempty"`
	ImageURL  string `json:"image_url,omitempty"`
	Plays     int    `json:"plays"`
	Duration  int64  `json:"duration"`
}

type embyPlaybackEvent struct {
	UserKey  string
	UserID   string
	UserName string
	ItemID   string
	PlayedAt int64
	Duration int64
}

type embyPlaybackStart struct {
	UserKey  string
	UserID   string
	UserName string
	ItemID   string
	Started  int64
}

type embyPlaybackItemMetadata struct {
	ID         string `json:"Id"`
	Name       string `json:"Name"`
	Type       string `json:"Type"`
	SeriesID   string `json:"SeriesId"`
	SeriesName string `json:"SeriesName"`
}

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
		count, didRefresh, err := a.refreshEmbyActivityLogs(r.Context(), refresh, time.Now().Add(-24*time.Hour))
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

func (a *App) refreshEmbyActivityLogs(ctx context.Context, force bool, since time.Time) (int, bool, error) {
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

	count, err := a.fetchAndStoreEmbyActivityLogsSince(ctx, since)
	if err != nil && !force {
		a.embyActivityMu.Lock()
		a.embyActivityNextAuto = now.Add(30 * time.Second)
		a.embyActivityMu.Unlock()
	}
	if err == nil {
		a.clearEmbyPlaybackStatsCache()
	}
	return count, true, err
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
			entries = append(entries, store.EmbyActivityLog{
				EmbyLogID: embyLogID,
				Type:      item.Type,
				Name:      item.Name,
				ItemID:    item.ItemID,
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
		a.clearEmbyPlaybackStatsCache()
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
	events := embyPlaybackEventsFromLogs(logs, sinceUnix, until)
	if len(events) == 0 {
		return 0, nil
	}
	usersByKey := playbackStatsUsersByKey(a.store().ListUsers())
	metadata := a.embyPlaybackMetadata(ctx, events)
	inserted := 0
	for _, event := range events {
		user := usersByKey[normalizePlaybackUserKey(event.UserID)]
		if user.UID == 0 {
			user = usersByKey[normalizePlaybackUserKey(event.UserName)]
		}
		if user.UID == 0 {
			user = usersByKey[normalizePlaybackUserKey(event.UserKey)]
		}
		if user.UID == 0 {
			continue
		}
		meta := metadata[event.ItemID]
		itemID := firstNonEmpty(meta.ID, event.ItemID)
		title := firstNonEmpty(meta.Name, event.ItemID)
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
			Duration:   clampPlaybackDuration(event.Duration),
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

func (a *App) handleEmbyPlaybackStats(w http.ResponseWriter, r *http.Request, params Params) {
	cfg := a.cfg()
	if !cfg.EmbyPlaybackStatsEnabled {
		failWithCode(w, http.StatusForbidden, ErrForbidden, "playback stats disabled")
		return
	}
	if !a.embyConfigured() {
		failWithCode(w, http.StatusBadGateway, ErrEmbyNotConfigured, "Emby not configured")
		return
	}

	caller := current(r).User
	isAdmin := caller.Role == store.RoleAdmin
	policy := playbackStatsViewerPolicy(*cfg, isAdmin)
	if !policy.UserEnabled {
		failWithCode(w, http.StatusForbidden, ErrWatchStatsForbidden, "playback stats are not available to users")
		return
	}
	scope, uid, valid := playbackStatsScope(r, params, caller)
	if !valid {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "invalid playback stats scope")
		return
	}
	if uid != caller.UID && uid != 0 && !policy.CanViewOthers {
		failWithCode(w, http.StatusForbidden, ErrWatchStatsForbidden, "cannot view another user's playback stats")
		return
	}
	if scope == "global" && !policy.CanViewGlobal {
		failWithCode(w, http.StatusForbidden, ErrWatchStatsForbidden, "cannot view global playback stats")
		return
	}

	query, err := parseEmbyPlaybackStatsQuery(r, policy)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, err.Error())
		return
	}
	if !playbackStatsPeriodAllowed(policy, query.PeriodKind) || !playbackStatsGroupingAllowed(policy, query.GroupBy) {
		failWithCode(w, http.StatusForbidden, ErrWatchStatsForbidden, "requested playback stats time dimension is disabled")
		return
	}
	_, _, _ = a.refreshEmbyActivityLogs(r.Context(), query.Force, query.Since.Add(-12*time.Hour))

	cacheKey := fmt.Sprintf(
		"%s:%d:%d:%d:%s:%d:%s:%s:%s:%d:%t:%t:%t",
		scope, uid, query.Since.Unix(), query.Until.Unix(), query.GroupBy, query.Limit, query.SortBy,
		query.MediaType, query.Search, query.MinDuration, policy.ShowUserRankings, policy.ShowItemRankings, policy.ShowDailySummary,
	)
	if !query.Force {
		if cached, found := a.cachedEmbyPlaybackStats(cacheKey); found {
			ok(w, "OK", cached)
			return
		}
	}

	result := a.buildEmbyPlaybackStats(r.Context(), scope, uid, query, policy)
	a.cacheEmbyPlaybackStats(cacheKey, result)
	ok(w, "OK", result)
}

func playbackStatsViewerPolicy(cfg config.Config, isAdmin bool) embyPlaybackViewerPolicy {
	if isAdmin {
		return embyPlaybackViewerPolicy{
			UserEnabled:      true,
			CanViewGlobal:    true,
			CanViewOthers:    true,
			ShowUserRankings: true,
			ShowItemRankings: true,
			ShowDailySummary: true,
			AllowedPeriods:   []string{"day", "week", "month", "custom"},
			AllowedGroupings: []string{"day", "week", "month"},
			MaxDays:          maxPlaybackStatsAdminDays,
		}
	}
	maxDays := clamp(cfg.EmbyPlaybackStatsUserMaxDays, 1, 365)
	periods := make([]string, 0, 4)
	groupings := make([]string, 0, 3)
	if cfg.EmbyPlaybackStatsUserDay {
		periods = append(periods, "day")
		groupings = append(groupings, "day")
	}
	if cfg.EmbyPlaybackStatsUserWeek {
		periods = append(periods, "week")
		groupings = append(groupings, "week")
	}
	if cfg.EmbyPlaybackStatsUserMonth {
		periods = append(periods, "month")
		groupings = append(groupings, "month")
	}
	if cfg.EmbyPlaybackStatsUserCustom {
		periods = append(periods, "custom")
	}
	return embyPlaybackViewerPolicy{
		UserEnabled:      cfg.EmbyPlaybackStatsUserEnabled,
		SelfOnly:         cfg.EmbyPlaybackStatsUserSelfOnly,
		CanViewGlobal:    !cfg.EmbyPlaybackStatsUserSelfOnly,
		CanViewOthers:    !cfg.EmbyPlaybackStatsUserSelfOnly,
		ShowUserRankings: cfg.EmbyPlaybackStatsUserRankings,
		ShowItemRankings: cfg.EmbyPlaybackStatsItemRankings,
		ShowDailySummary: cfg.EmbyPlaybackStatsDailySummary,
		AllowedPeriods:   periods,
		AllowedGroupings: groupings,
		MaxDays:          maxDays,
	}
}

func parseEmbyPlaybackStatsQuery(r *http.Request, policy embyPlaybackViewerPolicy) (embyPlaybackStatsQuery, error) {
	now := time.Now()
	location := now.Location()
	query := embyPlaybackStatsQuery{
		Until:       now,
		GroupBy:     "day",
		Limit:       clamp(queryInt(r, "limit", 20), 1, 100),
		MediaType:   strings.ToLower(strings.TrimSpace(r.URL.Query().Get("media_type"))),
		Search:      strings.TrimSpace(r.URL.Query().Get("query")),
		MinDuration: int64(clamp(queryInt(r, "min_duration", 0), 0, int(maxPlaybackDurationSeconds))),
		Force:       r.URL.Query().Get("refresh") == "1",
	}
	if query.MediaType == "" {
		query.MediaType = "all"
	}
	switch query.MediaType {
	case "all", "movie", "series", "other":
	default:
		return embyPlaybackStatsQuery{}, fmt.Errorf("invalid media_type")
	}
	if runes := []rune(query.Search); len(runes) > 100 {
		query.Search = string(runes[:100])
	}
	query.SortBy = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	switch query.SortBy {
	case "name", "duration", "plays":
	default:
		query.SortBy = "plays"
	}
	if groupBy := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("group_by"))); groupBy != "" {
		query.GroupBy = groupBy
	}
	if query.GroupBy != "day" && query.GroupBy != "week" && query.GroupBy != "month" {
		return embyPlaybackStatsQuery{}, fmt.Errorf("invalid group_by")
	}

	fromText := strings.TrimSpace(r.URL.Query().Get("from"))
	toText := strings.TrimSpace(r.URL.Query().Get("to"))
	rawPeriod := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("period")))
	if fromText != "" || toText != "" {
		if fromText == "" || toText == "" {
			return embyPlaybackStatsQuery{}, fmt.Errorf("from and to must be provided together")
		}
		from, err := time.ParseInLocation("2006-01-02", fromText, location)
		if err != nil {
			return embyPlaybackStatsQuery{}, fmt.Errorf("invalid from date")
		}
		to, err := time.ParseInLocation("2006-01-02", toText, location)
		if err != nil {
			return embyPlaybackStatsQuery{}, fmt.Errorf("invalid to date")
		}
		if to.Before(from) {
			return embyPlaybackStatsQuery{}, fmt.Errorf("to date must not be before from date")
		}
		query.Since = from
		query.Until = to.AddDate(0, 0, 1)
		if query.Until.After(now) {
			query.Until = now
		}
		if !query.Until.After(query.Since) {
			return embyPlaybackStatsQuery{}, fmt.Errorf("playback stats range is in the future")
		}
		query.Period = "custom"
		query.PeriodKind = "custom"
	} else {
		if r.URL.Query().Get("today") == "1" {
			rawPeriod = "today"
		}
		switch rawPeriod {
		case "today", "day":
			query.Since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
			query.Period = "today"
			query.PeriodKind = "day"
		case "week":
			weekday := (int(now.Weekday()) + 6) % 7
			startToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
			query.Since = startToday.AddDate(0, 0, -weekday)
			query.Period = "week"
			query.PeriodKind = "week"
		case "month":
			query.Since = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location)
			query.Period = "month"
			query.PeriodKind = "month"
		case "", "custom", "days":
			days := clamp(queryInt(r, "days", 30), 1, policy.MaxDays)
			query.Since = now.Add(-time.Duration(days) * 24 * time.Hour)
			query.Period = fmt.Sprintf("%d_days", days)
			query.PeriodKind = "custom"
		default:
			return embyPlaybackStatsQuery{}, fmt.Errorf("invalid period")
		}
	}

	query.Days = playbackRangeDays(query.Since, query.Until)
	if query.Days > policy.MaxDays {
		return embyPlaybackStatsQuery{}, fmt.Errorf("playback stats range exceeds %d days", policy.MaxDays)
	}
	query.From = query.Since.In(location).Format("2006-01-02")
	query.To = query.Until.Add(-time.Nanosecond).In(location).Format("2006-01-02")
	return query, nil
}

func playbackRangeDays(since, until time.Time) int {
	if !until.After(since) {
		return 1
	}
	duration := until.Sub(since)
	days := int(duration / (24 * time.Hour))
	if duration%(24*time.Hour) != 0 {
		days++
	}
	return max(days, 1)
}

func playbackStatsPeriodAllowed(policy embyPlaybackViewerPolicy, period string) bool {
	return stringInSlice(policy.AllowedPeriods, period)
}

func playbackStatsGroupingAllowed(policy embyPlaybackViewerPolicy, grouping string) bool {
	return stringInSlice(policy.AllowedGroupings, grouping)
}

func stringInSlice(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func playbackStatsScope(r *http.Request, params Params, caller store.User) (string, int64, bool) {
	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	uid := caller.UID
	if strings.HasPrefix(r.URL.Path, "/api/v1/admin/") {
		scope = firstNonEmpty(scope, "global")
		uid = 0
	}
	uidText := strings.TrimSpace(r.URL.Query().Get("uid"))
	if value, ok := params["uid"]; ok {
		uidText = value
	}
	if uidText != "" {
		parsed, err := strconv.ParseInt(uidText, 10, 64)
		if err != nil || parsed <= 0 {
			return "", 0, false
		}
		uid = parsed
		scope = "user"
	}
	switch scope {
	case "", "self", "me":
		return "self", caller.UID, true
	case "global", "all":
		return "global", 0, true
	case "user":
		if uid <= 0 {
			uid = caller.UID
		}
		return "user", uid, true
	default:
		return "", 0, false
	}
}

func (a *App) buildEmbyPlaybackStats(ctx context.Context, scope string, uid int64, query embyPlaybackStatsQuery, policy embyPlaybackViewerPolicy) embyPlaybackStatsResponse {
	if query.Limit >= 0 {
		return a.buildEmbyPlaybackStatsFromRecords(ctx, scope, uid, query, policy)
	}
	logs := a.store().ListEmbyActivityLogs(0, embyActivityFetchLimit)
	events := embyPlaybackEventsFromLogs(logs, query.Since.Unix(), query.Until.Unix())
	usersByKey := playbackStatsUsersByKey(a.store().ListUsers())
	filteredEvents := make([]embyPlaybackEvent, 0, len(events))
	for _, event := range events {
		localUser := usersByKey[normalizePlaybackUserKey(event.UserID)]
		if localUser.UID == 0 {
			localUser = usersByKey[normalizePlaybackUserKey(event.UserName)]
		}
		if scope == "global" || localUser.UID == uid {
			filteredEvents = append(filteredEvents, event)
		}
	}
	metadata := a.embyPlaybackMetadata(ctx, filteredEvents)

	userBuckets := map[string]*embyPlaybackUserRanking{}
	itemBuckets := map[string]*embyPlaybackItemRanking{}
	dailyBuckets := map[string]*embyPlaybackDailySummary{}
	uniqueItemKeys := map[string]struct{}{}
	totalPlays := 0
	totalDuration := int64(0)

	add := func(event embyPlaybackEvent, localUser store.User, itemKey, itemID, itemName, mediaType, imageID string) {
		if scope != "global" && localUser.UID != uid {
			return
		}
		duration := clampPlaybackDuration(event.Duration)
		if duration < query.MinDuration {
			return
		}
		mediaType = normalizePlaybackMediaType(mediaType)
		if query.MediaType != "all" && mediaType != query.MediaType {
			return
		}
		itemName = firstNonEmpty(itemName, itemID, "未知媒体")
		if query.Search != "" && !strings.Contains(strings.ToLower(itemName), strings.ToLower(query.Search)) {
			return
		}
		totalPlays++
		totalDuration += duration
		if itemKey == "" {
			itemKey = firstNonEmpty(itemID, itemName)
		}
		uniqueItemKeys[itemKey] = struct{}{}

		if policy.ShowUserRankings {
			userKey := normalizePlaybackUserKey(event.UserKey)
			if localUser.UID > 0 {
				userKey = fmt.Sprintf("uid:%d", localUser.UID)
			}
			bucket := userBuckets[userKey]
			if bucket == nil {
				bucket = &embyPlaybackUserRanking{
					UID:      localUser.UID,
					Username: firstNonEmpty(localUser.Username, event.UserName, event.UserKey, "未知用户"),
				}
				userBuckets[userKey] = bucket
			}
			bucket.Plays++
			bucket.Duration += duration
		}

		if policy.ShowItemRankings {
			item := itemBuckets[itemKey]
			if item == nil {
				item = &embyPlaybackItemRanking{
					ID:        itemID,
					Name:      itemName,
					MediaType: mediaType,
					ImageURL:  embyPlaybackImageURL(imageID),
				}
				itemBuckets[itemKey] = item
			}
			item.Plays++
			item.Duration += duration
		}

		if policy.ShowDailySummary {
			dateKey := playbackStatsBucketKey(time.Unix(event.PlayedAt, 0), query.GroupBy, query.Since.Location())
			daily := dailyBuckets[dateKey]
			if daily == nil {
				daily = &embyPlaybackDailySummary{Date: dateKey}
				dailyBuckets[dateKey] = daily
			}
			daily.Plays++
			daily.Duration += duration
		}
	}

	for _, event := range filteredEvents {
		localUser := usersByKey[normalizePlaybackUserKey(event.UserID)]
		if localUser.UID == 0 {
			localUser = usersByKey[normalizePlaybackUserKey(event.UserName)]
		}
		meta := metadata[event.ItemID]
		itemID := firstNonEmpty(meta.ID, event.ItemID)
		itemName := firstNonEmpty(meta.Name, event.ItemID)
		mediaType := strings.ToLower(strings.TrimSpace(meta.Type))
		itemKey := itemID
		imageID := itemID
		if mediaType == "episode" || mediaType == "series" {
			itemID = firstNonEmpty(meta.SeriesID, meta.ID, event.ItemID)
			itemKey = "series:" + itemID
			imageID = itemID
			itemName = firstNonEmpty(meta.SeriesName, meta.Name, event.ItemID)
			mediaType = "series"
		} else if mediaType == "movie" {
			itemID = firstNonEmpty(meta.ID, event.ItemID)
			itemKey = "movie:" + itemID
			imageID = itemID
		}
		add(event, localUser, itemKey, itemID, itemName, mediaType, imageID)
	}

	source := "emby_activity_log"
	if len(filteredEvents) == 0 {
		source = "local_playback_records"
		records := a.store().PlaybackRecords(0, query.Since.Unix(), 10000)
		fallbackEvents := make([]embyPlaybackEvent, 0, len(records))
		fallbackRecords := make([]store.PlaybackRecord, 0, len(records))
		for _, record := range records {
			if record.PlayedAt >= query.Until.Unix() {
				continue
			}
			localUser, _ := a.store().User(record.UID)
			if scope != "global" && localUser.UID != uid {
				continue
			}
			fallbackRecords = append(fallbackRecords, record)
			fallbackEvents = append(fallbackEvents, embyPlaybackEvent{UserKey: localUser.EmbyID, UserID: localUser.EmbyID, UserName: localUser.Username, ItemID: record.ItemID, PlayedAt: record.PlayedAt, Duration: record.Duration})
		}
		fallbackMetadata := a.embyPlaybackMetadata(ctx, fallbackEvents)
		for index, event := range fallbackEvents {
			record := fallbackRecords[index]
			localUser, _ := a.store().User(record.UID)
			meta := fallbackMetadata[record.ItemID]
			mediaType := firstNonEmpty(strings.ToLower(strings.TrimSpace(meta.Type)), strings.ToLower(strings.TrimSpace(record.MediaType)))
			itemID := firstNonEmpty(meta.ID, record.ItemID)
			itemKey := itemID
			imageID := itemID
			itemName := firstNonEmpty(meta.Name, record.Title, record.ItemID)
			if mediaType == "episode" || mediaType == "series" {
				itemID = firstNonEmpty(meta.SeriesID, meta.ID, record.ItemID)
				itemKey = "series:" + firstNonEmpty(itemID, record.SeriesName, record.Title)
				imageID = itemID
				itemName = firstNonEmpty(meta.SeriesName, record.SeriesName, meta.Name, record.Title, record.ItemID)
				mediaType = "series"
			} else if mediaType == "movie" {
				itemID = firstNonEmpty(meta.ID, record.ItemID)
				itemKey = "movie:" + firstNonEmpty(itemID, record.Title)
				imageID = itemID
			}
			add(event, localUser, itemKey, itemID, itemName, mediaType, imageID)
		}
	}

	userRankings := make([]embyPlaybackUserRanking, 0, len(userBuckets))
	for _, bucket := range userBuckets {
		userRankings = append(userRankings, *bucket)
	}
	sort.Slice(userRankings, func(i, j int) bool {
		switch query.SortBy {
		case "name":
			return strings.ToLower(userRankings[i].Username) < strings.ToLower(userRankings[j].Username)
		case "duration":
			if userRankings[i].Duration != userRankings[j].Duration {
				return userRankings[i].Duration > userRankings[j].Duration
			}
		default:
			if userRankings[i].Plays != userRankings[j].Plays {
				return userRankings[i].Plays > userRankings[j].Plays
			}
		}
		if userRankings[i].Duration != userRankings[j].Duration {
			return userRankings[i].Duration > userRankings[j].Duration
		}
		if userRankings[i].Plays != userRankings[j].Plays {
			return userRankings[i].Plays > userRankings[j].Plays
		}
		return strings.ToLower(userRankings[i].Username) < strings.ToLower(userRankings[j].Username)
	})
	if len(userRankings) > query.Limit {
		userRankings = userRankings[:query.Limit]
	}

	topItems := make([]embyPlaybackItemRanking, 0, len(itemBuckets))
	for _, bucket := range itemBuckets {
		topItems = append(topItems, *bucket)
	}
	sort.Slice(topItems, func(i, j int) bool {
		if query.SortBy == "name" {
			return strings.ToLower(topItems[i].Name) < strings.ToLower(topItems[j].Name)
		}
		if query.SortBy == "duration" && topItems[i].Duration != topItems[j].Duration {
			return topItems[i].Duration > topItems[j].Duration
		}
		if topItems[i].Plays != topItems[j].Plays {
			return topItems[i].Plays > topItems[j].Plays
		}
		if topItems[i].Duration != topItems[j].Duration {
			return topItems[i].Duration > topItems[j].Duration
		}
		return strings.ToLower(topItems[i].Name) < strings.ToLower(topItems[j].Name)
	})
	if len(topItems) > query.Limit {
		topItems = topItems[:query.Limit]
	}

	daily := make([]embyPlaybackDailySummary, 0, len(dailyBuckets))
	for _, bucket := range dailyBuckets {
		daily = append(daily, *bucket)
	}
	sort.Slice(daily, func(i, j int) bool { return daily[i].Date < daily[j].Date })

	return embyPlaybackStatsResponse{
		Scope:          scope,
		UID:            uid,
		Period:         query.Period,
		From:           query.From,
		To:             query.To,
		GroupBy:        query.GroupBy,
		MediaType:      query.MediaType,
		Query:          query.Search,
		MinDuration:    query.MinDuration,
		TotalPlays:     totalPlays,
		TotalDuration:  totalDuration,
		UniqueItems:    len(uniqueItemKeys),
		Days:           query.Days,
		Limit:          query.Limit,
		CanViewGlobal:  policy.CanViewGlobal,
		CanViewOthers:  policy.CanViewOthers,
		Source:         source,
		Policy:         policy,
		UserRankings:   userRankings,
		DailyBreakdown: daily,
		TopItems:       topItems,
	}
}

func (a *App) buildEmbyPlaybackStatsFromRecords(ctx context.Context, scope string, uid int64, query embyPlaybackStatsQuery, policy embyPlaybackViewerPolicy) embyPlaybackStatsResponse {
	records := a.store().PlaybackRecords(0, query.Since.Unix(), 10000)
	filtered := make([]store.PlaybackRecord, 0, len(records))
	metadataEvents := make([]embyPlaybackEvent, 0, len(records))
	for _, record := range records {
		if record.PlayedAt >= query.Until.Unix() {
			continue
		}
		localUser, _ := a.store().User(record.UID)
		if scope != "global" && localUser.UID != uid {
			continue
		}
		filtered = append(filtered, record)
		metadataEvents = append(metadataEvents, embyPlaybackEvent{ItemID: record.ItemID})
	}
	metadata := a.embyPlaybackMetadata(ctx, metadataEvents)

	userBuckets := map[string]*embyPlaybackUserRanking{}
	itemBuckets := map[string]*embyPlaybackItemRanking{}
	dailyBuckets := map[string]*embyPlaybackDailySummary{}
	uniqueItemKeys := map[string]struct{}{}
	totalPlays := 0
	totalDuration := int64(0)

	for _, record := range filtered {
		localUser, _ := a.store().User(record.UID)
		duration := clampPlaybackDuration(record.Duration)
		if duration < query.MinDuration {
			continue
		}
		meta := metadata[record.ItemID]
		itemID := firstNonEmpty(meta.ID, record.ItemID)
		itemName := firstNonEmpty(meta.Name, record.Title, record.ItemID, "未知媒体")
		mediaType := firstNonEmpty(strings.ToLower(strings.TrimSpace(meta.Type)), strings.ToLower(strings.TrimSpace(record.MediaType)))
		itemKey := itemID
		imageID := itemID
		if mediaType == "episode" || mediaType == "series" {
			itemID = firstNonEmpty(meta.SeriesID, meta.ID, record.ItemID)
			itemKey = "series:" + firstNonEmpty(itemID, record.SeriesName, record.Title)
			imageID = itemID
			itemName = firstNonEmpty(meta.SeriesName, record.SeriesName, meta.Name, record.Title, record.ItemID)
			mediaType = "series"
		} else if mediaType == "movie" {
			itemID = firstNonEmpty(meta.ID, record.ItemID)
			itemKey = "movie:" + firstNonEmpty(itemID, record.Title)
			imageID = itemID
		}
		mediaType = normalizePlaybackMediaType(mediaType)
		if query.MediaType != "all" && mediaType != query.MediaType {
			continue
		}
		if query.Search != "" && !strings.Contains(strings.ToLower(itemName), strings.ToLower(query.Search)) {
			continue
		}

		totalPlays++
		totalDuration += duration
		if itemKey == "" {
			itemKey = firstNonEmpty(itemID, itemName)
		}
		uniqueItemKeys[itemKey] = struct{}{}

		if policy.ShowUserRankings {
			userKey := fmt.Sprintf("uid:%d", localUser.UID)
			bucket := userBuckets[userKey]
			if bucket == nil {
				bucket = &embyPlaybackUserRanking{UID: localUser.UID, Username: firstNonEmpty(localUser.Username, "未知用户")}
				userBuckets[userKey] = bucket
			}
			bucket.Plays++
			bucket.Duration += duration
		}

		if policy.ShowItemRankings {
			item := itemBuckets[itemKey]
			if item == nil {
				item = &embyPlaybackItemRanking{
					ID:        itemID,
					Name:      itemName,
					MediaType: mediaType,
					ImageURL:  embyPlaybackImageURL(imageID),
				}
				itemBuckets[itemKey] = item
			}
			item.Plays++
			item.Duration += duration
		}

		if policy.ShowDailySummary {
			dateKey := playbackStatsBucketKey(time.Unix(record.PlayedAt, 0), query.GroupBy, query.Since.Location())
			daily := dailyBuckets[dateKey]
			if daily == nil {
				daily = &embyPlaybackDailySummary{Date: dateKey}
				dailyBuckets[dateKey] = daily
			}
			daily.Plays++
			daily.Duration += duration
		}
	}

	userRankings := make([]embyPlaybackUserRanking, 0, len(userBuckets))
	for _, bucket := range userBuckets {
		userRankings = append(userRankings, *bucket)
	}
	sort.Slice(userRankings, func(i, j int) bool {
		switch query.SortBy {
		case "name":
			return strings.ToLower(userRankings[i].Username) < strings.ToLower(userRankings[j].Username)
		case "duration":
			if userRankings[i].Duration != userRankings[j].Duration {
				return userRankings[i].Duration > userRankings[j].Duration
			}
		default:
			if userRankings[i].Plays != userRankings[j].Plays {
				return userRankings[i].Plays > userRankings[j].Plays
			}
		}
		if userRankings[i].Duration != userRankings[j].Duration {
			return userRankings[i].Duration > userRankings[j].Duration
		}
		if userRankings[i].Plays != userRankings[j].Plays {
			return userRankings[i].Plays > userRankings[j].Plays
		}
		return strings.ToLower(userRankings[i].Username) < strings.ToLower(userRankings[j].Username)
	})
	if len(userRankings) > query.Limit {
		userRankings = userRankings[:query.Limit]
	}

	topItems := make([]embyPlaybackItemRanking, 0, len(itemBuckets))
	for _, bucket := range itemBuckets {
		topItems = append(topItems, *bucket)
	}
	sort.Slice(topItems, func(i, j int) bool {
		if query.SortBy == "name" {
			return strings.ToLower(topItems[i].Name) < strings.ToLower(topItems[j].Name)
		}
		if query.SortBy == "duration" && topItems[i].Duration != topItems[j].Duration {
			return topItems[i].Duration > topItems[j].Duration
		}
		if topItems[i].Plays != topItems[j].Plays {
			return topItems[i].Plays > topItems[j].Plays
		}
		if topItems[i].Duration != topItems[j].Duration {
			return topItems[i].Duration > topItems[j].Duration
		}
		return strings.ToLower(topItems[i].Name) < strings.ToLower(topItems[j].Name)
	})
	if len(topItems) > query.Limit {
		topItems = topItems[:query.Limit]
	}

	daily := make([]embyPlaybackDailySummary, 0, len(dailyBuckets))
	for _, bucket := range dailyBuckets {
		daily = append(daily, *bucket)
	}
	sort.Slice(daily, func(i, j int) bool { return daily[i].Date < daily[j].Date })

	return embyPlaybackStatsResponse{
		Scope:          scope,
		UID:            uid,
		Period:         query.Period,
		From:           query.From,
		To:             query.To,
		GroupBy:        query.GroupBy,
		MediaType:      query.MediaType,
		Query:          query.Search,
		MinDuration:    query.MinDuration,
		TotalPlays:     totalPlays,
		TotalDuration:  totalDuration,
		UniqueItems:    len(uniqueItemKeys),
		Days:           query.Days,
		Limit:          query.Limit,
		CanViewGlobal:  policy.CanViewGlobal,
		CanViewOthers:  policy.CanViewOthers,
		Source:         "playback_records",
		Policy:         policy,
		UserRankings:   userRankings,
		DailyBreakdown: daily,
		TopItems:       topItems,
	}
}

func embyPlaybackEventsFromLogs(logs []store.EmbyActivityLog, since, until int64) []embyPlaybackEvent {
	sortedLogs := append([]store.EmbyActivityLog(nil), logs...)
	sort.Slice(sortedLogs, func(i, j int) bool {
		if sortedLogs[i].Date != sortedLogs[j].Date {
			return sortedLogs[i].Date < sortedLogs[j].Date
		}
		return sortedLogs[i].EmbyLogID < sortedLogs[j].EmbyLogID
	})
	starts := map[string][]embyPlaybackStart{}
	events := make([]embyPlaybackEvent, 0)
	for _, log := range sortedLogs {
		if log.Date <= 0 || strings.TrimSpace(log.ItemID) == "" {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(log.Type))
		userKey := firstNonEmpty(strings.TrimSpace(log.UserID), strings.TrimSpace(log.UserName))
		if userKey == "" {
			continue
		}
		key := normalizePlaybackUserKey(userKey) + "|" + strings.TrimSpace(log.ItemID)
		switch kind {
		case "playback.start", "videoplayback":
			starts[key] = append(starts[key], embyPlaybackStart{UserKey: userKey, UserID: log.UserID, UserName: log.UserName, ItemID: log.ItemID, Started: log.Date})
		case "playback.stop", "videoplaybackcomplete":
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
			events = append(events, embyPlaybackEvent{
				UserKey:  firstNonEmpty(start.UserKey, userKey),
				UserID:   firstNonEmpty(start.UserID, log.UserID),
				UserName: firstNonEmpty(start.UserName, log.UserName),
				ItemID:   log.ItemID,
				PlayedAt: log.Date,
				Duration: clampPlaybackDuration(log.Date - startedAt),
			})
		}
	}
	return events
}

func clampPlaybackDuration(seconds int64) int64 {
	if seconds < 0 {
		return 0
	}
	if seconds > maxPlaybackDurationSeconds {
		return maxPlaybackDurationSeconds
	}
	return seconds
}

func playbackStatsUsersByKey(users []store.User) map[string]store.User {
	out := make(map[string]store.User, len(users)*3)
	for _, user := range users {
		for _, key := range []string{user.EmbyID, user.EmbyUsername, user.Username} {
			if normalized := normalizePlaybackUserKey(key); normalized != "" {
				out[normalized] = user
			}
		}
	}
	return out
}

func normalizePlaybackUserKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizePlaybackMediaType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "movie":
		return "movie"
	case "episode", "series":
		return "series"
	default:
		return "other"
	}
}

func playbackStatsBucketKey(value time.Time, groupBy string, location *time.Location) string {
	local := value.In(location)
	switch groupBy {
	case "week":
		weekday := (int(local.Weekday()) + 6) % 7
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -weekday).Format("2006-01-02")
	case "month":
		return local.Format("2006-01")
	default:
		return local.Format("2006-01-02")
	}
}

func embyPlaybackImageURL(itemID string) string {
	itemID = strings.TrimSpace(itemID)
	if !validEmbyPlaybackImageID(itemID) {
		return ""
	}
	return "/api/v1/emby/playback-items/" + url.PathEscape(itemID) + "/image"
}

func validEmbyPlaybackImageID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func (a *App) handleEmbyPlaybackItemImage(w http.ResponseWriter, r *http.Request, params Params) {
	cfg := a.cfg()
	caller := current(r).User
	if !cfg.EmbyPlaybackStatsEnabled || (caller.Role != store.RoleAdmin && !cfg.EmbyPlaybackStatsUserEnabled) {
		failWithCode(w, http.StatusForbidden, ErrWatchStatsForbidden, "playback stats image unavailable")
		return
	}
	if !a.embyConfigured() {
		failWithCode(w, http.StatusBadGateway, ErrEmbyNotConfigured, "Emby not configured")
		return
	}
	itemID := strings.TrimSpace(params["item_id"])
	if !validEmbyPlaybackImageID(itemID) {
		failWithCode(w, http.StatusNotFound, ErrNotFound, "image not found")
		return
	}
	endpoint, err := a.validatedEmbyEndpoint("/Items/" + url.PathEscape(itemID) + "/Images/Primary?maxWidth=512&quality=90")
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "failed to resolve Emby image endpoint")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, "failed to create Emby image request")
		return
	}
	for key, value := range a.embyHeaders() {
		req.Header.Set(key, value)
	}
	req.Header.Set("Accept", "image/avif,image/webp,image/*")
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "failed to fetch Emby image")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		failWithCode(w, http.StatusNotFound, ErrNotFound, "image not found")
		return
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "Emby image request failed")
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxEmbyPlaybackImageBytes+1))
	if err != nil || len(data) == 0 || int64(len(data)) > maxEmbyPlaybackImageBytes {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "invalid Emby image response")
		return
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	detectedType := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0]))
	if isPlaybackImageContentType(detectedType) {
		contentType = detectedType
	} else if !isPlaybackImageContentType(contentType) {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "invalid Emby image content type")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=3600, stale-while-revalidate=86400")
	if etag := strings.TrimSpace(resp.Header.Get("ETag")); etag != "" && !strings.ContainsAny(etag, "\r\n") {
		w.Header().Set("ETag", etag)
	}
	_, _ = w.Write(data)
}

func isPlaybackImageContentType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image/jpeg", "image/png", "image/webp", "image/gif", "image/avif", "image/bmp":
		return true
	default:
		return false
	}
}

func (a *App) embyPlaybackMetadata(ctx context.Context, events []embyPlaybackEvent) map[string]embyPlaybackItemMetadata {
	ids := make([]string, 0)
	seen := map[string]bool{}
	for _, event := range events {
		id := strings.TrimSpace(event.ItemID)
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	result := make(map[string]embyPlaybackItemMetadata, len(ids))
	const batchSize = 100
	for start := 0; start < len(ids); start += batchSize {
		end := min(start+batchSize, len(ids))
		var payload struct {
			Items []embyPlaybackItemMetadata `json:"Items"`
		}
		query := embyItemQuery(map[string]string{
			"Ids":       strings.Join(ids[start:end], ","),
			"Recursive": "true",
			"Fields":    "SeriesId,SeriesName",
		})
		if err := a.embyGet(ctx, "/Items"+query, &payload); err != nil {
			zap.L().Warn("failed to batch read Emby item metadata for playback stats", zap.Error(err))
			continue
		}
		for _, item := range payload.Items {
			result[item.ID] = item
		}
	}
	return result
}

func (a *App) cachedEmbyPlaybackStats(key string) (embyPlaybackStatsResponse, bool) {
	a.embyPlaybackStatsMu.Lock()
	defer a.embyPlaybackStatsMu.Unlock()
	entry, ok := a.embyPlaybackStatsCache[key]
	if !ok || time.Now().After(entry.Until) {
		if ok {
			delete(a.embyPlaybackStatsCache, key)
		}
		return embyPlaybackStatsResponse{}, false
	}
	return entry.Value, true
}

func (a *App) cacheEmbyPlaybackStats(key string, value embyPlaybackStatsResponse) {
	a.embyPlaybackStatsMu.Lock()
	if a.embyPlaybackStatsCache == nil {
		a.embyPlaybackStatsCache = map[string]embyPlaybackStatsCacheEntry{}
	}
	if len(a.embyPlaybackStatsCache) >= 256 {
		for cacheKey, entry := range a.embyPlaybackStatsCache {
			if time.Now().After(entry.Until) {
				delete(a.embyPlaybackStatsCache, cacheKey)
			}
		}
		if len(a.embyPlaybackStatsCache) >= 256 {
			oldestKey := ""
			var oldestUntil time.Time
			for cacheKey, entry := range a.embyPlaybackStatsCache {
				if oldestKey == "" || entry.Until.Before(oldestUntil) {
					oldestKey = cacheKey
					oldestUntil = entry.Until
				}
			}
			delete(a.embyPlaybackStatsCache, oldestKey)
		}
	}
	a.embyPlaybackStatsCache[key] = embyPlaybackStatsCacheEntry{Until: time.Now().Add(embyPlaybackStatsCacheTTL), Value: value}
	a.embyPlaybackStatsMu.Unlock()
}

func (a *App) clearEmbyPlaybackStatsCache() {
	a.embyPlaybackStatsMu.Lock()
	a.embyPlaybackStatsCache = map[string]embyPlaybackStatsCacheEntry{}
	a.embyPlaybackStatsMu.Unlock()
}
