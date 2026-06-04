package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

const (
	telegramMembershipSchedulerCheckTimeout = 8 * time.Second
	telegramMembershipMaxCheckConcurrency   = 64
)

type telegramMembershipCheckResult struct {
	telegramID int64
	missing    []string
	updates    []store.TelegramRosterUpdate
	err        error
}

func (a *App) enforceTelegramMembership(ctx context.Context, autoEnableRejoined bool) (map[string]any, []string, error) {
	chats := telegramChatIDs(a.cfg().TelegramGroupIDs)
	result := map[string]any{
		"enabled": false, "telegram_available": a.telegramAvailable(), "groups": chats,
		"scanned": 0, "disabled": 0, "emby_disabled": 0, "banned": 0, "rejoined_enabled": 0,
		"rejoined_pending_review": 0, "rejoin_candidates": 0, "skipped": 0, "failed": 0,
		"auto_enable_rejoined": autoEnableRejoined,
	}
	rejoinCandidates := []map[string]any{}
	logs := []string{}
	if !a.cfg().TelegramRequireMembership || len(chats) == 0 {
		logs = append(logs, "Telegram membership enforcement disabled")
		return result, logs, nil
	}
	result["enabled"] = true
	if !a.telegramAvailable() {
		logs = append(logs, "Telegram unavailable; membership enforcement skipped")
		return result, logs, nil
	}
	now := time.Now().Unix()
	candidates := []store.User{}
	uniqueTelegramIDs := []int64{}
	seenTelegramIDs := map[int64]bool{}
	for _, u := range a.store().ListUsers() {
		if err := ctx.Err(); err != nil {
			result["terminated"] = true
			return result, append(logs, "job terminated"), err
		}
		if u.TelegramID == 0 || a.userIsProtected(u) {
			result["skipped"] = int(numeric(result["skipped"])) + 1
			continue
		}
		result["scanned"] = int(numeric(result["scanned"])) + 1
		candidates = append(candidates, u)
		if !seenTelegramIDs[u.TelegramID] {
			seenTelegramIDs[u.TelegramID] = true
			uniqueTelegramIDs = append(uniqueTelegramIDs, u.TelegramID)
		}
	}
	concurrency := a.telegramMembershipCheckConcurrency(len(uniqueTelegramIDs))
	result["unique_telegram_ids"] = len(uniqueTelegramIDs)
	result["concurrency"] = concurrency
	checks, rosterUpdates, err := a.checkTelegramMemberships(ctx, uniqueTelegramIDs, chats, concurrency)
	if err != nil {
		result["terminated"] = true
		return result, append(logs, "job terminated"), err
	}
	if err := a.store().ApplyTelegramRosterUpdates(rosterUpdates); err != nil {
		result["failed"] = int(numeric(result["failed"])) + 1
		if len(logs) < 50 {
			logs = append(logs, "failed to update telegram roster: "+err.Error())
		}
	}
	for _, u := range candidates {
		if err := ctx.Err(); err != nil {
			result["terminated"] = true
			return result, append(logs, "job terminated"), err
		}
		check, ok := checks[u.TelegramID]
		if !ok {
			result["failed"] = int(numeric(result["failed"])) + 1
			if len(logs) < 50 {
				logs = append(logs, fmt.Sprintf("failed to check uid=%d tg=%d: missing check result", u.UID, u.TelegramID))
			}
			continue
		}
		missing, err := check.missing, check.err
		if err != nil {
			result["failed"] = int(numeric(result["failed"])) + 1
			if len(logs) < 50 {
				logs = append(logs, fmt.Sprintf("failed to check uid=%d tg=%d: %s", u.UID, u.TelegramID, err.Error()))
			}
			continue
		}
		if u.Active && len(missing) > 0 {
			updated, err := a.store().SetUserActiveAtomic(u.UID, false)
			if err != nil {
				result["failed"] = int(numeric(result["failed"])) + 1
				continue
			}
			// 立即清除该用户所有 session（redis + memory + PG）。否则 stale token
			// 在 SessionTTL 到期前都还能访问受保护接口。
			sideCtx, sideCancel := schedulerSideEffectContext(ctx)
			if disabledRemote, err := a.disableRemoteEmbyForWebState(sideCtx, updated); err == nil && disabledRemote {
				result["emby_disabled"] = int(numeric(result["emby_disabled"])) + 1
			}
			a.sessions().DeleteUser(sideCtx, updated.UID)
			result["disabled"] = int(numeric(result["disabled"])) + 1
			sideCancel()
			if a.cfg().TelegramBanOnLeave {
				for _, chatID := range chats {
					if err := a.telegramBanChatMember(ctx, chatID, updated.TelegramID); err == nil {
						result["banned"] = int(numeric(result["banned"])) + 1
					}
				}
			}
			if len(logs) < 50 {
				logs = append(logs, fmt.Sprintf("disabled uid=%d username=%s missing=%s", updated.UID, updated.Username, strings.Join(missing, ",")))
			}
			continue
		}
		if !u.Active && len(missing) == 0 && (u.ExpiredAt <= 0 || u.ExpiredAt > now) {
			if autoEnableRejoined && !a.cfg().TelegramBanOnLeave {
				updated, err := a.store().UpdateUser(u.UID, func(u *store.User) error { u.Active = true; return nil })
				if err != nil {
					result["failed"] = int(numeric(result["failed"])) + 1
					continue
				}
				result["rejoined_enabled"] = int(numeric(result["rejoined_enabled"])) + 1
				if len(logs) < 50 {
					logs = append(logs, fmt.Sprintf("re-enabled uid=%d username=%s", updated.UID, updated.Username))
				}
				continue
			}
			result["rejoined_pending_review"] = int(numeric(result["rejoined_pending_review"])) + 1
			result["rejoin_candidates"] = int(numeric(result["rejoin_candidates"])) + 1
			if len(rejoinCandidates) < 200 {
				rejoinCandidates = append(rejoinCandidates, map[string]any{"uid": u.UID, "username": u.Username, "telegram_id": u.TelegramID, "emby_bound": u.EmbyID != "", "expired_at": zeroNil(u.ExpiredAt)})
			}
			if len(logs) < 50 {
				logs = append(logs, fmt.Sprintf("rejoin pending review uid=%d username=%s", u.UID, u.Username))
			}
		}
	}
	if len(rejoinCandidates) > 0 {
		result["rejoin_candidate_users"] = rejoinCandidates
	}
	return result, logs, nil
}

func (a *App) telegramMembershipCheckConcurrency(total int) int {
	if total <= 0 {
		return 0
	}
	concurrency := a.cfg().TelegramGroupCheckConcurrency
	if concurrency <= 0 {
		concurrency = 24
	}
	concurrency = clamp(concurrency, 1, telegramMembershipMaxCheckConcurrency)
	if concurrency > total {
		return total
	}
	return concurrency
}

func (a *App) checkTelegramMemberships(ctx context.Context, telegramIDs []int64, chats []string, concurrency int) (map[int64]telegramMembershipCheckResult, []store.TelegramRosterUpdate, error) {
	checks := make(map[int64]telegramMembershipCheckResult, len(telegramIDs))
	if len(telegramIDs) == 0 {
		return checks, nil, nil
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	jobs := make(chan int64)
	results := make(chan telegramMembershipCheckResult, len(telegramIDs))
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for telegramID := range jobs {
				if err := ctx.Err(); err != nil {
					results <- telegramMembershipCheckResult{telegramID: telegramID, err: err}
					continue
				}
				missing, updates, err := a.telegramMembershipMissingForScheduler(ctx, telegramID, chats)
				results <- telegramMembershipCheckResult{telegramID: telegramID, missing: missing, updates: updates, err: err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, telegramID := range telegramIDs {
			select {
			case <-ctx.Done():
				return
			case jobs <- telegramID:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()
	rosterUpdates := []store.TelegramRosterUpdate{}
	for result := range results {
		checks[result.telegramID] = result
		rosterUpdates = append(rosterUpdates, result.updates...)
	}
	if err := ctx.Err(); err != nil {
		return checks, rosterUpdates, err
	}
	return checks, rosterUpdates, nil
}

func (a *App) telegramMembershipMissingForScheduler(ctx context.Context, telegramID int64, chats []string) ([]string, []store.TelegramRosterUpdate, error) {
	missing := []string{}
	updates := []store.TelegramRosterUpdate{}
	if len(chats) == 0 || telegramID == 0 {
		return missing, updates, nil
	}
	for _, chatID := range chats {
		if err := ctx.Err(); err != nil {
			return missing, updates, err
		}
		member, err := a.telegramGetChatMemberWithTimeout(ctx, chatID, telegramID, telegramMembershipSchedulerCheckTimeout)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return missing, updates, ctxErr
			}
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "not found") || strings.Contains(msg, "participant") || strings.Contains(msg, "user not found") {
				missing = append(missing, chatID)
				updates = append(updates, store.TelegramRosterUpdate{ChatID: chatID, TelegramID: telegramID, Status: "left"})
				continue
			}
			if !telegramRateLimitPauseContext(ctx, err) {
				return missing, updates, ctx.Err()
			}
			return missing, updates, err
		}
		status := strings.ToLower(asString(member["status"]))
		if status == "left" || status == "kicked" {
			missing = append(missing, chatID)
			updates = append(updates, store.TelegramRosterUpdate{ChatID: chatID, TelegramID: telegramID, Status: status})
			continue
		}
		user, _ := member["user"].(map[string]any)
		updates = append(updates, store.TelegramRosterUpdate{ChatID: chatID, TelegramID: telegramID, Status: firstNonEmpty(status, "member"), IsBot: boolish(user["is_bot"])})
	}
	return missing, updates, nil
}

func (a *App) cleanupUnusedUploadAssets(maxAge time.Duration) map[string]any {
	result := map[string]any{"scanned": 0, "deleted": 0, "skipped_recent": 0, "failed": 0}
	root, err := filepath.Abs(a.cfg().UploadDir)
	if err != nil {
		result["failed"] = 1
		result["error"] = err.Error()
		return result
	}
	referenced := map[string]bool{}
	for _, u := range a.store().ListUsers() {
		addUploadReference(referenced, u.Avatar)
		addUploadReference(referenced, u.Background)
	}
	for _, kind := range []string{"avatar", "background", "avatars", "backgrounds"} {
		dir := filepath.Join(root, kind)
		absDir, err := filepath.Abs(dir)
		if err != nil || !isSubpath(root, absDir) {
			result["failed"] = int(numeric(result["failed"])) + 1
			continue
		}
		entries, err := os.ReadDir(absDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			result["failed"] = int(numeric(result["failed"])) + 1
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			result["scanned"] = int(numeric(result["scanned"])) + 1
			filename := entry.Name()
			if referenced[uploadRefKey(kind, filename)] {
				continue
			}
			path := filepath.Join(absDir, filename)
			info, err := entry.Info()
			if err != nil {
				result["failed"] = int(numeric(result["failed"])) + 1
				continue
			}
			if !info.Mode().IsRegular() {
				continue
			}
			if time.Since(info.ModTime()) < maxAge {
				result["skipped_recent"] = int(numeric(result["skipped_recent"])) + 1
				continue
			}
			if err := os.Remove(path); err != nil {
				result["failed"] = int(numeric(result["failed"])) + 1
				continue
			}
			result["deleted"] = int(numeric(result["deleted"])) + 1
		}
	}
	return result
}

func addUploadReference(refs map[string]bool, raw string) {
	kind, filename, ok := extractUploadReference(raw)
	if !ok {
		return
	}
	for _, alias := range uploadKindAliases(kind) {
		refs[uploadRefKey(alias, filename)] = true
	}
}

func extractUploadReference(raw string) (string, string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "", false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "url(") && strings.HasSuffix(value, ")") {
		value = strings.TrimSpace(value[4 : len(value)-1])
		value = strings.Trim(value, `"'`)
	}
	for _, prefix := range []string{"/api/v1/users/assets/", "/uploads/"} {
		if !strings.HasPrefix(value, prefix) {
			continue
		}
		rel := strings.TrimPrefix(value, prefix)
		parts := strings.SplitN(rel, "/", 2)
		if len(parts) != 2 {
			return "", "", false
		}
		kind := strings.TrimSpace(parts[0])
		filename := filepath.Base(parts[1])
		if kind == "" || filename == "." || filename == string(filepath.Separator) || strings.Contains(parts[1], "..") {
			return "", "", false
		}
		return kind, filename, true
	}
	return "", "", false
}

func uploadKindAliases(kind string) []string {
	switch kind {
	case "avatar", "avatars":
		return []string{"avatar", "avatars"}
	case "background", "backgrounds":
		return []string{"background", "backgrounds"}
	default:
		return []string{kind}
	}
}

func uploadRefKey(kind, filename string) string {
	return kind + "/" + filename
}

func isSubpath(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}
