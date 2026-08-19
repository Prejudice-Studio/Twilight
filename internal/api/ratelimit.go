package api

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/prejudice-studio/twilight/internal/redis"
)

type rateLimiter struct {
	mu          sync.Mutex
	items       map[string]rateBucket
	redis       *redis.Client
	prefix      string
	lastCleanup time.Time

	// redis 失败回退到内存桶时累加，运维通过 /system/stats 观察是否进入降级。
	// 任何非 nil err（连接断开、超时、命令拒绝）都计入；命中后 fallbackCount
	// 持续递增意味着 redis 已不可用，登录限流退化为单进程内存桶。
	fallbackCount atomic.Int64

	// 上次输出降级 Warn 的 unix 纳秒。redis 失联时每个请求都会走回退分支，
	// 若逐条 Warn，高 QPS 端点（global / apikey）会把 twilight_runtime_logs
	// 写爆——日志本身反噬成二次 DoS。这里节流：Warn 最多每 fallbackWarnInterval
	// 出一条，而 fallbackCount 仍逐次自增，保证 /system/stats 观测不失真。
	lastFallbackWarnNanos atomic.Int64
}

const (
	// fallbackWarnInterval 降级告警节流窗口：redis 持续不可用时，最多每隔这么久
	// 才落一条 Warn，避免高频端点把运行日志表打爆。
	fallbackWarnInterval = 30 * time.Second
	// rateLimiterMaxBuckets limits attacker-controlled high-cardinality keys while
	// Redis is unavailable. At capacity, existing buckets keep working and new
	// keys fail closed until an expired bucket can be reclaimed.
	rateLimiterMaxBuckets      = 10_000
	rateLimiterCleanupInterval = 5 * time.Minute
)

type rateBucket struct {
	Count   int
	ResetAt time.Time
}

func newRateLimiter(redisClient *redis.Client) *rateLimiter {
	return &rateLimiter{
		items:       map[string]rateBucket{},
		redis:       redisClient,
		prefix:      "twilight:rate:",
		lastCleanup: time.Now(),
	}
}

func (r *rateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) bool {
	if limit <= 0 {
		return true
	}
	if window < time.Second {
		window = time.Second
	}
	if r.redis != nil {
		// window 已在入口兜底到 1s，避免 EXPIRE key 0 立即删除桶。
		expireSeconds := int(window / time.Second)
		count, err := r.redis.IncrExpire(ctx, r.prefix+key, expireSeconds)
		if err == nil {
			return count <= int64(limit)
		}
		// 计数逐次自增（观测不失真），但 Warn 按 fallbackWarnInterval 节流，
		// 避免 redis 长时间失联时把 twilight_runtime_logs 写爆成二次 DoS。
		r.fallbackCount.Add(1)
		if r.shouldWarnFallback() {
			zap.L().Warn("redis rate limit failed; falling back to memory",
				zap.Error(err), zap.Int64("fallback_total", r.fallbackCount.Load()))
		}
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	if now.Sub(r.lastCleanup) > rateLimiterCleanupInterval {
		r.cleanupExpiredLocked(now)
		r.lastCleanup = now
	}

	bucket, exists := r.items[key]
	if !exists && len(r.items) >= rateLimiterMaxBuckets {
		// Capacity pressure should not wait for the periodic sweep. Reclaim expired
		// keys immediately, then fail closed instead of growing without bound.
		r.cleanupExpiredLocked(now)
		r.lastCleanup = now
		if len(r.items) >= rateLimiterMaxBuckets {
			return false
		}
	}
	if now.After(bucket.ResetAt) {
		bucket = rateBucket{ResetAt: now.Add(window)}
	}
	bucket.Count++
	r.items[key] = bucket
	return bucket.Count <= limit
}

func (r *rateLimiter) cleanupExpiredLocked(now time.Time) {
	for key, bucket := range r.items {
		if !now.Before(bucket.ResetAt) {
			delete(r.items, key)
		}
	}
}

// rateKey 把限流 key 的若干段拼成一个字符串。旧实现用 fmt.Sprint(parts...)，
// 每个请求都走 reflect 装箱 + 变长 []any 分配；限流 key 段全是 string / int64，
// 直接用 Builder 按最大长度预估缓冲，零反射、单次分配，且 key 字节与旧实现
// 完全一致（fmt.Sprint 对 string/int64 就是原样拼接）。
func rateKey(parts ...any) string {
	if len(parts) == 0 {
		return ""
	}
	total := 0
	for _, p := range parts {
		total += rateKeyPartLen(p)
	}
	var b strings.Builder
	b.Grow(total)
	for _, p := range parts {
		writeRateKeyPart(&b, p)
	}
	return b.String()
}

// rateKeyPartLen 预估单个 key 段拼接后的字节长度，仅用于 Builder.Grow 的容量
// 提示，多估或少估都不影响正确性（Grow 只是减少扩容次数）。
func rateKeyPartLen(p any) int {
	switch v := p.(type) {
	case string:
		return len(v)
	case int64:
		return intLen(v)
	case int:
		return intLen(int64(v))
	default:
		return 16
	}
}

func writeRateKeyPart(b *strings.Builder, p any) {
	switch v := p.(type) {
	case string:
		b.WriteString(v)
	case int64:
		writeInt64(b, v)
	case int:
		writeInt64(b, int64(v))
	default:
		// 兜底路径：与旧 fmt.Sprint 行为一致的非常规段（理论上不存在）。
		b.WriteString(fmt.Sprint(p))
	}
}

// writeInt64 把 int64 以十进制写入 Builder，避免经 strconv.AppendInt 返回切片
// 后再次复制到 Builder。
func writeInt64(b *strings.Builder, v int64) {
	var buf [20]byte
	n := 0
	negative := v < 0
	u := uint64(v)
	if negative {
		u = uint64(-v)
	}
	if u == 0 {
		b.WriteByte('0')
		return
	}
	for u > 0 {
		buf[n] = byte('0' + u%10)
		u /= 10
		n++
	}
	if negative {
		b.WriteByte('-')
	}
	for i := n - 1; i >= 0; i-- {
		b.WriteByte(buf[i])
	}
}

// intLen 返回十进制表示的字符数（负号不计入，用于 Grow 预估值）。
func intLen(v int64) int {
	if v < 0 {
		v = -v
	}
	if v == 0 {
		return 1
	}
	n := 0
	for v > 0 {
		v /= 10
		n++
	}
	return n
}

// shouldWarnFallback 判断当前这次降级是否应落一条 Warn。用 CAS 抢占时间戳：
// 并发请求里只有推进时间戳成功的那一路返回 true，其余静默，保证 redis 长时间
// 失联时告警频率被钉在 ~1 条 / fallbackWarnInterval，而非每请求一条。
func (r *rateLimiter) shouldWarnFallback() bool {
	now := time.Now().UnixNano()
	last := r.lastFallbackWarnNanos.Load()
	if now-last < int64(fallbackWarnInterval) {
		return false
	}
	// 只有 CAS 成功的调用方获得本轮告警资格；失败说明已有并发调用抢先，跳过。
	return r.lastFallbackWarnNanos.CompareAndSwap(last, now)
}

// FallbackCount 报告自启动以来 redis 限流失败回退到内存桶的累计次数。
// 仅观察用：值持续增长说明 redis 实例失联或被熔断，多副本部署会出现"每副本
// 各自一份内存桶"的降级，限流上限实际被放大 N 倍。
func (r *rateLimiter) FallbackCount() int64 {
	if r == nil {
		return 0
	}
	return r.fallbackCount.Load()
}
