import { fail } from "@sveltejs/kit";
import type { Actions, PageServerLoad, RequestEvent } from "./$types";
import { apiJSONWithResponse } from "$lib/server/api";
import { t } from "$lib/i18n";
import type { AdminMigrationPageData, MigrationStatus, MigrationSummary } from "$lib/types";

type FormState = { action?: "preview" | "import"; error?: string; summary?: MigrationSummary };
const confirmPhrase = "IMPORT_TWILIGHT_DATA";
const maxPasswordBytes = 1024;
const maxArchiveBytes = 528 * 1024 * 1024;

function text(form: FormData, name: string, max = 2048): string {
  const value = form.get(name);
  return typeof value === "string" ? value.trim().slice(0, max) : "";
}

function checked(form: FormData, name: string): boolean {
  return ["true", "1", "on", "yes"].includes(text(form, name, 12).toLowerCase());
}

function archiveFile(form: FormData): File | null {
  const value = form.get("archive");
  if (typeof value === "string" || !value || typeof (value as File).arrayBuffer !== "function") return null;
  const file = value as File;
  return file.size > 0 && file.size <= maxArchiveBytes ? file : null;
}

function importForm(form: FormData, includeConfirmation: boolean): FormData | null {
  const file = archiveFile(form);
  const password = text(form, "password", 1024);
  const resourceMode = text(form, "resource_mode", 20);
  if (!file || new TextEncoder().encode(password).byteLength > maxPasswordBytes) return null;
  if (resourceMode !== "preserve" && resourceMode !== "replace") return null;
  const body = new FormData();
  body.set("archive", file, file.name || "twilight-migration.zip");
  body.set("password", password);
  body.set("resource_mode", resourceMode);
  if (checked(form, "apply_config")) body.set("apply_config", "true");
  if (includeConfirmation) body.set("confirm", confirmPhrase);
  else body.set("preview", "true");
  return body;
}

async function forwardImport(event: RequestEvent, form: FormData, action: "preview" | "import") {
  const body = importForm(form, action === "import");
  if (!body) return fail(400, { action, error: t.adminMigrationOperationFailed } satisfies FormState);
  const result = await apiJSONWithResponse<MigrationSummary>(event, "/api/v2/admin/migration/import", { method: "POST", body });
  if (!result?.response.ok || !result.envelope?.success || !result.envelope.data) {
    return fail(result?.response.status || 503, { action, error: t.adminMigrationOperationFailed } satisfies FormState);
  }
  return { action, summary: result.envelope.data } satisfies FormState;
}

export const load: PageServerLoad = async (event): Promise<AdminMigrationPageData> => {
  const result = await apiJSONWithResponse<MigrationStatus>(event, "/api/v2/admin/migration/status", { cache: "no-store" });
  if (result?.response.status === 403) return { status: { enabled: false, format_version: "", database_schema_version: "", max_archive_bytes: 0, resource_namespaces: [] }, error: null };
  if (!result?.response.ok || !result.envelope?.success || !result.envelope.data) return { status: null, error: t.adminMigrationUnavailable };
  return { status: result.envelope.data, error: null };
};

export const actions: Actions = {
  preview: async (event) => forwardImport(event, await event.request.formData(), "preview"),
  import: async (event) => {
    const form = await event.request.formData();
    if (text(form, "confirm", 64) !== confirmPhrase) return fail(400, { action: "import", error: t.adminMigrationOperationFailed } satisfies FormState);
    return forwardImport(event, form, "import");
  }
};
