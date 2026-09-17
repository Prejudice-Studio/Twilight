package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2AdminEmbyResourcesKeepManualReadContracts(t *testing.T) {
	app := newTestApp(t)
	admin := registerAndLogin(t, app, "admin", "Admin123456")

	deviceAudit := doJSON(app, http.MethodGet, "/api/v2/admin/emby/device-audit", "", admin)
	if deviceAudit.Code != http.StatusOK || !strings.Contains(deviceAudit.Body.String(), `"emby_configured":false`) {
		t.Fatalf("v2 device audit status=%d body=%s", deviceAudit.Code, deviceAudit.Body.String())
	}

	activity := doJSON(app, http.MethodGet, "/api/v2/admin/emby/activity-logs?limit=20", "", admin)
	if activity.Code != http.StatusOK || !strings.Contains(activity.Body.String(), `"entries"`) {
		t.Fatalf("v2 activity log status=%d body=%s", activity.Code, activity.Body.String())
	}

	syncActivity := doJSON(app, http.MethodPost, "/api/v2/admin/emby/activity-logs/sync", `{}`, admin)
	if syncActivity.Code != http.StatusBadGateway || !strings.Contains(syncActivity.Body.String(), `"success":false`) {
		t.Fatalf("v2 activity sync status=%d body=%s", syncActivity.Code, syncActivity.Body.String())
	}

	connectivity := doJSON(app, http.MethodPost, "/api/v2/admin/emby/test", `{}`, admin)
	if connectivity.Code != http.StatusOK || !strings.Contains(connectivity.Body.String(), `"tests"`) {
		t.Fatalf("v2 connectivity status=%d body=%s", connectivity.Code, connectivity.Body.String())
	}
}

func TestV2AdminEmbyResourcesRejectNormalUsers(t *testing.T) {
	app := newTestApp(t)
	user := registerAndLogin(t, app, "v2-emby-normal", "User12345678")
	response := doJSON(app, http.MethodGet, "/api/v2/admin/emby/device-audit", "", user)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected admin rejection, got %d body=%s", response.Code, response.Body.String())
	}
}
