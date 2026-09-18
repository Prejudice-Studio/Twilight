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

// PlaybackRecordCounts returns playback row counts for a bounded set of users
// in one database query. The fallback scans the compact in-memory compatibility
// slice once when PostgreSQL is unavailable or the query fails.
func (s *Store) PlaybackRecordCounts(uids []int64) map[int64]int {
	counts := make(map[int64]int, len(uids))
	if len(uids) == 0 {
		return counts
	}
	wanted := make(map[int64]struct{}, len(uids))
	placeholders := make([]string, 0, len(uids))
	args := make([]any, 0, len(uids))
	for _, uid := range uids {
		if uid <= 0 {
			continue
		}
		if _, exists := wanted[uid]; exists {
			continue
		}
		wanted[uid] = struct{}{}
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)+1))
		args = append(args, uid)
	}
	if len(wanted) == 0 {
		return counts
	}
	if s.db != nil {
		query := `SELECT uid, COUNT(*) FROM twilight_playback_records WHERE uid IN (` + strings.Join(placeholders, ",") + `) GROUP BY uid`
		ctx, cancel := context.WithTimeout(context.Background(), pgPlaybackReadTimeout)
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err == nil {
			scanOK := true
			for rows.Next() {
				var uid int64
				var count int
				if scanErr := rows.Scan(&uid, &count); scanErr != nil {
					scanOK = false
					break
				}
				counts[uid] = count
			}
			rowErr := rows.Err()
			_ = rows.Close()
			cancel()
			if scanOK && rowErr == nil {
				return counts
			}
		} else {
			cancel()
		}
		counts = make(map[int64]int, len(uids))
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.state.PlaybackRecords {
		if _, ok := wanted[record.UID]; ok {
			counts[record.UID]++
		}
	}
	return counts
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

// PlaybackRecordCoverage 回答"系统到底记录了多少播放数据"：总条数、最早与最晚
// 播放时间。榜单默认只开窗到今天/本周，运营无从判断库里还沉淀了多少历史可看，
// 这个覆盖面让前端能把"系统已记录区间"直接显示出来。
func (s *Store) PlaybackRecordCoverage() (total int64, earliest int64, latest int64, err error) {
	s.mu.RLock()
	db := s.db
	s.mu.RUnlock()
	if db != nil {
		if err = queryPlaybackCoverageDB(db, &total, &earliest, &latest); err == nil {
			return total, earliest, latest, nil
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.state.PlaybackRecords {
		total++
		if record.PlayedAt <= 0 {
			continue
		}
		if earliest == 0 || record.PlayedAt < earliest {
			earliest = record.PlayedAt
		}
		if record.PlayedAt > latest {
			latest = record.PlayedAt
		}
	}
	return total, earliest, latest, nil
}

func queryPlaybackCoverageDB(db *sql.DB, total *int64, earliest *int64, latest *int64) error {
	query := `SELECT COUNT(*), COALESCE(MIN(played_at), 0), COALESCE(MAX(played_at), 0)
FROM twilight_playback_records`
	ctx, cancel := context.WithTimeout(context.Background(), pgPlaybackReadTimeout)
	defer cancel()
	return db.QueryRowContext(ctx, query).Scan(total, earliest, latest)
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
	query := fmt.Sprintf(`SELECT uid, item_id, title, series_name, media_type, index_number, duration, played_at, source
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
		if err := rows.Scan(&r.UID, &r.ItemID, &r.Title, &r.SeriesName, &r.MediaType, &r.IndexNumber, &r.Duration, &r.PlayedAt, &r.Source); err != nil {
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
	result, err := db.ExecContext(ctx, `INSERT INTO twilight_playback_records (uid, item_id, title, series_name, media_type, index_number, duration, played_at, source)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (uid, item_id, played_at) DO NOTHING`,
		record.UID, record.ItemID, record.Title, record.SeriesName, record.MediaType, record.IndexNumber, record.Duration, record.PlayedAt, playbackSourceForWrite(record.Source))
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
	b.WriteString(`INSERT INTO twilight_playback_records (uid, item_id, title, series_name, media_type, index_number, duration, played_at, source) VALUES `)
	args := make([]any, 0, len(chunk)*9)
	for i, record := range chunk {
		if i > 0 {
			b.WriteByte(',')
		}
		base := i * 9
		fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9)
		args = append(args, record.UID, record.ItemID, record.Title, record.SeriesName, record.MediaType, record.IndexNumber, record.Duration, record.PlayedAt, playbackSourceForWrite(record.Source))
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

// playbackSourceForWrite 把来源收敛到两个枚举值之一。空来源一律算活动日志——
// 在加 source 列之前写入的存量行只可能来自活动日志配对。
func playbackSourceForWrite(source string) string {
	if source == PlaybackSourceReporting {
		return PlaybackSourceReporting
	}
	return PlaybackSourceActivityLog
}

// playbackReportingMatchSlack 是判断"这两行是不是同一场播放"的容差（秒）。
//
// 两个数据源对"这一场播放发生在什么时刻"的记法并不一致，而且偏差没有可靠办法
// 推算：
//   - 插件记的是它自己那条 PlaybackActivity 的时间，活动日志记录的是停止事件的
//     时间，两者相差最多一场播放的长度；
//   - 插件的 SQLite 时间戳可能是 UTC（DateTime('now')），而活动日志走服务器本地
//     时区，两者再差若干个整小时，且取决于 Emby 那台机器的时区配置。
//
// 所以容差取得比较宽，并在候选里挑时间最接近的那一条：同一用户同一集在一天内
// 看两遍时，两条各配各的最近项，不会整体错位。放宽的代价是极端情况下（同一集
// 反复重看）可能配错，而配错只是把两场时长对调，不会凭空多出一次播放——相比
// 配不上导致的"一次播放记两遍"，这是划算的取舍。
const playbackReportingMatchSlack = 12 * 60 * 60

// ApplyPlaybackReportingRecords 用 Playback Reporting 插件的行修正并补充播放记录，
// 返回被修正的条数与新增的条数。
//
// 这里最要紧的是**不能把一次播放记成两次**：活动日志早就为同一场播放写过一行
// （时间是停止时刻、时长是墙上时钟差），插件若直接再插一行，"播放次数"就会翻倍，
// 榜单立刻失真。所以命中同场播放时只把那一行的时长换成净时长并改来源，只有没
// 命中时才插入新行。
//
// 只修正非插件来源的行：已经被插件修正过的行重复同步时不应再次参与匹配，否则
// 时长会被后来的、可能更短的窗口数据覆盖回去。
func (s *Store) ApplyPlaybackReportingRecords(records []PlaybackRecord) (int, int, error) {
	if len(records) == 0 {
		return 0, 0, nil
	}
	s.mu.RLock()
	db := s.db
	s.mu.RUnlock()
	if db != nil {
		matched, inserted, err := applyPlaybackReportingDB(db, records)
		if err == nil {
			return matched, inserted, nil
		}
	}
	return s.applyPlaybackReportingMemory(records)
}

func playbackReportingWindow(record PlaybackRecord) (int64, int64) {
	window := record.WallDuration
	if window < 0 {
		window = 0
	}
	return record.PlayedAt - window - playbackReportingMatchSlack, record.PlayedAt + window + playbackReportingMatchSlack
}

func applyPlaybackReportingDB(db *sql.DB, records []PlaybackRecord) (int, int, error) {
	matched, inserted := 0, 0
	for _, record := range records {
		if record.UID == 0 || record.ItemID == "" {
			continue
		}
		low, high := playbackReportingWindow(record)
		ctx, cancel := context.WithTimeout(context.Background(), pgPlaybackWriteTimeout)
		var id int64
		err := db.QueryRowContext(ctx, `SELECT id FROM twilight_playback_records
WHERE uid = $1 AND item_id = $2 AND played_at BETWEEN $3 AND $4 AND source <> $5
ORDER BY played_at DESC LIMIT 1`,
			record.UID, record.ItemID, low, high, PlaybackSourceReporting).Scan(&id)
		if err == nil {
			if _, execErr := db.ExecContext(ctx, `UPDATE twilight_playback_records
SET duration = $1, source = $2 WHERE id = $3`, record.Duration, PlaybackSourceReporting, id); execErr == nil {
				matched++
			}
			cancel()
			continue
		}
		// xmax = 0 表示这一行是真的插进去的，而不是撞上 (uid,item_id,played_at)
		// 走了 DO UPDATE。重复同步同一窗口时才不会把"新增"虚报一遍。
		var wasInsert bool
		err = db.QueryRowContext(ctx, `INSERT INTO twilight_playback_records
(uid, item_id, title, series_name, media_type, index_number, duration, played_at, source)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (uid, item_id, played_at) DO UPDATE SET duration = EXCLUDED.duration, source = EXCLUDED.source
RETURNING (xmax = 0)`,
			record.UID, record.ItemID, record.Title, record.SeriesName, record.MediaType, record.IndexNumber,
			record.Duration, record.PlayedAt, PlaybackSourceReporting).Scan(&wasInsert)
		cancel()
		if err != nil {
			return matched, inserted, err
		}
		if wasInsert {
			inserted++
		}
	}
	return matched, inserted, nil
}

func (s *Store) applyPlaybackReportingMemory(records []PlaybackRecord) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return 0, 0, err
	}
	matched, inserted := 0, 0
	accepted := make([]PlaybackRecord, 0, len(records))
	for _, record := range records {
		if record.UID == 0 || record.ItemID == "" {
			continue
		}
		low, high := playbackReportingWindow(record)
		hit := -1
		for index := range s.state.PlaybackRecords {
			existing := &s.state.PlaybackRecords[index]
			if existing.UID != record.UID || existing.ItemID != record.ItemID {
				continue
			}
			if existing.PlayedAt < low || existing.PlayedAt > high {
				continue
			}
			if existing.Source == PlaybackSourceReporting {
				continue
			}
			if hit < 0 || absInt64(existing.PlayedAt-record.PlayedAt) < absInt64(s.state.PlaybackRecords[hit].PlayedAt-record.PlayedAt) {
				hit = index
			}
		}
		if hit >= 0 {
			s.state.PlaybackRecords[hit].Duration = record.Duration
			s.state.PlaybackRecords[hit].Source = PlaybackSourceReporting
			matched++
			continue
		}
		duplicate := false
		for index := range s.state.PlaybackRecords {
			existing := &s.state.PlaybackRecords[index]
			if existing.UID == record.UID && existing.ItemID == record.ItemID && existing.PlayedAt == record.PlayedAt {
				existing.Duration = record.Duration
				existing.Source = PlaybackSourceReporting
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		stored := record
		stored.Source = PlaybackSourceReporting
		accepted = append(accepted, stored)
		inserted++
	}
	if len(accepted) == 0 {
		return matched, inserted, nil
	}
	head := make([]PlaybackRecord, 0, len(accepted)+len(s.state.PlaybackRecords))
	for i := len(accepted) - 1; i >= 0; i-- {
		head = append(head, accepted[i])
	}
	head = append(head, s.state.PlaybackRecords...)
	s.state.PlaybackRecords = compactHead(head, maxStoredPlaybackRecords)
	return matched, inserted, s.saveLocked()
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

// playbackRankDefaultLimit / playbackRankMaxLimit 约束榜单长度：排行榜是聚合
// 结果，返回全表没有意义，也容易被拿来拖库。
const (
	playbackRankDefaultLimit = 20
	playbackRankMaxLimit     = 100
)

// PlaybackMediaRank 是一部媒体在某个时间窗内的聚合：播放次数、累计时长和看过
// 的人数。它只回答"哪部最热"，不包含任何一次播放的时间点。
type PlaybackMediaRank struct {
	ItemID     string
	Title      string
	SeriesName string
	MediaType  string
	Plays      int
	Duration   int64
	Viewers    int
}

// PlaybackUserRank 是一个用户在同一时间窗内的聚合。UID 只下发给管理员接口，
// 普通用户接口会把用户名脱敏后丢弃 UID。
type PlaybackUserRank struct {
	UID      int64
	Plays    int
	Duration int64
	Items    int
}

// PlaybackRank 返回 since 之后的媒体榜与用户榜。PG 可用时两条 GROUP BY 直接
// 在库里聚合；PG 不可用或查询失败时回落到内存副本（maxStoredPlaybackRecords
// 条）做同样的聚合，保证降级时榜单仍然可用。
func (s *Store) PlaybackRank(since int64, limit int) ([]PlaybackMediaRank, []PlaybackUserRank, error) {
	if limit <= 0 {
		limit = playbackRankDefaultLimit
	}
	if limit > playbackRankMaxLimit {
		limit = playbackRankMaxLimit
	}
	if since < 0 {
		since = 0
	}

	s.mu.RLock()
	db := s.db
	s.mu.RUnlock()

	if db != nil {
		media, mediaErr := queryPlaybackMediaRankDB(db, since, limit)
		users, userErr := queryPlaybackUserRankDB(db, since, limit)
		if mediaErr == nil && userErr == nil {
			return media, users, nil
		}
		return s.playbackRankFromMemory(since, limit)
	}
	return s.playbackRankFromMemory(since, limit)
}

func queryPlaybackMediaRankDB(db *sql.DB, since int64, limit int) ([]PlaybackMediaRank, error) {
	query := `SELECT item_id, MAX(title), COALESCE(MAX(series_name), ''), COALESCE(MAX(media_type), ''),
	COUNT(*), COALESCE(SUM(duration), 0), COUNT(DISTINCT uid)
FROM twilight_playback_records
WHERE played_at >= $1 AND item_id <> ''
GROUP BY item_id
ORDER BY COUNT(*) DESC, COALESCE(SUM(duration), 0) DESC, item_id ASC
LIMIT $2`
	ctx, cancel := context.WithTimeout(context.Background(), pgPlaybackReadTimeout)
	defer cancel()
	rows, err := db.QueryContext(ctx, query, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PlaybackMediaRank, 0, limit)
	for rows.Next() {
		var item PlaybackMediaRank
		if err := rows.Scan(&item.ItemID, &item.Title, &item.SeriesName, &item.MediaType, &item.Plays, &item.Duration, &item.Viewers); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func queryPlaybackUserRankDB(db *sql.DB, since int64, limit int) ([]PlaybackUserRank, error) {
	query := `SELECT uid, COUNT(*), COALESCE(SUM(duration), 0), COUNT(DISTINCT item_id)
FROM twilight_playback_records
WHERE played_at >= $1 AND uid > 0
GROUP BY uid
ORDER BY COALESCE(SUM(duration), 0) DESC, COUNT(*) DESC, uid ASC
LIMIT $2`
	ctx, cancel := context.WithTimeout(context.Background(), pgPlaybackReadTimeout)
	defer cancel()
	rows, err := db.QueryContext(ctx, query, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PlaybackUserRank, 0, limit)
	for rows.Next() {
		var item PlaybackUserRank
		if err := rows.Scan(&item.UID, &item.Plays, &item.Duration, &item.Items); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// playbackRankFromMemory 是 PG 不可用时的兜底：内存副本本身就是最近的记录，
// 只做一次线性扫描 + map 聚合，再按同样的排序口径截断。
func (s *Store) playbackRankFromMemory(since int64, limit int) ([]PlaybackMediaRank, []PlaybackUserRank, error) {
	type mediaAgg struct {
		item    PlaybackMediaRank
		viewers map[int64]struct{}
	}
	type userAgg struct {
		item  PlaybackUserRank
		items map[string]struct{}
	}

	mediaMap := map[string]*mediaAgg{}
	userMap := map[int64]*userAgg{}

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.state.PlaybackRecords {
		if since > 0 && record.PlayedAt < since {
			continue
		}
		if record.ItemID != "" {
			agg := mediaMap[record.ItemID]
			if agg == nil {
				agg = &mediaAgg{
					item:    PlaybackMediaRank{ItemID: record.ItemID, Title: record.Title, SeriesName: record.SeriesName, MediaType: record.MediaType},
					viewers: map[int64]struct{}{},
				}
				mediaMap[record.ItemID] = agg
			}
			agg.item.Plays++
			agg.item.Duration += record.Duration
			if agg.item.Title == "" {
				agg.item.Title = record.Title
			}
			if agg.item.SeriesName == "" {
				agg.item.SeriesName = record.SeriesName
			}
			if agg.item.MediaType == "" {
				agg.item.MediaType = record.MediaType
			}
			if record.UID != 0 {
				agg.viewers[record.UID] = struct{}{}
			}
		}
		if record.UID > 0 {
			agg := userMap[record.UID]
			if agg == nil {
				agg = &userAgg{item: PlaybackUserRank{UID: record.UID}, items: map[string]struct{}{}}
				userMap[record.UID] = agg
			}
			agg.item.Plays++
			agg.item.Duration += record.Duration
			if record.ItemID != "" {
				agg.items[record.ItemID] = struct{}{}
			}
		}
	}

	media := make([]PlaybackMediaRank, 0, minInt(limit, len(mediaMap)))
	for _, agg := range mediaMap {
		agg.item.Viewers = len(agg.viewers)
		media = append(media, agg.item)
	}
	sort.Slice(media, func(i, j int) bool {
		if media[i].Plays != media[j].Plays {
			return media[i].Plays > media[j].Plays
		}
		if media[i].Duration != media[j].Duration {
			return media[i].Duration > media[j].Duration
		}
		return media[i].ItemID < media[j].ItemID
	})

	users := make([]PlaybackUserRank, 0, minInt(limit, len(userMap)))
	for _, agg := range userMap {
		agg.item.Items = len(agg.items)
		users = append(users, agg.item)
	}
	sort.Slice(users, func(i, j int) bool {
		if users[i].Duration != users[j].Duration {
			return users[i].Duration > users[j].Duration
		}
		if users[i].Plays != users[j].Plays {
			return users[i].Plays > users[j].Plays
		}
		return users[i].UID < users[j].UID
	})

	if len(media) > limit {
		media = media[:limit]
	}
	if len(users) > limit {
		users = users[:limit]
	}
	return media, users, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
