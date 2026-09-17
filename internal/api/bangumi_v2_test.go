package api

import (
	"net/http"
	"testing"
)

func TestV2BangumiResourcesRequireUserAndUseNoStore(t *testing.T) {
	app := newTestApp(t)
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v2/bangumi/summary"},
		{http.MethodPost, "/api/v2/bangumi/sync"},
		{http.MethodDelete, "/api/v2/bangumi/sync/history"},
		{http.MethodPut, "/api/v2/bangumi/preferences"},
		{http.MethodGet, "/api/v2/bangumi/collections?type=3&limit=24&offset=0"},
		{http.MethodPatch, "/api/v2/bangumi/collections/1"},
	} {
		response := doJSON(app, route.method, route.path, `{}`, nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s expected unauthenticated rejection, got %d body=%s", route.method, route.path, response.Code, response.Body.String())
		}
	}

	cookies := registerAndLogin(t, app, "v2-bangumi-user", "BangumiV2User123456")
	summary := doJSON(app, http.MethodGet, "/api/v2/bangumi/summary", "", cookies)
	if summary.Code != http.StatusOK || summary.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("summary status=%d cache=%q body=%s", summary.Code, summary.Header().Get("Cache-Control"), summary.Body.String())
	}
}
