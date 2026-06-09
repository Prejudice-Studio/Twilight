package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/security"
	"github.com/prejudice-studio/twilight/internal/store"
	"github.com/prejudice-studio/twilight/internal/validate"
)

// maskEmail 在响应里对邮箱做局部遮蔽（保留首尾少量字符），用于发码后的回显，
// 避免在共享屏幕 / 日志里完整暴露邮箱。
func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return email
	}
	local, domain := email[:at], email[at:]
	if len(local) <= 2 {
		return local[:1] + "***" + domain
	}
	return local[:2] + "***" + domain
}

// requireEmailVerified 是价值型接口的强制门：强制策略下未验证邮箱的普通/白名单
// 用户被 403 拦截（前端守卫只做体验，这里是不可绕过的服务端防线）。返回 true
// 表示已写出失败响应，调用方应直接 return。
func (a *App) requireEmailVerified(w http.ResponseWriter, user store.User) bool {
	if a.emailVerificationRequired(user) {
		failWithCode(w, http.StatusForbidden, ErrEmailVerificationRequired, "请先在个人中心绑定并验证邮箱后再继续操作")
		return true
	}
	return false
}

// consumePasswordChangeEmailCode 在强制邮箱验证时校验改密所需的邮箱验证码。
// 未强制时直接放行（向后兼容直接改密）。强制时要求 verification_id + email_code
// 命中一条属于本人(uid)、用途匹配的验证记录。返回 true 表示通过。
func (a *App) consumePasswordChangeEmailCode(w http.ResponseWriter, payload map[string]any, user store.User, purpose string) bool {
	if !a.emailGateActive(user) {
		return true
	}
	if !user.EmailVerified || strings.TrimSpace(user.Email) == "" {
		failWithCode(w, http.StatusForbidden, ErrEmailVerificationRequired, "请先在个人中心绑定并验证邮箱")
		return false
	}
	id := stringValue(payload, "verification_id")
	code := firstNonEmpty(stringValue(payload, "email_code"), stringValue(payload, "code"))
	if id == "" || code == "" {
		failWithCode(w, http.StatusBadRequest, ErrEmailCodeRequired, "请先获取并填写邮箱验证码")
		return false
	}
	rec, status, ec, msg := a.verifyEmailCodeByID(id, code)
	if ec != "" {
		failWithCode(w, status, ec, msg)
		return false
	}
	// 防越权：码必须是本人为该用途申请的，避免拿 bind / 其它用途的码改密。
	if rec.UID != user.UID || rec.Purpose != purpose {
		failWithCode(w, http.StatusBadRequest, ErrEmailCodeInvalid, "验证码无效或已失效，请重新获取")
		return false
	}
	return true
}

// handleSendEmailCode 登录态发码：purpose=bind 给新邮箱发码，change_password /
// change_emby_password 给已绑定邮箱发码。
func (a *App) handleSendEmailCode(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	payload := decodeMap(r)
	purpose := strings.TrimSpace(stringValue(payload, "purpose"))
	if purpose == "" {
		purpose = emailPurposeBind
	}
	var targetEmail string
	switch purpose {
	case emailPurposeBind:
		targetEmail = strings.TrimSpace(stringValue(payload, "email"))
		if targetEmail == "" {
			failWithCode(w, http.StatusBadRequest, ErrEmailInvalid, "请填写邮箱")
			return
		}
		if _, ok := a.store().EmailVerifiedOwner(targetEmail, p.User.UID); ok {
			failWithCode(w, http.StatusConflict, ErrEmailConflict, "该邮箱已被其他账号绑定")
			return
		}
	case emailPurposeChangePass:
		if !p.User.EmailVerified || strings.TrimSpace(p.User.Email) == "" {
			failWithCode(w, http.StatusForbidden, ErrEmailNotBound, "请先绑定并验证邮箱")
			return
		}
		targetEmail = p.User.Email
	case emailPurposeChangeEmby:
		if p.User.EmbyID == "" {
			failWithCode(w, http.StatusBadRequest, ErrEmbyAccountUnlinked, "当前账号未关联 Emby")
			return
		}
		if !p.User.EmailVerified || strings.TrimSpace(p.User.Email) == "" {
			failWithCode(w, http.StatusForbidden, ErrEmailNotBound, "请先绑定并验证邮箱")
			return
		}
		targetEmail = p.User.Email
	default:
		failWithCode(w, http.StatusBadRequest, ErrEmailPurposeInvalid, "验证用途无效")
		return
	}
	id, status, ec, msg := a.issueEmailCode(r.Context(), a.clientIP(r), purpose, targetEmail, p.User.UID)
	if ec != "" {
		failWithCode(w, status, ec, msg)
		return
	}
	ok(w, "验证码已发送", map[string]any{
		"verification_id": id,
		"email":           maskEmail(targetEmail),
		"expires_in":      a.cfg().EmailCodeTTLMinutes * 60,
		"resend_after":    a.cfg().EmailResendCooldownSeconds,
	})
}

// handleVerifyEmailCode 登录态校验 bind 验证码并完成邮箱绑定 + 标记已验证。
func (a *App) handleVerifyEmailCode(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	payload := decodeMap(r)
	id := stringValue(payload, "verification_id")
	code := firstNonEmpty(stringValue(payload, "code"), stringValue(payload, "email_code"))
	rec, status, ec, msg := a.verifyEmailCodeByID(id, code)
	if ec != "" {
		failWithCode(w, status, ec, msg)
		return
	}
	if rec.Purpose != emailPurposeBind || rec.UID != p.User.UID {
		failWithCode(w, http.StatusBadRequest, ErrEmailCodeInvalid, "验证码无效或已失效，请重新获取")
		return
	}
	u, err := a.store().SetUserEmailVerifiedAtomic(p.User.UID, rec.Email, true, false, time.Now().Unix())
	if errors.Is(err, store.ErrConflict) {
		failWithCode(w, http.StatusConflict, ErrEmailConflict, "该邮箱已被其他账号绑定")
		return
	}
	if statusFromError(w, err) {
		return
	}
	ok(w, "邮箱验证成功", publicUser(u))
}

// handleForgotPasswordEmailRequest 登出态找回第一步：向已验证邮箱发送重置验证码。
// 防枚举：无论邮箱是否对应账号，都回统一成功（仅 IP 级限流可见）。
func (a *App) handleForgotPasswordEmailRequest(w http.ResponseWriter, r *http.Request, _ Params) {
	cfg := a.cfg()
	if !emailConfigured(cfg) {
		failWithCode(w, http.StatusServiceUnavailable, ErrEmailDisabled, "邮箱功能未启用")
		return
	}
	if !a.allowRate(r.Context(), rateKey("email-reset:ip:", a.clientIP(r)), cfg.RateLimitForgotPasswordIPPer10m, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrPasswordResetTooMany, "重置密码尝试过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	email := strings.TrimSpace(stringValue(payload, "email"))
	if err := validate.ValidateEmailFormat(email); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrEmailInvalid, err.Error())
		return
	}
	// 仅当邮箱已被某账号验证时才真正发码；否则静默成功，避免账号枚举。
	if user, found := a.store().FindUserByEmailVerified(email); found && user.Active {
		// 发码内部失败（限流 / 冷却 / SMTP 故障）一律吞掉只记不抛，保持统一成功，
		// 防止以"邮箱级限流提示"反推账号存在。
		if _, _, ec, _ := a.issueEmailCode(r.Context(), a.clientIP(r), emailPurposeResetPassword, email, user.UID); ec != "" {
			_ = ec
		}
	}
	ok(w, "如果该邮箱已绑定账号，验证码已发送，请查收", map[string]any{
		"resend_after": cfg.EmailResendCooldownSeconds,
		"expires_in":   cfg.EmailCodeTTLMinutes * 60,
	})
}

// handleForgotPasswordEmailReset 登出态找回第二步：校验验证码并重置系统密码。
func (a *App) handleForgotPasswordEmailReset(w http.ResponseWriter, r *http.Request, _ Params) {
	cfg := a.cfg()
	if !emailConfigured(cfg) {
		failWithCode(w, http.StatusServiceUnavailable, ErrEmailDisabled, "邮箱功能未启用")
		return
	}
	if !a.allowRate(r.Context(), rateKey("email-reset:ip:", a.clientIP(r)), cfg.RateLimitForgotPasswordIPPer10m, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrPasswordResetTooMany, "重置密码尝试过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	email := strings.TrimSpace(stringValue(payload, "email"))
	code := firstNonEmpty(stringValue(payload, "code"), stringValue(payload, "email_code"))
	newPassword := stringValue(payload, "new_password")
	if email == "" || code == "" {
		failWithCode(w, http.StatusBadRequest, ErrEmailCodeRequired, "请填写邮箱和验证码")
		return
	}
	if err := validate.ValidatePasswordStrength(newPassword); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrPasswordWeak, err.Error())
		return
	}
	user, found := a.store().FindUserByEmailVerified(email)
	var rec store.EmailVerification
	active := false
	if found {
		rec, active = a.store().FindActiveEmailVerification(emailPurposeResetPassword, email, time.Now().Unix())
	}
	if !found || !active {
		// 不区分"账号不存在"与"无有效验证码"，避免账号枚举。
		failWithCode(w, http.StatusBadRequest, ErrEmailCodeInvalid, "验证码无效或已失效，请重新获取")
		return
	}
	verified, status, ec, msg := a.verifyEmailCodeByID(rec.ID, code)
	if ec != "" {
		failWithCode(w, status, ec, msg)
		return
	}
	if verified.UID != user.UID || verified.Purpose != emailPurposeResetPassword {
		failWithCode(w, http.StatusBadRequest, ErrEmailCodeInvalid, "验证码无效或已失效，请重新获取")
		return
	}
	// 码已校验（证明邮箱归属），再做账号状态守卫，与 Emby 找回口径一致。
	if !user.Active {
		if userExpiredOnly(user) {
			failWithCode(w, http.StatusForbidden, ErrAccountExpired, "账号有效期已到期，请续费后再重置密码")
			return
		}
		failWithCode(w, http.StatusForbidden, ErrAccountDisabled, "账号已被禁用")
		return
	}
	hash, err := security.HashPassword(newPassword)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrPasswordHashFailed, "密码处理失败")
		return
	}
	u, err := a.store().UpdateUser(user.UID, func(u *store.User) error { u.PasswordHash = hash; return nil })
	if statusFromError(w, err) {
		return
	}
	a.sessions().DeleteUser(r.Context(), u.UID)
	ok(w, "密码已重置，请使用新密码登录", map[string]any{"username": u.Username})
}

// handleAdminBindUserEmail 管理员强制把用户绑定到指定邮箱（默认直接标记已验证）。
// force=true 时跳过黑白名单与占用冲突校验（管理员断言归属）。
func (a *App) handleAdminBindUserEmail(w http.ResponseWriter, r *http.Request, params Params) {
	uid, err := int64Param(params, "uid")
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "用户 ID 无效")
		return
	}
	if _, ok := a.store().User(uid); !ok {
		failWithCode(w, http.StatusNotFound, ErrUserNotFound, userNotFoundMessage)
		return
	}
	payload := decodeMap(r)
	email := strings.TrimSpace(stringValue(payload, "email"))
	markVerified := boolValue(payload, "mark_verified", true)
	force := boolValue(payload, "force", false)
	if email == "" {
		failWithCode(w, http.StatusBadRequest, ErrEmailInvalid, "请填写邮箱")
		return
	}
	if err := validate.ValidateEmailFormat(email); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrEmailInvalid, err.Error())
		return
	}
	if !force {
		if len(a.cfg().EmailBlacklist) > 0 && validate.CheckEmailBlacklist(email, a.cfg().EmailBlacklist) {
			failWithCode(w, http.StatusBadRequest, ErrEmailInvalid, "该邮箱域名不在允许范围内（可勾选强制覆盖）")
			return
		}
		if len(a.cfg().EmailWhitelist) > 0 && !validate.CheckEmailWhitelist(email, a.cfg().EmailWhitelist) {
			failWithCode(w, http.StatusBadRequest, ErrEmailInvalid, "该邮箱域名不在允许范围内（可勾选强制覆盖）")
			return
		}
	}
	u, err := a.store().SetUserEmailVerifiedAtomic(uid, email, markVerified, force, time.Now().Unix())
	if errors.Is(err, store.ErrConflict) {
		failWithCode(w, http.StatusConflict, ErrEmailConflict, "该邮箱已被其他账号验证绑定（可勾选强制覆盖）")
		return
	}
	if statusFromError(w, err) {
		return
	}
	ok(w, "已绑定邮箱", publicUser(u))
}

// handleAdminSetUserEmailVerified 管理员手动置/撤销某用户的邮箱验证状态（不改邮箱）。
func (a *App) handleAdminSetUserEmailVerified(w http.ResponseWriter, r *http.Request, params Params) {
	uid, err := int64Param(params, "uid")
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "用户 ID 无效")
		return
	}
	target, exists := a.store().User(uid)
	if !exists {
		failWithCode(w, http.StatusNotFound, ErrUserNotFound, userNotFoundMessage)
		return
	}
	payload := decodeMap(r)
	verified := boolValue(payload, "verified", false)
	force := boolValue(payload, "force", false)
	if verified && strings.TrimSpace(target.Email) == "" {
		failWithCode(w, http.StatusBadRequest, ErrEmailNotBound, "该用户尚未绑定邮箱，无法标记为已验证")
		return
	}
	u, err := a.store().SetUserEmailVerifiedAtomic(uid, "", verified, force, time.Now().Unix())
	if errors.Is(err, store.ErrConflict) {
		failWithCode(w, http.StatusConflict, ErrEmailConflict, "该邮箱已被其他账号验证绑定（可勾选强制覆盖）")
		return
	}
	if statusFromError(w, err) {
		return
	}
	ok(w, "已更新邮箱验证状态", publicUser(u))
}

// handleAdminEmailTest 管理员发送测试邮件，验证 SMTP 配置是否可用。
func (a *App) handleAdminEmailTest(w http.ResponseWriter, r *http.Request, _ Params) {
	cfg := a.cfg()
	results := []map[string]any{}
	if !emailConfigured(cfg) {
		results = append(results, map[string]any{"target": "配置", "success": false, "error": "邮箱功能未启用或 SMTP 未配置完整"})
		ok(w, "测试完成", map[string]any{"results": results})
		return
	}
	payload := decodeMap(r)
	to := strings.TrimSpace(stringValue(payload, "to"))
	if to == "" {
		to = firstNonEmpty(cfg.SMTPFromAddress, cfg.SMTPUsername)
	}
	if err := validate.ValidateEmailFormat(to); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrEmailInvalid, "收件邮箱格式不正确")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	site := firstNonEmpty(cfg.AppName, "Twilight")
	subject := site + " 邮件发送测试"
	body := "这是一封来自 " + site + " 的 SMTP 测试邮件。\n\n如果你收到了它，说明邮件发送配置正常。"
	if err := smtpDeliver(ctx, *cfg, to, subject, body); err != nil {
		results = append(results, map[string]any{"target": "SMTP 发信", "success": false, "error": err.Error()})
		ok(w, "测试完成", map[string]any{"results": results})
		return
	}
	results = append(results, map[string]any{"target": "SMTP 发信", "success": true, "to": maskEmail(to)})
	ok(w, "测试完成", map[string]any{"results": results})
}

// emailVerificationDTO 把一条在用验证码记录脱敏成管理员可见的审查 DTO：
// 永不下发 CodeHash（HMAC 摘要也不外泄），解析关联本地账号用户名，标注是否已过期。
func (a *App) emailVerificationDTO(v store.EmailVerification, now int64) map[string]any {
	username := ""
	if v.UID != 0 {
		if u, okUser := a.store().User(v.UID); okUser {
			username = u.Username
		}
	}
	return map[string]any{
		"id":           v.ID,
		"purpose":      v.Purpose,
		"email":        v.Email,
		"email_masked": maskEmail(v.Email),
		"uid":          nullableInt(v.UID),
		"username":     emptyNil(username),
		"attempts":     v.Attempts,
		"max_attempts": v.MaxAttempts,
		"created_at":   v.CreatedAt,
		"expires_at":   v.ExpiresAt,
		"last_sent_at": v.LastSentAt,
		"expired":      v.ExpiresAt > 0 && v.ExpiresAt <= now,
	}
}

// handleAdminEmailVerifications 汇总邮箱管理审查数据：在用验证码记录（脱敏）+ 已设置
// 邮箱的账号清单（含验证状态/时间）+ 统计与 SMTP/强制策略状态。仅 AuthAdmin。
func (a *App) handleAdminEmailVerifications(w http.ResponseWriter, r *http.Request, _ Params) {
	cfg := a.cfg()
	now := time.Now().Unix()

	records := a.store().ListEmailVerifications()
	// 按最近发码时间倒序，最新的在前；前端可再排序。
	sort.SliceStable(records, func(i, j int) bool { return records[i].LastSentAt > records[j].LastSentAt })
	pending := make([]map[string]any, 0, len(records))
	expiredPending := 0
	for _, v := range records {
		if v.ExpiresAt > 0 && v.ExpiresAt <= now {
			expiredPending++
		}
		pending = append(pending, a.emailVerificationDTO(v, now))
	}

	accounts := []map[string]any{}
	verified := 0
	for _, u := range a.store().ListUsers() {
		if strings.TrimSpace(u.Email) == "" {
			continue
		}
		if u.EmailVerified {
			verified++
		}
		accounts = append(accounts, map[string]any{
			"uid":               u.UID,
			"username":          u.Username,
			"email":             u.Email,
			"email_verified":    u.EmailVerified,
			"email_verified_at": zeroNil(u.EmailVerifiedAt),
			"telegram_id":       nullableInt(u.TelegramID),
			"telegram_username": emptyNil(u.TelegramUsername),
			"role":              u.Role,
			"active":            u.Active,
		})
	}

	ok(w, "OK", map[string]any{
		"smtp_configured": emailConfigured(cfg),
		"email_enabled":   cfg.EmailEnabled,
		"force_bind":      cfg.EmailForceBind,
		"pending":         pending,
		"accounts":        accounts,
		"summary": map[string]any{
			"total_pending":    len(pending),
			"expired_pending":  expiredPending,
			"total_with_email": len(accounts),
			"verified":         verified,
			"unverified":       len(accounts) - verified,
		},
	})
}

// handleAdminDeleteEmailVerification 撤销一条在用验证码记录（让对应验证码立即失效）。仅 AuthAdmin。
func (a *App) handleAdminDeleteEmailVerification(w http.ResponseWriter, r *http.Request, params Params) {
	id := strings.TrimSpace(params["id"])
	if id == "" {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "缺少验证记录 id")
		return
	}
	if err := a.store().DeleteEmailVerification(id); statusFromError(w, err) {
		return
	}
	ok(w, "已撤销验证记录", map[string]any{"id": id})
}

// handleAdminCleanupEmailVerifications 手动清理所有已过期的在用验证码（与调度任务同口径）。仅 AuthAdmin。
func (a *App) handleAdminCleanupEmailVerifications(w http.ResponseWriter, r *http.Request, _ Params) {
	deleted, err := a.store().CleanupExpiredEmailVerifications(time.Now().Unix())
	if statusFromError(w, err) {
		return
	}
	ok(w, "已清理过期验证码", map[string]any{"deleted": deleted})
}
