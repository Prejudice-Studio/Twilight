package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/migration"
	"github.com/prejudice-studio/twilight/internal/playback"
)

const (
	migrationImportTimeout   = 5 * time.Minute
	maxImportedUsers         = 100000
	maxImportedPlaybackRows  = 1000000
	maxImportedRuntimeRows   = 1000000
	maxImportedAuditRows     = 1000000
	maxImportedRosterEntries = 1000000
)

var migrationDataFiles = []string{
	"data/state.json",
	"data/runtime-logs.json",
	"data/audit-logs.json",
	"data/telegram-roster.json",
	"data/telegram-runtime.json",
	"data/playback-records.json",
}

type migrationImportTelegramRuntime struct {
	UpdateOffset int64 `json:"update_offset"`
}

type migrationImportPlaybackRecord struct {
	ID int64 `json:"id"`
	PlaybackRecord
	CreatedAt time.Time `json:"created_at"`
}

type MigrationImportSummary struct {
	Users                   int `json:"users"`
	RuntimeLogs             int `json:"runtime_logs"`
	AuditLogs               int `json:"audit_logs"`
	RosterEntries           int `json:"telegram_roster"`
	PlaybackRecords         int `json:"playback_records"`
	TrustedPlaybackEvents   int `json:"trusted_playback_events"`
	TrustedPlaybackSegments int `json:"trusted_playback_segments"`
	TrustedPlaybackDaily    int `json:"trusted_playback_daily"`
}

// ImportMigrationArchive replaces Twilight's persistent business snapshot in
// one transaction. archive must have been returned by migration.Open; this
// method intentionally does not parse or write arbitrary archive paths.
//
// Session rows are never touched. They contain short-lived authentication
// credentials and must not survive a cross-instance data import.
func (s *Store) ImportMigrationArchive(ctx context.Context, archive migration.Archive) (MigrationImportSummary, error) {
	if s == nil || s.db == nil {
		return MigrationImportSummary{}, fmt.Errorf("store database is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, migrationImportTimeout)
	defer cancel()
	data, err := parseMigrationArchive(archive)
	if err != nil {
		return MigrationImportSummary{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return MigrationImportSummary{}, err
	}
	defer tx.Rollback()

	// Serialize with normal state writers before replacing the authoritative
	// JSONB row. The row lock is held until the dedicated tables are restored.
	var currentVersion int64
	lockErr := tx.QueryRowContext(ctx, `SELECT version FROM twilight_state WHERE id = 1 FOR UPDATE`).Scan(&currentVersion)
	if lockErr != nil && lockErr != sql.ErrNoRows {
		return MigrationImportSummary{}, lockErr
	}
	var nextVersion int64
	if lockErr == sql.ErrNoRows {
		nextVersion = 1
	} else {
		nextVersion = currentVersion + 1
	}

	state := data.state
	state.RuntimeLogs = nil
	state.AuditLogs = nil
	state.TelegramRoster = nil
	state.TelegramBotOffset = 0
	state.PlaybackRecords = playbackCompatibilityWindow(data.playbackRecords)
	state.NextRuntimeLogID = nextPositiveID(data.runtimeLogs, func(entry RuntimeLogEntry) int64 { return entry.ID })
	state.NextAuditLogID = nextPositiveID(data.auditLogs, func(entry AuditLog) int64 { return entry.ID })
	state.ensure()
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return MigrationImportSummary{}, err
	}

	var storedVersion int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO twilight_state (id, state, version, updated_at) VALUES (1, $1::jsonb, $2, now())
ON CONFLICT (id) DO UPDATE SET state = EXCLUDED.state, version = twilight_state.version + 1, updated_at = now()
RETURNING version`, string(stateBytes), nextVersion).Scan(&storedVersion)
	if err != nil {
		return MigrationImportSummary{}, err
	}

	if err := truncateMigrationTables(ctx, tx); err != nil {
		return MigrationImportSummary{}, err
	}
	if err := insertMigrationRuntimeLogs(ctx, tx, data.runtimeLogs); err != nil {
		return MigrationImportSummary{}, err
	}
	if err := insertMigrationAuditLogs(ctx, tx, data.auditLogs); err != nil {
		return MigrationImportSummary{}, err
	}
	if err := insertMigrationRoster(ctx, tx, data.roster); err != nil {
		return MigrationImportSummary{}, err
	}
	if err := insertMigrationRuntime(ctx, tx, data.telegramRuntime); err != nil {
		return MigrationImportSummary{}, err
	}
	if err := insertMigrationPlayback(ctx, tx, data.playbackRecords); err != nil {
		return MigrationImportSummary{}, err
	}
	if err := insertMigrationTrustedPlaybackEvents(ctx, tx, data.trustedEvents); err != nil {
		return MigrationImportSummary{}, err
	}
	if err := insertMigrationTrustedPlaybackSegments(ctx, tx, data.trustedSegments); err != nil {
		return MigrationImportSummary{}, err
	}
	if err := insertMigrationTrustedPlaybackDaily(ctx, tx, data.trustedDaily); err != nil {
		return MigrationImportSummary{}, err
	}
	if err := syncMigrationSequences(ctx, tx); err != nil {
		return MigrationImportSummary{}, err
	}
	if err := tx.Commit(); err != nil {
		return MigrationImportSummary{}, err
	}

	s.state = state
	s.stateVersion = storedVersion
	s.stateRaw = stateBytes
	s.rebuildUserIndexes()
	s.clearTelegramRosterCache()
	return MigrationImportSummary{
		Users: len(state.Users), RuntimeLogs: len(data.runtimeLogs), AuditLogs: len(data.auditLogs),
		RosterEntries: len(data.roster), PlaybackRecords: len(data.playbackRecords),
		TrustedPlaybackEvents: len(data.trustedEvents), TrustedPlaybackSegments: len(data.trustedSegments), TrustedPlaybackDaily: len(data.trustedDaily),
	}, nil
}

type parsedMigrationData struct {
	state           State
	runtimeLogs     []RuntimeLogEntry
	auditLogs       []AuditLog
	roster          map[string]TelegramRosterEntry
	telegramRuntime migrationImportTelegramRuntime
	playbackRecords []migrationImportPlaybackRecord
	trustedEvents   []migrationTrustedPlaybackEvent
	trustedSegments []migrationTrustedPlaybackSegment
	trustedDaily    []migrationTrustedPlaybackDaily
}

func parseMigrationArchive(archive migration.Archive) (parsedMigrationData, error) {
	files := archive.Files
	for _, name := range migrationDataFiles {
		if _, ok := files[name]; !ok {
			return parsedMigrationData{}, fmt.Errorf("migration archive missing %s", name)
		}
	}
	var result parsedMigrationData
	if err := decodeMigrationJSON(files["data/state.json"], &result.state); err != nil {
		return parsedMigrationData{}, fmt.Errorf("invalid migration state: %w", err)
	}
	if err := decodeMigrationJSON(files["data/runtime-logs.json"], &result.runtimeLogs); err != nil {
		return parsedMigrationData{}, fmt.Errorf("invalid runtime logs: %w", err)
	}
	if err := decodeMigrationJSON(files["data/audit-logs.json"], &result.auditLogs); err != nil {
		return parsedMigrationData{}, fmt.Errorf("invalid audit logs: %w", err)
	}
	if err := decodeMigrationJSON(files["data/telegram-roster.json"], &result.roster); err != nil {
		return parsedMigrationData{}, fmt.Errorf("invalid Telegram roster: %w", err)
	}
	if err := decodeMigrationJSON(files["data/telegram-runtime.json"], &result.telegramRuntime); err != nil {
		return parsedMigrationData{}, fmt.Errorf("invalid Telegram runtime: %w", err)
	}
	if err := decodeMigrationJSON(files["data/playback-records.json"], &result.playbackRecords); err != nil {
		return parsedMigrationData{}, fmt.Errorf("invalid playback records: %w", err)
	}
	if content, ok := files["data/trusted-playback-events.json"]; ok {
		if err := decodeMigrationJSON(content, &result.trustedEvents); err != nil {
			return parsedMigrationData{}, fmt.Errorf("invalid trusted playback events: %w", err)
		}
	}
	if content, ok := files["data/trusted-playback-segments.json"]; ok {
		if err := decodeMigrationJSON(content, &result.trustedSegments); err != nil {
			return parsedMigrationData{}, fmt.Errorf("invalid trusted playback segments: %w", err)
		}
	}
	if content, ok := files["data/trusted-playback-daily.json"]; ok {
		if err := decodeMigrationJSON(content, &result.trustedDaily); err != nil {
			return parsedMigrationData{}, fmt.Errorf("invalid trusted playback daily buckets: %w", err)
		}
	}
	if err := validateMigrationData(result); err != nil {
		return parsedMigrationData{}, err
	}
	result.state.ensure()
	return result, nil
}

func decodeMigrationJSON(data []byte, destination any) error {
	if len(data) == 0 {
		return fmt.Errorf("empty JSON file")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func validateMigrationData(data parsedMigrationData) error {
	if len(data.state.Users) > maxImportedUsers || len(data.runtimeLogs) > maxImportedRuntimeRows || len(data.auditLogs) > maxImportedAuditRows || len(data.roster) > maxImportedRosterEntries || len(data.playbackRecords) > maxImportedPlaybackRows || len(data.trustedEvents) > maxImportedPlaybackRows || len(data.trustedSegments) > maxImportedPlaybackRows || len(data.trustedDaily) > maxImportedPlaybackRows {
		return fmt.Errorf("migration data exceeds row limits")
	}
	if data.telegramRuntime.UpdateOffset < 0 {
		return fmt.Errorf("invalid Telegram update offset")
	}
	if data.state.RuntimeLogs != nil || data.state.AuditLogs != nil || data.state.TelegramRoster != nil || data.state.TelegramBotOffset != 0 {
		return fmt.Errorf("dedicated runtime data must not be embedded in state.json")
	}
	for uid, user := range data.state.Users {
		if uid <= 0 || user.UID != uid || strings.TrimSpace(user.Username) == "" {
			return fmt.Errorf("invalid user identity in migration state")
		}
	}
	seenPlayback := make(map[playbackKey]struct{}, len(data.playbackRecords))
	for _, record := range data.playbackRecords {
		if record.ID <= 0 || record.UID <= 0 || strings.TrimSpace(record.ItemID) == "" || record.PlayedAt <= 0 || record.Duration < 0 || record.CreatedAt.IsZero() {
			return fmt.Errorf("invalid playback record")
		}
		key := playbackKey{uid: record.UID, itemID: record.ItemID, playedAt: record.PlayedAt}
		if _, duplicate := seenPlayback[key]; duplicate {
			return fmt.Errorf("duplicate playback record")
		}
		seenPlayback[key] = struct{}{}
	}
	seenRoster := make(map[string]struct{}, len(data.roster))
	for _, entry := range data.roster {
		if strings.TrimSpace(entry.ChatID) == "" || entry.TelegramID <= 0 || entry.FirstSeen < 0 || entry.LastSeen < 0 {
			return fmt.Errorf("invalid Telegram roster entry")
		}
		key := telegramRosterKey(entry.ChatID, entry.TelegramID)
		if _, duplicate := seenRoster[key]; duplicate {
			return fmt.Errorf("duplicate Telegram roster entry")
		}
		seenRoster[key] = struct{}{}
	}
	for _, entry := range data.runtimeLogs {
		if entry.ID <= 0 || len(entry.Message) > 1<<20 {
			return fmt.Errorf("invalid runtime log entry")
		}
	}
	for _, entry := range data.auditLogs {
		if entry.ID <= 0 || entry.UID < 0 || entry.TargetUID < 0 || len(entry.Action) > 256 || len(entry.Category) > 128 {
			return fmt.Errorf("invalid audit log entry")
		}
	}
	if err := validateTrustedPlaybackMigrationData(data); err != nil {
		return err
	}
	return nil
}

func truncateMigrationTables(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `TRUNCATE TABLE twilight_runtime_logs, twilight_audit_logs, twilight_telegram_roster, twilight_telegram_runtime, twilight_playback_records, twilight_playback_events, twilight_playback_segments, twilight_playback_daily RESTART IDENTITY`)
	return err
}

func insertMigrationRuntimeLogs(ctx context.Context, tx *sql.Tx, entries []RuntimeLogEntry) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO twilight_runtime_logs (id, time, level, message, attrs) VALUES ($1, $2, $3, $4, $5::jsonb)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, entry := range entries {
		attrs, err := json.Marshal(entry.Attrs)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, entry.ID, entry.Time, entry.Level, entry.Message, string(attrs)); err != nil {
			return err
		}
	}
	return nil
}

func insertMigrationAuditLogs(ctx context.Context, tx *sql.Tx, entries []AuditLog) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO twilight_audit_logs (id, uid, username, action, category, source, method, target_uid, detail, ip, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, entry := range entries {
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

func insertMigrationRoster(ctx context.Context, tx *sql.Tx, entries map[string]TelegramRosterEntry) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO twilight_telegram_roster (chat_id, telegram_id, is_bot, last_status, first_seen, last_seen) VALUES ($1, $2, $3, $4, $5, $6)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, entry := range entries {
		if _, err := stmt.ExecContext(ctx, entry.ChatID, entry.TelegramID, entry.IsBot, entry.LastStatus, entry.FirstSeen, entry.LastSeen); err != nil {
			return err
		}
	}
	return nil
}

func insertMigrationRuntime(ctx context.Context, tx *sql.Tx, runtime migrationImportTelegramRuntime) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO twilight_telegram_runtime (id, update_offset, updated_at) VALUES (1, $1, now())`, runtime.UpdateOffset)
	return err
}

func insertMigrationPlayback(ctx context.Context, tx *sql.Tx, entries []migrationImportPlaybackRecord) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO twilight_playback_records (id, uid, item_id, title, series_name, media_type, index_number, duration, played_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, entry := range entries {
		if _, err := stmt.ExecContext(ctx, entry.ID, entry.UID, entry.ItemID, entry.Title, entry.SeriesName, entry.MediaType, entry.IndexNumber, entry.Duration, entry.PlayedAt, entry.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

func insertMigrationTrustedPlaybackEvents(ctx context.Context, tx *sql.Tx, entries []migrationTrustedPlaybackEvent) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO twilight_playback_events (uid, event_id, playback_id, device_id, item_id, title, series_name, media_type, event_type, event_at, received_at, sequence, time_zone, source, payload_hash, applied_seconds, finalized, processed_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, entry := range entries {
		hash := entry.PayloadHash
		if hash == "" {
			hash = migrationTrustedPlaybackPayloadHash(entry)
		}
		if _, err := stmt.ExecContext(ctx, entry.UID, entry.EventID, entry.PlaybackID, entry.DeviceID, entry.ItemID, entry.Title, entry.SeriesName, entry.MediaType, entry.EventType, entry.EventAt, entry.ReceivedAt, entry.Sequence, entry.TimeZone, entry.Source, hash, entry.AppliedSeconds, entry.Finalized, entry.ProcessedAt); err != nil {
			return err
		}
	}
	return nil
}

func insertMigrationTrustedPlaybackSegments(ctx context.Context, tx *sql.Tx, entries []migrationTrustedPlaybackSegment) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO twilight_playback_segments (uid, playback_id, device_id, item_id, title, series_name, media_type, started_at, last_at, ended_at, duration, status, last_sequence, time_zone, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, entry := range entries {
		if _, err := stmt.ExecContext(ctx, entry.UID, entry.PlaybackID, entry.DeviceID, entry.ItemID, entry.Title, entry.SeriesName, entry.MediaType, entry.StartedAt, entry.LastAt, entry.EndedAt, entry.Duration, entry.Status, entry.LastSequence, entry.TimeZone, entry.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func insertMigrationTrustedPlaybackDaily(ctx context.Context, tx *sql.Tx, entries []migrationTrustedPlaybackDaily) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO twilight_playback_daily (uid, day, time_zone, seconds, updated_at) VALUES ($1, $2, $3, $4, $5)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, entry := range entries {
		if _, err := stmt.ExecContext(ctx, entry.UID, entry.Day, entry.TimeZone, entry.Seconds, entry.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func syncMigrationSequences(ctx context.Context, tx *sql.Tx) error {
	for _, table := range []string{"twilight_runtime_logs", "twilight_audit_logs", "twilight_playback_records"} {
		query := `SELECT setval(pg_get_serial_sequence('` + table + `', 'id'), COALESCE((SELECT MAX(id) FROM ` + table + `), 1), (SELECT MAX(id) IS NOT NULL FROM ` + table + `))`
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

func playbackCompatibilityWindow(entries []migrationImportPlaybackRecord) []PlaybackRecord {
	if len(entries) == 0 {
		return []PlaybackRecord{}
	}
	limit := len(entries)
	if limit > maxStoredPlaybackRecords {
		limit = maxStoredPlaybackRecords
	}
	result := make([]PlaybackRecord, 0, limit)
	for i := len(entries) - 1; i >= 0 && len(result) < limit; i-- {
		result = append(result, entries[i].PlaybackRecord)
	}
	return result
}

func nextPositiveID[T any](entries []T, id func(T) int64) int64 {
	var maxID int64
	for _, entry := range entries {
		if value := id(entry); value > maxID {
			maxID = value
		}
	}
	return maxID + 1
}

func validateTrustedPlaybackMigrationData(data parsedMigrationData) error {
	knownUsers := data.state.Users
	eventKeys := make(map[string]struct{}, len(data.trustedEvents))
	segmentKeys := make(map[string]struct{}, len(data.trustedSegments))
	dailyKeys := make(map[string]struct{}, len(data.trustedDaily))
	for _, entry := range data.trustedEvents {
		if _, ok := knownUsers[entry.UID]; !ok || entry.UID <= 0 || strings.TrimSpace(entry.EventID) == "" || strings.TrimSpace(entry.PlaybackID) == "" || entry.EventAt <= 0 || entry.ReceivedAt <= 0 || entry.ProcessedAt <= 0 || entry.Sequence < 0 || entry.AppliedSeconds < 0 || entry.AppliedSeconds > playback.MaxSegmentSeconds || !validMigrationPlaybackEventType(entry.EventType) || !validMigrationPlaybackSource(entry.Source) || !validMigrationPlaybackTimeZone(entry.TimeZone) {
			return fmt.Errorf("invalid trusted playback event")
		}
		if err := validateMigrationPlaybackText(entry.EventID, 128); err != nil {
			return err
		}
		for _, value := range []struct {
			name  string
			value string
			limit int
		}{{"playback_id", entry.PlaybackID, 256}, {"device_id", entry.DeviceID, 256}, {"item_id", entry.ItemID, 256}, {"title", entry.Title, 1024}, {"series_name", entry.SeriesName, 1024}, {"media_type", entry.MediaType, 32}} {
			if err := validateMigrationPlaybackText(value.value, value.limit); err != nil {
				return fmt.Errorf("invalid trusted playback event %s: %w", value.name, err)
			}
		}
		expectedHash := migrationTrustedPlaybackPayloadHash(entry)
		if entry.PayloadHash != "" {
			decoded, err := hex.DecodeString(entry.PayloadHash)
			if err != nil || len(decoded) != 32 || !strings.EqualFold(entry.PayloadHash, expectedHash) {
				return fmt.Errorf("invalid trusted playback event payload hash")
			}
		}
		key := entry.EventID + "\x00" + fmt.Sprint(entry.UID)
		if _, exists := eventKeys[key]; exists {
			return fmt.Errorf("duplicate trusted playback event")
		}
		eventKeys[key] = struct{}{}
	}
	for _, entry := range data.trustedSegments {
		if _, ok := knownUsers[entry.UID]; !ok || entry.UID <= 0 || strings.TrimSpace(entry.PlaybackID) == "" || strings.TrimSpace(entry.DeviceID) == "" || entry.StartedAt <= 0 || entry.LastAt < entry.StartedAt || entry.EndedAt < 0 || entry.Duration < 0 || entry.Duration > playback.MaxSegmentSeconds || entry.LastSequence < 0 || !validMigrationPlaybackSegmentStatus(entry.Status) || !validMigrationPlaybackTimeZone(entry.TimeZone) {
			return fmt.Errorf("invalid trusted playback segment")
		}
		for _, value := range []struct {
			name  string
			value string
			limit int
		}{{"playback_id", entry.PlaybackID, 256}, {"device_id", entry.DeviceID, 256}, {"item_id", entry.ItemID, 256}, {"title", entry.Title, 1024}, {"series_name", entry.SeriesName, 1024}, {"media_type", entry.MediaType, 32}} {
			if err := validateMigrationPlaybackText(value.value, value.limit); err != nil {
				return fmt.Errorf("invalid trusted playback segment %s: %w", value.name, err)
			}
		}
		key := fmt.Sprintf("%d\x00%s\x00%s", entry.UID, entry.PlaybackID, entry.DeviceID)
		if _, exists := segmentKeys[key]; exists {
			return fmt.Errorf("duplicate trusted playback segment")
		}
		segmentKeys[key] = struct{}{}
	}
	for _, entry := range data.trustedDaily {
		if _, ok := knownUsers[entry.UID]; !ok || entry.UID <= 0 || entry.Seconds <= 0 || entry.UpdatedAt <= 0 || !validMigrationPlaybackDay(entry.Day) || !validMigrationPlaybackTimeZone(entry.TimeZone) {
			return fmt.Errorf("invalid trusted playback daily bucket")
		}
		key := fmt.Sprintf("%d\x00%s\x00%s", entry.UID, entry.Day, entry.TimeZone)
		if _, exists := dailyKeys[key]; exists {
			return fmt.Errorf("duplicate trusted playback daily bucket")
		}
		dailyKeys[key] = struct{}{}
	}
	return nil
}

func validateMigrationPlaybackText(value string, limit int) error {
	if len(value) > limit {
		return fmt.Errorf("trusted playback text exceeds limit")
	}
	for _, r := range value {
		if r == '\x00' || r == '\r' || r == '\n' || r == '\t' || r < 0x20 {
			return fmt.Errorf("trusted playback text contains control characters")
		}
	}
	return nil
}

func validMigrationPlaybackEventType(value string) bool {
	switch value {
	case string(playback.EventStarted), string(playback.EventPlaying), string(playback.EventPaused), string(playback.EventResumed), string(playback.EventStopped), string(playback.EventCompleted), string(playback.EventExpired):
		return true
	default:
		return false
	}
}

func validMigrationPlaybackSegmentStatus(value string) bool {
	switch value {
	case string(playback.EventPlaying), string(playback.EventPaused), string(playback.EventStopped), string(playback.EventCompleted), string(playback.EventExpired):
		return true
	default:
		return false
	}
}

func validMigrationPlaybackSource(value string) bool {
	switch value {
	case "client", "emby", "server", "scheduler":
		return true
	default:
		return false
	}
}

func validMigrationPlaybackTimeZone(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	_, err := time.LoadLocation(value)
	return err == nil
}

func validMigrationPlaybackDay(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func migrationTrustedPlaybackPayloadHash(entry migrationTrustedPlaybackEvent) string {
	return trustedPlaybackPayloadHash(playback.Event{
		UID: entry.UID, EventID: entry.EventID, PlaybackID: entry.PlaybackID,
		DeviceID: entry.DeviceID, ItemID: entry.ItemID, Title: entry.Title,
		SeriesName: entry.SeriesName, MediaType: entry.MediaType,
		Type: playback.EventType(entry.EventType), At: entry.EventAt,
		ReceivedAt: entry.ReceivedAt, Sequence: entry.Sequence, TimeZone: entry.TimeZone,
	}, entry.Source)
}
