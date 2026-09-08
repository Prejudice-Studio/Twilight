package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestV2BangumiCoverUsesCanonicalSafeAssetHandler(t *testing.T) {
	app := newTestApp(t)
	uploadDir := t.TempDir()
	app.cfg().UploadDir = uploadDir
	coverDir := filepath.Join(uploadDir, "bangumi")
	if err := os.MkdirAll(coverDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(coverDir, "42.jpg"), []byte("cover"), 0o600); err != nil {
		t.Fatal(err)
	}

	cover := doJSON(app, http.MethodGet, "/api/v2/bangumi/covers/42", "", nil)
	if cover.Code != http.StatusOK || cover.Header().Get("Cache-Control") != "public, max-age=86400" || cover.Body.String() != "cover" {
		t.Fatalf("v2 cover status=%d cache=%q body=%q", cover.Code, cover.Header().Get("Cache-Control"), cover.Body.String())
	}

	invalid := doJSON(app, http.MethodGet, "/api/v2/bangumi/covers/not-a-number", "", nil)
	if invalid.Code != http.StatusNotFound {
		t.Fatalf("invalid v2 cover status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}
