package store

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

// ClearEmbyGrantForUnboundUsers 必须只解锁"无 Emby 账号"用户的注册资格，并回退其
// 占用的注册码/邀请码使用记录；已绑定 Emby 与 PendingEmby 在飞的用户保持不动。
func TestClearEmbyGrantForUnboundUsers(t *testing.T) {
	st := newJSONStoreForTest(t)

	// blocked：无 Emby、非 pending，被迁移误锁——应被清理。
	blocked, err := st.CreateUser(User{Username: "blocked", PasswordHash: "x", EmbyGrantLocked: true, RegistrationSource: "regcode", RegistrationCode: "RC1"})
	if err != nil {
		t.Fatal(err)
	}
	// bound：已绑定 Emby——不可重置注册资格。
	bound, err := st.CreateUser(User{Username: "bound", PasswordHash: "x", EmbyID: "emby-x", EmbyGrantLocked: true, RegistrationSource: "regcode", RegistrationCode: "RC2"})
	if err != nil {
		t.Fatal(err)
	}
	// pending：待开通在飞——保留其资格。
	pending, err := st.CreateUser(User{Username: "pending", PasswordHash: "x", PendingEmby: true, EmbyGrantLocked: true, RegistrationSource: "invite", RegistrationCode: "INV1"})
	if err != nil {
		t.Fatal(err)
	}

	// 直接种入码侧记录（精确控制字段，避免 Upsert 归一化干扰）：
	//   RC1 被 blocked 用满（Active=false），清理后回退 UseCount=0 并恢复可用。
	//   INV1 + 邀请关系：blocked 作为被邀请者。
	st.mu.Lock()
	st.state.RegCodes["RC1"] = RegCode{Code: "RC1", Type: 1, Days: 30, ValidityTime: -1, UseCountLimit: 1, UseCount: 1, UsedBy: blocked.UID, UsedByUIDs: []int64{blocked.UID}, Active: false}
	st.state.InviteCodes["INV1"] = InviteCode{Code: "INV1", InviterUID: 999, UseCountLimit: 1, UseCount: 1, Used: true, UsedByUID: blocked.UID, Active: false}
	st.state.InviteRelations[blocked.UID] = InviteRelation{ParentUID: 999, ChildUID: blocked.UID, Code: "INV1"}
	if err := st.saveLocked(); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	st.mu.Unlock()

	res, err := st.ClearEmbyGrantForUnboundUsers([]int64{blocked.UID, bound.UID, pending.UID, 4242})
	if err != nil {
		t.Fatal(err)
	}

	if !containsInt64(res.Cleared, blocked.UID) {
		t.Fatalf("blocked must be cleared: %+v", res)
	}
	if !containsInt64(res.SkippedHasEmby, bound.UID) {
		t.Fatalf("bound must be skipped (has emby): %+v", res)
	}
	if !containsInt64(res.SkippedPending, pending.UID) {
		t.Fatalf("pending must be skipped (pending emby): %+v", res)
	}
	if !containsInt64(res.Missing, 4242) {
		t.Fatalf("4242 must be missing: %+v", res)
	}
	if res.RegcodeRefs != 1 {
		t.Fatalf("expected 1 regcode ref removed, got %d", res.RegcodeRefs)
	}
	if res.InviteRefs == 0 {
		t.Fatalf("expected invite ref removed, got %d", res.InviteRefs)
	}

	// 用户侧：blocked 解锁；bound/pending 保持锁定。
	if got, _ := st.User(blocked.UID); got.EmbyGrantLocked || got.RegistrationSource != "" || got.RegistrationCode != "" {
		t.Fatalf("blocked grant not cleared: %+v", got)
	}
	if got, _ := st.User(bound.UID); !got.EmbyGrantLocked {
		t.Fatal("bound must keep grant lock")
	}
	if got, _ := st.User(pending.UID); !got.EmbyGrantLocked {
		t.Fatal("pending must keep grant lock")
	}

	// 码侧：RC1 回退并恢复可用。
	rc, ok := st.RegCode("RC1")
	if !ok {
		t.Fatal("RC1 missing")
	}
	if rc.UseCount != 0 || rc.UsedBy != 0 || len(rc.UsedByUIDs) != 0 || !rc.Active {
		t.Fatalf("RC1 not rolled back: %+v", rc)
	}

	// 邀请关系断开 + 邀请码回退。
	if _, ok := st.ParentOf(blocked.UID); ok {
		t.Fatal("invite relation should be detached")
	}
	inv, ok := st.InviteCode("INV1")
	if !ok {
		t.Fatal("INV1 missing")
	}
	if inv.UsedByUID != 0 || inv.Used || inv.UseCount != 0 || !inv.Active {
		t.Fatalf("INV1 not rolled back: %+v", inv)
	}
}

func TestDetachInviteClearsInviteCodeUsage(t *testing.T) {
	st := newJSONStoreForTest(t)
	child, err := st.CreateUser(User{Username: "detach-child", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	st.mu.Lock()
	st.state.InviteCodes["INV-DETACH"] = InviteCode{Code: "INV-DETACH", InviterUID: 999, UseCountLimit: 1, UseCount: 1, Used: true, UsedByUID: child.UID, Active: false}
	st.state.InviteRelations[child.UID] = InviteRelation{ParentUID: 999, ChildUID: child.UID, Code: "INV-DETACH"}
	if err := st.saveLocked(); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	st.mu.Unlock()

	if err := st.DetachInvite(child.UID); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.ParentOf(child.UID); ok {
		t.Fatal("invite relation should be detached")
	}
	invite, ok := st.InviteCode("INV-DETACH")
	if !ok {
		t.Fatal("invite code should remain after detach")
	}
	if invite.UsedByUID != 0 || invite.Used || invite.UseCount != 0 || !invite.Active {
		t.Fatalf("invite code usage should be cleared after detach: %#v", invite)
	}
}

func TestDetachInviteClearsLegacyRelationByChildUID(t *testing.T) {
	st := newJSONStoreForTest(t)
	child, err := st.CreateUser(User{Username: "legacy-detach-child", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	st.mu.Lock()
	st.state.InviteCodes["INV-LEGACY-DETACH"] = InviteCode{Code: "INV-LEGACY-DETACH", InviterUID: 999, UseCountLimit: 1, UseCount: 1, Used: true, UsedByUID: child.UID, Active: false}
	st.state.InviteRelations[424242] = InviteRelation{ParentUID: 999, ChildUID: child.UID, Code: "INV-LEGACY-DETACH"}
	if err := st.saveLocked(); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	st.mu.Unlock()

	if _, ok := st.ParentOf(child.UID); !ok {
		t.Fatal("legacy relation should be visible before detach")
	}
	if children := st.ChildrenOf(999); len(children) != 1 || children[0].ChildUID != child.UID {
		t.Fatalf("legacy child should be visible before detach: %#v", children)
	}
	if err := st.DetachInvite(child.UID); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.ParentOf(child.UID); ok {
		t.Fatal("legacy relation should be detached")
	}
	if children := st.ChildrenOf(999); len(children) != 0 {
		t.Fatalf("legacy child should not reappear after refresh: %#v", children)
	}
	invite, ok := st.InviteCode("INV-LEGACY-DETACH")
	if !ok {
		t.Fatal("invite code should remain after legacy detach")
	}
	if invite.UsedByUID != 0 || invite.Used || invite.UseCount != 0 || !invite.Active {
		t.Fatalf("legacy invite code usage should be cleared after detach: %#v", invite)
	}
}

func TestConsumeInviteRejectsLegacyRelationByChildUID(t *testing.T) {
	st := newJSONStoreForTest(t)
	child, err := st.CreateUser(User{Username: "legacy-consume-child", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	st.mu.Lock()
	st.state.InviteCodes["INV-NEW-PARENT"] = InviteCode{Code: "INV-NEW-PARENT", InviterUID: 1000, UseCountLimit: 1, Active: true}
	st.state.InviteRelations[424243] = InviteRelation{ParentUID: 999, ChildUID: child.UID, Code: "INV-OLD-PARENT"}
	if err := st.saveLocked(); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	st.mu.Unlock()

	if _, _, err := st.ConsumeInviteCodeAndUpdateUser("INV-NEW-PARENT", child.UID, 10, 0, nil); err != ErrConflict {
		t.Fatalf("expected ErrConflict for legacy parent relation, got %v", err)
	}
}

func TestMediaRequestStatusHelpers(t *testing.T) {
	cases := map[string]string{
		"pending":        MediaRequestStatusUnhandled,
		"pending_review": MediaRequestStatusUnhandled,
		"approved":       MediaRequestStatusAccepted,
		"reject":         MediaRequestStatusRejected,
		"done":           MediaRequestStatusCompleted,
		"download":       MediaRequestStatusDownloading,
	}
	for input, want := range cases {
		if got := NormalizeMediaRequestStatus(input); got != want {
			t.Fatalf("NormalizeMediaRequestStatus(%q)=%q want %q", input, got, want)
		}
	}
	if got := NormalizeMediaRequestStatus(""); got != "" {
		t.Fatalf("empty status should be invalid outside create defaulting, got %q", got)
	}
	if !MediaRequestStatusMatches(MediaRequestStatusDownloading, "active") {
		t.Fatal("active filter should include downloading requests")
	}
	if MediaRequestStatusMatches(MediaRequestStatusDownloading, "pending") {
		t.Fatal("pending filter should include only unhandled requests")
	}
	if IsActiveMediaRequestStatus(MediaRequestStatusCompleted) {
		t.Fatal("completed request should not be active")
	}
	if got := MediaRequestStatusText(MediaRequestStatusAccepted); got != "已接受" {
		t.Fatalf("MediaRequestStatusText accepted=%q", got)
	}
}

func TestUpdateMediaRequestStatusNoteModes(t *testing.T) {
	st := newJSONStoreForTest(t)
	req, err := st.CreateMediaRequest(MediaRequest{UID: 1, Title: "Movie", Source: "tmdb", MediaID: 42, MediaType: "movie"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := st.UpdateMediaRequestStatus(req.ID, "accepted", "first note", false)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != MediaRequestStatusAccepted || updated.AdminNote != "first note" {
		t.Fatalf("unexpected first status update: %#v", updated)
	}
	updated, err = st.UpdateMediaRequestStatus(req.ID, "downloading", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != MediaRequestStatusDownloading || updated.AdminNote != "first note" {
		t.Fatalf("empty admin note should preserve existing note: %#v", updated)
	}
	updated, err = st.UpdateMediaRequestStatus(req.ID, "completed", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != MediaRequestStatusCompleted || updated.AdminNote != "" {
		t.Fatalf("replace note mode should clear note: %#v", updated)
	}
	if _, err := st.UpdateMediaRequestStatus(req.ID, "", "", true); err != ErrInvalid {
		t.Fatalf("expected ErrInvalid for empty status update, got %v", err)
	}
}

func TestCreateMediaRequestRejectsInvalidStatus(t *testing.T) {
	st := newJSONStoreForTest(t)
	if _, err := st.CreateMediaRequest(MediaRequest{UID: 1, Title: "Movie", Source: "tmdb", MediaID: 42, MediaType: "movie", Status: "wat"}); err != ErrInvalid {
		t.Fatalf("expected ErrInvalid for bad create status, got %v", err)
	}
	req, err := st.CreateMediaRequest(MediaRequest{UID: 1, Title: "Movie", Source: "tmdb", MediaID: 43, MediaType: "movie"})
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != MediaRequestStatusUnhandled {
		t.Fatalf("empty create status should default to unhandled, got %q", req.Status)
	}
}

func TestCreateMediaRequestEnforcesUserActiveLimitAtomically(t *testing.T) {
	st := newJSONStoreForTest(t)
	const attempts = 20
	var wg sync.WaitGroup
	errs := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := st.CreateMediaRequestWithOptions(MediaRequest{
				UID:       42,
				Title:     "Movie",
				Source:    "tmdb",
				MediaID:   int64(1000 + i),
				MediaType: "movie",
			}, MediaRequestCreateOptions{UserActiveLimit: 1})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)

	success := 0
	limited := 0
	for err := range errs {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrMediaRequestUserActiveLimit):
			limited++
		default:
			t.Fatalf("unexpected create error: %v", err)
		}
	}
	if success != 1 || limited != attempts-1 {
		t.Fatalf("expected 1 success and %d limited, got success=%d limited=%d", attempts-1, success, limited)
	}
	if got := st.ActiveMediaRequestCount(42); got != 1 {
		t.Fatalf("active request count=%d want 1", got)
	}
}

func TestCreateMediaRequestEnforcesGlobalActiveLimit(t *testing.T) {
	st := newJSONStoreForTest(t)
	if _, err := st.CreateMediaRequestWithOptions(MediaRequest{UID: 1, Title: "One", Source: "tmdb", MediaID: 1, MediaType: "movie"}, MediaRequestCreateOptions{GlobalActiveLimit: 1}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := st.CreateMediaRequestWithOptions(MediaRequest{UID: 2, Title: "Two", Source: "tmdb", MediaID: 2, MediaType: "movie"}, MediaRequestCreateOptions{GlobalActiveLimit: 1}); !errors.Is(err, ErrMediaRequestGlobalActiveLimit) {
		t.Fatalf("expected global active limit, got %v", err)
	}
}

func TestCreateTicketEnforcesUserOpenLimitAtomically(t *testing.T) {
	st := newJSONStoreForTest(t)
	const attempts = 20
	var wg sync.WaitGroup
	errs := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := st.CreateTicket(Ticket{
				UID:      100,
				Username: "user",
				Title:    "ticket",
				Content:  "content",
				Type:     TicketTypeDefault,
				Priority: TicketPriorityMedium,
			}, 1, 0)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	success := 0
	userLimited := 0
	for err := range errs {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrTicketUserOpenLimit):
			userLimited++
		default:
			t.Fatalf("unexpected create ticket error: %v", err)
		}
	}
	if success != 1 || userLimited != attempts-1 {
		t.Fatalf("expected one success and %d user-limit errors, got success=%d limit=%d", attempts-1, success, userLimited)
	}
	if got := st.CountUserOpenTickets(100); got != 1 {
		t.Fatalf("expected one persisted open ticket, got %d", got)
	}
}

func TestCreateTicketEnforcesGlobalOpenLimit(t *testing.T) {
	st := newJSONStoreForTest(t)
	if _, err := st.CreateTicket(Ticket{UID: 1, Username: "a", Title: "one", Content: "content", Type: TicketTypeDefault}, 0, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateTicket(Ticket{UID: 2, Username: "b", Title: "two", Content: "content", Type: TicketTypeDefault}, 0, 1); !errors.Is(err, ErrTicketGlobalOpenLimit) {
		t.Fatalf("expected global limit error, got %v", err)
	}
}

func TestUpdateTicketCanAppendReplyAtomically(t *testing.T) {
	st := newJSONStoreForTest(t)
	ticket, err := st.CreateTicket(Ticket{UID: 1, Username: "user", Title: "need help", Content: "content", Type: TicketTypeDefault}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	note := "admin is checking"
	updated, err := st.UpdateTicket(ticket.ID, TicketUpdate{
		AdminNote: &note,
		Reply: &TicketReply{
			UID:      2,
			Username: "admin",
			Role:     RoleAdmin,
			Content:  note,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.AdminNote != note {
		t.Fatalf("admin note not updated: %#v", updated)
	}
	if updated.Status != TicketStatusInProgress {
		t.Fatalf("admin reply should move open ticket to in_progress, got %q", updated.Status)
	}
	if len(updated.Replies) != 1 || updated.Replies[0].Content != note || updated.Replies[0].CreatedAt == 0 {
		t.Fatalf("reply should be appended with timestamp: %#v", updated.Replies)
	}
}

func TestClosedTicketStorePolicyAllowsAdminOnly(t *testing.T) {
	st := newJSONStoreForTest(t)
	ticket, err := st.CreateTicket(Ticket{UID: 1, Username: "user", Title: "closed", Content: "content", Type: TicketTypeDefault}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	closed := TicketStatusClosed
	if _, err := st.UpdateTicket(ticket.ID, TicketUpdate{Status: &closed}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddTicketReply(ticket.ID, TicketReply{UID: 1, Username: "user", Role: RoleNormal, Content: "user reply"}); !errors.Is(err, ErrTicketClosed) {
		t.Fatalf("expected user reply to closed ticket to fail with ErrTicketClosed, got %v", err)
	}
	if _, err := st.AddTicketAttachment(ticket.ID, TicketAttachment{Filename: "a.png"}, RoleNormal); !errors.Is(err, ErrTicketClosed) {
		t.Fatalf("expected user attachment to closed ticket to fail with ErrTicketClosed, got %v", err)
	}
	updated, err := st.AddTicketReply(ticket.ID, TicketReply{UID: 2, Username: "admin", Role: RoleAdmin, Content: "admin note"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != TicketStatusClosed || len(updated.Replies) != 1 {
		t.Fatalf("admin reply should preserve closed status and append reply: %#v", updated)
	}
	if _, err := st.AddTicketAttachment(ticket.ID, TicketAttachment{Filename: "admin.png"}, RoleAdmin); err != nil {
		t.Fatalf("admin attachment should be allowed on closed ticket: %v", err)
	}
}

func TestTicketTypeStoreRejectsInvalidNames(t *testing.T) {
	st := newJSONStoreForTest(t)
	before := st.TicketTypes()
	if err := st.AddTicketType("   "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for empty ticket type, got %v", err)
	}
	if err := st.AddTicketType(strings.Repeat("x", 51)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for long ticket type, got %v", err)
	}
	if err := st.DeleteTicketType(""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid deleting empty ticket type, got %v", err)
	}
	if _, err := st.RenameTicketType(TicketTypeDefault, ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid renaming to empty ticket type, got %v", err)
	}
	after := st.TicketTypes()
	if len(after) != len(before) || after[0] != before[0] {
		t.Fatalf("invalid type operations should not mutate types, before=%v after=%v", before, after)
	}
}

func containsInt64(xs []int64, v int64) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
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

func TestRepairLegacyTelegramBindResidueClearsPersistedBindCodes(t *testing.T) {
	st := newJSONStoreForTest(t)
	if err := st.UpsertBindCode(BindCode{Code: "LEGACYTG1", Scene: "register", Confirmed: true, TelegramID: 12345, CreatedAt: 1, ExpiresAt: 2}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertBindCode(BindCode{Code: "LEGACYTG2", Scene: "user", UID: 99, Confirmed: true, TelegramID: 67890, CreatedAt: 1, ExpiresAt: 2}); err != nil {
		t.Fatal(err)
	}
	deleted, err := st.RepairLegacyTelegramBindResidue()
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Fatalf("deleted=%d, want 2", deleted)
	}
	if _, ok := st.BindCode("LEGACYTG1"); ok {
		t.Fatal("legacy register bind code should be removed")
	}
	if _, ok := st.BindCode("LEGACYTG2"); ok {
		t.Fatal("legacy user bind code should be removed")
	}
	deleted, err = st.RepairLegacyTelegramBindResidue()
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Fatalf("second repair should be idempotent, deleted=%d", deleted)
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
