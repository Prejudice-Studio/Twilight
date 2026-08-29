package api

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
)

// newEmailTestApp 构造一个开启邮箱子系统 + 强制绑定的 App，用于测试 gate 与验证码
// HMAC/消费流程。SMTP 主机虽配但测试里不真正发信（只调 hash / verify / gate）。
func newEmailTestApp(t *testing.T, forceBind bool) *App {
	t.Helper()
	dir := t.TempDir()
	st := newTestStore(t)
	app, err := New(config.Config{
		AppName:           "Twilight Test",
		Version:           "test",
		Host:              "127.0.0.1",
		Port:              0,
		DatabaseDir:       dir,
		DatabaseDriver:    store.BackendPostgres,
		DatabaseBackupDir: filepath.Join(dir, "backups"),
		StateFile:         filepath.Join(dir, "state.json"),
		UploadDir:         filepath.Join(dir, "uploads"),
		MaxUploadSize:     1024 * 1024,
		SessionCookie:     "twilight_session",
		SessionTTL:        time.Hour,
		CookieSameSite:    "lax",
		BotInternalSecret: "test-secret-for-email-hmac",
		// 邮箱子系统：开启 + 配齐 SMTP 关键参数让 emailConfigured 为 true。
		EmailEnabled:               true,
		SMTPHost:                   "smtp.example.com",
		SMTPPort:                   587,
		SMTPFromAddress:            "noreply@example.com",
		SMTPEncryption:             "starttls",
		EmailForceBind:             forceBind,
		EmailCodeLength:            6,
		EmailCodeType:              "numeric",
		EmailCodeTTLMinutes:        10,
		EmailResendCooldownSeconds: 60,
		EmailMaxAttempts:           5,
	}, st)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func TestHashEmailCodeDeterministicAndScoped(t *testing.T) {
	app := newEmailTestApp(t, false)
	h1 := app.hashEmailCode("id1", "123456")
	if h1 != app.hashEmailCode("id1", "123456") {
		t.Fatal("hash must be deterministic for same id+code")
	}
	if h1 == app.hashEmailCode("id2", "123456") {
		t.Fatal("hash must differ when id differs (binds code to record)")
	}
	if h1 == app.hashEmailCode("id1", "654321") {
		t.Fatal("hash must differ when code differs")
	}
}

func TestVerifyEmailCodeByIDFlow(t *testing.T) {
	app := newEmailTestApp(t, false)
	now := time.Now().Unix()
	id := "veri-1"
	code := "246810"
	rec := store.EmailVerification{
		ID: id, Purpose: emailPurposeBind, Email: "u@example.com", UID: 42,
		CodeHash: app.hashEmailCode(id, code), MaxAttempts: 5, CreatedAt: now, ExpiresAt: now + 600, LastSentAt: now,
	}
	if err := app.store().PutEmailVerification(rec); err != nil {
		t.Fatal(err)
	}
	// 错误码 → EMAIL_CODE_INVALID，记录仍在。
	if _, _, ec, _ := app.verifyEmailCodeByID(id, "000000"); ec != ErrEmailCodeInvalid {
		t.Fatalf("wrong code should return EMAIL_CODE_INVALID, got %v", ec)
	}
	// 正确码 → 通过并返回记录，记录被消费。
	got, _, ec, _ := app.verifyEmailCodeByID(id, code)
	if ec != "" {
		t.Fatalf("correct code should pass, got errcode %v", ec)
	}
	if got.UID != 42 || got.Purpose != emailPurposeBind {
		t.Fatalf("unexpected record: %+v", got)
	}
	if _, ok := app.store().EmailVerification(id); ok {
		t.Fatal("record should be consumed after successful verify")
	}
}

func TestEmailGateActiveByRole(t *testing.T) {
	app := newEmailTestApp(t, true)
	cases := []struct {
		role int
		want bool
	}{
		{store.RoleNormal, true},
		{store.RoleWhitelist, true},
		{store.RoleAdmin, false},
		{store.RoleUnrecognized, false},
	}
	for _, c := range cases {
		got := app.emailGateActive(store.User{Role: c.role})
		if got != c.want {
			t.Fatalf("role %d gate active = %v, want %v", c.role, got, c.want)
		}
	}
	// 已验证用户不再被要求验证；未验证的普通用户被要求。
	if app.emailVerificationRequired(store.User{Role: store.RoleNormal, EmailVerified: true}) {
		t.Fatal("verified normal user should not be required")
	}
	if !app.emailVerificationRequired(store.User{Role: store.RoleNormal, EmailVerified: false}) {
		t.Fatal("unverified normal user should be required")
	}
}

func TestEmailGateInactiveWhenForceBindOff(t *testing.T) {
	app := newEmailTestApp(t, false)
	if app.emailGateActive(store.User{Role: store.RoleNormal}) {
		t.Fatal("gate must be inactive when force_bind is off")
	}
}

func TestGlobalEmailGateStillAppliesToEmbyPassword(t *testing.T) {
	app := newEmailTestApp(t, true)
	user := store.User{
		Role:                                    store.RoleNormal,
		RequireEmailForEmbyPasswordChange:       false,
		RequireOldPasswordForEmbyPasswordChange: true,
	}
	if !app.passwordChangeEmailRequired(user, emailPurposeChangeEmby) {
		t.Fatal("global email verification must still apply to Emby password changes")
	}
}
