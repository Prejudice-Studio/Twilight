"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { motion } from "framer-motion";
import {
  Server,
  Loader2,
  CheckCircle2,
  XCircle,
  Users,
  RefreshCw,
  Trash2,
  Download,
  AlertTriangle,
  Link2,
  Link2Off,
  Shield,
  Wifi,
  WifiOff,
  Send,
  MessageCircle,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

const container = {
  hidden: { opacity: 0 },
  show: { opacity: 1, transition: { staggerChildren: 0.1 } },
};

const item = {
  hidden: { opacity: 0, y: 20 },
  show: { opacity: 1, y: 0 },
};

interface TestResult {
  name: string;
  success: boolean;
  latency_ms?: number;
  message: string;
}

interface ConnectivityResult {
  emby_url: string;
  tests: TestResult[];
  overall: boolean;
  server_info?: { name: string; version: string; os: string; id: string };
}

interface EmbyUserItem {
  emby_id: string;
  emby_name: string;
  has_password: boolean;
  is_admin: boolean;
  is_disabled: boolean;
  is_hidden: boolean;
  last_login: string | null;
  last_activity: string | null;
  local_user: {
    uid: number;
    username: string;
    telegram_id: number | null;
    active: boolean;
    role: number;
  } | null;
  sync_status: "synced" | "name_mismatch" | "unlinked";
}

interface OrphanItem {
  uid: number;
  username: string;
  emby_id: string;
  telegram_id: number | null;
}

interface EmbyUsersData {
  emby_users: EmbyUserItem[];
  orphans: OrphanItem[];
  total_emby: number;
  total_linked: number;
  total_orphans: number;
}

export default function AdminEmbyPage() {
  const { toast } = useToast();

  // Connectivity test state
  const [testResult, setTestResult] = useState<ConnectivityResult | null>(null);
  const [isTesting, setIsTesting] = useState(false);

  // Emby users state
  const [embyData, setEmbyData] = useState<EmbyUsersData | null>(null);
  const [isLoadingUsers, setIsLoadingUsers] = useState(false);

  // Emby 用户表筛选：name 关键字 + 关联状态 + 属性
  const [userSearch, setUserSearch] = useState("");
  const [linkFilter, setLinkFilter] = useState<"all" | "linked" | "unlinked" | "name_mismatch">("all");
  const [attrFilter, setAttrFilter] = useState<"all" | "admin" | "disabled" | "hidden">("all");

  const filteredEmbyUsers = useMemo<EmbyUserItem[]>(() => {
    if (!embyData) return [];
    const q = userSearch.trim().toLowerCase();
    return embyData.emby_users.filter((eu) => {
      // 关键字匹配 emby_name / emby_id / 本地 username / 本地 UID
      if (q) {
        const haystacks = [
          eu.emby_name,
          eu.emby_id,
          eu.local_user?.username || "",
          eu.local_user ? String(eu.local_user.uid) : "",
          eu.local_user?.telegram_id ? String(eu.local_user.telegram_id) : "",
        ];
        if (!haystacks.some((h) => h.toLowerCase().includes(q))) return false;
      }

      // 关联状态
      if (linkFilter === "linked" && !eu.local_user) return false;
      if (linkFilter === "unlinked" && eu.local_user) return false;
      if (linkFilter === "name_mismatch" && eu.sync_status !== "name_mismatch") return false;

      // 属性
      if (attrFilter === "admin" && !eu.is_admin) return false;
      if (attrFilter === "disabled" && !eu.is_disabled) return false;
      if (attrFilter === "hidden" && !eu.is_hidden) return false;

      return true;
    });
  }, [embyData, userSearch, linkFilter, attrFilter]);

  // Action loading states
  const [isImporting, setIsImporting] = useState(false);
  const [isCleaning, setIsCleaning] = useState(false);
  const [isResetting, setIsResetting] = useState(false);
  const [isDeletingUnlinked, setIsDeletingUnlinked] = useState(false);
  const [isSyncing, setIsSyncing] = useState(false);

  // Confirm dialog
  const [resetDialogOpen, setResetDialogOpen] = useState(false);

  // Bot test state
  const [isBotTesting, setIsBotTesting] = useState(false);
  const [botResults, setBotResults] = useState<Array<{ target: string; success: boolean; error: string | null; username?: string; bot_id?: number; title?: string; bot_status?: string }> | null>(null);
  const [botRuntime, setBotRuntime] = useState<{ polling?: boolean; last_ok_at?: number | null; last_error_at?: number | null; last_error?: string } | null>(null);

  // Bot connectivity test
  const handleTestBot = useCallback(async () => {
    setIsBotTesting(true);
    setBotResults(null);
    setBotRuntime(null);
    try {
      const res = await api.testBotConnectivity();
      if (res.success && res.data) {
        setBotResults(res.data.results);
        setBotRuntime(res.data.runtime || null);
        const allOk = res.data.results.every((r) => r.success);
        toast({
          title: allOk ? "Bot 连通性测试成功" : "部分目标发送失败",
          description: allOk
            ? `成功向 ${res.data.results.length} 个目标发送测试消息`
            : "请检查群组/频道配置是否正确",
          variant: allOk ? "success" : "destructive",
        });
      } else {
        toast({ title: "Bot 测试失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "Bot 测试出错", description: err.message, variant: "destructive" });
    } finally {
      setIsBotTesting(false);
    }
  }, [toast]);

  // Connectivity test
  const handleTestConnectivity = useCallback(async () => {
    setIsTesting(true);
    try {
      const res = await api.testEmbyConnectivity();
      if (res.success && res.data) {
        setTestResult(res.data);
        toast({
          title: res.data.overall ? "连通性测试通过" : "部分测试失败",
          description: res.data.tests
            .map((t) => `${t.name}: ${t.success ? "✓" : "✗"}`)
            .join(", "),
          variant: res.data.overall ? "success" : "destructive",
        });
      } else {
        toast({ title: "测试失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "测试出错", description: err.message, variant: "destructive" });
    } finally {
      setIsTesting(false);
    }
  }, [toast]);

  // Load Emby users
  const handleLoadUsers = useCallback(async () => {
    setIsLoadingUsers(true);
    try {
      const res = await api.listEmbyUsers();
      if (res.success && res.data) {
        setEmbyData(res.data);
      } else {
        toast({ title: "加载失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "加载出错", description: err.message, variant: "destructive" });
    } finally {
      setIsLoadingUsers(false);
    }
  }, [toast]);

  // Sync all
  const handleSync = useCallback(async () => {
    setIsSyncing(true);
    try {
      const res = await api.syncAllEmbyUsers();
      if (res.success && res.data) {
        toast({
          title: "同步完成",
          description: `成功 ${res.data.success} 个，失败 ${res.data.failed} 个`,
          variant: res.data.failed > 0 ? "destructive" : "success",
        });
        await handleLoadUsers();
      } else {
        toast({ title: "同步失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "同步出错", description: err.message, variant: "destructive" });
    } finally {
      setIsSyncing(false);
    }
  }, [toast, handleLoadUsers]);

  // Import unlinked users
  const handleImport = useCallback(async () => {
    setIsImporting(true);
    try {
      const res = await api.importEmbyUsers();
      if (res.success && res.data) {
        toast({
          title: "扫描完成",
          description: `发现 ${res.data.unlinked_count} 个未绑定用户，跳过 ${res.data.skipped_count} 个`,
          variant: "success",
        });
        await handleLoadUsers();
      } else {
        toast({ title: "扫描失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "扫描出错", description: err.message, variant: "destructive" });
    } finally {
      setIsImporting(false);
    }
  }, [toast, handleLoadUsers]);

  const handleDeleteUnlinked = useCallback(async () => {
    setIsDeletingUnlinked(true);
    try {
      const res = await api.deleteUnlinkedEmbyUsers(false);
      if (res.success && res.data) {
        toast({
          title: "删除完成",
          description: `共 ${res.data.count} 个未绑定用户，成功删除 ${res.data.deleted.length} 个`,
          variant: res.data.failed.length > 0 ? "destructive" : "success",
        });
        await handleLoadUsers();
      } else {
        toast({ title: "删除失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "删除出错", description: err.message, variant: "destructive" });
    } finally {
      setIsDeletingUnlinked(false);
    }
  }, [toast, handleLoadUsers]);

  // Cleanup orphans
  const handleCleanup = useCallback(async () => {
    setIsCleaning(true);
    try {
      const res = await api.cleanupOrphanEmbyIds();
      if (res.success && res.data) {
        toast({
          title: "清理完成",
          description: `已清理 ${res.data.count} 条孤儿记录`,
          variant: "success",
        });
        await handleLoadUsers();
      } else {
        toast({ title: "清理失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "清理出错", description: err.message, variant: "destructive" });
    } finally {
      setIsCleaning(false);
    }
  }, [toast, handleLoadUsers]);

  // Reset all bindings
  const handleResetBindings = useCallback(async () => {
    setIsResetting(true);
    try {
      const res = await api.resetAllEmbyBindings();
      if (res.success && res.data) {
        toast({
          title: "重置完成",
          description: `已重置 ${res.data.count} 个用户的 Emby 绑定`,
          variant: "success",
        });
        setResetDialogOpen(false);
        await handleLoadUsers();
      } else {
        toast({ title: "重置失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "重置出错", description: err.message, variant: "destructive" });
    } finally {
      setIsResetting(false);
    }
  }, [toast, handleLoadUsers]);

  const syncStatusBadge = (status: string) => {
    // 用户名一致与否不展示——本地与 Emby 用户名是允许不一致的，
    // 只要本地账户绑定到了对应 Emby ID 即视为已绑定。
    if (status === "unlinked") {
      return <Badge variant="secondary">未绑定</Badge>;
    }
    return (
      <Badge
        variant="default"
        className="bg-emerald-500/10 text-emerald-500 border-emerald-500/20"
      >
        已绑定
      </Badge>
    );
  };

  useEffect(() => {
    void handleLoadUsers();
  }, [handleLoadUsers]);

  return (
    <motion.div
      variants={container}
      initial="hidden"
      animate="show"
      className="space-y-6"
    >
      {/* Page Header */}
      <div>
        <h1 className="text-3xl font-bold">Emby 管理</h1>
        <p className="text-muted-foreground">
          管理 Emby 服务器连接、用户同步与数据清理
        </p>
      </div>

      {/* Connectivity Test */}
      <motion.div variants={item}>
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle className="flex items-center gap-2">
                  <Wifi className="h-5 w-5" />
                  连通性测试
                </CardTitle>
                <CardDescription>
                  一键测试 Emby 服务器的网络连通、API 认证、用户列表和媒体库
                </CardDescription>
              </div>
              <Button onClick={handleTestConnectivity} disabled={isTesting}>
                {isTesting ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <RefreshCw className="mr-2 h-4 w-4" />
                )}
                测试连通性
              </Button>
            </div>
          </CardHeader>
          {testResult && (
            <CardContent className="space-y-4">
              {/* Server info */}
              {testResult.server_info && (
                <div className="rounded-lg border p-3 bg-muted/50">
                  <div className="flex items-center gap-2 mb-2">
                    <Server className="h-4 w-4 text-muted-foreground" />
                    <span className="text-sm font-medium">服务器信息</span>
                  </div>
                  <div className="grid grid-cols-2 md:grid-cols-4 gap-2 text-sm">
                    <div>
                      <span className="text-muted-foreground">名称：</span>
                      {testResult.server_info.name}
                    </div>
                    <div>
                      <span className="text-muted-foreground">版本：</span>
                      {testResult.server_info.version}
                    </div>
                    <div>
                      <span className="text-muted-foreground">系统：</span>
                      {testResult.server_info.os}
                    </div>
                    <div>
                      <span className="text-muted-foreground">URL：</span>
                      <span className="break-all">{testResult.emby_url}</span>
                    </div>
                  </div>
                </div>
              )}
              {/* Test results */}
              <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
                {testResult.tests.map((t, i) => (
                  <div
                    key={i}
                    className={`rounded-lg border p-3 ${
                      t.success
                        ? "border-emerald-500/20 bg-emerald-500/5"
                        : "border-red-500/20 bg-red-500/5"
                    }`}
                  >
                    <div className="flex items-center gap-2 mb-1">
                      {t.success ? (
                        <CheckCircle2 className="h-4 w-4 text-emerald-500" />
                      ) : (
                        <XCircle className="h-4 w-4 text-red-500" />
                      )}
                      <span className="font-medium text-sm">{t.name}</span>
                    </div>
                    <p className="text-xs text-muted-foreground">{t.message}</p>
                  </div>
                ))}
              </div>
              {/* Overall status */}
              <div className="flex items-center gap-2">
                {testResult.overall ? (
                  <>
                    <CheckCircle2 className="h-5 w-5 text-emerald-500" />
                    <span className="text-sm font-medium text-emerald-500">
                      所有测试通过
                    </span>
                  </>
                ) : (
                  <>
                    <WifiOff className="h-5 w-5 text-red-500" />
                    <span className="text-sm font-medium text-red-500">
                      部分测试未通过，请检查 Emby 配置
                    </span>
                  </>
                )}
              </div>
            </CardContent>
          )}
        </Card>
      </motion.div>

      {/* Bot Connectivity Test */}
      <motion.div variants={item}>
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle className="flex items-center gap-2">
                  <MessageCircle className="h-5 w-5" />
                  Telegram Bot 连通性测试
                </CardTitle>
                <CardDescription>
                  向所有已配置的群组和频道发送一条测试消息，验证 Bot 是否正常工作
                </CardDescription>
              </div>
              <Button onClick={handleTestBot} disabled={isBotTesting}>
                {isBotTesting ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Send className="mr-2 h-4 w-4" />
                )}
                发送测试
              </Button>
            </div>
          </CardHeader>
          {botResults && (
            <CardContent className="space-y-3">
              {botRuntime && (
                <div className="rounded-lg border bg-muted/30 p-3 text-xs text-muted-foreground">
                  <div>轮询状态：{botRuntime.polling ? "运行中" : "未运行或等待配置"}</div>
                  {botRuntime.last_ok_at ? <div>最近成功：{new Date(botRuntime.last_ok_at * 1000).toLocaleString()}</div> : null}
                  {botRuntime.last_error_at ? <div>最近错误时间：{new Date(botRuntime.last_error_at * 1000).toLocaleString()}</div> : null}
                  {botRuntime.last_error ? <div className="break-words text-red-500">最近错误：{botRuntime.last_error}</div> : null}
                </div>
              )}
              <div className="grid gap-2 sm:grid-cols-2">
                {botResults.map((r, i) => (
                  <div
                    key={i}
                    className={`rounded-lg border p-3 ${
                      r.success
                        ? "border-emerald-500/20 bg-emerald-500/5"
                        : "border-red-500/20 bg-red-500/5"
                    }`}
                  >
                    <div className="flex items-center gap-2 mb-1">
                      {r.success ? (
                        <CheckCircle2 className="h-4 w-4 text-emerald-500" />
                      ) : (
                        <XCircle className="h-4 w-4 text-red-500" />
                      )}
                      <span className="font-medium text-sm font-mono">{r.target}</span>
                    </div>
                    {r.error && (
                      <p className="text-xs text-red-500 mt-1">{r.error}</p>
                    )}
                    {!r.error && (r.username || r.title || r.bot_status) && (
                      <p className="mt-1 text-xs text-muted-foreground">
                        {[r.username ? `@${r.username}` : "", r.title || "", r.bot_status ? `Bot 状态：${r.bot_status}` : ""].filter(Boolean).join(" · ")}
                      </p>
                    )}
                    {r.success && (
                      <p className="text-xs text-muted-foreground">发送成功</p>
                    )}
                  </div>
                ))}
              </div>
              <div className="flex items-center gap-2">
                {botResults.every((r) => r.success) ? (
                  <>
                    <CheckCircle2 className="h-5 w-5 text-emerald-500" />
                    <span className="text-sm font-medium text-emerald-500">
                      所有目标发送成功
                    </span>
                  </>
                ) : (
                  <>
                    <WifiOff className="h-5 w-5 text-red-500" />
                    <span className="text-sm font-medium text-red-500">
                      部分目标发送失败，请检查群组/频道配置
                    </span>
                  </>
                )}
              </div>
            </CardContent>
          )}
        </Card>
      </motion.div>

      {/* User Management */}
      <motion.div variants={item}>
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between flex-wrap gap-4">
              <div>
                <CardTitle className="flex items-center gap-2">
                  <Users className="h-5 w-5" />
                  用户数据管理
                </CardTitle>
                <CardDescription>
                  从 Emby 拉取用户列表，对比本地数据库，执行导入、同步与清理
                </CardDescription>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button variant="outline" onClick={handleLoadUsers} disabled={isLoadingUsers}>
                  {isLoadingUsers ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <RefreshCw className="mr-2 h-4 w-4" />
                  )}
                  拉取数据
                </Button>
                <Button variant="outline" onClick={handleSync} disabled={isSyncing || !embyData}>
                  {isSyncing ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Link2 className="mr-2 h-4 w-4" />
                  )}
                  同步用户
                </Button>
                <Button variant="outline" onClick={handleImport} disabled={isImporting || !embyData}>
                  {isImporting ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Download className="mr-2 h-4 w-4" />
                  )}
                  扫描未绑定
                </Button>
                <Button variant="destructive" onClick={handleDeleteUnlinked} disabled={isDeletingUnlinked || !embyData}>
                  {isDeletingUnlinked ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Trash2 className="mr-2 h-4 w-4" />
                  )}
                  删除未绑定
                </Button>
              </div>
            </div>
          </CardHeader>
          {embyData && (
            <CardContent className="space-y-4">
              {/* Summary */}
              <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                <div className="rounded-lg border p-3 text-center">
                  <div className="text-2xl font-bold">{embyData.total_emby}</div>
                  <div className="text-xs text-muted-foreground">Emby 用户</div>
                </div>
                <div className="rounded-lg border p-3 text-center">
                  <div className="text-2xl font-bold text-emerald-500">{embyData.total_linked}</div>
                  <div className="text-xs text-muted-foreground">已关联</div>
                </div>
                <div className="rounded-lg border p-3 text-center">
                  <div className="text-2xl font-bold text-blue-500">
                    {embyData.total_emby - embyData.total_linked}
                  </div>
                  <div className="text-xs text-muted-foreground">未关联</div>
                </div>
                <div className="rounded-lg border p-3 text-center">
                  <div className={`text-2xl font-bold ${embyData.total_orphans > 0 ? "text-amber-500" : ""}`}>
                    {embyData.total_orphans}
                  </div>
                  <div className="text-xs text-muted-foreground">孤儿记录</div>
                </div>
              </div>

              {/* Emby Users Table */}
              <div>
                <div className="mb-3 flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
                  <h3 className="text-sm font-medium">Emby 用户列表</h3>
                  <Badge variant="outline" className="w-fit text-xs">
                    当前展示 {filteredEmbyUsers.length} / {embyData.emby_users.length}
                  </Badge>
                </div>

                <div className="mb-3 grid gap-2 md:grid-cols-3">
                  <Input
                    placeholder="搜索 Emby 名称 / EmbyID / 本地用户名 / UID / Telegram ID"
                    value={userSearch}
                    onChange={(event) => setUserSearch(event.target.value)}
                  />
                  <Select value={linkFilter} onValueChange={(value) => setLinkFilter(value as typeof linkFilter)}>
                    <SelectTrigger>
                      <SelectValue placeholder="关联状态" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">全部关联状态</SelectItem>
                      <SelectItem value="linked">仅已绑定本地用户</SelectItem>
                      <SelectItem value="unlinked">仅未绑定本地用户</SelectItem>
                      <SelectItem value="name_mismatch">仅名称不一致</SelectItem>
                    </SelectContent>
                  </Select>
                  <Select value={attrFilter} onValueChange={(value) => setAttrFilter(value as typeof attrFilter)}>
                    <SelectTrigger>
                      <SelectValue placeholder="属性筛选" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">全部属性</SelectItem>
                      <SelectItem value="admin">仅管理员</SelectItem>
                      <SelectItem value="disabled">仅已禁用</SelectItem>
                      <SelectItem value="hidden">仅隐藏</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="rounded-lg border overflow-hidden">
                  <div className="overflow-x-auto">
                    <table className="w-full text-sm">
                      <thead className="bg-muted/50">
                        <tr>
                          <th className="text-left p-3 font-medium">Emby 用户名</th>
                          <th className="text-left p-3 font-medium">属性</th>
                          <th className="text-left p-3 font-medium">本地用户</th>
                          <th className="text-left p-3 font-medium">状态</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y">
                        {filteredEmbyUsers.map((eu) => (
                          <tr key={eu.emby_id} className="hover:bg-muted/30">
                            <td className="p-3">
                              <div className="font-medium">{eu.emby_name}</div>
                              <div className="text-xs text-muted-foreground font-mono">
                                {eu.emby_id.slice(0, 8)}...
                              </div>
                            </td>
                            <td className="p-3">
                              <div className="flex flex-wrap gap-1">
                                {eu.is_admin && (
                                  <Badge variant="default" className="bg-purple-500/10 text-purple-500 border-purple-500/20">
                                    <Shield className="h-3 w-3 mr-1" />管理员
                                  </Badge>
                                )}
                                {eu.is_disabled && (
                                  <Badge variant="destructive" className="text-xs">已禁用</Badge>
                                )}
                                {eu.is_hidden && (
                                  <Badge variant="secondary" className="text-xs">隐藏</Badge>
                                )}
                              </div>
                            </td>
                            <td className="p-3">
                              {eu.local_user ? (
                                <div>
                                  <span className="font-medium">{eu.local_user.username}</span>
                                  <span className="text-xs text-muted-foreground ml-1">
                                    (UID: {eu.local_user.uid})
                                  </span>
                                </div>
                              ) : (
                                <span className="text-muted-foreground">—</span>
                              )}
                            </td>
                            <td className="p-3">{syncStatusBadge(eu.sync_status)}</td>
                          </tr>
                        ))}
                        {filteredEmbyUsers.length === 0 && (
                          <tr>
                            <td colSpan={4} className="p-6 text-center text-muted-foreground">
                              未匹配到筛选结果
                            </td>
                          </tr>
                        )}
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>

              {/* Orphans */}
              {embyData.orphans.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium mb-2 flex items-center gap-2">
                    <AlertTriangle className="h-4 w-4 text-amber-500" />
                    孤儿记录
                    <span className="text-xs text-muted-foreground">
                      （本地有 EMBYID 但 Emby 端已不存在）
                    </span>
                  </h3>
                  <div className="rounded-lg border overflow-hidden">
                    <div className="overflow-x-auto">
                      <table className="w-full text-sm">
                        <thead className="bg-muted/50">
                          <tr>
                            <th className="text-left p-3 font-medium">本地用户名</th>
                            <th className="text-left p-3 font-medium">UID</th>
                            <th className="text-left p-3 font-medium">已失效 EMBYID</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y">
                          {embyData.orphans.map((o) => (
                            <tr key={o.uid} className="hover:bg-muted/30">
                              <td className="p-3 font-medium">{o.username}</td>
                              <td className="p-3">{o.uid}</td>
                              <td className="p-3 font-mono text-xs text-muted-foreground">
                                {o.emby_id}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                </div>
              )}
            </CardContent>
          )}
        </Card>
      </motion.div>

      {/* Cleanup Actions */}
      <motion.div variants={item}>
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Trash2 className="h-5 w-5" />
              数据清理
            </CardTitle>
            <CardDescription>
              清理失效数据，方便测试和防止数据库混乱
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {/* Cleanup orphans */}
            <div className="flex items-center justify-between rounded-lg border p-4">
              <div>
                <div className="font-medium flex items-center gap-2">
                  <Link2Off className="h-4 w-4 text-amber-500" />
                  清理孤儿记录
                </div>
                <p className="text-sm text-muted-foreground mt-1">
                  清除本地数据库中指向已不存在的 Emby 用户的 EMBYID，不会删除本地用户
                </p>
              </div>
              <Button
                variant="outline"
                onClick={handleCleanup}
                disabled={isCleaning}
              >
                {isCleaning ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Trash2 className="mr-2 h-4 w-4" />
                )}
                清理
              </Button>
            </div>

            {/* Reset all bindings */}
            <div className="flex items-center justify-between rounded-lg border border-red-500/20 p-4 bg-red-500/5">
              <div>
                <div className="font-medium flex items-center gap-2 text-red-500">
                  <AlertTriangle className="h-4 w-4" />
                  重置所有 Emby 绑定
                </div>
                <p className="text-sm text-muted-foreground mt-1">
                  清空所有用户的 EMBYID 绑定，不会删除 Emby 端账户。适合测试环境重置
                </p>
              </div>
              <Button
                variant="destructive"
                onClick={() => setResetDialogOpen(true)}
                disabled={isResetting}
              >
                {isResetting ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Trash2 className="mr-2 h-4 w-4" />
                )}
                重置绑定
              </Button>
            </div>
          </CardContent>
        </Card>
      </motion.div>

      {/* Reset confirmation dialog */}
      <Dialog open={resetDialogOpen} onOpenChange={setResetDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-red-500">
              <AlertTriangle className="h-5 w-5" />
              确认重置所有 Emby 绑定
            </DialogTitle>
            <DialogDescription>
              此操作将清空所有本地用户的 EMBYID 绑定关系。这是一个危险操作，
              通常只在测试环境中使用。操作不可逆，需要重新同步才能恢复绑定。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setResetDialogOpen(false)}>
              取消
            </Button>
            <Button
              variant="destructive"
              onClick={handleResetBindings}
              disabled={isResetting}
            >
              {isResetting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认重置
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

    </motion.div>
  );
}
