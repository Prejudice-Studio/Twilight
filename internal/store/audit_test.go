package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestQueryAuditLogsStringSortUsesCaseInsensitiveOrder(t *testing.T) {
	st := newJSONStoreForTest(t)
	for _, entry := range []AuditLog{
		{Action: "Alpha", Username: "bravo", CreatedAt: 10},
		{Action: "alpha", Username: "alpha", CreatedAt: 20},
		{Action: "beta", Username: "charlie", CreatedAt: 30},
	} {
		if err := st.AddAuditLog(entry, 100); err != nil {
			t.Fatal(err)
		}
	}

	page := st.QueryAuditLogs(AuditLogQuery{SortBy: "action", Order: "asc", Limit: 10})
	if page.Total != 3 {
		t.Fatalf("total=%d, want 3", page.Total)
	}
	got := auditLogIDs(page.Logs)
	want := []int64{1, 2, 3}
	if !equalInt64Slices(got, want) {
		t.Fatalf("action asc IDs=%v, want %v", got, want)
	}

	page = st.QueryAuditLogs(AuditLogQuery{SortBy: "username", Order: "desc", Offset: 1, Limit: 1})
	got = auditLogIDs(page.Logs)
	want = []int64{1}
	if page.Total != 3 || !equalInt64Slices(got, want) {
		t.Fatalf("username desc page total=%d IDs=%v, want total=3 IDs=%v", page.Total, got, want)
	}
}

func TestQueryAuditLogsReturnsDeepCopiedDetails(t *testing.T) {
	st := newJSONStoreForTest(t)
	if err := st.AddAuditLog(AuditLog{
		Action:    "update",
		CreatedAt: 10,
		Detail: map[string]any{
			"nested": map[string]any{"name": "before"},
			"items":  []any{map[string]any{"id": float64(1)}},
		},
	}, 100); err != nil {
		t.Fatal(err)
	}

	page := st.QueryAuditLogs(AuditLogQuery{Limit: 1})
	if len(page.Logs) != 1 {
		t.Fatalf("expected one audit log, got %#v", page.Logs)
	}
	page.Logs[0].Detail["nested"].(map[string]any)["name"] = "mutated"
	page.Logs[0].Detail["items"].([]any)[0].(map[string]any)["id"] = float64(99)

	again := st.QueryAuditLogs(AuditLogQuery{Limit: 1})
	if got := again.Logs[0].Detail["nested"].(map[string]any)["name"]; got != "before" {
		t.Fatalf("nested detail should be copied, got %v", got)
	}
	if got := again.Logs[0].Detail["items"].([]any)[0].(map[string]any)["id"]; got != float64(1) {
		t.Fatalf("slice detail should be copied, got %v", got)
	}
}

func TestQueryAuditLogsSearchMatchesFieldsWithoutJoinedHaystack(t *testing.T) {
	st := newJSONStoreForTest(t)
	for _, entry := range []AuditLog{
		{UID: 42, TargetUID: 99, Username: "Alice", Action: "delete_user", Category: "admin", IP: "203.0.113.9", CreatedAt: 10},
		{UID: 7, Username: "Bob", Action: "login", Category: "user", IP: "198.51.100.8", CreatedAt: 20},
	} {
		if err := st.AddAuditLog(entry, 100); err != nil {
			t.Fatal(err)
		}
	}

	for _, query := range []string{"alice", "DELETE", "203.0.113", "42", "99"} {
		page := st.QueryAuditLogs(AuditLogQuery{Search: query, Limit: 10})
		if page.Total != 1 || len(page.Logs) != 1 || page.Logs[0].ID != 1 {
			t.Fatalf("search %q returned total=%d logs=%#v, want only audit #1", query, page.Total, page.Logs)
		}
	}

	compat := st.QueryAuditLogs(AuditLogQuery{Search: "alice delete_user", Limit: 10})
	if compat.Total != 1 || len(compat.Logs) != 1 || compat.Logs[0].ID != 1 {
		t.Fatalf("multi-token search should keep joined-field compatibility, got total=%d logs=%#v", compat.Total, compat.Logs)
	}
}

func TestAuditLogSearchTreatsWildcardsAsLiterals(t *testing.T) {
	st := newJSONStoreForTest(t)
	for _, entry := range []AuditLog{
		{Username: "percent%user", Action: "update"},
		{Username: "under_score", Action: "update"},
		{Username: "ordinary", Action: "update"},
	} {
		if err := st.AddAuditLog(entry, 100); err != nil {
			t.Fatal(err)
		}
	}
	for search, wantID := range map[string]int64{"%": 1, "_": 2} {
		page := st.QueryAuditLogs(AuditLogQuery{Search: search, Limit: 10})
		if page.Total != 1 || len(page.Logs) != 1 || page.Logs[0].ID != wantID {
			t.Fatalf("literal search %q returned total=%d logs=%#v", search, page.Total, page.Logs)
		}
	}
}

func TestAddAuditLogEnforcesRetentionWithoutStateRewrite(t *testing.T) {
	st := newJSONStoreForTest(t)
	for i := 0; i < 3; i++ {
		if err := st.AddAuditLog(AuditLog{Action: "event"}, 2); err != nil {
			t.Fatal(err)
		}
	}
	logs := st.ListAuditLogs()
	if got := auditLogIDs(logs); !equalInt64Slices(got, []int64{3, 2}) {
		t.Fatalf("retained IDs=%v, want [3 2]", got)
	}
	st.mu.RLock()
	defer st.mu.RUnlock()
	if len(st.state.AuditLogs) != 0 {
		t.Fatalf("audit rows leaked back into twilight_state: %#v", st.state.AuditLogs)
	}
}

func TestLoadSnapshotSplitsAuditLogsIntoDedicatedTable(t *testing.T) {
	st := newJSONStoreForTest(t)
	state := emptyState()
	state.AuditLogs = []AuditLog{{ID: 7, Action: "restored", Category: "system", CreatedAt: 42}}
	state.NextAuditLogID = 8
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.LoadSnapshot(data); err != nil {
		t.Fatal(err)
	}
	logs := st.ListAuditLogs()
	if len(logs) != 1 || logs[0].ID != 7 || logs[0].Action != "restored" {
		t.Fatalf("restored audit logs=%#v", logs)
	}
	if err := st.AddAuditLog(AuditLog{Action: "after_restore"}, 100); err != nil {
		t.Fatal(err)
	}
	logs = st.ListAuditLogs()
	if len(logs) != 2 || logs[0].ID != 8 {
		t.Fatalf("audit sequence was not restored: %#v", logs)
	}
}

func TestOpenPostgresMigratesLegacyStateAuditLogs(t *testing.T) {
	st := newJSONStoreForTest(t)
	st.mu.Lock()
	st.state.AuditLogs = []AuditLog{{ID: 12, Action: "legacy", Category: "admin", CreatedAt: 99}}
	st.state.NextAuditLogID = 13
	if err := st.saveLocked(); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	st.mu.Unlock()
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	reopened, err := OpenPostgres(ctx, testDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	logs := reopened.ListAuditLogs()
	if len(logs) != 1 || logs[0].ID != 12 || logs[0].Action != "legacy" {
		t.Fatalf("migrated audit logs=%#v", logs)
	}
	var embeddedCount int
	if err := reopened.db.QueryRowContext(ctx, `
SELECT jsonb_array_length(COALESCE(state->'audit_logs', '[]'::jsonb))
FROM twilight_state WHERE id = 1`).Scan(&embeddedCount); err != nil {
		t.Fatal(err)
	}
	if embeddedCount != 0 {
		t.Fatalf("legacy audit logs still embedded in twilight_state: %d", embeddedCount)
	}
}

func auditLogIDs(logs []AuditLog) []int64 {
	out := make([]int64, 0, len(logs))
	for _, log := range logs {
		out = append(out, log.ID)
	}
	return out
}

func equalInt64Slices(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
