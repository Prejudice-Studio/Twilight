import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import type { AdminViolationsPageData, ViolationLogPage } from "$lib/types";
import { t } from "$lib/i18n";

type FormState = { action?: string; error?: string };
type FormFailure = ActionFailure<FormState>;
type ViolationQuery = AdminViolationsPageData["query"];

const violationTypes = new Set(["", "all", "regcode_decoy", "regcode_target_mismatch"]);

function text(value: FormDataEntryValue | string | null | undefined, max = 200): string {
  return (typeof value === "string" ? value : "").trim().slice(0, max);
}

function integer(value: string | null | undefined, fallback: number): number {
  const parsed = Number(value || "");
  return Number.isSafeInteger(parsed) ? parsed : fallback;
}

function normalizeQuery(url: URL): ViolationQuery {
  const page = Math.max(1, Math.min(integer(url.searchParams.get("page"), 1), 1_000_000));
  const perPageValue = integer(url.searchParams.get("per_page"), 20);
  const type = text(url.searchParams.get("type"), 48);
  return {
    page,
    per_page: [20, 50, 100].includes(perPageValue) ? perPageValue : 20,
    type: violationTypes.has(type) ? type : "",
    search: text(url.searchParams.get("search"), 200)
  };
}

function queryString(query: ViolationQuery, overrides: Partial<ViolationQuery> = {}): string {
  const value = { ...query, ...overrides };
  const params = new URLSearchParams();
  if (value.page > 1) params.set("page", String(value.page));
  if (value.per_page !== 20) params.set("per_page", String(value.per_page));
  if (value.type && value.type !== "all") params.set("type", value.type);
  if (value.search) params.set("search", value.search);
  return params.toString();
}

function queryFromForm(form: FormData): ViolationQuery {
  const url = new URL("http://twilight.invalid/admin/violations");
  for (const name of ["page", "per_page", "type", "search"]) {
    const value = form.get(name);
    if (typeof value === "string") url.searchParams.set(name, value);
  }
  return normalizeQuery(url);
}

async function mutate(
  event: RequestEvent,
  path: string,
  method: "POST" | "DELETE",
  payload: unknown,
  action: string,
  fallbackMessage: string
): Promise<{ failure?: FormFailure }> {
  const result = await apiJSONWithResponse(event, path, {
    method,
    ...(payload === undefined
      ? {}
      : { headers: { "content-type": "application/json" }, body: JSON.stringify(payload) })
  });
  if (!result?.response.ok || !result.envelope?.success) {
    return { failure: fail(result?.response.status || 503, { action, error: fallbackMessage }) };
  }
  return {};
}

function redirectToList(form: FormData, notice: "deleted" | "cleared"): never {
  const query = queryString(queryFromForm(form));
  throw redirect(303, `/admin/violations${query ? `?${query}&notice=${notice}` : `?notice=${notice}`}`);
}

export const load: PageServerLoad = async (event): Promise<AdminViolationsPageData> => {
  const query = normalizeQuery(event.url);
  const params = new URLSearchParams({ page: String(query.page), per_page: String(query.per_page) });
  if (query.type && query.type !== "all") params.set("type", query.type);
  if (query.search) params.set("search", query.search);
  const result = await apiJSON<ViolationLogPage>(event, `/api/v1/admin/violations?${params}`, { cache: "no-store" });
  const noticeValue = text(event.url.searchParams.get("notice"), 16);
  const notice = noticeValue === "deleted" || noticeValue === "cleared" ? noticeValue : "";
  return {
    payload: result?.success ? result.data || null : null,
    query,
    notice,
    loadError: result?.success ? null : t.adminViolationsLoadFailed
  };
};

export const actions: Actions = {
  deleteViolation: async (event) => {
    const form = await event.request.formData();
    const rawID = text(form.get("violation_id"), 24);
    const id = integer(rawID, 0);
    if (!/^\d+$/.test(rawID) || id <= 0) {
      return fail(400, { action: "deleteViolation", error: t.adminViolationsDeleteFailed } satisfies FormState);
    }
    const result = await mutate(event, `/api/v1/admin/violations/${id}`, "DELETE", undefined, "deleteViolation", t.adminViolationsDeleteFailed);
    if (result.failure) return result.failure;
    redirectToList(form, "deleted");
  },

  clearViolations: async (event) => {
    const form = await event.request.formData();
    if (text(form.get("confirm"), 32) !== "CLEAR_VIOLATIONS") {
      return fail(400, { action: "clearViolations", error: t.adminViolationsClearFailed } satisfies FormState);
    }
    const result = await mutate(
      event,
      "/api/v1/admin/violations/clear",
      "POST",
      { confirm: "CLEAR_VIOLATIONS" },
      "clearViolations",
      t.adminViolationsClearFailed
    );
    if (result.failure) return result.failure;
    redirectToList(form, "cleared");
  }
};
