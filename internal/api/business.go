package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

func queryInt(r *http.Request, key string, fallback int) int {
	if value := r.URL.Query().Get(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func pages(total, perPage int) int {
	if perPage <= 0 {
		return 1
	}
	if total == 0 {
		return 0
	}
	return (total + perPage - 1) / perPage
}

const permanentExpiryUnix int64 = 253402214400

func (a *App) systemUserLimitReached() (bool, int, int) {
	limit := a.cfg.UserLimit
	current := a.store.UserCount()
	return limit > 0 && current >= limit, current, limit
}

func (a *App) embyCapacityReached(excludeUID int64) (bool, int, int) {
	return a.embyCapacityReachedExcluding(excludeUID, "", "")
}

func (a *App) embyCapacityReachedExcluding(excludeUID int64, excludeRegCode, excludeInviteCode string) (bool, int, int) {
	limit := a.cfg.EmbyUserLimit
	current := 0
	now := time.Now().Unix()
	users := a.store.ListUsers()
	for _, u := range users {
		if u.UID == excludeUID {
			continue
		}
		if u.EmbyID != "" || u.PendingEmby || (a.cfg.EmbyDirectRegisterEnabled && u.Active) {
			current++
		}
	}
	for _, code := range a.store.ListAllInviteCodes() {
		if excludeInviteCode != "" && strings.EqualFold(code.Code, excludeInviteCode) {
			continue
		}
		current += remainingInviteSlots(code, now)
	}
	for _, code := range a.store.ListRegCodes() {
		if excludeRegCode != "" && strings.EqualFold(code.Code, excludeRegCode) {
			continue
		}
		current += remainingRegCodeEmbySlots(code, now)
	}
	return limit > 0 && current >= limit, current, limit
}

func remainingInviteSlots(code store.InviteCode, now int64) int {
	if !code.Active || (code.ExpiredAt > 0 && code.ExpiredAt <= now) {
		return 0
	}
	return remainingUseSlots(code.UseCount, code.UseCountLimit)
}

func remainingRegCodeEmbySlots(code store.RegCode, now int64) int {
	if !code.Active || code.IsDecoy || (code.ValidityTime > 0 && code.CreatedAt+code.ValidityTime*3600 <= now) {
		return 0
	}
	if code.Type != 1 && code.Type != 3 {
		return 0
	}
	return remainingUseSlots(code.UseCount, code.UseCountLimit)
}

func remainingUseSlots(used, limit int) int {
	if limit == -1 {
		return 1
	}
	if limit <= used {
		return 0
	}
	return limit - used
}

func (a *App) protectedUserReason(u store.User) string {
	switch {
	case u.Role == store.RoleAdmin:
		return "admin"
	case u.Role == store.RoleWhitelist:
		return "whitelist"
	case a.configuredAdminMatch(u.UID, u.Username):
		return "configured_admin"
	default:
		return ""
	}
}

func (a *App) userIsProtected(u store.User) bool {
	return a.protectedUserReason(u) != ""
}

func expiryFromDays(days int, base time.Time) int64 {
	if days < 0 {
		return permanentExpiryUnix
	}
	if days == 0 {
		days = 30
	}
	return base.AddDate(0, 0, days).Unix()
}

func addDaysToExpiry(current int64, days int, now time.Time) int64 {
	if days < 0 {
		return permanentExpiryUnix
	}
	if days == 0 {
		days = 30
	}
	base := now
	if current > now.Unix() && current < permanentExpiryUnix {
		base = time.Unix(current, 0)
	}
	return base.AddDate(0, 0, days).Unix()
}

func paginate[T any](items []T, page, perPage int) []T {
	if perPage <= 0 {
		return items
	}
	start := (max(page, 1) - 1) * perPage
	if start >= len(items) {
		return []T{}
	}
	end := start + perPage
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func truncateString(value string, limit int) string {
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

func normalizeSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "bgm", "bangumi":
		return "bangumi"
	case "all":
		return "all"
	default:
		return "tmdb"
	}
}

func normalizeMediaStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "unhandled", "pending_review":
		return "UNHANDLED"
	case "accepted", "approved":
		return "ACCEPTED"
	case "rejected", "reject":
		return "REJECTED"
	case "completed", "complete", "done":
		return "COMPLETED"
	case "downloading", "download":
		return "DOWNLOADING"
	default:
		return ""
	}
}

func adminMediaStatus(status string) string {
	switch normalizeMediaStatus(status) {
	case "UNHANDLED":
		return "pending"
	case "ACCEPTED":
		return "accepted"
	case "REJECTED":
		return "rejected"
	case "COMPLETED":
		return "completed"
	case "DOWNLOADING":
		return "downloading"
	default:
		return "pending"
	}
}

func mediaStatusMatches(status, filter string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" || filter == "all" {
		return true
	}
	if filter == "pending" {
		return normalizeMediaStatus(status) == "UNHANDLED" || normalizeMediaStatus(status) == "ACCEPTED" || normalizeMediaStatus(status) == "DOWNLOADING"
	}
	return adminMediaStatus(status) == filter
}

func canAccessMediaRequest(user store.User, req store.MediaRequest) bool {
	return user.Role == store.RoleAdmin || req.UID == user.UID || (user.TelegramID != 0 && req.TelegramID == user.TelegramID)
}

func mediaRequestUserDTO(req store.MediaRequest) map[string]any {
	mediaInfo := req.MediaInfo
	if mediaInfo == nil {
		mediaInfo = map[string]any{}
	}
	if _, ok := mediaInfo["title"]; !ok {
		mediaInfo["title"] = req.Title
	}
	if req.Season > 0 {
		mediaInfo["season"] = req.Season
	}
	if _, ok := mediaInfo["media_type"]; !ok {
		mediaInfo["media_type"] = req.MediaType
	}
	status := normalizeMediaStatus(req.Status)
	if status == "" {
		status = "UNHANDLED"
	}
	return map[string]any{
		"id":          req.ID,
		"media_id":    req.MediaID,
		"source":      req.Source,
		"status":      status,
		"status_text": mediaStatusText(status),
		"timestamp":   req.CreatedAt,
		"title":       req.Title,
		"season":      zeroNil(int64(req.Season)),
		"year":        req.Year,
		"media_type":  req.MediaType,
		"require_key": req.RequireKey,
		"admin_note":  req.AdminNote,
		"media_info":  mediaInfo,
	}
}

func mediaRequestAdminDTO(req store.MediaRequest, st *store.Store) map[string]any {
	dto := mediaRequestUserDTO(req)
	dto["status"] = adminMediaStatus(req.Status)
	userData := map[string]any{"telegram_id": req.TelegramID, "username": req.Username, "uid": req.UID}
	if u, ok := st.User(req.UID); ok {
		userData = map[string]any{"telegram_id": u.TelegramID, "username": u.Username, "uid": u.UID}
	}
	dto["user"] = userData
	return dto
}

func mediaStatusText(status string) string {
	switch normalizeMediaStatus(status) {
	case "UNHANDLED":
		return "待处理"
	case "ACCEPTED":
		return "已接受"
	case "REJECTED":
		return "已拒绝"
	case "COMPLETED":
		return "已完成"
	case "DOWNLOADING":
		return "正在下载"
	default:
		return "未知"
	}
}

func regcodeTypeName(codeType int) string {
	switch codeType {
	case 1:
		return "注册"
	case 2:
		return "续期"
	case 3:
		return "白名单"
	default:
		return "未知"
	}
}

func regcodeStatus(code store.RegCode) string {
	now := time.Now().Unix()
	if !code.Active {
		return "disabled"
	}
	if code.ValidityTime > 0 && code.CreatedAt+code.ValidityTime*3600 <= now {
		return "expired"
	}
	if code.UseCountLimit != -1 && code.UseCount >= code.UseCountLimit {
		return "used_up"
	}
	return "available"
}

func regcodeDTO(code store.RegCode) map[string]any {
	created := code.CreatedTime
	if created == 0 {
		created = code.CreatedAt
	}
	usedByUIDs := regcodeUsedByUIDs(code)
	return map[string]any{
		"code":                 code.Code,
		"type":                 code.Type,
		"type_name":            regcodeTypeName(code.Type),
		"is_decoy":             code.IsDecoy,
		"validity_time":        code.ValidityTime,
		"use_count":            code.UseCount,
		"use_count_limit":      code.UseCountLimit,
		"days":                 code.Days,
		"active":               code.Active,
		"status":               regcodeStatus(code),
		"note":                 code.Note,
		"target_username":      code.TargetUsername,
		"created_time":         created,
		"used_by":              joinInt64(usedByUIDs),
		"used_by_uids":         usedByUIDs,
		"used_by_telegram_ids": code.UsedByTelegramIDs,
	}
}

func regcodeUsedByUIDs(code store.RegCode) []int64 {
	out := make([]int64, 0, len(code.UsedByUIDs))
	for _, uid := range code.UsedByUIDs {
		if uid != 0 {
			out = appendUniqueRegcodeInt64(out, uid)
		}
	}
	return out
}

func appendUniqueRegcodeInt64(values []int64, value int64) []int64 {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func joinInt64(values []int64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatInt(value, 10))
	}
	return strings.Join(parts, ",")
}

func generateRegCode(format string, codeType int, algorithm string, days int, index int, validity int64, useLimit int) string {
	switch strings.ToLower(strings.TrimSpace(algorithm)) {
	case "base32-16", "base32-20", "base32", "", "base32-24", "base32-32",
		"hex20", "hex", "hex32", "hex40",
		"alnum-16", "alnum-24", "alnum-32",
		"urlsafe-24", "urlsafe-32",
		"digits-12", "digits-16",
		"symbols-16", "symbols-24",
		"uuid", "legacy-sha1":
	default:
		algorithm = "base32-20"
	}
	random := regCodeRandomPart(algorithm)
	typeName := map[int]string{1: "REG", 2: "REN", 3: "VIP"}[codeType]
	if strings.TrimSpace(format) == "" {
		format = "TW-{type}-{random}"
	}
	if !strings.Contains(format, "{random}") {
		format += "-{random}"
	}
	replacements := map[string]string{
		"{type}":     typeName,
		"{random}":   random,
		"{days}":     strconv.Itoa(days),
		"{index}":    strconv.Itoa(index),
		"{validity}": strconv.FormatInt(validity, 10),
		"{limit}":    strconv.Itoa(useLimit),
	}
	code := format
	for placeholder, value := range replacements {
		code = strings.ReplaceAll(code, placeholder, value)
	}
	return code
}

func regCodeRandomPart(algorithm string) string {
	switch strings.ToLower(strings.TrimSpace(algorithm)) {
	case "hex20", "hex":
		return strings.ToUpper(randomCode(20))
	case "hex32":
		return strings.ToUpper(randomCode(32))
	case "hex40":
		return strings.ToUpper(randomCode(40))
	case "base32-16":
		return base32Code(16)
	case "base32-24":
		return base32Code(24)
	case "base32-32":
		return base32Code(32)
	case "alnum-16":
		return randomFromAlphabet("ABCDEFGHJKLMNPQRSTUVWXYZ23456789", 16)
	case "alnum-24":
		return randomFromAlphabet("ABCDEFGHJKLMNPQRSTUVWXYZ23456789", 24)
	case "alnum-32":
		return randomFromAlphabet("ABCDEFGHJKLMNPQRSTUVWXYZ23456789", 32)
	case "urlsafe-24":
		return randomFromAlphabet("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_", 24)
	case "urlsafe-32":
		return randomFromAlphabet("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_", 32)
	case "digits-12":
		return randomFromAlphabet("0123456789", 12)
	case "digits-16":
		return randomFromAlphabet("0123456789", 16)
	case "symbols-16":
		return randomFromAlphabet("ABCDEFGHJKLMNPQRSTUVWXYZ23456789!@$%^*_-+=.:", 16)
	case "symbols-24":
		return randomFromAlphabet("ABCDEFGHJKLMNPQRSTUVWXYZ23456789!@$%^*_-+=.:", 24)
	case "uuid":
		return regCodeUUID()
	case "legacy-sha1":
		return strings.ToUpper(randomCode(40))
	default:
		return base32Code(20)
	}
}

func base32Code(length int) string {
	return randomFromAlphabet("ABCDEFGHJKLMNPQRSTUVWXYZ23456789", length)
}

func randomFromAlphabet(alphabet string, length int) string {
	if length <= 0 || alphabet == "" {
		return ""
	}
	var b strings.Builder
	for b.Len() < length {
		n, _ := strconv.ParseInt(randomCode(2), 16, 64)
		b.WriteByte(alphabet[int(n)%len(alphabet)])
	}
	return b.String()
}

func regCodeUUID() string {
	raw := randomCode(32)
	if len(raw) < 32 {
		return strings.ToUpper(raw)
	}
	return fmt.Sprintf("%s-%s-4%s-%s%s-%s", raw[0:8], raw[8:12], raw[13:16], string("89AB"[int(raw[16])%4]), raw[17:20], raw[20:32])
}

func legacyGenerateRegCode(format string, codeType int, algorithm string) string {
	random := strings.ToUpper(randomCode(20))
	switch strings.ToLower(algorithm) {
	case "hex20":
		random = strings.ToUpper(randomCode(20))
	case "base32-20", "base32":
		const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
		var b strings.Builder
		for len(b.String()) < 20 {
			n, _ := strconv.ParseInt(randomCode(2), 16, 64)
			b.WriteByte(alphabet[int(n)%len(alphabet)])
		}
		random = b.String()
	default:
		random = strings.ToUpper(randomCode(20))
	}
	typeName := map[int]string{1: "REG", 2: "REN", 3: "VIP"}[codeType]
	code := strings.ReplaceAll(format, "{type}", typeName)
	code = strings.ReplaceAll(code, "{random}", random)
	return strings.ToUpper(code)
}

func (a *App) previewCode(code string, user store.User) (map[string]any, string, bool) {
	if reg, ok := a.store.RegCode(code); ok {
		// Decoy code: record violation and apply configured action
		if reg.IsDecoy {
			a.recordViolation(user, code, "regcode_decoy", "使用诱饵注册码")
			return nil, "", false
		}
		// Target username restriction: record violation if mismatch
		if reg.TargetUsername != "" && !strings.EqualFold(reg.TargetUsername, user.Username) {
			a.recordViolation(user, code, "regcode_target_mismatch", "使用指名注册码（目标用户: "+reg.TargetUsername+"）")
			return nil, "", false
		}
		if regcodeStatus(reg) != "available" {
			return nil, "", false
		}
		return codePreview("regcode", reg.Type, reg.Days, ""), "regcode", true
	}
	if invite, ok := a.store.InviteCode(code); ok {
		if !invite.Active || (invite.ExpiredAt > 0 && invite.ExpiredAt <= time.Now().Unix()) || (invite.UseCountLimit != -1 && invite.UseCount >= invite.UseCountLimit) {
			return nil, "", false
		}
		if _, hasParent := a.store.ParentOf(user.UID); hasParent {
			return nil, "", false
		}
		if invite.InviterUID == user.UID {
			return nil, "", false
		}
		if invite.TargetUsername != "" && !strings.EqualFold(invite.TargetUsername, user.Username) {
			return nil, "", false
		}
		inviter := ""
		if u, ok := a.store.User(invite.InviterUID); ok {
			if !u.Active {
				return nil, "", false
			}
			if maxDays, _ := a.maxCodeDays(u); maxDays <= 0 {
				return nil, "", false
			}
			inviter = u.Username
		} else {
			return nil, "", false
		}
		return codePreview("invite", 1, invite.Days, inviter), "invite", true
	}
	return nil, "", false
}

// recordViolation logs a code violation and applies the configured punitive action.
func (a *App) recordViolation(user store.User, code, codeType, reason string) {
	if a.userIsProtected(user) {
		action := strings.ToLower(strings.TrimSpace(a.cfg.DecoyAction))
		if action == "" {
			action = "log_only"
		}
		if err := a.store.AddViolationLog(store.ViolationLog{
			UID:        user.UID,
			Username:   user.Username,
			Code:       code,
			CodeType:   codeType,
			Reason:     reason + "；受保护账号未执行处罚",
			Action:     action,
			TelegramID: user.TelegramID,
			CreatedAt:  time.Now().Unix(),
		}); err != nil {
			zap.L().Warn("failed to record protected code violation", zap.Int64("uid", user.UID), zap.String("code_type", codeType), zap.Error(err))
		}
		return
	}
	action := strings.ToLower(strings.TrimSpace(a.cfg.DecoyAction))
	if action == "" {
		action = "log_only"
	}
	if err := a.store.AddViolationLog(store.ViolationLog{
		UID:        user.UID,
		Username:   user.Username,
		Code:       code,
		CodeType:   codeType,
		Reason:     reason,
		Action:     action,
		TelegramID: user.TelegramID,
		CreatedAt:  time.Now().Unix(),
	}); err != nil {
		zap.L().Warn("failed to record code violation", zap.Int64("uid", user.UID), zap.String("code_type", codeType), zap.Error(err))
	}
	switch action {
	case "disable_user":
		if _, err := a.store.UpdateUser(user.UID, func(u *store.User) error {
			u.Active = false
			return nil
		}); err != nil {
			zap.L().Warn("failed to disable violating user", zap.Int64("uid", user.UID), zap.Error(err))
		} else {
			// 违规自动禁用：会话立即失效，避免 stale token 在 SessionTTL 到期前
			// 仍可访问。
			a.sessions.DeleteUser(context.Background(), user.UID)
		}
	case "disable_emby":
		if user.EmbyID != "" {
			_ = a.embySetUserEnabled(context.Background(), user.EmbyID, false)
		}
	}
}

func sortUsers(items []map[string]any, sortKey string) {
	switch sortKey {
	case "uid_asc":
		sort.Slice(items, func(i, j int) bool { return numeric(items[i]["uid"]) < numeric(items[j]["uid"]) })
	case "uid_desc", "":
		sort.Slice(items, func(i, j int) bool { return numeric(items[i]["uid"]) > numeric(items[j]["uid"]) })
	case "username_asc":
		sort.Slice(items, func(i, j int) bool { return fmt.Sprint(items[i]["username"]) < fmt.Sprint(items[j]["username"]) })
	case "username_desc":
		sort.Slice(items, func(i, j int) bool { return fmt.Sprint(items[i]["username"]) > fmt.Sprint(items[j]["username"]) })
	case "register_time_desc", "created_desc":
		sort.Slice(items, func(i, j int) bool { return numeric(items[i]["register_time"]) > numeric(items[j]["register_time"]) })
	case "register_time_asc", "created_asc":
		sort.Slice(items, func(i, j int) bool { return numeric(items[i]["register_time"]) < numeric(items[j]["register_time"]) })
	case "expired_at_asc", "expire_asc":
		sort.Slice(items, func(i, j int) bool { return numeric(items[i]["expired_at"]) < numeric(items[j]["expired_at"]) })
	case "expired_at_desc", "expire_desc":
		sort.Slice(items, func(i, j int) bool { return numeric(items[i]["expired_at"]) > numeric(items[j]["expired_at"]) })
	default:
		sort.Slice(items, func(i, j int) bool { return numeric(items[i]["uid"]) > numeric(items[j]["uid"]) })
	}
}

func numeric(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		parsed, _ := strconv.ParseInt(v, 10, 64)
		return parsed
	default:
		return 0
	}
}

// cascadeMaxResults 控制单次级联遍历返回的 UID 数量上限。原实现 depth==0 会
// 解释成 maxDepth=1<<30，对深邀请森林一次请求可遍历整个树并对每个目标做串行
// emby 同步删除（见 handleAdminDeleteUser），admin 自己 payload 即可拒绝服务自己。
// 5000 这个数对最严肃的真实部署仍宽裕，且能让 4xx 比 5xx 早返回。
const cascadeMaxResults = 5000

func (a *App) collectCascadeUIDs(root int64, depth int) []int64 {
	if depth == 1 {
		return []int64{root}
	}
	// depth==0 之前等价于"全树"，调用方很容易写成默认 0 触发巨大遍历。
	// 现在 0 收紧为 1（仅自身），调用方若真要全树需显式写 -1。
	maxDepth := depth
	switch {
	case depth == 0:
		return []int64{root}
	case depth < 0 || depth >= 999:
		maxDepth = 1 << 30
	}
	result := []int64{}
	queue := []struct {
		uid   int64
		level int
	}{{root, 1}}
	seen := map[int64]bool{root: true}
	for len(queue) > 0 {
		if len(result) >= cascadeMaxResults {
			break
		}
		item := queue[0]
		queue = queue[1:]
		result = append(result, item.uid)
		if item.level >= maxDepth {
			continue
		}
		for _, rel := range a.store.ChildrenOf(item.uid) {
			if !seen[rel.ChildUID] {
				seen[rel.ChildUID] = true
				queue = append(queue, struct {
					uid   int64
					level int
				}{rel.ChildUID, item.level + 1})
			}
		}
	}
	return result
}

func inviteCodeDTO(code store.InviteCode) map[string]any {
	return map[string]any{
		"code":            code.Code,
		"inviter_uid":     code.InviterUID,
		"days":            code.Days,
		"use_count_limit": code.UseCountLimit,
		"use_count":       code.UseCount,
		"expires_at":      code.ExpiredAt,
		"active":          code.Active,
		"created_at":      code.CreatedAt,
		"used_by_uid":     zeroNil(code.UsedByUID),
		"used_at":         zeroNil(code.UsedAt),
		"note":            code.Note,
		"target_username": code.TargetUsername,
	}
}

func (a *App) maxCodeDays(user store.User) (int, string) {
	permanentMaxDays := a.cfg.PermanentInviteMaxDays
	if permanentMaxDays <= 0 {
		permanentMaxDays = 365
	}
	if user.ExpiredAt < 0 || user.ExpiredAt >= 253402214400 {
		return permanentMaxDays, ""
	}
	if user.ExpiredAt <= time.Now().Unix() {
		return 0, "邀请人 Emby 有效期已到期，不能生成邀请码"
	}
	days := int((user.ExpiredAt - time.Now().Unix() + 86399) / 86400)
	if days > permanentMaxDays {
		days = permanentMaxDays
	}
	return days, ""
}

func (a *App) inviteRootUID(uid int64) int64 {
	root := uid
	seen := map[int64]bool{uid: true}
	for {
		rel, ok := a.store.ParentOf(root)
		if !ok || seen[rel.ParentUID] {
			return root
		}
		root = rel.ParentUID
		seen[root] = true
	}
}

func (a *App) inviteDescendantCount(uid int64) int {
	count := 0
	queue := []int64{uid}
	seen := map[int64]bool{uid: true}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, rel := range a.store.ChildrenOf(current) {
			if seen[rel.ChildUID] {
				continue
			}
			seen[rel.ChildUID] = true
			count++
			queue = append(queue, rel.ChildUID)
		}
	}
	return count
}

func (a *App) inviteDepth(uid int64) int {
	depth := 1
	seen := map[int64]bool{uid: true}
	for {
		rel, ok := a.store.ParentOf(uid)
		if !ok || seen[rel.ParentUID] {
			return depth
		}
		uid = rel.ParentUID
		seen[uid] = true
		depth++
		if depth > 64 {
			return depth
		}
	}
}

func (a *App) inviteTreeNode(uid int64, depth int, seen map[int64]bool) (map[string]any, int) {
	u, ok := a.store.User(uid)
	if !ok {
		return nil, 0
	}
	node := map[string]any{
		"uid":           u.UID,
		"username":      u.Username,
		"active":        u.Active,
		"has_emby":      u.EmbyID != "",
		"expired_at":    u.ExpiredAt,
		"expire_status": expireStatus(u.ExpiredAt),
		"emby_expired":  u.ExpiredAt > 0 && u.ExpiredAt < time.Now().Unix(),
		"depth":         depth,
		"children":      []map[string]any{},
	}
	if seen[uid] {
		return node, 0
	}
	seen[uid] = true

	children := []map[string]any{}
	count := 0
	for _, rel := range a.store.ChildrenOf(uid) {
		child, childCount := a.inviteTreeNode(rel.ChildUID, depth+1, seen)
		if child == nil {
			continue
		}
		children = append(children, child)
		count += 1 + childCount
	}
	node["children"] = children
	return node, count
}

func (a *App) inviteTreeFor(user store.User) map[string]any {
	self, count := a.inviteTreeNode(user.UID, a.inviteDepth(user.UID), map[int64]bool{})
	if self == nil {
		self = map[string]any{
			"uid":           user.UID,
			"username":      user.Username,
			"active":        user.Active,
			"has_emby":      user.EmbyID != "",
			"expired_at":    user.ExpiredAt,
			"expire_status": expireStatus(user.ExpiredAt),
			"emby_expired":  user.ExpiredAt > 0 && user.ExpiredAt < time.Now().Unix(),
			"depth":         a.inviteDepth(user.UID),
			"children":      []map[string]any{},
		}
	}
	descendants, _ := self["children"].([]map[string]any)
	if descendants == nil {
		descendants = []map[string]any{}
		self["children"] = descendants
	}
	return map[string]any{
		"self":             self,
		"descendants":      descendants,
		"descendant_count": count,
	}
}

func (a *App) canInvite(user store.User) (bool, string) {
	if !a.cfg.InviteEnabled {
		return false, "邀请系统未启用"
	}
	if !user.Active {
		return false, "账号已被禁用，无法生成邀请码"
	}
	if a.cfg.InviteRequireEmby && user.EmbyID == "" {
		return false, "请先绑定 Emby 账号后再生成邀请码"
	}
	if maxDays, reason := a.maxCodeDays(user); maxDays <= 0 {
		return false, reason
	}
	if a.inviteDepth(user.UID) >= a.cfg.InviteMaxDepth {
		return false, fmt.Sprintf("已达到最大邀请层级 (%d)，不能再向下邀请", a.cfg.InviteMaxDepth)
	}
	if a.cfg.InviteRootUserLimit > 0 {
		rootUID := a.inviteRootUID(user.UID)
		if a.inviteDescendantCount(rootUID) >= a.cfg.InviteRootUserLimit {
			return false, fmt.Sprintf("邀请树人数已达上限 (%d)，不能继续邀请", a.cfg.InviteRootUserLimit)
		}
	}
	if a.cfg.InviteLimit != -1 {
		active := 0
		for _, code := range a.store.ListInviteCodes(user.UID) {
			if code.Active && code.UseCount == 0 && (code.ExpiredAt <= 0 || code.ExpiredAt > time.Now().Unix()) {
				active++
			}
		}
		if active >= a.cfg.InviteLimit {
			return false, fmt.Sprintf("未使用的邀请码已达上限 (%d)，请先撤销旧的", a.cfg.InviteLimit)
		}
	}
	return true, ""
}

func (a *App) inviteForest() map[string]any {
	rels := a.store.InviteRelations()
	users := a.store.ListUsers()
	userByID := map[int64]store.User{}
	allUIDs := map[int64]bool{}
	parentOf := map[int64]int64{}
	children := map[int64][]int64{}
	for _, u := range users {
		userByID[u.UID] = u
	}
	for _, rel := range rels {
		allUIDs[rel.ParentUID] = true
		allUIDs[rel.ChildUID] = true
		parentOf[rel.ChildUID] = rel.ParentUID
		children[rel.ParentUID] = append(children[rel.ParentUID], rel.ChildUID)
	}
	for _, code := range a.store.ListAllInviteCodes() {
		allUIDs[code.InviterUID] = true
	}
	nodes := []map[string]any{}
	for uid := range allUIDs {
		if u, ok := userByID[uid]; ok {
			nodes = append(nodes, map[string]any{"uid": u.UID, "username": u.Username, "role": u.Role, "emby_id": emptyNil(u.EmbyID), "active": u.Active, "telegram_id": nullableInt(u.TelegramID), "register_time": u.RegisterTime, "expired_at": u.ExpiredAt, "is_root": parentOf[uid] == 0})
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return numeric(nodes[i]["uid"]) < numeric(nodes[j]["uid"]) })
	edges := []map[string]any{}
	for _, rel := range rels {
		edges = append(edges, map[string]any{"parent": rel.ParentUID, "child": rel.ChildUID, "code": rel.Code, "created_at": rel.CreatedAt})
	}
	roots := []int64{}
	for _, node := range nodes {
		uid := numeric(node["uid"])
		if parentOf[uid] == 0 {
			roots = append(roots, uid)
		}
	}
	globalDepth := 0
	for _, root := range roots {
		globalDepth = max(globalDepth, subtreeDepth(root, children))
	}
	return map[string]any{"nodes": nodes, "edges": edges, "roots": roots, "max_depth": globalDepth, "config": map[string]any{"enabled": a.cfg.InviteEnabled, "max_depth": a.cfg.InviteMaxDepth, "invite_limit": a.cfg.InviteLimit, "invite_root_user_limit": a.cfg.InviteRootUserLimit, "require_emby": a.cfg.InviteRequireEmby}}
}

func subtreeDepth(root int64, children map[int64][]int64) int {
	maxDepth := 1
	queue := []struct {
		uid   int64
		depth int
	}{{root, 1}}
	seen := map[int64]bool{root: true}
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if item.depth > maxDepth {
			maxDepth = item.depth
		}
		for _, child := range children[item.uid] {
			if !seen[child] {
				seen[child] = true
				queue = append(queue, struct {
					uid   int64
					depth int
				}{child, item.depth + 1})
			}
		}
	}
	return maxDepth
}

func emptyNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func int64Slice(value any) []int64 {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]int64, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case float64:
			out = append(out, int64(v))
		case int64:
			out = append(out, v)
		case string:
			if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
				out = append(out, parsed)
			}
		}
	}
	return out
}

func batchResult(total int) map[string]any {
	return map[string]any{"total": total, "success": 0, "failed": 0, "errors": []map[string]any{}}
}

func addBatchOutcome(result map[string]any, uid int64, err error) {
	if err == nil {
		result["success"] = result["success"].(int) + 1
		return
	}
	result["failed"] = result["failed"].(int) + 1
	errorsList := result["errors"].([]map[string]any)
	errorsList = append(errorsList, map[string]any{"uid": uid, "error": err.Error()})
	result["errors"] = errorsList
}

func formatSeconds(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	if days > 0 {
		return fmt.Sprintf("%d天%d小时", days, hours)
	}
	return fmt.Sprintf("%d小时", hours)
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func schedulerJobExists(jobID string) bool {
	for _, job := range schedulerJobs {
		if fmt.Sprint(job["id"]) == jobID {
			return true
		}
	}
	return false
}

func defaultTriggerSpec(jobID string) map[string]any {
	switch jobID {
	case "cleanup_sessions":
		return map[string]any{"type": "interval", "seconds": 3600}
	case "emby_sync", "kick_unknown_group_members":
		return map[string]any{"type": "manual"}
	default:
		return map[string]any{"type": "cron_daily", "hour": 3, "minute": 0}
	}
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}
