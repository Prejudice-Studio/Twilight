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
import { api, type UserInfo } from "@/lib/api";
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
  const [selectedUser, setSelectedUser] = useState<UserInfo | null>(null);
  const [isActionLoading, setIsActionLoading] = useState(false);
  
  // Cleanup dialog states
  const [cleanupOpen, setCleanupOpen] = useState(false);
  const [cleanupMinDays, setCleanupMinDays] = useState("7");
  const [cleanupPreview, setCleanupPreview] = useState<any[] | null>(null);
  const [cleanupLoading, setCleanupLoading] = useState(false);

  // Edit dialog states
  const [editOpen, setEditOpen] = useState(false);
  const [editForm, setEditForm] = useState({
    role: 1,
    emby_id: "",
    active: true,
  });

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
    setPage(1);
    invalidateUsersCache();
    setExpandedUserIds(new Set());
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
    if (!selectedUser || !renewDays) return;

    setIsActionLoading(true);
    try {
      const res = await api.renewUser(selectedUser.uid, parseInt(renewDays));
      if (res.success) {
        toast({ title: "续期成功", variant: "success" });
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

  const handleResetPassword = async (user: UserInfo) => {
    try {
      const res = await api.resetPassword(user.uid);
      if (res.success && res.data) {
        toast({
          title: "密码已重置",
          description: `新密码: ${res.data.new_password}`,
        });
      } else {
        toast({ title: "重置失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "重置失败", description: error.message, variant: "destructive" });
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
            setRenewOpen(true);
          }}
        >
          <RefreshCw className="mr-2 h-4 w-4" />
          续期
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handleResetPassword(user)}>
          <Key className="mr-2 h-4 w-4" />
          重置密码
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handleOpenBindEmby(user)}>
          <Link2 className="mr-2 h-4 w-4" />
          绑定 Emby
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

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl sm:text-3xl font-bold">用户管理</h1>
          <p className="text-sm text-muted-foreground">管理所有注册用户</p>
        </div>
        <div className="flex flex-wrap items-center gap-2 sm:gap-3">
          <Button
            variant="outline"
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
          </Button>
          <Button
            variant="outline"
            onClick={() => {
              setStandaloneName("");
              setStandalonePwd("");
              setStandaloneEmail("");
              setStandaloneOpen(true);
            }}
          >
            <UserPlus className="mr-2 h-4 w-4" />
            新建独立 Emby 账号
          </Button>
          <Button
            variant="outline"
            className="text-destructive hover:text-destructive"
            onClick={() => {
              setCleanupPreview(null);
              setCleanupMinDays("7");
              setCleanupOpen(true);
            }}
          >
            <UserX className="mr-2 h-4 w-4" />
            清理无效用户
          </Button>
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
              <Button onClick={handleSearch} className="flex-1 md:flex-none">
                <Search className="mr-2 h-4 w-4" />
                搜索
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
                      <p className={user.expired_at && new Date(user.expired_at) < new Date() ? "mt-1 text-destructive" : "mt-1"}>
                        {user.expired_at ? formatDate(user.expired_at) : "永久"}
                      </p>
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
                      <td className="px-4 py-3 text-sm">
                        {user.expired_at ? (
                          <span className={new Date(user.expired_at) < new Date() ? "text-destructive" : ""}>
                            {formatDate(user.expired_at)}
                          </span>
                        ) : (
                          <span className="text-emerald-500">永久</span>
                        )}
                      </td>
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
                            </div>
                            <div>
                              <p className="font-medium">账号详情</p>
                              <p>Emby ID: {user.emby_id || "未绑定"}</p>
                              <p>BGM 模式: {user.bgm_mode ? "已开启" : "未开启"}</p>
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
      <Dialog open={renewOpen} onOpenChange={setRenewOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>用户续期</DialogTitle>
            <DialogDescription>
              为用户 {selectedUser?.username} 延长账号时间
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>续期天数</Label>
              <Input
                type="number"
                placeholder="输入续期天数"
                value={renewDays}
                onChange={(e) => setRenewDays(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRenewOpen(false)}>
              取消
            </Button>
            <Button onClick={handleRenew} disabled={isActionLoading}>
              {isActionLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认续期
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
                若该 Emby 已被其他系统账号占用，将出现确认按钮以"夺取"绑定。
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
                  确认"夺取"将清空旧账号的 EMBYID，旧账号会被标记为"待重新绑定 Emby"状态。
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

