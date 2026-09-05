import { fail, redirect } from "@sveltejs/kit";
import type { Actions, RequestEvent } from "@sveltejs/kit";
import type { PageServerLoad } from "./$types";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { AdminTelegramRebindPageData, ApiEnvelope, TelegramRebindPage } from "$lib/types";

const pageSize = 20;
const statuses = new Set(["all", "pending", "approved", "rejected", "revoked"]);
const maxNoteBytes = 2 * 1024;
const revokeConfirm = "REVOKE_ALL_REBIND_APPROVALS";

type FormState = { action?: "review" | "batch" | "revoke"; error?: string };

function bytes(value: string): number {
  return new TextEncoder().encode(value).byteLength;
}

function parseStatus(value: string | null): AdminTelegramRebindPageData["status"] {
  const status = (value || "pending").trim().toLowerCase();
  return statuses.has(status) ? status as AdminTelegramRebindPageData["status"] : "pending";
}

function parsePage(value: string | null): number {
  const page = Number(value || 1);
  return Number.isSafeInteger(page) && page > 0 ? Math.min(page, 100000) : 1;
}

function text(value: FormDataEntryValue | null, limit: number): string {
  return typeof value === "string" ? value.trim().slice(0, limit) : "";
}

function note(value: FormDataEntryValue | null): string | null {
  const result = typeof value === "string" ? value.trim() : "";
  return bytes(result) <= maxNoteBytes ? result : null;
}

function positiveID(value: FormDataEntryValue | null): number | null {
  const result = Number(text(value, 24));
  return Number.isSafeInteger(result) && result > 0 ? result : null;
}

function formQuery(event: RequestEvent): string {
  const status = parseStatus(event.url.searchParams.get("status"));
  const page = parsePage(event.url.searchParams.get("page"));
  return `?status=${encodeURIComponent(status)}&page=${page}`;
}

function readEnvelope<T>(result: PromiseSettledResult<ApiEnvelope<T>>): T | null {
  return result.status === "fulfilled" && result.value.success ? result.value.data || null : null;
}

async function review(event: RequestEvent, id: number, action: "approve" | "reject", adminNote: string) {
  return apiJSONWithResponse(event, `/api/v1/admin/telegram/rebind-requests/${id}/${action}`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ admin_note: adminNote })
  });
}

export const load: PageServerLoad = async (event): Promise<AdminTelegramRebindPageData> => {
  const status = parseStatus(event.url.searchParams.get("status"));
  const page = parsePage(event.url.searchParams.get("page"));
  const result = await apiJSON<TelegramRebindPage>(event, `/api/v1/admin/telegram/rebind-requests?status=${encodeURIComponent(status)}&page=${page}&per_page=${pageSize}`, { cache: "no-store" });
  return {
    payload: result?.success && result.data ? result.data : null,
    status,
    page,
    perPage: pageSize,
    loadError: result?.success ? null : "换绑申请暂时无法读取，请手动刷新",
    notice: ["reviewed", "batch_reviewed", "revoked"].includes(event.url.searchParams.get("notice") || "")
      ? event.url.searchParams.get("notice") as AdminTelegramRebindPageData["notice"]
      : ""
  };
};

export const actions: Actions = {
  review: async (event) => {
    const form = await event.request.formData();
    const id = positiveID(form.get("id"));
    const action = text(form.get("review_action"), 16);
    const adminNote = note(form.get("admin_note"));
    if (!id || (action !== "approve" && action !== "reject") || adminNote === null) {
      return fail(400, { action: "review", error: "换绑申请参数无效" } satisfies FormState);
    }
    const result = await review(event, id, action, adminNote);
    if (!result?.response.ok || !result.envelope?.success) {
      return fail(result?.response.status || 503, { action: "review", error: "换绑申请处理失败，请稍后重试" } satisfies FormState);
    }
    throw redirect(303, `/admin/telegram-rebind-requests${formQuery(event)}&notice=reviewed`);
  },

  batch: async (event) => {
    const form = await event.request.formData();
    const action = text(form.get("review_action"), 16);
    const ids = form.getAll("ids").map(positiveID).filter((id): id is number => id !== null);
    const adminNote = note(form.get("admin_note"));
    if ((action !== "approve" && action !== "reject") || ids.length === 0 || ids.length > 100 || adminNote === null) {
      return fail(400, { action: "batch", error: "批量换绑申请参数无效" } satisfies FormState);
    }
    const uniqueIDs = [...new Set(ids)];
    const result = await apiJSONWithResponse(event, "/api/v1/admin/telegram/rebind-requests/batch", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ ids: uniqueIDs, action, admin_note: adminNote })
    });
    if (!result?.response.ok || !result.envelope?.success) {
      return fail(result?.response.status || 503, { action: "batch", error: "批量换绑申请处理失败，请稍后重试" } satisfies FormState);
    }
    throw redirect(303, `/admin/telegram-rebind-requests${formQuery(event)}&notice=batch_reviewed`);
  },

  revoke: async (event) => {
    const form = await event.request.formData();
    const confirmation = text(form.get("confirm"), 64);
    if (confirmation !== revokeConfirm) {
      return fail(400, { action: "revoke", error: "请输入正确的换绑许可撤销确认短语" } satisfies FormState);
    }
    const adminNote = note(form.get("admin_note"));
    if (adminNote === null) return fail(400, { action: "revoke", error: "处理备注长度无效" } satisfies FormState);
    const result = await apiJSONWithResponse<{ revoked: number }>(event, "/api/v1/admin/telegram/rebind-requests/revoke-approved", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ admin_note: adminNote })
    });
    if (!result?.response.ok || !result.envelope?.success) {
      return fail(result?.response.status || 503, { action: "revoke", error: "撤销换绑许可失败，请稍后重试" } satisfies FormState);
    }
    throw redirect(303, `/admin/telegram-rebind-requests${formQuery(event)}&notice=revoked`);
  }
};
