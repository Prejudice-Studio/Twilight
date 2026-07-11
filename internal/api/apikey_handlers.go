package api

// API Key 域 handler。从 handlers.go 抽出来的目的：
//   - handlers.go 长期聚合 9+ 业务域 2200+ 行，拆分阶段分批进行；
//   - API Key 同时存在 v1（legacy）和 v2 两套：legacy 与 user 行内字段绑定，
//     单个用户最多 1 把；v2 走 store.APIKey 表，多键 + 独立限速 + 独立权限。
//     两套共用 maskAPIKey / hashAPIKey / defaultPermissions / publicAPIKey
//     工具，集中到一处后单测可针对工具与端点分别覆盖；
//   - "API Key 视角"的辅助端点（info/status/renew/permissions）一并迁移，
//     避免新人接手时跨文件追读"为什么 API Key 会调 handleRenew"。
//
// 修改时务必保持与原有契约一致：
//   - failWithCode 的 ErrCode 参数集中复用 errcode.go，不在这里临时新增；
//   - publicAPIKey / publicUser 继续在本文件 / business.go 维护，输出字段
//     不能私自增删（前端 admin/api-keys 直接消费）。

import (
	"net/http"
	"strconv"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleLegacyAPIKeyStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	u := current(r).User
	ok(w, "OK", map[string]any{"enabled": u.LegacyAPIKeyStatus, "has_key": u.LegacyAPIKeyHash != ""})
}

func (a *App) handleLegacyAPIKeyGenerate(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	key := "key-" + randomCode(40)
	prefix, suffix, _ := maskAPIKey(key)
	u, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error {
		u.LegacyAPIKeyHash = hashAPIKey(key)
		u.LegacyAPIKeyPrefix = prefix
		u.LegacyAPIKeySuffix = suffix
		u.LegacyAPIKeyStatus = true
		if len(u.LegacyPermissions) == 0 {
			u.LegacyPermissions = defaultPermissions()
		}
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "generate_legacy_apikey", "user", p.User.UID, map[string]any{"permissions": u.LegacyPermissions})
	ok(w, "API key generated", map[string]any{"apikey": key, "enabled": true, "user": publicUser(u)})
}

func (a *App) handleLegacyAPIKeyDelete(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	_, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error { u.LegacyAPIKeyStatus = false; u.LegacyAPIKeyHash = ""; return nil })
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "delete_legacy_apikey", "user", p.User.UID, nil)
	ok(w, "API key deleted", nil)
}

func (a *App) handleLegacyAPIKeyEnable(w http.ResponseWriter, r *http.Request, params Params) {
	a.handleLegacyAPIKeyGenerate(w, r, params)
}

func (a *App) handleLegacyAPIKeyPermissions(w http.ResponseWriter, r *http.Request, _ Params) {
	perms := current(r).User.LegacyPermissions
	if len(perms) == 0 {
		perms = defaultPermissions()
	}
	ok(w, "OK", map[string]any{"permissions": perms, "all_permissions": defaultPermissions()})
}

func (a *App) handleLegacyAPIKeyPermissionsUpdate(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	permissions := stringSlice(payload["permissions"])
	if len(permissions) == 0 {
		permissions = defaultPermissions()
	}
	p := current(r)
	_, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error { u.LegacyPermissions = permissions; return nil })
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "update_legacy_apikey_permissions", "user", p.User.UID, map[string]any{"permissions": permissions})
	ok(w, "permissions updated", map[string]any{"permissions": permissions})
}

func (a *App) handleListAPIKeys(w http.ResponseWriter, r *http.Request, _ Params) {
	keys := a.store().ListAPIKeys(current(r).User.UID)
	items := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		items = append(items, publicAPIKey(key, ""))
	}
	ok(w, "OK", map[string]any{"keys": items, "total": len(items)})
}

func (a *App) handleCreateAPIKey(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	key := "key-" + randomCode(40)
	prefix, suffix, _ := maskAPIKey(key)
	name := stringValue(payload, "name")
	if name == "" {
		name = "Default"
	}
	k, err := a.store().CreateAPIKey(store.APIKey{UID: current(r).User.UID, Name: name, Hash: hashAPIKey(key), Prefix: prefix, Suffix: suffix, AllowQuery: boolValue(payload, "allow_query", false), RateLimit: intValue(payload, "rate_limit", a.cfg().RateLimitAPIKeyDefaultPerMinute), Permissions: defaultPermissions()})
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "create_apikey", "user", k.UID, map[string]any{
		"key_id": k.ID, "name": k.Name, "allow_query": k.AllowQuery, "rate_limit": k.RateLimit, "permissions": k.Permissions,
	})
	ok(w, "API key created", publicAPIKey(k, key))
}

func (a *App) handleUpdateAPIKey(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "key_id")
	payload := decodeMap(r)
	k, err := a.store().UpdateAPIKey(current(r).User.UID, id, func(k *store.APIKey) error {
		if name := stringValue(payload, "name"); name != "" {
			k.Name = name
		}
		if _, ok := payload["enabled"]; ok {
			k.Enabled = boolValue(payload, "enabled", k.Enabled)
		}
		if _, ok := payload["allow_query"]; ok {
			k.AllowQuery = boolValue(payload, "allow_query", k.AllowQuery)
		}
		if _, ok := payload["rate_limit"]; ok {
			k.RateLimit = intValue(payload, "rate_limit", k.RateLimit)
		}
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "update_apikey", "user", k.UID, map[string]any{
		"key_id": k.ID, "name": k.Name, "enabled": k.Enabled, "allow_query": k.AllowQuery, "rate_limit": k.RateLimit,
	})
	ok(w, "API key updated", publicAPIKey(k, ""))
}

func (a *App) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "key_id")
	if statusFromError(w, a.store().DeleteAPIKey(current(r).User.UID, id)) {
		return
	}
	a.audit(r, "delete_apikey", "user", current(r).User.UID, map[string]any{"key_id": id})
	ok(w, "API key deleted", nil)
}

func (a *App) handleAPIKeyEnableAccount(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	u, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error { u.Active = true; return nil })
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "enable_account_via_apikey", "user", u.UID, nil)
	ok(w, "account enabled", publicUser(u))
}

func (a *App) handleAPIKeyDisableAccount(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	u, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error { u.Active = false; return nil })
	if statusFromError(w, err) {
		return
	}
	// 用户主动禁用账号后，所有现存会话必须立即失效；否则除了发起本次请求的
	// session，其它设备 / cookie 仍能访问受保护接口直到 SessionTTL 到期
	_, _ = a.disableRemoteEmbyForWebState(r.Context(), u)
	a.sessions().DeleteUser(r.Context(), u.UID)
	a.audit(r, "disable_account_via_apikey", "user", u.UID, nil)
	ok(w, "account disabled", publicUser(u))
}

func (a *App) handleAPIKeyDisableKey(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if p.APIKey.ID > 0 {
		_, err := a.store().UpdateAPIKey(p.User.UID, p.APIKey.ID, func(k *store.APIKey) error { k.Enabled = false; return nil })
		if statusFromError(w, err) {
			return
		}
	} else {
		_, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error { u.LegacyAPIKeyStatus = false; return nil })
		if statusFromError(w, err) {
			return
		}
	}
	a.audit(r, "disable_current_apikey", "user", p.User.UID, map[string]any{"key_id": p.APIKey.ID})
	ok(w, "API key disabled", nil)
}

func (a *App) handleAPIKeyEnableKey(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if p.APIKey.ID > 0 {
		_, err := a.store().UpdateAPIKey(p.User.UID, p.APIKey.ID, func(k *store.APIKey) error { k.Enabled = true; return nil })
		if statusFromError(w, err) {
			return
		}
	}
	a.audit(r, "enable_current_apikey", "user", p.User.UID, map[string]any{"key_id": p.APIKey.ID})
	ok(w, "API key enabled", nil)
}

func (a *App) handleAPIKeyEmbyKick(w http.ResponseWriter, r *http.Request, params Params) {
	a.handleKickUser(w, r, Params{"uid": strconv.FormatInt(current(r).User.UID, 10)})
}

func publicAPIKey(k store.APIKey, plain string) map[string]any {
	masked := k.Prefix + "..." + k.Suffix
	data := map[string]any{"id": k.ID, "name": k.Name, "key": masked, "key_prefix": k.Prefix, "key_suffix": k.Suffix, "enabled": k.Enabled, "allow_query": k.AllowQuery, "permissions": k.Permissions, "rate_limit": k.RateLimit, "request_count": k.RequestCount, "last_used": zeroNil(k.LastUsed), "created_at": k.CreatedAt, "expired_at": zeroNil(k.ExpiredAt)}
	if plain != "" {
		data["key"] = plain
	}
	return data
}

func defaultPermissions() []string {
	return []string{"account:read", "account:write", "emby:read", "emby:write"}
}

func (a *App) handleAPIKeyInfo(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"user": publicUser(current(r).User), "permissions": current(r).APIKey.Permissions})
}

func (a *App) handleAPIKeyStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"active": current(r).User.Active, "expired_at": current(r).User.ExpiredAt})
}

func (a *App) handleAPIKeyRenew(w http.ResponseWriter, r *http.Request, params Params) {
	a.handleRenew(w, r, params)
}

func (a *App) handleAPIKeyPermissions(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"permissions": current(r).APIKey.Permissions, "all_permissions": defaultPermissions()})
}

func (a *App) handleForbiddenSelfPermission(w http.ResponseWriter, r *http.Request, _ Params) {
	failWithCode(w, http.StatusForbidden, ErrAPIKeySelfPermForbidden, "不允许通过当前 API Key 修改自身权限")
}
