package store

import (
	"context"
	"database/sql"
	"time"
)

// TelegramIdentityHistory 记录用户 Telegram 身份的历史变更
type TelegramIdentityHistory struct {
	ID               int64
	UID              int64
	TelegramID       int64
	TelegramUsername string
	ChangeType       string
	RecordedAt       time.Time
	RecordedUnix     int64
}

// RecordTelegramIdentity 记录用户的 Telegram 身份历史
func (s *Store) RecordTelegramIdentity(ctx context.Context, uid, telegramID int64, username, changeType string) error {
	if s.db == nil {
		return nil
	}
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO twilight_telegram_identity_history (uid, telegram_id, telegram_username, change_type, recorded_at, recorded_unix)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, uid, telegramID, username, changeType, now, now.Unix())
	return err
}

// GetTelegramIdentityHistory 获取用户的 Telegram 身份历史记录
func (s *Store) GetTelegramIdentityHistory(ctx context.Context, uid int64, limit int) ([]TelegramIdentityHistory, error) {
	if s.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, uid, telegram_id, telegram_username, change_type, recorded_at, recorded_unix
		FROM twilight_telegram_identity_history
		WHERE uid = $1
		ORDER BY recorded_at DESC
		LIMIT $2
	`, uid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []TelegramIdentityHistory
	for rows.Next() {
		var h TelegramIdentityHistory
		if err := rows.Scan(&h.ID, &h.UID, &h.TelegramID, &h.TelegramUsername, &h.ChangeType, &h.RecordedAt, &h.RecordedUnix); err != nil {
			return nil, err
		}
		history = append(history, h)
	}
	return history, rows.Err()
}

// GetTelegramIdentityHistoryByTelegramID 根据 TelegramID 查询历史记录
func (s *Store) GetTelegramIdentityHistoryByTelegramID(ctx context.Context, telegramID int64, limit int) ([]TelegramIdentityHistory, error) {
	if s.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, uid, telegram_id, telegram_username, change_type, recorded_at, recorded_unix
		FROM twilight_telegram_identity_history
		WHERE telegram_id = $1
		ORDER BY recorded_at DESC
		LIMIT $2
	`, telegramID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []TelegramIdentityHistory
	for rows.Next() {
		var h TelegramIdentityHistory
		if err := rows.Scan(&h.ID, &h.UID, &h.TelegramID, &h.TelegramUsername, &h.ChangeType, &h.RecordedAt, &h.RecordedUnix); err != nil {
			return nil, err
		}
		history = append(history, h)
	}
	return history, rows.Err()
}

// CleanupOldTelegramIdentityHistory 清理旧的历史记录（保留最近 N 天）
func (s *Store) CleanupOldTelegramIdentityHistory(ctx context.Context, retentionDays int) (int64, error) {
	if s.db == nil {
		return 0, nil
	}
	if retentionDays <= 0 {
		retentionDays = 365
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM twilight_telegram_identity_history
		WHERE recorded_unix < $1
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetLatestTelegramIdentity 获取用户最新的 Telegram 身份记录
func (s *Store) GetLatestTelegramIdentity(ctx context.Context, uid int64) (*TelegramIdentityHistory, error) {
	if s.db == nil {
		return nil, nil
	}
	var h TelegramIdentityHistory
	err := s.db.QueryRowContext(ctx, `
		SELECT id, uid, telegram_id, telegram_username, change_type, recorded_at, recorded_unix
		FROM twilight_telegram_identity_history
		WHERE uid = $1
		ORDER BY recorded_at DESC
		LIMIT 1
	`, uid).Scan(&h.ID, &h.UID, &h.TelegramID, &h.TelegramUsername, &h.ChangeType, &h.RecordedAt, &h.RecordedUnix)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &h, nil
}
