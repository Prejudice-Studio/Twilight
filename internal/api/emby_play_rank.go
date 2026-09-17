package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

// 播放排行榜（日榜/周榜）只读聚合：回答"哪部最热、谁看得最多"，不回答"谁正在
// 看什么"。数据来源是本地播放记录表（Emby 活动日志同步写入），因此榜单依赖
// 活动日志同步；管理员后台可点同步拉取最新数据。
//
// 隐私口径（与仪表盘在线人数一致收紧）：
//   - 普通用户 / 访客接口只返回脱敏后的用户名，且不下发 uid；
//   - 管理员接口才返回 uid 与完整用户名，用于后台核查。
const (
	playRankRangeDay  = "day"
	playRankRangeWeek = "week"

	playRankDefaultLimit = 20
	playRankMaxLimit     = 100

	// playRankCacheTTL 让榜单在有人反复刷新页面时不至于每次都跑一遍聚合查询。
	playRankCacheTTL = 60 * time.Second
)

// playRankSnapshot 是榜单缓存条目：until 之前可以直接复用 data。
type playRankSnapshot struct {
	until time.Time
	data  map[string]any
}

// playRankWindow 返回榜单起点：日榜是今天 00:00，周榜是本周周一 00:00，
// 都用服务器本地时区——与管理员在后台看到的时间口径一致。
func playRankWindow(now time.Time, rangeKey string) int64 {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if rangeKey == playRankRangeWeek {
		// time.Weekday 以周日为 0，换算成"周一为一周之始"的回退天数。
		offset := int(now.Weekday()) - 1
		if offset < 0 {
			offset = 6
		}
		start = start.AddDate(0, 0, -offset)
	}
	return start.Unix()
}

// playRankQuery 解析并夹取 range / limit。非法取值一律回退默认，避免把
// 未过滤的参数带进 store 查询。
func playRankQuery(r *http.Request) (string, int) {
	rangeKey := playRankRangeDay
	switch r.URL.Query().Get("range") {
	case playRankRangeWeek:
		rangeKey = playRankRangeWeek
	}
	limit := playRankDefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > playRankMaxLimit {
		limit = playRankMaxLimit
	}
	return rangeKey, limit
}

// maskPlayRankUsername 把用户名打码：保留首尾字符，中间固定抹掉。打码后的
// 名字只用于"排名趣味"，不能用来反查账号。
func maskPlayRankUsername(name string) string {
	runes := []rune(name)
	switch len(runes) {
	case 0:
		return "***"
	case 1:
		return "*"
	case 2:
		return string(runes[0]) + "*"
	default:
		return string(runes[0]) + "***" + string(runes[len(runes)-1])
	}
}

// playRankData 返回榜单数据。includeIdentity=true 时带 uid 与完整用户名；
// refresh=true 时跳过缓存并写回新结果。
func (a *App) playRankData(rangeKey string, limit int, includeIdentity bool, refresh bool) map[string]any {
	key := rangeKey + "|" + strconv.Itoa(limit) + "|" + strconv.FormatBool(includeIdentity)

	a.playRankMu.Lock()
	if !refresh {
		if cached, ok := a.playRankCache[key]; ok && time.Now().Before(cached.until) {
			a.playRankMu.Unlock()
			return cached.data
		}
	}
	a.playRankMu.Unlock()

	data := a.buildPlayRank(rangeKey, limit, includeIdentity)

	a.playRankMu.Lock()
	// 缓存按 key 分开存放：脱敏版与管理员版互不相通，避免管理员的完整用户名
	// 被后续的同 key 普通请求拿到。
	if a.playRankCache == nil {
		a.playRankCache = map[string]playRankSnapshot{}
	}
	a.playRankCache[key] = playRankSnapshot{until: time.Now().Add(playRankCacheTTL), data: data}
	a.playRankMu.Unlock()
	return data
}

func (a *App) buildPlayRank(rangeKey string, limit int, includeIdentity bool) map[string]any {
	since := playRankWindow(time.Now(), rangeKey)
	media, users, _ := a.store().PlaybackRank(since, limit)
	totalPlays, totalDuration, uniqueUsers, _ := a.store().PlaybackRecordSummary(since)

	mediaItems := make([]map[string]any, 0, len(media))
	for _, item := range media {
		mediaItems = append(mediaItems, map[string]any{
			"item_id":     item.ItemID,
			"title":       firstNonEmpty(item.Title, item.ItemID),
			"series_name": item.SeriesName,
			"media_type":  firstNonEmpty(item.MediaType, "unknown"),
			"plays":       item.Plays,
			"duration":    item.Duration,
			"viewers":     item.Viewers,
		})
	}

	userItems := make([]map[string]any, 0, len(users))
	for _, item := range users {
		username := ""
		if user, ok := a.store().User(item.UID); ok {
			username = user.Username
		}
		display := username
		if display == "" {
			display = fmt.Sprintf("UID %d", item.UID)
		}
		if !includeIdentity {
			display = maskPlayRankUsername(display)
		}
		entry := map[string]any{
			"user_name": display,
			"plays":     item.Plays,
			"duration":  item.Duration,
			"items":     item.Items,
		}
		if includeIdentity {
			entry["uid"] = item.UID
			entry["username"] = firstNonEmpty(username, fmt.Sprintf("UID %d", item.UID))
		}
		userItems = append(userItems, entry)
	}

	return map[string]any{
		"range":      rangeKey,
		"since":      since,
		"updated_at": time.Now().Unix(),
		"summary": map[string]any{
			"plays":    totalPlays,
			"duration": totalDuration,
			"viewers":  uniqueUsers,
			"items":    len(mediaItems),
		},
		"media": mediaItems,
		"users": userItems,
	}
}

// handleV2PlayRank 是普通用户的榜单接口。路由级别就是 AuthUser：排行榜不向无账号
// 访客开放，未登录请求在鉴权层即被拒绝，这里只按配置区分"普通用户能不能看"。
func (a *App) handleV2PlayRank(w http.ResponseWriter, r *http.Request, _ Params) {
	cfg := a.cfg()
	if !cfg.PlayRankEnabled {
		failWithCode(w, http.StatusForbidden, ErrForbidden, "播放排行榜未启用")
		return
	}
	p := current(r)
	if p.User.Role != store.RoleAdmin && !cfg.PlayRankUserVisible {
		failWithCode(w, http.StatusForbidden, ErrForbidden, "排行榜未对普通用户开放")
		return
	}

	rangeKey, limit := playRankQuery(r)
	refresh := r.URL.Query().Get("refresh") == "1"
	ok(w, "OK", a.playRankData(rangeKey, limit, false, refresh))
}

// handleV2AdminPlayRank 是管理员榜单：带 uid 与完整用户名，且不受排行榜总开关
// 影响（管理员后台始终要能看到数据）。
func (a *App) handleV2AdminPlayRank(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	rangeKey, limit := playRankQuery(r)
	refresh := r.URL.Query().Get("refresh") == "1"
	data := a.playRankData(rangeKey, limit, true, refresh)
	data["enabled"] = a.cfg().PlayRankEnabled
	data["user_visible"] = a.cfg().PlayRankUserVisible
	ok(w, "OK", data)
}

// invalidatePlayRankCache 在数据同步后调用：新写入的播放记录必须立刻体现在榜单上，
// 否则管理员点了同步还要等缓存过期。
func (a *App) invalidatePlayRankCache() {
	a.playRankMu.Lock()
	a.playRankCache = map[string]playRankSnapshot{}
	a.playRankMu.Unlock()
}
