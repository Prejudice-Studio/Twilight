package store

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestDeleteRegCodePhysicallyDeletesUsedCode(t *testing.T) {
	st := newJSONStoreForTest(t)
	if err := st.UpsertRegCode(RegCode{Code: "USED-REG", Type: 2, Days: 30, ValidityTime: -1, UseCountLimit: 5, UseCount: 1, UsedByUIDs: []int64{101}, UsedByTelegramIDs: []int64{202}, Active: true}); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteRegCode("USED-REG"); err != nil {
		t.Fatal(err)
	}

	if _, ok := st.RegCode("USED-REG"); ok {
		t.Fatal("used regcode should be physically deleted")
	}
}

func TestBatchDeleteRegCodesPhysicallyDeletesUsedAndUnused(t *testing.T) {
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
	if _, ok := st.RegCode("USED-REG"); ok {
		t.Fatal("used regcode was not physically deleted")
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
	//
	// 非 Unix 平台（Windows）上 flock_other.go 是 no-op，多进程部署的建
	// 议是切到 Postgres 后端，这里直接跳过避免 CI 误报。
	if runtime.GOOS == "windows" {
		t.Skip("flock 仅在 Unix 平台启用；Windows 不强制单进程")
	}
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

// TestSaveLockedBackupCopyDoesNotLeaveBakTmp 锁定 saveLocked 写 .bak 影子副
// 本时复用 writeFileAtomicSync：tmp 文件必须在 rename 后消失（O_EXCL 保证下
// 一次写入会因为残留 tmp 直接失败），且 .bak 内容是上一次成功写入的 state，
// 而不是当前正在写的新版本。这条不变量保护 refreshLocked 的解析失败回退路径。
func TestSaveLockedBackupCopyDoesNotLeaveBakTmp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	// 第一次写入：state.json 落盘后还没有 .bak（saveLocked 写前才会复制旧文件）。
	if err := st.UpsertRegCode(RegCode{Code: "FIRST", Type: 2, Days: 30, Active: true}); err != nil {
		t.Fatal(err)
	}

	// 第二次写入：触发 saveLocked 的 .bak 影子副本路径。
	if err := st.UpsertRegCode(RegCode{Code: "SECOND", Type: 2, Days: 30, Active: true}); err != nil {
		t.Fatal(err)
	}

	bak := path + ".bak"
	bakTmp := path + ".bak.tmp"
	if _, err := os.Stat(bakTmp); err == nil {
		t.Fatalf(".bak.tmp leaked, writeFileAtomicSync should have renamed it: %s", bakTmp)
	}
	bakData, err := os.ReadFile(bak)
	if err != nil {
		t.Fatalf("expected .bak to exist after second save, got: %v", err)
	}
	// .bak 是 *上一次* 成功写入的快照——必须包含 FIRST 但不包含 SECOND。
	s := string(bakData)
	if !strings.Contains(s, "FIRST") {
		t.Fatalf(".bak missing FIRST regcode: %s", s)
	}
	if strings.Contains(s, "SECOND") {
		t.Fatalf(".bak unexpectedly contains current state's SECOND regcode: %s", s)
	}
}
