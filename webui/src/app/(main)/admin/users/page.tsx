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
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
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
import { PageError } from "@/components/layout/page-state";
import { api, type UserInfo } from "@/lib/api";
import { ApiError } from "@/lib/api-request";
import { ErrCodes } from "@/lib/errcode";
import { formatDate } from "@/lib/utils";
import {
  batchDeleteConfirmConfig,
  batchLockEmbyUnbindConfirmConfig,
  batchToggleConfirmConfig,
  buildUsersCacheKey,
  hasStrongAdminPassword,
  toggleSetMember,
  usersBatchFilterParams,
  usersListParams,
} from "./admin-users-helpers";
import { renderExpireCell, renderRoleBadge, UserActionsMenu } from "./admin-users-cells";
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
  const [users, setUsers] = useState<UserInfo[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(20);
  const [pages, setPages] = useState(1);
  const [search, setSearch] = useState("");
  // 筛选 / 排序
  const [roleFilter, setRoleFilter] = useState<string>("all"); // "all" | "0" | "1" | "2"
  const [activeFilter, setActiveFilter] = useState<string>("all"); // "all" | "true" | "false"
  const [embyFilter, setEmbyFilter] = useState<string>("all"); // "all" | "bound" | "unbound"
  const [sortBy, setSortBy] = useState<string>("uid_asc");
  const [expandedUserIds, setExpandedUserIds] = useState<Set<number>>(new Set());

  // Dialog states
  const [renewOpen, setRenewOpen] = useState(false);
  const [renewDays, setRenewDays] = useState("30");
  const [renewPermanent, setRenewPermanent] = useState(false);
  const [renewMode, setRenewMode] = useState<"renew" | "cancelPermanent">("renew");
  const [selectedUser, setSelectedUser] = useState<UserInfo | null>(null);
  const [isActionLoading, setIsActionLoading] = useState(false);
  
  // Cleanup dialog states
  const [cleanupOpen, setCleanupOpen] = useState(false);
  const [cleanupMinDays, setCleanupMinDays] = useState("7");
  const [cleanupPreview, setCleanupPreview] = useState<any[] | null>(null);
  const [cleanupConfirmText, setCleanupConfirmText] = useState("");
  const [cleanupLoading, setCleanupLoading] = useState(false);
  const [stalePendingLoading, setStalePendingLoading] = useState(false);
  const [registrationQueueLoading, setRegistrationQueueLoading] = useState(false);

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
    const listState = { page, perPage, search, roleFilter, activeFilter, embyFilter, sortBy };
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
  }, [page, perPage, search, roleFilter, activeFilter, embyFilter, sortBy]);

  const {
    isLoading,
    error,
    execute: loadUsers,
  } = useAsyncResource(loadUsersResource, { immediate: true });

  const selectedUsers = useMemo(
    () => users.filter((user) => selectedUserIds.has(user.uid)),
    [users, selectedUserIds],
  );
  const selectedUids = useMemo(() => Array.from(selectedUserIds), [selectedUserIds]);
  const selectedCount = selectionScope === "all" ? total : selectionScope === "emby" ? selectionScopeCount : selectedUserIds.size;
  const selectedEmbyCount = useMemo(
    () => selectedUsers.filter((user) => Boolean(user.emby_id)).length,
    [selectedUsers],
  );
  const selectedAdminCount = useMemo(
    () => selectedUsers.filter((user) => user.role === 0).length,
    [selectedUsers],
  );
  const currentListState = useMemo(
    () => ({ page, perPage, search, roleFilter, activeFilter, embyFilter, sortBy }),
    [page, perPage, search, roleFilter, activeFilter, embyFilter, sortBy],
  );

  const isUserSelected = (user: UserInfo) =>
    selectionScope === "all" ||
    (selectionScope === "emby" && Boolean(user.emby_id)) ||
    selectedUserIds.has(user.uid);

  const allPageSelected = users.length > 0 && users.every(isUserSelected);

  const clearUserSelection = () => {
    setSelectedUserIds(new Set());
    setSelectionScope("manual");
    setSelectionScopeCount(0);
  };

  const embyMatchingFilter = () => ({
    ...usersBatchFilterParams(currentListState),
    emby: "bound" as const,
  });

  const selectedBatchTarget = () => (
    selectionScope === "all"
      ? { select_all: true, filter: usersBatchFilterParams(currentListState) }
      : selectionScope === "emby"
        ? { select_all: true, filter: embyMatchingFilter() }
      : selectedUids
  );

  const toggleSelectedUser = (uid: number) => {
    if (selectionScope !== "manual") {
      setSelectionScope("manual");
      setSelectionScopeCount(0);
      setSelectedUserIds(new Set(users.filter((user) => isUserSelected(user) && user.uid !== uid).map((user) => user.uid)));
      return;
    }
    setSelectedUserIds((prev) => toggleSetMember(prev, uid));
  };

  const toggleSelectCurrentPage = () => {
    if (selectionScope !== "manual") {
      clearUserSelection();
      return;
    }
    setSelectedUserIds((prev) => {
      const next = new Set(prev);
      if (allPageSelected) {
        users.forEach((user) => next.delete(user.uid));
      } else {
        users.forEach((user) => next.add(user.uid));
      }
      return next;
    });
  };

  const selectCurrentPageUsers = () => {
    setSelectionScope("manual");
    setSelectionScopeCount(0);
    setSelectedUserIds(new Set(users.map((user) => user.uid)));
  };

  const selectAllMatchingUsers = () => {
    setSelectedUserIds(new Set(users.map((user) => user.uid)));
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
      setSelectedUserIds(new Set(users.filter((user) => Boolean(user.emby_id)).map((user) => user.uid)));
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
  }, [roleFilter, activeFilter, embyFilter, sortBy]);

  useEffect(() => {
    clearUserSelection();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [roleFilter, activeFilter, embyFilter, sortBy]);

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
        : await api.renewUser(selectedUser.uid, days);
      if (res.success) {
        toast({ title: renewMode === "cancelPermanent" ? "已取消永久" : "续期成功", variant: "success" });
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
    const filter: { role?: number; active?: boolean; emby?: "bound" | "unbound"; search?: string } = {};
    if (options.currentFilter) {
      if (roleFilter !== "all") filter.role = Number(roleFilter);
      if (activeFilter !== "all") filter.active = activeFilter === "true";
      if (embyFilter === "bound") filter.emby = "bound";
      else if (embyFilter === "unbound") filter.emby = "unbound";
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
    const ok = await confirmAction(batchToggleConfirmConfig(enable, selectedCount));
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
    const ok = await confirmAction(batchLockEmbyUnbindConfirmConfig(selectedCount));
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

  const handleSelectedDelete = async () => {
    if (selectedCount === 0) return;
    const action = await confirmAction(batchDeleteConfirmConfig(
      selectedCount,
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

  if (error) {
    return <PageError message={error} onRetry={() => void loadUsers()} />;
  }

  const getRoleBadge = renderRoleBadge;

  const renderUserActions = (user: UserInfo) => (
    <UserActionsMenu
      user={user}
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
        onResetPassword: handleResetPassword,
        onBindEmby: handleOpenBindEmby,
        onSyncBindings: (u) => handleSyncBindings({ uid: u.uid }),
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
        <div className="flex flex-wrap items-center gap-2 sm:gap-3">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline">
                <MoreHorizontal className="mr-2 h-4 w-4" />
                管理操作
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              <DropdownMenuItem
                onClick={() => {
                  setWebUserName("");
                  setWebUserPwd("");
                  setWebUserEmail("");
                  setWebUserRole(1);
                  setWebUserOpen(true);
                }}
              >
                <UserPlus className="mr-2 h-4 w-4" />
                新建 Web 账号
              </DropdownMenuItem>
              <DropdownMenuSeparator />
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
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
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
                  <SelectItem value="last_login_time_desc">最近登录 新→旧</SelectItem>
                  <SelectItem value="role_asc">角色 管理→普通</SelectItem>
                  <SelectItem value="active_desc">状态 启用优先</SelectItem>
                  <SelectItem value="active_asc">状态 禁用优先</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          {(roleFilter !== "all" || activeFilter !== "all" || embyFilter !== "all" || sortBy !== "uid_asc") && (
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>已应用筛选 / 排序</span>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  setRoleFilter("all");
                  setActiveFilter("all");
                  setEmbyFilter("all");
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
        <Card className="border-primary/30">
          <CardContent className="flex flex-col gap-4 p-4 xl:flex-row xl:items-center xl:justify-between">
            <div className="space-y-1">
              <p className="font-medium">
                {selectionScope === "all"
                  ? `已选择全部 ${total} 个匹配用户`
                  : selectionScope === "emby"
                    ? `已选择 ${selectionScopeCount} 个拥有 Emby 的匹配用户`
                    : selectedCount > 0
                      ? `已选择 ${selectedCount} 个用户`
                      : "批量选择"}
              </p>
              {selectedCount > 0 ? (
                <p className="text-xs text-muted-foreground">
                  {selectionScope === "all"
                    ? "将按当前筛选条件作用于全部匹配用户；管理员账号会被后端自动跳过。"
                    : selectionScope === "emby"
                      ? "将按当前筛选条件中已绑定 Emby 的用户执行；未绑定 Emby 的用户不会进入本次选择。"
                    : `当前页可见已选用户中 ${selectedEmbyCount} 个已绑定 Emby${selectedAdminCount > 0 ? `，${selectedAdminCount} 个管理员会被后端自动跳过` : "；管理员账号会被后端自动跳过"}。`}
                </p>
              ) : (
                <p className="text-xs text-muted-foreground">可选择当前页、当前筛选下拥有 Emby 的用户，或当前筛选下全部用户。</p>
              )}
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <div className="flex flex-wrap items-center gap-2 rounded-md border bg-muted/20 p-1">
                <span className="px-2 text-xs text-muted-foreground">选择范围</span>
                <Button variant={selectionScope === "manual" && selectedCount > 0 && allPageSelected ? "default" : "outline"} size="sm" onClick={selectCurrentPageUsers} disabled={batchUserLoading || users.length === 0}>
                  选中当前页
                </Button>
                <Button variant={selectionScope === "emby" ? "default" : "outline"} size="sm" onClick={() => void selectEmbyMatchingUsers()} disabled={batchUserLoading || selectEmbyLoading || total === 0}>
                  {selectEmbyLoading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Link2 className="mr-2 h-4 w-4" />}
                  选中拥有 Emby 的
                </Button>
                <Button variant={selectionScope === "all" ? "default" : "outline"} size="sm" onClick={selectAllMatchingUsers} disabled={batchUserLoading || total === 0}>
                  选中全部
                </Button>
              </div>
              <Button variant="outline" size="sm" onClick={clearUserSelection} disabled={batchUserLoading || selectedCount === 0}>
                清空选择
              </Button>
              <div className="flex flex-wrap items-center gap-2 rounded-md border bg-muted/20 p-1">
                <span className="px-2 text-xs text-muted-foreground">账号状态</span>
                <Button variant="outline" size="sm" onClick={() => void handleSelectedToggleActive(true)} disabled={batchUserLoading || selectedCount === 0}>
                  <UserCheck className="mr-2 h-4 w-4" />
                  启用
                </Button>
                <Button variant="outline" size="sm" onClick={() => void handleSelectedToggleActive(false)} disabled={batchUserLoading || selectedCount === 0}>
                  <Ban className="mr-2 h-4 w-4" />
                  禁用
                </Button>
              </div>
              <Button variant="outline" size="sm" onClick={() => void handleSelectedLockEmbyUnbind()} disabled={batchUserLoading || selectedCount === 0}>
                <LockKeyhole className="mr-2 h-4 w-4" />
                禁止解绑 Emby
              </Button>
              <Button variant="destructive" size="sm" onClick={() => void handleSelectedDelete()} disabled={batchUserLoading || selectedCount === 0}>
                {batchUserLoading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Trash2 className="mr-2 h-4 w-4" />}
                删除所选
              </Button>
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
                      <div className="min-w-0">
                        <p className="truncate text-base font-medium">{user.username}</p>
                        <p className="mt-1 text-xs text-muted-foreground">UID: {user.uid}</p>
                        <p className="mt-0.5 truncate text-xs text-muted-foreground" title={user.email || "未设置"}>
                          邮箱: {user.email || "未设置"}
                        </p>
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
                      <p className="text-xs text-muted-foreground">状态</p>
                      <div className="mt-1">
                        <Badge variant={user.active ? "success" : "destructive"}>
                          {user.active ? "正常" : "禁用"}
                        </Badge>
                      </div>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">Emby</p>
                      <div className="mt-1 flex flex-col gap-0.5 min-w-0">
                        {user.emby_id ? (
                          <>
                            <Badge variant="success" className="w-fit text-[10px]">已绑定</Badge>
                            <span className="text-xs text-muted-foreground truncate" title={user.emby_username || user.username}>
                              {user.emby_username || user.username}
                            </span>
                          </>
                        ) : (
                          <Badge variant="outline" className="w-fit text-[10px] text-muted-foreground">
                            未绑定
                          </Badge>
                        )}
                      </div>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">到期时间</p>
                      <p className="mt-1">{renderExpireCell(user)}</p>
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
                    <th className="px-4 py-3 text-left text-sm font-medium">状态</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">Emby</th>
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
                            <p className="max-w-[260px] truncate text-xs text-muted-foreground" title={user.email || "未设置"}>
                              邮箱: {user.email || "未设置"}
                            </p>
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
                      <td className="px-4 py-3">
                        <Badge variant={user.active ? "success" : "destructive"}>
                          {user.active ? "正常" : "禁用"}
                        </Badge>
                      </td>
                      <td className="px-4 py-3">
                        {user.emby_id ? (
                          <div className="flex flex-col gap-0.5 min-w-0">
                            <Badge variant="success" className="w-fit text-[10px]">已绑定</Badge>
                            <span
                              className="text-xs text-muted-foreground truncate max-w-[160px]"
                              title={user.emby_username || user.username}
                            >
                              {user.emby_username || user.username}
                            </span>
                          </div>
                        ) : (
                          <Badge variant="outline" className="text-[10px] text-muted-foreground">
                            未绑定
                          </Badge>
                        )}
                      </td>
                      <td className="px-4 py-3 text-sm">{renderExpireCell(user)}</td>
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
      {pages > 1 && (
        <div className="flex items-center justify-center gap-2">
          <Button
            variant="outline"
            size="icon"
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            disabled={page === 1}
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <span className="text-sm">
            第 {page} 页，共 {pages} 页
          </span>
          <Button
            variant="outline"
            size="icon"
            onClick={() => setPage((p) => Math.min(pages, p + 1))}
            disabled={page === pages}
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      )}

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
    </div>
  );
}
