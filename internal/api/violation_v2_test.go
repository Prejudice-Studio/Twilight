package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestV2ViolationResourcesAreAdminOnlyAndNoStore(t *testing.T) {
	app := newTestApp(t)
	userCookies := registerAndLogin(t, app, "violation-user", "User12345678")
	denied := doJSON(app, http.MethodGet, "/api/v2/admin/violations", "", userCookies)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("normal user violation access status=%d body=%s", denied.Code, denied.Body.String())
	}

	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")
	if err := app.store().AddViolationLog(store.ViolationLog{UID: 2, Username: "target", Code: "hidden", CodeType: "regcode_decoy", Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	response := doJSON(app, http.MethodGet, "/api/v2/admin/violations?per_page=20", "", adminCookies)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("admin violation status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}
	var envelope struct {
		Data struct {
			Violations []map[string]any `json:"violations"`
			Total      int              `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Total != 1 || len(envelope.Data.Violations) != 1 || envelope.Data.Violations[0]["code"] != "hidden" {
		t.Fatalf("unexpected violation payload: %#v", envelope.Data)
	}

	cleared := doJSON(app, http.MethodPost, "/api/v2/admin/violations/clear", `{"confirm":"CLEAR_VIOLATIONS"}`, adminCookies)
	if cleared.Code != http.StatusOK || len(app.store().ListViolationLogs()) != 0 {
		t.Fatalf("v2 violation clear status=%d body=%s remaining=%d", cleared.Code, cleared.Body.String(), len(app.store().ListViolationLogs()))
	}
	if !strings.Contains(cleared.Body.String(), "violation logs cleared") {
		t.Fatalf("unexpected clear response: %s", cleared.Body.String())
	}
}
