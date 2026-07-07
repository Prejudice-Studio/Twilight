package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

func (a *App) handleBangumiSyncStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	u := p.User
	logs := a.store().ListBangumiSyncLogs(u.UID, 50)
	records := a.store().PlaybackRecords(u.UID, 0, 0)
	totalRecords := len(records)
	syncedCount := 0
	for _, log := range logs {
		if log.Status == "success" {
			syncedCount++
		}
	}
	ok(w, "OK", map[string]any{
		"bgm_mode":        u.BGMMode,
		"bgm_manage_mode": u.BGMManageMode,
		"bgm_token_set":   u.BGMToken != "",
		"sync_ready":      u.BGMMode && u.BGMToken != "",
		"total_records":   totalRecords,
		"synced_count":    syncedCount,
		"recent_logs":     logs,
	})
}

func (a *App) handleBangumiSyncTrigger(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.cfg().BangumiEnabled {
		failWithCode(w, http.StatusBadRequest, ErrBangumiSyncDisabled, "Bangumi 同步未启用")
		return
	}
	p := current(r)
	u := p.User
	if !u.BGMMode || u.BGMToken == "" {
		failWithCode(w, http.StatusBadRequest, ErrBangumiTokenMissing, "请先配置 Bangumi Token 并开启同步")
		return
	}
	ctx := r.Context()
	zap.L().Info("bangumi sync triggered by user", zap.Int64("uid", u.UID))
	synced, skipped, failed, logs := a.syncBangumiForUser(ctx, u.UID)
	ok(w, "同步完成", map[string]any{
		"synced":  synced,
		"skipped": skipped,
		"failed":  failed,
		"logs":    logs,
	})
}

func (a *App) handleBangumiSyncHistory(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.cfg().BangumiEnabled {
		failWithCode(w, http.StatusBadRequest, ErrBangumiSyncDisabled, "Bangumi 同步未启用")
		return
	}
	p := current(r)
	limit := queryInt(r, "limit", 50)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	logs := a.store().ListBangumiSyncLogs(p.User.UID, limit)
	ok(w, "OK", map[string]any{
		"logs":  logs,
		"total": len(logs),
	})
}

func (a *App) handleBangumiClearHistory(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.cfg().BangumiEnabled {
		failWithCode(w, http.StatusBadRequest, ErrBangumiSyncDisabled, "Bangumi 同步未启用")
		return
	}
	p := current(r)
	if err := a.store().ClearBangumiSyncLogs(p.User.UID); err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, "清除失败: "+err.Error())
		return
	}
	ok(w, "已清除同步历史", nil)
}

func (a *App) handleAdminBangumiUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	users := a.store().ListUsers()
	page := max(1, queryInt(r, "page", 1))
	perPage := clamp(queryInt(r, "per_page", 20), 1, 100)
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))

	filteredUsers := make([]store.User, 0)
	for _, u := range users {
		if search != "" {
			uidStr := strconv.FormatInt(u.UID, 10)
			if !strings.Contains(strings.ToLower(u.Username), search) && !strings.Contains(uidStr, search) {
				continue
			}
		}
		filteredUsers = append(filteredUsers, u)
	}

	total := len(filteredUsers)
	paginatedUsers := paginate(filteredUsers, page, perPage)

	type BangumiUserInfo struct {
		UID           int64  `json:"uid"`
		Username      string `json:"username"`
		BGMMode       bool   `json:"bgm_mode"`
		BGMManageMode bool   `json:"bgm_manage_mode"`
		TokenSet      bool   `json:"token_set"`
		SyncReady     bool   `json:"sync_ready"`
		SyncCount     int    `json:"sync_count"`
		RecordCount   int    `json:"record_count"`
	}

	result := make([]BangumiUserInfo, 0, len(paginatedUsers))
	for _, u := range paginatedUsers {
		info := BangumiUserInfo{
			UID:           u.UID,
			Username:      u.Username,
			BGMMode:       u.BGMMode,
			BGMManageMode: u.BGMManageMode,
			TokenSet:      u.BGMToken != "",
			SyncReady:     u.BGMMode && u.BGMToken != "",
		}
		syncLogs := a.store().ListBangumiSyncLogs(u.UID, 100)
		for _, log := range syncLogs {
			if log.Status == "success" {
				info.SyncCount++
			}
		}
		info.RecordCount = len(a.store().PlaybackRecords(u.UID, 0, 0))
		result = append(result, info)
	}

	totalPages := (total + perPage - 1) / perPage
	ok(w, "OK", map[string]any{
		"users":    result,
		"total":    total,
		"page":     page,
		"per_page": perPage,
		"pages":    totalPages,
	})
}

func (a *App) handleAdminBangumiRecords(w http.ResponseWriter, r *http.Request, ps Params) {
	uid, err := strconv.ParseInt(ps["uid"], 10, 64)
	if err != nil || uid <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "无效的用户 ID")
		return
	}
	limit := queryInt(r, "limit", 100)
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	records := a.store().PlaybackRecords(uid, 0, limit)
	logs := a.store().ListBangumiSyncLogs(uid, limit)
	logMap := make(map[string]string)
	for _, log := range logs {
		if log.RecordItemID != "" && log.Status == "success" {
			logMap[log.RecordItemID] = log.SubjectName
		}
	}
	type RecordWithSync struct {
		UID         int64  `json:"uid"`
		ItemID      string `json:"item_id"`
		Title       string `json:"title"`
		SeriesName  string `json:"series_name,omitempty"`
		MediaType   string `json:"media_type"`
		IndexNumber int    `json:"index_number,omitempty"`
		Duration    int64  `json:"duration"`
		PlayedAt    int64  `json:"played_at"`
		SyncedName  string `json:"synced_name,omitempty"`
	}
	out := make([]RecordWithSync, 0, len(records))
	for _, rec := range records {
		out = append(out, RecordWithSync{
			UID: rec.UID, ItemID: rec.ItemID, Title: rec.Title,
			SeriesName: rec.SeriesName, MediaType: rec.MediaType,
			IndexNumber: rec.IndexNumber, Duration: rec.Duration,
			PlayedAt: rec.PlayedAt, SyncedName: logMap[rec.ItemID],
		})
	}
	ok(w, "OK", map[string]any{"records": out, "total": len(out)})
}

func (a *App) handleAdminBangumiSyncUser(w http.ResponseWriter, r *http.Request, ps Params) {
	if !a.cfg().BangumiEnabled {
		failWithCode(w, http.StatusBadRequest, ErrBangumiSyncDisabled, "Bangumi 同步未启用")
		return
	}
	uid, err := strconv.ParseInt(ps["uid"], 10, 64)
	if err != nil || uid <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "无效的用户 ID")
		return
	}
	u, found := a.store().User(uid)
	if !found {
		failWithCode(w, http.StatusNotFound, ErrUserNotFound, "用户不存在")
		return
	}
	if !u.BGMMode || u.BGMToken == "" {
		failWithCode(w, http.StatusBadRequest, ErrBangumiTokenMissing, "该用户未配置 Bangumi Token 或未开启同步")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	zap.L().Info("bangumi sync triggered by admin", zap.Int64("uid", uid), zap.Int64("admin_uid", current(r).User.UID))
	synced, skipped, failed, logs := a.syncBangumiForUser(ctx, uid)
	ok(w, "同步完成", map[string]any{
		"synced":  synced,
		"skipped": skipped,
		"failed":  failed,
		"logs":    logs,
	})
}

func (a *App) handleAdminBangumiSyncLogs(w http.ResponseWriter, r *http.Request, ps Params) {
	uid, err := strconv.ParseInt(ps["uid"], 10, 64)
	if err != nil || uid <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "无效的用户 ID")
		return
	}
	limit := queryInt(r, "limit", 100)
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	logs := a.store().ListBangumiSyncLogs(uid, limit)
	ok(w, "OK", map[string]any{"logs": logs, "total": len(logs)})
}

func (a *App) handleAdminBangumiClearLogs(w http.ResponseWriter, r *http.Request, ps Params) {
	uid, err := strconv.ParseInt(ps["uid"], 10, 64)
	if err != nil || uid <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "无效的用户 ID")
		return
	}
	if err := a.store().ClearBangumiSyncLogs(uid); err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, "清除失败: "+err.Error())
		return
	}
	ok(w, "已清除", nil)
}

func (a *App) handleBangumiMe(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	u := p.User

	if u.BGMToken == "" {
		ok(w, "Token not set", map[string]any{
			"bgm_token_set": false,
		})
		return
	}

	if !a.cfg().BangumiManageEnabled || !u.BGMManageMode {
		ok(w, "Management disabled", map[string]any{
			"bgm_token_set":       true,
			"bgm_manage_disabled": true,
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	me, expired, err := a.getBangumiMe(ctx, u.BGMToken)
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "获取 Bangumi 用户信息失败: "+err.Error())
		return
	}
	if expired {
		ok(w, "Token expired", map[string]any{
			"bgm_token_set": true,
			"expired":       true,
		})
		return
	}

	username := asString(me["username"])
	if username == "" {
		username = fmt.Sprint(me["id"])
	}

	// Fetch watching collections (type 3 - 在看)
	watching, watchingTotal, _ := a.getBangumiUserCollections(ctx, username, u.BGMToken, 3, 8, 0)
	// Fetch wishlist collections (type 1 - 想看)
	wishlist, wishlistTotal, _ := a.getBangumiUserCollections(ctx, username, u.BGMToken, 1, 8, 0)
	// Fetch collected collections (type 2 - 看过)
	collected, collectedTotal, _ := a.getBangumiUserCollections(ctx, username, u.BGMToken, 2, 8, 0)

	ok(w, "OK", map[string]any{
		"bgm_token_set":   true,
		"expired":         false,
		"me":              me,
		"watching":        watching,
		"watching_total":  watchingTotal,
		"wishlist":        wishlist,
		"wishlist_total":  wishlistTotal,
		"collected":       collected,
		"collected_total": collectedTotal,
	})
}

func (a *App) handleBangumiCollections(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.cfg().BangumiManageEnabled {
		failWithCode(w, http.StatusBadRequest, ErrBangumiManageDisabled, "Bangumi 管理功能未启用")
		return
	}
	p := current(r)
	u := p.User
	if u.BGMToken == "" {
		failWithCode(w, http.StatusBadRequest, ErrBangumiTokenMissing, "请先配置 Bangumi Token")
		return
	}
	if !u.BGMManageMode {
		failWithCode(w, http.StatusBadRequest, ErrInternal, "BGM 管理功能未启用")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	me, _, err := a.getBangumiMe(ctx, u.BGMToken)
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "获取 Bangumi 用户信息失败: "+err.Error())
		return
	}
	username := asString(me["username"])
	if username == "" {
		username = fmt.Sprint(me["id"])
	}

	collectType := queryInt(r, "type", 3) // 1:想看, 2:看过, 3:在看
	limit := queryInt(r, "limit", 20)
	offset := queryInt(r, "offset", 0)

	entries, total, err := a.getBangumiUserCollections(ctx, username, u.BGMToken, collectType, limit, offset)
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "获取 Bangumi 收藏列表失败: "+err.Error())
		return
	}

	ok(w, "OK", map[string]any{
		"entries": entries,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

func (a *App) handleUpdateBangumiCollection(w http.ResponseWriter, r *http.Request, ps Params) {
	if !a.cfg().BangumiManageEnabled {
		failWithCode(w, http.StatusBadRequest, ErrBangumiManageDisabled, "Bangumi 管理功能未启用")
		return
	}
	subjectID := ps["subject_id"]
	if !isPositiveNumericID(subjectID) {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "无效的 Subject ID")
		return
	}

	p := current(r)
	u := p.User
	if u.BGMToken == "" {
		failWithCode(w, http.StatusBadRequest, ErrBangumiTokenMissing, "请先配置 Bangumi Token")
		return
	}
	if !u.BGMManageMode {
		failWithCode(w, http.StatusBadRequest, ErrInternal, "BGM 管理功能未启用")
		return
	}

	payload := decodeMap(r)
	collectType := int(numeric(payload["type"])) // 1: 想看, 2: 看过, 3: 在看, 4: 搁置, 5: 抛弃
	_, hasEpStatus := payload["ep_status"]
	epStatus := int(numeric(payload["ep_status"]))
	_, hasRate := payload["rate"]
	rate := int(numeric(payload["rate"]))

	if collectType <= 0 || collectType > 5 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "收藏状态不合法 (应为 1-5)")
		return
	}
	if hasRate && (rate < 0 || rate > 10) {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "评分分值不合法 (应为 0-10)")
		return
	}
	if hasEpStatus && epStatus < 0 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "观看进度不能小于 0")
		return
	}
	if collectType != 2 && collectType != 3 {
		epStatus = 0
		hasEpStatus = false
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := a.updateBangumiCollection(ctx, subjectID, u.BGMToken, collectType, rate, hasRate); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "更新 Bangumi 收藏失败: "+err.Error())
		return
	}
	if hasEpStatus {
		if err := a.updateBangumiEpisodeProgress(ctx, subjectID, u.BGMToken, epStatus); err != nil {
			failWithCode(w, http.StatusBadGateway, ErrInternal, "更新 Bangumi 观看进度失败: "+err.Error())
			return
		}
	}

	detail := map[string]any{"subject_id": subjectID, "type": collectType}
	if hasEpStatus {
		detail["ep_status"] = epStatus
	}
	if hasRate {
		detail["rate"] = rate
	}
	a.audit(r, "update_bangumi_collection", "user", 0, detail)
	ok(w, "更新成功", nil)
}
