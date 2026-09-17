package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2DeveloperResourcesKeepAdminBoundaryAndNoStore(t *testing.T) {
	app := newTestApp(t)
	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")

	docs := doJSONWithHeaders(app, http.MethodGet, "/api/v2/admin/developer/js-docs", "", adminCookies, nil)
	if docs.Code != http.StatusForbidden {
		// Developer mode is disabled by default; the route must still reach the
		// canonical capability gate rather than expose documentation.
		t.Fatalf("disabled developer docs status=%d body=%s", docs.Code, docs.Body.String())
	}

	presets := doJSONWithHeaders(app, http.MethodGet, "/api/v2/admin/developer/js-presets", "", adminCookies, nil)
	if presets.Code != http.StatusOK || presets.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(presets.Body.String(), `"developer_mode_enabled":false`) {
		t.Fatalf("v2 presets status=%d cache=%q body=%s", presets.Code, presets.Header().Get("Cache-Control"), presets.Body.String())
	}

	userCookies := registerAndLogin(t, app, "developer-reader", "User123456")
	for _, methodPath := range [][2]string{{http.MethodGet, "/api/v2/admin/developer/js-docs"}, {http.MethodGet, "/api/v2/admin/developer/js-presets"}, {http.MethodPost, "/api/v2/admin/developer/js-sandbox"}} {
		response := doJSONWithHeaders(app, methodPath[0], methodPath[1], `{}`, userCookies, nil)
		if response.Code != http.StatusForbidden {
			t.Fatalf("non-admin %s %s status=%d body=%s", methodPath[0], methodPath[1], response.Code, response.Body.String())
		}
	}
}
