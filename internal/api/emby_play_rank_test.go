package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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

	rangeKey, since, limit := playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?range=all", nil))
	if rangeKey != playRankRangeAll || since != 0 || limit != playRankDefaultLimit {
		t.Fatalf("range=all parsed as %q since=%d limit=%d", rangeKey, since, limit)
	}

	rangeKey, since, _ = playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?range=month", nil))
	wantMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
	if rangeKey != playRankRangeMonth || since != wantMonth {
		t.Fatalf("range=month parsed as %q since=%d want %d", rangeKey, since, wantMonth)
	}

	// days 是"过去 N 天"的滑动窗口，优先于 range，并成为缓存 key 的一部分。
	rangeKey, since, _ = playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?range=week&days=30", nil))
	if rangeKey != "30d" {
		t.Fatalf("days should win over range, got %q", rangeKey)
	}
	wantDays := now.AddDate(0, 0, -30).Unix()
	if since < wantDays-5 || since > wantDays+5 {
		t.Fatalf("days=30 since=%d want ~%d", since, wantDays)
	}

	// 超长窗口夹到上限，避免一个请求把整表聚合拖垮。
	rangeKey, _, _ = playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?days=99999", nil))
	if rangeKey != "730d" {
		t.Fatalf("days should be clamped, got %q", rangeKey)
	}

	// 非法取值一律回退默认，绝不把未过滤的参数带进 store 查询。
	rangeKey, since, limit = playRankQuery(httptest.NewRequest(http.MethodGet, "/play-rank?range=year&days=abc&limit=500", nil))
	wantDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	if rangeKey != playRankRangeDay || since != wantDay || limit != playRankMaxLimit {
		t.Fatalf("invalid input parsed as %q since=%d limit=%d", rangeKey, since, limit)
	}
}
