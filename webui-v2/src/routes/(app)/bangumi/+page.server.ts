import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import { t } from "$lib/i18n";
import type { Actions, PageServerLoad } from "./$types";
import type { BangumiSummary } from "$lib/types";

type FormState = { action?: string; error?: string };

function text(form: FormData, name: string): string {
  return String(form.get(name) ?? "").trim();
}

function booleanValue(form: FormData, name: string): boolean {
  return text(form, name) === "true";
}

async function mutate(
  event: Parameters<NonNullable<Actions["sync"]>>[0],
  path: string,
  method: "POST" | "DELETE" | "PUT",
  body: Record<string, unknown> | undefined,
  action: string,
  fallback: string
) {
  const result = await apiJSONWithResponse(event, path, {
    method,
    headers: body ? { "content-type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined
  });
  const response = result?.response;
  const envelope = result?.envelope;
  if (!response || !envelope || !response.ok || !envelope.success) {
    return fail(response?.status || 503, { action, error: envelope?.message || fallback } satisfies FormState);
  }
  throw redirect(303, `/bangumi?result=${encodeURIComponent(action)}`);
}

export const load: PageServerLoad = async (event) => {
  const result = await apiJSON<BangumiSummary>(event, "/api/v2/bangumi/summary", { cache: "no-store" });
  return {
    payload: result?.success ? result.data || null : null,
    loadError: result?.success ? null : t.bangumiCollectionLoadFailed,
    result: event.url.searchParams.get("result") || ""
  };
};

export const actions: Actions = {
  sync: async (event) => mutate(event, "/api/v2/bangumi/sync", "POST", {}, "sync", t.bangumiSyncFailed),

  clearHistory: async (event) => mutate(event, "/api/v2/bangumi/sync/history", "DELETE", undefined, "clear-history", t.bangumiClearFailed),

  saveSettings: async (event) => {
    const form = await event.request.formData();
    const payload: Record<string, unknown> = {};
    if (form.has("bgm_mode")) payload.bgm_mode = booleanValue(form, "bgm_mode");
    if (form.has("bgm_manage_mode")) payload.bgm_manage_mode = booleanValue(form, "bgm_manage_mode");
    const token = text(form, "bgm_token");
    if (token) payload.bgm_token = token;
    if (Object.keys(payload).length === 0) {
      return fail(400, { action: "settings", error: t.bangumiSettingsFailed } satisfies FormState);
    }
    return mutate(event, "/api/v2/bangumi/preferences", "PUT", payload, "settings", t.bangumiSettingsFailed);
  },

  clearToken: async (event) => mutate(event, "/api/v2/bangumi/preferences", "PUT", {
    bgm_token: ""
  }, "clear-token", t.bangumiClearFailed)
};
