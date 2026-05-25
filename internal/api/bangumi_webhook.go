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
	if !a.cfg.BangumiEnabled {
		fail(w, http.StatusBadRequest, "Bangumi 同步未启用")
		return
	}
	secret := firstNonEmpty(r.URL.Query().Get("token"), r.Header.Get("X-Twilight-Bangumi-Token"), r.Header.Get("X-Webhook-Token"))
	payload := decodeMap(r)
	if secret == "" {
		secret = stringValue(payload, "token")
	}
	if a.cfg.BangumiWebhookSecret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(a.cfg.BangumiWebhookSecret)) != 1 {
		fail(w, http.StatusForbidden, "Webhook 密钥无效")
		return
	}
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
		if local, okUser := a.store.FindUserByEmbyID(userID); okUser {
			duration := numeric(payload["PlaybackPositionTicks"]) / 10000000
			if duration <= 0 {
				duration = numeric(item["RunTimeTicks"]) / 10000000
			}
			if err := a.store.AddPlaybackRecord(store.PlaybackRecord{
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
