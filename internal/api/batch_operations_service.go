package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

type batchOperationRequest struct {
	UIDs       []int64
	SelectAll  bool
	Filter     map[string]any
	ExcludeUIDs []int64
	Confirm    string
	Reason     string
	Days       int
	DeleteEmby bool
	Scope      string
}

type batchOperationResult struct {
	Success     []int64          `json:"success"`
	Failed      []batchFailure   `json:"failed"`
	SelectedAll bool             `json:"selected_all,omitempty"`
	Metadata    map[string]any   `json:"metadata,omitempty"`
}

type batchFailure struct {
	UID     int64  `json:"uid"`
	Code    ErrCode `json:"code"`
	Message string `json:"message"`
}

func (a *App) batchToggleUsers(ctx context.Context, req batchOperationRequest, enable bool, currentUID int64) (batchOperationResult, error) {
	result := batchOperationResult{
		Success:     make([]int64, 0),
		Failed:      make([]batchFailure, 0),
		SelectedAll: req.SelectAll,
		Metadata:    make(map[string]any),
	}

	for _, uid := range req.UIDs {
		target, okUser := a.store().User(uid)
		if !okUser {
			result.Failed = append(result.Failed, batchFailure{
				UID:     uid,
				Code:    ErrUserNotFound,
				Message: userNotFoundMessage,
			})
			continue
		}

		if a.userIsProtected(target) {
			result.Failed = append(result.Failed, batchFailure{
				UID:     uid,
				Code:    ErrUserProtected,
				Message: fmt.Sprintf("cannot batch toggle protected account: %s", a.protectedUserReason(target)),
			})
			continue
		}

		updated, err := a.store().SetUserActiveAtomic(uid, enable)
		if err == nil && !enable {
			if _, syncErr := a.disableRemoteEmbyForWebState(ctx, updated); syncErr != nil {
				err = syncErr
			}
		}

		if err != nil {
			result.Failed = append(result.Failed, batchFailure{
				UID:     uid,
				Code:    ErrInternal,
				Message: err.Error(),
			})
		} else {
			result.Success = append(result.Success, uid)
		}
	}

	return result, nil
}

func (a *App) batchRenewUsers(ctx context.Context, req batchOperationRequest) (batchOperationResult, error) {
	result := batchOperationResult{
		Success:     make([]int64, 0),
		Failed:      make([]batchFailure, 0),
		SelectedAll: req.SelectAll,
		Metadata:    map[string]any{"days": req.Days},
	}

	uids := uniqueInt64s(req.UIDs)
	now := time.Now()

	outcomes, batchErr := a.store().UpdateUsers(uids, func(u *store.User) error {
		renewExpiryAndReactivate(u, addDaysToExpiry(u.ExpiredAt, req.Days, now))
		return nil
	})

	if batchErr != nil {
		for _, uid := range uids {
			result.Failed = append(result.Failed, batchFailure{
				UID:     uid,
				Code:    ErrInternal,
				Message: batchErr.Error(),
			})
		}
	} else {
		for _, uid := range uids {
			if outcomes[uid] != nil {
				result.Failed = append(result.Failed, batchFailure{
					UID:     uid,
					Code:    ErrInternal,
					Message: outcomes[uid].Error(),
				})
			} else {
				result.Success = append(result.Success, uid)
			}
		}
	}

	return result, nil
}

func (a *App) batchDeleteUsers(ctx context.Context, req batchOperationRequest, currentUID int64) (batchOperationResult, error) {
	result := batchOperationResult{
		Success:     make([]int64, 0),
		Failed:      make([]batchFailure, 0),
		SelectedAll: req.SelectAll,
		Metadata:    make(map[string]any),
	}

	for _, uid := range req.UIDs {
		if uid == currentUID {
			result.Failed = append(result.Failed, batchFailure{
				UID:     uid,
				Code:    ErrBatchSelfTarget,
				Message: "cannot delete current admin",
			})
			continue
		}

		target, okUser := a.store().User(uid)
		if !okUser {
			result.Failed = append(result.Failed, batchFailure{
				UID:     uid,
				Code:    ErrUserNotFound,
				Message: userNotFoundMessage,
			})
			continue
		}

		if a.userIsProtected(target) {
			result.Failed = append(result.Failed, batchFailure{
				UID:     uid,
				Code:    ErrUserProtected,
				Message: fmt.Sprintf("cannot batch delete protected account: %s", a.protectedUserReason(target)),
			})
			continue
		}

		if req.DeleteEmby && target.EmbyID != "" {
			if !a.embyConfigured() {
				result.Failed = append(result.Failed, batchFailure{
					UID:     uid,
					Code:    ErrEmbyNotConfigured,
					Message: "delete_emby: emby not configured",
				})
				continue
			}
			if err := a.embyDelete(ctx, "/Users/"+urlPathEscape(target.EmbyID)); err != nil {
				if !strings.Contains(err.Error(), "remote status 404") {
					result.Failed = append(result.Failed, batchFailure{
						UID:     uid,
						Code:    ErrInternal,
						Message: err.Error(),
					})
					continue
				}
			}
		}

		if err := a.deleteLocalUser(context.WithoutCancel(ctx), target); err != nil {
			result.Failed = append(result.Failed, batchFailure{
				UID:     uid,
				Code:    ErrInternal,
				Message: err.Error(),
			})
		} else {
			result.Success = append(result.Success, uid)
		}
	}

	return result, nil
}

func (a *App) batchToggleEmby(ctx context.Context, req batchOperationRequest, enable bool) (batchOperationResult, error) {
	result := batchOperationResult{
		Success:     make([]int64, 0),
		Failed:      make([]batchFailure, 0),
		SelectedAll: req.SelectAll,
		Metadata:    map[string]any{"emby_enabled": enable, "skipped_no_emby": 0},
	}

	if !a.embyConfigured() {
		return result, fmt.Errorf("Emby URL 或 API Token 未配置")
	}

	skippedNoEmby := 0
	for _, uid := range req.UIDs {
		target, okUser := a.store().User(uid)
		if !okUser {
			result.Failed = append(result.Failed, batchFailure{
				UID:     uid,
				Code:    ErrUserNotFound,
				Message: userNotFoundMessage,
			})
			continue
		}

		if a.userIsProtected(target) {
			result.Failed = append(result.Failed, batchFailure{
				UID:     uid,
				Code:    ErrUserProtected,
				Message: fmt.Sprintf("cannot batch toggle Emby for protected account: %s", a.protectedUserReason(target)),
			})
			continue
		}

		if strings.TrimSpace(target.EmbyID) == "" {
			skippedNoEmby++
			continue
		}

		if enable && !a.embyShouldEnableUser(target) {
			result.Failed = append(result.Failed, batchFailure{
				UID:     uid,
				Code:    ErrConflict,
				Message: "web account disabled or expired; refusing to enable Emby",
			})
			continue
		}

		if err := a.embyApplyEnabledState(ctx, uid, target.EmbyID, enable); err != nil {
			result.Failed = append(result.Failed, batchFailure{
				UID:     uid,
				Code:    ErrInternal,
				Message: err.Error(),
			})
		} else {
			result.Success = append(result.Success, uid)
		}
	}

	result.Metadata["skipped_no_emby"] = skippedNoEmby
	return result, nil
}

func (a *App) batchRefreshStatus(ctx context.Context, req batchOperationRequest) (batchOperationResult, error) {
	result := batchOperationResult{
		Success:     make([]int64, 0),
		Failed:      make([]batchFailure, 0),
		SelectedAll: req.SelectAll,
		Metadata:    map[string]any{"telegram_updated": 0, "emby_disabled": 0},
	}

	tgUpdated, embyDisabled := 0, 0
	scope := normalizeRefreshScope(req.Scope)

	for _, uid := range req.UIDs {
		target, okUser := a.store().User(uid)
		if !okUser {
			result.Failed = append(result.Failed, batchFailure{
				UID:     uid,
				Code:    ErrUserNotFound,
				Message: userNotFoundMessage,
			})
			continue
		}

		summary := a.refreshUserExternalStatus(ctx, target, scope)
		if boolish(summary["telegram_username_updated"]) {
			tgUpdated++
		}
		if boolish(summary["emby_disabled_synced"]) {
			embyDisabled++
		}

		result.Success = append(result.Success, uid)
	}

	result.Metadata["telegram_updated"] = tgUpdated
	result.Metadata["emby_disabled"] = embyDisabled
	return result, nil
}
