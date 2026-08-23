package api

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/prejudice-studio/twilight/internal/store"
)

// testDSN 保存 TWILIGHT_TEST_DSN。整套部署已收敛为单一 PostgreSQL 后端，
// api 层测试同样打真库：设了才跑，没设整包直接跳过（见 TestMain）。
var testDSN string

// TestMain only validates PostgreSQL when a DSN is configured. Tests that need
// the database skip in resetTestDatabase; pure routing/response/security tests
// must still run locally so protocol regressions cannot hide behind a green
// package-level early exit.
func TestMain(m *testing.M) {
	testDSN = os.Getenv("TWILIGHT_TEST_DSN")
	if testDSN == "" {
		fmt.Fprintln(os.Stderr, "TWILIGHT_TEST_DSN not set; database-backed api tests will be skipped")
		os.Exit(m.Run())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := store.CheckPostgres(ctx, testDSN); err != nil {
		fmt.Fprintf(os.Stderr, "TWILIGHT_TEST_DSN unreachable: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// resetTestDatabase 把测试库清空到裸状态：直接 DROP 运行时表（IF EXISTS +
// CASCADE，缺表也不报错），随后的 store.OpenPostgres 会幂等重建 schema 并
// 播种空 state，等价于给每个用例一个全新的数据库。
func resetTestDatabase(t *testing.T) {
	t.Helper()
	if testDSN == "" {
		t.Skip("TWILIGHT_TEST_DSN not set")
	}
	db, err := sql.Open("pgx", testDSN)
	if err != nil {
		t.Fatalf("open reset connection: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS twilight_state, twilight_audit_logs, twilight_runtime_logs, twilight_sessions, twilight_telegram_roster, twilight_telegram_runtime, twilight_playback_records CASCADE`); err != nil {
		t.Fatalf("reset test database: %v", err)
	}
}

// newTestStore 重置测试库并打开一个干净的 PostgreSQL Store，供 newTestApp 与
// 少数直接建 store 的用例使用。
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	resetTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	st, err := store.OpenPostgres(ctx, testDSN)
	if err != nil {
		t.Fatalf("open postgres test store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// reopenTestStore 复用同一测试库再开一个 Store，但**不**重置——用于断言
// 「服务重启 / 另一进程改库后数据仍在持久层」这类语义。
func reopenTestStore(t *testing.T) *store.Store {
	t.Helper()
	if testDSN == "" {
		t.Skip("TWILIGHT_TEST_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	st, err := store.OpenPostgres(ctx, testDSN)
	if err != nil {
		t.Fatalf("reopen postgres test store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
