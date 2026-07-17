package store

import (
	"sort"
	"strconv"
	"strings"
)

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

// QueryAuditLogs filters and paginates while holding one read lock. The common
// created_at order scans the append-only slice directly and copies only the
// requested page instead of cloning and sorting the complete audit history.
func (s *Store) QueryAuditLogs(query AuditLogQuery) AuditLogPage {
	query = normalizeAuditLogQuery(query)
	s.mu.RLock()
	defer s.mu.RUnlock()

	if query.SortBy == "created_at" {
		return queryAuditLogsByTime(s.state.AuditLogs, query)
	}

	filtered := make([]AuditLog, 0, len(s.state.AuditLogs))
	for _, entry := range s.state.AuditLogs {
		if auditLogMatchesQuery(entry, query) {
			filtered = append(filtered, entry)
		}
	}
	sortAuditLogEntries(filtered, query.SortBy, query.Order)
	page := AuditLogPage{Logs: []AuditLog{}, Total: len(filtered)}
	if query.Limit <= 0 || query.Offset >= len(filtered) {
		return page
	}
	end := query.Offset + query.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	page.Logs = make([]AuditLog, 0, end-query.Offset)
	for _, entry := range filtered[query.Offset:end] {
		page.Logs = append(page.Logs, cloneAuditLogEntry(entry))
	}
	return page
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
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.mutateAndSaveLocked(func() error {
		if options.MaxEntries > 0 && len(s.state.AuditLogs) > options.MaxEntries {
			result.RemovedByLimit = len(s.state.AuditLogs) - options.MaxEntries
			s.state.AuditLogs = s.state.AuditLogs[result.RemovedByLimit:]
		}
		if options.CutoffUnix > 0 {
			filtered := s.state.AuditLogs[:0]
			for _, entry := range s.state.AuditLogs {
				if entry.CreatedAt < options.CutoffUnix && !(options.PreserveAdmin && strings.EqualFold(entry.Category, "admin")) {
					result.RemovedByAge++
					continue
				}
				filtered = append(filtered, entry)
			}
			s.state.AuditLogs = filtered
		}
		result.Current = len(s.state.AuditLogs)
		return nil
	})
	return result, err
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
