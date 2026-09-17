package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2SetupResourcesKeepOneTimeGateAndSessionBoundary(t *testing.T) {
	app := newSetupTestApp(t)

	status := doJSON(app, http.MethodGet, "/api/v2/setup/status", "", nil)
	if status.Code != http.StatusOK || status.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(status.Body.String(), `"available":true`) {
		t.Fatalf("v2 setup status=%d cache=%q body=%s", status.Code, status.Header().Get("Cache-Control"), status.Body.String())
	}

	body := `{"admin":{"username":"v2-owner","password":"Owner123456"}}`
	withoutIntent := doJSON(app, http.MethodPost, "/api/v2/setup/complete", body, nil)
	if withoutIntent.Code != http.StatusBadRequest || app.store().UserCount() != 0 {
		t.Fatalf("v2 setup without intent status=%d users=%d body=%s", withoutIntent.Code, app.store().UserCount(), withoutIntent.Body.String())
	}

	complete := doJSONWithHeaders(app, http.MethodPost, "/api/v2/setup/complete", body, nil, setupIntentHeaders())
	if complete.Code != http.StatusCreated || findCookie(complete.Result().Cookies(), "twilight_session") == nil {
		t.Fatalf("v2 setup complete status=%d cookies=%v body=%s", complete.Code, complete.Result().Cookies(), complete.Body.String())
	}

	second := doJSONWithHeaders(app, http.MethodPost, "/api/v2/setup/complete", body, nil, setupIntentHeaders())
	if second.Code != http.StatusForbidden {
		t.Fatalf("v2 setup second attempt status=%d body=%s", second.Code, second.Body.String())
	}
}
