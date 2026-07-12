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
	userOrderKey := func(id, name string) string {
		id = strings.TrimSpace(id)
		if id != "" {
			return "id:" + id
		}
		name = strings.TrimSpace(name)
		if name != "" {
			return "name:" + strings.ToLower(name)
		}
		return "unknown"
	}
	getUser := func(id, name string) *embyAuditUser {
		key := userOrderKey(id, name)
		u := users[key]
		if u == nil {
			u = &embyAuditUser{embyID: strings.TrimSpace(id), embyName: strings.TrimSpace(name), ipSet: map[string]bool{}, deviceAgg: map[string]map[string]any{}}
			users[key] = u
		}
		if u.embyID == "" && strings.TrimSpace(id) != "" {
			u.embyID = strings.TrimSpace(id)
		}
		if u.embyName == "" && strings.TrimSpace(name) != "" {
			u.embyName = strings.TrimSpace(name)
		}
		return u
	}
	for _, local := range a.store().ListUsers() {
		if local.EmbyID != "" {
			getUser(local.EmbyID, local.EmbyUsername)
		}
	}

	type liveSession struct {
		deviceID     string
		deviceName   string
		client       string
		appVersion   string
		userID       string
		userName     string
		ip           string
		lastActivity string
	}
	sessions, _ := a.embySessionsSnapshot(ctx, false)
	liveByDevice := map[string]liveSession{}
	for _, s := range sessions {
		ls := liveSession{
			deviceID:     firstNonEmpty(asString(s["DeviceId"]), asString(s["DeviceID"]), asString(s["Id"])),
			deviceName:   firstNonEmpty(asString(s["DeviceName"]), asString(s["Device"])),
			client:       firstNonEmpty(asString(s["Client"]), asString(s["AppName"])),
			appVersion:   firstNonEmpty(asString(s["ApplicationVersion"]), asString(s["AppVersion"])),
			userID:       asString(s["UserId"]),
			userName:     asString(s["UserName"]),
			ip:           parseRemoteIP(asString(s["RemoteEndPoint"])),
			lastActivity: firstNonEmpty(asString(s["LastActivityDate"]), asString(s["DateLastActivity"])),
		}
		if ls.deviceID != "" {
			liveByDevice[ls.deviceID] = ls
		}
		u := getUser(ls.userID, ls.userName)
		if ls.ip != "" {
			u.ipSet[ls.ip] = true
		}
		if ls.lastActivity > u.lastSeen {
			u.lastSeen = ls.lastActivity
		}
	}

	var devResp struct {
		Items []map[string]any `json:"Items"`
	}
	if err := a.embyGet(ctx, "/Devices", &devResp); err != nil {
		return nil, err
	}

	type clientStat struct {
		devices int
		online  int
		users   map[string]bool
	}
	clientStats := map[string]*clientStat{}
	seenDevices := map[string]bool{}
	totalDevices := 0
	onlineDevices := 0
	addClient := func(app string, online bool, userKey string) {
		app = firstNonEmpty(strings.TrimSpace(app), "Unknown")
		cs := clientStats[app]
		if cs == nil {
			cs = &clientStat{users: map[string]bool{}}
			clientStats[app] = cs
		}
		cs.devices++
		if online {
			cs.online++
		}
		if userKey != "" {
			cs.users[userKey] = true
		}
	}
	addDevice := func(device map[string]any, live liveSession, online bool) {
		deviceID := firstNonEmpty(asString(device["Id"]), live.deviceID)
		uid := firstNonEmpty(asString(device["LastUserId"]), live.userID)
		uname := firstNonEmpty(asString(device["LastUserName"]), live.userName)
		u := getUser(uid, uname)
		last := firstNonEmpty(asString(device["DateLastActivity"]), live.lastActivity)
		app := firstNonEmpty(asString(device["AppName"]), live.client, "Unknown")
		name := firstNonEmpty(asString(device["Name"]), live.deviceName, deviceID, "Unknown")
		if live.ip != "" {
			u.ipSet[live.ip] = true
		}
		if last > u.lastSeen {
			u.lastSeen = last
		}
		if online {
			u.online++
			onlineDevices++
		}
		record := map[string]any{
			"device_id":     deviceID,
			"device_name":   name,
			"app_name":      app,
			"app_version":   firstNonEmpty(asString(device["AppVersion"]), live.appVersion),
			"last_activity": last,
			"ip":            live.ip,
			"ip_approx":     false,
			"online":        online,
			"count":         1,
		}
		u.devices = append(u.devices, record)
		addClient(app, online, firstNonEmpty(uid, uname))
		totalDevices++
		if deviceID != "" {
			seenDevices[deviceID] = true
		}
	}
	for _, d := range devResp.Items {
		deviceID := asString(d["Id"])
		live, online := liveByDevice[deviceID]
		addDevice(d, live, online)
	}
	for deviceID, live := range liveByDevice {
		if seenDevices[deviceID] {
			continue
		}
		addDevice(map[string]any{"Id": deviceID, "Name": live.deviceName, "AppName": live.client, "AppVersion": live.appVersion, "LastUserId": live.userID, "LastUserName": live.userName, "DateLastActivity": live.lastActivity}, live, true)
	}

	activityAvailable := false
	var actResp struct {
		Items []map[string]any `json:"Items"`
	}
	if err := a.embyGet(ctx, "/System/ActivityLog/Entries?StartIndex=0&Limit="+strconv.Itoa(embyDeviceAuditActivityLimit), &actResp); err == nil {
		activityAvailable = true
		for _, e := range actResp.Items {
			uid := asString(e["UserId"])
			uname := asString(e["UserName"])
			ips := activityEntryIPs(asString(e["ShortOverview"]))
			if len(ips) == 0 {
				continue
			}
			u := getUser(uid, uname)
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
		if u.devices == nil {
			u.devices = []map[string]any{}
		}
		fillDeviceIPsFromHistory(u)
		ips := make([]string, 0, len(u.ipSet))
		for ip := range u.ipSet {
			ips = append(ips, ip)
			allIPs[ip] = true
		}
		sort.Strings(ips)
		sort.SliceStable(u.devices, func(i, j int) bool {
			return asString(u.devices[i]["last_activity"]) > asString(u.devices[j]["last_activity"])
		})
		var local any
		if u.embyID != "" {
			if lu, okUser := a.store().FindUserByEmbyID(u.embyID); okUser {
				linked++
				local = embyAuditLocalUser(lu)
				if u.embyName == "" {
					u.embyName = lu.EmbyUsername
				}
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
	sort.SliceStable(out, func(i, j int) bool {
		di, _ := out[i]["device_count"].(int)
		dj, _ := out[j]["device_count"].(int)
		if di != dj {
			return di > dj
		}
		ii, _ := out[i]["ip_count"].(int)
		ij, _ := out[j]["ip_count"].(int)
		if ii != ij {
			return ii > ij
		}
		return asString(out[i]["emby_user_name"]) < asString(out[j]["emby_user_name"])
	})

	clients := make([]map[string]any, 0, len(clientStats))
	for name, cs := range clientStats {
		clients = append(clients, map[string]any{"name": name, "devices": cs.devices, "online": cs.online, "users": len(cs.users)})
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
