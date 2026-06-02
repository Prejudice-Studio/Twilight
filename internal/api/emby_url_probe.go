package api

// Emby 线路测速代理端点。
//
// 由后端代发 HEAD 请求并测量 RTT，避免前端遇到的两类硬伤：
//   1. 跨域：Emby 服务通常不返回 Access-Control-Allow-Origin，浏览器即便
//      用 mode:"no-cors" 也会被 CORP/COEP / 私网混合内容拦下；
//   2. 混合内容：面板跑在 HTTPS、Emby 列表里有 http://内网 IP 这种条目，
//      浏览器直接 block，没办法用任何前端技巧绕过。
//
// 把测速放到后端就两类问题一次解决。URL 必须命中已配置白名单条目
// （EmbyURL / EmbyPublicURL / EmbyURLList / EmbyWhitelistURL[*]），不接受
// 任意 URL，避免变成 SSRF 跳板。

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

const (
	embyProbeTimeout     = 5 * time.Second
	embyProbeRateLimit   = 20
	embyProbeRateWindow  = time.Minute
	embyProbeURLMaxBytes = 512
)

// allowedEmbyProbeURL 返回 (whitelistOnly, ok)。
//   - ok=false：URL 不在任何配置列表里，拒绝；
//   - whitelistOnly=true：URL 仅在 whitelist 列表里出现，需要 admin/whitelist；
//   - whitelistOnly=false：在通用列表里出现，所有已登录用户都能测。
func (a *App) allowedEmbyProbeURL(raw string) (whitelistOnly bool, ok bool) {
	normalized := normalizeProbeURL(raw)
	if normalized == "" {
		return false, false
	}
	cfg := a.cfg()
	for _, candidate := range []string{cfg.EmbyURL, cfg.EmbyPublicURL} {
		if normalized == normalizeProbeURL(candidate) {
			return false, true
		}
	}
	for _, line := range cfg.EmbyURLList {
		if normalized == normalizeProbeURL(line.URL) {
			return false, true
		}
	}
	if normalized == normalizeProbeURL(cfg.EmbyWhitelistURL) {
		return true, true
	}
	for _, line := range cfg.EmbyWhitelistURLList {
		if normalized == normalizeProbeURL(line.URL) {
			return true, true
		}
	}
	return false, false
}

func normalizeProbeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	return strings.TrimRight(trimmed, "/")
}

func (a *App) handleEmbyURLProbe(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if !a.allowRate(r.Context(), rateKey("emby-probe:", p.User.UID), embyProbeRateLimit, embyProbeRateWindow) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "测速请求过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	raw := stringValue(payload, "url")
	if len(raw) > embyProbeURLMaxBytes {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "url 长度超过限制")
		return
	}
	whitelistOnly, allowed := a.allowedEmbyProbeURL(raw)
	if !allowed {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "url 不在已配置的 Emby 线路列表中")
		return
	}
	if whitelistOnly && p.User.Role != store.RoleAdmin && p.User.Role != store.RoleWhitelist {
		failWithCode(w, http.StatusForbidden, ErrForbidden, "该线路仅管理员和白名单用户可测速")
		return
	}
	// 即便已经在配置白名单内，也走一遍 validateOutboundBaseURL：把 link-local、
	// 0.0.0.0、云元数据地址（100.100.100.200 等）挡掉。admin 误把内部 IP 写进
	// emby_url_list 时，避免后端被借道做 SSRF / 元数据泄露。
	target, err := validateOutboundBaseURL(raw, "Emby Probe")
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, err.Error())
		return
	}
	// 用 /web/favicon.ico 而不是根路径：根路径在 Emby 上是 302→/web/index.html
	// 的较重路径，favicon.ico 是静态资源，更贴近"网络可达 + RTT"的语义。
	probeURL := target + "/web/favicon.ico?tw_ping=" + time.Now().Format("150405.000")

	ctx, cancel := context.WithTimeout(r.Context(), embyProbeTimeout)
	defer cancel()
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodHead, probeURL, nil)
	if reqErr != nil {
		ok(w, "OK", map[string]any{"status": "error", "latency_ms": 0})
		return
	}
	req.Header.Set("User-Agent", "Twilight-Probe/1.0")
	start := time.Now()
	resp, doErr := sharedHTTPClient.Do(req)
	latency := time.Since(start).Milliseconds()
	if doErr != nil {
		probeStatus := "error"
		if ctx.Err() == context.DeadlineExceeded {
			probeStatus = "timeout"
		}
		ok(w, "OK", map[string]any{"status": probeStatus, "latency_ms": latency})
		return
	}
	_ = resp.Body.Close()
	if latency <= 0 {
		latency = 1
	}
	ok(w, "OK", map[string]any{"status": "ok", "latency_ms": latency, "http_status": resp.StatusCode})
}
