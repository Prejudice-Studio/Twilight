package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPlaybackRecordsFiltersDefaultsAndLimits(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	records := []PlaybackRecord{
		{UID: 1, ItemID: "old", PlayedAt: 10},
		{UID: 2, ItemID: "other", PlayedAt: 20},
		{UID: 1, ItemID: "mid", PlayedAt: 30},
		{UID: 1, ItemID: "new", PlayedAt: 40},
	}
	for _, record := range records {
		if err := st.AddPlaybackRecord(record); err != nil {
			t.Fatal(err)
		}
	}

	uidRecords := st.PlaybackRecords(1, 0, 10)
	if len(uidRecords) != 3 || uidRecords[0].ItemID != "new" || uidRecords[2].ItemID != "old" {
		t.Fatalf("unexpected uid records: %#v", uidRecords)
	}
	counts := st.PlaybackRecordCounts([]int64{1, 2, 999})
	if counts[1] != 3 || counts[2] != 1 || counts[999] != 0 {
		t.Fatalf("unexpected playback counts: %#v", counts)
	}

	recent := st.PlaybackRecords(1, 25, 1)
	if len(recent) != 1 || recent[0].ItemID != "new" {
		t.Fatalf("unexpected recent limited records: %#v", recent)
	}

	if err := st.AddPlaybackRecord(PlaybackRecord{UID: 3, ItemID: "default-time"}); err != nil {
		t.Fatal(err)
	}
	defaulted := st.PlaybackRecords(3, 0, 1)
	if len(defaulted) != 1 || defaulted[0].PlayedAt == 0 {
		t.Fatalf("expected PlayedAt default, got %#v", defaulted)
	}
}

// TestPlaybackRecordCoverageReportsStoredExtent 盯住"系统到底记录了多少"这个口径：
// 榜单窗口可以很窄，但覆盖面必须是整库的，否则前端没法告诉用户还能往回看多远。
// TestPlaybackRankGroupBySeriesMergesEpisodes 盯住"按整部剧聚合"这个口径：一部
// 剧的两集必须并成一行、Episodes 记为 2，而电影因为没有剧名只能自己成一行。
// 反过来，逐条模式下每一行都必须是单个 item（Episodes 恒为 1）。
func TestPlaybackRankGroupBySeriesMergesEpisodes(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for _, record := range []PlaybackRecord{
		{UID: 1, ItemID: "ep1", Title: "第一集", SeriesName: "长征", MediaType: "episode", PlayedAt: 100, Duration: 60},
		{UID: 2, ItemID: "ep2", Title: "第二集", SeriesName: "长征", MediaType: "episode", PlayedAt: 200, Duration: 60},
		{UID: 1, ItemID: "movie", Title: "某部电影", MediaType: "movie", PlayedAt: 300, Duration: 90},
	} {
		if err := st.AddPlaybackRecord(record); err != nil {
			t.Fatal(err)
		}
	}

	byItem, _, err := st.PlaybackRank(PlaybackRankOptions{Limit: 10, GroupBy: PlaybackRankGroupItem})
	if err != nil {
		t.Fatal(err)
	}
	if len(byItem) != 3 {
		t.Fatalf("item mode should keep one row per item, got %d: %#v", len(byItem), byItem)
	}
	for _, item := range byItem {
		if item.Episodes != 1 {
			t.Fatalf("item mode Episodes should always be 1, got %d for %+v", item.Episodes, item)
		}
		if item.ItemID == "" {
			t.Fatalf("item mode must keep item_id for episode lookup, got %+v", item)
		}
	}

	bySeries, _, err := st.PlaybackRank(PlaybackRankOptions{Limit: 10, GroupBy: PlaybackRankGroupSeries})
	if err != nil {
		t.Fatal(err)
	}
	if len(bySeries) != 2 {
		t.Fatalf("series mode should merge the two episodes into one row, got %d: %#v", len(bySeries), bySeries)
	}
	// 两集合计 2 次播放，排在最前。
	top := bySeries[0]
	if top.Title != "长征" || top.Plays != 2 || top.Episodes != 2 || top.Viewers != 2 {
		t.Fatalf("unexpected top series row: %+v", top)
	}
	// series 模式下这一行代表整部剧，不该再带单集的 item_id / series_name。
	if top.ItemID != "" || top.SeriesName != "" {
		t.Fatalf("series row must not carry per-episode identity: %+v", top)
	}
	if bySeries[1].Title != "某部电影" || bySeries[1].Episodes != 1 {
		t.Fatalf("unexpected movie row: %+v", bySeries[1])
	}

	// 未知分组值必须退回逐条明细，绝不能把未过滤的字符串带进 GROUP BY。
	fallback, _, err := st.PlaybackRank(PlaybackRankOptions{Limit: 10, GroupBy: "series; DROP TABLE twilight_playback_records"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fallback) != 3 {
		t.Fatalf("unknown group must fall back to item mode, got %d rows", len(fallback))
	}
}

// TestPlaybackRankSortBySeparatesPlaysFromDuration 盯住"次数"和"时长"是两套名次：
// 一段 10 分钟的短视频刷 5 次（50 分钟）与一部 3 小时电影看 1 次（180 分钟），
// 按次数排短视频第一，按时长排电影第一。两个榜（媒体 / 用户）都必须跟着 sortBy
// 走——此前媒体榜写死按次数、用户榜写死按时长，两边"第一名"根本不是同一个口径。
func TestPlaybackRankSortBySeparatesPlaysFromDuration(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// clip: 5 次 × 600 秒 = 3000 秒；movie: 1 次 × 10800 秒 = 10800 秒。
	for _, record := range []PlaybackRecord{
		{UID: 1, ItemID: "clip", Title: "短视频", MediaType: "movie", PlayedAt: 100, Duration: 600},
		{UID: 1, ItemID: "clip", Title: "短视频", MediaType: "movie", PlayedAt: 101, Duration: 600},
		{UID: 1, ItemID: "clip", Title: "短视频", MediaType: "movie", PlayedAt: 102, Duration: 600},
		{UID: 1, ItemID: "clip", Title: "短视频", MediaType: "movie", PlayedAt: 103, Duration: 600},
		{UID: 1, ItemID: "clip", Title: "短视频", MediaType: "movie", PlayedAt: 104, Duration: 600},
		{UID: 2, ItemID: "movie", Title: "长片", MediaType: "movie", PlayedAt: 200, Duration: 10800},
	} {
		if err := st.AddPlaybackRecord(record); err != nil {
			t.Fatal(err)
		}
	}

	byPlays, usersByPlays, err := st.PlaybackRank(PlaybackRankOptions{Limit: 10, SortBy: PlaybackRankSortPlays})
	if err != nil {
		t.Fatal(err)
	}
	if byPlays[0].Title != "短视频" || byPlays[0].Plays != 5 {
		t.Fatalf("sort by plays must rank the frequently replayed item first, got %+v", byPlays[0])
	}
	if len(usersByPlays) < 2 || usersByPlays[0].UID != 1 {
		t.Fatalf("user board must follow sort_by=plays too, got %+v", usersByPlays)
	}

	byDuration, usersByDuration, err := st.PlaybackRank(PlaybackRankOptions{Limit: 10, SortBy: PlaybackRankSortDuration})
	if err != nil {
		t.Fatal(err)
	}
	if byDuration[0].Title != "长片" || byDuration[0].Plays != 1 {
		t.Fatalf("sort by duration must rank the long item first, got %+v", byDuration[0])
	}
	if len(usersByDuration) < 2 || usersByDuration[0].UID != 2 {
		t.Fatalf("user board must follow sort_by=duration too, got %+v", usersByDuration)
	}

	// series 模式的 ORDER BY 用的是列序号兜底键（"2 ASC"），与 item 模式的
	// item_id 不同，得单独跑一次确认 SQL 有效。
	seriesByDuration, _, err := st.PlaybackRank(PlaybackRankOptions{
		Limit:   10,
		GroupBy: PlaybackRankGroupSeries,
		SortBy:  PlaybackRankSortDuration,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seriesByDuration) == 0 || seriesByDuration[0].Title != "长片" {
		t.Fatalf("series mode with duration sort failed: %+v", seriesByDuration)
	}

	// 未知取值回退 plays，绝不能把未过滤的字符串带进 ORDER BY。
	fallback, _, err := st.PlaybackRank(PlaybackRankOptions{Limit: 10, SortBy: "duration; DROP TABLE twilight_playback_records"})
	if err != nil {
		t.Fatal(err)
	}
	if fallback[0].Title != "短视频" {
		t.Fatalf("unknown sort must fall back to plays, got %+v", fallback[0])
	}
	// 同口径下两种排序必须是同一批数据的两种排列，条数不能变。
	if len(byPlays) != len(byDuration) {
		t.Fatalf("sorting must not change the row set: %d vs %d", len(byPlays), len(byDuration))
	}
}

func TestPlaybackRecordCoverageReportsStoredExtent(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	total, earliest, latest, err := st.PlaybackRecordCoverage()
	if err != nil || total != 0 || earliest != 0 || latest != 0 {
		t.Fatalf("empty coverage should be zeroed: total=%d earliest=%d latest=%d err=%v", total, earliest, latest, err)
	}

	for _, record := range []PlaybackRecord{
		{UID: 1, ItemID: "a", PlayedAt: 300},
		{UID: 2, ItemID: "b", PlayedAt: 100},
		{UID: 1, ItemID: "c", PlayedAt: 200},
	} {
		if err := st.AddPlaybackRecord(record); err != nil {
			t.Fatal(err)
		}
	}

	total, earliest, latest, err = st.PlaybackRecordCoverage()
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || earliest != 100 || latest != 300 {
		t.Fatalf("unexpected coverage: total=%d earliest=%d latest=%d", total, earliest, latest)
	}
}

// TestApplyPlaybackReportingUpdatesSameSessionInsteadOfAddingRow 盯住这条链路最
// 容易出事的地方：插件和活动日志都记录了同一场播放。若不去重，同一场播放在榜单
// 里会变成两次，播放次数直接翻倍。
func TestApplyPlaybackReportingUpdatesSameSessionInsteadOfAddingRow(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// 活动日志先写一行：停止时刻 1000，墙上时长 3600（其中 1200 秒是暂停）。
	if err := st.AddPlaybackRecord(PlaybackRecord{
		UID: 1, ItemID: "item-1", Duration: 3600, PlayedAt: 1000, Source: PlaybackSourceActivityLog,
	}); err != nil {
		t.Fatal(err)
	}

	// 插件这一行的时间戳比活动日志早 8 小时（时区记法不同），净时长 2400。
	matched, inserted, err := st.ApplyPlaybackReportingRecords([]PlaybackRecord{{
		UID: 1, ItemID: "item-1", Duration: 2400, PlayedAt: 1000 - 8*3600,
		WallDuration: 3600, Source: PlaybackSourceReporting,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if matched != 1 || inserted != 0 {
		t.Fatalf("same session must be corrected, not duplicated: matched=%d inserted=%d", matched, inserted)
	}
	records := st.PlaybackRecords(1, 0, 10)
	if len(records) != 1 {
		t.Fatalf("expected one row, got %#v", records)
	}
	if records[0].Duration != 2400 {
		t.Fatalf("duration should be the net 2400, got %d", records[0].Duration)
	}
	if records[0].Source != PlaybackSourceReporting {
		t.Fatalf("source should be upgraded to reporting, got %q", records[0].Source)
	}
}

// TestApplyPlaybackReportingAppendsUnknownSessionAndStaysIdempotent 覆盖另外两种
// 情况：活动日志没记到的播放要补进来；同一批数据重放时既不该新增也不该重复修正。
func TestApplyPlaybackReportingAppendsUnknownSessionAndStaysIdempotent(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	reporting := []PlaybackRecord{{
		UID: 7, ItemID: "item-7", Title: "仅插件记到的一场", Duration: 1800, PlayedAt: 5000,
		WallDuration: 2000, Source: PlaybackSourceReporting,
	}}
	matched, inserted, err := st.ApplyPlaybackReportingRecords(reporting)
	if err != nil {
		t.Fatal(err)
	}
	if matched != 0 || inserted != 1 {
		t.Fatalf("unknown session must be inserted: matched=%d inserted=%d", matched, inserted)
	}

	// 重放同一批：行数不变，也不该再报一次新增。
	matched, inserted, err = st.ApplyPlaybackReportingRecords(reporting)
	if err != nil {
		t.Fatal(err)
	}
	records := st.PlaybackRecords(7, 0, 10)
	if len(records) != 1 {
		t.Fatalf("replay must not duplicate rows, got %#v", records)
	}
	if inserted != 0 {
		t.Fatalf("replay must not report inserts, got %d", inserted)
	}
	if matched != 0 {
		t.Fatalf("rows already corrected must not be matched again, got %d", matched)
	}
}

func TestBangumiSyncSuccessCountsUseRecentHundredPerUser(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	st.mu.Lock()
	for i := 0; i < 101; i++ {
		st.state.BangumiSyncLogs = append(st.state.BangumiSyncLogs, BangumiSyncLog{UID: 1, Status: "success"})
	}
	st.state.BangumiSyncLogs = append(st.state.BangumiSyncLogs, BangumiSyncLog{UID: 1, Status: "failed"})
	st.state.BangumiSyncLogs = append(st.state.BangumiSyncLogs, BangumiSyncLog{UID: 2, Status: "success"})
	st.mu.Unlock()

	// 窗口是"最近 100 条同步日志"而不是"最近 100 条成功日志"：uid=1 最新的那条
	// 是 failed，它同样占掉一个窗口名额，所以成功数封顶在 99。
	counts := st.BangumiSyncSuccessCounts([]int64{1, 2})
	if counts[1] != 99 || counts[2] != 1 {

		t.Fatalf("unexpected Bangumi success counts: %#v", counts)
	}
}

func TestPlaybackRecordsPruneToMax(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	st.mu.Lock()
	st.state.PlaybackRecords = make([]PlaybackRecord, maxStoredPlaybackRecords)
	for i := range st.state.PlaybackRecords {
		st.state.PlaybackRecords[i] = PlaybackRecord{UID: 1, ItemID: "bulk", PlayedAt: int64(maxStoredPlaybackRecords - i)}
	}
	st.mu.Unlock()
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPlaybackRecord(PlaybackRecord{UID: 1, ItemID: "new", PlayedAt: int64(maxStoredPlaybackRecords + 1)}); err != nil {
		t.Fatal(err)
	}
	if got := len(st.state.PlaybackRecords); got != maxStoredPlaybackRecords {
		t.Fatalf("expected prune to %d records, got %d", maxStoredPlaybackRecords, got)
	}
	if got := cap(st.state.PlaybackRecords); got != maxStoredPlaybackRecords {
		t.Fatalf("expected compacted capacity %d, got %d", maxStoredPlaybackRecords, got)
	}
	latest := st.PlaybackRecords(1, 0, 1)
	if len(latest) != 1 || latest[0].ItemID != "new" {
		t.Fatalf("unexpected latest record after prune: %#v", latest)
	}
}

func TestPlaybackSessionsPruneCompactsCapacity(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	st.mu.Lock()
	st.state.PlaybackSessions = make([]PlaybackSession, maxPlaybackSessions, maxPlaybackSessions*2)
	st.mu.Unlock()
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPlaybackSession(PlaybackSession{UID: 1, ItemID: "latest"}); err != nil {
		t.Fatal(err)
	}
	if got := len(st.state.PlaybackSessions); got != maxPlaybackSessions {
		t.Fatalf("expected prune to %d sessions, got %d", maxPlaybackSessions, got)
	}
	if got := cap(st.state.PlaybackSessions); got != maxPlaybackSessions {
		t.Fatalf("expected compacted capacity %d, got %d", maxPlaybackSessions, got)
	}
	if latest := st.state.PlaybackSessions[len(st.state.PlaybackSessions)-1]; latest.ItemID != "latest" {
		t.Fatalf("unexpected latest session after prune: %#v", latest)
	}
}

func TestPlaybackRecordPrependCompactsOversizedCapacity(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	st.mu.Lock()
	st.state.PlaybackRecords = make([]PlaybackRecord, maxStoredPlaybackRecords, maxStoredPlaybackRecords*2)
	for i := range st.state.PlaybackRecords {
		st.state.PlaybackRecords[i] = PlaybackRecord{UID: 1, ItemID: "bulk", PlayedAt: int64(maxStoredPlaybackRecords - i)}
	}
	st.mu.Unlock()
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	if err := st.AddPlaybackRecord(PlaybackRecord{UID: 1, ItemID: "new", PlayedAt: int64(maxStoredPlaybackRecords + 1)}); err != nil {
		t.Fatal(err)
	}
	if got := len(st.state.PlaybackRecords); got != maxStoredPlaybackRecords {
		t.Fatalf("expected prune to %d records, got %d", maxStoredPlaybackRecords, got)
	}
	if got := cap(st.state.PlaybackRecords); got != maxStoredPlaybackRecords {
		t.Fatalf("expected compacted capacity %d, got %d", maxStoredPlaybackRecords, got)
	}
	if got := st.state.PlaybackRecords[0].ItemID; got != "new" {
		t.Fatalf("expected newest record at head, got %q", got)
	}
}

func TestAddPlaybackRecordsIdempotentMatchesSingleWrite(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// 预置一条已有记录，验证批量去重会命中现有状态。
	if err := st.AddPlaybackRecord(PlaybackRecord{UID: 1, ItemID: "existing", PlayedAt: 5}); err != nil {
		t.Fatal(err)
	}

	inserted, err := st.AddPlaybackRecordsIdempotent([]PlaybackRecord{
		{UID: 1, ItemID: "old", PlayedAt: 10},
		{UID: 2, ItemID: "other", PlayedAt: 20},
		{UID: 1, ItemID: "existing", PlayedAt: 5}, // 与预置记录重复 → 跳过
		{UID: 1, ItemID: "mid", PlayedAt: 30},
		{UID: 1, ItemID: "old", PlayedAt: 10}, // 批内重复 → 跳过
		{UID: 1, ItemID: "new", PlayedAt: 40},
	})
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 4 {
		t.Fatalf("expected 4 inserted (2 dups skipped), got %d", inserted)
	}

	// 逐条 prepend 语义：批内最后接受的 new 应在最前，existing 在最后。
	all := st.PlaybackRecords(0, 0, 10)
	if len(all) != 5 {
		t.Fatalf("expected 5 total records, got %d: %#v", len(all), all)
	}
	if all[0].ItemID != "new" {
		t.Fatalf("expected newest record 'new' at head, got %q", all[0].ItemID)
	}
	if all[len(all)-1].ItemID != "existing" {
		t.Fatalf("expected 'existing' at tail, got %q", all[len(all)-1].ItemID)
	}

	uidRecords := st.PlaybackRecords(1, 0, 10)
	if len(uidRecords) != 4 || uidRecords[0].ItemID != "new" {
		t.Fatalf("unexpected uid=1 records: %#v", uidRecords)
	}
}

func TestAddPlaybackRecordsIdempotentDefaultsPlayedAt(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	before := time.Now().Unix()
	inserted, err := st.AddPlaybackRecordsIdempotent([]PlaybackRecord{
		{UID: 7, ItemID: "no-time"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 1 {
		t.Fatalf("expected 1 inserted, got %d", inserted)
	}
	got := st.PlaybackRecords(7, 0, 1)
	if len(got) != 1 || got[0].PlayedAt < before {
		t.Fatalf("expected PlayedAt default >= %d, got %#v", before, got)
	}
}

func TestAddPlaybackRecordsIdempotentEmptyIsNoop(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if inserted, err := st.AddPlaybackRecordsIdempotent(nil); err != nil || inserted != 0 {
		t.Fatalf("empty batch inserted=%d err=%v, want 0 nil", inserted, err)
	}
}

func TestListEmbyActivityLogsFiltersByTargetUserEmbyID(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	alpha, err := st.CreateUser(User{Username: "alpha", EmbyID: "emby-alpha", Role: RoleNormal})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := st.CreateUser(User{Username: "beta", EmbyID: "emby-beta", Role: RoleNormal})
	if err != nil {
		t.Fatal(err)
	}
	unbound, err := st.CreateUser(User{Username: "unbound", Role: RoleNormal})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SyncEmbyActivityLogs([]EmbyActivityLog{
		{EmbyLogID: 1, UserID: "emby-alpha", Name: "alpha-old", Date: 10},
		{EmbyLogID: 2, UserID: "emby-beta", Name: "beta", Date: 20},
		{EmbyLogID: 3, UserID: "emby-alpha", Name: "alpha-new", Date: 30},
	}); err != nil {
		t.Fatal(err)
	}

	alphaLogs := st.ListEmbyActivityLogs(alpha.UID, 10)
	if len(alphaLogs) != 2 || alphaLogs[0].Name != "alpha-new" || alphaLogs[1].Name != "alpha-old" {
		t.Fatalf("unexpected alpha logs: %#v", alphaLogs)
	}
	betaLogs := st.ListEmbyActivityLogs(beta.UID, 10)
	if len(betaLogs) != 1 || betaLogs[0].Name != "beta" {
		t.Fatalf("unexpected beta logs: %#v", betaLogs)
	}
	if logs := st.ListEmbyActivityLogs(unbound.UID, 10); len(logs) != 0 {
		t.Fatalf("unbound user should not match activity logs: %#v", logs)
	}
}

func TestSyncEmbyActivityLogsEmptyInputIsNoop(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	added, err := st.SyncEmbyActivityLogs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Fatalf("empty input added=%d, want 0", added)
	}
	if logs := st.ListEmbyActivityLogs(0, 10); len(logs) != 0 {
		t.Fatalf("empty sync should not create logs: %#v", logs)
	}
}

func TestSyncEmbyActivityLogsSkipsUnchangedDuplicates(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	entry := EmbyActivityLog{EmbyLogID: 1001, UserID: "emby-user", Name: "playback.start", Date: 100}
	added, err := st.SyncEmbyActivityLogs([]EmbyActivityLog{entry})
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 {
		t.Fatalf("first sync added=%d, want 1", added)
	}
	before := st.ListEmbyActivityLogs(0, 10)
	if len(before) != 1 {
		t.Fatalf("first sync logs=%#v", before)
	}

	added, err = st.SyncEmbyActivityLogs([]EmbyActivityLog{entry})
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Fatalf("duplicate sync added=%d, want 0", added)
	}
	after := st.ListEmbyActivityLogs(0, 10)
	if len(after) != 1 {
		t.Fatalf("duplicate sync should keep one log: %#v", after)
	}
	if after[0] != before[0] {
		t.Fatalf("unchanged duplicate should preserve stored fields: before=%#v after=%#v", before[0], after[0])
	}
}

func TestSyncEmbyActivityLogsUpdatesExistingWithoutCountingAdded(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	entry := EmbyActivityLog{EmbyLogID: 2001, UserID: "emby-user", Name: "old", Date: 100}
	if added, err := st.SyncEmbyActivityLogs([]EmbyActivityLog{entry}); err != nil || added != 1 {
		t.Fatalf("first sync added=%d err=%v, want 1 nil", added, err)
	}
	before := st.ListEmbyActivityLogs(0, 10)[0]

	updated := entry
	updated.Name = "updated"
	updated.Date = 200
	added, err := st.SyncEmbyActivityLogs([]EmbyActivityLog{updated})
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Fatalf("updated existing added=%d, want 0", added)
	}
	after := st.ListEmbyActivityLogs(0, 10)
	if len(after) != 1 {
		t.Fatalf("updated existing should keep one log: %#v", after)
	}
	if after[0].Name != "updated" || after[0].Date != 200 {
		t.Fatalf("existing log was not updated: %#v", after[0])
	}
	if after[0].ID != before.ID || after[0].CreatedAt != before.CreatedAt {
		t.Fatalf("existing log should preserve local identity fields: before=%#v after=%#v", before, after[0])
	}
}

func TestStateEnsureCompactsHistory(t *testing.T) {
	state := State{
		LoginLogs:        make([]LoginLog, maxStoredLoginLogs+1),
		PlaybackRecords:  make([]PlaybackRecord, maxStoredPlaybackRecords+1),
		PlaybackSessions: make([]PlaybackSession, maxPlaybackSessions+1),
		EmbyActivityLogs: make([]EmbyActivityLog, maxEmbyActivityLogs+1),
		BangumiSyncLogs:  make([]BangumiSyncLog, maxStoredBangumiSyncLogs+1),
		RuntimeLogs:      make([]RuntimeLogEntry, defaultRuntimeLogLimit+1),
		Signin:           map[int64]Signin{1: {UID: 1, Records: make([]SigninRecord, maxSigninRecords+1)}},
	}
	state.ensure()
	if len(state.LoginLogs) != maxStoredLoginLogs ||
		len(state.PlaybackRecords) != maxStoredPlaybackRecords ||
		len(state.PlaybackSessions) != maxPlaybackSessions ||
		len(state.EmbyActivityLogs) != maxEmbyActivityLogs ||
		len(state.BangumiSyncLogs) != maxStoredBangumiSyncLogs ||
		len(state.RuntimeLogs) != defaultRuntimeLogLimit {
		t.Fatalf("history was not compacted: login=%d playback=%d sessions=%d activity=%d bangumi=%d runtime=%d",
			len(state.LoginLogs), len(state.PlaybackRecords), len(state.PlaybackSessions), len(state.EmbyActivityLogs), len(state.BangumiSyncLogs), len(state.RuntimeLogs))
	}
	if got := len(state.Signin[1].Records); got != maxSigninRecords {
		t.Fatalf("signin records were not compacted: got %d", got)
	}
}
