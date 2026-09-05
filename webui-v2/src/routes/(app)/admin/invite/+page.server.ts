import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { ActionFailure, RequestEvent } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import type {
  AdminInviteCodesPage,
  AdminInvitePageData,
  AdminInviteTreePage,
  AdminInviteTreeRow,
  ConfigSchema,
  ConfigSection,
  InviteCodeItem,
  InviteConfig
} from "$lib/types";
import { t } from "$lib/i18n";

type FormState = { action?: string; error?: string };
type FormFailure = ActionFailure<FormState>;
type View = AdminInvitePageData["view"];

type InviteForestNode = {
  uid: number;
  username: string;
  role: number;
  emby_id?: string | null;
  emby_disabled?: boolean;
  active: boolean;
  telegram_id?: number | null;
  register_time?: number | null;
  expired_at?: number | null;
  is_root: boolean;
};

type InviteForest = {
  nodes: InviteForestNode[];
  edges: Array<{ parent: number; child: number; code?: string; created_at?: number }>;
  roots: number[];
  max_depth: number;
  config: InviteConfig;
};

type InviteCodesResponse = {
  codes: InviteCodeItem[];
  total: number;
  page?: number;
  per_page?: number;
  pages?: number;
};

const treePerPage = 300;
const codePerPage = 50;
const inviteConfigKeys = new Set([
  "invite_enabled",
  "invite_limit",
  "invite_root_user_limit",
  "invite_max_depth",
  "invite_require_emby",
  "invite_code_default_days",
  "permanent_invite_max_days",
  "invite_code_format",
  "invite_code_random_algorithm"
]);

function text(value: FormDataEntryValue | string | null | undefined, max = 200): string {
  return (typeof value === "string" ? value : "").trim().slice(0, max);
}

function integer(value: string | null | undefined, fallback: number): number {
  const parsed = Number(value || "");
  return Number.isSafeInteger(parsed) ? parsed : fallback;
}

function positiveInteger(value: string | null | undefined): number {
  const raw = text(value, 24);
  return /^\d+$/.test(raw) ? integer(raw, 0) : 0;
}

function formBoolean(form: FormData, name: string, fallback = false): boolean {
  const values = form.getAll(name);
  return values.length ? values.some((value) => value === "true" || value === "on" || value === "1") : fallback;
}

function queryBoolean(value: string | null, fallback = false): boolean {
  if (value === null || value === "") return fallback;
  return value === "true" || value === "1";
}

function normalizeView(value: string | null): View {
  return value === "codes" || value === "config" ? value : "tree";
}

function parseCollapsed(value: string | null): number[] {
  if (!value) return [];
  const result: number[] = [];
  for (const item of value.split(",").slice(0, 200)) {
    const uid = positiveInteger(item);
    if (uid > 0 && !result.includes(uid)) result.push(uid);
  }
  return result;
}

function normalizeQuery(url: URL): AdminInvitePageData["query"] {
  const view = normalizeView(url.searchParams.get("view"));
  const perPage = integer(url.searchParams.get("per_page"), treePerPage);
  const codePageSize = integer(url.searchParams.get("code_per_page"), codePerPage);
  const root = text(url.searchParams.get("root"), 24);
  return {
    view,
    page: Math.max(1, Math.min(1_000_000, integer(url.searchParams.get("page"), 1))),
    per_page: [100, 300, 500].includes(perPage) ? perPage : treePerPage,
    search: text(url.searchParams.get("search"), 120),
    root: root === "all" ? "all" : /^\d+$/.test(root) ? root : "all",
    selected: positiveInteger(url.searchParams.get("selected")),
    collapsed: parseCollapsed(url.searchParams.get("collapsed")),
    code_page: Math.max(1, Math.min(1_000_000, integer(url.searchParams.get("code_page"), 1))),
    code_per_page: [20, 50, 100].includes(codePageSize) ? codePageSize : codePerPage,
    code_search: text(url.searchParams.get("code_search"), 120)
  };
}

function queryString(query: AdminInvitePageData["query"], overrides: Partial<AdminInvitePageData["query"]> = {}): string {
  const value = { ...query, ...overrides };
  const params = new URLSearchParams();
  if (value.view !== "tree") params.set("view", value.view);
  if (value.page > 1) params.set("page", String(value.page));
  if (value.per_page !== treePerPage) params.set("per_page", String(value.per_page));
  if (value.search) params.set("search", value.search);
  if (value.root !== "all") params.set("root", value.root);
  if (value.selected > 0) params.set("selected", String(value.selected));
  if (value.collapsed.length) params.set("collapsed", value.collapsed.join(","));
  if (value.code_page > 1) params.set("code_page", String(value.code_page));
  if (value.code_per_page !== codePerPage) params.set("code_per_page", String(value.code_per_page));
  if (value.code_search) params.set("code_search", value.code_search);
  return params.toString();
}

function queryFromForm(form: FormData): AdminInvitePageData["query"] {
  const url = new URL("http://twilight.invalid/admin/invite");
  for (const name of ["view", "page", "per_page", "search", "root", "selected", "collapsed", "code_page", "code_per_page", "code_search"]) {
    const value = form.get(name);
    if (typeof value === "string") url.searchParams.set(name, value);
  }
  return normalizeQuery(url);
}

function redirectToPage(form: FormData, notice: AdminInvitePageData["notice"]): never {
  const query = queryString(queryFromForm(form), { selected: 0 });
  throw redirect(303, `/admin/invite${query ? `?${query}&notice=${notice}` : `?notice=${notice}`}`);
}

async function mutate(
  event: RequestEvent,
  path: string,
  method: "POST" | "PUT" | "DELETE",
  payload: unknown,
  action: string
): Promise<FormFailure | null> {
  const result = await apiJSONWithResponse(event, path, {
    method,
    ...(payload === undefined ? {} : { headers: { "content-type": "application/json" }, body: JSON.stringify(payload) })
  });
  if (!result?.response.ok || !result.envelope?.success) {
    return fail(result?.response.status || 503, {
      action,
      error: result?.envelope?.message || t.adminInviteOperationFailed
    } satisfies FormState);
  }
  return null;
}

function selectedUIDs(form: FormData, max = 200): number[] | null {
  const result: number[] = [];
  const seen = new Set<number>();
  for (const value of form.getAll("uids")) {
    const uid = positiveInteger(typeof value === "string" ? value : "");
    if (uid <= 0 || seen.has(uid)) continue;
    seen.add(uid);
    result.push(uid);
    if (result.length > max) return null;
  }
  return result;
}

function inviteTreeRow(
  node: InviteForestNode,
  depth: number,
  rootUID: number,
  directChildren: number,
  descendants: number,
  collapsed: boolean
): AdminInviteTreeRow {
  return {
    uid: node.uid,
    username: node.username,
    role: node.role,
    emby_bound: Boolean(node.emby_id),
    emby_disabled: Boolean(node.emby_disabled),
    active: node.active,
    telegram_id: node.telegram_id ?? null,
    register_time: node.register_time ?? null,
    expired_at: node.expired_at ?? null,
    is_root: node.is_root,
    depth,
    root_uid: rootUID,
    direct_children: directChildren,
    descendants,
    collapsed
  };
}

function buildTreePage(forest: InviteForest, query: AdminInvitePageData["query"]): AdminInviteTreePage {
  const nodes = new Map<number, InviteForestNode>();
  const children = new Map<number, number[]>();
  const parent = new Map<number, number>();
  for (const node of forest.nodes || []) nodes.set(node.uid, node);
  for (const edge of forest.edges || []) {
    if (!nodes.has(edge.parent) || !nodes.has(edge.child)) continue;
    const list = children.get(edge.parent) || [];
    list.push(edge.child);
    children.set(edge.parent, list);
    if (!parent.has(edge.child)) parent.set(edge.child, edge.parent);
  }
  for (const list of children.values()) list.sort((a, b) => a - b);

  const roots = (forest.roots || []).filter((uid) => nodes.has(uid)).sort((a, b) => a - b);
  const rootOf = new Map<number, number>();
  const depthOf = new Map<number, number>();
  const descendants = new Map<number, number>();
  for (const root of roots) {
    if (rootOf.has(root)) continue;
    const order: number[] = [];
    const stack: Array<{ uid: number; depth: number }> = [{ uid: root, depth: 0 }];
    rootOf.set(root, root);
    depthOf.set(root, 0);
    while (stack.length) {
      const current = stack.pop()!;
      order.push(current.uid);
      for (const child of children.get(current.uid) || []) {
        if (rootOf.has(child)) continue;
        rootOf.set(child, root);
        depthOf.set(child, current.depth + 1);
        stack.push({ uid: child, depth: current.depth + 1 });
      }
    }
    for (let index = order.length - 1; index >= 0; index -= 1) {
      const uid = order[index];
      let count = 0;
      for (const child of children.get(uid) || []) count += 1 + (descendants.get(child) || 0);
      descendants.set(uid, count);
    }
  }

  const term = query.search.toLowerCase();
  const included = new Set<number>();
  if (term) {
    for (const node of nodes.values()) {
      if (!`${node.username} ${node.uid} ${node.telegram_id || ""}`.toLowerCase().includes(term)) continue;
      let current: number | undefined = node.uid;
      while (current && !included.has(current)) {
        included.add(current);
        current = parent.get(current);
      }
    }
  }

  const selectedRoot = query.root === "all" ? null : positiveInteger(query.root);
  const visibleRoots = selectedRoot && roots.includes(selectedRoot) ? [selectedRoot] : roots;
  const collapsed = new Set(query.collapsed);
  const rows: AdminInviteTreeRow[] = [];
  for (const root of visibleRoots) {
    const stack: Array<{ uid: number; depth: number }> = [{ uid: root, depth: 0 }];
    while (stack.length) {
      const current = stack.pop()!;
      const node = nodes.get(current.uid);
      if (!node) continue;
      const matched = !term || included.has(current.uid);
      if (matched) {
        rows.push(inviteTreeRow(
          node,
          current.depth,
          rootOf.get(current.uid) || root,
          (children.get(current.uid) || []).length,
          descendants.get(current.uid) || 0,
          collapsed.has(current.uid)
        ));
      }
      // Search results always keep the matching path open, otherwise a collapsed
      // ancestor would hide the result that the administrator just searched for.
      if (collapsed.has(current.uid) && !term) continue;
      const childIDs = children.get(current.uid) || [];
      for (let index = childIDs.length - 1; index >= 0; index -= 1) {
        stack.push({ uid: childIDs[index], depth: current.depth + 1 });
      }
    }
  }

  const perPage = query.per_page;
  const pages = Math.max(1, Math.ceil(rows.length / perPage));
  const page = Math.min(query.page, pages);
  const pageRows = rows.slice((page - 1) * perPage, page * perPage);
  const selectedNode = query.selected > 0 ? nodes.get(query.selected) : undefined;
  const selected = selectedNode
    ? inviteTreeRow(
        selectedNode,
        depthOf.get(selectedNode.uid) || 0,
        rootOf.get(selectedNode.uid) || selectedNode.uid,
        (children.get(selectedNode.uid) || []).length,
        descendants.get(selectedNode.uid) || 0,
        collapsed.has(selectedNode.uid)
      )
    : null;
  return {
    rows: pageRows,
    selected,
    roots: roots.map((uid) => ({ uid, username: nodes.get(uid)?.username || String(uid) })),
    total_rows: rows.length,
    total_nodes: nodes.size,
    total_relations: (forest.edges || []).length,
    max_depth: forest.max_depth || 0,
    page,
    per_page: perPage,
    pages,
    config: forest.config
  };
}

function buildCodePage(payload: InviteCodesResponse, query: AdminInvitePageData["query"]): AdminInviteCodesPage {
  const term = query.code_search.toLowerCase();
  const codes = (payload.codes || []).filter((code) => {
    if (!term) return true;
    return `${code.code} ${code.inviter_username || ""} ${code.inviter_uid} ${code.target_username || ""} ${code.note || ""}`.toLowerCase().includes(term);
  });
  const pages = Math.max(1, Math.ceil(codes.length / query.code_per_page));
  const page = Math.min(query.code_page, pages);
  return {
    codes: codes.slice((page - 1) * query.code_per_page, page * query.code_per_page),
    total: codes.length,
    page,
    per_page: query.code_per_page,
    pages
  };
}

function inviteConfigSection(schema: ConfigSchema | null): ConfigSection | null {
  const section = schema?.sections?.find((item) => item.key === "SAR");
  if (!section) return null;
  return { ...section, fields: section.fields.filter((field) => inviteConfigKeys.has(field.key)) };
}

function configValue(form: FormData, field: ConfigSection["fields"][number]): unknown {
  if (field.type === "bool") return formBoolean(form, field.key, Boolean(field.value));
  const value = text(form.get(field.key), 1000);
  if (!value) return field.value;
  if (field.type === "int") {
    const parsed = Number(value);
    return Number.isSafeInteger(parsed) ? parsed : field.value;
  }
  if (field.type === "float") {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : field.value;
  }
  return value;
}

export const load: PageServerLoad = async (event): Promise<AdminInvitePageData> => {
  const query = normalizeQuery(event.url);
  let tree: AdminInviteTreePage | null = null;
  let codes: AdminInviteCodesPage | null = null;
  let config: ConfigSection | null = null;
  let loadError: string | null = null;

  if (query.view === "tree") {
    const result = await apiJSON<InviteForest>(event, "/api/v1/admin/invite/tree", { cache: "no-store" });
    if (result?.success && result.data) tree = buildTreePage(result.data, query);
    else loadError = t.adminInviteLoadFailed;
  } else if (query.view === "codes") {
    const params = new URLSearchParams({ page: String(query.code_page), per_page: String(query.code_per_page) });
    if (query.code_search) params.set("search", query.code_search);
    const result = await apiJSON<InviteCodesResponse>(event, `/api/v1/admin/invite/codes?${params}`, { cache: "no-store" });
    if (result?.success && result.data) {
      // The paged backend response is already bounded. Keep the fallback
      // slicer for older servers that return only codes/total.
      codes = result.data.page && result.data.per_page && result.data.pages
        ? {
            codes: result.data.codes || [],
            total: result.data.total || 0,
            page: result.data.page,
            per_page: result.data.per_page,
            pages: result.data.pages
          }
        : buildCodePage(result.data, query);
    }
    else loadError = t.adminInviteCodesLoadFailed;
  } else {
    const result = await apiJSON<ConfigSchema>(event, "/api/v1/system/admin/config/schema", { cache: "no-store" });
    config = result?.success ? inviteConfigSection(result.data || null) : null;
    if (!config) loadError = t.adminInviteConfigLoadFailed;
  }

  const noticeValue = text(event.url.searchParams.get("notice"), 24);
  const notices = new Set<AdminInvitePageData["notice"]>([
    "detached",
    "deleted_emby",
    "batch_detached",
    "quick_maintained",
    "cascade_updated",
    "deleted",
    "config_saved"
  ]);
  return {
    view: query.view,
    tree,
    codes,
    config,
    query,
    notice: notices.has(noticeValue as AdminInvitePageData["notice"]) ? noticeValue as AdminInvitePageData["notice"] : "",
    loadError
  };
};

export const actions: Actions = {
  detach: async (event) => {
    const form = await event.request.formData();
    const uid = positiveInteger(text(form.get("uid"), 24));
    if (uid <= 0) return fail(400, { action: "detach", error: t.adminInviteOperationFailed } satisfies FormState);
    const failure = await mutate(event, `/api/v1/admin/invite/users/${uid}/detach`, "POST", undefined, "detach");
    if (failure) return failure;
    redirectToPage(form, "detached");
  },

  detachDeleteEmby: async (event) => {
    const form = await event.request.formData();
    const uid = positiveInteger(text(form.get("uid"), 24));
    if (uid <= 0) return fail(400, { action: "detachDeleteEmby", error: t.adminInviteOperationFailed } satisfies FormState);
    const failure = await mutate(event, `/api/v1/admin/invite/users/${uid}/detach-delete-emby`, "POST", undefined, "detachDeleteEmby");
    if (failure) return failure;
    redirectToPage(form, "deleted_emby");
  },

  batchDetach: async (event) => {
    const form = await event.request.formData();
    const uids = selectedUIDs(form);
    if (uids === null) return fail(400, { action: "batchDetach", error: t.adminInviteSelectionTooMany } satisfies FormState);
    if (!uids.length) return fail(400, { action: "batchDetach", error: t.adminInviteNoSelection } satisfies FormState);
    const operation = text(form.get("operation"), 32);
    const deleteEmby = operation === "delete_emby" || operation === "only_emby_disabled";
    const onlyEmbyDisabled = operation === "only_emby_disabled";
    if (onlyEmbyDisabled && !deleteEmby) return fail(400, { action: "batchDetach", error: t.adminInviteOperationFailed } satisfies FormState);
    const failure = await mutate(event, "/api/v1/admin/invite/users/detach-batch", "POST", {
      uids,
      delete_emby: deleteEmby,
      only_emby_disabled: onlyEmbyDisabled
    }, "batchDetach");
    if (failure) return failure;
    redirectToPage(form, "batch_detached");
  },

  quickMaintenance: async (event) => {
    const form = await event.request.formData();
    const scope = text(form.get("scope"), 16);
    if (scope !== "selected" && scope !== "subtree" && scope !== "all") {
      return fail(400, { action: "quickMaintenance", error: t.adminInviteOperationFailed } satisfies FormState);
    }
    const rawDays = integer(text(form.get("renew_days"), 16), 0);
    if (rawDays === 0 || rawDays < -1 || rawDays > 36500) {
      return fail(400, { action: "quickMaintenance", error: t.adminInviteRenewDaysInvalid } satisfies FormState);
    }
    const payload: Record<string, unknown> = {
      confirm: "INVITE_QUICK_MAINTENANCE",
      scope,
      detach: true,
      renew_days: rawDays
    };
    if (scope === "selected") {
      const uids = selectedUIDs(form);
      if (uids === null) return fail(400, { action: "quickMaintenance", error: t.adminInviteSelectionTooMany } satisfies FormState);
      if (!uids.length) return fail(400, { action: "quickMaintenance", error: t.adminInviteNoSelection } satisfies FormState);
      payload.uids = uids;
    }
    if (scope === "subtree") {
      const rootUID = positiveInteger(text(form.get("root_uid"), 24));
      if (rootUID <= 0) return fail(400, { action: "quickMaintenance", error: t.adminInviteOperationFailed } satisfies FormState);
      payload.root_uid = rootUID;
      payload.depth = Math.max(-1, Math.min(5000, integer(text(form.get("depth"), 16), -1)));
      payload.include_root = formBoolean(form, "include_root");
    }
    const failure = await mutate(event, "/api/v1/admin/invite/quick-maintenance", "POST", payload, "quickMaintenance");
    if (failure) return failure;
    redirectToPage(form, "quick_maintained");
  },

  cascadeToggle: async (event) => {
    const form = await event.request.formData();
    const uid = positiveInteger(text(form.get("uid"), 24));
    const enable = form.get("enable") === "true";
    const depth = Math.max(-1, Math.min(5000, integer(text(form.get("depth"), 16), 1)));
    if (uid <= 0) return fail(400, { action: "cascadeToggle", error: t.adminInviteOperationFailed } satisfies FormState);
    const failure = await mutate(event, `/api/v1/admin/users/${uid}/${enable ? "enable" : "disable"}`, "POST", { cascade_depth: depth }, "cascadeToggle");
    if (failure) return failure;
    redirectToPage(form, "cascade_updated");
  },

  cascadeDelete: async (event) => {
    const form = await event.request.formData();
    const uid = positiveInteger(text(form.get("uid"), 24));
    const depth = Math.max(-1, Math.min(5000, integer(text(form.get("depth"), 16), 1)));
    if (uid <= 0) return fail(400, { action: "cascadeDelete", error: t.adminInviteOperationFailed } satisfies FormState);
    const failure = await mutate(event, `/api/v1/admin/users/${uid}/delete`, "POST", { mode: "with_emby", cascade_depth: depth }, "cascadeDelete");
    if (failure) return failure;
    redirectToPage(form, "deleted");
  },

  saveConfig: async (event) => {
    const form = await event.request.formData();
    const result = await apiJSON<ConfigSchema>(event, "/api/v1/system/admin/config/schema", { cache: "no-store" });
    const schema = result?.success ? result.data : null;
    if (!schema) return fail(503, { action: "saveConfig", error: t.adminInviteConfigLoadFailed } satisfies FormState);
    const sections = Object.fromEntries(schema.sections.map((section) => [
      section.key,
      Object.fromEntries(section.fields.map((field) => [
        field.key,
        inviteConfigKeys.has(field.key) ? configValue(form, field) : field.value
      ]))
    ]));
    const failure = await mutate(event, "/api/v1/system/admin/config/schema", "PUT", { sections }, "saveConfig");
    if (failure) return failure;
    redirectToPage(form, "config_saved");
  }
};
