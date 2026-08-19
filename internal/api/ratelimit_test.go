package api

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestRateLimiterRejectsNewKeysAtCapacity(t *testing.T) {
	limiter := newRateLimiter(nil)
	resetAt := time.Now().Add(time.Minute)
	for i := 0; i < rateLimiterMaxBuckets; i++ {
		limiter.items[fmt.Sprintf("key-%d", i)] = rateBucket{Count: 1, ResetAt: resetAt}
	}

	if limiter.Allow(context.Background(), "new-key", 10, time.Minute) {
		t.Fatal("new key was allowed after the in-memory bucket cap was reached")
	}
	if len(limiter.items) != rateLimiterMaxBuckets {
		t.Fatalf("bucket count = %d, want %d", len(limiter.items), rateLimiterMaxBuckets)
	}
	if !limiter.Allow(context.Background(), "key-0", 10, time.Minute) {
		t.Fatal("existing key should remain usable at capacity")
	}
}

func TestRateLimiterReclaimsExpiredBucketAtCapacity(t *testing.T) {
	limiter := newRateLimiter(nil)
	future := time.Now().Add(time.Minute)
	for i := 0; i < rateLimiterMaxBuckets; i++ {
		limiter.items[fmt.Sprintf("key-%d", i)] = rateBucket{Count: 1, ResetAt: future}
	}
	limiter.items["key-0"] = rateBucket{Count: 1, ResetAt: time.Now().Add(-time.Second)}

	if !limiter.Allow(context.Background(), "new-key", 10, time.Minute) {
		t.Fatal("new key should use capacity reclaimed from an expired bucket")
	}
	if _, ok := limiter.items["key-0"]; ok {
		t.Fatal("expired bucket was not reclaimed")
	}
	if len(limiter.items) != rateLimiterMaxBuckets {
		t.Fatalf("bucket count = %d, want %d", len(limiter.items), rateLimiterMaxBuckets)
	}
}

func TestRateLimiterAppliesMinimumWindowInMemory(t *testing.T) {
	limiter := newRateLimiter(nil)
	if !limiter.Allow(context.Background(), "short", 1, 0) {
		t.Fatal("first request should be allowed")
	}
	if limiter.Allow(context.Background(), "short", 1, 0) {
		t.Fatal("second request should be limited within the one-second minimum window")
	}
}

// TestRateKeyMatchesLegacyFormat 回归校验：rateKey 与旧 fmt.Sprint(parts...)
// 的字节输出必须逐字节一致，确保限流 key 在跨进程（Redis / 内存桶）语义不变。
func TestRateKeyMatchesLegacyFormat(t *testing.T) {
	cases := [][]any{
		{},
		{"global:", "127.0.0.1"},
		{"login:", "192.168.1.1"},
		{"login:user:", "alice"},
		{"use-code:uid:", int64(42)},
		{"use-code:uid:", 42},
		{"ticket:uid:", int64(123456789)},
		{"emby-probe:", int64(-7)},
		{"apikey:", "hash", "suffix"},
	}
	for _, parts := range cases {
		got := rateKey(parts...)
		want := fmt.Sprint(parts...)
		if got != want {
			t.Fatalf("rateKey(%v) = %q, want %q", parts, got, want)
		}
	}
}
