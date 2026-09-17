package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/security"
	"github.com/prejudice-studio/twilight/internal/store"
	"github.com/prejudice-studio/twilight/internal/validate"
)

type emailPasswordResetResult struct {
	ResendAfter int
	ExpiresIn   int
}

func (a *App) requestEmailPasswordReset(ctx context.Context, ip, email string) (emailPasswordResetResult, error) {
	email = strings.TrimSpace(email)
	if err := validate.ValidateEmailFormat(email); err != nil {
		return emailPasswordResetResult{}, passwordResetFail(http.StatusBadRequest, ErrEmailInvalid, err.Error())
	}
	if user, found := a.store().FindUserByEmailVerified(email); found && user.Active {
		_, _, _, _ = a.issueEmailCode(ctx, ip, emailPurposeResetPassword, email, user.UID)
	}
	cfg := a.cfg()
	return emailPasswordResetResult{
		ResendAfter: cfg.EmailResendCooldownSeconds,
		ExpiresIn:   cfg.EmailCodeTTLMinutes * 60,
	}, nil
}

func (a *App) resetPasswordByEmail(ctx context.Context, email, code, newPassword string) (store.User, error) {
	email = strings.TrimSpace(email)
	if email == "" || code == "" {
		return store.User{}, passwordResetFail(http.StatusBadRequest, ErrEmailCodeRequired, "请填写邮箱和验证码")
	}
	if err := validate.ValidatePasswordStrength(newPassword); err != nil {
		return store.User{}, passwordResetFail(http.StatusBadRequest, ErrPasswordWeak, err.Error())
	}
	user, found := a.store().FindUserByEmailVerified(email)
	if !found {
		return store.User{}, passwordResetFail(http.StatusBadRequest, ErrEmailCodeInvalid, "验证码无效或已失效，请重新获取")
	}
	record, active := a.store().FindActiveEmailVerification(emailPurposeResetPassword, email, time.Now().Unix())
	if !active {
		return store.User{}, passwordResetFail(http.StatusBadRequest, ErrEmailCodeInvalid, "验证码无效或已失效，请重新获取")
	}
	verified, status, codeName, message := a.verifyEmailCodeByID(record.ID, code)
	if codeName != "" {
		return store.User{}, passwordResetFail(status, codeName, message)
	}
	if verified.UID != user.UID || verified.Purpose != emailPurposeResetPassword {
		return store.User{}, passwordResetFail(http.StatusBadRequest, ErrEmailCodeInvalid, "验证码无效或已失效，请重新获取")
	}
	if !user.Active {
		if userExpiredOnly(user) {
			return store.User{}, passwordResetFail(http.StatusForbidden, ErrAccountExpired, "账号有效期已到期，请续费后再重置密码")
		}
		return store.User{}, passwordResetFail(http.StatusForbidden, ErrAccountDisabled, "账号已被禁用")
	}
	hash, err := security.HashPassword(newPassword)
	if err != nil {
		return store.User{}, passwordResetFail(http.StatusInternalServerError, ErrPasswordHashFailed, "密码处理失败")
	}
	updated, err := a.store().UpdateUser(user.UID, func(existing *store.User) error {
		existing.PasswordHash = hash
		return nil
	})
	if err != nil {
		return store.User{}, err
	}
	a.sessions().DeleteUser(ctx, updated.UID)
	return updated, nil
}

func (a *App) handleEmailPasswordResetRequestResource(w http.ResponseWriter, r *http.Request) {
	cfg := a.cfg()
	if !cfg.ForgotPasswordEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrForgotPasswordDisabled, "找回密码功能已关闭")
		return
	}
	if !cfg.ForgotPasswordEmailEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrForgotPasswordDisabled, "通过邮箱找回密码已关闭")
		return
	}
	if !emailConfigured(cfg) {
		failWithCode(w, http.StatusServiceUnavailable, ErrEmailDisabled, "邮箱功能未启用")
		return
	}
	if !a.allowRate(r.Context(), rateKey("email-reset:ip:", a.clientIP(r)), cfg.RateLimitForgotPasswordIPPer10m, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrPasswordResetTooMany, "重置密码尝试过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	result, err := a.requestEmailPasswordReset(r.Context(), a.clientIP(r), stringValue(payload, "email"))
	if err != nil {
		if failure, ok := err.(*passwordResetFailure); ok {
			failWithCode(w, failure.Status, failure.Code, failure.Message)
			return
		}
		statusFromError(w, err)
		return
	}
	ok(w, "如果该邮箱已绑定账号，验证码已发送，请查收", map[string]any{
		"resend_after": result.ResendAfter,
		"expires_in":   result.ExpiresIn,
	})
}

func (a *App) handleEmailPasswordResetResource(w http.ResponseWriter, r *http.Request) {
	cfg := a.cfg()
	if !cfg.ForgotPasswordEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrForgotPasswordDisabled, "找回密码功能已关闭")
		return
	}
	if !cfg.ForgotPasswordEmailEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrForgotPasswordDisabled, "通过邮箱找回密码已关闭")
		return
	}
	if !emailConfigured(cfg) {
		failWithCode(w, http.StatusServiceUnavailable, ErrEmailDisabled, "邮箱功能未启用")
		return
	}
	if !a.allowRate(r.Context(), rateKey("email-reset:ip:", a.clientIP(r)), cfg.RateLimitForgotPasswordIPPer10m, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrPasswordResetTooMany, "重置密码尝试过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	email := stringValue(payload, "email")
	code := firstNonEmpty(stringValue(payload, "code"), stringValue(payload, "email_code"))
	user, err := a.resetPasswordByEmail(r.Context(), email, code, stringValue(payload, "new_password"))
	if err != nil {
		if failure, ok := err.(*passwordResetFailure); ok {
			failWithCode(w, failure.Status, failure.Code, failure.Message)
			return
		}
		statusFromError(w, err)
		return
	}
	ok(w, "密码已重置，请使用新密码登录", map[string]any{"username": user.Username})
}
