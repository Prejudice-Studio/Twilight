package store

import "testing"

func TestQueryAuditLogsStringSortUsesCaseInsensitiveOrder(t *testing.T) {
	st := newJSONStoreForTest(t)
	st.mu.Lock()
	st.state.AuditLogs = []AuditLog{
		{ID: 3, Action: "beta", Username: "charlie", CreatedAt: 30},
		{ID: 1, Action: "Alpha", Username: "bravo", CreatedAt: 10},
		{ID: 2, Action: "alpha", Username: "alpha", CreatedAt: 20},
	}
	if err := st.saveLocked(); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	st.mu.Unlock()

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
	st.mu.Lock()
	st.state.AuditLogs = []AuditLog{
		{
			ID:        1,
			Action:    "update",
			CreatedAt: 10,
			Detail: map[string]any{
				"nested": map[string]any{"name": "before"},
				"items":  []any{map[string]any{"id": float64(1)}},
			},
		},
	}
	if err := st.saveLocked(); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	st.mu.Unlock()

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
	st.mu.Lock()
	st.state.AuditLogs = []AuditLog{
		{ID: 1, UID: 42, TargetUID: 99, Username: "Alice", Action: "delete_user", Category: "admin", IP: "203.0.113.9", CreatedAt: 10},
		{ID: 2, UID: 7, Username: "Bob", Action: "login", Category: "user", IP: "198.51.100.8", CreatedAt: 20},
	}
	if err := st.saveLocked(); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	st.mu.Unlock()

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
