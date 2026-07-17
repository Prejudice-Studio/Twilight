package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/security"
	"github.com/prejudice-studio/twilight/internal/store"
)

const (
	adminClearEmailsConfirmMessage = "请输入确认短语 " + confirmClearAllEmails + " 后再执行"
	adminClearEmailsSuccessMessage = "user emails cleared"
)

func (a *App) handleEmbySyncV2(w http.ResponseWriter, r *http.Request, _ Params) {
	updated := 0
	missing := []map[string]any{}
	if a.requireEmbyConfigured(w) {
		return
	}
	var remote []map[string]any
	if err := a.embyGet(r.Context(), "/Users", &remote); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyRemoteUsersFailed, "读取 Emby 用户列表失败，请稍后重试或检查上游 Emby 状态")
		return
	}
	remoteByID := map[string]map[string]any{}
	for _, user := range remote {
		if id := asString(user["Id"]); id != "" {
			remoteByID[id] = user
		}
	}
	for _, u := range a.store().ListUsers() {
		if u.EmbyID == "" {
			continue
		}
		remoteUser, exists := remoteByID[u.EmbyID]
		if !exists {
			missing = append(missing, map[string]any{"uid": u.UID, "username": u.Username, "emby_id": u.EmbyID})
			continue
		}
		name := asString(remoteUser["Name"])
		if name != "" && name != u.EmbyUsername {
			if _, err := a.store().UpdateUser(u.UID, func(u *store.User) error { u.EmbyUsername = name; return nil }); err == nil {
				updated++
			}
		}
	}
	ok(w, "sync complete", map[string]any{"success": updated, "failed": len(missing), "errors": missing, "updated": updated, "missing": missing})
}

func (a *App) handleEmbyActivity(w http.ResponseWriter, r *http.Request, _ Params) {
	limit := clamp(queryInt(r, "limit", 50), 1, 200)
	if a.requireEmbyConfigured(w) {
		return
	}
	var payload map[string]any
	if err := a.embyGet(r.Context(), "/System/ActivityLog/Entries?StartIndex=0&Limit="+strconv.Itoa(limit), &payload); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyRemoteActivityFail, "读取 Emby 活动日志失败，请稍后重试或检查上游 Emby 状态")
		return
	}
	if items, okItems := payload["Items"]; okItems {
		ok(w, "OK", items)
		return
	}
	ok(w, "OK", payload)
}

func (a *App) handleAdminEmbyUsersV2(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.requireEmbyConfigured(w) {
		return
	}
	var remote []map[string]any
	if err := a.embyGet(r.Context(), "/Users", &remote); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyRemoteUsersFailed, "读取 Emby 用户列表失败，请稍后重试或检查上游 Emby 状态")
		return
	}
	users := a.store().ListUsers()
	localByEmbyID := map[string]store.User{}
	for _, u := range users {
		if u.EmbyID != "" {
			localByEmbyID[u.EmbyID] = u
		}
	}
	embyUsers := make([]map[string]any, 0, len(remote))
	seen := map[string]bool{}
	for _, eu := range remote {
		id := asString(eu["Id"])
		name := asString(eu["Name"])
		seen[id] = true
		policy := embyPolicy(eu)
		local := any(nil)
		status := "unlinked"
		if u, okUser := localByEmbyID[id]; okUser {
			local = map[string]any{"uid": u.UID, "username": u.Username, "telegram_id": nullableInt(u.TelegramID), "active": u.Active, "role": u.Role}
			status = "synced"
			if u.EmbyUsername != "" && !strings.EqualFold(u.EmbyUsername, name) {
				status = "name_mismatch"
			}
		}
		embyUsers = append(embyUsers, map[string]any{
			"emby_id": id, "emby_name": name, "has_password": eu["HasPassword"],
			"is_admin": boolish(policy["IsAdministrator"]), "is_disabled": boolish(policy["IsDisabled"]), "is_hidden": boolish(policy["IsHidden"]),
			"last_login": emptyNil(asString(eu["LastLoginDate"])), "last_activity": emptyNil(firstNonEmpty(asString(eu["LastActivityDate"]), asString(eu["DateLastActivity"]))),
			"local_user": local, "sync_status": status,
		})
	}
	orphans := []map[string]any{}
	for _, u := range users {
		if u.EmbyID != "" && !seen[u.EmbyID] {
			orphans = append(orphans, map[string]any{"uid": u.UID, "username": u.Username, "emby_id": u.EmbyID, "telegram_id": nullableInt(u.TelegramID)})
		}
	}
	ok(w, "OK", map[string]any{"emby_users": embyUsers, "users": embyUsers, "orphans": orphans, "total": len(embyUsers), "total_emby": len(embyUsers), "total_linked": len(localByEmbyID), "total_orphans": len(orphans)})
}

func (a *App) handleEmbyBroadcast(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	text := stringValue(payload, "text")
	if text == "" {
		failWithCode(w, http.StatusBadRequest, ErrEmbyBroadcastTextEmpty, "请填写广播消息内容")
		return
	}
	header := firstNonEmpty(stringValue(payload, "header"), "通知")
	userIDs := map[string]bool{}
	for _, id := range stringSlice(payload["user_ids"]) {
		userIDs[id] = true
	}
	sessions, err := a.embySessionsSnapshot(r.Context(), false)
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyRemoteSessionsFail, "读取 Emby 会话失败，请稍后重试或检查上游 Emby 状态")
		return
	}
	sent := 0
	failedItems := []map[string]any{}
	for _, session := range sessions {
		userID := asString(session["UserId"])
		if len(userIDs) > 0 && !userIDs[userID] {
			continue
		}
		sid := asString(session["Id"])
		if sid == "" {
			continue
		}
		var ignored map[string]any
		err := a.embyPost(r.Context(), "/Sessions/"+urlPathEscape(sid)+"/Message", map[string]any{"Header": header, "Text": text, "TimeoutMs": 10000}, &ignored)
		if err != nil {
			failedItems = append(failedItems, map[string]any{"session_id": sid, "error": err.Error()})
			continue
		}
		sent++
	}
	ok(w, "broadcast complete", map[string]any{"sent_count": sent, "failed": failedItems})
}

func (a *App) handleEmbyConnectivityTest(w http.ResponseWriter, r *http.Request, _ Params) {
	configuredURL := strings.TrimSpace(a.cfg().EmbyURL) != ""
	configuredToken := strings.TrimSpace(a.cfg().EmbyToken) != ""
	tests := []map[string]any{
		{"name": "configuration_url", "success": configuredURL, "message": "Emby URL configured"},
		{"name": "configuration_token", "success": configuredToken, "message": "Emby API token configured"},
	}
	overall := configuredURL && configuredToken
	var info map[string]any
	if configuredURL && configuredToken {
		start := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		got, err := a.embyHealthDetailed(ctx)
		cancel()
		success := err == nil
		overall = overall && success
		message := "OK"
		if !success {
			message = truncateString(redactSensitiveText(err.Error()), 180)
		} else if got != nil {
			info = got
		}
		tests = append(tests, map[string]any{"name": "backend_server_info", "success": success, "latency_ms": time.Since(start).Milliseconds(), "message": message})

		start = time.Now()
		var users []map[string]any
		ctx, cancel = context.WithTimeout(r.Context(), 8*time.Second)
		usersErr := a.embyGet(ctx, "/Users", &users)
		cancel()
		usersOK := usersErr == nil
		overall = overall && usersOK
		usersMessage := "OK"
		if usersErr != nil {
			usersMessage = truncateString(redactSensitiveText(usersErr.Error()), 180)
		}
		tests = append(tests, map[string]any{"name": "backend_users", "success": usersOK, "latency_ms": time.Since(start).Milliseconds(), "message": usersMessage, "count": len(users)})

		start = time.Now()
		var libraries []map[string]any
		ctx, cancel = context.WithTimeout(r.Context(), 8*time.Second)
		libraryErr := a.embyGet(ctx, "/Library/VirtualFolders", &libraries)
		cancel()
		libraryOK := libraryErr == nil
		overall = overall && libraryOK
		libraryMessage := "OK"
		if libraryErr != nil {
			libraryMessage = truncateString(redactSensitiveText(libraryErr.Error()), 180)
		}
		tests = append(tests, map[string]any{"name": "backend_libraries", "success": libraryOK, "latency_ms": time.Since(start).Milliseconds(), "message": libraryMessage, "count": len(libraries)})

		for _, candidate := range a.embyBackendLocalProbeCandidates() {
			start = time.Now()
			ctx, cancel = context.WithTimeout(r.Context(), 4*time.Second)
			localInfo, localErr := a.embyHealthAt(ctx, candidate)
			cancel()
			localOK := localErr == nil
			localMessage := "OK"
			if localErr != nil {
				localMessage = truncateString(redactSensitiveText(localErr.Error()), 180)
			}
			tests = append(tests, map[string]any{
				"name":       "backend_local_" + candidate,
				"success":    localOK,
				"latency_ms": time.Since(start).Milliseconds(),
				"message":    localMessage,
				"server":     firstNonEmpty(asString(localInfo["ServerName"]), asString(localInfo["Name"])),
				"version":    asString(localInfo["Version"]),
			})
		}
	}
	ok(w, "OK", map[string]any{"success": overall, "url": a.cfg().EmbyURL, "emby_url": a.cfg().EmbyURL, "tests": tests, "overall": overall, "server_info": info})
}

func (a *App) embyBackendLocalProbeCandidates() []string {
	configured := normalizeProbeURL(a.cfg().EmbyURL)
	candidates := []string{"http://127.0.0.1:8096", "http://localhost:8096"}
	out := make([]string, 0, len(candidates))
	seen := map[string]bool{}
	for _, candidate := range candidates {
		normalized := normalizeProbeURL(candidate)
		if normalized == "" || normalized == configured || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	return out
}

func (a *App) embyHealthAt(ctx context.Context, base string) (map[string]any, error) {
	target, err := validateEmbyURL(base)
	if err != nil {
		return nil, err
	}
	var info map[string]any
	publicErr := getJSON(ctx, target+"/System/Info/Public", a.embyHeaders(), &info)
	if publicErr == nil && info != nil {
		return info, nil
	}
	info = nil
	infoErr := getJSON(ctx, target+"/System/Info", a.embyHeaders(), &info)
	if infoErr == nil && info != nil {
		return info, nil
	}
	if publicErr != nil && infoErr != nil {
		return nil, publicErr
	}
	if publicErr != nil {
		return nil, publicErr
	}
	if infoErr != nil {
		return nil, infoErr
	}
	return nil, context.Canceled
}

func (a *App) handleEmbyCleanupOrphans(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.requireEmbyConfigured(w) {
		return
	}
	var remote []map[string]any
	if err := a.embyGet(r.Context(), "/Users", &remote); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyRemoteUsersFailed, "读取 Emby 用户列表失败，请稍后重试或检查上游 Emby 状态")
		return
	}
	remoteIDs := map[string]bool{}
	for _, eu := range remote {
		remoteIDs[asString(eu["Id"])] = true
	}
	cleaned := []map[string]any{}
	for _, u := range a.store().ListUsers() {
		if u.EmbyID == "" || remoteIDs[u.EmbyID] {
			continue
		}
		oldID := u.EmbyID
		updated, err := a.store().UpdateUser(u.UID, func(u *store.User) error { u.EmbyID = ""; u.EmbyUsername = ""; return nil })
		if err == nil {
			cleaned = append(cleaned, map[string]any{"uid": updated.UID, "username": updated.Username, "old_emby_id": oldID})
		}
	}
	ok(w, "cleanup complete", map[string]any{"cleaned": cleaned, "count": len(cleaned)})
}

func (a *App) handleEmbyImportUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.requireEmbyConfigured(w) {
		return
	}
	payload := decodeMap(r)
	targetIDs := map[string]bool{}
	for _, id := range stringSlice(payload["emby_ids"]) {
		targetIDs[id] = true
	}
	var remote []map[string]any
	if err := a.embyGet(r.Context(), "/Users", &remote); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyRemoteUsersFailed, "读取 Emby 用户列表失败，请稍后重试或检查上游 Emby 状态")
		return
	}
	linked := map[string]bool{}
	for _, u := range a.store().ListUsers() {
		if u.EmbyID != "" {
			linked[u.EmbyID] = true
		}
	}
	unlinked := []map[string]any{}
	skipped := []map[string]any{}
	for _, eu := range remote {
		id := asString(eu["Id"])
		name := asString(eu["Name"])
		policy := embyPolicy(eu)
		if boolish(policy["IsAdministrator"]) {
			skipped = append(skipped, map[string]any{"emby_id": id, "name": name, "reason": "admin"})
			continue
		}
		if len(targetIDs) > 0 && !targetIDs[id] {
			skipped = append(skipped, map[string]any{"emby_id": id, "name": name, "reason": "filtered"})
			continue
		}
		if linked[id] {
			skipped = append(skipped, map[string]any{"emby_id": id, "name": name, "reason": "linked"})
			continue
		}
		unlinked = append(unlinked, map[string]any{"emby_id": id, "emby_name": name, "is_disabled": boolish(policy["IsDisabled"]), "is_hidden": boolish(policy["IsHidden"])})
	}
	ok(w, "scan complete", map[string]any{"unlinked": unlinked, "skipped": skipped, "unlinked_count": len(unlinked), "skipped_count": len(skipped)})
}

func (a *App) handleEmbyResetBindings(w http.ResponseWriter, r *http.Request, _ Params) {
	if stringValue(decodeMap(r), "confirm") != "RESET_ALL_EMBY" {
		failWithCode(w, http.StatusBadRequest, ErrAdminEmbyResetConfirm, "请输入确认短语 RESET_ALL_EMBY 后再执行")
		return
	}
	targets := []store.User{}
	for _, u := range a.store().ListUsers() {
		if u.EmbyID == "" && u.EmbyUsername == "" && !u.PendingEmby {
			continue
		}
		targets = append(targets, u)
	}
	remoteDisabledByUID := map[int64]bool{}
	for _, u := range targets {
		disabled, proceed := a.disableRemoteEmbyForUnbind(w, r, u.EmbyID)
		if !proceed {
			return
		}
		remoteDisabledByUID[u.UID] = disabled
	}
	count := 0
	remoteDisabled := 0
	failedItems := []map[string]any{}
	for _, target := range targets {
		if remoteDisabledByUID[target.UID] {
			remoteDisabled++
		}
		if _, err := a.store().UpdateUser(target.UID, func(u *store.User) error {
			if target.EmbyID != "" && u.EmbyID != "" && u.EmbyID != target.EmbyID {
				return store.ErrConflict
			}
			u.EmbyID = ""
			u.EmbyUsername = ""
			u.PendingEmby = false
			u.PendingEmbyDays = nil
			return nil
		}); err == nil {
			count++
		} else {
			failedItems = append(failedItems, map[string]any{"uid": target.UID, "username": target.Username, "error": err.Error()})
		}
	}
	ok(w, "bindings reset", map[string]any{"count": count, "remote_disabled": remoteDisabled, "failed": failedItems})
}

func (a *App) handleEmbyDeleteUnlinked(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.requireEmbyConfigured(w) {
		return
	}
	dryRun := boolValue(decodeMap(r), "dry_run", false)
	var remote []map[string]any
	if err := a.embyGet(r.Context(), "/Users", &remote); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyRemoteUsersFailed, "读取 Emby 用户列表失败，请稍后重试或检查上游 Emby 状态")
		return
	}
	linked := map[string]bool{}
	for _, u := range a.store().ListUsers() {
		if u.EmbyID != "" {
			linked[u.EmbyID] = true
		}
	}
	candidates := []map[string]any{}
	deleted := []map[string]any{}
	failedItems := []map[string]any{}
	for _, eu := range remote {
		id := asString(eu["Id"])
		name := asString(eu["Name"])
		policy := embyPolicy(eu)
		if id == "" || linked[id] || boolish(policy["IsAdministrator"]) {
			continue
		}
		record := map[string]any{"emby_id": id, "emby_name": name, "is_disabled": boolish(policy["IsDisabled"]), "is_hidden": boolish(policy["IsHidden"])}
		candidates = append(candidates, record)
		if dryRun {
			continue
		}
		if err := a.embyDelete(r.Context(), "/Users/"+urlPathEscape(id)); err != nil {
			failedItems = append(failedItems, map[string]any{"emby_id": id, "emby_name": name, "reason": err.Error()})
			continue
		}
		deleted = append(deleted, record)
	}
	ok(w, "delete complete", map[string]any{"candidates": candidates, "deleted": deleted, "failed": failedItems, "count": len(candidates), "dry_run": dryRun})
}

func (a *App) handleCreateStandaloneEmbyV2(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	username := stringValue(payload, "username")
	password := stringValue(payload, "password")
	if username == "" || len(username) > 64 {
		failWithCode(w, http.StatusBadRequest, ErrEmbyUsernameInvalid, "Emby 用户名不合法，长度需在 1-64 之间")
		return
	}
	if len(password) < 8 {
		failWithCode(w, http.StatusBadRequest, ErrEmbyPasswordTooShort, "密码长度需至少 8 位")
		return
	}
	if a.requireEmbyConfigured(w) {
		return
	}
	var createdUser map[string]any
	if err := a.embyPost(r.Context(), "/Users/New", map[string]any{"Name": username}, &createdUser); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyCreateFailed, "创建 Emby 用户失败，请稍后重试或检查上游 Emby 状态")
		return
	}
	embyID := asString(createdUser["Id"])
	if embyID == "" {
		failWithCode(w, http.StatusBadGateway, ErrEmbyCreateNoID, "Emby 未返回用户 ID，请检查上游 Emby 状态")
		return
	}
	if err := a.embyUpdatePolicy(r.Context(), embyID, func(policy map[string]any) {}); err != nil {
		_ = a.embyDelete(r.Context(), "/Users/"+urlPathEscape(embyID))
		failWithCode(w, http.StatusBadGateway, ErrEmbyCreateFailed, "设置 Emby 用户权限失败，请稍后重试或检查上游 Emby 状态")
		return
	}
	if err := a.embyPost(r.Context(), "/Users/"+urlPathEscape(embyID)+"/Password", map[string]any{"CurrentPw": "", "NewPw": password}, nil); err != nil {
		_ = a.embyDelete(r.Context(), "/Users/"+urlPathEscape(embyID))
		failWithCode(w, http.StatusBadGateway, ErrEmbySetPasswordFailed, "设置 Emby 用户密码失败，请稍后重试或检查上游 Emby 状态")
		return
	}
	ok(w, "Emby user created", map[string]any{"emby_id": embyID, "emby_username": firstNonEmpty(asString(createdUser["Name"]), username)})
}

func (a *App) handleWhitelist(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	username := stringValue(payload, "username")
	if username == "" {
		failWithCode(w, http.StatusBadRequest, ErrAdminWhitelistUsernameEmpty, "请填写用户名")
		return
	}
	password := "Twilight-" + randomCode(14)
	hash, err := security.HashPassword(password)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrPasswordHashFailed, "密码哈希失败，请稍后重试")
		return
	}
	u, err := a.store().CreateUser(store.User{
		Username: username, Email: stringValue(payload, "email"), TelegramID: int64(intValue(payload, "telegram_id", 0)),
		Role: store.RoleWhitelist, Active: true, ExpiredAt: 253402214400, PasswordHash: hash,
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "whitelist user created", map[string]any{"username": u.Username, "password": password, "user": publicUser(u)})
}

func (a *App) handleAdminBulkExpire(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	if stringValue(payload, "confirm") != "BULK_EXPIRE_OK" {
		failWithCode(w, http.StatusBadRequest, ErrAdminBulkExpireConfirm, "请输入确认短语 BULK_EXPIRE_OK 后再执行")
		return
	}
	expiredAt := numeric(payload["expired_at"])
	if expiredAt == 0 {
		if _, ok := payload["days"]; !ok {
			failWithCode(w, http.StatusBadRequest, ErrAdminBulkExpireInvalid, "必须提供 expired_at 或 days")
			return
		}
		days := normalizeRegCodeDays(intValue(payload, "days", 0))
		if days > 36500 {
			failWithCode(w, http.StatusBadRequest, ErrAdminBulkExpireDaysTooLarge, "过期天数超出允许范围")
			return
		}
		if days < 0 {
			expiredAt = permanentExpiryUnix
		} else {
			expiredAt = time.Now().AddDate(0, 0, days).Unix()
		}
	}
	if expiryIsPermanent(expiredAt) {
		expiredAt = permanentExpiryUnix
	}
	if expiredAt == 0 || expiredAt < -1 {
		failWithCode(w, http.StatusBadRequest, ErrAdminBulkExpireInvalid, "过期时间不合法")
		return
	}
	includeAdmin := boolValue(payload, "include_admin", false)
	includeWhitelist := boolValue(payload, "include_whitelist", false)
	affectedUIDs := make([]int64, 0)
	matched, updated, skipped := a.updateFilteredUsers(payload["filter"], func(u store.User) (bool, string) {
		if u.Role == store.RoleAdmin && !includeAdmin {
			return false, "admin"
		}
		if a.configuredAdminMatch(u.UID, u.Username) && !includeAdmin {
			return false, "configured_admin"
		}
		if u.Role == store.RoleWhitelist && !includeWhitelist {
			return false, "whitelist"
		}
		if u.Role == store.RoleUnrecognized {
			return false, "unrecognized"
		}
		if u.UID == current(r).User.UID && !expiryIsPermanent(expiredAt) {
			return false, "current_admin"
		}
		if u.EmbyID == "" || u.PendingEmby {
			return false, "pending_emby"
		}
		return true, ""
	}, func(u *store.User) {
		u.ExpiredAt = expiredAt
		affectedUIDs = append(affectedUIDs, u.UID)
	})
	// 若新到期时间已使账号过期，必须同步关停其 Emby 账号——否则会留下「Web 已过期、
	// Emby 仍可登录」的越权窗口（与单用户禁用、调度自动过期口径一致）。到期时间设为
	// 未来/永久时 disableRemoteEmbyForWebState 会自行判定不需要关停，安全空转。
	embyDisabled, embySyncFailed := 0, 0
	if !expiryIsPermanent(expiredAt) && expiredAt < time.Now().Unix() {
		for _, uid := range affectedUIDs {
			target, found := a.store().User(uid)
			if !found {
				continue
			}
			if changed, err := a.disableRemoteEmbyForWebState(r.Context(), target); err != nil {
				embySyncFailed++
			} else if changed {
				embyDisabled++
			}
		}
	}
	resp := map[string]any{
		"matched":              matched,
		"updated":              updated,
		"expired_at":           expiredAt,
		"skipped":              skipped,
		"skipped_admins":       skipped["admin"] + skipped["current_admin"],
		"skipped_whitelist":    skipped["whitelist"],
		"skipped_pending_emby": skipped["pending_emby"],
		"skipped_unrecognized": skipped["unrecognized"],
		"emby_disabled":        embyDisabled,
	}
	if embySyncFailed > 0 {
		resp["emby_sync_failed"] = embySyncFailed
	}
	ok(w, "bulk expire complete", resp)
}

func (a *App) handleAdminBulkEnableDisabled(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	if stringValue(payload, "confirm") != "BULK_ENABLE_DISABLED_OK" {
		failWithCode(w, http.StatusBadRequest, ErrAdminBulkEnableConfirm, "请输入确认短语 BULK_ENABLE_DISABLED_OK 后再执行")
		return
	}
	includeAdmin := boolValue(payload, "include_admin", false)
	includeWhitelist := boolValue(payload, "include_whitelist", false)
	candidates := a.filteredUsers(payload)
	matched := len(candidates)
	eligible := 0
	enabledUsers := []map[string]any{}
	failedItems := []map[string]any{}
	skippedItems := []map[string]any{}
	skippedAdmins := 0
	skippedWhitelist := 0
	skippedUnrecognized := 0
	skippedActive := 0
	for _, u := range candidates {
		if u.Active {
			skippedActive++
			skippedItems = append(skippedItems, map[string]any{"uid": u.UID, "reason": "already_active"})
			continue
		}
		if u.Role == store.RoleAdmin && !includeAdmin {
			skippedAdmins++
			skippedItems = append(skippedItems, map[string]any{"uid": u.UID, "reason": "admin"})
			continue
		}
		if a.configuredAdminMatch(u.UID, u.Username) && !includeAdmin {
			skippedAdmins++
			skippedItems = append(skippedItems, map[string]any{"uid": u.UID, "reason": "configured_admin"})
			continue
		}
		if u.Role == store.RoleWhitelist && !includeWhitelist {
			skippedWhitelist++
			skippedItems = append(skippedItems, map[string]any{"uid": u.UID, "reason": "whitelist"})
			continue
		}
		if u.Role == store.RoleUnrecognized {
			skippedUnrecognized++
			skippedItems = append(skippedItems, map[string]any{"uid": u.UID, "reason": "unrecognized"})
			continue
		}
		eligible++
		updated, err := a.store().UpdateUser(u.UID, func(u *store.User) error { u.Active = true; return nil })
		if err != nil {
			failedItems = append(failedItems, map[string]any{"uid": u.UID, "username": u.Username, "reason": err.Error()})
			continue
		}
		enabledUsers = append(enabledUsers, map[string]any{"uid": updated.UID, "username": updated.Username})
	}
	ok(w, "bulk enable complete", map[string]any{
		"matched":              matched,
		"eligible":             eligible,
		"enabled":              len(enabledUsers),
		"updated":              len(enabledUsers),
		"failed":               failedItems,
		"skipped":              skippedItems,
		"skipped_admins":       skippedAdmins,
		"skipped_whitelist":    skippedWhitelist,
		"skipped_unrecognized": skippedUnrecognized,
		"skipped_active":       skippedActive,
		"enabled_users":        enabledUsers,
	})
}

func (a *App) handleAdminCleanupInvalid(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	minDays := max(1, intValue(payload, "min_days", 7))
	dryRun := boolValue(payload, "dry_run", true)
	if !dryRun && stringValue(payload, "confirm") != confirmCleanupInvalidUsers {
		failWithCode(w, http.StatusBadRequest, ErrBatchConfirmRequired, "missing confirm "+confirmCleanupInvalidUsers)
		return
	}
	threshold := time.Now().Add(-time.Duration(minDays) * 24 * time.Hour).Unix()
	targets := []store.User{}
	for _, u := range a.store().ListUsers() {
		if u.Role == store.RoleAdmin || u.Role == store.RoleWhitelist || u.TelegramID != 0 || u.EmbyID != "" {
			continue
		}
		registered := u.RegisterTime
		if registered == 0 {
			registered = u.CreatedAt
		}
		if registered > threshold {
			continue
		}
		targets = append(targets, u)
	}
	items := usersSummary(targets)
	deleted := 0
	if !dryRun {
		for _, u := range targets {
			if err := a.deleteLocalUser(context.WithoutCancel(r.Context()), u); err == nil {
				deleted++
			}
		}
	}
	ok(w, "cleanup complete", map[string]any{"users": items, "count": map[bool]int{true: len(targets), false: deleted}[dryRun], "dry_run": dryRun})
}

func (a *App) handleAdminClearStalePendingEmby(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	dryRun := boolValue(payload, "dry_run", true)
	if !dryRun && stringValue(payload, "confirm") != "CLEAR_PENDING_EMBY_OK" {
		failWithCode(w, http.StatusBadRequest, ErrAdminClearPendingEmbyConfirm, "请输入确认短语 CLEAR_PENDING_EMBY_OK 后再执行")
		return
	}
	targets := []store.User{}
	for _, u := range a.store().ListUsers() {
		if u.EmbyID == "" && u.PendingEmby {
			targets = append(targets, u)
		}
	}
	cleared := 0
	failedItems := []map[string]any{}
	if !dryRun {
		for _, u := range targets {
			if _, err := a.store().UpdateUser(u.UID, func(u *store.User) error { u.PendingEmby = false; u.PendingEmbyDays = nil; return nil }); err == nil {
				cleared++
			} else {
				failedItems = append(failedItems, map[string]any{"uid": u.UID, "username": u.Username, "error": err.Error()})
			}
		}
	}
	ok(w, "clear pending complete", map[string]any{"users": usersSummary(targets), "count": len(targets), "cleared": cleared, "failed": failedItems, "dry_run": dryRun})
}

func (a *App) handleAdminClearUserEmails(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	dryRun := boolValue(payload, "dry_run", true)
	if !dryRun && stringValue(payload, "confirm") != confirmClearAllEmails {
		failWithCode(w, http.StatusBadRequest, ErrAdminClearEmailsConfirm, adminClearEmailsConfirmMessage)
		return
	}
	users := a.store().ListUsers()
	totalUsers := len(users)
	targets := []store.User{}
	for _, u := range users {
		if strings.TrimSpace(u.Email) != "" {
			targets = append(targets, u)
		}
	}
	cleared := 0
	if !dryRun {
		total, count, err := a.store().ClearUserEmails()
		if statusFromError(w, err) {
			return
		}
		cleared = count
		totalUsers = total
	}
	ok(w, adminClearEmailsSuccessMessage, map[string]any{"users": usersSummary(targets), "count": len(targets), "cleared": cleared, "total_users": totalUsers, "dry_run": dryRun})
}

func (a *App) handleAdminKickNoEmby(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	dryRun := boolValue(payload, "dry_run", false)
	if !dryRun && stringValue(payload, "confirm") != "KICK_NO_EMBY_OK" {
		failWithCode(w, http.StatusBadRequest, ErrAdminKickNoEmbyConfirm, "请输入确认短语 KICK_NO_EMBY_OK 后再执行")
		return
	}
	minDays := clamp(intValue(payload, "min_days", 0), 0, 3650)
	threshold := int64(0)
	if minDays > 0 {
		threshold = time.Now().Add(-time.Duration(minDays) * 24 * time.Hour).Unix()
	}
	preservePending := boolValue(payload, "preserve_pending_register", true)
	targets := []store.User{}
	skipped := map[string]int{"admin": 0, "whitelist": 0, "unrecognized": 0, "pending_register": 0, "too_recent": 0}
	for _, u := range a.store().ListUsers() {
		if u.UID == current(r).User.UID || u.Role == store.RoleAdmin {
			skipped["admin"]++
			continue
		}
		if u.Role == store.RoleWhitelist {
			skipped["whitelist"]++
			continue
		}
		if u.Role == store.RoleUnrecognized {
			skipped["unrecognized"]++
			continue
		}
		if u.EmbyID != "" {
			continue
		}
		if preservePending && u.TelegramID != 0 {
			skipped["pending_register"]++
			continue
		}
		registered := u.RegisterTime
		if registered == 0 {
			registered = u.CreatedAt
		}
		if threshold > 0 && registered > threshold {
			skipped["too_recent"]++
			continue
		}
		targets = append(targets, u)
	}
	deleted := 0
	failedItems := []map[string]any{}
	if !dryRun {
		for _, u := range targets {
			if err := a.deleteLocalUser(context.WithoutCancel(r.Context()), u); err == nil {
				deleted++
			} else {
				failedItems = append(failedItems, map[string]any{"uid": u.UID, "username": u.Username, "error": err.Error()})
			}
		}
	}
	ok(w, "kick no Emby complete", map[string]any{"candidates": usersSummary(targets), "candidate_count": len(targets), "deleted_count": deleted, "failed": failedItems, "dry_run": dryRun, "skipped": skipped, "min_days": minDays, "preserve_pending_register": preservePending})
}

func (a *App) handleInviteDetach(w http.ResponseWriter, r *http.Request, params Params) {
	if _, okUser := a.userFromPath(w, params, "uid"); !okUser {
		return
	}
	uid, _ := int64Param(params, "uid")
	_, hadParent := a.store().ParentOf(uid)
	if err := a.store().DetachInvite(uid); statusFromError(w, err) {
		return
	}
	a.audit(r, "invite_detach", "admin", uid, map[string]any{"changed": hadParent})
	ok(w, "detached", map[string]any{"uid": uid, "is_root": true, "changed": hadParent})
}

func (a *App) handleListRebindRequests(w http.ResponseWriter, r *http.Request, _ Params) {
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	if status != "" && status != "all" && status != "pending" && status != "approved" && status != "rejected" && status != "revoked" {
		failWithCode(w, http.StatusBadRequest, ErrRebindStatusInvalid, "无效的状态过滤值")
		return
	}
	page := max(1, queryInt(r, "page", 1))
	perPage := clamp(queryInt(r, "per_page", 20), 1, 100)
	requests := a.store().ListRebindRequests(status)
	total := len(requests)
	requests = paginate(requests, page, perPage)
	items := make([]map[string]any, 0, len(requests))
	for _, req := range requests {
		username := req.Username
		if u, okUser := a.store().User(req.UID); okUser && username == "" {
			username = u.Username
		}
		items = append(items, map[string]any{"id": req.ID, "uid": req.UID, "username": username, "old_telegram_id": req.OldTelegramID, "status": req.Status, "reason": req.Reason, "admin_note": req.AdminNote, "reviewer_uid": zeroNil(req.ReviewerUID), "created_at": req.CreatedAt, "reviewed_at": zeroNil(req.ReviewedAt)})
	}
	ok(w, "OK", map[string]any{"requests": items, "total": total, "page": page, "per_page": perPage})
}

func (a *App) handleReviewRebindRequest(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "request_id")
	status := "rejected"
	if strings.HasSuffix(r.URL.Path, "/approve") {
		status = "approved"
	}
	req, err := a.store().ReviewRebindRequest(id, current(r).User.UID, status, truncateString(stringValue(decodeMap(r), "admin_note"), 500))
	if statusFromError(w, err) {
		return
	}
	ok(w, "reviewed", req)
}

func (a *App) handleBatchReviewRebindRequests(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	action := strings.ToLower(stringValue(payload, "action"))
	if action != "approve" && action != "reject" {
		failWithCode(w, http.StatusBadRequest, ErrRebindActionInvalid, "操作必须是 approve 或 reject")
		return
	}
	status := map[string]string{"approve": "approved", "reject": "rejected"}[action]
	ids := int64Slice(payload["ids"])
	if len(ids) == 0 || len(ids) > 100 {
		failWithCode(w, http.StatusBadRequest, ErrRebindBatchSizeInvalid, "ids 数量需在 1-100 之间")
		return
	}
	result := map[string]any{"success": 0, "failed": 0, "errors": []map[string]any{}}
	note := truncateString(stringValue(payload, "admin_note"), 500)
	for _, id := range ids {
		if _, err := a.store().ReviewRebindRequest(id, current(r).User.UID, status, note); err == nil {
			result["success"] = result["success"].(int) + 1
		} else {
			result["failed"] = result["failed"].(int) + 1
			errorsList := result["errors"].([]map[string]any)
			result["errors"] = append(errorsList, map[string]any{"id": id, "error": err.Error()})
		}
	}
	ok(w, "batch review complete", result)
}

// handleRevokeAllRebindApprovals 一键撤销所有"已批准"的换绑许可：策略收紧后用来
// 清理历史遗留的换绑权限——所有人立即变为不可换绑，但仍可重新提交申请。
func (a *App) handleRevokeAllRebindApprovals(w http.ResponseWriter, r *http.Request, _ Params) {
	note := truncateString(stringValue(decodeMap(r), "admin_note"), 500)
	if note == "" {
		note = "管理员批量撤销换绑权限"
	}
	count, err := a.store().RevokeApprovedRebindRequests(current(r).User.UID, note)
	if statusFromError(w, err) {
		return
	}
	ok(w, "revoked", map[string]any{"revoked": count})
}

func (a *App) handleTelegramRejoinedEnable(w http.ResponseWriter, r *http.Request, _ Params) {
	if stringValue(decodeMap(r), "confirm") != "ENABLE_REJOINED_OK" {
		failWithCode(w, http.StatusBadRequest, ErrAdminEnableRejoinedConfirm, "请输入确认短语 ENABLE_REJOINED_OK 后再执行")
		return
	}
	enabled := []map[string]any{}
	skipped := []map[string]any{}
	now := time.Now().Unix()
	for _, u := range a.store().ListUsers() {
		if u.Active || u.TelegramID == 0 {
			continue
		}
		if u.ExpiredAt > 0 && u.ExpiredAt < now {
			skipped = append(skipped, map[string]any{"uid": u.UID, "username": u.Username, "reason": "expired"})
			continue
		}
		if a.cfg().TelegramRequireMembership {
			missing, err := a.telegramMembershipMissing(r.Context(), u.TelegramID, true)
			if err != nil {
				skipped = append(skipped, map[string]any{"uid": u.UID, "username": u.Username, "reason": "telegram_check_failed", "error": err.Error()})
				continue
			}
			if len(missing) > 0 {
				skipped = append(skipped, map[string]any{"uid": u.UID, "username": u.Username, "reason": "not_in_required_group", "missing_groups": missing})
				continue
			}
		}
		updated, err := a.store().UpdateUser(u.UID, func(u *store.User) error { u.Active = true; return nil })
		if err == nil {
			enabled = append(enabled, map[string]any{"uid": updated.UID, "username": updated.Username, "telegram_id": updated.TelegramID})
		}
	}
	ok(w, "rejoined enable complete", map[string]any{"scanned": len(enabled) + len(skipped), "enabled": len(enabled), "enabled_users": enabled, "failed": []any{}, "skipped": skipped})
}

func (a *App) handleTelegramKickUnbound(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	dryRun := boolValue(payload, "dry_run", false)
	if !dryRun && stringValue(payload, "confirm") != "KICK_UNBOUND_OK" {
		failWithCode(w, http.StatusBadRequest, ErrAdminKickUnboundConfirm, "请输入确认短语 KICK_UNBOUND_OK 后再执行")
		return
	}
	maxPerRun := clamp(intValue(payload, "max_per_run", 200), 1, 500)
	chats := telegramChatIDs(a.cfg().TelegramGroupIDs)
	if len(chats) == 0 {
		chats = telegramChatIDs(a.cfg().TelegramChannelIDs)
	}
	if len(chats) == 0 {
		ok(w, "telegram group not configured", map[string]any{
			"available": false, "chat_id": "", "roster_size": 0, "bots_in_roster": 0, "preserved_bound": 0,
			"admins_excluded": 0, "excluded_total": 0, "targets": 0, "reason_no_account": 0, "reason_no_emby": 0,
			"reason_disabled": 0, "dry_run": dryRun, "max_per_run": maxPerRun, "kicked": 0, "skipped": 0,
			"failed": 0, "not_in_group": 0, "scanned": 0, "preview_targets": []any{},
		})
		return
	}
	chatID := chats[0]
	plan := a.telegramKickPlan(chatID)
	targets := plan.Targets
	skippedByType := plan.Skipped
	preservedBound := plan.PreservedBound
	reasonCounts := map[string]int{"no_account": 0, "no_emby": 0, "disabled": 0}
	for _, target := range targets {
		reasonCounts[target.Reason]++
	}
	preview := []map[string]any{}
	for _, target := range targets {
		if len(preview) >= 200 {
			break
		}
		preview = append(preview, target.dto())
	}
	base := map[string]any{
		"available":         a.telegramAvailable(),
		"chat_id":           chatID,
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
		"preview_targets":   preview,
		"known_only":        plan.KnownOnly,
		"skipped_no_tg":     skippedByType["no_telegram"],
		"skipped_whitelist": skippedByType["whitelist"],
		"skipped_bound":     skippedByType["bound"],
		"details":           []any{},
	}
	if dryRun || len(targets) == 0 {
		ok(w, "telegram kick preview complete", base)
		return
	}
	if a.requireTelegramConfigured(w) {
		return
	}
	adminSet := a.telegramAdminSet(r.Context(), chatID)
	base["admins_excluded"] = skippedByType["admin"] + len(adminSet)
	kicked := 0
	skipped := 0
	failedCount := 0
	notInGroup := 0
	scanned := 0
	details := []map[string]any{}
	for _, target := range targets {
		if scanned >= maxPerRun {
			break
		}
		scanned++
		if adminSet[target.TelegramID] {
			skipped++
			continue
		}
		member, err := a.telegramGetChatMember(r.Context(), chatID, target.TelegramID)
		if err != nil {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "not found") || strings.Contains(msg, "participant") {
				notInGroup++
				continue
			}
			failedCount++
			details = append(details, map[string]any{"tg_id": target.TelegramID, "uid": target.UID, "error": err.Error()})
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
		if err := a.telegramKickChatMember(r.Context(), chatID, target.TelegramID); err != nil {
			failedCount++
			details = append(details, map[string]any{"tg_id": target.TelegramID, "uid": target.UID, "error": err.Error()})
			telegramRateLimitPause(err)
			continue
		}
		kicked++
	}
	base["kicked"] = kicked
	base["skipped"] = skipped
	base["failed"] = failedCount
	base["not_in_group"] = notInGroup
	base["scanned"] = scanned
	base["details"] = details
	ok(w, "telegram kick complete", base)
}

func (a *App) updateFilteredUsers(filterValue any, include func(store.User) (bool, string), update func(*store.User)) (matched int, updated int, skipped map[string]int) {
	skipped = map[string]int{}
	uids := int64SliceFromAnyMap(filterValue, "uids")
	uidSet := map[int64]bool{}
	for _, uid := range uids {
		uidSet[uid] = true
	}
	filter, _ := filterValue.(map[string]any)
	roleFilter, hasRole := filter["role"]
	activeFilter, hasActive := filter["active"]
	embyFilter := strings.ToLower(asString(filter["emby"]))
	search := strings.ToLower(asString(filter["search"]))
	for _, u := range a.store().ListUsers() {
		if !adminUserMatchesFilter(u, uidSet, roleFilter, hasRole, activeFilter, hasActive, embyFilter, search) {
			continue
		}
		matched++
		if matched > 5000 {
			skipped["over_limit"]++
			continue
		}
		if okInclude, reason := include(u); !okInclude {
			skipped[reason]++
			continue
		}
		if _, err := a.store().UpdateUser(u.UID, func(u *store.User) error { update(u); return nil }); err == nil {
			updated++
		}
	}
	return matched, updated, skipped
}

func (a *App) filteredUsers(payload map[string]any) []store.User {
	filter, _ := payload["filter"].(map[string]any)
	uids := int64Slice(payload["uids"])
	uidSet := map[int64]bool{}
	for _, uid := range uids {
		uidSet[uid] = true
	}
	roleFilter, hasRole := filter["role"]
	activeFilter, hasActive := filter["active"]
	embyFilter := strings.ToLower(asString(filter["emby"]))
	search := strings.ToLower(asString(filter["search"]))
	out := []store.User{}
	for _, u := range a.store().ListUsers() {
		if !adminUserMatchesFilter(u, uidSet, roleFilter, hasRole, activeFilter, hasActive, embyFilter, search) {
			continue
		}
		out = append(out, u)
		if len(out) >= 5000 {
			break
		}
	}
	return out
}

func adminUserMatchesFilter(u store.User, uidSet map[int64]bool, roleFilter any, hasRole bool, activeFilter any, hasActive bool, embyFilter string, search string) bool {
	if len(uidSet) > 0 && !uidSet[u.UID] {
		return false
	}
	if hasRole && strconv.Itoa(u.Role) != asString(roleFilter) {
		return false
	}
	if hasActive && u.Active != boolish(activeFilter) {
		return false
	}
	if embyFilter == "bound" && u.EmbyID == "" {
		return false
	}
	if embyFilter == "unbound" && u.EmbyID != "" {
		return false
	}
	if search != "" && !strings.Contains(adminUserFilterHaystack(u), search) {
		return false
	}
	return true
}

func adminUserFilterHaystack(u store.User) string {
	return strings.ToLower(u.Username + " " + u.Email + " " + u.EmbyID + " " + strconv.FormatInt(u.UID, 10) + " " + strconv.FormatInt(u.TelegramID, 10))
}

type adminUserListFilter struct {
	roleFilter        any
	hasRole           bool
	activeFilter      any
	hasActive         bool
	strictQueryActive bool
	embyFilter        string
	embyStatusFilter  string
	emailFilter       string
	search            string
	now               int64
}

func adminUserMatchesListFilters(u store.User, filter adminUserListFilter) bool {
	if filter.hasRole && strconv.Itoa(u.Role) != asString(filter.roleFilter) {
		return false
	}
	if filter.hasActive {
		if filter.strictQueryActive {
			active := asString(filter.activeFilter)
			if active == "true" && !u.Active {
				return false
			}
			if active == "false" && u.Active {
				return false
			}
		} else if u.Active != boolish(filter.activeFilter) {
			return false
		}
	}
	if filter.embyFilter == "bound" && u.EmbyID == "" {
		return false
	}
	if filter.embyFilter == "unbound" && u.EmbyID != "" {
		return false
	}
	if !adminUserMatchesEmbyStatusFilter(u, filter.embyStatusFilter, filter.now) {
		return false
	}
	if !adminUserMatchesEmailStatusFilter(u, filter.emailFilter) {
		return false
	}
	if filter.search != "" && !strings.Contains(adminUserFilterHaystack(u), filter.search) {
		return false
	}
	return true
}

func adminUserMatchesEmbyStatusFilter(u store.User, filter string, now int64) bool {
	switch filter {
	case "active":
		return u.EmbyID != "" && !adminUserEmbyDisabledForFilter(u, now)
	case "disabled":
		return u.EmbyID != "" && adminUserEmbyDisabledForFilter(u, now)
	default:
		return true
	}
}

func adminUserEmbyDisabledForFilter(u store.User, now int64) bool {
	return u.EmbyDisabled || (u.Role == store.RoleNormal && u.ExpiredAt > 0 && u.ExpiredAt < now)
}

func adminUserMatchesEmailStatusFilter(u store.User, filter string) bool {
	emailBound := strings.TrimSpace(u.Email) != ""
	switch filter {
	case "verified":
		return u.EmailVerified
	case "unverified":
		return !u.EmailVerified && emailBound
	case "bound":
		return emailBound
	case "none":
		return !emailBound
	default:
		return true
	}
}

func int64SliceFromAnyMap(value any, key string) []int64 {
	if payload, ok := value.(map[string]any); ok {
		return int64Slice(payload[key])
	}
	return nil
}

func usersSummary(users []store.User) []map[string]any {
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, map[string]any{"uid": u.UID, "username": u.Username, "role": u.Role, "active": u.Active, "register_time": u.RegisterTime, "created_at": u.CreatedAt, "telegram_id": nullableInt(u.TelegramID), "emby_id": emptyNil(u.EmbyID), "pending_emby": u.PendingEmby})
	}
	return out
}

func embyPolicy(user map[string]any) map[string]any {
	if policy, ok := user["Policy"].(map[string]any); ok {
		return policy
	}
	return map[string]any{}
}

func boolish(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return false
	}
}
