import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import type { AdminUserListResponse, AdminUsersPageData, UserInfo } from "$lib/types";
import { t } from "$lib/i18n";

type FormState = {
  action?: string;
  success?: boolean;
  error?: string;
  created?: { username: string; password: string };
};
type FormFailure = ActionFailure<FormState>;

const maxSearchLength = 100;
const validRoles = new Set(["", "0", "1", "2"]);
const validActive = new Set(["", "true", "false"]);
const validEmby = new Set(["", "bound", "unbound"]);
const validEmbyStatus = new Set(["", "active", "disabled"]);
const validEmailStatus = new Set(["", "verified", "unverified", "bound", "none"]);
const validSort = new Set(["", "uid_asc", "uid_desc", "username_asc", "username_desc", "expire_asc", "expire_desc", "created_asc", "created_desc"]);

function text(value: string | null | undefined, max = 200): string {
  return (value || "").trim().slice(0, max);
}

function formText(form: FormData, name: string, max = 200): string {
  const value = form.get(name);
  return typeof value === "string" ? text(value, max) : "";
}

function safeInteger(value: string, fallback = 0): number {
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) ? parsed : fallback;
}

function bool(value: string): boolean {
  return value === "true" || value === "on" || value === "1";
}

function normalizeQuery(url: URL): AdminUsersPageData["query"] {
  const raw = (name: string, max = 32) => text(url.searchParams.get(name), max);
  const page = Math.max(1, safeInteger(raw("page"), 1));
  const perPageValue = safeInteger(raw("per_page"), 20);
  const per_page = [20, 50, 100].includes(perPageValue) ? perPageValue : 20;
  const roleValue = raw("role");
  const activeValue = raw("active");
  const embyValue = raw("emby");
  const embyStatusValue = raw("emby_status");
  const emailStatusValue = raw("email_status");
  const sortValue = raw("sort");
  return {
    page,
    per_page,
    search: raw("search", maxSearchLength),
    role: validRoles.has(roleValue) ? roleValue : "",
    active: validActive.has(activeValue) ? activeValue : "",
    emby: validEmby.has(embyValue) ? embyValue : "",
    emby_status: validEmbyStatus.has(embyStatusValue) ? embyStatusValue : "",
    email_status: validEmailStatus.has(emailStatusValue) ? emailStatusValue : "",
    sort: validSort.has(sortValue) ? sortValue : "uid_asc"
  };
}

function queryString(query: AdminUsersPageData["query"], page = query.page): string {
  const params = new URLSearchParams();
  params.set("page", String(Math.max(1, page)));
  params.set("per_page", String(query.per_page));
  for (const name of ["search", "role", "active", "emby", "emby_status", "email_status", "sort"] as const) {
    if (query[name]) params.set(name, query[name]);
  }
  return params.toString();
}

function formQuery(form: FormData): AdminUsersPageData["query"] {
  const url = new URL("http://twilight.invalid/admin/users");
  for (const name of ["page", "per_page", "search", "role", "active", "emby", "emby_status", "email_status", "sort"]) {
    const value = form.get(name);
    if (typeof value === "string") url.searchParams.set(name, value);
  }
  return normalizeQuery(url);
}

function actionPath(uid: number, suffix: string): string {
  return `/api/v2/admin/users/${uid}${suffix}`;
}

async function mutate<T>(
  event: RequestEvent,
  path: string,
  method: "POST" | "PUT" | "DELETE",
  payload: Record<string, unknown> | undefined,
  action: string,
  fallback: string
): Promise<{ data?: T; error?: FormFailure }> {
  const result = await apiJSONWithResponse<T>(event, path, {
    method,
    ...(payload ? { headers: { "content-type": "application/json" }, body: JSON.stringify(payload) } : {})
  });
  const response = result?.response;
  const envelope = result?.envelope;
  if (!response || !envelope || !response.ok || !envelope.success) {
    return { error: fail(response?.status || 503, { action, error: envelope?.message || fallback } satisfies FormState) };
  }
  return { data: envelope.data };
}

function redirectToList(query: AdminUsersPageData["query"]): never {
  throw redirect(303, `/admin/users?${queryString(query)}`);
}

function requireUID(form: FormData): number | FormFailure {
  const uid = safeInteger(formText(form, "uid", 24));
  return uid > 0 ? uid : fail(400, { action: formText(form, "action"), error: t.adminUsersInvalidUID } satisfies FormState);
}

export const load: PageServerLoad = async (event) => {
  const query = normalizeQuery(event.url);
  const result = await apiJSON<AdminUserListResponse>(event, `/api/v2/admin/users?${queryString(query)}`, { cache: "no-store" });
  return {
    payload: result?.success ? result.data || null : null,
    query,
    loadError: result?.success ? null : t.adminUsersLoadFailed
  } satisfies { payload: AdminUserListResponse | null; query: typeof query; loadError: string | null };
};

export const actions: Actions = {
  toggle: async (event) => {
    const form = await event.request.formData();
    const uid = requireUID(form);
    if (typeof uid !== "number") return uid;
    const enable = bool(formText(form, "enable"));
    const result = await mutate<UserInfo>(event, actionPath(uid, enable ? "/enable" : "/disable"), "POST", {
      ...(formText(form, "reason", 200) ? { reason: formText(form, "reason", 200) } : {}),
      cascade_depth: Math.max(-1, Math.min(100, safeInteger(formText(form, "cascade_depth", 8), 1)))
    }, "toggle", t.adminUsersToggleFailed);
    if (result.error) return result.error;
    redirectToList(formQuery(form));
  },

  renew: async (event) => {
    const form = await event.request.formData();
    const uid = requireUID(form);
    if (typeof uid !== "number") return uid;
    const rawDays = safeInteger(formText(form, "days", 12), 0);
    if (rawDays < -1 || rawDays > 36500 || rawDays === 0) return fail(400, { action: "renew", error: t.adminUsersRenewDaysInvalid } satisfies FormState);
    const result = await mutate<UserInfo>(event, actionPath(uid, "/renew"), "POST", { days: rawDays }, "renew", t.adminUsersRenewFailed);
    if (result.error) return result.error;
    redirectToList(formQuery(form));
  },

  embyToggle: async (event) => {
    const form = await event.request.formData();
    const uid = requireUID(form);
    if (typeof uid !== "number") return uid;
    const enable = bool(formText(form, "enable"));
    const result = await mutate(event, actionPath(uid, `/emby/${enable ? "enable" : "disable"}`), "POST", {
      ...(formText(form, "reason", 200) ? { reason: formText(form, "reason", 200) } : {})
    }, "emby-toggle", t.adminUsersEmbyToggleFailed);
    if (result.error) return result.error;
    redirectToList(formQuery(form));
  },

  refresh: async (event) => {
    const form = await event.request.formData();
    const uid = requireUID(form);
    if (typeof uid !== "number") return uid;
    const scope = ["telegram", "emby", "both"].includes(formText(form, "scope")) ? formText(form, "scope") : "both";
    const result = await mutate(event, actionPath(uid, "/refresh-status"), "POST", { scope }, "refresh", t.adminUsersRefreshFailed);
    if (result.error) return result.error;
    redirectToList(formQuery(form));
  },

  unbindTelegram: async (event) => {
    const form = await event.request.formData();
    const uid = requireUID(form);
    if (typeof uid !== "number") return uid;
    const result = await mutate(event, actionPath(uid, "/unbind-telegram"), "POST", {}, "unbind-telegram", t.adminUsersUnbindTelegramFailed);
    if (result.error) return result.error;
    redirectToList(formQuery(form));
  },

  unbindEmby: async (event) => {
    const form = await event.request.formData();
    const uid = requireUID(form);
    if (typeof uid !== "number") return uid;
    const result = await mutate(event, actionPath(uid, "/force-unbind"), "POST", { scope: "emby" }, "unbind-emby", t.adminUsersUnbindEmbyFailed);
    if (result.error) return result.error;
    redirectToList(formQuery(form));
  },

  role: async (event) => {
    const form = await event.request.formData();
    const uid = requireUID(form);
    if (typeof uid !== "number") return uid;
    const result = await mutate(event, actionPath(uid, "/admin"), "PUT", { admin: bool(formText(form, "admin")) }, "role", t.adminUsersRoleFailed);
    if (result.error) return result.error;
    redirectToList(formQuery(form));
  },

  delete: async (event) => {
    const form = await event.request.formData();
    const uid = requireUID(form);
    if (typeof uid !== "number") return uid;
    const mode = ["local_only", "with_emby", "emby_only"].includes(formText(form, "mode")) ? formText(form, "mode") : "local_only";
    const depth = Math.max(-1, Math.min(100, safeInteger(formText(form, "cascade_depth", 8), 1)));
    const result = await mutate(event, actionPath(uid, "/delete"), "POST", { mode, cascade_depth: depth }, "delete", t.adminUsersDeleteFailed);
    if (result.error) return result.error;
    redirectToList(formQuery(form));
  },

  create: async (event) => {
    const form = await event.request.formData();
    const username = formText(form, "username", 64);
    const password = formText(form, "password", 256);
    const email = formText(form, "email", 254);
    const role = ["0", "1", "2"].includes(formText(form, "role")) ? safeInteger(formText(form, "role"), 1) : 1;
    const days = safeInteger(formText(form, "days", 12), 30);
    if (!username) return fail(400, { action: "create", error: t.adminUsersUsernameRequired } satisfies FormState);
    if (days < -1 || days > 36500) return fail(400, { action: "create", error: t.adminUsersDaysInvalid } satisfies FormState);
    const result = await mutate<{ user: UserInfo; password: string }>(event, "/api/v2/admin/users", "POST", {
      username,
      ...(password ? { password } : {}),
      ...(email ? { email } : {}),
      role,
      days
    }, "create", t.adminUsersCreateFailed);
    if (result.error) return result.error;
    if (!result.data?.password) return fail(502, { action: "create", error: t.adminUsersCreateNoPassword } satisfies FormState);
    return { action: "create", success: true, created: { username: result.data.user?.username || username, password: result.data.password } } satisfies FormState;
  }
};
