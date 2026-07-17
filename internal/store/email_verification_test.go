package store

import "testing"

func TestEmailVerificationConsumeOKThenGone(t *testing.T) {
	st := newJSONStoreForTest(t)
	now := int64(1000)
	rec := EmailVerification{ID: "id1", Purpose: "bind", Email: "user@example.com", UID: 7, CodeHash: "hashAAAA", MaxAttempts: 5, CreatedAt: now, ExpiresAt: now + 600, LastSentAt: now}
	if err := st.PutEmailVerification(rec); err != nil {
		t.Fatal(err)
	}
	// 错误码：Mismatch + Attempts+1，记录仍在。
	got, result, err := st.ConsumeEmailVerificationAtomic("id1", "wrongAAA", now+1)
	if err != nil || result != EmailVerificationMismatch {
		t.Fatalf("expected mismatch, got result=%v err=%v", result, err)
	}
	if got.Attempts != 1 {
		t.Fatalf("expected attempts=1, got %d", got.Attempts)
	}
	// 正确码：OK 并消费记录。
	_, result, err = st.ConsumeEmailVerificationAtomic("id1", "hashAAAA", now+2)
	if err != nil || result != EmailVerificationOK {
		t.Fatalf("expected ok, got result=%v err=%v", result, err)
	}
	if _, ok := st.EmailVerification("id1"); ok {
		t.Fatal("verified record should be consumed/deleted")
	}
	// 再次消费：NotFound。
	if _, result, _ := st.ConsumeEmailVerificationAtomic("id1", "hashAAAA", now+3); result != EmailVerificationNotFound {
		t.Fatalf("expected not-found, got %v", result)
	}
}

func TestEmailVerificationExpiredConsumed(t *testing.T) {
	st := newJSONStoreForTest(t)
	now := int64(1000)
	rec := EmailVerification{ID: "exp", Purpose: "reset_password", Email: "a@b.com", CodeHash: "hash1234", MaxAttempts: 5, ExpiresAt: now - 1, LastSentAt: now - 100}
	if err := st.PutEmailVerification(rec); err != nil {
		t.Fatal(err)
	}
	_, result, err := st.ConsumeEmailVerificationAtomic("exp", "hash1234", now)
	if err != nil || result != EmailVerificationExpired {
		t.Fatalf("expected expired, got result=%v err=%v", result, err)
	}
	if _, ok := st.EmailVerification("exp"); ok {
		t.Fatal("expired record should be deleted on consume")
	}
}

func TestEmailVerificationTooManyAttempts(t *testing.T) {
	st := newJSONStoreForTest(t)
	now := int64(1000)
	rec := EmailVerification{ID: "lim", Purpose: "bind", Email: "a@b.com", CodeHash: "realCODE", MaxAttempts: 2, ExpiresAt: now + 600, LastSentAt: now}
	if err := st.PutEmailVerification(rec); err != nil {
		t.Fatal(err)
	}
	if _, result, _ := st.ConsumeEmailVerificationAtomic("lim", "badCODE1", now); result != EmailVerificationMismatch {
		t.Fatalf("first wrong attempt should be mismatch, got %v", result)
	}
	// 第二次错误达上限：TooMany，记录作废。
	if _, result, _ := st.ConsumeEmailVerificationAtomic("lim", "badCODE2", now); result != EmailVerificationTooMany {
		t.Fatalf("second wrong attempt should be too-many, got %v", result)
	}
	if _, ok := st.EmailVerification("lim"); ok {
		t.Fatal("record should be deleted after exhausting attempts")
	}
}

func TestPutEmailVerificationReplacesSamePurposeEmail(t *testing.T) {
	st := newJSONStoreForTest(t)
	now := int64(1000)
	_ = st.PutEmailVerification(EmailVerification{ID: "old", Purpose: "bind", Email: "Dup@Example.com", CodeHash: "h1", MaxAttempts: 5, ExpiresAt: now + 600, LastSentAt: now})
	_ = st.PutEmailVerification(EmailVerification{ID: "new", Purpose: "bind", Email: "dup@example.com", CodeHash: "h2", MaxAttempts: 5, ExpiresAt: now + 600, LastSentAt: now + 5})
	if _, ok := st.EmailVerification("old"); ok {
		t.Fatal("old record for same purpose+email should be replaced (case-insensitive)")
	}
	got, ok := st.FindActiveEmailVerification("bind", "dup@example.com", now+6)
	if !ok || got.ID != "new" {
		t.Fatalf("expected newest active record, got ok=%v id=%v", ok, got.ID)
	}
}

func TestCleanupExpiredEmailVerifications(t *testing.T) {
	st := newJSONStoreForTest(t)
	now := int64(1000)
	_ = st.PutEmailVerification(EmailVerification{ID: "live", Purpose: "bind", Email: "live@x.com", CodeHash: "h", MaxAttempts: 5, ExpiresAt: now + 600, LastSentAt: now})
	_ = st.PutEmailVerification(EmailVerification{ID: "dead", Purpose: "bind", Email: "dead@x.com", CodeHash: "h", MaxAttempts: 5, ExpiresAt: now - 1, LastSentAt: now - 100})
	deleted, err := st.CleanupExpiredEmailVerifications(now)
	if err != nil || deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d err=%v", deleted, err)
	}
	if _, ok := st.EmailVerification("live"); !ok {
		t.Fatal("live record should survive cleanup")
	}
}

func TestEmailVerificationAdminSnapshot(t *testing.T) {
	st := newJSONStoreForTest(t)
	now := int64(1000)
	alice, err := st.CreateUser(User{Username: "alice", Email: "alice@example.com", EmailVerified: true, EmailVerifiedAt: now - 10, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := st.CreateUser(User{Username: "bob", Email: "bob@example.com", Active: false})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser(User{Username: "carol", Active: true}); err != nil {
		t.Fatal(err)
	}
	_ = st.PutEmailVerification(EmailVerification{ID: "old", Purpose: "bind", Email: "alice@example.com", UID: alice.UID, CodeHash: "h1", ExpiresAt: now - 1, LastSentAt: now - 10})
	_ = st.PutEmailVerification(EmailVerification{ID: "new", Purpose: "bind", Email: "bob@example.com", UID: bob.UID, CodeHash: "h2", ExpiresAt: now + 600, LastSentAt: now + 10})

	snapshot := st.EmailVerificationAdminSnapshot(now)
	if len(snapshot.Records) != 2 {
		t.Fatalf("records len = %d, want 2", len(snapshot.Records))
	}
	if snapshot.Records[0].ID != "new" || snapshot.Records[1].ID != "old" {
		t.Fatalf("records not sorted by LastSentAt desc: %#v", snapshot.Records)
	}
	if snapshot.UsernamesByUID[alice.UID] != "alice" || snapshot.UsernamesByUID[bob.UID] != "bob" {
		t.Fatalf("username map = %#v", snapshot.UsernamesByUID)
	}
	if snapshot.ExpiredPending != 1 {
		t.Fatalf("expired pending = %d, want 1", snapshot.ExpiredPending)
	}
	if snapshot.Verified != 1 {
		t.Fatalf("verified accounts = %d, want 1", snapshot.Verified)
	}
	if len(snapshot.Accounts) != 2 {
		t.Fatalf("accounts len = %d, want 2", len(snapshot.Accounts))
	}
	if snapshot.Accounts[0].UID != alice.UID || snapshot.Accounts[1].UID != bob.UID {
		t.Fatalf("accounts should follow user UID order and skip empty emails: %#v", snapshot.Accounts)
	}
}

func TestSetUserEmailVerifiedConflictAndForce(t *testing.T) {
	st := newJSONStoreForTest(t)
	now := int64(1000)
	u1, err := st.CreateUser(User{Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	u2, err := st.CreateUser(User{Username: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetUserEmailVerifiedAtomic(u1.UID, "shared@example.com", true, false, now); err != nil {
		t.Fatalf("first verify should succeed: %v", err)
	}
	// 另一账号验证同一邮箱（非 force）→ 冲突。
	if _, err := st.SetUserEmailVerifiedAtomic(u2.UID, "shared@example.com", true, false, now); err != ErrConflict {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	// force=true 覆盖冲突。
	if _, err := st.SetUserEmailVerifiedAtomic(u2.UID, "shared@example.com", true, true, now); err != nil {
		t.Fatalf("force verify should succeed: %v", err)
	}
	if owner, ok := st.EmailVerifiedOwner("SHARED@example.com", u1.UID); !ok || owner.UID != u2.UID {
		t.Fatalf("expected u2 to own shared email, got ok=%v owner=%v", ok, owner.UID)
	}
}

func TestFindUserByEmailVerifiedRequiresVerified(t *testing.T) {
	st := newJSONStoreForTest(t)
	now := int64(1000)
	u, err := st.CreateUser(User{Username: "carol", Email: "carol@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	// 未验证邮箱不应被找回命中。
	if _, ok := st.FindUserByEmailVerified("carol@example.com"); ok {
		t.Fatal("unverified email must not be resolvable for reset")
	}
	if _, err := st.SetUserEmailVerifiedAtomic(u.UID, "carol@example.com", true, false, now); err != nil {
		t.Fatal(err)
	}
	got, ok := st.FindUserByEmailVerified("Carol@Example.com")
	if !ok || got.UID != u.UID {
		t.Fatalf("verified email should resolve case-insensitively, got ok=%v", ok)
	}
	if _, err := st.SetUserEmailVerifiedAtomic(u.UID, "", false, false, now+1); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.FindUserByEmailVerified("carol@example.com"); ok {
		t.Fatal("revoked verified email remained indexed")
	}
}
