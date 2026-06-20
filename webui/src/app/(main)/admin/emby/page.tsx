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
  MonitorSmartphone,
  Wifi,
  WifiOff,
} from "lucide-react";
import AdminDeviceAuditPanel from "@/components/admin/emby-device-audit-panel";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useToast } from "@/hooks/use-toast";
import { useI18n } from "@/lib/i18n";
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
  const { t } = useI18n();
  const [activeTab, setActiveTab] = useState("accounts");
  const [deviceAuditVisited, setDeviceAuditVisited] = useState(false);

  useEffect(() => {
    if (new URLSearchParams(window.location.search).get("tab") === "devices") {
      setActiveTab("devices");
      setDeviceAuditVisited(true);
    }
  }, []);

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

  // Connectivity test
  const handleTestConnectivity = useCallback(async () => {
    setIsTesting(true);
    try {
      const res = await api.testEmbyConnectivity();
      if (res.success && res.data) {
        setTestResult(res.data);
        toast({
          title: res.data.overall ? t("adminEmby.connOk") : t("adminEmby.connPartialFail"),
          description: res.data.tests
            .map((tr) => `${tr.name}: ${tr.success ? "✓" : "✗"}`)
            .join(", "),
          variant: res.data.overall ? "success" : "destructive",
        });
      } else {
        toast({ title: t("adminEmby.testFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("adminEmby.testError"), description: err.message, variant: "destructive" });
    } finally {
      setIsTesting(false);
    }
  }, [toast, t]);

  // Load Emby users
  const handleLoadUsers = useCallback(async () => {
    setIsLoadingUsers(true);
    try {
      const res = await api.listEmbyUsers();
      if (res.success && res.data) {
        setEmbyData(res.data);
      } else {
        toast({ title: t("adminEmby.loadFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("adminEmby.loadError"), description: err.message, variant: "destructive" });
    } finally {
      setIsLoadingUsers(false);
    }
  }, [toast, t]);

  // Sync all
  const handleSync = useCallback(async () => {
    setIsSyncing(true);
    try {
      const res = await api.syncAllEmbyUsers();
      if (res.success && res.data) {
        toast({
          title: t("adminEmby.syncComplete"),
          description: t("adminEmby.syncResult", { success: res.data.success, failed: res.data.failed }),
          variant: res.data.failed > 0 ? "destructive" : "success",
        });
        await handleLoadUsers();
      } else {
        toast({ title: t("adminEmby.syncFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("adminEmby.syncError"), description: err.message, variant: "destructive" });
    } finally {
      setIsSyncing(false);
    }
  }, [toast, handleLoadUsers, t]);

  // Import unlinked users
  const handleImport = useCallback(async () => {
    setIsImporting(true);
    try {
      const res = await api.importEmbyUsers();
      if (res.success && res.data) {
        toast({
          title: t("adminEmby.scanComplete"),
          description: t("adminEmby.scanResult", { unlinked: res.data.unlinked_count, skipped: res.data.skipped_count }),
          variant: "success",
        });
        await handleLoadUsers();
      } else {
        toast({ title: t("adminEmby.scanFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("adminEmby.scanError"), description: err.message, variant: "destructive" });
    } finally {
      setIsImporting(false);
    }
  }, [toast, handleLoadUsers, t]);

  const handleDeleteUnlinked = useCallback(async () => {
    setIsDeletingUnlinked(true);
    try {
      const res = await api.deleteUnlinkedEmbyUsers(false);
      if (res.success && res.data) {
        toast({
          title: t("adminEmby.deleteComplete"),
          description: t("adminEmby.deleteResult", { count: res.data.count, deleted: res.data.deleted.length }),
          variant: res.data.failed.length > 0 ? "destructive" : "success",
        });
        await handleLoadUsers();
      } else {
        toast({ title: t("adminEmby.deleteFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("adminEmby.deleteError"), description: err.message, variant: "destructive" });
    } finally {
      setIsDeletingUnlinked(false);
    }
  }, [toast, handleLoadUsers, t]);

  // Cleanup orphans
  const handleCleanup = useCallback(async () => {
    setIsCleaning(true);
    try {
      const res = await api.cleanupOrphanEmbyIds();
      if (res.success && res.data) {
        toast({
          title: t("adminEmby.cleanupComplete"),
          description: t("adminEmby.cleanupResult", { count: res.data.count }),
          variant: "success",
        });
        await handleLoadUsers();
      } else {
        toast({ title: t("adminEmby.cleanupFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("adminEmby.cleanupError"), description: err.message, variant: "destructive" });
    } finally {
      setIsCleaning(false);
    }
  }, [toast, handleLoadUsers, t]);

  // Reset all bindings
  const handleResetBindings = useCallback(async () => {
    setIsResetting(true);
    try {
      const res = await api.resetAllEmbyBindings();
      if (res.success && res.data) {
        toast({
          title: t("adminEmby.resetComplete"),
          description: t("adminEmby.resetResult", { count: res.data.count }),
          variant: "success",
        });
        setResetDialogOpen(false);
        await handleLoadUsers();
      } else {
        toast({ title: t("adminEmby.resetFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("adminEmby.resetError"), description: err.message, variant: "destructive" });
    } finally {
      setIsResetting(false);
    }
  }, [toast, handleLoadUsers, t]);

  const syncStatusBadge = (status: string) => {
    // 用户名一致与否不展示——本地与 Emby 用户名是允许不一致的，
    // 只要本地账户绑定到了对应 Emby ID 即视为已绑定。
    if (status === "unlinked") {
      return <Badge variant="secondary">{t("adminEmby.statusUnbound")}</Badge>;
    }
    return (
      <Badge
        variant="default"
        className="bg-emerald-500/10 text-emerald-500 border-emerald-500/20"
      >
        {t("adminEmby.statusBound")}
      </Badge>
    );
  };

  return (
    <motion.div
      variants={container}
      initial="hidden"
      animate="show"
      className="space-y-6"
    >
      {/* Page Header */}
      <div>
        <h1 className="text-3xl font-bold">{t("adminEmby.title")}</h1>
        <p className="text-muted-foreground">
          {t("adminEmby.description")}
        </p>
      </div>

      <Tabs
        value={activeTab}
        onValueChange={(value) => {
          setActiveTab(value);
          if (value === "devices") setDeviceAuditVisited(true);
        }}
        className="space-y-5"
      >
        <TabsList className="i18n-stable-tabs grid h-auto w-full grid-cols-2 sm:w-fit">
          <TabsTrigger value="accounts" className="gap-2">
            <Server className="h-4 w-4" />
            {t("adminEmby.accountsTab")}
          </TabsTrigger>
          <TabsTrigger value="devices" className="gap-2">
            <MonitorSmartphone className="h-4 w-4" />
            {t("adminEmby.deviceAuditTab")}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="accounts" className="mt-0 space-y-6">
      {/* Connectivity Test */}
      <motion.div variants={item}>
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle className="flex items-center gap-2">
                  <Wifi className="h-5 w-5" />
                  {t("adminEmby.connTitle")}
                </CardTitle>
                <CardDescription>
                  {t("adminEmby.connDesc")}
                </CardDescription>
              </div>
              <Button onClick={handleTestConnectivity} disabled={isTesting}>
                {isTesting ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <RefreshCw className="mr-2 h-4 w-4" />
                )}
                {t("adminEmby.connTest")}
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
                    <span className="text-sm font-medium">{t("adminEmby.serverInfo")}</span>
                  </div>
                  <div className="grid grid-cols-2 md:grid-cols-4 gap-2 text-sm">
                    <div>
                      <span className="text-muted-foreground">{t("adminEmby.nameLabel")}</span>
                      {testResult.server_info.name}
                    </div>
                    <div>
                      <span className="text-muted-foreground">{t("adminEmby.versionLabel")}</span>
                      {testResult.server_info.version}
                    </div>
                    <div>
                      <span className="text-muted-foreground">{t("adminEmby.systemLabel")}</span>
                      {testResult.server_info.os}
                    </div>
                    <div>
                      <span className="text-muted-foreground">{t("adminEmby.urlLabel")}</span>
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
                      {t("adminEmby.allTestsPassed")}
                    </span>
                  </>
                ) : (
                  <>
                    <WifiOff className="h-5 w-5 text-red-500" />
                    <span className="text-sm font-medium text-red-500">
                      {t("adminEmby.partialTestsFailed")}
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
                  {t("adminEmby.userMgmtTitle")}
                </CardTitle>
                <CardDescription>
                  {t("adminEmby.userMgmtDesc")}
                </CardDescription>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button variant="outline" onClick={handleLoadUsers} disabled={isLoadingUsers}>
                  {isLoadingUsers ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <RefreshCw className="mr-2 h-4 w-4" />
                  )}
                  {t("adminEmby.fetchData")}
                </Button>
                <Button variant="outline" onClick={handleSync} disabled={isSyncing || !embyData}>
                  {isSyncing ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Link2 className="mr-2 h-4 w-4" />
                  )}
                  {t("adminEmby.syncUsers")}
                </Button>
                <Button variant="outline" onClick={handleImport} disabled={isImporting || !embyData}>
                  {isImporting ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Download className="mr-2 h-4 w-4" />
                  )}
                  {t("adminEmby.scanUnlinked")}
                </Button>
                <Button variant="destructive" onClick={handleDeleteUnlinked} disabled={isDeletingUnlinked || !embyData}>
                  {isDeletingUnlinked ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Trash2 className="mr-2 h-4 w-4" />
                  )}
                  {t("adminEmby.deleteUnlinkedBtn")}
                </Button>
              </div>
            </div>
          </CardHeader>
          {embyData ? (
            <CardContent className="space-y-4">
              {/* Summary */}
              <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                <div className="rounded-lg border p-3 text-center">
                  <div className="text-2xl font-bold">{embyData.total_emby}</div>
                  <div className="text-xs text-muted-foreground">{t("adminEmby.statEmbyUsers")}</div>
                </div>
                <div className="rounded-lg border p-3 text-center">
                  <div className="text-2xl font-bold text-emerald-500">{embyData.total_linked}</div>
                  <div className="text-xs text-muted-foreground">{t("adminEmby.statLinked")}</div>
                </div>
                <div className="rounded-lg border p-3 text-center">
                  <div className="text-2xl font-bold text-blue-500">
                    {embyData.total_emby - embyData.total_linked}
                  </div>
                  <div className="text-xs text-muted-foreground">{t("adminEmby.statUnlinked")}</div>
                </div>
                <div className="rounded-lg border p-3 text-center">
                  <div className={`text-2xl font-bold ${embyData.total_orphans > 0 ? "text-amber-500" : ""}`}>
                    {embyData.total_orphans}
                  </div>
                  <div className="text-xs text-muted-foreground">{t("adminEmby.statOrphans")}</div>
                </div>
              </div>

              {/* Emby Users Table */}
              <div>
                <div className="mb-3 flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
                  <h3 className="text-sm font-medium">{t("adminEmby.userListTitle")}</h3>
                  <Badge variant="outline" className="w-fit text-xs">
                    {t("adminEmby.showingCount", { shown: filteredEmbyUsers.length, total: embyData.emby_users.length })}
                  </Badge>
                </div>

                <div className="mb-3 grid gap-2 md:grid-cols-3">
                  <Input
                    placeholder={t("adminEmby.searchPlaceholder")}
                    value={userSearch}
                    onChange={(event) => setUserSearch(event.target.value)}
                  />
                  <Select value={linkFilter} onValueChange={(value) => setLinkFilter(value as typeof linkFilter)}>
                    <SelectTrigger>
                      <SelectValue placeholder={t("adminEmby.linkFilterPlaceholder")} />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">{t("adminEmby.linkAll")}</SelectItem>
                      <SelectItem value="linked">{t("adminEmby.linkLinked")}</SelectItem>
                      <SelectItem value="unlinked">{t("adminEmby.linkUnlinked")}</SelectItem>
                      <SelectItem value="name_mismatch">{t("adminEmby.linkNameMismatch")}</SelectItem>
                    </SelectContent>
                  </Select>
                  <Select value={attrFilter} onValueChange={(value) => setAttrFilter(value as typeof attrFilter)}>
                    <SelectTrigger>
                      <SelectValue placeholder={t("adminEmby.attrFilterPlaceholder")} />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">{t("adminEmby.attrAll")}</SelectItem>
                      <SelectItem value="admin">{t("adminEmby.attrAdmin")}</SelectItem>
                      <SelectItem value="disabled">{t("adminEmby.attrDisabled")}</SelectItem>
                      <SelectItem value="hidden">{t("adminEmby.attrHidden")}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="rounded-lg border overflow-hidden">
                  <div className="overflow-x-auto">
                    <table className="w-full text-sm">
                      <thead className="bg-muted/50">
                        <tr>
                          <th className="text-left p-3 font-medium">{t("adminEmby.thEmbyUsername")}</th>
                          <th className="text-left p-3 font-medium">{t("adminEmby.thAttr")}</th>
                          <th className="text-left p-3 font-medium">{t("adminEmby.thLocalUser")}</th>
                          <th className="text-left p-3 font-medium">{t("adminEmby.thStatus")}</th>
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
                                    <Shield className="h-3 w-3 mr-1" />{t("adminEmby.badgeAdmin")}
                                  </Badge>
                                )}
                                {eu.is_disabled && (
                                  <Badge variant="destructive" className="text-xs">{t("adminEmby.badgeDisabled")}</Badge>
                                )}
                                {eu.is_hidden && (
                                  <Badge variant="secondary" className="text-xs">{t("adminEmby.badgeHidden")}</Badge>
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
                              {t("adminEmby.noFilterMatch")}
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
                    {t("adminEmby.orphansTitle")}
                    <span className="text-xs text-muted-foreground">
                      {t("adminEmby.orphansHint")}
                    </span>
                  </h3>
                  <div className="rounded-lg border overflow-hidden">
                    <div className="overflow-x-auto">
                      <table className="w-full text-sm">
                        <thead className="bg-muted/50">
                          <tr>
                            <th className="text-left p-3 font-medium">{t("adminEmby.thLocalUsername")}</th>
                            <th className="text-left p-3 font-medium">UID</th>
                            <th className="text-left p-3 font-medium">{t("adminEmby.thInvalidEmbyId")}</th>
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
          ) : (
            <CardContent>
              <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
                {t("adminEmby.usersNotAutoLoaded")}
              </div>
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
              {t("adminEmby.cleanupTitle")}
            </CardTitle>
            <CardDescription>
              {t("adminEmby.cleanupCardDesc")}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {/* Cleanup orphans */}
            <div className="flex items-center justify-between rounded-lg border p-4">
              <div>
                <div className="font-medium flex items-center gap-2">
                  <Link2Off className="h-4 w-4 text-amber-500" />
                  {t("adminEmby.cleanOrphansTitle")}
                </div>
                <p className="text-sm text-muted-foreground mt-1">
                  {t("adminEmby.cleanOrphansDesc")}
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
                {t("adminEmby.cleanBtn")}
              </Button>
            </div>

            {/* Reset all bindings */}
            <div className="flex items-center justify-between rounded-lg border border-red-500/20 p-4 bg-red-500/5">
              <div>
                <div className="font-medium flex items-center gap-2 text-red-500">
                  <AlertTriangle className="h-4 w-4" />
                  {t("adminEmby.resetBindingsTitle")}
                </div>
                <p className="text-sm text-muted-foreground mt-1">
                  {t("adminEmby.resetBindingsDesc")}
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
                {t("adminEmby.resetBindingsBtn")}
              </Button>
            </div>
          </CardContent>
        </Card>
      </motion.div>
        </TabsContent>

        <TabsContent value="devices" className="mt-0">
          {deviceAuditVisited && <AdminDeviceAuditPanel embedded />}
        </TabsContent>
      </Tabs>

      {/* Reset confirmation dialog */}
      <Dialog open={resetDialogOpen} onOpenChange={setResetDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-red-500">
              <AlertTriangle className="h-5 w-5" />
              {t("adminEmby.resetDialogTitle")}
            </DialogTitle>
            <DialogDescription>
              {t("adminEmby.resetDialogDesc")}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setResetDialogOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={handleResetBindings}
              disabled={isResetting}
            >
              {isResetting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t("adminEmby.resetConfirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

    </motion.div>
  );
}
