package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

func (a *App) handleSendReminders(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	defaultDays := a.cfg().NotificationExpiryRemindDays
	if defaultDays <= 0 {
		defaultDays = 3
	}
	days := clamp(intValue(payload, "days", defaultDays), 1, 365)
	result := a.sendExpiryReminders(r.Context(), days)
	ok(w, "reminders sent", result)
}

func (a *App) sendExpiryReminders(ctx context.Context, days int) map[string]any {
	deadline := time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
	now := time.Now().Unix()
	users := []map[string]any{}
	failedItems := []map[string]any{}
	sent := 0
	// 30 msg/s 是 Telegram bot 全局发送上限的安全边界：35ms inter-message
	// 间隔 ≈ 28.5 msg/s，留 5% 余量给非提醒路径（kick / 双向交互）共享 quota。
	// 没有这一行，100 个即将到期用户的提醒批从第 31 个开始全部 429，下一轮
	// expiry_reminders 跑还要重炸一遍——这是用户面提醒链路最常见的"提醒
	// 雪崩"症状。
	const reminderPerMessageSpacing = 35 * time.Millisecond
	// rate-limit 连续触发 N 次后 break：通常 Telegram 把 bot 拉黑后接下来
	// 每条都 429，继续打只会让 retry_after 越加越长。让本轮 partial 成功
	// 比"全失败 + 1 分钟自旋 sleep" 对 admin 友好得多。
	const maxConsecutiveRateLimited = 5
	consecutiveRateLimited := 0
	first := true
	for _, u := range a.store().ListUsers() {
		if u.Active && u.ExpiredAt > now && u.ExpiredAt <= deadline {
			remaining := u.ExpiredAt - now
			item := map[string]any{"uid": u.UID, "username": u.Username, "telegram_id": nullableInt(u.TelegramID), "expired_at": u.ExpiredAt, "remaining_seconds": remaining, "remaining_str": formatSeconds(remaining)}
			users = append(users, item)
			if !a.cfg().NotificationEnabled || !a.telegramAvailable() || u.TelegramID == 0 {
				continue
			}
			// 第一条不 sleep，后续每条之间 spacing；ctx 取消立刻退出，
			// 不让 admin 在 manual run 时被 35ms × N 拖住关闭。
			if !first {
				select {
				case <-ctx.Done():
					return map[string]any{"sent": sent, "total": len(users), "count": len(users), "users": users, "failed": failedItems, "telegram_enabled": a.telegramAvailable(), "notification_enabled": a.cfg().NotificationEnabled, "days": days, "aborted": "context_canceled"}
				case <-time.After(reminderPerMessageSpacing):
				}
			}
			first = false
			text := fmt.Sprintf("%s，您的账号将在 %s 后到期，请及时续期。", u.Username, formatSeconds(remaining))
			if err := a.telegramSendMessage(ctx, u.TelegramID, text); err != nil {
				failedItems = append(failedItems, map[string]any{"uid": u.UID, "username": u.Username, "telegram_id": u.TelegramID, "error": err.Error()})
				if _, isRL := telegramRetryAfterFromError(err); isRL || strings.Contains(strings.ToLower(err.Error()), "too many requests") {
					consecutiveRateLimited++
					// 关键：把 retry_after 真实秒数 sleep 出来，下一条才有
					// 机会通过；R61-3 的 telegramRateLimitPause 已经 cap 在 60s。
					telegramRateLimitPause(err)
					if consecutiveRateLimited >= maxConsecutiveRateLimited {
						return map[string]any{"sent": sent, "total": len(users), "count": len(users), "users": users, "failed": failedItems, "telegram_enabled": a.telegramAvailable(), "notification_enabled": a.cfg().NotificationEnabled, "days": days, "aborted": "rate_limited", "consecutive_rate_limited": consecutiveRateLimited}
					}
					continue
				}
				consecutiveRateLimited = 0
				continue
			}
			consecutiveRateLimited = 0
			sent++
		}
	}
	return map[string]any{"sent": sent, "total": len(users), "count": len(users), "users": users, "failed": failedItems, "telegram_enabled": a.telegramAvailable(), "notification_enabled": a.cfg().NotificationEnabled, "days": days}
}

func (a *App) handleSchedulerRunV2(w http.ResponseWriter, r *http.Request, params Params) {
	jobID := params["job_id"]
	if !schedulerJobExists(jobID) {
		failWithCode(w, http.StatusNotFound, ErrSchedulerJobNotFound, "调度任务不存在")
		return
	}
	run, okRun := a.startManualSchedulerJob(context.Background(), jobID, schedulerRequestParams(r))
	if !okRun {
		failWithCode(w, http.StatusConflict, ErrSchedulerJobRunning, "调度任务正在运行中")
		return
	}
	ok(w, "job started", map[string]any{"job_id": run.JobID, "last_run": run})
}

func (a *App) runSchedulerJob(r *http.Request, jobID string) (map[string]any, []string, error) {
	if !schedulerJobExists(jobID) {
		return map[string]any{"success": false}, nil, fmt.Errorf("job not found")
	}
	params := a.schedulerEffectiveParams(r, jobID)
	if err := r.Context().Err(); err != nil {
		return map[string]any{"success": false, "terminated": true}, []string{"job terminated before execution"}, err
	}
	now := time.Now().Unix()
	switch jobID {
	case "check_expired":
		disabled := 0
		embyDisabled := 0
		skippedProtected := 0
		users := a.store().ListUsers()
		invitedUIDs := map[int64]bool{}
		for _, rel := range a.store().InviteRelations() {
			invitedUIDs[rel.ChildUID] = true
		}
		batchCount := 0
		for _, u := range users {
			batchCount++
			if batchCount%50 == 0 {
				if err := r.Context().Err(); err != nil {
					return map[string]any{"success": false, "terminated": true, "disabled": disabled, "emby_disabled": embyDisabled, "skipped_protected": skippedProtected}, []string{"job terminated"}, err
				}
			}
			// 守护管理员 / 白名单不被自动禁用：运维约定"绝不会给 admin 设
			// finite ExpiredAt"，但 demote-then-repromote 路径 / 手动 SQL /
			// 旧迁移可能在 admin 上留下 ExpiredAt > 0；一旦 check_expired 命中
			// 就会把 admin Active=false 并 DeleteUser session——管理员从此登
			// 不上自己的 panel，且只有数据库直改才能解锁。这里统一走保护用
			// 户口径（角色 + 配置管理员），并 emit `skipped_protected` 计数让
			// admin 在调度报告里看到守护命中（区别于"无人需要禁用"的 0 值）。
			if a.userIsProtected(u) {
				if u.Active && u.ExpiredAt > 0 && u.ExpiredAt < now {
					skippedProtected++
				}
				continue
			}
			if u.Active && u.ExpiredAt > 0 && u.ExpiredAt < now {
				// For invited users (have invite relation), only disable Emby access
				// but keep the account active so they can still log in and renew
				isInvited := invitedUIDs[u.UID]
				if isInvited {
					sideCtx, sideCancel := schedulerSideEffectContext(r.Context())
					// Only disable Emby, keep account active so the user
					// can re-login (or the inviter can renew on their behalf)
					if disabledRemote, err := a.disableRemoteEmbyForWebState(sideCtx, u); err == nil && disabledRemote {
						embyDisabled++
					}
					// 即便保留 Active=true 让用户能重新登录续期，已经过期的
					// 时刻必须立刻让现有会话失效——否则 stale cookie 在
					// SessionTTL 内仍能访问受保护接口（包括非续期接口），
					// 与 authenticateAPIKey 的 `!u.Active` / 过期兜底语义
					// 不一致。续期成功后用户重新登录即可拿新 session。
					a.sessions().DeleteUser(sideCtx, u.UID)
					sideCancel()
					disabled++
				} else {
					// Non-invited users: disable the whole account
					updated, err := a.store().SetUserActiveAtomic(u.UID, false)
					if err == nil {
						sideCtx, sideCancel := schedulerSideEffectContext(r.Context())
						if disabledRemote, err := a.disableRemoteEmbyForWebState(sideCtx, updated); err == nil && disabledRemote {
							embyDisabled++
						}
						// 立即清除该用户的所有会话。否则 stale
						// token 仍可访问受保护接口直到 SessionTTL 自然到期。
						a.sessions().DeleteUser(sideCtx, updated.UID)
						disabled++
						sideCancel()
					}
				}
			}
		}
		if disabled > 0 || embyDisabled > 0 {
			a.auditSystem("scheduler", "disable_expired_users", 0, map[string]any{
				"disabled":          disabled,
				"emby_disabled":     embyDisabled,
				"skipped_protected": skippedProtected,
			})
		}
		return map[string]any{"success": true, "disabled": disabled, "emby_disabled": embyDisabled, "skipped_protected": skippedProtected}, []string{fmt.Sprintf("disabled %d expired users", disabled)}, nil
	case "check_expiring", "expiry_reminders":
		defaultDays := a.cfg().NotificationExpiryRemindDays
		if defaultDays <= 0 {
			defaultDays = 3
		}
		days := clamp(jobParamInt(params, "days", queryInt(r, "days", defaultDays)), 1, 365)
		if jobID == "expiry_reminders" {
			result := a.sendExpiryReminders(r.Context(), days)
			result["success"] = true
			return result, []string{fmt.Sprintf("sent %d reminders for %d expiring users", int(numeric(result["sent"])), int(numeric(result["count"])))}, nil
		}
		deadline := time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
		count := 0
		for _, u := range a.store().ListUsers() {
			if err := r.Context().Err(); err != nil {
				return map[string]any{"success": false, "terminated": true, "expiring": count, "days": days}, []string{"job terminated"}, err
			}
			if u.Active && u.ExpiredAt > now && u.ExpiredAt <= deadline {
				count++
			}
		}
		return map[string]any{"success": true, "expiring": count, "days": days}, []string{fmt.Sprintf("found %d expiring users", count)}, nil
	case "daily_stats":
		users := a.store().ListUsers()
		return map[string]any{"success": true, "users": len(users), "active": countActive(users)}, []string{"daily stats generated"}, nil
	case "sync_emby_activity_logs":
		if !a.embyConfigured() {
			return map[string]any{"success": true, "configured": false, "new_entries": 0}, []string{"Emby not configured"}, nil
		}
		sinceHours := clamp(jobParamInt(params, "since_hours", 24), 1, 720)
		count, err := a.fetchAndStoreEmbyActivityLogsSince(r.Context(), time.Now().Add(-time.Duration(sinceHours)*time.Hour))
		if err != nil {
			return map[string]any{"success": false, "since_hours": sinceHours}, nil, err
		}
		return map[string]any{"success": true, "configured": true, "new_entries": count, "since_hours": sinceHours}, []string{fmt.Sprintf("synced %d Emby activity log entries from the last %d hours", count, sinceHours)}, nil
	case "cleanup_sessions":
		expiredSessions := a.sessions().CleanupExpired(r.Context())
		cfg := a.cfg()
		expiredEmailCodes := 0
		if cfg.EmailAutoCleanupExpiredVerifications {
			// 顺带回收过期邮箱验证码：短 TTL，借会话清理槽周期回收，避免 state 里
			// 堆积已失效记录。失败不阻断主流程（下一轮再清）。
			expiredEmailCodes, _ = a.store().CleanupExpiredEmailVerifications(time.Now().Unix())
		}
		staleCleared := 0
		if cfg.EmailAutoCleanupUnverified {
			// 定期清理已绑定但长期未验证的邮箱，释放邮箱地址供其他用户使用。
			// 使用基于 CreatedAt 的年龄门限而非 ClearUnverifiedEmails 的全量清理，
			// 避免刚注册几分钟的用户还没查收验证码就被清掉邮箱。
			hours := cfg.EmailAutoCleanupUnverifiedHours
			if hours <= 0 {
				hours = 24
			}
			_, staleCleared, _ = a.store().CleanupUnverifiedEmailsByAge(time.Now().Add(-time.Duration(hours) * time.Hour).Unix())
		}
		if !a.embyConfigured() {
			return map[string]any{"success": true, "configured": false, "active": 0, "total": 0, "expired_sessions": expiredSessions, "expired_email_codes": expiredEmailCodes, "cleared_unverified_emails": staleCleared}, []string{"Emby not configured", fmt.Sprintf("cleaned up %d expired sessions", expiredSessions), fmt.Sprintf("cleaned up %d expired email codes", expiredEmailCodes), fmt.Sprintf("cleared %d stale unverified emails", staleCleared)}, nil
		}
		sessions, err := a.embySessionsSnapshot(r.Context(), false)
		if err != nil {
			return map[string]any{"success": false}, nil, err
		}
		active := countEmbyPlayingSessions(sessions)
		return map[string]any{"success": true, "active": active, "total": len(sessions), "expired_sessions": expiredSessions, "expired_email_codes": expiredEmailCodes, "cleared_unverified_emails": staleCleared}, []string{fmt.Sprintf("read %d Emby sessions", len(sessions)), fmt.Sprintf("cleaned up %d expired sessions", expiredSessions), fmt.Sprintf("cleaned up %d expired email codes", expiredEmailCodes), fmt.Sprintf("cleared %d stale unverified emails", staleCleared)}, nil
	case "emby_sync":
		if !a.embyConfigured() {
			return map[string]any{"success": true, "configured": false}, []string{"Emby not configured"}, nil
		}
		// 给 emby_sync 加 30 分钟硬上限，避免 emby 反代僵死时整个调度槽被占住——
		// 调度器是单 goroutine 串行的，emby_sync hang 住会让 check_expired /
		// expiry_reminders 这些下游任务推迟整轮。即便 500 个用户每人 5s，仍不
		// 到 45 分钟；超过 30min 几乎一定是 emby 不健康，让本轮 fail-fast 优于
		// 拖到管理员手动 cancel。
		syncCtx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()
		logs := []string{}
		var remote []map[string]any
		// /Users 列表是幂等 GET，遇 5xx / 连接抖动重试 2 次更划算（详见
		// embyRetryOn5xx 的注释）。一开局拉用户列表如果直接挂掉，整轮 sync 全
		// 部 user 都被记成 missing，下一轮还要再炸一遍。
		if err := embyRetryOn5xx(syncCtx, func(ctx context.Context) error {
			return a.embyGet(ctx, "/Users", &remote)
		}); err != nil {
			return map[string]any{"success": false}, nil, err
		}
		remoteByID := map[string]map[string]any{}
		remoteByName := map[string]map[string]any{}
		duplicateRemoteNames := map[string]bool{}
		for _, user := range remote {
			if id := embyRemoteID(user); id != "" {
				remoteByID[id] = user
			}
			if name := normalizeEmbyName(embyRemoteName(user)); name != "" {
				if _, exists := remoteByName[name]; exists {
					duplicateRemoteNames[name] = true
				}
				remoteByName[name] = user
			}
		}
		for name := range duplicateRemoteNames {
			delete(remoteByName, name)
		}
		users := a.store().ListUsers()
		maxUsers := clamp(jobParamInt(params, "max_users", 1000), 1, 50000)
		if len(users) > maxUsers {
			users = users[:maxUsers]
		}
		claimedRemoteIDs := map[string]int64{}
		for _, u := range users {
			if u.EmbyID != "" && !isSyntheticEmbyID(u.EmbyID, u.UID) {
				claimedRemoteIDs[u.EmbyID] = u.UID
			}
		}
		updatedNames, syncedState, stateUnchanged, missing, filledIDs, repairedPlaceholders, conflicts := 0, 0, 0, 0, 0, 0, 0
		for _, u := range users {
			if err := syncCtx.Err(); err != nil {
				return map[string]any{"success": false, "terminated": true, "updated_names": updatedNames, "synced_state": syncedState, "state_unchanged": stateUnchanged, "missing": missing, "filled_emby_ids": filledIDs, "repaired_placeholders": repairedPlaceholders, "conflicts": conflicts}, []string{"job terminated"}, err
			}
			placeholder := isSyntheticEmbyID(u.EmbyID, u.UID)
			remoteUser, okRemote := remoteByID[u.EmbyID]
			if !okRemote {
				for _, name := range []string{u.EmbyUsername, u.Username} {
					if candidate, okByName := remoteByName[normalizeEmbyName(name)]; okByName {
						remoteUser = candidate
						okRemote = true
						break
					}
				}
			}
			if !okRemote {
				if u.EmbyID != "" {
					missing++
				}
				continue
			}
			remoteID := embyRemoteID(remoteUser)
			if remoteID == "" {
				missing++
				continue
			}
			if ownerUID, claimed := claimedRemoteIDs[remoteID]; claimed && ownerUID != u.UID {
				conflicts++
				continue
			}
			name := embyRemoteName(remoteUser)
			updatedUser := u
			changes := []string{}
			if remoteID != u.EmbyID {
				changes = append(changes, "emby_id: "+u.EmbyID+"→"+remoteID)
			}
			if name != "" && name != u.EmbyUsername {
				changes = append(changes, "username: "+u.EmbyUsername+"→"+name)
			}
			if u.PendingEmby {
				changes = append(changes, "pending_emby: cleared")
			}
			if remoteID != u.EmbyID || (name != "" && name != u.EmbyUsername) || u.PendingEmby {
				var err error
				updatedUser, err = a.store().UpdateUser(u.UID, func(u *store.User) error {
					if remoteID != u.EmbyID {
						u.EmbyID = remoteID
					}
					if name != "" {
						u.EmbyUsername = name
					}
					u.PendingEmby = false
					u.PendingEmbyDays = nil
					return nil
				})
				if err == nil {
					if remoteID != u.EmbyID {
						filledIDs++
						if placeholder {
							repairedPlaceholders++
						}
						logs = append(logs, "user #"+fmt.Sprintf("%d", u.UID)+" ("+u.Username+"): "+strings.Join(changes, ", "))
					}
					updatedNames++
					claimedRemoteIDs[remoteID] = u.UID
				} else {
					conflicts++
					logs = append(logs, "user #"+fmt.Sprintf("%d", u.UID)+" ("+u.Username+"): sync conflict - "+truncateString(err.Error(), 120))
					continue
				}
			}
			shouldDisable := embyShouldDisableForWebState(updatedUser)
			if !shouldDisable {
				syncedState++
				stateUnchanged++
				continue
			}
			if remoteDisabled, ok := embyRemoteDisabled(remoteUser); ok && remoteDisabled {
				syncedState++
				stateUnchanged++
				continue
			}
			if embyRetryOn5xx(syncCtx, func(ctx context.Context) error {
				_, err := a.disableRemoteEmbyForWebState(ctx, updatedUser)
				return err
			}) == nil {
				syncedState++
				logs = append(logs, "user #"+fmt.Sprintf("%d", updatedUser.UID)+" ("+updatedUser.Username+"): emby disabled by policy")
			}
		}
		logs = append(logs, fmt.Sprintf("read %d Emby users, %d synced, %d unchanged, %d missing, %d conflicts", len(remote), syncedState, stateUnchanged, missing, conflicts))
		if syncedState > 0 || filledIDs > 0 {
			a.auditSystem("scheduler", "emby_sync", 0, map[string]any{
				"remote_users":          len(remote),
				"updated_names":         updatedNames,
				"synced_state":          syncedState,
				"state_unchanged":       stateUnchanged,
				"filled_emby_ids":       filledIDs,
				"repaired_placeholders": repairedPlaceholders,
				"missing":               missing,
				"conflicts":             conflicts,
			})
		}
		return map[string]any{"success": true, "remote_users": len(remote), "updated_names": updatedNames, "synced_state": syncedState, "state_unchanged": stateUnchanged, "missing": missing, "filled_emby_ids": filledIDs, "repaired_placeholders": repairedPlaceholders, "conflicts": conflicts}, logs, nil
	case "cleanup_no_emby":
		ignoreEnabled := jobParamBool(params, "ignore_enabled_flag", false)
		enabled := jobParamBool(params, "enabled", jobParamBool(params, "auto_enabled", a.cfg().AutoCleanupNoEmby))
		if !enabled && !ignoreEnabled {
			return map[string]any{"success": true, "enabled": false, "deleted": 0}, []string{"auto cleanup no-Emby disabled"}, nil
		}
		days := jobParamInt(params, "days", queryInt(r, "days", a.cfg().AutoCleanupNoEmbyDays))
		if days <= 0 {
			days = 7
		}
		threshold := int64(0)
		if days > 0 {
			threshold = time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
		}
		preserveTG := jobParamBool(params, "preserve_tg_bound", a.cfg().EmbyDirectRegisterEnabled)
		dryRun := jobParamBool(params, "dry_run", false)
		candidates := 0
		deleted := 0
		failed := 0
		skippedPending := 0
		for _, u := range a.store().ListUsers() {
			if err := r.Context().Err(); err != nil {
				return map[string]any{"success": false, "terminated": true, "candidates": candidates, "deleted": deleted, "failed": failed, "dry_run": dryRun, "skipped_pending_emby": skippedPending}, []string{"job terminated"}, err
			}
			if a.userIsProtected(u) || u.EmbyID != "" {
				continue
			}
			if u.PendingEmby {
				skippedPending++
				continue
			}
			if preserveTG && u.TelegramID != 0 {
				continue
			}
			registered := u.RegisterTime
			if registered == 0 {
				registered = u.CreatedAt
			}
			if threshold > 0 && registered > threshold {
				continue
			}
			candidates++
			if dryRun {
				continue
			}
			if err := a.store().DeleteUser(u.UID); err != nil {
				failed++
			} else {
				deleted++
			}
		}
		if deleted > 0 {
			a.auditSystem("scheduler", "delete_no_emby_users", 0, map[string]any{
				"deleted":    deleted,
				"candidates": candidates,
				"failed":     failed,
			})
		}
		return map[string]any{"success": true, "enabled": true, "candidates": candidates, "deleted": deleted, "failed": failed, "dry_run": dryRun, "days": days, "days_threshold": days, "preserve_tg_bound": preserveTG, "skipped_pending_emby": skippedPending}, []string{fmt.Sprintf("processed %d no-Emby web users", candidates)}, nil
	case "cleanup_pending_emby_entitlements":
		ignoreEnabled := jobParamBool(params, "ignore_enabled_flag", false)
		enabled := jobParamBool(params, "enabled", jobParamBool(params, "auto_enabled", a.cfg().AutoCleanupPendingEmby))
		if !enabled && !ignoreEnabled {
			return map[string]any{"success": true, "enabled": false, "cleared": 0}, []string{"auto cleanup pending-Emby entitlement disabled"}, nil
		}
		dryRun := jobParamBool(params, "dry_run", false)
		candidates := 0
		cleared := 0
		failed := 0
		for _, u := range a.store().ListUsers() {
			if err := r.Context().Err(); err != nil {
				return map[string]any{"success": false, "terminated": true, "candidates": candidates, "cleared": cleared, "failed": failed, "dry_run": dryRun}, []string{"job terminated"}, err
			}
			if a.userIsProtected(u) || u.EmbyID != "" || !u.PendingEmby {
				continue
			}
			candidates++
			if dryRun {
				continue
			}
			if _, err := a.store().UpdateUser(u.UID, func(u *store.User) error {
				u.PendingEmby = false
				u.PendingEmbyDays = nil
				return nil
			}); err != nil {
				failed++
			} else {
				cleared++
			}
		}
		return map[string]any{"success": true, "enabled": true, "candidates": candidates, "cleared": cleared, "failed": failed, "dry_run": dryRun, "scope": "all"}, []string{fmt.Sprintf("cleared %d pending Emby entitlements", cleared)}, nil
	case "enforce_group_membership":
		autoEnableRejoined := jobParamBool(params, "auto_enable_rejoined", a.cfg().TelegramAutoEnableRejoined)
		result, logs, err := a.enforceTelegramMembership(r.Context(), autoEnableRejoined)
		result["success"] = err == nil
		return result, logs, err
	case "check_telegram_bindings":
		seen := map[int64]int64{}
		duplicates := 0
		for _, u := range a.store().ListUsers() {
			if err := r.Context().Err(); err != nil {
				return map[string]any{"success": false, "terminated": true, "duplicates": duplicates, "bound": len(seen)}, []string{"job terminated"}, err
			}
			if u.TelegramID == 0 {
				continue
			}
			if seen[u.TelegramID] != 0 {
				duplicates++
			}
			seen[u.TelegramID] = u.UID
		}
		return map[string]any{"success": true, "duplicates": duplicates, "bound": len(seen)}, []string{fmt.Sprintf("found %d duplicate telegram bindings", duplicates)}, nil
	case "kick_unknown_group_members":
		dryRun := jobParamBool(params, "dry_run", true)
		maxPerRun := clamp(jobParamInt(params, "max_per_run", 200), 1, 500)
		chats := telegramChatIDs(a.cfg().TelegramGroupIDs)
		if len(chats) == 0 {
			return map[string]any{"success": true, "enabled": false, "targets": 0, "dry_run": dryRun, "max_per_run": maxPerRun}, []string{"Telegram group not configured"}, nil
		}
		plan := a.telegramKickPlan(chats[0])
		targets := plan.Targets
		skippedByType := plan.Skipped
		preservedBound := plan.PreservedBound
		reasonCounts := map[string]int{"no_account": 0, "no_emby": 0, "disabled": 0}
		for _, target := range targets {
			reasonCounts[target.Reason]++
		}
		summary := map[string]any{
			"success":           true,
			"enabled":           true,
			"known_only":        plan.KnownOnly,
			"chat_id":           chats[0],
			"roster_size":       plan.RosterSize,
			"bots_in_roster":    plan.Bots,
			"preserved_bound":   preservedBound,
			"admins_excluded":   skippedByType["admin"],
			"excluded_total":    skippedByType["admin"] + skippedByType["whitelist"] + skippedByType["bound"],
			"targets":           len(targets),
			"reason_no_account": reasonCounts["no_account"],
			"reason_no_emby":    reasonCounts["no_emby"],
			"reason_disabled":   reasonCounts["disabled"],
			"dry_run":           dryRun,
			"max_per_run":       maxPerRun,
			"kicked":            0,
			"skipped":           0,
			"failed":            0,
			"not_in_group":      0,
			"scanned":           0,
			"skipped_no_tg":     skippedByType["no_telegram"],
			"skipped_whitelist": skippedByType["whitelist"],
			"skipped_bound":     skippedByType["bound"],
		}
		if dryRun || len(targets) == 0 {
			return summary, []string{fmt.Sprintf("found %d known Telegram kick candidates", len(targets))}, nil
		}
		if !a.telegramAvailable() {
			summary["success"] = false
			return summary, nil, fmt.Errorf("Telegram not configured")
		}
		adminSet := a.telegramAdminSet(r.Context(), chats[0])
		kicked, skipped, failedCount, notInGroup, scanned := 0, 0, 0, 0, 0
		logs := []string{}
		for _, target := range targets {
			if err := r.Context().Err(); err != nil {
				summary["success"] = false
				summary["terminated"] = true
				return summary, append(logs, "job terminated"), err
			}
			if scanned >= maxPerRun {
				break
			}
			scanned++
			if adminSet[target.TelegramID] {
				skipped++
				continue
			}
			member, err := a.telegramGetChatMember(r.Context(), chats[0], target.TelegramID)
			if err != nil {
				msg := strings.ToLower(err.Error())
				if strings.Contains(msg, "not found") || strings.Contains(msg, "participant") {
					notInGroup++
					continue
				}
				failedCount++
				if len(logs) < 20 {
					// err 来自 telegram bot API，body 偶尔会带 bot token 反弹（API
					// 4xx 时 telegram 偶尔在 description 里回显请求 URL）。logs 落
					// 到 SchedulerRun.Logs 后被持久化到 PG，admin 后台可见，必须
					// 走 redactSensitiveText 脱敏。
					logs = append(logs, fmt.Sprintf("failed to inspect tg=%d uid=%d: %s", target.TelegramID, target.UID, redactSensitiveText(err.Error())))
				}
				telegramRateLimitPause(err)
				continue
			}
			if telegramMemberIsGone(member) {
				notInGroup++
				continue
			}
			if telegramMemberIsAdminOrBot(member) {
				skipped++
				continue
			}
			if err := a.telegramKickChatMember(r.Context(), chats[0], target.TelegramID); err != nil {
				failedCount++
				if len(logs) < 20 {
					// 同上：tg API 错误持久化前必须脱敏。
					logs = append(logs, fmt.Sprintf("failed to kick tg=%d uid=%d: %s", target.TelegramID, target.UID, redactSensitiveText(err.Error())))
				}
				telegramRateLimitPause(err)
				continue
			}
			kicked++
		}
		summary["kicked"] = kicked
		summary["skipped"] = skipped
		summary["failed"] = failedCount
		summary["not_in_group"] = notInGroup
		summary["scanned"] = scanned
		return summary, logs, nil
	case "cleanup_emby_devices":
		result, logs, err := a.cleanupEmbyDevices(r.Context(), embyDeviceCleanupOptions{
			DryRun:        jobParamBool(params, "dry_run", true),
			MaxWorkers:    jobParamInt(params, "max_workers", embyDeviceCleanupDefaultWorkers),
			SkipUsernames: embyDeviceCleanupSkipList(params["skip_usernames"]),
		})
		return result, logs, err
	case "cleanup_unused_uploads":
		result := a.cleanupUnusedUploadAssets(24 * time.Hour)
		result["success"] = true
		return result, []string{fmt.Sprintf("scanned %d upload files, deleted %d", int(numeric(result["scanned"])), int(numeric(result["deleted"])))}, nil
	case "system_auto_update":
		if !a.cfg().SystemUpdateEnabled && !schedulerManualRun(r) {
			return map[string]any{"success": true, "skipped": true, "enabled": false}, []string{"system auto update disabled"}, nil
		}
		result := applyGitUpdate(r.Context(), a.cfg().SystemUpdateRepoURL, a.cfg().SystemUpdateBranch, a.cfg().SystemUpdateRestartServices, false, false)
		if !boolish(result["success"]) {
			return result, nil, fmt.Errorf("%s", asString(result["message"]))
		}
		return result, []string{asString(result["message"])}, nil
	case "cleanup_audit_logs":
		enabled := jobParamBool(params, "enabled", true)
		if !enabled {
			return map[string]any{"success": true, "skipped": true, "reason": "auto cleanup disabled"}, nil, nil
		}
		logs := []string{}
		// 按条数裁剪（保留最新 N 条）
		if maxEntries := jobParamInt(params, "max_entries", 0); maxEntries > 0 {
			_ = a.store().PruneAuditLogs(maxEntries)
			logs = append(logs, fmt.Sprintf("enforced max %d entries (current: %d)", maxEntries, a.store().AuditLogCount()))
		}
		// 按天数裁剪
		if retentionDays := jobParamInt(params, "retention_days", 0); retentionDays > 0 {
			preserveAdmin := jobParamBool(params, "preserve_admin", true)
			cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix()
			removed := a.store().PruneAuditLogsByAge(cutoff, preserveAdmin)
			logs = append(logs, fmt.Sprintf("removed %d entries older than %d days (preserve_admin=%v)", removed, retentionDays, preserveAdmin))
		}
		return map[string]any{"success": true, "current": a.store().AuditLogCount()}, logs, nil
	case "cleanup_unlinked_emby":
		if !a.embyConfigured() {
			return map[string]any{"success": true, "configured": false}, []string{"Emby not configured"}, nil
		}
		logs := []string{}
		dryRun := jobParamBool(params, "dry_run", true)
		delete := jobParamBool(params, "delete", false)
		if dryRun && !delete {
			logs = append(logs, "dry-run mode: scanning only, no deletions")
		}
		var remote []map[string]any
		syncCtx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()
		if err := embyRetryOn5xx(syncCtx, func(ctx context.Context) error {
			return a.embyGet(ctx, "/Users", &remote)
		}); err != nil {
			return map[string]any{"success": false}, nil, err
		}
		localEmbyIDs := map[string]bool{}
		for _, u := range a.store().ListUsers() {
			if u.EmbyID != "" {
				localEmbyIDs[u.EmbyID] = true
			}
		}
		unlinked := []map[string]any{}
		for _, user := range remote {
			id := embyRemoteID(user)
			if id == "" || localEmbyIDs[id] {
				continue
			}
			name := embyRemoteName(user)
			if name == "" {
				name = "(no name)"
			}
			unlinked = append(unlinked, user)
			logs = append(logs, "unlinked Emby user: "+id+" ("+name+")")
		}
		deleted := 0
		if !dryRun && delete {
			for _, user := range unlinked {
				id := embyRemoteID(user)
				if id == "" {
					continue
				}
				if err := a.embyDeleteUser(syncCtx, id); err != nil {
					logs = append(logs, "delete failed for "+id+": "+truncateString(err.Error(), 120))
					continue
				}
				deleted++
				logs = append(logs, "deleted Emby user: "+id)
			}
		}
		if deleted > 0 {
			a.auditSystem("scheduler", "delete_unlinked_emby", 0, map[string]any{
				"unlinked": len(unlinked),
				"deleted":  deleted,
				"dry_run":  dryRun || !delete,
			})
		}
		return map[string]any{"success": true, "unlinked": len(unlinked), "deleted": deleted, "dry_run": dryRun || !delete}, logs, nil
	case "cleanup_ticket_images":
		retentionDays := jobParamInt(params, "retention_days", a.cfg().TicketImageRetentionDays)
		if retentionDays <= 0 {
			return map[string]any{"success": true, "skipped": true, "reason": "retention disabled"}, []string{"ticket image retention disabled"}, nil
		}
		cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix()
		tickets := a.store().ClosedTicketsWithAttachmentsBefore(cutoff)
		cleanedTickets := 0
		removedImages := 0
		for _, t := range tickets {
			a.removeTicketAttachmentDir(t.ID)
			removedImages += len(t.Attachments)
			if err := a.store().ClearTicketAttachments(t.ID); err != nil {
				zap.L().Warn("清空工单图片元数据失败", zap.Int64("ticket_id", t.ID), zap.Error(err))
				continue
			}
			cleanedTickets++
		}
		if cleanedTickets > 0 || removedImages > 0 {
			a.auditSystem("scheduler", "cleanup_ticket_images", 0, map[string]any{
				"tickets": cleanedTickets,
				"images":  removedImages,
			})
		}
		return map[string]any{"success": true, "tickets": cleanedTickets, "images": removedImages},
			[]string{fmt.Sprintf("cleaned %d tickets, %d images older than %d days", cleanedTickets, removedImages, retentionDays)}, nil
	case "refresh_bangumi_collections":
		if !a.cfg().BangumiManageEnabled {
			return map[string]any{"success": true, "enabled": false, "refreshed_users": 0}, []string{"Bangumi manage disabled"}, nil
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()
		scanned := 0
		eligible := 0
		refreshedUsers := 0
		refreshedLists := 0
		cachedEntries := 0
		failed := 0
		logs := []string{}
		for _, u := range a.store().ListUsers() {
			if err := ctx.Err(); err != nil {
				return map[string]any{"success": false, "terminated": true, "scanned": scanned, "eligible": eligible, "refreshed_users": refreshedUsers, "failed": failed}, logs, err
			}
			scanned++
			if u.BGMToken == "" || !u.BGMManageMode {
				continue
			}
			eligible++
			lists, entries, err := a.refreshBangumiCollectionCacheForUser(ctx, u)
			if err != nil {
				failed++
				logs = append(logs, fmt.Sprintf("user #%d (%s): %s", u.UID, u.Username, truncateString(err.Error(), 120)))
				continue
			}
			if lists > 0 {
				refreshedUsers++
				refreshedLists += lists
				cachedEntries += entries
			}
		}
		return map[string]any{"success": failed == 0, "enabled": true, "scanned": scanned, "eligible": eligible, "refreshed_users": refreshedUsers, "refreshed_lists": refreshedLists, "cached_entries": cachedEntries, "failed": failed},
			append(logs, fmt.Sprintf("refreshed %d users, %d lists, %d entries", refreshedUsers, refreshedLists, cachedEntries)), nil
	default:
		return map[string]any{"success": false}, nil, fmt.Errorf("unknown scheduler job: %s", jobID)
	}
}

func schedulerRequestParams(r *http.Request) map[string]any {
	if params, ok := r.Context().Value(schedulerParamsContextKey).(map[string]any); ok {
		return params
	}
	payload := decodeMap(r)
	if params, ok := payload["params"].(map[string]any); ok {
		return params
	}
	if params, ok := payload["runtime_params"].(map[string]any); ok {
		return params
	}
	return nil
}

func (a *App) schedulerEffectiveParams(r *http.Request, jobID string) map[string]any {
	var stored map[string]any
	if schedule, ok := a.store().SchedulerSchedule(jobID); ok {
		stored = schedule.RuntimeParams
	}
	params := a.schedulerRuntimeParamsFromSchedule(jobID, stored)
	requestParams := schedulerRequestParams(r)
	if len(params) == 0 {
		return requestParams
	}
	for key, value := range requestParams {
		params[key] = value
	}
	return params
}

func embyRemoteID(user map[string]any) string {
	return strings.TrimSpace(firstNonEmpty(asString(user["Id"]), asString(user["ID"]), asString(user["id"])))
}

func embyRemoteName(user map[string]any) string {
	return strings.TrimSpace(firstNonEmpty(asString(user["Name"]), asString(user["name"]), asString(user["UserName"]), asString(user["Username"])))
}

func embyRemoteDisabled(user map[string]any) (bool, bool) {
	policy, _ := user["Policy"].(map[string]any)
	if policy == nil {
		policy, _ = user["policy"].(map[string]any)
	}
	if policy == nil {
		return false, false
	}
	value, ok := policy["IsDisabled"]
	if !ok {
		value, ok = policy["is_disabled"]
	}
	if !ok {
		return false, false
	}
	return boolish(value), true
}

func normalizeEmbyName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func isSyntheticEmbyID(id string, uid int64) bool {
	value := strings.TrimSpace(id)
	if value == "" || uid == 0 {
		return false
	}
	return strings.EqualFold(value, fmt.Sprintf("emby_%d", uid))
}

type schedulerParamsKey struct{}
type schedulerManualKey struct{}

var schedulerParamsContextKey schedulerParamsKey
var schedulerManualContextKey schedulerManualKey

func schedulerManualRun(r *http.Request) bool {
	manual, _ := r.Context().Value(schedulerManualContextKey).(bool)
	return manual
}

func schedulerSideEffectContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), 15*time.Second)
}

func jobParamInt(params map[string]any, key string, fallback int) int {
	if params == nil {
		return fallback
	}
	return intValue(params, key, fallback)
}

func jobParamBool(params map[string]any, key string, fallback bool) bool {
	if params == nil {
		return fallback
	}
	return boolValue(params, key, fallback)
}
