package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2SchedulerResourcesRequireAdmin(t *testing.T) {
	app := newTestApp(t)
	user := registerAndLogin(t, app, "scheduler-user", "User123456")
	for _, path := range []string{"/api/v2/admin/scheduler/jobs", "/api/v2/admin/scheduler/jobs/daily_stats/history?limit=20"} {
		response := doJSON(app, http.MethodGet, path, "", user)
		if response.Code != http.StatusForbidden {
			t.Fatalf("non-admin scheduler access %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestV2SchedulerHistoryIsBounded(t *testing.T) {
	app := newTestApp(t)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	response := doJSON(app, http.MethodGet, "/api/v2/admin/scheduler/jobs/daily_stats/history?limit=2000", "", admin)
	if response.Code != http.StatusOK {
		t.Fatalf("scheduler history status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "text/event-stream") {
		t.Fatal("scheduler history unexpectedly returned a stream")
	}
}
