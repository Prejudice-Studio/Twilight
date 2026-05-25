package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestIPBlacklistLifecycleExpiryAndSorting(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now().Unix()
	if err := st.AddIPBlacklist("203.0.113.1", "expired", now-1); err != nil {
		t.Fatal(err)
	}
	if err := st.AddIPBlacklist("203.0.113.2", "permanent", -1); err != nil {
		t.Fatal(err)
	}
	if err := st.AddIPBlacklist("203.0.113.3", "future", now+3600); err != nil {
		t.Fatal(err)
	}

	if st.IsIPBlacklisted("203.0.113.1") {
		t.Fatal("expired IP should not be active")
	}
	if !st.IsIPBlacklisted("203.0.113.2") || !st.IsIPBlacklisted("203.0.113.3") {
		t.Fatal("permanent/future IPs should be active")
	}

	entries := st.ListIPBlacklist()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %#v", entries)
	}
	for i := 1; i < len(entries); i++ {
		if entries[i-1].CreatedAt < entries[i].CreatedAt {
			t.Fatalf("entries not sorted newest-first: %#v", entries)
		}
	}

	if err := st.RemoveIPBlacklist("203.0.113.2"); err != nil {
		t.Fatal(err)
	}
	if st.IsIPBlacklisted("203.0.113.2") {
		t.Fatal("removed IP should not be active")
	}
}
