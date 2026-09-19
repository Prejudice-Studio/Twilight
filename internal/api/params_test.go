package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestQueryHelpersFallBackWithoutSilentZero 盯住这类 helper 存在的理由：
// 非法入参必须回退到调用方显式给的 fallback，而不是静默变成 0。散落在各
// handler 里的内联 ParseInt 大多写 `_` 吞错误，"limit=abc" 于是变成 0，
// 而 0 在有的接口里是"不限制"、在有的接口里是"取不到任何东西"。
func TestQueryHelpersFallBackWithoutSilentZero(t *testing.T) {
	cases := []struct {
		url         string
		wantInt     int
		wantInt64   int64
		wantClamped int
	}{
		{"/x?limit=25&since=1700000000", 25, 1700000000, 25},
		// 缺失 → fallback
		{"/x", 7, 9, 7},
		// 非法 → fallback，而不是 0
		{"/x?limit=abc&since=abc", 7, 9, 7},
		{"/x?limit=&since=", 7, 9, 7},
		// 越界 → 夹到上下界（since 未传，回退 9）
		{"/x?limit=99999", 99999, 9, 1000},
		{"/x?limit=-5", -5, 9, 1},
		// 负数时间戳是合法的（1970 年之前），不该被夹掉；limit 未传回退 7
		{"/x?since=-1", 7, -1, 7},
	}

	for _, item := range cases {
		r := httptest.NewRequest(http.MethodGet, item.url, nil)
		if got := queryInt(r, "limit", 7); got != item.wantInt {
			t.Fatalf("queryInt(limit) on %s = %d want %d", item.url, got, item.wantInt)
		}
		if got := queryInt64(r, "since", 9); got != item.wantInt64 {
			t.Fatalf("queryInt64(since) on %s = %d want %d", item.url, got, item.wantInt64)
		}
		if got := queryIntClamped(r, "limit", 7, 1, 1000); got != item.wantClamped {
			t.Fatalf("queryIntClamped(limit) on %s = %d want %d", item.url, got, item.wantClamped)
		}
	}

	if got := queryInt(nil, "limit", 7); got != 7 {
		t.Fatalf("nil request should fall back, got %d", got)
	}
	if got := queryInt64(nil, "since", 9); got != 9 {
		t.Fatalf("nil request should fall back, got %d", got)
	}
}

func TestGuardOutboundDialAddressRefusesMetadataAndLinkLocal(t *testing.T) {
	// 云元数据 / 链路本地：即使 URL 层写的是域名、解析后落到这里也必须挡住。
	for _, address := range []string{"169.254.169.254:80", "[fe80::1]:80", "100.100.100.200:80", "0.0.0.0:80"} {
		if err := guardOutboundDialAddress("tcp4", address); err == nil {
			t.Fatalf("dial guard must refuse %s", address)
		}
	}

	// loopback 与内网地址必须放行：Emby 常与本项目同机 / 同 compose 网络部署，
	// 禁掉会直接打死现网部署和 httptest 单测。
	for _, address := range []string{"127.0.0.1:8096", "[::1]:8096", "192.168.1.10:8096", "172.18.0.5:8096", "93.184.216.34:443"} {
		if err := guardOutboundDialAddress("tcp4", address); err != nil {
			t.Fatalf("dial guard must allow %s, got %v", address, err)
		}
	}

	// 拿不到可校验的 IP 时按拒绝处理——宁可拒绝也不能放过去。
	if err := guardOutboundDialAddress("tcp4", "not-an-ip:80"); err == nil {
		t.Fatalf("unparseable address must be refused")
	}
	// 非 TCP 网络不在本层职责内（连接池只发 HTTP），直接放行。
	if err := guardOutboundDialAddress("udp4", "169.254.169.254:80"); err != nil {
		t.Fatalf("udp should be out of scope, got %v", err)
	}
}
