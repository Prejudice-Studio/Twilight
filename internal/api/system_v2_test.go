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
	app.cfg().AuthBackgroundURL = "/system/auth-background"

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
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("system info exposed %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, `"auth_background_url":"/api/v2/system/auth-background"`) {
		t.Fatalf("missing public authentication background: %s", body)
	}
}

func TestPublicAuthBackgroundURL(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"", ""},
		{" /system/auth-background ", "/api/v2/system/auth-background"},
		{"/api/v1/system/auth-background?v=2", "/api/v2/system/auth-background?v=2"},
		{"/api/v2/system/auth-background", "/api/v2/system/auth-background"},
		{"https://images.example/background%20image.jpg", "https://images.example/background%20image.jpg"},
		{"http://images.example/background.jpg", "http://images.example/background.jpg"},
		{"https://user:secret@images.example/bg.jpg", ""},
		{"//images.example/bg.jpg", ""},
		{"/srv/twilight/uploads/background.png", ""},
		{`C:\uploads\background.png`, ""},
		{"javascript:alert(1)", ""},
		{"https://images.example/\nbg.jpg", ""},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			if got := publicAuthBackgroundURL(tc.raw); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
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
