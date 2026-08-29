package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
	"github.com/prejudice-studio/twilight/internal/validate"
)

// 邮箱验证码用途常量（与 store.EmailVerification.Purpose 对齐）。
const (
	emailPurposeBind          = "bind"
	emailPurposeResetPassword = "reset_password"
	emailPurposeChangePass    = "change_password"
	emailPurposeChangeEmby    = "change_emby_password"
	emailPurposeDelAccount    = "del_account"
)

func validEmailPurpose(p string) bool {
	switch p {
	case emailPurposeBind, emailPurposeResetPassword, emailPurposeChangePass, emailPurposeChangeEmby, emailPurposeDelAccount:
		return true
	}
	return false
}

// emailCodeProcessSecret 是 cfg.BotInternalSecret 为空时的进程级兜底 HMAC 密钥。
// 进程内稳定（一次生成），重启后失效——验证码 TTL 很短，重启让在飞码作废属可接受
// 的 fail-closed 行为，而不是退回可预测密钥。
var (
	emailCodeSecretOnce sync.Once
	emailCodeSecretVal  []byte
)

func emailCodeProcessSecret() []byte {
	emailCodeSecretOnce.Do(func() {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			panic(fmt.Sprintf("crypto/rand failure (email code secret): %v", err))
		}
		emailCodeSecretVal = buf
	})
	return emailCodeSecretVal
}

// emailCodeSecret 优先用配置里的 BotInternalSecret 派生（跨重启稳定），缺省回退到
// 进程级随机密钥。加前缀做域分隔，避免和其它复用 BotInternalSecret 的场景撞用途。
func (a *App) emailCodeSecret() []byte {
	if s := strings.TrimSpace(a.cfg().BotInternalSecret); s != "" {
		return []byte("twilight-email-code|" + s)
	}
	return emailCodeProcessSecret()
}

// hashEmailCode 用服务端密钥对 (id|code) 做 HMAC-SHA256，返回 hex。store 只存这个
// 哈希并做定长常量时间比较；即使 state 泄露也读不出明文码，配合短码 + 尝试上限 +
// 短 TTL，离线爆破窗口受限。
func (a *App) hashEmailCode(id, code string) string {
	mac := hmac.New(sha256.New, a.emailCodeSecret())
	mac.Write([]byte(id))
	mac.Write([]byte("|"))
	mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

// emailCodeAlnum 去掉了易混字符（I/O/0/1），便于用户手抄。
const (
	emailCodeDigits = "0123456789"
	emailCodeAlnum  = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

func (a *App) generateEmailCode() string {
	length := a.cfg().EmailCodeLength
	if length < 4 {
		length = 6
	}
	if length > 12 {
		length = 12
	}
	alphabet := emailCodeDigits
	if strings.ToLower(strings.TrimSpace(a.cfg().EmailCodeType)) == "alphanumeric" {
		alphabet = emailCodeAlnum
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("crypto/rand failure (email code): %v", err))
	}
	n := byte(len(alphabet))
	out := make([]byte, length)
	for i := range buf {
		out[i] = alphabet[buf[i]%n]
	}
	return string(out)
}

func (a *App) renderEmailCodeMessage(code string, ttlMinutes int) (string, string) {
	cfg := a.cfg()
	site := strings.TrimSpace(cfg.AppName)
	if site == "" {
		site = "Twilight"
	}
	subjTmpl := firstNonEmpty(cfg.EmailSubjectTemplate, config.DefaultEmailSubjectTemplate)
	bodyTmpl := firstNonEmpty(cfg.EmailBodyTemplate, config.DefaultEmailBodyTemplate)
	rep := strings.NewReplacer("{site}", site, "{code}", code, "{ttl}", strconv.Itoa(ttlMinutes))
	return rep.Replace(subjTmpl), rep.Replace(bodyTmpl)
}

// issueEmailCode 校验邮箱、限流、冷却，生成并投递验证码、落库。返回
// (verificationID, httpStatus, errCode, message)，成功时 errCode 为空、status=200。
// 校验顺序为先廉价校验（格式/名单）后限流后冷却，最后才发信，避免无谓投递。
func (a *App) issueEmailCode(ctx context.Context, ip, purpose, email string, uid int64) (string, int, ErrCode, string) {
	cfg := a.cfg()
	if !emailConfigured(cfg) {
		return "", http.StatusServiceUnavailable, ErrEmailDisabled, "邮箱功能未启用"
	}
	if !validEmailPurpose(purpose) {
		return "", http.StatusBadRequest, ErrEmailPurposeInvalid, "验证用途无效"
	}
	email = strings.TrimSpace(email)
	if err := validate.ValidateEmailFormat(email); err != nil {
		return "", http.StatusBadRequest, ErrEmailInvalid, err.Error()
	}
	if len(cfg.EmailBlacklist) > 0 && validate.CheckEmailBlacklist(email, cfg.EmailBlacklist) {
		return "", http.StatusBadRequest, ErrEmailInvalid, "该邮箱域名不在允许范围内"
	}
	if len(cfg.EmailWhitelist) > 0 && !validate.CheckEmailWhitelist(email, cfg.EmailWhitelist) {
		return "", http.StatusBadRequest, ErrEmailInvalid, "该邮箱域名不在允许范围内"
	}
	if !a.allowRate(ctx, rateKey("email-code:ip:", ip), cfg.RateLimitEmailCodeIPPer10m, 10*time.Minute) {
		return "", http.StatusTooManyRequests, ErrEmailRateLimited, "验证码请求过于频繁，请稍后再试"
	}
	// 单账号限流：登录态发码（绑定 / 改密）按 uid 计数，防止同一账号轮换不同收件
	// 邮箱绕过「按地址」限流来滥刷发信。uid<=0（理论上不会出现）或限额<=0 时跳过。
	if uid > 0 && !a.allowRate(ctx, rateKey("email-code:uid:", strconv.FormatInt(uid, 10)), cfg.RateLimitEmailCodeUIDPer10m, 10*time.Minute) {
		return "", http.StatusTooManyRequests, ErrEmailRateLimited, "该账号验证码请求过于频繁，请稍后再试"
	}
	if !a.allowRate(ctx, rateKey("email-code:addr:", strings.ToLower(email)), cfg.RateLimitEmailCodeAddrPer10m, 10*time.Minute) {
		return "", http.StatusTooManyRequests, ErrEmailRateLimited, "该邮箱验证码请求过于频繁，请稍后再试"
	}
	now := time.Now().Unix()
	cooldown := int64(cfg.EmailResendCooldownSeconds)
	if cooldown > 0 {
		if existing, ok := a.store().FindActiveEmailVerification(purpose, email, now); ok {
			if wait := existing.LastSentAt + cooldown - now; wait > 0 {
				return "", http.StatusTooManyRequests, ErrEmailResendCooldown, fmt.Sprintf("请 %d 秒后再获取验证码", wait)
			}
		}
	}
	ttlMin := cfg.EmailCodeTTLMinutes
	if ttlMin <= 0 {
		ttlMin = 10
	}
	maxAttempts := cfg.EmailMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	code := a.generateEmailCode()
	id := randomCode(32)
	rec := store.EmailVerification{
		ID:          id,
		Purpose:     purpose,
		Email:       email,
		UID:         uid,
		CodeHash:    a.hashEmailCode(id, code),
		MaxAttempts: maxAttempts,
		CreatedAt:   now,
		ExpiresAt:   now + int64(ttlMin)*60,
		LastSentAt:  now,
	}
	// 先发信、成功后落库：发信失败不在 store 里留下用户永远拿不到的"幽灵码"。
	subject, body := a.renderEmailCodeMessage(code, ttlMin)
	if err := smtpDeliver(ctx, *cfg, email, subject, body); err != nil {
		// 错误已在 smtpDeliver 内经 redactSensitiveText 脱敏，这里只记不向用户透传
		// SMTP 原始拒绝原因（可能含配额提示 / 服务器主机名等）。
		zap.L().Warn("send email verification code failed", zap.String("purpose", purpose), zap.Error(err))
		// bind 类发码失败：清理"幽灵绑定"——删除可能残留的待验证记录，并把仍未验证
		// 的同地址绑定邮箱清空，避免用户卡在"已填邮箱却永远收不到验证码"的状态。
		// reset_password / change_password / del_account 等路径不清理：那些邮箱通常
		// 已验证或不涉及绑定写入，清空会误删已验证邮箱或破坏找回流程。
		if purpose == emailPurposeBind {
			a.purgeFailedBindEmail(uid, email)
		}
		// 普通用户只能看到统一失败文案；SMTP 主机、网络地址、认证失败原因和
		// 服务商返回内容不得通过响应暴露。管理员测试接口另有受控诊断结果。
		return "", http.StatusBadGateway, ErrEmailSendFailed, "邮件发送失败，请稍后重试，如持续失败请联系管理员"
	}
	if err := a.store().PutEmailVerification(rec); err != nil {
		return "", http.StatusInternalServerError, ErrInternal, "验证码保存失败"
	}
	return id, http.StatusOK, "", ""
}

// purgeFailedBindEmail 在 bind 类验证码发送失败后清理"幽灵绑定"：
//   - 删除该地址在 bind 用途下尚未消费的活跃验证记录（重发失败时的残留）；
//   - 仅当用户邮箱仍未验证且正是该地址时清空 Email/EmailVerified，避免误删已验证邮箱。
//
// 该操作幂等且容错：uid<=0 或 email 为空时直接返回；store 错误只吞掉不影响发码失败响应。
func (a *App) purgeFailedBindEmail(uid int64, email string) {
	email = strings.TrimSpace(email)
	if uid <= 0 || email == "" {
		return
	}
	if rec, ok := a.store().FindActiveEmailVerification(emailPurposeBind, email, time.Now().Unix()); ok {
		_ = a.store().DeleteEmailVerification(rec.ID)
	}
	if u, ok := a.store().User(uid); ok && !u.EmailVerified && strings.EqualFold(strings.TrimSpace(u.Email), email) {
		_, _ = a.store().UpdateUser(uid, func(user *store.User) error {
			user.Email = ""
			user.EmailVerified = false
			user.EmailVerifiedAt = 0
			return nil
		})
	}
}

// verifyEmailCodeByID 用 verification_id + code 校验（登录态绑定 / 改密路径）。
func (a *App) verifyEmailCodeByID(id, code string) (store.EmailVerification, int, ErrCode, string) {
	now := time.Now().Unix()
	id = strings.TrimSpace(id)
	code = strings.TrimSpace(code)
	if id == "" || code == "" {
		return store.EmailVerification{}, http.StatusBadRequest, ErrEmailCodeRequired, "验证码不能为空"
	}
	candidate := a.hashEmailCode(id, code)
	rec, result, err := a.store().ConsumeEmailVerificationAtomic(id, candidate, now)
	if err != nil {
		return store.EmailVerification{}, http.StatusInternalServerError, ErrInternal, "验证码校验失败"
	}
	return emailVerifyResultToResponse(rec, result)
}

func emailVerifyResultToResponse(rec store.EmailVerification, result store.EmailVerificationResult) (store.EmailVerification, int, ErrCode, string) {
	switch result {
	case store.EmailVerificationOK:
		return rec, http.StatusOK, "", ""
	case store.EmailVerificationExpired:
		return rec, http.StatusBadRequest, ErrEmailCodeExpired, "验证码已过期，请重新获取"
	case store.EmailVerificationTooMany:
		return rec, http.StatusTooManyRequests, ErrEmailCodeTooMany, "验证码错误次数过多，请重新获取"
	case store.EmailVerificationMismatch:
		return rec, http.StatusBadRequest, ErrEmailCodeInvalid, "验证码错误"
	default: // NotFound
		return rec, http.StatusBadRequest, ErrEmailCodeInvalid, "验证码无效或已失效，请重新获取"
	}
}

// emailGateActive 判定某用户是否处于"强制邮箱验证"约束下：邮箱子系统可用 +
// force_bind 开启 + 角色为普通/白名单。管理员不被强制（仅前端提示）。
func (a *App) emailGateActive(u store.User) bool {
	cfg := a.cfg()
	if !emailConfigured(cfg) || !cfg.EmailForceBind {
		return false
	}
	return u.Role == store.RoleNormal || u.Role == store.RoleWhitelist
}

// emailVerificationRequired 表示该用户受强制约束但尚未验证邮箱，应被引导去验证、
// 并被价值型接口拒绝。
func (a *App) emailVerificationRequired(u store.User) bool {
	return a.emailGateActive(u) && !u.EmailVerified
}
