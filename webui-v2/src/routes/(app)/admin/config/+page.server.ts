import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import type {
  AdminConfigPageData,
  ApiEnvelope,
  ConfigBackupList,
  ConfigBackupView,
  ConfigRestoreResult,
  ConfigSchema,
  ConfigToml
} from "$lib/types";
import { t } from "$lib/i18n";

type FormState = {
  action?: string;
  success?: boolean;
  message?: string;
  error?: string;
  backupView?: ConfigBackupView;
  restorePreview?: ConfigRestoreResult;
};
type FormFailure = ActionFailure<FormState>;
type ParsedSections = Record<string, Record<string, unknown>>;

const maxTomlBytes = 4 * 1024 * 1024;

function text(value: FormDataEntryValue | null, max = 200): string {
  return typeof value === "string" ? value.trim().slice(0, max) : "";
}

function failResult(action: string, response: Response | undefined, message: string): FormFailure {
  return fail(response?.status || 503, { action, error: message } satisfies FormState);
}

async function jsonMutation<T>(
  event: RequestEvent,
  path: string,
  method: "POST" | "PUT" | "DELETE",
  payload: unknown,
  action: string,
  fallback: string
): Promise<{ data?: T; failure?: FormFailure; message?: string }> {
  const result = await apiJSONWithResponse<T>(event, path, {
    method,
    headers: { "content-type": "application/json" },
    body: payload === undefined ? undefined : JSON.stringify(payload)
  });
  if (!result?.response.ok || !result.envelope?.success) {
    return { failure: failResult(action, result?.response, result?.envelope?.message || fallback) };
  }
  return { data: result.envelope.data, message: result.envelope.message };
}

function parseSections(form: FormData): { value?: ParsedSections; failure?: FormFailure } {
  const raw = form.get("sections");
  if (typeof raw !== "string" || raw.length === 0 || new TextEncoder().encode(raw).byteLength > maxTomlBytes) {
    return { failure: fail(400, { action: "saveSchema", error: t.adminConfigSaveFailed } satisfies FormState) };
  }
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("invalid sections");
    for (const fields of Object.values(parsed as Record<string, unknown>)) {
      if (!fields || typeof fields !== "object" || Array.isArray(fields)) throw new Error("invalid section");
    }
    return { value: parsed as ParsedSections };
  } catch {
    return { failure: fail(400, { action: "saveSchema", error: t.adminConfigSaveFailed } satisfies FormState) };
  }
}

function backupName(form: FormData): string {
  const name = text(form.get("name"), 160);
  return /^[A-Za-z0-9._-]+\.toml$/i.test(name) ? name : "";
}

function readEnvelope<T>(result: PromiseSettledResult<ApiEnvelope<T>>): T | null {
  return result.status === "fulfilled" && result.value.success ? result.value.data || null : null;
}

export const load: PageServerLoad = async (event): Promise<AdminConfigPageData> => {
  const results = await Promise.allSettled([
    apiJSON<ConfigSchema>(event, "/api/v1/system/admin/config/schema", { cache: "no-store" }),
    apiJSON<ConfigToml>(event, "/api/v1/system/admin/config/toml", { cache: "no-store" }),
    apiJSON<ConfigBackupList>(event, "/api/v1/system/admin/config/backups", { cache: "no-store" })
  ]);
  const schema = readEnvelope(results[0] as PromiseSettledResult<ApiEnvelope<ConfigSchema>>);
  const toml = readEnvelope(results[1] as PromiseSettledResult<ApiEnvelope<ConfigToml>>);
  const backups = readEnvelope(results[2] as PromiseSettledResult<ApiEnvelope<ConfigBackupList>>);
  const errors: string[] = [];
  if (!schema) errors.push(t.adminConfigNoSchema);
  if (!toml) errors.push(t.adminConfigNoToml);
  if (!backups) errors.push(t.adminConfigBackupsLoadFailed);
  return { schema, toml, backups, errors };
};

export const actions: Actions = {
  saveSchema: async (event) => {
    const parsed = parseSections(await event.request.formData());
    if (parsed.failure || !parsed.value) return parsed.failure;
    const result = await jsonMutation(event, "/api/v1/system/admin/config/schema", "PUT", { sections: parsed.value }, "saveSchema", t.adminConfigSaveFailed);
    if (result.failure) return result.failure;
    throw redirect(303, "/admin/config");
  },

  saveToml: async (event) => {
    const form = await event.request.formData();
    const content = form.get("content");
    if (typeof content !== "string" || new TextEncoder().encode(content).byteLength > maxTomlBytes) {
      return fail(413, { action: "saveToml", error: t.adminConfigSaveFailed } satisfies FormState);
    }
    const result = await jsonMutation(event, "/api/v1/system/admin/config/toml", "PUT", { content }, "saveToml", t.adminConfigSaveFailed);
    if (result.failure) return result.failure;
    throw redirect(303, "/admin/config");
  },

  createBackup: async (event) => {
    const result = await jsonMutation(event, "/api/v1/system/admin/config/backup", "POST", {}, "createBackup", t.adminConfigSaveFailed);
    if (result.failure) return result.failure;
    throw redirect(303, "/admin/config");
  },

  inspectBackup: async (event) => {
    const name = backupName(await event.request.formData());
    if (!name) return fail(400, { action: "inspectBackup", error: t.adminConfigBackupsLoadFailed } satisfies FormState);
    const result = await apiJSONWithResponse<ConfigBackupView>(event, `/api/v1/system/admin/config/backups/${encodeURIComponent(name)}`, { cache: "no-store" });
    if (!result?.response.ok || !result.envelope?.success || !result.envelope.data) return failResult("inspectBackup", result?.response, t.adminConfigBackupsLoadFailed);
    return { action: "inspectBackup", backupView: result.envelope.data } satisfies FormState;
  },

  deleteBackup: async (event) => {
    const name = backupName(await event.request.formData());
    if (!name) return fail(400, { action: "deleteBackup", error: t.adminConfigBackupsLoadFailed } satisfies FormState);
    const result = await jsonMutation(event, `/api/v1/system/admin/config/backups/${encodeURIComponent(name)}`, "DELETE", undefined, "deleteBackup", t.adminConfigBackupsLoadFailed);
    if (result.failure) return result.failure;
    throw redirect(303, "/admin/config");
  },

  restorePreview: async (event) => {
    const name = backupName(await event.request.formData());
    if (!name) return fail(400, { action: "restorePreview", error: t.adminConfigBackupsLoadFailed } satisfies FormState);
    const result = await jsonMutation<ConfigRestoreResult>(event, "/api/v1/system/admin/config/restore", "POST", { name, dry_run: true }, "restorePreview", t.adminConfigBackupsLoadFailed);
    if (result.failure) return result.failure;
    return { action: "restorePreview", restorePreview: result.data } satisfies FormState;
  },

  restore: async (event) => {
    const form = await event.request.formData();
    const name = backupName(form);
    if (!name || text(form.get("confirm"), 64) !== "RESTORE_CONFIG_BACKUP") return fail(400, { action: "restore", error: t.adminConfigRestoreConfirm } satisfies FormState);
    const result = await jsonMutation(event, "/api/v1/system/admin/config/restore", "POST", { name, confirm: "RESTORE_CONFIG_BACKUP" }, "restore", t.adminConfigSaveFailed);
    if (result.failure) return result.failure;
    throw redirect(303, "/admin/config");
  },

  sweep: async (event) => {
    const result = await jsonMutation(event, "/api/v1/system/admin/config/sweep", "POST", {}, "sweep", t.adminConfigSaveFailed);
    if (result.failure) return result.failure;
    throw redirect(303, "/admin/config");
  },

  uploadBackground: async (event) => {
    const form = await event.request.formData();
    const file = form.get("file");
    if (typeof file === "string" || !file || typeof file.arrayBuffer !== "function" || file.size <= 0 || file.size > 5 * 1024 * 1024) {
      return fail(400, { action: "uploadBackground", error: t.adminConfigUploadHelp } satisfies FormState);
    }
    const body = new FormData();
    body.set("file", file, file.name || "background");
    const result = await apiJSONWithResponse(event, "/api/v1/system/admin/config/upload-auth-background", { method: "POST", body });
    if (!result?.response.ok || !result.envelope?.success) return failResult("uploadBackground", result?.response, t.adminConfigSaveFailed);
    throw redirect(303, "/admin/config");
  }
};
