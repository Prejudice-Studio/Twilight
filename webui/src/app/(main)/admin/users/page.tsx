"use client";

import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Search,
  MoreHorizontal,
  RefreshCw,
  Ban,
  Trash2,
  Key,
  Loader2,
  ChevronLeft,
  ChevronRight,
  ChevronDown,
  UserX,
  Link2,
  AlertTriangle,
  UserPlus,
  UserCheck,
  CalendarClock,
  Send,
  LockKeyhole,
  Eraser,
  FlipHorizontal2,
  MonitorCheck,
  MonitorX,
  RefreshCcw,
  BookOpen,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError, PageLoading } from "@/components/layout/page-state";
import { api, type UserInfo } from "@/lib/api";
import { ApiError } from "@/lib/api-request";
import { useI18n } from "@/lib/i18n";
import { ErrCodes } from "@/lib/errcode";
import { formatDate, readStoredPerPage, storePerPage } from "@/lib/utils";

const USERS_PER_PAGE_OPTIONS = [20, 50, 100];
const USERS_PER_PAGE_STORAGE_KEY = "twilight.admin.users.perPage";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { sanitizeImageUrl } from "@/lib/safe-url";
import { API_BASE } from "@/lib/api-request";

// normalizeAssetUrl prepends API_BASE to relative asset paths
// so avatars work when frontend and backend are on different domains.
function normalizeAssetUrl(url?: string): string | undefined {
  if (!url) return undefined;
  const trimmed = url.trim();
  if (!trimmed) return undefined;
  if (trimmed.startsWith("http")) return sanitizeImageUrl(trimmed);
  if (trimmed.startsWith("/")) return sanitizeImageUrl(`${API_BASE}${trimmed}`);
  return sanitizeImageUrl(trimmed);
}
import {
  batchClearEmbyGrantConfirmConfig,
  batchDeleteConfirmConfig,
  batchLockEmbyUnbindConfirmConfig,
  batchToggleConfirmConfig,
  buildUsersCacheKey,
  hasStrongAdminPassword,
  toggleSetMember,
  usersBatchFilterParams,
  usersListParams,
} from "./admin-users-helpers";
import { AdminEmailDialog } from "./admin-email-dialog";
import { useSystemStore } from "@/store/system";
import {
  renderEmbyStatusCell,
  renderExpireCell,
  renderRoleBadge,
  renderWebStatusBadge,
  UserActionsMenu,
} from "./admin-users-cells";
import {
  BindEmbyDialog,
  BulkEnableDialog,
  BulkExpireDialog,
  CleanupInvalidUsersDialog,
  DeleteUserDialog,
  EditUserDialog,
  ForceEmbyPasswordDialog,
  KickUnboundDialog,
  NoEmbyKickDialog,
  RenewUserDialog,
  ResetPasswordDialog,
  StandaloneEmbyCreateDialog,
  ToggleActiveDialog,
  WebUserCreateDialog,
} from "./admin-users-dialogs";

type UserSelectionScope = "manual" | "emby" | "all";

export default function AdminUsersPage() {
  const { toast } = useToast();
  const { confirmAction } = useConfirm();
  const { t } = useI18n();
  const [users, setUsers] = useState<UserInfo[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(() =>
    readStoredPerPage(USERS_PER_PAGE_STORAGE_KEY, 20, USERS_PER_PAGE_OPTIONS)
  );
  const [pages, setPages] = useState(1);
  const [search, setSearch] = useState("");
  // 筛选 / 排序
  const [roleFilter, setRoleFilter] = useState<string>("all"); // "all" | "0" | "1" | "2"
  const [activeFilter, setActiveFilter] = useState<string>("all"); // "all" | "true" | "false"
  const [embyFilter, setEmbyFilter] = useState<string>("all"); // "all" | "bound" | "unbound"
  const [embyStatusFilter, setEmbyStatusFilter] = useState<string>("all"); // "all" | "active" | "disabled"
  const [emailStatusFilter, setEmailStatusFilter] = useState<string>("all"); // "all" | "verified" | "unverified" | "bound" | "none"
  const [sortBy, setSortBy] = useState<string>("uid_asc");
  const [expandedUserIds, setExpandedUserIds] = useState<Set<number>>(new Set());

  // Dialog states
  const [renewOpen, setRenewOpen] = useState(false);
  const [renewDays, setRenewDays] = useState("30");
  const [renewPermanent, setRenewPermanent] = useState(false);
  const [renewMode, setRenewMode] = useState<"renew" | "cancelPermanent" | "setExact">("renew");
  const [selectedUser, setSelectedUser] = useState<UserInfo | null>(null);
  const [isActionLoading, setIsActionLoading] = useState(false);

  // 邮箱验证管理弹窗（强制绑定 / 置验证状态）
  const { info: systemInfo, fetchInfo } = useSystemStore();
  const emailEnabled = Boolean(systemInfo?.features?.email_enabled);
  const [emailDialogOpen, setEmailDialogOpen] = useState(false);
  const [emailDialogUser, setEmailDialogUser] = useState<UserInfo | null>(null);
  
  // Cleanup dialog states
  const [cleanupOpen, setCleanupOpen] = useState(false);
  const [cleanupMinDays, setCleanupMinDays] = useState("7");
  const [cleanupPreview, setCleanupPreview] = useState<any[] | null>(null);
  const [cleanupConfirmText, setCleanupConfirmText] = useState("");
  const [cleanupLoading, setCleanupLoading] = useState(false);
  const [stalePendingLoading, setStalePendingLoading] = useState(false);
  const [registrationQueueLoading, setRegistrationQueueLoading] = useState(false);
  const [clearEmailLoading, setClearEmailLoading] = useState(false);

  // Edit dialog states
  const [editOpen, setEditOpen] = useState(false);
  const [editForm, setEditForm] = useState({
    role: 1,
    emby_id: "",
    active: true,
  });

  const [selectedUserIds, setSelectedUserIds] = useState<Set<number>>(new Set());
  const [selectionScope, setSelectionScope] = useState<UserSelectionScope>("manual");
  const [selectionScopeCount, setSelectionScopeCount] = useState(0);
  // 虚拟范围（all / emby）下被逐个取消勾选的 UID。跨页保留，使「选中全部后再
  // 取消个别用户」「反选全部」这类操作不会塌缩成仅当前页。批量请求会把它作为
  // exclude_uids 透传给后端。manual 范围不使用此集合。
  const [excludedUserIds, setExcludedUserIds] = useState<Set<number>>(new Set());
  const [selectEmbyLoading, setSelectEmbyLoading] = useState(false);
  const [batchUserLoading, setBatchUserLoading] = useState(false);

  // 新建 Web 账号（只写本地 users 表）
  const [webUserOpen, setWebUserOpen] = useState(false);
  const [webUserName, setWebUserName] = useState("");
  const [webUserPwd, setWebUserPwd] = useState("");
  const [webUserEmail, setWebUserEmail] = useState("");
  const [webUserRole, setWebUserRole] = useState(1);
  const [webUserSubmitting, setWebUserSubmitting] = useState(false);

  // 删除（含邀请树级联）对话框
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<UserInfo | null>(null);
  const [deleteScope, setDeleteScope] = useState<"with_emby" | "local_only" | "emby_only">("with_emby");
  const [cascadeDepth, setCascadeDepth] = useState<number>(1);
  const [isDeleting, setIsDeleting] = useState(false);

  // 禁用/启用对话框（支持邀请树级联）
  const [toggleOpen, setToggleOpen] = useState(false);
  const [toggleTarget, setToggleTarget] = useState<UserInfo | null>(null);
  const [isToggling, setIsToggling] = useState(false);
  const [toggleCascadeDepth, setToggleCascadeDepth] = useState<number>(1);
  const [toggleReason, setToggleReason] = useState<string>("");

  // 单独禁用 Emby 对话框（带可选理由，记入操作日志）
  const [embyDisableOpen, setEmbyDisableOpen] = useState(false);
  const [embyDisableTarget, setEmbyDisableTarget] = useState<UserInfo | null>(null);
  const [embyDisableReason, setEmbyDisableReason] = useState<string>("");
  const [embyDisableLoading, setEmbyDisableLoading] = useState(false);

  // 新建独立 Emby 账号（不写本地 users 表）
  const [standaloneOpen, setStandaloneOpen] = useState(false);
  const [standaloneName, setStandaloneName] = useState("");
  const [standalonePwd, setStandalonePwd] = useState("");
  const [standaloneEmail, setStandaloneEmail] = useState("");
  const [standaloneSubmitting, setStandaloneSubmitting] = useState(false);

  // 绑定 Emby 到当前系统账号
  const [bindOpen, setBindOpen] = useState(false);
  const [bindTarget, setBindTarget] = useState<UserInfo | null>(null);
  const [bindEmbyName, setBindEmbyName] = useState("");
  const [bindSubmitting, setBindSubmitting] = useState(false);
  const [bindConflict, setBindConflict] = useState<null | {
    emby_id: string;
    emby_username: string;
    conflict_uid: number;
    conflict_username: string;
  }>(null);

  const [bindTgOpen, setBindTgOpen] = useState(false);
  const [bindTgTarget, setBindTgTarget] = useState<UserInfo | null>(null);
  const [bindTgId, setBindTgId] = useState("");
  const [bindTgSubmitting, setBindTgSubmitting] = useState(false);

  // 强制重置 Emby 密码（按 Emby 用户名）
  const [forcePwdOpen, setForcePwdOpen] = useState(false);
  const [forcePwdEmbyName, setForcePwdEmbyName] = useState("");
  const [forcePwdNewPwd, setForcePwdNewPwd] = useState("");
  const [forcePwdAuto, setForcePwdAuto] = useState(true);
  const [forcePwdLoading, setForcePwdLoading] = useState(false);
  const [forcePwdResult, setForcePwdResult] = useState<null | { emby_username: string; linked_local_user: boolean; new_password: string }>(null);

  // 一键批量到期调控（仅按当前筛选条件作用于普通用户）
  const [bulkExpireOpen, setBulkExpireOpen] = useState(false);
  const [bulkExpireMode, setBulkExpireMode] = useState<"permanent" | "days">("permanent");
  const [bulkExpireDays, setBulkExpireDays] = useState<string>("30");
  const [bulkExpireIncludeAdmin, setBulkExpireIncludeAdmin] = useState(false);
  const [bulkExpireIncludeWhitelist, setBulkExpireIncludeWhitelist] = useState(false);
  const [bulkExpireConfirmText, setBulkExpireConfirmText] = useState("");
  const [bulkExpireLoading, setBulkExpireLoading] = useState(false);

  // 批量启用已禁用账号（按当前筛选条件）
  const [bulkEnableOpen, setBulkEnableOpen] = useState(false);
  const [bulkEnableIncludeAdmin, setBulkEnableIncludeAdmin] = useState(false);
  const [bulkEnableIncludeWhitelist, setBulkEnableIncludeWhitelist] = useState(false);
  const [bulkEnableConfirmText, setBulkEnableConfirmText] = useState("");
  const [bulkEnableLoading, setBulkEnableLoading] = useState(false);

  // 一键踢出 TG 群里未绑账号的成员
  type KickPreview = NonNullable<Awaited<ReturnType<typeof api.kickUnboundGroupMembers>>["data"]>;
  type RosterStats = NonNullable<Awaited<ReturnType<typeof api.getTelegramRosterStats>>["data"]>;
  const [kickOpen, setKickOpen] = useState(false);
  const [kickLoading, setKickLoading] = useState(false);
  const [kickPreview, setKickPreview] = useState<KickPreview | null>(null);
  const [kickRoster, setKickRoster] = useState<RosterStats | null>(null);
  const [kickConfirmText, setKickConfirmText] = useState("");
  const [kickResult, setKickResult] = useState<KickPreview | null>(null);

  // 一键踢出所有未绑定 Emby 的系统账号
  type NoEmbyPreview = NonNullable<Awaited<ReturnType<typeof api.kickNoEmbyUsers>>["data"]>;
  const [noEmbyOpen, setNoEmbyOpen] = useState(false);
  const [noEmbyLoading, setNoEmbyLoading] = useState(false);
  const [noEmbyPreview, setNoEmbyPreview] = useState<NoEmbyPreview | null>(null);
  const [noEmbyConfirmText, setNoEmbyConfirmText] = useState("");
  // 0 = 不按注册时间过滤，>0 = 仅清理注册超过 N 天的账号
  const [noEmbyMinDays, setNoEmbyMinDays] = useState("0");
  // 是否保留"已绑 TG / 可自助补建 Emby"的待激活账号
  const [noEmbyPreserveDirect, setNoEmbyPreserveDirect] = useState(true);

  // 重置密码对话框
  const [resetPwdOpen, setResetPwdOpen] = useState(false);
  const [resetPwdUser, setResetPwdUser] = useState<UserInfo | null>(null);
  const [resetPwdScope, setResetPwdScope] = useState<"system" | "emby" | "both">("both");
  const [resetPwdAuto, setResetPwdAuto] = useState(true);
  const [resetPwdCustom, setResetPwdCustom] = useState("");
  const [resetPwdLoading, setResetPwdLoading] = useState(false);
  const [resetPwdResult, setResetPwdResult] = useState<null | {
    scope: "system" | "emby" | "both";
    new_password: string;
    auto_generated: boolean;
  }>(null);

  const usersCacheRef = useRef<
    Map<string, { users: UserInfo[]; total: number; pages: number }>
  >(new Map());
  const loadedUsersRef = useRef(false);

  const invalidateUsersCache = () => {
    usersCacheRef.current.clear();
  };

  const toggleUserDetails = (uid: number) => {
    setExpandedUserIds((prev) => toggleSetMember(prev, uid));
  };

  const loadUsersResource = useCallback(async (signal?: AbortSignal) => {
    const listState = { page, perPage, search, roleFilter, activeFilter, embyFilter, embyStatusFilter, emailStatusFilter, sortBy };
    const cacheKey = buildUsersCacheKey(listState);
    const cached = usersCacheRef.current.get(cacheKey);
    if (cached) {
      setUsers(cached.users);
      setTotal(cached.total);
      setPages(cached.pages);
      return true;
    }

    const res = await api.getUsers(usersListParams(listState), signal);
    if (res.success && res.data) {
      setUsers(res.data.users);
      setTotal(res.data.total);
      setPages(res.data.pages);
      usersCacheRef.current.set(cacheKey, {
        users: res.data.users,
        total: res.data.total,
        pages: res.data.pages,
      });
    }
    return true;
  }, [page, perPage, search, roleFilter, activeFilter, embyFilter, embyStatusFilter, emailStatusFilter, sortBy]);

  const {
    isLoading,
    error,
    execute: loadUsers,
  } = useAsyncResource(loadUsersResource, { immediate: true });

  // 拉取系统信息以判定邮箱功能是否启用：深链直达 /admin/users 时也能正确显示
  // 邮箱验证筛选 / 行内状态徽标。store 自带 TTL 与在飞去重，重复调用开销可忽略。
  useEffect(() => {
    void fetchInfo();
  }, [fetchInfo]);

  const selectedUsers = useMemo(
    () => users.filter((user) => selectedUserIds.has(user.uid)),
    [users, selectedUserIds],
  );
  const selectedUids = useMemo(() => Array.from(selectedUserIds), [selectedUserIds]);
  const selectedCount =
    selectionScope === "all"
      ? Math.max(0, total - excludedUserIds.size)
      : selectionScope === "emby"
        ? Math.max(0, selectionScopeCount - excludedUserIds.size)
        : selectedUserIds.size;
  const selectedEmbyCount = useMemo(
    () => selectedUsers.filter((user) => Boolean(user.emby_id)).length,
    [selectedUsers],
  );
  const selectedAdminCount = useMemo(
    () => selectedUsers.filter((user) => user.role === 0).length,
    [selectedUsers],
  );
  const currentListState = useMemo(
    () => ({ page, perPage, search, roleFilter, activeFilter, embyFilter, embyStatusFilter, emailStatusFilter, sortBy }),
    [page, perPage, search, roleFilter, activeFilter, embyFilter, embyStatusFilter, emailStatusFilter, sortBy],
  );

  const isUserSelected = (user: UserInfo) =>
    selectionScope === "all"
      ? !excludedUserIds.has(user.uid)
      : selectionScope === "emby"
        ? Boolean(user.emby_id) && !excludedUserIds.has(user.uid)
        : selectedUserIds.has(user.uid);

  const allPageSelected = users.length > 0 && users.every(isUserSelected);

  const clearUserSelection = () => {
    setSelectedUserIds(new Set());
    setExcludedUserIds(new Set());
    setSelectionScope("manual");
    setSelectionScopeCount(0);
  };

  // 切换每页数量：持久化偏好、重置到第 1 页并清空选择。
  // 清空选择是因为 manual 范围选中的是当前页 UID，换页大小后页边界变化，
  // 保留旧选中集会让用户对“当前页全选”状态产生误解；scope 类选择同样重置更直观。
  const handlePerPageChange = (value: number) => {
    if (value === perPage) return;
    storePerPage(USERS_PER_PAGE_STORAGE_KEY, value);
    clearUserSelection();
    invalidateUsersCache();
    setExpandedUserIds(new Set());
    setPage(1);
    setPerPage(value);
  };

  const embyMatchingFilter = () => ({
    ...usersBatchFilterParams(currentListState),
    emby: "bound" as const,
  });

  const selectedBatchTarget = () => (
    selectionScope === "all"
      ? { select_all: true, filter: usersBatchFilterParams(currentListState), exclude_uids: Array.from(excludedUserIds) }
      : selectionScope === "emby"
        ? { select_all: true, filter: embyMatchingFilter(), exclude_uids: Array.from(excludedUserIds) }
      : selectedUids
  );

  // 行级勾选：manual 范围维护 includes 集；all / emby 虚拟范围维护 excludes 集，
  // 这样取消勾选某行不会丢掉其它页的选择（旧实现会塌缩成仅当前页）。
  const toggleSelectedUser = (uid: number) => {
    if (selectionScope === "manual") {
      setSelectedUserIds((prev) => toggleSetMember(prev, uid));
      return;
    }
    if (selectionScope === "emby") {
      const target = users.find((user) => user.uid === uid);
      if (!target || !target.emby_id) return; // emby 范围只作用于已绑定行
    }
    setExcludedUserIds((prev) => toggleSetMember(prev, uid));
  };

  // 当前页可勾选的行：emby 范围下仅已绑定 Emby 的行参与。
  const selectablePageRows = () =>
    selectionScope === "emby" ? users.filter((user) => Boolean(user.emby_id)) : users;

  const toggleSelectCurrentPage = () => {
    if (selectionScope === "manual") {
      setSelectedUserIds((prev) => {
        const next = new Set(prev);
        if (allPageSelected) users.forEach((user) => next.delete(user.uid));
        else users.forEach((user) => next.add(user.uid));
        return next;
      });
      return;
    }
    // all / emby：用排除集翻转当前页，保留跨页选择
    setExcludedUserIds((prev) => {
      const next = new Set(prev);
      const rows = selectablePageRows();
      if (allPageSelected) rows.forEach((user) => next.add(user.uid));
      else rows.forEach((user) => next.delete(user.uid));
      return next;
    });
  };

  const selectCurrentPageUsers = () => {
    setSelectionScope("manual");
    setSelectionScopeCount(0);
    setExcludedUserIds(new Set());
    setSelectedUserIds(new Set(users.map((user) => user.uid)));
  };

  // 反选本页：仅翻转当前页可见行的勾选态，不改变范围（manual 翻 includes，
  // all / emby 翻 excludes），跨页选择保持不变。
  const invertPageSelection = () => {
    if (users.length === 0) return;
    if (selectionScope === "manual") {
      setSelectedUserIds((prev) => {
        const next = new Set(prev);
        users.forEach((user) => {
          if (next.has(user.uid)) next.delete(user.uid);
          else next.add(user.uid);
        });
        return next;
      });
      return;
    }
    setExcludedUserIds((prev) => {
      const next = new Set(prev);
      selectablePageRows().forEach((user) => {
        if (next.has(user.uid)) next.delete(user.uid);
        else next.add(user.uid);
      });
      return next;
    });
  };

  // 反选全部：对整个筛选结果取补集（跨页）。
  //   - all 范围：补集 =「当前被排除的具体 UID」→ 落为 manual 精确选中；排除集为空
  //     表示全选，补集为空 → 清空。
  //   - manual 范围：补集 =「全部匹配 − 当前选中」→ 落为 all 范围且排除当前选中集。
  //   - emby 范围：补集无法用单一 select_all filter 精确表达（含未绑定行），按钮禁用。
  const invertAllSelection = () => {
    if (total === 0 || selectionScope === "emby") return;
    if (selectionScope === "all") {
      if (excludedUserIds.size === 0) {
        clearUserSelection();
        return;
      }
      const revived = new Set(excludedUserIds);
      setExcludedUserIds(new Set());
      setSelectionScope("manual");
      setSelectionScopeCount(0);
      setSelectedUserIds(revived);
      return;
    }
    // manual → all minus 当前选中
    const exclude = new Set(selectedUserIds);
    setSelectedUserIds(new Set());
    setExcludedUserIds(exclude);
    setSelectionScope("all");
    setSelectionScopeCount(total);
  };

  const selectAllMatchingUsers = () => {
    setSelectedUserIds(new Set());
    setExcludedUserIds(new Set());
    setSelectionScope("all");
    setSelectionScopeCount(total);
  };

  const selectEmbyMatchingUsers = async () => {
    setSelectEmbyLoading(true);
    try {
      const res = await api.getUsers({
        ...usersListParams(currentListState),
        page: 1,
        per_page: 1,
        emby: "bound",
      });
      const count = res.success && res.data ? res.data.total : users.filter((user) => Boolean(user.emby_id)).length;
      if (count <= 0) {
        clearUserSelection();
        toast({ title: "没有可选择的 Emby 用户", description: "当前筛选条件下没有已绑定 Emby 的用户。" });
        return;
      }
      setSelectedUserIds(new Set());
      setExcludedUserIds(new Set());
      setSelectionScope("emby");
      setSelectionScopeCount(count);
    } catch (error: any) {
      toast({ title: "选择失败", description: error.message || "无法获取拥有 Emby 的用户数量", variant: "destructive" });
    } finally {
      setSelectEmbyLoading(false);
    }
  };

  const handleSearch = () => {
    invalidateUsersCache();
    setExpandedUserIds(new Set());
    clearUserSelection();
    if (page !== 1) {
      setPage(1);
      return;
    }
    void loadUsers();
  };

  useEffect(() => {
    if (loadedUsersRef.current) {
      void loadUsers();
    } else {
      loadedUsersRef.current = true;
    }
  }, [page, perPage, loadUsers]);

  // 筛选/排序变更：重置页码并立即刷新（绕过 perPage useEffect 的边界条件）
  useEffect(() => {
    if (!loadedUsersRef.current) return;
    setPage(1);
    invalidateUsersCache();
    setExpandedUserIds(new Set());
    void loadUsers();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [roleFilter, activeFilter, embyFilter, embyStatusFilter, emailStatusFilter, sortBy]);

  useEffect(() => {
    clearUserSelection();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [roleFilter, activeFilter, embyFilter, embyStatusFilter, emailStatusFilter, sortBy]);

  const handleForceSetEmbyPassword = async () => {
    const emby = forcePwdEmbyName.trim();
    if (!emby) {
      toast({ title: "请输入 Emby 用户名", variant: "destructive" });
      return;
    }
    if (!forcePwdAuto) {
      const pwd = forcePwdNewPwd;
      if (!hasStrongAdminPassword(pwd)) {
        toast({
          title: "密码强度不足",
          description: "至少 8 位，且包含大小写字母和数字",
          variant: "destructive",
        });
        return;
      }
    }
    setForcePwdLoading(true);
    try {
      const res = await api.adminForceSetEmbyPassword(
        emby,
        forcePwdAuto ? undefined : forcePwdNewPwd,
      );
      if (res.success && res.data) {
        setForcePwdResult({
          emby_username: res.data.emby_username,
          linked_local_user: res.data.linked_local_user,
          new_password: res.data.new_password,
        });
        toast({
          title: "Emby 密码已重置",
          description: res.data.linked_local_user
            ? "本地账号密码已同步"
            : "该 Emby 用户当前未绑定本地账号",
          variant: "success",
        });
      } else {
        toast({ title: "重置失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "重置失败", description: error.message, variant: "destructive" });
    } finally {
      setForcePwdLoading(false);
    }
  };

  const handleRenew = async () => {
    if (!selectedUser || (!renewPermanent && !renewDays)) return;

    setIsActionLoading(true);
    try {
      const days = renewPermanent ? -1 : parseInt(renewDays, 10);
      const res = renewMode === "cancelPermanent"
        ? await api.cancelUserPermanent(selectedUser.uid, days)
        : renewMode === "setExact"
          ? await api.adminSetUserExpiry(selectedUser.uid, renewPermanent ? { permanent: true } : { days })
          : await api.renewUser(selectedUser.uid, days);
      if (res.success) {
        toast({
          title:
            renewMode === "cancelPermanent"
              ? "已取消永久"
              : renewMode === "setExact"
                ? "到期时间已设置"
                : "续期成功",
          variant: "success",
        });
        setRenewOpen(false);
        setSelectedUser(null);
        invalidateUsersCache();
        loadUsers();
      } else {
        toast({ title: "续期失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "续期失败", description: error.message, variant: "destructive" });
    } finally {
      setIsActionLoading(false);
    }
  };

  const handleResetPassword = (user: UserInfo) => {
    // 没绑 Emby 的账号默认只能重置系统密码
    const defaultScope: "system" | "emby" | "both" = user.emby_bound ? "both" : "system";
    setResetPwdUser(user);
    setResetPwdScope(defaultScope);
    setResetPwdAuto(true);
    setResetPwdCustom("");
    setResetPwdResult(null);
    setResetPwdOpen(true);
  };

  const handleResetPasswordSubmit = async () => {
    if (!resetPwdUser) return;
    if (!resetPwdAuto && !resetPwdCustom.trim()) {
      toast({ title: "请输入新密码", description: "或勾选随机生成", variant: "destructive" });
      return;
    }
    setResetPwdLoading(true);
    try {
      const res = await api.resetPassword(resetPwdUser.uid, {
        scope: resetPwdScope,
        password: resetPwdAuto ? undefined : resetPwdCustom.trim(),
      });
      if (res.success && res.data) {
        setResetPwdResult(res.data);
        toast({ title: "密码已重置", variant: "success" });
      } else {
        toast({ title: "重置失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "重置失败", description: error.message, variant: "destructive" });
    } finally {
      setResetPwdLoading(false);
    }
  };

  const handleCreateStandaloneEmby = async () => {
    if (!standaloneName.trim() || !standalonePwd) {
      toast({ title: "请填写用户名与密码", variant: "destructive" });
      return;
    }
    setStandaloneSubmitting(true);
    try {
      const res = await api.adminCreateStandaloneEmby({
        username: standaloneName.trim(),
        password: standalonePwd,
        email: standaloneEmail.trim() || undefined,
      });
      if (res.success && res.data) {
        toast({
          title: "Emby 账号已创建",
          description: `用户名：${res.data.emby_username}（未绑定任何系统账号）`,
          variant: "success",
        });
        setStandaloneOpen(false);
        setStandaloneName("");
        setStandalonePwd("");
        setStandaloneEmail("");
      } else {
        toast({ title: "创建失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "创建出错", description: err?.message, variant: "destructive" });
    } finally {
      setStandaloneSubmitting(false);
    }
  };

  const handleCreateWebUser = async () => {
    if (!webUserName.trim()) {
      toast({ title: "请填写用户名", variant: "destructive" });
      return;
    }
    setWebUserSubmitting(true);
    try {
      const res = await api.adminCreateUser({
        username: webUserName.trim(),
        password: webUserPwd || undefined,
        email: webUserEmail.trim() || undefined,
        role: webUserRole,
      });
      if (res.success && res.data) {
        toast({
          title: "Web 账号已创建",
          description: `用户名：${res.data.user.username}，初始密码：${res.data.password}`,
          variant: "success",
        });
        setWebUserOpen(false);
        setWebUserName("");
        setWebUserPwd("");
        setWebUserEmail("");
        setWebUserRole(1);
        invalidateUsersCache();
        await loadUsers();
      } else {
        toast({ title: "创建失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "创建出错", description: err?.message || "网络异常", variant: "destructive" });
    } finally {
      setWebUserSubmitting(false);
    }
  };

  const handleOpenBindEmby = (user: UserInfo) => {
    setBindTarget(user);
    setBindEmbyName(user.emby_username || "");
    setBindConflict(null);
    setBindOpen(true);
  };

  const submitBindEmby = async (force: boolean) => {
    if (!bindTarget) return;
    const name = bindEmbyName.trim();
    if (!name) {
      toast({ title: "请输入 Emby 用户名", variant: "destructive" });
      return;
    }
    setBindSubmitting(true);
    try {
      const res = await api.adminBindEmbyToUser(bindTarget.uid, { emby_username: name, force });
      if (res.success && res.data) {
        const takeOver = res.data.force_taken ? `（已从 UID ${res.data.previous_uid} 夺取）` : "";
        toast({
          title: "绑定成功",
          description: `${bindTarget.username} → ${res.data.emby_username} ${takeOver}`,
          variant: "success",
        });
        invalidateUsersCache();
        await loadUsers();
        setBindOpen(false);
        setBindConflict(null);
      } else {
        toast({ title: "绑定失败", description: res.message, variant: "destructive" });
      }
    } catch (error: unknown) {
      // 后端把"目标 emby 账号已被其它用户绑定"从 200+data.conflict 改为
      // 409 + ErrCode=EMBY_ACCOUNT_CONFLICT，conflict 元数据塞在 envelope.data
      // 上由 ApiError.data 透传过来。其它失败仍走默认 toast。
      if (error instanceof ApiError && error.errorCode === ErrCodes.EmbyAccountConflict) {
        const conflictData = error.data as
          | {
              emby_id?: string;
              emby_username?: string;
              conflict_uid?: number;
              conflict_username?: string;
            }
          | null
          | undefined;
        if (
          conflictData &&
          typeof conflictData.conflict_uid === "number" &&
          typeof conflictData.conflict_username === "string" &&
          typeof conflictData.emby_id === "string" &&
          typeof conflictData.emby_username === "string"
        ) {
          setBindConflict({
            emby_id: conflictData.emby_id,
            emby_username: conflictData.emby_username,
            conflict_uid: conflictData.conflict_uid,
            conflict_username: conflictData.conflict_username,
          });
          return;
        }
      }
      const message = error instanceof Error ? error.message : "绑定失败";
      toast({ title: "绑定失败", description: message, variant: "destructive" });
    } finally {
      setBindSubmitting(false);
    }
  };

  const handleOpenBindTelegram = (user: UserInfo) => {
    setBindTgTarget(user);
    setBindTgId("");
    setBindTgOpen(true);
  };

  const submitBindTelegram = async () => {
    if (!bindTgTarget) return;
    const tgId = parseInt(bindTgId.trim(), 10);
    if (!tgId || tgId <= 0) {
      toast({ title: "请输入有效的 Telegram ID", variant: "destructive" });
      return;
    }
    setBindTgSubmitting(true);
    try {
      const res = await api.adminBindTelegramToUser(bindTgTarget.uid, tgId);
      if (res.success) {
        toast({
          title: "绑定成功",
          description: `${bindTgTarget.username} → Telegram ID ${res.data?.telegram_id}`,
          variant: "success",
        });
        invalidateUsersCache();
        loadUsers();
        setBindTgOpen(false);
      } else {
        toast({ title: "绑定失败", description: res.message, variant: "destructive" });
      }
    } catch (error: unknown) {
      if (error instanceof ApiError && error.errorCode === ErrCodes.TGIDTaken) {
        toast({ title: "绑定失败", description: "该 Telegram ID 已被其他用户绑定", variant: "destructive" });
        return;
      }
      const message = error instanceof Error ? error.message : "绑定失败";
      toast({ title: "绑定失败", description: message, variant: "destructive" });
    } finally {
      setBindTgSubmitting(false);
    }
  };

  const handleForceUnbind = async (user: UserInfo) => {
    const scope = await confirmAction({
      title: `强制解绑 ${user.username}`,
      description: "仅解除本地绑定关系，不删除 Telegram/Emby 外部账号。",
      tone: "warning",
      actions: [
        { label: "解绑 TG", value: "telegram", variant: "outline" },
        { label: "解绑 Emby", value: "emby", variant: "outline" },
        { label: "全部解绑", value: "both", variant: "destructive" },
      ],
    });
    if (!scope) return;
    try {
      const res = await api.forceUnbindUser(user.uid, scope as "telegram" | "emby" | "both");
      if (res.success) {
        toast({ title: "解绑完成", description: res.message, variant: "success" });
        invalidateUsersCache();
        await loadUsers();
      } else {
        toast({ title: "解绑失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "解绑失败", description: err.message || "网络异常", variant: "destructive" });
    }
  };

  const handleClearRegistrationQueue = async (user: UserInfo) => {
    const ok = await confirmAction({
      title: `清理 ${user.username} 的注册队列状态`,
      description: "会清理该用户残留的注册码处理队列和 Emby 注册队列状态，用于修复旧队列堆积导致无法重新使用注册码的问题。正在执行中的外部 Emby 创建请求无法被强制中断。",
      tone: "warning",
      confirmLabel: "清理队列",
    });
    if (!ok) return;
    try {
      const res = await api.clearUserRegistrationQueue(user.uid);
      if (res.success) {
        toast({ title: "清理完成", description: res.message, variant: "success" });
        invalidateUsersCache();
        await loadUsers();
      } else {
        toast({ title: "清理失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "清理失败", description: err.message || "网络异常", variant: "destructive" });
    }
  };

  const handleGrantRegistrationEntitlement = async (user: UserInfo) => {
    const action = await confirmAction({
      title: `给予 ${user.username} Emby 注册权利`,
      description: "会清理该用户残留队列，并授予一次待补建 Emby 资格。用户登录后会在仪表盘注册码兑换下方看到开通入口。默认开通时长沿用后端配置。",
      tone: "warning",
      actions: [
        { label: "授予默认时长", value: "default", variant: "default" },
        { label: "授予永久", value: "permanent", variant: "outline" },
      ],
    });
    if (!action) return;
    try {
      const days = action === "permanent" ? -1 : undefined;
      const res = await api.grantUserRegistrationEntitlement(user.uid, days);
      if (res.success) {
        toast({ title: "已授予注册权利", description: res.message, variant: "success" });
        invalidateUsersCache();
        await loadUsers();
      } else {
        toast({ title: "授予失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "授予失败", description: err.message || "网络异常", variant: "destructive" });
    }
  };

  const handleGrantRegistrationEntitlementAndDequeue = async (user: UserInfo) => {
    const action = await confirmAction({
      title: `授权并移出 ${user.username} 的未处理队列`,
      description: "会先授予该用户 Emby 补建资格，然后只把尚未开始处理的注册码/Emby 注册请求从队列移出。若请求已经 processing，将保留执行状态，避免破坏正在创建的 Emby 账号。",
      tone: "warning",
      actions: [
        { label: "授权默认时长并移出", value: "default", variant: "default" },
        { label: "授权永久并移出", value: "permanent", variant: "outline" },
      ],
    });
    if (!action) return;
    try {
      const days = action === "permanent" ? -1 : undefined;
      const res = await api.grantUserRegistrationEntitlementAndDequeue(user.uid, days);
      if (res.success) {
        toast({
          title: res.data?.dequeued ? "已授权并移出队列" : "已授权",
          description: res.message,
          variant: res.data?.processing_blocked?.length ? "default" : "success",
        });
        invalidateUsersCache();
        await loadUsers();
      } else {
        toast({ title: "操作失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "操作失败", description: err.message || "网络异常", variant: "destructive" });
    }
  };

  const handleSyncBindings = async (options: { uid?: number; currentFilter?: boolean } = {}) => {
    const ok = await confirmAction({
      title: options.uid ? "同步该用户绑定状态" : options.currentFilter ? "同步当前筛选用户" : "同步所有用户",
      description: "会检查 TG/Emby 绑定，清理非法/重复 TGID 与失效 EmbyID，并同步 Emby 启停状态。",
      tone: "warning",
      confirmLabel: "开始同步",
    });
    if (!ok) return;
    const filter: { role?: number; active?: boolean; emby?: "bound" | "unbound"; email_status?: "verified" | "unverified" | "bound" | "none"; search?: string } = {};
    if (options.currentFilter) {
      if (roleFilter !== "all") filter.role = Number(roleFilter);
      if (activeFilter !== "all") filter.active = activeFilter === "true";
      if (embyFilter === "bound") filter.emby = "bound";
      else if (embyFilter === "unbound") filter.emby = "unbound";
      if (emailStatusFilter !== "all") filter.email_status = emailStatusFilter as "verified" | "unverified" | "bound" | "none";
      if (search.trim()) filter.search = search.trim();
    }
    try {
      const res = await api.syncUserBindings({
        scope: "both",
        uid: options.uid,
        filter: options.uid ? undefined : options.currentFilter ? filter : undefined,
        repair: true,
      });
      if (res.success && res.data) {
        toast({
          title: "同步完成",
          description: `匹配 ${res.data.matched}，TG修复 ${res.data.telegram_repaired}，Emby修复 ${res.data.emby_repaired}，失败 ${res.data.failed.length}`,
          variant: res.data.failed.length ? "default" : "success",
        });
        invalidateUsersCache();
        await loadUsers();
      } else {
        toast({ title: "同步失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "同步失败", description: err.message || "网络异常", variant: "destructive" });
    }
  };

  // 强制刷新单个用户的外部状态：scope=telegram 仅拉取最新 TG 用户名、scope=emby 仅核对
  // Emby 启停。TG 同步与 Emby 同步分开触发，便于管理员只刷需要的那一侧。
  const handleRefreshStatus = async (user: UserInfo, scope: "telegram" | "emby") => {
    try {
      const res = await api.refreshUserStatus(user.uid, scope);
      if (res.success && res.data) {
        const r = res.data.refresh ?? {};
        const notes: string[] = [];
        if (r.telegram_username_updated) notes.push(t("adminUsers.refreshTgUpdated"));
        if (r.emby_username_updated) notes.push(t("adminUsers.refreshEmbyNameUpdated"));
        if (r.emby_disabled_synced) notes.push(t("adminUsers.refreshEmbyDisabled"));
        if (r.emby_missing) notes.push(t("adminUsers.refreshEmbyMissing"));
        const errs: string[] = [];
        if (r.telegram_error) errs.push(`Telegram: ${r.telegram_error}`);
        if (r.emby_error) errs.push(`Emby: ${r.emby_error}`);
        toast({
          title: t("adminUsers.refreshStatusDone"),
          description: notes.length ? notes.join("，") : t("adminUsers.refreshNoChange"),
          variant: errs.length ? "default" : "success",
        });
        if (errs.length) {
          toast({ title: t("adminUsers.refreshPartialFail"), description: errs.join("；"), variant: "destructive" });
        }
        invalidateUsersCache();
        await loadUsers();
      } else {
        toast({ title: t("adminUsers.refreshStatusFail"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("adminUsers.refreshStatusFail"), description: err.message || "网络异常", variant: "destructive" });
    }
  };

  // 单独启停某用户的 Emby 账号（不动 Web 账号）。启用方向后端会再校验 Web 未禁用/未过期。
  // 禁用方向走带理由输入的专用对话框（理由记入操作日志）；启用方向无需理由，沿用简单确认。
  const handleEmbyToggle = async (user: UserInfo, enable: boolean) => {
    if (!enable) {
      setEmbyDisableTarget(user);
      setEmbyDisableReason("");
      setEmbyDisableOpen(true);
      return;
    }
    const confirmed = await confirmAction({
      title: t("adminUsers.embyToggleEnableTitle"),
      description: t("adminUsers.embyToggleEnableDesc"),
      tone: "warning",
      confirmLabel: t("adminUsers.menuEmbyEnable"),
    });
    if (!confirmed) return;
    try {
      const res = await api.setUserEmbyEnabled(user.uid, true);
      if (res.success) {
        toast({ title: t("adminUsers.embyToggleEnabled"), variant: "success" });
        invalidateUsersCache();
        await loadUsers();
      } else {
        toast({ title: t("adminUsers.embyToggleFail"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("adminUsers.embyToggleFail"), description: err.message || "网络异常", variant: "destructive" });
    }
  };

  // 提交单独禁用 Emby（携带可选理由）。
  const confirmEmbyDisable = async () => {
    if (!embyDisableTarget) return;
    setEmbyDisableLoading(true);
    try {
      const res = await api.setUserEmbyEnabled(embyDisableTarget.uid, false, embyDisableReason.trim() || undefined);
      if (res.success) {
        toast({ title: t("adminUsers.embyToggleDisabled"), variant: "success" });
        invalidateUsersCache();
        await loadUsers();
        setEmbyDisableOpen(false);
      } else {
        toast({ title: t("adminUsers.embyToggleFail"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("adminUsers.embyToggleFail"), description: err.message || "网络异常", variant: "destructive" });
    } finally {
      setEmbyDisableLoading(false);
    }
  };

  const handleToggleActive = (user: UserInfo) => {
    setToggleTarget(user);
    setToggleCascadeDepth(1);
    setToggleReason("");
    setToggleOpen(true);
  };

  const confirmToggleActive = async () => {
    if (!toggleTarget) return;
    setIsToggling(true);
    try {
      const res = await api.toggleUserActive(toggleTarget.uid, {
        enable: !toggleTarget.active,
        cascadeDepth: toggleCascadeDepth,
        reason: toggleReason.trim() || undefined,
      });
      if (res.success) {
        const action = toggleTarget.active ? "禁用" : "启用";
        const affected = res.data?.affected?.length ?? 0;
        const skipped = res.data?.skipped?.length ?? 0;
        const desc = toggleCascadeDepth !== 1
          ? `成功 ${affected}，跳过 ${skipped}（层级 ${toggleCascadeDepth === 0 ? "整棵子树" : toggleCascadeDepth}）`
          : undefined;
        toast({ title: `已${action}`, description: desc, variant: "success" });
        // 本地禁用成功但 Emby 远端关停失败时，后端会返回 emby_failed 让管理员知晓
        // 「Web 已禁用、Emby 可能仍可登录」，需要手动复核而不是以为已彻底禁用。
        const embyFailed = res.data?.emby_failed;
        if (Array.isArray(embyFailed) && embyFailed.length > 0) {
          toast({
            title: t("adminUsers.embySyncFailedTitle"),
            description: t("adminUsers.embySyncFailedDesc", { count: embyFailed.length }),
            variant: "destructive",
          });
        }
        invalidateUsersCache();
        loadUsers();
        setToggleOpen(false);
      } else {
        toast({ title: "操作失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "操作失败", description: error.message, variant: "destructive" });
    } finally {
      setIsToggling(false);
    }
  };

  const handleDelete = (user: UserInfo) => {
    setDeleteTarget(user);
    setDeleteScope(user.emby_id ? "with_emby" : "local_only");
    setCascadeDepth(1);
    setDeleteOpen(true);
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setIsDeleting(true);
    try {
      const res = await api.deleteUserScoped(deleteTarget.uid, {
        mode: deleteScope,
        cascadeDepth, // 0 表示整棵子树
      });

      if (res.success) {
        const summary =
          res.data && (res.data.deleted?.length ?? 0) > 1
            ? `成功 ${res.data.deleted.length}，跳过 ${res.data.skipped?.length ?? 0}`
            : undefined;
        const titles: Record<string, string> = {
          with_emby: "用户与 Emby 账户已删除",
          local_only: "本地账户已删除（Emby 保留）",
          emby_only: "Emby 账户已删除（本地与邀请关系保留）",
        };
        toast({
          title: cascadeDepth !== 1 ? `级联完成（层级 ${cascadeDepth === 0 ? "整棵子树" : cascadeDepth}）` : titles[deleteScope],
          description: summary,
          variant: "success",
        });
        invalidateUsersCache();
        loadUsers();
        setDeleteOpen(false);
      } else {
        toast({ title: "操作失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "操作失败", description: error.message, variant: "destructive" });
    } finally {
      setIsDeleting(false);
    }
  };

  const handleOpenEdit = (user: UserInfo) => {
    setSelectedUser(user);
    setEditForm({
      role: user.role,
      emby_id: user.emby_id || "",
      active: user.active,
    });
    setEditOpen(true);
  };

  const handleEdit = async () => {
    if (!selectedUser) return;

    setIsActionLoading(true);
    try {
      const res = await api.updateUser(selectedUser.uid, editForm);
      if (res.success) {
        toast({ title: "更新成功", variant: "success" });
        // 改成禁用时若 Emby 远端关停失败，后端返回 emby_sync_failed，提示管理员复核。
        if (res.data?.emby_sync_failed) {
          toast({
            title: t("adminUsers.embySyncFailedTitle"),
            description: t("adminUsers.embySyncFailedDesc", { count: 1 }),
            variant: "destructive",
          });
        }
        setEditOpen(false);
        setSelectedUser(null);
        invalidateUsersCache();
        loadUsers();
      } else {
        toast({ title: "更新失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "更新失败", description: error.message, variant: "destructive" });
    } finally {
      setIsActionLoading(false);
    }
  };

  const refreshAfterBatch = async () => {
    invalidateUsersCache();
    clearUserSelection();
    await loadUsers();
  };

  const handleSelectedToggleActive = async (enable: boolean) => {
    if (selectedCount === 0) return;
    const ok = await confirmAction(batchToggleConfirmConfig(enable, selectedCount, t));
    if (!ok) return;
    setBatchUserLoading(true);
    try {
      const res = await api.batchToggleUsers(selectedBatchTarget(), enable);
      if (res.success && res.data) {
        toast({
          title: enable ? "批量启用完成" : "批量禁用完成",
          description: `成功 ${res.data.success} 个，失败 ${res.data.failed} 个`,
          variant: res.data.failed ? "default" : "success",
        });
        await refreshAfterBatch();
      } else {
        toast({ title: "批量操作失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "批量操作失败", description: error.message, variant: "destructive" });
    } finally {
      setBatchUserLoading(false);
    }
  };

  const handleSelectedLockEmbyUnbind = async () => {
    if (selectedCount === 0) return;
    const ok = await confirmAction(batchLockEmbyUnbindConfirmConfig(selectedCount, t));
    if (!ok) return;
    setBatchUserLoading(true);
    try {
      const res = await api.batchLockEmbyUnbind(selectedBatchTarget());
      if (res.success && res.data) {
        const skippedNoEmby = res.data.skipped_no_emby || 0;
        toast({
          title: "已禁止自助解绑 Emby",
          description: `成功 ${res.data.success} 个，跳过未绑定 Emby ${skippedNoEmby} 个，失败 ${res.data.failed} 个`,
          variant: res.data.failed ? "default" : "success",
        });
        await refreshAfterBatch();
      } else {
        toast({ title: "批量操作失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "批量操作失败", description: error.message, variant: "destructive" });
    } finally {
      setBatchUserLoading(false);
    }
  };

  const handleSelectedClearEmbyGrant = async () => {
    if (selectedCount === 0) return;
    const ok = await confirmAction(batchClearEmbyGrantConfirmConfig(selectedCount, t));
    if (!ok) return;
    setBatchUserLoading(true);
    try {
      const res = await api.batchClearEmbyGrant(selectedBatchTarget());
      if (res.success && res.data) {
        const cleared = res.data.cleared ?? res.data.success;
        const skippedHasEmby = res.data.skipped_has_emby || 0;
        const skippedPending = res.data.skipped_pending || 0;
        toast({
          title: "已清理注册资格记录",
          description: `清理 ${cleared} 个，跳过已绑定 Emby ${skippedHasEmby} 个，跳过待开通 ${skippedPending} 个，失败 ${res.data.failed} 个`,
          variant: res.data.failed ? "default" : "success",
        });
        await refreshAfterBatch();
      } else {
        toast({ title: "批量操作失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "批量操作失败", description: error.message, variant: "destructive" });
    } finally {
      setBatchUserLoading(false);
    }
  };

  // 批量单独启停 Emby（保留 Web）。与逐个操作同源，受保护/未绑定自动跳过。
  const handleSelectedEmbyToggle = async (enable: boolean) => {
    if (selectedCount === 0) return;
    const confirmed = await confirmAction({
      title: enable ? "批量启用 Emby 账号？" : "批量禁用 Emby 账号（保留 Web）？",
      description: `将对 ${selectedCount} 个已选用户单独${enable ? "启用" : "禁用"} Emby 账号，不改动其网页账号。受保护账号与未绑定 Emby 的用户会自动跳过。`,
      tone: "warning",
      confirmLabel: enable ? "启用 Emby" : "禁用 Emby",
    });
    if (!confirmed) return;
    setBatchUserLoading(true);
    try {
      const res = await api.batchToggleEmby(selectedBatchTarget(), enable);
      if (res.success && res.data) {
        const skippedNoEmby = res.data.skipped_no_emby || 0;
        toast({
          title: enable ? "批量启用 Emby 完成" : "批量禁用 Emby 完成",
          description: `成功 ${res.data.success} 个，跳过未绑定 ${skippedNoEmby} 个，失败 ${res.data.failed} 个`,
          variant: res.data.failed ? "default" : "success",
        });
        await refreshAfterBatch();
      } else {
        toast({ title: "批量操作失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "批量操作失败", description: error.message, variant: "destructive" });
    } finally {
      setBatchUserLoading(false);
    }
  };

  const handleSelectedGrantAllLibraries = async () => {
    if (selectedCount === 0) return;
    const confirmed = await confirmAction({
      title: t("adminUsers.batchGrantLibrariesTitle"),
      description: t("adminUsers.batchGrantLibrariesDescription", { count: selectedCount }),
      tone: "warning",
      confirmLabel: t("adminUsers.batchGrantLibrariesConfirm"),
    });
    if (!confirmed) return;
    setBatchUserLoading(true);
    try {
      const res = await api.batchGrantAllLibraries(selectedBatchTarget());
      if (res.success && res.data) {
        const skippedNoEmby = res.data.skipped_no_emby || 0;
        const skippedRemoteAdmin = res.data.skipped_remote_admin || 0;
        toast({
          title: t("adminUsers.batchGrantLibrariesDone"),
          description: t("adminUsers.batchGrantLibrariesResult", {
            success: res.data.success,
            skipped: skippedNoEmby,
            admin: skippedRemoteAdmin,
            failed: res.data.failed,
          }),
          variant: res.data.failed ? "default" : "success",
        });
        await refreshAfterBatch();
      } else {
        toast({ title: t("adminUsers.batchFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminUsers.batchFailed"), description: error.message, variant: "destructive" });
    } finally {
      setBatchUserLoading(false);
    }
  };

  // 批量强制刷新外部状态。scope 分开触发：telegram 仅刷 TG 用户名、emby 仅核对 Emby 启停。
  // 不限制数量，对所选全部用户处理；顺序外发，目标较多时耗时较长。
  const handleSelectedRefreshStatus = async (scope: "telegram" | "emby") => {
    if (selectedCount === 0) return;
    const label = scope === "telegram" ? "Telegram 用户名" : "Emby 启停状态";
    const confirmed = await confirmAction({
      title: scope === "telegram" ? "批量刷新 Telegram 用户名？" : "批量刷新 Emby 状态？",
      description: `将对 ${selectedCount} 个已选用户刷新${label}。这会对外发起请求，目标较多时耗时较长，请耐心等待。`,
      tone: "warning",
      confirmLabel: "开始刷新",
    });
    if (!confirmed) return;
    setBatchUserLoading(true);
    try {
      const res = await api.batchRefreshStatus(selectedBatchTarget(), scope);
      if (res.success && res.data) {
        const parts = [`处理 ${res.data.success} 个`];
        if (scope !== "emby") parts.push(`TG 更新 ${res.data.telegram_updated ?? 0}`);
        if (scope !== "telegram") parts.push(`Emby 关停 ${res.data.emby_disabled ?? 0}`);
        parts.push(`失败 ${res.data.failed} 个`);
        toast({
          title: "批量刷新完成",
          description: parts.join("，"),
          variant: res.data.failed ? "default" : "success",
        });
        await refreshAfterBatch();
      } else {
        toast({ title: "批量刷新失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "批量刷新失败", description: error.message, variant: "destructive" });
    } finally {
      setBatchUserLoading(false);
    }
  };

  const handleSelectedDelete = async () => {
    if (selectedCount === 0) return;
    const action = await confirmAction(batchDeleteConfirmConfig(
      selectedCount,
      t,
      selectionScope === "emby" ? selectedCount : selectionScope === "manual" ? selectedEmbyCount : undefined,
    ));
    if (!action) return;
    const deleteEmby = action === "with_emby";
    setBatchUserLoading(true);
    try {
      const res = await api.batchDeleteUsers(selectedBatchTarget(), deleteEmby);
      if (res.success && res.data) {
        toast({
          title: "批量删除完成",
          description: `成功 ${res.data.success} 个，失败 ${res.data.failed} 个`,
          variant: res.data.failed ? "default" : "success",
        });
        await refreshAfterBatch();
      } else {
        toast({ title: "批量删除失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "批量删除失败", description: error.message, variant: "destructive" });
    } finally {
      setBatchUserLoading(false);
    }
  };

  const handleBulkExpire = async () => {
    if (bulkExpireConfirmText.trim() !== "确认") {
      toast({ title: "需要在文本框输入「确认」二字以继续", variant: "destructive" });
      return;
    }
    let payload: Parameters<typeof api.adminBulkSetExpire>[0];
    if (bulkExpireMode === "permanent") {
      payload = { expired_at: -1 };
    } else {
      const days = parseInt(bulkExpireDays, 10);
      if (!Number.isFinite(days) || days <= 0) {
        toast({ title: "请输入正整数天数", variant: "destructive" });
        return;
      }
      payload = { days };
    }
    // 把当前筛选条件透传到后端，作用范围与列表展示保持一致
    const filter: NonNullable<Parameters<typeof api.adminBulkSetExpire>[0]["filter"]> = {};
    if (roleFilter !== "all") filter.role = Number(roleFilter);
    if (activeFilter !== "all") filter.active = activeFilter === "true";
    if (embyFilter === "bound") filter.emby = "bound";
    else if (embyFilter === "unbound") filter.emby = "unbound";
    if (emailStatusFilter !== "all") filter.email_status = emailStatusFilter as "verified" | "unverified" | "bound" | "none";
    if (Object.keys(filter).length > 0) payload.filter = filter;
    payload.include_admin = bulkExpireIncludeAdmin;
    payload.include_whitelist = bulkExpireIncludeWhitelist;
    // 未绑定 Emby 的账号永远跳过：后端会强制忽略这个字段

    setBulkExpireLoading(true);
    try {
      const res = await api.adminBulkSetExpire(payload);
      if (res.success && res.data) {
        const d = res.data;
        const targetText =
          bulkExpireMode === "permanent"
            ? "永久"
            : `${bulkExpireDays} 天后`;
        toast({
          title: `已更新 ${d.updated} 个用户到期时间为 ${targetText}`,
          description:
            `匹配 ${d.matched}；跳过：管理员 ${d.skipped_admins}` +
            `，白名单 ${d.skipped_whitelist}` +
            `，未开通 Emby ${d.skipped_pending_emby}`,
          variant: "success",
        });
        invalidateUsersCache();
        await loadUsers();
        setBulkExpireOpen(false);
        setBulkExpireConfirmText("");
      } else {
        toast({ title: "批量更新失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "批量更新失败", description: err.message || "网络异常", variant: "destructive" });
    } finally {
      setBulkExpireLoading(false);
    }
  };

  const handleBulkEnableDisabled = async () => {
    if (activeFilter === "true") {
      toast({ title: "当前筛选仅包含已启用账号", description: "请切换为全部状态或仅已禁用后再执行", variant: "destructive" });
      return;
    }
    if (bulkEnableConfirmText.trim() !== "确认") {
      toast({ title: "需要在文本框输入「确认」二字以继续", variant: "destructive" });
      return;
    }
    const filter: NonNullable<Parameters<typeof api.adminBulkEnableDisabledUsers>[0]["filter"]> = {};
    if (roleFilter !== "all") filter.role = Number(roleFilter);
    if (activeFilter !== "all") filter.active = activeFilter === "true";
    if (embyFilter === "bound") filter.emby = "bound";
    else if (embyFilter === "unbound") filter.emby = "unbound";
    if (emailStatusFilter !== "all") filter.email_status = emailStatusFilter as "verified" | "unverified" | "bound" | "none";
    if (search.trim()) filter.search = search.trim();

    setBulkEnableLoading(true);
    try {
      const res = await api.adminBulkEnableDisabledUsers({
        filter,
        include_admin: bulkEnableIncludeAdmin,
        include_whitelist: bulkEnableIncludeWhitelist,
      });
      if (res.success && res.data) {
        const d = res.data;
        toast({
          title: `已启用 ${d.enabled} 个禁用账号`,
          description: `匹配 ${d.matched}；跳过：管理员 ${d.skipped_admins}，白名单 ${d.skipped_whitelist}，未识别 ${d.skipped_unrecognized}`,
          variant: d.failed.length ? "default" : "success",
        });
        invalidateUsersCache();
        await loadUsers();
        setBulkEnableOpen(false);
        setBulkEnableConfirmText("");
      } else {
        toast({ title: "批量启用失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "批量启用失败", description: err.message || "网络异常", variant: "destructive" });
    } finally {
      setBulkEnableLoading(false);
    }
  };

  // ============== 一键踢出 TG 群里未绑账号的成员 ==============
  const openKickDialog = async () => {
    setKickOpen(true);
    setKickPreview(null);
    setKickResult(null);
    setKickConfirmText("");
    setKickRoster(null);
    setKickLoading(true);
    try {
      const [statsRes, dryRes] = await Promise.all([
        api.getTelegramRosterStats(),
        api.kickUnboundGroupMembers({ dryRun: true }),
      ]);
      if (statsRes.success && statsRes.data) setKickRoster(statsRes.data);
      if (dryRes.success && dryRes.data) setKickPreview(dryRes.data);
      else if (!dryRes.success) {
        toast({ title: "无法预览", description: dryRes.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "预览失败", description: err.message || "网络异常", variant: "destructive" });
    } finally {
      setKickLoading(false);
    }
  };

  const handleKickConfirm = async () => {
    if (kickConfirmText.trim() !== "确认") {
      toast({ title: "需要在文本框输入「确认」二字以继续", variant: "destructive" });
      return;
    }
    setKickLoading(true);
    try {
      const res = await api.kickUnboundGroupMembers({ dryRun: false, maxPerRun: 200 });
      if (res.success && res.data) {
        setKickResult(res.data);
        toast({
          title: `已踢出 ${res.data.kicked} 人`,
          description: `失败 ${res.data.failed}，已不在群 ${res.data.not_in_group}`,
          variant: "success",
        });
      } else {
        toast({ title: "踢出失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "踢出失败", description: err.message || "网络异常", variant: "destructive" });
    } finally {
      setKickLoading(false);
    }
  };

  const handleCleanupPreview = async () => {
    setCleanupLoading(true);
    setCleanupPreview(null);
    setCleanupConfirmText("");
    try {
      const res = await api.cleanupInvalidUsers(parseInt(cleanupMinDays) || 7, true);
      if (res.success && res.data) {
        setCleanupPreview(res.data.users || []);
      } else {
        toast({ title: "预览失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "预览失败", description: error.message, variant: "destructive" });
    } finally {
      setCleanupLoading(false);
    }
  };

  const handleCleanupConfirm = async () => {
    if (cleanupConfirmText.trim() !== "确认") {
      toast({ title: "需要在文本框输入「确认」二字以继续", variant: "destructive" });
      return;
    }
    setCleanupLoading(true);
    try {
      const res = await api.cleanupInvalidUsers(parseInt(cleanupMinDays) || 7, false);
      if (res.success && res.data) {
        toast({
          title: "清理完成",
          description: `已删除 ${res.data.count} 个无效用户`,
          variant: "success",
        });
        setCleanupOpen(false);
        setCleanupPreview(null);
        setCleanupConfirmText("");
        invalidateUsersCache();
        loadUsers();
      } else {
        toast({ title: "清理失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "清理失败", description: error.message, variant: "destructive" });
    } finally {
      setCleanupLoading(false);
    }
  };

  const handleClearStalePendingEmby = async () => {
    setStalePendingLoading(true);
    try {
      const preview = await api.clearStalePendingEmbyUsers({ dryRun: true });
      if (!preview.success || !preview.data) {
        toast({ title: "预览失败", description: preview.message, variant: "destructive" });
        return;
      }
      if (preview.data.count === 0) {
        toast({ title: "无需清理", description: "没有旧自由注册残留的 Emby 开通资格", variant: "success" });
        return;
      }

      const ok = await confirmAction({
        title: "清理旧 Emby 开通资格",
        description: `将清理 ${preview.data.count} 个旧自由注册残留资格。使用注册码产生的待补建资格不会受影响。`,
        tone: "warning",
        confirmLabel: "确认清理",
      });
      if (!ok) return;

      const res = await api.clearStalePendingEmbyUsers({ dryRun: false });
      if (res.success && res.data) {
        toast({
          title: `已清理 ${res.data.cleared} 个旧资格`,
          description: res.data.failed.length ? `失败 ${res.data.failed.length} 个` : "个人设置里的错误开通提示会消失",
          variant: res.data.failed.length ? "default" : "success",
        });
        invalidateUsersCache();
        await loadUsers();
      } else {
        toast({ title: "清理失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "清理失败", description: error.message || "网络异常", variant: "destructive" });
    } finally {
      setStalePendingLoading(false);
    }
  };

  const handleClearAllEmails = async () => {
    setClearEmailLoading(true);
    try {
      const preview = await api.clearAllUserEmails({ dryRun: true });
      if (!preview.success || !preview.data) {
        toast({ title: t("adminUsers.previewFailed"), description: preview.message, variant: "destructive" });
        return;
      }
      if (preview.data.count === 0) {
        toast({ title: t("adminUsers.noEmailsTitle"), description: t("adminUsers.noEmailsDescription"), variant: "success" });
        return;
      }
      const ok = await confirmAction({
        title: t("adminUsers.clearEmailsConfirmTitle"),
        description: t("adminUsers.clearEmailsConfirmDescription", { count: preview.data.count }),
        tone: "warning",
        confirmLabel: t("adminUsers.clearEmailsConfirmLabel"),
      });
      if (!ok) return;
      const res = await api.clearAllUserEmails({ dryRun: false });
      if (res.success && res.data) {
        toast({
          title: t("adminUsers.clearEmailsSuccessTitle", { count: res.data.cleared }),
          description: t("adminUsers.clearEmailsSuccessDescription", { totalUsers: res.data.total_users }),
          variant: "success",
        });
        invalidateUsersCache();
        await loadUsers();
      } else {
        toast({ title: t("adminUsers.cleanupFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminUsers.cleanupFailed"), description: error.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setClearEmailLoading(false);
    }
  };

  const handleClearRegistrationQueueUsers = async () => {
    setRegistrationQueueLoading(true);
    try {
      const preview = await api.previewRegistrationQueueUsers();
      if (!preview.success || !preview.data) {
        toast({ title: "预览失败", description: preview.message, variant: "destructive" });
        return;
      }
      if (preview.data.count === 0) {
        toast({ title: "无需清理", description: "当前没有注册队列用户", variant: "success" });
        return;
      }
      const ok = await confirmAction({
        title: "清理注册队列用户",
        description: `将从注册队列中移出 ${preview.data.count} 个尚未完成的用户。已开始 processing 的请求不会被强制移出。`,
        tone: "warning",
        confirmLabel: "确认清理",
      });
      if (!ok) return;
      const res = await api.clearRegistrationQueueUsers();
      if (res.success && res.data) {
        toast({
          title: `已清理 ${res.data.cleared} 个队列项`,
          description: res.data.blocked ? `${res.data.blocked} 个已开始处理，未移出` : undefined,
          variant: res.data.blocked ? "default" : "success",
        });
        invalidateUsersCache();
        await loadUsers();
      } else {
        toast({ title: "清理失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "清理失败", description: error.message || "网络异常", variant: "destructive" });
    } finally {
      setRegistrationQueueLoading(false);
    }
  };

  const handleGrantRegistrationQueueUsersEntitlementAndClear = async () => {
    const action = await confirmAction({
      title: "给予注册队列用户注册资格并清理",
      description: "会先预览当前注册队列用户，之后给未绑定 Emby 且启用的用户授予补建资格，并只移出尚未开始处理的队列项。",
      tone: "warning",
      actions: [
        { label: "默认时长", value: "default", variant: "default" },
        { label: "永久", value: "permanent", variant: "outline" },
      ],
    });
    if (!action) return;
    const days = action === "permanent" ? -1 : undefined;
    setRegistrationQueueLoading(true);
    try {
      const preview = await api.previewGrantRegistrationQueueUsersEntitlement(days);
      if (!preview.success || !preview.data) {
        toast({ title: "预览失败", description: preview.message, variant: "destructive" });
        return;
      }
      if (preview.data.eligible === 0) {
        toast({ title: "没有可授权用户", description: `队列匹配 ${preview.data.matched}，可授权 0`, variant: "default" });
        return;
      }
      const ok = await confirmAction({
        title: `确认授权 ${preview.data.eligible} 个队列用户？`,
        description: `队列匹配 ${preview.data.matched}，将授予 Emby 补建资格（${preview.data.days === -1 ? "永久" : `${preview.data.days} 天`}）并清理未处理队列。`,
        tone: "warning",
        confirmLabel: "确认授权并清理",
      });
      if (!ok) return;
      const res = await api.grantRegistrationQueueUsersEntitlementAndClear(days);
      if (res.success && res.data) {
        toast({
          title: `已授权 ${res.data.granted} 个用户`,
          description: `移出队列 ${res.data.dequeued}，已开始处理 ${res.data.blocked}，失败 ${res.data.failed.length}`,
          variant: res.data.failed.length ? "default" : "success",
        });
        invalidateUsersCache();
        await loadUsers();
      } else {
        toast({ title: "操作失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "操作失败", description: error.message || "网络异常", variant: "destructive" });
    } finally {
      setRegistrationQueueLoading(false);
    }
  };

  // ============== 一键踢出未绑定 Emby 的系统账号 ==============
  const noEmbyMinDaysParsed = (() => {
    const n = Number(noEmbyMinDays);
    return Number.isFinite(n) && n >= 0 ? Math.floor(n) : 0;
  })();

  const handleNoEmbyPreview = async () => {
    setNoEmbyLoading(true);
    try {
      const res = await api.kickNoEmbyUsers({
        dryRun: true,
        minDays: noEmbyMinDaysParsed,
        preservePendingRegister: noEmbyPreserveDirect,
      });
      if (res.success && res.data) {
        setNoEmbyPreview(res.data);
      } else {
        toast({ title: "预览失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "预览失败", description: err.message || "网络异常", variant: "destructive" });
    } finally {
      setNoEmbyLoading(false);
    }
  };

  const handleNoEmbyConfirm = async () => {
    if (noEmbyConfirmText.trim() !== "确认") {
      toast({ title: "请在输入框中输入「确认」以执行", variant: "destructive" });
      return;
    }
    setNoEmbyLoading(true);
    try {
      const res = await api.kickNoEmbyUsers({
        dryRun: false,
        minDays: noEmbyMinDaysParsed,
        preservePendingRegister: noEmbyPreserveDirect,
      });
      if (res.success && res.data) {
        toast({
          title: "清理完成",
          description: `已删除 ${res.data.deleted_count} 个未绑 Emby 账号` + (
            res.data.failed.length ? `，失败 ${res.data.failed.length} 个` : ""
          ),
          variant: "success",
        });
        setNoEmbyOpen(false);
        setNoEmbyPreview(null);
        setNoEmbyConfirmText("");
        invalidateUsersCache();
        loadUsers();
      } else {
        toast({ title: "清理失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "清理失败", description: err.message || "网络异常", variant: "destructive" });
    } finally {
      setNoEmbyLoading(false);
    }
  };

  if (isLoading && !users.length) {
    return <PageLoading />;
  }

  if (error) {
    return <PageError message={error} onRetry={() => void loadUsers()} />;
  }

  const getRoleBadge = (role: number) => renderRoleBadge(role, t);

  const renderUserActions = (user: UserInfo) => (
    <UserActionsMenu
      user={user}
      t={t}
      handlers={{
        onEdit: handleOpenEdit,
        onRenew: (u) => {
          setSelectedUser(u);
          setRenewMode("renew");
          setRenewPermanent(false);
          setRenewDays("30");
          setRenewOpen(true);
        },
        onCancelPermanent: (u) => {
          setSelectedUser(u);
          setRenewMode("cancelPermanent");
          setRenewPermanent(false);
          setRenewDays("30");
          setRenewOpen(true);
        },
        onSetExpiry: (u) => {
          setSelectedUser(u);
          setRenewMode("setExact");
          setRenewPermanent(false);
          setRenewDays("30");
          setRenewOpen(true);
        },
        onResetPassword: handleResetPassword,
        onBindEmby: handleOpenBindEmby,
        onEmbyDisable: (u) => handleEmbyToggle(u, false),
        onEmbyEnable: (u) => handleEmbyToggle(u, true),
        onBindEmail: (u) => {
          setEmailDialogUser(u);
          setEmailDialogOpen(true);
        },
        onBindTelegram: handleOpenBindTelegram,
        onSyncBindings: (u) => handleSyncBindings({ uid: u.uid }),
        onRefreshStatus: handleRefreshStatus,
        onForceUnbind: handleForceUnbind,
        onClearRegistrationQueue: handleClearRegistrationQueue,
        onGrantRegistrationEntitlement: handleGrantRegistrationEntitlement,
        onGrantRegistrationEntitlementAndDequeue: handleGrantRegistrationEntitlementAndDequeue,
        onToggleActive: handleToggleActive,
        onDelete: handleDelete,
      }}
    />
  );

  const bulkEnableBlocked = activeFilter === "true";

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl sm:text-3xl font-bold">用户管理</h1>
          <p className="text-sm text-muted-foreground">管理所有注册用户</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline" size="sm"
            onClick={() => { setWebUserName(""); setWebUserPwd(""); setWebUserEmail(""); setWebUserRole(1); setWebUserOpen(true); }}
            title="创建新的 Web 面板账号"
          >
            <UserPlus className="mr-1.5 h-4 w-4" />新建用户
          </Button>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="sm" title="管理 Emby 账号与同步">
                <MoreHorizontal className="mr-1.5 h-4 w-4" />管理
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              <DropdownMenuSub>
                <DropdownMenuSubTrigger>
                  <Key className="mr-2 h-4 w-4" />
                  Emby 操作
                </DropdownMenuSubTrigger>
                <DropdownMenuSubContent className="w-56">
                  <DropdownMenuItem
                    onClick={() => {
                      setForcePwdEmbyName("");
                      setForcePwdNewPwd("");
                      setForcePwdAuto(true);
                      setForcePwdResult(null);
                      setForcePwdOpen(true);
                    }}
                  >
                    <Key className="mr-2 h-4 w-4" />
                    按 Emby 用户名改密
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={() => {
                      setStandaloneName("");
                      setStandalonePwd("");
                      setStandaloneEmail("");
                      setStandaloneOpen(true);
                    }}
                  >
                    <UserPlus className="mr-2 h-4 w-4" />
                    新建独立 Emby 账号
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => void handleSyncBindings({ currentFilter: true })}>
                    <RefreshCw className="mr-2 h-4 w-4" />
                    同步当前筛选
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => void handleSyncBindings()}>
                    <RefreshCw className="mr-2 h-4 w-4" />
                    同步全部绑定
                  </DropdownMenuItem>
                </DropdownMenuSubContent>
              </DropdownMenuSub>
              <DropdownMenuSub>
                <DropdownMenuSubTrigger>
                  <CalendarClock className="mr-2 h-4 w-4" />
                  批量处理
                </DropdownMenuSubTrigger>
                <DropdownMenuSubContent className="w-56">
                  <DropdownMenuItem
                    onClick={() => {
                      setBulkExpireMode("permanent");
                      setBulkExpireDays("30");
                      setBulkExpireIncludeAdmin(false);
                      setBulkExpireIncludeWhitelist(false);
                      setBulkExpireConfirmText("");
                      setBulkExpireOpen(true);
                    }}
                  >
                    <CalendarClock className="mr-2 h-4 w-4" />
                    批量到期调控
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    disabled={bulkEnableBlocked}
                    onClick={() => {
                      setBulkEnableIncludeAdmin(false);
                      setBulkEnableIncludeWhitelist(false);
                      setBulkEnableConfirmText("");
                      setBulkEnableOpen(true);
                    }}
                  >
                    <UserCheck className="mr-2 h-4 w-4" />
                    批量启用禁用账号
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    disabled={clearEmailLoading}
                    onClick={() => void handleClearAllEmails()}
                  >
                    {clearEmailLoading ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <Trash2 className="mr-2 h-4 w-4" />
                    )}
                    {t("adminUsers.clearEmailsAction")}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    disabled={registrationQueueLoading}
                    onClick={() => void handleClearRegistrationQueueUsers()}
                  >
                    {registrationQueueLoading ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <CalendarClock className="mr-2 h-4 w-4" />
                    )}
                    清理注册队列
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    disabled={registrationQueueLoading}
                    onClick={() => void handleGrantRegistrationQueueUsersEntitlementAndClear()}
                  >
                    {registrationQueueLoading ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <UserCheck className="mr-2 h-4 w-4" />
                    )}
                    队列用户授权并清理
                  </DropdownMenuItem>
                </DropdownMenuSubContent>
              </DropdownMenuSub>
              <DropdownMenuSeparator />
              <DropdownMenuSub>
                <DropdownMenuSubTrigger className="text-destructive focus:text-destructive">
                  <UserX className="mr-2 h-4 w-4" />
                  清理 / 踢出
                </DropdownMenuSubTrigger>
                <DropdownMenuSubContent className="w-60">
                  <DropdownMenuItem
                    disabled={stalePendingLoading}
                    onClick={() => void handleClearStalePendingEmby()}
                  >
                    {stalePendingLoading ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <UserX className="mr-2 h-4 w-4" />
                    )}
                    清理旧 Emby 开通资格
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    className="text-destructive focus:text-destructive"
                    onClick={() => {
                      setCleanupPreview(null);
                      setCleanupMinDays("7");
                      setCleanupOpen(true);
                    }}
                  >
                    <UserX className="mr-2 h-4 w-4" />
                    清理无效用户
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    className="text-destructive focus:text-destructive"
                    onClick={() => {
                      setNoEmbyPreview(null);
                      setNoEmbyConfirmText("");
                      setNoEmbyOpen(true);
                    }}
                  >
                    <UserX className="mr-2 h-4 w-4" />
                    踢出未绑 Emby 账号
                  </DropdownMenuItem>
                  <DropdownMenuItem className="text-destructive focus:text-destructive" onClick={openKickDialog}>
                    <Send className="mr-2 h-4 w-4" />
                    踢出未绑 TG 成员
                  </DropdownMenuItem>
                </DropdownMenuSubContent>
              </DropdownMenuSub>
            </DropdownMenuContent>
          </DropdownMenu>
          <Badge variant="outline" className="text-lg px-4 py-2">
            共 {total} 用户
          </Badge>
        </div>
      </div>

      {/* Search */}
      <Card>
        <CardContent className="pt-6 space-y-3">
          <div className="flex flex-col gap-3 md:flex-row md:gap-4">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="搜索用户名、邮箱、UID 或 Telegram ID..."
                value={search}
                onChange={(e) => {
                  setSearch(e.target.value);
                  clearUserSelection();
                }}
                onKeyDown={(e) => e.key === "Enter" && handleSearch()}
                className="pl-10"
              />
            </div>
            <div className="flex items-center gap-2 md:w-auto">
              <Select value={perPage.toString()} onValueChange={(value) => { setPerPage(Number(value)); setPage(1); invalidateUsersCache(); }}>
                <SelectTrigger className="w-28 md:w-24">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="20">20 / 页</SelectItem>
                  <SelectItem value="50">50 / 页</SelectItem>
                  <SelectItem value="100">100 / 页</SelectItem>
                </SelectContent>
              </Select>
              <Button onClick={handleSearch} disabled={isLoading} className="flex-1 md:flex-none">
                {isLoading ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Search className="mr-2 h-4 w-4" />
                )}
                {isLoading ? "加载中" : "搜索"}
              </Button>
            </div>
          </div>

          {/* 筛选 / 排序 */}
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-5">
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">角色</p>
              <Select value={roleFilter} onValueChange={setRoleFilter}>
                <SelectTrigger>
                  <SelectValue placeholder="全部角色" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">全部角色</SelectItem>
                  <SelectItem value="0">管理员</SelectItem>
                  <SelectItem value="2">白名单</SelectItem>
                  <SelectItem value="1">普通用户</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">状态</p>
              <Select value={activeFilter} onValueChange={setActiveFilter}>
                <SelectTrigger>
                  <SelectValue placeholder="全部状态" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">全部状态</SelectItem>
                  <SelectItem value="true">仅已启用</SelectItem>
                  <SelectItem value="false">仅已禁用</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">Emby 绑定</p>
              <Select value={embyFilter} onValueChange={setEmbyFilter}>
                <SelectTrigger>
                  <SelectValue placeholder="全部" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">全部</SelectItem>
                  <SelectItem value="bound">已绑定 Emby</SelectItem>
                  <SelectItem value="unbound">未绑定 Emby</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">Emby 状态</p>
              <Select value={embyStatusFilter} onValueChange={setEmbyStatusFilter}>
                <SelectTrigger>
                  <SelectValue placeholder="全部" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">全部</SelectItem>
                  <SelectItem value="active">Emby 正常</SelectItem>
                  <SelectItem value="disabled">Emby 已禁用</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {emailEnabled && (
              <div className="space-y-1">
                <p className="text-xs text-muted-foreground">{t("email.admin.statusColumn")}</p>
                <Select value={emailStatusFilter} onValueChange={setEmailStatusFilter}>
                  <SelectTrigger>
                    <SelectValue placeholder={t("email.admin.filterAll")} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t("email.admin.filterAll")}</SelectItem>
                    <SelectItem value="verified">{t("email.admin.filterVerified")}</SelectItem>
                    <SelectItem value="unverified">{t("email.admin.filterUnverified")}</SelectItem>
                    <SelectItem value="bound">{t("email.admin.filterBound")}</SelectItem>
                    <SelectItem value="none">{t("email.admin.filterNone")}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            )}
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">排序</p>
              <Select value={sortBy} onValueChange={setSortBy}>
                <SelectTrigger>
                  <SelectValue placeholder="排序" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="uid_asc">UID 从旧到新（默认）</SelectItem>
                  <SelectItem value="uid_desc">UID 从新到旧</SelectItem>
                  <SelectItem value="username_asc">用户名 A→Z</SelectItem>
                  <SelectItem value="username_desc">用户名 Z→A</SelectItem>
                  <SelectItem value="register_time_desc">注册时间 新→旧</SelectItem>
                  <SelectItem value="register_time_asc">注册时间 旧→新</SelectItem>
                  <SelectItem value="expired_at_asc">到期时间 近→远</SelectItem>
                  <SelectItem value="expired_at_desc">到期时间 远→近</SelectItem>
                  <SelectItem value="role_asc">角色 管理→普通</SelectItem>
                  <SelectItem value="active_desc">状态 启用优先</SelectItem>
                  <SelectItem value="active_asc">状态 禁用优先</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          {(roleFilter !== "all" || activeFilter !== "all" || embyFilter !== "all" || embyStatusFilter !== "all" || emailStatusFilter !== "all" || sortBy !== "uid_asc") && (
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>已应用筛选 / 排序</span>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  setRoleFilter("all");
                  setActiveFilter("all");
                  setEmbyFilter("all");
                  setEmailStatusFilter("all");
                  setSortBy("uid_asc");
                }}
              >
                重置
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Users Table */}
      {users.length > 0 && (
        <Card>
          <CardContent className="flex flex-col gap-3 p-3 sm:flex-row sm:items-center sm:flex-wrap">
            <div className="flex items-center gap-2 min-w-0">
              <p className="text-sm font-medium whitespace-nowrap">批量选择</p>
              {selectedCount > 0 && (
                <Badge variant="outline" className="text-[10px]">已选 {selectedCount} 个{selectionScope !== "manual" ? ` · ${selectionScope === "all" ? "全筛选" : "Emby"}` : ""}</Badge>
              )}
            </div>
            <div className="flex flex-wrap items-center gap-1.5">
              <Button variant="ghost" size="sm" onClick={selectCurrentPageUsers} disabled={users.length === 0}>选中当前页</Button>
              <Button variant="ghost" size="sm" onClick={() => void selectEmbyMatchingUsers()} disabled={selectEmbyLoading || total === 0}>
                {selectEmbyLoading ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : <Link2 className="mr-1 h-3.5 w-3.5" />}拥有 Emby 的
              </Button>
              <Button variant="ghost" size="sm" onClick={selectAllMatchingUsers} disabled={total === 0}>全部 ({total})</Button>
              <Button variant="ghost" size="sm" onClick={invertPageSelection} disabled={users.length === 0}>
                <FlipHorizontal2 className="mr-1 h-3.5 w-3.5" />反选本页
              </Button>
              <Button variant="ghost" size="sm" onClick={invertAllSelection} disabled={total === 0 || selectionScope === "emby"}>反选全部</Button>
              <Button variant="ghost" size="sm" onClick={clearUserSelection} disabled={selectedCount === 0} className="text-muted-foreground">清除</Button>
            </div>

            <div className="flex-1 hidden sm:block" />

            <div className="flex flex-wrap items-center gap-1.5">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="outline" size="sm" disabled={selectedCount === 0} title="对已选用户执行批量操作">
                    <MoreHorizontal className="mr-1.5 h-3.5 w-3.5" />
                    批量操作
                    <ChevronDown className="ml-1 h-3.5 w-3.5" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-60">
                  <DropdownMenuSub>
                    <DropdownMenuSubTrigger>
                      <UserCheck className="mr-2 h-4 w-4" />
                      Web 账号
                    </DropdownMenuSubTrigger>
                    <DropdownMenuSubContent className="w-56">
                      <DropdownMenuItem onClick={() => void handleSelectedToggleActive(true)}>
                        <UserCheck className="mr-2 h-4 w-4" />
                        启用 Web 账号
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => void handleSelectedToggleActive(false)}>
                        <Ban className="mr-2 h-4 w-4" />
                        禁用 Web 账号
                      </DropdownMenuItem>
                    </DropdownMenuSubContent>
                  </DropdownMenuSub>
                  <DropdownMenuSub>
                    <DropdownMenuSubTrigger>
                      <MonitorCheck className="mr-2 h-4 w-4" />
                      Emby 账号
                    </DropdownMenuSubTrigger>
                    <DropdownMenuSubContent className="w-60">
                      <DropdownMenuItem onClick={() => void handleSelectedEmbyToggle(true)}>
                        <MonitorCheck className="mr-2 h-4 w-4" />
                        启用 Emby
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => void handleSelectedEmbyToggle(false)}>
                        <MonitorX className="mr-2 h-4 w-4" />
                        禁用 Emby
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem onClick={() => void handleSelectedGrantAllLibraries()}>
                        <BookOpen className="mr-2 h-4 w-4" />
                        修复媒体库权限
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => void handleSelectedLockEmbyUnbind()}>
                        <LockKeyhole className="mr-2 h-4 w-4" />
                        禁止用户自助解绑
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => void handleSelectedClearEmbyGrant()}>
                        <Eraser className="mr-2 h-4 w-4" />
                        清理 Emby 授权标记
                      </DropdownMenuItem>
                    </DropdownMenuSubContent>
                  </DropdownMenuSub>
                  <DropdownMenuSub>
                    <DropdownMenuSubTrigger>
                      <RefreshCcw className="mr-2 h-4 w-4" />
                      刷新状态
                    </DropdownMenuSubTrigger>
                    <DropdownMenuSubContent className="w-56">
                      <DropdownMenuItem onClick={() => void handleSelectedRefreshStatus("telegram")}>
                        <RefreshCcw className="mr-2 h-4 w-4" />
                        刷新 Telegram
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => void handleSelectedRefreshStatus("emby")}>
                        <RefreshCcw className="mr-2 h-4 w-4" />
                        刷新 Emby
                      </DropdownMenuItem>
                    </DropdownMenuSubContent>
                  </DropdownMenuSub>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem className="text-destructive focus:text-destructive" onClick={() => void handleSelectedDelete()}>
                    <Trash2 className="mr-2 h-4 w-4" />
                    删除已选用户 ({selectedCount})
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="flex h-64 items-center justify-center">
              <Loader2 className="h-8 w-8 animate-spin text-primary" />
            </div>
          ) : (
            <>
            <div className="space-y-3 p-3 md:hidden">
              {users.map((user) => (
                <div key={user.uid} className="rounded-xl border bg-background p-4 shadow-sm">
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex min-w-0 items-start gap-3">
                      <input
                        type="checkbox"
                        checked={isUserSelected(user)}
                        onChange={() => toggleSelectedUser(user.uid)}
                        className="mt-1 h-4 w-4"
                        aria-label={`选择 ${user.username}`}
                      />
                      <Avatar className="h-10 w-10 shrink-0 ring-1 ring-border/50">
                              <AvatarImage src={normalizeAssetUrl(user.avatar)} />
                        <AvatarFallback className="bg-primary/10 text-primary text-sm font-bold">
                          {user.username.charAt(0).toUpperCase()}
                        </AvatarFallback>
                      </Avatar>
                      <div className="min-w-0">
                        <p className="truncate text-base font-medium">{user.username}</p>
                        <p className="mt-1 text-xs text-muted-foreground">UID: {user.uid}</p>
                        <div className="mt-0.5 flex items-center gap-1.5">
                          <span className="truncate text-xs text-muted-foreground" title={user.email || "未设置"}>
                            邮箱: {user.email || "未设置"}
                          </span>
                          {emailEnabled && user.email && (
                            <Badge variant="outline" className={`shrink-0 px-1.5 py-0 text-[10px] ${user.email_verified ? "border-emerald-500/40 text-emerald-600" : "border-amber-500/40 text-amber-600"}`}>
                              {user.email_verified ? t("email.verified") : t("email.unverified")}
                            </Badge>
                          )}
                        </div>
                      </div>
                    </div>
                    {renderUserActions(user)}
                  </div>

                  <div className="mt-3 grid grid-cols-2 gap-3 text-sm">
                    <div>
                      <p className="text-xs text-muted-foreground">角色</p>
                      <div className="mt-1">{getRoleBadge(user.role)}</div>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">Web 账号状态</p>
                      <div className="mt-1">{renderWebStatusBadge(user)}</div>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">Emby 账号状态</p>
                      <div className="mt-1 flex min-w-0 flex-col gap-0.5">{renderEmbyStatusCell(user)}</div>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">到期时间</p>
                      <p className="mt-1">{renderExpireCell(user, t)}</p>
                    </div>
                  </div>

                  {(user.telegram_id || user.emby_id) && (
                    <div className="mt-3 space-y-1 border-t pt-3 text-xs text-muted-foreground">
                      {user.telegram_id && (
                        <p>
                          TG: {user.telegram_username ? `@${user.telegram_username} (${user.telegram_id})` : user.telegram_id}
                        </p>
                      )}
                      {user.emby_id && <p className="break-all">Emby ID: {user.emby_id}</p>}
                    </div>
                  )}
                </div>
              ))}
            </div>

            <div className="hidden overflow-x-auto md:block">
              <table className="w-full">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="w-10 px-4 py-3 text-left text-sm font-medium">
                      <input
                        type="checkbox"
                        checked={allPageSelected}
                        onChange={toggleSelectCurrentPage}
                        className="h-4 w-4"
                        aria-label="选择当前页用户"
                      />
                    </th>
                    <th className="px-4 py-3 text-left text-sm font-medium">用户</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">角色</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">Web 账号状态</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">Emby 账号状态</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">到期时间</th>
                    <th className="px-4 py-3 text-right text-sm font-medium">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((user) => (
                    <Fragment key={user.uid}>
                      <tr className="border-b hover:bg-muted/30">
                      <td className="px-4 py-3">
                        <input
                          type="checkbox"
                          checked={isUserSelected(user)}
                          onChange={() => toggleSelectedUser(user.uid)}
                          className="h-4 w-4"
                          aria-label={`选择 ${user.username}`}
                        />
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-start justify-between gap-4">
                          <div className="flex items-center gap-3 min-w-0">
                            <Avatar className="h-8 w-8 shrink-0 ring-1 ring-border/50">
                        <AvatarImage src={normalizeAssetUrl(user.avatar)} />
                              <AvatarFallback className="bg-primary/10 text-primary text-xs font-bold">
                                {user.username.charAt(0).toUpperCase()}
                              </AvatarFallback>
                            </Avatar>
                            <div className="min-w-0">
                            <p className="font-medium">{user.username}</p>
                            <p className="text-xs text-muted-foreground truncate">
                              UID: {user.uid}
                              {user.telegram_id && (
                                <span>
                                  {" | TG: "}
                                  {user.telegram_username ? (
                                    <span>
                                      @{user.telegram_username} ({user.telegram_id})
                                    </span>
                                  ) : (
                                    user.telegram_id
                                  )}
                                </span>
                              )}
                            </p>
                            <div className="flex items-center gap-1.5">
                              <span className="max-w-[260px] truncate text-xs text-muted-foreground" title={user.email || "未设置"}>
                                邮箱: {user.email || "未设置"}
                              </span>
                              {emailEnabled && user.email && (
                                <Badge variant="outline" className={`shrink-0 px-1.5 py-0 text-[10px] ${user.email_verified ? "border-emerald-500/40 text-emerald-600" : "border-amber-500/40 text-amber-600"}`}>
                                  {user.email_verified ? t("email.verified") : t("email.unverified")}
                                </Badge>
                              )}
                            </div>
                          </div>
                          </div>
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => toggleUserDetails(user.uid)}
                            className="mt-1"
                          >
                            <ChevronDown className={`h-4 w-4 transition-transform ${expandedUserIds.has(user.uid) ? "rotate-180" : ""}`} />
                          </Button>
                        </div>
                      </td>
                      <td className="px-4 py-3">{getRoleBadge(user.role)}</td>
                      <td className="px-4 py-3">{renderWebStatusBadge(user)}</td>
                      <td className="px-4 py-3">{renderEmbyStatusCell(user)}</td>
                      <td className="px-4 py-3 text-sm">{renderExpireCell(user, t)}</td>
                      <td className="px-4 py-3 text-right">
                        {renderUserActions(user)}
                      </td>
                    </tr>
                    {expandedUserIds.has(user.uid) && (
                      <tr className="bg-muted/10">
                        <td colSpan={7} className="px-4 py-3 text-sm text-muted-foreground">
                          <div className="grid gap-3 sm:grid-cols-2">
                            <div>
                              <p className="font-medium">更多信息</p>
                              <p>邮箱: {user.email || "未设置"}</p>
                              <p>注册时间: {user.register_time ? formatDate(user.register_time) : "未知"}</p>
                              <p>创建时间: {user.created_at ? formatDate(user.created_at) : "未记录"}</p>
                            </div>
                            <div>
                              <p className="font-medium">账号详情</p>
                              <p>Emby ID: {user.emby_id || "未绑定"}</p>
                              <p>BGM 同步: {user.bgm_mode ? "已开启" : "未开启"}</p>
                              <p>BGM Token: {user.bgm_token_set ? "已配置" : "未配置"}</p>
                              <p>BGM 状态: {user.bgm_sync_ready ? "可同步" : user.bgm_mode ? "缺少个人 Token" : "未启用"}</p>
                            </div>
                          </div>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                ))}
                </tbody>
              </table>
            </div>
            </>
          )}
        </CardContent>
      </Card>

      {/* Pagination */}
      <div className="flex flex-wrap items-center justify-center gap-3">
        <div className="flex items-center gap-1">
          <span className="text-xs text-muted-foreground">{t("adminUsers.perPage")}</span>
          <Select value={String(perPage)} onValueChange={(value) => handlePerPageChange(Number(value))}>
            <SelectTrigger className="h-8 w-24 text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {USERS_PER_PAGE_OPTIONS.map((option) => (
                <SelectItem key={option} value={String(option)}>
                  {option} / {t("adminUsers.perPage")}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {pages > 1 && (
          <div className="flex items-center gap-2">
            <Button variant="outline" size="icon" onClick={() => setPage((p) => Math.max(1, p - 1))} disabled={page === 1}>
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <span className="text-sm">第 {page} 页，共 {pages} 页</span>
            <Button variant="outline" size="icon" onClick={() => setPage((p) => Math.min(pages, p + 1))} disabled={page === pages}>
              <ChevronRight className="h-4 w-4" />
            </Button>
            <div className="flex items-center gap-1 ml-2">
              <span className="text-xs text-muted-foreground">跳转：</span>
              <Input type="number" min={1} max={pages} className="h-8 w-16 text-xs"
                onKeyDown={(e) => { if (e.key === "Enter") { const v = parseInt((e.target as HTMLInputElement).value, 10); if (v >= 1 && v <= pages) setPage(v); }}} />
            </div>
          </div>
        )}
      </div>

      {/* Edit Dialog */}
      <EditUserDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        user={selectedUser}
        form={editForm}
        onFormChange={setEditForm}
        onSubmit={handleEdit}
        isLoading={isActionLoading}
      />

      {/* Cleanup Invalid Users Dialog */}
      <CleanupInvalidUsersDialog
        open={cleanupOpen}
        onOpenChange={(open) => {
          setCleanupOpen(open);
          if (!open) {
            setCleanupPreview(null);
            setCleanupConfirmText("");
          }
        }}
        minDays={cleanupMinDays}
        onMinDaysChange={(next) => {
          setCleanupMinDays(next);
          setCleanupPreview(null);
          setCleanupConfirmText("");
        }}
        preview={cleanupPreview}
        confirmText={cleanupConfirmText}
        onConfirmTextChange={setCleanupConfirmText}
        onPreview={handleCleanupPreview}
        onConfirm={handleCleanupConfirm}
        isLoading={cleanupLoading}
      />

      {/* 一键踢出未绑定 Emby 的系统账号 */}
      <NoEmbyKickDialog
        open={noEmbyOpen}
        onOpenChange={(open) => {
          setNoEmbyOpen(open);
          if (!open) {
            setNoEmbyPreview(null);
            setNoEmbyConfirmText("");
          }
        }}
        minDays={noEmbyMinDays}
        onMinDaysChange={(next) => {
          setNoEmbyMinDays(next);
          setNoEmbyPreview(null);
        }}
        minDaysParsed={noEmbyMinDaysParsed}
        preserveDirect={noEmbyPreserveDirect}
        onPreserveDirectChange={(next) => {
          setNoEmbyPreserveDirect(next);
          setNoEmbyPreview(null);
        }}
        preview={noEmbyPreview}
        confirmText={noEmbyConfirmText}
        onConfirmTextChange={setNoEmbyConfirmText}
        onPreview={handleNoEmbyPreview}
        onConfirm={handleNoEmbyConfirm}
        isLoading={noEmbyLoading}
      />

      {/* Reset User Password Dialog（区分系统 / Emby / 两者） */}
      <ResetPasswordDialog
        open={resetPwdOpen}
        onOpenChange={(open) => {
          setResetPwdOpen(open);
          if (!open) {
            setResetPwdUser(null);
            setResetPwdResult(null);
            setResetPwdCustom("");
          }
        }}
        user={resetPwdUser}
        scope={resetPwdScope}
        onScopeChange={setResetPwdScope}
        auto={resetPwdAuto}
        onAutoChange={setResetPwdAuto}
        custom={resetPwdCustom}
        onCustomChange={setResetPwdCustom}
        result={resetPwdResult}
        onSubmit={handleResetPasswordSubmit}
        onCopyPassword={(pwd) => {
          void navigator.clipboard.writeText(pwd);
          toast({ title: "已复制到剪贴板", variant: "success" });
        }}
        isLoading={resetPwdLoading}
      />

      {/* Force-set Emby Password Dialog */}
      <ForceEmbyPasswordDialog
        open={forcePwdOpen}
        onOpenChange={(open) => {
          setForcePwdOpen(open);
          if (!open) {
            setForcePwdEmbyName("");
            setForcePwdNewPwd("");
            setForcePwdAuto(true);
            setForcePwdResult(null);
          }
        }}
        embyName={forcePwdEmbyName}
        onEmbyNameChange={setForcePwdEmbyName}
        newPwd={forcePwdNewPwd}
        onNewPwdChange={setForcePwdNewPwd}
        auto={forcePwdAuto}
        onAutoChange={setForcePwdAuto}
        result={forcePwdResult}
        onSubmit={handleForceSetEmbyPassword}
        onCopyPassword={(pwd) => {
          void navigator.clipboard.writeText(pwd);
          toast({ title: "已复制到剪贴板", variant: "success" });
        }}
        isLoading={forcePwdLoading}
      />

      {/* Renew Dialog */}
      <RenewUserDialog
        open={renewOpen}
        onOpenChange={setRenewOpen}
        user={selectedUser}
        mode={renewMode}
        days={renewDays}
        onDaysChange={setRenewDays}
        permanent={renewPermanent}
        onPermanentChange={setRenewPermanent}
        onSubmit={handleRenew}
        isLoading={isActionLoading}
      />

      {/* 删除（含邀请树级联）对话框 */}
      <DeleteUserDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        target={deleteTarget}
        scope={deleteScope}
        onScopeChange={setDeleteScope}
        cascadeDepth={cascadeDepth}
        onCascadeDepthChange={setCascadeDepth}
        onConfirm={confirmDelete}
        isLoading={isDeleting}
      />

      {/* 启停（支持邀请树级联） */}
      <ToggleActiveDialog
        open={toggleOpen}
        onOpenChange={setToggleOpen}
        target={toggleTarget}
        reason={toggleReason}
        onReasonChange={setToggleReason}
        cascadeDepth={toggleCascadeDepth}
        onCascadeDepthChange={setToggleCascadeDepth}
        onConfirm={confirmToggleActive}
        isLoading={isToggling}
      />

      {/* 单独禁用 Emby（带可选理由，记入操作日志） */}
      <Dialog open={embyDisableOpen} onOpenChange={setEmbyDisableOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{t("adminUsers.embyToggleDisableTitle")}</DialogTitle>
            <DialogDescription>{t("adminUsers.embyToggleDisableDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3 text-sm">
            {embyDisableTarget && (
              <p className="text-xs text-muted-foreground">
                {embyDisableTarget.username} (UID: {embyDisableTarget.uid})
              </p>
            )}
            <div className="space-y-1.5">
              <Label className="text-xs uppercase tracking-wider text-muted-foreground">
                {t("adminUsers.disableReasonLabel")}
              </Label>
              <Input
                value={embyDisableReason}
                onChange={(e) => setEmbyDisableReason(e.target.value)}
                placeholder={t("adminUsers.disableReasonPlaceholder")}
                maxLength={200}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEmbyDisableOpen(false)} disabled={embyDisableLoading}>
              {t("common.cancel")}
            </Button>
            <Button variant="destructive" onClick={() => void confirmEmbyDisable()} disabled={embyDisableLoading}>
              {embyDisableLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t("adminUsers.menuEmbyDisable")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 绑定 Emby 对话框 */}
      <BindEmbyDialog
        open={bindOpen}
        onOpenChange={(open) => {
          setBindOpen(open);
          if (!open) setBindConflict(null);
        }}
        target={bindTarget}
        embyName={bindEmbyName}
        onEmbyNameChange={(next) => {
          setBindEmbyName(next);
          setBindConflict(null);
        }}
        conflict={bindConflict}
        onSubmit={(force) => void submitBindEmby(force)}
        isSubmitting={bindSubmitting}
      />

      <Dialog open={bindTgOpen} onOpenChange={setBindTgOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>绑定 Telegram 到 {bindTgTarget?.username}</DialogTitle>
            <DialogDescription>强制将指定 Telegram ID 绑定到此用户。此操作会覆盖用户当前的 Telegram 绑定。</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="bind-tg-id">Telegram ID</Label>
            <Input
              id="bind-tg-id"
              type="number"
              value={bindTgId}
              onChange={(e) => setBindTgId(e.target.value)}
              placeholder="请输入 Telegram 数字 ID"
            />
            <p className="text-xs text-muted-foreground">Telegram 用户 ID 可以通过 @userinfobot 获取</p>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setBindTgOpen(false)} disabled={bindTgSubmitting}>
              取消
            </Button>
            <Button onClick={() => void submitBindTelegram()} disabled={bindTgSubmitting}>
              {bindTgSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认绑定
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 一键踢出 TG 群里未绑账号的成员 */}
      {/* 一键踢出 TG 群里未绑账号的成员 */}
      <KickUnboundDialog
        open={kickOpen}
        onOpenChange={setKickOpen}
        loading={kickLoading}
        roster={kickRoster}
        preview={kickPreview}
        result={kickResult}
        confirmText={kickConfirmText}
        onConfirmTextChange={setKickConfirmText}
        onConfirm={handleKickConfirm}
      />

      {/* 一键批量到期调控 */}
      <BulkExpireDialog
        open={bulkExpireOpen}
        onOpenChange={setBulkExpireOpen}
        mode={bulkExpireMode}
        onModeChange={setBulkExpireMode}
        days={bulkExpireDays}
        onDaysChange={setBulkExpireDays}
        includeAdmin={bulkExpireIncludeAdmin}
        onIncludeAdminChange={setBulkExpireIncludeAdmin}
        includeWhitelist={bulkExpireIncludeWhitelist}
        onIncludeWhitelistChange={setBulkExpireIncludeWhitelist}
        confirmText={bulkExpireConfirmText}
        onConfirmTextChange={setBulkExpireConfirmText}
        filterSummary={{
          role:
            roleFilter === "all"
              ? "全部"
              : roleFilter === "0"
                ? "管理员"
                : roleFilter === "1"
                  ? "普通用户"
                  : "白名单",
          active:
            activeFilter === "all"
              ? "全部"
              : activeFilter === "true"
                ? "仅已启用"
                : "仅已禁用",
          emby:
            embyFilter === "all"
              ? "全部"
              : embyFilter === "bound"
                ? "已绑定"
                : "未绑定",
        }}
        onSubmit={handleBulkExpire}
        isLoading={bulkExpireLoading}
      />

      {/* 批量启用已禁用账号 */}
      <BulkEnableDialog
        open={bulkEnableOpen}
        onOpenChange={setBulkEnableOpen}
        includeAdmin={bulkEnableIncludeAdmin}
        onIncludeAdminChange={setBulkEnableIncludeAdmin}
        includeWhitelist={bulkEnableIncludeWhitelist}
        onIncludeWhitelistChange={setBulkEnableIncludeWhitelist}
        confirmText={bulkEnableConfirmText}
        onConfirmTextChange={setBulkEnableConfirmText}
        filterSummary={{
          search: search.trim() || "无",
          role:
            roleFilter === "all"
              ? "全部"
              : roleFilter === "0"
                ? "管理员"
                : roleFilter === "1"
                  ? "普通用户"
                  : "白名单",
          active:
            activeFilter === "all"
              ? "全部（实际只处理已禁用）"
              : activeFilter === "true"
                ? "仅已启用（不会匹配禁用账号）"
                : "仅已禁用",
          emby:
            embyFilter === "all"
              ? "全部"
              : embyFilter === "bound"
                ? "已绑定"
                : "未绑定",
        }}
        blocked={bulkEnableBlocked}
        onSubmit={handleBulkEnableDisabled}
        isLoading={bulkEnableLoading}
      />

      {/* 新建 Web 账号 */}
      <WebUserCreateDialog
        open={webUserOpen}
        onOpenChange={setWebUserOpen}
        form={{ username: webUserName, password: webUserPwd, email: webUserEmail, role: webUserRole }}
        onFormChange={(next) => {
          setWebUserName(next.username);
          setWebUserPwd(next.password);
          setWebUserEmail(next.email);
          setWebUserRole(next.role);
        }}
        onSubmit={handleCreateWebUser}
        isSubmitting={webUserSubmitting}
      />

      {/* 新建独立 Emby 账号 */}
      <StandaloneEmbyCreateDialog
        open={standaloneOpen}
        onOpenChange={setStandaloneOpen}
        form={{ name: standaloneName, pwd: standalonePwd, email: standaloneEmail }}
        onFormChange={(next) => {
          setStandaloneName(next.name);
          setStandalonePwd(next.pwd);
          setStandaloneEmail(next.email);
        }}
        onSubmit={handleCreateStandaloneEmby}
        isSubmitting={standaloneSubmitting}
      />

      {/* 邮箱验证管理：强制绑定 / 置验证状态 */}
      <AdminEmailDialog
        open={emailDialogOpen}
        onOpenChange={setEmailDialogOpen}
        user={emailDialogUser}
        onDone={() => {
          invalidateUsersCache();
          void loadUsers();
        }}
      />
    </div>
  );
}
