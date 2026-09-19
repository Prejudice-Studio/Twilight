package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestPlayRankEpisodesSkipMoviesAndUnknownItems(t *testing.T) {
	// Emby 不可用时 embyItemMetadata 返回空 map，这里不能报错也不能产出集数——
	// 榜单照常返回，只是少了集数标识。
	app := &App{}
	episodes := app.playRankEpisodes(context.Background(), []store.PlaybackMediaRank{
		{ItemID: "ep1", Title: "第八集"},
		{ItemID: "", Title: "没有 item_id 的一行"},
	})
	if len(episodes) != 0 {
		t.Fatalf("unexpected episodes without Emby metadata: %#v", episodes)
	}
	if got := app.playRankEpisodes(context.Background(), nil); len(got) != 0 {
		t.Fatalf("empty media should yield no episodes, got %#v", got)
	}
}

func TestPlayRankWindowCoversCalendarRangesAndAllTime(t *testing.T) {
	// 2026-03-17 是周二，因此周榜起点应为同周的周一 03-16。
	now := time.Date(2026, 3, 17, 15, 30, 0, 0, time.Local)
	local := time.Local

	if got, want := playRankWindow(now, playRankRangeDay), time.Date(2026, 3, 17, 0, 0, 0, 0, local).Unix(); got != want {
		t.Fatalf("day window=%d want %d", got, want)
	}
	if got, want := playRankWindow(now, playRankRangeWeek), time.Date(2026, 3, 16, 0, 0, 0, 0, local).Unix(); got != want {
		t.Fatalf("week window=%d want %d", got, want)
	}
	if got, want := playRankWindow(now, playRankRangeMonth), time.Date(2026, 3, 1, 0, 0, 0, 0, local).Unix(); got != want {
		t.Fatalf("month window=%d want %d", got, want)
	}
	// all 必须是不设下限的 0：store 层把 0 当作"不限时间"，榜单才能覆盖系统
	// 已记录的全部数据。任何非 0 起点都会把历史数据挡在窗外。
	if got := playRankWindow(now, playRankRangeAll); got != 0 {
		t.Fatalf("all window=%d want 0", got)
	}
}

func TestPlayRankQueryParsesRangeDaysAndLimit(t *testing.T) {
	now := time.Now()

	req := playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?range=all", nil))
	if req.rangeKey != playRankRangeAll || req.since != 0 || req.limit != playRankDefaultLimit {
		t.Fatalf("range=all parsed as %q since=%d limit=%d", req.rangeKey, req.since, req.limit)
	}

	req = playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?range=month", nil))
	wantMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
	if req.rangeKey != playRankRangeMonth || req.since != wantMonth {
		t.Fatalf("range=month parsed as %q since=%d want %d", req.rangeKey, req.since, wantMonth)
	}

	// days 是"过去 N 天"的滑动窗口，优先于 range，并成为缓存 key 的一部分。
	req = playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?range=week&days=30", nil))
	if req.rangeKey != "30d" {
		t.Fatalf("days should win over range, got %q", req.rangeKey)
	}
	wantDays := now.AddDate(0, 0, -30).Unix()
	if req.since < wantDays-5 || req.since > wantDays+5 {
		t.Fatalf("days=30 since=%d want ~%d", req.since, wantDays)
	}

	// 超长窗口夹到上限，避免一个请求把整表聚合拖垮。
	req = playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?days=99999", nil))
	if req.rangeKey != "730d" {
		t.Fatalf("days should be clamped, got %q", req.rangeKey)
	}

	// 非法取值一律回退默认，绝不把未过滤的参数带进 store 查询。
	req = playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?range=year&days=abc&limit=500", nil))
	wantDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	if req.rangeKey != playRankRangeDay || req.since != wantDay || req.limit != playRankMaxLimit {
		t.Fatalf("invalid input parsed as %q since=%d limit=%d", req.rangeKey, req.since, req.limit)
	}
}

func TestPlayRankQueryParsesGroupBy(t *testing.T) {
	// 默认（不带参数、空值、未知值）都必须是逐条明细：宁可退回明细，也不能把
	// 未经白名单校验的字符串带进 store 的 GROUP BY。
	for _, raw := range []string{"", "item", "series", "SERIES", "all", "; DROP TABLE x"} {
		// 必须转义：httptest.NewRequest 会把未编码的空格当成 HTTP 版本分隔符。
		req := playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?group_by="+url.QueryEscape(raw), nil))
		want := playRankGroupItem
		if raw == "series" {
			want = playRankGroupSeries
		}
		if req.groupBy != want {
			t.Fatalf("group_by=%q parsed as %q want %q", raw, req.groupBy, want)
		}
	}
}

// TestPlayRankQueryParsesSortBy 盯住排序口径的白名单：只有 plays / duration 两个
// 合法值，其余一律回退 plays。排序表达式要拼进 SQL 的 ORDER BY，未经校验的字符
// 串绝不能流到那里。
func TestPlayRankQueryParsesSortBy(t *testing.T) {
	for _, raw := range []string{"", "plays", "duration", "DURATION", "count", "; DROP TABLE x"} {
		req := playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?sort_by="+url.QueryEscape(raw), nil))
		want := playRankSortPlays
		if raw == "duration" {
			want = playRankSortDuration
		}
		if req.sortBy != want {
			t.Fatalf("sort_by=%q parsed as %q want %q", raw, req.sortBy, want)
		}
	}
}

// TestPlayRankEpisodesKeepsSeasonUnknownRatherThanGuessing 盯住"季号缺失"的口径：
// 后端必须如实交出 Season=0，不能替前端猜第几季，更不能拼成 "E8" 这种自造格式——
// 那是拿格式掩盖"其实不知道第几季"。没有集号的行（电影等）整条不收录。
func TestPlayRankEpisodesKeepsSeasonUnknownRatherThanGuessing(t *testing.T) {
	episodes := playRankEpisodesFromMetadata(map[string]embyItemMetadata{
		"s1e8":  {ID: "s1e8", Type: "Episode", ParentIndexNumber: 1, IndexNumber: 8},
		"nosed": {ID: "nosed", Type: "Episode", IndexNumber: 8},
		"movie": {ID: "movie", Type: "Movie"},
		"neg":   {ID: "neg", Type: "Episode", ParentIndexNumber: -2, IndexNumber: 3},
	})

	if got := episodes["s1e8"]; got.Season != 1 || got.Episode != 8 {
		t.Fatalf("season/episode not passed through: %+v", got)
	}
	// 季号缺失必须如实留 0：不能替前端猜第几季，更不能拼成 "E8" 那种自造格式。
	if got := episodes["nosed"]; got.Season != 0 || got.Episode != 8 {
		t.Fatalf("missing season must stay 0, not be guessed: %+v", got)
	}
	if got := episodes["neg"]; got.Season != 0 || got.Episode != 3 {
		t.Fatalf("negative season must normalize to 0: %+v", got)
	}
	// 电影没有集号，整条不收录——前端据此不渲染徽标。
	if _, ok := episodes["movie"]; ok {
		t.Fatalf("items without an episode number must be omitted: %#v", episodes)
	}
	if len(episodes) != 3 {
		t.Fatalf("unexpected episode count: %#v", episodes)
	}
}
