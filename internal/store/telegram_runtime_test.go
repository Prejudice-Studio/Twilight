package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestTelegramBotOffsetUsesDedicatedRuntimeRow(t *testing.T) {
	st := newJSONStoreForTest(t)
	beforeVersion := st.stateVersion
	if err := st.SetTelegramBotOffset(42); err != nil {
		t.Fatal(err)
	}
	if err := st.SetTelegramBotOffset(12); err != nil {
		t.Fatal(err)
	}
	offset, err := st.TelegramBotOffset()
	if err != nil {
		t.Fatal(err)
	}
	if offset != 42 {
		t.Fatalf("offset=%d, want 42", offset)
	}
	if st.stateVersion != beforeVersion {
		t.Fatalf("state version changed from %d to %d", beforeVersion, st.stateVersion)
	}

	var embeddedOffset, persistedVersion int64
	if err := st.db.QueryRow(`
SELECT COALESCE((state->>'telegram_bot_offset')::bigint, 0), version
FROM twilight_state WHERE id = 1`).Scan(&embeddedOffset, &persistedVersion); err != nil {
		t.Fatal(err)
	}
	if embeddedOffset != 0 || persistedVersion != beforeVersion {
		t.Fatalf("offset rewrote twilight_state: embedded=%d version=%d want version=%d", embeddedOffset, persistedVersion, beforeVersion)
	}

	reopened := reopenTestStore(t)
	reopenedOffset, err := reopened.TelegramBotOffset()
	if err != nil {
		t.Fatal(err)
	}
	if reopenedOffset != 42 {
		t.Fatalf("reopened offset=%d, want 42", reopenedOffset)
	}
	if err := reopened.ResetTelegramBotOffset(); err != nil {
		t.Fatal(err)
	}
	resetOffset, err := reopened.TelegramBotOffset()
	if err != nil {
		t.Fatal(err)
	}
	if resetOffset != 0 {
		t.Fatalf("reset offset=%d, want 0", resetOffset)
	}
}

func TestOpenPostgresMigratesLegacyTelegramBotOffset(t *testing.T) {
	st := newJSONStoreForTest(t)
	st.mu.Lock()
	st.state.TelegramBotOffset = 77
	if err := st.saveLocked(); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	st.mu.Unlock()
	if _, err := st.db.Exec(`DELETE FROM twilight_telegram_runtime WHERE id = 1`); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	reopened, err := OpenPostgres(ctx, testDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	offset, err := reopened.TelegramBotOffset()
	if err != nil {
		t.Fatal(err)
	}
	if offset != 77 {
		t.Fatalf("migrated offset=%d, want 77", offset)
	}
	var embeddedOffset int64
	var hasEmbeddedOffset bool
	if err := reopened.db.QueryRow(`
SELECT COALESCE((state->>'telegram_bot_offset')::bigint, 0), state ? 'telegram_bot_offset'
FROM twilight_state WHERE id = 1`).Scan(&embeddedOffset, &hasEmbeddedOffset); err != nil {
		t.Fatal(err)
	}
	if embeddedOffset != 0 || hasEmbeddedOffset {
		t.Fatalf("legacy embedded offset was not cleared: value=%d present=%v", embeddedOffset, hasEmbeddedOffset)
	}
}

func TestLoadSnapshotMigratesLegacyTelegramBotOffset(t *testing.T) {
	st := newJSONStoreForTest(t)
	legacy := emptyState()
	legacy.TelegramBotOffset = 99
	snapshot, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.LoadSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	offset, err := st.TelegramBotOffset()
	if err != nil {
		t.Fatal(err)
	}
	if offset != 99 {
		t.Fatalf("snapshot offset=%d, want 99", offset)
	}
	var embeddedOffset int64
	var hasEmbeddedOffset bool
	if err := st.db.QueryRow(`
SELECT COALESCE((state->>'telegram_bot_offset')::bigint, 0), state ? 'telegram_bot_offset'
FROM twilight_state WHERE id = 1`).Scan(&embeddedOffset, &hasEmbeddedOffset); err != nil {
		t.Fatal(err)
	}
	if embeddedOffset != 0 || hasEmbeddedOffset {
		t.Fatalf("snapshot retained legacy embedded offset: value=%d present=%v", embeddedOffset, hasEmbeddedOffset)
	}
}
