package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2UserSettingsResourcesRequireUserAndUsePrivateNoStore(t *testing.T) {
	app := newTestApp(t)
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v2/settings"},
		{http.MethodPut, "/api/v2/settings/preferences"},
		{http.MethodPost, "/api/v2/settings/email/send-code"},
		{http.MethodPost, "/api/v2/settings/email/verify"},
		{http.MethodPost, "/api/v2/settings/password/system"},
		{http.MethodPost, "/api/v2/settings/password/emby"},
		{http.MethodPost, "/api/v2/settings/emby/bind"},
		{http.MethodPost, "/api/v2/settings/emby/register"},
		{http.MethodPost, "/api/v2/settings/emby/unbind"},
	}
	for _, route := range routes {
		response := doJSON(app, route.method, route.path, `{}`, nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s expected unauthenticated rejection, got %d body=%s", route.method, route.path, response.Code, response.Body.String())
		}
	}

	cookies := registerAndLogin(t, app, "v2-settings-user", "SettingsV2User123456")
	read := doJSON(app, http.MethodGet, "/api/v2/settings", "", cookies)
	if read.Code != http.StatusOK || read.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("settings read status=%d cache=%q body=%s", read.Code, read.Header().Get("Cache-Control"), read.Body.String())
	}
	if !strings.Contains(read.Body.String(), `"telegram"`) || !strings.Contains(read.Body.String(), `"emby_status"`) {
		t.Fatalf("settings read missing account projections: %s", read.Body.String())
	}

	preference := doJSON(app, http.MethodPut, "/api/v2/settings/preferences", `{"notify_on_login_email":false}`, cookies)
	if preference.Code != http.StatusOK || preference.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("settings preference status=%d cache=%q body=%s", preference.Code, preference.Header().Get("Cache-Control"), preference.Body.String())
	}
	user, ok := app.store().FindUserByUsername("v2-settings-user")
	if !ok || user.NotifyOnLoginEmail {
		t.Fatalf("settings preference did not persist through V2 resource: %+v", user)
	}
}
