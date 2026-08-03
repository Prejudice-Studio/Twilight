package api

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/security"
	"github.com/prejudice-studio/twilight/internal/store"
	"github.com/prejudice-studio/twilight/internal/validate"
)

var telegramPublicUsernamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}$`)

// generatedPasswordHexLen 是自动生成密码主体段（"Twilight-" 之后）的 hex 长度。
// 32 hex char = 128 bit 熵；攻击者即使拿到响应模板也无法在合理时间内枚举。
// 历史值为 12 (=48 bit)，对在线 + 离线攻击均偏弱。
const generatedPasswordHexLen = 32

func (a *App) handleRoot(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "Twilight API", map[string]any{"name": a.cfg().AppName, "version": a.cfg().Version, "docs": "/api/v1/docs"})
}

func (a *App) handleOpenAPI(w http.ResponseWriter, r *http.Request, _ Params) {
	// openapi.json 是 AuthPublic（无需登录即可读）。只输出 AuthPublic 路由，
	// 不暴露 admin / 鉴权端点的 pattern——未鉴权枚举 admin/* 等于给攻击者一份
	// 完整攻击面地图。admin 端点本身仍受 AuthAdmin 保护，这里仅去除信息披露。
	// 需要完整路由清单的运维场景走 AuthAdmin 的 /system/admin/apis。
	paths := map[string]map[string]any{}
	for _, route := range a.routes {
		if route.Auth != AuthPublic {
			continue
		}
		pathItem := paths[route.Pattern]
		if pathItem == nil {
			pathItem = map[string]any{}
			paths[route.Pattern] = pathItem
		}
		pathItem[strings.ToLower(route.Method)] = map[string]any{"responses": map[string]any{"200": map[string]string{"description": "OK"}}}
	}
	ok(w, "OK", map[string]any{"openapi": "3.0.3", "info": map[string]string{"title": "Twilight Go API", "version": a.cfg().Version}, "paths": paths})
}

func (a *App) handleDocs(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(apiConsoleHTML))
}

const apiConsoleHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex">
<title>Twilight API 控制台</title>
<style>
:root{--bg:#0b0f14;--panel:#111821;--card:#17202b;--soft:#1f2b38;--border:#2b3948;--text:#eef4fa;--muted:#a8b3bf;--primary:#38bdf8;--green:#34d399;--amber:#fbbf24;--red:#fb7185;--violet:#a78bfa}
*{box-sizing:border-box}html,body{margin:0;min-height:100%;background:var(--bg);color:var(--text)}body{font:14px/1.5 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}button,input,select,textarea{font:inherit}button{cursor:pointer}.topbar{position:sticky;top:0;z-index:10;display:grid;grid-template-columns:minmax(220px,1fr) minmax(160px,240px) minmax(180px,260px) auto;gap:10px;align-items:center;border-bottom:1px solid var(--border);background:rgba(17,24,33,.96);padding:14px 18px;backdrop-filter:blur(10px)}.brand strong{display:block;font-size:17px}.brand span{display:block;color:var(--muted);font-size:12px}.control,input,select,textarea{min-width:0;width:100%;border:1px solid var(--border);border-radius:8px;background:#0d131a;color:var(--text);padding:9px 10px}.btn{border:1px solid var(--border);border-radius:8px;background:var(--primary);color:#001018;font-weight:700;padding:9px 13px;white-space:nowrap}.btn.secondary{background:transparent;color:var(--text)}.layout{display:grid;grid-template-columns:minmax(280px,370px) minmax(0,1fr);height:calc(100vh - 69px)}.sidebar{min-width:0;overflow:auto;border-right:1px solid var(--border);background:var(--panel)}.filters{display:grid;grid-template-columns:1fr 1fr;gap:8px;padding:12px;border-bottom:1px solid var(--border)}.filters input{grid-column:1/-1}.summary{display:flex;justify-content:space-between;gap:8px;padding:9px 12px;border-bottom:1px solid var(--border);color:var(--muted);font-size:12px}.routes{padding-bottom:18px}.group{padding:13px 14px 5px;color:var(--muted);font-size:11px;font-weight:800;letter-spacing:.04em;text-transform:uppercase}.route{display:grid;grid-template-columns:62px minmax(0,1fr);gap:8px;align-items:center;width:100%;border:0;border-left:3px solid transparent;background:transparent;color:var(--muted);padding:8px 12px;text-align:left}.route:hover,.route.active{background:rgba(56,189,248,.1);color:var(--text)}.route.active{border-left-color:var(--primary)}.path{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.method{display:inline-flex;align-items:center;justify-content:center;width:56px;border-radius:5px;padding:2px 0;font-size:11px;font-weight:900}.get{background:rgba(52,211,153,.14);color:var(--green)}.post{background:rgba(56,189,248,.14);color:var(--primary)}.put{background:rgba(251,191,36,.14);color:var(--amber)}.patch{background:rgba(167,139,250,.16);color:var(--violet)}.delete{background:rgba(251,113,133,.15);color:var(--red)}.main{min-width:0;overflow:auto;padding:20px}.hero,.panel,.notice{border:1px solid var(--border);border-radius:10px;background:var(--card)}.hero{padding:20px;margin-bottom:16px}.hero h1{margin:0 0 8px;font-size:22px}.hero p{max-width:820px;margin:0;color:var(--muted)}.notice{margin-top:12px;padding:10px 12px;border-color:rgba(251,191,36,.4);background:rgba(251,191,36,.08);color:#fde68a}.panel{padding:18px}.panel h2{display:flex;align-items:center;gap:8px;margin:0 0 8px;font-size:20px}.endpoint{margin-bottom:14px;color:var(--muted);font-family:ui-monospace,SFMono-Regular,Consolas,monospace;overflow-wrap:anywhere}.meta{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:14px}.pill{border:1px solid var(--border);border-radius:999px;padding:3px 9px;color:var(--muted);font-size:12px}.grid{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:12px}.field label{display:block;margin-bottom:5px;color:var(--muted);font-size:12px}.field textarea{min-height:132px;resize:vertical;font-family:ui-monospace,SFMono-Regular,Consolas,monospace}.actions{display:flex;flex-wrap:wrap;gap:8px}.response{margin-top:14px;border:1px solid var(--border);border-radius:10px;background:#0d131a;padding:14px}.response pre{max-height:520px;overflow:auto;white-space:pre-wrap;word-break:break-word;margin:8px 0 0;font:12px/1.55 ui-monospace,SFMono-Regular,Consolas,monospace}.status{display:inline-flex;border-radius:5px;padding:2px 8px;font-size:12px;font-weight:900}.ok{background:rgba(52,211,153,.14);color:var(--green)}.err{background:rgba(251,113,133,.15);color:var(--red)}.empty{padding:48px 18px;text-align:center;color:var(--muted)}@media(max-width:920px){.topbar{grid-template-columns:1fr}.layout{display:block;height:auto}.sidebar{max-height:48vh;border-right:0;border-bottom:1px solid var(--border)}.grid{grid-template-columns:1fr}.main{padding:14px}.path{white-space:normal;overflow-wrap:anywhere}}
</style>
</head>
<body>
<header class="topbar">
  <div class="brand">
    <strong>Twilight API 控制台</strong>
    <span>未登录只显示公开 OpenAPI；管理员登录后优先显示完整路由清单。</span>
  </div>
  <input id="baseurl" class="control" value="/api/v1" aria-label="Base URL">
  <input id="apikey" class="control" type="password" autocomplete="off" placeholder="API Key（可选，留空使用 Cookie）" aria-label="API Key">
  <button id="reload" class="btn" type="button">刷新路由</button>
</header>
<div class="layout">
  <aside class="sidebar" aria-label="API 路由列表">
    <div class="filters">
      <input id="search" placeholder="搜索路径、分组、方法或鉴权级别" aria-label="搜索路由">
      <select id="methodFilter" aria-label="方法过滤">
        <option value="">全部方法</option>
        <option value="GET">GET</option>
        <option value="POST">POST</option>
        <option value="PUT">PUT</option>
        <option value="PATCH">PATCH</option>
        <option value="DELETE">DELETE</option>
      </select>
      <select id="authFilter" aria-label="鉴权过滤">
        <option value="">全部鉴权</option>
        <option value="Public">Public</option>
        <option value="User">User</option>
        <option value="Admin">Admin</option>
        <option value="API Key">API Key</option>
      </select>
    </div>
    <div class="summary"><span id="source">正在加载</span><span id="count">0 条</span></div>
    <div id="routes" class="routes"><div class="empty">正在加载路由...</div></div>
  </aside>
  <main id="main" class="main">
    <section class="hero">
      <h1>API Explorer</h1>
      <p>从左侧选择接口后，可查看鉴权级别并发送测试请求。公开视图只来自 /api/v1/openapi.json；完整路由清单仅在当前会话具有管理员权限时从 /api/v1/system/admin/apis 加载。</p>
      <div class="notice">提示：不要在共享屏幕、截图或日志中暴露 API Key、Cookie、Token、真实用户资料或数据库信息。</div>
    </section>
  </main>
</div>
<script>
"use strict";
const API_KEY_HEADER = "X-API-Key";
const state = { routes: [], active: null, sourceLabel: "公开路由" };
const $ = (id) => document.getElementById(id);

function methodClass(method) {
  return String(method || "GET").toLowerCase();
}

function normalizePath(path) {
  const value = String(path || "/").trim().replace(/^\/api\/v1(?=\/|$)/, "");
  return value.startsWith("/") ? value : "/" + value;
}

function normalizeAuth(auth) {
  const value = String(auth || "Public").replace(/^Auth/, "");
  if (/api.?key/i.test(value)) return "API Key";
  if (/admin/i.test(value)) return "Admin";
  if (/user/i.test(value)) return "User";
  return "Public";
}

function groupOf(path) {
  const parts = normalizePath(path).split("/").filter(Boolean);
  return parts[0] || "root";
}

function baseURL() {
  return ($("baseurl").value || "/api/v1").trim().replace(/\/+$/, "") || "/api/v1";
}

function endpointURL(route) {
  return baseURL() + normalizePath(route.path);
}

function prettyJSON(text) {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}

function setLoading(message) {
  const box = document.createElement("div");
  box.className = "empty";
  box.textContent = message;
  $("routes").replaceChildren(box);
}

function filteredRoutes() {
  const q = $("search").value.trim().toLowerCase();
  const method = $("methodFilter").value;
  const auth = $("authFilter").value;
  return state.routes.filter((route) => {
    if (method && route.method !== method) return false;
    if (auth && route.auth !== auth) return false;
    if (!q) return true;
    return route.path.toLowerCase().includes(q) ||
      route.method.toLowerCase().includes(q) ||
      route.auth.toLowerCase().includes(q) ||
      groupOf(route.path).toLowerCase().includes(q);
  });
}

function routeButton(route) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "route";
  if (state.active && state.active.method === route.method && state.active.path === route.path) {
    button.className += " active";
  }
  const method = document.createElement("span");
  method.className = "method " + methodClass(route.method);
  method.textContent = route.method;
  const path = document.createElement("span");
  path.className = "path";
  path.textContent = route.path;
  button.append(method, path);
  button.addEventListener("click", () => {
    state.active = route;
    renderRoutes();
    renderDetail();
  });
  return button;
}

function renderRoutes() {
  const items = filteredRoutes();
  $("source").textContent = state.sourceLabel;
  $("count").textContent = items.length + " 条";
  if (!items.length) {
    setLoading("没有匹配的接口");
    return;
  }
  const groups = new Map();
  for (const route of items) {
    const key = groupOf(route.path);
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(route);
  }
  const fragment = document.createDocumentFragment();
  for (const name of Array.from(groups.keys()).sort()) {
    const title = document.createElement("div");
    title.className = "group";
    title.textContent = name;
    fragment.appendChild(title);
    for (const route of groups.get(name)) fragment.appendChild(routeButton(route));
  }
  $("routes").replaceChildren(fragment);
}

function field(label, element) {
  const wrap = document.createElement("div");
  wrap.className = "field";
  const lab = document.createElement("label");
  lab.textContent = label;
  wrap.append(lab, element);
  return wrap;
}

function responseBox(status, text, ok, elapsed) {
  const box = document.createElement("div");
  box.className = "response";
  const badge = document.createElement("span");
  badge.className = "status " + (ok ? "ok" : "err");
  badge.textContent = status;
  box.appendChild(badge);
  if (elapsed) box.appendChild(document.createTextNode(" " + elapsed + "ms"));
  const pre = document.createElement("pre");
  pre.textContent = text;
  box.appendChild(pre);
  return box;
}

function renderDetail() {
  const route = state.active;
  if (!route) return;
  const panel = document.createElement("section");
  panel.className = "panel";
  const title = document.createElement("h2");
  const method = document.createElement("span");
  method.className = "method " + methodClass(route.method);
  method.textContent = route.method;
  title.append(method, document.createTextNode(" 请求详情"));
  const endpoint = document.createElement("div");
  endpoint.className = "endpoint";
  endpoint.textContent = endpointURL(route);
  const meta = document.createElement("div");
  meta.className = "meta";
  for (const value of ["鉴权：" + route.auth, "分组：" + groupOf(route.path), "来源：" + state.sourceLabel]) {
    const pill = document.createElement("span");
    pill.className = "pill";
    pill.textContent = value;
    meta.appendChild(pill);
  }
  const query = document.createElement("input");
  query.id = "query";
  query.placeholder = "limit=20&refresh=1";
  const headers = document.createElement("input");
  headers.id = "headers";
  headers.placeholder = "{\"X-Trace-ID\":\"demo\"}";
  const grid = document.createElement("div");
  grid.className = "grid";
  grid.append(field("查询字符串（可选，不含 ?）", query), field("额外请求头（JSON，可选）", headers));
  const body = document.createElement("textarea");
  body.id = "body";
  body.placeholder = "{\"key\":\"value\"}";
  const needsBody = !["GET", "HEAD"].includes(route.method);
  const actions = document.createElement("div");
  actions.className = "actions";
  const send = document.createElement("button");
  send.type = "button";
  send.className = "btn";
  send.textContent = "发送请求";
  send.addEventListener("click", sendRequest);
  const copyURL = document.createElement("button");
  copyURL.type = "button";
  copyURL.className = "btn secondary";
  copyURL.textContent = "复制 URL";
  copyURL.addEventListener("click", () => navigator.clipboard && navigator.clipboard.writeText(endpointURL(route)));
  const copyCurl = document.createElement("button");
  copyCurl.type = "button";
  copyCurl.className = "btn secondary";
  copyCurl.textContent = "复制 cURL";
  copyCurl.addEventListener("click", () => navigator.clipboard && navigator.clipboard.writeText(buildCurl()));
  actions.append(send, copyURL, copyCurl);
  const response = document.createElement("div");
  response.id = "response";
  panel.append(title, endpoint, meta, grid, field("请求体 JSON" + (needsBody ? "" : "（GET/HEAD 会忽略）"), body), actions, response);
  $("main").replaceChildren(panel);
}

function currentURL() {
  const route = state.active;
  if (!route) return "";
  const query = ($("query") && $("query").value.trim()) || "";
  return endpointURL(route) + (query ? (query.startsWith("?") ? query : "?" + query) : "");
}

function parseExtraHeaders() {
  const raw = ($("headers") && $("headers").value.trim()) || "";
  if (!raw) return {};
  const parsed = JSON.parse(raw);
  if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") {
    throw new Error("请求头必须是 JSON 对象");
  }
  return parsed;
}

function buildHeaders() {
  const headers = { "Accept": "application/json" };
  const key = $("apikey").value.trim();
  if (key) headers[API_KEY_HEADER] = key;
  Object.assign(headers, parseExtraHeaders());
  return headers;
}

function shellQuote(value) {
  return "'" + String(value).replace(/'/g, "'\\''") + "'";
}

function buildCurl() {
  if (!state.active) return "";
  const headers = buildHeaders();
  const parts = ["curl", "-i", "-X", state.active.method, shellQuote(currentURL())];
  for (const [key, value] of Object.entries(headers)) {
    parts.push("-H", shellQuote(key + ": " + value));
  }
  const body = ($("body") && $("body").value.trim()) || "";
  if (!["GET", "HEAD"].includes(state.active.method) && body) {
    parts.push("-H", shellQuote("Content-Type: application/json"), "--data", shellQuote(body));
  }
  return parts.join(" ");
}

function showError(message) {
  $("response").replaceChildren(responseBox("ERROR", message, false, 0));
}

async function sendRequest() {
  if (!state.active) return;
  let headers;
  try {
    headers = buildHeaders();
  } catch (err) {
    showError(err.message || String(err));
    return;
  }
  const opts = { method: state.active.method, headers, credentials: "include" };
  const body = $("body").value.trim();
  if (!["GET", "HEAD"].includes(state.active.method) && body) {
    headers["Content-Type"] = "application/json";
    opts.body = body;
  }
  $("response").replaceChildren(responseBox("PENDING", "请求中...", true, 0));
  try {
    const start = performance.now();
    const resp = await fetch(currentURL(), opts);
    const elapsed = Math.round(performance.now() - start);
    const text = await resp.text();
    $("response").replaceChildren(responseBox(String(resp.status), prettyJSON(text), resp.status < 400, elapsed));
  } catch (err) {
    showError(err.message || String(err));
  }
}

async function loadRoutes() {
  setLoading("正在加载路由...");
  state.routes = [];
  state.active = null;
  state.sourceLabel = "公开路由";
  try {
    let resp = await fetch("/api/v1/system/admin/apis", { credentials: "include" });
    if (resp.ok) {
      const json = await resp.json();
      const items = (json.data && json.data.apis) || [];
      state.routes = items.map((item) => ({
        method: String(item.method || "GET").toUpperCase(),
        path: normalizePath(item.full_path || item.endpoint || item.path),
        auth: normalizeAuth(item.auth),
      }));
      state.sourceLabel = "管理员完整路由";
    } else {
      resp = await fetch("/api/v1/openapi.json", { credentials: "include" });
      const json = await resp.json();
      const data = json.data || json;
      const paths = data.paths || {};
      state.routes = Object.entries(paths).flatMap(([path, methods]) =>
        Object.keys(methods || {}).map((method) => ({
          method: method.toUpperCase(),
          path: normalizePath(path),
          auth: "Public",
        }))
      );
      state.sourceLabel = "公开路由";
    }
    state.routes.sort((a, b) => a.path.localeCompare(b.path) || a.method.localeCompare(b.method));
    renderRoutes();
  } catch (err) {
    setLoading("加载失败：" + (err.message || String(err)));
  }
}

$("reload").addEventListener("click", loadRoutes);
$("search").addEventListener("input", renderRoutes);
$("methodFilter").addEventListener("change", renderRoutes);
$("authFilter").addEventListener("change", renderRoutes);
$("baseurl").addEventListener("change", () => { if (state.active) renderDetail(); });
loadRoutes();
</script>
</body>
</html>`

func (a *App) handleUpdateMe(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	payload := decodeMap(r)
	bgmModeNext, bgmModeSet, okBool := requireStrictBoolValue(w, payload, "bgm_mode")
	if !okBool {
		return
	}
	bgmManageModeNext, bgmManageModeSet, okBool := requireStrictBoolValue(w, payload, "bgm_manage_mode")
	if !okBool {
		return
	}
	notifyLoginTelegramNext, notifyLoginTelegramSet, okBool := requireStrictBoolValue(w, payload, "notify_on_login_telegram")
	if !okBool {
		return
	}
	notifyLoginEmailNext, notifyLoginEmailSet, okBool := requireStrictBoolValue(w, payload, "notify_on_login_email")
	if !okBool {
		return
	}
	notifyTicketTelegramNext, notifyTicketTelegramSet, okBool := requireStrictBoolValue(w, payload, "notify_on_ticket_telegram")
	if !okBool {
		return
	}
	signinAutoRenewalNext, signinAutoRenewalSet, okBool := requireStrictBoolValue(w, payload, "signin_auto_renewal")
	if !okBool {
		return
	}
	if signinAutoRenewalSet && signinAutoRenewalNext {
		cfg := *a.cfg()
		switch {
		case !signinAutoRenewalEnabled(cfg):
			failWithCode(w, http.StatusForbidden, ErrSigninAutoRenewalDisabled, "管理员未开启签到自动续期")
			return
		case a.userIsProtected(p.User):
			failWithCode(w, http.StatusConflict, ErrConflict, "管理员和白名单账号无需自动续期")
			return
		case validateSelfServiceRenewalTarget(p.User) != nil:
			failWithCode(w, http.StatusConflict, ErrRenewRequiresEmby, "请先绑定或开通 Emby 账号，再开启自动续期")
			return
		case p.User.ExpiredAt <= 0 || expiryIsPermanent(p.User.ExpiredAt):
			failWithCode(w, http.StatusConflict, ErrConflict, "当前账号没有需要自动续期的有效期")
			return
		}
	}
	if !a.cfg().BangumiEnabled {
		if bgmModeSet {
			failWithCode(w, http.StatusForbidden, ErrBangumiSyncDisabled, "Bangumi 同步未开启")
			return
		}
	}
	if !a.cfg().BangumiManageEnabled {
		if bgmManageModeSet {
			failWithCode(w, http.StatusForbidden, ErrBangumiManageDisabled, "Bangumi 管理功能未开启")
			return
		}
	}
	if token := stringValue(payload, "bgm_token"); token != "" && !a.cfg().BangumiEnabled && !a.cfg().BangumiManageEnabled {
		failWithCode(w, http.StatusForbidden, ErrBangumiSyncDisabled, "Bangumi 功能未开启")
		return
	}
	if token := stringValue(payload, "bgm_token"); len(token) > 4096 {
		failWithCode(w, http.StatusBadRequest, ErrBangumiTokenTooLong, "Bangumi Token 过长")
		return
	}
	nextToken := p.User.BGMToken
	if _, ok := payload["bgm_token"]; ok {
		nextToken = stringValue(payload, "bgm_token")
	}
	if a.cfg().BangumiEnabled && bgmModeSet && bgmModeNext && nextToken == "" {
		failWithCode(w, http.StatusBadRequest, ErrBangumiTokenMissing, "启用 Bangumi 同步前请先填写个人 Token")
		return
	}
	if a.cfg().BangumiManageEnabled && bgmManageModeSet && bgmManageModeNext && nextToken == "" {
		failWithCode(w, http.StatusBadRequest, ErrBangumiTokenMissing, "启用 Bangumi 管理前请先填写个人 Token")
		return
	}
	bgmTokenChanged := false
	if _, ok := payload["bgm_token"]; ok {
		bgmTokenChanged = nextToken != p.User.BGMToken
	}
	// 强制邮箱验证开启时，普通/白名单用户不能再走通用资料更新随意改邮箱（那会绕过
	// 验证码校验），必须到邮箱验证区用验证码绑定 / 换绑。
	if email := strings.TrimSpace(stringValue(payload, "email")); email != "" && !strings.EqualFold(email, p.User.Email) && a.emailGateActive(p.User) {
		failWithCode(w, http.StatusForbidden, ErrEmailVerificationRequired, "已开启强制邮箱验证，请在邮箱验证区通过验证码绑定或更换邮箱")
		return
	}
	passwordChangeEmailRequiredSet := false
	passwordChangeEmailRequiredNext := p.User.RequireEmailForPasswordChange
	if _, ok := payload["password_change_email_required"]; ok {
		next, _, okBool := requireStrictBoolValue(w, payload, "password_change_email_required")
		if !okBool {
			return
		}
		passwordChangeEmailRequiredSet = true
		passwordChangeEmailRequiredNext = next
		if next {
			if !emailConfigured(a.cfg()) {
				failWithCode(w, http.StatusServiceUnavailable, ErrEmailDisabled, "邮箱功能未启用")
				return
			}
			if !p.User.EmailVerified || strings.TrimSpace(p.User.Email) == "" {
				failWithCode(w, http.StatusForbidden, ErrEmailVerificationRequired, "请先绑定并验证邮箱")
				return
			}
		}
		if !next {
			if a.emailGateActive(p.User) {
				failWithCode(w, http.StatusForbidden, ErrEmailVerificationRequired, "全局已强制邮箱验证，不能关闭该项")
				return
			}
			if p.User.RequireEmailForPasswordChange && !a.consumePasswordChangeEmailCode(w, payload, p.User, emailPurposeChangePass) {
				return
			}
		}
	}
	embyPasswordEmailRequiredSet := false
	embyPasswordEmailRequiredNext := p.User.RequireEmailForEmbyPasswordChange
	if _, ok := payload["emby_password_email_required"]; ok {
		next, _, okBool := requireStrictBoolValue(w, payload, "emby_password_email_required")
		if !okBool {
			return
		}
		embyPasswordEmailRequiredSet = true
		embyPasswordEmailRequiredNext = next
		if next {
			if !emailConfigured(a.cfg()) {
				failWithCode(w, http.StatusServiceUnavailable, ErrEmailDisabled, "邮箱功能未启用")
				return
			}
			if !p.User.EmailVerified || strings.TrimSpace(p.User.Email) == "" {
				failWithCode(w, http.StatusForbidden, ErrEmailVerificationRequired, "请先绑定并验证邮箱")
				return
			}
		}
		if !next {
			if a.emailGateActive(p.User) {
				failWithCode(w, http.StatusForbidden, ErrEmailVerificationRequired, "全局已强制邮箱验证，不能关闭该项")
				return
			}
			if p.User.RequireEmailForEmbyPasswordChange && !a.consumePasswordChangeEmailCode(w, payload, p.User, emailPurposeChangeEmby) {
				return
			}
		}
	}
	embyPasswordOldPasswordRequiredSet := false
	embyPasswordOldPasswordRequiredNext := p.User.RequireOldPasswordForEmbyPasswordChange
	if _, ok := payload["emby_password_old_password_required"]; ok {
		next, _, okBool := requireStrictBoolValue(w, payload, "emby_password_old_password_required")
		if !okBool {
			return
		}
		embyPasswordOldPasswordRequiredSet = true
		embyPasswordOldPasswordRequiredNext = next
		if p.User.RequireOldPasswordForEmbyPasswordChange && !next {
			if !security.VerifyPassword(stringValue(payload, "old_password"), p.User.PasswordHash) {
				failWithCode(w, http.StatusForbidden, ErrPasswordOldMismatch, "原密码不正确")
				return
			}
		}
	}
	u, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error {
		if email := stringValue(payload, "email"); email != "" {
			if err := validate.ValidateEmailFormat(email); err != nil {
				return err
			}
			email = strings.TrimSpace(email)
			if len(a.cfg().EmailBlacklist) > 0 && validate.CheckEmailBlacklist(email, a.cfg().EmailBlacklist) {
				return fmt.Errorf("该邮箱域名不在允许范围内")
			}
			if len(a.cfg().EmailWhitelist) > 0 && !validate.CheckEmailWhitelist(email, a.cfg().EmailWhitelist) {
				return fmt.Errorf("该邮箱域名不在允许范围内")
			}
			// 邮箱发生变化即视为未验证：通用资料更新不经过验证码，不能保留旧的
			// 已验证状态，否则用户改成任意邮箱后仍显示"已验证"。
			if !strings.EqualFold(email, u.Email) {
				u.EmailVerified = false
				u.EmailVerifiedAt = 0
			}
			u.Email = email
		}
		if username := stringValue(payload, "username"); username != "" {
			if err := validate.ValidateUsername(username); err != nil {
				return err
			}
			u.Username = username
		}
		if bgmModeSet {
			u.BGMMode = bgmModeNext
		}
		if bgmManageModeSet {
			u.BGMManageMode = bgmManageModeNext
		}
		if _, ok := payload["bgm_token"]; ok {
			token := stringValue(payload, "bgm_token")
			u.BGMToken = token
			if token == "" {
				u.BGMMode = false
				u.BGMManageMode = false
			}
		}
		if notifyLoginTelegramSet {
			u.NotifyOnLoginTelegram = notifyLoginTelegramNext
		}
		if notifyLoginEmailSet {
			u.NotifyOnLoginEmail = notifyLoginEmailNext
		}
		if notifyTicketTelegramSet {
			u.NotifyOnTicketTelegram = notifyTicketTelegramNext
		}
		if signinAutoRenewalSet {
			if signinAutoRenewalNext {
				if !signinAutoRenewalEnabled(*a.cfg()) || a.userIsProtected(*u) || u.ExpiredAt <= 0 || expiryIsPermanent(u.ExpiredAt) {
					return store.ErrConflict
				}
				if err := validateSelfServiceRenewalTarget(*u); err != nil {
					return err
				}
			}
			u.SigninAutoRenewal = signinAutoRenewalNext
		}
		if passwordChangeEmailRequiredSet {
			u.RequireEmailForPasswordChange = passwordChangeEmailRequiredNext
		}
		if embyPasswordEmailRequiredSet {
			u.RequireEmailForEmbyPasswordChange = embyPasswordEmailRequiredNext
		}
		if embyPasswordOldPasswordRequiredSet {
			u.RequireOldPasswordForEmbyPasswordChange = embyPasswordOldPasswordRequiredNext
		}
		return nil
	})
	if signinAutoRenewalSet && signinAutoRenewalNext {
		switch {
		case errors.Is(err, store.ErrEmbyRequired):
			failWithCode(w, http.StatusConflict, ErrRenewRequiresEmby, "请先绑定或开通 Emby 账号，再开启自动续期")
			return
		case errors.Is(err, store.ErrConflict):
			failWithCode(w, http.StatusConflict, ErrConflict, "账号状态已变化，请刷新后重试")
			return
		}
	}
	if statusFromError(w, err) {
		return
	}
	if bgmTokenChanged {
		_ = a.store().DeleteBangumiCollectionCache(u.UID, 0)
	}
	if signinAutoRenewalSet {
		a.audit(r, "update_signin_auto_renewal", "user", 0, map[string]any{"enabled": signinAutoRenewalNext})
	} else {
		a.audit(r, "update_profile", "user", 0, nil)
	}
	ok(w, "更新成功", publicUser(u))
}

func (a *App) handleUpdateUsername(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	payload := decodeMap(r)
	username := stringValue(payload, "new_username")
	if username == "" {
		failWithCode(w, http.StatusBadRequest, ErrUserNewUsernameRequired, "请填写新用户名")
		return
	}
	if err := validate.ValidateUsername(username); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrUsernameInvalid, err.Error())
		return
	}
	u, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error {
		u.Username = username
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "用户名已更新", publicUser(u))
}

// rotateSessionsAfterPasswordChange 在用户自助修改密码后吊销其全部会话（驱逐任何
// 被盗 token），再为当前调用方签发一份新会话，使其在本设备保持登录：cookie 客户端
// 透明续期（重写 session cookie），Bearer 客户端用返回体里的新 token 续用。
// 与 handleAdminResetPassword / handleForgotPassword 的「改密即吊销旧会话」口径一致。
// 失败时已写响应，返回 ok=false，调用方直接 return。
func (a *App) rotateSessionsAfterPasswordChange(w http.ResponseWriter, r *http.Request, uid int64) (string, bool) {
	a.sessions().DeleteUser(r.Context(), uid)
	token, expires, err := a.sessions().Create(r.Context(), uid)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrSessionCreateFailed, "创建会话失败")
		return "", false
	}
	a.issueSessionCookies(w, token, expires)
	return token, true
}

func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	if !a.allowRate(r.Context(), rateKey("change-pwd:", p.User.UID), 10, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "操作过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	oldPassword := stringValue(payload, "old_password")
	newPassword := stringValue(payload, "new_password")
	if !security.VerifyPassword(oldPassword, p.User.PasswordHash) {
		failWithCode(w, http.StatusForbidden, ErrPasswordOldMismatch, "原密码不正确")
		return
	}
	// 强制邮箱验证开启时，改系统密码需先通过邮箱验证码（未强制则直接放行）。
	if !a.consumePasswordChangeEmailCode(w, payload, p.User, emailPurposeChangePass) {
		return
	}
	if err := validate.ValidatePasswordStrength(newPassword); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrPasswordWeak, err.Error())
		return
	}
	hash, err := security.HashPassword(newPassword)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrPasswordHashFailed, "密码处理失败")
		return
	}
	_, err = a.store().UpdateUser(p.User.UID, func(u *store.User) error { u.PasswordHash = hash; return nil })
	if statusFromError(w, err) {
		return
	}
	token, rotated := a.rotateSessionsAfterPasswordChange(w, r, p.User.UID)
	if !rotated {
		return
	}
	a.audit(r, "change_password", "user", 0, nil)
	ok(w, "password updated", map[string]any{"token": token})
}

func (a *App) handleGeneratedPassword(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	if !a.allowRate(r.Context(), rateKey("gen-pwd:", p.User.UID), 5, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "操作过于频繁，请稍后再试")
		return
	}
	// 自动生成密码至少 128 bit 熵：32 hex chars。
	// 旧实现使用 randomCode(12) = 48 bit，对在线/离线攻击都过弱。
	password := "Twilight-" + randomCode(generatedPasswordHexLen)
	hash, err := security.HashPassword(password)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrPasswordHashFailed, "密码处理失败")
		return
	}
	_, err = a.store().UpdateUser(p.User.UID, func(u *store.User) error { u.PasswordHash = hash; return nil })
	if statusFromError(w, err) {
		return
	}
	token, rotated := a.rotateSessionsAfterPasswordChange(w, r, p.User.UID)
	if !rotated {
		return
	}
	a.audit(r, "generate_password", "user", 0, nil)
	ok(w, "password reset", map[string]any{"new_password": password, "token": token})
}

func (a *App) handleChangeEmbyPassword(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	if p.User.EmbyID == "" {
		failWithCode(w, http.StatusBadRequest, ErrEmbyAccountUnlinked, "当前账号未关联 Emby")
		return
	}
	payload := decodeMap(r)
	newPassword := stringValue(payload, "new_password")
	if p.User.RequireOldPasswordForEmbyPasswordChange {
		if !security.VerifyPassword(stringValue(payload, "old_password"), p.User.PasswordHash) {
			failWithCode(w, http.StatusForbidden, ErrPasswordOldMismatch, "原密码不正确")
			return
		}
	}
	if okPass, msg := validateStrongPassword(newPassword, "Emby password"); !okPass {
		failWithCode(w, http.StatusBadRequest, ErrPasswordWeak, msg)
		return
	}
	// 强制邮箱验证开启时，改 Emby 密码需先通过邮箱验证码（未强制则直接放行）。
	if !a.consumePasswordChangeEmailCode(w, payload, p.User, emailPurposeChangeEmby) {
		return
	}
	if err := a.embySetPassword(r.Context(), p.User.EmbyID, newPassword); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyPasswordUpdateFailed, "更新 Emby 密码失败，请稍后重试")
		return
	}
	a.audit(r, "change_emby_password", "user", 0, nil)
	ok(w, "Emby password updated", nil)
}

func (a *App) handleBindEmby(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireEmailVerified(w, p.User) {
		return
	}
	if p.User.EmbyID != "" {
		failWithCode(w, http.StatusBadRequest, ErrEmbyAlreadyLinked, "当前账号已关联 Emby 账号")
		return
	}
	payload := decodeMap(r)
	embyUsername := stringValue(payload, "emby_username")
	embyPassword := stringValue(payload, "emby_password")
	if embyUsername == "" {
		failWithCode(w, http.StatusBadRequest, ErrEmbyMissingCreds, "请填写 Emby 用户名")
		return
	}
	embyUser, okAuth, err := a.embyAuthenticateByName(r.Context(), embyUsername, embyPassword)
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyAuthFailed, "Emby 鉴权失败，请稍后重试或检查上游 Emby 状态")
		return
	}
	if !okAuth {
		failWithCode(w, http.StatusUnauthorized, ErrEmbyAuthFailed, "Emby 用户名或密码错误")
		return
	}
	embyID := firstNonEmpty(asString(embyUser["Id"]), asString(embyUser["ID"]), asString(embyUser["id"]))
	if embyID == "" {
		failWithCode(w, http.StatusBadGateway, ErrEmbyCreateNoID, "Emby 未返回用户 ID")
		return
	}
	// Security: prevent non-admin users from binding Emby administrator accounts
	if p.User.Role != store.RoleAdmin {
		policy := embyPolicy(embyUser)
		if boolish(policy["IsAdministrator"]) {
			failWithCode(w, http.StatusForbidden, ErrEmbyAdminLinkForbidden, "安全限制：不允许绑定 Emby 管理员账号。如需绑定，请联系系统管理员。")
			return
		}
	}
	u, _, err := a.store().BindUserEmbyAtomicWithUpdate(p.User.UID, embyID, firstNonEmpty(asString(embyUser["Name"]), embyUsername), false, func(u *store.User, before store.User) error {
		if strings.TrimSpace(before.EmbyID) != "" {
			return store.ErrConflict
		}
		a.consumePendingEmbyEntitlementOnBind(u, before)
		return nil
	})
	if errors.Is(err, store.ErrConflict) {
		if latest, ok := a.store().User(p.User.UID); ok && strings.TrimSpace(latest.EmbyID) != "" {
			failWithCode(w, http.StatusBadRequest, ErrEmbyAlreadyLinked, "当前账号已关联 Emby 账号")
			return
		}
		failWithCode(w, http.StatusConflict, ErrEmbyLinkedOtherUser, "该 Emby 账号已关联其他用户")
		return
	}
	if statusFromError(w, err) {
		return
	}
	_ = a.embyApplyEnabledState(r.Context(), u.UID, u.EmbyID, a.embyShouldEnableUser(u))
	a.audit(r, "bind_emby", "user", 0, nil)
	ok(w, "Emby account linked", map[string]any{"emby_id": u.EmbyID, "emby_username": u.EmbyUsername, "user": publicUser(u)})
}

func (a *App) consumePendingEmbyEntitlementOnBind(u *store.User, before store.User) {
	if !before.PendingEmby {
		return
	}
	days := a.cfg().EmbyDirectRegisterDays
	if before.PendingEmbyDays != nil {
		days = *before.PendingEmbyDays
	}
	if days == 0 {
		days = 30
	}
	markRegistrationGrant(u, firstNonEmpty(before.RegistrationSource, registrationSourceRegCode), before.RegistrationCode)
	if u.Role == store.RoleUnrecognized {
		u.Role = store.RoleNormal
	}
	if u.Role == store.RoleAdmin || u.Role == store.RoleWhitelist || days < 0 {
		u.ExpiredAt = permanentExpiryUnix
	} else {
		u.ExpiredAt = expiryFromDays(days, time.Now())
	}
}

func (a *App) handleRegisterEmby(w http.ResponseWriter, r *http.Request, params Params) {
	p := current(r)
	if a.requireEmailVerified(w, p.User) {
		return
	}
	if p.User.EmbyID != "" {
		failWithCode(w, http.StatusBadRequest, ErrEmbyAlreadyLinked, "当前账号已关联 Emby 账号")
		return
	}
	if !p.User.PendingEmby && !a.cfg().EmbyDirectRegisterEnabled {
		failWithCode(w, http.StatusBadRequest, ErrEmbyNoRegistrationGrant, "当前账号没有 Emby 注册资格")
		return
	}
	if !p.User.PendingEmby && a.userHasEmbyGrantHistory(p.User) {
		failWithCode(w, http.StatusBadRequest, ErrCodeRegistrationGrantAlreadyUsed, "当前账号已经使用过 Emby 注册资格，不能重复自助开通 Emby")
		return
	}
	payload := decodeMap(r)
	embyUsername := stringValue(payload, "emby_username")
	embyPassword := stringValue(payload, "emby_password")
	if embyUsername == "" {
		failWithCode(w, http.StatusBadRequest, ErrEmbyMissingCreds, "请填写 Emby 用户名")
		return
	}
	if okPass, msg := validateStrongPassword(embyPassword, "Emby password"); !okPass {
		failWithCode(w, http.StatusBadRequest, ErrPasswordWeak, msg)
		return
	}
	if existing, exists, err := a.embyUserByName(r.Context(), embyUsername); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyUsernameLookupFailed, "查询 Emby 用户名失败，请稍后重试")
		return
	} else if exists {
		failWithCode(w, http.StatusConflict, ErrEmbyUsernameTaken, "Emby 用户名已存在："+asString(existing["Name"]))
		return
	}
	if reached, current, limit := a.embyCapacityReached(p.User.UID); reached {
		failWithCode(w, http.StatusConflict, ErrEmbyCapacityReached, fmt.Sprintf("Emby 用户数量已达上限 %d/%d", current, limit))
		return
	}
	createdUser, err := a.embyCreateUser(r.Context(), embyUsername, embyPassword)
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyCreateFailed, "创建 Emby 用户失败，请稍后重试")
		return
	}
	embyID := asString(createdUser["Id"])
	u, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error {
		if strings.TrimSpace(u.EmbyID) != "" {
			return store.ErrConflict
		}
		days := a.cfg().EmbyDirectRegisterDays
		if u.PendingEmbyDays != nil {
			days = *u.PendingEmbyDays
		}
		if days == 0 {
			days = 30
		}
		hadPendingEmby := u.PendingEmby
		u.EmbyID = embyID
		u.EmbyUsername = firstNonEmpty(asString(createdUser["Name"]), embyUsername)
		u.PendingEmby = false
		u.PendingEmbyDays = nil
		if hadPendingEmby {
			markRegistrationGrant(u, firstNonEmpty(u.RegistrationSource, registrationSourceRegCode), u.RegistrationCode)
		} else {
			markRegistrationGrant(u, registrationSourceRegCode, "")
		}
		if u.Role == store.RoleUnrecognized {
			u.Role = store.RoleNormal
		}
		if u.Role == store.RoleAdmin || u.Role == store.RoleWhitelist || days < 0 {
			u.ExpiredAt = permanentExpiryUnix
		} else {
			u.ExpiredAt = expiryFromDays(days, time.Now())
		}
		return nil
	})
	if errors.Is(err, store.ErrConflict) {
		_ = a.embyDelete(r.Context(), "/Users/"+urlPathEscape(embyID))
		failWithCode(w, http.StatusBadRequest, ErrEmbyAlreadyLinked, "当前账号已关联 Emby 账号")
		return
	}
	if statusFromError(w, err) {
		_ = a.embyDelete(r.Context(), "/Users/"+urlPathEscape(embyID))
		return
	}
	_ = a.embyApplyEnabledState(r.Context(), u.UID, u.EmbyID, a.embyShouldEnableUser(u))
	a.audit(r, "register_emby", "user", 0, nil)
	ok(w, "Emby account created", map[string]any{"user": publicUser(u), "emby_id": u.EmbyID, "emby_username": u.EmbyUsername, "request_id": ""})
}

func (a *App) handleUnbindEmby(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	user, okUser := a.store().User(p.User.UID)
	if !okUser {
		failWithCode(w, http.StatusNotFound, ErrUserNotFound, userNotFoundMessage)
		return
	}
	if a.requireNonEmbyAdmin(w, r, user) {
		return
	}
	if !a.userCanSelfUnbindEmby(user) {
		failWithCode(w, http.StatusForbidden, ErrEmbyUnbindForbidden, "当前账号的 Emby 注册资格来自注册码、邀请码或管理员授予，不能自助解绑后重复注册")
		return
	}
	embyID := user.EmbyID
	remoteDisabled, proceed := a.disableRemoteEmbyForUnbind(w, r, embyID)
	if !proceed {
		return
	}
	u, err := a.store().UpdateUser(user.UID, func(u *store.User) error {
		if embyID != "" && u.EmbyID != "" && u.EmbyID != embyID {
			return store.ErrConflict
		}
		u.EmbyID = ""
		u.EmbyUsername = ""
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	data := publicUser(u)
	data["remote_emby_disabled"] = remoteDisabled
	data["old_emby_id"] = embyID
	a.audit(r, "unbind_emby", "user", 0, nil)
	ok(w, "远端 Emby 账号已禁用，并已解除本地绑定", data)
}

func (a *App) handleRenew(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireEmailVerified(w, p.User) {
		return
	}
	// Self-service renewal requires a reg_code; bare renewal without code is forbidden
	payload := decodeMap(r)
	regCode := stringValue(payload, "reg_code")
	if regCode == "" {
		failWithCode(w, http.StatusBadRequest, ErrRenewCodeRequired, "续期需要提供注册码")
		return
	}
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	if !a.allowRate(r.Context(), rateKey("renew:", p.User.UID), 10, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "操作过于频繁，请稍后再试")
		return
	}
	if user, ok := a.refreshCurrentUserForRequest(w, r); ok {
		p.User = user
	} else {
		return
	}
	preview, source, okPreview := a.previewCode(r.Context(), regCode, p.User)
	if !okPreview || source != "regcode" || int(numeric(preview["type"])) != 2 {
		failWithCode(w, http.StatusBadRequest, ErrRenewCodeInvalid, "续期码无效、已用完、已过期或不属于当前用户")
		return
	}
	if rejectSelfServiceRenewalWithoutEmby(w, p.User) {
		return
	}
	if a.rejectRegcodeWriteIfStorageMismatch(w) {
		return
	}
	u, _, err := a.store().ConsumeRegCodeAndUpdateUser(regCode, p.User.UID, p.User.TelegramID, func(u *store.User, code store.RegCode) error {
		if err := validateSelfServiceRenewalTarget(*u); err != nil {
			return err
		}
		days := normalizeRegCodeDays(code.Days)
		// 用 renewExpiryAndReactivate 而不是裸 ExpiredAt = ...：自助续费会
		// 把曾被 check_expired 设成 Active=false 的非邀请账号同步解禁，避免
		// "续完仍登不上"的死循环。
		renewExpiryAndReactivate(u, addDaysToExpiry(u.ExpiredAt, days, time.Now()))
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrEmbyRequired) {
			failWithCode(w, http.StatusConflict, ErrRenewRequiresEmby, "请先绑定或开通 Emby 账号，再使用续期码")
			return
		}
		if !errors.Is(err, store.ErrNotFound) && !errors.Is(err, store.ErrExpired) && !errors.Is(err, store.ErrConflict) {
			statusFromError(w, err)
			return
		}
		failWithCode(w, http.StatusBadRequest, ErrRegcodeInvalid, "注册码无效、已用完或已过期")
		return
	}
	a.audit(r, "renew_account", "user", 0, map[string]any{"code": regCode})
	ok(w, "续期成功", map[string]any{"expire_status": expireStatus(u.ExpiredAt), "expired_at": publicExpiryUnix(u.ExpiredAt), "user": publicUser(u)})
}

func (a *App) handleQueueStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"status": "success", "pending": false, "terminal": true, "result": map[string]any{}})
}

func (a *App) handleRegisterBindCode(w http.ResponseWriter, r *http.Request, _ Params) {
	if !requireWebUIIntent(w, r, twilightIntentCreateBindCode) {
		return
	}
	if !a.telegramAvailable() {
		failWithCode(w, http.StatusServiceUnavailable, ErrTGNotConfigured, "Telegram Bot 未配置")
		return
	}
	if !a.allowRate(r.Context(), rateKey("register-bind-code:", a.clientIP(r)), a.cfg().RateLimitRegisterPer10m, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrBindCodeRateLimited, "绑定码请求过于频繁")
		return
	}
	a.createBindCode(w, 0, "register")
}

func (a *App) handleUserBindCode(w http.ResponseWriter, r *http.Request, _ Params) {
	if !requireWebUIIntent(w, r, twilightIntentCreateBindCode) {
		return
	}
	if !a.telegramAvailable() {
		failWithCode(w, http.StatusServiceUnavailable, ErrTGNotConfigured, "Telegram Bot 未配置")
		return
	}
	if !a.allowRate(r.Context(), rateKey("user-bind-code:", current(r).User.UID), a.cfg().RateLimitLoginPerMinute, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrBindCodeRateLimited, "绑定码请求过于频繁")
		return
	}
	a.createBindCode(w, current(r).User.UID, "user")
}

func (a *App) createBindCode(w http.ResponseWriter, uid int64, scene string) {
	a.cleanupExpiredBindCodes(time.Now().Unix())
	code := ""
	for attempt := 0; attempt < 20; attempt++ {
		candidate := strings.ToUpper(randomCode(6))
		if _, exists := a.bindCode(candidate); exists {
			continue
		}
		code = candidate
		break
	}
	if code == "" {
		failWithCode(w, http.StatusConflict, ErrBindCodeConflict, "绑定码生成冲突，请重试")
		return
	}
	now := time.Now().Unix()
	if err := a.upsertBindCode(store.BindCode{Code: code, Scene: scene, UID: uid, CreatedAt: now, ExpiresAt: now + 300}); err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrBindCodeSaveFailed, "绑定码保存失败，请稍后重试")
		return
	}
	ok(w, "OK", map[string]any{"bind_code": code, "expires_in": 300})
}

func (a *App) handleBindCodeStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	// Long-poll 支持：客户端传 wait=N（秒）表示愿意等待最多 N 秒。
	// 在等待期间每 500ms 检查一次 bind code 状态，一旦变为终态立即返回。
	// 不传 wait 或 wait<=0 时退化为即时响应（兼容旧客户端）。
	waitSec := clamp(queryInt(r, "wait", 0), 0, 60)

	respond := func() {
		state := a.telegramBindCodeState(code, 0, "register", time.Now().Unix(), true)
		writeTelegramBindCodeState(w, state)
	}

	// 即时模式
	if waitSec <= 0 {
		respond()
		return
	}

	// Long-poll 模式：先检查一次，如果已经是终态直接返回
	state := a.telegramBindCodeState(code, 0, "register", time.Now().Unix(), true)
	if state.Terminal {
		respond()
		return
	}

	// 挂起等待，每 500ms 轮询 store 直到终态或超时
	deadline := time.After(time.Duration(waitSec) * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-deadline:
			respond()
			return
		case <-ticker.C:
			state = a.telegramBindCodeState(code, 0, "register", time.Now().Unix(), true)
			if state.Terminal {
				respond()
				return
			}
		}
	}
}

func (a *App) handleBindCodeStatusWS(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.allowRate(r.Context(), rateKey("register-bind-status-ws:", a.clientIP(r)), max(30, a.cfg().RateLimitLoginPerMinute), time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "请求过于频繁，请稍后再试")
		return
	}
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	if !telegramBindCodePattern.MatchString(code) {
		failWithCode(w, http.StatusBadRequest, ErrTGBindCodeFormat, "Telegram 绑定码格式不正确")
		return
	}
	conn, err := acceptWebSocket(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	sendState := func(state telegramBindCodeState) bool {
		data := state.response()
		data["type"] = "status"
		payload, err := json.Marshal(data)
		if err != nil {
			return true
		}
		return writeWebSocketText(conn, payload) == nil
	}

	updates, unsubscribe := a.bindStatus.subscribe(code)
	defer unsubscribe()

	state := a.telegramBindCodeState(code, 0, "register", time.Now().Unix(), true)
	if !sendState(state) || state.Terminal {
		writeWebSocketClose(conn)
		return
	}

	expiryTimer := time.NewTimer(bindStateExpiryWait(state))
	defer expiryTimer.Stop()
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	resetExpiryTimer := func(next telegramBindCodeState) {
		if !expiryTimer.Stop() {
			select {
			case <-expiryTimer.C:
			default:
			}
		}
		expiryTimer.Reset(bindStateExpiryWait(next))
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-updates:
			state = a.telegramBindCodeState(code, 0, "register", time.Now().Unix(), true)
			if !sendState(state) || state.Terminal {
				writeWebSocketClose(conn)
				return
			}
			resetExpiryTimer(state)
		case <-expiryTimer.C:
			state = a.telegramBindCodeState(code, 0, "register", time.Now().Unix(), true)
			if !sendState(state) || state.Terminal {
				writeWebSocketClose(conn)
				return
			}
			resetExpiryTimer(state)
		case <-heartbeat.C:
			state = a.telegramBindCodeState(code, 0, "register", time.Now().Unix(), true)
			if !sendState(state) || state.Terminal {
				writeWebSocketClose(conn)
				return
			}
		}
	}
}

func (a *App) handleUserBindCodeStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	if !telegramBindCodePattern.MatchString(code) {
		failWithCode(w, http.StatusBadRequest, ErrTGBindCodeFormat, "Telegram 绑定码格式不正确")
		return
	}
	uid := current(r).User.UID
	waitSec := clamp(queryInt(r, "wait", 0), 0, 60)

	respond := func() {
		state := a.telegramBindCodeState(code, uid, "", time.Now().Unix(), true)
		writeTelegramBindCodeState(w, state)
	}

	if waitSec <= 0 {
		respond()
		return
	}

	state := a.telegramBindCodeState(code, uid, "", time.Now().Unix(), true)
	if state.Terminal {
		respond()
		return
	}

	deadline := time.After(time.Duration(waitSec) * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-deadline:
			respond()
			return
		case <-ticker.C:
			state = a.telegramBindCodeState(code, uid, "", time.Now().Unix(), true)
			if state.Terminal {
				respond()
				return
			}
		}
	}
}

func (a *App) handleUserBindCodeStatusWS(w http.ResponseWriter, r *http.Request, _ Params) {
	uid := current(r).User.UID
	if !a.allowRate(r.Context(), rateKey("user-bind-status-ws:", uid), 10, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "请求过于频繁，请稍后再试")
		return
	}
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	if !telegramBindCodePattern.MatchString(code) {
		failWithCode(w, http.StatusBadRequest, ErrTGBindCodeFormat, "Telegram 绑定码格式不正确")
		return
	}
	conn, err := acceptWebSocket(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	sendState := func(state telegramBindCodeState) bool {
		data := state.response()
		data["type"] = "status"
		payload, err := json.Marshal(data)
		if err != nil {
			return true
		}
		return writeWebSocketText(conn, payload) == nil
	}

	updates, unsubscribe := a.bindStatus.subscribe(code)
	defer unsubscribe()

	state := a.telegramBindCodeState(code, uid, "", time.Now().Unix(), true)
	if !sendState(state) || state.Terminal {
		writeWebSocketClose(conn)
		return
	}

	expiryTimer := time.NewTimer(bindStateExpiryWait(state))
	defer expiryTimer.Stop()
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	resetExpiryTimer := func(next telegramBindCodeState) {
		if !expiryTimer.Stop() {
			select {
			case <-expiryTimer.C:
			default:
			}
		}
		expiryTimer.Reset(bindStateExpiryWait(next))
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-updates:
			state = a.telegramBindCodeState(code, uid, "", time.Now().Unix(), true)
			if !sendState(state) || state.Terminal {
				writeWebSocketClose(conn)
				return
			}
			resetExpiryTimer(state)
		case <-expiryTimer.C:
			state = a.telegramBindCodeState(code, uid, "", time.Now().Unix(), true)
			if !sendState(state) || state.Terminal {
				writeWebSocketClose(conn)
				return
			}
			resetExpiryTimer(state)
		case <-heartbeat.C:
			state = a.telegramBindCodeState(code, uid, "", time.Now().Unix(), true)
			if !sendState(state) || state.Terminal {
				writeWebSocketClose(conn)
				return
			}
		}
	}
}

func bindStateExpiryWait(state telegramBindCodeState) time.Duration {
	if state.ExpiresIn <= 0 {
		return time.Second
	}
	return time.Duration(state.ExpiresIn)*time.Second + time.Second
}

func (a *App) handleRebindComplete(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if !p.User.RebindingInProgress {
		ok(w, "not in rebinding", nil)
		return
	}
	if p.User.TelegramID == 0 {
		failWithCode(w, http.StatusBadRequest, ErrTGNotBound, "尚未完成 Telegram 绑定")
		return
	}
	// 换绑完成后检查新 Telegram 账号是否在要求的群组/频道中
	if missing, err := a.telegramBindRequirementMissing(r.Context(), p.User.TelegramID); err != nil {
		_, _ = a.store().UpdateUser(p.User.UID, func(u *store.User) error {
			u.RebindingInProgress = true
			u.RebindingSince = time.Now().Unix()
			return nil
		})
		failWithCode(w, http.StatusForbidden, ErrTGBindGroupCheckFailed, "Telegram 账号未加入要求的群组/频道，换绑失败")
		return
	} else if len(missing) > 0 {
		_, _ = a.store().UpdateUser(p.User.UID, func(u *store.User) error {
			u.RebindingInProgress = true
			u.RebindingSince = time.Now().Unix()
			return nil
		})
		failWithCode(w, http.StatusForbidden, ErrTGBindGroupCheckFailed, "Telegram 账号未加入要求的群组/频道："+strings.Join(missing, "、"))
		return
	}
	u, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error {
		u.RebindingInProgress = false
		u.RebindingSince = 0
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	// 换绑完成后同步恢复 Emby 账号
	if u.EmbyID != "" {
		sideCtx, sideCancel := schedulerSideEffectContext(r.Context())
		if a.embyShouldEnableUser(u) {
			_ = a.embyApplyEnabledState(sideCtx, u.UID, u.EmbyID, true)
		}
		sideCancel()
	}
	ok(w, "rebinding complete", publicUser(u))
}

// telegramStatusFields 统一计算用户 Telegram 绑定 / 换绑状态，供
// handleTelegramStatus（/users/me/telegram，dashboard 用）与 handleUserSettings
// （/users/me/settings，设置页用）复用。历史上两处各自手算，导致
// can_change / can_unbind 在 "used"（换绑已消费）与管理员场景下口径不一致。
// can_unbind 口径必须与 handleUnbindTelegram 的服务端校验完全一致：换绑一律走
// 审批，非管理员仅在存在 approved 换绑请求时才可解绑（与 force_bind 无关），
// 否则只能走 can_change 提交换绑申请。管理员不受限。
func (a *App) telegramStatusFields(u store.User) map[string]any {
	forceBind := a.cfg().ForceBindTelegram
	admin := u.Role == store.RoleAdmin
	canUnbind := admin
	canChange := true
	pendingRebind := false
	rebindApproved := false
	var rebindStatus any
	var rebindID any
	if latestReq, hasReq := a.store().UserLatestRebindRequest(u.UID); hasReq {
		rebindStatus = latestReq.Status
		rebindID = latestReq.ID
		switch latestReq.Status {
		case "pending":
			pendingRebind = true
			if !admin {
				canChange = false
			}
		case "approved":
			rebindApproved = true
			canUnbind = true
		}
		// "used"（解绑已消费）/ "rejected"（被驳回）不再限制 canChange：用户应能
		// 重新生成绑定码 / 重新发起换绑请求。
	}
	return map[string]any{
		"bound":                  u.TelegramID != 0,
		"telegram_id":            nullableInt(u.TelegramID),
		"telegram_id_full":       nullableInt(u.TelegramID),
		"telegram_username":      u.TelegramUsername,
		"force_bind":             forceBind,
		"can_unbind":             canUnbind,
		"can_change":             canChange,
		"rebind_approved":        rebindApproved,
		"pending_rebind_request": pendingRebind,
		"rebind_request_status":  rebindStatus,
		"rebind_request_id":      rebindID,
		"rebinding_in_progress":  u.RebindingInProgress,
	}
}

func (a *App) handleTelegramStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", a.telegramStatusFields(current(r).User))
}

func (a *App) handleUnbindTelegram(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	// 换绑一律走审批：非管理员解绑必须先有一条 approved 的换绑申请，解绑时消费掉
	// （approved→used）。这条门禁与 force_bind_telegram 无关——即使关闭强制绑定，
	// 也不允许用户自助无限解绑/重绑（每次更换都需管理员批准一次，用过即作废）。
	// 管理员自身不受限。
	var consumeRebindID int64
	if p.User.Role != store.RoleAdmin {
		latestReq, hasReq := a.store().UserLatestRebindRequest(p.User.UID)
		if !hasReq || latestReq.Status != "approved" {
			failWithCode(w, http.StatusForbidden, ErrTGUnbindForbidden, "更换 Telegram 需要先提交换绑申请并经管理员批准")
			return
		}
		consumeRebindID = latestReq.ID
	}
	u, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error { u.TelegramID = 0; u.TelegramUsername = ""; return nil })
	if statusFromError(w, err) {
		return
	}
	// Mark approved rebind request as consumed so it cannot be reused.
	// 走 ConsumeRebindRequest 而非 ReviewRebindRequest：后者会把 ReviewerUID
	// 覆盖成 0、用 "auto-consumed" 抹掉管理员原始审核备注、并把 ReviewedAt
	// 重置为 now，等于销毁了"哪位管理员、何时、为何批准"的审计痕迹。
	// ConsumeRebindRequest 只把 Status 由 approved 翻成 used，保留审核元数据。
	if consumeRebindID > 0 {
		_ = a.store().ConsumeRebindRequest(consumeRebindID)
	}
	_, _ = a.store().UpdateUser(p.User.UID, func(u2 *store.User) error {
		if u2.Role != store.RoleAdmin {
			u2.RebindingInProgress = true
			u2.RebindingSince = time.Now().Unix()
		}
		return nil
	})
	// 解绑时同步禁用 Emby 账号，防止用户通过旧 TG 关联的 Emby 继续使用
	if u.EmbyID != "" {
		sideCtx, sideCancel := schedulerSideEffectContext(r.Context())
		_, _ = a.disableRemoteEmbyForWebState(sideCtx, u)
		sideCancel()
	}
	a.audit(r, "unbind_telegram", "user", 0, nil)
	ok(w, "Telegram unbound. rebinding required", publicUser(u))
}

func (a *App) handleTelegramRebindRequest(w http.ResponseWriter, r *http.Request, _ Params) {
	u := current(r).User
	if u.TelegramID == 0 {
		failWithCode(w, http.StatusBadRequest, ErrTGNotBound, "当前账号未绑定 Telegram")
		return
	}
	req, err := a.store().CreateRebindRequest(store.RebindRequest{UID: u.UID, Username: u.Username, OldTelegramID: u.TelegramID, Reason: truncateString(stringValue(decodeMap(r), "reason"), 500)})
	if statusFromError(w, err) {
		return
	}
	ok(w, "Telegram rebind request submitted", req)
}

func (a *App) handleUserSettings(w http.ResponseWriter, r *http.Request, _ Params) {
	u := current(r).User

	ok(w, "OK", map[string]any{
		"bgm_mode": u.BGMMode, "bgm_token_set": u.BGMToken != "", "api_key_enabled": u.LegacyAPIKeyStatus,
		"notify_on_login_telegram": u.NotifyOnLoginTelegram, "notify_on_login_email": u.NotifyOnLoginEmail, "notify_on_ticket_telegram": u.NotifyOnTicketTelegram,
		"signin_auto_renewal":                 u.SigninAutoRenewal,
		"password_change_email_required":      a.passwordChangeEmailRequired(u, emailPurposeChangePass),
		"emby_password_email_required":        a.passwordChangeEmailRequired(u, emailPurposeChangeEmby),
		"emby_password_old_password_required": u.RequireOldPasswordForEmbyPasswordChange,
		"password_change_email_forced":        a.emailGateActive(u),
		"emby_password_email_forced":          a.emailGateActive(u),
		// 与 /users/me/telegram 共用 telegramStatusFields，避免两个端点对换绑状态
		// 给出互相矛盾的 can_change / can_unbind。
		"telegram":      a.telegramStatusFields(u),
		"emby_status":   map[string]any{"is_synced": u.EmbyID != "", "is_active": u.Active, "can_unbind": a.userCanSelfUnbindEmby(u), "active_sessions": 0, "message": "OK"},
		"system_config": map[string]any{"device_limit_enabled": a.cfg().DeviceLimitEnabled, "max_devices": a.cfg().MaxDevices, "max_streams": a.cfg().MaxStreams, "bangumi_sync_enabled": a.cfg().BangumiEnabled, "bangumi_manage_enabled": a.cfg().BangumiManageEnabled},
	})
}

func (a *App) handleDevices(w http.ResponseWriter, r *http.Request, params Params) {
	// :uid 仅出现在 AuthAdmin 路由（/api/v1/security/users/:uid/devices）。
	// 走 requireAdminForUIDParam 把"caller 不是 admin 但路径带 :uid"路由错配
	// 的情况打回 403，避免 user A 走错路由就拿 user B 的设备列表。
	uid, written := requireAdminForUIDParam(w, r, params)
	if written {
		return
	}
	items := []map[string]any{}
	for _, d := range a.store().ListDevices(uid) {
		items = append(items, map[string]any{"device_id": d.DeviceID, "device_name": d.DeviceName, "client": d.Client, "last_ip": d.LastIP, "first_seen": d.FirstSeen, "last_seen": d.LastSeen, "is_trusted": d.Trusted})
	}
	ok(w, "OK", items)
}

func (a *App) handleSessions(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.embyConfigured() {
		ok(w, "OK", []any{})
		return
	}
	remote, err := a.embySessionsSnapshot(r.Context(), false)
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyRemoteSessionsFail, "failed to read Emby sessions")
		return
	}
	user := current(r).User
	adminView := strings.Contains(r.URL.Path, "/admin/")
	items := []map[string]any{}
	for _, session := range remote {
		// 非 admin 视图只能看到自己 Emby 账号的会话。注意必须用「EmbyID 不匹配」
		// 直接过滤，而不能加 `user.EmbyID != ""` 短路：未绑定 Emby 的普通用户
		// EmbyID 为空，一旦短路成立整段过滤失效，会把全站正在播放的用户名册和
		// telegram_id（敏感信息）泄露给任意登录用户。EmbyID 为空时下面的相等判断
		// 永不成立，自然返回空列表，正是期望行为。
		if !adminView && asString(session["UserId"]) != user.EmbyID {
			continue
		}
		nowPlaying := any(nil)
		if item, ok := session["NowPlayingItem"].(map[string]any); ok {
			nowPlaying = map[string]any{"id": item["Id"], "name": item["Name"], "type": item["Type"]}
		}
		local := any(nil)
		if u, okUser := a.store().FindUserByEmbyID(asString(session["UserId"])); okUser {
			local = map[string]any{"uid": u.UID, "username": u.Username, "telegram_id": nullableInt(u.TelegramID)}
		}
		items = append(items, map[string]any{
			"session_id":          asString(session["Id"]),
			"user_id":             asString(session["UserId"]),
			"user_name":           firstNonEmpty(asString(session["UserName"]), asString(session["UserName"])),
			"client":              asString(session["Client"]),
			"device_name":         asString(session["DeviceName"]),
			"device_id":           asString(session["DeviceId"]),
			"remote_endpoint":     asString(session["RemoteEndPoint"]),
			"application_version": asString(session["ApplicationVersion"]),
			"is_active":           boolish(session["IsActive"]),
			"now_playing":         nowPlaying,
			"local_user":          local,
		})
	}
	ok(w, "OK", items)
}

func (a *App) handleLoginHistory(w http.ResponseWriter, r *http.Request, params Params) {
	// 路由表把 /security/login-history（AuthUser）和 /security/login-history/:uid
	// （AuthAdmin）挂到同 handler。原实现完全靠 splitPath + Contains 推断 uid，
	// 字符串变化或路由重排都会让鉴权静默失效。改为读 params["uid"]，并显式断
	// 言"路径带 :uid 必须是 admin"。
	uid, written := requireAdminForUIDParam(w, r, params)
	if written {
		return
	}
	limit := clamp(queryInt(r, "limit", 50), 1, 100)
	logs := a.store().LoginHistory(uid, false, 0, limit)
	items := make([]map[string]any, 0, len(logs))
	for _, log := range logs {
		items = append(items, map[string]any{"id": log.ID, "ip": log.IP, "device": log.DeviceName, "client": log.Client, "time": log.Time, "blocked": log.Blocked, "country": log.Country, "city": log.City})
	}
	ok(w, "OK", map[string]any{"records": items, "total": len(items)})
}

func (a *App) handleBlockDevice(w http.ResponseWriter, r *http.Request, params Params) {
	// :uid 只能来自 AuthAdmin 路由；若 AuthUser 路由表手抖加上 :uid，下面这条
	// 断言会立刻 403，避免 user A 用伪造路径 block user B 的设备。
	uid, written := requireAdminForUIDParam(w, r, params)
	if written {
		return
	}
	deviceID := params["device_id"]
	if deviceID == "" {
		failWithCode(w, http.StatusBadRequest, ErrDeviceIDRequired, "设备 ID 不能为空")
		return
	}
	if err := a.store().UpdateDevice(uid, deviceID, func(d *store.Device) { d.Blocked = true; d.Trusted = false }); statusFromError(w, err) {
		return
	}
	a.audit(r, "block_device", auditCategoryForRole(current(r).User.Role), uid, map[string]any{"device_id": deviceID})
	ok(w, "device blocked", nil)
}

func (a *App) handleTrustDevice(w http.ResponseWriter, r *http.Request, params Params) {
	uid := current(r).User.UID
	deviceID := params["device_id"]
	if err := a.store().UpdateDevice(uid, deviceID, func(d *store.Device) { d.Trusted = true; d.Blocked = false }); statusFromError(w, err) {
		return
	}
	a.audit(r, "trust_device", auditCategoryForRole(current(r).User.Role), uid, map[string]any{"device_id": deviceID})
	ok(w, "device trusted", nil)
}

func (a *App) handleDeleteDevice(w http.ResponseWriter, r *http.Request, params Params) {
	deviceID := params["device_id"]
	if deviceID == "" {
		failWithCode(w, http.StatusBadRequest, ErrDeviceIDRequired, "设备 ID 不能为空")
		return
	}
	uid := current(r).User.UID
	if err := a.store().DeleteDevice(uid, deviceID); statusFromError(w, err) {
		return
	}
	a.audit(r, "delete_device", auditCategoryForRole(current(r).User.Role), uid, map[string]any{"device_id": deviceID})
	ok(w, "device removed", nil)
}

func (a *App) handleIPBlacklist(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", a.store().ListIPBlacklist())
}

func (a *App) handleAddIPBlacklist(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	ip := stringValue(payload, "ip")
	if ip == "" {
		failWithCode(w, http.StatusBadRequest, ErrIPRequired, "IP 不能为空")
		return
	}
	// hours 上限：10 年。time.Duration 是 int64 纳秒，hours * time.Hour 在 hours
	// 接近 math.MaxInt32 时会整数溢出，得到一个绕到过去的 expireAt（负数）。
	// admin 误填或 admin 凭据被盗时可借此构造"永久封禁"或"立即解封"的歧义状态，
	// 把 IP 黑名单做成不可信视图。-1 仍按"永久"语义保留，超过 87600 直接拒。
	const maxBlacklistHours = 24 * 365 * 10
	hours := intValue(payload, "hours", -1)
	if hours > maxBlacklistHours {
		failWithCode(w, http.StatusBadRequest, ErrIPBlacklistDurationInvalid, "封禁时长超出允许范围")
		return
	}
	if hours == 0 || (hours < 0 && hours != -1) {
		failWithCode(w, http.StatusBadRequest, ErrIPBlacklistDurationInvalid, "封禁时长非法")
		return
	}
	expireAt := int64(-1)
	if hours > 0 {
		expireAt = time.Now().Add(time.Duration(hours) * time.Hour).Unix()
	}
	reason := stringValue(payload, "reason")
	if err := a.store().AddIPBlacklist(ip, reason, expireAt); statusFromError(w, err) {
		return
	}
	a.audit(r, "add_ip_blacklist", "admin", 0, map[string]any{"ip": ip, "expire_at": expireAt, "reason": reason})
	ok(w, "IP 已加入黑名单", nil)
}

func (a *App) handleDeleteIPBlacklist(w http.ResponseWriter, r *http.Request, _ Params) {
	ip := stringValue(decodeMap(r), "ip")
	if ip == "" {
		failWithCode(w, http.StatusBadRequest, ErrIPRequired, "IP 不能为空")
		return
	}
	if err := a.store().RemoveIPBlacklist(ip); statusFromError(w, err) {
		return
	}
	a.audit(r, "delete_ip_blacklist", "admin", 0, map[string]any{"ip": ip})
	ok(w, "IP 已移出黑名单", nil)
}

func (a *App) handleSuspicious(w http.ResponseWriter, r *http.Request, _ Params) {
	hours := queryInt(r, "hours", 24)
	logs := a.store().LoginHistory(0, true, time.Now().Add(-time.Duration(hours)*time.Hour).Unix(), 100)
	items := make([]map[string]any, 0, len(logs))
	for _, log := range logs {
		items = append(items, map[string]any{"uid": log.UID, "ip": log.IP, "device": log.DeviceName, "time": log.Time, "reason": firstNonEmpty(log.Reason, "blocked")})
	}
	ok(w, "OK", items)
}

func (a *App) handleSystemInfo(w http.ResponseWriter, r *http.Request, _ Params) {
	cfg := a.cfg()
	if !a.allowRate(r.Context(), rateKey("system-info:", a.clientIP(r)), 30, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "请求过于频繁")
		return
	}
	ok(w, "OK", map[string]any{
		"name":                cfg.AppName,
		"icon":                a.publicServerIconURL(),
		"version":             cfg.Version,
		"api_version":         "v1",
		"session_cookie_name": cfg.SessionCookie,
		"features": map[string]any{
			"register":                      cfg.RegisterEnabled,
			"emby_direct_register":          cfg.EmbyDirectRegisterEnabled,
			"telegram":                      cfg.TelegramMode,
			"force_bind_telegram":           cfg.ForceBindTelegram,
			"force_bind_group":              cfg.TelegramForceBindGroup,
			"force_bind_channel":            cfg.TelegramForceBindChannel,
			"bangumi_sync":                  cfg.BangumiEnabled,
			"bangumi_manage":                cfg.BangumiManageEnabled,
			"media_request":                 cfg.MediaRequestEnabled,
			"signin":                        cfg.SigninEnabled,
			"invite":                        cfg.InviteEnabled,
			"email_enabled":                 emailConfigured(cfg),
			"force_bind_email":              cfg.EmailForceBind,
			"forgot_password_enabled":       cfg.ForgotPasswordEnabled,
			"forgot_password_emby_enabled":  cfg.ForgotPasswordEmbyEnabled,
			"forgot_password_email_enabled": cfg.ForgotPasswordEmailEnabled,
			"ticket_system":                 cfg.TicketSystemEnabled,
			"developer_mode":                a.store().DeveloperModeEnabled(),
			"emby_stats":                    cfg.EmbyStatsEnabled,
		},
		"auth_background_url": cfg.AuthBackgroundURL,
		"limits": map[string]any{
			"user_limit":             cfg.UserLimit,
			"stream_limit":           cfg.MaxStreams,
			"ticket_image_max_size":  cfg.TicketImageMaxSize,
			"ticket_image_max_count": cfg.TicketImageMaxCount,
		},
		"telegram_bot":   a.publicTelegramBotInfo(r.Context()),
		"setup":          a.setupStatusData(),
		"telegram_links": publicTelegramLinks(cfg.TelegramGroupIDs, cfg.TelegramChannelIDs),
		"required_telegram_links": publicTelegramLinks(
			requiredTelegramLinkIDs(cfg.TelegramGroupIDs, cfg.TelegramForceBindGroup),
			requiredTelegramLinkIDs(cfg.TelegramChannelIDs, cfg.TelegramForceBindChannel),
		),
		"telegram_mode":        cfg.TelegramMode,
		"bangumi_sync_enabled": cfg.BangumiEnabled,
		"storage_mismatch":     a.runtimeDatabaseMismatch(),
		"storage_warning":      a.databaseMismatchWarning(),
	})
}

func (a *App) publicTelegramBotInfo(ctx context.Context) map[string]any {
	empty := map[string]any{"username": nil, "url": nil, "enabled": a.cfg().TelegramMode, "configured": strings.TrimSpace(a.cfg().TelegramBotToken) != "", "ok": false, "error": ""}
	if !a.telegramAvailable() {
		empty["error"] = "Telegram 未启用或未配置 Bot Token"
		return empty
	}
	token := strings.TrimSpace(a.cfg().TelegramBotToken)
	now := time.Now()
	a.telegramBotMu.Lock()
	if a.telegramBotCacheToken == token && now.Before(a.telegramBotCacheUntil) && a.telegramBotCache != nil {
		cached := cloneMap(a.telegramBotCache)
		a.telegramBotMu.Unlock()
		return cached
	}
	a.telegramBotMu.Unlock()

	lookupCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	me, err := a.telegramGetMe(lookupCtx)
	bot := empty
	if err == nil {
		username := strings.TrimPrefix(asString(me["username"]), "@")
		if telegramPublicUsernamePattern.MatchString(username) {
			bot = map[string]any{"username": username, "url": "https://t.me/" + username, "enabled": a.cfg().TelegramMode, "configured": true, "ok": true, "error": ""}
		}
	} else {
		bot["error"] = err.Error()
	}

	a.telegramBotMu.Lock()
	a.telegramBotCacheToken = token
	if err == nil {
		a.telegramBotCacheUntil = now.Add(10 * time.Minute)
	} else {
		a.telegramBotCacheUntil = now.Add(30 * time.Second)
	}
	a.telegramBotCache = cloneMap(bot)
	a.telegramBotMu.Unlock()
	return bot
}

func publicTelegramLinks(groupIDs, channelIDs []string) map[string]any {
	return map[string]any{
		"groups":   publicTelegramLinkList(groupIDs),
		"channels": publicTelegramLinkList(channelIDs),
	}
}

func requiredTelegramLinkIDs(ids []string, required bool) []string {
	if !required {
		return nil
	}
	return ids
}

func publicTelegramLinkList(values []string) []map[string]string {
	out := []map[string]string{}
	seen := map[string]bool{}
	for _, value := range values {
		item, ok := publicTelegramLink(value)
		if !ok || seen[item["url"]] {
			continue
		}
		seen[item["url"]] = true
		out = append(out, item)
	}
	return out
}

func publicTelegramLink(raw string) (map[string]string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" || strings.HasPrefix(value, "-") || strings.ContainsAny(value, "\x00\r\n\t ") {
		return nil, false
	}
	if strings.HasPrefix(value, "@") {
		username := strings.TrimPrefix(value, "@")
		if telegramPublicUsernamePattern.MatchString(username) {
			return map[string]string{"label": "@" + username, "url": "https://t.me/" + username}, true
		}
		return nil, false
	}
	if strings.HasPrefix(strings.ToLower(value), "t.me/") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err == nil && parsed.Scheme != "" {
		if parsed.Scheme != "https" {
			return nil, false
		}
		host := strings.ToLower(parsed.Hostname())
		if host != "t.me" && host != "telegram.me" {
			return nil, false
		}
		username := strings.Trim(strings.TrimPrefix(parsed.Path, "/"), "/")
		if !telegramPublicUsernamePattern.MatchString(username) {
			return nil, false
		}
		cleanURL := "https://t.me/" + username
		return map[string]string{"label": "@" + username, "url": cleanURL}, true
	}
	if telegramPublicUsernamePattern.MatchString(value) {
		return map[string]string{"label": "@" + value, "url": "https://t.me/" + value}, true
	}
	return nil, false
}

func (a *App) handleServerIcon(w http.ResponseWriter, r *http.Request, _ Params) {
	iconPath, contentType, okIcon := a.configuredServerIconPath()
	if okIcon {
		file, err := os.Open(iconPath)
		if err == nil {
			defer file.Close()
			if info, statErr := file.Stat(); statErr == nil {
				w.Header().Set("Content-Type", contentType)
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				http.ServeContent(w, r, info.Name(), info.ModTime(), file)
				return
			}
			w.Header().Set("Content-Type", contentType)
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			_, _ = io.Copy(w, file)
			return
		}
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(serverIconPNG)
}

func (a *App) publicServerIconURL() string {
	value := strings.TrimSpace(a.cfg().ServerIcon)
	if value == "" {
		return "/api/v1/system/server-icon"
	}
	if u, err := url.Parse(value); err == nil && u.Scheme != "" {
		if u.Scheme == "https" && u.User == nil && u.Hostname() != "" {
			return value
		}
		return "/api/v1/system/server-icon"
	}
	if iconPath, _, okIcon := a.configuredServerIconPath(); okIcon {
		if info, err := os.Stat(iconPath); err == nil {
			return "/api/v1/system/server-icon?v=" + strconv.FormatInt(info.ModTime().UnixNano(), 36) + "-" + strconv.FormatInt(info.Size(), 36)
		}
	}
	return "/api/v1/system/server-icon"
}

func (a *App) configuredServerIconPath() (string, string, bool) {
	value := strings.TrimSpace(a.cfg().ServerIcon)
	if value == "" {
		return "", "", false
	}
	if u, err := url.Parse(value); err == nil && u.Scheme != "" {
		return "", "", false
	}
	ext := strings.ToLower(filepath.Ext(value))
	contentTypes := map[string]string{
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".webp": "image/webp",
		".gif":  "image/gif",
		".bmp":  "image/bmp",
		".ico":  "image/x-icon",
	}
	contentType, ok := contentTypes[ext]
	if !ok {
		return "", "", false
	}
	// 必须经过 ResolveWithinRoot 约束在上传目录内：server_icon 是管理员可写的
	// 配置项，而 /api/v1/system/server-icon 是 AuthPublic。若直接接受绝对路径或
	// 含 ".." 的相对路径，一次"管理员写配置"就会变成"任意人读主机任意图片扩展名
	// 文件"（也可经 handleConfigRestore 用构造的备份触发）。绝对路径不再被接受。
	path, err := ResolveWithinRoot(firstNonEmpty(a.cfg().UploadDir, "uploads"), value)
	if err != nil {
		return "", "", false
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > 2*1024*1024 {
		return "", "", false
	}
	return path, contentType, true
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.allowRate(r.Context(), rateKey("health:", a.clientIP(r)), 60, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "请求过于频繁")
		return
	}
	isAuth := current(r).User.UID != 0
	api := a.apiHealth()
	database := a.databaseHealth(r.Context())
	emby := a.embyStatusSnapshot(r.Context(), false)
	data := map[string]any{
		"status": "healthy",
		"time":   time.Now().Unix(),
		"api":    boolValue(api, "ok", false),
	}
	if isAuth {
		sessionFallback := a.sessions().FallbackCount()
		rateFallback := a.limiter().FallbackCount()
		data["api_detail"] = api
		data["database"] = boolValue(database, "ok", false)
		data["database_detail"] = database
		data["emby"] = boolValue(emby, "online", false)
		data["emby_detail"] = emby
		data["emby_configured"] = boolValue(emby, "configured", false)
		data["redis"] = a.redis() != nil
		data["redis_degraded"] = a.redis() != nil && (sessionFallback > 0 || rateFallback > 0)
		data["storage"] = a.store().Backend()
		data["active_database"] = a.store().Backend()
		data["config_database"] = strings.ToLower(a.cfg().DatabaseDriver)
		data["storage_mismatch"] = a.runtimeDatabaseMismatch()
		data["storage_warning"] = a.databaseMismatchWarning()
	}
	ok(w, "OK", data)
}

func (a *App) apiHealth() map[string]any {
	return map[string]any{
		"ok":        true,
		"status":    "online",
		"routes":    len(a.routes),
		"uptime":    int64(time.Since(runtimeStartedAt).Seconds()),
		"timestamp": time.Now().Unix(),
	}
}

func (a *App) handleHealthAPI(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", a.apiHealth())
}

func (a *App) handleHealthDatabase(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", a.databaseHealth(r.Context()))
}

func (a *App) handleHealthEmby(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", a.embyStatusSnapshot(r.Context(), true))
}

func (a *App) handleSystemStats(w http.ResponseWriter, r *http.Request, _ Params) {
	users := a.store().ListUsers()
	activeUsers := countActive(users)
	regcodes := a.store().ListRegCodes()
	activeRegcodes := 0
	for _, code := range regcodes {
		if code.Active {
			activeRegcodes++
		}
	}
	usage := 0
	if a.cfg().UserLimit > 0 {
		usage = int(float64(len(users)) / float64(a.cfg().UserLimit) * 100)
	}
	ok(w, "OK", map[string]any{
		"timestamp":     time.Now().Unix(),
		"cpu_count":     nil,
		"users":         map[string]any{"active": activeUsers, "total": len(users), "limit": zeroNil(int64(a.cfg().UserLimit)), "usage_percent": usage},
		"regcodes":      map[string]any{"active": activeRegcodes, "total": len(regcodes)},
		"total_users":   len(users),
		"active_users":  activeUsers,
		"redis_enabled": a.redis() != nil,
		"redis_fallback": map[string]any{
			"session": a.sessions().FallbackCount(),
			"rate":    a.limiter().FallbackCount(),
		},
		"routes": len(a.routes),
		"uptime": int64(time.Since(runtimeStartedAt).Seconds()),
	})
}

func (a *App) databaseHealth(parent context.Context) map[string]any {
	st := a.store()
	if st == nil {
		return map[string]any{
			"ok":                false,
			"backend":           "none",
			"configured_driver": strings.ToLower(a.cfg().DatabaseDriver),
			"error":             "store is not initialized",
		}
	}
	backend := st.Backend()
	userCount := st.UserCount()
	result := map[string]any{
		"ok":                true,
		"backend":           backend,
		"configured_driver": strings.ToLower(a.cfg().DatabaseDriver),
		"storage_mismatch":  a.runtimeDatabaseMismatch(),
		"storage_warning":   a.databaseMismatchWarning(),
		"state_read_ok":     true,
		"user_count":        userCount,
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	if db := st.DB(); db != nil || backend == store.BackendPostgres {
		if db == nil {
			result["ok"] = false
			result["error"] = "postgres backend has no active connection"
			return result
		}
		if err := db.PingContext(ctx); err != nil {
			result["ping_ok"] = false
			result["ping_error"] = truncateString(redactSensitiveText(err.Error()), 180)
			result["warning"] = "database ping failed; active store remains readable"
			return result
		}
		result["ping_ok"] = true
		stats := db.Stats()
		result["open_connections"] = stats.OpenConnections
		result["in_use"] = stats.InUse
		result["idle"] = stats.Idle
		return result
	}
	if _, err := st.Snapshot(); err != nil {
		result["ok"] = false
		result["error"] = "state snapshot failed"
	}
	return result
}

func (a *App) embyOverview(ctx context.Context) map[string]any {
	return a.embyStatusSnapshot(ctx, true)
}

func (a *App) embyStatusSnapshot(parent context.Context, includeSessions bool) map[string]any {
	result := map[string]any{
		"online":          false,
		"configured":      a.embyConfigured(),
		"server":          a.cfg().EmbyURL,
		"active_sessions": 0,
		"total_sessions":  0,
	}
	if !a.embyConfigured() {
		result["status"] = "not_configured"
		return result
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	info, err := a.embyHealthDetailed(ctx)
	cancel()
	if err != nil {
		result["status"] = "unreachable"
		result["error"] = "Emby status request failed"
		result["error_detail"] = truncateString(redactSensitiveText(err.Error()), 180)
		return result
	}
	if info == nil {
		info = map[string]any{}
	}
	result["online"] = true
	result["status"] = "online"
	result["server_name"] = firstNonEmpty(asString(info["ServerName"]), asString(info["Name"]))
	result["version"] = firstNonEmpty(asString(info["Version"]), "unknown")
	result["operating_system"] = asString(info["OperatingSystem"])
	if includeSessions {
		sessionCtx, sessionCancel := context.WithTimeout(parent, 5*time.Second)
		sessions, sessionErr := a.embySessionsSnapshot(sessionCtx, false)
		sessionCancel()
		if sessionErr == nil {
			result["active_sessions"] = countEmbyPlayingSessions(sessions)
			result["total_sessions"] = len(sessions)
		} else {
			result["sessions_error"] = "Emby sessions request failed"
			result["sessions_error_detail"] = truncateString(redactSensitiveText(sessionErr.Error()), 180)
		}
	}
	return result
}
func (a *App) handleEmbyURLs(w http.ResponseWriter, r *http.Request, _ Params) {
	u := current(r).User
	// No Emby account and not pending: hide URLs
	if u.Role == store.RoleNormal && u.EmbyID == "" && !u.PendingEmby {
		ok(w, "OK", map[string]any{"lines": []any{}, "whitelist_lines": []any{}, "requires_emby_account": true, "requires_renewal": false, "emby_disabled_by_expiry": false})
		return
	}
	// Expired normal user: hide URLs regardless of whether account is active
	if u.Role == store.RoleNormal && u.ExpiredAt > 0 && u.ExpiredAt < time.Now().Unix() {
		ok(w, "OK", map[string]any{"lines": []any{}, "whitelist_lines": []any{}, "requires_emby_account": false, "requires_renewal": true, "emby_disabled_by_expiry": true})
		return
	}
	// Disabled account (non-admin): hide URLs
	if u.Role == store.RoleNormal && !u.Active {
		ok(w, "OK", map[string]any{"lines": []any{}, "whitelist_lines": []any{}, "requires_emby_account": false, "requires_renewal": false, "emby_disabled_by_expiry": false})
		return
	}
	lines := []map[string]string{}
	for _, line := range a.cfg().EmbyURLList {
		lines = append(lines, map[string]string{"name": line.Name, "url": line.URL})
	}
	if a.cfg().EmbyPublicURL != "" {
		lines = append(lines, map[string]string{"name": "默认线路", "url": a.cfg().EmbyPublicURL})
	}
	whitelist := []map[string]string{}
	if u.Role == store.RoleAdmin || u.Role == store.RoleWhitelist {
		for _, line := range a.cfg().EmbyWhitelistURLList {
			whitelist = append(whitelist, map[string]string{"name": line.Name, "url": line.URL})
		}
		if a.cfg().EmbyWhitelistURL != "" {
			whitelist = append(whitelist, map[string]string{"name": "whitelist route", "url": a.cfg().EmbyWhitelistURL})
		}
	}
	ok(w, "OK", map[string]any{"lines": lines, "whitelist_lines": whitelist, "requires_emby_account": false, "requires_renewal": false, "emby_disabled_by_expiry": false})
}

func (a *App) handlePublicConfig(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{
		"upload_limit":          a.cfg().MaxUploadSize,
		"bangumi_sync_enabled":  a.cfg().BangumiEnabled,
		"telegram_mode":         a.cfg().TelegramMode,
		"media_request_enabled": a.cfg().MediaRequestEnabled,
		"signin_enabled":        a.cfg().SigninEnabled,
		"invite_enabled":        a.cfg().InviteEnabled,
		"device_limit":          map[string]any{"enabled": a.cfg().DeviceLimitEnabled, "max_devices": a.cfg().MaxDevices, "max_streams": a.cfg().MaxStreams},
		"bangumi_sync":          map[string]any{"enabled": a.cfg().BangumiEnabled},
		"media_request": map[string]any{
			"enabled":                 a.cfg().MediaRequestEnabled,
			"max_concurrent_per_user": a.cfg().MaxConcurrentRequestsPerUser,
			"max_concurrent_global":   a.cfg().MaxConcurrentRequestsGlobal,
		},
		"signin": signinConfigPayload(*a.cfg()),
		"invite": map[string]any{"enabled": a.cfg().InviteEnabled},
		"email":  map[string]any{"enabled": emailConfigured(a.cfg()), "force_bind": a.cfg().EmailForceBind},
	})
}

func (a *App) handleAdminConfig(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"host": a.cfg().Host, "port": a.cfg().Port, "redis_enabled": a.redis() != nil, "state_file": a.cfg().StateFile, "upload_dir": a.cfg().UploadDir})
}

func (a *App) handleConfigTOMLGet(w http.ResponseWriter, r *http.Request, _ Params) {
	path := a.configFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		failWithCode(w, http.StatusNotFound, ErrConfigFileNotFound, "config file not found")
		return
	}
	// 密钥遮蔽：raw TOML GET 与 schema GET 必须同口径屏蔽真实密钥，否则本接口
	// 会成为绕过 schema 遮蔽、把全部密钥（Postgres DSN、Emby Token、Bot Token、
	// BotInternalSecret、Webhook Secret 等）泄露到浏览器 DOM/缓存/历史的旁路。
	//   - content（规范化渲染）：先在 values 上 maskConfigSecrets 再 render；
	//   - raw_content（磁盘原文）：按 section 上下文做行级 maskTOMLSecrets。
	// 两侧用同一哨兵，completed 比较仍对非密钥字段有效。PUT 路径
	// （handleConfigTOMLPutSafe）会把回传的哨兵还原为真实值，避免写盘覆盖。
	maskedValues := configValues(*a.cfg())
	maskConfigSecrets(maskedValues)
	normalizedContent := stripProtectedAdminConfig(renderConfigTOML(maskedValues))
	rawContent := stripProtectedAdminConfig(maskTOMLSecrets(string(data)))
	ok(w, "OK", map[string]any{"content": normalizedContent, "raw_content": rawContent, "path": path, "completed": normalizedContent != rawContent})
}

func (a *App) handleConfigTOMLPut(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleConfigTOMLPutSafe(w, r, nil)
}

func (a *App) handleConfigSchema(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleConfigSchemaFull(w, r, nil)
}

func (a *App) handleConfigSchemaUpdate(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleConfigSchemaUpdateSafe(w, r, nil)
}

func (a *App) handleConfigSweep(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "config check completed", map[string]any{"changed": false, "config_file": a.cfg().ConfigFile})
}

func (a *App) handleAPIRoutes(w http.ResponseWriter, r *http.Request, _ Params) {
	items := make([]map[string]string, 0, len(a.routes))
	for _, route := range a.routes {
		items = append(items, map[string]string{"method": route.Method, "path": strings.TrimPrefix(route.Pattern, "/api/v1"), "endpoint": route.Pattern, "full_path": route.Pattern})
	}
	ok(w, "OK", map[string]any{"apis": items, "total": len(items)})
}

func (a *App) handleBotTest(w http.ResponseWriter, r *http.Request, _ Params) {
	results := []map[string]any{}
	if !a.cfg().TelegramMode {
		results = append(results, map[string]any{"target": "配置", "success": false, "error": "telegram_mode 未启用"})
		ok(w, "测试完成", map[string]any{"results": results, "runtime": a.telegramRuntimeStatus()})
		return
	}
	if strings.TrimSpace(a.cfg().TelegramBotToken) == "" {
		results = append(results, map[string]any{"target": "Bot Token", "success": false, "error": "未配置 Telegram Bot Token"})
		ok(w, "测试完成", map[string]any{"results": results, "runtime": a.telegramRuntimeStatus()})
		return
	}
	testCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	me, err := a.telegramGetMe(testCtx)
	if err != nil {
		results = append(results, map[string]any{"target": "Bot getMe", "success": false, "error": err.Error()})
		ok(w, "测试完成", map[string]any{"results": results, "runtime": a.telegramRuntimeStatus()})
		return
	}
	botID := int64(numeric(me["id"]))
	username := strings.TrimPrefix(asString(me["username"]), "@")
	results = append(results, map[string]any{"target": "Bot getMe", "success": true, "username": username, "bot_id": zeroNil(botID)})
	for _, chatID := range telegramChatIDs(a.cfg().TelegramGroupIDs) {
		var chat map[string]any
		err := a.telegramPost(testCtx, "getChat", map[string]any{"chat_id": chatID}, &chat)
		item := map[string]any{"target": " 群组 " + chatID, "success": err == nil}
		if err != nil {
			item["error"] = err.Error()
		} else {
			item["title"] = firstNonEmpty(asString(chat["title"]), asString(chat["username"]))
			if botID != 0 {
				if member, memberErr := a.telegramGetChatMember(testCtx, chatID, botID); memberErr != nil {
					item["success"] = false
					item["error"] = memberErr.Error()
				} else {
					item["bot_status"] = asString(member["status"])
				}
			}
		}
		results = append(results, item)
	}
	for _, chatID := range telegramChatIDs(a.cfg().TelegramChannelIDs) {
		var chat map[string]any
		err := a.telegramPost(testCtx, "getChat", map[string]any{"chat_id": chatID}, &chat)
		item := map[string]any{"target": "频道 " + chatID, "success": err == nil}
		if err != nil {
			item["error"] = err.Error()
		} else {
			item["title"] = firstNonEmpty(asString(chat["title"]), asString(chat["username"]))
		}
		results = append(results, item)
	}
	ok(w, "测试完成", map[string]any{"results": results, "runtime": a.telegramRuntimeStatus()})
}

func (a *App) handleEmbyStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	u := current(r).User
	payload := a.embyStatusSnapshot(r.Context(), true)
	payload["is_synced"] = u.EmbyID != ""
	payload["is_active"] = u.Active
	payload["can_unbind"] = a.userCanSelfUnbindEmby(u)
	payload["message"] = "OK"
	if boolValue(payload, "online", false) {
		libCounts := a.embyLibraryCounts(r.Context())
		if libCounts != nil {
			payload["movie_count"] = libCounts["movies"]
			payload["series_count"] = libCounts["series"]
			payload["episode_count"] = libCounts["episodes"]
		}
	}
	ok(w, "OK", payload)
}

func (a *App) embyLibraryCounts(ctx context.Context) map[string]int {
	if !a.embyConfigured() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	var payload struct {
		MovieCount   int `json:"MovieCount"`
		SeriesCount  int `json:"SeriesCount"`
		EpisodeCount int `json:"EpisodeCount"`
	}
	if err := a.embyGet(ctx, "/Items/Counts", &payload); err != nil {
		return nil
	}
	return map[string]int{
		"movies":   max(payload.MovieCount, 0),
		"series":   max(payload.SeriesCount, 0),
		"episodes": max(payload.EpisodeCount, 0),
	}
}
func (a *App) handleDeprecatedEmbyURLs(w http.ResponseWriter, r *http.Request, _ Params) {
	failWithCode(w, http.StatusGone, ErrAPIDeprecated, "该接口已废弃，请使用 /api/v1/system/emby-urls")
}

func (a *App) handleEmbyLatest(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.embyConfigured() {
		ok(w, "OK", map[string]any{"items": []any{}, "total": 0})
		return
	}
	limit := clamp(queryInt(r, "limit", 20), 1, 100)
	itemTypes := firstNonEmpty(r.URL.Query().Get("item_types"), "Movie,Series")
	query := embyItemQuery(map[string]string{
		"IncludeItemTypes": itemTypes,
		"Recursive":        "true",
		"SortBy":           "DateCreated",
		"SortOrder":        "Descending",
		"Limit":            strconv.Itoa(limit),
		"Fields":           "Overview,ProviderIds,DateCreated,ProductionYear,PremiereDate",
	})
	var payload map[string]any
	if err := a.embyGet(r.Context(), "/Items"+query, &payload); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyLatestFailed, "failed to read latest Emby media")
		return
	}
	items, _ := payload["Items"].([]any)
	ok(w, "OK", map[string]any{"items": items, "total": int(numeric(payload["TotalRecordCount"]))})
}

func (a *App) handleSessionCount(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.embyConfigured() {
		ok(w, "OK", map[string]any{"active": 0, "total": 0})
		return
	}
	sessions, err := a.embySessionsSnapshot(r.Context(), false)
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyRemoteSessionsFail, "failed to read Emby sessions")
		return
	}
	ok(w, "OK", map[string]any{"active": countEmbyPlayingSessions(sessions), "total": len(sessions)})
}

// csvSafe 中和 CSV 公式注入：Excel / WPS / LibreOffice 会把以 = + - @ 开头
// （或以 Tab / CR / LF 起始）的单元格当作公式求值，恶意用户名 / 媒体标题
// 形如 "=HYPERLINK(...)"、"=1+2"、"+cmd" 在管理员打开导出文件时会被执行或
// 用于钓鱼。前置一个单引号 ' 让电子表格按文本处理，不影响 CSV 数据语义。
// 空串与普通文本原样返回。
func csvSafe(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r', '\n':
		return "'" + value
	}
	return value
}

func (a *App) handleExportUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=users.csv")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"uid", "username", "email", "role", "active"})
	for _, u := range a.store().ListUsers() {
		_ = cw.Write([]string{strconv.FormatInt(u.UID, 10), csvSafe(u.Username), csvSafe(u.Email), strconv.Itoa(u.Role), strconv.FormatBool(u.Active)})
	}
	cw.Flush()
}

func (a *App) handleExpiringUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	days := queryInt(r, "days", 3)
	deadline := time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
	now := time.Now().Unix()
	items := []map[string]any{}
	for _, u := range a.store().ListUsers() {
		if u.ExpiredAt > now && u.ExpiredAt <= deadline {
			remaining := u.ExpiredAt - now
			items = append(items, map[string]any{"uid": u.UID, "username": u.Username, "telegram_id": nullableInt(u.TelegramID), "expired_at": u.ExpiredAt, "remaining_seconds": remaining, "remaining_str": formatSeconds(remaining)})
		}
	}
	ok(w, "OK", map[string]any{"days": days, "count": len(items), "users": items})
}

// randomCode 生成 hex 编码的随机字符串，用于 API key、绑定码、上传文件名、
// 临时密码等所有"必须不可预测"的场景。
// 安全性约束：crypto/rand 故障时**绝不**回退到
// time.Now().UnixNano() — 那会让攻击者用本机时钟近似猜出 token，等同于
// 给 API key / 密码出一个可预测后门。这里直接 panic：HTTP 路径会被 app.go
// 的 recover 中间件兜成 500（更符合 fail-closed 原则）；telegram bot 和
// scheduler daemon 的入口都已加 defer recover，单条任务失败不会拖垮进程。
// crypto/rand 在 Linux/macOS/Windows 现代内核上从不返回 error；这里 panic
// 仅在熵源完全坏掉时触发（容器无 /dev/urandom、自定义 sandbox 等），属于
// 真正应该让上层感知的故障。
func randomCode(length int) string {
	buf := make([]byte, (length+1)/2)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("crypto/rand failure (cannot generate secure random): %v", err))
	}
	return hex.EncodeToString(buf)[:length]
}

func stringSlice(v any) []string {
	switch typed := v.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return typed
	case string:
		if typed == "" {
			return nil
		}
		return strings.Split(typed, ",")
	default:
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func zeroNil(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func countActive(users []store.User) int {
	count := 0
	for _, u := range users {
		if u.Active {
			count++
		}
	}
	return count
}
