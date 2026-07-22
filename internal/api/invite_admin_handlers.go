package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleInviteTree(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.refreshStoreForRequest(w) {
		return
	}
	ok(w, "OK", a.inviteForest())
}
func (a *App) handleAdminInviteCodes(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.refreshStoreForRequest(w) {
		return
	}
	codes := a.store().ListAllInviteCodes()
	items := make([]map[string]any, 0, len(codes))
	for _, code := range codes {
		items = append(items, a.inviteCodeDTO(code))
	}
	ok(w, "OK", map[string]any{"codes": items, "total": len(items)})
}

func (a *App) handleAdminInviteDetachDeleteEmby(w http.ResponseWriter, r *http.Request, params Params) {
	if a.refreshStoreForRequest(w) {
		return
	}
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
	if a.refreshStoreForRequest(w) {
		return
	}
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

func (a *App) handleAdminInviteQuickMaintenance(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	if stringValue(payload, "confirm") != confirmInviteQuickMaintain {
		failWithCode(w, http.StatusBadRequest, ErrBatchConfirmRequired, "missing confirm "+confirmInviteQuickMaintain)
		return
	}
	if a.refreshStoreForRequest(w) {
		return
	}
	scope := stringValue(payload, "scope")
	if scope == "" {
		scope = "selected"
	}
	detach := boolValue(payload, "detach", true)
	renewDays := intValue(payload, "renew_days", 0)
	if renewDays > 36500 {
		failWithCode(w, http.StatusBadRequest, ErrBatchDaysInvalid, "renew_days 不能超过 36500")
		return
	}
	if renewDays < -1 {
		failWithCode(w, http.StatusBadRequest, ErrBatchDaysInvalid, "renew_days 必须为 -1、0 或正整数")
		return
	}
	if !detach && renewDays == 0 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "至少选择一个维护操作")
		return
	}
	targets, okTargets := a.inviteQuickMaintenanceTargets(w, payload, scope)
	if !okTargets {
		return
	}
	dryRun := boolValue(payload, "dry_run", false)
	result := map[string]any{
		"scope":       scope,
		"total":       len(targets),
		"success":     0,
		"failed":      0,
		"detached":    0,
		"renewed":     0,
		"renew_days":  renewDays,
		"dry_run":     dryRun,
		"errors":      []map[string]any{},
		"target_uids": targets,
	}
	errorsOut := []map[string]any{}
	success := 0
	failed := 0
	detached := 0
	renewed := 0
	for _, uid := range targets {
		targetErrors := []map[string]any{}
		target, found := a.store().User(uid)
		if renewDays != 0 {
			if !found {
				targetErrors = append(targetErrors, map[string]any{"uid": uid, "code": ErrUserNotFound, "error": userNotFoundMessage})
			} else if a.userIsProtected(target) {
				targetErrors = append(targetErrors, map[string]any{"uid": uid, "code": ErrUserProtected, "error": a.protectedUserReason(target)})
			} else if !dryRun {
				_, err := a.store().UpdateUser(uid, func(u *store.User) error {
					if renewDays < 0 {
						renewExpiryAndReactivate(u, permanentExpiryUnix)
						return nil
					}
					renewExpiryAndReactivate(u, addDaysToExpiry(u.ExpiredAt, renewDays, time.Now()))
					return nil
				})
				if err != nil {
					targetErrors = append(targetErrors, map[string]any{"uid": uid, "error": err.Error()})
				} else {
					renewed++
				}
			} else if found && !a.userIsProtected(target) {
				renewed++
			}
		}
		if detach {
			_, hadParent := a.store().ParentOf(uid)
			if !dryRun {
				if err := a.store().DetachInvite(uid); err != nil {
					targetErrors = append(targetErrors, map[string]any{"uid": uid, "error": err.Error()})
				} else if hadParent {
					detached++
				}
			} else if hadParent {
				detached++
			}
		}
		if len(targetErrors) > 0 {
			failed++
			errorsOut = append(errorsOut, targetErrors...)
		} else {
			success++
		}
	}
	result["success"] = success
	result["failed"] = failed
	result["detached"] = detached
	result["renewed"] = renewed
	result["errors"] = errorsOut
	if !dryRun {
		a.audit(r, "admin_invite_quick_maintenance", "admin", 0, map[string]any{
			"scope": scope, "total": len(targets), "success": success, "failed": failed, "detached": detached, "renewed": renewed, "renew_days": renewDays,
		})
	}
	ok(w, "邀请快捷维护完成", result)
}

func (a *App) inviteQuickMaintenanceTargets(w http.ResponseWriter, payload map[string]any, scope string) ([]int64, bool) {
	targets := []int64{}
	switch scope {
	case "selected":
		targets = uniqueInt64s(int64Slice(payload["uids"]))
	case "subtree":
		rootUID := int64(intValue(payload, "root_uid", 0))
		if rootUID <= 0 {
			failWithCode(w, http.StatusBadRequest, ErrBatchUIDsRequired, "root_uid required")
			return nil, false
		}
		depth := intValue(payload, "depth", -1)
		includeRoot := boolValue(payload, "include_root", false)
		for _, uid := range a.collectCascadeUIDs(rootUID, depth) {
			if uid == rootUID && !includeRoot {
				continue
			}
			targets = append(targets, uid)
		}
	case "all":
		seen := map[int64]bool{}
		for _, rel := range a.store().InviteRelations() {
			if rel.ChildUID <= 0 || seen[rel.ChildUID] {
				continue
			}
			seen[rel.ChildUID] = true
			targets = append(targets, rel.ChildUID)
		}
	default:
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "scope 必须是 selected、subtree 或 all")
		return nil, false
	}
	targets = uniqueInt64s(targets)
	if len(targets) == 0 {
		failWithCode(w, http.StatusBadRequest, ErrBatchUIDsRequired, "没有可处理的邀请目标")
		return nil, false
	}
	if len(targets) > cascadeMaxResults {
		failWithCode(w, http.StatusBadRequest, ErrBatchTooManyTargets, "一次最多处理 5000 个邀请目标")
		return nil, false
	}
	return targets, true
}
