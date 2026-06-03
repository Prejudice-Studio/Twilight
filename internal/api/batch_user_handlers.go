package api

import (
	"fmt"
	"net/http"
	"strings"
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
			addBatchOutcomeWithCode(result, uid, ErrUserNotFound, fmt.Errorf("%s", userNotFoundMessage))
			continue
		}
		if a.userIsProtected(target) {
			addBatchOutcomeWithCode(result, uid, ErrUserProtected, fmt.Errorf("cannot batch toggle protected account: %s", a.protectedUserReason(target)))
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
	payload, uids, okPayload := requireBatchPayload(w, r, confirmBatchRenewUsers, 200, "too many users in one batch")
	if !okPayload {
		return
	}
	days := intValue(payload, "days", 30)
	if days <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrBatchDaysInvalid, "days 必须大于 0")
		return
	}
	if days > 36500 {
		failWithCode(w, http.StatusBadRequest, ErrBatchDaysInvalid, "days 不能超过 36500")
		return
	}
	result := batchResult(len(uids))
	for _, uid := range uniqueInt64s(uids) {
		_, err := a.store().UpdateUser(uid, func(u *store.User) error {
			// 与 self / admin renew 对齐：批量续期同样把被 check_expired 自禁
			// 的账号一并解禁，避免 admin 在批量页看到"成功 200"但用户登录依旧
			// 失败的灰色状态。
			renewExpiryAndReactivate(u, addDaysToExpiry(u.ExpiredAt, days, time.Now()))
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
			addBatchOutcomeWithCode(result, uid, ErrBatchSelfTarget, fmt.Errorf("cannot delete current admin"))
			continue
		}
		target, okUser := a.store().User(uid)
		if !okUser {
			addBatchOutcomeWithCode(result, uid, ErrUserNotFound, fmt.Errorf("%s", userNotFoundMessage))
			continue
		}
		if a.userIsProtected(target) {
			addBatchOutcomeWithCode(result, uid, ErrUserProtected, fmt.Errorf("cannot batch delete protected account: %s", a.protectedUserReason(target)))
			continue
		}
		if deleteEmby && target.EmbyID != "" {
			if !a.embyConfigured() {
				addBatchOutcomeWithCode(result, uid, ErrEmbyNotConfigured, fmt.Errorf("delete_emby: emby not configured"))
				continue
			}
			if err := a.embyDelete(r.Context(), "/Users/"+urlPathEscape(target.EmbyID)); err != nil {
				if !strings.Contains(err.Error(), "remote status 404") {
					addBatchOutcome(result, uid, err)
					continue
				}
			}
		}
		addBatchOutcome(result, uid, a.store().DeleteUser(uid))
	}
	ok(w, "批量删除完成", result)
}
