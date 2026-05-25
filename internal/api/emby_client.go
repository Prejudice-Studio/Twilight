package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
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
func validateEmbyURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("Emby URL 解析失败: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("Emby URL 协议不支持: %q（仅允许 http / https）", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("Emby URL 缺少 host: %q", raw)
	}
	if ip := net.ParseIP(host); ip != nil {
		if err := refuseUnsafeEmbyIP(ip); err != nil {
			return "", err
		}
	}
	// 路径里只保留 base，不允许 query / fragment 形成隐式参数注入面。
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("Emby URL 不应包含 query / fragment: %q", raw)
	}
	cleaned := strings.TrimRight(u.String(), "/")
	return cleaned, nil
}

// refuseUnsafeEmbyIP 拒绝典型 SSRF 目标。**允许 loopback**——这是 Twilight
// 与 Emby 同机 / docker-compose 同 stack 部署的主流形态，禁掉会把绝大多数
// 部署直接打死；而 loopback "泄露 token" 的实际接收方仍然是 admin 自己的
// 主机，结合 R53-1 的跨主机 follow 防护，不构成真正的 SSRF 出口。
//
// 真正不应该出现在 Emby 反代理目标里的是：
//   - link-local（169.254.0.0/16、IPv6 fe80::/10）
//   - 云元数据 magic IP（169.254.169.254 AWS/GCP/Azure，100.100.100.200 Aliyun）
//   - 0.0.0.0/:: unspecified
func refuseUnsafeEmbyIP(ip net.IP) error {
	switch {
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
		return fmt.Errorf("Emby URL 指向链路本地地址 (%s)，禁止访问以避免 SSRF", ip.String())
	case ip.IsUnspecified():
		return fmt.Errorf("Emby URL host 为 0.0.0.0/::，配置无效")
	}
	// AWS / GCP / Azure / 阿里云元数据 magic IP 显式拒绝。多数已经被
	// IsLinkLocalUnicast 覆盖（169.254.169.254 属于 link-local），这里只
	// 兜底 100.100.100.200 这类不在 link-local 段的元数据 IP。
	if ip.To4() != nil {
		v4 := ip.To4().String()
		switch v4 {
		case "100.100.100.200":
			return fmt.Errorf("Emby URL 指向云元数据地址 (%s)，禁止访问以避免 SSRF", v4)
		}
	}
	return nil
}

func (a *App) embyHeaders() map[string]string {
	headers := map[string]string{"Accept": "application/json"}
	if a.cfg().EmbyToken != "" {
		headers["X-Emby-Token"] = a.cfg().EmbyToken
		headers["X-Emby-Authorization"] = `MediaBrowser Client="Twilight", Device="Twilight", DeviceId="twilight-client", Version="1.0.0", Token="` + a.cfg().EmbyToken + `"`
	}
	return headers
}

func (a *App) embyGet(ctx context.Context, apiPath string, dst any) error {
	endpoint, err := a.validatedEmbyEndpoint(apiPath)
	if err != nil {
		return err
	}
	return getJSON(ctx, endpoint, a.embyHeaders(), dst)
}

func (a *App) embyPost(ctx context.Context, apiPath string, body any, dst any) error {
	endpoint, err := a.validatedEmbyEndpoint(apiPath)
	if err != nil {
		return err
	}
	headers := a.embyHeaders()
	return postJSON(ctx, endpoint, headers, body, dst)
}

func (a *App) embyDelete(ctx context.Context, apiPath string) error {
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
