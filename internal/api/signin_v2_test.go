package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2SigninResourcesKeepActionsBehindUserAuthAndNoStore(t *testing.T) {
	app := newTestApp(t)
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v2/signin/summary"},
		{http.MethodPost, "/api/v2/signin"},
		{http.MethodPost, "/api/v2/signin/renew"},
		{http.MethodPut, "/api/v2/signin/preferences"},
	} {
		response := doJSON(app, route.method, route.path, `{}`, nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s expected unauthenticated rejection, got %d body=%s", route.method, route.path, response.Code, response.Body.String())
		}
	}

	cookies := registerAndLogin(t, app, "v2-signin-user", "SigninV2User123456")
	app.cfg().SigninEnabled = true
	app.cfg().SigninDailyMin = 1
	app.cfg().SigninDailyMax = 1

	read := doJSON(app, http.MethodGet, "/api/v2/signin/summary", "", cookies)
	if read.Code != http.StatusOK || read.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("summary status=%d cache=%q body=%s", read.Code, read.Header().Get("Cache-Control"), read.Body.String())
	}

	signin := doJSON(app, http.MethodPost, "/api/v2/signin", `{}`, cookies)
	if signin.Code != http.StatusOK || signin.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(signin.Body.String(), `"created":true`) {
		t.Fatalf("signin status=%d cache=%q body=%s", signin.Code, signin.Header().Get("Cache-Control"), signin.Body.String())
	}

	preference := doJSON(app, http.MethodPut, "/api/v2/signin/preferences", `{"signin_auto_renewal":false}`, cookies)
	if preference.Code != http.StatusOK || preference.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("preference status=%d cache=%q body=%s", preference.Code, preference.Header().Get("Cache-Control"), preference.Body.String())
	}
}
