package api

import (
	"net/http"
	"strings"
	"time"
)

// 演示模式（TestWeb / Demo）handlers — 这一组接口对外只返回写死的展示数据，
// 不读登录态、不写 store。集中到独立文件以便：
//   1. 跟生产业务 handlers 视觉隔离，新人不会误以为它们参与正常用户流；
//   2. 修改文案/示例数据时风险面收窄到一处；
//   3. handlers.go 减肥。
// 所有响应都先调用 setDemoHeaders 把 X-Twilight-Demo: true / Cache-Control: no-store
// 写入响应头，前端可凭头判断是否处于 demo 模式。

func (a *App) handleDemoBootstrap(w http.ResponseWriter, r *http.Request, _ Params) {
	setDemoHeaders(w)
	role := strings.ToLower(firstNonEmpty(r.URL.Query().Get("role"), "user"))
	if role != "admin" {
		role = "user"
	}
	ok(w, "OK", map[string]any{
		"readonly": true,
		"notice":   "TestWeb 演示接口只返回固定模拟数据，不读取登录态，不执行真实写入。",
		"user":     map[string]any{"uid": 1, "username": "demo_" + role, "role": map[string]int{"admin": 0, "user": 1}[role], "role_name": role, "active": true},
		"metrics": map[string]any{
			"admin": []map[string]string{{"label": "总用户", "value": "186", "description": "+12 本月"}, {"label": "Emby 绑定", "value": "143", "description": "77%"}, {"label": "待处理求片", "value": "8", "description": "3 个下载中"}, {"label": "定时任务", "value": "11", "description": "9 个启用"}},
			"user":  []map[string]string{{"label": "账号状态", "value": "正常", "description": "Emby 已绑定"}, {"label": "剩余天数", "value": "42", "description": "到期提醒开启"}, {"label": "积分", "value": "1,280", "description": "今日已签到"}, {"label": "求片", "value": "3", "description": "1 个已完成"}},
		},
		"stats": map[string]any{"users": 186, "requests": 8, "readonly": true},
	})
}

func (a *App) handleDemoMe(w http.ResponseWriter, r *http.Request, _ Params) {
	setDemoHeaders(w)
	ok(w, "OK", map[string]any{"uid": 1, "username": "demo", "role": 0, "role_name": "Admin", "active": true})
}

func (a *App) handleDemoUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	setDemoHeaders(w)
	ok(w, "OK", map[string]any{"users": []map[string]any{{"uid": 1, "username": "demo", "role": 0, "active": true}}, "total": 1})
}

func (a *App) handleDemoRegcodes(w http.ResponseWriter, r *http.Request, _ Params) {
	setDemoHeaders(w)
	ok(w, "OK", map[string]any{"regcodes": []any{}, "total": 0})
}

func (a *App) handleDemoMediaSearch(w http.ResponseWriter, r *http.Request, _ Params) {
	setDemoHeaders(w)
	query := strings.ToLower(strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("q"), r.URL.Query().Get("query"), r.URL.Query().Get("keyword"))))
	items := []map[string]any{
		{"title": "The Bear", "type": "剧集", "year": "2022", "status": "可求片", "rating": "8.6", "source": "demo"},
		{"title": "Dune: Part Two", "type": "电影", "year": "2024", "status": "已入库", "rating": "8.4", "source": "demo"},
		{"title": "Frieren", "type": "动画", "year": "2023", "status": "处理中", "rating": "9.1", "source": "demo"},
	}
	results := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if query == "" || strings.Contains(strings.ToLower(asString(item["title"])), query) {
			results = append(results, item)
		}
	}
	ok(w, "OK", map[string]any{"results": results, "total": len(results), "readonly": true})
}

func (a *App) handleDemoAction(w http.ResponseWriter, r *http.Request, params Params) {
	setDemoHeaders(w)
	if !a.limiter().Allow(r.Context(), rateKey("demo-action:", a.clientIP(r)), 60, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrDemoActionRateLimited, "演示操作过于频繁")
		return
	}
	action := strings.TrimSpace(params["action_name"])
	if action == "" {
		action = "noop"
	}
	if !demoActionPattern.MatchString(action) || strings.ContainsAny(action, "/\\\x00\r\n\t") {
		failWithCode(w, http.StatusBadRequest, ErrDemoActionInvalid, "演示操作名称无效")
		return
	}
	ok(w, "OK", map[string]any{"demo": true, "action": action, "mutated": false, "readonly": true, "simulated": true})
}

func setDemoHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Twilight-Demo", "true")
}
