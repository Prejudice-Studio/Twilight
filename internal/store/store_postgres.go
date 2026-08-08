package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
)

func OpenPostgres(ctx context.Context, dsn string) (*Store, error) {
	db, _, err := openPreparedPostgres(ctx, dsn)
	if err != nil {
		return nil, err
	}

	st := &Store{db: db, state: emptyState()}
	var raw []byte
	var version int64
	err = db.QueryRowContext(ctx, `SELECT state, version FROM twilight_state WHERE id = 1`).Scan(&raw, &version)
	if errors.Is(err, sql.ErrNoRows) {
		// 冷启动首次落库：用 force 变体播种，绕过版本守卫。多个进程同时冷启动时
		// 各自 seed 的都是同一份 emptyState，force 递增 version 也无害（内容一致），
		// 避免其中一方因守卫 0 行冲突而启动失败。
		if err := st.saveLockedForce(); err != nil {
			_ = db.Close()
			return nil, err
		}
		if err := st.ensureTelegramRuntime(ctx); err != nil {
			_ = db.Close()
			return nil, err
		}
		if err := st.migrateLegacyTelegramRoster(ctx); err != nil {
			_ = db.Close()
			return nil, err
		}
		if err := st.clearLegacyTelegramBotOffset(); err != nil {
			_ = db.Close()
			return nil, err
		}
		st.startAPIKeyUsageFlusher()
		return st, nil
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &st.state); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	st.stateVersion = version
	st.stateRaw = raw
	st.state.ensure()
	st.rebuildUserIndexes()
	if err := st.migrateLegacyAuditLogs(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := st.ensureTelegramRuntime(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := st.migrateLegacyTelegramRoster(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := st.clearLegacyTelegramBotOffset(); err != nil {
		_ = db.Close()
		return nil, err
	}
	st.startAPIKeyUsageFlusher()
	return st, nil
}

func CreatePostgresDatabase(ctx context.Context, dsn string) error {
	cfg, err := pgconn.ParseConfig(strings.TrimSpace(dsn))
	if err != nil {
		return err
	}
	target := strings.TrimSpace(cfg.Database)
	if target == "" {
		return fmt.Errorf("target database name is empty")
	}
	if strings.EqualFold(target, "postgres") || strings.EqualFold(target, "template1") {
		return fmt.Errorf("refusing to auto-create maintenance database %q", target)
	}
	maintenanceDSNs := maintenancePostgresDSNs(dsn)
	var lastErr error
	for _, maintenanceDSN := range maintenanceDSNs {
		maintenance := postgresTargetInfo(maintenanceDSN)
		zap.L().Info("attempting PostgreSQL database creation through maintenance database", zap.String("target_database", target), zap.String("maintenance_database", maintenance.Database), zap.String("user", maintenance.User), zap.String("host", maintenance.Host))
		db, err := sql.Open("pgx", maintenanceDSN)
		if err != nil {
			lastErr = err
			continue
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(0)
		_, err = db.ExecContext(ctx, `CREATE DATABASE `+quotePostgresIdentifier(target))
		closeErr := db.Close()
		if err == nil && closeErr == nil {
			return nil
		}
		if err == nil {
			err = closeErr
		}
		if isDuplicateDatabaseError(err) {
			return nil
		}
		targetInfo := maintenance
		targetInfo.Database = target
		lastErr = describePostgresConnectionError(targetInfo, err)
		zap.L().Warn("PostgreSQL automatic database creation attempt failed", zap.String("target_database", target), zap.String("maintenance_database", maintenance.Database), zap.String("user", maintenance.User), zap.String("host", maintenance.Host), zap.Error(lastErr))
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no maintenance database connection strings could be built")
	}
	return lastErr
}

func maintenancePostgresDSNs(dsn string) []string {
	databases := []string{"postgres", "template1"}
	out := make([]string, 0, len(databases))
	for _, database := range databases {
		if rewritten, ok := rewritePostgresDatabaseInDSN(dsn, database); ok {
			out = append(out, rewritten)
		}
	}
	return out
}

func rewritePostgresDatabaseInDSN(dsn, database string) (string, bool) {
	dsn = strings.TrimSpace(dsn)
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			return "", false
		}
		parsed.Path = "/" + database
		return parsed.String(), true
	}
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil || cfg.Host == "" || cfg.User == "" {
		return "", false
	}
	u := url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port))),
		Path:   "/" + database,
	}
	if cfg.Password == "" {
		u.User = url.User(cfg.User)
	} else {
		u.User = url.UserPassword(cfg.User, cfg.Password)
	}
	q := u.Query()
	if cfg.TLSConfig == nil {
		q.Set("sslmode", "disable")
	}
	for key, value := range cfg.RuntimeParams {
		if strings.TrimSpace(value) != "" {
			q.Set(key, value)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), true
}

func isUndefinedDatabaseError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "3D000" {
		return true
	}
	return strings.Contains(err.Error(), "SQLSTATE 3D000")
}

func isDuplicateDatabaseError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P04" {
		return true
	}
	return strings.Contains(err.Error(), "SQLSTATE 42P04")
}

type postgresInfo struct {
	Host     string
	User     string
	Database string
}

func postgresTargetInfo(dsn string) postgresInfo {
	cfg, err := pgconn.ParseConfig(strings.TrimSpace(dsn))
	if err != nil {
		return postgresInfo{}
	}
	return postgresInfo{Host: cfg.Host, User: cfg.User, Database: cfg.Database}
}

func describePostgresConnectionError(info postgresInfo, err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "28P01":
			return fmt.Errorf("PostgreSQL authentication failed for user %q on host %q: password is incorrect or pg_hba.conf rejected the login (SQLSTATE 28P01): %w", info.User, info.Host, err)
		case "28000":
			return fmt.Errorf("PostgreSQL login rejected for user %q on host %q (SQLSTATE 28000): %w", info.User, info.Host, err)
		case "42501":
			return fmt.Errorf("PostgreSQL user %q does not have permission to create or modify database %q; grant CREATEDB or create the database manually (SQLSTATE 42501): %w", info.User, info.Database, err)
		case "3D000":
			return fmt.Errorf("PostgreSQL database %q does not exist (SQLSTATE 3D000): %w", info.Database, err)
		case "42P04":
			return fmt.Errorf("PostgreSQL database %q already exists (SQLSTATE 42P04): %w", info.Database, err)
		}
	}
	text := err.Error()
	switch {
	case strings.Contains(text, "SQLSTATE 28P01"):
		return fmt.Errorf("PostgreSQL authentication failed for user %q on host %q: password is incorrect or pg_hba.conf rejected the login (SQLSTATE 28P01): %w", info.User, info.Host, err)
	case strings.Contains(text, "SQLSTATE 42501"):
		return fmt.Errorf("PostgreSQL user %q does not have permission to create or modify database %q; grant CREATEDB or create the database manually (SQLSTATE 42501): %w", info.User, info.Database, err)
	case strings.Contains(text, "SQLSTATE 3D000"):
		return fmt.Errorf("PostgreSQL database %q does not exist (SQLSTATE 3D000): %w", info.Database, err)
	default:
		return fmt.Errorf("PostgreSQL connection failed for user %q database %q host %q: %w", info.User, info.Database, info.Host, err)
	}
}

func quotePostgresIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func CheckPostgres(ctx context.Context, dsn string) error {
	db, _, err := openPreparedPostgres(ctx, dsn)
	if err != nil {
		return err
	}
	return db.Close()
}

func CheckPostgresTarget(ctx context.Context, dsn string) (PostgresTargetStatus, error) {
	db, status, err := openPreparedPostgres(ctx, dsn)
	if err != nil {
		return status, err
	}
	if closeErr := db.Close(); closeErr != nil {
		return status, closeErr
	}
	return status, nil
}

func openPreparedPostgres(ctx context.Context, dsn string) (*sql.DB, PostgresTargetStatus, error) {
	dsn = strings.TrimSpace(dsn)
	target := postgresTargetInfo(dsn)
	status := PostgresTargetStatus{
		Host:     target.Host,
		User:     target.User,
		Database: target.Database,
	}
	if dsn == "" {
		return nil, status, fmt.Errorf("postgres dsn is empty")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, status, err
	}
	configurePostgresDB(db, 8, 4)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		if !isUndefinedDatabaseError(err) {
			return nil, status, describePostgresConnectionError(target, err)
		}
		zap.L().Warn("PostgreSQL database does not exist; attempting automatic creation", zap.String("database", target.Database), zap.String("user", target.User), zap.String("host", target.Host))
		if createErr := CreatePostgresDatabase(ctx, dsn); createErr != nil {
			return nil, status, fmt.Errorf("PostgreSQL database %q does not exist and automatic creation failed: %w", target.Database, describePostgresConnectionError(target, createErr))
		}
		status.DatabaseCreated = true
		zap.L().Info("PostgreSQL database created", zap.String("database", target.Database), zap.String("user", target.User), zap.String("host", target.Host))
		db, err = sql.Open("pgx", dsn)
		if err != nil {
			return nil, status, err
		}
		configurePostgresDB(db, 8, 4)
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, status, describePostgresConnectionError(target, err)
		}
	}
	status.Connected = true
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS twilight_state (
	id integer PRIMARY KEY,
	state jsonb NOT NULL,
	version bigint NOT NULL DEFAULT 0,
	updated_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	// 存量部署迁移：老表没有 version 列。加列并默认 0，既有那一行 state 从版本 0 起步，
	// 与冷启动播种（saveLockedForce 从持久层 version 递增）语义一致。幂等，可反复执行。
	if _, err := db.ExecContext(ctx, `
ALTER TABLE twilight_state ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 0`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS twilight_runtime_logs (
	id bigserial PRIMARY KEY,
	time bigint NOT NULL,
	level text NOT NULL,
	message text NOT NULL,
	attrs jsonb,
	created_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS twilight_runtime_logs_time_idx ON twilight_runtime_logs (time DESC)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS twilight_runtime_logs_id_desc_idx ON twilight_runtime_logs (id DESC)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS twilight_audit_logs (
	id bigserial PRIMARY KEY,
	uid bigint NOT NULL DEFAULT 0,
	username text NOT NULL DEFAULT '',
	action text NOT NULL,
	category text NOT NULL,
	source text NOT NULL DEFAULT '',
	method text NOT NULL DEFAULT '',
	target_uid bigint NOT NULL DEFAULT 0,
	detail jsonb,
	ip text NOT NULL DEFAULT '',
	created_at bigint NOT NULL
)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS twilight_audit_logs_created_id_idx ON twilight_audit_logs (created_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS twilight_audit_logs_category_idx ON twilight_audit_logs (LOWER(category), created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS twilight_audit_logs_action_idx ON twilight_audit_logs (LOWER(action), created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS twilight_audit_logs_uid_idx ON twilight_audit_logs (uid, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS twilight_audit_logs_target_uid_idx ON twilight_audit_logs (target_uid, created_at DESC)`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			_ = db.Close()
			return nil, status, describePostgresConnectionError(target, err)
		}
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS twilight_sessions (
	token text PRIMARY KEY,
	uid bigint NOT NULL,
	expires_at bigint NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS twilight_sessions_uid_idx ON twilight_sessions (uid)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS twilight_sessions_expires_at_idx ON twilight_sessions (expires_at)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS twilight_telegram_roster (
	chat_id text NOT NULL,
	telegram_id bigint NOT NULL,
	is_bot boolean NOT NULL DEFAULT false,
	last_status text NOT NULL DEFAULT '',
	first_seen bigint NOT NULL,
	last_seen bigint NOT NULL,
	PRIMARY KEY (chat_id, telegram_id)
)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS twilight_telegram_runtime (
	id smallint PRIMARY KEY CHECK (id = 1),
	update_offset bigint NOT NULL DEFAULT 0,
	updated_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS twilight_playback_records (
	id bigserial PRIMARY KEY,
	uid bigint NOT NULL,
	item_id text NOT NULL,
	title text NOT NULL DEFAULT '',
	series_name text NOT NULL DEFAULT '',
	media_type text NOT NULL DEFAULT '',
	index_number int NOT NULL DEFAULT 0,
	duration bigint NOT NULL DEFAULT 0,
	played_at bigint NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS twilight_playback_records_uid_idx ON twilight_playback_records (uid)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS twilight_playback_records_played_at_idx ON twilight_playback_records (played_at DESC)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE UNIQUE INDEX IF NOT EXISTS twilight_playback_records_uid_item_played_idx ON twilight_playback_records (uid, item_id, played_at)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS twilight_playback_records_item_id_idx ON twilight_playback_records (item_id)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	status.SchemaReady = true
	return db, status, nil
}

func configurePostgresDB(db *sql.DB, maxOpen, maxIdle int) {
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(30 * time.Minute)
	// 空闲连接 5 分钟后回收：低峰期不必长期占着 maxIdle 条连接，也避免
	// 中间件/PG 端提前掐断留下的半死连接被复用（配合 30 分钟硬上限）。
	db.SetConnMaxIdleTime(5 * time.Minute)
}
