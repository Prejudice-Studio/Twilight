package api

import (
	"net/http"
)

func (a *App) handleV2BatchDisableUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleV2BatchToggleUsers(w, r, false)
}

func (a *App) handleV2BatchEnableUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleV2BatchToggleUsers(w, r, true)
}

func (a *App) handleV2BatchToggleUsers(w http.ResponseWriter, r *http.Request, enable bool) {
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

	req := batchOperationRequest{
		UIDs:      uids,
		SelectAll: boolValue(payload, "select_all", false),
		Reason:    stringValue(payload, "reason"),
	}

	result, err := a.batchToggleUsers(r.Context(), req, enable, current(r).User.UID)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, err.Error())
		return
	}

	action := "batch_disable_users"
	if enable {
		action = "batch_enable_users"
	}
	a.audit(r, action, "admin", 0, map[string]any{
		"success": len(result.Success),
		"failed":  len(result.Failed),
	})

	ok(w, "批量操作完成", result)
}

func (a *App) handleV2BatchRenewUsers(w http.ResponseWriter, r *http.Request, _ Params) {
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

	req := batchOperationRequest{
		UIDs:      uids,
		SelectAll: boolValue(payload, "select_all", false),
		Days:      days,
	}

	result, err := a.batchRenewUsers(r.Context(), req)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, err.Error())
		return
	}

	a.audit(r, "batch_renew_users", "admin", 0, map[string]any{
		"days":    days,
		"success": len(result.Success),
		"failed":  len(result.Failed),
	})

	ok(w, "批量续期完成", result)
}

func (a *App) handleV2BatchDeleteUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	if stringValue(payload, "confirm") != confirmBatchDeleteUsers {
		failWithCode(w, http.StatusBadRequest, ErrBatchConfirmRequired, "missing confirm "+confirmBatchDeleteUsers)
		return
	}

	uids, okPayload := a.batchUserUIDsFromPayload(w, payload, 200, 5000, "too many users in one batch")
	if !okPayload {
		return
	}

	req := batchOperationRequest{
		UIDs:       uids,
		SelectAll:  boolValue(payload, "select_all", false),
		DeleteEmby: boolValue(payload, "delete_emby", r.URL.Query().Get("delete_emby") != "false"),
	}

	result, err := a.batchDeleteUsers(r.Context(), req, current(r).User.UID)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, err.Error())
		return
	}

	a.audit(r, "batch_delete_users", "admin", 0, map[string]any{
		"success": len(result.Success),
		"failed":  len(result.Failed),
	})

	ok(w, "批量删除完成", result)
}

func (a *App) handleV2BatchEmbyEnable(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleV2BatchToggleEmby(w, r, true)
}

func (a *App) handleV2BatchEmbyDisable(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleV2BatchToggleEmby(w, r, false)
}

func (a *App) handleV2BatchToggleEmby(w http.ResponseWriter, r *http.Request, enable bool) {
	confirmPhrase := confirmBatchEmbyDisable
	if enable {
		confirmPhrase = confirmBatchEmbyEnable
	}

	payload := decodeMap(r)
	if stringValue(payload, "confirm") != confirmPhrase {
		failWithCode(w, http.StatusBadRequest, ErrBatchConfirmRequired, "missing confirm "+confirmPhrase)
		return
	}

	uids, okPayload := a.batchBoundEmbyUserUIDsFromPayload(w, payload, 200, 5000, "too many users in one batch")
	if !okPayload {
		return
	}

	req := batchOperationRequest{
		UIDs:      uids,
		SelectAll: boolValue(payload, "select_all", false),
	}

	result, err := a.batchToggleEmby(r.Context(), req, enable)
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyNotConfigured, err.Error())
		return
	}

	a.audit(r, "batch_toggle_emby", "admin", 0, map[string]any{
		"enable":          enable,
		"total":           len(req.UIDs),
		"skipped_no_emby": result.Metadata["skipped_no_emby"],
	})

	ok(w, "批量 Emby 状态更新完成", result)
}

func (a *App) handleV2BatchRefreshStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	scope := normalizeRefreshScope(stringValue(payload, "scope"))

	uids, okPayload := a.batchUserUIDsFromPayload(w, payload, 0, 0, "too many users in one batch")
	if !okPayload {
		return
	}

	req := batchOperationRequest{
		UIDs:      uids,
		SelectAll: boolValue(payload, "select_all", false),
		Scope:     scope,
	}

	result, err := a.batchRefreshStatus(r.Context(), req)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, err.Error())
		return
	}

	a.audit(r, "batch_refresh_status", "admin", 0, map[string]any{
		"scope":       scope,
		"total":       len(req.UIDs),
		"tg_updated":  result.Metadata["telegram_updated"],
		"emby_disabled": result.Metadata["emby_disabled"],
	})

	ok(w, "批量刷新状态完成", result)
}
