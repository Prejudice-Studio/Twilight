package api

import (
	"context"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

// embyDeviceAuditActivityLimit 控制为补全历史登录 IP 而拉取的活动日志条数。
// 设备审查是管理员按需触发的低频操作，取较大窗口以尽量覆盖离线设备的来源 IP。
const embyDeviceAuditActivityLimit = 500

// parseRemoteIP 从 Emby RemoteEndPoint 中提取纯 IP。Emby 的 RemoteEndPoint 常见
// 形态是 "IP:port"（IPv6 形如 "[::1]:port"），偶尔是裸 IP。旧实现把整段原样当作
// IP 下发，结果前端 IP 列显示成 "1.2.3.4:54321"，IPv6 更会被错误截断——也就是
// 用户反馈的「读不到正确的登录设备 IP」。net.SplitHostPort 能同时正确拆分
// IPv4/IPv6+端口；没有端口时回退到去掉首尾方括号的原值。空串原样返回。
func parseRemoteIP(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(endpoint); err == nil {
		return strings.TrimSpace(host)
	}
	// 没有端口：可能是裸 IPv4 / 裸 IPv6（含冒号但无端口）/ 带方括号的 IPv6。
	return strings.Trim(endpoint, "[]")
}

// activityEntryIPs 从一条 Emby 活动日志的 ShortOverview 中提取所有合法 IP。
// 登录类事件（如 AuthenticationSucceeded）会把客户端 IP 写进 ShortOverview，
// 不同 Emby 版本可能是裸 IP、"IP:port" 或带前缀文字，这里按分隔符拆词后逐个用
// net.ParseIP 校验，只收真正的 IP，自然忽略非登录事件，无需依赖事件类型字段。
func activityEntryIPs(short string) []string {
	if strings.TrimSpace(short) == "" {
		return nil
	}
	var out []string
	for _, tok := range strings.FieldsFunc(short, func(r rune) bool {
		switch r {
		case ' ', ',', ';', '\t', '\n', '\r', '(', ')':
			return true
		default:
			return false
		}
	}) {
		ip := parseRemoteIP(tok)
		if ip != "" && net.ParseIP(ip) != nil {
			out = append(out, ip)
		}
	}
	return out
}

// embyAuditUser 是「按 Emby 登录用户聚合」的中间状态：每个 Emby 用户的设备、
// 去重后的登录 IP、在线设备数与最近活跃时间。
type embyAuditUser struct {
	embyID   string
	embyName string
	devices  []map[string]any
	ipSet    map[string]bool
	online   int
	lastSeen string // RFC3339（UTC），可直接按字符串比较取最大
}

// embyAuditLocalUser 把本地账号映射成审查视图需要的完整信息：网页账号、Emby 账号
// 与 Telegram 绑定都在这里一次性带出，方便管理员在一处核对一个人的全部身份。
func embyAuditLocalUser(u store.User) map[string]any {
	return map[string]any{
		"uid":               u.UID,
		"username":          u.Username,
		"email":             emptyNil(u.Email),
		"email_verified":    u.EmailVerified,
		"telegram_id":       nullableInt(u.TelegramID),
		"telegram_username": emptyNil(u.TelegramUsername),
		"emby_username":     emptyNil(u.EmbyUsername),
		"role":              u.Role,
		"active":            u.Active,
		"expired_at":        u.ExpiredAt,
		"register_time":     u.RegisterTime,
		"created_at":        u.CreatedAt,
		"pending_emby":      u.PendingEmby,
	}
}

// buildEmbyDeviceAudit 汇总「Emby 登录用户的设备 / IP 审查」并按用户聚合：
//   - /Devices 设备清单（含 LastUser / 最近活跃）作为设备基底；
//   - 实时 /Sessions 的 RemoteEndPoint 补当前 IP 与在线状态（解析掉端口）；
//   - /System/ActivityLog 的登录事件补历史登录 IP，使离线设备也能审查到来源 IP；
//   - 每个 Emby 用户映射回本地账号，带出完整网页 / Emby / Telegram 信息。
//
// /Devices 读取失败视为致命（无设备基底无从审查）；Sessions / ActivityLog 读取失败
// 均降级处理（分别退化为「无在线信息」「无历史 IP」），不阻断整体审查。
func (a *App) buildEmbyDeviceAudit(ctx context.Context) (map[string]any, error) {
	users := map[string]*embyAuditUser{}
	getUser := func(id string) *embyAuditUser {
		u := users[id]
		if u == nil {
			u = &embyAuditUser{embyID: id, ipSet: map[string]bool{}}
			users[id] = u
		}
		return u
	}

	// 实时会话：DeviceId -> 解析后的 IP，并顺带收集每个 Emby 用户当前 IP 与用户名。
	var sessions []map[string]any
	_ = a.embyGet(ctx, "/Sessions", &sessions)
	liveIPByDevice := make(map[string]string, len(sessions))
	for _, s := range sessions {
		ip := parseRemoteIP(asString(s["RemoteEndPoint"]))
		if did := asString(s["DeviceId"]); did != "" {
			liveIPByDevice[did] = ip
		}
		uid := asString(s["UserId"])
		if uid == "" {
			continue
		}
		u := getUser(uid)
		if name := asString(s["UserName"]); name != "" && u.embyName == "" {
			u.embyName = name
		}
		if ip != "" {
			u.ipSet[ip] = true
		}
	}

	// 设备清单（QueryResult: { Items: [...] }）。
	var devResp struct {
		Items []map[string]any `json:"Items"`
	}
	if err := a.embyGet(ctx, "/Devices", &devResp); err != nil {
		return nil, err
	}
	totalDevices := 0
	onlineDevices := 0
	for _, d := range devResp.Items {
		deviceID := asString(d["Id"])
		uid := asString(d["LastUserId"])
		last := asString(d["DateLastActivity"])
		u := getUser(uid)
		if name := asString(d["LastUserName"]); name != "" && u.embyName == "" {
			u.embyName = name
		}
		ip, isOnline := liveIPByDevice[deviceID]
		if ip != "" {
			u.ipSet[ip] = true
		}
		if last > u.lastSeen {
			u.lastSeen = last
		}
		if isOnline {
			u.online++
			onlineDevices++
		}
		u.devices = append(u.devices, map[string]any{
			"device_id":     deviceID,
			"device_name":   asString(d["Name"]),
			"app_name":      asString(d["AppName"]),
			"app_version":   asString(d["AppVersion"]),
			"last_activity": last,
			"ip":            ip,
			"online":        isOnline,
		})
		totalDevices++
	}

	// 历史登录 IP：活动日志失败降级为「无历史 IP」。
	activityAvailable := false
	var actResp struct {
		Items []map[string]any `json:"Items"`
	}
	if err := a.embyGet(ctx, "/System/ActivityLog/Entries?StartIndex=0&Limit="+strconv.Itoa(embyDeviceAuditActivityLimit), &actResp); err == nil {
		activityAvailable = true
		for _, e := range actResp.Items {
			uid := asString(e["UserId"])
			if uid == "" {
				continue
			}
			ips := activityEntryIPs(asString(e["ShortOverview"]))
			if len(ips) == 0 {
				continue
			}
			u := getUser(uid)
			for _, ip := range ips {
				u.ipSet[ip] = true
			}
			if date := asString(e["Date"]); date > u.lastSeen {
				u.lastSeen = date
			}
		}
	}

	linked := 0
	allIPs := map[string]bool{}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		ips := make([]string, 0, len(u.ipSet))
		for ip := range u.ipSet {
			ips = append(ips, ip)
			allIPs[ip] = true
		}
		sort.Strings(ips)
		// 设备按最近活跃倒序，方便审查最新登录。
		sort.SliceStable(u.devices, func(i, j int) bool {
			return asString(u.devices[i]["last_activity"]) > asString(u.devices[j]["last_activity"])
		})
		var local any
		if lu, okUser := a.store().FindUserByEmbyID(u.embyID); okUser {
			linked++
			local = embyAuditLocalUser(lu)
			if u.embyName == "" {
				u.embyName = lu.EmbyUsername
			}
		}
		out = append(out, map[string]any{
			"emby_user_id":   u.embyID,
			"emby_user_name": u.embyName,
			"device_count":   len(u.devices),
			"online_count":   u.online,
			"ip_count":       len(ips),
			"ips":            ips,
			"last_activity":  emptyNil(u.lastSeen),
			"devices":        u.devices,
			"local_user":     local,
		})
	}
	// 默认按设备数量倒序（设备多者优先审查），并列时按 IP 数量倒序；前端可再排序。
	sort.SliceStable(out, func(i, j int) bool {
		di, _ := out[i]["device_count"].(int)
		dj, _ := out[j]["device_count"].(int)
		if di != dj {
			return di > dj
		}
		ii, _ := out[i]["ip_count"].(int)
		ij, _ := out[j]["ip_count"].(int)
		return ii > ij
	})

	return map[string]any{
		"emby_configured": true,
		"users":           out,
		"summary": map[string]any{
			"total_users":        len(out),
			"linked_users":       linked,
			"total_devices":      totalDevices,
			"online_devices":     onlineDevices,
			"total_ips":          len(allIPs),
			"activity_available": activityAvailable,
		},
	}, nil
}

// handleAdminEmbyDeviceAudit 暴露按用户聚合的设备 / IP 审查数据。仅 AuthAdmin。
func (a *App) handleAdminEmbyDeviceAudit(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.embyConfigured() {
		ok(w, "OK", map[string]any{
			"emby_configured": false,
			"users":           []any{},
			"summary": map[string]any{
				"total_users": 0, "linked_users": 0, "total_devices": 0,
				"online_devices": 0, "total_ips": 0, "activity_available": false,
			},
		})
		return
	}
	data, err := a.buildEmbyDeviceAudit(r.Context())
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyRemoteSessionsFail, "读取 Emby 设备列表失败")
		return
	}
	ok(w, "OK", data)
}
