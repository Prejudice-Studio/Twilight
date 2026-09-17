package api

import (
	"net/http"
	"sort"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleV2TelegramStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	result := a.telegram().status(current(r).User)
	ok(w, "OK", result)
}

func (a *App) handleV2TelegramUnbind(w http.ResponseWriter, r *http.Request, _ Params) {
	result, err := a.telegram().unbind(r.Context(), current(r).User)
	if err != nil {
		failWithCode(w, http.StatusForbidden, ErrTGUnbindForbidden, err.Error())
		return
	}
	a.audit(r, "unbind_telegram", "user", 0, nil)
	ok(w, result.Message, publicUser(*result.User))
}

func (a *App) handleV2TelegramRebindRequest(w http.ResponseWriter, r *http.Request, _ Params) {
	u := current(r).User
	if u.TelegramID == 0 {
		failWithCode(w, http.StatusBadRequest, ErrTGNotBound, "当前账号未绑定 Telegram")
		return
	}
	req, err := a.store().CreateRebindRequest(store.RebindRequest{
		UID:            u.UID,
		Username:       u.Username,
		OldTelegramID:  u.TelegramID,
		Reason:         truncateString(stringValue(decodeMap(r), "reason"), 500),
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "Telegram rebind request submitted", req)
}

func (a *App) handleV2TelegramCommandCatalog(w http.ResponseWriter, r *http.Request, _ Params) {
	disabledSet := a.telegramDisabledCommandSet()
	disabled := make([]string, 0, len(disabledSet))
	for name := range disabledSet {
		disabled = append(disabled, name)
	}
	sort.Strings(disabled)
	ok(w, "ok", map[string]any{
		"commands":          a.telegramCommandCatalogWithDisabled(disabledSet),
		"disabled_commands": disabled,
	})
}

func (a *App) handleV2TelegramRosterStats(w http.ResponseWriter, r *http.Request, _ Params) {
	result, err := a.telegram().rosterStats()
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, "读取 Telegram 花名册统计失败")
		return
	}
	ok(w, "OK", result)
}
