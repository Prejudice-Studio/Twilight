package api

import (
	"fmt"
	"net/http"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleInviteTree(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", a.inviteForest())
}
func (a *App) handleAdminInviteCodes(w http.ResponseWriter, r *http.Request, _ Params) {
	codes := a.store().ListAllInviteCodes()
	items := make([]map[string]any, 0, len(codes))
	for _, code := range codes {
		items = append(items, a.inviteCodeDTO(code))
	}
	ok(w, "OK", map[string]any{"codes": items, "total": len(items)})
}

func (a *App) handleAdminInviteDetachDeleteEmby(w http.ResponseWriter, r *http.Request, params Params) {
	target, okUser := a.userFromPath(w, params, "uid")
	if !okUser {
		return
	}
	if target.Role == store.RoleAdmin {
		failWithCode(w, http.StatusForbidden, ErrUserProtected, "不能通过邀请管理删除管理员的 Emby 账号")
		return
	}
	updated, deletedEmby, okDelete := a.deleteEmbyAndDetachInviteUser(w, r, target, "admin_invite_detach_delete_emby", "admin")
	if !okDelete {
		return
	}
	ok(w, "已断开邀请关系并删除 Emby 账号", map[string]any{
		"uid":          target.UID,
		"detached":     true,
		"deleted_emby": deletedEmby,
		"user":         publicUser(updated),
	})
}

func (a *App) handleAdminInviteDetachBatch(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	uids := uniqueInt64s(int64Slice(payload["uids"]))
	if len(uids) == 0 {
		failWithCode(w, http.StatusBadRequest, ErrBatchUIDsRequired, "uids required")
		return
	}
	if len(uids) > 200 {
		failWithCode(w, http.StatusBadRequest, ErrBatchTooManyTargets, "too many users in one batch")
		return
	}
	deleteEmby := boolValue(payload, "delete_emby", false)
	if deleteEmby && !a.embyConfigured() {
		failWithCode(w, http.StatusBadGateway, ErrEmbyDeleteFailed, "Emby 未配置，无法删除 Emby 账号")
		return
	}

	result := batchResult(len(uids))
	deletedEmbyCount := 0
	for _, uid := range uids {
		target, okUser := a.store().User(uid)
		if !okUser {
			addBatchOutcomeWithCode(result, uid, ErrUserNotFound, fmt.Errorf("%s", userNotFoundMessage))
			continue
		}
		if deleteEmby && target.Role == store.RoleAdmin {
			addBatchOutcomeWithCode(result, uid, ErrUserProtected, fmt.Errorf("cannot delete administrator Emby account through invite management"))
			continue
		}
		if deleteEmby {
			_, deletedEmby, err := a.detachInviteUserCleanup(r.Context(), target, true)
			if deletedEmby {
				deletedEmbyCount++
			}
			addBatchOutcome(result, uid, err)
			continue
		}
		addBatchOutcome(result, uid, a.store().DetachInvite(uid))
	}

	result["delete_emby"] = deleteEmby
	result["deleted_emby"] = deletedEmbyCount
	result["detached"] = result["success"]
	action := "admin_invite_detach_batch"
	if deleteEmby {
		action = "admin_invite_detach_delete_emby_batch"
	}
	a.audit(r, action, "admin", 0, map[string]any{
		"total":        result["total"],
		"success":      result["success"],
		"failed":       result["failed"],
		"delete_emby":  deleteEmby,
		"deleted_emby": deletedEmbyCount,
	})
	ok(w, "批量邀请关系处理完成", result)
}
