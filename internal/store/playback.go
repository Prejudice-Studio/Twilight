package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const maxStoredPlaybackRecords = 5000

const maxPlaybackSessions = 2000

// playback 相关 PG 读写全部走带超时的 context：这些语句都可能因连接假死
// 而无限阻塞，而 insertPlaybackRecordDB 曾在持有 s.mu 时同步执行——一旦
// PG 卡住会拖垮所有 store 写操作。超时兜底让连接到期自行释放。
const (
	pgPlaybackReadTimeout  = 5 * time.Second
	pgPlaybackWriteTimeout = 5 * time.Second
)

// playbackInsertChunk 是批量 INSERT 单条语句的最大行数：8 列 × 500 行 = 4000
// 占位符，远低于 PG 65535 参数上限，同时避免单条 SQL 过长。
const playbackInsertChunk = 500

// playbackKey 是 (UID, ItemID, PlayedAt) 幂等键，批量去重时用它把逐条 O(N*M)
// 扫描压到 O(N+M)。
type playbackKey struct {
	uid      int64
	itemID   string
	playedAt int64
}

func (s *Store) AddPlaybackSession(session PlaybackSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		s.state.PlaybackSessions = append(s.state.PlaybackSessions, session)
		if len(s.state.PlaybackSessions) > maxPlaybackSessions {
			s.state.PlaybackSessions = compactTail(s.state.PlaybackSessions, maxPlaybackSessions)
		}
		return nil
	})
}

func (s *Store) UserPlaybackSessions(uid int64, limit int) []PlaybackSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.state.PlaybackSessions) {
		limit = len(s.state.PlaybackSessions)
	}
	out := make([]PlaybackSession, 0, limit)
	for i := len(s.state.PlaybackSessions) - 1; i >= 0 && len(out) < limit; i-- {
		sess := s.state.PlaybackSessions[i]
		if uid > 0 && sess.UID != uid {
			continue
		}
		out = append(out, sess)
	}
	return out
}

const maxEmbyActivityLogs = 10000

var errNoEmbyActivityLogChange = errors.New("emby activity logs unchanged")

func (s *Store) SyncEmbyActivityLogs(entries []EmbyActivityLog) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	added := 0
	err := s.mutateAndSaveLocked(func() error {
		changed := false
		if s.state.NextEmbyActivityLogID <= 0 {
			maxID := int64(0)
			for _, entry := range s.state.EmbyActivityLogs {
				if entry.ID > maxID {
					maxID = entry.ID
				}
			}
			s.state.NextEmbyActivityLogID = maxID + 1
			changed = true
		}
		existing := map[int64]int{}
		for i, entry := range s.state.EmbyActivityLogs {
			existing[entry.EmbyLogID] = i
		}
		for _, entry := range entries {
			if index, ok := existing[entry.EmbyLogID]; ok {
				current := s.state.EmbyActivityLogs[index]
				entry.ID = current.ID
				entry.CreatedAt = current.CreatedAt
				if entry == current {
					continue
				}
				s.state.EmbyActivityLogs[index] = entry
				changed = true
				continue
			}
			if entry.ID == 0 {
				entry.ID = s.state.NextEmbyActivityLogID
				s.state.NextEmbyActivityLogID++
			}
			if entry.CreatedAt == 0 {
				entry.CreatedAt = time.Now().Unix()
			}
			s.state.EmbyActivityLogs = append(s.state.EmbyActivityLogs, entry)
			existing[entry.EmbyLogID] = len(s.state.EmbyActivityLogs) - 1
			added++
			changed = true
		}
		if !changed {
			return errNoEmbyActivityLogChange
		}
		sort.Slice(s.state.EmbyActivityLogs, func(i, j int) bool {
			if s.state.EmbyActivityLogs[i].Date != s.state.EmbyActivityLogs[j].Date {
				return s.state.EmbyActivityLogs[i].Date < s.state.EmbyActivityLogs[j].Date
			}
			return s.state.EmbyActivityLogs[i].EmbyLogID < s.state.EmbyActivityLogs[j].EmbyLogID
		})
		if len(s.state.EmbyActivityLogs) > maxEmbyActivityLogs {
			s.state.EmbyActivityLogs = compactTail(s.state.EmbyActivityLogs, maxEmbyActivityLogs)
		}
		return nil
	})
	if errors.Is(err, errNoEmbyActivityLogChange) {
		return added, nil
	}
	return added, err
}

func (s *Store) ListEmbyActivityLogs(uid int64, limit int) []EmbyActivityLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.state.EmbyActivityLogs) {
		limit = len(s.state.EmbyActivityLogs)
	}
	targetEmbyID := ""
	if uid > 0 {
		user, ok := s.state.Users[uid]
		if !ok || user.EmbyID == "" {
			return nil
		}
		targetEmbyID = user.EmbyID
	}
	out := make([]EmbyActivityLog, 0, limit)
	for i := len(s.state.EmbyActivityLogs) - 1; i >= 0 && len(out) < limit; i-- {
		entry := s.state.EmbyActivityLogs[i]
		if uid > 0 {
			if entry.UserID != targetEmbyID {
				continue
			}
		}
		out = append(out, entry)
	}
	return out
}

func (s *Store) AddPlaybackRecord(record PlaybackRecord) error {
	_, err := s.AddPlaybackRecordIdempotent(record)
	return err
}

// AddPlaybackRecordIdempotent 在 (UID, ItemID, PlayedAt) 已经存在时跳过写入，
// 返回 inserted=false。bangumi webhook 没有 timestamp + nonce 强签名，
// 攻击者重放同一份合法请求体本来会让 PlaybackRecords 不停堆积；这里以
// (uid + 媒体条目 + 播放秒) 作为天然幂等键阻断 replay 放大写入。
//
// 真实业务里同一用户在同一秒 stop 同一条目的概率为 0：webhook 由 Emby
// "PlaybackStopped" 事件触发，事件之间至少相隔几秒。即使因网络重试导致同
// 一事件重发，时间戳也会被 server 端的 time.Now() 在重试间隔内推进——所
// 以受害的并不是合法用户，幂等去重不会丢失任何真实播放。
//
// 注意：本方法仍然假定调用方已经做过身份校验。它只阻止"已校验请求"被
// 多次重放——签名伪造 / token 泄露这类外部信任问题不在这里处理。
func (s *Store) AddPlaybackRecordIdempotent(record PlaybackRecord) (bool, error) {
	if record.PlayedAt == 0 {
		record.PlayedAt = time.Now().Unix()
	}
	s.mu.Lock()
	if err := s.refreshLocked(); err != nil {
		s.mu.Unlock()
		return false, err
	}
	if record.UID != 0 && record.ItemID != "" {
		for _, existing := range s.state.PlaybackRecords {
			if existing.UID == record.UID && existing.ItemID == record.ItemID && existing.PlayedAt == record.PlayedAt {
				s.mu.Unlock()
				return false, nil
			}
		}
	}
	s.state.PlaybackRecords = prependBoundedHead(s.state.PlaybackRecords, record, maxStoredPlaybackRecords)
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		return false, err
	}
	db := s.db
	s.mu.Unlock()

	// PG 插入是网络调用，放到锁外执行：记录已经由 saveLocked 落盘（JSON 文件或
	// PG state jsonb），此处的独立表行只是读路径优先命中的副本，ON CONFLICT
	// 保证幂等。持锁跨这段网络 I/O 会让一个卡死的 PG 连接冻结全部 store 写操作。
	if db != nil && record.UID != 0 && record.ItemID != "" {
		if _, dbErr := insertPlaybackRecordDB(db, record); dbErr != nil {
			return false, nil
		}
	}
	return true, nil
}

// AddPlaybackRecordsIdempotent 是 AddPlaybackRecordIdempotent 的批量版本：整批
// 只做一次 refreshLocked + saveLocked，把 emby 活动同步里"逐条落全量状态"的
// N 次 jsonb 序列化压成 1 次。去重语义与单条一致——(UID, ItemID, PlayedAt)
// 命中即跳过，UID==0 或 ItemID=="" 的记录不参与去重（保持旧行为）。返回真正
// 新增进状态的条数。
func (s *Store) AddPlaybackRecordsIdempotent(records []PlaybackRecord) (int, error) {
	if len(records) == 0 {
		return 0, nil
	}
	now := time.Now().Unix()
	s.mu.Lock()
	if err := s.refreshLocked(); err != nil {
		s.mu.Unlock()
		return 0, err
	}
	seen := make(map[playbackKey]struct{}, len(s.state.PlaybackRecords)+len(records))
	for _, existing := range s.state.PlaybackRecords {
		if existing.UID != 0 && existing.ItemID != "" {
			seen[playbackKey{existing.UID, existing.ItemID, existing.PlayedAt}] = struct{}{}
		}
	}
	accepted := make([]PlaybackRecord, 0, len(records))
	for _, record := range records {
		if record.PlayedAt == 0 {
			record.PlayedAt = now
		}
		if record.UID != 0 && record.ItemID != "" {
			key := playbackKey{record.UID, record.ItemID, record.PlayedAt}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
		}
		accepted = append(accepted, record)
	}
	if len(accepted) == 0 {
		s.mu.Unlock()
		return 0, nil
	}
	// 逐条 prepend 等价于把 accepted 反序拼到 head 前再截断到上限。
	head := make([]PlaybackRecord, 0, len(accepted)+len(s.state.PlaybackRecords))
	for i := len(accepted) - 1; i >= 0; i-- {
		head = append(head, accepted[i])
	}
	head = append(head, s.state.PlaybackRecords...)
	s.state.PlaybackRecords = compactHead(head, maxStoredPlaybackRecords)
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		return 0, err
	}
	db := s.db
	s.mu.Unlock()

	if db != nil {
		insertPlaybackRecordsDB(db, accepted)
	}
	return len(accepted), nil
}

func (s *Store) PlaybackRecords(uid int64, since int64, limit int) []PlaybackRecord {
	s.mu.RLock()
	db := s.db
	s.mu.RUnlock()
	if db != nil {
		records, err := queryPlaybackRecordsDB(db, uid, since, limit)
		if err == nil && len(records) > 0 {
			return records
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > maxStoredPlaybackRecords {
		limit = maxStoredPlaybackRecords
	}
	out := make([]PlaybackRecord, 0, minInt(limit, len(s.state.PlaybackRecords)))
	for _, record := range s.state.PlaybackRecords {
		if uid != 0 && record.UID != uid {
			continue
		}
		if since > 0 && record.PlayedAt < since {
			continue
		}
		out = append(out, record)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Store) PlaybackRecordSummary(since int64) (totalPlays int, totalDuration int64, uniqueUsers int, err error) {
	if s.db != nil {
		err = queryPlaybackSummaryDB(s.db, since, &totalPlays, &totalDuration, &uniqueUsers)
		if err == nil {
			return
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	users := map[int64]bool{}
	for _, record := range s.state.PlaybackRecords {
		if since > 0 && record.PlayedAt < since {
			continue
		}
		totalPlays++
		totalDuration += record.Duration
		users[record.UID] = true
	}
	uniqueUsers = len(users)
	return
}

func queryPlaybackRecordsDB(db *sql.DB, uid int64, since int64, limit int) ([]PlaybackRecord, error) {
	var args []any
	var clauses []string
	if uid > 0 {
		clauses = append(clauses, fmt.Sprintf("uid = $%d", len(args)+1))
		args = append(args, uid)
	}
	if since > 0 {
		clauses = append(clauses, fmt.Sprintf("played_at >= $%d", len(args)+1))
		args = append(args, since)
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	if limit <= 0 {
		limit = 10000
	}
	query := fmt.Sprintf(`SELECT uid, item_id, title, series_name, media_type, index_number, duration, played_at
FROM twilight_playback_records %s ORDER BY played_at DESC LIMIT $%d`, where, len(args)+1)
	args = append(args, limit)
	ctx, cancel := context.WithTimeout(context.Background(), pgPlaybackReadTimeout)
	defer cancel()
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []PlaybackRecord
	for rows.Next() {
		var r PlaybackRecord
		if err := rows.Scan(&r.UID, &r.ItemID, &r.Title, &r.SeriesName, &r.MediaType, &r.IndexNumber, &r.Duration, &r.PlayedAt); err != nil {
			return records, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func queryPlaybackSummaryDB(db *sql.DB, since int64, totalPlays *int, totalDuration *int64, uniqueUsers *int) error {
	query := `SELECT COALESCE(COUNT(*), 0), COALESCE(SUM(duration), 0), COALESCE(COUNT(DISTINCT uid), 0)
FROM twilight_playback_records WHERE played_at >= $1`
	ctx, cancel := context.WithTimeout(context.Background(), pgPlaybackReadTimeout)
	defer cancel()
	return db.QueryRowContext(ctx, query, since).Scan(totalPlays, totalDuration, uniqueUsers)
}

func insertPlaybackRecordDB(db *sql.DB, record PlaybackRecord) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pgPlaybackWriteTimeout)
	defer cancel()
	result, err := db.ExecContext(ctx, `INSERT INTO twilight_playback_records (uid, item_id, title, series_name, media_type, index_number, duration, played_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (uid, item_id, played_at) DO NOTHING`,
		record.UID, record.ItemID, record.Title, record.SeriesName, record.MediaType, record.IndexNumber, record.Duration, record.PlayedAt)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// insertPlaybackRecordsDB 把整批记录用分块多行 INSERT 落到独立表。与单条路径
// 一致：DB 失败不影响已落盘的状态副本，因此这里吞掉错误只做尽力写入。空 UID /
// 空 ItemID 记录不入表（与单条路径的写入前置条件一致）。
func insertPlaybackRecordsDB(db *sql.DB, records []PlaybackRecord) {
	valid := records[:0:0]
	for _, record := range records {
		if record.UID != 0 && record.ItemID != "" {
			valid = append(valid, record)
		}
	}
	for start := 0; start < len(valid); start += playbackInsertChunk {
		end := start + playbackInsertChunk
		if end > len(valid) {
			end = len(valid)
		}
		insertPlaybackRecordChunkDB(db, valid[start:end])
	}
}

func insertPlaybackRecordChunkDB(db *sql.DB, chunk []PlaybackRecord) {
	if len(chunk) == 0 {
		return
	}
	var b strings.Builder
	b.WriteString(`INSERT INTO twilight_playback_records (uid, item_id, title, series_name, media_type, index_number, duration, played_at) VALUES `)
	args := make([]any, 0, len(chunk)*8)
	for i, record := range chunk {
		if i > 0 {
			b.WriteByte(',')
		}
		base := i * 8
		fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8)
		args = append(args, record.UID, record.ItemID, record.Title, record.SeriesName, record.MediaType, record.IndexNumber, record.Duration, record.PlayedAt)
	}
	b.WriteString(` ON CONFLICT (uid, item_id, played_at) DO NOTHING`)
	ctx, cancel := context.WithTimeout(context.Background(), pgPlaybackWriteTimeout)
	defer cancel()
	_, _ = db.ExecContext(ctx, b.String(), args...)
}

func (s *Store) DeletePlaybackRecordsBefore(ctx context.Context, cutoff int64) (int64, error) {
	if s.db == nil {
		return 0, nil
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM twilight_playback_records WHERE played_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
