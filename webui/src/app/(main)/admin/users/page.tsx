"use client";

import { useEffect, useState } from "react";
import { motion } from "framer-motion";
import {
  Users,
  Search,
  UserPlus,
  MoreHorizontal,
  RefreshCw,
  Ban,
  Trash2,
  Key,
  Clock,
  Loader2,
  ChevronLeft,
  ChevronRight,
  Edit,
  Coins,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
import { Switch } from "@/components/ui/switch";
import { Separator } from "@/components/ui/separator";
import { useToast } from "@/hooks/use-toast";
import { api, type UserInfo } from "@/lib/api";
import { formatDate } from "@/lib/utils";

export default function AdminUsersPage() {
  const { toast } = useToast();
  const [users, setUsers] = useState<UserInfo[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pages, setPages] = useState(1);
  const [search, setSearch] = useState("");
  const [isLoading, setIsLoading] = useState(true);

  // Dialog states
  const [renewOpen, setRenewOpen] = useState(false);
  const [renewDays, setRenewDays] = useState("30");
  const [selectedUser, setSelectedUser] = useState<UserInfo | null>(null);
  const [isActionLoading, setIsActionLoading] = useState(false);
  
  // Edit dialog states
  const [editOpen, setEditOpen] = useState(false);
  const [editForm, setEditForm] = useState({
    role: 1,
    score: 0,
    emby_id: "",
    active: true,
  });
  const [userNsfwInfo, setUserNsfwInfo] = useState<{
    enabled: boolean;
    has_permission: boolean;
  } | null>(null);

  useEffect(() => {
    loadUsers();
  }, [page]);

  const loadUsers = async () => {
    setIsLoading(true);
    try {
      const res = await api.getUsers({
        page,
        per_page: 20,
        search: search || undefined,
      });
      if (res.success && res.data) {
        setUsers(res.data.users);
        setTotal(res.data.total);
        setPages(res.data.pages);
      }
    } catch (error) {
      console.error(error);
    } finally {
      setIsLoading(false);
    }
  };

  const handleSearch = () => {
    setPage(1);
    loadUsers();
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

  const handleToggleActive = async (user: UserInfo) => {
    try {
      const res = await api.updateUser(user.uid, { active: !user.active });
      if (res.success) {
        toast({ title: user.active ? "已禁用" : "已启用", variant: "success" });
        loadUsers();
      } else {
        toast({ title: "操作失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "操作失败", description: error.message, variant: "destructive" });
    }
  };

  const handleDelete = async (user: UserInfo) => {
    if (!confirm(`确定要删除用户 ${user.username} 吗？此操作不可恢复。`)) {
      return;
    }

    try {
      const res = await api.deleteUser(user.uid);
      if (res.success) {
        toast({ title: "用户已删除", variant: "success" });
        loadUsers();
      } else {
        toast({ title: "删除失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "删除失败", description: error.message, variant: "destructive" });
    }
  };

  const handleOpenEdit = async (user: UserInfo) => {
    setSelectedUser(user);
    setEditForm({
      role: user.role,
      score: user.score || 0,
      emby_id: user.emby_id || "",
      active: user.active,
    });
    
    // 获取用户的 NSFW 权限信息
    try {
      const res = await api.getUser(user.uid);
      if (res.success && res.data) {
        setUserNsfwInfo({
          enabled: res.data.nsfw?.enabled || false,
          has_permission: res.data.nsfw?.has_permission || false,
        });
      }
    } catch (error) {
      console.error("获取用户NSFW信息失败:", error);
      setUserNsfwInfo(null);
    }
    
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
        setUserNsfwInfo(null);
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

  const handleToggleNsfwPermission = async (grant: boolean) => {
    if (!selectedUser) return;

    setIsActionLoading(true);
    try {
      const res = await api.setUserNsfwPermission(selectedUser.uid, grant);
      if (res.success) {
        toast({
          title: grant ? "已授予 NSFW 权限" : "已撤销 NSFW 权限",
          variant: "success",
        });
        // 更新本地状态
        if (userNsfwInfo) {
          setUserNsfwInfo({
            ...userNsfwInfo,
            has_permission: grant,
            enabled: grant ? userNsfwInfo.enabled : false, // 撤销权限时关闭显示
          });
        }
        loadUsers();
      } else {
        toast({ title: "操作失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "操作失败", description: error.message, variant: "destructive" });
    } finally {
      setIsActionLoading(false);
    }
  };

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

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">用户管理</h1>
          <p className="text-muted-foreground">管理所有注册用户</p>
        </div>
        <Badge variant="outline" className="text-lg px-4 py-2">
          共 {total} 用户
        </Badge>
      </div>

      {/* Search */}
      <Card>
        <CardContent className="pt-6">
          <div className="flex gap-4">
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
            <Button onClick={handleSearch}>
              <Search className="mr-2 h-4 w-4" />
              搜索
            </Button>
          </div>
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
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left text-sm font-medium">用户</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">角色</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">状态</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">到期时间</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">积分</th>
                    <th className="px-4 py-3 text-right text-sm font-medium">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((user) => (
                    <tr key={user.uid} className="border-b hover:bg-muted/30">
                      <td className="px-4 py-3">
                        <div>
                          <p className="font-medium">{user.username}</p>
                          <p className="text-xs text-muted-foreground">
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
                      </td>
                      <td className="px-4 py-3">{getRoleBadge(user.role)}</td>
                      <td className="px-4 py-3">
                        <Badge variant={user.active ? "success" : "destructive"}>
                          {user.active ? "正常" : "禁用"}
                        </Badge>
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
                      <td className="px-4 py-3 font-medium">{user.score || 0}</td>
                      <td className="px-4 py-3 text-right">
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
                            <DropdownMenuSeparator />
                            <DropdownMenuItem onClick={() => handleToggleActive(user)}>
                              <Ban className="mr-2 h-4 w-4" />
                              {user.active ? "禁用" : "启用"}
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              className="text-destructive"
                              onClick={() => handleDelete(user)}
                            >
                              <Trash2 className="mr-2 h-4 w-4" />
                              删除
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
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
              <Label>积分</Label>
              <Input
                type="number"
                placeholder="输入积分"
                value={editForm.score}
                onChange={(e) => setEditForm({ ...editForm, score: parseInt(e.target.value) || 0 })}
              />
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

            {selectedUser?.emby_id && (
              <>
                <Separator />
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <div className="space-y-0.5">
                      <Label>NSFW 库访问权限</Label>
                      <p className="text-xs text-muted-foreground">
                        控制用户是否可以访问 NSFW 媒体库
                      </p>
                    </div>
                    <Switch
                      checked={userNsfwInfo?.has_permission || false}
                      onCheckedChange={handleToggleNsfwPermission}
                      disabled={isActionLoading || !selectedUser?.emby_id}
                    />
                  </div>
                  {userNsfwInfo && (
                    <div className="rounded-lg bg-accent/50 p-3 text-xs">
                      <p className="text-muted-foreground">
                        权限状态:{" "}
                        <span className="font-medium">
                          {userNsfwInfo.has_permission ? "有权限" : "无权限"}
                        </span>
                      </p>
                      <p className="text-muted-foreground mt-1">
                        显示状态:{" "}
                        <span className="font-medium">
                          {userNsfwInfo.enabled ? "已启用" : "已禁用"}
                        </span>
                      </p>
                      <p className="text-muted-foreground mt-1 text-[10px]">
                        提示: 权限控制访问，显示状态由用户自己在设置中控制
                      </p>
                    </div>
                  )}
                </div>
              </>
            )}
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

      {/* Renew Dialog */}
      <Dialog open={renewOpen} onOpenChange={setRenewOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>用户续期</DialogTitle>
            <DialogDescription>
              为用户 {selectedUser?.username} 延长会员时间
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
    </div>
  );
}

