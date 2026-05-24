package store

import (
	"path/filepath"
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
