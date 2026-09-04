import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import type { AdminDatabasePageData, ApiEnvelope, DatabaseBackup, DatabaseBackupInspectResult, DatabaseOperationResult, DatabaseStatus } from "$lib/types";
import { t } from "$lib/i18n";

type FormState = {
  action?: string;
  error?: string;
  backupPreview?: DatabaseBackupInspectResult;
  restorePreview?: DatabaseOperationResult;
  migrationPreview?: DatabaseOperationResult;
};
type FormFailure = ActionFailure<FormState>;

const restoreConfirm = "RESTORE_DATABASE_BACKUP";
const migrateConfirm = "MIGRATE_DATABASE";

function text(value: FormDataEntryValue | null, max = 200): string {
  return typeof value === "string" ? value.trim().slice(0, max) : "";
}

function failResult(action: string, response: Response | undefined, message: string): FormFailure {
  return fail(response?.status || 503, { action, error: message });
}

async function mutation<T>(
  event: RequestEvent,
  path: string,
  method: "POST" | "DELETE",
  payload: unknown,
  action: string
): Promise<{ data?: T; failure?: FormFailure }> {
  const result = await apiJSONWithResponse<T>(event, path, {
    method,
    headers: { "content-type": "application/json" },
    body: payload === undefined ? undefined : JSON.stringify(payload)
  });
  if (!result?.response.ok || !result.envelope?.success) {
    return { failure: failResult(action, result?.response, result?.envelope?.message || t.adminDatabaseOperationFailed) };
  }
  return { data: result.envelope.data };
}

function backupName(form: FormData): string {
  const value = text(form.get("name"), 180);
  return /^[A-Za-z0-9._-]+$/.test(value) ? value : "";
}

function readEnvelope<T>(result: PromiseSettledResult<ApiEnvelope<T>>): T | null {
  return result.status === "fulfilled" && result.value.success ? result.value.data || null : null;
}

export const load: PageServerLoad = async (event): Promise<AdminDatabasePageData> => {
  const results = await Promise.allSettled([
    apiJSON<DatabaseStatus>(event, "/api/v1/system/admin/database/status", { cache: "no-store" }),
    apiJSON<{ backups: DatabaseBackup[] }>(event, "/api/v1/system/admin/database/backups", { cache: "no-store" })
  ]);
  const status = readEnvelope(results[0] as PromiseSettledResult<ApiEnvelope<DatabaseStatus>>);
  const backupData = readEnvelope(results[1] as PromiseSettledResult<ApiEnvelope<{ backups: DatabaseBackup[] }>>);
  const errors: string[] = [];
  if (!status) errors.push(t.adminDatabaseNoStatus);
  if (!backupData) errors.push(t.adminDatabaseNoBackups);
  return { status, backups: backupData?.backups || [], errors };
};

export const actions: Actions = {
  createBackup: async (event) => {
    const form = await event.request.formData();
    const result = await mutation(event, "/api/v1/system/admin/database/backup", "POST", { note: text(form.get("note"), 200) }, "createBackup");
    if (result.failure) return result.failure;
    throw redirect(303, "/admin/database");
  },

  inspectBackup: async (event) => {
    const name = backupName(await event.request.formData());
    if (!name) return fail(400, { action: "inspectBackup", error: t.adminDatabaseOperationFailed } satisfies FormState);
    const result = await apiJSONWithResponse<DatabaseBackupInspectResult>(event, `/api/v1/system/admin/database/backups/${encodeURIComponent(name)}`, { cache: "no-store" });
    if (!result?.response.ok || !result.envelope?.success || !result.envelope.data) return failResult("inspectBackup", result?.response, t.adminDatabaseOperationFailed);
    return { action: "inspectBackup", backupPreview: result.envelope.data } satisfies FormState;
  },

  deleteBackup: async (event) => {
    const name = backupName(await event.request.formData());
    if (!name) return fail(400, { action: "deleteBackup", error: t.adminDatabaseOperationFailed } satisfies FormState);
    const result = await mutation(event, `/api/v1/system/admin/database/backups/${encodeURIComponent(name)}`, "DELETE", undefined, "deleteBackup");
    if (result.failure) return result.failure;
    throw redirect(303, "/admin/database");
  },

  restorePreview: async (event) => {
    const name = backupName(await event.request.formData());
    if (!name) return fail(400, { action: "restorePreview", error: t.adminDatabaseOperationFailed } satisfies FormState);
    const result = await mutation<DatabaseOperationResult>(event, "/api/v1/system/admin/database/restore", "POST", { name, dry_run: true }, "restorePreview");
    if (result.failure) return result.failure;
    return { action: "restorePreview", restorePreview: result.data } satisfies FormState;
  },

  restore: async (event) => {
    const form = await event.request.formData();
    const name = backupName(form);
    if (!name || text(form.get("confirm"), 64) !== restoreConfirm) return fail(400, { action: "restore", error: t.adminDatabaseRestoreConfirm } satisfies FormState);
    const result = await mutation(event, "/api/v1/system/admin/database/restore", "POST", { name, confirm: restoreConfirm }, "restore");
    if (result.failure) return result.failure;
    throw redirect(303, "/admin/database");
  },

  migrationPreview: async (event) => {
    const form = await event.request.formData();
    const target = ["postgres", "json"].includes(text(form.get("target_driver"), 20)) ? text(form.get("target_driver"), 20) : "json";
    const stateFile = text(form.get("state_file"), 240);
    const result = await mutation<DatabaseOperationResult>(event, "/api/v1/system/admin/database/migrate", "POST", { target_driver: target, ...(stateFile ? { state_file: stateFile } : {}), dry_run: true }, "migrationPreview");
    if (result.failure) return result.failure;
    return { action: "migrationPreview", migrationPreview: result.data } satisfies FormState;
  },

  migrate: async (event) => {
    const form = await event.request.formData();
    const target = ["postgres", "json"].includes(text(form.get("target_driver"), 20)) ? text(form.get("target_driver"), 20) : "json";
    const stateFile = text(form.get("state_file"), 240);
    if (text(form.get("confirm"), 64) !== migrateConfirm) return fail(400, { action: "migrate", error: t.adminDatabaseMigrationHelp } satisfies FormState);
    const result = await mutation<DatabaseOperationResult>(event, "/api/v1/system/admin/database/migrate", "POST", { target_driver: target, ...(stateFile ? { state_file: stateFile } : {}), confirm: migrateConfirm }, "migrate");
    if (result.failure) return result.failure;
    throw redirect(303, "/admin/database");
  }
};
