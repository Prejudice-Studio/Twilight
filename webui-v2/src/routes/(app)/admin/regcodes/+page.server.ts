import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import type { AdminRegcodesPageData, RegcodePage, RegcodeUsagePage } from "$lib/types";
import { t } from "$lib/i18n";

type FormState = { action?: string; error?: string; codes?: string[] };
type FormFailure = ActionFailure<FormState>;
type RegcodeQuery = AdminRegcodesPageData["query"];

const types = new Set(["", "all", "1", "2", "3"]);
const statuses = new Set(["", "all", "available", "disabled", "used_up", "expired", "decoy"]);
const sources = new Set(["", "all", "admin", "invite"]);
const sorts = new Set(["created_time", "code", "days", "type", "use_count", "note"]);

function text(value: FormDataEntryValue | string | null | undefined, max = 200): string {
  return (typeof value === "string" ? value : "").trim().slice(0, max);
}

function integer(value: string | null | undefined, fallback: number): number {
  const parsed = Number(value || "");
  return Number.isSafeInteger(parsed) ? parsed : fallback;
}

function booleanValue(value: FormDataEntryValue | null): boolean {
  return value === "true" || value === "on" || value === "1";
}

function normalizeQuery(url: URL): RegcodeQuery {
  const page = Math.max(1, Math.min(integer(url.searchParams.get("page"), 1), 1_000_000));
  const perPageValue = integer(url.searchParams.get("per_page"), 20);
  const type = text(url.searchParams.get("type"), 8);
  const status = text(url.searchParams.get("status"), 24);
  const source = text(url.searchParams.get("source"), 24);
  const sort = text(url.searchParams.get("sort"), 24);
  const order = text(url.searchParams.get("order"), 8).toLowerCase() === "asc" ? "asc" : "desc";
  return {
    page,
    per_page: [20, 50, 100].includes(perPageValue) ? perPageValue : 20,
    type: types.has(type) ? type : "",
    status: statuses.has(status) ? status : "",
    source: sources.has(source) ? source : "",
    search: text(url.searchParams.get("search"), 120),
    sort: sorts.has(sort) ? sort : "created_time",
    order,
    usage: text(url.searchParams.get("usage"), 200)
  };
}

function queryString(query: RegcodeQuery, overrides: Partial<RegcodeQuery> = {}): string {
  const value = { ...query, ...overrides };
  const params = new URLSearchParams();
  if (value.page > 1) params.set("page", String(value.page));
  if (value.per_page !== 20) params.set("per_page", String(value.per_page));
  if (value.type && value.type !== "all") params.set("type", value.type);
  if (value.status && value.status !== "all") params.set("status", value.status);
  if (value.source && value.source !== "all") params.set("source", value.source);
  if (value.search) params.set("search", value.search);
  if (value.sort !== "created_time") params.set("sort", value.sort);
  if (value.order !== "desc") params.set("order", value.order);
  return params.toString();
}

function queryFromForm(form: FormData): RegcodeQuery {
  const url = new URL("http://twilight.invalid/admin/regcodes");
  for (const name of ["page", "per_page", "type", "status", "source", "search", "sort", "order"]) {
    const value = form.get(name);
    if (typeof value === "string") url.searchParams.set(name, value);
  }
  return normalizeQuery(url);
}

function listParams(query: RegcodeQuery): URLSearchParams {
  const params = new URLSearchParams({ page: String(query.page), per_page: String(query.per_page), sort: query.sort, order: query.order });
  if (query.type && query.type !== "all") params.set("type", query.type);
  if (query.status && query.status !== "all") params.set("status", query.status);
  if (query.source && query.source !== "all") params.set("source", query.source);
  if (query.search) params.set("search", query.search);
  return params;
}

function regcodePath(code: string, suffix = ""): string {
  return `/api/v1/admin/regcodes/${encodeURIComponent(code)}${suffix}`;
}

async function mutate(
  event: RequestEvent,
  path: string,
  method: "POST" | "PUT" | "DELETE",
  payload: unknown,
  action: string,
  fallbackMessage = t.adminRegcodesOperationFailed
): Promise<{ data?: Record<string, unknown>; failure?: FormFailure }> {
  const result = await apiJSONWithResponse<Record<string, unknown>>(event, path, {
    method,
    ...(payload === undefined
      ? {}
      : { headers: { "content-type": "application/json" }, body: JSON.stringify(payload) })
  });
  if (!result?.response.ok || !result.envelope?.success) {
    return { failure: fail(result?.response.status || 503, { action, error: fallbackMessage }) };
  }
  return { data: result.envelope.data };
}

function redirectToList(form: FormData, notice: AdminRegcodesPageData["notice"]): never {
  const query = queryString(queryFromForm(form));
  throw redirect(303, `/admin/regcodes${query ? `?${query}&notice=${notice}` : `?notice=${notice}`}`);
}

function boundedNumber(form: FormData, name: string, fallback: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, integer(text(form.get(name), 24), fallback)));
}

export const load: PageServerLoad = async (event): Promise<AdminRegcodesPageData> => {
  const query = normalizeQuery(event.url);
  const usageCode = query.usage;
  const listPromise = apiJSON<RegcodePage>(event, `/api/v1/admin/regcodes?${listParams(query)}`, { cache: "no-store" });
  const usagePromise = usageCode
    ? apiJSON<RegcodeUsagePage>(event, regcodePath(usageCode, "/users"), { cache: "no-store" })
    : Promise.resolve(null);
  const [listResult, usageResult] = await Promise.all([listPromise, usagePromise]);
  const noticeValue = text(event.url.searchParams.get("notice"), 24);
  const notices = new Set(["created", "updated", "deleted", "batch_deleted", "usage_cleared"]);
  return {
    payload: listResult?.success ? listResult.data || null : null,
    usage: usageResult?.success ? usageResult.data || null : null,
    query,
    notice: notices.has(noticeValue) ? noticeValue as AdminRegcodesPageData["notice"] : "",
    loadError: listResult?.success ? null : t.adminRegcodesOperationFailed,
    usageError: usageCode && !usageResult?.success ? t.adminRegcodesOperationFailed : null
  };
};

export const actions: Actions = {
  create: async (event) => {
    const form = await event.request.formData();
    const type = boundedNumber(form, "type", 1, 1, 3);
    const rawDays = integer(text(form.get("days"), 16), 30);
    const days = rawDays === -1 ? -1 : Math.max(0, Math.min(36500, rawDays));
    const rawValidity = integer(text(form.get("validity_time"), 16), -1);
    const validity = rawValidity === -1 ? -1 : Math.max(1, Math.min(876000, rawValidity));
    const rawLimit = integer(text(form.get("use_count_limit"), 16), 1);
    const useLimit = rawLimit === -1 ? -1 : Math.max(1, Math.min(1_000_000, rawLimit));
    const count = boundedNumber(form, "count", 1, 1, 100);
    const targetUsername = text(form.get("target_username"), 32);
    const targetTelegramUsername = text(form.get("target_telegram_username"), 32).replace(/^@+/, "");
    const targetTelegramIDText = text(form.get("target_telegram_id"), 24);
    const targetUIDText = text(form.get("target_uid"), 24);
    const targetTelegramID = targetTelegramIDText ? integer(targetTelegramIDText, 0) : 0;
    const targetUID = targetUIDText ? integer(targetUIDText, 0) : 0;
    if ((targetTelegramIDText && targetTelegramID <= 0) || (targetUIDText && targetUID <= 0)) {
      return fail(400, { action: "create", error: t.adminRegcodesOperationFailed } satisfies FormState);
    }
    if ([targetUsername, targetTelegramUsername, targetTelegramIDText, targetUIDText].filter(Boolean).length > 1) {
      return fail(400, { action: "create", error: t.adminRegcodesOperationFailed } satisfies FormState);
    }
    const result = await apiJSONWithResponse<{ codes?: string[] }>(event, "/api/v1/admin/regcodes", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        type,
        days,
        validity_time: validity,
        use_count_limit: useLimit,
        count,
        decoy: booleanValue(form.get("decoy")),
        format: text(form.get("format"), 120),
        random_algorithm: text(form.get("random_algorithm"), 48),
        target_username: targetUsername,
        target_telegram_username: targetTelegramUsername,
        target_telegram_id: targetTelegramID || undefined,
        target_uid: targetUID || undefined,
        note: text(form.get("note"), 120)
      })
    });
    if (!result?.response.ok || !result.envelope?.success) {
      return fail(result?.response.status || 503, { action: "create", error: t.adminRegcodesOperationFailed } satisfies FormState);
    }
    return { action: "created", codes: result.envelope.data?.codes || [] } satisfies FormState;
  },

  update: async (event) => {
    const form = await event.request.formData();
    const code = text(form.get("code"), 200);
    if (!code) return fail(400, { action: "update", error: t.adminRegcodesOperationFailed } satisfies FormState);
    const result = await mutate(event, regcodePath(code), "PUT", {
      active: form.getAll("active").some((value) => value === "true" || value === "on" || value === "1"),
      days: integer(text(form.get("days"), 16), 30),
      validity_time: integer(text(form.get("validity_time"), 16), -1),
      use_count_limit: integer(text(form.get("use_count_limit"), 16), 1),
      note: text(form.get("note"), 120)
    }, "update");
    if (result.failure) return result.failure;
    redirectToList(form, "updated");
  },

  delete: async (event) => {
    const form = await event.request.formData();
    const code = text(form.get("code"), 200);
    if (!code) return fail(400, { action: "delete", error: t.adminRegcodesOperationFailed } satisfies FormState);
    const result = await mutate(event, regcodePath(code), "DELETE", undefined, "delete");
    if (result.failure) return result.failure;
    redirectToList(form, "deleted");
  },

  batchDelete: async (event) => {
    const form = await event.request.formData();
    const selected = form.getAll("codes").filter((value): value is string => typeof value === "string").map((value) => text(value, 200)).filter(Boolean);
    const selectAll = booleanValue(form.get("select_all"));
    if (!selectAll && selected.length === 0) {
      return fail(400, { action: "batchDelete", error: t.adminRegcodesOperationFailed } satisfies FormState);
    }
    const filter: Record<string, string> = {};
    for (const [name, key] of [["filter_type", "type"], ["filter_status", "status"], ["filter_source", "source"], ["filter_search", "search"]] as const) {
      const value = text(form.get(name), 120);
      if (value && value !== "all") filter[key] = value;
    }
    const result = await mutate(event, "/api/v1/admin/regcodes/batch-delete", "POST", {
      confirm: "BATCH_DELETE_REGCODES",
      select_all: selectAll,
      ...(selectAll ? { filter, exclude_codes: [] } : { codes: [...new Set(selected)] })
    }, "batchDelete");
    if (result.failure) return result.failure;
    redirectToList(form, "batch_deleted");
  },

  clearUsage: async (event) => {
    const form = await event.request.formData();
    const code = text(form.get("code"), 200);
    if (!code) return fail(400, { action: "clearUsage", error: t.adminRegcodesOperationFailed } satisfies FormState);
    const result = await mutate(event, regcodePath(code, "/clear-usage"), "POST", { confirm: "CLEAR_REGCODE_USAGE" }, "clearUsage");
    if (result.failure) return result.failure;
    const query = queryString(queryFromForm(form));
    throw redirect(303, `/admin/regcodes${query ? `?${query}&notice=usage_cleared` : "?notice=usage_cleared"}`);
  }
};
