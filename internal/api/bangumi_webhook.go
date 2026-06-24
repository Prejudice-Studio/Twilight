package api

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

// bangumiWebhookReplayWindowSeconds 是 X-Twilight-Bangumi-Timestamp 的容忍窗
// 口。攻击者抓到一份合法请求后，在窗口外重放会被直接拒绝；窗口内的重放仍然
// 由 store 层的 (UID, ItemID, PlayedAt) 幂等键挡住的双层防御。
//
// 客户端时钟漂移最常见在 ±60s，留 5 分钟避免合法请求被误杀。Header 缺失时
// 走旧的兼容路径（仅校验 secret），日志会打 Warn 提示运维补上。
const bangumiWebhookReplayWindowSeconds = 300

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
	if a.cfg().BangumiWebhookSecret == "" || !constantTimeStringEqual(secret, a.cfg().BangumiWebhookSecret) {
		failWithCode(w, http.StatusForbidden, ErrUnauthorized, "Webhook 密钥无效")
		return
	}
	if usingQuerySecret {
		zap.L().Warn(
			"bangumi webhook 仍在使用 ?token= 查询参数；查询字符串可能被代理 / CDN access log 收集，请尽快改用 X-Twilight-Bangumi-Token 头",
			zap.String("remote", r.RemoteAddr),
		)
	}
	// 时间戳 replay window：header 缺失时仅打 Warn（兼容旧回调），存在则严格
	// 校验。窗口外的请求直接 410 拒绝，告诉客户端"这次请求已经过期不必重发"。
	// 同一份 header 时间戳还会被透传给 store 作为 PlayedAt 幂等键的一部分，
	// 让"同字节重放"在 store 层落到同一行而被静默丢弃——time.Now() 在跨秒
	// 边界会让相隔 1s 的两次重放绕过 (uid,item_id,played_at) 唯一性。
	var headerPlayedAt int64
	if tsHeader := strings.TrimSpace(r.Header.Get("X-Twilight-Bangumi-Timestamp")); tsHeader != "" {
		ts, parseErr := strconv.ParseInt(tsHeader, 10, 64)
		if parseErr != nil {
			failWithCode(w, http.StatusBadRequest, ErrUnauthorized, "Webhook timestamp 非法")
			return
		}
		now := time.Now().Unix()
		drift := now - ts
		if drift < 0 {
			drift = -drift
		}
		if drift > bangumiWebhookReplayWindowSeconds {
			zap.L().Warn(
				"bangumi webhook timestamp outside replay window",
				zap.String("remote", r.RemoteAddr),
				zap.Int64("drift_seconds", drift),
				zap.Int("window_seconds", bangumiWebhookReplayWindowSeconds),
			)
			failWithCode(w, http.StatusGone, ErrUnauthorized, "Webhook 请求已过期")
			return
		}
		headerPlayedAt = ts
	} else {
		zap.L().Warn(
			"bangumi webhook 未携带 X-Twilight-Bangumi-Timestamp header，无法做 replay-window 校验，建议客户端补齐",
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
			// PlayedAt 优先用 header 时间戳：同一份字节重放总是命中相同 PlayedAt，
			// store 层的 (uid, item_id, played_at) 唯一键保证去重；只有缺 header
			// 的兼容路径才回落到 time.Now()，那条路径在 SECRET 已被合法持有时
			// 才会进入，重放风险在这里能容忍。
			playedAt := headerPlayedAt
			if playedAt == 0 {
				playedAt = time.Now().Unix()
			}
			// 走幂等版：即便攻击者绕过了 timestamp window 在同一秒内重放同一条
			// 合法请求，store 层的 (uid, item_id, played_at) 三元组检查会让第二
			// 次以后的写入直接静默丢弃，不会让 PlaybackRecords 无限堆积。
			inserted, err := a.store().AddPlaybackRecordIdempotent(store.PlaybackRecord{
				UID:         local.UID,
				ItemID:      firstNonEmpty(asString(item["Id"]), asString(item["ID"])),
				Title:       firstNonEmpty(asString(item["Name"]), asString(item["SeriesName"])),
				SeriesName:  asString(item["SeriesName"]),
				MediaType:   asString(item["Type"]),
				IndexNumber: int(intValue(item, "IndexNumber", 0)),
				Duration:    duration,
				PlayedAt:    playedAt,
			})
			if err != nil {
				zap.L().Warn("failed to record Bangumi playback webhook", zap.Int64("uid", local.UID), zap.Error(err))
			} else if !inserted {
				zap.L().Info(
					"bangumi webhook playback record deduplicated by idempotency key",
					zap.Int64("uid", local.UID),
					zap.String("item_id", firstNonEmpty(asString(item["Id"]), asString(item["ID"]))),
				)
			}
		}
	}
	ok(w, "webhook accepted", map[string]any{"accepted": true, "subject_name": stringValue(item, "SeriesName"), "episode": intValue(item, "IndexNumber", 0)})
}

// constantTimeStringEqual 在 ConstantTimeCompare 基础上消除 length-mismatch
// 提前 return 0 引入的 timing oracle：先把两侧 zero-pad 到相同长度再比对，
// 然后 AND 上长度等价位。即便攻击者通过响应时延区分"长度不同"与"长度相同
// 但内容不同"，这层补丁保证两条路径的执行时间一致。
//
// 该 helper 局限于 secret 长度 <= 1024（够用所有合理 secret），超过则视为
// 明显非法直接 false——避免攻击者用极长 string 搜出更多 timing 信号。
func constantTimeStringEqual(got, want string) bool {
	const maxSecretBytes = 1024
	if len(got) > maxSecretBytes || len(want) > maxSecretBytes {
		return false
	}
	maxLen := len(got)
	if len(want) > maxLen {
		maxLen = len(want)
	}
	gotBuf := make([]byte, maxLen)
	wantBuf := make([]byte, maxLen)
	copy(gotBuf, got)
	copy(wantBuf, want)
	cmp := subtle.ConstantTimeCompare(gotBuf, wantBuf)
	lenEq := subtle.ConstantTimeEq(int32(len(got)), int32(len(want)))
	return cmp&lenEq == 1
}
