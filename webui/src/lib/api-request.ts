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
  /**
   * 后端在 envelope.data 上携带的结构化元数据。绝大多数失败响应 data 为 null
   * 或 undefined；少数 handler（典型如 admin 绑定 Emby 冲突）会把 conflict
   * 上下文塞进 data，让前端在拿到 errorCode 后还能读出 conflict_uid /
   * conflict_username 这种二次决策需要的字段。无类型约束—— consumer 自己负
   * 责类型断言（与底层 ApiResponse<T> 的 data: T | null 对齐）。
   */
  readonly data?: unknown;

  constructor(init: {
    status: number;
    endpoint: string;
    method: string;
    errorCode?: string;
    backendMessage?: string;
    message: string;
    data?: unknown;
  }) {
    super(init.message);
    this.name = "ApiError";
    this.status = init.status;
    this.endpoint = init.endpoint;
    this.method = init.method;
    this.errorCode = init.errorCode;
    this.backendMessage = init.backendMessage;
    this.data = init.data;
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

function isTimeoutError(error: unknown): boolean {
  if (error instanceof DOMException && error.name === "TimeoutError") {
    return true;
  }
  if (error instanceof Error && error.name === "TimeoutError") {
    return true;
  }
  return false;
}

function requestMethod(options: RequestInit, fallback = "GET"): string {
  return (options.method || fallback).toString().toUpperCase();
}

function headersInitToRecord(init?: HeadersInit): Record<string, string> {
  if (!init) return {};
  const out: Record<string, string> = {};
  new Headers(init).forEach((value, key) => {
    out[key] = value;
  });
  return out;
}

function hasHeader(headers: Record<string, string>, name: string): boolean {
  return Object.keys(headers).some((key) => key.toLowerCase() === name.toLowerCase());
}

/** 默认 fetch 超时（毫秒）。慢响应或 socket 挂起时让 Promise 不至于永挂。 */
const DEFAULT_REQUEST_TIMEOUT_MS = 20_000;
/** 表单 / multipart 上传放宽到 2 分钟，覆盖较大背景图 / 头像。 */
const FORM_REQUEST_TIMEOUT_MS = 120_000;
const READ_REQUEST_DEDUPE_WINDOW_MS = 750;

const inFlightReadRequests = new Map<string, { promise: Promise<ApiResponse<unknown>>; startedAt: number }>();

function headerCacheKey(headers: Record<string, string>): string {
  return Object.entries(headers)
    .map(([key, value]) => [key.toLowerCase(), value] as const)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => `${key}:${value}`)
    .join("|");
}

function readRequestDedupeKey(url: string, method: string, headers: Record<string, string>): string {
  return `${method} ${url} ${headerCacheKey(headers)}`;
}

/**
 * 把"调用方 signal"与"超时 signal"合并成一个：任一触发都 abort。
 * 走 AbortSignal.timeout + AbortSignal.any 优先（现代浏览器/Node 18+），
 * 退化路径手写同样语义，保证 SSR / 老浏览器也能跑。
 *
 * timeoutMs 传 0 / Infinity 表示不加超时；典型用于 SSE / 长轮询通道，调用方
 * 应当传入自己的 signal（fetch 完后立即关流不会受 20s 限制影响）。
 *
 * 返回 cleanup 函数，无论 fetch 成功/失败都应调用，否则未触发的 setTimeout
 * 会留 200~500 个挂起的定时器。
 */
function withTimeoutSignal(
  caller: AbortSignal | null | undefined,
  timeoutMs: number,
): { signal: AbortSignal | undefined; cleanup: () => void; isTimeout: () => boolean } {
  const finite = Number.isFinite(timeoutMs) && timeoutMs > 0;
  if (!finite && !caller) {
    return { signal: undefined, cleanup: () => {}, isTimeout: () => false };
  }
  if (!finite) {
    return { signal: caller ?? undefined, cleanup: () => {}, isTimeout: () => false };
  }

  // 优先使用原生 AbortSignal.timeout / any（Node 20、Chromium 124+、Safari 17+）。
  type AbortStatic = typeof AbortSignal & {
    timeout?: (ms: number) => AbortSignal;
    any?: (signals: AbortSignal[]) => AbortSignal;
  };
  const native = AbortSignal as AbortStatic;
  if (typeof native.timeout === "function") {
    const timeoutSig = native.timeout(timeoutMs);
    if (!caller) {
      return { signal: timeoutSig, cleanup: () => {}, isTimeout: () => timeoutSig.aborted };
    }
    if (typeof native.any === "function") {
      return {
        signal: native.any([caller, timeoutSig]),
        cleanup: () => {},
        isTimeout: () => timeoutSig.aborted && !caller.aborted,
      };
    }
  }

  // 退化路径：手动桥接。
  const controller = new AbortController();
  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    controller.abort(new DOMException("timeout", "TimeoutError"));
  }, timeoutMs);
  const onCallerAbort = () => controller.abort(caller?.reason);
  if (caller) {
    if (caller.aborted) controller.abort(caller.reason);
    else caller.addEventListener("abort", onCallerAbort, { once: true });
  }
  return {
    signal: controller.signal,
    cleanup: () => {
      clearTimeout(timer);
      caller?.removeEventListener("abort", onCallerAbort);
    },
    isTimeout: () => timedOut,
  };
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

export interface ApiRequestExtraOptions {
  /** 自定义超时（毫秒）；传 0 / Infinity 表示不加超时（SSE / 长轮询场景）。 */
  timeoutMs?: number;
  /** Disable same-tick GET/HEAD request coalescing for endpoints that must be fetched independently. */
  dedupe?: boolean;
}

export async function apiRequest<T>(
  endpoint: string,
  options: RequestInit = {},
  extra: ApiRequestExtraOptions = {},
): Promise<ApiResponse<T>> {
  const method = requestMethod(options);
  const headers: Record<string, string> = {
    "Accept": "application/json; charset=utf-8",
    ...headersInitToRecord(options.headers),
  };
  if (options.body !== undefined && !hasHeader(headers, "content-type")) {
    headers["Content-Type"] = "application/json; charset=utf-8";
  }
  if ((options.body !== undefined || (method !== "GET" && method !== "HEAD")) && !hasHeader(headers, "x-twilight-client")) {
    headers["X-Twilight-Client"] = "webui";
  }

  const url = `${API_BASE}/api/v1${endpoint}`;
  const isReadRequest = (method === "GET" || method === "HEAD") && options.body === undefined;
  if (isReadRequest && options.signal === undefined && extra.dedupe !== false) {
    const key = readRequestDedupeKey(url, method, headers);
    const now = Date.now();
    const existing = inFlightReadRequests.get(key);
    if (existing && now - existing.startedAt <= READ_REQUEST_DEDUPE_WINDOW_MS) {
      return existing.promise as Promise<ApiResponse<T>>;
    }
    const promise = apiRequest<T>(endpoint, options, { ...extra, dedupe: false }) as Promise<ApiResponse<unknown>>;
    inFlightReadRequests.set(key, { promise, startedAt: now });
    void promise.finally(() => {
      if (inFlightReadRequests.get(key)?.promise === promise) {
        inFlightReadRequests.delete(key);
      }
    }).catch(() => undefined);
    return promise as Promise<ApiResponse<T>>;
  }

  const timeoutMs = extra.timeoutMs ?? DEFAULT_REQUEST_TIMEOUT_MS;
  const guard = withTimeoutSignal(options.signal ?? null, timeoutMs);

  let response: Response;
  try {
    response = await fetch(url, {
      ...options,
      headers,
      cache: options.cache ?? (isReadRequest ? "no-cache" : "no-store"),
      credentials: "include",
      signal: guard.signal ?? options.signal ?? null,
    });
  } catch (error) {
    if (guard.isTimeout() || isTimeoutError(error)) {
      throw new ApiError({
        status: 0,
        endpoint,
        method,
        errorCode: "REQUEST_TIMEOUT",
        message: `请求超时：${describeApiTarget(endpoint, method)}\n网络或后端响应过慢，请稍后重试。`,
      });
    }
    if (isAbortError(error)) {
      throw error;
    }
    console.error("Network error:", error);
    throw new Error(
      `无法连接后端接口：${describeApiTarget(endpoint, method)}\n请检查后端服务是否启动、API 地址是否正确、反向代理是否可达.`
    );
  } finally {
    guard.cleanup();
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
      data: data?.data,
    });
  }

  return data;
}

export async function apiRequestForm<T>(
  endpoint: string,
  formData: FormData,
  method: "POST" | "PUT" = "POST",
  extra: ApiRequestExtraOptions = {},
): Promise<ApiResponse<T>> {
  const headers: Record<string, string> = {
    "Accept": "application/json; charset=utf-8",
    "X-Twilight-Client": "webui",
  };
  const url = `${API_BASE}/api/v1${endpoint}`;
  const methodName = method.toUpperCase();

  const timeoutMs = extra.timeoutMs ?? FORM_REQUEST_TIMEOUT_MS;
  const guard = withTimeoutSignal(null, timeoutMs);

  let response: Response;
  try {
    response = await fetch(url, {
      method,
      headers,
      body: formData,
      cache: "no-store",
      credentials: "include",
      signal: guard.signal ?? null,
    });
  } catch (error) {
    if (guard.isTimeout() || isTimeoutError(error)) {
      throw new ApiError({
        status: 0,
        endpoint,
        method: methodName,
        errorCode: "REQUEST_TIMEOUT",
        message: `上传超时：${describeApiTarget(endpoint, methodName)}\n文件较大或网络较慢，请稍后重试。`,
      });
    }
    if (isAbortError(error)) {
      throw error;
    }
    console.error("Network error:", error);
    throw new Error(
      `无法连接后端接口：${describeApiTarget(endpoint, methodName)}\n请检查后端服务是否启动、API 地址是否正确、反向代理是否可达。`
    );
  } finally {
    guard.cleanup();
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
      data: data?.data,
    });
  }

  return data;
}
