import { fail, redirect } from "@sveltejs/kit";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import { t } from "$lib/i18n";
import type { AdminEmailPageData, EmailAdminData } from "$lib/types";

type EmailTestResult = { target?: string; success?: boolean; to?: string };
type FormState = {
  action?: "revoke" | "cleanup" | "clearUnverified" | "testEmail";
  error?: string;
  test?: { success: boolean; email: string };
};
type FormFailure = ActionFailure<FormState>;
type View = AdminEmailPageData["view"];
type Verified = AdminEmailPageData["verified"];

const pageSizeDefault = 25;
const validViews = new Set<View>(["pending", "accounts"]);
const validVerified = new Set<Verified>(["all", "verified", "unverified"]);

function text(value: string | null | undefined, max = 200): string {
  return (value || "").trim().slice(0, max);
}

function formText(form: FormData, name: string, max = 200): string {
  const value = form.get(name);
  return typeof value === "string" ? value.trim().slice(0, max) : "";
}

function integer(value: string | null | undefined, fallback: number): number {
  const parsed = Number(value || "");
  return Number.isSafeInteger(parsed) ? parsed : fallback;
}

function viewValue(value: string | null | undefined): View {
  const view = text(value, 16) as View;
  return validViews.has(view) ? view : "pending";
}

function verifiedValue(value: string | null | undefined): Verified {
  const verified = text(value, 16) as Verified;
  return validVerified.has(verified) ? verified : "all";
}

function normalizeQuery(url: URL): Pick<AdminEmailPageData, "view" | "page" | "perPage" | "search" | "verified"> {
  const perPageValue = integer(url.searchParams.get("per_page"), pageSizeDefault);
  const view = viewValue(url.searchParams.get("view"));
  return {
    view,
    page: Math.max(1, Math.min(integer(url.searchParams.get("page"), 1), 1_000_000)),
    perPage: [25, 50, 100].includes(perPageValue) ? perPageValue : pageSizeDefault,
    search: text(url.searchParams.get("search"), 120),
    verified: view === "accounts" ? verifiedValue(url.searchParams.get("verified")) : "all"
  };
}

function queryString(query: Pick<AdminEmailPageData, "view" | "page" | "perPage" | "search" | "verified">): string {
  const params = new URLSearchParams({ view: query.view, page: String(query.page), per_page: String(query.perPage) });
  if (query.search) params.set("search", query.search);
  if (query.view === "accounts" && query.verified !== "all") params.set("verified", query.verified);
  return params.toString();
}

function queryFromForm(form: FormData): Pick<AdminEmailPageData, "view" | "page" | "perPage" | "search" | "verified"> {
  const url = new URL("http://twilight.invalid/admin/email");
  for (const name of ["view", "page", "per_page", "search", "verified"]) {
    const value = form.get(name);
    if (typeof value === "string") url.searchParams.set(name, value);
  }
  return normalizeQuery(url);
}

function redirectToList(form: FormData, notice: "revoked" | "cleaned" | "cleared"): never {
  throw redirect(303, `/admin/email?${queryString(queryFromForm(form))}&notice=${notice}`);
}

function failure(action: FormState["action"], response: Response | undefined): FormFailure {
  return fail(response?.status || 503, { action, error: t.adminEmailMaintenanceFailed } satisfies FormState);
}

async function mutation(event: RequestEvent, path: string, method: "POST" | "DELETE", action: FormState["action"], payload?: unknown): Promise<FormFailure | null> {
  const result = await apiJSONWithResponse(event, path, {
    method,
    ...(payload === undefined ? {} : { headers: { "content-type": "application/json" }, body: JSON.stringify(payload) })
  });
  if (!result?.response.ok || !result.envelope?.success) return failure(action, result?.response);
  return null;
}

export const load: PageServerLoad = async (event): Promise<AdminEmailPageData> => {
  const query = normalizeQuery(event.url);
  const params = new URLSearchParams({ view: query.view, page: String(query.page), per_page: String(query.perPage) });
  if (query.search) params.set("search", query.search);
  if (query.view === "accounts") params.set("verified", query.verified);
  const result = await apiJSON<EmailAdminData>(event, `/api/v2/admin/email/verifications?${params}`, { cache: "no-store" });
  const rawNotice = text(event.url.searchParams.get("notice"), 16);
  return {
    payload: result?.success && result.data ? result.data : null,
    ...query,
    loadError: result?.success ? null : t.adminEmailLoadFailed,
    notice: ["revoked", "cleaned", "cleared"].includes(rawNotice)
      ? rawNotice as AdminEmailPageData["notice"]
      : ""
  };
};

export const actions: Actions = {
  revoke: async (event) => {
    const form = await event.request.formData();
    const id = formText(form, "verification_id", 128);
    if (!/^[A-Za-z0-9_-]{1,128}$/.test(id)) return failure("revoke", undefined);
    const result = await mutation(event, `/api/v2/admin/email/verifications/${encodeURIComponent(id)}`, "DELETE", "revoke");
    if (result) return result;
    redirectToList(form, "revoked");
  },

  cleanup: async (event) => {
    const form = await event.request.formData();
    if (formText(form, "confirm", 64) !== "CLEANUP_EXPIRED_EMAILS") return failure("cleanup", undefined);
    const result = await mutation(event, "/api/v2/admin/email/verifications/cleanup", "POST", "cleanup");
    if (result) return result;
    redirectToList(form, "cleaned");
  },

  clearUnverified: async (event) => {
    const form = await event.request.formData();
    if (formText(form, "confirm", 64) !== "CLEAR_UNVERIFIED_EMAILS") return failure("clearUnverified", undefined);
    const result = await mutation(event, "/api/v2/admin/email/verifications/clear-unverified", "POST", "clearUnverified");
    if (result) return result;
    redirectToList(form, "cleared");
  },

  testEmail: async (event) => {
    const form = await event.request.formData();
    const to = formText(form, "to", 254);
    const result = await apiJSONWithResponse<{ results?: EmailTestResult[] }>(event, "/api/v2/admin/email/test", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ to })
    });
    if (!result?.response.ok || !result.envelope?.success) return failure("testEmail", result?.response);
    const item = result.envelope.data?.results?.[0];
    return {
      action: "testEmail",
      test: { success: item?.success === true, email: text(item?.to || to || t.adminEmailNone, 254) }
    } satisfies FormState;
  }
};
