package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2InviteActionsUseAuthenticatedNoStoreResources(t *testing.T) {
	app := newTestApp(t)
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v2/invite/summary"},
		{http.MethodPost, "/api/v2/invite/codes"},
		{http.MethodPost, "/api/v2/invite/renew-codes"},
		{http.MethodDelete, "/api/v2/invite/codes/test"},
		{http.MethodPost, "/api/v2/invite/me/detach-expired"},
	} {
		response := doJSON(app, route.method, route.path, `{}`, nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s expected unauthenticated rejection, got %d body=%s", route.method, route.path, response.Code, response.Body.String())
		}
	}

	cookies := registerAndLogin(t, app, "v2-invite-owner", "InviteV2Owner123456")
	app.cfg().InviteEnabled = true
	app.cfg().InviteRequireEmby = false

	read := doJSON(app, http.MethodGet, "/api/v2/invite/summary", "", cookies)
	if read.Code != http.StatusOK || read.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("summary status=%d cache=%q body=%s", read.Code, read.Header().Get("Cache-Control"), read.Body.String())
	}

	created := doJSON(app, http.MethodPost, "/api/v2/invite/codes", `{"days":7}`, cookies)
	if created.Code != http.StatusCreated || created.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(created.Body.String(), `"code"`) {
		t.Fatalf("create status=%d cache=%q body=%s", created.Code, created.Header().Get("Cache-Control"), created.Body.String())
	}
}
