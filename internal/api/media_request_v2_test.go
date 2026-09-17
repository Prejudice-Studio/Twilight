package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestV2AdminMediaRequestsUseResourceShapeAndSharedGrouping(t *testing.T) {
	app := newTestApp(t)
	admin := registerAdmin(t, app, "v2-media-admin", "Admin123456")
	first, err := app.store().CreateMediaRequest(store.MediaRequest{
		UID: 1, Username: "v2-media-admin", Title: "Shared title", Source: "tmdb", MediaID: 101, MediaType: "movie",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.store().CreateMediaRequest(store.MediaRequest{
		UID: 1, Username: "v2-media-admin", Title: " shared   title ", Source: "bangumi", MediaID: 202, MediaType: "tv",
	})
	if err != nil {
		t.Fatal(err)
	}

	response := doJSON(app, http.MethodGet, "/api/v2/admin/media-requests?status=all&source=all&per_page=1", "", admin)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("v2 list status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}
	var envelope struct {
		Data struct {
			Items []struct {
				GroupCount      int `json:"group_count"`
				GroupedRequests []struct {
					RequireKey string `json:"require_key"`
				} `json:"grouped_requests"`
			} `json:"items"`
			Pagination struct {
				Total      int `json:"total"`
				TotalPages int `json:"total_pages"`
			} `json:"pagination"`
			RequestTotal int `json:"request_total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Pagination.Total != 1 || envelope.Data.Pagination.TotalPages != 1 || envelope.Data.RequestTotal != 2 || len(envelope.Data.Items) != 1 {
		t.Fatalf("unexpected v2 grouped response: %#v", envelope.Data)
	}
	if envelope.Data.Items[0].GroupCount != 2 || len(envelope.Data.Items[0].GroupedRequests) != 2 {
		t.Fatalf("unexpected v2 group members: %#v", envelope.Data.Items[0])
	}

	path := "/api/v2/admin/media-requests/by-key/" + first.RequireKey
	updated := doJSONWithHeaders(app, http.MethodPut, path, `{"status":"accepted","note":"handled"}`, admin, map[string]string{"If-Match": `"1"`})
	if updated.Code != http.StatusOK || updated.Header().Get("ETag") != `"2"` {
		t.Fatalf("v2 update status=%d etag=%q body=%s", updated.Code, updated.Header().Get("ETag"), updated.Body.String())
	}
	stale := doJSONWithHeaders(app, http.MethodPut, path, `{"status":"completed"}`, admin, map[string]string{"If-Match": `"1"`})
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), `"MEDIA_REQUEST_CONFLICT"`) {
		t.Fatalf("v2 stale update status=%d body=%s", stale.Code, stale.Body.String())
	}

	body := `{"status":"accepted","note":"both","items":[{"require_key":"` + second.RequireKey + `","revision":1}]}`
	batch := doJSON(app, http.MethodPut, "/api/v2/admin/media-requests/batch", body, admin)
	if batch.Code != http.StatusOK {
		t.Fatalf("v2 batch status=%d body=%s", batch.Code, batch.Body.String())
	}
	if updatedRequest, ok := app.store().FindMediaRequestByKey(second.RequireKey); !ok || updatedRequest.Status != store.MediaRequestStatusAccepted || updatedRequest.Revision != 2 {
		t.Fatalf("v2 batch did not update member: %#v", updatedRequest)
	}
}

func TestV2AdminMediaRequestsRejectNormalUsersAndInvalidFilters(t *testing.T) {
	app := newTestApp(t)
	user := registerAndLogin(t, app, "v2-media-user", "User12345678")
	response := doJSON(app, http.MethodGet, "/api/v2/admin/media-requests", "", user)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected admin rejection, got %d body=%s", response.Code, response.Body.String())
	}
	admin := registerAdmin(t, app, "v2-media-filter-admin", "Admin123456")
	invalid := doJSON(app, http.MethodGet, "/api/v2/admin/media-requests?source=invalid", "", admin)
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), `"MEDIA_REQUEST_SOURCE_INVALID"`) {
		t.Fatalf("invalid source status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}
