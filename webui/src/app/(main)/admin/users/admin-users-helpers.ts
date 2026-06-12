import type { ConfirmOptions } from "@/components/ui/confirm-dialog";
import type { AdminUserListParams } from "@/lib/api-types";
import type { MessageKey, MessageParams } from "@/lib/i18n";

// 翻译函数类型：与 LocaleContextValue.t 同构。这些 confirm 配置构造器是纯函数，
// 由 page.tsx 注入 useI18n() 的 t，保持无状态、可单测。
type TFunc = (key: MessageKey, params?: MessageParams) => string;

export type BatchDeleteAction = "local_only" | "with_emby";

// EmailStatusFilter 与后端 listUsers / filteredBatchUserUIDs 的 email_status 取值对齐。
export type EmailStatusFilter = "verified" | "unverified" | "bound" | "none";

export interface UsersListState {
  page: number;
  perPage: number;
  search: string;
  roleFilter: string;
  activeFilter: string;
  embyFilter: string;
  embyStatusFilter: string;
  emailStatusFilter: string;
  sortBy: string;
}

export function buildUsersCacheKey(state: UsersListState): string {
  return [
    state.page,
    state.perPage,
    state.search || "",
    state.roleFilter,
    state.activeFilter,
    state.embyFilter,
    state.embyStatusFilter,
    state.emailStatusFilter,
    state.sortBy,
  ].join("-");
}

export function usersListParams(state: UsersListState): AdminUserListParams {
  return {
    page: state.page,
    per_page: state.perPage,
    search: state.search || undefined,
    role: state.roleFilter === "all" ? undefined : Number(state.roleFilter),
    active: state.activeFilter === "all" ? undefined : state.activeFilter === "true",
    emby: state.embyFilter === "bound" ? "bound" : state.embyFilter === "unbound" ? "unbound" : undefined,
    emby_status: state.embyStatusFilter === "all" ? undefined : (state.embyStatusFilter as "active" | "disabled"),
    email_status: state.emailStatusFilter === "all" ? undefined : (state.emailStatusFilter as EmailStatusFilter),
    sort: state.sortBy || undefined,
  };
}

export function usersBatchFilterParams(state: UsersListState) {
  const params = usersListParams(state);
  return {
    role: params.role,
    active: params.active,
    emby: params.emby,
    emby_status: params.emby_status,
    email_status: params.email_status,
    search: params.search,
  };
}

export function hasStrongAdminPassword(password: string): boolean {
  return password.length >= 8 && /[a-z]/.test(password) && /[A-Z]/.test(password) && /\d/.test(password);
}

export function toggleSetMember<T>(values: Set<T>, value: T): Set<T> {
  const next = new Set(values);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  return next;
}

export function batchToggleConfirmConfig(enable: boolean, count: number, t: TFunc): ConfirmOptions {
  return {
    title: enable ? t("adminUsers.batchEnableTitle") : t("adminUsers.batchDisableTitle"),
    description: t("adminUsers.batchToggleDescription", { count }),
    tone: enable ? "warning" : "danger",
    confirmLabel: enable ? t("adminUsers.batchEnableConfirm") : t("adminUsers.batchDisableConfirm"),
  };
}

export function batchLockEmbyUnbindConfirmConfig(count: number, t: TFunc): ConfirmOptions {
  return {
    title: t("adminUsers.batchLockEmbyTitle"),
    description: t("adminUsers.batchLockEmbyDescription", { count }),
    tone: "warning",
    confirmLabel: t("adminUsers.batchLockEmbyConfirm"),
  };
}

export function batchClearEmbyGrantConfirmConfig(count: number, t: TFunc): ConfirmOptions {
  return {
    title: t("adminUsers.batchClearGrantTitle"),
    description: t("adminUsers.batchClearGrantDescription", { count }),
    tone: "warning",
    confirmLabel: t("adminUsers.batchClearGrantConfirm"),
  };
}

export function batchDeleteConfirmConfig(count: number, t: TFunc, embyCount?: number): ConfirmOptions {
  const embyLabel = typeof embyCount === "number"
    ? t("adminUsers.batchDeleteEmbyWithCount", { count: embyCount })
    : t("adminUsers.batchDeleteEmbyGeneric");
  return {
    title: t("adminUsers.batchDeleteTitle"),
    description: t("adminUsers.batchDeleteDescription", { count }),
    tone: "danger",
    cancelLabel: t("adminUsers.cancel"),
    actions: [
      { label: t("adminUsers.batchDeleteLocalOnly"), value: "local_only" as BatchDeleteAction, variant: "destructive" },
      { label: embyLabel, value: "with_emby" as BatchDeleteAction, variant: "destructive" },
    ],
  };
}
