import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import type { AdminAuditLogsPageData, AuditLogPage } from "$lib/types";
import { t } from "$lib/i18n";

type FormState = { action?: string; error?: string };
type FormFailure = ActionFailure<FormState>;
type AuditQuery = AdminAuditLogsPageData["query"];

const presets = new Set(["", "all", "admin", "user", "system", "destructive", "security", "today", "week"]);
const categories = new Set(["", "all", "admin", "user", "system"]);
const times = new Set(["", "all", "today", "24h", "7d", "30d"]);
const sorts = new Set(["created_desc", "created_asc", "action_asc", "action_desc", "user_asc", "user_desc", "category_asc", "uid_asc"]);
const actionFilters = new Set([
  "", "all", "create_regcode", "update_regcode", "delete_regcode", "batch_delete_regcode",
  "clear_regcode_usage", "create_invite_code", "create_renew_code", "use_code", "update_user",
  "set_role", "enable_user", "disable_user", "delete_user", "batch_enable_users",
  "batch_disable_users", "batch_renew_users", "batch_delete_users"
]);

function text(value: string | null | undefined, max = 200): string {
  return (value || "").trim().slice(0, max);
}

function integer(value: string, fallback = 0): number {
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) ? parsed : fallback;
}

function normalizeQuery(url: URL): AuditQuery {
  const pageValue = integer(text(url.searchParams.get("page"), 12), 1);
  const perPageValue = integer(text(url.searchParams.get("per_page"), 8), 50);
  const preset = text(url.searchParams.get("preset"), 24);
  const category = text(url.searchParams.get("category"), 24);
  const action = text(url.searchParams.get("action"), 64);
  const time = text(url.searchParams.get("time"), 24);
  const sort = text(url.searchParams.get("sort"), 24);
  const order = text(url.searchParams.get("order"), 8).toLowerCase() === "asc" ? "asc" : "desc";
  return {
    page: Math.max(1, Math.min(pageValue, 1_000_000)),
    per_page: [25, 50, 100, 200].includes(perPageValue) ? perPageValue : 50,
    preset: presets.has(preset) ? preset : "",
    category: categories.has(category) ? category : "",
    action: actionFilters.has(action) ? action : "",
    time: times.has(time) ? time : "",
    sort: sorts.has(sort) ? sort : "created_desc",
    order,
    uid: Math.max(0, integer(text(url.searchParams.get("uid"), 24), 0)),
    target_uid: Math.max(0, integer(text(url.searchParams.get("target_uid"), 24), 0)),
    search: text(url.searchParams.get("search"), 200)
  };
}

function queryString(query: AuditQuery, overrides: Partial<AuditQuery> = {}): string {
  const value = { ...query, ...overrides };
  const params = new URLSearchParams();
  for (const [key, item] of Object.entries(value)) {
    if (item === "" || item === 0 || (key === "page" && item === 1) || (key === "per_page" && item === 50) || (key === "sort" && item === "created_desc") || (key === "order" && item === "desc")) continue;
    params.set(key, String(item));
  }
  return params.toString();
}

function queryFromForm(form: FormData): AuditQuery {
  const url = new URL("http://twilight.invalid/admin/audit-logs");
  for (const name of ["page", "per_page", "preset", "category", "action", "time", "sort", "order", "uid", "target_uid", "search"]) {
    const value = form.get(name);
    if (typeof value === "string") url.searchParams.set(name, value);
  }
  return normalizeQuery(url);
}

function sortParts(sort: string): { sort: string; order: string } {
  switch (sort) {
    case "created_asc": return { sort: "created_at", order: "asc" };
    case "action_asc": return { sort: "action", order: "asc" };
    case "action_desc": return { sort: "action", order: "desc" };
    case "user_asc": return { sort: "username", order: "asc" };
    case "user_desc": return { sort: "username", order: "desc" };
    case "category_asc": return { sort: "category", order: "asc" };
    case "uid_asc": return { sort: "uid", order: "asc" };
    default: return { sort: "created_at", order: "desc" };
  }
}

function addTimeFilter(params: URLSearchParams, value: string): void {
  const now = Math.floor(Date.now() / 1000);
  if (value === "24h") params.set("from", String(now - 24 * 60 * 60));
  if (value === "7d") params.set("from", String(now - 7 * 24 * 60 * 60));
  if (value === "30d") params.set("from", String(now - 30 * 24 * 60 * 60));
  if (value === "24h" || value === "7d" || value === "30d") params.set("to", String(now));
}

async function mutate<T>(event: RequestEvent, path: string, method: "POST" | "DELETE", payload: unknown, action: string): Promise<{ data?: T; failure?: FormFailure }> {
  const result = await apiJSONWithResponse<T>(event, path, {
    method,
    ...(payload === undefined ? {} : { headers: { "content-type": "application/json" }, body: JSON.stringify(payload) })
  });
  if (!result?.response.ok || !result.envelope?.success) {
    return { failure: fail(result?.response.status || 503, { action, error: result?.envelope?.message || t.adminAuditLogOperationFailed } satisfies FormState) };
  }
  return { data: result.envelope.data };
}

function redirectToList(form: FormData, notice: string): never {
  const query = queryString(queryFromForm(form));
  throw redirect(303, `/admin/audit-logs${query ? `?${query}&notice=${notice}` : `?notice=${notice}`}`);
}

export const load: PageServerLoad = async (event): Promise<AdminAuditLogsPageData> => {
  const query = normalizeQuery(event.url);
  const params = new URLSearchParams({ page: String(query.page), per_page: String(query.per_page) });
  if (query.preset && query.preset !== "all") params.set("preset", query.preset);
  if (query.category && query.category !== "all") params.set("category", query.category);
  if (query.action && query.action !== "all") params.set("action", query.action);
  if (query.uid > 0) params.set("uid", String(query.uid));
  if (query.target_uid > 0) params.set("target_uid", String(query.target_uid));
  if (query.search) params.set("search", query.search);
  if (query.time === "today") params.set("preset", "today");
  if (query.time === "week") params.set("preset", "week");
  addTimeFilter(params, query.time);
  const sort = sortParts(query.sort);
  params.set("sort", sort.sort);
  params.set("order", sort.order);
  const result = await apiJSON<AuditLogPage>(event, `/api/v2/admin/audit-logs?${params}`, { cache: "no-store" });
  const notice = ["deleted", "cleared", "pruned"].includes(text(event.url.searchParams.get("notice"), 16)) ? text(event.url.searchParams.get("notice"), 16) : "";
  return {
    payload: result?.success ? result.data || null : null,
    query,
    notice,
    loadError: result?.success ? null : t.adminAuditLogLoadFailed
  } as AdminAuditLogsPageData;
};

export const actions: Actions = {
  deleteLog: async (event) => {
    const form = await event.request.formData();
    const id = integer(text(form.get("log_id") as string | null, 24), 0);
    if (id <= 0) return fail(400, { action: "deleteLog", error: t.adminAuditLogOperationFailed } satisfies FormState);
    const result = await mutate(event, `/api/v2/admin/audit-logs/${id}`, "DELETE", undefined, "deleteLog");
    if (result.failure) return result.failure;
    redirectToList(form, "deleted");
  },

  clear: async (event) => {
    const form = await event.request.formData();
    const result = await mutate(event, "/api/v2/admin/audit-logs/clear", "POST", { confirm: "CLEAR_AUDIT_LOGS" }, "clear");
    if (result.failure) return result.failure;
    redirectToList(form, "cleared");
  },

  prune: async (event) => {
    const form = await event.request.formData();
    const days = Math.max(0, Math.min(3650, integer(text(form.get("retention_days") as string | null, 8), 0)));
    const entries = Math.max(0, Math.min(100000, integer(text(form.get("max_entries") as string | null, 10), 0)));
    if (days === 0 && entries === 0) return fail(400, { action: "prune", error: t.adminAuditLogOperationFailed } satisfies FormState);
    const preserveAdmin = form.getAll("preserve_admin").some((value) => value === "true" || value === "on");
    const result = await mutate(event, "/api/v2/admin/audit-logs/prune", "POST", {
      confirm: "PRUNE_AUDIT_LOGS",
      ...(days > 0 ? { retention_days: days } : {}),
      ...(entries > 0 ? { max_entries: entries } : {}),
      preserve_admin: preserveAdmin
    }, "prune");
    if (result.failure) return result.failure;
    redirectToList(form, "pruned");
  }
};
