package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2AdminAnnouncementResourcesRequireAdminAndDoNotCache(t *testing.T) {
	app := newTestApp(t)
	admin := registerAndLogin(t, app, "admin", "Admin123456")

	response := doJSON(app, http.MethodGet, "/api/v2/admin/announcements?page=1&per_page=20", "", admin)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(response.Body.String(), `"announcements"`) {
		t.Fatalf("v2 announcement status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}

	user := registerAndLogin(t, app, "v2-announcement-normal", "User12345678")
	rejected := doJSON(app, http.MethodGet, "/api/v2/admin/announcements", "", user)
	if rejected.Code != http.StatusForbidden {
		t.Fatalf("expected normal user rejection, got %d body=%s", rejected.Code, rejected.Body.String())
	}
}
