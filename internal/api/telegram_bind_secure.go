package api

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

// requestIsLoopbackInternal 判定请求是否来自本机回环且未经任何反代转发，即
// “同机受信内部调用”（独立 Bot 进程直连 http://127.0.0.1:<port>）。
//
// 取代 bind-confirm 过去的共享密钥：Bot 与 API 部署在同一台机器、同一网络环境，
// 彼此互信，只需把外部请求挡在门外即可，无需再维护一个容易漏配的密钥
// （漏配会一路 403: INTERNAL_SECRET_INVALID，绑定永远确认不了）。
//
// 为什么不能只看 RemoteAddr 是回环：同机反代（nginx 等）会把外部流量也以
// 127.0.0.1 转进来，RemoteAddr 同样落在回环。区别在于反代必定带上
// X-Forwarded-For / X-Real-IP / CF-Connecting-IP / Forwarded 等转发头，而 Bot
// 直连不带任何代理头。因此“回环对端 + 无转发头”才等于真正的同机内部直连；
// 任何带转发头的请求（= 经反代来的外部流量）一律视为外部、拒绝。
func requestIsLoopbackInternal(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP", "Forwarded"} {
		if strings.TrimSpace(r.Header.Get(header)) != "" {
			return false
		}
	}
	return true
}

func (a *App) handleBindConfirmSecure(w http.ResponseWriter, r *http.Request, _ Params) {
	// 同机内部直连才放行；外部（含经反代转发的外部流量）一律拒绝。
	// 不再校验共享密钥——Bot 与 API 同机互信。
	if !requestIsLoopbackInternal(r) {
		failWithCode(w, http.StatusForbidden, ErrInternalSecretInvalid, "仅允许本机内部调用")
		return
	}
	payload := decodeMap(r)
	code := strings.ToUpper(strings.TrimSpace(stringValue(payload, "code")))
	if !telegramBindCodePattern.MatchString(code) {
		failWithCode(w, http.StatusBadRequest, ErrTGBindCodeFormat, "绑定码格式无效")
		return
	}
	bind, okBind := a.bindCode(code)
	if !okBind || bind.ExpiresAt <= time.Now().Unix() {
		if okBind {
			_ = a.deleteBindCode(code)
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
			a.rejectRegisterBindCode(bind, code, "telegram_taken", ErrTGBindTargetTaken, http.StatusConflict, "绑定码已绑定其他 Telegram，无法重放")
			failWithCode(w, http.StatusConflict, ErrTGBindTargetTaken, "绑定码已绑定其他 Telegram，无法重放")
			return
		}
		if bind.UID == 0 {
			if existing, okUser := a.store().FindUserByTelegramID(telegramID); okUser {
				a.rejectRegisterBindCode(bind, code, "telegram_taken", ErrTGBindTargetTaken, http.StatusConflict, "该 Telegram 已绑定到账号 "+existing.Username)
				failWithCode(w, http.StatusConflict, ErrTGBindTargetTaken, "该 Telegram 已绑定到账号 "+existing.Username)
				return
			}
		}
		ok(w, "绑定已确认", map[string]any{"code": code, "confirmed": true})
		return
	}
	if existing, okUser := a.store().FindUserByTelegramID(telegramID); okUser && (bind.UID == 0 || existing.UID != bind.UID) {
		a.rejectRegisterBindCode(bind, code, "telegram_taken", ErrTGBindTargetTaken, http.StatusConflict, "该 Telegram 已绑定到账号 "+existing.Username)
		failWithCode(w, http.StatusConflict, ErrTGBindTargetTaken, "该 Telegram 已绑定到账号 "+existing.Username)
		return
	}
	// per-tg-id 速率限制：阻止用同一个 tg 账号反复 confirm 不同的合法格式 code
	// 触发 N×getChatMember，对 bot token 做流量放大。沿用 login 桶的 per-minute
	// 配置即可，不需要引入新字段。
	if !a.allowRate(r.Context(), rateKey("tg-bind-confirm:", telegramID), a.cfg().RateLimitLoginPerMinute, time.Minute) {
		a.recordRegisterBindFailure(bind, code, "rate_limited", ErrUploadRateLimited, http.StatusTooManyRequests, "操作过于频繁，请稍后再试")
		failWithCode(w, http.StatusTooManyRequests, ErrUploadRateLimited, "操作过于频繁，请稍后再试")
		return
	}
	if missing, err := a.telegramBindRequirementMissing(r.Context(), telegramID); err != nil {
		a.recordRegisterBindFailure(bind, code, "group_check_failed", ErrTGBindGroupCheckFailed, http.StatusBadGateway, "Telegram 加群/频道校验失败，请稍后重试")
		failWithCode(w, http.StatusBadGateway, ErrTGBindGroupCheckFailed, "Telegram 加群/频道校验失败，请稍后重试")
		return
	} else if len(missing) > 0 {
		a.recordRegisterBindFailure(bind, code, "group_membership_required", ErrTGBindGroupMembershipRequired, http.StatusForbidden, "绑定前需要先加入指定 Telegram 群组/频道: "+strings.Join(missing, ", "))
		failWithCode(w, http.StatusForbidden, ErrTGBindGroupMembershipRequired, "绑定前需要先加入指定 Telegram 群组/频道: "+strings.Join(missing, ", "))
		return
	}
	_, _, _, err := a.confirmBindCodeAtomic(code, telegramID, stringValue(payload, "telegram_username"), time.Now().Unix())
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrExpired) {
		failWithCode(w, http.StatusNotFound, ErrTGBindCodeNotFound, "绑定码不存在或已过期")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		a.rejectRegisterBindCode(bind, code, "conflict", ErrTGBindTargetTaken, http.StatusConflict, "该 Telegram 已绑定到其他账号或绑定码状态已变化")
		failWithCode(w, http.StatusConflict, ErrTGBindTargetTaken, "该 Telegram 已绑定到其他账号或绑定码状态已变化")
		return
	}
	if statusFromError(w, err) {
		return
	}
	a.clearRegisterBindFailure(code)
	ok(w, "绑定已确认", map[string]any{"code": code, "confirmed": true})
}
