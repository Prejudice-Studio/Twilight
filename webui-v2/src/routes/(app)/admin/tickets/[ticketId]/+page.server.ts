import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import type { AdminTicketDetailResponse, TicketStatus, TicketPriority } from "$lib/types";
import { t } from "$lib/i18n";

type FormState = { action?: string; error?: string };
type FormFailure = ActionFailure<FormState>;

const statuses = new Set<TicketStatus>(["open", "in_progress", "resolved", "closed"]);
const priorities = new Set<TicketPriority>(["low", "medium", "high", "urgent"]);

function text(value: FormDataEntryValue | null, max = 200): string {
  return typeof value === "string" ? value.trim().slice(0, max) : "";
}

function idFromParams(value: string | undefined): number {
  const id = Number(value || 0);
  return Number.isSafeInteger(id) && id > 0 ? id : 0;
}

function idFromForm(form: FormData): number {
  return idFromParams(text(form.get("ticket_id"), 24));
}

async function mutate<T>(
  event: RequestEvent,
  path: string,
  method: "POST" | "PATCH" | "DELETE",
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

function redirectToDetail(id: number): never {
  throw redirect(303, `/admin/tickets/${id}`);
}

export const load: PageServerLoad = async (event) => {
  const id = idFromParams(event.params.ticketId);
  if (!id) throw redirect(303, "/admin/tickets");
  const result = await apiJSON<AdminTicketDetailResponse>(event, `/api/v2/admin/tickets/${id}`, { cache: "no-store" });
  return {
    payload: result?.success ? result.data || null : null,
    loadError: result?.success ? null : t.adminTicketsLoadFailed,
    ticketID: id
  } satisfies { payload: AdminTicketDetailResponse | null; loadError: string | null; ticketID: number };
};

export const actions: Actions = {
  updateMeta: async (event) => {
    const form = await event.request.formData();
    const id = idFromForm(form);
    if (!id) return fail(400, { action: "updateMeta", error: t.adminTicketsInvalidTicketID } satisfies FormState);
    const field = text(form.get("field"), 20);
    const value = text(form.get("value"), field === "admin_note" ? 5000 : 60);
    if (field === "status" && !statuses.has(value as TicketStatus)) return fail(400, { action: "updateMeta", error: t.adminTicketsInvalidStatus } satisfies FormState);
    if (field === "priority" && !priorities.has(value as TicketPriority)) return fail(400, { action: "updateMeta", error: t.adminTicketsInvalidPriority } satisfies FormState);
    if (field === "type" && !value) return fail(400, { action: "updateMeta", error: t.adminTicketsTypeRequired } satisfies FormState);
    if (!["status", "priority", "type", "admin_note"].includes(field)) return fail(400, { action: "updateMeta", error: t.adminTicketsInvalidMetadataField } satisfies FormState);
    const result = await mutate(event, `/api/v2/admin/tickets/${id}`, "PATCH", { [field]: value }, "updateMeta", t.adminTicketsUpdateFailed);
    if (result.error) return result.error;
    redirectToDetail(id);
  },

  reply: async (event) => {
    const form = await event.request.formData();
    const id = idFromForm(form);
    const content = text(form.get("content"), 5000);
    if (!id) return fail(400, { action: "reply", error: t.adminTicketsInvalidTicketID } satisfies FormState);
    if (!content) return fail(400, { action: "reply", error: t.adminTicketsReplyRequired } satisfies FormState);
    const result = await mutate(event, `/api/v2/admin/tickets/${id}/replies`, "POST", { content }, "reply", t.adminTicketsReplyFailed);
    if (result.error) return result.error;
    redirectToDetail(id);
  },

  uploadImage: async (event) => {
    const form = await event.request.formData();
    const id = idFromForm(form);
    const file = form.get("file");
    if (!id) return fail(400, { action: "uploadImage", error: t.adminTicketsInvalidTicketID } satisfies FormState);
    if (typeof file === "string" || !file || typeof file.arrayBuffer !== "function" || file.size <= 0) {
      return fail(400, { action: "uploadImage", error: t.adminTicketsImageRequired } satisfies FormState);
    }
    const body = new FormData();
    body.set("file", file, file.name || "attachment");
    const result = await apiJSONWithResponse<unknown>(event, `/api/v2/admin/tickets/${id}/attachments`, { method: "POST", body });
    const response = result?.response;
    const envelope = result?.envelope;
    if (!response || !envelope || !response.ok || !envelope.success) {
      return fail(response?.status || 503, { action: "uploadImage", error: envelope?.message || t.adminTicketsImageUploadFailed } satisfies FormState);
    }
    redirectToDetail(id);
  },

  deleteImage: async (event) => {
    const form = await event.request.formData();
    const id = idFromForm(form);
    const filename = text(form.get("filename"), 80);
    if (!id || !filename) return fail(400, { action: "deleteImage", error: t.adminTicketsImageInvalid } satisfies FormState);
    const result = await mutate(event, `/api/v2/admin/tickets/${id}/attachments/${encodeURIComponent(filename)}`, "DELETE", undefined, "deleteImage", t.adminTicketsImageDeleteFailed);
    if (result.error) return result.error;
    redirectToDetail(id);
  },

  delete: async (event) => {
    const form = await event.request.formData();
    const id = idFromForm(form);
    if (!id) return fail(400, { action: "delete", error: t.adminTicketsInvalidTicketID } satisfies FormState);
    const result = await mutate(event, `/api/v2/admin/tickets/${id}`, "DELETE", undefined, "delete", t.adminTicketsDeleteFailed);
    if (result.error) return result.error;
    throw redirect(303, "/admin/tickets");
  }
};
