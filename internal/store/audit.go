package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
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

func queryAuditLogsByTime(logs []AuditLog, query AuditLogQuery) AuditLogPage {
	page := AuditLogPage{Logs: []AuditLog{}}
	appendMatch := func(entry AuditLog) {
		matchIndex := page.Total
		page.Total++
		if query.Limit > 0 && matchIndex >= query.Offset && len(page.Logs) < query.Limit {
			page.Logs = append(page.Logs, cloneAuditLogEntry(entry))
		}
	}
	if query.Order == "asc" {
		for _, entry := range logs {
			if auditLogMatchesQuery(entry, query) {
				appendMatch(entry)
			}
		}
		return page
	}
	for i := len(logs) - 1; i >= 0; i-- {
		if auditLogMatchesQuery(logs[i], query) {
			appendMatch(logs[i])
		}
	}
	return page
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

func auditLogMatchesQuery(entry AuditLog, query AuditLogQuery) bool {
	if query.Category != "" && !strings.EqualFold(entry.Category, query.Category) {
		return false
	}
	if query.Action != "" && !strings.EqualFold(entry.Action, query.Action) {
		return false
	}
	if query.UID > 0 && entry.UID != query.UID {
		return false
	}
	if query.TargetUID > 0 && entry.TargetUID != query.TargetUID {
		return false
	}
	if query.From > 0 && entry.CreatedAt < query.From {
		return false
	}
	if query.To > 0 && entry.CreatedAt > query.To {
		return false
	}
	if len(query.ActionKeywords) > 0 {
		action := strings.ToLower(entry.Action)
		matched := false
		for _, keyword := range query.ActionKeywords {
			if keyword != "" && strings.Contains(action, keyword) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if query.Search == "" {
		return true
	}
	return auditLogMatchesSearch(entry, query.Search)
}

func auditLogMatchesSearch(entry AuditLog, search string) bool {
	if strings.Contains(search, " ") {
		return strings.Contains(auditLogSearchHaystack(entry), search)
	}
	return containsLower(entry.Username, search) ||
		containsLower(entry.Action, search) ||
		containsLower(entry.Category, search) ||
		containsLower(entry.Source, search) ||
		containsLower(entry.Method, search) ||
		containsLower(entry.IP, search) ||
		strings.Contains(formatInt64(entry.UID), search) ||
		strings.Contains(formatInt64(entry.TargetUID), search)
}

func containsLower(value, search string) bool {
	return value != "" && strings.Contains(strings.ToLower(value), search)
}

func auditLogSearchHaystack(entry AuditLog) string {
	var b strings.Builder
	b.Grow(len(entry.Username) + len(entry.Action) + len(entry.Category) + len(entry.Source) + len(entry.Method) + len(entry.IP) + 44)
	b.WriteString(entry.Username)
	b.WriteByte(' ')
	b.WriteString(entry.Action)
	b.WriteByte(' ')
	b.WriteString(entry.Category)
	b.WriteByte(' ')
	b.WriteString(entry.Source)
	b.WriteByte(' ')
	b.WriteString(entry.Method)
	b.WriteByte(' ')
	b.WriteString(entry.IP)
	b.WriteByte(' ')
	b.WriteString(formatInt64(entry.UID))
	b.WriteByte(' ')
	b.WriteString(formatInt64(entry.TargetUID))
	return strings.ToLower(b.String())
}

func normalizeAuditLogSortField(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "id", "action", "category", "source", "method", "username", "uid", "target_uid", "ip":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "created_at"
	}
}

func sortAuditLogEntries(logs []AuditLog, sortBy, order string) {
	desc := order != "asc"
	if stringAuditLogSortField(sortBy) {
		sortAuditLogEntriesByString(logs, sortBy, desc)
		return
	}
	sort.SliceStable(logs, func(i, j int) bool {
		left, right := logs[i], logs[j]
		cmp := 0
		switch sortBy {
		case "id":
			cmp = compareInt64(left.ID, right.ID)
		case "uid":
			cmp = compareInt64(left.UID, right.UID)
		case "target_uid":
			cmp = compareInt64(left.TargetUID, right.TargetUID)
		default:
			cmp = compareInt64(left.CreatedAt, right.CreatedAt)
		}
		if cmp == 0 {
			cmp = compareInt64(left.ID, right.ID)
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
}

func stringAuditLogSortField(sortBy string) bool {
	switch sortBy {
	case "action", "category", "source", "method", "username", "ip":
		return true
	default:
		return false
	}
}

type auditLogStringSortEntry struct {
	log AuditLog
	key string
}

func sortAuditLogEntriesByString(logs []AuditLog, sortBy string, desc bool) {
	items := make([]auditLogStringSortEntry, len(logs))
	for i, entry := range logs {
		items[i] = auditLogStringSortEntry{log: entry, key: auditLogStringSortKey(entry, sortBy)}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		cmp := strings.Compare(left.key, right.key)
		if cmp == 0 {
			cmp = compareInt64(left.log.ID, right.log.ID)
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
	for i, item := range items {
		logs[i] = item.log
	}
}

func auditLogStringSortKey(entry AuditLog, sortBy string) string {
	switch sortBy {
	case "action":
		return strings.ToLower(entry.Action)
	case "category":
		return strings.ToLower(entry.Category)
	case "source":
		return strings.ToLower(entry.Source)
	case "method":
		return strings.ToLower(entry.Method)
	case "username":
		return strings.ToLower(entry.Username)
	case "ip":
		return strings.ToLower(entry.IP)
	default:
		return ""
	}
}

func compareInt64(left, right int64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func formatInt64(value int64) string {
	return strconv.FormatInt(value, 10)
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

func cloneAuditLogEntry(entry AuditLog) AuditLog {
	entry.Detail = cloneAuditLogDetail(entry.Detail)
	return entry
}

func cloneAuditLogDetail(detail map[string]any) map[string]any {
	if len(detail) == 0 {
		return nil
	}
	clone := make(map[string]any, len(detail))
	for key, value := range detail {
		clone[key] = cloneAuditLogDetailValue(value)
	}
	return clone
}

func cloneAuditLogDetailValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAuditLogDetail(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneAuditLogDetailValue(item)
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	case []int:
		return append([]int(nil), typed...)
	case []int64:
		return append([]int64(nil), typed...)
	case []float64:
		return append([]float64(nil), typed...)
	case []bool:
		return append([]bool(nil), typed...)
	default:
		return value
	}
}
