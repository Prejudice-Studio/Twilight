import type { RequestHandler } from "./$types";
import { apiRequest } from "$lib/server/api";

const maxPasswordBytes = 1024;

export const POST: RequestHandler = async (event) => {
  const form = await event.request.formData();
  const value = form.get("password");
  const password = typeof value === "string" ? value.slice(0, 1024) : "";
  if (new TextEncoder().encode(password).byteLength > maxPasswordBytes) return new Response("请求无效", { status: 400, headers: { "cache-control": "no-store" } });
  const upstream = await apiRequest(event, "/api/v1/system/admin/migration/export", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ password }) });
  const headers = new Headers();
  for (const name of ["content-type", "content-disposition", "x-twilight-migration-format", "x-twilight-migration-files"]) { const header = upstream.headers.get(name); if (header) headers.set(name, header); }
  headers.set("cache-control", "no-store, private"); headers.set("pragma", "no-cache");
  return new Response(upstream.body, { status: upstream.status, statusText: upstream.statusText, headers });
};
