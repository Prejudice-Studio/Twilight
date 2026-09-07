import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { Actions, PageServerLoad } from "./$types";
import type { InviteCodeItem, V2InviteSummary } from "$lib/types";
import { t } from "$lib/i18n";

type FormState = {
  action?: string;
  error?: string;
  success?: boolean;
  generated?: {
    code: string;
    target_username: string;
    days: number;
    validity_hours: number;
  };
};

function text(form: FormData, name: string): string {
  const value = form.get(name);
  return typeof value === "string" ? value.trim() : "";
}

function integer(form: FormData, name: string): number {
  const value = Number(text(form, name));
  return Number.isSafeInteger(value) ? value : 0;
}

async function mutate<T>(
  event: Parameters<NonNullable<Actions["create"]>>[0],
  path: string,
  method: "POST" | "DELETE",
  body: Record<string, unknown> | undefined,
  action: string,
  fallback: string
) {
  const result = await apiJSONWithResponse<T>(event, path, {
    method,
    ...(body ? { headers: { "content-type": "application/json" }, body: JSON.stringify(body) } : {})
  });
  const response = result?.response;
  const envelope = result?.envelope;
  if (!response || !envelope || !response.ok || !envelope.success) {
    return fail(response?.status || 503, { action, error: envelope?.message || fallback } satisfies FormState);
  }
  return envelope;
}

export const load: PageServerLoad = async (event) => {
  const result = await apiJSON<V2InviteSummary>(event, "/api/v2/invite/summary", { cache: "no-store" });
  return {
    payload: result?.success ? result.data || null : null,
    loadError: result?.success ? null : t.inviteLoadFailed,
    result: event.url.searchParams.get("result") || ""
  };
};

export const actions: Actions = {
  create: async (event) => {
    const form = await event.request.formData();
    const days = integer(form, "days");
    const note = text(form, "note");
    const targetUsername = text(form, "target_username");
    if (days <= 0) return fail(400, { action: "create", error: t.inviteInvalidDays } satisfies FormState);
    const result = await mutate<InviteCodeItem>(event, "/api/v2/invite/codes", "POST", {
      days,
      ...(note ? { note } : {}),
      ...(targetUsername ? { target_username: targetUsername } : {})
    }, "create", t.inviteCreateFailed);
    if (result instanceof Response) return result;
    if ("status" in result && typeof result.status === "number") return result;
    throw redirect(303, "/invite?result=created");
  },

  deleteCode: async (event) => {
    const form = await event.request.formData();
    const code = text(form, "code");
    if (!code || code.length > 128 || /[\\/]/.test(code)) {
      return fail(400, { action: "delete-code", error: t.inviteCodeOperationFailed } satisfies FormState);
    }
    const result = await mutate<null>(event, `/api/v2/invite/codes/${encodeURIComponent(code)}`, "DELETE", undefined, "delete-code", t.inviteCodeOperationFailed);
    if (result instanceof Response) return result;
    if ("status" in result && typeof result.status === "number") return result;
    throw redirect(303, "/invite?result=deleted");
  },

  renew: async (event) => {
    const form = await event.request.formData();
    const targetUID = integer(form, "target_uid");
    const days = integer(form, "days");
    const validityHours = integer(form, "validity_hours");
    const note = text(form, "note");
    if (targetUID <= 0 || days <= 0 || validityHours <= 0) {
      return fail(400, { action: "renew", error: t.inviteInvalidDays } satisfies FormState);
    }
    const result = await mutate<{
      code: string;
      target_uid: number;
      target_username: string;
      days: number;
      validity_hours: number;
    }>(event, "/api/v2/invite/renew-codes", "POST", {
      target_uid: targetUID,
      days,
      validity_hours: validityHours,
      ...(note ? { note } : {})
    }, "renew", t.inviteRenewFailed);
    if (result instanceof Response) return result;
    if ("status" in result && typeof result.status === "number") return result;
    const generated = "data" in result ? result.data as FormState["generated"] : undefined;
    if (!generated || typeof generated.code !== "string" || typeof generated.target_username !== "string") {
      return fail(502, { action: "renew", error: t.inviteRenewFailed } satisfies FormState);
    }
    return {
      action: "renew",
      success: true,
      generated
    } satisfies FormState;
  },

  detachChild: async (event) => {
    const uid = integer(await event.request.formData(), "uid");
    if (uid <= 0) return fail(400, { action: "detach-child", error: t.inviteDetachFailed } satisfies FormState);
    const result = await mutate<unknown>(event, `/api/v2/invite/children/${uid}/detach-expired`, "POST", undefined, "detach-child", t.inviteDetachFailed);
    if (result instanceof Response) return result;
    if ("status" in result && typeof result.status === "number") return result;
    throw redirect(303, "/invite?result=detached");
  },

  detachSelf: async (event) => {
    const result = await mutate<unknown>(event, "/api/v2/invite/me/detach-expired", "POST", undefined, "detach-self", t.inviteDetachFailed);
    if (result instanceof Response) return result;
    if ("status" in result && typeof result.status === "number") return result;
    throw redirect(303, "/invite?result=self-detached");
  }
};
