package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2RuntimeResourcesRequireAdmin(t *testing.T) {
	app := newTestApp(t)
	user := registerAndLogin(t, app, "runtime-user", "User123456")
	for _, path := range []string{"/api/v2/admin/runtime/status", "/api/v2/admin/runtime/logs?limit=100"} {
		response := doJSON(app, http.MethodGet, path, "", user)
		if response.Code != http.StatusForbidden {
			t.Fatalf("non-admin runtime access %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestV2RuntimeLogSnapshotIsBounded(t *testing.T) {
	app := newTestApp(t)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	response := doJSON(app, http.MethodGet, "/api/v2/admin/runtime/logs?limit=999999", "", admin)
	if response.Code != http.StatusOK {
		t.Fatalf("runtime log status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "text/event-stream") {
		t.Fatal("runtime snapshot unexpectedly returned an SSE response")
	}
}
