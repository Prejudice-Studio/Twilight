package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2MigrationResourcesKeepFeatureGateAndPrivateStatus(t *testing.T) {
	app := newTestApp(t)
	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")

	status := doJSONWithHeaders(app, http.MethodGet, "/api/v2/admin/migration/status", "", adminCookies, nil)
	if status.Code != http.StatusForbidden {
		t.Fatalf("disabled migration status=%d body=%s", status.Code, status.Body.String())
	}

	app.cfg().DatabaseMigrationPanelEnabled = true
	enabled := doJSONWithHeaders(app, http.MethodGet, "/api/v2/admin/migration/status", "", adminCookies, nil)
	if enabled.Code != http.StatusOK || enabled.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(enabled.Body.String(), `"enabled":true`) {
		t.Fatalf("v2 migration status=%d cache=%q body=%s", enabled.Code, enabled.Header().Get("Cache-Control"), enabled.Body.String())
	}

	userCookies := registerAndLogin(t, app, "migration-reader", "User123456")
	for _, methodPath := range [][2]string{{http.MethodGet, "/api/v2/admin/migration/status"}, {http.MethodPost, "/api/v2/admin/migration/export"}, {http.MethodPost, "/api/v2/admin/migration/import"}} {
		response := doJSONWithHeaders(app, methodPath[0], methodPath[1], `{}`, userCookies, nil)
		if response.Code != http.StatusForbidden {
			t.Fatalf("non-admin %s %s status=%d body=%s", methodPath[0], methodPath[1], response.Code, response.Body.String())
		}
	}
}
