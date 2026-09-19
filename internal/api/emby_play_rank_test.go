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

func TestPlayRankEpisodeLabelsSkipMoviesAndUnknownItems(t *testing.T) {
	// Emby 不可用时 embyItemMetadata 返回空 map，这里不能报错也不能产出标识——
	// 榜单照常返回，只是少了 S1E8。
	app := &App{}
	labels := app.playRankEpisodeLabels(context.Background(), []store.PlaybackMediaRank{
		{ItemID: "ep1", Title: "第八集"},
		{ItemID: "", Title: "没有 item_id 的一行"},
	})
	if len(labels) != 0 {
		t.Fatalf("unexpected labels without Emby metadata: %#v", labels)
	}
	if got := app.playRankEpisodeLabels(context.Background(), nil); len(got) != 0 {
		t.Fatalf("empty media should yield no labels, got %#v", got)
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

	rangeKey, since, limit, _ := playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?range=all", nil))
	if rangeKey != playRankRangeAll || since != 0 || limit != playRankDefaultLimit {
		t.Fatalf("range=all parsed as %q since=%d limit=%d", rangeKey, since, limit)
	}

	rangeKey, since, _, _ = playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?range=month", nil))
	wantMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
	if rangeKey != playRankRangeMonth || since != wantMonth {
		t.Fatalf("range=month parsed as %q since=%d want %d", rangeKey, since, wantMonth)
	}

	// days 是"过去 N 天"的滑动窗口，优先于 range，并成为缓存 key 的一部分。
	rangeKey, since, _, _ = playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?range=week&days=30", nil))
	if rangeKey != "30d" {
		t.Fatalf("days should win over range, got %q", rangeKey)
	}
	wantDays := now.AddDate(0, 0, -30).Unix()
	if since < wantDays-5 || since > wantDays+5 {
		t.Fatalf("days=30 since=%d want ~%d", since, wantDays)
	}

	// 超长窗口夹到上限，避免一个请求把整表聚合拖垮。
	rangeKey, _, _, _ = playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?days=99999", nil))
	if rangeKey != "730d" {
		t.Fatalf("days should be clamped, got %q", rangeKey)
	}

	// 非法取值一律回退默认，绝不把未过滤的参数带进 store 查询。
	rangeKey, since, limit, _ = playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?range=year&days=abc&limit=500", nil))
	wantDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	if rangeKey != playRankRangeDay || since != wantDay || limit != playRankMaxLimit {
		t.Fatalf("invalid input parsed as %q since=%d limit=%d", rangeKey, since, limit)
	}
}

func TestPlayRankQueryParsesGroupBy(t *testing.T) {
	// 默认（不带参数、空值、未知值）都必须是逐条明细：宁可退回明细，也不能把
	// 未经白名单校验的字符串带进 store 的 GROUP BY。
	for _, raw := range []string{"", "item", "series", "SERIES", "all", "; DROP TABLE x"} {
		// 必须转义：httptest.NewRequest 会把未编码的空格当成 HTTP 版本分隔符。
		_, _, _, groupBy := playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?group_by="+url.QueryEscape(raw), nil))
		want := playRankGroupItem
		if raw == "series" {
			want = playRankGroupSeries
		}
		if groupBy != want {
			t.Fatalf("group_by=%q parsed as %q want %q", raw, groupBy, want)
		}
	}
}

func TestPlayRankEpisodeLabelFormatsSeasonAndEpisode(t *testing.T) {
	cases := []struct {
		season, episode int
		want            string
	}{
		{1, 8, "S1E8"},
		{12, 205, "S12E205"},
		// 只有集号：退化为 E8，总比什么都不显示强。
		{0, 8, "E8"},
		// 没有集号就没有"第几集"可言，季号再明确也不显示。
		{2, 0, ""},
		{2, -3, ""},
		// 电影：两个编号都没有。
		{0, 0, ""},
	}
	for _, item := range cases {
		if got := playRankEpisodeLabel(item.season, item.episode); got != item.want {
			t.Fatalf("playRankEpisodeLabel(%d, %d)=%q want %q", item.season, item.episode, got, item.want)
		}
	}
}
