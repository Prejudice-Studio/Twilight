package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// testDSN 保存 TWILIGHT_TEST_DSN。store 已收敛为单一 PostgreSQL 后端，测试
// 也统一打真库：设了 DSN 才跑，没设整包直接跳过（见 TestMain）。
var testDSN string

// TestMain 是 store 包所有测试的总闸门。没有 TWILIGHT_TEST_DSN 时不连库、
// 直接以 0 退出（等价于整包 skip），保证无库环境下 `go test` 依然是绿的；
// 设了就先跑一次连通性校验，连不上直接 fail（而不是让每个用例各报一次）。
func TestMain(m *testing.M) {
	testDSN = os.Getenv("TWILIGHT_TEST_DSN")
	if testDSN == "" {
		fmt.Fprintln(os.Stderr, "TWILIGHT_TEST_DSN not set; skipping store package tests")
		os.Exit(0)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := CheckPostgres(ctx, testDSN); err != nil {
		fmt.Fprintf(os.Stderr, "TWILIGHT_TEST_DSN unreachable: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// Open 是给测试用的构造 shim：生产环境已经没有 JSON 后端 / store.Open，这里
// 忽略传入的文件路径，统一连到 TWILIGHT_TEST_DSN 指向的 PostgreSQL，并在每次
// 打开时把库重置为空，让每个用例都从干净状态起步（用例之间互不串数据）。
// 保留 path 形参只是为了让历史调用点 Open(filepath.Join(t.TempDir(), ...)) 原样编译。
func Open(_ string) (*Store, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	st, err := OpenPostgres(ctx, testDSN)
	if err != nil {
		return nil, err
	}
	if err := st.resetForTest(ctx); err != nil {
		_ = st.Close()
		return nil, err
	}
	return st, nil
}

// reopenTestStore 复用同一个测试库再开一个 Store，但**不**重置数据——用于
// 断言「跨进程 / 跨重开数据仍在持久层」这类语义（对应旧 JSON 后端的
// Close+Open 复读）。
func reopenTestStore(t *testing.T) *Store {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	st, err := OpenPostgres(ctx, testDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// resetForTest 把测试库清空并重新播种一份空 state。先清 runtime log / audit log /
// session / playback 边表（RESTART IDENTITY 让自增 ID 回到 1，满足日志 ID 断言），
// 再删掉 state 行并以 force 语义重写空 state（绕过版本守卫，version 归零）。
func (s *Store) resetForTest(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `TRUNCATE twilight_runtime_logs, twilight_audit_logs, twilight_sessions, twilight_telegram_roster, twilight_telegram_runtime, twilight_playback_records RESTART IDENTITY`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM twilight_state`); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = emptyState()
	s.stateVersion = 0
	s.stateRaw = nil
	s.clearTelegramRosterCache()
	s.rebuildUserIndexes()
	if err := s.saveLockedForce(); err != nil {
		return err
	}
	return s.ensureTelegramRuntime(ctx)
}
