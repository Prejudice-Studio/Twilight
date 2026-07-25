package store

import (
	"path/filepath"
	"testing"
)

// TestUpdateUsersMatchesPerUserUpdate 防回归：批量 UpdateUsers 的逐用户结果与
// 落盘效果，必须与逐个 UpdateUser 等价——批量只是把 N 次整库落盘压成 1 次，
// 不改任何单用户语义。
func TestUpdateUsersMatchesPerUserUpdate(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a, err := st.CreateUser(User{Username: "alice", Active: false, PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateUser(User{Username: "bob", Active: false, PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}

	// 目标集含：两个真实用户 + 一个不存在的 UID + a 的重复项。
	uids := []int64{a.UID, b.UID, 999999, a.UID}
	results, batchErr := st.UpdateUsers(uids, func(u *User) error {
		u.Active = true
		u.ExpiredAt = 4102444800 // 固定未来时间戳
		return nil
	})
	if batchErr != nil {
		t.Fatalf("UpdateUsers returned error: %v", batchErr)
	}
	if got := results[a.UID]; got != nil {
		t.Fatalf("alice outcome = %v, want nil", got)
	}
	if got := results[b.UID]; got != nil {
		t.Fatalf("bob outcome = %v, want nil", got)
	}
	if got := results[999999]; got != ErrNotFound {
		t.Fatalf("missing uid outcome = %v, want ErrNotFound", got)
	}

	// 落盘生效：重新读取应看到两人都被激活且续期。
	ra, ok := st.User(a.UID)
	if !ok || !ra.Active || ra.ExpiredAt != 4102444800 {
		t.Fatalf("alice not persisted: %+v ok=%v", ra, ok)
	}
	rb, ok := st.User(b.UID)
	if !ok || !rb.Active || rb.ExpiredAt != 4102444800 {
		t.Fatalf("bob not persisted: %+v ok=%v", rb, ok)
	}
}

// TestUpdateUsersConflictIsolatedPerUser 冲突（如改成已被占用的用户名）只影响
// 该用户，其余用户照常提交——与 UpdateUser 的 ErrConflict 分支一致。
func TestUpdateUsersConflictIsolatedPerUser(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a, err := st.CreateUser(User{Username: "alice", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateUser(User{Username: "bob", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}

	// 让 a 改名成 "bob"（已被占用）→ ErrConflict；同批把 b 改名成 "carol" → 成功。
	results, batchErr := st.UpdateUsers([]int64{a.UID, b.UID}, func(u *User) error {
		if u.UID == a.UID {
			u.Username = "bob"
		} else {
			u.Username = "carol"
		}
		return nil
	})
	if batchErr != nil {
		t.Fatalf("UpdateUsers returned error: %v", batchErr)
	}
	if got := results[a.UID]; got != ErrConflict {
		t.Fatalf("alice outcome = %v, want ErrConflict", got)
	}
	if got := results[b.UID]; got != nil {
		t.Fatalf("bob outcome = %v, want nil", got)
	}

	// a 未改动（名字仍是 alice），b 已改名并可经索引反查。
	ra, _ := st.User(a.UID)
	if ra.Username != "alice" {
		t.Fatalf("alice username = %q, want alice (conflict must not persist)", ra.Username)
	}
	if _, ok := st.FindUserByUsername("carol"); !ok {
		t.Fatal("bob rename to carol not indexed")
	}
}

// TestUpdateUsersEmptyAndNoop 空目标集与「全部不存在」都不应报错，也不触发落盘。
func TestUpdateUsersEmptyAndNoop(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	results, err := st.UpdateUsers(nil, func(*User) error { return nil })
	if err != nil {
		t.Fatalf("empty UpdateUsers error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("empty results len = %d, want 0", len(results))
	}

	results, err = st.UpdateUsers([]int64{123, 456}, func(u *User) error {
		u.Active = true
		return nil
	})
	if err != nil {
		t.Fatalf("all-missing UpdateUsers error: %v", err)
	}
	if results[123] != ErrNotFound || results[456] != ErrNotFound {
		t.Fatalf("all-missing outcomes = %v, want both ErrNotFound", results)
	}
}
