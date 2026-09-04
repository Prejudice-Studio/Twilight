import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import type { AdminTicketListResponse } from "$lib/types";
import { t } from "$lib/i18n";

type FormState = { action?: string; error?: string };
type FormFailure = ActionFailure<FormState>;

const validStatuses = new Set(["", "all", "open", "in_progress", "resolved", "closed"]);
const validPriorities = new Set(["", "all", "low", "medium", "high", "urgent"]);

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

function normalizeQuery(url: URL) {
  const rawStatus = text(url.searchParams.get("status"), 24);
  const rawPriority = text(url.searchParams.get("priority"), 24);
  const page = Math.max(1, Math.min(1_000_000, safeInteger(text(url.searchParams.get("page"), 16), 1)));
  const perPageValue = safeInteger(text(url.searchParams.get("per_page"), 8), 20);
  const uid = Math.max(0, safeInteger(text(url.searchParams.get("uid"), 24), 0));
  return {
    page,
    per_page: [20, 50, 100].includes(perPageValue) ? perPageValue : 20,
    status: validStatuses.has(rawStatus) ? rawStatus : "",
    type: text(url.searchParams.get("type"), 50),
    priority: validPriorities.has(rawPriority) ? rawPriority : "",
    uid
  };
}

function queryString(query: ReturnType<typeof normalizeQuery>, page = query.page): string {
  const params = new URLSearchParams();
  params.set("page", String(Math.max(1, page)));
  params.set("per_page", String(query.per_page));
  for (const name of ["status", "type", "priority"] as const) {
    if (query[name]) params.set(name, query[name]);
  }
  if (query.uid > 0) params.set("uid", String(query.uid));
  return params.toString();
}

function formQuery(form: FormData) {
  const url = new URL("http://twilight.invalid/admin/tickets");
  for (const name of ["page", "per_page", "status", "type", "priority", "uid"]) {
    const value = name === "status" ? (form.get("filter_status") ?? form.get(name)) : form.get(name);
    if (typeof value === "string") url.searchParams.set(name, value);
  }
  return normalizeQuery(url);
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

function redirectToList(form: FormData): never {
  throw redirect(303, `/admin/tickets?${queryString(formQuery(form))}`);
}

function requireTicketID(form: FormData): number | FormFailure {
  const id = safeInteger(formText(form, "ticket_id", 24), 0);
  return id > 0 ? id : fail(400, { action: formText(form, "action"), error: t.adminTicketsInvalidTicketID } satisfies FormState);
}

export const load: PageServerLoad = async (event) => {
  const rawTicket = text(event.url.searchParams.get("ticket"), 24);
  if (rawTicket) {
    const id = safeInteger(rawTicket, 0);
    if (id > 0) throw redirect(303, `/admin/tickets/${id}`);
  }
  const query = normalizeQuery(event.url);
  const params = new URLSearchParams(queryString(query));
  if (query.status === "all") params.set("all", "1");
  const result = await apiJSON<AdminTicketListResponse>(event, `/api/v1/admin/tickets?${params.toString()}`, { cache: "no-store" });
  return {
    payload: result?.success ? result.data || null : null,
    query,
    loadError: result?.success ? null : t.adminTicketsLoadFailed
  } satisfies { payload: AdminTicketListResponse | null; query: typeof query; loadError: string | null };
};

export const actions: Actions = {
  quickStatus: async (event) => {
    const form = await event.request.formData();
    const id = requireTicketID(form);
    if (typeof id !== "number") return id;
    const status = formText(form, "status", 24);
    if (!["open", "in_progress", "resolved", "closed"].includes(status)) {
      return fail(400, { action: "quickStatus", error: t.adminTicketsInvalidStatus } satisfies FormState);
    }
    const result = await mutate(event, `/api/v1/admin/tickets/${id}`, "PUT", { status }, "quickStatus", t.adminTicketsUpdateFailed);
    if (result.error) return result.error;
    redirectToList(form);
  },

  delete: async (event) => {
    const form = await event.request.formData();
    const id = requireTicketID(form);
    if (typeof id !== "number") return id;
    const result = await mutate(event, `/api/v1/admin/tickets/${id}`, "DELETE", undefined, "delete", t.adminTicketsDeleteFailed);
    if (result.error) return result.error;
    redirectToList(form);
  },

  addType: async (event) => {
    const form = await event.request.formData();
    const name = formText(form, "name", 50);
    if (!name) return fail(400, { action: "addType", error: t.adminTicketsTypeRequired } satisfies FormState);
    const result = await mutate(event, "/api/v1/admin/ticket-types", "POST", { name }, "addType", t.adminTicketsTypeOperationFailed);
    if (result.error) return result.error;
    redirectToList(form);
  },

  deleteType: async (event) => {
    const form = await event.request.formData();
    const name = formText(form, "name", 50);
    if (!name) return fail(400, { action: "deleteType", error: t.adminTicketsTypeRequired } satisfies FormState);
    const result = await mutate(event, "/api/v1/admin/ticket-types", "DELETE", { name }, "deleteType", t.adminTicketsTypeOperationFailed);
    if (result.error) return result.error;
    redirectToList(form);
  },

  renameType: async (event) => {
    const form = await event.request.formData();
    const oldName = formText(form, "old_name", 50);
    const newName = formText(form, "new_name", 50);
    if (!oldName || !newName) return fail(400, { action: "renameType", error: t.adminTicketsTypeRequired } satisfies FormState);
    const result = await mutate(event, "/api/v1/admin/ticket-types", "PUT", { old_name: oldName, new_name: newName }, "renameType", t.adminTicketsTypeOperationFailed);
    if (result.error) return result.error;
    redirectToList(form);
  }
};
