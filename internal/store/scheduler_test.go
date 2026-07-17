package store

import (
	"path/filepath"
	"testing"
	"time"
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

func TestSchedulerRunPrependCompactsOversizedCapacity(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	st.mu.Lock()
	st.state.SchedulerRuns = make([]SchedulerRun, maxStoredSchedulerRuns, maxStoredSchedulerRuns*2)
	for i := range st.state.SchedulerRuns {
		st.state.SchedulerRuns[i] = SchedulerRun{ID: int64(i + 1), JobID: "bulk", Status: "success", StartedAt: int64(maxStoredSchedulerRuns - i)}
	}
	st.mu.Unlock()
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	run, err := st.AddSchedulerRunReturning(SchedulerRun{JobID: "new", Status: "success", StartedAt: int64(maxStoredSchedulerRuns + 1)})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(st.state.SchedulerRuns); got != maxStoredSchedulerRuns {
		t.Fatalf("expected prune to %d runs, got %d", maxStoredSchedulerRuns, got)
	}
	if got := cap(st.state.SchedulerRuns); got != maxStoredSchedulerRuns {
		t.Fatalf("expected compacted capacity %d, got %d", maxStoredSchedulerRuns, got)
	}
	if got := st.state.SchedulerRuns[0].ID; got != run.ID {
		t.Fatalf("expected newest run at head, got id=%d want=%d", got, run.ID)
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

// TestLastSchedulerRunByTypeBypassesRecentWindow 锁定 LastSchedulerRunByType
// 不被 SchedulerRuns(20) 的滑动窗口影响：先记一条 auto run，再压入 25 条
// manual run，最新 auto 必须仍能被 LastSchedulerRunByType("job","auto") 找
// 到——这是 schedulerJobDue cron_daily 路径"今天是否已自动跑过"判定的基础。
func TestLastSchedulerRunByTypeBypassesRecentWindow(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	autoRun, err := st.AddSchedulerRunReturning(SchedulerRun{JobID: "check_expired", Type: "auto", Trigger: "scheduler", Status: "success", StartedAt: 1000})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		if _, err := st.AddSchedulerRunReturning(SchedulerRun{JobID: "check_expired", Type: "manual", Trigger: "manual", Status: "success", StartedAt: int64(2000 + i)}); err != nil {
			t.Fatal(err)
		}
	}

	// SchedulerRuns(20) 只返回 manual——auto 已被挤出窗口。
	window := st.SchedulerRuns("check_expired", 20)
	for _, r := range window {
		if r.Type == "auto" {
			t.Fatalf("auto run unexpectedly still in 20-row window: %#v", r)
		}
	}

	// 但 LastSchedulerRunByType 仍然能返回最早那条 auto run。
	got, ok := st.LastSchedulerRunByType("check_expired", "auto")
	if !ok {
		t.Fatal("LastSchedulerRunByType should find auto run beyond 20-row window")
	}
	if got.ID != autoRun.ID || got.StartedAt != 1000 {
		t.Fatalf("LastSchedulerRunByType returned wrong run: got=%#v want id=%d started=1000", got, autoRun.ID)
	}

	// runType 留空时按 jobID 取最新一条（应当是最后一条 manual）。
	latest, ok := st.LastSchedulerRunByType("check_expired", "")
	if !ok || latest.Type != "manual" || latest.StartedAt != 2024 {
		t.Fatalf("LastSchedulerRunByType('','') returned wrong run: %#v", latest)
	}

	// 不存在的 jobID 应返回 false。
	if _, ok := st.LastSchedulerRunByType("missing_job", "auto"); ok {
		t.Fatal("LastSchedulerRunByType should return false for missing job")
	}
}

func TestSchedulerRunSnapshotAndInterruptedMarking(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now().Unix()
	if _, err := st.AddSchedulerRunReturning(SchedulerRun{JobID: "job", Type: "auto", Trigger: "scheduler", Status: "success", StartedAt: now - 120}); err != nil {
		t.Fatal(err)
	}
	fresh, err := st.AddSchedulerRunReturning(SchedulerRun{JobID: "job", Type: "auto", Trigger: "scheduler", Status: "running", StartedAt: now - 30})
	if err != nil {
		t.Fatal(err)
	}
	stale, err := st.AddSchedulerRunReturning(SchedulerRun{JobID: "job", Type: "manual", Trigger: "manual", Status: "running", StartedAt: now - 3600})
	if err != nil {
		t.Fatal(err)
	}

	snapshot := st.SchedulerRunSnapshot("job", 2)
	if len(snapshot.Runs) != 2 || !snapshot.HasLatestAuto || snapshot.LatestAuto.ID != fresh.ID || !snapshot.HasLatestRunning || snapshot.LatestRunning.ID != fresh.ID {
		t.Fatalf("unexpected scheduler snapshot: %#v", snapshot)
	}
	changed, err := st.MarkInterruptedSchedulerRuns("job", now-1800, now)
	if err != nil {
		t.Fatal(err)
	}
	if changed != 1 {
		t.Fatalf("changed=%d, want 1", changed)
	}
	runs := st.SchedulerRuns("job", 10)
	for _, run := range runs {
		switch run.ID {
		case fresh.ID:
			if run.Status != "running" {
				t.Fatalf("fresh running row should not be interrupted: %#v", run)
			}
		case stale.ID:
			if run.Status != "failed" || run.FinishedAt != now || !boolishStore(run.Summary["interrupted"]) {
				t.Fatalf("stale running row was not interrupted: %#v", run)
			}
		}
	}
}

func boolishStore(value any) bool {
	b, _ := value.(bool)
	return b
}
