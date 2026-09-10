package api

import (
	"net/http"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleSetupStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", a.setupStatusData())
}

func (a *App) handleSetupComplete(w http.ResponseWriter, r *http.Request, _ Params) {
	if !requireWebUIIntent(w, r, twilightIntentCompleteSetup) {
		return
	}
	limiter := a.limiter()
	if limiter != nil && !limiter.Allow(r.Context(), rateKey("setup:", a.clientIP(r)), 5, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "初始化尝试过于频繁，请稍后再试")
		return
	}

	payload := decodeMap(r)
	result, err := a.completeSetup(r.Context(), payload)
	if err != nil {
		if failure, ok := err.(*setupFailure); ok {
			failWithCode(w, failure.Status, failure.Code, failure.Message)
			return
		}
		if statusFromError(w, err) {
			return
		}
		failWithCode(w, http.StatusInternalServerError, ErrInternal, "创建初始化管理员失败")
		return
	}

	token, expires, err := a.sessions().Create(r.Context(), result.User.UID)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrSessionCreateFailed, "初始化已完成，但自动登录会话创建失败，请返回登录页手动登录")
		return
	}
	a.issueSessionCookies(w, token, expires)
	now := time.Now().Unix()
	deviceID := firstNonEmpty(r.Header.Get("X-Twilight-Device"), r.UserAgent(), a.clientIP(r))
	ua := firstNonEmpty(r.UserAgent(), "unknown")
	ip := a.clientIP(r)
	_ = a.store().UpdateDevice(result.User.UID, deviceID, func(d *store.Device) {
		d.DeviceName = ua
		d.Client = "web"
		d.LastIP = ip
		d.LastSeen = now
	})
	_ = a.store().AddLoginLog(store.LoginLog{UID: result.User.UID, IP: ip, DeviceID: deviceID, DeviceName: ua, Client: "web", Time: now})
	a.auditWithUser(r, result.User.UID, result.User.Username, "complete_setup_wizard", "system", result.User.UID, map[string]any{
		"configured_sections": result.ConfiguredSections,
		"ip":                  ip,
		"device":              deviceID,
	})

	created(w, "初始化完成", map[string]any{
		"user":            publicUser(result.User),
		"setup_completed": true,
		"config":          result.Config,
	})
}
