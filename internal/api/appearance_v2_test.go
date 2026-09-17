package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2UserAppearanceIsPrivateAndUsesOneProjection(t *testing.T) {
	app := newTestApp(t)
	if response := doJSON(app, http.MethodGet, "/api/v2/settings/appearance", "", nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated appearance read status=%d body=%s", response.Code, response.Body.String())
	}

	cookies := registerAndLogin(t, app, "v2-appearance-user", "Appearance123456")
	read := doJSON(app, http.MethodGet, "/api/v2/settings/appearance", "", cookies)
	if read.Code != http.StatusOK || read.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("appearance read status=%d cache=%q body=%s", read.Code, read.Header().Get("Cache-Control"), read.Body.String())
	}
	if !strings.Contains(read.Body.String(), `"avatar"`) || !strings.Contains(read.Body.String(), `"background"`) {
		t.Fatalf("appearance read did not return the projection: %s", read.Body.String())
	}

	update := doJSON(app, http.MethodPut, "/api/v2/settings/appearance/background", `{"lightBg":"linear-gradient(90deg, #fff 0%, #eee 100%)"}`, cookies)
	if update.Code != http.StatusOK || update.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("appearance update status=%d cache=%q body=%s", update.Code, update.Header().Get("Cache-Control"), update.Body.String())
	}

	reset := doJSON(app, http.MethodDelete, "/api/v2/settings/appearance/background", "", cookies)
	if reset.Code != http.StatusOK || reset.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("appearance reset status=%d cache=%q body=%s", reset.Code, reset.Header().Get("Cache-Control"), reset.Body.String())
	}
}
