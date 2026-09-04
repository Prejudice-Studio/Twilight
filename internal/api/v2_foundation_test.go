package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestV2HealthIsPublicAndDoesNotExposeDependencyDetails(t *testing.T) {
	app := newTestApp(t)
	resp := doJSON(app, http.MethodGet, "/api/v2/system/health", "", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("v2 health status=%d body=%s", resp.Code, resp.Body.String())
	}

	var env envelope
	if err := json.Unmarshal(resp.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode v2 health: %v", err)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("v2 health data type=%T", env.Data)
	}
	if data["api_version"] != "v2" || data["status"] != "ok" {
		t.Fatalf("unexpected v2 health data=%v", data)
	}
	for _, forbidden := range []string{"database", "emby", "telegram", "password", "token"} {
		if strings.Contains(strings.ToLower(resp.Body.String()), forbidden) {
			t.Fatalf("v2 health leaked dependency detail %q: %s", forbidden, resp.Body.String())
		}
	}
}

func TestV2CapabilitiesDoNotExposeSecrets(t *testing.T) {
	app := newTestApp(t)
	app.cfg().EmbyURL = "http://emby.invalid"
	app.cfg().EmbyToken = "emby-secret-for-test"
	app.cfg().TelegramBotToken = "telegram-secret-for-test"
	app.cfg().PostgresPassword = "postgres-secret-for-test"

	resp := doJSON(app, http.MethodGet, "/api/v2/system/capabilities", "", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("v2 capabilities status=%d body=%s", resp.Code, resp.Body.String())
	}
	body := strings.ToLower(resp.Body.String())
	for _, secret := range []string{"emby-secret-for-test", "telegram-secret-for-test", "postgres-secret-for-test", "emby.invalid"} {
		if strings.Contains(body, strings.ToLower(secret)) {
			t.Fatalf("v2 capabilities leaked %q: %s", secret, resp.Body.String())
		}
	}
	if !strings.Contains(body, `"activity_logs":true`) || !strings.Contains(body, `"viewing_stats":false`) {
		t.Fatalf("v2 public feature contract missing: %s", resp.Body.String())
	}
}

func TestV2DashboardSummaryKeepsLocalDataWhenEmbyUnavailable(t *testing.T) {
	app := newTestApp(t)
	cookies := registerAndLogin(t, app, "dashboard-user", "Dashboard123456")
	app.cfg().EmbyURL = "http://emby.invalid"
	app.cfg().EmbyToken = "test-token"

	resp := doJSON(app, http.MethodGet, "/api/v2/dashboard/summary", "", cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("dashboard summary status=%d body=%s", resp.Code, resp.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(resp.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode dashboard summary: %v", err)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("dashboard summary data type=%T", env.Data)
	}
	if _, ok := data["user"].(map[string]any); !ok {
		t.Fatalf("dashboard summary missing user: %v", data)
	}
	viewers, ok := data["viewers"].(map[string]any)
	if !ok || viewers["available"] != false {
		t.Fatalf("expected unavailable viewers state, got %v", data["viewers"])
	}
}

func TestV2SigninSummaryUsesOneBoundedPagePayload(t *testing.T) {
	app := newTestApp(t)
	cookies := registerAndLogin(t, app, "signin-user", "Signin123456")
	app.cfg().SigninRenewalEnabled = true
	app.cfg().SigninRenewalCost = 2
	app.cfg().SigninRenewalDays = 7

	resp := doJSON(app, http.MethodGet, "/api/v2/signin/summary", "", cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("signin summary status=%d body=%s", resp.Code, resp.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(resp.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode signin summary: %v", err)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("signin summary data type=%T", env.Data)
	}
	for _, key := range []string{"summary", "config", "history"} {
		if _, ok := data[key]; !ok {
			t.Fatalf("signin summary missing %q: %v", key, data)
		}
	}
}
