package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleBindConfirmSecure(w http.ResponseWriter, r *http.Request, _ Params) {
	secret := firstNonEmpty(r.Header.Get("X-Internal-Secret"), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if a.cfg().BotInternalSecret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(a.cfg().BotInternalSecret)) != 1 {
		failWithCode(w, http.StatusForbidden, ErrInternalSecretInvalid, "内部密钥无效")
		return
	}
	payload := decodeMap(r)
	code := strings.ToUpper(strings.TrimSpace(stringValue(payload, "code")))
	if !telegramBindCodePattern.MatchString(code) {
		failWithCode(w, http.StatusBadRequest, ErrTGBindCodeFormat, "绑定码格式无效")
		return
	}
	bind, okBind := a.store().BindCode(code)
	if !okBind || bind.ExpiresAt <= time.Now().Unix() {
		if okBind {
			_ = a.store().DeleteBindCode(code)
		}
		failWithCode(w, http.StatusNotFound, ErrTGBindCodeNotFound, "绑定码不存在或已过期")
		return
	}
	telegramID := int64(intValue(payload, "telegram_id", 0))
	if telegramID == 0 {
		failWithCode(w, http.StatusBadRequest, ErrTGBindTGIDInvalid, "Telegram ID 无效")
		return
	}
	// 幂等：注册流（bind.UID == 0）下 confirm 写完后 BindCode 仍会留到 ExpiresAt
	// 才被注册接口消费删除，期间窃听者拿到 code 重放可以把 telegramID 改写。
	// 第二次进来若已 Confirmed：
	//   - 同一 telegramID → 视作幂等成功，跳过 group check 与 store 写入；
	//   - 不同 telegramID → 拒绝，避免转绑攻击。
	if bind.Confirmed && bind.TelegramID != 0 {
		if bind.TelegramID != telegramID {
			failWithCode(w, http.StatusConflict, ErrTGBindTargetTaken, "绑定码已绑定其他 Telegram，无法重放")
			return
		}
		ok(w, "绑定已确认", map[string]any{"code": code, "confirmed": true})
		return
	}
	if existing, okUser := a.store().FindUserByTelegramID(telegramID); okUser && (bind.UID == 0 || existing.UID != bind.UID) {
		failWithCode(w, http.StatusConflict, ErrTGBindTargetTaken, "该 Telegram 已绑定到账号 "+existing.Username)
		return
	}
	// per-tg-id 速率限制：阻止用同一个 tg 账号反复 confirm 不同的合法格式 code
	// 触发 N×getChatMember，对 bot token 做流量放大。沿用 login 桶的 per-minute
	// 配置即可，不需要引入新字段。
	if !a.allowRate(r.Context(), rateKey("tg-bind-confirm:", telegramID), a.cfg().RateLimitLoginPerMinute, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrUploadRateLimited, "操作过于频繁，请稍后再试")
		return
	}
	if missing, err := a.telegramBindRequirementMissing(r.Context(), telegramID); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrTGBindGroupCheckFailed, "Telegram 加群/频道校验失败，请稍后重试")
		return
	} else if len(missing) > 0 {
		failWithCode(w, http.StatusForbidden, ErrTGBindGroupMembershipRequired, "绑定前需要先加入指定 Telegram 群组/频道: "+strings.Join(missing, ", "))
		return
	}
	bind.Confirmed = true
	bind.TelegramID = telegramID
	bind.TelegramUsername = strings.TrimSpace(stringValue(payload, "telegram_username"))
	_ = a.store().UpsertBindCode(bind)
	if bind.UID != 0 {
		_, err := a.store().UpdateUser(bind.UID, func(u *store.User) error {
			u.TelegramID = telegramID
			u.TelegramUsername = bind.TelegramUsername
			return nil
		})
		if statusFromError(w, err) {
			return
		}
		_ = a.store().DeleteBindCode(code)
	}
	ok(w, "绑定已确认", map[string]any{"code": code, "confirmed": true})
}
