package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newJSONStoreForTest(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestDeleteRegCodePreservesUsedAudit(t *testing.T) {
	st := newJSONStoreForTest(t)
	if err := st.UpsertRegCode(RegCode{Code: "USED-REG", Type: 2, Days: 30, ValidityTime: -1, UseCountLimit: 5, UseCount: 1, UsedByUIDs: []int64{101}, UsedByTelegramIDs: []int64{202}, Active: true}); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteRegCode("USED-REG"); err != nil {
		t.Fatal(err)
	}

	reg, ok := st.RegCode("USED-REG")
	if !ok {
		t.Fatal("used regcode was physically deleted")
	}
	if reg.Active {
		t.Fatalf("used regcode should be disabled after delete: %#v", reg)
	}
	if reg.UseCount != 1 || len(reg.UsedByUIDs) != 1 || reg.UsedByUIDs[0] != 101 || len(reg.UsedByTelegramIDs) != 1 || reg.UsedByTelegramIDs[0] != 202 {
		t.Fatalf("used regcode audit fields were not preserved: %#v", reg)
	}
}

func TestBatchDeleteRegCodesDeletesUnusedAndDisablesUsed(t *testing.T) {
	st := newJSONStoreForTest(t)
	if err := st.UpsertRegCode(RegCode{Code: "UNUSED-REG", Type: 1, Days: 7, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertRegCode(RegCode{Code: "USED-REG", Type: 2, Days: 7, ValidityTime: -1, UseCountLimit: 3, UseCount: 1, UsedByUIDs: []int64{303}, Active: true}); err != nil {
		t.Fatal(err)
	}

	deleted, missing, err := st.DeleteRegCodes([]string{"UNUSED-REG", "USED-REG", "MISSING-REG"})
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 2 || len(missing) != 1 || missing[0] != "MISSING-REG" {
		t.Fatalf("unexpected delete result deleted=%v missing=%v", deleted, missing)
	}
	if _, ok := st.RegCode("UNUSED-REG"); ok {
		t.Fatal("unused regcode was not physically deleted")
	}
	used, ok := st.RegCode("USED-REG")
	if !ok || used.Active || used.UseCount != 1 || len(used.UsedByUIDs) != 1 || used.UsedByUIDs[0] != 303 {
		t.Fatalf("used regcode should be disabled with audit preserved: ok=%v reg=%#v", ok, used)
	}
}

func TestUpsertRegCodeDoesNotReactivateExistingDisabledCode(t *testing.T) {
	st := newJSONStoreForTest(t)
	now := time.Now().Unix()
	st.mu.Lock()
	st.state.RegCodes["DISABLED-REG"] = RegCode{Code: "DISABLED-REG", Type: 1, Days: 7, ValidityTime: -1, UseCountLimit: 1, Active: false, CreatedAt: now, CreatedTime: now}
	st.mu.Unlock()
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	if err := st.UpsertRegCode(RegCode{Code: "DISABLED-REG", Type: 1, Days: 7, ValidityTime: -1, UseCountLimit: 1, Active: false, CreatedAt: now, CreatedTime: now, Note: "edited"}); err != nil {
		t.Fatal(err)
	}

	reg, ok := st.RegCode("DISABLED-REG")
	if !ok {
		t.Fatal("disabled regcode disappeared after update")
	}
	if reg.Active {
		t.Fatalf("disabled regcode was unexpectedly reactivated: %#v", reg)
	}
	if reg.Note != "edited" {
		t.Fatalf("upsert did not apply update: %#v", reg)
	}
}

func TestRegCodesPersistAcrossStoreReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertRegCode(RegCode{Code: "PERSIST-REG", Type: 1, Days: 30, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reg, ok := reopened.RegCode("PERSIST-REG")
	if !ok || reg.Code != "PERSIST-REG" || reg.Type != 1 {
		t.Fatalf("regcode did not persist correctly: ok=%v reg=%#v", ok, reg)
	}
}

func TestStaleStoreWriteDoesNotDropRegCodes(t *testing.T) {
	// JSON 后端从此采用进程级 flock，
	// 第二个 Open() 必须立刻得到 ErrLockBusy；之前那种 "两个 Store 共
	// 用一份 state.json" 的 stale-clobber 路径已经不可能在生产中触达。
	// 这里改为契约测试：断言锁会立刻挡住第二个 Open，而第一个 Close
	// 之后 Open 又能正常成功。
	path := filepath.Join(t.TempDir(), "state.json")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Open(path); err == nil {
		t.Fatal("expected second Open to fail while first holds the flock")
	} else if !strings.Contains(err.Error(), "locked by another Twilight process") {
		t.Fatalf("expected lock-busy error, got %v", err)
	}

	if err := first.UpsertRegCode(RegCode{Code: "NO-CLOBBER", Type: 1, Days: 30, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("expected Open to succeed after first Close; got %v", err)
	}
	defer reopened.Close()
	if _, ok := reopened.RegCode("NO-CLOBBER"); !ok {
		t.Fatal("regcode lost across lock cycle")
	}
}

func TestFailedRegCodeSaveRollsBackMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := os.Mkdir(path+".tmp", 0o700); err != nil {
		t.Fatal(err)
	}

	err = st.UpsertRegCode(RegCode{Code: "UNSAVED-REG", Type: 1, Days: 30, ValidityTime: -1, UseCountLimit: 1, Active: true})
	if err == nil {
		t.Fatal("expected save failure")
	}
	if _, ok := st.RegCode("UNSAVED-REG"); ok {
		t.Fatal("failed regcode save left unsaved code in memory")
	}
}

func TestCleanupExpiredBindCodesDoesNotDeleteSameValueRegCode(t *testing.T) {
	st := newJSONStoreForTest(t)
	now := time.Now().Unix()
	if err := st.UpsertBindCode(BindCode{Code: "SAMEVALUE123", Scene: "register", CreatedAt: now - 700, ExpiresAt: now - 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertRegCode(RegCode{Code: "SAMEVALUE123", Type: 1, Days: 30, ValidityTime: -1, UseCountLimit: 1, Active: true, CreatedAt: now - 700}); err != nil {
		t.Fatal(err)
	}

	deleted, err := st.CleanupExpiredBindCodes(now)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1", deleted)
	}
	if _, ok := st.BindCode("SAMEVALUE123"); ok {
		t.Fatal("expired bind code was not deleted")
	}
	if _, ok := st.RegCode("SAMEVALUE123"); !ok {
		t.Fatal("regcode with same value as bind code was deleted")
	}
}
