package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestV2AdminEmailResourcesUseSanitizedPagedView(t *testing.T) {
	app := newEmailTestApp(t, false)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	owner, err := app.store().CreateUser(store.User{Username: "v2-email-owner", Email: "owner@example.com", EmailVerified: true, EmailVerifiedAt: time.Now().Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().PutEmailVerification(store.EmailVerification{
		ID: "v2-email-record", Purpose: emailPurposeBind, Email: owner.Email, UID: owner.UID,
		CodeHash: "SHOULD_NOT_LEAK", MaxAttempts: 5, CreatedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(), LastSentAt: time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}

	response := doJSON(app, http.MethodGet, "/api/v2/admin/email/verifications?view=accounts&page=1&per_page=25&search=v2-email-owner", "", admin)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("v2 email status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}
	var payload struct {
		Data struct {
			View     string           `json:"view"`
			Accounts []map[string]any `json:"accounts"`
			Pending  []map[string]any `json:"pending"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.View != "accounts" || len(payload.Data.Accounts) != 1 || len(payload.Data.Pending) != 0 || strings.Contains(response.Body.String(), "SHOULD_NOT_LEAK") {
		t.Fatalf("unexpected sanitized v2 email response: %#v body=%s", payload.Data, response.Body.String())
	}

	deleted := doJSON(app, http.MethodDelete, "/api/v2/admin/email/verifications/v2-email-record", "", admin)
	if deleted.Code != http.StatusOK {
		t.Fatalf("v2 email revoke status=%d body=%s", deleted.Code, deleted.Body.String())
	}
}
