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
