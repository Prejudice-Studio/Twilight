package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

func (a *App) embyLibraries(ctx context.Context) ([]map[string]any, error) {
	var remote []map[string]any
	if err := a.embyGet(ctx, "/Library/VirtualFolders", &remote); err != nil {
		return nil, err
	}
	libraries := make([]map[string]any, 0, len(remote))
	for _, item := range remote {
		id := firstNonEmpty(asString(item["ItemId"]), asString(item["Guid"]), asString(item["Id"]), asString(item["Name"]))
		libraries = append(libraries, map[string]any{"id": id, "name": asString(item["Name"]), "type": asString(item["CollectionType"])})
	}
	return libraries, nil
}

func normalizeLibraryNames(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func libraryIDsByName(libraries []map[string]any, names []string) ([]string, []string) {
	nameSet := map[string]string{}
	for _, lib := range libraries {
		name := strings.ToLower(strings.TrimSpace(asString(lib["name"])))
		if name != "" {
			nameSet[name] = asString(lib["id"])
		}
	}
	ids := []string{}
	missing := []string{}
	for _, name := range normalizeLibraryNames(names) {
		id := nameSet[strings.ToLower(name)]
		if id == "" {
			missing = append(missing, name)
			continue
		}
		ids = append(ids, id)
	}
	return ids, missing
}

// applyDefaultHiddenLibraries 在用户首次绑定/创建 Emby 账号后执行：把
// EmbyDefaultHiddenLibraries 配置的库一次性隐藏掉，跳过管理员/白名单角色。
// 抽取自 handleBindEmby / handleRegisterEmby 两处重复。
// 失败不阻断主流程：默认隐藏只是 UX 优化，绑定/创建本身已经完成；只在 logger
// 留痕便于追踪 emby 端可能的同步异常。
func (a *App) applyDefaultHiddenLibraries(ctx context.Context, u store.User) {
	if len(a.cfg.EmbyDefaultHiddenLibraries) == 0 {
		return
	}
	if u.Role == store.RoleAdmin || u.Role == store.RoleWhitelist {
		return
	}
	if err := a.embySetLibrariesByAction(ctx, u, "hide", nil, a.cfg.EmbyDefaultHiddenLibraries, false); err != nil {
		zap.L().Warn("apply default hidden libraries failed",
			zap.Int64("uid", u.UID), zap.String("emby_id", u.EmbyID), zap.Error(err))
	}
}

func (a *App) embyLibraryAccess(ctx context.Context, user store.User, includeSelfService bool) map[string]any {
	defaultHidden := normalizeLibraryNames(a.cfg.EmbyDefaultHiddenLibraries)
	selfService := []string{}
	if includeSelfService || user.LibrarySelfService {
		selfService = normalizeLibraryNames(a.cfg.EmbySelfServiceLibraries)
	}
	if user.EmbyID == "" || a.cfg.EmbyURL == "" {
		return map[string]any{"has_emby": false, "enable_all": false, "enabled_ids": []string{}, "blocked_names": []string{}, "all_libraries": []any{}, "libraries": []any{}, "default_hidden_libraries": defaultHidden, "self_service_libraries": selfService, "self_service_enabled": user.LibrarySelfService}
	}
	libraries, err := a.embyLibraries(ctx)
	if err != nil {
		return map[string]any{"has_emby": true, "enable_all": false, "enabled_ids": []string{}, "blocked_names": []string{}, "all_libraries": []any{}, "libraries": []any{}, "default_hidden_libraries": defaultHidden, "self_service_libraries": selfService, "self_service_enabled": user.LibrarySelfService, "error": err.Error()}
	}
	embyUser, found, err := a.embyUserByID(ctx, user.EmbyID)
	if err != nil || !found {
		return map[string]any{"has_emby": true, "enable_all": false, "enabled_ids": []string{}, "blocked_names": []string{}, "all_libraries": libraries, "libraries": []any{}, "default_hidden_libraries": defaultHidden, "self_service_libraries": selfService, "self_service_enabled": user.LibrarySelfService}
	}
	policy := embyPolicy(embyUser)
	enableAll := true
	if value, ok := policy["EnableAllFolders"]; ok {
		enableAll = boolish(value)
	}
	enabledIDs := stringSlice(policy["EnabledFolders"])
	blockedNames := normalizeLibraryNames(stringSlice(policy["BlockedMediaFolders"]))
	blockedSet := map[string]bool{}
	for _, name := range blockedNames {
		blockedSet[strings.ToLower(name)] = true
	}
	enabledSet := map[string]bool{}
	for _, id := range enabledIDs {
		enabledSet[id] = true
	}
	visible := []map[string]any{}
	for _, lib := range libraries {
		if enableAll {
			if !blockedSet[strings.ToLower(asString(lib["name"]))] {
				visible = append(visible, lib)
			}
			continue
		}
		if enabledSet[asString(lib["id"])] {
			visible = append(visible, lib)
		}
	}
	if enableAll && len(enabledIDs) == 0 {
		for _, lib := range libraries {
			enabledIDs = append(enabledIDs, asString(lib["id"]))
		}
	}
	return map[string]any{"has_emby": true, "enable_all": enableAll, "enabled_ids": enabledIDs, "blocked_names": blockedNames, "all_libraries": libraries, "libraries": visible, "default_hidden_libraries": defaultHidden, "self_service_libraries": selfService, "self_service_enabled": user.LibrarySelfService}
}

func (a *App) embySetLibraryState(ctx context.Context, userID string, enabledIDs []string, blockedNames []string, enableAll bool) error {
	return a.embyUpdatePolicy(ctx, userID, func(policy map[string]any) {
		policy["EnableAllFolders"] = enableAll
		policy["EnabledFolders"] = enabledIDs
		policy["BlockedMediaFolders"] = normalizeLibraryNames(blockedNames)
	})
}

func (a *App) embySetLibrariesByAction(ctx context.Context, user store.User, action string, libraryIDs, libraryNames []string, enableAll bool) error {
	if user.EmbyID == "" {
		return fmt.Errorf("user has no Emby account")
	}
	libraries, err := a.embyLibraries(ctx)
	if err != nil {
		return err
	}
	if len(libraryIDs) == 0 && len(libraryNames) > 0 {
		ids, missing := libraryIDsByName(libraries, libraryNames)
		if len(missing) > 0 && len(ids) == 0 {
			return fmt.Errorf("libraries not found: %s", strings.Join(missing, ", "))
		}
		libraryIDs = ids
	}
	current := a.embyLibraryAccess(ctx, user, true)
	currentEnabled := stringSlice(current["enabled_ids"])
	currentBlocked := normalizeLibraryNames(stringSlice(current["blocked_names"]))
	currentEnableAll := boolish(current["enable_all"])
	switch action {
	case "enable_all":
		return a.embySetLibraryState(ctx, user.EmbyID, allLibraryIDs(libraries), []string{}, true)
	case "disable_all":
		return a.embySetLibraryState(ctx, user.EmbyID, []string{}, allLibraryNames(libraries), false)
	case "set":
		blocked := []string{}
		allowed := map[string]bool{}
		for _, id := range libraryIDs {
			allowed[id] = true
		}
		for _, lib := range libraries {
			if !allowed[asString(lib["id"])] {
				blocked = append(blocked, asString(lib["name"]))
			}
		}
		return a.embySetLibraryState(ctx, user.EmbyID, libraryIDs, blocked, enableAll)
	case "show":
		currentEnabled = appendUniqueStrings(currentEnabled, libraryIDs...)
		currentBlocked = removeLibraryNames(currentBlocked, libraryNames, libraries, libraryIDs)
		return a.embySetLibraryState(ctx, user.EmbyID, currentEnabled, currentBlocked, currentEnableAll)
	case "hide":
		currentEnabled = removeStrings(currentEnabled, libraryIDs...)
		currentBlocked = appendUniqueStrings(currentBlocked, namesForLibraryIDs(libraries, libraryIDs)...)
		return a.embySetLibraryState(ctx, user.EmbyID, currentEnabled, currentBlocked, false)
	default:
		return fmt.Errorf("unsupported library action")
	}
}

func allLibraryIDs(libraries []map[string]any) []string {
	out := []string{}
	for _, lib := range libraries {
		if id := asString(lib["id"]); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func allLibraryNames(libraries []map[string]any) []string {
	out := []string{}
	for _, lib := range libraries {
		if name := asString(lib["name"]); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func namesForLibraryIDs(libraries []map[string]any, ids []string) []string {
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	out := []string{}
	for _, lib := range libraries {
		if wanted[asString(lib["id"])] && asString(lib["name"]) != "" {
			out = append(out, asString(lib["name"]))
		}
	}
	return out
}

func appendUniqueStrings(values []string, extra ...string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range append(values, extra...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func removeStrings(values []string, remove ...string) []string {
	blocked := map[string]bool{}
	for _, value := range remove {
		blocked[value] = true
	}
	out := []string{}
	for _, value := range values {
		if !blocked[value] {
			out = append(out, value)
		}
	}
	return out
}

func removeLibraryNames(current []string, names []string, libraries []map[string]any, ids []string) []string {
	remove := map[string]bool{}
	for _, name := range names {
		remove[strings.ToLower(strings.TrimSpace(name))] = true
	}
	for _, name := range namesForLibraryIDs(libraries, ids) {
		remove[strings.ToLower(strings.TrimSpace(name))] = true
	}
	out := []string{}
	for _, name := range current {
		if !remove[strings.ToLower(strings.TrimSpace(name))] {
			out = append(out, name)
		}
	}
	return out
}
