package api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

func bangumiCoverImageExts() []string {
	return []string{".jpg", ".png", ".gif", ".webp", ".bmp"}
}

func isBangumiImageHost(host string) bool {
	host = strings.ToLower(host)
	return strings.HasSuffix(host, ".bgm.tv") || strings.HasSuffix(host, ".bangumi.tv")
}

func isSafeBangumiImageURL(raw string) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	if !isBangumiImageHost(u.Hostname()) {
		return false
	}
	return true
}

func (a *App) handleBangumiCover(w http.ResponseWriter, r *http.Request, params Params) {
	subjectID := params["subject_id"]
	if !isPositiveNumericID(subjectID) {
		failWithCode(w, http.StatusNotFound, ErrAssetNotFound, "resource not found")
		return
	}

	uploadRoot := firstNonEmpty(a.cfg().UploadDir, "uploads")
	dir, err := ResolveWithinRoot(uploadRoot, "bangumi")
	if err != nil {
		failWithCode(w, http.StatusNotFound, ErrAssetNotFound, "resource not found")
		return
	}

	for _, ext := range bangumiCoverImageExts() {
		filePath := filepath.Join(dir, subjectID+ext)
		info, statErr := os.Lstat(filePath)
		if statErr == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular() {
			w.Header().Set("Cache-Control", "public, max-age=86400")
			http.ServeFile(w, r, filePath)
			return
		}
	}

	redirectURL := a.bangumiCoverFallbackURL(subjectID)
	if redirectURL != "" && isSafeBangumiImageURL(redirectURL) {
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	failWithCode(w, http.StatusNotFound, ErrAssetNotFound, "resource not found")
}

func (a *App) bangumiCoverFallbackURL(subjectID string) string {
	sid, err := strconv.ParseInt(subjectID, 10, 64)
	if err != nil {
		return ""
	}
	entry, ok := a.store().RawBangumiSubjectCache(sid)
	if !ok || entry.Subject == nil {
		return ""
	}
	images, _ := entry.Subject["images"].(map[string]any)
	if images == nil {
		return ""
	}
	return firstNonEmpty(asString(images["large"]), asString(images["common"]), asString(images["medium"]))
}

func (a *App) downloadBangumiCover(subjectID int64, imageURL string) {
	if !isSafeBangumiImageURL(imageURL) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "Twilight/1.0")

	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		zap.L().Debug("bangumi cover download failed", zap.String("url", imageURL), zap.Error(err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil || len(data) == 0 {
		return
	}

	contentType := strings.ToLower(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if contentType == "" {
		contentType = strings.ToLower(strings.Split(http.DetectContentType(data), ";")[0])
	}
	ext, ok := uploadImageExtension(contentType)
	if !ok {
		return
	}

	uploadRoot := firstNonEmpty(a.cfg().UploadDir, "uploads")
	dir, err := ResolveWithinRoot(uploadRoot, "bangumi")
	if err != nil {
		return
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	if info, lerr := os.Lstat(dir); lerr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return
	}

	sidStr := strconv.FormatInt(subjectID, 10)
	filename := sidStr + ext
	target := filepath.Join(dir, filename)

	if _, err := os.Lstat(target); err == nil {
		return
	}

	if err := store.WriteFileAtomicSync(target, data, 0o600); err != nil {
		zap.L().Debug("bangumi cover save failed", zap.Int64("subject_id", subjectID), zap.Error(err))
	}
}

func (a *App) downloadBangumiCoversForEntries(entries []map[string]any) {
	for _, item := range entries {
		subject, _ := item["subject"].(map[string]any)
		if subject == nil {
			continue
		}
		sid := int64(numeric(subject["id"]))
		if sid <= 0 {
			continue
		}
		images, _ := subject["images"].(map[string]any)
		if images == nil {
			continue
		}
		imageURL := firstNonEmpty(asString(images["large"]), asString(images["common"]), asString(images["medium"]))
		if !isSafeBangumiImageURL(imageURL) {
			continue
		}
		go a.downloadBangumiCover(sid, imageURL)
	}
}
