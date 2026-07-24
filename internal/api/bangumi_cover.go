package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

func bangumiCoverImageExts() []string {
	return []string{".jpg", ".png", ".gif", ".webp", ".bmp"}
}

// bangumiCoverDownloadConcurrency 限制封面下载并发。用户整库刷新可能带回上百条
// 条目，旧代码 `for range entries { go download }` 会瞬间拉起同等数量 goroutine +
// 出站 HTTP，既冲高本进程内存/CPU，也容易触发 bgm.tv 限流/封 IP。与
// schedulerAutoConcurrency 同档，配合 sharedHTTPTransport 的每 host 空闲连接上限。
const bangumiCoverDownloadConcurrency = 4

// bangumiCoverCachedOnDisk 判断某 subject 的封面是否已落地（任一扩展名命中即可），
// 用于在发起出站请求前短路：命中则完全跳过网络拉取。dir 必须是已经过
// ResolveWithinRoot 校验的 bangumi 目录。
func bangumiCoverCachedOnDisk(dir, sidStr string) bool {
	for _, ext := range bangumiCoverImageExts() {
		if info, err := os.Lstat(filepath.Join(dir, sidStr+ext)); err == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular() {
			return true
		}
	}
	return false
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
	return firstNonEmpty(asString(images["common"]), asString(images["large"]), asString(images["medium"]))
}

func (a *App) downloadBangumiCover(subjectID int64, imageURL string) {
	if !isSafeBangumiImageURL(imageURL) {
		return
	}

	// 出站前先看本地是否已缓存：避免整库刷新时对已有封面重复拉满 5MB 再在
	// 落盘阶段丢弃（旧代码先下载后 Lstat，白白消耗带宽 / CPU / bgm.tv 配额）。
	uploadRoot := firstNonEmpty(a.cfg().UploadDir, "uploads")
	dir, err := ResolveWithinRoot(uploadRoot, "bangumi")
	if err != nil {
		return
	}
	sidStr := strconv.FormatInt(subjectID, 10)
	if bangumiCoverCachedOnDisk(dir, sidStr) {
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

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	if info, lerr := os.Lstat(dir); lerr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return
	}

	filename := sidStr + ext
	target := filepath.Join(dir, filename)

	if _, err := os.Lstat(target); err == nil {
		return
	}

	if err := store.WriteFileAtomicSync(target, data, 0o600); err != nil {
		zap.L().Debug("bangumi cover save failed", zap.Int64("subject_id", subjectID), zap.Error(err))
	}
}

// downloadBangumiCoversForEntries 后台补齐一批条目的封面。整个批次由单个协调
// goroutine 驱动，内部用容量 bangumiCoverDownloadConcurrency 的信号量限流，避免
// 旧实现「每条目一个 goroutine」在整库刷新时瞬时拉起上百并发。调用方立即返回，
// 下载在后台进行。
func (a *App) downloadBangumiCoversForEntries(entries []map[string]any) {
	type coverJob struct {
		sid int64
		url string
	}
	seen := make(map[int64]struct{}, len(entries))
	jobs := make([]coverJob, 0, len(entries))
	for _, item := range entries {
		subject, _ := item["subject"].(map[string]any)
		if subject == nil {
			continue
		}
		sid := int64(numeric(subject["id"]))
		if sid <= 0 {
			continue
		}
		if _, dup := seen[sid]; dup {
			continue
		}
		images, _ := subject["images"].(map[string]any)
		if images == nil {
			continue
		}
		imageURL := firstNonEmpty(asString(images["common"]), asString(images["large"]), asString(images["medium"]))
		if !isSafeBangumiImageURL(imageURL) {
			continue
		}
		seen[sid] = struct{}{}
		jobs = append(jobs, coverJob{sid: sid, url: imageURL})
	}
	if len(jobs) == 0 {
		return
	}
	go func() {
		sem := make(chan struct{}, bangumiCoverDownloadConcurrency)
		var wg sync.WaitGroup
		for _, job := range jobs {
			sem <- struct{}{}
			wg.Add(1)
			go func(sid int64, url string) {
				defer func() {
					<-sem
					wg.Done()
					if rec := recover(); rec != nil {
						zap.L().Warn("bangumi 封面下载协程异常", zap.Int64("subject_id", sid), zap.String("panic", redactSensitiveText(fmt.Sprintf("%v", rec))))
					}
				}()
				a.downloadBangumiCover(sid, url)
			}(job.sid, job.url)
		}
		wg.Wait()
	}()
}
