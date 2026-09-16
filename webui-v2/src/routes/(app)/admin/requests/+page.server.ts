import { fail, redirect } from "@sveltejs/kit";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { AdminMediaRequest, AdminMediaRequestListResponse } from "$lib/types";
import { t } from "$lib/i18n";

type FormState = { action?: string; error?: string };
type FormFailure = ActionFailure<FormState>;
type RequestStatus = "active" | "all" | "pending" | "accepted" | "downloading" | "rejected" | "completed";
type RequestSource = "all" | "tmdb" | "bangumi";

type V2AdminMediaRequestResource = {
  items: AdminMediaRequest[];
  pagination: {
    page: number;
    per_page: number;
    total: number;
    total_pages: number;
  };
  request_total: number;
  has_next: boolean;
  status_counts: AdminMediaRequestListResponse["status_counts"];
};

const statuses = new Set<RequestStatus>(["active", "all", "pending", "accepted", "downloading", "rejected", "completed"]);
const sources = new Set<RequestSource>(["all", "tmdb", "bangumi"]);

function text(value: string | null | undefined, max = 200): string {
  return (value || "").replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g, "").trim().slice(0, max);
}

function formText(form: FormData, name: string, max = 200): string {
  const value = form.get(name);
  return typeof value === "string" ? text(value, max) : "";
}

function integer(value: string | null | undefined, fallback: number): number {
  const parsed = Number(value || "");
  return Number.isSafeInteger(parsed) ? parsed : fallback;
}

function normalizeQuery(url: URL) {
  const rawStatus = text(url.searchParams.get("status"), 24) as RequestStatus;
  const rawSource = text(url.searchParams.get("source"), 24) as RequestSource;
  const perPage = integer(url.searchParams.get("per_page"), 20);
  return {
    status: statuses.has(rawStatus) ? rawStatus : "active" as RequestStatus,
    source: sources.has(rawSource) ? rawSource : "all" as RequestSource,
    query: text(url.searchParams.get("q") || url.searchParams.get("query"), 120),
    page: Math.max(1, Math.min(1_000_000, integer(url.searchParams.get("page"), 1))),
    per_page: [20, 50, 100].includes(perPage) ? perPage : 20,
    split: parseSplit(url.searchParams.get("split"))
  };
}

function parseSplit(value: string | null): string[] {
  if (!value) return [];
  const result: string[] = [];
  for (const raw of value.split(",").slice(0, 100)) {
    try {
      const key = decodeURIComponent(raw).trim().slice(0, 160);
      if (key && !/[\\/\u0000-\u001F\u007F]/.test(key) && !result.includes(key)) result.push(key);
    } catch {
      // Ignore malformed URL state instead of failing the entire admin page.
    }
  }
  return result;
}

function queryString(query: ReturnType<typeof normalizeQuery>, page = query.page): string {
  const params = new URLSearchParams();
  params.set("status", query.status);
  params.set("source", query.source);
  params.set("page", String(Math.max(1, page)));
  params.set("per_page", String(query.per_page));
  if (query.query) params.set("q", query.query);
  if (query.split.length) params.set("split", query.split.map((key) => encodeURIComponent(key)).join(","));
  return params.toString();
}

function queryFromForm(form: FormData) {
  const url = new URL("http://twilight.invalid/admin/requests");
  for (const name of ["status", "source", "q", "page", "per_page", "split"]) {
    const value = form.get(name);
    if (typeof value === "string") url.searchParams.set(name, value);
  }
  return normalizeQuery(url);
}

function validKey(value: string): boolean {
  return value.length > 0 && value.length <= 160 && !/[\\/\u0000-\u001F\u007F]/.test(value);
}

function validStatus(value: string): value is Exclude<RequestStatus, "active" | "all"> {
  return value === "pending" || value === "accepted" || value === "downloading" || value === "rejected" || value === "completed";
}

function revisionValue(value: string): number | null {
  if (!/^\d{1,18}$/.test(value)) return null;
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed >= 0 ? parsed : null;
}

async function mutate(
  event: RequestEvent,
  path: string,
  method: "PUT" | "DELETE",
  payload: Record<string, unknown> | undefined,
  revision: number | null,
  action: string,
  fallback: string
): Promise<FormFailure | null> {
  const result = await apiJSONWithResponse<unknown>(event, path, {
    method,
    headers: {
      ...(payload ? { "content-type": "application/json" } : {}),
      ...(revision !== null ? { "if-match": `"${revision}"` } : {})
    },
    ...(payload ? { body: JSON.stringify(payload) } : {})
  });
  if (!result?.response.ok || !result.envelope?.success) {
    return fail(result?.response.status || 503, {
      action,
      error: result?.envelope?.message || fallback
    } satisfies FormState);
  }
  return null;
}

function redirectToList(form: FormData, notice: string): never {
  const query = queryString(queryFromForm(form));
  throw redirect(303, `/admin/requests?${query}&notice=${encodeURIComponent(notice)}`);
}

export const load: PageServerLoad = async (event) => {
  const query = normalizeQuery(event.url);
  const params = new URLSearchParams({
    status: query.status,
    source: query.source,
    page: String(query.page),
    per_page: String(query.per_page)
  });
  if (query.query) params.set("q", query.query);
  const result = await apiJSON<V2AdminMediaRequestResource>(event, `/api/v2/admin/media-requests?${params}`, { cache: "no-store" });
  const payload = result?.success && result.data ? {
    requests: result.data.items,
    total: result.data.pagination.total,
    request_total: result.data.request_total,
    page: result.data.pagination.page,
    per_page: result.data.pagination.per_page,
    total_pages: result.data.pagination.total_pages,
    has_next: result.data.has_next,
    status_counts: result.data.status_counts
  } satisfies AdminMediaRequestListResponse : null;
  return {
    payload,
    query,
    notice: text(event.url.searchParams.get("notice"), 32),
    loadError: result?.success ? null : t.adminRequestsLoadFailed
  };
};

export const actions: Actions = {
  update: async (event) => {
    const form = await event.request.formData();
    const key = formText(form, "require_key", 160);
    const status = formText(form, "status", 24);
    const note = formText(form, "note", 1000);
    const revision = revisionValue(formText(form, "revision", 24));
    if (!validKey(key) || !validStatus(status) || revision === null) {
      return fail(400, { action: "update", error: t.adminRequestsInvalidPayload } satisfies FormState);
    }
    const failure = await mutate(event, `/api/v2/admin/media-requests/by-key/${encodeURIComponent(key)}`, "PUT", { status, note }, revision, "update", t.adminRequestsUpdateFailed);
    if (failure) return failure;
    redirectToList(form, "updated");
  },

  updateGroup: async (event) => {
    const form = await event.request.formData();
    const status = formText(form, "status", 24);
    const note = formText(form, "note", 1000);
    let rawItems: unknown;
    try {
      rawItems = JSON.parse(formText(form, "items", 24_000));
    } catch {
      rawItems = null;
    }
    if (!validStatus(status) || !Array.isArray(rawItems) || rawItems.length < 2 || rawItems.length > 100) {
      return fail(400, { action: "updateGroup", error: t.adminRequestsInvalidPayload } satisfies FormState);
    }
    const items: Array<{ require_key: string; revision: number }> = [];
    const seen = new Set<string>();
    for (const item of rawItems) {
      if (!item || typeof item !== "object") return fail(400, { action: "updateGroup", error: t.adminRequestsInvalidPayload } satisfies FormState);
      const record = item as Record<string, unknown>;
      const key = typeof record.require_key === "string" ? text(record.require_key, 160) : "";
      const revision = typeof record.revision === "number" ? record.revision : revisionValue(typeof record.revision === "string" ? record.revision : "");
      if (!validKey(key) || revision === null || seen.has(key)) return fail(400, { action: "updateGroup", error: t.adminRequestsInvalidPayload } satisfies FormState);
      seen.add(key);
      items.push({ require_key: key, revision });
    }
    const failure = await mutate(event, "/api/v2/admin/media-requests/batch", "PUT", { status, note, items }, null, "updateGroup", t.adminRequestsUpdateFailed);
    if (failure) return failure;
    redirectToList(form, "updated");
  },

  delete: async (event) => {
    const form = await event.request.formData();
    const key = formText(form, "require_key", 160);
    const revision = revisionValue(formText(form, "revision", 24));
    if (!validKey(key) || revision === null) return fail(400, { action: "delete", error: t.adminRequestsInvalidPayload } satisfies FormState);
    const failure = await mutate(event, `/api/v2/admin/media-requests/by-key/${encodeURIComponent(key)}`, "DELETE", undefined, revision, "delete", t.adminRequestsDeleteFailed);
    if (failure) return failure;
    redirectToList(form, "deleted");
  }
};
