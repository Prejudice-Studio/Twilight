package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/security"
	"github.com/prejudice-studio/twilight/internal/store"
)

type embyPasswordResetInput struct {
	Username string
	Password string
}

type passwordResetFailure struct {
	Status  int
	Code    ErrCode
	Message string
}

func (e *passwordResetFailure) Error() string { return e.Message }

func passwordResetFail(status int, code ErrCode, message string) error {
	return &passwordResetFailure{Status: status, Code: code, Message: message}
}

func (a *App) resetPasswordByEmby(ctx context.Context, input embyPasswordResetInput) (store.User, string, error) {
	if input.Username == "" || input.Password == "" {
		return store.User{}, "", passwordResetFail(http.StatusBadRequest, ErrEmbyMissingCreds, "缺少 Emby 用户名或密码")
	}
	if len(input.Username) > 100 || len(input.Password) > 200 {
		return store.User{}, "", passwordResetFail(http.StatusBadRequest, ErrEmbyInputTooLong, "输入内容过长")
	}
	embyUser, authenticated, err := a.embyAuthenticateByName(ctx, input.Username, input.Password)
	if err != nil {
		return store.User{}, "", passwordResetFail(http.StatusUnauthorized, ErrEmbyAuthFailed, "Emby 鉴权失败")
	}
	if !authenticated {
		return store.User{}, "", passwordResetFail(http.StatusUnauthorized, ErrLoginInvalid, "Emby 用户名或密码错误")
	}
	embyID := firstNonEmpty(asString(embyUser["Id"]), asString(embyUser["ID"]), asString(embyUser["id"]))
	user, found := a.store().FindUserByEmbyID(embyID)
	if !found {
		return store.User{}, "", passwordResetFail(http.StatusNotFound, ErrEmbyAccountUnlinked, "该 Emby 账号未关联面板账号")
	}
	if !user.Active {
		if userExpiredOnly(user) {
			return store.User{}, "", passwordResetFail(http.StatusForbidden, ErrAccountExpired, "账号有效期已到期，请续费后再重置密码")
		}
		return store.User{}, "", passwordResetFail(http.StatusForbidden, ErrAccountDisabled, "账号已被禁用")
	}
	if !userEntitlementOK(user) {
		return store.User{}, "", passwordResetFail(http.StatusForbidden, ErrAccountExpired, "账号有效期已到期，请先续期再重置密码")
	}
	newPassword := "Twilight-" + randomCode(18)
	hash, err := security.HashPassword(newPassword)
	if err != nil {
		return store.User{}, "", passwordResetFail(http.StatusInternalServerError, ErrPasswordHashFailed, "密码处理失败")
	}
	updated, err := a.store().UpdateUser(user.UID, func(existing *store.User) error {
		existing.PasswordHash = hash
		return nil
	})
	if err != nil {
		return store.User{}, "", err
	}
	a.sessions().DeleteUser(ctx, updated.UID)
	return updated, newPassword, nil
}

func (a *App) handleForgotPasswordResource(w http.ResponseWriter, r *http.Request) {
	cfg := a.cfg()
	if !cfg.ForgotPasswordEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrForgotPasswordDisabled, "找回密码功能已关闭")
		return
	}
	if !cfg.ForgotPasswordEmbyEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrForgotPasswordDisabled, "通过 Emby 找回密码已关闭")
		return
	}
	ip := a.clientIP(r)
	if !a.allowRate(r.Context(), rateKey("forgot-password:ip:", ip), cfg.RateLimitForgotPasswordIPPer10m, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrPasswordResetTooMany, "重置密码尝试过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	input := embyPasswordResetInput{
		Username: stringValue(payload, "emby_username"),
		Password: stringValue(payload, "emby_password"),
	}
	if !a.allowRate(r.Context(), rateKey("forgot-password:user:", strings.ToLower(input.Username)), cfg.RateLimitForgotPasswordUserPer30m, 30*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrPasswordResetTooMany, "该账号重置密码尝试过于频繁")
		return
	}
	user, newPassword, err := a.resetPasswordByEmby(r.Context(), input)
	if err != nil {
		if failure, ok := err.(*passwordResetFailure); ok {
			failWithCode(w, failure.Status, failure.Code, failure.Message)
			return
		}
		statusFromError(w, err)
		return
	}
	ok(w, "密码已重置", map[string]any{"username": user.Username, "new_password": newPassword})
}
