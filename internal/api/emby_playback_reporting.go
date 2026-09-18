package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

// 本文件是播放记录的第二个数据源：Emby 的 Playback Reporting 插件。
//
// 为什么要有第二个源：活动日志只能配对出"停止时刻 − 开始时刻"的墙上时钟差，
// 用户中途暂停、挂机，这些时间全算进播放时长。插件的 PlaybackActivity 表则记了
// PlayDuration 与 PauseDuration，两者相减才是真正看进去的净时长。
//
// 插件没装时这里必须完全静默——每次同步都去探测、失败就记一条 warn，日志很快
// 会被刷满，管理员也就看不见真正的故障了。

const (
	// playbackReportingProbeTTL 是探测结果的缓存时长。探测要往 Emby 发一次自定义
	// SQL，没必要每次同步都做；插件装卸是低频操作，十分钟足够。
	playbackReportingProbeTTL = 10 * time.Minute

	// playbackReportingMaxRows 限制单次同步取回的行数：窗口开得很大（30 天）时，
	// 一次拖回几十万行既拖垮 Emby 也拖垮自己的写入。
	playbackReportingMaxRows = 5000

	// playbackReportingTimeLayout 与插件写入 SQLite 的时间格式一致。插件存的是
	// TEXT 时间戳，字符串比较必须与它的格式对齐，否则 WHERE 直接落空。
	playbackReportingTimeLayout = "2006-01-02 15:04:05"

	playbackReportingQueryPath = "/emby/user_usage_stats/submit_custom_query"
)

// playbackReportingQueryResponse 是 submit_custom_query 的返回结构。
//
// 列名那个键插件拼成了 "colums"（少一个 n）——Sakura EmbyBoss 也是按这个拼写取
// 的。两个拼写都接，免得插件哪天改正之后我们反而解析不出来。results 是二维数组，
// 值可能是字符串也可能是数字，逐格按位置取（SQL 由本文件拼，列顺序是已知的）。
type playbackReportingQueryResponse struct {
	Columns []string `json:"columns"`
	Colums  []string `json:"colums"`
	Results [][]any  `json:"results"`
}

type playbackReportingRow struct {
	UserID        string
	ItemID        string
	ItemType      string
	ItemName      string
	PlayDuration  int64
	PauseDuration int64
	PlayedAt      int64
}

// playbackReportingEnabled 报告这条数据源是否被允许使用。开关默认开，真正的门槛
// 是探测：没装插件时 Emby 会拒绝这个端点，同步流程照旧走活动日志。
func (a *App) playbackReportingEnabled() bool {
	return a.cfg().PlaybackReportingEnabled && a.embyConfigured()
}

// playbackReportingAvailable 探测插件是否可用，结果缓存十分钟。
func (a *App) playbackReportingAvailable(ctx context.Context) bool {
	if !a.playbackReportingEnabled() {
		return false
	}
	a.playbackReportingMu.Lock()
	if time.Now().Before(a.playbackReportingUntil) {
		ready := a.playbackReportingReady
		a.playbackReportingMu.Unlock()
		return ready
	}
	a.playbackReportingMu.Unlock()

	ready, probeErr := a.probePlaybackReporting(ctx)

	a.playbackReportingMu.Lock()
	a.playbackReportingUntil = time.Now().Add(playbackReportingProbeTTL)
	a.playbackReportingReady = ready
	if probeErr != nil {
		a.playbackReportingLastError = probeErr.Error()
	} else {
		a.playbackReportingLastError = ""
	}
	a.playbackReportingMu.Unlock()
	return ready
}

// probePlaybackReporting 用一条不会对数据产生副作用的聚合查询试水。表不存在、
// 端点不存在、鉴权失败都表现为请求错误，一律按"不可用"处理——调用方会照常回退
// 活动日志，不需要区分失败原因。
//
// 失败原因要留在 playbackReportingLastError 里：插件确实装了但探测不过时，管理员
// 在界面上看到"活动日志"却无从下手，只能去翻日志。日志只记 Debug（插件没装是常态，
// 每轮同步写一条 warn 会淹没真正的故障），界面那一条才是给人看的。
func (a *App) probePlaybackReporting(ctx context.Context) (bool, error) {
	var resp playbackReportingQueryResponse
	err := a.embyPost(ctx, playbackReportingQueryPath, map[string]any{
		"CustomQueryString": "SELECT COUNT(*) FROM PlaybackActivity",
		"ReplaceUserId":     false,
	}, &resp)
	if err != nil {
		zap.L().Debug("playback reporting plugin not available", zap.Error(err))
		return false, err
	}
	return true, nil
}

// syncPlaybackReporting 把插件在 [since, until) 里的播放行落到播放记录表，
// 返回修正的条数与新增的条数。
//
// 每一步失败都返回 (0, 0, nil) 而不是 error：这条链路是增强，不是关键路径。让它
// 失败冒泡会打断活动日志同步的调度任务，而管理员真正需要的是"榜单照常有数据，
// 只是时长口径差一点"。
func (a *App) syncPlaybackReporting(ctx context.Context, since, until time.Time) (int, int, error) {
	if !a.playbackReportingAvailable(ctx) {
		return 0, 0, nil
	}
	rows, err := a.fetchPlaybackReportingRows(ctx, since, until)
	if err != nil || len(rows) == 0 {
		return 0, 0, nil
	}

	userKeys := make([]string, 0, len(rows))
	seenKey := map[string]struct{}{}
	for _, row := range rows {
		key := normalizeEmbyActivityUserKey(row.UserID)
		if key == "" {
			continue
		}
		if _, ok := seenKey[key]; ok {
			continue
		}
		seenKey[key] = struct{}{}
		userKeys = append(userKeys, key)
	}
	// 插件给的是 Emby 内部 UserId，要换成 Twilight 的 UID。匹配规则与活动日志
	// 那条链路保持一致（EmbyID / Emby 用户名 / 站点用户名）。
	matched := a.store().UsersMatching(len(userKeys), func(user store.User) bool {
		for _, key := range []string{user.EmbyID, user.EmbyUsername, user.Username} {
			if normalized := normalizeEmbyActivityUserKey(key); normalized != "" {
				if _, ok := seenKey[normalized]; ok {
					return true
				}
			}
		}
		return false
	})
	usersByKey := embyActivityUsersByKey(matched)

	itemIDs := make([]string, 0, len(rows))
	seenItem := map[string]struct{}{}
	for _, row := range rows {
		id := strings.TrimSpace(row.ItemID)
		if id == "" || !validEmbyItemID(id) {
			continue
		}
		if _, ok := seenItem[id]; ok {
			continue
		}
		seenItem[id] = struct{}{}
		itemIDs = append(itemIDs, id)
	}
	metadata := a.embyItemMetadata(ctx, itemIDs)

	pending := make([]store.PlaybackRecord, 0, len(rows))
	for _, row := range rows {
		user := usersByKey[normalizeEmbyActivityUserKey(row.UserID)]
		if user.UID == 0 {
			continue
		}
		if row.PlayedAt <= 0 {
			continue
		}
		itemID := strings.TrimSpace(row.ItemID)
		if itemID == "" {
			continue
		}
		meta := metadata[itemID]
		resolvedID := firstNonEmpty(meta.ID, itemID)
		title := firstNonEmpty(meta.Name, row.ItemName, itemID)
		seriesName := strings.TrimSpace(meta.SeriesName)
		mediaType := strings.ToLower(strings.TrimSpace(firstNonEmpty(meta.Type, row.ItemType)))
		if mediaType == "episode" || mediaType == "series" {
			title = firstNonEmpty(meta.Name, row.ItemName, itemID)
			seriesName = firstNonEmpty(meta.SeriesName, meta.Name)
			mediaType = "episode"
		} else if mediaType == "" {
			mediaType = "unknown"
		}
		// 净时长才是这里的目的：暂停掉的时间不算观看。为 0 或负数时退回毛时长，
		// 免得把一次真实播放记成 0 秒。
		wall := row.PlayDuration
		if wall < 0 {
			wall = 0
		}
		net := wall - row.PauseDuration
		if net <= 0 {
			net = wall
		}
		pending = append(pending, store.PlaybackRecord{
			UID:          user.UID,
			ItemID:       resolvedID,
			Title:        title,
			SeriesName:   seriesName,
			MediaType:    mediaType,
			Duration:     net,
			PlayedAt:     row.PlayedAt,
			Source:       store.PlaybackSourceReporting,
			WallDuration: wall,
		})
	}
	if len(pending) == 0 {
		return 0, 0, nil
	}
	updated, inserted, err := a.store().ApplyPlaybackReportingRecords(pending)
	if err != nil {
		zap.L().Warn("failed to apply playback reporting records", zap.Error(err))
		return 0, 0, nil
	}
	if updated > 0 || inserted > 0 {
		a.invalidatePlayRankCache()
	}
	a.playbackReportingMu.Lock()
	a.playbackReportingSyncedAt = time.Now().Unix()
	a.playbackReportingMu.Unlock()
	return updated, inserted, nil
}

// fetchPlaybackReportingRows 取 [since, until) 内的播放行。
//
// SQL 完全由本函数拼装：时间来自 time.Time 的格式化输出，不含任何用户输入，
// 所以不存在注入面。反过来，这也意味着绝不能把外部参数拼进这条语句。
func (a *App) fetchPlaybackReportingRows(ctx context.Context, since, until time.Time) ([]playbackReportingRow, error) {
	query := fmt.Sprintf(`SELECT UserId, ItemId, ItemType, ItemName, PlayDuration, PauseDuration, DateCreated
FROM PlaybackActivity
WHERE DateCreated >= '%s' AND DateCreated < '%s'
LIMIT %d`,
		since.Format(playbackReportingTimeLayout), until.Format(playbackReportingTimeLayout), playbackReportingMaxRows)
	var resp playbackReportingQueryResponse
	if err := a.embyPost(ctx, playbackReportingQueryPath, map[string]any{
		"CustomQueryString": query,
		"ReplaceUserId":     false,
	}, &resp); err != nil {
		return nil, err
	}
	return parsePlaybackReportingRows(resp), nil
}

func parsePlaybackReportingRows(resp playbackReportingQueryResponse) []playbackReportingRow {
	rows := make([]playbackReportingRow, 0, len(resp.Results))
	for _, raw := range resp.Results {
		if len(raw) < 7 {
			continue
		}
		rows = append(rows, playbackReportingRow{
			UserID:        playbackReportingString(raw[0]),
			ItemID:        playbackReportingString(raw[1]),
			ItemType:      playbackReportingString(raw[2]),
			ItemName:      playbackReportingString(raw[3]),
			PlayDuration:  numeric(raw[4]),
			PauseDuration: numeric(raw[5]),
			PlayedAt:      parsePlaybackReportingTime(playbackReportingString(raw[6])),
		})
	}
	return rows
}

// playbackReportingString 兼容插件返回的字符串与数字两种形态。
func playbackReportingString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return fmt.Sprintf("%d", int64(typed))
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

// parsePlaybackReportingTime 解析插件写入的时间戳。插件的 SQLite 列是 TEXT，常见
// 是 "2006-01-02 15:04:05"，但不同版本也可能带毫秒或写成 ISO，这里都接住。
// 解析不出来时返回 0，调用方会跳过该行——一条时间不对的记录比没有记录更糟。
func parsePlaybackReportingTime(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	layouts := []string{
		playbackReportingTimeLayout,
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05Z07:00",
		time.RFC3339,
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed.Unix()
		}
	}
	return 0
}

// playbackReportingStatus 给管理接口用：说明当前用的是哪个数据源。
func (a *App) playbackReportingStatus(ctx context.Context) map[string]any {
	status := map[string]any{
		"enabled":   a.cfg().PlaybackReportingEnabled,
		"available": false,
	}
	a.playbackReportingMu.Lock()
	status["last_error"] = a.playbackReportingLastError
	status["last_sync_at"] = a.playbackReportingSyncedAt
	a.playbackReportingMu.Unlock()
	if !a.playbackReportingEnabled() {
		return status
	}
	status["available"] = a.playbackReportingAvailable(ctx)
	// 可用状态下再读一次：available 为 true 说明刚刚那次探测成功，last_error
	// 可能还是上一次失败留下的残值，要清掉。
	a.playbackReportingMu.Lock()
	status["last_error"] = a.playbackReportingLastError
	status["last_sync_at"] = a.playbackReportingSyncedAt
	a.playbackReportingMu.Unlock()
	return status
}
