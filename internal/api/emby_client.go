package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/prejudice-studio/twilight/internal/security"

	"go.uber.org/zap"
)

func (a *App) embyHeaders() map[string]string {
	headers := map[string]string{"Accept": "application/json"}
	if a.cfg().EmbyToken != "" {
		headers["X-Emby-Token"] = a.cfg().EmbyToken
		headers["X-Emby-Authorization"] = `MediaBrowser Client="Twilight", Device="Twilight", DeviceId="twilight-client", Version="1.0.0", Token="` + a.cfg().EmbyToken + `"`
	}
	return headers
}

func (a *App) embyGet(ctx context.Context, apiPath string, dst any) error {
	if a.cfg().EmbyURL == "" {
		return fmt.Errorf("Emby URL 未配置")
	}
	endpoint := strings.TrimRight(a.cfg().EmbyURL, "/") + apiPath
	return getJSON(ctx, endpoint, a.embyHeaders(), dst)
}

func (a *App) embyPost(ctx context.Context, apiPath string, body any, dst any) error {
	if a.cfg().EmbyURL == "" {
		return fmt.Errorf("Emby URL 未配置")
	}
	endpoint := strings.TrimRight(a.cfg().EmbyURL, "/") + apiPath
	headers := a.embyHeaders()
	return postJSON(ctx, endpoint, headers, body, dst)
}

func (a *App) embyDelete(ctx context.Context, apiPath string) error {
	if a.cfg().EmbyURL == "" {
		return fmt.Errorf("Emby URL not configured")
	}
	endpoint := strings.TrimRight(a.cfg().EmbyURL, "/") + apiPath
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
	if a.cfg().EmbyURL == "" {
		return nil, false, fmt.Errorf("Emby URL not configured")
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
	endpoint := strings.TrimRight(a.cfg().EmbyURL, "/") + "/Users/AuthenticateByName"
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
