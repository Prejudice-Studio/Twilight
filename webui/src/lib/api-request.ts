import type { ApiResponse } from "./api-types";

export const API_BASE = (process.env.NEXT_PUBLIC_API_URL || "").replace(/\/$/, "");

/**
 * ApiError 承载 HTTP 状态码 + 后端 envelope.error_code，
 * 让业务层可以通过 instanceof ApiError 分流处理：
 *   - 401  → 跳登录 / 刷新会话
 *   - 403  → 权限提示
 *   - 429  → 退避重试
 *   - 5xx  → 显示通用故障
 *   - 自定义 error_code（如 AUTH_ACCOUNT_DISABLED） → 业务级处理
 *
 *
 */
export class ApiError extends Error {
  readonly status: number;
  readonly endpoint: string;
  readonly method: string;
  readonly errorCode?: string;
  readonly backendMessage?: string;

  constructor(init: {
    status: number;
    endpoint: string;
    method: string;
    errorCode?: string;
    backendMessage?: string;
    message: string;
  }) {
    super(init.message);
    this.name = "ApiError";
    this.status = init.status;
    this.endpoint = init.endpoint;
    this.method = init.method;
    this.errorCode = init.errorCode;
    this.backendMessage = init.backendMessage;
  }

  isAuth(): boolean {
    return this.status === 401;
  }
  isForbidden(): boolean {
    return this.status === 403;
  }
  isRateLimited(): boolean {
    return this.status === 429;
  }
  /** 5xx 与 429 都建议退避重试 */
  isRetryable(): boolean {
    return this.status === 429 || this.status === 502 || this.status === 503 || this.status === 504;
  }
}

function isAbortError(error: unknown): boolean {
  if (error instanceof DOMException && error.name === "AbortError") {
    return true;
  }
  if (error instanceof Error && error.name === "AbortError") {
    return true;
  }
  return false;
}

function requestMethod(options: RequestInit, fallback = "GET"): string {
  return (options.method || fallback).toString().toUpperCase();
}

/**
 * 读取 CSRF cookie。后端登录 / refresh 时下发非 HttpOnly 的
 * `<session>_csrf` cookie，前端读出后塞进 X-CSRF-Token 请求头，
 * 形成"双提交 cookie"模式抵御 CSRF 攻击。
 *
 * cookie 名约定：`twilight_session_csrf`（后端 `csrfCookieName()`）。
 *
 * 名称解析优先级：
 *   1. systemInfo 拿到的 csrf_cookie_name —— 通过 setCsrfCookieName() 注入
 *      到模块级缓存，按精确名匹配 cookie，杜绝同域 / 父域第三方应用下发
 *      其它 *_csrf cookie 时被取错；
 *   2. 缓存为空时回退到旧的"首个 *_csrf 后缀"启发式，并在开发态打印一次
 *      console.warn 提醒尽快走 systemInfo 路径；
 *   3. 都拿不到返回空串，让 mutating 请求被后端 CSRF 中间件拒绝（403），
 *      避免静默裸跑。
 */
let cachedCsrfCookieName: string | null = null;
let csrfFallbackWarned = false;

/**
 * 把后端 systemInfo.csrf_cookie_name 注入模块级缓存，让 readCSRFCookie()
 * 按精确名匹配。系统信息 store 拿到响应后调用一次即可。
 *
 * 传 null / 空串相当于清空缓存（用于测试或登出场景）。
 */
export function setCsrfCookieName(name: string | null | undefined): void {
  cachedCsrfCookieName = name && name.length > 0 ? name : null;
}

function readCSRFCookie(): string {
  if (typeof document === "undefined") return "";
  const all = document.cookie ? document.cookie.split(";") : [];

  // 路径 1：systemInfo 已下发精确名，直接按名取，零歧义。
  if (cachedCsrfCookieName) {
    for (const raw of all) {
      const eq = raw.indexOf("=");
      if (eq <= 0) continue;
      const name = raw.slice(0, eq).trim();
      if (name === cachedCsrfCookieName) {
        return decodeURIComponent(raw.slice(eq + 1).trim());
      }
    }
    return "";
  }

  // 路径 2：缓存还没就绪（典型场景：systemInfo 还没拉回来就触发了 mutating
  // 请求，比如登录前的 telegram-link 探针）。退回旧的 *_csrf 后缀启发式，
  // 同时仅一次开发态告警，不污染生产控制台。
  if (process.env.NODE_ENV !== "production" && !csrfFallbackWarned) {
    csrfFallbackWarned = true;
    // eslint-disable-next-line no-console
    console.warn(
      "[CSRF] systemInfo.csrf_cookie_name 尚未注入，临时使用 *_csrf 后缀启发式；" +
        "请确保进入受 CSRF 保护接口前已调用 useSystemStore.fetchInfo()。",
    );
  }
  for (const raw of all) {
    const eq = raw.indexOf("=");
    if (eq <= 0) continue;
    const name = raw.slice(0, eq).trim();
    if (name.endsWith("_csrf")) {
      return decodeURIComponent(raw.slice(eq + 1).trim());
    }
  }
  return "";
}

function isMutating(method: string): boolean {
  switch (method.toUpperCase()) {
    case "POST":
    case "PUT":
    case "PATCH":
    case "DELETE":
      return true;
    default:
      return false;
  }
}

function describeApiTarget(endpoint: string, method: string): string {
  return `${method} /api/v1${endpoint}`;
}

function buildHttpErrorMessage(
  status: number,
  endpoint: string,
  method: string,
  backendMessage?: string,
): string {
  const target = describeApiTarget(endpoint, method);
  const detail = backendMessage && backendMessage !== "接口不存在" ? `后端返回：${backendMessage}` : "";

  if (status === 404) {
    if (backendMessage && backendMessage !== "接口不存在") {
      return backendMessage;
    }
    return [
      `接口不存在：${target}`,
      detail,
      "常见原因：后端未更新/未重启、前后端版本不一致，或当前功能在后端尚未实现。",
    ].filter(Boolean).join("\n");
  }
  if (status === 405) {
    return [
      `请求方法不允许：${target}`,
      detail,
      "请确认前端调用的方法与后端路由一致，例如 GET/POST/PUT/DELETE 是否写反。",
    ].filter(Boolean).join("\n");
  }
  if (status === 401) {
    return backendMessage || "登录状态已失效，请重新登录。";
  }
  if (status === 403) {
    return backendMessage || `权限不足：当前账号无权访问 ${target}。`;
  }
  if (status === 413) {
    return backendMessage || "上传内容过大，请压缩文件或联系管理员调整上传上限。";
  }
  if (status === 429) {
    return backendMessage || "请求过于频繁，请稍后再试。";
  }
  if (status === 500) {
    return [
      `后端接口执行失败：${target}`,
      detail || "服务器内部错误，请查看后端日志。",
    ].filter(Boolean).join("\n");
  }
  if (status === 502 || status === 503 || status === 504) {
    return `后端服务暂不可用 (${status})：${target}\n请确认 API 服务、反向代理或网关正在运行。`;
  }
  return backendMessage || `请求失败 (${status})：${target}`;
}

function buildParseErrorMessage(status: number, endpoint: string, method: string): string {
  const target = describeApiTarget(endpoint, method);
  if (status === 404) {
    return `接口不存在：${target}\n服务器没有返回标准 JSON，可能命中了前端页面 404、反向代理路径错误，或后端缺少该路由。`;
  }
  if (status >= 500) {
    return `后端响应格式异常：${target}\nHTTP ${status} 未返回标准 JSON，请查看后端或网关日志。`;
  }
  return `服务器响应解析失败 (${status})：${target}\n接口没有返回标准 JSON。`;
}

async function parseApiResponse<T>(
  response: Response,
  endpoint: string,
  method: string,
): Promise<ApiResponse<T>> {
  if (response.status === 204) {
    return { success: true, message: "OK" };
  }

  const text = await response.text();
  if (!text) {
    return { success: response.ok, message: response.ok ? "OK" : response.statusText };
  }

  try {
    return JSON.parse(text) as ApiResponse<T>;
  } catch (error) {
    if (!response.ok) {
      return { success: false, message: response.statusText || `HTTP ${response.status}` };
    }
    console.error("JSON parse error:", error);
    throw new Error(buildParseErrorMessage(response.status, endpoint, method));
  }
}

export async function apiRequest<T>(endpoint: string, options: RequestInit = {}): Promise<ApiResponse<T>> {
  const headers: Record<string, string> = {
    "Accept": "application/json; charset=utf-8",
    "Content-Type": "application/json; charset=utf-8",
    "X-Twilight-Client": "webui",
    ...((options.headers as Record<string, string>) || {}),
  };

  const url = `${API_BASE}/api/v1${endpoint}`;
  const method = requestMethod(options);

  // CSRF: cookie-auth 的 mutating 请求必须带 X-CSRF-Token。
  // 用户尚未登录时无 csrf cookie，跳过即可（公共端点不会进 CSRF 校验）。
  if (isMutating(method) && !headers["X-CSRF-Token"]) {
    const csrf = readCSRFCookie();
    if (csrf) headers["X-CSRF-Token"] = csrf;
  }

  let response: Response;
  try {
    response = await fetch(url, {
      ...options,
      headers,
      credentials: "include",
    });
  } catch (error) {
    if (isAbortError(error)) {
      throw error;
    }
    console.error("Network error:", error);
    throw new Error(
      `无法连接后端接口：${describeApiTarget(endpoint, method)}\n请检查后端服务是否启动、API 地址是否正确、反向代理是否可达.`
    );
  }

  const data = await parseApiResponse<T>(response, endpoint, method);

  if (!response.ok) {
    throw new ApiError({
      status: response.status,
      endpoint,
      method,
      errorCode: data?.error_code,
      backendMessage: data?.message,
      message: buildHttpErrorMessage(response.status, endpoint, method, data?.message),
    });
  }

  return data;
}

export async function apiRequestForm<T>(
  endpoint: string,
  formData: FormData,
  method: "POST" | "PUT" = "POST",
): Promise<ApiResponse<T>> {
  const headers: Record<string, string> = {
    "Accept": "application/json; charset=utf-8",
    "X-Twilight-Client": "webui",
  };
  // form upload 一定是 mutating，必须带 CSRF
  const csrf = readCSRFCookie();
  if (csrf) headers["X-CSRF-Token"] = csrf;

  const url = `${API_BASE}/api/v1${endpoint}`;
  const methodName = method.toUpperCase();

  let response: Response;
  try {
    response = await fetch(url, {
      method,
      headers,
      body: formData,
      credentials: "include",
    });
  } catch (error) {
    if (isAbortError(error)) {
      throw error;
    }
    console.error("Network error:", error);
    throw new Error(
      `无法连接后端接口：${describeApiTarget(endpoint, methodName)}\n请检查后端服务是否启动、API 地址是否正确、反向代理是否可达。`
    );
  }

  const data = await parseApiResponse<T>(response, endpoint, methodName);

  if (!response.ok) {
    throw new ApiError({
      status: response.status,
      endpoint,
      method: methodName,
      errorCode: data?.error_code,
      backendMessage: data?.message,
      message: buildHttpErrorMessage(response.status, endpoint, methodName, data?.message),
    });
  }

  return data;
}
