package api

import (
	"net/http"
	"strconv"

	"github.com/prejudice-studio/twilight/internal/store"
)

// handleGetUserPlaybackHistory 返回用户的播放记录列表。
// 支持时间范围筛选和分页。
func (a *App) handleGetUserPlaybackHistory(w http.ResponseWriter, r *http.Request, params Params) {
	p := current(r)
	uid, _ := int64Param(params, "uid")

	// 只能查看自己的播放记录，管理员可以查看任意用户
	if uid != p.User.UID && p.User.Role != store.RoleAdmin {
		failWithCode(w, http.StatusForbidden, ErrForbidden, "无权查看他人播放记录")
		return
	}

	// 解析查询参数
	since := int64(0)
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if parsed, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			since = parsed
		}
	}

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	records := a.store().PlaybackRecords(uid, since, limit)

	ok(w, "OK", map[string]any{
		"records": records,
		"count":   len(records),
	})
}

// handleGetUserPlaybackSessions 返回用户的播放会话列表。
// 会话是连续播放的聚合视图。
func (a *App) handleGetUserPlaybackSessions(w http.ResponseWriter, r *http.Request, params Params) {
	p := current(r)
	uid, _ := int64Param(params, "uid")

	// 只能查看自己的播放会话，管理员可以查看任意用户
	if uid != p.User.UID && p.User.Role != store.RoleAdmin {
		failWithCode(w, http.StatusForbidden, ErrForbidden, "无权查看他人播放会话")
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}

	sessions := a.store().UserPlaybackSessions(uid, limit)

	ok(w, "OK", map[string]any{
		"sessions": sessions,
		"count":    len(sessions),
	})
}

// handleGetPlaybackSummary 返回全局播放统计摘要（仅管理员）。
func (a *App) handleGetPlaybackSummary(w http.ResponseWriter, r *http.Request, _ Params) {
	since := int64(0)
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if parsed, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			since = parsed
		}
	}

	totalPlays, totalDuration, uniqueUsers, err := a.store().PlaybackRecordSummary(since)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, "查询播放统计失败")
		return
	}

	ok(w, "OK", map[string]any{
		"total_plays":    totalPlays,
		"total_duration": totalDuration,
		"unique_users":   uniqueUsers,
		"since":          since,
	})
}
