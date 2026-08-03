package store

import (
	"testing"
	"time"
)

func TestTelegramRosterObservationNeedsWrite(t *testing.T) {
	now := int64(10_000)
	intervalSeconds := int64(telegramRosterObservationWriteInterval / time.Second)
	base := TelegramRosterEntry{
		ChatID:     "-1001",
		TelegramID: 42,
		LastStatus: "member",
		LastSeen:   now - intervalSeconds + 1,
	}

	tests := []struct {
		name   string
		entry  TelegramRosterEntry
		exists bool
		status string
		isBot  bool
		want   bool
	}{
		{name: "new member", status: "member", exists: false, want: true},
		{name: "recent unchanged member", entry: base, status: "member", exists: true, want: false},
		{name: "status changed", entry: base, status: "administrator", exists: true, want: true},
		{name: "bot flag discovered", entry: base, status: "member", isBot: true, exists: true, want: true},
		{name: "refresh interval elapsed", entry: func() TelegramRosterEntry {
			entry := base
			entry.LastSeen = now - intervalSeconds
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

func TestUpsertTelegramRosterSkipsRecentUnchangedPersistence(t *testing.T) {
	st := newJSONStoreForTest(t)
	if err := st.UpsertTelegramRoster("-1001", 42, "member", false); err != nil {
		t.Fatal(err)
	}
	st.mu.RLock()
	firstVersion := st.stateVersion
	st.mu.RUnlock()

	if err := st.UpsertTelegramRoster("-1001", 42, "member", false); err != nil {
		t.Fatal(err)
	}
	st.mu.RLock()
	unchangedVersion := st.stateVersion
	st.mu.RUnlock()
	if unchangedVersion != firstVersion {
		t.Fatalf("unchanged recent observation persisted state: version %d -> %d", firstVersion, unchangedVersion)
	}

	if err := st.UpsertTelegramRoster("-1001", 42, "administrator", false); err != nil {
		t.Fatal(err)
	}
	st.mu.RLock()
	changedVersion := st.stateVersion
	st.mu.RUnlock()
	if changedVersion <= unchangedVersion {
		t.Fatalf("status change was not persisted: version %d -> %d", unchangedVersion, changedVersion)
	}
}
