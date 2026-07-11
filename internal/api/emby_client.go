package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/prejudice-studio/twilight/internal/security"

	"go.uber.org/zap"
)

// embyURLValidationCache 缓存最近一次校验过的 EmbyURL 与其结果。配置热重载
// 之后下一次 emby 调用会重算，普通用户请求路径不会重复 DNS / parse。
var (
	embyURLCacheMu     sync.RWMutex
	embyURLCacheRaw    string
	embyURLCacheParsed string
	embyURLCacheErr    error
)

// validatedEmbyEndpoint 校验 cfg.EmbyURL 后返回拼接好的目标 URL。任何
// 不可信 scheme（非 http/https）、空 host、解析为 loopback / link-local /
// 私有 / 元数据 IP 的目标会立即报错，避免 admin 误配 / 被入侵的配置面
// 把可信的 X-Emby-Token 发到内部敏感端点（结合 R53-1 的跨域跟随防护构成
// 一道额外的出站 SSRF 拦截）。
//
// 设计取舍：
//   - 不强制要求 HTTPS，因为常见部署是 Twilight 与 Emby 同机/同 VPC + HTTP。
//     HTTPS 强制留给 R53-3。
//   - 拒绝 link-local / loopback / 169.254.169.254 元数据这类典型 SSRF 目标。
//     用 net.ParseIP 即时判断；hostname 形式不做反向 DNS（成本太高且会让
//     断网环境炸毁所有 emby 调用），由部署侧负责解析正确性。
//   - apiPath 拼接前 trim 右斜杠，与原 strings.TrimRight 行为一致，保持
//     调用方零迁移成本。
func (a *App) validatedEmbyEndpoint(apiPath string) (string, error) {
	raw := strings.TrimSpace(a.cfg().EmbyURL)
	if raw == "" {
		return "", fmt.Errorf("Emby URL 未配置")
	}

	embyURLCacheMu.RLock()
	cachedRaw := embyURLCacheRaw
	cachedParsed := embyURLCacheParsed
	cachedErr := embyURLCacheErr
	embyURLCacheMu.RUnlock()
	if cachedRaw != raw {
		parsed, err := validateEmbyURL(raw)
		embyURLCacheMu.Lock()
		embyURLCacheRaw = raw
		embyURLCacheParsed = parsed
		embyURLCacheErr = err
		embyURLCacheMu.Unlock()
		cachedParsed = parsed
		cachedErr = err
	}
	if cachedErr != nil {
		return "", cachedErr
	}
	return cachedParsed + apiPath, nil
}

// validateEmbyURL 拆出来便于单测；返回 trim 右斜杠的 base URL。
//
// 现在转调通用 validateOutboundBaseURL，与 Bangumi/Telegram/TMDB 共享同一套
// scheme / host / link-local / 元数据 IP / query-fragment 否决规则——避免
// "Emby 校验三道、其他三家裸奔" 的异步 SSRF 表面。
func validateEmbyURL(raw string) (string, error) {
	return validateOutboundBaseURL(raw, "Emby")
}

func (a *App) embyHeaders() map[string]string {
	headers := map[string]string{"Accept": "application/json"}
	if a.cfg().EmbyToken != "" {
		headers["X-Emby-Token"] = a.cfg().EmbyToken
		headers["X-Emby-Authorization"] = `MediaBrowser Client="Twilight", Device="Twilight", DeviceId="twilight-client", Version="1.0.0", Token="` + a.cfg().EmbyToken + `"`
	}
	return headers
}

// embyConfigured 返回 Emby URL 和 Token 是否都已配置。
// 仅 URL 配置而 Token 为空时视为未鉴权配置，不应发起请求。
func (a *App) embyConfigured() bool {
	return strings.TrimSpace(a.cfg().EmbyURL) != "" && strings.TrimSpace(a.cfg().EmbyToken) != ""
}

func (a *App) embyGet(ctx context.Context, apiPath string, dst any) error {
	if strings.TrimSpace(a.cfg().EmbyToken) == "" {
		return fmt.Errorf("Emby API Token 未配置，拒绝发送未鉴权请求")
	}
	endpoint, err := a.validatedEmbyEndpoint(apiPath)
	if err != nil {
		return err
	}
	return getJSON(ctx, endpoint, a.embyHeaders(), dst)
}

func (a *App) embyPost(ctx context.Context, apiPath string, body any, dst any) error {
	if strings.TrimSpace(a.cfg().EmbyToken) == "" {
		return fmt.Errorf("Emby API Token 未配置，拒绝发送未鉴权请求")
	}
	endpoint, err := a.validatedEmbyEndpoint(apiPath)
	if err != nil {
		return err
	}
	headers := a.embyHeaders()
	return postJSON(ctx, endpoint, headers, body, dst)
}

func (a *App) embyDelete(ctx context.Context, apiPath string) error {
	if strings.TrimSpace(a.cfg().EmbyToken) == "" {
		return fmt.Errorf("Emby API Token 未配置，拒绝发送未鉴权请求")
	}
	endpoint, err := a.validatedEmbyEndpoint(apiPath)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	for key, value := range a.embyHeaders() {
		req.Header.Set(key, value)
	}
	return doJSONRequest(req, nil)
}

func (a *App) embyAuthenticateByName(ctx context.Context, username, password string) (map[string]any, bool, error) {
	endpoint, err := a.validatedEmbyEndpoint("/Users/AuthenticateByName")
	if err != nil {
		return nil, false, err
	}
	// DeviceId 必须是不可预测的随机值：
	// 旧实现 sha256("twilight-bind-" + lower(username)) 完全可被第三方推算，
	// 等价于把 bind 行为暴露成可重放的稳定指纹。
	// crypto/rand 失败时退回到一个明确标记的占位 ID，不再静默继续。
	deviceID, err := security.RandomHex(16)
	if err != nil {
		zap.L().Warn("emby bind device id rand failed", zap.Error(err))
		return nil, false, fmt.Errorf("生成 Emby 绑定设备 ID 失败: %w", err)
	}
	authHeader := fmt.Sprintf(`MediaBrowser Client="Twilight", Device="Twilight Bind", DeviceId="%s", Version="1.0.0"`, deviceID)
	headers := map[string]string{"Accept": "application/json", "X-Emby-Authorization": authHeader}
	var payload map[string]any
	if err := postJSON(ctx, endpoint, headers, map[string]any{"Username": username, "Pw": password}, &payload); err != nil {
		if strings.Contains(err.Error(), "remote status 401") || strings.Contains(err.Error(), "remote status 403") {
			return nil, false, nil
		}
		return nil, false, err
	}
	if user, ok := payload["User"].(map[string]any); ok {
		return user, true, nil
	}
	if id := firstNonEmpty(asString(payload["Id"]), asString(payload["ID"]), asString(payload["id"])); id != "" {
		return payload, true, nil
	}
	return nil, false, nil
}

func (a *App) handleEmbyStats(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.cfg().EmbyStatsEnabled {
		ok(w, "Emby 统计功能未启用", map[string]any{"enabled": false})
		return
	}
	if !a.embyConfigured() {
		ok(w, "Emby 未配置", map[string]any{"enabled": true, "configured": false})
		return
	}
	var counts map[string]any
	if err := a.embyGet(r.Context(), "/Items/Counts", &counts); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "获取 Emby 统计失败: "+err.Error())
		return
	}
	ok(w, "OK", map[string]any{
		"enabled":       true,
		"configured":    true,
		"movie_count":   int64(numeric(counts["MovieCount"])),
		"series_count":  int64(numeric(counts["SeriesCount"])),
		"episode_count": int64(numeric(counts["EpisodeCount"])),
	})
}

func (a *App) handleEmbyViewerCount(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.embyConfigured() {
		ok(w, "OK", map[string]any{"viewers": 0})
		return
	}
	var sessResp struct {
		Items []map[string]any `json:"Items"`
	}
	if err := a.embyGet(r.Context(), "/Sessions", &sessResp); err != nil {
		ok(w, "OK", map[string]any{"viewers": 0})
		return
	}
	ok(w, "OK", map[string]any{"viewers": len(sessResp.Items)})
}
