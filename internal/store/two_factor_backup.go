package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/prejudice-studio/twilight/internal/security"
)

func snapshotTwoFactors(ctx context.Context, tx *sql.Tx) (map[int64]TwoFactorAccount, error) {
	rows, err := tx.QueryContext(ctx, `SELECT data FROM twilight_two_factor_accounts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[int64]TwoFactorAccount{}
	for rows.Next() {
		var raw []byte
		var a TwoFactorAccount
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
		result[a.UID] = a
	}
	return result, rows.Err()
}

func ValidateTwoFactorBackup(state State, configuredKey ...string) error {
	for uid, a := range state.TwoFactorAccounts {
		if uid <= 0 || a.UID != uid || a.Version == "" || state.Users[uid].UID != uid || a.EnabledAt < 0 || len(a.Recovery) > 10 {
			return fmt.Errorf("invalid two-factor backup")
		}
		if a.EnabledAt > 0 {
			key, err := security.TwoFactorKey(configuredKey...)
			if err != nil {
				return err
			}
			secret, err := security.OpenTOTP(key, uid, a.Secret)
			if err != nil {
				return err
			}
			if _, err = security.TOTPCode(secret, time.Now()); err != nil {
				return err
			}
			for _, hash := range a.Recovery {
				if !validTelegramLinkHex(hash, 32) {
					return fmt.Errorf("invalid two-factor recovery digest")
				}
			}
		} else if a.Secret != "" || len(a.Recovery) > 0 {
			return fmt.Errorf("invalid disabled two-factor backup")
		}
	}
	return nil
}

func restoreTwoFactors(ctx context.Context, tx *sql.Tx, accounts map[int64]TwoFactorAccount) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM twilight_two_factor_requests; DELETE FROM twilight_two_factor_accounts; DELETE FROM twilight_sessions`); err != nil {
		return err
	}
	for _, a := range accounts {
		var err error
		a.Version, err = security.RandomHex(16)
		if err != nil {
			return err
		}
		if err = saveTwoFactorAccount(ctx, tx, a); err != nil {
			return err
		}
	}
	return nil
}
