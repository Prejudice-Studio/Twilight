package store

import (
	"path/filepath"
	"testing"
)

func TestRuntimeLogsJSONBackendCursorAndPrune(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for i := 1; i <= 105; i++ {
		if _, err := st.AddRuntimeLog(RuntimeLogEntry{Level: "info", Message: "entry", Time: int64(i)}, 100); err != nil {
			t.Fatal(err)
		}
	}

	maxID, count := st.RuntimeLogStats()
	if maxID != 105 || count != 100 {
		t.Fatalf("unexpected stats max=%d count=%d", maxID, count)
	}

	logs, next := st.RuntimeLogs(3, 0)
	if len(logs) != 3 || logs[0].ID != 103 || logs[2].ID != 105 || next != 105 {
		t.Fatalf("unexpected latest logs next=%d logs=%#v", next, logs)
	}

	logs, next = st.RuntimeLogs(10, 103)
	if len(logs) != 2 || logs[0].ID != 104 || logs[1].ID != 105 || next != 105 {
		t.Fatalf("unexpected cursor logs next=%d logs=%#v", next, logs)
	}

	logs, next = st.RuntimeLogs(3, 7)
	if len(logs) != 3 || logs[0].ID != 8 || logs[1].ID != 9 || logs[2].ID != 10 || next != 10 {
		t.Fatalf("unexpected bounded cursor logs next=%d logs=%#v", next, logs)
	}

	logs, next = st.RuntimeLogs(3, 105)
	if len(logs) != 0 || next != 105 {
		t.Fatalf("unexpected empty cursor logs next=%d logs=%#v", next, logs)
	}

	if err := st.PruneRuntimeLogs(100); err != nil {
		t.Fatal(err)
	}
	_, count = st.RuntimeLogStats()
	if count != 100 {
		t.Fatalf("unexpected count after prune: %d", count)
	}
}

func TestClampRuntimeLogLimits(t *testing.T) {
	if got := clampRuntimeLogReadLimit(0); got != 200 {
		t.Fatalf("read default = %d", got)
	}
	if got := clampRuntimeLogReadLimit(60000); got != 50000 {
		t.Fatalf("read max = %d", got)
	}
	if got := clampRuntimeLogLimit(1); got != 100 {
		t.Fatalf("write min = %d", got)
	}
	if got := clampRuntimeLogLimit(60000); got != 50000 {
		t.Fatalf("write max = %d", got)
	}
}
