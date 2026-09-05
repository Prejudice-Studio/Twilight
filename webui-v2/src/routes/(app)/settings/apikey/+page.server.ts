import { fail, redirect } from "@sveltejs/kit";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import { t } from "$lib/i18n";
import type { ApiKeyItem, MyApiKeysPageData } from "$lib/types";

type FormState = { action?: "create" | "update" | "delete"; error?: string; created?: { id: number; name: string; key: string } };
type FormFailure = ActionFailure<FormState>;

function text(form: FormData, name: string, max: number): string {
  const value = form.get(name);
  return typeof value === "string" ? value.trim().slice(0, max) : "";
}

function integer(form: FormData, name: string): number {
  const value = text(form, name, 16);
  return Number.isSafeInteger(Number(value)) ? Number(value) : -1;
}

function boolean(form: FormData, name: string): boolean {
  return form.getAll(name).some((value) => value === "true" || value === "on" || value === "1");
}

function keyID(form: FormData): number {
  const value = integer(form, "key_id");
  return value > 0 ? value : 0;
}

function invalid(action: FormState["action"], message: string = t.apiKeyOperationFailed): FormFailure {
  return fail(400, { action, error: message } satisfies FormState);
}

async function update(event: RequestEvent, keyId: number, payload: Record<string, unknown>): Promise<FormFailure | null> {
  const result = await apiJSONWithResponse(event, `/api/v1/users/me/apikeys/${keyId}`, {
    method: "PUT",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(payload)
  });
  return !result?.response.ok || !result.envelope?.success ? fail(result?.response.status || 503, { action: "update", error: t.apiKeyOperationFailed } satisfies FormState) : null;
}

export const load: PageServerLoad = async (event): Promise<MyApiKeysPageData> => {
  const result = await apiJSON<{ keys?: ApiKeyItem[]; total?: number }>(event, "/api/v1/users/me/apikeys", { cache: "no-store" });
  return {
    keys: result?.success && Array.isArray(result.data?.keys) ? result.data.keys : [],
    total: result?.success ? result.data?.total || 0 : 0,
    loadError: result?.success ? null : t.apiKeyLoadFailed,
    notice: event.url.searchParams.get("notice") === "updated" || event.url.searchParams.get("notice") === "deleted"
      ? event.url.searchParams.get("notice") as MyApiKeysPageData["notice"]
      : ""
  };
};

export const actions: Actions = {
  create: async (event) => {
    const form = await event.request.formData();
    const name = text(form, "name", 120);
    const rateLimit = integer(form, "rate_limit");
    if (!name) return invalid("create", t.apiKeyNameRequired);
    if (rateLimit < 0 || rateLimit > 100000) return invalid("create", t.apiKeyRateLimitInvalid);
    const result = await apiJSONWithResponse<{ id?: number; name?: string; key?: string }>(event, "/api/v1/users/me/apikeys", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ name, allow_query: boolean(form, "allow_query"), rate_limit: rateLimit })
    });
    if (!result?.response.ok || !result.envelope?.success || !result.envelope.data?.key || !result.envelope.data.id) {
      return fail(result?.response.status || 503, { action: "create", error: t.apiKeyOperationFailed } satisfies FormState);
    }
    return { action: "create", created: { id: result.envelope.data.id, name: result.envelope.data.name || name, key: result.envelope.data.key } } satisfies FormState;
  },

  update: async (event) => {
    const form = await event.request.formData();
    const keyId = keyID(form);
    const name = text(form, "name", 120);
    const rateLimit = integer(form, "rate_limit");
    if (!keyId || !name) return invalid("update", t.apiKeyNameRequired);
    if (rateLimit < 0 || rateLimit > 100000) return invalid("update", t.apiKeyRateLimitInvalid);
    const result = await update(event, keyId, { name, enabled: boolean(form, "enabled"), allow_query: boolean(form, "allow_query"), rate_limit: rateLimit });
    if (result) return result;
    throw redirect(303, "/settings/apikey?notice=updated");
  },

  delete: async (event) => {
    const form = await event.request.formData();
    const keyId = keyID(form);
    if (!keyId) return invalid("delete");
    const result = await apiJSONWithResponse(event, `/api/v1/users/me/apikeys/${keyId}`, { method: "DELETE" });
    if (!result?.response.ok || !result.envelope?.success) return fail(result?.response.status || 503, { action: "delete", error: t.apiKeyOperationFailed } satisfies FormState);
    throw redirect(303, "/settings/apikey?notice=deleted");
  }
};
