package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

const (
	telegramRosterObservationWriteInterval = 5 * time.Minute
	telegramRosterDBTimeout                = 5 * time.Second
	telegramRosterCacheCapacity            = 4096
)

type telegramRosterCacheKey struct {
	chatID     string
	telegramID int64
}

type telegramRosterCacheEntry struct {
	status      string
	isBot       bool
	lastWritten int64
	sequence    uint64
}

type telegramRosterCacheReservation struct {
	key      telegramRosterCacheKey
	previous telegramRosterCacheEntry
	existed  bool
	sequence uint64
}

type telegramRosterRow struct {
	ChatID     string `json:"chat_id"`
	TelegramID int64  `json:"telegram_id"`
	IsBot      bool   `json:"is_bot"`
	LastStatus string `json:"last_status"`
	FirstSeen  int64  `json:"first_seen"`
	LastSeen   int64  `json:"last_seen"`
}

type telegramRosterExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func normalizeTelegramRosterStatus(status, fallback string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return fallback
	}
	return status
}

func telegramRosterObservationNeedsWrite(entry telegramRosterCacheEntry, exists bool, status string, isBot bool, now int64) bool {
	if !exists || entry.status != status || (isBot && !entry.isBot) {
		return true
	}
	return entry.lastWritten <= 0 || now-entry.lastWritten >= int64(telegramRosterObservationWriteInterval/time.Second)
}

func (s *Store) reserveTelegramRosterObservation(key telegramRosterCacheKey, status string, isBot bool, now int64) (telegramRosterCacheReservation, bool) {
	s.telegramRosterCacheMu.Lock()
	defer s.telegramRosterCacheMu.Unlock()
	if s.telegramRosterCache == nil {
		s.telegramRosterCache = make(map[telegramRosterCacheKey]telegramRosterCacheEntry, 64)
	}
	previous, exists := s.telegramRosterCache[key]
	if !telegramRosterObservationNeedsWrite(previous, exists, status, isBot, now) {
		return telegramRosterCacheReservation{}, false
	}
	if !exists && len(s.telegramRosterCache) >= telegramRosterCacheCapacity {
		s.pruneTelegramRosterCacheLocked(now)
	}
	s.telegramRosterCacheSeq++
	current := telegramRosterCacheEntry{
		status:      status,
		isBot:       isBot || previous.isBot,
		lastWritten: now,
		sequence:    s.telegramRosterCacheSeq,
	}
	s.telegramRosterCache[key] = current
	return telegramRosterCacheReservation{key: key, previous: previous, existed: exists, sequence: current.sequence}, true
}

func (s *Store) rollbackTelegramRosterObservation(reservation telegramRosterCacheReservation) {
	s.telegramRosterCacheMu.Lock()
	defer s.telegramRosterCacheMu.Unlock()
	current, ok := s.telegramRosterCache[reservation.key]
	if !ok || current.sequence != reservation.sequence {
		return
	}
	if reservation.existed {
		s.telegramRosterCache[reservation.key] = reservation.previous
		return
	}
	delete(s.telegramRosterCache, reservation.key)
}

func (s *Store) rememberTelegramRosterObservation(key telegramRosterCacheKey, status string, isBot bool, now int64) {
	s.telegramRosterCacheMu.Lock()
	defer s.telegramRosterCacheMu.Unlock()
	if s.telegramRosterCache == nil {
		s.telegramRosterCache = make(map[telegramRosterCacheKey]telegramRosterCacheEntry, 64)
	}
	previous, exists := s.telegramRosterCache[key]
	if !exists && len(s.telegramRosterCache) >= telegramRosterCacheCapacity {
		s.pruneTelegramRosterCacheLocked(now)
	}
	s.telegramRosterCacheSeq++
	s.telegramRosterCache[key] = telegramRosterCacheEntry{
		status:      status,
		isBot:       isBot || previous.isBot,
		lastWritten: now,
		sequence:    s.telegramRosterCacheSeq,
	}
}

func (s *Store) pruneTelegramRosterCacheLocked(now int64) {
	cutoff := now - int64(telegramRosterObservationWriteInterval/time.Second)
	for key, entry := range s.telegramRosterCache {
		if entry.lastWritten <= cutoff {
			delete(s.telegramRosterCache, key)
		}
	}
	if len(s.telegramRosterCache) < telegramRosterCacheCapacity {
		return
	}
	var oldestKey telegramRosterCacheKey
	var oldestSeq uint64
	for key, entry := range s.telegramRosterCache {
		if oldestSeq == 0 || entry.sequence < oldestSeq {
			oldestKey = key
			oldestSeq = entry.sequence
		}
	}
	if oldestSeq != 0 {
		delete(s.telegramRosterCache, oldestKey)
	}
}

func (s *Store) clearTelegramRosterCache() {
	s.telegramRosterCacheMu.Lock()
	s.telegramRosterCache = nil
	s.telegramRosterCacheMu.Unlock()
}

// UpsertTelegramRoster records an observed member while keeping the message hot
// path in memory. PostgreSQL still enforces the five-minute write interval after
// restarts or cache eviction, so a cold cache cannot cause a physical write
// storm.
func (s *Store) UpsertTelegramRoster(chatID string, telegramID int64, status string, isBot bool) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" || telegramID <= 0 {
		return nil
	}
	status = normalizeTelegramRosterStatus(status, "member")
	now := time.Now().Unix()
	key := telegramRosterCacheKey{chatID: chatID, telegramID: telegramID}
	reservation, write := s.reserveTelegramRosterObservation(key, status, isBot, now)
	if !write {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), telegramRosterDBTimeout)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO twilight_telegram_roster
	(chat_id, telegram_id, is_bot, last_status, first_seen, last_seen)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (chat_id, telegram_id) DO UPDATE SET
	is_bot = twilight_telegram_roster.is_bot OR EXCLUDED.is_bot,
	last_status = EXCLUDED.last_status,
	last_seen = EXCLUDED.last_seen
WHERE twilight_telegram_roster.last_status IS DISTINCT FROM EXCLUDED.last_status
	OR (EXCLUDED.is_bot AND NOT twilight_telegram_roster.is_bot)
	OR twilight_telegram_roster.last_seen <= EXCLUDED.last_seen - $6`,
		chatID, telegramID, isBot, status, now, int64(telegramRosterObservationWriteInterval/time.Second))
	if err != nil {
		s.rollbackTelegramRosterObservation(reservation)
	}
	return err
}

func (s *Store) MarkTelegramRosterLeft(chatID string, telegramID int64, status string) error {
	return s.UpsertTelegramRoster(chatID, telegramID, normalizeTelegramRosterStatus(status, "left"), false)
}

// ApplyTelegramRosterUpdates consolidates one scheduler pass into one SQL
// statement. Duplicate members in the same pass use the last status and retain
// any observed bot flag.
func (s *Store) ApplyTelegramRosterUpdates(updates []TelegramRosterUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	now := time.Now().Unix()
	entries := make(map[telegramRosterCacheKey]TelegramRosterEntry, len(updates))
	for _, update := range updates {
		chatID := strings.TrimSpace(update.ChatID)
		if chatID == "" || update.TelegramID <= 0 {
			continue
		}
		key := telegramRosterCacheKey{chatID: chatID, telegramID: update.TelegramID}
		entry := entries[key]
		entry.ChatID = chatID
		entry.TelegramID = update.TelegramID
		entry.LastStatus = normalizeTelegramRosterStatus(update.Status, "member")
		entry.IsBot = entry.IsBot || update.IsBot
		entry.FirstSeen = now
		entry.LastSeen = now
		entries[key] = entry
	}
	rows := make([]TelegramRosterEntry, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, entry)
	}
	ctx, cancel := context.WithTimeout(context.Background(), telegramRosterDBTimeout)
	defer cancel()
	if err := upsertTelegramRosterEntries(ctx, s.db, rows); err != nil {
		return err
	}
	for key, entry := range entries {
		s.rememberTelegramRosterObservation(key, entry.LastStatus, entry.IsBot, now)
	}
	return nil
}

func upsertTelegramRosterEntries(ctx context.Context, execer telegramRosterExecer, entries []TelegramRosterEntry) error {
	if len(entries) == 0 {
		return nil
	}
	now := time.Now().Unix()
	merged := make(map[telegramRosterCacheKey]TelegramRosterEntry, len(entries))
	for _, raw := range entries {
		chatID := strings.TrimSpace(raw.ChatID)
		if chatID == "" || raw.TelegramID <= 0 {
			continue
		}
		raw.ChatID = chatID
		raw.LastStatus = normalizeTelegramRosterStatus(raw.LastStatus, "member")
		if raw.LastSeen <= 0 {
			raw.LastSeen = now
		}
		if raw.FirstSeen <= 0 || raw.FirstSeen > raw.LastSeen {
			raw.FirstSeen = raw.LastSeen
		}
		key := telegramRosterCacheKey{chatID: chatID, telegramID: raw.TelegramID}
		if current, ok := merged[key]; ok {
			raw.IsBot = raw.IsBot || current.IsBot
			if current.FirstSeen > 0 && current.FirstSeen < raw.FirstSeen {
				raw.FirstSeen = current.FirstSeen
			}
			if current.LastSeen > raw.LastSeen {
				raw.LastSeen = current.LastSeen
				raw.LastStatus = current.LastStatus
			}
		}
		merged[key] = raw
	}
	rows := make([]telegramRosterRow, 0, len(merged))
	for _, entry := range merged {
		rows = append(rows, telegramRosterRow{
			ChatID: entry.ChatID, TelegramID: entry.TelegramID, IsBot: entry.IsBot,
			LastStatus: entry.LastStatus, FirstSeen: entry.FirstSeen, LastSeen: entry.LastSeen,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	_, err = execer.ExecContext(ctx, `
WITH incoming AS (
	SELECT chat_id, telegram_id, is_bot, last_status, first_seen, last_seen
	FROM jsonb_to_recordset($1::jsonb) AS x(
		chat_id text, telegram_id bigint, is_bot boolean, last_status text,
		first_seen bigint, last_seen bigint
	)
)
INSERT INTO twilight_telegram_roster
	(chat_id, telegram_id, is_bot, last_status, first_seen, last_seen)
SELECT chat_id, telegram_id, is_bot, last_status, first_seen, last_seen FROM incoming
ON CONFLICT (chat_id, telegram_id) DO UPDATE SET
	is_bot = twilight_telegram_roster.is_bot OR EXCLUDED.is_bot,
	last_status = CASE
		WHEN EXCLUDED.last_seen >= twilight_telegram_roster.last_seen THEN EXCLUDED.last_status
		ELSE twilight_telegram_roster.last_status
	END,
	first_seen = CASE
		WHEN twilight_telegram_roster.first_seen <= 0 THEN EXCLUDED.first_seen
		WHEN EXCLUDED.first_seen <= 0 THEN twilight_telegram_roster.first_seen
		ELSE LEAST(twilight_telegram_roster.first_seen, EXCLUDED.first_seen)
	END,
	last_seen = GREATEST(twilight_telegram_roster.last_seen, EXCLUDED.last_seen)`, payload)
	return err
}

func (s *Store) TelegramRoster(chatID string, activeOnly bool) ([]TelegramRosterEntry, error) {
	chatID = strings.TrimSpace(chatID)
	ctx, cancel := context.WithTimeout(context.Background(), telegramRosterDBTimeout)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `
SELECT chat_id, telegram_id, is_bot, last_status, first_seen, last_seen
FROM twilight_telegram_roster
WHERE ($1 = '' OR chat_id = $1)
	AND (NOT $2 OR LOWER(BTRIM(last_status)) IN ('', 'member', 'administrator', 'creator', 'restricted'))
ORDER BY chat_id ASC, telegram_id ASC`, chatID, activeOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TelegramRosterEntry, 0)
	for rows.Next() {
		var entry TelegramRosterEntry
		if err := rows.Scan(&entry.ChatID, &entry.TelegramID, &entry.IsBot, &entry.LastStatus, &entry.FirstSeen, &entry.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (s *Store) TelegramRosterStats(chatID string) (map[string]any, error) {
	chatID = strings.TrimSpace(chatID)
	ctx, cancel := context.WithTimeout(context.Background(), telegramRosterDBTimeout)
	defer cancel()
	var total, active, inactive, bots int
	var firstSeen, lastSeen int64
	err := s.db.QueryRowContext(ctx, `
SELECT
	COUNT(*)::int,
	COUNT(*) FILTER (WHERE LOWER(BTRIM(last_status)) IN ('', 'member', 'administrator', 'creator', 'restricted'))::int,
	COUNT(*) FILTER (WHERE LOWER(BTRIM(last_status)) NOT IN ('', 'member', 'administrator', 'creator', 'restricted'))::int,
	COUNT(*) FILTER (WHERE is_bot)::int,
	COALESCE(MIN(NULLIF(first_seen, 0)), 0),
	COALESCE(MAX(last_seen), 0)
FROM twilight_telegram_roster
WHERE ($1 = '' OR chat_id = $1)`, chatID).Scan(&total, &active, &inactive, &bots, &firstSeen, &lastSeen)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"chat_id": chatID, "total": total, "active": active, "inactive": inactive,
		"bots": bots, "first_seen_at": nil, "last_seen_at": nil,
	}
	if firstSeen > 0 {
		result["first_seen_at"] = firstSeen
	}
	if lastSeen > 0 {
		result["last_seen_at"] = lastSeen
	}
	return result, nil
}

func telegramRosterKey(chatID string, telegramID int64) string {
	return strings.TrimSpace(chatID) + ":" + strconv36(telegramID)
}

func (s *Store) snapshotTelegramRosterLocked() (map[string]TelegramRosterEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `
SELECT chat_id, telegram_id, is_bot, last_status, first_seen, last_seen
FROM twilight_telegram_roster ORDER BY chat_id ASC, telegram_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]TelegramRosterEntry)
	for rows.Next() {
		var entry TelegramRosterEntry
		if err := rows.Scan(&entry.ChatID, &entry.TelegramID, &entry.IsBot, &entry.LastStatus, &entry.FirstSeen, &entry.LastSeen); err != nil {
			return nil, err
		}
		out[telegramRosterKey(entry.ChatID, entry.TelegramID)] = entry
	}
	return out, rows.Err()
}

func (s *Store) replaceTelegramRosterLocked(entries map[string]TelegramRosterEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `TRUNCATE TABLE twilight_telegram_roster`); err != nil {
		return err
	}
	rows := make([]TelegramRosterEntry, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, entry)
	}
	if err := upsertTelegramRosterEntries(ctx, tx, rows); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.clearTelegramRosterCache()
	return nil
}

// migrateLegacyTelegramRoster moves the historical JSONB map into its narrow
// table before removing the in-memory copy. Repeated startup by multiple
// processes is safe because the merge is idempotent and keeps earliest/latest
// timestamps monotonically.
func (s *Store) migrateLegacyTelegramRoster(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.state.TelegramRoster) == 0 {
		s.state.TelegramRoster = nil
		return nil
	}
	entries := make([]TelegramRosterEntry, 0, len(s.state.TelegramRoster))
	for _, entry := range s.state.TelegramRoster {
		entries = append(entries, entry)
	}
	if err := upsertTelegramRosterEntries(ctx, s.db, entries); err != nil {
		return err
	}
	return s.mutateAndSaveLocked(func() error {
		s.state.TelegramRoster = nil
		return nil
	})
}
