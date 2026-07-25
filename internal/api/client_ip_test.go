package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// enableTrustedProxies 直接走 runtime.Store 打开 TrustProxyHeaders 并重建
// trustedProxies 匹配器——与生产 reload 路径同源（buildTrustedProxyMatcher），
// 单纯改 a.cfg() 指针字段不会重建匹配器，也会被下次 reload 覆盖。
func enableTrustedProxies(t *testing.T, app *App, cidrs ...string) {
	t.Helper()
	rt := app.runtime.Load()
	next := *rt
	next.cfg.TrustProxyHeaders = true
	next.cfg.TrustedProxyCIDRs = cidrs
	next.trustedProxies = buildTrustedProxyMatcher(cidrs)
	app.runtime.Store(&next)
}

func TestTrustedProxyMatcher(t *testing.T) {
	m := buildTrustedProxyMatcher([]string{"10.0.0.0/24", "192.168.1.5", " ", "bogus/cidr", "::1"})
	cases := []struct {
		ip   string
		want bool
	}{
		{"10.0.0.1", true},    // 落在 /24
		{"10.0.1.1", false},   // 网段外
		{"192.168.1.5", true}, // 单 IP 精确命中
		{"192.168.1.6", false},
		{"::1", true}, // IPv6 单 IP
		{"203.0.113.9", false},
	}
	for _, c := range cases {
		if got := m.contains(net.ParseIP(c.ip)); got != c.want {
			t.Fatalf("contains(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
	// 空匹配器 fail-closed：任何 IP 都不受信。
	var empty trustedProxyMatcher
	if empty.contains(net.ParseIP("10.0.0.1")) {
		t.Fatal("empty matcher should trust nothing (fail-closed)")
	}
	if m.contains(nil) {
		t.Fatal("nil ip must never be trusted")
	}
}

func TestComputeClientIPProxyTrust(t *testing.T) {
	app := newTestApp(t)
	newReq := func(remoteAddr string, headers map[string]string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remoteAddr
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return req
	}

	// 默认不信任代理头：即便带 XFF 也只认 RemoteAddr。
	if got := app.computeClientIP(newReq("10.0.0.1:1234", map[string]string{"X-Forwarded-For": "203.0.113.7"})); got != "10.0.0.1" {
		t.Fatalf("untrusted proxy path = %q, want 10.0.0.1", got)
	}

	enableTrustedProxies(t, app, "10.0.0.0/24")

	// 受信对端 + CF-Connecting-IP 优先。
	if got := app.computeClientIP(newReq("10.0.0.1:1234", map[string]string{"CF-Connecting-IP": "203.0.113.7"})); got != "203.0.113.7" {
		t.Fatalf("CF-Connecting-IP = %q, want 203.0.113.7", got)
	}
	// XFF 右向左剥离：spoofed 在最左，1.1.1.1 是第一个不受信跳。
	if got := app.computeClientIP(newReq("10.0.0.1:1234", map[string]string{"X-Forwarded-For": "spoofed, 1.1.1.1, 10.0.0.5"})); got != "1.1.1.1" {
		t.Fatalf("XFF strip = %q, want 1.1.1.1", got)
	}
	// 对端不在受信网段：忽略所有代理头，回落 RemoteAddr。
	if got := app.computeClientIP(newReq("203.0.113.9:1234", map[string]string{"X-Forwarded-For": "1.1.1.1"})); got != "203.0.113.9" {
		t.Fatalf("untrusted peer = %q, want 203.0.113.9", got)
	}
}

func TestClientIPContextCacheHit(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	// 缓存被显式塞入一个与 RemoteAddr 无关的值：clientIP 必须直接命中缓存，
	// 而不会二次解析 RemoteAddr。这正是 ServeHTTP 一次解析、全链路复用的语义。
	cached := "cached-sentinel"
	req = req.WithContext(context.WithValue(req.Context(), clientIPKey, cached))
	if got := app.clientIP(req); got != cached {
		t.Fatalf("clientIP cache hit = %q, want %q", got, cached)
	}
}

func TestRedactPrefilterPreservesBehavior(t *testing.T) {
	// 命中路径：含触发词的样本必须被脱敏（快路径不得放行）。
	sensitive := []struct {
		in   string
		leak string
	}{
		{"Authorization: Bearer abcdefghijklmnopqrstuvwxyz", "abcdefghijklmnopqrstuvwxyz"},
		{"emby_token=secret-emby-token-XYZ123", "secret-emby-token-XYZ123"},
		{"session_id=sess-abcdef-1234567890", "sess-abcdef-1234567890"},
		{"postgres dsn=host=db password=p@ssw0rd-supersecret", "p@ssw0rd-supersecret"},
	}
	for _, c := range sensitive {
		out := redactSensitiveText(c.in)
		if out == c.in {
			t.Fatalf("prefilter wrongly short-circuited sensitive input: %q", c.in)
		}
		if strings.Contains(out, c.leak) {
			t.Fatalf("secret leaked past redaction: in=%q out=%q", c.in, out)
		}
	}
	// 快路径：不含任何触发词的普通文本必须原样返回（且不改写）。
	for _, plain := range []string{
		"GET /api/v1/users completed in 12ms",
		"uid=1024 username=alice status=200",
		"scheduler job daily_stats finished, 3 rows updated",
		"",
	} {
		if out := redactSensitiveText(plain); out != plain {
			t.Fatalf("prefilter altered non-sensitive text: in=%q out=%q", plain, out)
		}
	}
	// mightContainSecret 判定：触发词大小写不敏感。
	if !mightContainSecret("X-EMBY-TOKEN: xyz") {
		t.Fatal("mightContainSecret should be case-insensitive on triggers")
	}
	if mightContainSecret("plain audit line with no markers") {
		t.Fatal("mightContainSecret false positive on benign text")
	}
}
