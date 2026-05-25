package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleBatchDisableUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleBatchToggleUsers(w, r, false)
}

func (a *App) handleBatchEnableUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleBatchToggleUsers(w, r, true)
}

func (a *App) handleBatchToggleUsers(w http.ResponseWriter, r *http.Request, enable bool) {
	confirmPhrase := confirmBatchDisableUsers
	if enable {
		confirmPhrase = confirmBatchEnableUsers
	}
	_, uids, okPayload := requireBatchPayload(w, r, confirmPhrase, 200, "too many users in one batch")
	if !okPayload {
		return
	}
	result := batchResult(len(uids))
	for _, uid := range uniqueInt64s(uids) {
		target, okUser := a.store().User(uid)
		if !okUser {
			addBatchOutcome(result, uid, fmt.Errorf("user not found"))
			continue
		}
		if a.userIsProtected(target) {
			addBatchOutcome(result, uid, fmt.Errorf("cannot batch toggle protected account: %s", a.protectedUserReason(target)))
			continue
		}
		updated, err := a.store().UpdateUser(uid, func(u *store.User) error { u.Active = enable; return nil })
		if err == nil && updated.EmbyID != "" && a.cfg().EmbyURL != "" {
			if syncErr := a.embySetUserEnabled(r.Context(), updated.EmbyID, a.embyShouldEnableUser(updated)); syncErr != nil {
				err = syncErr
			}
		}
		addBatchOutcome(result, uid, err)
	}
	ok(w, "批量操作完成", result)
}

func (a *App) handleBatchRenewUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	uids := int64Slice(payload["uids"])
	days := intValue(payload, "days", 30)
	if days <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrBatchDaysInvalid, "days 必须大于 0")
		return
	}
	result := batchResult(len(uids))
	for _, uid := range uids {
		_, err := a.store().UpdateUser(uid, func(u *store.User) error {
			u.ExpiredAt = addDaysToExpiry(u.ExpiredAt, days, time.Now())
			return nil
		})
		addBatchOutcome(result, uid, err)
	}
	result["days"] = days
	ok(w, "批量续期完成", result)
}

func (a *App) handleBatchDeleteUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	payload, uids, okPayload := requireBatchPayload(w, r, confirmBatchDeleteUsers, 200, "too many users in one batch")
	if !okPayload {
		return
	}
	deleteEmby := boolValue(payload, "delete_emby", r.URL.Query().Get("delete_emby") != "false")
	result := batchResult(len(uids))
	for _, uid := range uniqueInt64s(uids) {
		if uid == current(r).User.UID {
			addBatchOutcome(result, uid, fmt.Errorf("cannot delete current admin"))
			continue
		}
		target, okUser := a.store().User(uid)
		if !okUser {
			addBatchOutcome(result, uid, fmt.Errorf("user not found"))
			continue
		}
		if a.userIsProtected(target) {
			addBatchOutcome(result, uid, fmt.Errorf("cannot batch delete protected account: %s", a.protectedUserReason(target)))
			continue
		}
		if deleteEmby && a.cfg().EmbyURL != "" {
			if target.EmbyID != "" {
				if err := a.embyDelete(r.Context(), "/Users/"+urlPathEscape(target.EmbyID)); err != nil {
					addBatchOutcome(result, uid, err)
					continue
				}
			}
		}
		addBatchOutcome(result, uid, a.store().DeleteUser(uid))
	}
	ok(w, "批量删除完成", result)
}

func (a *App) handleBatchLibrarySelfService(w http.ResponseWriter, r *http.Request, _ Params) {
	payload, uids, okPayload := requireBatchPayload(w, r, confirmBatchLibrarySelfService, 200, "too many users in one batch")
	if !okPayload {
		return
	}
	enabled := boolValue(payload, "enabled", true)
	result := batchResult(len(uids))
	for _, uid := range uniqueInt64s(uids) {
		_, err := a.store().UpdateUser(uid, func(u *store.User) error {
			u.LibrarySelfService = enabled
			return nil
		})
		addBatchOutcome(result, uid, err)
	}
	result["enabled"] = enabled
	ok(w, "library self-service updated", result)
}

func (a *App) handleBatchUserLibraries(w http.ResponseWriter, r *http.Request, _ Params) {
	payload, uids, okPayload := requireBatchPayload(w, r, confirmBatchUserLibraries, 100, "too many users in one library batch")
	if !okPayload {
		return
	}
	action := firstNonEmpty(stringValue(payload, "action"), "set")
	switch action {
	case "set", "show", "hide", "enable_all", "disable_all":
	default:
		failWithCode(w, http.StatusBadRequest, ErrBatchLibraryActionInvalid, "unsupported library action")
		return
	}
	ids := stringSlice(payload["library_ids"])
	names := normalizeLibraryNames(stringSlice(payload["library_names"]))
	enableAll := boolValue(payload, "enable_all", false)
	result := batchResult(len(uids))
	for _, uid := range uniqueInt64s(uids) {
		target, okUser := a.store().User(uid)
		if !okUser {
			addBatchOutcome(result, uid, fmt.Errorf("user not found"))
			continue
		}
		if target.EmbyID == "" {
			addBatchOutcome(result, uid, fmt.Errorf("user has no Emby account"))
			continue
		}
		addBatchOutcome(result, uid, a.embySetLibrariesByAction(r.Context(), target, action, ids, names, enableAll))
	}
	result["action"] = action
	ok(w, "library permissions updated", result)
}
