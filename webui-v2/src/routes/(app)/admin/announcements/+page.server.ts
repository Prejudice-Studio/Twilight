import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import type { AdminAnnouncementsPageData, AdminAnnouncementPage } from "$lib/types";
import { t } from "$lib/i18n";

type FormState = { action?: string; error?: string };
type FormFailure = ActionFailure<FormState>;
type AnnouncementQuery = AdminAnnouncementsPageData["query"];

const levels = new Set(["info", "notice", "warning", "critical"]);
const renderModes = new Set(["plain", "markdown", "bbcode"]);

function text(value: string | null | undefined, max = 200): string {
  return (value || "").trim().slice(0, max);
}

function formText(form: FormData, name: string, max = 200): string {
  const value = form.get(name);
  return typeof value === "string" ? value.trim().slice(0, max) : "";
}

function integer(value: string, fallback = 0): number {
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) ? parsed : fallback;
}

function booleanField(form: FormData, name: string, fallback: boolean): boolean {
  const values = form.getAll(name);
  if (values.length === 0) return fallback;
  return values.some((value) => value === "true" || value === "on" || value === "1");
}

function booleanQuery(value: string | null, fallback: boolean): boolean {
  if (value === null || value === "") return fallback;
  return value === "true" || value === "1";
}

function normalizeQuery(url: URL): AnnouncementQuery {
  const page = Math.max(1, Math.min(1_000_000, integer(text(url.searchParams.get("page"), 12), 1)));
  const perPageValue = integer(text(url.searchParams.get("per_page"), 8), 20);
  return {
    page,
    per_page: [20, 50, 100].includes(perPageValue) ? perPageValue : 20,
    include_invisible: booleanQuery(url.searchParams.get("include_invisible"), true),
    include_expired: booleanQuery(url.searchParams.get("include_expired"), true)
  };
}

function queryString(query: AnnouncementQuery): string {
  const params = new URLSearchParams({
    page: String(query.page),
    per_page: String(query.per_page),
    include_invisible: String(query.include_invisible),
    include_expired: String(query.include_expired)
  });
  return params.toString();
}

function queryFromForm(form: FormData): AnnouncementQuery {
  const url = new URL("http://twilight.invalid/admin/announcements");
  for (const name of ["page", "per_page", "include_invisible", "include_expired"]) {
    const value = form.get(name);
    if (typeof value === "string") url.searchParams.set(name, value);
  }
  return normalizeQuery(url);
}

function redirectToList(form: FormData, notice: string): never {
  throw redirect(303, `/admin/announcements?${queryString(queryFromForm(form))}&notice=${notice}`);
}

function expiryValue(form: FormData): number {
  const raw = formText(form, "expires_at", 64);
  if (!raw) return -1;
  const numeric = Number(raw);
  if (Number.isSafeInteger(numeric)) return Math.max(-1, numeric);
  const parsed = Date.parse(raw);
  return Number.isFinite(parsed) ? Math.floor(parsed / 1000) : -1;
}

function formPayload(form: FormData): { payload?: Record<string, unknown>; failure?: FormFailure } {
  const content = formText(form, "content", 10000);
  if (!content) return { failure: fail(400, { action: "save", error: t.adminAnnouncementsContentRequired } satisfies FormState) };
  const level = formText(form, "level", 24);
  const renderMode = formText(form, "render_mode", 24);
  if (!levels.has(level) || !renderModes.has(renderMode)) {
    return { failure: fail(400, { action: "save", error: t.adminAnnouncementsOperationFailed } satisfies FormState) };
  }
  const forceReadSeconds = Math.max(0, Math.min(31 * 24 * 60 * 60, integer(formText(form, "force_read_seconds", 12), 0)));
  return {
    payload: {
      title: formText(form, "title", 200) || undefined,
      content,
      level,
      render_mode: renderMode,
      pinned: booleanField(form, "pinned", false),
      visible: booleanField(form, "visible", true),
      force_read: booleanField(form, "force_read", false),
      force_read_seconds: forceReadSeconds,
      expires_at: expiryValue(form)
    }
  };
}

async function mutate<T>(event: RequestEvent, path: string, method: "POST" | "PUT" | "DELETE", payload: unknown, action: string): Promise<{ data?: T; failure?: FormFailure }> {
  const result = await apiJSONWithResponse<T>(event, path, {
    method,
    ...(payload === undefined ? {} : { headers: { "content-type": "application/json" }, body: JSON.stringify(payload) })
  });
  if (!result?.response.ok || !result.envelope?.success) {
    return { failure: fail(result?.response.status || 503, { action, error: result?.envelope?.message || t.adminAnnouncementsOperationFailed } satisfies FormState) };
  }
  return { data: result.envelope.data };
}

export const load: PageServerLoad = async (event): Promise<AdminAnnouncementsPageData> => {
  const query = normalizeQuery(event.url);
  const params = new URLSearchParams({
    page: String(query.page),
    per_page: String(query.per_page),
    include_invisible: String(query.include_invisible),
    include_expired: String(query.include_expired)
  });
  const result = await apiJSON<AdminAnnouncementPage>(event, `/api/v1/admin/announcements?${params}`, { cache: "no-store" });
  const rawNotice = text(event.url.searchParams.get("notice"), 16);
  const notice = ["created", "updated", "deleted", "hidden", "shown", "pinned", "unpinned"].includes(rawNotice)
    ? rawNotice as AdminAnnouncementsPageData["notice"]
    : "";
  return {
    payload: result?.success ? result.data || null : null,
    query,
    notice,
    loadError: result?.success ? null : t.adminAnnouncementsLoadFailed
  };
};

export const actions: Actions = {
  save: async (event) => {
    const form = await event.request.formData();
    const parsed = formPayload(form);
    if (parsed.failure || !parsed.payload) return parsed.failure;
    const id = Math.max(0, integer(formText(form, "announcement_id", 24), 0));
    const result = await mutate(event, id > 0 ? `/api/v1/admin/announcements/${id}` : "/api/v1/admin/announcements", id > 0 ? "PUT" : "POST", parsed.payload, "save");
    if (result.failure) return result.failure;
    redirectToList(form, id > 0 ? "updated" : "created");
  },

  toggleVisible: async (event) => {
    const form = await event.request.formData();
    const id = integer(formText(form, "announcement_id", 24), 0);
    if (id <= 0) return fail(400, { action: "toggleVisible", error: t.adminAnnouncementsOperationFailed } satisfies FormState);
    const visible = booleanField(form, "visible", false);
    const result = await mutate(event, `/api/v1/admin/announcements/${id}`, "PUT", { visible }, "toggleVisible");
    if (result.failure) return result.failure;
    redirectToList(form, visible ? "shown" : "hidden");
  },

  togglePinned: async (event) => {
    const form = await event.request.formData();
    const id = integer(formText(form, "announcement_id", 24), 0);
    if (id <= 0) return fail(400, { action: "togglePinned", error: t.adminAnnouncementsOperationFailed } satisfies FormState);
    const pinned = booleanField(form, "pinned", false);
    const result = await mutate(event, `/api/v1/admin/announcements/${id}`, "PUT", { pinned }, "togglePinned");
    if (result.failure) return result.failure;
    redirectToList(form, pinned ? "pinned" : "unpinned");
  },

  delete: async (event) => {
    const form = await event.request.formData();
    const id = integer(formText(form, "announcement_id", 24), 0);
    if (id <= 0) return fail(400, { action: "delete", error: t.adminAnnouncementsOperationFailed } satisfies FormState);
    const result = await mutate(event, `/api/v1/admin/announcements/${id}`, "DELETE", undefined, "delete");
    if (result.failure) return result.failure;
    redirectToList(form, "deleted");
  }
};
