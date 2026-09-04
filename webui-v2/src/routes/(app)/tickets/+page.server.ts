import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import type { Ticket, TicketSummary } from "$lib/types";
import { t } from "$lib/i18n";

type TicketListData = {
  tickets: TicketSummary[];
  total: number;
  page: number;
  per_page: number;
  ticket_types: string[];
};

type TicketDetailData = { ticket: Ticket; ticket_types: string[] };

type FormState = { error?: string; action?: string };

function text(form: FormData, name: string): string {
  return String(form.get(name) ?? "").trim();
}

function checked(form: FormData, name: string): boolean {
  return form.getAll(name).some((value) => value === "true" || value === "on");
}

async function mutate(event: RequestEvent, path: string, method: string, payload: Record<string, unknown>, action: string, fallback: string) {
  const result = await apiJSONWithResponse<unknown>(event, path, {
    method,
    headers: { "content-type": "application/json" },
    body: JSON.stringify(payload)
  });
  const response = result?.response;
  const envelope = result?.envelope;
  if (!response || !envelope || !response.ok || !envelope.success) {
    return fail(response?.status || 503, { action, error: envelope?.message || fallback } satisfies FormState);
  }
  return envelope;
}

export const load: PageServerLoad = async (event) => {
  const page = Math.max(1, Number(event.url.searchParams.get("page") || 1));
  const selectedID = Math.max(0, Number(event.url.searchParams.get("ticket") || 0));
  const list = await apiJSON<TicketListData>(event, `/api/v1/tickets?page=${page}&per_page=20`, { cache: "no-store" });
  let detail: TicketDetailData | null = null;
  if (selectedID > 0) {
    const response = await apiJSON<TicketDetailData>(event, `/api/v1/tickets/${selectedID}`, { cache: "no-store" });
    if (response?.success && response.data) detail = response.data;
  }
  return {
    list: list?.success ? list.data || null : null,
    detail,
    selectedID,
    loadError: list?.success ? null : t.ticketListLoadError
  };
};

export const actions: Actions = {
  create: async (event) => {
    const form = await event.request.formData();
    const title = text(form, "title");
    const content = text(form, "content");
    const draft = { title, content, type: text(form, "type"), priority: text(form, "priority") };
    if (!title) return fail(400, { action: "create", error: t.ticketNeedTitle, draft } satisfies FormState & { draft: typeof draft });
    if (!content) return fail(400, { action: "create", error: t.ticketNeedContent, draft } satisfies FormState & { draft: typeof draft });
    const result = await mutate(event, "/api/v1/tickets", "POST", {
      title,
      content,
      ...(draft.type ? { type: draft.type } : {}),
      ...(draft.priority ? { priority: draft.priority } : {}),
      notify_telegram: checked(form, "notify_telegram")
    }, "create", t.ticketCreateFailed);
    if (result instanceof Response) return result;
    if ("status" in result && typeof result.status === "number") return result;
    throw redirect(303, "/tickets");
  },

  reply: async (event) => {
    const form = await event.request.formData();
    const id = Number(text(form, "ticket_id"));
    const content = text(form, "content");
    if (!Number.isSafeInteger(id) || id <= 0 || !content) return fail(400, { action: "reply", error: t.ticketReplyEmpty } satisfies FormState);
    const result = await mutate(event, `/api/v1/tickets/${id}/reply`, "POST", { content }, "reply", t.ticketReplyFailed);
    if (result instanceof Response) return result;
    if ("status" in result && typeof result.status === "number") return result;
    throw redirect(303, `/tickets?ticket=${id}`);
  },

  close: async (event) => {
    const id = Number((await event.request.formData()).get("ticket_id"));
    if (!Number.isSafeInteger(id) || id <= 0) return fail(400, { action: "close", error: t.ticketNumberInvalid } satisfies FormState);
    const result = await mutate(event, `/api/v1/tickets/${id}/close`, "POST", {}, "close", t.ticketCloseFailed);
    if (result instanceof Response) return result;
    if ("status" in result && typeof result.status === "number") return result;
    throw redirect(303, `/tickets?ticket=${id}`);
  },

  reopen: async (event) => {
    const id = Number((await event.request.formData()).get("ticket_id"));
    if (!Number.isSafeInteger(id) || id <= 0) return fail(400, { action: "reopen", error: t.ticketNumberInvalid } satisfies FormState);
    const result = await mutate(event, `/api/v1/tickets/${id}/reopen`, "POST", {}, "reopen", t.ticketReopenFailed);
    if (result instanceof Response) return result;
    if ("status" in result && typeof result.status === "number") return result;
    throw redirect(303, `/tickets?ticket=${id}`);
  },

  notify: async (event) => {
    const form = await event.request.formData();
    const id = Number(text(form, "ticket_id"));
    const enabled = checked(form, "enabled");
    if (!Number.isSafeInteger(id) || id <= 0) return fail(400, { action: "notify", error: t.ticketNumberInvalid } satisfies FormState);
    const result = await mutate(event, `/api/v1/tickets/${id}/notify-telegram`, "PUT", { enabled }, "notify", t.ticketNotifyFailed);
    if (result instanceof Response) return result;
    if ("status" in result && typeof result.status === "number") return result;
    throw redirect(303, `/tickets?ticket=${id}`);
  }
};
