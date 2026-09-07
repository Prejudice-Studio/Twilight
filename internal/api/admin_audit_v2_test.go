package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2AdminAuditResourcesAreBoundedAndAdminOnly(t *testing.T) {
	app := newTestApp(t)
	admin := registerAndLogin(t, app, "admin", "Admin123456")

	response := doJSON(app, http.MethodGet, "/api/v2/admin/audit-logs?page=1&per_page=25", "", admin)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(response.Body.String(), `"logs"`) {
		t.Fatalf("v2 audit status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}

	missing := doJSON(app, http.MethodDelete, "/api/v2/admin/audit-logs/999999999", "", admin)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected missing audit log to return 404, got %d body=%s", missing.Code, missing.Body.String())
	}

	user := registerAndLogin(t, app, "v2-audit-normal", "User12345678")
	rejected := doJSON(app, http.MethodGet, "/api/v2/admin/audit-logs", "", user)
	if rejected.Code != http.StatusForbidden {
		t.Fatalf("expected normal user rejection, got %d body=%s", rejected.Code, rejected.Body.String())
	}
}
