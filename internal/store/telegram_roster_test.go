package store

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestTelegramRosterObservationNeedsWrite(t *testing.T) {
	now := int64(10_000)
	intervalSeconds := int64(telegramRosterObservationWriteInterval / time.Second)
	base := telegramRosterCacheEntry{
		status:      "member",
		lastWritten: now - intervalSeconds + 1,
	}

	tests := []struct {
		name   string
		entry  telegramRosterCacheEntry
		exists bool
		status string
		isBot  bool
		want   bool
	}{
		{name: "new member", status: "member", exists: false, want: true},
		{name: "recent unchanged member", entry: base, status: "member", exists: true, want: false},
		{name: "status changed", entry: base, status: "administrator", exists: true, want: true},
		{name: "bot flag discovered", entry: base, status: "member", isBot: true, exists: true, want: true},
		{name: "refresh interval elapsed", entry: func() telegramRosterCacheEntry {
			entry := base
			entry.lastWritten = now - intervalSeconds
			return entry
		}(), status: "member", exists: true, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := telegramRosterObservationNeedsWrite(tt.entry, tt.exists, tt.status, tt.isBot, now); got != tt.want {
				t.Fatalf("needs write = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTelegramRosterUsesDedicatedTableAndSkipsRecentObservation(t *testing.T) {
	st := newJSONStoreForTest(t)
	beforeVersion := st.stateVersion
	if err := st.UpsertTelegramRoster("-1001", 42, "member", false); err != nil {
		t.Fatal(err)
	}
	var firstXID, firstStatus string
	if err := st.db.QueryRow(`
SELECT xmin::text, last_status FROM twilight_telegram_roster
WHERE chat_id = '-1001' AND telegram_id = 42`).Scan(&firstXID, &firstStatus); err != nil {
		t.Fatal(err)
	}
	if firstStatus != "member" {
		t.Fatalf("status=%q, want member", firstStatus)
	}

	if err := st.UpsertTelegramRoster("-1001", 42, "member", false); err != nil {
		t.Fatal(err)
	}
	var unchangedXID string
	if err := st.db.QueryRow(`
SELECT xmin::text FROM twilight_telegram_roster
WHERE chat_id = '-1001' AND telegram_id = 42`).Scan(&unchangedXID); err != nil {
		t.Fatal(err)
	}
	if unchangedXID != firstXID {
		t.Fatalf("recent observation performed a physical update: xid %s -> %s", firstXID, unchangedXID)
	}

	if err := st.UpsertTelegramRoster("-1001", 42, "administrator", true); err != nil {
		t.Fatal(err)
	}
	var status string
	var isBot bool
	if err := st.db.QueryRow(`
SELECT last_status, is_bot FROM twilight_telegram_roster
WHERE chat_id = '-1001' AND telegram_id = 42`).Scan(&status, &isBot); err != nil {
		t.Fatal(err)
	}
	if status != "administrator" || !isBot {
		t.Fatalf("status=%q is_bot=%v", status, isBot)
	}
	if st.stateVersion != beforeVersion {
		t.Fatalf("roster rewrote twilight_state: version %d -> %d", beforeVersion, st.stateVersion)
	}
}

func TestTelegramRosterColdCacheStillAvoidsRecentPhysicalWrite(t *testing.T) {
	st := newJSONStoreForTest(t)
	if err := st.UpsertTelegramRoster("-1001", 42, "member", false); err != nil {
		t.Fatal(err)
	}
	var beforeXID string
	if err := st.db.QueryRow(`SELECT xmin::text FROM twilight_telegram_roster WHERE chat_id = '-1001' AND telegram_id = 42`).Scan(&beforeXID); err != nil {
		t.Fatal(err)
	}

	reopened := reopenTestStore(t)
	if err := reopened.UpsertTelegramRoster("-1001", 42, "member", false); err != nil {
		t.Fatal(err)
	}
	var afterXID string
	if err := reopened.db.QueryRow(`SELECT xmin::text FROM twilight_telegram_roster WHERE chat_id = '-1001' AND telegram_id = 42`).Scan(&afterXID); err != nil {
		t.Fatal(err)
	}
	if afterXID != beforeXID {
		t.Fatalf("cold cache rewrote recent row: xid %s -> %s", beforeXID, afterXID)
	}
}

func TestApplyTelegramRosterUpdatesBatchesAndStats(t *testing.T) {
	st := newJSONStoreForTest(t)
	beforeVersion := st.stateVersion
	updates := []TelegramRosterUpdate{
		{ChatID: "-1002", TelegramID: 9, Status: "left"},
		{ChatID: "-1001", TelegramID: 3, Status: "member"},
		{ChatID: "-1001", TelegramID: 2, Status: "member"},
		{ChatID: "-1001", TelegramID: 2, Status: "administrator", IsBot: true},
	}
	if err := st.ApplyTelegramRosterUpdates(updates); err != nil {
		t.Fatal(err)
	}
	entries, err := st.TelegramRoster("", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries=%d, want 3: %#v", len(entries), entries)
	}
	if entries[0].ChatID != "-1001" || entries[0].TelegramID != 2 || entries[1].TelegramID != 3 || entries[2].ChatID != "-1002" {
		t.Fatalf("unexpected order: %#v", entries)
	}
	if entries[0].LastStatus != "administrator" || !entries[0].IsBot {
		t.Fatalf("duplicate update was not consolidated: %#v", entries[0])
	}
	stats, err := st.TelegramRosterStats("")
	if err != nil {
		t.Fatal(err)
	}
	if stats["total"] != 3 || stats["active"] != 2 || stats["inactive"] != 1 || stats["bots"] != 1 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	if st.stateVersion != beforeVersion {
		t.Fatalf("batch roster update rewrote twilight_state: version %d -> %d", beforeVersion, st.stateVersion)
	}
}

func TestTelegramRosterConcurrentObservation(t *testing.T) {
	st := newJSONStoreForTest(t)
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- st.UpsertTelegramRoster("-1001", 42, "member", false)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM twilight_telegram_roster WHERE chat_id = '-1001' AND telegram_id = 42`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("rows=%d, want 1", count)
	}
}

func TestTelegramRosterFailedWriteReleasesCacheReservation(t *testing.T) {
	st := newJSONStoreForTest(t)
	if err := st.db.Close(); err != nil {
		t.Fatal(err)
	}
	key := telegramRosterCacheKey{chatID: "-1001", telegramID: 42}
	if err := st.UpsertTelegramRoster(key.chatID, key.telegramID, "member", false); err == nil {
		t.Fatal("expected closed database error")
	}
	st.telegramRosterCacheMu.Lock()
	_, reserved := st.telegramRosterCache[key]
	st.telegramRosterCacheMu.Unlock()
	if reserved {
		t.Fatal("failed write left a cache reservation that would suppress retries")
	}
}

func TestOpenPostgresMigratesLegacyTelegramRoster(t *testing.T) {
	st := newJSONStoreForTest(t)
	st.mu.Lock()
	st.state.TelegramRoster = map[string]TelegramRosterEntry{
		"legacy-key": {ChatID: "-1001", TelegramID: 42, IsBot: true, LastStatus: "administrator", FirstSeen: 100, LastSeen: 200},
	}
	if err := st.saveLocked(); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	st.mu.Unlock()
	if _, err := st.db.Exec(`TRUNCATE twilight_telegram_roster`); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	reopened, err := OpenPostgres(ctx, testDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	entries, err := reopened.TelegramRoster("-1001", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].TelegramID != 42 || entries[0].FirstSeen != 100 || entries[0].LastSeen != 200 || !entries[0].IsBot {
		t.Fatalf("migrated entries=%#v", entries)
	}
	var hasRoster bool
	if err := reopened.db.QueryRow(`SELECT state ? 'telegram_roster' FROM twilight_state WHERE id = 1`).Scan(&hasRoster); err != nil {
		t.Fatal(err)
	}
	if hasRoster {
		t.Fatal("legacy telegram_roster remained in twilight_state")
	}
	if reopened.state.TelegramRoster != nil {
		t.Fatal("legacy telegram roster remained resident in Store state")
	}
}

func TestTelegramRosterSnapshotRoundTrip(t *testing.T) {
	st := newJSONStoreForTest(t)
	if err := st.ApplyTelegramRosterUpdates([]TelegramRosterUpdate{
		{ChatID: "-1001", TelegramID: 1, Status: "member"},
		{ChatID: "-1001", TelegramID: 2, Status: "left", IsBot: true},
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := st.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var exported State
	if err := json.Unmarshal(snapshot, &exported); err != nil {
		t.Fatal(err)
	}
	if len(exported.TelegramRoster) != 2 {
		t.Fatalf("snapshot roster=%d, want 2", len(exported.TelegramRoster))
	}
	if err := st.UpsertTelegramRoster("-1002", 3, "member", false); err != nil {
		t.Fatal(err)
	}
	if err := st.LoadSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	entries, err := st.TelegramRoster("", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].ChatID != "-1001" || entries[1].ChatID != "-1001" {
		t.Fatalf("restored entries=%#v", entries)
	}
	var hasRoster bool
	if err := st.db.QueryRow(`SELECT state ? 'telegram_roster' FROM twilight_state WHERE id = 1`).Scan(&hasRoster); err != nil {
		t.Fatal(err)
	}
	if hasRoster {
		t.Fatal("restored roster was embedded in twilight_state")
	}
}

func TestTelegramRosterHotCacheIsBounded(t *testing.T) {
	st := newJSONStoreForTest(t)
	now := time.Now().Unix()
	for i := 0; i < telegramRosterCacheCapacity+128; i++ {
		st.rememberTelegramRosterObservation(
			telegramRosterCacheKey{chatID: "-1001", telegramID: int64(i + 1)},
			"member", false, now,
		)
	}
	st.telegramRosterCacheMu.Lock()
	size := len(st.telegramRosterCache)
	st.telegramRosterCacheMu.Unlock()
	if size > telegramRosterCacheCapacity {
		t.Fatalf("cache size=%d, capacity=%d", size, telegramRosterCacheCapacity)
	}
	if size == 0 {
		t.Fatal("cache unexpectedly empty")
	}
}
