package api

// 出站基础 URL 校验：所有从 cfg 读出后送进 net/http 的远端 base URL 必须经过
// 这一层。原本只有 EmbyURL 走 validateEmbyURL，BangumiAPIURL / TelegramAPIURL
// /TMDBAPIURL 是直接 strings.TrimRight 后拼路径，含义如下风险：
//
//  1. 配置面被入侵或 admin 误填后，可信的 X-Emby-Token / Bot-Token /
//     TMDB API Key 被发到攻击者控制的内部地址（云元数据 169.254.169.254、
//     link-local fe80::/10、其他内网服务），等价于跨域 SSRF；
//  2. scheme 为空或非 http/https 时（例如 file://、javascript: 误粘贴），
//     net/http 仍可能尝试拨号，泄露环境信息；
//  3. 含 query / fragment 的 base URL 会让后续 path 拼接结果出现非预期参数，
//     攻击者可借此影响 outbound API 行为。
//
// 该 helper 的设计与 emby_client.go::validateEmbyURL 完全等价（事实上后者
// 现在转调本函数），但 service 参数让错误消息保留排查上下文：
//
//	"Bangumi URL 协议不支持: ..." vs "Emby URL 协议不支持: ..."
//
// 性能：Emby 保留专用 RWMutex 缓存（embyURLCache*）；Telegram 会按当前 API URL
// 与 Bot Token 缓存校验后的请求前缀，配置变化时由缓存键自动失效。Bangumi/TMDB
// 仅在搜索路径调用，仍按请求校验。

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// validateOutboundBaseURL 用于所有出站远端服务（Emby / Bangumi / Telegram /
// TMDB）配置的 base URL 校验。返回 trim 右斜杠的规范化 base URL。
//
// 规则：
//   - scheme 必须为 http 或 https；
//   - host 不能为空；
//   - 若 host 是字面 IP，则交由 refuseUnsafeOutboundIP 否决 link-local /
//     unspecified / 云元数据；
//   - base URL 不允许包含 query / fragment，避免后续 path 拼接污染。
//
// 注意：不强制 HTTPS——Emby/自部署 Bangumi 反代 + 同机 docker-compose 部署
// 用 HTTP 极其常见，强制 HTTPS 会让现网部署直接打死，HTTPS 强制由部署侧
// reverse proxy 承担。loopback (127.0.0.1 / ::1) 显式允许：httptest 与
// docker-compose 同 stack 都依赖该路径，禁掉会让单测和绝大多数自托管部署炸。
func validateOutboundBaseURL(raw, service string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%s URL 解析失败: %w", service, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%s URL 协议不支持: %q（仅允许 http / https）", service, u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("%s URL 缺少 host: %q", service, raw)
	}
	if ip := net.ParseIP(host); ip != nil {
		if err := refuseUnsafeOutboundIP(ip, service); err != nil {
			return "", err
		}
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%s URL 不应包含 query / fragment: %q", service, raw)
	}
	cleaned := strings.TrimRight(u.String(), "/")
	return cleaned, nil
}

// refuseUnsafeOutboundIP 否决典型 SSRF 目标。允许 loopback（见上方注释）；
// 拒绝 link-local（169.254.0.0/16、IPv6 fe80::/10）、unspecified（0.0.0.0/::）
// 与未被 link-local 覆盖的云元数据 magic IP（阿里云 100.100.100.200）。
func refuseUnsafeOutboundIP(ip net.IP, service string) error {
	switch {
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
		return fmt.Errorf("%s URL 指向链路本地地址 (%s)，禁止访问以避免 SSRF", service, ip.String())
	case ip.IsUnspecified():
		return fmt.Errorf("%s URL host 为 0.0.0.0/::，配置无效", service)
	}
	if ip.To4() != nil {
		v4 := ip.To4().String()
		switch v4 {
		case "100.100.100.200":
			return fmt.Errorf("%s URL 指向云元数据地址 (%s)，禁止访问以避免 SSRF", service, v4)
		}
	}
	return nil
}
