"use client";

import { Fragment, useCallback, useEffect, useRef, useState } from "react";
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
  Edit,
  UserX,
  Link2,
  AlertTriangle,
  UserPlus,
  UserCheck,
  CalendarClock,
  Send,
  Unlink,
  Eye,
  EyeOff,
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError } from "@/components/layout/page-state";
import { api, type UserInfo, type EmbyLibraryAccess } from "@/lib/api";
import { formatDate } from "@/lib/utils";

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

  // 媒体库权限对话框
  const [libraryOpen, setLibraryOpen] = useState(false);
  const [libraryTarget, setLibraryTarget] = useState<UserInfo | null>(null);
  const [libraryAccess, setLibraryAccess] = useState<EmbyLibraryAccess | null>(null);
  const [selectedLibraryIds, setSelectedLibraryIds] = useState<Set<string>>(new Set());
  const [libraryLoading, setLibraryLoading] = useState(false);
  const [librarySelfServiceBulkLoading, setLibrarySelfServiceBulkLoading] = useState(false);

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
    setExpandedUserIds((prev) => {
      const next = new Set(prev);
      if (next.has(uid)) {
        next.delete(uid);
      } else {
        next.add(uid);
      }
      return next;
    });
  };

  const loadUsersResource = useCallback(async (signal?: AbortSignal) => {
    const cacheKey = `${page}-${perPage}-${search || ""}-${roleFilter}-${activeFilter}-${embyFilter}-${sortBy}`;
    const cached = usersCacheRef.current.get(cacheKey);
    if (cached) {
      setUsers(cached.users);
      setTotal(cached.total);
      setPages(cached.pages);
      return true;
    }

    const role = roleFilter === "all" ? undefined : Number(roleFilter);
    const active =
      activeFilter === "all" ? undefined : activeFilter === "true";
    const emby =
      embyFilter === "bound" ? "bound" : embyFilter === "unbound" ? "unbound" : undefined;

    const res = await api.getUsers({
      page,
      per_page: perPage,
      search: search || undefined,
      role,
      active,
      emby,
      sort: sortBy || undefined,
    }, signal);
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

  const handleSearch = () => {
    invalidateUsersCache();
    setExpandedUserIds(new Set());
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

  const handleForceSetEmbyPassword = async () => {
    const emby = forcePwdEmbyName.trim();
    if (!emby) {
      toast({ title: "请输入 Emby 用户名", variant: "destructive" });
      return;
    }
    if (!forcePwdAuto) {
      const pwd = forcePwdNewPwd;
      if (pwd.length < 8 || !/[a-z]/.test(pwd) || !/[A-Z]/.test(pwd) || !/\d/.test(pwd)) {
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
      } else if (res.data?.conflict) {
        setBindConflict({
          emby_id: res.data.emby_id,
          emby_username: res.data.emby_username,
          conflict_uid: res.data.conflict_uid!,
          conflict_username: res.data.conflict_username!,
        });
      } else {
        toast({ title: "绑定失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "绑定失败", description: error.message, variant: "destructive" });
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

  const applyLibraryAccessState = (data: EmbyLibraryAccess) => {
    setLibraryAccess(data);
    setSelectedLibraryIds(new Set((data.libraries || []).map((lib) => lib.id)));
  };

  const handleOpenLibraries = async (user: UserInfo) => {
    setLibraryTarget(user);
    setLibraryAccess(null);
    setSelectedLibraryIds(new Set());
    setLibraryOpen(true);
    setLibraryLoading(true);
    try {
      const res = await api.getUserLibraries(user.uid);
      if (res.success && res.data) {
        applyLibraryAccessState(res.data);
      } else {
        toast({ title: "加载媒体库权限失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "加载媒体库权限失败", description: error.message, variant: "destructive" });
    } finally {
      setLibraryLoading(false);
    }
  };

  const toggleSelectedLibrary = (libraryId: string) => {
    setSelectedLibraryIds((prev) => {
      const next = new Set(prev);
      if (next.has(libraryId)) next.delete(libraryId);
      else next.add(libraryId);
      return next;
    });
  };

  const updateTargetLibraries = async (payload: Parameters<typeof api.updateUserLibraries>[1]) => {
    if (!libraryTarget) return;
    setLibraryLoading(true);
    try {
      const res = await api.updateUserLibraries(libraryTarget.uid, payload);
      if (res.success && res.data) {
        applyLibraryAccessState(res.data);
        toast({ title: "媒体库权限已更新", variant: "success" });
      } else {
        toast({ title: "更新失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "更新失败", description: error.message, variant: "destructive" });
    } finally {
      setLibraryLoading(false);
    }
  };

  const saveSelectedLibraries = async () => {
    await updateTargetLibraries({
      action: "set",
      library_ids: Array.from(selectedLibraryIds),
      enable_all: false,
    });
  };

  const applyLibrarySelfServiceState = (uid: number, enabled: boolean) => {
    setUsers((prev) => prev.map((item) => item.uid === uid ? { ...item, library_self_service: enabled } : item));
    setLibraryTarget((prev) => prev && prev.uid === uid ? { ...prev, library_self_service: enabled } : prev);
    if (libraryTarget?.uid === uid) {
      setLibraryAccess((prev) => prev ? { ...prev, self_service_enabled: enabled } : prev);
    }
  };

  const handleSetLibrarySelfService = async (user: UserInfo, enabled: boolean) => {
    setIsActionLoading(true);
    try {
      const res = await api.setUserLibrarySelfService(user.uid, enabled);
      if (res.success && res.data) {
        applyLibrarySelfServiceState(user.uid, res.data.library_self_service);
        invalidateUsersCache();
        toast({ title: enabled ? "已开启自助显隐权限" : "已关闭自助显隐权限", variant: "success" });
      } else {
        toast({ title: "更新失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "更新失败", description: error.message, variant: "destructive" });
    } finally {
      setIsActionLoading(false);
    }
  };

  const handleBulkEnableLibrarySelfService = async () => {
    const action = await confirmAction({
      title: "为所有用户开启媒体库自助显隐？",
      description: "只开启入口授权，不会改变任何用户当前可见的媒体库。用户仍只能操作管理员在 Emby 配置中开放的媒体库。",
      tone: "warning",
      confirmLabel: "批量开启",
    });
    if (!action) return;

    setLibrarySelfServiceBulkLoading(true);
    try {
      const res = await api.bulkEnableLibrarySelfService();
      if (res.success && res.data) {
        toast({ title: `已为 ${res.data.updated} 个用户开启自助显隐权限`, variant: "success" });
        invalidateUsersCache();
        await loadUsers();
      } else {
        toast({ title: "批量开启失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "批量开启失败", description: error.message, variant: "destructive" });
    } finally {
      setLibrarySelfServiceBulkLoading(false);
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

  const getRoleBadge = (role: number) => {
    switch (role) {
      case 0:
        return <Badge variant="gradient">管理员</Badge>;
      case 2:
        return <Badge variant="success">白名单</Badge>;
      default:
        return <Badge variant="secondary">普通用户</Badge>;
    }
  };

  /**
   * 根据 emby_bound / expired_at / pending_emby 渲染到期时间单元格。
   * - 未绑定 Emby（emby_bound===false / pending_emby / expired_at===0）→"未绑定"
   * - -1 / "-1" → "永久"
   * - 真实时间戳 → 用 formatDate；已过期红字
   */
  const renderExpireCell = (user: UserInfo) => {
    const exp = user.expired_at;
    const isUnbound =
      user.emby_bound === false ||
      Boolean(user.pending_emby) ||
      exp === 0 ||
      exp === "0";
    if (isUnbound) {
      return <span className="text-muted-foreground italic">未绑定</span>;
    }
    if (exp === -1 || exp === "-1" || exp == null) {
      return <span className="text-emerald-500">永久</span>;
    }
    const expMs = typeof exp === "number" && exp < 10000000000 ? exp * 1000 : Number(exp);
    const expired = !Number.isNaN(expMs) && expMs < Date.now();
    return (
      <span className={expired ? "text-destructive" : undefined}>
        {formatDate(exp)}
      </span>
    );
  };

  const renderUserActions = (user: UserInfo) => (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon">
          <MoreHorizontal className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onClick={() => handleOpenEdit(user)}>
          <Edit className="mr-2 h-4 w-4" />
          编辑信息
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => {
            setSelectedUser(user);
            setRenewMode("renew");
            setRenewPermanent(false);
            setRenewDays("30");
            setRenewOpen(true);
          }}
        >
          <RefreshCw className="mr-2 h-4 w-4" />
          续期
        </DropdownMenuItem>
        {(user.expired_at === -1 || user.expired_at === "-1") && user.role !== 0 && Boolean(user.emby_id) && (
          <DropdownMenuItem
            onClick={() => {
              setSelectedUser(user);
              setRenewMode("cancelPermanent");
              setRenewPermanent(false);
              setRenewDays("30");
              setRenewOpen(true);
            }}
          >
            <CalendarClock className="mr-2 h-4 w-4" />
            取消永久到期
          </DropdownMenuItem>
        )}
        <DropdownMenuItem onClick={() => handleResetPassword(user)}>
          <Key className="mr-2 h-4 w-4" />
          重置密码
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handleOpenBindEmby(user)}>
          <Link2 className="mr-2 h-4 w-4" />
          绑定 Emby
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => void handleOpenLibraries(user)} disabled={!Boolean(user.emby_id)}>
          <Eye className="mr-2 h-4 w-4" />
          媒体库权限
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => void handleSetLibrarySelfService(user, !Boolean(user.library_self_service))}
          disabled={isActionLoading}
        >
          {user.library_self_service ? <EyeOff className="mr-2 h-4 w-4" /> : <Eye className="mr-2 h-4 w-4" />}
          {user.library_self_service ? "关闭自助显隐权" : "开启自助显隐权"}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handleSyncBindings({ uid: user.uid })}>
          <RefreshCw className="mr-2 h-4 w-4" />
          同步绑定状态
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handleForceUnbind(user)}>
          <Unlink className="mr-2 h-4 w-4" />
          强制解绑
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => handleClearRegistrationQueue(user)}>
          <CalendarClock className="mr-2 h-4 w-4" />
          清理注册队列用户
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => handleGrantRegistrationEntitlement(user)}
          disabled={Boolean(user.emby_id) || !user.active}
        >
          <UserPlus className="mr-2 h-4 w-4" />
          给予队列用户注册权利
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => handleGrantRegistrationEntitlementAndDequeue(user)}
          disabled={Boolean(user.emby_id) || !user.active}
        >
          <UserCheck className="mr-2 h-4 w-4" />
          授权并移出未处理队列
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => handleToggleActive(user)}>
          <Ban className="mr-2 h-4 w-4" />
          {user.active ? "禁用" : "启用"}
        </DropdownMenuItem>
        <DropdownMenuItem className="text-destructive" onClick={() => handleDelete(user)}>
          <Trash2 className="mr-2 h-4 w-4" />
          删除
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
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
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    disabled={librarySelfServiceBulkLoading}
                    onClick={() => void handleBulkEnableLibrarySelfService()}
                  >
                    {librarySelfServiceBulkLoading ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <Eye className="mr-2 h-4 w-4" />
                    )}
                    批量开启自助显隐
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
                placeholder="搜索用户名、UID 或 Telegram ID..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
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
                    <div className="min-w-0">
                      <p className="truncate text-base font-medium">{user.username}</p>
                      <p className="mt-1 text-xs text-muted-foreground">UID: {user.uid}</p>
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

                  {(user.telegram_id || user.emby_id || user.library_self_service) && (
                    <div className="mt-3 space-y-1 border-t pt-3 text-xs text-muted-foreground">
                      {user.telegram_id && (
                        <p>
                          TG: {user.telegram_username ? `@${user.telegram_username} (${user.telegram_id})` : user.telegram_id}
                        </p>
                      )}
                      {user.emby_id && <p className="break-all">Emby ID: {user.emby_id}</p>}
                      {user.library_self_service && <p>媒体库自助显隐：已开启</p>}
                    </div>
                  )}
                </div>
              ))}
            </div>

            <div className="hidden overflow-x-auto md:block">
              <table className="w-full">
                <thead>
                  <tr className="border-b bg-muted/50">
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
                            {user.library_self_service && (
                              <Badge variant="outline" className="w-fit text-[10px]">自助显隐</Badge>
                            )}
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
                        <td colSpan={6} className="px-4 py-3 text-sm text-muted-foreground">
                          <div className="grid gap-3 sm:grid-cols-2">
                            <div>
                              <p className="font-medium">更多信息</p>
                              <p>注册时间: {user.register_time ? formatDate(user.register_time) : "未知"}</p>
                              <p>创建时间: {user.created_at ? formatDate(user.created_at) : "未记录"}</p>
                              <p>媒体库自助显隐: {user.library_self_service ? "已开启" : "未开启"}</p>
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
      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>编辑用户信息</DialogTitle>
            <DialogDescription>
              编辑用户 {selectedUser?.username} 的详细信息
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>角色</Label>
              <Select
                value={editForm.role.toString()}
                onValueChange={(v) => setEditForm({ ...editForm, role: parseInt(v) })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="0">管理员</SelectItem>
                  <SelectItem value="1">普通用户</SelectItem>
                  <SelectItem value="2">白名单</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>Emby ID（可选）</Label>
              <Input
                placeholder="输入 Emby 用户 ID"
                value={editForm.emby_id}
                onChange={(e) => setEditForm({ ...editForm, emby_id: e.target.value })}
              />
            </div>

            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="active"
                checked={editForm.active}
                onChange={(e) => setEditForm({ ...editForm, active: e.target.checked })}
                className="h-4 w-4 rounded border-gray-300"
              />
              <Label htmlFor="active" className="cursor-pointer">
                启用账号
              </Label>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditOpen(false)}>
              取消
            </Button>
            <Button onClick={handleEdit} disabled={isActionLoading}>
              {isActionLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              保存更改
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Library Access Dialog */}
      <Dialog open={libraryOpen} onOpenChange={(open) => { if (!libraryLoading) setLibraryOpen(open); }}>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>媒体库权限</DialogTitle>
            <DialogDescription>
              {libraryTarget ? `调整 ${libraryTarget.username} 的 Emby 媒体库显示/隐藏权限` : "调整用户媒体库权限"}
            </DialogDescription>
          </DialogHeader>

          {libraryLoading && !libraryAccess ? (
            <div className="flex items-center gap-2 rounded-lg bg-accent/50 p-4 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              正在加载媒体库权限
            </div>
          ) : !libraryAccess?.has_emby ? (
            <div className="rounded-lg bg-accent/50 p-4 text-sm text-muted-foreground">
              该用户尚未绑定 Emby 账号，无法设置媒体库权限。
            </div>
          ) : libraryAccess ? (
            <div className="space-y-4">
              <div className="flex flex-col gap-3 rounded-lg border bg-card/60 p-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="space-y-1">
                  <Label>自助显隐授权</Label>
                  <p className="text-xs text-muted-foreground">
                    只控制该用户能否在个人设置里自行操作；下方按钮才会改变当前媒体库可见状态。
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant={libraryAccess.self_service_enabled ? "success" : "outline"}>
                    {libraryAccess.self_service_enabled ? "已开启" : "未开启"}
                  </Badge>
                  {libraryTarget && (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={libraryLoading || isActionLoading}
                      onClick={() => void handleSetLibrarySelfService(libraryTarget, !libraryAccess.self_service_enabled)}
                    >
                      {libraryAccess.self_service_enabled ? "关闭授权" : "开启授权"}
                    </Button>
                  )}
                </div>
              </div>

              <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-card/60 p-3">
                <Badge variant={libraryAccess.enable_all ? "default" : "secondary"}>
                  {libraryAccess.enable_all ? "策略：显示全部" : "策略：按选择显示"}
                </Badge>
                <Badge variant="outline">可见 {libraryAccess.libraries.length}/{libraryAccess.all_libraries.length}</Badge>
                {libraryAccess.blocked_names.length > 0 && (
                  <Badge variant="outline">隐藏：{libraryAccess.blocked_names.join("、")}</Badge>
                )}
              </div>

              <div className="flex flex-wrap gap-2">
                <Button variant="outline" size="sm" disabled={libraryLoading} onClick={() => void updateTargetLibraries({ action: "enable_all" })}>
                  <Eye className="mr-2 h-4 w-4" />
                  显示全部
                </Button>
                <Button variant="outline" size="sm" disabled={libraryLoading} onClick={() => void updateTargetLibraries({ action: "disable_all" })}>
                  <EyeOff className="mr-2 h-4 w-4" />
                  隐藏全部
                </Button>
                {libraryAccess.default_hidden_libraries.length > 0 && (
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={libraryLoading}
                    onClick={() => void updateTargetLibraries({ action: "hide", library_names: libraryAccess.default_hidden_libraries })}
                  >
                    隐藏默认库
                  </Button>
                )}
              </div>

              <div className="space-y-2">
                <Label>选择可见媒体库</Label>
                <div className="grid max-h-72 gap-2 overflow-y-auto rounded-lg border p-3 sm:grid-cols-2">
                  {libraryAccess.all_libraries.map((lib) => {
                    const selected = selectedLibraryIds.has(lib.id);
                    return (
                      <button
                        key={lib.id}
                        type="button"
                        className={`rounded-lg border p-3 text-left transition-colors ${selected ? "border-primary bg-primary/10" : "bg-background hover:bg-accent"}`}
                        onClick={() => toggleSelectedLibrary(lib.id)}
                      >
                        <div className="flex items-center justify-between gap-2">
                          <span className="font-medium">{lib.name}</span>
                          <Badge variant={selected ? "default" : "secondary"}>{selected ? "显示" : "隐藏"}</Badge>
                        </div>
                        {lib.type && <p className="mt-1 text-xs text-muted-foreground">{lib.type}</p>}
                      </button>
                    );
                  })}
                </div>
              </div>

              {libraryAccess.self_service_libraries.length > 0 && (
                <div className="space-y-2 rounded-lg border p-3">
                  <Label>可自助操作的媒体库（全局配置）</Label>
                  <p className="text-xs text-muted-foreground">
                    这些库来自 Emby 配置；这里的“显示/隐藏”会直接调整当前用户可见状态。
                  </p>
                  <div className="flex flex-wrap gap-2">
                    {libraryAccess.self_service_libraries.map((name) => (
                      <div key={name} className="flex items-center gap-2 rounded-md bg-accent/50 px-2 py-1">
                        <span className="text-sm">{name}</span>
                        <Button variant="ghost" size="sm" disabled={libraryLoading} onClick={() => void updateTargetLibraries({ action: "show", library_names: [name] })}>
                          显示
                        </Button>
                        <Button variant="ghost" size="sm" disabled={libraryLoading} onClick={() => void updateTargetLibraries({ action: "hide", library_names: [name] })}>
                          隐藏
                        </Button>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          ) : null}

          <DialogFooter>
            <Button variant="outline" onClick={() => setLibraryOpen(false)} disabled={libraryLoading}>
              关闭
            </Button>
            <Button onClick={() => void saveSelectedLibraries()} disabled={libraryLoading || !libraryAccess || !libraryAccess.has_emby || libraryAccess.all_libraries.length === 0}>
              {libraryLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              保存选择
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Cleanup Invalid Users Dialog */}
      <Dialog open={cleanupOpen} onOpenChange={(open) => { setCleanupOpen(open); if (!open) setCleanupPreview(null); }}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>清理无效用户</DialogTitle>
            <DialogDescription>
              删除未绑定 Telegram 且未绑定 Emby 的非管理员 / 白名单用户
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>最少注册天数</Label>
              <Input
                type="number"
                min={1}
                placeholder="注册超过多少天的无效用户"
                value={cleanupMinDays}
                onChange={(e) => {
                  setCleanupMinDays(e.target.value);
                  setCleanupPreview(null);
                }}
              />
              <p className="text-xs text-muted-foreground">
                仅清理注册超过 {cleanupMinDays || 7} 天的无效用户
              </p>
            </div>

            {cleanupPreview !== null && (
              <div className="space-y-2">
                <Label>匹配到 {cleanupPreview.length} 个无效用户</Label>
                {cleanupPreview.length > 0 ? (
                  <div className="max-h-48 overflow-y-auto rounded-md border">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b bg-muted/50">
                          <th className="px-3 py-2 text-left">用户名</th>
                          <th className="px-3 py-2 text-left">注册时间</th>
                        </tr>
                      </thead>
                      <tbody>
                        {cleanupPreview.map((u: any) => (
                          <tr key={u.uid} className="border-b">
                            <td className="px-3 py-1.5">{u.username}</td>
                            <td className="px-3 py-1.5 text-muted-foreground">
                              {u.register_time ? new Date(u.register_time * 1000).toLocaleDateString() : "-"}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <p className="text-sm text-muted-foreground">没有符合条件的无效用户</p>
                )}
              </div>
            )}
          </div>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button variant="outline" onClick={() => setCleanupOpen(false)}>
              取消
            </Button>
            <Button
              variant="outline"
              onClick={handleCleanupPreview}
              disabled={cleanupLoading}
            >
              {cleanupLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              预览
            </Button>
            <Button
              variant="destructive"
              onClick={handleCleanupConfirm}
              disabled={cleanupLoading || !cleanupPreview || cleanupPreview.length === 0}
            >
              {cleanupLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认清理
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 一键踢出未绑定 Emby 的系统账号 */}
      <Dialog
        open={noEmbyOpen}
        onOpenChange={(open) => {
          setNoEmbyOpen(open);
          if (!open) {
            setNoEmbyPreview(null);
            setNoEmbyConfirmText("");
          }
        }}
      >
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>踢出未绑 Emby 账号</DialogTitle>
            <DialogDescription>
              删除未绑定 Emby 的系统账号。管理员 / 白名单 / 未识别角色 / 当前正在 Emby
              注册队列里的 UID 一律跳过。可选限制最少注册天数。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 text-sm">
            <div className="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end">
              <div className="space-y-2">
                <Label>最少注册天数</Label>
                <Input
                  type="number"
                  min={0}
                  value={noEmbyMinDays}
                  onChange={(e) => {
                    setNoEmbyMinDays(e.target.value);
                    setNoEmbyPreview(null);
                  }}
                  placeholder="0 表示不卡注册时间"
                />
                <p className="text-[11px] text-muted-foreground">
                  {noEmbyMinDaysParsed > 0
                    ? `仅清理注册超过 ${noEmbyMinDaysParsed} 天的账号`
                    : "无视注册时间，匹配所有未绑 Emby 的账号"}
                </p>
              </div>
              <label className="flex items-center gap-2 rounded-md border border-border/60 bg-muted/30 px-3 py-2">
                <input
                  type="checkbox"
                  checked={noEmbyPreserveDirect}
                  onChange={(e) => {
                    setNoEmbyPreserveDirect(e.target.checked);
                    setNoEmbyPreview(null);
                  }}
                  className="h-4 w-4 rounded border-gray-300"
                />
                <span className="text-xs">
                  保留可直接补建 Emby 的用户
                  <span className="block text-[10px] text-muted-foreground">
                    (已绑 Telegram 的待激活账号)
                  </span>
                </span>
              </label>
            </div>

            <p className="rounded-md border border-amber-500/40 bg-amber-500/5 px-3 py-2 text-amber-700 dark:text-amber-300">
              ⚠️ 请先「预览」确认范围后再确认执行。
            </p>

            {noEmbyPreview && (
              <div className="space-y-2">
                <Label>
                  匹配到 {noEmbyPreview.candidate_count} 个待清理账号
                </Label>
                <div className="flex flex-wrap gap-1.5 text-[10px]">
                  <Badge variant="outline">管理员 {noEmbyPreview.skipped_admins}</Badge>
                  <Badge variant="outline">白名单 {noEmbyPreview.skipped_whitelist}</Badge>
                  <Badge variant="outline">未识别 {noEmbyPreview.skipped_unrecognized}</Badge>
                  <Badge variant="outline">注册队列 {noEmbyPreview.skipped_in_queue}</Badge>
                  {noEmbyPreview.skipped_pending_register > 0 && (
                    <Badge variant="outline">可补建 Emby {noEmbyPreview.skipped_pending_register}</Badge>
                  )}
                  {noEmbyPreview.skipped_too_recent > 0 && (
                    <Badge variant="outline">注册时间不够 {noEmbyPreview.skipped_too_recent}</Badge>
                  )}
                </div>
                {noEmbyPreview.candidate_count > 0 ? (
                  <div className="max-h-56 overflow-y-auto rounded-md border">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b bg-muted/50">
                          <th className="px-3 py-2 text-left">UID</th>
                          <th className="px-3 py-2 text-left">用户名</th>
                          <th className="px-3 py-2 text-left">注册时间</th>
                          <th className="px-3 py-2 text-left">状态</th>
                        </tr>
                      </thead>
                      <tbody>
                        {noEmbyPreview.candidates.slice(0, 200).map((u) => (
                          <tr key={u.uid} className="border-b">
                            <td className="px-3 py-1.5 font-mono text-xs">{u.uid}</td>
                            <td className="px-3 py-1.5">{u.username}</td>
                            <td className="px-3 py-1.5 text-muted-foreground">
                              {u.register_time ? new Date(u.register_time * 1000).toLocaleDateString() : "-"}
                            </td>
                            <td className="px-3 py-1.5 text-xs">
                              {u.pending_emby ? "待激活" : "未绑定"}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                    {noEmbyPreview.candidates.length > 200 && (
                      <p className="px-3 py-1.5 text-xs text-muted-foreground">
                        仅展示前 200 个，实际仍会按候选总数执行
                      </p>
                    )}
                  </div>
                ) : (
                  <p className="text-sm text-muted-foreground">没有需要清理的账号</p>
                )}
              </div>
            )}

            {noEmbyPreview && noEmbyPreview.candidate_count > 0 && (
              <div className="space-y-2">
                <Label>输入「确认」以执行</Label>
                <Input
                  value={noEmbyConfirmText}
                  onChange={(e) => setNoEmbyConfirmText(e.target.value)}
                  placeholder="确认"
                />
              </div>
            )}
          </div>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button variant="outline" onClick={() => setNoEmbyOpen(false)}>
              取消
            </Button>
            <Button
              variant="outline"
              onClick={handleNoEmbyPreview}
              disabled={noEmbyLoading}
            >
              {noEmbyLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              预览
            </Button>
            <Button
              variant="destructive"
              onClick={handleNoEmbyConfirm}
              disabled={
                noEmbyLoading ||
                !noEmbyPreview ||
                noEmbyPreview.candidate_count === 0 ||
                noEmbyConfirmText.trim() !== "确认"
              }
            >
              {noEmbyLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认踢出
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Reset User Password Dialog（区分系统 / Emby / 两者） */}
      <Dialog
        open={resetPwdOpen}
        onOpenChange={(open) => {
          setResetPwdOpen(open);
          if (!open) {
            setResetPwdUser(null);
            setResetPwdResult(null);
            setResetPwdCustom("");
          }
        }}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>重置密码 · {resetPwdUser?.username}</DialogTitle>
            <DialogDescription>
              可单独重置系统登录密码或 Emby 播放密码；同时支持自定义新密码或随机生成。
            </DialogDescription>
          </DialogHeader>

          {resetPwdResult ? (
            <div className="space-y-3">
              <div className="rounded-lg border bg-muted/30 p-3 space-y-1.5">
                <p className="text-sm">
                  <span className="text-muted-foreground">重置范围：</span>
                  <span className="font-medium">
                    {resetPwdResult.scope === "system"
                      ? "仅系统密码"
                      : resetPwdResult.scope === "emby"
                        ? "仅 Emby 密码"
                        : "系统 + Emby"}
                  </span>
                </p>
                <p className="text-sm text-muted-foreground">
                  {resetPwdResult.auto_generated ? "随机生成" : "管理员指定"}的新密码：
                </p>
                <code className="block break-all rounded bg-background px-2 py-1.5 text-base font-mono">
                  {resetPwdResult.new_password}
                </code>
                <p className="text-xs text-muted-foreground">
                  请尽快告知用户。本对话框关闭后无法再次查看明文。
                </p>
              </div>
              <DialogFooter className="gap-2">
                <Button
                  variant="outline"
                  onClick={() => {
                    void navigator.clipboard.writeText(resetPwdResult.new_password);
                    toast({ title: "已复制到剪贴板", variant: "success" });
                  }}
                >
                  复制密码
                </Button>
                <Button onClick={() => setResetPwdOpen(false)}>完成</Button>
              </DialogFooter>
            </div>
          ) : (
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>重置范围</Label>
                <Select
                  value={resetPwdScope}
                  onValueChange={(v) => setResetPwdScope(v as typeof resetPwdScope)}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="both" disabled={!resetPwdUser?.emby_bound}>
                      系统 + Emby（同一个密码）
                    </SelectItem>
                    <SelectItem value="system">仅系统登录密码</SelectItem>
                    <SelectItem value="emby" disabled={!resetPwdUser?.emby_bound}>
                      仅 Emby 密码
                    </SelectItem>
                  </SelectContent>
                </Select>
                {!resetPwdUser?.emby_bound && (
                  <p className="text-xs text-amber-600 dark:text-amber-400">
                    该用户未绑定 Emby，只能重置系统密码。
                  </p>
                )}
              </div>

              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={resetPwdAuto}
                  onChange={(e) => setResetPwdAuto(e.target.checked)}
                  className="h-4 w-4 rounded border-gray-300"
                />
                <span>随机生成 12 位强密码</span>
              </label>

              {!resetPwdAuto && (
                <div className="space-y-2">
                  <Label>自定义新密码</Label>
                  <Input
                    type="text"
                    value={resetPwdCustom}
                    onChange={(e) => setResetPwdCustom(e.target.value)}
                    placeholder="≥ 8 位，含大小写字母 + 数字"
                    autoComplete="new-password"
                  />
                  <p className="text-[11px] text-muted-foreground">
                    至少 8 位，且包含大小写字母与数字。
                  </p>
                </div>
              )}
            </div>
          )}

          {!resetPwdResult && (
            <DialogFooter className="gap-2">
              <Button variant="outline" onClick={() => setResetPwdOpen(false)}>
                取消
              </Button>
              <Button onClick={handleResetPasswordSubmit} disabled={resetPwdLoading}>
                {resetPwdLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                重置密码
              </Button>
            </DialogFooter>
          )}
        </DialogContent>
      </Dialog>

      {/* Force-set Emby Password Dialog */}
      <Dialog
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
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>强制重置 Emby 密码</DialogTitle>
            <DialogDescription>
              通过 Emby 用户名直接重置密码。即使该 Emby 账号没有绑定本地系统账号也可执行。
            </DialogDescription>
          </DialogHeader>
          {forcePwdResult ? (
            <div className="space-y-3">
              <div className="rounded-lg border bg-muted/30 p-3 space-y-1.5">
                <p className="text-sm">
                  <span className="text-muted-foreground">Emby 用户：</span>
                  <span className="font-medium break-all">{forcePwdResult.emby_username}</span>
                </p>
                <p className="text-sm">
                  <span className="text-muted-foreground">绑定本地账号：</span>
                  <span className="font-medium">{forcePwdResult.linked_local_user ? "是" : "否"}</span>
                </p>
                <p className="text-sm">
                  <span className="text-muted-foreground">新密码：</span>
                </p>
                <code className="block break-all rounded bg-background px-2 py-1.5 text-base font-mono">
                  {forcePwdResult.new_password}
                </code>
                <p className="text-xs text-muted-foreground">请尽快将新密码告知用户。该密码仅本次显示。</p>
              </div>
              <DialogFooter>
                <Button
                  variant="outline"
                  onClick={() => {
                    void navigator.clipboard.writeText(forcePwdResult.new_password);
                    toast({ title: "已复制到剪贴板", variant: "success" });
                  }}
                >
                  复制密码
                </Button>
                <Button onClick={() => setForcePwdOpen(false)}>完成</Button>
              </DialogFooter>
            </div>
          ) : (
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>Emby 用户名</Label>
                <Input
                  placeholder="输入要重置密码的 Emby 用户名"
                  value={forcePwdEmbyName}
                  onChange={(e) => setForcePwdEmbyName(e.target.value)}
                />
              </div>
              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="forcePwdAuto"
                  checked={forcePwdAuto}
                  onChange={(e) => setForcePwdAuto(e.target.checked)}
                  className="h-4 w-4 rounded"
                />
                <Label htmlFor="forcePwdAuto" className="cursor-pointer">
                  自动生成 12 位强密码
                </Label>
              </div>
              {!forcePwdAuto && (
                <div className="space-y-2">
                  <Label>新密码</Label>
                  <Input
                    type="text"
                    placeholder="至少 8 位，含大小写字母和数字"
                    value={forcePwdNewPwd}
                    onChange={(e) => setForcePwdNewPwd(e.target.value)}
                  />
                </div>
              )}
              <DialogFooter>
                <Button variant="outline" onClick={() => setForcePwdOpen(false)}>
                  取消
                </Button>
                <Button onClick={handleForceSetEmbyPassword} disabled={forcePwdLoading}>
                  {forcePwdLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  确认重置
                </Button>
              </DialogFooter>
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* Renew Dialog */}
      <Dialog open={renewOpen} onOpenChange={(open) => {
        setRenewOpen(open);
        if (!open) setRenewPermanent(false);
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{renewMode === "cancelPermanent" ? "取消永久有效期" : "用户续期"}</DialogTitle>
            <DialogDescription>
              {renewMode === "cancelPermanent"
                ? `将用户 ${selectedUser?.username} 改为指定天数后到期`
                : `为用户 ${selectedUser?.username} 延长账号时间`}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{renewMode === "cancelPermanent" ? "改为多少天后到期" : "续期天数"}</Label>
              <Input
                type="number"
                placeholder="输入续期天数"
                value={renewDays}
                onChange={(e) => setRenewDays(e.target.value)}
                disabled={renewPermanent}
              />
            </div>
            {renewMode !== "cancelPermanent" && <label className="flex items-center gap-2 rounded-md border bg-muted/30 p-3 text-sm">
              <input
                type="checkbox"
                checked={renewPermanent}
                onChange={(e) => setRenewPermanent(e.target.checked)}
                className="h-4 w-4"
              />
              设置为永久有效
            </label>}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRenewOpen(false)}>
              取消
            </Button>
            <Button onClick={handleRenew} disabled={isActionLoading}>
              {isActionLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {renewMode === "cancelPermanent" ? "确认取消永久" : "确认续期"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 删除（含邀请树级联）对话框 */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>删除用户 {deleteTarget?.username}</DialogTitle>
            <DialogDescription>
              本地账户与（可选）Emby 账户的删除不可恢复。请同时选择是否级联删除该用户邀请的下级。
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 text-sm">
            <div className="space-y-2">
              <Label className="text-xs uppercase tracking-wider text-muted-foreground">删除范围</Label>
              <Select
                value={deleteScope}
                onValueChange={(v) => setDeleteScope(v as "with_emby" | "local_only" | "emby_only")}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {deleteTarget?.emby_id && (
                    <SelectItem value="with_emby">同时删除本地账户 + Emby 账户</SelectItem>
                  )}
                  <SelectItem value="local_only">仅删除本地账户</SelectItem>
                  {deleteTarget?.emby_id && (
                    <SelectItem value="emby_only">仅删除 Emby 账户（不动本地）</SelectItem>
                  )}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2 rounded-md border bg-muted/30 p-3">
              <div className="flex items-center justify-between gap-2">
                <Label className="text-xs uppercase tracking-wider text-muted-foreground">
                  邀请树级联深度
                </Label>
                <span className="text-[10px] text-muted-foreground">
                  当前：{cascadeDepth === 0 ? "整棵子树" : `${cascadeDepth} 层`}
                </span>
              </div>

              {/* 预设快捷键 */}
              <div className="flex flex-wrap gap-1.5">
                {[1, 2, 3, 5, 0].map((preset) => (
                  <Button
                    key={preset}
                    type="button"
                    size="sm"
                    variant={cascadeDepth === preset ? "default" : "outline"}
                    className="h-7 px-2 text-[11px]"
                    onClick={() => setCascadeDepth(preset)}
                  >
                    {preset === 0 ? "全部" : `${preset} 层`}
                  </Button>
                ))}
              </div>

              {/* 自定义输入 */}
              <div className="flex items-center gap-2">
                <Input
                  type="number"
                  inputMode="numeric"
                  min={0}
                  max={999}
                  value={cascadeDepth}
                  onChange={(e) => {
                    const v = parseInt(e.target.value, 10);
                    if (Number.isNaN(v)) {
                      setCascadeDepth(1);
                    } else {
                      setCascadeDepth(Math.max(0, Math.min(999, v)));
                    }
                  }}
                  placeholder="自定义层级，输入 0 表示整棵子树"
                  className="h-9"
                />
              </div>

              <p className="text-[11px] text-muted-foreground leading-relaxed">
                {cascadeDepth === 1 ? (
                  <>仅处理该用户本人；被他邀请的下级会自动成为新树根，互相之间不再有上下级关系。</>
                ) : cascadeDepth === 0 ? (
                  <>
                    将沿邀请关系，一路处理该用户及其
                    <span className="font-semibold text-foreground"> 全部后代</span>
                    （
                    {deleteScope === "with_emby"
                      ? "本地账号 + Emby 账号"
                      : deleteScope === "local_only"
                        ? "仅本地账号，Emby 保留"
                        : "仅 Emby 账号，本地账号与邀请关系保留"}
                    ）。请二次确认！
                  </>
                ) : (
                  <>
                    将一并处理该用户向下 {cascadeDepth - 1} 层的所有下级（
                    {deleteScope === "with_emby"
                      ? "本地 + Emby"
                      : deleteScope === "local_only"
                        ? "仅本地"
                        : "仅 Emby"}
                    ）。
                  </>
                )}
              </p>
              {deleteScope === "emby_only" && cascadeDepth !== 1 && (
                <p className="text-[11px] text-amber-600 dark:text-amber-400">
                  注意：「仅删除 Emby」级联只会删除下级用户的 Emby 账号，本地账号与邀请关系完全保留。
                </p>
              )}
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteOpen(false)} disabled={isDeleting}>
              取消
            </Button>
            <Button variant="destructive" onClick={confirmDelete} disabled={isDeleting}>
              {isDeleting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 启停（支持邀请树级联） */}
      <Dialog open={toggleOpen} onOpenChange={setToggleOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>
              {toggleTarget?.active ? "禁用" : "启用"}用户 {toggleTarget?.username}
            </DialogTitle>
            <DialogDescription>
              {toggleTarget?.active
                ? "禁用后用户将无法登录系统与 Emby，但邀请树结构（上下级关系、已发出邀请码）完全不变；重新启用即可恢复访问。"
                : "启用后用户可以重新登录系统与 Emby。"}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3 text-sm">
            {toggleTarget?.active && (
              <div className="space-y-1.5">
                <Label className="text-xs uppercase tracking-wider text-muted-foreground">
                  原因（可选）
                </Label>
                <Input
                  value={toggleReason}
                  onChange={(e) => setToggleReason(e.target.value)}
                  placeholder="将记入操作日志，留空也可"
                  maxLength={200}
                />
              </div>
            )}

            <div className="space-y-2 rounded-md border bg-muted/30 p-3">
              <div className="flex items-center justify-between gap-2">
                <Label className="text-xs uppercase tracking-wider text-muted-foreground">
                  邀请树级联深度
                </Label>
                <span className="text-[10px] text-muted-foreground">
                  当前：{toggleCascadeDepth === 0 ? "整棵子树" : `${toggleCascadeDepth} 层`}
                </span>
              </div>

              <div className="flex flex-wrap gap-1.5">
                {[1, 2, 3, 5, 0].map((preset) => (
                  <Button
                    key={preset}
                    type="button"
                    size="sm"
                    variant={toggleCascadeDepth === preset ? "default" : "outline"}
                    className="h-7 px-2 text-[11px]"
                    onClick={() => setToggleCascadeDepth(preset)}
                  >
                    {preset === 0 ? "全部" : `${preset} 层`}
                  </Button>
                ))}
              </div>

              <Input
                type="number"
                inputMode="numeric"
                min={0}
                max={999}
                value={toggleCascadeDepth}
                onChange={(e) => {
                  const v = parseInt(e.target.value, 10);
                  if (Number.isNaN(v)) {
                    setToggleCascadeDepth(1);
                  } else {
                    setToggleCascadeDepth(Math.max(0, Math.min(999, v)));
                  }
                }}
                placeholder="自定义层级，输入 0 表示整棵子树"
                className="h-9"
              />

              <p className="text-[11px] text-muted-foreground leading-relaxed">
                {toggleCascadeDepth === 1 ? (
                  <>仅处理该用户本人；下级账号不受影响。</>
                ) : toggleCascadeDepth === 0 ? (
                  <>
                    将
                    <span className="font-semibold text-foreground">
                      {toggleTarget?.active ? "禁用" : "启用"}
                    </span>
                    该用户及其全部后代（沿邀请关系递归）。已经处于目标状态的会被跳过。
                  </>
                ) : (
                  <>
                    将{toggleTarget?.active ? "禁用" : "启用"}该用户及向下 {toggleCascadeDepth - 1} 层的所有下级；
                    其他管理员账号会被跳过。
                  </>
                )}
              </p>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setToggleOpen(false)} disabled={isToggling}>
              取消
            </Button>
            <Button
              variant={toggleTarget?.active ? "destructive" : "default"}
              onClick={confirmToggleActive}
              disabled={isToggling}
            >
              {isToggling && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {toggleTarget?.active ? "确认禁用" : "确认启用"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 绑定 Emby 对话框 */}
      <Dialog
        open={bindOpen}
        onOpenChange={(open) => {
          setBindOpen(open);
          if (!open) setBindConflict(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Link2 className="h-5 w-5" />
              绑定 Emby 账号
            </DialogTitle>
            <DialogDescription>
              将一个已存在的 Emby 账号强制绑定到系统账号 {bindTarget?.username}（UID {bindTarget?.uid}）。
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3 py-2">
            <div className="space-y-2">
              <Label htmlFor="bind-emby-name">Emby 用户名</Label>
              <Input
                id="bind-emby-name"
                value={bindEmbyName}
                onChange={(e) => {
                  setBindEmbyName(e.target.value);
                  setBindConflict(null);
                }}
                placeholder="输入要绑定的 Emby 用户名"
                disabled={bindSubmitting}
              />
              <p className="text-xs text-muted-foreground">
                若该 Emby 已被其他系统账号占用，将出现确认按钮以“夺取”绑定。
              </p>
            </div>

            {bindConflict && (
              <div className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm">
                <div className="mb-1 flex items-center gap-2 text-amber-600 dark:text-amber-400">
                  <AlertTriangle className="h-4 w-4" />
                  <span className="font-medium">绑定冲突</span>
                </div>
                <p>
                  Emby 用户 <span className="font-mono">{bindConflict.emby_username}</span> 当前已绑定到
                  系统账号 <span className="font-mono">{bindConflict.conflict_username}</span>
                  （UID {bindConflict.conflict_uid}）。
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  确认“夺取”将清空旧账号的 EMBYID，旧账号会被标记为“待重新绑定 Emby”状态。
                </p>
              </div>
            )}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setBindOpen(false)} disabled={bindSubmitting}>
              取消
            </Button>
            {bindConflict ? (
              <Button
                variant="destructive"
                onClick={() => void submitBindEmby(true)}
                disabled={bindSubmitting}
              >
                {bindSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                确认夺取并绑定
              </Button>
            ) : (
              <Button onClick={() => void submitBindEmby(false)} disabled={bindSubmitting}>
                {bindSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                绑定
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 一键踢出 TG 群里未绑账号的成员 */}
      <Dialog open={kickOpen} onOpenChange={(v) => { if (!kickLoading) setKickOpen(v); }}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Send className="h-5 w-5" />
              一键踢出未绑 Web 账号的 TG 成员
            </DialogTitle>
            <DialogDescription>
              Bot API 没法主动枚举群成员，本功能依赖 Bot 长期被动累积的“花名册”
              （chat_member 事件 + 群消息观察）。Bot 必须在群里是有“封禁成员”权限的管理员。
              群管理员、Bot、配置中的 ADMIN_ID 与所有持有 Web 账号的人都会被自动排除；
              踢出策略是 ban + 立即 unban（临时移除，仍可重新加入）。
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3 text-sm">
            {kickRoster && (
              <div className="rounded-md border bg-muted/30 p-3 space-y-1 text-xs">
                <p className="font-medium">花名册概况（chat_id: {kickRoster.chat_id || "—"}）</p>
                <p>活跃: {kickRoster.active ?? 0}　已离群: {kickRoster.inactive ?? 0}　Bot: {kickRoster.bots ?? 0}</p>
                {kickRoster.first_seen_at && (
                  <p className="text-muted-foreground">
                    最早观察：{formatDate(kickRoster.first_seen_at)}
                    　最新观察：{formatDate(kickRoster.last_seen_at || kickRoster.first_seen_at)}
                  </p>
                )}
              </div>
            )}

            {kickLoading && !kickPreview ? (
              <div className="flex items-center justify-center py-6">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                <span className="ml-2 text-xs text-muted-foreground">正在统计候选人...</span>
              </div>
            ) : kickPreview ? (
              <div className="rounded-md border p-3 space-y-1 text-xs">
                <p>花名册成员: <strong>{kickPreview.roster_size}</strong>（含 bot {kickPreview.bots_in_roster}）</p>
                <p>系统内活跃且已绑 Emby: <strong>{kickPreview.preserved_bound}</strong></p>
                <p>群管理员排除: <strong>{kickPreview.admins_excluded}</strong></p>
                <p>排除总数: <strong>{kickPreview.excluded_total}</strong></p>
                <p className="text-destructive">
                  实际待踢: <strong>{kickPreview.targets}</strong>
                  <span className="ml-2 text-muted-foreground font-normal">
                    （无账号 {kickPreview.reason_no_account} · 无 Emby {kickPreview.reason_no_emby} · 已禁用 {kickPreview.reason_disabled}）
                  </span>
                </p>
                {kickPreview.preview_targets && kickPreview.preview_targets.length > 0 && (
                  <div className="pt-1">
                    <p className="text-muted-foreground">前 {kickPreview.preview_targets.length} 个目标：</p>
                    <p className="break-all text-[10px] text-muted-foreground">
                      {kickPreview.preview_targets
                        .map((t) => `${t.tg_id}(${t.reason})`)
                        .join(", ")}
                    </p>
                  </div>
                )}
              </div>
            ) : null}

            {kickResult ? (
              <div className="rounded-md border border-emerald-500/40 bg-emerald-500/5 p-3 space-y-1 text-xs">
                <p className="font-medium text-emerald-600 dark:text-emerald-400">执行结果</p>
                <p>已踢出: {kickResult.kicked}　跳过: {kickResult.skipped}</p>
                <p>已不在群: {kickResult.not_in_group}　失败: {kickResult.failed}</p>
              </div>
            ) : (
              <div className="space-y-1.5 rounded-md border border-destructive/40 bg-destructive/5 p-3">
                <Label className="text-xs uppercase tracking-wider text-destructive">二次确认</Label>
                <p className="text-xs text-muted-foreground">
                  请在下方输入 <span className="font-mono text-foreground">确认</span> 二字以继续执行：
                </p>
                <Input
                  value={kickConfirmText}
                  onChange={(e) => setKickConfirmText(e.target.value)}
                  placeholder="确认"
                  className="h-9"
                  disabled={kickLoading}
                />
              </div>
            )}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setKickOpen(false)} disabled={kickLoading}>
              关闭
            </Button>
            {!kickResult && (
              <Button
                variant="destructive"
                onClick={handleKickConfirm}
                disabled={
                  kickLoading ||
                  kickConfirmText.trim() !== "确认" ||
                  !kickPreview ||
                  (kickPreview && kickPreview.targets === 0)
                }
              >
                {kickLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                确认踢出 {kickPreview ? kickPreview.targets : 0} 人
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 一键批量到期调控 */}
      <Dialog open={bulkExpireOpen} onOpenChange={setBulkExpireOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <CalendarClock className="h-5 w-5" />
              一键批量调控到期时间
            </DialogTitle>
            <DialogDescription>
              将根据上方筛选条件（角色/状态/Emby 绑定）批量覆盖普通用户的到期时间。
              管理员、白名单与未开通 Emby 的账号默认会被跳过，避免误伤。
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 text-sm">
            <div className="space-y-2">
              <Label className="text-xs uppercase tracking-wider text-muted-foreground">目标到期时间</Label>
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  size="sm"
                  variant={bulkExpireMode === "permanent" ? "default" : "outline"}
                  onClick={() => setBulkExpireMode("permanent")}
                >
                  永久
                </Button>
                <div className="flex items-center gap-1.5">
                  <Button
                    type="button"
                    size="sm"
                    variant={bulkExpireMode === "days" ? "default" : "outline"}
                    onClick={() => setBulkExpireMode("days")}
                  >
                    自定义天数
                  </Button>
                  <Input
                    type="number"
                    min={1}
                    value={bulkExpireDays}
                    onChange={(e) => {
                      setBulkExpireDays(e.target.value);
                      setBulkExpireMode("days");
                    }}
                    placeholder="天数"
                    className="h-8 w-24"
                  />
                  <span className="text-xs text-muted-foreground">天</span>
                </div>
              </div>
              <p className="text-[11px] text-muted-foreground">
                {bulkExpireMode === "permanent"
                  ? "将设置为永久（EXPIRED_AT=-1）。"
                  : "从当前时间起 N 天后过期。"}
              </p>
            </div>

            <div className="rounded-md border bg-muted/30 p-3 space-y-2">
              <p className="text-xs font-medium">作用范围（当前筛选条件）</p>
              <div className="text-xs text-muted-foreground space-y-0.5">
                <p>角色：{roleFilter === "all" ? "全部" : roleFilter === "0" ? "管理员" : roleFilter === "1" ? "普通用户" : "白名单"}</p>
                <p>启用状态：{activeFilter === "all" ? "全部" : activeFilter === "true" ? "仅已启用" : "仅已禁用"}</p>
                <p>Emby 绑定：{embyFilter === "all" ? "全部" : embyFilter === "bound" ? "已绑定" : "未绑定"}</p>
              </div>
            </div>

            <div className="space-y-2">
              <Label className="text-xs uppercase tracking-wider text-muted-foreground">额外选项</Label>
              <div className="space-y-1.5">
                <label className="flex items-start gap-2 text-xs cursor-pointer">
                  <input
                    type="checkbox"
                    checked={bulkExpireIncludeAdmin}
                    onChange={(e) => setBulkExpireIncludeAdmin(e.target.checked)}
                    className="mt-0.5 h-4 w-4 rounded"
                  />
                  <span>
                    包含管理员账号（默认跳过；包含后<strong>仍不会改你自己</strong>，除非把“永久”选项手动改成具体天数前请慎重）
                  </span>
                </label>
                <label className="flex items-start gap-2 text-xs cursor-pointer">
                  <input
                    type="checkbox"
                    checked={bulkExpireIncludeWhitelist}
                    onChange={(e) => setBulkExpireIncludeWhitelist(e.target.checked)}
                    className="mt-0.5 h-4 w-4 rounded"
                  />
                  <span>包含白名单用户（一旦取消勾选，白名单的“永久”标签将被覆盖）</span>
                </label>
                <p className="rounded-md border border-muted-foreground/20 bg-muted/40 px-3 py-2 text-[11px] text-muted-foreground">
                  ⚠ 未绑定 Emby 的账号一律强制跳过，「未开通」sentinel（EXPIRED_AT=0）由系统保护，无法通过批量操作覆盖。
                </p>
              </div>
            </div>

            <div className="space-y-1.5 rounded-md border border-destructive/40 bg-destructive/5 p-3">
              <Label className="text-xs uppercase tracking-wider text-destructive">
                二次确认
              </Label>
              <p className="text-xs text-muted-foreground">
                请在下方输入 <span className="font-mono text-foreground">确认</span> 二字以继续：
              </p>
              <Input
                value={bulkExpireConfirmText}
                onChange={(e) => setBulkExpireConfirmText(e.target.value)}
                placeholder="确认"
                className="h-9"
              />
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setBulkExpireOpen(false)} disabled={bulkExpireLoading}>
              取消
            </Button>
            <Button
              variant="destructive"
              onClick={handleBulkExpire}
              disabled={bulkExpireLoading || bulkExpireConfirmText.trim() !== "确认"}
            >
              {bulkExpireLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              执行批量调控
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 批量启用已禁用账号 */}
      <Dialog open={bulkEnableOpen} onOpenChange={setBulkEnableOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <UserCheck className="h-5 w-5" />
              批量启用已禁用账号
            </DialogTitle>
            <DialogDescription>
              将按当前搜索、角色、状态与 Emby 筛选查找已禁用账号，并同步启用对应 Emby 账户。
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 text-sm">
            <div className="rounded-md border bg-muted/30 p-3 space-y-2">
              <p className="text-xs font-medium">作用范围（当前筛选条件）</p>
              <div className="text-xs text-muted-foreground space-y-0.5">
                <p>搜索：{search.trim() || "无"}</p>
                <p>角色：{roleFilter === "all" ? "全部" : roleFilter === "0" ? "管理员" : roleFilter === "1" ? "普通用户" : "白名单"}</p>
                <p>启用状态：{activeFilter === "all" ? "全部（实际只处理已禁用）" : activeFilter === "true" ? "仅已启用（不会匹配禁用账号）" : "仅已禁用"}</p>
                <p>Emby 绑定：{embyFilter === "all" ? "全部" : embyFilter === "bound" ? "已绑定" : "未绑定"}</p>
              </div>
            </div>

            <div className="space-y-2">
              <Label className="text-xs uppercase tracking-wider text-muted-foreground">额外选项</Label>
              <div className="space-y-1.5">
                <label className="flex items-start gap-2 text-xs cursor-pointer">
                  <input
                    type="checkbox"
                    checked={bulkEnableIncludeAdmin}
                    onChange={(e) => setBulkEnableIncludeAdmin(e.target.checked)}
                    className="mt-0.5 h-4 w-4 rounded"
                  />
                  <span>包含管理员账号（默认跳过，避免误恢复管理员访问权限）</span>
                </label>
                <label className="flex items-start gap-2 text-xs cursor-pointer">
                  <input
                    type="checkbox"
                    checked={bulkEnableIncludeWhitelist}
                    onChange={(e) => setBulkEnableIncludeWhitelist(e.target.checked)}
                    className="mt-0.5 h-4 w-4 rounded"
                  />
                  <span>包含白名单用户（默认跳过）</span>
                </label>
                <p className="rounded-md border border-muted-foreground/20 bg-muted/40 px-3 py-2 text-[11px] text-muted-foreground">
                  未识别角色会被后端强制跳过；单次最多启用 5000 个账号。
                </p>
              </div>
            </div>

            <div className="space-y-1.5 rounded-md border border-amber-500/40 bg-amber-500/5 p-3">
              <Label className="text-xs uppercase tracking-wider text-amber-700 dark:text-amber-300">
                二次确认
              </Label>
              <p className="text-xs text-muted-foreground">
                请在下方输入 <span className="font-mono text-foreground">确认</span> 二字以继续：
              </p>
              <Input
                value={bulkEnableConfirmText}
                onChange={(e) => setBulkEnableConfirmText(e.target.value)}
                placeholder="确认"
                className="h-9"
              />
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setBulkEnableOpen(false)} disabled={bulkEnableLoading}>
              取消
            </Button>
            <Button
              onClick={handleBulkEnableDisabled}
              disabled={bulkEnableLoading || bulkEnableConfirmText.trim() !== "确认" || bulkEnableBlocked}
            >
              {bulkEnableLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              执行批量启用
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 新建独立 Emby 账号 */}
      <Dialog open={standaloneOpen} onOpenChange={setStandaloneOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <UserPlus className="h-5 w-5" />
              新建独立 Emby 账号
            </DialogTitle>
            <DialogDescription>
              直接调用 Emby API 创建一个新账号；该账号不会写入本地用户表、不参与 Twilight 系统的过期/权限管理。
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <Label htmlFor="standalone-name">Emby 用户名</Label>
              <Input
                id="standalone-name"
                value={standaloneName}
                onChange={(e) => setStandaloneName(e.target.value)}
                placeholder="如 guest123"
                disabled={standaloneSubmitting}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="standalone-pwd">Emby 密码</Label>
              <Input
                id="standalone-pwd"
                type="password"
                value={standalonePwd}
                onChange={(e) => setStandalonePwd(e.target.value)}
                placeholder="至少 8 位，含大小写和数字"
                disabled={standaloneSubmitting}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="standalone-email">邮箱（可选）</Label>
              <Input
                id="standalone-email"
                value={standaloneEmail}
                onChange={(e) => setStandaloneEmail(e.target.value)}
                placeholder="仅做备注，不会同步到 Emby"
                disabled={standaloneSubmitting}
              />
            </div>
          </div>

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setStandaloneOpen(false)}
              disabled={standaloneSubmitting}
            >
              取消
            </Button>
            <Button onClick={handleCreateStandaloneEmby} disabled={standaloneSubmitting}>
              {standaloneSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              创建
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
