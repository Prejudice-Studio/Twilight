package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2AuthResourcesKeepSessionBoundaries(t *testing.T) {
	app := newTestApp(t)
	if response := doJSON(app, http.MethodGet, "/api/v2/auth/me", "", nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated current-user status=%d body=%s", response.Code, response.Body.String())
	}

	register := doJSON(app, http.MethodPost, "/api/v2/registration", `{"username":"v2-auth-user","password":"AuthUser123456"}`, nil)
	if register.Code != http.StatusCreated || register.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("v2 registration status=%d cache=%q body=%s", register.Code, register.Header().Get("Cache-Control"), register.Body.String())
	}

	availability := doJSON(app, http.MethodGet, "/api/v2/registration/availability?username=v2-auth-user", "", nil)
	if availability.Code != http.StatusOK || availability.Header().Get("Cache-Control") != "no-store" || !strings.Contains(availability.Body.String(), `"available":false`) {
		t.Fatalf("v2 availability status=%d cache=%q body=%s", availability.Code, availability.Header().Get("Cache-Control"), availability.Body.String())
	}

	login := doJSON(app, http.MethodPost, "/api/v2/auth/login", `{"username":"v2-auth-user","password":"AuthUser123456"}`, nil)
	if login.Code != http.StatusOK || login.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("v2 login status=%d cache=%q body=%s", login.Code, login.Header().Get("Cache-Control"), login.Body.String())
	}
	cookie := findCookie(login.Result().Cookies(), "twilight_session")
	if cookie == nil {
		t.Fatal("v2 login did not issue a session cookie")
	}

	me := doJSON(app, http.MethodGet, "/api/v2/auth/me", "", []*http.Cookie{cookie})
	if me.Code != http.StatusOK || me.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(me.Body.String(), `"username":"v2-auth-user"`) {
		t.Fatalf("v2 current-user status=%d cache=%q body=%s", me.Code, me.Header().Get("Cache-Control"), me.Body.String())
	}

	logout := doJSON(app, http.MethodPost, "/api/v2/auth/logout", "", []*http.Cookie{cookie})
	if logout.Code != http.StatusOK || logout.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("v2 logout status=%d cache=%q body=%s", logout.Code, logout.Header().Get("Cache-Control"), logout.Body.String())
	}
	if after := doJSON(app, http.MethodGet, "/api/v2/auth/me", "", []*http.Cookie{cookie}); after.Code != http.StatusUnauthorized {
		t.Fatalf("v2 logged-out session still authenticated: status=%d body=%s", after.Code, after.Body.String())
	}
}

func TestV2PublicCapabilitiesExposeOnlyAuthFeatureFlags(t *testing.T) {
	app := newTestApp(t)
	response := doJSON(app, http.MethodGet, "/api/v2/system/capabilities", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("capabilities status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, secret := range []string{"password", "token", "database", "emby.invalid"} {
		if strings.Contains(strings.ToLower(body), secret) {
			t.Fatalf("capabilities exposed %q: %s", secret, body)
		}
	}
	for _, flag := range []string{"recovery", "recovery_emby", "recovery_email", "force_bind_telegram"} {
		if !strings.Contains(body, `"`+flag+`"`) {
			t.Fatalf("capabilities missing %q: %s", flag, body)
		}
	}
}
