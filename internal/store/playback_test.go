package store

import (
	"path/filepath"
	"testing"
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
