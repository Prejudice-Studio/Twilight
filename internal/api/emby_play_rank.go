package api

import (
	"context"
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
	playRankRangeDay   = "day"
	playRankRangeWeek  = "week"
	playRankRangeMonth = "month"
	// playRankRangeAll 是"系统已记录到的全部数据"：起点取 0，store 层把 0 视为
	// 不设下限，于是榜单覆盖整张播放记录表，而不是只覆盖一个日历窗口。
	playRankRangeAll = "all"

	playRankDefaultLimit = 20
	playRankMaxLimit     = 100

	// playRankMaxDays 是 days= 的上限。自定义窗口要能覆盖"很久以前"，但也不能
	// 让一个请求把整表聚合拖垮——上限取两年，与活动日志的最长同步窗口同量级。
	playRankMaxDays = 730

	// playRankCacheTTL 让榜单在有人反复刷新页面时不至于每次都跑一遍聚合查询。
	playRankCacheTTL = 60 * time.Second
)

// playRankSnapshot 是榜单缓存条目：until 之前可以直接复用 data。
type playRankSnapshot struct {
	until time.Time
	data  map[string]any
}

// playRankWindow 返回榜单起点（Unix 秒），都用服务器本地时区——与管理员在后台
// 看到的时间口径一致：
//   - day：今天 00:00
//   - week：本周一 00:00
//   - month：本月 1 号 00:00
//   - all：0（不设下限，覆盖系统已记录的全部数据）
func playRankWindow(now time.Time, rangeKey string) int64 {
	switch rangeKey {
	case playRankRangeAll:
		return 0
	case playRankRangeMonth:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
	case playRankRangeWeek:
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		// time.Weekday 以周日为 0，换算成"周一为一周之始"的回退天数。
		offset := int(now.Weekday()) - 1
		if offset < 0 {
			offset = 6
		}
		return start.AddDate(0, 0, -offset).Unix()
	default:
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	}
}

// 媒体榜的分组维度：item 是逐集/逐部，series 是把一部剧的所有集合成一行。
const (
	playRankGroupItem   = "item"
	playRankGroupSeries = "series"
)

// playRankQuery 解析并夹取 range / days / limit / group_by。非法取值一律回退默认，
// 避免把未过滤的参数带进 store 查询。
//
// days 是"过去 N 天"的滑动窗口，与日/周/月这些日历窗口互补：运营想看"最近 30
// 天"时不必等到月末。显式给出 days 时它优先于 range，并成为缓存 key 的一部分。
func playRankQuery(r *http.Request) (string, int64, int, string) {
	query := r.URL.Query()
	rangeKey := playRankRangeDay
	switch query.Get("range") {
	case playRankRangeWeek:
		rangeKey = playRankRangeWeek
	case playRankRangeMonth:
		rangeKey = playRankRangeMonth
	case playRankRangeAll:
		rangeKey = playRankRangeAll
	}
	now := time.Now()
	since := playRankWindow(now, rangeKey)
	if raw := query.Get("days"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			if parsed > playRankMaxDays {
				parsed = playRankMaxDays
			}
			rangeKey = strconv.Itoa(parsed) + "d"
			since = now.AddDate(0, 0, -parsed).Unix()
		}
	}
	limit := playRankDefaultLimit
	if raw := query.Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > playRankMaxLimit {
		limit = playRankMaxLimit
	}
	groupBy := playRankGroupItem
	if query.Get("group_by") == playRankGroupSeries {
		groupBy = playRankGroupSeries
	}
	return rangeKey, since, limit, groupBy
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
func (a *App) playRankData(ctx context.Context, rangeKey string, since int64, limit int, groupBy string, includeIdentity bool, refresh bool) map[string]any {
	key := rangeKey + "|" + strconv.FormatInt(since, 10) + "|" + strconv.Itoa(limit) + "|" + groupBy + "|" + strconv.FormatBool(includeIdentity)

	a.playRankMu.Lock()
	if !refresh {
		if cached, ok := a.playRankCache[key]; ok && time.Now().Before(cached.until) {
			a.playRankMu.Unlock()
			return cached.data
		}
	}
	a.playRankMu.Unlock()

	data := a.buildPlayRank(ctx, rangeKey, since, limit, groupBy, includeIdentity)

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

func (a *App) buildPlayRank(ctx context.Context, rangeKey string, since int64, limit int, groupBy string, includeIdentity bool) map[string]any {
	media, users, _ := a.store().PlaybackRank(since, limit, groupBy)
	totalPlays, totalDuration, uniqueUsers, _ := a.store().PlaybackRecordSummary(since)
	// 覆盖面不受窗口影响：它回答的是"系统一共记录了多少、最早记到什么时候"，
	// 前端拿它提示用户还能往回看多远，而不是当前窗口里有多少条。
	totalRecords, earliestAt, latestAt, _ := a.store().PlaybackRecordCoverage()

	// 季号/集号不落在播放记录表里，而是每次构建榜单时向 Emby 批量取回来补上。
	// 这样历史记录不需要迁移就能显示 S1E8；Emby 不可用时拿不到编号，只是少了
	// 一个标识，榜单本身照常返回。
	episodes := a.playRankEpisodeLabels(ctx, media)

	mediaItems := make([]map[string]any, 0, len(media))
	for _, item := range media {
		entry := map[string]any{
			"item_id":     item.ItemID,
			"title":       firstNonEmpty(item.Title, item.ItemID),
			"series_name": item.SeriesName,
			"media_type":  firstNonEmpty(item.MediaType, "unknown"),
			"plays":       item.Plays,
			"duration":    item.Duration,
			"viewers":     item.Viewers,
			"episodes":    item.Episodes,
		}
		if label := episodes[item.ItemID]; label != "" {
			entry["episode_label"] = label
		}
		mediaItems = append(mediaItems, entry)
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
		"group_by":   groupBy,
		"updated_at": time.Now().Unix(),
		"summary": map[string]any{
			"plays":    totalPlays,
			"duration": totalDuration,
			"viewers":  uniqueUsers,
			"items":    len(mediaItems),
		},
		// recorded 是整库的覆盖面，与上面 summary（当前窗口）刻意区分开：
		// 窗口为空但库里有历史时，前端要靠它告诉用户"往回切能看到东西"。
		"recorded": map[string]any{
			"total":    totalRecords,
			"earliest": earliestAt,
			"latest":   latestAt,
		},
		"media": mediaItems,
		"users": userItems,
	}
}

// playRankEpisodeLabel 拼出界面上的集数标识，形如 S1E8。
//
// 两个编号都缺失（电影、音乐、或 Emby 没给）时返回空串——调用方据此决定要不要
// 带这个字段，前端也就不会渲染出一个空的徽标。只有集号没有季号时退化为 E8，
// 反过来只有季号没有集号没有意义（不知道是这季的哪一集），一律不显示。
func playRankEpisodeLabel(season, episode int) string {
	if episode <= 0 {
		return ""
	}
	if season <= 0 {
		return "E" + strconv.Itoa(episode)
	}
	return "S" + strconv.Itoa(season) + "E" + strconv.Itoa(episode)
}

// playRankEpisodeLabels 批量补上媒体榜每一行的集数标识，返回 item_id -> 标识。
//
// 只在 item 模式下有意义：series 模式一行代表整部剧，本来就没有"第几集"可言。
// 这里最多几百个 id，embyItemMetadata 内部按 100 个一批去问，通常一次请求搞定；
// 榜单本身有 60 秒缓存，不会每次刷新都打到 Emby。
func (a *App) playRankEpisodeLabels(ctx context.Context, media []store.PlaybackMediaRank) map[string]string {
	labels := make(map[string]string, len(media))
	if len(media) == 0 {
		return labels
	}
	ids := make([]string, 0, len(media))
	for _, item := range media {
		if item.ItemID == "" {
			continue
		}
		ids = append(ids, item.ItemID)
	}
	if len(ids) == 0 {
		return labels
	}
	for id, meta := range a.embyItemMetadata(ctx, ids) {
		if label := playRankEpisodeLabel(meta.ParentIndexNumber, meta.IndexNumber); label != "" {
			labels[id] = label
		}
	}
	return labels
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

	rangeKey, since, limit, groupBy := playRankQuery(r)
	refresh := r.URL.Query().Get("refresh") == "1"
	ok(w, "OK", a.playRankData(r.Context(), rangeKey, since, limit, groupBy, false, refresh))
}

// handleV2AdminPlayRank 是管理员榜单：带 uid 与完整用户名，且不受排行榜总开关
// 影响（管理员后台始终要能看到数据）。
func (a *App) handleV2AdminPlayRank(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	rangeKey, since, limit, groupBy := playRankQuery(r)
	refresh := r.URL.Query().Get("refresh") == "1"
	data := a.playRankData(r.Context(), rangeKey, since, limit, groupBy, true, refresh)
	data["enabled"] = a.cfg().PlayRankEnabled
	data["user_visible"] = a.cfg().PlayRankUserVisible
	// 管理员要能确认自己看到的时长到底是净时长还是墙上时钟差，否则"排行榜数字
	// 和 Emby 对不上"根本无从排查。available 走探测结果的 10 分钟缓存，不会每次
	// 刷新都去问 Emby。
	data["playback_reporting"] = a.playbackReportingStatus(r.Context())
	ok(w, "OK", data)
}

// invalidatePlayRankCache 在数据同步后调用：新写入的播放记录必须立刻体现在榜单上，
// 否则管理员点了同步还要等缓存过期。
func (a *App) invalidatePlayRankCache() {
	a.playRankMu.Lock()
	a.playRankCache = map[string]playRankSnapshot{}
	a.playRankMu.Unlock()
}
