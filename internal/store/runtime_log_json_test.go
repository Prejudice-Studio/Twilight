package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestRuntimeLogSidecarPersistsAcrossReopen 验证 JSON 后端 runtime log 落在旁路文件里，
// 且跨 Close+Open 存活、游标 / nextID 不回退。
func TestRuntimeLogSidecarPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		if _, err := st.AddRuntimeLog(RuntimeLogEntry{Level: "info", Message: "seed", Time: int64(i)}, 100); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(sidecarPath(path)); err != nil {
		t.Fatalf("sidecar file should exist after append: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	maxID, count := st2.RuntimeLogStats()
	if maxID != 5 || count != 5 {
		t.Fatalf("reopen stats max=%d count=%d, want 5/5", maxID, count)
	}
	// nextID 不回退：新增一条应拿到 ID 6。
	entry, err := st2.AddRuntimeLog(RuntimeLogEntry{Level: "info", Message: "after-reopen"}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if entry.ID != 6 {
		t.Fatalf("after reopen new entry ID=%d, want 6", entry.ID)
	}
}

// TestRuntimeLogMigratesFromEmbeddedState 验证历史 state.json（runtime log 内嵌在
// RuntimeLogs 字段里）在 Open 时被迁移进旁路文件，且此后 state.json 不再承载 runtime log。
func TestRuntimeLogMigratesFromEmbeddedState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	legacy := emptyState()
	legacy.NextRuntimeLogID = 4
	legacy.RuntimeLogs = []RuntimeLogEntry{
		{ID: 1, Level: "info", Message: "a", Time: 1},
		{ID: 2, Level: "warn", Message: "b", Time: 2},
		{ID: 3, Level: "error", Message: "c", Time: 3},
	}
	raw, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// 迁移后旁路文件应已生成，日志可读。
	maxID, count := st.RuntimeLogStats()
	if maxID != 3 || count != 3 {
		t.Fatalf("migrated stats max=%d count=%d, want 3/3", maxID, count)
	}
	if _, err := os.Stat(sidecarPath(path)); err != nil {
		t.Fatalf("sidecar should be materialized on migration: %v", err)
	}
	// 触发一次落盘（Prune 走 saveLocked 之外，这里换用会写 state 的路径）：新增日志后
	// state.json 不应再含 runtime log。AddRuntimeLog 不写 state.json，故用 Snapshot 校验
	// 内存 state 已清空 RuntimeLogs，再读盘确认 state.json 未回填。
	if _, err := st.AddRuntimeLog(RuntimeLogEntry{Level: "info", Message: "d"}, 100); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded State
	if err := json.Unmarshal(onDisk, &reloaded); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.RuntimeLogs) != 0 {
		t.Fatalf("state.json should not carry runtime logs after migration, got %d", len(reloaded.RuntimeLogs))
	}

	// 重新 Open：仍以旁路为准，第 4 条（migrate 后新增的）存活。
	st2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	if maxID, count := st2.RuntimeLogStats(); maxID != 4 || count != 4 {
		t.Fatalf("reopen after migration stats max=%d count=%d, want 4/4", maxID, count)
	}
}

// TestRuntimeLogSnapshotRoundTrip 验证 Snapshot 把旁路日志注入 State，LoadSnapshot 再
// 落回旁路文件（含空快照清空语义）。
func TestRuntimeLogSnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for i := 1; i <= 3; i++ {
		if _, err := st.AddRuntimeLog(RuntimeLogEntry{Level: "info", Message: "x", Time: int64(i)}, 100); err != nil {
			t.Fatal(err)
		}
	}
	snap, err := st.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var snapState State
	if err := json.Unmarshal(snap, &snapState); err != nil {
		t.Fatal(err)
	}
	if len(snapState.RuntimeLogs) != 3 {
		t.Fatalf("snapshot should embed 3 runtime logs, got %d", len(snapState.RuntimeLogs))
	}

	// LoadSnapshot 一个无日志的空快照：旁路应被清空。
	empty := emptyState()
	emptyBytes, err := json.MarshalIndent(empty, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.LoadSnapshot(emptyBytes); err != nil {
		t.Fatal(err)
	}
	if _, count := st.RuntimeLogStats(); count != 0 {
		t.Fatalf("after loading empty snapshot count=%d, want 0", count)
	}

	// LoadSnapshot 回原快照：3 条日志恢复。
	if err := st.LoadSnapshot(snap); err != nil {
		t.Fatal(err)
	}
	if maxID, count := st.RuntimeLogStats(); maxID != 3 || count != 3 {
		t.Fatalf("after restoring snapshot stats max=%d count=%d, want 3/3", maxID, count)
	}
}
