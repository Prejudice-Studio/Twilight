package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2SystemInfoIsPublicSafeProjection(t *testing.T) {
	app := newTestApp(t)
	app.cfg().EmbyURL = "http://emby.internal:8096"
	app.cfg().EmbyToken = "system-info-emby-secret"
	app.cfg().TelegramBotToken = "system-info-telegram-secret"
	app.cfg().PostgresPassword = "system-info-postgres-secret"

	response := doJSON(app, http.MethodGet, "/api/v2/system/info", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("system info status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"api_version":"v2"`) {
		t.Fatalf("missing v2 system info marker: %s", body)
	}
	for _, forbidden := range []string{
		"system-info-emby-secret",
		"system-info-telegram-secret",
		"system-info-postgres-secret",
		"emby.internal:8096",
		"auth_background_url",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("system info exposed %q: %s", forbidden, body)
		}
	}
}

func TestV2AdminHealthResourcesAreIndependentAndPrivate(t *testing.T) {
	app := newTestApp(t)
	adminCookies := registerAdmin(t, app, "system-admin", "SystemAdmin123456")

	for _, path := range []string{
		"/api/v2/admin/health/api",
		"/api/v2/admin/health/database",
		"/api/v2/admin/health/emby",
		"/api/v2/admin/stats",
	} {
		unauthorized := doJSON(app, http.MethodGet, path, "", nil)
		if unauthorized.Code != http.StatusUnauthorized {
			t.Fatalf("unauthorized %s status=%d body=%s", path, unauthorized.Code, unauthorized.Body.String())
		}

		response := doJSON(app, http.MethodGet, path, "", adminCookies)
		if response.Code != http.StatusOK {
			t.Fatalf("admin %s status=%d body=%s", path, response.Code, response.Body.String())
		}
		if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
			t.Fatalf("admin %s cache-control=%q", path, got)
		}
	}

	emby := doJSON(app, http.MethodGet, "/api/v2/admin/health/emby", "", adminCookies)
	if !strings.Contains(emby.Body.String(), `"status":"not_configured"`) {
		t.Fatalf("unconfigured emby health was not isolated: %s", emby.Body.String())
	}
}
