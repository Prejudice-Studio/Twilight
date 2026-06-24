package redis

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	addr     string
	password string
	db       int
	tls      bool
	timeout  time.Duration
	pool     chan net.Conn
	mu       sync.RWMutex
	closed   bool
}

// maxBulkLen 限制单条 redis bulk-string 回复的最大长度（64MB）。
// 没有这道闸时，对端发送 `$2147483647\r\n` 会立刻 `make([]byte, n+2)`
// 直接 OOM；中间人也能用同样手法搞挂进程。64MB 远超任何 rate-limit / session
// kv 的合理体量，但仍给 Lua 脚本回执之类的留足空间。
const maxBulkLen = 64 * 1024 * 1024

// maxArrayLen 给 multi-bulk 数组元素数加上同等约束，避免 `*2147483647\r\n` 让
// `make([]any, 0, n)` 直接吃光内存。256K 元素够覆盖任何合理的 LRANGE/HGETALL
// 场景。
const maxArrayLen = 256 * 1024

func New(rawURL string, poolSize int) (*Client, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "redis" && u.Scheme != "rediss" {
		return nil, fmt.Errorf("unsupported redis scheme %q", u.Scheme)
	}
	addr := u.Host
	if !strings.Contains(addr, ":") {
		addr += ":6379"
	}
	password, _ := u.User.Password()
	db := 0
	if path := strings.Trim(u.Path, "/"); path != "" {
		db, _ = strconv.Atoi(path)
	}
	if poolSize <= 0 {
		poolSize = 16
	}
	return &Client{addr: addr, password: password, db: db, tls: u.Scheme == "rediss", timeout: 2 * time.Second, pool: make(chan net.Conn, poolSize)}, nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.pool)
	c.mu.Unlock()
	var err error
	for conn := range c.pool {
		if closeErr := conn.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	return err
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil {
		return nil
	}
	_, err := c.Do(ctx, "PING")
	return err
}

func (c *Client) Get(ctx context.Context, key string) (string, bool, error) {
	reply, err := c.Do(ctx, "GET", key)
	if err != nil {
		if errors.Is(err, ErrNil) {
			return "", false, nil
		}
		return "", false, err
	}
	value, ok := reply.(string)
	return value, ok, nil
}

func (c *Client) SetEX(ctx context.Context, key string, seconds int, value string) error {
	_, err := c.Do(ctx, "SETEX", key, strconv.Itoa(seconds), value)
	return err
}

func (c *Client) Del(ctx context.Context, key string) error {
	_, err := c.Do(ctx, "DEL", key)
	return err
}

// incrExpireScript 在 redis 服务端单次原子完成 INCR + EXPIRE。
// 旧实现（分两次 Do）有两类失败模式：
//  1. 第一次 INCR 成功后进程崩溃 / 网络中断 → key 永远没 TTL，落地到 redis 重启
//     才能清；
//  2. 多个 worker 并发首次 INCR 时，仅 count==1 那个发 EXPIRE，若那一路因网络
//     丢包失败但 INCR 已落库，剩下所有调用都 count>1 不再补 TTL。
//
// rate-limit 桶失去 TTL 等价于"永远命中限速"，高 QPS 端点（global / apikey）
// 单 IP 一次攻击即可让该 IP 永久被屏蔽。改用 Lua 脚本让 redis 原子串接两条命令，
// 任一节点失败都不会留下"已 INCR 但无 TTL"的中间态。
const incrExpireScript = `local v = redis.call('INCR', KEYS[1])
if v == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return v`

func (c *Client) IncrExpire(ctx context.Context, key string, seconds int) (int64, error) {
	reply, err := c.Do(ctx, "EVAL", incrExpireScript, "1", key, strconv.Itoa(seconds))
	if err != nil {
		return 0, err
	}
	count, ok := reply.(int64)
	if !ok {
		return 0, fmt.Errorf("unexpected EVAL reply %T", reply)
	}
	return count, nil
}

func (c *Client) Do(ctx context.Context, args ...string) (any, error) {
	if c == nil {
		return nil, errors.New("redis client is nil")
	}
	conn, err := c.getConn(ctx)
	if err != nil {
		return nil, err
	}
	usable := false
	defer func() {
		if usable {
			c.putConn(conn)
		} else {
			_ = conn.Close()
		}
	}()
	deadline := time.Now().Add(c.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	if err := writeCommand(conn, args...); err != nil {
		return nil, err
	}
	reply, err := readReply(bufio.NewReader(conn))
	if err != nil {
		return nil, err
	}
	usable = true
	return reply, nil
}

func (c *Client) getConn(ctx context.Context) (net.Conn, error) {
	select {
	case conn := <-c.pool:
		if conn != nil {
			return conn, nil
		}
	default:
	}
	dialer := &net.Dialer{Timeout: c.timeout}
	var conn net.Conn
	var err error
	if c.tls {
		conn, err = tls.DialWithDialer(dialer, "tcp", c.addr, &tls.Config{MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", c.addr)
	}
	if err != nil {
		return nil, err
	}
	if c.password != "" {
		if err := c.authAndSelect(ctx, conn, "AUTH", c.password); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	if c.db > 0 {
		if err := c.authAndSelect(ctx, conn, "SELECT", strconv.Itoa(c.db)); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

func (c *Client) authAndSelect(ctx context.Context, conn net.Conn, args ...string) error {
	deadline := time.Now().Add(c.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	if err := writeCommand(conn, args...); err != nil {
		return err
	}
	_, err := readReply(bufio.NewReader(conn))
	return err
}

func (c *Client) putConn(conn net.Conn) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		_ = conn.Close()
		return
	}
	defer c.mu.RUnlock()
	defer func() {
		// 与 Close() 的 close(c.pool) 竞争时，向已关闭 channel 发送会 panic；
		// 这里 RLock 已能阻挡 Close 进入临界区，但 recover 兜底以防极端情况。
		if r := recover(); r != nil {
			_ = conn.Close()
		}
	}()
	select {
	case c.pool <- conn:
	default:
		_ = conn.Close()
	}
}

func writeCommand(w io.Writer, args ...string) error {
	if _, err := fmt.Fprintf(w, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, arg := range args {
		if _, err := fmt.Fprintf(w, "$%d\r\n%s\r\n", len(arg), arg); err != nil {
			return err
		}
	}
	return nil
}

func readReply(r *bufio.Reader) (any, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch prefix {
	case '+':
		line, err := readLine(r)
		return line, err
	case '-':
		line, _ := readLine(r)
		return nil, errors.New(line)
	case ':':
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		return strconv.ParseInt(line, 10, 64)
	case '$':
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, ErrNil
		}
		if n > maxBulkLen {
			return nil, fmt.Errorf("redis bulk reply exceeds limit (%d > %d)", n, maxBulkLen)
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*':
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, ErrNil
		}
		if n > maxArrayLen {
			return nil, fmt.Errorf("redis array reply exceeds limit (%d > %d)", n, maxArrayLen)
		}
		arr := make([]any, 0, n)
		for i := 0; i < n; i++ {
			v, err := readReply(r)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		return arr, nil
	default:
		return nil, fmt.Errorf("unknown redis reply prefix %q", prefix)
	}
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}

var ErrNil = errors.New("redis nil")
