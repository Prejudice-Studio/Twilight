package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
)

func TestV2UserAPIKeyResourcesKeepOwnershipAndNoStore(t *testing.T) {
	app := newTestApp(t)
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v2/settings/apikeys"},
		{http.MethodPost, "/api/v2/settings/apikeys"},
		{http.MethodPut, "/api/v2/settings/apikeys/1"},
		{http.MethodDelete, "/api/v2/settings/apikeys/1"},
	} {
		response := doJSON(app, route.method, route.path, `{}`, nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s expected unauthenticated rejection, got %d body=%s", route.method, route.path, response.Code, response.Body.String())
		}
	}

	cookies := registerAndLogin(t, app, "v2-apikey-user", "APIKeyV2User123456")
	created := doJSON(app, http.MethodPost, "/api/v2/settings/apikeys", `{"name":"v2-key","rate_limit":5}`, cookies)
	if created.Code != http.StatusOK || created.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("create status=%d cache=%q body=%s", created.Code, created.Header().Get("Cache-Control"), created.Body.String())
	}
	var createdEnvelope struct {
		Data struct {
			ID  int64  `json:"id"`
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdEnvelope); err != nil {
		t.Fatalf("decode created key: %v", err)
	}
	if createdEnvelope.Data.ID <= 0 || createdEnvelope.Data.Key == "" {
		t.Fatalf("create response did not contain one-time key data: %s", created.Body.String())
	}

	list := doJSON(app, http.MethodGet, "/api/v2/settings/apikeys", "", cookies)
	if list.Code != http.StatusOK || list.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("list status=%d cache=%q body=%s", list.Code, list.Header().Get("Cache-Control"), list.Body.String())
	}
	if list.Body.String() == createdEnvelope.Data.Key {
		t.Fatalf("list response exposed the one-time plaintext key: %s", list.Body.String())
	}

	path := "/api/v2/settings/apikeys/" + strconv.FormatInt(createdEnvelope.Data.ID, 10)
	updated := doJSON(app, http.MethodPut, path, `{"name":"v2-key-updated","enabled":false,"rate_limit":10}`, cookies)
	if updated.Code != http.StatusOK || updated.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("update status=%d cache=%q body=%s", updated.Code, updated.Header().Get("Cache-Control"), updated.Body.String())
	}
	deleted := doJSON(app, http.MethodDelete, path, "", cookies)
	if deleted.Code != http.StatusOK || deleted.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("delete status=%d cache=%q body=%s", deleted.Code, deleted.Header().Get("Cache-Control"), deleted.Body.String())
	}
}
