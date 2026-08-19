package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const pgAuditLogTimeout = 10 * time.Second

// AuditLogQuery describes a bounded server-side audit log query. Offset and
// Limit are applied after filtering and sorting; Limit <= 0 returns no rows but
// still computes Total.
type AuditLogQuery struct {
	Category       string
	Action         string
	UID            int64
	TargetUID      int64
	From           int64
	To             int64
	Search         string
	ActionKeywords []string
	SortBy         string
	Order          string
	Offset         int
	Limit          int
}

type AuditLogPage struct {
	Logs  []AuditLog
	Total int
}

// QueryAuditLogs filters, sorts, and paginates in PostgreSQL. Audit history is
// intentionally outside the single state document so listing it does not copy
// the complete in-memory State and appending it does not rewrite twilight_state.
func (s *Store) QueryAuditLogs(query AuditLogQuery) AuditLogPage {
	query = normalizeAuditLogQuery(query)
	page := AuditLogPage{Logs: []AuditLog{}}
	if s == nil || s.db == nil {
		return page
	}
	where, args := auditLogWhereSQL(query)
	ctx, cancel := context.WithTimeout(context.Background(), pgAuditLogTimeout)
	defer cancel()
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM twilight_audit_logs`+where, args...).Scan(&page.Total); err != nil {
		return AuditLogPage{Logs: []AuditLog{}}
	}
	if query.Limit <= 0 || query.Offset >= page.Total {
		return page
	}
	limitArg := len(args) + 1
	offsetArg := limitArg + 1
	args = append(args, query.Limit, query.Offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT id, uid, username, action, category, source, method, target_uid,
       COALESCE(detail, '{}'::jsonb)::text, ip, created_at
FROM twilight_audit_logs`+where+`
ORDER BY `+auditLogOrderSQL(query.SortBy, query.Order)+`
LIMIT $`+strconv.Itoa(limitArg)+` OFFSET $`+strconv.Itoa(offsetArg), args...)
	if err != nil {
		return AuditLogPage{Logs: []AuditLog{}, Total: page.Total}
	}
	defer rows.Close()
	for rows.Next() {
		entry, scanErr := scanAuditLog(rows.Scan)
		if scanErr == nil {
			page.Logs = append(page.Logs, entry)
		}
	}
	return page
}

type auditLogScanner func(dest ...any) error

func scanAuditLog(scan auditLogScanner) (AuditLog, error) {
	var entry AuditLog
	var detailText string
	if err := scan(&entry.ID, &entry.UID, &entry.Username, &entry.Action, &entry.Category, &entry.Source, &entry.Method, &entry.TargetUID, &detailText, &entry.IP, &entry.CreatedAt); err != nil {
		return AuditLog{}, err
	}
	if detailText != "" && detailText != "{}" {
		if err := json.Unmarshal([]byte(detailText), &entry.Detail); err != nil {
			return AuditLog{}, err
		}
	}
	return entry, nil
}

func auditLogWhereSQL(query AuditLogQuery) (string, []any) {
	clauses := make([]string, 0, 8)
	args := make([]any, 0, 8)
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if query.Category != "" {
		add("LOWER(category) = $%d", query.Category)
	}
	if query.Action != "" {
		add("LOWER(action) = $%d", query.Action)
	}
	if query.UID > 0 {
		add("uid = $%d", query.UID)
	}
	if query.TargetUID > 0 {
		add("target_uid = $%d", query.TargetUID)
	}
	if query.From > 0 {
		add("created_at >= $%d", query.From)
	}
	if query.To > 0 {
		add("created_at <= $%d", query.To)
	}
	if len(query.ActionKeywords) > 0 {
		parts := make([]string, 0, len(query.ActionKeywords))
		for _, keyword := range query.ActionKeywords {
			if keyword == "" {
				continue
			}
			args = append(args, auditLogLikePattern(keyword))
			parts = append(parts, "LOWER(action) LIKE $"+strconv.Itoa(len(args))+` ESCAPE E'\\'`)
		}
		if len(parts) > 0 {
			clauses = append(clauses, "("+strings.Join(parts, " OR ")+")")
		}
	}
	if query.Search != "" {
		args = append(args, auditLogLikePattern(query.Search))
		placeholder := "$" + strconv.Itoa(len(args))
		if strings.Contains(query.Search, " ") {
			clauses = append(clauses, `LOWER(CONCAT_WS(' ', username, action, category, source, method, ip, uid::text, target_uid::text)) LIKE `+placeholder+` ESCAPE E'\\'`)
		} else {
			fields := []string{"username", "action", "category", "source", "method", "ip", "uid::text", "target_uid::text"}
			parts := make([]string, 0, len(fields))
			for _, field := range fields {
				parts = append(parts, "LOWER("+field+") LIKE "+placeholder+` ESCAPE E'\\'`)
			}
			clauses = append(clauses, "("+strings.Join(parts, " OR ")+")")
		}
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func auditLogLikePattern(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return "%" + strings.ToLower(value) + "%"
}

func auditLogOrderSQL(sortBy, order string) string {
	direction := "DESC"
	if order == "asc" {
		direction = "ASC"
	}
	field := "created_at"
	switch sortBy {
	case "id", "uid", "target_uid":
		field = sortBy
	case "action", "category", "source", "method", "username", "ip":
		field = "LOWER(" + sortBy + ")"
	}
	return field + " " + direction + ", id " + direction
}

func normalizeAuditLogQuery(query AuditLogQuery) AuditLogQuery {
	query.Category = strings.ToLower(strings.TrimSpace(query.Category))
	query.Action = strings.ToLower(strings.TrimSpace(query.Action))
	query.Search = strings.ToLower(strings.TrimSpace(query.Search))
	query.SortBy = normalizeAuditLogSortField(query.SortBy)
	if !strings.EqualFold(query.Order, "asc") {
		query.Order = "desc"
	} else {
		query.Order = "asc"
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	if query.Limit < 0 {
		query.Limit = 0
	}
	for i, keyword := range query.ActionKeywords {
		query.ActionKeywords[i] = strings.ToLower(strings.TrimSpace(keyword))
	}
	return query
}

func normalizeAuditLogSortField(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "id", "action", "category", "source", "method", "username", "uid", "target_uid", "ip":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "created_at"
	}
}

type AuditLogPruneOptions struct {
	MaxEntries    int
	CutoffUnix    int64
	PreserveAdmin bool
}

type AuditLogPruneResult struct {
	RemovedByLimit int
	RemovedByAge   int
	Current        int
}

// PruneAuditLogsWithPolicy applies count and age retention in one mutation and
// one persistence cycle. Count retention runs first to preserve legacy behavior.
func (s *Store) PruneAuditLogsWithPolicy(options AuditLogPruneOptions) (AuditLogPruneResult, error) {
	result := AuditLogPruneResult{}
	if options.MaxEntries <= 0 && options.CutoffUnix <= 0 {
		result.Current = s.AuditLogCount()
		return result, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), pgAuditLogTimeout)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if options.MaxEntries > 0 {
		res, execErr := tx.ExecContext(ctx, `
WITH cutoff AS (
	SELECT MIN(id) AS min_id FROM (
		SELECT id FROM twilight_audit_logs ORDER BY id DESC LIMIT $1
	) latest
)
DELETE FROM twilight_audit_logs
WHERE id < COALESCE((SELECT min_id FROM cutoff), 0)`, options.MaxEntries)
		if execErr != nil {
			return result, execErr
		}
		removed, _ := res.RowsAffected()
		result.RemovedByLimit = int(removed)
	}
	if options.CutoffUnix > 0 {
		query := `DELETE FROM twilight_audit_logs WHERE created_at < $1`
		if options.PreserveAdmin {
			query += ` AND LOWER(category) <> 'admin'`
		}
		res, execErr := tx.ExecContext(ctx, query, options.CutoffUnix)
		if execErr != nil {
			return result, execErr
		}
		removed, _ := res.RowsAffected()
		result.RemovedByAge = int(removed)
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM twilight_audit_logs`).Scan(&result.Current); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

// AddAuditLog appends one security audit row without touching twilight_state.
// Retention runs in the same transaction so a successful return means both the
// new event and the configured bound are durable.
func (s *Store) AddAuditLog(entry AuditLog, limit int) error {
	if s == nil || s.db == nil {
		return ErrNotFound
	}
	if entry.CreatedAt == 0 {
		entry.CreatedAt = time.Now().Unix()
	}
	detail, err := json.Marshal(entry.Detail)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), pgAuditLogTimeout)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO twilight_audit_logs
    (uid, username, action, category, source, method, target_uid, detail, ip, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10)`,
		entry.UID, entry.Username, entry.Action, entry.Category, entry.Source, entry.Method,
		entry.TargetUID, string(detail), entry.IP, entry.CreatedAt); err != nil {
		return err
	}
	if limit > 0 {
		if _, err := tx.ExecContext(ctx, `
WITH cutoff AS (
	SELECT MIN(id) AS min_id FROM (
		SELECT id FROM twilight_audit_logs ORDER BY id DESC LIMIT $1
	) latest
)
DELETE FROM twilight_audit_logs
WHERE id < COALESCE((SELECT min_id FROM cutoff), 0)`, limit); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListAuditLogs returns all rows newest first. Runtime callers should prefer
// QueryAuditLogs so normal page reads remain bounded.
func (s *Store) ListAuditLogs() []AuditLog {
	if s == nil || s.db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), pgAuditLogTimeout)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `
SELECT id, uid, username, action, category, source, method, target_uid,
       COALESCE(detail, '{}'::jsonb)::text, ip, created_at
FROM twilight_audit_logs ORDER BY id DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]AuditLog, 0)
	for rows.Next() {
		entry, scanErr := scanAuditLog(rows.Scan)
		if scanErr == nil {
			out = append(out, entry)
		}
	}
	return out
}

func (s *Store) DeleteAuditLog(id int64) error {
	if id <= 0 {
		return ErrNotFound
	}
	ctx, cancel := context.WithTimeout(context.Background(), pgAuditLogTimeout)
	defer cancel()
	result, err := s.db.ExecContext(ctx, `DELETE FROM twilight_audit_logs WHERE id = $1`, id)
	if err != nil {
		return err
	}
	removed, _ := result.RowsAffected()
	if removed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ClearAuditLogs() error {
	ctx, cancel := context.WithTimeout(context.Background(), pgAuditLogTimeout)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `TRUNCATE TABLE twilight_audit_logs RESTART IDENTITY`)
	return err
}

func (s *Store) PruneAuditLogs(keep int) error {
	if keep <= 0 {
		return nil
	}
	_, err := s.PruneAuditLogsWithPolicy(AuditLogPruneOptions{MaxEntries: keep})
	return err
}

func (s *Store) PruneAuditLogsByAge(cutoffUnix int64, preserveAdmin bool) int {
	result, err := s.PruneAuditLogsWithPolicy(AuditLogPruneOptions{CutoffUnix: cutoffUnix, PreserveAdmin: preserveAdmin})
	if err != nil {
		return 0
	}
	return result.RemovedByAge
}

func (s *Store) AuditLogCount() int {
	if s == nil || s.db == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), pgAuditLogTimeout)
	defer cancel()
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM twilight_audit_logs`).Scan(&count); err != nil {
		return 0
	}
	return count
}

// migrateLegacyAuditLogs moves historical state.audit_logs into the dedicated
// table. ON CONFLICT plus an exclusive table lock makes repeated or concurrent
// startup migrations safe; the state payload is cleared only after inserts are
// durable, so an interrupted migration can be retried without data loss.
func (s *Store) migrateLegacyAuditLogs(parent context.Context) error {
	if len(s.state.AuditLogs) == 0 {
		return nil
	}
	entries := append([]AuditLog(nil), s.state.AuditLogs...)
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `LOCK TABLE twilight_audit_logs IN EXCLUSIVE MODE`); err != nil {
		return err
	}
	if err := insertAuditLogsTx(ctx, tx, entries); err != nil {
		return err
	}
	if err := syncAuditLogSequence(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		s.state.AuditLogs = nil
		s.state.NextAuditLogID = 1
		return nil
	})
}

func insertAuditLogsTx(ctx context.Context, tx *sql.Tx, entries []AuditLog) error {
	if len(entries) == 0 {
		return nil
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO twilight_audit_logs
    (id, uid, username, action, category, source, method, target_uid, detail, ip, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11)
ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, entry := range entries {
		if entry.ID <= 0 {
			continue
		}
		detail, err := json.Marshal(entry.Detail)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, entry.ID, entry.UID, entry.Username, entry.Action, entry.Category, entry.Source, entry.Method, entry.TargetUID, string(detail), entry.IP, entry.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

func syncAuditLogSequence(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
SELECT setval(
	pg_get_serial_sequence('twilight_audit_logs', 'id'),
	COALESCE((SELECT MAX(id) FROM twilight_audit_logs), 1),
	EXISTS (SELECT 1 FROM twilight_audit_logs)
)`)
	return err
}
