import { fail, redirect } from "@sveltejs/kit";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import { t } from "$lib/i18n";
import type { ApiEnvelope, AdminDeveloperPageData, DeveloperJSDocs, DeveloperJSPreviewResult, DeveloperJSPreset } from "$lib/types";

type FormState = { action?: "preview" | "save" | "delete"; error?: string; preview?: DeveloperJSPreviewResult };
type FormFailure = ActionFailure<FormState>;

function text(form: FormData, name: string, max: number): string {
  const value = form.get(name);
  return typeof value === "string" ? value.trim().slice(0, max) : "";
}

function id(form: FormData): number {
  const value = text(form, "preset_id", 16);
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : 0;
}

function boolean(form: FormData, name: string): boolean {
  return form.getAll(name).some((value) => value === "true" || value === "on" || value === "1");
}

function command(value: string): string {
  const name = value.trim().replace(/^\/+/, "").toLowerCase().replace(/[^a-z0-9_]/g, "").slice(0, 32);
  return name ? `/${name}` : "/preview";
}

function failure(action: FormState["action"], message: string = t.adminDeveloperOperationFailed): FormFailure {
  return fail(400, { action, error: message } satisfies FormState);
}

function read<T>(result: PromiseSettledResult<ApiEnvelope<T>>): T | null {
  return result.status === "fulfilled" && result.value.success ? result.value.data || null : null;
}

export const load: PageServerLoad = async (event): Promise<AdminDeveloperPageData> => {
  const results = await Promise.allSettled([
    apiJSON<{ presets?: DeveloperJSPreset[]; total?: number; developer_mode_enabled?: boolean }>(event, "/api/v2/admin/developer/js-presets", { cache: "no-store" }),
    apiJSON<DeveloperJSDocs>(event, "/api/v2/admin/developer/js-docs", { cache: "no-store" })
  ]);
  const presetData = read(results[0] as PromiseSettledResult<ApiEnvelope<{ presets?: DeveloperJSPreset[]; total?: number; developer_mode_enabled?: boolean }>>);
  const docs = read(results[1] as PromiseSettledResult<ApiEnvelope<DeveloperJSDocs>>);
  const developerModeEnabled = presetData?.developer_mode_enabled === true || docs !== null;
  return {
    developerModeEnabled,
    presets: Array.isArray(presetData?.presets) ? presetData.presets : [],
    docs,
    loadError: presetData || docs ? null : t.adminDeveloperLoadFailed,
    notice: event.url.searchParams.get("notice") === "saved" || event.url.searchParams.get("notice") === "deleted"
      ? event.url.searchParams.get("notice") as AdminDeveloperPageData["notice"]
      : ""
  };
};

export const actions: Actions = {
  preview: async (event) => {
    const form = await event.request.formData();
    const code = text(form, "code", 8000);
    if (!code) return failure("preview", t.adminDeveloperCodeRequired);
    if (new TextEncoder().encode(code).byteLength > 8000) return failure("preview", t.adminDeveloperCodeTooLong);
    const args = text(form, "args_text", 2400);
    const result = await apiJSONWithResponse<DeveloperJSPreviewResult>(event, "/api/v2/admin/developer/js-sandbox", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ code, command: command(text(form, "command", 80)), args_text: args, private_chat: boolean(form, "private_chat") })
    });
    if (!result?.response.ok || !result.envelope?.success || !result.envelope.data) {
      return fail(result?.response.status || 503, { action: "preview", error: t.adminDeveloperPreviewFailed } satisfies FormState);
    }
    return { action: "preview", preview: result.envelope.data } satisfies FormState;
  },

  save: async (event) => {
    const form = await event.request.formData();
    const presetID = id(form);
    const name = text(form, "name", 80);
    const description = text(form, "description", 500);
    const code = text(form, "code", 8000);
    if (!name) return failure("save", t.adminDeveloperNameRequired);
    if (new TextEncoder().encode(code).byteLength > 8000) return failure("save", t.adminDeveloperCodeTooLong);
    const path = presetID > 0 ? `/api/v2/admin/developer/js-presets/${presetID}` : "/api/v2/admin/developer/js-presets";
    const result = await apiJSONWithResponse(event, path, {
      method: presetID > 0 ? "PUT" : "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ name, description, code })
    });
    if (!result?.response.ok || !result.envelope?.success) {
      return fail(result?.response.status || 503, { action: "save", error: t.adminDeveloperOperationFailed } satisfies FormState);
    }
    throw redirect(303, "/admin/developer?notice=saved");
  },

  delete: async (event) => {
    const presetID = id(await event.request.formData());
    if (!presetID) return failure("delete", t.adminDeveloperPresetRequired);
    const result = await apiJSONWithResponse(event, `/api/v2/admin/developer/js-presets/${presetID}`, { method: "DELETE" });
    if (!result?.response.ok || !result.envelope?.success) {
      return fail(result?.response.status || 503, { action: "delete", error: t.adminDeveloperOperationFailed } satisfies FormState);
    }
    throw redirect(303, "/admin/developer?notice=deleted");
  }
};
