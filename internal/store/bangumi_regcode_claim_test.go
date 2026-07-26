package store

import (
	"sync"
	"testing"
)

// TestClaimBangumiRegcodeFirstWriteWins 锁定「一个 Bangumi 账号永远只能领一张注册码」
// 这一全局不变量：首个 claim 生效并 claimed=true；同一 Bangumi id 的后续 claim 一律
// 不覆盖、返回既有记录且 claimed=false。这是自助发码通道幂等复用旧码的持久层依据。
func TestClaimBangumiRegcodeFirstWriteWins(t *testing.T) {
	st := newJSONStoreForTest(t)

	stored, claimed, err := st.ClaimBangumiRegcode(BangumiRegcodeClaim{
		BangumiID: "899391", Code: "CODE-A", UID: 7, Username: "alice", Days: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("first claim must succeed with claimed=true")
	}
	if stored.Code != "CODE-A" || stored.ClaimedAt == 0 {
		t.Fatalf("stored claim malformed: %+v", stored)
	}

	// 同一 Bangumi id 再次 claim（哪怕换了码/用户）必须复用旧记录，绝不覆盖。
	stored2, claimed2, err := st.ClaimBangumiRegcode(BangumiRegcodeClaim{
		BangumiID: "899391", Code: "CODE-B", UID: 9, Username: "mallory", Days: 90,
	})
	if err != nil {
		t.Fatal(err)
	}
	if claimed2 {
		t.Fatal("second claim for same bangumi id must not succeed")
	}
	if stored2.Code != "CODE-A" || stored2.UID != 7 {
		t.Fatalf("second claim must return the original record, got: %+v", stored2)
	}

	// 不同 Bangumi id 互不影响。
	if _, claimed3, err := st.ClaimBangumiRegcode(BangumiRegcodeClaim{
		BangumiID: "123456", Code: "CODE-C", UID: 11,
	}); err != nil {
		t.Fatal(err)
	} else if !claimed3 {
		t.Fatal("claim for a different bangumi id must succeed")
	}

	// 读接口口径与写入一致。
	got, ok := st.BangumiRegcodeClaimByID("899391")
	if !ok || got.Code != "CODE-A" {
		t.Fatalf("BangumiRegcodeClaimByID mismatch: ok=%v got=%+v", ok, got)
	}
	if _, ok := st.BangumiRegcodeClaimByID("nope"); ok {
		t.Fatal("unknown bangumi id must report absent")
	}
}

// TestClaimBangumiRegcodeInvalidInput 校验缺失去重键 / 码时拒绝写入。
func TestClaimBangumiRegcodeInvalidInput(t *testing.T) {
	st := newJSONStoreForTest(t)
	if _, _, err := st.ClaimBangumiRegcode(BangumiRegcodeClaim{Code: "X"}); err == nil {
		t.Fatal("empty bangumi id must be rejected")
	}
	if _, _, err := st.ClaimBangumiRegcode(BangumiRegcodeClaim{BangumiID: "1"}); err == nil {
		t.Fatal("empty code must be rejected")
	}
}

// TestClaimBangumiRegcodeConcurrent 并发下同一 Bangumi id 仅一次 claimed=true，
// 其余全部复用同一条码——防「一个 Bangumi 领多张」的核心竞态断言。
func TestClaimBangumiRegcodeConcurrent(t *testing.T) {
	st := newJSONStoreForTest(t)
	const n = 16
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	codes := map[string]struct{}{}
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			stored, claimed, err := st.ClaimBangumiRegcode(BangumiRegcodeClaim{
				BangumiID: "555", Code: "CODE-" + strconv36(int64(i)), UID: int64(i),
			})
			if err != nil {
				t.Errorf("claim %d error: %v", i, err)
				return
			}
			mu.Lock()
			if claimed {
				successes++
			}
			codes[stored.Code] = struct{}{}
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	if successes != 1 {
		t.Fatalf("exactly one concurrent claim must win, got %d", successes)
	}
	if len(codes) != 1 {
		t.Fatalf("all callers must observe the same winning code, got %d distinct", len(codes))
	}
}
