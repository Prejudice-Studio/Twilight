package api

import (
	"context"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

// embyDeviceAuditActivityLimit 控制为补全历史登录 IP 而拉取的活动日志条数。
// 设备审查是管理员按需触发的低频操作，取较大窗口以尽量覆盖离线设备的来源 IP。
const embyDeviceAuditActivityLimit = 500

// embyDeviceAuditCacheTTL protects Emby from repeated admin refreshes while
// keeping quick moderation actions fresh via ?refresh=1.
const embyDeviceAuditCacheTTL = 15 * time.Second

// embyDeviceIPCorrelationWindow 限定「用历史登录事件回填离线设备 IP」时，设备最近
// 活跃时间与登录事件时间的最大允许间隔。Emby 的 /Devices 不返回 IP，离线设备也拿不到
// 实时会话 IP；当某用户有多个历史登录 IP 时，只把时间上最接近设备最近活跃、且落在窗口
// 内的那次登录 IP 作为「推断值」回填，超出窗口就不猜，避免把别处的 IP 张冠李戴。
const embyDeviceIPCorrelationWindow = 12 * time.Hour

// parseEmbyTime 解析 Emby 返回的时间戳（ISO8601，常见为带小数秒的 UTC，也可能带时区
// 偏移）。解析失败返回零值 + false，调用方据此跳过该条，不影响其余审查数据。
func parseEmbyTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if ts, err := time.Parse(layout, s); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}

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

// embyAuthEvent 是从活动日志解析出的一次登录事件：发生时间 + 来源 IP，用于把
// 历史登录 IP 时间相关地回填到离线设备。
type embyAuthEvent struct {
	at time.Time
	ip string
}

// embyAuditUser 是「按 Emby 登录用户聚合」的中间状态：每个 Emby 用户的设备、
// 去重后的登录 IP、历史登录事件、在线设备数与最近活跃时间。
type embyAuditUser struct {
	embyID     string
	embyName   string
	devices    []map[string]any
	deviceAgg  map[string]map[string]any // device_name|app_name -> device，用于聚合
	ipSet      map[string]bool
	authEvents []embyAuthEvent
	online     int
	lastSeen   string // RFC3339（UTC），可直接按字符串比较取最大
}

// fillDeviceIPsFromHistory 用历史登录 IP 回填没有实时会话 IP 的（通常是离线）设备。
// Emby /Devices 不带 IP，离线设备拿不到实时会话 IP，但管理员审查时仍想知道这台设备
// 大致来自哪个 IP。两种回填都标记 ip_approx=true（推断值，非实时会话）：
//   - 该用户全程只出现过一个 IP：所有设备必然来自它，直接回填；
//   - 出现多个 IP：取时间上最接近设备最近活跃、且落在允许窗口内的那次登录 IP。
func fillDeviceIPsFromHistory(u *embyAuditUser) {
	if len(u.ipSet) == 0 {
		return
	}
	soleIP := ""
	if len(u.ipSet) == 1 {
		for ip := range u.ipSet {
			soleIP = ip
		}
	}
	for _, dev := range u.devices {
		if asString(dev["ip"]) != "" {
			continue
		}
		// 在线设备的 IP 只认实时会话；没拿到就留空，不用历史值倒推（避免自相矛盾）。
		if online, _ := dev["online"].(bool); online {
			continue
		}
		if soleIP != "" {
			dev["ip"] = soleIP
			dev["ip_approx"] = true
			continue
		}
		if len(u.authEvents) == 0 {
			continue
		}
		devAt, ok := parseEmbyTime(asString(dev["last_activity"]))
		if !ok {
			continue
		}
		best := ""
		var bestDiff time.Duration
		for _, ev := range u.authEvents {
			diff := devAt.Sub(ev.at)
			if diff < 0 {
				diff = -diff
			}
			if diff > embyDeviceIPCorrelationWindow {
				continue
			}
			if best == "" || diff < bestDiff {
				best = ev.ip
				bestDiff = diff
			}
		}
		if best != "" {
			dev["ip"] = best
			dev["ip_approx"] = true
		}
	}
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
			u = &embyAuditUser{embyID: id, ipSet: map[string]bool{}, deviceAgg: map[string]map[string]any{}}
			users[id] = u
		}
		return u
	}

	// 实时会话：DeviceId -> 解析后的 IP，并顺带收集每个 Emby 用户当前 IP 与用户名。
	sessions, _ := a.embySessionsSnapshot(ctx, false)
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
	// 客户端类型聚合：按 AppName 统计设备数 / 在线数 / 去重用户数，给前端做归类与筛选。
	type clientStat struct {
		devices int
		online  int
		users   map[string]bool
	}
	clientStats := map[string]*clientStat{}

	totalDevices := 0
	onlineDevices := 0
	for _, d := range devResp.Items {
		deviceID := asString(d["Id"])
		uid := asString(d["LastUserId"])
		last := asString(d["DateLastActivity"])
		app := asString(d["AppName"])
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
		// 按 device_name + app_name 聚合相同设备，前端以 count 字段展示批量。
		aggKey := asString(d["Name"]) + "|" + app
		if existing, ok := u.deviceAgg[aggKey]; ok {
			existing["count"] = int(numeric(existing["count"])) + 1
			existing["device_id"] = deviceID
			if last > asString(existing["last_activity"]) {
				existing["last_activity"] = last
				existing["ip"] = ip
			}
			if isOnline {
				existing["online"] = true
			}
		} else {
			u.deviceAgg[aggKey] = map[string]any{
				"device_id":     deviceID,
				"device_name":   asString(d["Name"]),
				"app_name":      app,
				"app_version":   asString(d["AppVersion"]),
				"last_activity": last,
				"ip":            ip,
				"ip_approx":     false,
				"online":        isOnline,
				"count":         1,
			}
			u.devices = append(u.devices, u.deviceAgg[aggKey])
		}
		cs := clientStats[app]
		if cs == nil {
			cs = &clientStat{users: map[string]bool{}}
			clientStats[app] = cs
		}
		cs.devices++
		if isOnline {
			cs.online++
		}
		if uid != "" {
			cs.users[uid] = true
		}
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
			date := asString(e["Date"])
			eventAt, hasAt := parseEmbyTime(date)
			for _, ip := range ips {
				u.ipSet[ip] = true
				if hasAt {
					u.authEvents = append(u.authEvents, embyAuthEvent{at: eventAt, ip: ip})
				}
			}
			if date > u.lastSeen {
				u.lastSeen = date
			}
		}
	}

	linked := 0
	allIPs := map[string]bool{}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		// 确保 devices 非 nil——用户可能仅出现在 Sessions/ActivityLog 而不在
		// /Devices 列表中（如已删除设备），nil slice 序列化为 JSON null 会导致
		// 前端对 .filter()/.some() 调用崩溃。
		if u.devices == nil {
			u.devices = []map[string]any{}
		}
		// 先用历史登录 IP 回填离线设备，再展开 IP 列表与设备排序。
		fillDeviceIPsFromHistory(u)
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

	// 客户端归类：按设备数量倒序，并列按名称升序，便于前端做分布展示与下拉筛选。
	clients := make([]map[string]any, 0, len(clientStats))
	for name, cs := range clientStats {
		clients = append(clients, map[string]any{
			"name":    name,
			"devices": cs.devices,
			"online":  cs.online,
			"users":   len(cs.users),
		})
	}
	sort.SliceStable(clients, func(i, j int) bool {
		di, _ := clients[i]["devices"].(int)
		dj, _ := clients[j]["devices"].(int)
		if di != dj {
			return di > dj
		}
		return asString(clients[i]["name"]) < asString(clients[j]["name"])
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
			"clients":            clients,
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
				"clients": []any{},
			},
		})
		return
	}
	refresh := r.URL.Query().Get("refresh") == "1" || strings.EqualFold(r.URL.Query().Get("refresh"), "true")
	if !refresh {
		now := time.Now()
		a.embyDeviceAuditMu.Lock()
		if a.embyDeviceAuditCache != nil && now.Before(a.embyDeviceAuditUntil) {
			data := a.embyDeviceAuditCache
			a.embyDeviceAuditMu.Unlock()
			ok(w, "OK", data)
			return
		}
		a.embyDeviceAuditMu.Unlock()
	} else {
		a.invalidateEmbySessionsSnapshot()
	}
	data, err := a.buildEmbyDeviceAudit(r.Context())
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyRemoteSessionsFail, "读取 Emby 设备列表失败")
		return
	}
	a.embyDeviceAuditMu.Lock()
	a.embyDeviceAuditCache = data
	a.embyDeviceAuditUntil = time.Now().Add(embyDeviceAuditCacheTTL)
	a.embyDeviceAuditMu.Unlock()
	ok(w, "OK", data)
}
