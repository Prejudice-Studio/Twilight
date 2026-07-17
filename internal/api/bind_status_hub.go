package api

import (
	"strings"
	"sync"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

type bindCodeFailure struct {
	Code       string
	Status     string
	ErrorCode  ErrCode
	HTTPStatus int
	Message    string
	ExpiresAt  int64
}

type bindStatusHub struct {
	mu       sync.Mutex
	codes    map[string]store.BindCode
	watchers map[string]map[chan struct{}]struct{}
	failures map[string]bindCodeFailure
}

func newBindStatusHub() *bindStatusHub {
	return &bindStatusHub{
		codes:    map[string]store.BindCode{},
		watchers: map[string]map[chan struct{}]struct{}{},
		failures: map[string]bindCodeFailure{},
	}
}

func (h *bindStatusHub) upsertBindCode(bind store.BindCode) error {
	bind.Code = normalizeBindStatusCode(bind.Code)
	if bind.Code == "" {
		return store.ErrNotFound
	}
	h.mu.Lock()
	h.codes[bind.Code] = bind
	delete(h.failures, bind.Code)
	h.notifyLocked(bind.Code)
	h.mu.Unlock()
	return nil
}

func (h *bindStatusHub) bindCode(code string) (store.BindCode, bool) {
	code = normalizeBindStatusCode(code)
	h.mu.Lock()
	defer h.mu.Unlock()
	bind, ok := h.codes[code]
	return bind, ok
}

func (h *bindStatusHub) deleteBindCode(code string) error {
	code = normalizeBindStatusCode(code)
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.codes[code]; !ok {
		return store.ErrNotFound
	}
	delete(h.codes, code)
	delete(h.failures, code)
	h.notifyLocked(code)
	return nil
}

func (h *bindStatusHub) deleteBindCodesForUser(uid int64, telegramID int64) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	deleted := 0
	for code, bind := range h.codes {
		if (uid != 0 && bind.UID == uid) || (telegramID != 0 && bind.TelegramID == telegramID) {
			delete(h.codes, code)
			delete(h.failures, code)
			h.notifyLocked(code)
			deleted++
		}
	}
	return deleted
}

func (h *bindStatusHub) cleanupOrphanedUserBindCodes(userExists func(uid int64) bool) int {
	if userExists == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	deleted := 0
	for code, bind := range h.codes {
		if bind.UID == 0 || userExists(bind.UID) {
			continue
		}
		delete(h.codes, code)
		delete(h.failures, code)
		h.notifyLocked(code)
		deleted++
	}
	return deleted
}

func (h *bindStatusHub) cleanupExpiredBindCodes(now int64) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	deleted := 0
	for code, bind := range h.codes {
		if bind.ExpiresAt > 0 && bind.ExpiresAt <= now {
			delete(h.codes, code)
			delete(h.failures, code)
			h.notifyLocked(code)
			deleted++
		}
	}
	for code, failure := range h.failures {
		if failure.ExpiresAt <= now {
			delete(h.failures, code)
		}
	}
	return deleted
}

func (h *bindStatusHub) consumeConfirmedRegisterBindCode(code string, now int64, create func(store.BindCode) (store.User, store.RegCode, error)) (store.User, store.RegCode, store.BindCode, error) {
	code = normalizeBindStatusCode(code)
	h.mu.Lock()
	defer h.mu.Unlock()
	bind, ok := h.codes[code]
	if !ok {
		return store.User{}, store.RegCode{}, store.BindCode{}, store.ErrNotFound
	}
	if now == 0 {
		now = time.Now().Unix()
	}
	if bind.ExpiresAt > 0 && bind.ExpiresAt <= now {
		delete(h.codes, code)
		delete(h.failures, code)
		h.notifyLocked(code)
		return store.User{}, store.RegCode{}, store.BindCode{}, store.ErrExpired
	}
	if bind.Scene != "register" || !bind.Confirmed || bind.TelegramID == 0 {
		return store.User{}, store.RegCode{}, store.BindCode{}, store.ErrConflict
	}
	if create == nil {
		return store.User{}, store.RegCode{}, store.BindCode{}, store.ErrConflict
	}
	user, consumed, err := create(bind)
	if err != nil {
		delete(h.codes, code)
		h.failures[code] = bindCodeFailure{
			Code:       code,
			Status:     "register_failed",
			ErrorCode:  ErrBindCodeConflict,
			HTTPStatus: 409,
			Message:    "注册未完成，请重新获取 Telegram 绑定码后再试",
			ExpiresAt:  bind.ExpiresAt,
		}
		h.notifyLocked(code)
		return store.User{}, store.RegCode{}, bind, err
	}
	delete(h.codes, code)
	delete(h.failures, code)
	h.notifyLocked(code)
	return user, consumed, bind, nil
}

// confirmBindCodeAtomic atomically marks a bind code as confirmed. For user-scene
// codes (bind.UID != 0), it also calls bindUser to immediately bind the user, then
// keeps the confirmed code in the hub for a 30‑second grace period so that status
// polling / WebSocket watchers can discover the "bound" result before the code is
// cleaned up. For register-scene codes (bind.UID == 0), the code remains in the
// hub until expired or consumed by the registration flow.
func (h *bindStatusHub) confirmBindCodeAtomic(code string, telegramID int64, telegramUsername string, now int64, telegramIDTaken func(telegramID, allowedUID int64) bool, bindUser func(store.BindCode) (store.User, error)) (store.BindCode, store.User, bool, error) {
	code = normalizeBindStatusCode(code)
	h.mu.Lock()
	defer h.mu.Unlock()
	bind, ok := h.codes[code]
	if !ok {
		return store.BindCode{}, store.User{}, false, store.ErrNotFound
	}
	if now == 0 {
		now = time.Now().Unix()
	}
	if bind.ExpiresAt > 0 && bind.ExpiresAt <= now {
		delete(h.codes, code)
		delete(h.failures, code)
		h.notifyLocked(code)
		return store.BindCode{}, store.User{}, false, store.ErrExpired
	}
	if telegramID == 0 {
		return store.BindCode{}, store.User{}, false, store.ErrConflict
	}
	if bind.Confirmed && bind.TelegramID != 0 {
		if bind.TelegramID != telegramID {
			return store.BindCode{}, store.User{}, false, store.ErrConflict
		}
		return bind, store.User{}, false, nil
	}
	if telegramIDTaken != nil && telegramIDTaken(telegramID, bind.UID) {
		return store.BindCode{}, store.User{}, false, store.ErrConflict
	}
	bind.Confirmed = true
	bind.TelegramID = telegramID
	bind.TelegramUsername = strings.TrimSpace(telegramUsername)
	if bind.UID != 0 {
		var updated store.User
		if bindUser != nil {
			user, err := bindUser(bind)
			if err != nil {
				return store.BindCode{}, store.User{}, false, err
			}
			updated = user
		}
		bind.ExpiresAt = now + 30
		h.codes[code] = bind
		delete(h.failures, code)
		h.notifyLocked(code)
		return bind, updated, true, nil
	}
	h.codes[code] = bind
	delete(h.failures, code)
	h.notifyLocked(code)
	return bind, store.User{}, false, nil
}

func (h *bindStatusHub) subscribe(code string) (<-chan struct{}, func()) {
	code = normalizeBindStatusCode(code)
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	if h.watchers[code] == nil {
		h.watchers[code] = map[chan struct{}]struct{}{}
	}
	h.watchers[code][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if watchers := h.watchers[code]; watchers != nil {
			delete(watchers, ch)
			if len(watchers) == 0 {
				delete(h.watchers, code)
			}
		}
		h.mu.Unlock()
	}
}

func (h *bindStatusHub) fail(code string, failure bindCodeFailure) {
	code = normalizeBindStatusCode(code)
	if code == "" {
		return
	}
	if failure.ExpiresAt <= time.Now().Unix() {
		failure.ExpiresAt = time.Now().Add(10 * time.Minute).Unix()
	}
	failure.Code = code
	h.mu.Lock()
	h.failures[code] = failure
	h.notifyLocked(code)
	h.mu.Unlock()
}

func (h *bindStatusHub) clear(code string) {
	code = normalizeBindStatusCode(code)
	if code == "" {
		return
	}
	h.mu.Lock()
	delete(h.failures, code)
	h.notifyLocked(code)
	h.mu.Unlock()
}

func (h *bindStatusHub) notify(code string) {
	code = normalizeBindStatusCode(code)
	if code == "" {
		return
	}
	h.mu.Lock()
	h.notifyLocked(code)
	h.mu.Unlock()
}

func (h *bindStatusHub) failure(code string, now int64) (bindCodeFailure, bool) {
	code = normalizeBindStatusCode(code)
	h.mu.Lock()
	defer h.mu.Unlock()
	failure, ok := h.failures[code]
	if !ok {
		return bindCodeFailure{}, false
	}
	if failure.ExpiresAt <= now {
		delete(h.failures, code)
		return bindCodeFailure{}, false
	}
	return failure, true
}

func (h *bindStatusHub) notifyLocked(code string) {
	for ch := range h.watchers[code] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func normalizeBindStatusCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}
