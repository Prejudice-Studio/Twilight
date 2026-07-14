package api

import (
	"context"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) deleteLocalUser(ctx context.Context, u store.User) error {
	if err := a.store().DeleteUser(u.UID); err != nil {
		return err
	}
	a.cleanupDeletedUserTelegramResidue(u)
	if ctx != nil {
		a.sessions().DeleteUser(ctx, u.UID)
	}
	return nil
}

func (a *App) cleanupDeletedUserTelegramResidue(u store.User) int {
	if a.bindStatus == nil {
		return 0
	}
	return a.bindStatus.deleteBindCodesForUser(u.UID, u.TelegramID)
}

func (a *App) cleanupOrphanedUserBindCodes() int {
	if a.bindStatus == nil || a.store() == nil {
		return 0
	}
	return a.bindStatus.cleanupOrphanedUserBindCodes(func(uid int64) bool {
		_, ok := a.store().User(uid)
		return ok
	})
}
