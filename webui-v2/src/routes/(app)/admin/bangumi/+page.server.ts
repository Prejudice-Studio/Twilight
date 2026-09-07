import { fail, redirect } from "@sveltejs/kit";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import { t } from "$lib/i18n";
import type {
  AdminBangumiDetail,
  AdminBangumiPageData,
  AdminBangumiUsersResult,
  BangumiSyncLog,
  PlaybackRecordWithSync,
  V2Capabilities
} from "$lib/types";

type FormState = { action?: "sync" | "clearLogs"; error?: string };
type FormFailure = ActionFailure<FormState>;
type Query = AdminBangumiPageData["query"];

const detailKinds = new Set(["records", "logs"]);
const pageSizes = new Set([20, 50, 100]);

function text(value: string | null | undefined, max = 200): string {
  return (value || "").trim().slice(0, max);
}

function formText(form: FormData, name: string, max = 200): string {
  const value = form.get(name);
  return typeof value === "string" ? text(value, max) : "";
}

function integer(value: string | null | undefined, fallback: number): number {
  const parsed = Number(value || "");
  return Number.isSafeInteger(parsed) ? parsed : fallback;
}

function uidFromText(value: string | null | undefined): number {
  const raw = text(value, 24);
  const parsed = Number(raw);
  return /^\d+$/.test(raw) && Number.isSafeInteger(parsed) && parsed > 0 ? parsed : 0;
}

function normalizeQuery(url: URL): Query {
  const page = Math.max(1, Math.min(integer(url.searchParams.get("page"), 1), 1_000_000));
  const perPageValue = integer(url.searchParams.get("per_page"), 20);
  const detailValue = text(url.searchParams.get("detail"), 16);
  const uid = uidFromText(url.searchParams.get("uid"));
  return {
    page,
    per_page: pageSizes.has(perPageValue) ? perPageValue : 20,
    search: text(url.searchParams.get("search"), 100),
    detail: detailKinds.has(detailValue) && uid > 0 ? detailValue as Query["detail"] : "",
    uid: detailKinds.has(detailValue) ? uid : 0
  };
}

function queryString(query: Query, overrides: Partial<Query> = {}): string {
  const value = { ...query, ...overrides };
  const params = new URLSearchParams();
  if (value.page > 1) params.set("page", String(value.page));
  if (value.per_page !== 20) params.set("per_page", String(value.per_page));
  if (value.search) params.set("search", value.search);
  if (value.detail && value.uid > 0) {
    params.set("detail", value.detail);
    params.set("uid", String(value.uid));
  }
  return params.toString();
}

function queryFromForm(form: FormData): Query {
  const url = new URL("http://twilight.invalid/admin/bangumi");
  for (const name of ["page", "per_page", "search", "detail", "uid"]) {
    const value = form.get(name);
    if (typeof value === "string") url.searchParams.set(name, value);
  }
  return normalizeQuery(url);
}

function failure(action: FormState["action"], message: string = t.adminBangumiOperationFailed): FormFailure {
  return fail(400, { action, error: message } satisfies FormState);
}

function redirectToList(form: FormData, notice: AdminBangumiPageData["notice"]): never {
  const query = queryString(queryFromForm(form));
  throw redirect(303, `/admin/bangumi${query ? `?${query}&notice=${notice}` : `?notice=${notice}`}`);
}

async function readDetail(event: RequestEvent, query: Query, users: AdminBangumiUserLookup): Promise<AdminBangumiDetail | null> {
  if (!query.detail || query.uid <= 0) return null;
  const user = users.get(query.uid) || null;
  if (query.detail === "records") {
    const result = await apiJSON<{ records?: PlaybackRecordWithSync[] }>(event, `/api/v2/admin/bangumi/users/${query.uid}/records?limit=200`, { cache: "no-store" });
    return {
      kind: "records",
      uid: query.uid,
      user,
      records: result?.success && Array.isArray(result.data?.records) ? result.data.records : [],
      logs: [],
      error: result?.success ? null : t.adminBangumiOperationFailed
    };
  }
  const result = await apiJSON<{ logs?: BangumiSyncLog[] }>(event, `/api/v2/admin/bangumi/users/${query.uid}/logs?limit=200`, { cache: "no-store" });
  return {
    kind: "logs",
    uid: query.uid,
    user,
    records: [],
    logs: result?.success && Array.isArray(result.data?.logs) ? result.data.logs : [],
    error: result?.success ? null : t.adminBangumiOperationFailed
  };
}

type AdminBangumiUser = AdminBangumiUsersResult["users"][number];
type AdminBangumiUserLookup = Map<number, AdminBangumiUser>;

export const load: PageServerLoad = async (event): Promise<AdminBangumiPageData> => {
  const query = normalizeQuery(event.url);
  const params = new URLSearchParams({ page: String(query.page), per_page: String(query.per_page) });
  if (query.search) params.set("search", query.search);
  const userPromise = apiJSON<AdminBangumiUsersResult>(event, `/api/v2/admin/bangumi/users?${params}`, { cache: "no-store" });
  const infoPromise = apiJSON<V2Capabilities>(event, "/api/v2/system/capabilities", { cache: "no-store" });
  const usersResult = await userPromise;
  const infoResult = await infoPromise;
  const users = usersResult?.success && usersResult.data ? usersResult.data : null;
  const info = infoResult?.success && infoResult.data ? infoResult.data : null;
  const lookup: AdminBangumiUserLookup = new Map((users?.users || []).map((user) => [user.uid, user]));
  const detail = await readDetail(event, query, lookup);
  const noticeValue = text(event.url.searchParams.get("notice"), 20);
  const notice = noticeValue === "synced" || noticeValue === "logs_cleared" ? noticeValue : "";
  return {
    info,
    users,
    detail,
    query,
    notice,
    loadError: users ? null : t.adminBangumiLoadFailed
  };
};

async function mutate(event: RequestEvent, path: string, method: "POST" | "DELETE", action: FormState["action"]): Promise<void | FormFailure> {
  const result = await apiJSONWithResponse(event, path, { method });
  if (!result?.response.ok || !result.envelope?.success) return failure(action);
}

export const actions: Actions = {
  sync: async (event) => {
    const form = await event.request.formData();
    const uid = uidFromText(formText(form, "uid", 24));
    if (!uid) return failure("sync", t.adminBangumiInvalidUser);
    const result = await mutate(event, `/api/v2/admin/bangumi/users/${uid}/sync`, "POST", "sync");
    if (result) return result;
    redirectToList(form, "synced");
  },

  clearLogs: async (event) => {
    const form = await event.request.formData();
    const uid = uidFromText(formText(form, "uid", 24));
    if (!uid) return failure("clearLogs", t.adminBangumiInvalidUser);
    const result = await mutate(event, `/api/v2/admin/bangumi/users/${uid}/logs`, "DELETE", "clearLogs");
    if (result) return result;
    redirectToList(form, "logs_cleared");
  }
};
