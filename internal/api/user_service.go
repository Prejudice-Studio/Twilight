package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"github.com/prejudice-studio/twilight/internal/validate"
)

type userService struct {
	app *App
}

type updateProfileResult struct {
	User store.User
}

type updateUsernameResult struct {
	User store.User
}

type renewResult struct {
	User       store.User
	ExpiredAt  int64
	ExpireStatus string
}

// updateProfile 更新用户资料（邮箱、用户名、Bangumi 设置、通知偏好等）
func (s *userService) updateProfile(ctx context.Context, user store.User, payload map[string]any) (updateProfileResult, error) {
	bgmModeSet := false
	bgmModeNext := false
	bgmManageModeSet := false
	bgmManageModeNext := false
	notifyLoginTelegramSet := false
	notifyLoginTelegramNext := false
	notifyLoginEmailSet := false
	notifyLoginEmailNext := false
	notifyTicketTelegramSet := false
	notifyTicketTelegramNext := false
	signinAutoRenewalSet := false
	signinAutoRenewalNext := false
	passwordChangeEmailRequiredSet := false
	passwordChangeEmailRequiredNext := false
	embyPasswordEmailRequiredSet := false
	embyPasswordEmailRequiredNext := false
	embyPasswordOldPasswordRequiredSet := false
	embyPasswordOldPasswordRequiredNext := false
	bgmTokenChanged := false

	if _, ok := payload["bgm_mode"]; ok {
		bgmModeNext = boolValue(payload, "bgm_mode", false)
		bgmModeSet = true
	}
	if _, ok := payload["bgm_manage_mode"]; ok {
		bgmManageModeNext = boolValue(payload, "bgm_manage_mode", false)
		bgmManageModeSet = true
	}
	if _, ok := payload["notify_on_login_telegram"]; ok {
		notifyLoginTelegramNext = boolValue(payload, "notify_on_login_telegram", false)
		notifyLoginTelegramSet = true
	}
	if _, ok := payload["notify_on_login_email"]; ok {
		notifyLoginEmailNext = boolValue(payload, "notify_on_login_email", false)
		notifyLoginEmailSet = true
	}
	if _, ok := payload["notify_on_ticket_telegram"]; ok {
		notifyTicketTelegramNext = boolValue(payload, "notify_on_ticket_telegram", false)
		notifyTicketTelegramSet = true
	}
	if _, ok := payload["signin_auto_renewal"]; ok {
		signinAutoRenewalNext = boolValue(payload, "signin_auto_renewal", false)
		signinAutoRenewalSet = true
	}
	if _, ok := payload["require_email_for_password_change"]; ok {
		passwordChangeEmailRequiredNext = boolValue(payload, "require_email_for_password_change", false)
		passwordChangeEmailRequiredSet = true
	}
	if _, ok := payload["require_email_for_emby_password_change"]; ok {
		embyPasswordEmailRequiredNext = boolValue(payload, "require_email_for_emby_password_change", false)
		embyPasswordEmailRequiredSet = true
		if embyPasswordEmailRequiredNext {
			embyPasswordOldPasswordRequiredNext = false
			embyPasswordOldPasswordRequiredSet = true
		}
	} else if embyPasswordEmailRequiredSet && embyPasswordEmailRequiredNext {
		embyPasswordOldPasswordRequiredNext = false
		embyPasswordOldPasswordRequiredSet = true
	}

	u, err := s.app.store().UpdateUser(user.UID, func(u *store.User) error {
		if email := stringValue(payload, "email"); email != "" {
			if err := validate.ValidateEmailFormat(email); err != nil {
				return err
			}
			email = strings.TrimSpace(email)
			if len(s.app.cfg().EmailBlacklist) > 0 && validate.CheckEmailBlacklist(email, s.app.cfg().EmailBlacklist) {
				return fmt.Errorf("该邮箱域名不在允许范围内")
			}
			if len(s.app.cfg().EmailWhitelist) > 0 && !validate.CheckEmailWhitelist(email, s.app.cfg().EmailWhitelist) {
				return fmt.Errorf("该邮箱域名不在允许范围内")
			}
			if !strings.EqualFold(email, u.Email) {
				u.EmailVerified = false
				u.EmailVerifiedAt = 0
			}
			u.Email = email
		}
		if username := stringValue(payload, "username"); username != "" {
			if err := validate.ValidateUsername(username); err != nil {
				return err
			}
			u.Username = username
		}
		if bgmModeSet {
			u.BGMMode = bgmModeNext
		}
		if bgmManageModeSet {
			u.BGMManageMode = bgmManageModeNext
		}
		if _, ok := payload["bgm_token"]; ok {
			token := stringValue(payload, "bgm_token")
			if token != u.BGMToken {
				bgmTokenChanged = true
			}
			u.BGMToken = token
			if token == "" {
				u.BGMMode = false
				u.BGMManageMode = false
			}
		}
		if notifyLoginTelegramSet {
			u.NotifyOnLoginTelegram = notifyLoginTelegramNext
		}
		if notifyLoginEmailSet {
			u.NotifyOnLoginEmail = notifyLoginEmailNext
		}
		if notifyTicketTelegramSet {
			u.NotifyOnTicketTelegram = notifyTicketTelegramNext
		}
		if signinAutoRenewalSet {
			if signinAutoRenewalNext {
				if !signinAutoRenewalEnabled(*s.app.cfg()) || s.app.userIsProtected(*u) || u.ExpiredAt <= 0 || expiryIsPermanent(u.ExpiredAt) {
					return store.ErrConflict
				}
				if err := validateSelfServiceRenewalTarget(*u); err != nil {
					return err
				}
			}
			u.SigninAutoRenewal = signinAutoRenewalNext
		}
		if passwordChangeEmailRequiredSet {
			u.RequireEmailForPasswordChange = passwordChangeEmailRequiredNext
		}
		if embyPasswordEmailRequiredSet {
			u.RequireEmailForEmbyPasswordChange = embyPasswordEmailRequiredNext
		}
		if embyPasswordOldPasswordRequiredSet {
			u.RequireOldPasswordForEmbyPasswordChange = embyPasswordOldPasswordRequiredNext
		}
		return nil
	})

	if signinAutoRenewalSet && signinAutoRenewalNext {
		if errors.Is(err, store.ErrEmbyRequired) {
			return updateProfileResult{}, &serviceError{
				Status:  http.StatusConflict,
				Code:    ErrRenewRequiresEmby,
				Message: "请先绑定或开通 Emby 账号，再开启自动续期",
			}
		}
		if errors.Is(err, store.ErrConflict) {
			return updateProfileResult{}, &serviceError{
				Status:  http.StatusConflict,
				Code:    ErrConflict,
				Message: "账号状态已变化，请刷新后重试",
			}
		}
	}

	if err != nil {
		return updateProfileResult{}, err
	}

	if bgmTokenChanged {
		_ = s.app.store().DeleteBangumiCollectionCache(u.UID, 0)
	}

	return updateProfileResult{User: u}, nil
}

// updateUsername 更新用户名
func (s *userService) updateUsername(ctx context.Context, user store.User, newUsername string) (updateUsernameResult, error) {
	if newUsername == "" {
		return updateUsernameResult{}, &serviceError{
			Status:  http.StatusBadRequest,
			Code:    ErrUserNewUsernameRequired,
			Message: "请填写新用户名",
		}
	}

	if err := validate.ValidateUsername(newUsername); err != nil {
		return updateUsernameResult{}, &serviceError{
			Status:  http.StatusBadRequest,
			Code:    ErrUsernameInvalid,
			Message: err.Error(),
		}
	}

	u, err := s.app.store().UpdateUser(user.UID, func(u *store.User) error {
		u.Username = newUsername
		return nil
	})

	if err != nil {
		return updateUsernameResult{}, err
	}

	return updateUsernameResult{User: u}, nil
}

// renew 使用续期码续期账号
func (s *userService) renew(ctx context.Context, user store.User, regCode string) (renewResult, error) {
	if regCode == "" {
		return renewResult{}, &serviceError{
			Status:  http.StatusBadRequest,
			Code:    ErrRenewCodeRequired,
			Message: "续期需要提供注册码",
		}
	}

	preview, source, okPreview := s.app.previewCode(ctx, regCode, user)
	if !okPreview || source != "regcode" || int(numeric(preview["type"])) != 2 {
		return renewResult{}, &serviceError{
			Status:  http.StatusBadRequest,
			Code:    ErrRenewCodeInvalid,
			Message: "续期码无效、已用完、已过期或不属于当前用户",
		}
	}

	if err := validateSelfServiceRenewalTarget(user); err != nil {
		if errors.Is(err, store.ErrEmbyRequired) {
			return renewResult{}, &serviceError{
				Status:  http.StatusConflict,
				Code:    ErrRenewRequiresEmby,
				Message: "请先绑定或开通 Emby 账号，再使用续期码",
			}
		}
		return renewResult{}, err
	}

	u, _, err := s.app.store().ConsumeRegCodeAndUpdateUser(regCode, user.UID, user.TelegramID, func(u *store.User, code store.RegCode) error {
		if err := validateSelfServiceRenewalTarget(*u); err != nil {
			return err
		}
		days := normalizeRegCodeDays(code.Days)
		renewExpiryAndReactivate(u, addDaysToExpiry(u.ExpiredAt, days, time.Now()))
		return nil
	})

	if err != nil {
		if errors.Is(err, store.ErrEmbyRequired) {
			return renewResult{}, &serviceError{
				Status:  http.StatusConflict,
				Code:    ErrRenewRequiresEmby,
				Message: "请先绑定或开通 Emby 账号，再使用续期码",
			}
		}
		if !errors.Is(err, store.ErrNotFound) && !errors.Is(err, store.ErrExpired) && !errors.Is(err, store.ErrConflict) {
			return renewResult{}, err
		}
		return renewResult{}, &serviceError{
			Status:  http.StatusBadRequest,
			Code:    ErrRegcodeInvalid,
			Message: "注册码无效、已用完或已过期",
		}
	}

	return renewResult{
		User:         u,
		ExpiredAt:    publicExpiryUnix(u.ExpiredAt),
		ExpireStatus: expireStatus(u.ExpiredAt),
	}, nil
}

type serviceError struct {
	Status  int
	Code    ErrCode
	Message string
}

func (e *serviceError) Error() string { return e.Message }

func (a *App) userSvc() *userService {
	return &userService{app: a}
}
