package api

import (
	"testing"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

// TestEmailVerificationDTOSanitizes 锁定管理员审查 DTO 的安全约定：永不外泄 CodeHash，
// 正确解析关联本地账号用户名，并按 ExpiresAt 标注过期状态。
func TestEmailVerificationDTOSanitizes(t *testing.T) {
	app := newEmailTestApp(t, false)
	u, err := app.store().CreateUser(store.User{Username: "alice", Role: store.RoleNormal})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	rec := store.EmailVerification{
		ID: "veri-x", Purpose: emailPurposeBind, Email: "alice@example.com", UID: u.UID,
		CodeHash: "TOPSECRETHASH", Attempts: 1, MaxAttempts: 5,
		CreatedAt: now, ExpiresAt: now + 600, LastSentAt: now,
	}
	if err := app.store().PutEmailVerification(rec); err != nil {
		t.Fatal(err)
	}

	dto := app.emailVerificationDTO(rec, now)
	if _, leaked := dto["code_hash"]; leaked {
		t.Fatal("DTO must not expose a code_hash field")
	}
	for k, v := range dto {
		if s, ok := v.(string); ok && s == "TOPSECRETHASH" {
			t.Fatalf("DTO field %q leaked the code hash value", k)
		}
	}
	if dto["username"] != "alice" {
		t.Fatalf("username = %v, want alice", dto["username"])
	}
	if dto["expired"] != false {
		t.Fatalf("expired = %v, want false for a future ExpiresAt", dto["expired"])
	}

	expiredRec := rec
	expiredRec.ExpiresAt = now - 1
	if app.emailVerificationDTO(expiredRec, now)["expired"] != true {
		t.Fatal("record past ExpiresAt should be flagged expired")
	}

	if got := len(app.store().ListEmailVerifications()); got != 1 {
		t.Fatalf("ListEmailVerifications len = %d, want 1", got)
	}
}
