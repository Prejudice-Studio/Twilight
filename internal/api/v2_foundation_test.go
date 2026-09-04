package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
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

func TestV2InviteSummaryAggregatesConfigAndUserProjection(t *testing.T) {
	app := newTestApp(t)
	cookies := registerAndLogin(t, app, "invite-summary-user", "InviteSummary123456")
	app.cfg().InviteEnabled = true
	app.cfg().InviteDefaultDays = 14
	app.cfg().InviteCodeFormat = "INV-{random}"

	resp := doJSON(app, http.MethodGet, "/api/v2/invite/summary", "", cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("invite summary status=%d body=%s", resp.Code, resp.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(resp.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode invite summary: %v", err)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("invite summary data type=%T", env.Data)
	}
	configData, ok := data["config"].(map[string]any)
	if !ok || configData["enabled"] != true {
		t.Fatalf("invite summary missing config: %v", data["config"])
	}
	inviteData, ok := data["invite"].(map[string]any)
	if !ok {
		t.Fatalf("invite summary missing invite projection: %v", data["invite"])
	}
	if _, ok := inviteData["children"]; !ok {
		t.Fatalf("invite summary missing children: %v", inviteData)
	}
	if strings.Contains(resp.Body.String(), "InviteSummary123456") {
		t.Fatalf("invite summary leaked password-like test value")
	}
}

func TestV2BangumiSummaryDoesNotExposeToken(t *testing.T) {
	app := newTestApp(t)
	cookies := registerAndLogin(t, app, "bangumi-summary-user", "BangumiSummary123456")
	app.cfg().BangumiEnabled = true
	app.cfg().BangumiManageEnabled = false
	user, ok := app.store().FindUserByUsername("bangumi-summary-user")
	if !ok {
		t.Fatal("missing bangumi summary user")
	}
	const token = "bgm-secret-token-for-summary-test"
	if _, err := app.store().UpdateUser(user.UID, func(u *store.User) error {
		u.BGMToken = token
		return nil
	}); err != nil {
		t.Fatalf("set Bangumi token: %v", err)
	}

	resp := doJSON(app, http.MethodGet, "/api/v2/bangumi/summary", "", cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("bangumi summary status=%d body=%s", resp.Code, resp.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(resp.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode bangumi summary: %v", err)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("bangumi summary data type=%T", env.Data)
	}
	if _, ok := data["status"].(map[string]any); !ok {
		t.Fatalf("bangumi summary missing status: %v", data)
	}
	status, ok := data["status"].(map[string]any)
	if !ok || status["token_set"] != true {
		t.Fatalf("bangumi summary did not preserve token presence: %v", data["status"])
	}
	if strings.Contains(resp.Body.String(), token) || strings.Contains(strings.ToLower(resp.Body.String()), "access_token") {
		t.Fatalf("bangumi summary exposed token-shaped data: %s", resp.Body.String())
	}
}

func TestV2BangumiSubjectPreservesDecimalScore(t *testing.T) {
	subject := publicBangumiSubject(map[string]any{
		"id":   float64(42),
		"name": "Test Subject",
		"rating": map[string]any{
			"score": float64(8.7),
			"rank":  float64(12),
		},
	})
	rating, ok := subject["rating"].(map[string]any)
	if !ok {
		t.Fatalf("missing rating: %v", subject)
	}
	if got, ok := rating["score"].(float64); !ok || got != 8.7 {
		t.Fatalf("score was truncated: %#v", rating["score"])
	}
}
