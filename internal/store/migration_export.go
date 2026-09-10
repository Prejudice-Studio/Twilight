package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/prejudice-studio/twilight/internal/migration"
)

const migrationExportTimeout = 5 * time.Minute

type migrationTelegramRuntime struct {
	UpdateOffset int64 `json:"update_offset"`
}

type migrationPlaybackRecord struct {
	ID int64 `json:"id"`
	PlaybackRecord
	CreatedAt time.Time `json:"created_at"`
}

// ExportMigrationFiles reads all persistent Twilight business data from one
// PostgreSQL snapshot. The result is intentionally a list of validated,
// namespaced files; archive construction and password protection stay in the
// independent migration package.
//
// Active sessions are deliberately excluded. They are short-lived credentials
// and must never be copied to another instance. The playback compatibility
// slice remains in state.json for older readers, while the complete dedicated
// table is exported separately as data/playback-records.json.
func (s *Store) ExportMigrationFiles(ctx context.Context) ([]migration.InputFile, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store database is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, migrationExportTimeout)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	state, err := migrationStateTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	runtimeLogs, err := migrationRuntimeLogsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	auditLogs, err := migrationAuditLogsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	roster, err := migrationTelegramRosterTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	telegramRuntime, err := migrationTelegramRuntimeTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	playbackRecords, err := migrationPlaybackRecordsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	trustedEvents, err := migrationTrustedPlaybackEventsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	trustedSegments, err := migrationTrustedPlaybackSegmentsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	trustedDaily, err := migrationTrustedPlaybackDailyTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// These tables are exported as their own files. Removing the compatibility
	// fields prevents a new importer from restoring the same row set twice.
	state.RuntimeLogs = nil
	state.AuditLogs = nil
	state.TelegramRoster = nil
	state.TelegramBotOffset = 0

	stateData, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	files := []migration.InputFile{
		migrationJSONFile("data/state.json", stateData),
	}
	additional := []struct {
		name  string
		value any
	}{
		{name: "data/runtime-logs.json", value: runtimeLogs},
		{name: "data/audit-logs.json", value: auditLogs},
		{name: "data/telegram-roster.json", value: roster},
		{name: "data/telegram-runtime.json", value: telegramRuntime},
		{name: "data/playback-records.json", value: playbackRecords},
		{name: "data/trusted-playback-events.json", value: trustedEvents},
		{name: "data/trusted-playback-segments.json", value: trustedSegments},
		{name: "data/trusted-playback-daily.json", value: trustedDaily},
	}
	for _, item := range additional {
		data, marshalErr := json.Marshal(item.value)
		if marshalErr != nil {
			return nil, marshalErr
		}
		files = append(files, migrationJSONFile(item.name, data))
	}
	return files, nil
}

func migrationJSONFile(name string, data []byte) migration.InputFile {
	return migration.InputFile{Path: name, Kind: "data", ContentType: "application/json", Data: data}
}

func migrationStateTx(ctx context.Context, tx *sql.Tx) (State, error) {
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT state FROM twilight_state WHERE id = 1`).Scan(&raw)
	if err == sql.ErrNoRows {
		state := emptyState()
		return state, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &state); err != nil {
			return State{}, err
		}
	}
	state.ensure()
	return state, nil
}

func migrationRuntimeLogsTx(ctx context.Context, tx *sql.Tx) ([]RuntimeLogEntry, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, time, level, message, COALESCE(attrs, '{}'::jsonb)::text
FROM twilight_runtime_logs ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]RuntimeLogEntry, 0)
	for rows.Next() {
		var entry RuntimeLogEntry
		var attrs string
		if err := rows.Scan(&entry.ID, &entry.Time, &entry.Level, &entry.Message, &attrs); err != nil {
			return nil, err
		}
		if attrs != "" && attrs != "{}" {
			if err := json.Unmarshal([]byte(attrs), &entry.Attrs); err != nil {
				return nil, err
			}
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func migrationAuditLogsTx(ctx context.Context, tx *sql.Tx) ([]AuditLog, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, uid, username, action, category, source, method, target_uid,
       COALESCE(detail, '{}'::jsonb)::text, ip, created_at
FROM twilight_audit_logs ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AuditLog, 0)
	for rows.Next() {
		entry, scanErr := scanAuditLog(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func migrationTelegramRosterTx(ctx context.Context, tx *sql.Tx) (map[string]TelegramRosterEntry, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT chat_id, telegram_id, is_bot, last_status, first_seen, last_seen
FROM twilight_telegram_roster ORDER BY chat_id ASC, telegram_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]TelegramRosterEntry)
	for rows.Next() {
		var entry TelegramRosterEntry
		if err := rows.Scan(&entry.ChatID, &entry.TelegramID, &entry.IsBot, &entry.LastStatus, &entry.FirstSeen, &entry.LastSeen); err != nil {
			return nil, err
		}
		result[telegramRosterKey(entry.ChatID, entry.TelegramID)] = entry
	}
	return result, rows.Err()
}

func migrationTelegramRuntimeTx(ctx context.Context, tx *sql.Tx) (migrationTelegramRuntime, error) {
	var runtime migrationTelegramRuntime
	err := tx.QueryRowContext(ctx, `SELECT update_offset FROM twilight_telegram_runtime WHERE id = 1`).Scan(&runtime.UpdateOffset)
	if err == sql.ErrNoRows {
		return runtime, nil
	}
	return runtime, err
}

func migrationPlaybackRecordsTx(ctx context.Context, tx *sql.Tx) ([]migrationPlaybackRecord, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, uid, item_id, title, series_name, media_type, index_number, duration, played_at, created_at
FROM twilight_playback_records ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]migrationPlaybackRecord, 0)
	for rows.Next() {
		var entry migrationPlaybackRecord
		if err := rows.Scan(&entry.ID, &entry.UID, &entry.ItemID, &entry.Title, &entry.SeriesName, &entry.MediaType, &entry.IndexNumber, &entry.Duration, &entry.PlayedAt, &entry.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func migrationTrustedPlaybackEventsTx(ctx context.Context, tx *sql.Tx) ([]migrationTrustedPlaybackEvent, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT uid, event_id, playback_id, device_id, item_id, title, series_name,
       media_type, event_type, event_at, received_at, sequence, time_zone,
       source, payload_hash, applied_seconds, finalized, processed_at
FROM twilight_playback_events ORDER BY uid ASC, event_at ASC, event_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]migrationTrustedPlaybackEvent, 0)
	for rows.Next() {
		var entry migrationTrustedPlaybackEvent
		if err := rows.Scan(
			&entry.UID, &entry.EventID, &entry.PlaybackID, &entry.DeviceID, &entry.ItemID,
			&entry.Title, &entry.SeriesName, &entry.MediaType, &entry.EventType,
			&entry.EventAt, &entry.ReceivedAt, &entry.Sequence, &entry.TimeZone,
			&entry.Source, &entry.PayloadHash, &entry.AppliedSeconds, &entry.Finalized,
			&entry.ProcessedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func migrationTrustedPlaybackSegmentsTx(ctx context.Context, tx *sql.Tx) ([]migrationTrustedPlaybackSegment, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT uid, playback_id, device_id, item_id, title, series_name, media_type,
       started_at, last_at, ended_at, duration, status, last_sequence,
       time_zone, updated_at
FROM twilight_playback_segments ORDER BY uid ASC, updated_at ASC, playback_id ASC, device_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]migrationTrustedPlaybackSegment, 0)
	for rows.Next() {
		var entry migrationTrustedPlaybackSegment
		if err := rows.Scan(
			&entry.UID, &entry.PlaybackID, &entry.DeviceID, &entry.ItemID, &entry.Title,
			&entry.SeriesName, &entry.MediaType, &entry.StartedAt, &entry.LastAt,
			&entry.EndedAt, &entry.Duration, &entry.Status, &entry.LastSequence,
			&entry.TimeZone, &entry.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func migrationTrustedPlaybackDailyTx(ctx context.Context, tx *sql.Tx) ([]migrationTrustedPlaybackDaily, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT uid, day, time_zone, seconds, updated_at
FROM twilight_playback_daily ORDER BY uid ASC, day ASC, time_zone ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]migrationTrustedPlaybackDaily, 0)
	for rows.Next() {
		var entry migrationTrustedPlaybackDaily
		if err := rows.Scan(&entry.UID, &entry.Day, &entry.TimeZone, &entry.Seconds, &entry.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}
