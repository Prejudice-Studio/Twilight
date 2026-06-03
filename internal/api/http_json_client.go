package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// sharedHTTPTransport 为所有外部 HTTP 调用（emby / telegram / 系统更新自检）
// 共享连接池，避免之前每次调用都 `&http.Client{}` 导致 TCP / TLS 握手不复用、
// 文件描述符累积、GC 压力上升等问题。
// 注意：超时不在 client 上设置，而是通过 context.WithTimeout 在每次调用时
// 控制，这样可以实现"每端点不同超时"（health 1.5s / userOp 5s / admin 10s）
// 又复用同一个 Transport。
var sharedHTTPTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   16,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

// sharedHTTPClient 是 transport-only 共享 client，不带 client.Timeout，
// 所有超时都通过传入的 ctx 控制（context.DeadlineExceeded 优雅取消，
// 而 client.Timeout 触发的是连接强杀，不利于排查）。
//
// CheckRedirect 显式拒绝跨主机重定向。Go 默认策略只剥离 Authorization /
// Cookie / WWW-Authenticate 三个头，**不会**剥离自定义头；而本 client
// 的所有调用方（Emby `X-Emby-Token`、Telegram bot token in URL、Bangumi
// `Authorization: Bearer`、TMDB `api_key=` 查询串）都会把可信凭据带在
// 跨主机依然保留的位置。一个被入侵 / 错配的上游 302 到攻击者控制的 host
// 就足以把 token 原样泄露。当前所有调用方都是单次 JSON 请求，没有任何
// 路径需要跨主机 follow；同主机（scheme + host:port 完全一致）保留最多
// 5 跳以应对 trailing-slash / index 这类合理重定向。
var sharedHTTPClient = &http.Client{
	Transport:     sharedHTTPTransport,
	CheckRedirect: sameHostRedirectPolicy,
}

// sameHostRedirectPolicy 只允许同 scheme + 同 host:port 的重定向，且最多 5 跳。
// 跨主机直接返回 ErrUseLastResponse —— 调用方拿到的是原始 302 响应（结合
// `resp.StatusCode >= 400` 检查会被当作非预期状态返回错误，不会出现 token
// 已被发到第三方的窗口）。
func sameHostRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return http.ErrUseLastResponse
	}
	if len(via) == 0 {
		return nil
	}
	prev := via[len(via)-1].URL
	next := req.URL
	if prev == nil || next == nil {
		return http.ErrUseLastResponse
	}
	if !strings.EqualFold(prev.Scheme, next.Scheme) || !strings.EqualFold(prev.Host, next.Host) {
		return http.ErrUseLastResponse
	}
	return nil
}

func getJSON(ctx context.Context, endpoint string, headers map[string]string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return doJSONRequest(req, dst)
}

func postJSON(ctx context.Context, endpoint string, headers map[string]string, body any, dst any) error {
	return postJSONWithTimeout(ctx, endpoint, headers, body, dst, 10*time.Second)
}

func postJSONWithTimeout(ctx context.Context, endpoint string, headers map[string]string, body any, dst any, timeout time.Duration) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return doJSONRequestWithTimeout(req, dst, timeout)
}

func doJSONRequest(req *http.Request, dst any) error {
	return doJSONRequestWithTimeout(req, dst, 10*time.Second)
}

// doJSONRequestWithTimeout 把 timeout 包成 context deadline 后用共享 client 发送，
// 既能复用连接池又能保留每端点不同的超时语义。
// 边界：req 已经携带 ctx（NewRequestWithContext），如果调用方 ctx 已带 deadline
// 且早于 timeout，会沿用调用方的 ctx；否则我们 wrap 一层确保有上界。
func doJSONRequestWithTimeout(req *http.Request, dst any, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	parentCtx := req.Context()
	if _, ok := parentCtx.Deadline(); !ok {
		ctx, cancel := context.WithTimeout(parentCtx, timeout)
		defer cancel()
		req = req.WithContext(ctx)
	}
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		detail := strings.TrimSpace(string(data))
		if detail != "" {
			return fmt.Errorf("remote status %d: %s", resp.StatusCode, truncateString(detail, 300))
		}
		return fmt.Errorf("remote status %d", resp.StatusCode)
	}
	// 3xx 仅会出现在 sameHostRedirectPolicy 拒绝的跨主机 / 超跳数场景：
	// Client 拿到原始 3xx 响应，不再继续 follow。把 Location 当作错误抛出，
	// 既能让调用方明确感知"被禁的重定向"，又不会因为 json.Unmarshal HTML/空 body
	// 报出难以定位的错。Token 在这一刻还没被发到第三方。
	if resp.StatusCode >= 300 {
		loc := strings.TrimSpace(resp.Header.Get("Location"))
		if loc == "" {
			return fmt.Errorf("remote status %d: cross-host or excessive redirect refused", resp.StatusCode)
		}
		return fmt.Errorf("remote status %d: cross-host redirect refused (Location=%s)", resp.StatusCode, truncateString(loc, 200))
	}
	if dst == nil {
		return nil
	}
	return json.Unmarshal(data, dst)
}
