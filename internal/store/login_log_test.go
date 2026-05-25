package store

import (
	"path/filepath"
	"testing"
)

func TestLoginHistoryFiltersAndLimits(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	logs := []LoginLog{
		{UID: 1, DeviceName: "old", Time: 10},
		{UID: 2, DeviceName: "other", Time: 20},
		{UID: 1, DeviceName: "blocked", Time: 30, Blocked: true},
		{UID: 1, DeviceName: "new", Time: 40},
	}
	for _, log := range logs {
		if err := st.AddLoginLog(log); err != nil {
			t.Fatal(err)
		}
	}

	uidHistory := st.LoginHistory(1, false, 0, 10)
	if len(uidHistory) != 3 || uidHistory[0].DeviceName != "new" || uidHistory[2].DeviceName != "old" {
		t.Fatalf("unexpected uid history order/filter: %#v", uidHistory)
	}

	blocked := st.LoginHistory(1, true, 0, 10)
	if len(blocked) != 1 || blocked[0].DeviceName != "blocked" {
		t.Fatalf("unexpected blocked history: %#v", blocked)
	}

	recent := st.LoginHistory(0, false, 25, 1)
	if len(recent) != 1 || recent[0].DeviceName != "new" {
		t.Fatalf("unexpected recent limited history: %#v", recent)
	}
}

func TestLoginLogDefaultsAndPrunes(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.AddLoginLog(LoginLog{UID: 1, DeviceName: "first"}); err != nil {
		t.Fatal(err)
	}
	first := st.LoginHistory(1, false, 0, 1)[0]
	if first.ID == 0 || first.Time == 0 {
		t.Fatalf("expected id/time defaults, got %#v", first)
	}

	for i := 0; i < maxStoredLoginLogs+5; i++ {
		if err := st.AddLoginLog(LoginLog{UID: 2, DeviceName: "bulk", Time: int64(i + 1)}); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(st.state.LoginLogs); got != maxStoredLoginLogs {
		t.Fatalf("expected prune to %d logs, got %d", maxStoredLoginLogs, got)
	}
}
