package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV2UserAnnouncementResourcesRequireUserAndDoNotCache(t *testing.T) {
	app := newTestApp(t)

	unauthenticated := doJSON(app, http.MethodGet, "/api/v2/announcements", "", nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated announcement read to fail, got %d body=%s", unauthenticated.Code, unauthenticated.Body.String())
	}

	cookies := registerAndLogin(t, app, "v2-announcement-user", "Announcement123456")
	read := doJSON(app, http.MethodGet, "/api/v2/announcements", "", cookies)
	if read.Code != http.StatusOK || read.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("v2 announcement read status=%d cache=%q body=%s", read.Code, read.Header().Get("Cache-Control"), read.Body.String())
	}
	for _, field := range []string{"\"announcements\"", "\"unseen_force_read\"", "\"unseen_force_read_ids\""} {
		if !strings.Contains(read.Body.String(), field) {
			t.Fatalf("v2 announcement read missing %s: %s", field, read.Body.String())
		}
	}

	ack := doJSON(app, http.MethodPost, "/api/v2/announcements/ack", `{"ids":[]}`, cookies)
	if ack.Code != http.StatusOK || ack.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(ack.Body.String(), `"acknowledged":0`) {
		t.Fatalf("v2 announcement acknowledgement status=%d cache=%q body=%s", ack.Code, ack.Header().Get("Cache-Control"), ack.Body.String())
	}
}
