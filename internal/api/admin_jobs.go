package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) enforceTelegramMembership(ctx context.Context) (map[string]any, []string, error) {
	chats := telegramChatIDs(a.cfg().TelegramGroupIDs)
	result := map[string]any{
		"enabled": false, "telegram_available": a.telegramAvailable(), "groups": chats,
		"scanned": 0, "disabled": 0, "emby_disabled": 0, "banned": 0, "rejoined_enabled": 0,
		"rejoined_pending_review": 0, "rejoin_candidates": 0, "skipped": 0, "failed": 0,
		"auto_enable_rejoined": a.cfg().TelegramAutoEnableRejoined,
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
	for _, u := range a.store().ListUsers() {
		if u.TelegramID == 0 || u.Role == store.RoleAdmin || u.Role == store.RoleWhitelist {
			result["skipped"] = int(numeric(result["skipped"])) + 1
			continue
		}
		result["scanned"] = int(numeric(result["scanned"])) + 1
		missing, err := a.telegramMembershipMissing(ctx, u.TelegramID, false)
		if err != nil {
			result["failed"] = int(numeric(result["failed"])) + 1
			if len(logs) < 50 {
				logs = append(logs, fmt.Sprintf("failed to check uid=%d tg=%d: %s", u.UID, u.TelegramID, err.Error()))
			}
			continue
		}
		if u.Active && len(missing) > 0 {
			updated, err := a.store().UpdateUser(u.UID, func(u *store.User) error { u.Active = false; return nil })
			if err != nil {
				result["failed"] = int(numeric(result["failed"])) + 1
				continue
			}
			// 立即清除该用户所有 session（redis + memory + PG）。否则 stale token
			// 在 SessionTTL 到期前都还能访问受保护接口。
			a.sessions().DeleteUser(ctx, updated.UID)
			result["disabled"] = int(numeric(result["disabled"])) + 1
			if updated.EmbyID != "" && a.embySetUserEnabled(ctx, updated.EmbyID, false) == nil {
				result["emby_disabled"] = int(numeric(result["emby_disabled"])) + 1
			}
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
			if a.cfg().TelegramAutoEnableRejoined && !a.cfg().TelegramBanOnLeave {
				updated, err := a.store().UpdateUser(u.UID, func(u *store.User) error { u.Active = true; return nil })
				if err != nil {
					result["failed"] = int(numeric(result["failed"])) + 1
					continue
				}
				if updated.EmbyID != "" {
					_ = a.embySetUserEnabled(ctx, updated.EmbyID, a.embyShouldEnableUser(updated))
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
			if entry.IsDir() {
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
