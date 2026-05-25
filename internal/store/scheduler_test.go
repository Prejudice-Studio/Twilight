package store

import (
	"path/filepath"
	"testing"
)

func TestAddSchedulerRunNormalizesDefaultsAndLegacyEndedAt(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	run, err := st.AddSchedulerRunReturning(SchedulerRun{JobID: "job", Status: "success", EndedAt: 123})
	if err != nil {
		t.Fatal(err)
	}
	if run.ID == 0 {
		t.Fatal("expected scheduler run id")
	}
	if run.Type != "manual" || run.Trigger != "manual" {
		t.Fatalf("expected manual defaults, got type=%q trigger=%q", run.Type, run.Trigger)
	}
	if run.FinishedAt != run.EndedAt {
		t.Fatalf("expected FinishedAt to mirror legacy EndedAt, got finished=%d ended=%d", run.FinishedAt, run.EndedAt)
	}
}

func TestSetSchedulerScheduleDeletesCustomWhenDefault(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.SetSchedulerSchedule("job", map[string]any{"type": "manual"}, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.SchedulerSchedule("job"); !ok {
		t.Fatal("expected custom schedule")
	}
	if _, err := st.SetSchedulerSchedule("job", map[string]any{"type": "manual"}, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.SchedulerSchedule("job"); ok {
		t.Fatal("expected default schedule to remove custom override")
	}
}
