package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestV2OpenAPIKeepsAnonymousSpecificationPublicOnly(t *testing.T) {
	app := newTestApp(t)
	response := doJSON(app, http.MethodGet, "/api/v2/openapi.json", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("v2 openapi status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "/api/v2/admin/") || strings.Contains(response.Body.String(), "/api/v1/admin/") {
		t.Fatalf("anonymous v2 openapi exposed admin routes: %s", response.Body.String())
	}
}

func TestV2AdminAPIRoutesRequiresAdminAndIncludesAuthMetadata(t *testing.T) {
	app := newTestApp(t)
	user := registerAndLogin(t, app, "docs-user", "DocsUser123456")
	if response := doJSON(app, http.MethodGet, "/api/v2/admin/docs/routes", "", user); response.Code != http.StatusForbidden {
		t.Fatalf("normal user route inventory status=%d body=%s", response.Code, response.Body.String())
	}

	admin := registerAndLogin(t, app, "admin", "DocsAdmin123456")
	response := doJSON(app, http.MethodGet, "/api/v2/admin/docs/routes", "", admin)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store, private" {
		t.Fatalf("admin route inventory status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}
	var envelope struct {
		Data struct {
			APIs []map[string]string `json:"apis"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode route inventory: %v", err)
	}
	if len(envelope.Data.APIs) == 0 {
		t.Fatal("admin route inventory is empty")
	}
	foundV2 := false
	foundAdmin := false
	for _, route := range envelope.Data.APIs {
		if route["path"] == "/api/v2/system/health" && route["auth"] == "Public" {
			foundV2 = true
		}
		if strings.HasPrefix(route["path"], "/api/v2/admin/") && route["auth"] == "Admin" {
			foundAdmin = true
		}
	}
	if !foundV2 || !foundAdmin {
		t.Fatalf("route inventory lacks version/auth metadata: v2=%v admin=%v", foundV2, foundAdmin)
	}
}
