package store

import (
	"path/filepath"
	"testing"
)

// 防回归：删除任意一条违规日志后再插入新日志，新 ID 必须严格大于
// 历史所有 ID。旧实现用 len()+1 作为主键，删除后会复用已用过的 ID，从而错乱
// admin UI / 操作日志的引用关系，并污染审计追溯链路。
func TestViolationLogIDDoesNotReuseAfterDelete(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for i := 0; i < 3; i++ {
		if err := st.AddViolationLog(ViolationLog{UID: int64(i + 1), Code: "X", Reason: "decoy"}); err != nil {
			t.Fatalf("add #%d: %v", i, err)
		}
	}

	logs := st.ListViolationLogs()
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(logs))
	}
	// ListViolationLogs 反序返回，找出当前最大 ID 用于稍后比较。
	maxID := int64(0)
	for _, log := range logs {
		if log.ID > maxID {
			maxID = log.ID
		}
	}

	// 删除中间那条。
	if err := st.DeleteViolationLog(2); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// 再插一条；新 ID 必须 > 之前最大 ID，禁止与已删 ID（=2）相同，
	// 也禁止与现存 ID（1, 3）相同。
	if err := st.AddViolationLog(ViolationLog{UID: 99, Code: "Y", Reason: "second"}); err != nil {
		t.Fatalf("add after delete: %v", err)
	}
	logs = st.ListViolationLogs()
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs after delete+add, got %d", len(logs))
	}
	seen := map[int64]int{}
	for _, log := range logs {
		seen[log.ID]++
	}
	for id, count := range seen {
		if count > 1 {
			t.Fatalf("violation log id %d reused %d times: %#v", id, count, logs)
		}
	}
	// 找出新插入那条（UID=99）的 ID，必须大于之前最大 ID。
	for _, log := range logs {
		if log.UID == 99 && log.ID <= maxID {
			t.Fatalf("new violation log id %d should exceed previous max %d", log.ID, maxID)
		}
	}
}

// 兜底验证：state 里历史只有几条遗留日志（NextViolationLogID = 0）时，
// ensure() 应当把计数器初始化为 max(existing IDs)+1，避免新插入条目与旧条目撞 ID。
func TestViolationLogEnsureBackfillsNextID(t *testing.T) {
	s := State{
		ViolationLogs: []ViolationLog{{ID: 7}, {ID: 12}, {ID: 3}},
	}
	s.ensure()
	if s.NextViolationLogID != 13 {
		t.Fatalf("expected NextViolationLogID=13 after ensure, got %d", s.NextViolationLogID)
	}
}
