import type { RequestEvent } from "@sveltejs/kit";
import type { ApiEnvelope, UserInfo } from "$lib/types";

const maxResponseBytes = 8 * 1024 * 1024;

const defaultBackendURL = "http://127.0.0.1:5000";

function backendURL(): URL {
  const raw = (process.env.BACKEND_URL || defaultBackendURL).trim();
  const url = new URL(raw);
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("BACKEND_URL must use http or https");
  }
  url.pathname = url.pathname.replace(/\/+$/, "");
  url.search = "";
  url.hash = "";
  if (url.username || url.password) {
    throw new Error("BACKEND_URL must not contain credentials");
  }
  return url;
}

function backendRequestURL(path: string): string {
  if (!path.startsWith("/api/")) {
    throw new Error("V2 server API paths must stay under /api");
  }
  return new URL(path, backendURL()).toString();
}

export async function apiRequest<T>(
  event: Pick<RequestEvent, "request">,
  path: string,
  init: RequestInit = {}
): Promise<Response> {
  const headers = new Headers(init.headers);
  const cookie = event.request.headers.get("cookie");
  if (cookie && !headers.has("cookie")) headers.set("cookie", cookie);
  headers.delete("authorization");

  return fetch(backendRequestURL(path), {
    ...init,
    headers,
    redirect: "manual",
    credentials: "include"
  });
}

async function readJSON<T>(response: Response): Promise<T | null> {
  const length = Number(response.headers.get("content-length") || 0);
  if (length > maxResponseBytes) return null;
  if (!response.body) return null;
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  try {
    while (true) {
      const next = await reader.read();
      if (next.done) break;
      total += next.value.byteLength;
      if (total > maxResponseBytes) {
        await reader.cancel();
        return null;
      }
      chunks.push(next.value);
    }
  } catch {
    await reader.cancel().catch(() => undefined);
    return null;
  }
  const bytes = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  try {
    return JSON.parse(new TextDecoder().decode(bytes)) as T;
  } catch {
    return null;
  }
}

async function readBoundedRequestBody(request: Request): Promise<ArrayBuffer | null> {
  if (!request.body) return new ArrayBuffer(0);
  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  try {
    while (true) {
      const next = await reader.read();
      if (next.done) break;
      total += next.value.byteLength;
      if (total > maxResponseBytes) {
        await reader.cancel();
        return null;
      }
      chunks.push(next.value);
    }
  } catch {
    await reader.cancel().catch(() => undefined);
    return null;
  }
  const bytes = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return bytes.buffer;
}

export async function apiJSONWithResponse<T>(
  event: Pick<RequestEvent, "request">,
  path: string,
  init?: RequestInit
): Promise<{ response: Response; envelope: ApiEnvelope<T> | null } | null> {
  try {
    const response = await apiRequest<T>(event, path, init);
    const contentType = response.headers.get("content-type")?.toLowerCase() || "";
    const envelope = contentType.includes("application/json")
      ? await readJSON<ApiEnvelope<T>>(response)
      : null;
    return { response, envelope };
  } catch {
    return null;
  }
}

export async function apiJSON<T>(
  event: Pick<RequestEvent, "request">,
  path: string,
  init?: RequestInit
): Promise<ApiEnvelope<T> | null> {
  return (await apiJSONWithResponse<T>(event, path, init))?.envelope || null;
}

export async function currentUser(event: Pick<RequestEvent, "request">): Promise<UserInfo | null> {
  const cookie = event.request.headers.get("cookie");
  if (!cookie) return null;
  const envelope = await apiJSON<UserInfo>(event, "/api/v1/auth/me", {
    cache: "no-store"
  });
  return envelope?.success && envelope.data ? envelope.data : null;
}

function parseSetCookie(value: string): { name: string; value: string; options: Parameters<import("@sveltejs/kit").Cookies["set"]>[2] } | null {
  const parts = value.split(";").map((part) => part.trim());
  const first = parts.shift();
  if (!first) return null;
  const separator = first.indexOf("=");
  if (separator <= 0) return null;
  const name = first.slice(0, separator);
  const cookieValue = first.slice(separator + 1);
  const options: Parameters<import("@sveltejs/kit").Cookies["set"]>[2] = {
    path: "/",
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax"
  };
  const allowedName = process.env.SESSION_COOKIE_NAME?.trim() || "twilight_session";
  if (name !== allowedName) return null;
  for (const attribute of parts) {
    const [rawKey, ...rawValue] = attribute.split("=");
    const key = rawKey.toLowerCase();
    const joinedValue = rawValue.join("=");
    if (key === "path" && joinedValue) options.path = joinedValue;
    if (key === "max-age") options.maxAge = Number(joinedValue);
    // The upstream domain belongs to the backend origin. The SSR frontend
    // must issue a host-only cookie so it never widens the browser scope.
    if (key === "samesite" && ["lax", "strict", "none"].includes(joinedValue.toLowerCase())) {
      options.sameSite = joinedValue.toLowerCase() as "lax" | "strict" | "none";
    }
  }
  return { name, value: cookieValue, options };
}

export function copySetCookies(event: Pick<RequestEvent, "cookies" | "url">, response: Response): void {
  const values = typeof response.headers.getSetCookie === "function"
    ? response.headers.getSetCookie()
    : (response.headers.get("set-cookie") || "").split(/,(?=[^;,]+=)/g).filter(Boolean);
  for (const value of values) {
    const parsed = parseSetCookie(value);
    if (parsed) event.cookies.set(parsed.name, parsed.value, parsed.options);
  }
}

export async function proxyAPIRequest(event: RequestEvent): Promise<Response> {
  const suffix = event.params.path || "";
  if (!/^(v1|v2)(?:\/|$)/.test(suffix)) {
    return new Response("Not found", { status: 404 });
  }
  const contentLength = Number(event.request.headers.get("content-length") || 0);
  if (contentLength > maxResponseBytes) {
    return new Response("Payload too large", { status: 413 });
  }

  const headers = new Headers(event.request.headers);
  for (const name of ["connection", "host", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade"]) {
    headers.delete(name);
  }
  headers.delete("origin");
  headers.delete("referer");
  headers.delete("authorization");
  headers.delete("x-api-key");
  headers.delete("content-length");

  const hasBody = !["GET", "HEAD"].includes(event.request.method);
  const body = hasBody ? await readBoundedRequestBody(event.request) : undefined;
  if (body === null) {
    return new Response("Payload too large", { status: 413 });
  }

  let upstream: Response;
  try {
    upstream = await fetch(backendRequestURL(`/api/${suffix}`), {
      method: event.request.method,
      headers,
      body,
      redirect: "manual"
    });
  } catch {
    return new Response(JSON.stringify({ success: false, code: 503, error_code: "UPSTREAM_UNAVAILABLE", message: "服务暂时不可用" }), {
      status: 503,
      headers: { "content-type": "application/json; charset=utf-8", "cache-control": "no-store" }
    });
  }

  const responseHeaders = new Headers();
  const blocked = new Set(["connection", "content-length", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade"]);
  for (const [name, value] of upstream.headers) {
    if (!blocked.has(name.toLowerCase()) && name.toLowerCase() !== "set-cookie") responseHeaders.append(name, value);
  }
  const setCookies = typeof upstream.headers.getSetCookie === "function" ? upstream.headers.getSetCookie() : [];
  for (const value of setCookies) responseHeaders.append("set-cookie", value);
  if (!responseHeaders.has("cache-control")) responseHeaders.set("cache-control", "no-store");
  return new Response(upstream.body, { status: upstream.status, statusText: upstream.statusText, headers: responseHeaders });
}
