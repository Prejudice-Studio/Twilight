package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2AdminBangumiResourcesKeepScopedReads(t *testing.T) {
	app := newTestApp(t)
	admin := registerAndLogin(t, app, "admin", "Admin123456")

	users := doJSON(app, http.MethodGet, "/api/v2/admin/bangumi/users?per_page=20", "", admin)
	if users.Code != http.StatusOK || users.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(users.Body.String(), `"users"`) {
		t.Fatalf("v2 bangumi users status=%d cache=%q body=%s", users.Code, users.Header().Get("Cache-Control"), users.Body.String())
	}

	records := doJSON(app, http.MethodGet, "/api/v2/admin/bangumi/users/1/records?limit=20", "", admin)
	if records.Code != http.StatusOK || !strings.Contains(records.Body.String(), `"records"`) {
		t.Fatalf("v2 bangumi records status=%d body=%s", records.Code, records.Body.String())
	}

	logs := doJSON(app, http.MethodGet, "/api/v2/admin/bangumi/users/1/logs?limit=20", "", admin)
	if logs.Code != http.StatusOK || !strings.Contains(logs.Body.String(), `"logs"`) {
		t.Fatalf("v2 bangumi logs status=%d body=%s", logs.Code, logs.Body.String())
	}
}

func TestV2AdminBangumiResourcesRejectNormalUsers(t *testing.T) {
	app := newTestApp(t)
	user := registerAndLogin(t, app, "v2-bangumi-normal", "User12345678")
	response := doJSON(app, http.MethodGet, "/api/v2/admin/bangumi/users", "", user)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected admin rejection, got %d body=%s", response.Code, response.Body.String())
	}
}
