package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleSendReminders(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	defaultDays := a.cfg.NotificationExpiryRemindDays
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
	for _, u := range a.store.ListUsers() {
		if u.Active && u.ExpiredAt > now && u.ExpiredAt <= deadline {
			remaining := u.ExpiredAt - now
			item := map[string]any{"uid": u.UID, "username": u.Username, "telegram_id": nullableInt(u.TelegramID), "expired_at": u.ExpiredAt, "remaining_seconds": remaining, "remaining_str": formatSeconds(remaining)}
			users = append(users, item)
			if !a.cfg.NotificationEnabled || !a.telegramAvailable() || u.TelegramID == 0 {
				continue
			}
			text := fmt.Sprintf("%s，您的账号将在 %s 后到期，请及时续期。", u.Username, formatSeconds(remaining))
			if err := a.telegramSendMessage(ctx, u.TelegramID, text); err != nil {
				failedItems = append(failedItems, map[string]any{"uid": u.UID, "username": u.Username, "telegram_id": u.TelegramID, "error": err.Error()})
				continue
			}
			sent++
		}
	}
	return map[string]any{"sent": sent, "total": len(users), "count": len(users), "users": users, "failed": failedItems, "telegram_enabled": a.telegramAvailable(), "notification_enabled": a.cfg.NotificationEnabled, "days": days}
}

func (a *App) handleSchedulerRunV2(w http.ResponseWriter, r *http.Request, params Params) {
	now := time.Now().Unix()
	summary, logs, err := a.runSchedulerJob(r, params["job_id"])
	status := "success"
	message := "job completed"
	errText := ""
	if err != nil {
		status = "failed"
		message = err.Error()
		errText = err.Error()
	}
	finished := time.Now().Unix()
	run := store.SchedulerRun{JobID: params["job_id"], Type: "manual", Trigger: "manual", Status: status, Message: message, StartedAt: now, FinishedAt: finished, EndedAt: finished, Summary: summary, Logs: logs, Error: errText}
	_ = a.store.AddSchedulerRun(run)
	ok(w, "job executed", map[string]any{"job_id": run.JobID, "last_run": run})
}

func (a *App) runSchedulerJob(r *http.Request, jobID string) (map[string]any, []string, error) {
	if !schedulerJobExists(jobID) {
		return map[string]any{"success": false}, nil, fmt.Errorf("job not found")
	}
	params := schedulerRequestParams(r)
	now := time.Now().Unix()
	switch jobID {
	case "check_expired":
		disabled := 0
		embyDisabled := 0
		for _, u := range a.store.ListUsers() {
			if u.Active && u.ExpiredAt > 0 && u.ExpiredAt < now {
				// For invited users (have invite relation), only disable Emby access
				// but keep the account active so they can still log in and renew
				_, isInvited := a.store.ParentOf(u.UID)
				if isInvited {
					// Only disable Emby, keep account active
					if u.EmbyID != "" && a.embySetUserEnabled(r.Context(), u.EmbyID, false) == nil {
						embyDisabled++
					}
					disabled++
				} else {
					// Non-invited users: disable the whole account
					updated, err := a.store.UpdateUser(u.UID, func(u *store.User) error { u.Active = false; return nil })
					if err == nil {
						disabled++
						if updated.EmbyID != "" && a.embySetUserEnabled(r.Context(), updated.EmbyID, false) == nil {
							embyDisabled++
						}
					}
				}
			}
		}
		return map[string]any{"success": true, "disabled": disabled, "emby_disabled": embyDisabled}, []string{fmt.Sprintf("disabled %d expired users", disabled)}, nil
	case "check_expiring", "expiry_reminders":
		defaultDays := a.cfg.NotificationExpiryRemindDays
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
		for _, u := range a.store.ListUsers() {
			if u.Active && u.ExpiredAt > now && u.ExpiredAt <= deadline {
				count++
			}
		}
		return map[string]any{"success": true, "expiring": count, "days": days}, []string{fmt.Sprintf("found %d expiring users", count)}, nil
	case "daily_stats":
		users := a.store.ListUsers()
		return map[string]any{"success": true, "users": len(users), "active": countActive(users)}, []string{"daily stats generated"}, nil
	case "cleanup_sessions":
		if a.cfg.EmbyURL == "" {
			return map[string]any{"success": true, "configured": false, "active": 0, "total": 0}, []string{"Emby not configured"}, nil
		}
		var sessions []map[string]any
		if err := a.embyGet(r.Context(), "/Sessions", &sessions); err != nil {
			return map[string]any{"success": false}, nil, err
		}
		active := 0
		for _, session := range sessions {
			if asString(session["NowPlayingItem"]) != "" || session["NowPlayingItem"] != nil {
				active++
			}
		}
		return map[string]any{"success": true, "active": active, "total": len(sessions)}, []string{fmt.Sprintf("read %d Emby sessions", len(sessions))}, nil
	case "emby_sync":
		if a.cfg.EmbyURL == "" {
			return map[string]any{"success": true, "configured": false}, []string{"Emby not configured"}, nil
		}
		var remote []map[string]any
		if err := a.embyGet(r.Context(), "/Users", &remote); err != nil {
			return map[string]any{"success": false}, nil, err
		}
		remoteByID := map[string]map[string]any{}
		for _, user := range remote {
			if id := asString(user["Id"]); id != "" {
				remoteByID[id] = user
			}
		}
		updatedNames, syncedState, missing := 0, 0, 0
		for _, u := range a.store.ListUsers() {
			if u.EmbyID == "" {
				continue
			}
			remoteUser, okRemote := remoteByID[u.EmbyID]
			if !okRemote {
				missing++
				continue
			}
			name := asString(remoteUser["Name"])
			if name != "" && name != u.EmbyUsername {
				if _, err := a.store.UpdateUser(u.UID, func(u *store.User) error { u.EmbyUsername = name; return nil }); err == nil {
					updatedNames++
				}
			}
			if a.embySetUserEnabled(r.Context(), u.EmbyID, a.embyShouldEnableUser(u)) == nil {
				syncedState++
			}
		}
		return map[string]any{"success": true, "remote_users": len(remote), "updated_names": updatedNames, "synced_state": syncedState, "missing": missing}, []string{fmt.Sprintf("read %d Emby users", len(remote))}, nil
	case "cleanup_no_emby":
		ignoreEnabled := jobParamBool(params, "ignore_enabled_flag", false)
		if !a.cfg.AutoCleanupNoEmby && !ignoreEnabled {
			return map[string]any{"success": true, "enabled": false, "deleted": 0}, []string{"auto cleanup no-Emby disabled"}, nil
		}
		days := jobParamInt(params, "days", queryInt(r, "days", a.cfg.AutoCleanupNoEmbyDays))
		if days <= 0 {
			days = 7
		}
		threshold := int64(0)
		if days > 0 {
			threshold = time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
		}
		preserveTG := jobParamBool(params, "preserve_tg_bound", a.cfg.EmbyDirectRegisterEnabled)
		dryRun := jobParamBool(params, "dry_run", false)
		candidates := 0
		deleted := 0
		failed := 0
		for _, u := range a.store.ListUsers() {
			if u.Role == store.RoleAdmin || u.Role == store.RoleWhitelist || u.EmbyID != "" {
				continue
			}
			if preserveTG && u.TelegramID != 0 {
				continue
			}
			if threshold > 0 && u.RegisterTime > threshold {
				continue
			}
			candidates++
			if dryRun {
				continue
			}
			if err := a.store.DeleteUser(u.UID); err != nil {
				failed++
			} else {
				deleted++
			}
		}
		return map[string]any{"success": true, "enabled": true, "candidates": candidates, "deleted": deleted, "failed": failed, "dry_run": dryRun, "days": days, "preserve_tg_bound": preserveTG}, []string{fmt.Sprintf("processed %d no-Emby users", candidates)}, nil
	case "enforce_group_membership":
		result, logs, err := a.enforceTelegramMembership(r.Context())
		result["success"] = err == nil
		return result, logs, err
	case "check_telegram_bindings":
		seen := map[int64]int64{}
		duplicates := 0
		for _, u := range a.store.ListUsers() {
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
		chats := telegramChatIDs(a.cfg.TelegramGroupIDs)
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
					logs = append(logs, fmt.Sprintf("failed to inspect tg=%d uid=%d: %s", target.TelegramID, target.UID, err.Error()))
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
					logs = append(logs, fmt.Sprintf("failed to kick tg=%d uid=%d: %s", target.TelegramID, target.UID, err.Error()))
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
	case "cleanup_unused_uploads":
		result := a.cleanupUnusedUploadAssets(24 * time.Hour)
		result["success"] = true
		return result, []string{fmt.Sprintf("scanned %d upload files, deleted %d", int(numeric(result["scanned"])), int(numeric(result["deleted"])))}, nil
	case "system_auto_update":
		if !a.cfg.SystemUpdateEnabled {
			return map[string]any{"success": true, "skipped": true, "enabled": false}, []string{"system auto update disabled"}, nil
		}
		result := applyGitUpdate(r.Context(), a.cfg.SystemUpdateRepoURL, a.cfg.SystemUpdateBranch, a.cfg.SystemUpdateRestartServices, false, false)
		if !boolish(result["success"]) {
			return result, nil, fmt.Errorf("%s", asString(result["message"]))
		}
		return result, []string{asString(result["message"])}, nil
	default:
		return map[string]any{"success": false}, nil, fmt.Errorf("unknown scheduler job: %s", jobID)
	}
}

func schedulerRequestParams(r *http.Request) map[string]any {
	payload := decodeMap(r)
	if params, ok := payload["params"].(map[string]any); ok {
		return params
	}
	return payload
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
