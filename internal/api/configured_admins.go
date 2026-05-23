package api

import (
	"go.uber.org/zap"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) applyConfiguredAdmins() {
	if a.store == nil {
		return
	}
	uidSet := a.configuredAdminUIDSet()
	nameSet := a.configuredAdminUsernameSet()
	if len(uidSet) == 0 && len(nameSet) == 0 {
		return
	}
	for _, user := range a.store.ListUsers() {
		if !configuredAdminMatchSets(uidSet, nameSet, user.UID, user.Username) {
			continue
		}
		updated, err := a.store.UpdateUser(user.UID, func(u *store.User) error {
			u.Role = store.RoleAdmin
			u.Active = true
			return nil
		})
		if err == nil {
			zap.L().Info("configured administrator applied", zap.Int64("uid", updated.UID), zap.String("username", updated.Username))
		}
	}
}

func (a *App) configuredAdminMatch(uid int64, username string) bool {
	return configuredAdminMatchSets(a.configuredAdminUIDSet(), a.configuredAdminUsernameSet(), uid, username)
}

func configuredAdminMatchSets(uidSet map[int64]bool, nameSet map[string]bool, uid int64, username string) bool {
	if uid > 0 && uidSet[uid] {
		return true
	}
	username = strings.ToLower(strings.TrimSpace(username))
	return username != "" && nameSet[username]
}

func (a *App) configuredAdminUIDSet() map[int64]bool {
	uidSet := map[int64]bool{}
	for _, uid := range a.cfg.AdminUIDs {
		if uid > 0 {
			uidSet[uid] = true
		}
	}
	return uidSet
}

func (a *App) configuredAdminUsernameSet() map[string]bool {
	nameSet := map[string]bool{}
	for _, username := range a.cfg.AdminUsernames {
		username = strings.ToLower(strings.TrimSpace(username))
		if username != "" {
			nameSet[username] = true
		}
	}
	return nameSet
}
