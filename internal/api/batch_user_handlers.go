package api

import (
	"fmt"
	"net/http"
	"strconv"
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
	payload := decodeMap(r)
	if confirmPhrase != "" && stringValue(payload, "confirm") != confirmPhrase {
		failWithCode(w, http.StatusBadRequest, ErrBatchConfirmRequired, "missing confirm "+confirmPhrase)
		return
	}
	uids, okPayload := a.batchUserUIDsFromPayload(w, payload, 200, 5000, "too many users in one batch")
	if !okPayload {
		return
	}
	result := batchResult(len(uids))
	for _, uid := range uids {
		target, okUser := a.store().User(uid)
		if !okUser {
			addBatchOutcomeWithCode(result, uid, ErrUserNotFound, fmt.Errorf("%s", userNotFoundMessage))
			continue
		}
		if a.userIsProtected(target) {
			addBatchOutcomeWithCode(result, uid, ErrUserProtected, fmt.Errorf("cannot batch toggle protected account: %s", a.protectedUserReason(target)))
			continue
		}
		updated, err := a.store().SetUserActiveAtomic(uid, enable)
		if err == nil && !enable {
			if _, syncErr := a.disableRemoteEmbyForWebState(r.Context(), updated); syncErr != nil {
				err = syncErr
			}
		}
		addBatchOutcome(result, uid, err)
	}
	result["selected_all"] = boolValue(payload, "select_all", false)
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
	payload := decodeMap(r)
	if stringValue(payload, "confirm") != confirmBatchDeleteUsers {
		failWithCode(w, http.StatusBadRequest, ErrBatchConfirmRequired, "missing confirm "+confirmBatchDeleteUsers)
		return
	}
	uids, okPayload := a.batchUserUIDsFromPayload(w, payload, 200, 5000, "too many users in one batch")
	if !okPayload {
		return
	}
	deleteEmby := boolValue(payload, "delete_emby", r.URL.Query().Get("delete_emby") != "false")
	result := batchResult(len(uids))
	for _, uid := range uids {
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
	result["selected_all"] = boolValue(payload, "select_all", false)
	ok(w, "批量删除完成", result)
}

func (a *App) handleBatchLockEmbyUnbind(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	if stringValue(payload, "confirm") != confirmBatchLockEmbyUnbind {
		failWithCode(w, http.StatusBadRequest, ErrBatchConfirmRequired, "missing confirm "+confirmBatchLockEmbyUnbind)
		return
	}
	uids, okPayload := a.batchBoundEmbyUserUIDsFromPayload(w, payload, 200, 5000, "too many users in one batch")
	if !okPayload {
		return
	}
	result := batchResult(len(uids))
	usersByUID := map[int64]store.User{}
	for _, user := range a.store().ListUsers() {
		usersByUID[user.UID] = user
	}
	eligible := make([]int64, 0, len(uids))
	skippedNoEmby := 0
	for _, uid := range uids {
		target, okUser := usersByUID[uid]
		if !okUser {
			addBatchOutcomeWithCode(result, uid, ErrUserNotFound, fmt.Errorf("%s", userNotFoundMessage))
			continue
		}
		if strings.TrimSpace(target.EmbyID) == "" {
			skippedNoEmby++
			continue
		}
		if a.userIsProtected(target) {
			addBatchOutcomeWithCode(result, uid, ErrUserProtected, fmt.Errorf("cannot lock protected account: %s", a.protectedUserReason(target)))
			continue
		}
		eligible = append(eligible, uid)
	}
	updated, missing, skippedDuringWrite, err := a.store().LockEmbyGrantForBoundUsers(eligible)
	if err != nil {
		for _, uid := range eligible {
			addBatchOutcome(result, uid, err)
		}
	} else {
		for _, uid := range updated {
			addBatchOutcome(result, uid, nil)
		}
		for _, uid := range missing {
			addBatchOutcomeWithCode(result, uid, ErrUserNotFound, fmt.Errorf("%s", userNotFoundMessage))
		}
		skippedNoEmby += len(skippedDuringWrite)
	}
	result["selected_all"] = boolValue(payload, "select_all", false)
	result["emby_grant_locked"] = true
	result["skipped_no_emby"] = skippedNoEmby
	ok(w, "批量禁止 Emby 自助解绑完成", result)
}

// handleBatchClearEmbyGrantUnbound 批量清理"没有 Emby 账号"用户的注册码/邀请码
// 使用记录（EmbyGrantLocked / RegistrationSource / RegistrationCode 及码侧引用），
// 让被历史迁移误判为"已用过注册资格"的用户重新可以使用注册码 / 邀请码。
// 已绑定 Emby 的用户与待开通(PendingEmby)用户由 store 自动跳过；管理员账号受保护。
func (a *App) handleBatchClearEmbyGrantUnbound(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	if stringValue(payload, "confirm") != confirmBatchClearEmbyGrant {
		failWithCode(w, http.StatusBadRequest, ErrBatchConfirmRequired, "missing confirm "+confirmBatchClearEmbyGrant)
		return
	}
	uids, okPayload := a.batchUnboundEmbyUserUIDsFromPayload(w, payload, 200, 5000, "too many users in one batch")
	if !okPayload {
		return
	}
	result := batchResult(len(uids))
	usersByUID := map[int64]store.User{}
	for _, user := range a.store().ListUsers() {
		usersByUID[user.UID] = user
	}
	eligible := make([]int64, 0, len(uids))
	for _, uid := range uids {
		target, okUser := usersByUID[uid]
		if !okUser {
			addBatchOutcomeWithCode(result, uid, ErrUserNotFound, fmt.Errorf("%s", userNotFoundMessage))
			continue
		}
		if a.userIsProtected(target) {
			addBatchOutcomeWithCode(result, uid, ErrUserProtected, fmt.Errorf("cannot clear protected account: %s", a.protectedUserReason(target)))
			continue
		}
		eligible = append(eligible, uid)
	}
	res, err := a.store().ClearEmbyGrantForUnboundUsers(eligible)
	if err != nil {
		for _, uid := range eligible {
			addBatchOutcome(result, uid, err)
		}
		result["selected_all"] = boolValue(payload, "select_all", false)
		ok(w, "批量清理注册资格记录完成", result)
		return
	}
	for _, uid := range res.Cleared {
		addBatchOutcome(result, uid, nil)
	}
	for _, uid := range res.AlreadyClean {
		addBatchOutcome(result, uid, nil)
	}
	for _, uid := range res.Missing {
		addBatchOutcomeWithCode(result, uid, ErrUserNotFound, fmt.Errorf("%s", userNotFoundMessage))
	}
	result["selected_all"] = boolValue(payload, "select_all", false)
	result["emby_grant_cleared"] = true
	result["cleared"] = len(res.Cleared)
	result["skipped_has_emby"] = len(res.SkippedHasEmby)
	result["skipped_pending"] = len(res.SkippedPending)
	result["regcode_refs_removed"] = res.RegcodeRefs
	result["invite_refs_removed"] = res.InviteRefs
	ok(w, "批量清理注册资格记录完成", result)
}

func (a *App) batchUserUIDsFromPayload(w http.ResponseWriter, payload map[string]any, maxExplicit, maxSelectedAll int, tooManyMessage string) ([]int64, bool) {
	if boolValue(payload, "select_all", false) {
		uids, matched := a.filteredBatchUserUIDs(payload, maxSelectedAll)
		if maxSelectedAll > 0 && matched > maxSelectedAll {
			failWithCode(w, http.StatusBadRequest, ErrBatchTooManyTargets, fmt.Sprintf("too many users in selected filter: %d > %d", matched, maxSelectedAll))
			return nil, false
		}
		return uids, true
	}
	uids := int64Slice(payload["uids"])
	if len(uids) == 0 {
		failWithCode(w, http.StatusBadRequest, ErrBatchUIDsRequired, "uids required")
		return nil, false
	}
	if maxExplicit > 0 && len(uids) > maxExplicit {
		failWithCode(w, http.StatusBadRequest, ErrBatchTooManyTargets, tooManyMessage)
		return nil, false
	}
	return uniqueInt64s(uids), true
}

func (a *App) batchBoundEmbyUserUIDsFromPayload(w http.ResponseWriter, payload map[string]any, maxExplicit, maxSelectedAll int, tooManyMessage string) ([]int64, bool) {
	if !boolValue(payload, "select_all", false) {
		return a.batchUserUIDsFromPayload(w, payload, maxExplicit, maxSelectedAll, tooManyMessage)
	}
	filter, _ := payload["filter"].(map[string]any)
	if strings.EqualFold(asString(filter["emby"]), "unbound") {
		return []int64{}, true
	}
	boundFilter := map[string]any{}
	for key, value := range filter {
		boundFilter[key] = value
	}
	boundFilter["emby"] = "bound"
	boundPayload := map[string]any{}
	for key, value := range payload {
		boundPayload[key] = value
	}
	boundPayload["filter"] = boundFilter
	return a.batchUserUIDsFromPayload(w, boundPayload, maxExplicit, maxSelectedAll, tooManyMessage)
}

// batchUnboundEmbyUserUIDsFromPayload 与 batchBoundEmbyUserUIDsFromPayload 对偶：
// select_all 时强制把过滤器收窄为"未绑定 Emby"，避免一次性把已绑定用户也拉进
// 清理目标（store 仍会二次跳过已绑定用户，这里只是缩小扫描面）。
func (a *App) batchUnboundEmbyUserUIDsFromPayload(w http.ResponseWriter, payload map[string]any, maxExplicit, maxSelectedAll int, tooManyMessage string) ([]int64, bool) {
	if !boolValue(payload, "select_all", false) {
		return a.batchUserUIDsFromPayload(w, payload, maxExplicit, maxSelectedAll, tooManyMessage)
	}
	filter, _ := payload["filter"].(map[string]any)
	if strings.EqualFold(asString(filter["emby"]), "bound") {
		return []int64{}, true
	}
	unboundFilter := map[string]any{}
	for key, value := range filter {
		unboundFilter[key] = value
	}
	unboundFilter["emby"] = "unbound"
	unboundPayload := map[string]any{}
	for key, value := range payload {
		unboundPayload[key] = value
	}
	unboundPayload["filter"] = unboundFilter
	return a.batchUserUIDsFromPayload(w, unboundPayload, maxExplicit, maxSelectedAll, tooManyMessage)
}

func (a *App) filteredBatchUserUIDs(payload map[string]any, limit int) ([]int64, int) {
	filter, _ := payload["filter"].(map[string]any)
	roleFilter, hasRole := filter["role"]
	activeFilter, hasActive := filter["active"]
	embyFilter := strings.ToLower(asString(filter["emby"]))
	search := strings.ToLower(strings.TrimSpace(asString(filter["search"])))
	// exclude_uids 支持「反选全部 / 全选后取消个别」：前端用排除集表达跨页选择，
	// 这里把它从匹配集中剔除。只能缩小目标集，无法越过筛选 / 鉴权扩大目标——
	// 后续每个 batch handler 仍会逐 UID 复核 userIsProtected 等约束。
	excluded := map[int64]struct{}{}
	for _, id := range int64Slice(payload["exclude_uids"]) {
		excluded[id] = struct{}{}
	}
	uids := []int64{}
	matched := 0
	for _, u := range a.store().ListUsers() {
		if hasRole && strconv.Itoa(u.Role) != asString(roleFilter) {
			continue
		}
		if hasActive && u.Active != boolish(activeFilter) {
			continue
		}
		if embyFilter == "bound" && u.EmbyID == "" {
			continue
		}
		if embyFilter == "unbound" && u.EmbyID != "" {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(u.Username+" "+u.Email+" "+u.EmbyID+" "+strconv.FormatInt(u.UID, 10)+" "+strconv.FormatInt(u.TelegramID, 10)), search) {
			continue
		}
		if _, skip := excluded[u.UID]; skip {
			continue
		}
		matched++
		if limit <= 0 || len(uids) < limit {
			uids = append(uids, u.UID)
		}
	}
	return uids, matched
}
