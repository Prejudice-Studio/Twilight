import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import type {
  AdminEmbyActivityResult,
  AdminEmbyDeviceAuditResult,
  AdminEmbyPageData,
  AdminEmbyUsersResult,
  EmbyConnectivityResult
} from "$lib/types";
import { t } from "$lib/i18n";

type FormState = {
  action?: string;
  success?: boolean;
  error?: string;
  connectivity?: EmbyConnectivityResult;
  activity?: AdminEmbyActivityResult;
  broadcast?: BroadcastResult;
  standalone?: StandaloneEmbyResult;
  passwordReset?: EmbyPasswordResetResult;
};
type FormFailure = ActionFailure<FormState>;

type BroadcastResult = { sent_count: number; failed?: Array<{ session_id?: string; error?: string }> };
type StandaloneEmbyResult = { emby_id: string; emby_username: string };
type EmbyPasswordResetResult = { emby_id: string; emby_username: string; linked_local_user: boolean; new_password: string };

const validTabs = new Set(["accounts", "devices", "activity"]);
const validLinks = new Set(["", "linked", "unlinked", "name_mismatch"]);
const validAttributes = new Set(["", "admin", "disabled", "hidden"]);

function text(value: string | null | undefined, max = 200): string {
  return (value || "").trim().slice(0, max);
}

function formText(form: FormData, name: string, max = 200): string {
  const value = form.get(name);
  return typeof value === "string" ? text(value, max) : "";
}

function formValue(form: FormData, name: string, max = 200): string {
  const value = form.get(name);
  return typeof value === "string" ? value.slice(0, max) : "";
}

function integer(value: string, fallback: number): number {
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) ? parsed : fallback;
}

function normalizeQuery(url: URL): AdminEmbyPageData["query"] {
  const linkValue = text(url.searchParams.get("link"), 24);
  const attributeValue = text(url.searchParams.get("attribute"), 24);
  const pageValue = integer(text(url.searchParams.get("page"), 12), 1);
  const perPageValue = integer(text(url.searchParams.get("per_page"), 12), 50);
  const devicePageValue = integer(text(url.searchParams.get("device_page"), 12), 1);
  const devicePerPageValue = integer(text(url.searchParams.get("device_per_page"), 12), 50);
  const orphanPageValue = integer(text(url.searchParams.get("orphan_page"), 12), 1);
  const orphanPerPageValue = integer(text(url.searchParams.get("orphan_per_page"), 12), 300);
  return {
    page: Math.max(1, Math.min(pageValue, 1_000_000)),
    per_page: [20, 50, 100, 200].includes(perPageValue) ? perPageValue : 50,
    search: text(url.searchParams.get("search"), 100),
    link: validLinks.has(linkValue) ? linkValue : "",
    attribute: validAttributes.has(attributeValue) ? attributeValue : "",
    device_page: Math.max(1, Math.min(devicePageValue, 1_000_000)),
    device_per_page: [20, 50, 100, 200].includes(devicePerPageValue) ? devicePerPageValue : 50,
    device_search: text(url.searchParams.get("device_search"), 100),
    orphan_page: Math.max(1, Math.min(orphanPageValue, 1_000_000)),
    orphan_per_page: [100, 200, 300].includes(orphanPerPageValue) ? orphanPerPageValue : 300
  };
}

function queryString(query: AdminEmbyPageData["query"], overrides: Record<string, string | number> = {}): string {
  const params = new URLSearchParams();
  const values: Record<string, string | number> = {
    page: query.page,
    per_page: query.per_page,
    search: query.search,
    link: query.link,
    attribute: query.attribute,
    device_page: query.device_page,
    device_per_page: query.device_per_page,
    device_search: query.device_search,
    orphan_page: query.orphan_page,
    orphan_per_page: query.orphan_per_page,
    ...overrides
  };
  for (const [key, value] of Object.entries(values)) {
    if (value !== "" && value !== 0) params.set(key, String(value));
  }
  return params.toString();
}

function queryFromForm(form: FormData): AdminEmbyPageData["query"] {
  const url = new URL("http://twilight.invalid/admin/emby");
  for (const name of ["page", "per_page", "search", "link", "attribute", "device_page", "device_per_page", "device_search", "orphan_page", "orphan_per_page"]) {
    const value = form.get(name);
    if (typeof value === "string") url.searchParams.set(name, value);
  }
  return normalizeQuery(url);
}

function tabOf(url: URL): AdminEmbyPageData["tab"] {
  const tab = text(url.searchParams.get("tab"), 20);
  return validTabs.has(tab) ? tab as AdminEmbyPageData["tab"] : "accounts";
}

async function mutation<T>(event: RequestEvent, path: string, method: "GET" | "POST" | "DELETE", payload: unknown, action: string): Promise<{ data?: T; failure?: FormFailure }> {
  const result = await apiJSONWithResponse<T>(event, path, {
    method,
    ...(payload === undefined ? {} : { headers: { "content-type": "application/json" }, body: JSON.stringify(payload) })
  });
  if (!result?.response.ok || !result.envelope?.success) {
    return { failure: fail(result?.response.status || 503, { action, error: result?.envelope?.message || t.adminEmbyOperationFailed }) };
  }
  return { data: result.envelope.data };
}

function redirectToTab(form: FormData, tab: AdminEmbyPageData["tab"]): never {
  throw redirect(303, `/admin/emby?tab=${tab}&${queryString(queryFromForm(form))}`);
}

export const load: PageServerLoad = async (event): Promise<AdminEmbyPageData> => {
  const tab = tabOf(event.url);
  const query = normalizeQuery(event.url);
  const errors: string[] = [];
  let users: AdminEmbyUsersResult | null = null;
  let deviceAudit: AdminEmbyDeviceAuditResult | null = null;
  let activity: AdminEmbyActivityResult | null = null;

  if (tab === "accounts") {
    const params = new URLSearchParams({ page: String(query.page), per_page: String(query.per_page) });
    if (query.search) params.set("search", query.search);
		if (query.link) params.set("link", query.link);
		if (query.attribute) params.set("attribute", query.attribute);
		params.set("orphan_page", String(query.orphan_page));
		params.set("orphan_per_page", String(query.orphan_per_page));
    const result = await apiJSON<AdminEmbyUsersResult>(event, `/api/v1/admin/emby/users?${params}`, { cache: "no-store" });
    users = result?.success ? result.data || null : null;
    if (!users) errors.push(t.adminEmbyOperationFailed);
  } else if (tab === "devices") {
    const params = new URLSearchParams({ page: String(query.device_page), per_page: String(query.device_per_page) });
    if (query.device_search) params.set("search", query.device_search);
    if (event.url.searchParams.get("refresh") === "1" || event.url.searchParams.get("refresh") === "true") params.set("refresh", "1");
    const result = await apiJSON<AdminEmbyDeviceAuditResult>(event, `/api/v1/admin/emby/device-audit?${params}`, { cache: "no-store" });
    deviceAudit = result?.success ? result.data || null : null;
    if (!deviceAudit) errors.push(t.adminEmbyDeviceDescription);
  } else {
    const result = await apiJSON<AdminEmbyActivityResult>(event, "/api/v1/admin/emby/activity-logs?limit=200", { cache: "no-store" });
    activity = result?.success ? result.data || null : null;
    if (!activity) errors.push(t.adminEmbyActivityReadFailed);
  }
  return { tab, users, deviceAudit, activity, query, errors };
};

export const actions: Actions = {
  testConnectivity: async (event) => {
    const result = await mutation<EmbyConnectivityResult>(event, "/api/v1/admin/emby/test", "POST", {}, "testConnectivity");
    if (result.failure) return result.failure;
    return { action: "testConnectivity", success: true, connectivity: result.data } satisfies FormState;
  },

  broadcast: async (event) => {
    const form = await event.request.formData();
    const message = formText(form, "text", 2000);
    if (!message) return fail(400, { action: "broadcast", error: t.adminEmbyBroadcastRequired } satisfies FormState);
    const header = formText(form, "header", 80);
    const result = await mutation<BroadcastResult>(event, "/api/v1/admin/emby/broadcast", "POST", { text: message, ...(header ? { header } : {}) }, "broadcast");
    if (result.failure) return result.failure;
    return { action: "broadcast", success: true, broadcast: result.data } satisfies FormState;
  },

  createStandalone: async (event) => {
    const form = await event.request.formData();
    const username = formText(form, "username", 64);
    const password = formValue(form, "password", 256);
    if (!username || password.length < 8 || password.length > 128) return fail(400, { action: "createStandalone", error: t.adminEmbyPasswordInvalid } satisfies FormState);
    const result = await mutation<StandaloneEmbyResult>(event, "/api/v1/admin/emby/create-standalone", "POST", { username, password }, "createStandalone");
    if (result.failure) return result.failure;
    return { action: "createStandalone", success: true, standalone: result.data } satisfies FormState;
  },

  forceSetPassword: async (event) => {
    const form = await event.request.formData();
    const username = formText(form, "emby_username", 64);
    const password = formValue(form, "new_password", 256);
    if (!username) return fail(400, { action: "forceSetPassword", error: t.adminEmbyForcePasswordRequired } satisfies FormState);
    if (password && (password.length < 8 || password.length > 128)) return fail(400, { action: "forceSetPassword", error: t.adminEmbyPasswordInvalid } satisfies FormState);
    const result = await mutation<EmbyPasswordResetResult>(event, "/api/v1/admin/emby/force-set-password", "POST", { emby_username: username, ...(password ? { new_password: password } : {}) }, "forceSetPassword");
    if (result.failure) return result.failure;
    return { action: "forceSetPassword", success: true, passwordReset: result.data } satisfies FormState;
  },

  sync: async (event) => {
    const form = await event.request.formData();
    const result = await mutation(event, "/api/v1/admin/emby/sync", "POST", {}, "sync");
    if (result.failure) return result.failure;
    redirectToTab(form, "accounts");
  },

  importUsers: async (event) => {
    const form = await event.request.formData();
    const result = await mutation(event, "/api/v1/admin/emby/import-users", "POST", {}, "importUsers");
    if (result.failure) return result.failure;
    redirectToTab(form, "accounts");
  },

  deleteUnlinked: async (event) => {
    const form = await event.request.formData();
    if (formText(form, "confirm", 80) !== "DELETE_UNLINKED_EMBY") return fail(400, { action: "deleteUnlinked", error: t.adminEmbyDeleteUnlinkedConfirm } satisfies FormState);
    const result = await mutation(event, "/api/v1/admin/emby/delete-unlinked", "POST", { dry_run: false }, "deleteUnlinked");
    if (result.failure) return result.failure;
    redirectToTab(form, "accounts");
  },

  cleanup: async (event) => {
    const form = await event.request.formData();
    const result = await mutation(event, "/api/v1/admin/emby/cleanup-orphans", "POST", {}, "cleanup");
    if (result.failure) return result.failure;
    redirectToTab(form, "accounts");
  },

  reset: async (event) => {
    const form = await event.request.formData();
    if (formText(form, "confirm", 80) !== "RESET_ALL_EMBY") return fail(400, { action: "reset", error: t.adminEmbyResetConfirm } satisfies FormState);
    const result = await mutation(event, "/api/v1/admin/emby/reset-bindings", "POST", { confirm: "RESET_ALL_EMBY" }, "reset");
    if (result.failure) return result.failure;
    redirectToTab(form, "accounts");
  },

  syncActivity: async (event) => {
    const result = await mutation<AdminEmbyActivityResult>(event, "/api/v1/admin/emby/activity-logs?limit=200&refresh=1&since_hours=24", "GET", undefined, "syncActivity");
    if (result.failure) return result.failure;
    return { action: "syncActivity", success: true, activity: result.data } satisfies FormState;
  },

  setEmbyEnabled: async (event) => {
    const form = await event.request.formData();
    const embyID = formText(form, "emby_id", 256);
    if (!embyID) return fail(400, { action: "setEmbyEnabled", error: t.adminEmbyOperationFailed } satisfies FormState);
    const enabled = formText(form, "enabled", 8) === "true";
    const result = await mutation(event, `/api/v1/admin/emby/users/${encodeURIComponent(embyID)}/${enabled ? "enable" : "disable"}`, "POST", {}, "setEmbyEnabled");
    if (result.failure) return result.failure;
    redirectToTab(form, "accounts");
  },

  kickEmby: async (event) => {
    const form = await event.request.formData();
    const embyID = formText(form, "emby_id", 256);
    if (!embyID) return fail(400, { action: "kickEmby", error: t.adminEmbyOperationFailed } satisfies FormState);
    const result = await mutation(event, `/api/v1/admin/emby/users/${encodeURIComponent(embyID)}/kick`, "POST", {}, "kickEmby");
    if (result.failure) return result.failure;
    redirectToTab(form, "accounts");
  }
};
