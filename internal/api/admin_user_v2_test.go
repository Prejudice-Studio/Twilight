package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestV2AdminUsersUseResourceCollectionAndSharedFilters(t *testing.T) {
	app := newTestApp(t)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	created, err := app.store().CreateUser(store.User{
		Username: "v2-resource-user",
		Email:    "v2-resource@example.com",
		Role:     store.RoleNormal,
		Active:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	response := doJSON(app, http.MethodGet, "/api/v2/admin/users?search=v2-resource&per_page=1", "", admin)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("v2 users status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}
	var envelope struct {
		Data struct {
			Items      []map[string]any `json:"items"`
			Pagination struct {
				Page       int `json:"page"`
				PerPage    int `json:"per_page"`
				Total      int `json:"total"`
				TotalPages int `json:"total_pages"`
			} `json:"pagination"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Pagination.Total != 1 || envelope.Data.Pagination.Page != 1 || envelope.Data.Pagination.PerPage != 1 || len(envelope.Data.Items) != 1 {
		t.Fatalf("unexpected v2 users resource: %+v", envelope.Data)
	}
	if got := envelope.Data.Items[0]["uid"]; got != float64(created.UID) {
		t.Fatalf("unexpected v2 user item uid=%v want=%d", got, created.UID)
	}

	detail := doJSON(app, http.MethodGet, "/api/v2/admin/users/"+strconv.FormatInt(created.UID, 10), "", admin)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"item"`) {
		t.Fatalf("v2 user detail status=%d body=%s", detail.Code, detail.Body.String())
	}

	legacy := doJSON(app, http.MethodGet, "/api/v1/admin/users?search=v2-resource", "", admin)
	if legacy.Code != http.StatusOK || !strings.Contains(legacy.Body.String(), `"users"`) {
		t.Fatalf("legacy user list compatibility status=%d body=%s", legacy.Code, legacy.Body.String())
	}
}

func TestV2AdminUsersRejectNormalUsers(t *testing.T) {
	app := newTestApp(t)
	user := registerAndLogin(t, app, "v2-normal-user", "User12345678")
	response := doJSON(app, http.MethodGet, "/api/v2/admin/users", "", user)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected normal user rejection, got %d body=%s", response.Code, response.Body.String())
	}
}
