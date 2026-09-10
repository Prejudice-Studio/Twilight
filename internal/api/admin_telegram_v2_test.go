package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2AdminTelegramResourcesKeepAdminBoundaryAndNoStore(t *testing.T) {
	app := newTestApp(t)
	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")

	catalog := doJSONWithHeaders(app, http.MethodGet, "/api/v2/admin/telegram/commands/catalog", "", adminCookies, nil)
	if catalog.Code != http.StatusOK || catalog.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(catalog.Body.String(), `"commands"`) {
		t.Fatalf("v2 catalog status=%d cache=%q body=%s", catalog.Code, catalog.Header().Get("Cache-Control"), catalog.Body.String())
	}

	roster := doJSONWithHeaders(app, http.MethodGet, "/api/v2/admin/telegram/roster/stats", "", adminCookies, nil)
	if roster.Code != http.StatusOK || roster.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("v2 roster status=%d cache=%q body=%s", roster.Code, roster.Header().Get("Cache-Control"), roster.Body.String())
	}

	test := doJSONWithHeaders(app, http.MethodPost, "/api/v2/admin/telegram/test", `{}`, adminCookies, nil)
	if test.Code != http.StatusOK || test.Header().Get("Cache-Control") != "private, no-store" || strings.Contains(test.Body.String(), "TelegramBotToken") {
		t.Fatalf("v2 bot test status=%d cache=%q body=%s", test.Code, test.Header().Get("Cache-Control"), test.Body.String())
	}

	userCookies := registerAndLogin(t, app, "telegram-reader", "User123456")
	for _, path := range []string{"/api/v2/admin/telegram/commands/catalog", "/api/v2/admin/telegram/roster/stats"} {
		response := doJSONWithHeaders(app, http.MethodGet, path, "", userCookies, nil)
		if response.Code != http.StatusForbidden {
			t.Fatalf("non-admin %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}
