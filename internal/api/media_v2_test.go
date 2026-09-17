package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestV2UserMediaResourcesUseBoundedShapes(t *testing.T) {
	app := newTestApp(t)
	cookies := registerAndLogin(t, app, "media-v2-user", "User12345678")
	user, ok := app.store().FindUserByUsername("media-v2-user")
	if !ok {
		t.Fatal("media user was not created")
	}
	request, err := app.store().CreateMediaRequest(store.MediaRequest{
		UID: user.UID, Username: user.Username, Title: "V2 title", Source: "tmdb", MediaID: 321, MediaType: "movie",
	})
	if err != nil {
		t.Fatal(err)
	}

	search := doJSON(app, http.MethodGet, "/api/v2/media/search?q=", "", cookies)
	if search.Code != http.StatusOK || search.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("v2 empty search status=%d cache=%q body=%s", search.Code, search.Header().Get("Cache-Control"), search.Body.String())
	}
	var searchEnvelope struct {
		Data struct {
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(search.Body.Bytes(), &searchEnvelope); err != nil {
		t.Fatal(err)
	}
	if searchEnvelope.Data.Items == nil || searchEnvelope.Data.Total != 0 {
		t.Fatalf("unexpected v2 search shape: %#v", searchEnvelope.Data)
	}

	requests := doJSON(app, http.MethodGet, "/api/v2/media/requests", "", cookies)
	if requests.Code != http.StatusOK || requests.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("v2 request list status=%d cache=%q body=%s", requests.Code, requests.Header().Get("Cache-Control"), requests.Body.String())
	}
	var requestsEnvelope struct {
		Data struct {
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(requests.Body.Bytes(), &requestsEnvelope); err != nil {
		t.Fatal(err)
	}
	if requestsEnvelope.Data.Total != 1 || len(requestsEnvelope.Data.Items) != 1 || requestsEnvelope.Data.Items[0]["require_key"] != request.RequireKey {
		t.Fatalf("unexpected v2 request list: %#v", requestsEnvelope.Data)
	}

	invalidDetail := doJSON(app, http.MethodGet, "/api/v2/media/detail?source=tmdb&media_id=0", "", cookies)
	if invalidDetail.Code != http.StatusBadRequest || !strings.Contains(invalidDetail.Body.String(), `"error_code":"MEDIA_REQUEST_PAYLOAD_EMPTY"`) {
		t.Fatalf("invalid v2 detail status=%d body=%s", invalidDetail.Code, invalidDetail.Body.String())
	}
}

func TestV2UserMediaRequestsAreIsolatedAndFeatureGated(t *testing.T) {
	app := newTestApp(t)
	ownerCookies := registerAndLogin(t, app, "media-v2-owner", "Owner123456")
	otherCookies := registerAndLogin(t, app, "media-v2-other", "Other123456")
	owner, _ := app.store().FindUserByUsername("media-v2-owner")
	if _, err := app.store().CreateMediaRequest(store.MediaRequest{
		UID: owner.UID, Username: owner.Username, Title: "Private request", Source: "bangumi", MediaID: 654, MediaType: "动画",
	}); err != nil {
		t.Fatal(err)
	}

	other := doJSON(app, http.MethodGet, "/api/v2/media/requests", "", otherCookies)
	if other.Code != http.StatusOK || !strings.Contains(other.Body.String(), `"total":0`) {
		t.Fatalf("other user saw private request: status=%d body=%s", other.Code, other.Body.String())
	}

	app.cfg().MediaRequestEnabled = false
	disabled := doJSON(app, http.MethodGet, "/api/v2/media/requests", "", ownerCookies)
	if disabled.Code != http.StatusForbidden || !strings.Contains(disabled.Body.String(), `"error_code":"MEDIA_REQUEST_DISABLED"`) {
		t.Fatalf("disabled v2 media requests status=%d body=%s", disabled.Code, disabled.Body.String())
	}
}
