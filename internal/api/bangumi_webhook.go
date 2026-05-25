package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

func (a *App) handleBangumiWebhook(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.cfg().BangumiEnabled {
		failWithCode(w, http.StatusBadRequest, ErrBangumiSyncDisabled, "Bangumi 同步未启用")
		return
	}
	// 优先 header，避免 secret 被上游代理 / CDN access log 记录到 query string。
	// query token 仍被读取以兼容旧回调，但每次命中都会打 Warn 提示运维迁移到
	// X-Twilight-Bangumi-Token 头。
	secret := firstNonEmpty(r.Header.Get("X-Twilight-Bangumi-Token"), r.Header.Get("X-Webhook-Token"))
	usingQuerySecret := false
	if secret == "" {
		if q := r.URL.Query().Get("token"); q != "" {
			secret = q
			usingQuerySecret = true
		}
	}
	// 鉴权必须在 decodeMap 之前完成：旧实现先 `decodeMap(r)` 再 `ConstantTimeCompare`，
	// 任何未鉴权的请求都能让 server 把 body（受 MaxUploadSize 上限约束）读完并构建
	// 完整 map[string]any，攻击者可以无凭据投递大体积 JSON 触发 GC 放大。改为只允许
	// header / query token；body-token 废弃后整个 hot path 不再读 body。
	if a.cfg().BangumiWebhookSecret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(a.cfg().BangumiWebhookSecret)) != 1 {
		failWithCode(w, http.StatusForbidden, ErrUnauthorized, "Webhook 密钥无效")
		return
	}
	if usingQuerySecret {
		zap.L().Warn(
			"bangumi webhook 仍在使用 ?token= 查询参数；查询字符串可能被代理 / CDN access log 收集，请尽快改用 X-Twilight-Bangumi-Token 头",
			zap.String("remote", r.RemoteAddr),
		)
	}
	payload := decodeMap(r)
	item, _ := payload["Item"].(map[string]any)
	eventName := strings.ToLower(firstNonEmpty(asString(payload["Event"]), asString(payload["NotificationType"]), asString(payload["Name"])))
	if item != nil && (strings.Contains(eventName, "stop") || strings.Contains(eventName, "played") || payload["PlaybackPositionTicks"] != nil) {
		userID := firstNonEmpty(asString(payload["UserId"]), asString(payload["UserID"]))
		if userID == "" {
			if userData, ok := payload["User"].(map[string]any); ok {
				userID = firstNonEmpty(asString(userData["Id"]), asString(userData["ID"]))
			}
		}
		if userID == "" {
			if sessionData, ok := payload["Session"].(map[string]any); ok {
				userID = firstNonEmpty(asString(sessionData["UserId"]), asString(sessionData["UserID"]))
			}
		}
		if local, okUser := a.store().FindUserByEmbyID(userID); okUser {
			duration := numeric(payload["PlaybackPositionTicks"]) / 10000000
			if duration <= 0 {
				duration = numeric(item["RunTimeTicks"]) / 10000000
			}
			if err := a.store().AddPlaybackRecord(store.PlaybackRecord{
				UID:       local.UID,
				ItemID:    firstNonEmpty(asString(item["Id"]), asString(item["ID"])),
				Title:     firstNonEmpty(asString(item["Name"]), asString(item["SeriesName"])),
				MediaType: asString(item["Type"]),
				Duration:  duration,
				PlayedAt:  time.Now().Unix(),
			}); err != nil {
				zap.L().Warn("failed to record Bangumi playback webhook", zap.Int64("uid", local.UID), zap.Error(err))
			}
		}
	}
	ok(w, "webhook accepted", map[string]any{"accepted": true, "subject_name": stringValue(item, "SeriesName"), "episode": intValue(item, "IndexNumber", 0)})
}
