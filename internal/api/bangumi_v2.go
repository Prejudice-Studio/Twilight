package api

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

type v2BangumiSummary struct {
	Status             v2BangumiStatus                       `json:"status"`
	Account            *v2BangumiAccount                     `json:"account,omitempty"`
	Collections        map[string]v2BangumiCollectionPreview `json:"collections,omitempty"`
	RecentActivity     []map[string]any                      `json:"recent_activity,omitempty"`
	AccountError       bool                                  `json:"account_error,omitempty"`
	CollectionsPartial bool                                  `json:"collections_partial,omitempty"`
}

type v2BangumiStatus struct {
	SyncEnabled   bool                   `json:"sync_enabled"`
	ManageEnabled bool                   `json:"manage_enabled"`
	BGMMode       bool                   `json:"bgm_mode"`
	BGMManageMode bool                   `json:"bgm_manage_mode"`
	TokenSet      bool                   `json:"token_set"`
	SyncReady     bool                   `json:"sync_ready"`
	TotalRecords  int                    `json:"total_records"`
	SyncedCount   int                    `json:"synced_count"`
	RecentLogs    []store.BangumiSyncLog `json:"recent_logs"`
}

type v2BangumiAccount struct {
	ID       int64             `json:"id,omitempty"`
	Username string            `json:"username,omitempty"`
	Nickname string            `json:"nickname,omitempty"`
	Sign     string            `json:"sign,omitempty"`
	Avatar   map[string]string `json:"avatar,omitempty"`
	Expired  bool              `json:"expired,omitempty"`
}

type v2BangumiCollectionPreview struct {
	Entries        []map[string]any `json:"entries"`
	Total          int              `json:"total"`
	Cached         bool             `json:"cached"`
	CacheUpdatedAt int64            `json:"cache_updated_at,omitempty"`
}

type v2BangumiCollectionSpec struct {
	Key  string
	Type int
}

type v2BangumiCollectionResult struct {
	Preview v2BangumiCollectionPreview
	Err     error
}

// handleV2BangumiSummary keeps the Bangumi landing page to one authenticated
// request. The upstream account and collection calls are independent: a
// partial Bangumi failure must not erase the local sync status or other cached
// collection previews.
func (a *App) handleV2BangumiSummary(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	u := current(r).User
	logs := a.store().ListBangumiSyncLogs(u.UID, 50)
	syncedCount := 0
	for _, log := range logs {
		if log.Status == "success" {
			syncedCount++
		}
	}

	status := v2BangumiStatus{
		SyncEnabled:   a.cfg().BangumiEnabled,
		ManageEnabled: a.cfg().BangumiManageEnabled,
		BGMMode:       u.BGMMode,
		BGMManageMode: u.BGMManageMode,
		TokenSet:      u.BGMToken != "",
		SyncReady:     u.BGMMode && u.BGMToken != "",
		TotalRecords:  len(a.store().PlaybackRecords(u.UID, 0, 0)),
		SyncedCount:   syncedCount,
		RecentLogs:    logs,
	}
	summary := v2BangumiSummary{Status: status}

	if u.BGMToken == "" || !a.cfg().BangumiManageEnabled || !u.BGMManageMode {
		ok(w, "OK", summary)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	me, expired, err := a.getBangumiMe(ctx, u.BGMToken)
	if err != nil {
		summary.AccountError = true
		ok(w, "OK", summary)
		return
	}
	if expired {
		summary.Account = &v2BangumiAccount{Expired: true}
		ok(w, "OK", summary)
		return
	}

	account := publicBangumiAccount(me)
	summary.Account = &account
	username := asString(me["username"])
	if username == "" {
		username = asString(me["id"])
	}
	if username == "" {
		summary.AccountError = true
		ok(w, "OK", summary)
		return
	}

	collections, partial := a.loadV2BangumiPreviews(ctx, u, username)
	summary.Collections = collections
	summary.CollectionsPartial = partial
	summary.RecentActivity = recentBangumiActivity(collections, 8)
	ok(w, "OK", summary)
}

func (a *App) handleV2BangumiSync(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleBangumiSyncTrigger(w, r, p)
}

func (a *App) handleV2BangumiClearHistory(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleBangumiClearHistory(w, r, p)
}

func (a *App) handleV2BangumiPreferences(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleUpdateMe(w, r, p)
}

func (a *App) handleV2BangumiCollections(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleBangumiCollections(w, r, p)
}

func (a *App) handleV2UpdateBangumiCollection(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleUpdateBangumiCollection(w, r, p)
}

func publicBangumiAccount(me map[string]any) v2BangumiAccount {
	account := v2BangumiAccount{
		ID:       int64(numeric(me["id"])),
		Username: truncateString(strings.TrimSpace(asString(me["username"])), 128),
		Nickname: truncateString(strings.TrimSpace(asString(me["nickname"])), 128),
		Sign:     truncateString(strings.TrimSpace(asString(me["sign"])), 500),
	}
	if raw, ok := me["avatar"].(map[string]any); ok {
		account.Avatar = make(map[string]string, 4)
		for _, key := range []string{"large", "medium", "small"} {
			if value := strings.TrimSpace(asString(raw[key])); value != "" {
				account.Avatar[key] = value
			}
		}
		if len(account.Avatar) == 0 {
			account.Avatar = nil
		}
	}
	return account
}

func (a *App) loadV2BangumiPreviews(ctx context.Context, u store.User, username string) (map[string]v2BangumiCollectionPreview, bool) {
	specs := []v2BangumiCollectionSpec{
		{Key: "watching", Type: 3},
		{Key: "collected", Type: 2},
		{Key: "wishlist", Type: 1},
		{Key: "on_hold", Type: 4},
		{Key: "dropped", Type: 5},
	}
	results := make([]v2BangumiCollectionResult, len(specs))
	var wg sync.WaitGroup
	wg.Add(len(specs))
	for index, spec := range specs {
		index, spec := index, spec
		go func() {
			defer wg.Done()
			entries, total, cached, updatedAt, err := a.bangumiCollectionsCached(ctx, u, username, spec.Type, 8, 0, false)
			if err != nil {
				results[index].Err = err
				return
			}
			results[index].Preview = v2BangumiCollectionPreview{
				Entries:        bangumiCollectionPublicEntries(entries, spec.Type),
				Total:          total,
				Cached:         cached,
				CacheUpdatedAt: updatedAt,
			}
		}()
	}
	wg.Wait()

	previews := make(map[string]v2BangumiCollectionPreview, len(specs))
	partial := false
	for index, spec := range specs {
		if results[index].Err != nil {
			partial = true
			continue
		}
		previews[spec.Key] = results[index].Preview
	}
	return previews, partial
}

func bangumiCollectionPublicEntries(entries []map[string]any, collectionType int) []map[string]any {
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		item := map[string]any{
			"subject_id":      int64(numeric(entry["subject_id"])),
			"type":            int(numeric(entry["type"])),
			"ep_status":       int(numeric(entry["ep_status"])),
			"rate":            int(numeric(entry["rate"])),
			"updated_at":      bangumiTimestamp(entry["updated_at"]),
			"collection_type": collectionType,
		}
		if item["subject_id"] == int64(0) {
			if subject, ok := entry["subject"].(map[string]any); ok {
				item["subject_id"] = int64(numeric(subject["id"]))
			}
		}
		if subject, ok := entry["subject"].(map[string]any); ok {
			item["subject"] = publicBangumiSubject(subject)
		}
		out = append(out, item)
	}
	return out
}

func publicBangumiSubject(subject map[string]any) map[string]any {
	out := make(map[string]any, 12)
	for _, key := range []string{"id", "name", "name_cn", "summary", "date", "platform"} {
		if value := strings.TrimSpace(asString(subject[key])); value != "" {
			limit := 300
			if key == "summary" {
				limit = 1200
			}
			out[key] = truncateString(value, limit)
		}
	}
	for _, key := range []string{"eps", "volumes"} {
		if value := int(numeric(subject[key])); value > 0 {
			out[key] = value
		}
	}
	if images, ok := subject["images"].(map[string]any); ok {
		imageOut := make(map[string]string, 4)
		for _, key := range []string{"large", "common", "medium", "small"} {
			if value := strings.TrimSpace(asString(images[key])); value != "" {
				imageOut[key] = value
			}
		}
		if len(imageOut) > 0 {
			out["images"] = imageOut
		}
	}
	if rating, ok := subject["rating"].(map[string]any); ok {
		ratingOut := map[string]any{}
		if score := mediaFloat(rating["score"]); score > 0 {
			ratingOut["score"] = score
		}
		for _, key := range []string{"rank", "total"} {
			if value := numeric(rating[key]); value > 0 {
				ratingOut[key] = value
			}
		}
		if len(ratingOut) > 0 {
			out["rating"] = ratingOut
		}
	}
	if tags, ok := subject["tags"].([]any); ok {
		tagOut := make([]string, 0, minInt(len(tags), 8))
		for _, raw := range tags {
			if tag, ok := raw.(map[string]any); ok {
				if name := strings.TrimSpace(asString(tag["name"])); name != "" {
					tagOut = append(tagOut, name)
				}
			}
			if len(tagOut) >= 8 {
				break
			}
		}
		if len(tagOut) > 0 {
			out["tags"] = tagOut
		}
	}
	return out
}

func recentBangumiActivity(collections map[string]v2BangumiCollectionPreview, limit int) []map[string]any {
	activity := make([]map[string]any, 0)
	for _, preview := range collections {
		activity = append(activity, preview.Entries...)
	}
	sort.SliceStable(activity, func(i, j int) bool {
		return int64(numeric(activity[i]["updated_at"])) > int64(numeric(activity[j]["updated_at"]))
	})
	if limit <= 0 || len(activity) <= limit {
		return activity
	}
	return activity[:limit]
}

func bangumiTimestamp(value any) int64 {
	if number := numeric(value); number > 0 {
		if number > 100000000000 {
			return number / 1000
		}
		return number
	}
	raw := strings.TrimSpace(asString(value))
	if raw == "" {
		return 0
	}
	if number, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if number > 100000000000 {
			return number / 1000
		}
		return number
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.Unix()
		}
	}
	return 0
}
