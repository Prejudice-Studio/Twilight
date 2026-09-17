package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/prejudice-studio/twilight/internal/playback"
)

const (
	trustedPlaybackDBTimeout     = 10 * time.Second
	trustedPlaybackUnknownDevice = "unknown"
	maxTrustedPlaybackEventID    = 128
	maxTrustedPlaybackPlaybackID = 256
	maxTrustedPlaybackDeviceID   = 256
	maxTrustedPlaybackItemID     = 256
	maxTrustedPlaybackText       = 1024
	maxTrustedPlaybackSource     = 32
	maxTrustedPlaybackDay        = 10
	maxTrustedPlaybackMediaType  = 32
	maxTrustedPlaybackTimeZone   = 128
)

type TrustedPlaybackDaily struct {
	UID      int64  `json:"uid"`
	Day      string `json:"day"`
	TimeZone string `json:"time_zone"`
	Seconds  int64  `json:"seconds"`
}

type TrustedPlaybackSummary struct {
	Events          int64 `json:"events"`
	Seconds         int64 `json:"seconds"`
	ActiveSegments  int64 `json:"active_segments"`
	FinalizedEvents int64 `json:"finalized_events"`
}

type TrustedPlaybackResult struct {
	Duplicate    bool
	AddedSeconds int64
	Finalized    bool
	Segment      playback.Segment
	Buckets      []playback.DailyBucket
}

// ApplyTrustedPlaybackEvent persists one authenticated playback event and
// applies the domain state machine in the same PostgreSQL transaction.
func (s *Store) ApplyTrustedPlaybackEvent(ctx context.Context, event playback.Event, source string) (TrustedPlaybackResult, error) {
	return s.applyTrustedPlaybackEvent(ctx, event, source, time.Now().Unix())
}

func (s *Store) applyTrustedPlaybackEvent(ctx context.Context, event playback.Event, source string, now int64) (TrustedPlaybackResult, error) {
	if s == nil || s.db == nil {
		return TrustedPlaybackResult{}, errors.New("store database is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	normalized, source, err := normalizeTrustedPlaybackEvent(event, source, now)
	if err != nil {
		return TrustedPlaybackResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, trustedPlaybackDBTimeout)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TrustedPlaybackResult{}, err
	}
	defer tx.Rollback()

	inserted, err := insertTrustedPlaybackEvent(ctx, tx, normalized, source, now)
	if err != nil {
		return TrustedPlaybackResult{}, err
	}
	if !inserted {
		var addedSeconds int64
		var finalized bool
		if err := tx.QueryRowContext(ctx, `
SELECT applied_seconds, finalized
FROM twilight_playback_events
WHERE uid = $1 AND event_id = $2`, normalized.UID, normalized.EventID).Scan(&addedSeconds, &finalized); err != nil {
			return TrustedPlaybackResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return TrustedPlaybackResult{}, err
		}
		return TrustedPlaybackResult{Duplicate: true, AddedSeconds: addedSeconds, Finalized: finalized}, nil
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO twilight_playback_segments
	(uid, playback_id, device_id, item_id, title, series_name, media_type, time_zone, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (uid, playback_id, device_id) DO NOTHING`,
		normalized.UID, normalized.PlaybackID, normalized.DeviceID, normalized.ItemID,
		normalized.Title, normalized.SeriesName, normalized.MediaType, normalized.TimeZone, now); err != nil {
		return TrustedPlaybackResult{}, err
	}

	before, err := loadTrustedPlaybackSegment(ctx, tx, normalized.UID, normalized.PlaybackID, normalized.DeviceID)
	if err != nil {
		return TrustedPlaybackResult{}, err
	}
	segment := before
	applied, err := playback.Apply(&segment, normalized, now)
	if err != nil {
		return TrustedPlaybackResult{}, err
	}
	if segment != before {
		if err := saveTrustedPlaybackSegment(ctx, tx, segment, now); err != nil {
			return TrustedPlaybackResult{}, err
		}
	}
	for _, bucket := range applied.Buckets {
		if err := upsertTrustedPlaybackDaily(ctx, tx, bucket, now); err != nil {
			return TrustedPlaybackResult{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE twilight_playback_events
SET applied_seconds = $3, finalized = $4, processed_at = $5
WHERE uid = $1 AND event_id = $2`,
		normalized.UID, normalized.EventID, applied.AddedSeconds, applied.Finalized, now); err != nil {
		return TrustedPlaybackResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return TrustedPlaybackResult{}, err
	}
	return TrustedPlaybackResult{
		AddedSeconds: applied.AddedSeconds,
		Finalized:    applied.Finalized,
		Segment:      segment,
		Buckets:      applied.Buckets,
	}, nil
}

func normalizeTrustedPlaybackEvent(event playback.Event, source string, now int64) (playback.Event, string, error) {
	event.EventID = strings.TrimSpace(event.EventID)
	event.PlaybackID = strings.TrimSpace(event.PlaybackID)
	event.DeviceID = strings.TrimSpace(event.DeviceID)
	event.ItemID = strings.TrimSpace(event.ItemID)
	event.Title = strings.TrimSpace(event.Title)
	event.SeriesName = strings.TrimSpace(event.SeriesName)
	event.MediaType = strings.ToLower(strings.TrimSpace(event.MediaType))
	source = strings.TrimSpace(source)
	source = strings.ToLower(source)
	if event.DeviceID == "" {
		event.DeviceID = trustedPlaybackUnknownDevice
	}
	if source == "" {
		source = "client"
	}
	if event.UID <= 0 || event.EventID == "" || event.PlaybackID == "" {
		return playback.Event{}, "", ErrInvalid
	}
	if len(event.EventID) > maxTrustedPlaybackEventID ||
		len(event.PlaybackID) > maxTrustedPlaybackPlaybackID ||
		len(event.DeviceID) > maxTrustedPlaybackDeviceID ||
		len(event.ItemID) > maxTrustedPlaybackItemID ||
		len(event.Title) > maxTrustedPlaybackText ||
		len(event.SeriesName) > maxTrustedPlaybackText ||
		len(event.MediaType) > maxTrustedPlaybackMediaType ||
		len(source) > maxTrustedPlaybackSource {
		return playback.Event{}, "", ErrInvalid
	}
	for _, value := range []string{event.EventID, event.PlaybackID, event.DeviceID, event.ItemID, event.Title, event.SeriesName, event.MediaType, source} {
		for _, r := range value {
			if unicode.IsControl(r) {
				return playback.Event{}, "", ErrInvalid
			}
		}
	}
	if !validTrustedPlaybackToken(event.MediaType, maxTrustedPlaybackMediaType) ||
		!validTrustedPlaybackToken(source, maxTrustedPlaybackSource) {
		return playback.Event{}, "", ErrInvalid
	}
	event.ReceivedAt = now
	if event.TimeZone == "" {
		event.TimeZone = "UTC"
	}
	event.TimeZone = strings.TrimSpace(event.TimeZone)
	if len(event.TimeZone) > maxTrustedPlaybackTimeZone {
		return playback.Event{}, "", ErrInvalid
	}
	for _, r := range event.TimeZone {
		if unicode.IsControl(r) {
			return playback.Event{}, "", ErrInvalid
		}
	}
	if _, err := time.LoadLocation(event.TimeZone); err != nil {
		return playback.Event{}, "", ErrInvalid
	}
	switch source {
	case "client", "emby", "server", "scheduler":
	default:
		return playback.Event{}, "", ErrInvalid
	}
	event.At = playback.CanonicalEventTime(event, now)
	return event, source, nil
}

func validTrustedPlaybackToken(value string, max int) bool {
	if value == "" {
		return true
	}
	for i, r := range value {
		if i == 0 {
			if !(r >= 'a' && r <= 'z') {
				return false
			}
			continue
		}
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == ':') {
			return false
		}
	}
	return len(value) <= max
}

func trustedPlaybackPayloadHash(event playback.Event, source string) string {
	payload, _ := json.Marshal(struct {
		UID        int64
		EventID    string
		PlaybackID string
		DeviceID   string
		ItemID     string
		Title      string
		SeriesName string
		MediaType  string
		Type       playback.EventType
		At         int64
		Sequence   int64
		TimeZone   string
		Source     string
	}{
		event.UID, event.EventID, event.PlaybackID, event.DeviceID, event.ItemID,
		event.Title, event.SeriesName, event.MediaType, event.Type, event.At,
		event.Sequence, event.TimeZone, source,
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func insertTrustedPlaybackEvent(ctx context.Context, tx *sql.Tx, event playback.Event, source string, now int64) (bool, error) {
	payloadHash := trustedPlaybackPayloadHash(event, source)
	result, err := tx.ExecContext(ctx, `
INSERT INTO twilight_playback_events
	(uid, event_id, playback_id, device_id, item_id, title, series_name, media_type,
	 event_type, event_at, received_at, sequence, time_zone, source, payload_hash, processed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
ON CONFLICT (uid, event_id) DO NOTHING`,
		event.UID, event.EventID, event.PlaybackID, event.DeviceID, event.ItemID, event.Title,
		event.SeriesName, event.MediaType, event.Type, event.At, event.ReceivedAt, event.Sequence,
		event.TimeZone, source, payloadHash, now)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows > 0 {
		return rows > 0, err
	}
	var storedHash string
	var stored playback.Event
	var storedSource string
	if err := tx.QueryRowContext(ctx, `
SELECT uid, event_id, playback_id, device_id, item_id, title, series_name, media_type,
       event_type, event_at, received_at, sequence, time_zone, source, payload_hash
FROM twilight_playback_events
WHERE uid = $1 AND event_id = $2
FOR UPDATE`, event.UID, event.EventID).Scan(
		&stored.UID, &stored.EventID, &stored.PlaybackID, &stored.DeviceID, &stored.ItemID,
		&stored.Title, &stored.SeriesName, &stored.MediaType, &stored.Type, &stored.At,
		&stored.ReceivedAt, &stored.Sequence, &stored.TimeZone, &storedSource, &storedHash); err != nil {
		return false, err
	}
	if storedHash == "" {
		storedHash = trustedPlaybackPayloadHash(stored, storedSource)
		if _, err := tx.ExecContext(ctx, `UPDATE twilight_playback_events SET payload_hash = $3 WHERE uid = $1 AND event_id = $2 AND payload_hash = ''`, event.UID, event.EventID, storedHash); err != nil {
			return false, err
		}
	}
	if storedHash != payloadHash {
		return false, fmt.Errorf("%w: playback event payload differs", ErrConflict)
	}
	return false, nil
}

func loadTrustedPlaybackSegment(ctx context.Context, tx *sql.Tx, uid int64, playbackID, deviceID string) (playback.Segment, error) {
	var segment playback.Segment
	err := tx.QueryRowContext(ctx, `
SELECT uid, playback_id, device_id, item_id, title, series_name, media_type,
       started_at, last_at, ended_at, duration, status, last_sequence, time_zone
FROM twilight_playback_segments
WHERE uid = $1 AND playback_id = $2 AND device_id = $3
FOR UPDATE`, uid, playbackID, deviceID).Scan(
		&segment.UID, &segment.PlaybackID, &segment.DeviceID, &segment.ItemID,
		&segment.Title, &segment.SeriesName, &segment.MediaType, &segment.StartedAt,
		&segment.LastAt, &segment.EndedAt, &segment.Duration, &segment.Status,
		&segment.LastSequence, &segment.TimeZone)
	return segment, err
}

func saveTrustedPlaybackSegment(ctx context.Context, tx *sql.Tx, segment playback.Segment, now int64) error {
	_, err := tx.ExecContext(ctx, `
UPDATE twilight_playback_segments
SET item_id = $4, title = $5, series_name = $6, media_type = $7,
    started_at = $8, last_at = $9, ended_at = $10, duration = $11,
    status = $12, last_sequence = $13, time_zone = $14, updated_at = $15
WHERE uid = $1 AND playback_id = $2 AND device_id = $3`,
		segment.UID, segment.PlaybackID, segment.DeviceID, segment.ItemID, segment.Title,
		segment.SeriesName, segment.MediaType, segment.StartedAt, segment.LastAt, segment.EndedAt,
		segment.Duration, segment.Status, segment.LastSequence, segment.TimeZone, now)
	return err
}

func upsertTrustedPlaybackDaily(ctx context.Context, tx *sql.Tx, bucket playback.DailyBucket, now int64) error {
	if bucket.UID <= 0 || bucket.Day == "" || len(bucket.Day) > maxTrustedPlaybackDay || bucket.Seconds <= 0 {
		return ErrInvalid
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO twilight_playback_daily (uid, day, time_zone, seconds, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (uid, day, time_zone) DO UPDATE
SET seconds = twilight_playback_daily.seconds + EXCLUDED.seconds,
    updated_at = EXCLUDED.updated_at`,
		bucket.UID, bucket.Day, bucket.TimeZone, bucket.Seconds, now)
	return err
}

func (s *Store) TrustedPlaybackDaily(ctx context.Context, uid int64, fromDay, toDay string, limit int) ([]TrustedPlaybackDaily, error) {
	if s == nil || s.db == nil || uid <= 0 {
		return nil, ErrInvalid
	}
	fromDay, toDay, err := normalizeTrustedPlaybackDayRange(fromDay, toDay)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 366 {
		limit = 366
	}
	clauses := []string{"uid = $1"}
	args := []any{uid}
	if fromDay != "" {
		clauses = append(clauses, fmt.Sprintf("day >= $%d", len(args)+1))
		args = append(args, fromDay)
	}
	if toDay != "" {
		clauses = append(clauses, fmt.Sprintf("day <= $%d", len(args)+1))
		args = append(args, toDay)
	}
	args = append(args, limit)
	query := `SELECT uid, day, time_zone, seconds FROM twilight_playback_daily WHERE ` +
		strings.Join(clauses, " AND ") + fmt.Sprintf(" ORDER BY day DESC, time_zone ASC LIMIT $%d", len(args))
	ctx, cancel := context.WithTimeout(ctxOrBackground(ctx), trustedPlaybackDBTimeout)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TrustedPlaybackDaily, 0, limit)
	for rows.Next() {
		var item TrustedPlaybackDaily
		if err := rows.Scan(&item.UID, &item.Day, &item.TimeZone, &item.Seconds); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) TrustedPlaybackSummary(ctx context.Context, uid int64, fromDay, toDay string) (TrustedPlaybackSummary, error) {
	if s == nil || s.db == nil || uid <= 0 {
		return TrustedPlaybackSummary{}, ErrInvalid
	}
	fromDay, toDay, err := normalizeTrustedPlaybackDayRange(fromDay, toDay)
	if err != nil {
		return TrustedPlaybackSummary{}, err
	}
	ctx, cancel := context.WithTimeout(ctxOrBackground(ctx), trustedPlaybackDBTimeout)
	defer cancel()
	var summary TrustedPlaybackSummary
	eventDay := "to_char(to_timestamp(event_at) AT TIME ZONE time_zone, 'YYYY-MM-DD')"
	segmentDay := "to_char(to_timestamp(CASE WHEN last_at > 0 THEN last_at ELSE started_at END) AT TIME ZONE time_zone, 'YYYY-MM-DD')"
	err = s.db.QueryRowContext(ctx, `
SELECT
	(SELECT COUNT(*) FROM twilight_playback_events WHERE uid = $1 AND ($2 = '' OR `+eventDay+` >= $2) AND ($3 = '' OR `+eventDay+` <= $3)),
	(SELECT COALESCE(SUM(seconds), 0) FROM twilight_playback_daily WHERE uid = $1 AND ($2 = '' OR day >= $2) AND ($3 = '' OR day <= $3)),
	(SELECT COUNT(*) FROM twilight_playback_segments WHERE uid = $1 AND status IN ('started', 'playing', 'paused') AND ($2 = '' OR `+segmentDay+` >= $2) AND ($3 = '' OR `+segmentDay+` <= $3)),
	(SELECT COUNT(*) FROM twilight_playback_events WHERE uid = $1 AND finalized AND ($2 = '' OR `+eventDay+` >= $2) AND ($3 = '' OR `+eventDay+` <= $3))`, uid, fromDay, toDay).Scan(
		&summary.Events, &summary.Seconds, &summary.ActiveSegments, &summary.FinalizedEvents)
	return summary, err
}

func normalizeTrustedPlaybackDayRange(fromDay, toDay string) (string, string, error) {
	fromDay = strings.TrimSpace(fromDay)
	toDay = strings.TrimSpace(toDay)
	for _, day := range []string{fromDay, toDay} {
		if day == "" {
			continue
		}
		parsed, err := time.Parse("2006-01-02", day)
		if err != nil || parsed.Format("2006-01-02") != day {
			return "", "", ErrInvalid
		}
	}
	if fromDay != "" && toDay != "" && fromDay > toDay {
		return "", "", ErrInvalid
	}
	return fromDay, toDay, nil
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
