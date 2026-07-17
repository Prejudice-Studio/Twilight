package api

import (
	"net/http"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

type telegramBindCodeState struct {
	Code             string
	Status           string
	ErrorCode        ErrCode
	HTTPStatus       int
	Message          string
	Bind             store.BindCode
	Confirmed        bool
	Invalid          bool
	Terminal         bool
	ExpiresIn        int64
	TelegramID       int64
	TelegramUsername string
	TelegramBound    bool
}

// telegramBindCodeState checks the status of a bind code. If uid is non-zero
// (user scene), it also verifies the bind code belongs to that user and checks
// whether the user's Telegram has been bound as a result of the code.
// If requireScene is non-empty, the code must match that scene exactly.
func (a *App) telegramBindCodeState(code string, uid int64, requireScene string, now int64, cleanupExpired bool) telegramBindCodeState {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !telegramBindCodePattern.MatchString(code) {
		return telegramBindCodeState{Code: code, Status: "invalid_format", ErrorCode: ErrTGBindCodeFormat, HTTPStatus: http.StatusBadRequest, Message: "Telegram 绑定码格式不正确", Invalid: true, Terminal: true}
	}
	if a.bindStatus != nil {
		if failure, ok := a.bindStatus.failure(code, now); ok {
			return telegramBindCodeState{
				Code:       code,
				Status:     failure.Status,
				ErrorCode:  failure.ErrorCode,
				HTTPStatus: failure.HTTPStatus,
				Message:    failure.Message,
				Invalid:    true,
				Terminal:   true,
			}
		}
	}
	bind, okBind := a.bindCode(code)
	if !okBind {
		if uid != 0 {
			for _, u := range a.store().ListUsers() {
				if u.UID == uid && u.TelegramID != 0 {
					return telegramBindCodeState{Code: code, Status: "bound", Message: "Telegram 已绑定", Confirmed: true, Terminal: true, TelegramBound: true, TelegramID: u.TelegramID, TelegramUsername: u.TelegramUsername}
				}
			}
		}
		return telegramBindCodeState{Code: code, Status: "not_found", ErrorCode: ErrTGBindCodeNotFound, HTTPStatus: http.StatusBadRequest, Message: "绑定码不存在", Invalid: true, Terminal: true}
	}
	if uid != 0 && bind.UID != uid {
		return telegramBindCodeState{Code: code, Status: "not_found", ErrorCode: ErrTGBindCodeNotFound, HTTPStatus: http.StatusBadRequest, Message: "绑定码不存在", Invalid: true, Terminal: true}
	}
	if requireScene != "" && bind.Scene != requireScene {
		return telegramBindCodeState{Code: code, Status: "wrong_scene", ErrorCode: ErrTGBindCodeSceneBad, HTTPStatus: http.StatusBadRequest, Message: "绑定码场景无效", Invalid: true, Terminal: true}
	}
	if bind.Confirmed && bind.TelegramID != 0 {
		// 已确认的注册场景绑定码（bind.UID == 0）仍受 TTL 约束：confirm 之后若
		// 超过有效期还没被 /register 消费，必须按过期处理。hub 没有后台清扫
		// （仅 createBindCode 时 sweep），否则一个确认态注册码会无限期停留在
		// "confirmed"——既能跨越 300s TTL 被重放注册，也会在静默系统里堆积不释放。
		// user 场景（bind.UID != 0）confirm 即已写库绑定，过期由顶部 ListUsers
		// 回退与 30s grace 兜底，这里不改其语义。
		if bind.UID == 0 && bind.ExpiresAt > 0 && bind.ExpiresAt <= now {
			if cleanupExpired {
				_ = a.deleteBindCode(code)
			}
			return telegramBindCodeState{Code: code, Status: "expired", ErrorCode: ErrTGBindCodeExpired, HTTPStatus: http.StatusBadRequest, Message: "绑定码无效或已过期", Bind: bind, Invalid: true, Terminal: true}
		}
		if bind.UID == 0 {
			if existing, okUser := a.store().FindUserByTelegramID(bind.TelegramID); okUser {
				if cleanupExpired {
					a.rejectRegisterBindCode(bind, code, "telegram_taken", ErrTGBindTargetTaken, http.StatusConflict, "该 Telegram 已绑定到账号 "+existing.Username)
				}
				return telegramBindCodeState{Code: code, Status: "telegram_taken", ErrorCode: ErrTGBindTargetTaken, HTTPStatus: http.StatusConflict, Message: "该 Telegram 已绑定到账号 " + existing.Username, Bind: bind, Invalid: true, Terminal: true, TelegramID: bind.TelegramID, TelegramUsername: bind.TelegramUsername}
			}
		} else {
			u, okUser := a.store().User(bind.UID)
			if !okUser {
				if cleanupExpired {
					_ = a.deleteBindCode(code)
				}
				return telegramBindCodeState{Code: code, Status: "not_found", ErrorCode: ErrTGBindCodeNotFound, HTTPStatus: http.StatusBadRequest, Message: "绑定码不存在", Bind: bind, Invalid: true, Terminal: true}
			}
			if u.TelegramID != bind.TelegramID {
				if cleanupExpired {
					_ = a.deleteBindCode(code)
				}
				return telegramBindCodeState{Code: code, Status: "telegram_taken", ErrorCode: ErrTGBindTargetTaken, HTTPStatus: http.StatusConflict, Message: "绑定状态已变化，请重新获取绑定码", Bind: bind, Invalid: true, Terminal: true}
			}
		}
		state := telegramBindCodeState{Code: code, Bind: bind, ExpiresIn: bind.ExpiresAt - now, TelegramID: bind.TelegramID, TelegramUsername: bind.TelegramUsername}
		state.Status = "confirmed"
		state.Message = "绑定码已确认"
		state.Confirmed = true
		state.Terminal = true
		state.TelegramBound = bind.UID != 0
		return state
	}
	if bind.ExpiresAt <= now {
		if cleanupExpired {
			_ = a.deleteBindCode(code)
		}
		return telegramBindCodeState{Code: code, Status: "expired", ErrorCode: ErrTGBindCodeExpired, HTTPStatus: http.StatusBadRequest, Message: "绑定码无效或已过期", Bind: bind, Invalid: true, Terminal: true}
	}
	state := telegramBindCodeState{Code: code, Bind: bind, ExpiresIn: bind.ExpiresAt - now, TelegramID: bind.TelegramID, TelegramUsername: bind.TelegramUsername}
	state.Status = "pending"
	state.Message = "绑定码尚未在 Telegram 中确认"
	state.Confirmed = false
	state.Terminal = false
	return state
}

func writeTelegramBindCodeState(w http.ResponseWriter, state telegramBindCodeState) {
	data := state.response()
	if state.Invalid {
		writeJSONWithCode(w, http.StatusOK, false, state.ErrorCode, state.Message, data)
		return
	}
	ok(w, "OK", data)
}

func (a *App) recordRegisterBindFailure(bind store.BindCode, code string, status string, errorCode ErrCode, httpStatus int, message string) {
	if a.bindStatus == nil {
		return
	}
	a.bindStatus.fail(code, bindCodeFailure{Status: status, ErrorCode: errorCode, HTTPStatus: httpStatus, Message: message, ExpiresAt: bind.ExpiresAt})
}

func (a *App) rejectRegisterBindCode(bind store.BindCode, code string, status string, errorCode ErrCode, httpStatus int, message string) {
	if bind.UID == 0 {
		_ = a.deleteBindCode(code)
	}
	a.recordRegisterBindFailure(bind, code, status, errorCode, httpStatus, message)
}

func (a *App) clearRegisterBindFailure(code string) {
	if a.bindStatus == nil {
		return
	}
	a.bindStatus.clear(code)
}

func (a *App) bindCode(code string) (store.BindCode, bool) {
	if a.bindStatus == nil {
		return store.BindCode{}, false
	}
	return a.bindStatus.bindCode(code)
}

func (a *App) upsertBindCode(bind store.BindCode) error {
	if a.bindStatus == nil {
		return store.ErrNotFound
	}
	return a.bindStatus.upsertBindCode(bind)
}

func (a *App) deleteBindCode(code string) error {
	if a.bindStatus == nil {
		return store.ErrNotFound
	}
	return a.bindStatus.deleteBindCode(code)
}

func (a *App) cleanupExpiredBindCodes(now int64) int {
	if a.bindStatus == nil {
		return 0
	}
	return a.bindStatus.cleanupExpiredBindCodes(now)
}

func (a *App) consumeConfirmedRegisterBindCode(code string, now int64, create func(store.BindCode) (store.User, store.RegCode, error)) (store.User, store.RegCode, store.BindCode, error) {
	if a.bindStatus == nil {
		return store.User{}, store.RegCode{}, store.BindCode{}, store.ErrNotFound
	}
	return a.bindStatus.consumeConfirmedRegisterBindCode(code, now, create)
}

func (a *App) confirmBindCodeAtomic(code string, telegramID int64, telegramUsername string, now int64) (store.BindCode, store.User, bool, error) {
	if a.bindStatus == nil {
		return store.BindCode{}, store.User{}, false, store.ErrNotFound
	}
	return a.bindStatus.confirmBindCodeAtomic(code, telegramID, telegramUsername, now, func(tgid, allowedUID int64) bool {
		if existing, ok := a.store().FindUserByTelegramID(tgid); ok && existing.UID != allowedUID {
			return true
		}
		return false
	}, func(bind store.BindCode) (store.User, error) {
		updated, _, err := a.store().BindUserTelegramAtomic(bind.UID, bind.TelegramID, bind.UID)
		if err != nil {
			return store.User{}, err
		}
		if strings.TrimSpace(bind.TelegramUsername) != "" {
			updated, err = a.store().UpdateUser(bind.UID, func(u *store.User) error {
				u.TelegramUsername = strings.TrimSpace(bind.TelegramUsername)
				return nil
			})
			if err != nil {
				return store.User{}, err
			}
		}
		a.auditTelegramAction(bind.TelegramID, "bind_telegram_via_telegram", "user", updated.UID, map[string]any{"scene": bind.Scene})
		return updated, nil
	})
}

func (s telegramBindCodeState) response() map[string]any {
	data := map[string]any{
		"code":           s.Code,
		"status":         s.Status,
		"confirmed":      s.Confirmed,
		"invalid":        s.Invalid,
		"terminal":       s.Terminal,
		"message":        s.Message,
		"telegram_bound": s.TelegramBound,
	}
	if s.ErrorCode != "" {
		data["error_code"] = s.ErrorCode
	}
	if s.ExpiresIn > 0 {
		data["expires_in"] = s.ExpiresIn
	}
	if s.TelegramID != 0 {
		data["telegram_id"] = s.TelegramID
	}
	if strings.TrimSpace(s.TelegramUsername) != "" {
		data["telegram_username"] = s.TelegramUsername
	}
	return data
}
