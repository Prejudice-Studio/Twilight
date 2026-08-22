package api

import (
	"encoding/json"
	"net/http"
	"strings"
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

	dto := app.emailVerificationDTO(rec, now, "alice")
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
	if app.emailVerificationDTO(expiredRec, now, "alice")["expired"] != true {
		t.Fatal("record past ExpiresAt should be flagged expired")
	}

	if got := len(app.store().ListEmailVerifications()); got != 1 {
		t.Fatalf("ListEmailVerifications len = %d, want 1", got)
	}
}

func TestAdminEmailVerificationViewPaginates(t *testing.T) {
	app := newEmailTestApp(t, false)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	owner, err := app.store().CreateUser(store.User{Username: "email-owner", Email: "owner@example.com", EmailVerified: true, EmailVerifiedAt: time.Now().Unix()})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if err := app.store().PutEmailVerification(store.EmailVerification{
		ID: "paged-email", Purpose: emailPurposeBind, Email: owner.Email, UID: owner.UID,
		CodeHash: "TOPSECRETPAGEDHASH", MaxAttempts: 5, CreatedAt: now, ExpiresAt: now + 600, LastSentAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	response := doJSON(app, http.MethodGet, "/api/v1/admin/email/verifications?view=accounts&page=1&per_page=1&search=email-owner", ``, admin)
	if response.Code != http.StatusOK {
		t.Fatalf("admin email page status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			View     string           `json:"view"`
			Accounts []map[string]any `json:"accounts"`
			Pending  []map[string]any `json:"pending"`
			Total    map[string]int   `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.View != "accounts" || len(payload.Data.Accounts) != 1 || len(payload.Data.Pending) != 0 || payload.Data.Total["accounts"] != 1 {
		t.Fatalf("unexpected paged email response: %#v", payload.Data)
	}

	compat := doJSON(app, http.MethodGet, "/api/v1/admin/email/verifications", ``, admin)
	if compat.Code != http.StatusOK {
		t.Fatalf("compat email response status=%d body=%s", compat.Code, compat.Body.String())
	}
	payload = struct {
		Data struct {
			View     string           `json:"view"`
			Accounts []map[string]any `json:"accounts"`
			Pending  []map[string]any `json:"pending"`
			Total    map[string]int   `json:"total"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(compat.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.View != "" || len(payload.Data.Accounts) != 1 || len(payload.Data.Pending) != 1 {
		t.Fatalf("unexpected compatibility response: %#v", payload.Data)
	}
	if strings.Contains(compat.Body.String(), "TOPSECRETPAGEDHASH") {
		t.Fatal("admin email response leaked verification code hash")
	}
}
