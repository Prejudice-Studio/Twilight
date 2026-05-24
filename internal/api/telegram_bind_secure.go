package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleBindConfirmSecure(w http.ResponseWriter, r *http.Request, _ Params) {
	secret := firstNonEmpty(r.Header.Get("X-Internal-Secret"), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if a.cfg.BotInternalSecret == "" || secret != a.cfg.BotInternalSecret {
		fail(w, http.StatusForbidden, "内部密钥无效")
		return
	}
	payload := decodeMap(r)
	code := strings.ToUpper(strings.TrimSpace(stringValue(payload, "code")))
	if !telegramBindCodePattern.MatchString(code) {
		fail(w, http.StatusBadRequest, "绑定码格式无效")
		return
	}
	bind, okBind := a.store.BindCode(code)
	if !okBind || bind.ExpiresAt <= time.Now().Unix() {
		if okBind {
			_ = a.store.DeleteBindCode(code)
		}
		fail(w, http.StatusNotFound, "绑定码不存在或已过期")
		return
	}
	telegramID := int64(intValue(payload, "telegram_id", 0))
	if telegramID == 0 {
		fail(w, http.StatusBadRequest, "Telegram ID 无效")
		return
	}
	if existing, okUser := a.store.FindUserByTelegramID(telegramID); okUser && (bind.UID == 0 || existing.UID != bind.UID) {
		fail(w, http.StatusConflict, "该 Telegram 已绑定到账号 "+existing.Username)
		return
	}
	if missing, err := a.telegramBindRequirementMissing(r.Context(), telegramID); err != nil {
		fail(w, http.StatusBadGateway, "Telegram 加群/频道校验失败，请稍后重试")
		return
	} else if len(missing) > 0 {
		fail(w, http.StatusForbidden, "绑定前需要先加入指定 Telegram 群组/频道: "+strings.Join(missing, ", "))
		return
	}
	bind.Confirmed = true
	bind.TelegramID = telegramID
	bind.TelegramUsername = strings.TrimSpace(stringValue(payload, "telegram_username"))
	_ = a.store.UpsertBindCode(bind)
	if bind.UID != 0 {
		_, err := a.store.UpdateUser(bind.UID, func(u *store.User) error {
			u.TelegramID = telegramID
			u.TelegramUsername = bind.TelegramUsername
			return nil
		})
		if statusFromError(w, err) {
			return
		}
		_ = a.store.DeleteBindCode(code)
	}
	ok(w, "绑定已确认", map[string]any{"code": code, "confirmed": true})
}
