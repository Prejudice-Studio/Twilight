"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  MonitorSmartphone,
  RefreshCw,
  Loader2,
  AlertTriangle,
  Search,
  ChevronDown,
  ChevronRight,
  Wifi,
  Network,
  Users as UsersIcon,
  Shield,
  AppWindow,
  LayoutList,
  X,
  Ban,
  UserCheck,
  MonitorX,
  MonitorCheck,
  LogOut,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/hooks/use-toast";
import { useI18n, type MessageKey, type MessageParams } from "@/lib/i18n";
import { api } from "@/lib/api";
import type { EmbyAuditDevice, EmbyAuditUser, EmbyDeviceAuditData } from "@/lib/api-types";

type TFunc = (key: MessageKey, params?: MessageParams) => string;
type SortKey = "devices" | "ips" | "online" | "activity" | "name";
type CategoryKey = "all" | "linked" | "unlinked" | "online" | "multiDevice" | "multiIp";
type ViewMode = "users" | "devices";

// 与后端 permanentExpiryUnix 同量级：超过该阈值视为「永久有效」。
const PERMANENT_THRESHOLD = 253402214400;

// 客户端筛选的「全部」哨兵，以及「未知客户端」（AppName 为空）的哨兵——Radix Select
// 不接受空字符串作为 value，所以空 AppName 在下拉里用一个稳定占位值表示。
const CLIENT_ALL = "all";
const CLIENT_UNKNOWN = "__unknown__";

// clientFilterValue 把后端的客户端名（可能为空串）映射成可用于 Select / 比较的稳定值。
function clientFilterValue(name: string): string {
  return name === "" ? CLIENT_UNKNOWN : name;
}

function roleLabelKey(role: number): MessageKey {
  if (role === 0) return "deviceAudit.roleAdmin";
  if (role === 2) return "deviceAudit.roleWhitelist";
  return "deviceAudit.roleUser";
}

function formatUnix(
  seconds: number | null | undefined,
  locale: string,
  permanent: string,
): string {
  if (seconds == null || seconds === 0) return "—";
  if (seconds < 0 || seconds >= PERMANENT_THRESHOLD) return permanent;
  return new Date(seconds * 1000).toLocaleString(locale);
}

function formatIso(value: string | null | undefined, locale: string): string {
  if (!value) return "—";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString(locale);
}

function userKey(u: EmbyAuditUser, index: number): string {
  return u.emby_user_id || (u.local_user ? `uid:${u.local_user.uid}` : `idx:${index}`);
}

// 安全访问 devices 数组——后端在某些边缘情况下可能返回 null 而非 []。
function safeDevices(u: EmbyAuditUser): EmbyAuditDevice[] {
  const devs = u.devices ?? [];
  // 排除 Twilight 自身（绑定 Bot 产生的设备记录），不对管理员展示
  return devs.filter((d) => {
    const app = (d.app_name || "").toLowerCase();
    const dev = (d.device_name || "").toLowerCase();
    if (app.includes("twilight")) return false;
    if (dev.includes("twilight")) return false;
    return true;
  });
}

// 用户级搜索命中字段：Emby 名 / EmbyID / 本地用户名 / UID / 邮箱 / Telegram / 任意 IP。
function userHaystack(u: EmbyAuditUser): string[] {
  return [
    u.emby_user_name,
    u.emby_user_id,
    u.local_user?.username ?? "",
    u.local_user ? String(u.local_user.uid) : "",
    u.local_user?.email ?? "",
    u.local_user?.telegram_username ?? "",
    u.local_user?.telegram_id != null ? String(u.local_user.telegram_id) : "",
    ...(u.ips ?? []),
  ];
}

// 设备级搜索：在用户字段基础上加上设备名 / 客户端 / 版本 / 该设备 IP。
function deviceHaystack(u: EmbyAuditUser, d: EmbyAuditDevice): string[] {
  return [d.device_name, d.app_name, d.app_version, d.ip, ...userHaystack(u)];
}

function matchesSearch(haystacks: string[], q: string): boolean {
  if (!q) return true;
  return haystacks.some((h) => h.toLowerCase().includes(q));
}

// 归类筛选对「用户」与「单台设备」两种粒度分别判定，保证两个视图语义一致：
// linked/unlinked/multiDevice/multiIp 都看归属用户，online 在设备视图看设备自身。
function userMatchesCategory(u: EmbyAuditUser, category: CategoryKey): boolean {
  switch (category) {
    case "linked":
      return Boolean(u.local_user);
    case "unlinked":
      return !u.local_user;
    case "online":
      return u.online_count > 0;
    case "multiDevice":
      return u.device_count >= 2;
    case "multiIp":
      return u.ip_count >= 2;
    default:
      return true;
  }
}

function deviceMatchesCategory(
  u: EmbyAuditUser,
  d: EmbyAuditDevice,
  category: CategoryKey,
): boolean {
  if (category === "online") return d.online;
  return userMatchesCategory(u, category);
}

export default function AdminDeviceAuditPanel({ embedded = false }: { embedded?: boolean } = {}) {
  const { t, locale } = useI18n();
  const { toast } = useToast();
  const { confirmAction } = useConfirm();

  const [data, setData] = useState<EmbyDeviceAuditData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [view, setView] = useState<ViewMode>("users");
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState<CategoryKey>("all");
  const [clientFilter, setClientFilter] = useState<string>(CLIENT_ALL);
  const [sortKey, setSortKey] = useState<SortKey>("devices");
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [lastRefreshAt, setLastRefreshAt] = useState<number | null>(null);

  const reload = useCallback(async (refresh = false) => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.adminGetEmbyDeviceAudit(refresh);
      if (res.success && res.data) {
        setData(res.data);
        setLastRefreshAt(Date.now());
      } else {
        throw new Error(res.message || t("deviceAudit.loadFailed"));
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : t("deviceAudit.loadFailed");
      setError(message);
      toast({ title: t("deviceAudit.loadFailed"), description: message, variant: "destructive" });
    } finally {
      setLoading(false);
    }
  }, [t, toast]);

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    if (!autoRefresh) return;
    const timer = window.setInterval(() => {
      if (document.visibilityState !== "visible") return;
      void reload(false);
    }, 30000);
    return () => window.clearInterval(timer);
  }, [autoRefresh, reload]);

  const toggleExpand = useCallback((id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  // 审查页快速处置：禁用/启用 Web、禁用/启用 Emby、踢出会话。统一走确认弹窗 + 成功后
  // 重载审查数据。Emby 启停按 emby_user_id 下发，后端对已关联本地用户会复用保护/有效期
  // 约束并同步镜像；未关联也可直接处置可疑账号。
  const runAuditAction = useCallback(
    async (
      title: string,
      name: string,
      fn: () => Promise<{ success: boolean; message?: string }>,
    ) => {
      const confirmed = await confirmAction({
        title,
        description: t("deviceAudit.actionConfirm", { name }),
        tone: "warning",
        confirmLabel: title,
      });
      if (!confirmed) return;
      try {
        const res = await fn();
        if (res.success) {
          toast({ title: t("deviceAudit.actionSuccess"), variant: "success" });
          await reload(true);
        } else {
          toast({ title: t("deviceAudit.actionFailed"), description: res.message, variant: "destructive" });
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        toast({ title: t("deviceAudit.actionFailed"), description: message, variant: "destructive" });
      }
    },
    [confirmAction, t, toast, reload],
  );

  const auditName = useCallback(
    (u: EmbyAuditUser) => u.emby_user_name || u.local_user?.username || t("deviceAudit.unknownUser"),
    [t],
  );

  const onWebToggle = useCallback(
    (u: EmbyAuditUser, enable: boolean) => {
      if (!u.local_user) return;
      void runAuditAction(
        enable ? t("deviceAudit.actionEnableWeb") : t("deviceAudit.actionDisableWeb"),
        auditName(u),
        () => api.toggleUserActive(u.local_user!.uid, { enable }),
      );
    },
    [runAuditAction, auditName, t],
  );

  const onEmbyToggle = useCallback(
    (u: EmbyAuditUser, enable: boolean) => {
      void runAuditAction(
        enable ? t("deviceAudit.actionEnableEmby") : t("deviceAudit.actionDisableEmby"),
        auditName(u),
        () => api.setEmbyUserEnabledById(u.emby_user_id, enable),
      );
    },
    [runAuditAction, auditName, t],
  );

  const onKick = useCallback(
    (u: EmbyAuditUser) => {
      void runAuditAction(t("deviceAudit.actionKick"), auditName(u), () =>
        api.kickEmbyUserById(u.emby_user_id),
      );
    },
    [runAuditAction, auditName, t],
  );

  const summary = data?.summary;
  const embyConfigured = data?.emby_configured ?? true;

  const clientFilterActive = clientFilter !== CLIENT_ALL;
  const clientFilterTarget = clientFilter === CLIENT_UNKNOWN ? "" : clientFilter;
  const clientFilterLabel = clientFilterTarget || t("deviceAudit.clientUnknown");
  const q = search.trim().toLowerCase();

  // 当前客户端筛选下，某用户应展示的设备子集（筛选未生效则为全部设备）。
  const devicesForUser = useCallback(
    (u: EmbyAuditUser) =>
      clientFilterActive
        ? safeDevices(u).filter((d) => (d.app_name || "") === clientFilterTarget)
        : safeDevices(u),
    [clientFilterActive, clientFilterTarget],
  );

  // 用户视图：先按客户端 / 归类 / 搜索过滤，再排序。
  const visibleUsers = useMemo<EmbyAuditUser[]>(() => {
    if (!data) return [];
    const users = data.users ?? [];
    const filtered = users.filter((u) => {
      if (clientFilterActive && !safeDevices(u).some((d) => (d.app_name || "") === clientFilterTarget)) {
        return false;
      }
      if (!userMatchesCategory(u, category)) return false;
      return matchesSearch(userHaystack(u), q);
    });

    const sorted = [...filtered];
    sorted.sort((a, b) => {
      switch (sortKey) {
        case "ips":
          return b.ip_count - a.ip_count || b.device_count - a.device_count;
        case "online":
          return b.online_count - a.online_count || b.device_count - a.device_count;
        case "name": {
          const an = a.emby_user_name || a.local_user?.username || a.emby_user_id;
          const bn = b.emby_user_name || b.local_user?.username || b.emby_user_id;
          return an.localeCompare(bn);
        }
        case "activity":
          return (b.last_activity || "").localeCompare(a.last_activity || "");
        case "devices":
        default:
          return b.device_count - a.device_count || b.ip_count - a.ip_count;
      }
    });
    return sorted;
  }, [data, q, category, clientFilterActive, clientFilterTarget, sortKey]);

  // 设备视图：把所有用户的设备拍平成行，逐行套同一套筛选——客户端筛选在这里是
  // 行级精确匹配，不存在「用户筛进来、设备表却混着别的客户端」的歧义。
  const visibleDevices = useMemo(() => {
    if (!data) return [] as { user: EmbyAuditUser; device: EmbyAuditDevice; rowKey: string }[];
    const rows: { user: EmbyAuditUser; device: EmbyAuditDevice; rowKey: string }[] = [];
    (data.users ?? []).forEach((u, ui) => {
      const uk = userKey(u, ui);
      safeDevices(u).forEach((d, di) => {
        if (clientFilterActive && (d.app_name || "") !== clientFilterTarget) return;
        if (!deviceMatchesCategory(u, d, category)) return;
        if (!matchesSearch(deviceHaystack(u, d), q)) return;
        rows.push({ user: u, device: d, rowKey: `${uk}::${d.device_id || `idx:${di}`}` });
      });
    });
    rows.sort((a, b) => {
      switch (sortKey) {
        case "name":
          return (a.device.device_name || "").localeCompare(b.device.device_name || "");
        case "online":
          return Number(b.device.online) - Number(a.device.online) ||
            (b.device.last_activity || "").localeCompare(a.device.last_activity || "");
        case "ips":
          return b.user.ip_count - a.user.ip_count ||
            (b.device.last_activity || "").localeCompare(a.device.last_activity || "");
        case "devices":
          return b.user.device_count - a.user.device_count ||
            (b.device.last_activity || "").localeCompare(a.device.last_activity || "");
        case "activity":
        default:
          return (b.device.last_activity || "").localeCompare(a.device.last_activity || "");
      }
    });
    return rows;
  }, [data, q, category, clientFilterActive, clientFilterTarget, sortKey]);

  const renderToolbar = () => (
    <Card>
      <CardContent className="flex flex-col gap-3 p-4">
        <div className="flex flex-col gap-2 lg:flex-row lg:items-center">
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={t("deviceAudit.searchPlaceholder")}
              className="pl-9"
            />
          </div>
          <div className="inline-flex shrink-0 rounded-md border p-0.5">
            <Button
              size="sm"
              variant={view === "users" ? "default" : "ghost"}
              className="h-8"
              onClick={() => setView("users")}
            >
              <UsersIcon className="mr-1.5 h-4 w-4" />
              {t("deviceAudit.viewByUser")}
            </Button>
            <Button
              size="sm"
              variant={view === "devices" ? "default" : "ghost"}
              className="h-8"
              onClick={() => setView("devices")}
            >
              <LayoutList className="mr-1.5 h-4 w-4" />
              {t("deviceAudit.viewByDevice")}
            </Button>
          </div>
        </div>

        <div className="grid gap-2 sm:grid-cols-3">
          <Select value={category} onValueChange={(value) => setCategory(value as CategoryKey)}>
            <SelectTrigger>
              <SelectValue placeholder={t("deviceAudit.filterLabel")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("deviceAudit.filterAll")}</SelectItem>
              <SelectItem value="linked">{t("deviceAudit.filterLinked")}</SelectItem>
              <SelectItem value="unlinked">{t("deviceAudit.filterUnlinked")}</SelectItem>
              <SelectItem value="online">{t("deviceAudit.filterOnline")}</SelectItem>
              <SelectItem value="multiDevice">{t("deviceAudit.filterMultiDevice")}</SelectItem>
              <SelectItem value="multiIp">{t("deviceAudit.filterMultiIp")}</SelectItem>
            </SelectContent>
          </Select>
          <Select value={clientFilter} onValueChange={setClientFilter}>
            <SelectTrigger>
              <SelectValue placeholder={t("deviceAudit.clientFilterLabel")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={CLIENT_ALL}>{t("deviceAudit.clientFilterAll")}</SelectItem>
              {summary?.clients.map((c) => (
                <SelectItem key={clientFilterValue(c.name)} value={clientFilterValue(c.name)}>
                  {(c.name || t("deviceAudit.clientUnknown")) + " · " + c.devices}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={sortKey} onValueChange={(value) => setSortKey(value as SortKey)}>
            <SelectTrigger>
              <SelectValue placeholder={t("deviceAudit.sortLabel")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="devices">{t("deviceAudit.sortDevices")}</SelectItem>
              <SelectItem value="ips">{t("deviceAudit.sortIps")}</SelectItem>
              <SelectItem value="online">{t("deviceAudit.sortOnline")}</SelectItem>
              <SelectItem value="activity">{t("deviceAudit.sortActivity")}</SelectItem>
              <SelectItem value="name">{t("deviceAudit.sortName")}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {summary && (
          <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
            <Badge variant="outline" className="gap-1">
              <UsersIcon className="h-3 w-3" />
              {t("deviceAudit.statUsers", { count: summary.total_users })}
            </Badge>
            <Badge variant="outline" className="gap-1">
              <MonitorSmartphone className="h-3 w-3" />
              {t("deviceAudit.statDevices", { count: summary.total_devices })}
            </Badge>
            <Badge variant="outline" className="gap-1">
              <Wifi className="h-3 w-3" />
              {t("deviceAudit.statOnline", { count: summary.online_devices })}
            </Badge>
            <Badge variant="outline" className="gap-1">
              <Network className="h-3 w-3" />
              {t("deviceAudit.statIps", { count: summary.total_ips })}
            </Badge>
            {!summary.activity_available && (
              <Badge variant="outline" className="border-amber-500/30 text-amber-500">
                {t("deviceAudit.activityUnavailable")}
              </Badge>
            )}
          </div>
        )}

        {summary && summary.clients.length > 0 && (
          <div className="flex flex-col gap-1.5">
            <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
              <AppWindow className="h-3.5 w-3.5" />
              {t("deviceAudit.clientsTitle")}
            </div>
            <div className="flex flex-wrap gap-1.5">
              {summary.clients.filter((c) => {
                const name = (c.name || "").toLowerCase();
                return !name.includes("twilight");
              }).map((c) => {
                const value = clientFilterValue(c.name);
                const active = clientFilter === value;
                const label = c.name || t("deviceAudit.clientUnknown");
                return (
                  <button
                    key={value}
                    type="button"
                    onClick={() => setClientFilter(active ? CLIENT_ALL : value)}
                    title={t("deviceAudit.clientChipTitle", {
                      devices: c.devices,
                      online: c.online,
                      users: c.users,
                    })}
                    className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors ${
                      active
                        ? "border-primary bg-primary/10 text-primary"
                        : "border-border text-muted-foreground hover:bg-muted/60"
                    }`}
                  >
                    <span className="font-medium">{label}</span>
                    <span className="rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-semibold text-foreground/70">
                      {c.devices}
                    </span>
                    {c.online > 0 && (
                      <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" aria-hidden />
                    )}
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {clientFilterActive && (
          <div className="flex items-center gap-2 rounded-md border border-primary/30 bg-primary/5 px-3 py-2 text-xs text-primary">
            <AppWindow className="h-3.5 w-3.5 shrink-0" />
            <span className="flex-1">
              {t("deviceAudit.clientFilterActiveHint", { name: clientFilterLabel })}
            </span>
            <Button
              size="sm"
              variant="ghost"
              className="h-6 px-2 text-primary hover:text-primary"
              onClick={() => setClientFilter(CLIENT_ALL)}
            >
              <X className="mr-1 h-3 w-3" />
              {t("deviceAudit.clientFilterAll")}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className={`flex items-center gap-2 font-bold ${embedded ? "text-xl" : "text-2xl"}`}>
            <MonitorSmartphone className="h-5 w-5" />
            {t("deviceAudit.title")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">{t("deviceAudit.description")}</p>
          <div className="mt-2 flex flex-wrap gap-2 text-xs text-muted-foreground">
            {autoRefresh && <span>{t("deviceAuditPatch.autoRefreshHint")}</span>}
            {lastRefreshAt && <span>{t("deviceAuditPatch.refreshedAt", { time: new Date(lastRefreshAt).toLocaleTimeString(locale) })}</span>}
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <label className="flex min-h-9 items-center gap-2 rounded-md border border-border/60 px-3 text-xs">
            <Switch checked={autoRefresh} onCheckedChange={setAutoRefresh} />
            <span>{t("deviceAuditPatch.autoRefresh")}</span>
          </label>
          <Button variant="destructive" size="sm" onClick={async () => {
            const ok = await confirmAction({
              title: t("deviceAudit.kickAllTitle"),
              description: t("deviceAudit.kickAllDesc"),
              tone: "danger",
              confirmLabel: t("deviceAudit.kickAllConfirm"),
            });
            if (!ok) return;
            try {
              const res = await api.kickAllEmbySessions();
              if (res.success) {
                const data: any = res.data || {};
                toast({
                  title: t("deviceAudit.kickAllDone"),
                  description: t("deviceAuditPatch.kickAllDone", {
                    kicked: data.kicked ?? 0,
                    devices: data.deleted_devices ?? 0,
                  }),
                  variant: "success",
                });
                void reload(true);
              } else {
                toast({ title: t("deviceAudit.actionFailed"), description: res.message, variant: "destructive" });
              }
            } catch (err) {
              toast({ title: t("deviceAudit.actionFailed"), description: err instanceof Error ? err.message : String(err), variant: "destructive" });
            }
          }}>
            <LogOut className="mr-1.5 h-4 w-4" />
            {t("deviceAudit.kickAll")}
          </Button>
          <Button variant="outline" size="sm" onClick={() => void reload(true)} disabled={loading}>
            {loading ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="mr-2 h-4 w-4" />
            )}
            {t("common.refresh")}
          </Button>
        </div>
      </div>

      {/* 分布统计 */}
      {summary && (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <Card>
            <CardContent className="p-3 space-y-2">
              <p className="text-xs font-medium text-muted-foreground">{t("deviceAudit.chartClients")}</p>
              <div className="space-y-1">
                {summary.clients.slice(0, 8).map((c) => {
                  const pct = summary.total_devices > 0 ? Math.round(c.devices / summary.total_devices * 100) : 0;
                  return (
                    <div key={c.name} className="flex items-center gap-2 text-xs">
                      <span className="w-24 truncate shrink-0">{c.name || t("deviceAudit.clientUnknown")}</span>
                      <div className="flex-1 h-3 rounded-full bg-muted overflow-hidden">
                        <div className="h-full rounded-full bg-primary/60" style={{ width: pct + "%" }} />
                      </div>
                      <span className="w-8 text-right text-muted-foreground shrink-0">{pct}%</span>
                    </div>
                  );
                })}
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-3 space-y-2">
              <p className="text-xs font-medium text-muted-foreground">{t("deviceAudit.chartStatus")}</p>
              <div className="space-y-1">
                {(() => {
                  const online = summary.online_devices;
                  const offline = Math.max(0, summary.total_devices - online);
                  const total = online + offline;
                  const bars = [
                    { label: t("deviceAudit.statusOnline"), value: online, color: "bg-emerald-500" },
                    { label: t("deviceAudit.statusOffline"), value: offline, color: "bg-muted-foreground/40" },
                  ];
                  return bars.map((b) => {
                    const pct = total > 0 ? Math.round(b.value / total * 100) : 0;
                    return (
                      <div key={b.label} className="flex items-center gap-2 text-xs">
                        <span className="w-24 truncate shrink-0">{b.label}</span>
                        <div className="flex-1 h-3 rounded-full bg-muted overflow-hidden">
                          <div className={`h-full rounded-full ${b.color}`} style={{ width: pct + "%" }} />
                        </div>
                        <span className="w-8 text-right text-muted-foreground shrink-0">{b.value}</span>
                      </div>
                    );
                  });
                })()}
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-3 space-y-2">
              <p className="text-xs font-medium text-muted-foreground">{t("deviceAudit.chartDevices")}</p>
              <div className="space-y-1">
                {(() => {
                  const modelCounts: Record<string, number> = {};
                  for (const u of data?.users ?? []) {
                    for (const d of safeDevices(u)) {
                      const model = d.device_name?.trim() || t("deviceAudit.unknownDevice");
                      modelCounts[model] = (modelCounts[model] || 0) + 1;
                    }
                  }
                  const sorted = Object.entries(modelCounts).sort((a, b) => b[1] - a[1]).slice(0, 8);
                  const totalDevices = sorted.reduce((s, [, c]) => s + c, 0);
                  return sorted.map(([model, count]) => {
                    const pct = totalDevices > 0 ? Math.round(count / totalDevices * 100) : 0;
                    return (
                      <div key={model} className="flex items-center gap-2 text-xs">
                        <span className="w-24 truncate shrink-0">{model}</span>
                        <div className="flex-1 h-3 rounded-full bg-muted overflow-hidden">
                          <div className="h-full rounded-full bg-info/60" style={{ width: pct + "%" }} />
                        </div>
                        <span className="w-8 text-right text-muted-foreground shrink-0">{count}</span>
                      </div>
                    );
                  });
                })()}
              </div>
            </CardContent>
          </Card>
        </div>
      )}

      {renderToolbar()}

      {/* States + content */}
      {error ? (
        <Card className="border-destructive/40">
          <CardContent className="flex items-center gap-2 p-4 text-sm text-destructive">
            <AlertTriangle className="h-4 w-4" />
            {error}
          </CardContent>
        </Card>
      ) : loading && !data ? (
        <Card>
          <CardContent className="flex items-center justify-center gap-2 p-10 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            {t("common.loading")}
          </CardContent>
        </Card>
      ) : !embyConfigured ? (
        <Card>
          <CardContent className="p-6 text-center text-sm text-muted-foreground">
            {t("deviceAudit.notConfigured")}
          </CardContent>
        </Card>
      ) : view === "devices" ? (
        <DeviceTableView rows={visibleDevices} t={t} locale={locale} />
      ) : visibleUsers.length === 0 ? (
        <Card>
          <CardContent className="p-6 text-center text-sm text-muted-foreground">
            {t("deviceAudit.empty")}
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-2">
          {visibleUsers.map((u, index) => (
            <UserAuditCard
              key={userKey(u, index)}
              user={u}
              devices={devicesForUser(u)}
              clientFiltered={clientFilterActive}
              clientLabel={clientFilterLabel}
              open={expanded.has(userKey(u, index))}
              onToggle={() => toggleExpand(userKey(u, index))}
              onWebToggle={onWebToggle}
              onEmbyToggle={onEmbyToggle}
              onKick={onKick}
              t={t}
              locale={locale}
            />
          ))}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// 用户视图：单个可展开卡片
// ---------------------------------------------------------------------------

function UserAuditCard({
  user: u,
  devices,
  clientFiltered,
  clientLabel,
  open,
  onToggle,
  onWebToggle,
  onEmbyToggle,
  onKick,
  t,
  locale,
}: {
  user: EmbyAuditUser;
  devices: EmbyAuditDevice[];
  clientFiltered: boolean;
  clientLabel: string;
  open: boolean;
  onToggle: () => void;
  onWebToggle: (u: EmbyAuditUser, enable: boolean) => void;
  onEmbyToggle: (u: EmbyAuditUser, enable: boolean) => void;
  onKick: (u: EmbyAuditUser) => void;
  t: TFunc;
  locale: string;
}) {
  const displayName = u.emby_user_name || u.local_user?.username || t("deviceAudit.unknownUser");
  // 客户端筛选生效时，头部计数与设备表都按子集，避免「头部按全部、表格按子集」的不一致。
  const deviceCount = clientFiltered ? devices.length : u.device_count;
  const onlineCount = clientFiltered ? devices.filter((d) => d.online).length : u.online_count;

  return (
    <Card className="overflow-hidden">
      <button
        type="button"
        onClick={onToggle}
        className="flex w-full items-center gap-3 p-4 text-left transition-colors hover:bg-muted/40"
        aria-expanded={open}
      >
        {open ? (
          <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
        ) : (
          <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
        )}
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium">{displayName}</span>
            {u.local_user ? (
              <span className="text-xs text-muted-foreground">
                {u.local_user.username} · UID {u.local_user.uid}
              </span>
            ) : (
              <Badge variant="secondary" className="text-[10px]">
                {t("deviceAudit.unlinked")}
              </Badge>
            )}
            {u.local_user && u.local_user.role === 0 && (
              <Badge className="border-purple-500/20 bg-purple-500/10 text-purple-500">
                <Shield className="mr-1 h-3 w-3" />
                {t("deviceAudit.roleAdmin")}
              </Badge>
            )}
            {u.local_user && !u.local_user.active && (
              <Badge variant="destructive" className="text-[10px]">
                {t("deviceAudit.statusDisabled")}
              </Badge>
            )}
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-1.5">
          {clientFiltered && (
            <Badge variant="secondary" className="gap-1 text-[10px]">
              <AppWindow className="h-3 w-3" />
              {clientLabel}
            </Badge>
          )}
          <Badge variant="outline" className="text-xs">
            {t("deviceAudit.deviceCountBadge", { count: deviceCount })}
          </Badge>
          <Badge variant="outline" className="text-xs">
            {t("deviceAudit.ipCountBadge", { count: u.ip_count })}
          </Badge>
          {onlineCount > 0 && (
            <Badge className="border-emerald-500/20 bg-emerald-500/10 text-xs text-emerald-500">
              {t("deviceAudit.onlineBadge", { count: onlineCount })}
            </Badge>
          )}
        </div>
      </button>

      {open && (
        <div className="space-y-4 border-t bg-muted/20 p-4">
          {/* 快速处置：禁用/启用 Web（仅已关联本地账号）、禁用/启用 Emby、踢出会话 */}
          <div className="flex flex-wrap items-center gap-2 rounded-md border border-dashed bg-background p-2.5">
            <span className="mr-1 text-xs font-medium text-muted-foreground">
              {t("deviceAudit.quickActions")}
            </span>
            {u.local_user &&
              (u.local_user.active ? (
                <Button size="sm" variant="outline" className="h-7" onClick={() => onWebToggle(u, false)}>
                  <Ban className="mr-1.5 h-3.5 w-3.5" />
                  {t("deviceAudit.actionDisableWeb")}
                </Button>
              ) : (
                <Button size="sm" variant="outline" className="h-7" onClick={() => onWebToggle(u, true)}>
                  <UserCheck className="mr-1.5 h-3.5 w-3.5" />
                  {t("deviceAudit.actionEnableWeb")}
                </Button>
              ))}
            <Button size="sm" variant="outline" className="h-7" onClick={() => onEmbyToggle(u, false)}>
              <MonitorX className="mr-1.5 h-3.5 w-3.5" />
              {t("deviceAudit.actionDisableEmby")}
            </Button>
            <Button size="sm" variant="outline" className="h-7" onClick={() => onEmbyToggle(u, true)}>
              <MonitorCheck className="mr-1.5 h-3.5 w-3.5" />
              {t("deviceAudit.actionEnableEmby")}
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="h-7"
              onClick={() => onKick(u)}
              disabled={u.online_count === 0}
            >
              <LogOut className="mr-1.5 h-3.5 w-3.5" />
              {t("deviceAudit.actionKick")}
            </Button>
          </div>
          <div className="grid gap-4 md:grid-cols-3">
            <AccountPanel title={t("deviceAudit.webAccount")}>
              {u.local_user ? (
                <dl className="space-y-1 text-sm">
                  <Row label={t("deviceAudit.fieldUsername")} value={u.local_user.username} />
                  <Row label="UID" value={String(u.local_user.uid)} mono />
                  <Row
                    label={t("deviceAudit.fieldEmail")}
                    value={
                      u.local_user.email ? (
                        <span className="inline-flex items-center gap-1">
                          <span className="break-all">{u.local_user.email}</span>
                          <Badge
                            variant="outline"
                            className={
                              u.local_user.email_verified
                                ? "border-emerald-500/30 text-emerald-500"
                                : "border-amber-500/30 text-amber-500"
                            }
                          >
                            {u.local_user.email_verified
                              ? t("deviceAudit.verified")
                              : t("deviceAudit.unverified")}
                          </Badge>
                        </span>
                      ) : (
                        "—"
                      )
                    }
                  />
                  <Row label={t("deviceAudit.fieldRole")} value={t(roleLabelKey(u.local_user.role))} />
                  <Row
                    label={t("deviceAudit.fieldStatus")}
                    value={
                      u.local_user.active
                        ? t("deviceAudit.statusActive")
                        : t("deviceAudit.statusDisabled")
                    }
                  />
                  <Row
                    label={t("deviceAudit.fieldExpiry")}
                    value={formatUnix(u.local_user.expired_at, locale, t("deviceAudit.permanent"))}
                  />
                  <Row
                    label={t("deviceAudit.fieldRegistered")}
                    value={formatUnix(
                      u.local_user.register_time || u.local_user.created_at,
                      locale,
                      t("deviceAudit.permanent"),
                    )}
                  />
                  {u.local_user.pending_emby && (
                    <Badge variant="outline" className="mt-1 border-amber-500/30 text-amber-500">
                      {t("deviceAudit.pendingEmby")}
                    </Badge>
                  )}
                </dl>
              ) : (
                <p className="text-sm text-muted-foreground">{t("deviceAudit.unlinked")}</p>
              )}
            </AccountPanel>

            <AccountPanel title={t("deviceAudit.embyAccount")}>
              <dl className="space-y-1 text-sm">
                <Row label={t("deviceAudit.fieldEmbyName")} value={u.emby_user_name || "—"} />
                <Row label={t("deviceAudit.fieldEmbyId")} value={u.emby_user_id || "—"} mono />
                {u.local_user?.emby_username && (
                  <Row label={t("deviceAudit.fieldLocalEmbyName")} value={u.local_user.emby_username} />
                )}
                <Row label={t("deviceAudit.lastActivity")} value={formatIso(u.last_activity, locale)} />
              </dl>
            </AccountPanel>

            <AccountPanel title={t("deviceAudit.telegram")}>
              {u.local_user && (u.local_user.telegram_id != null || u.local_user.telegram_username) ? (
                <dl className="space-y-1 text-sm">
                  <Row
                    label={t("deviceAudit.fieldTgId")}
                    value={u.local_user.telegram_id != null ? String(u.local_user.telegram_id) : "—"}
                    mono
                  />
                  <Row
                    label={t("deviceAudit.fieldTgUser")}
                    value={u.local_user.telegram_username ? `@${u.local_user.telegram_username}` : "—"}
                  />
                </dl>
              ) : (
                <p className="text-sm text-muted-foreground">{t("deviceAudit.notBound")}</p>
              )}
            </AccountPanel>
          </div>

          {/* Login IPs（用户级，跨全部设备 + 历史登录聚合，不随客户端筛选收窄） */}
          <div>
            <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              {t("deviceAudit.ipsTitle")}
            </div>
            {(u.ips ?? []).length > 0 ? (
              <div className="flex flex-wrap gap-1.5">
                {(u.ips ?? []).map((ip) => (
                  <Badge key={ip} variant="secondary" className="font-mono text-xs">
                    {ip}
                  </Badge>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">{t("deviceAudit.noIps")}</p>
            )}
          </div>

          {/* Devices（随客户端筛选收窄） */}
          <div>
            <div className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              {t("deviceAudit.devicesTitle")}
              {clientFiltered && (
                <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-normal normal-case text-muted-foreground">
                  {t("deviceAudit.deviceMatchCount", { count: devices.length })}
                </span>
              )}
            </div>
            {devices.length > 0 ? (
              <div className="overflow-hidden rounded-lg border bg-background">
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead className="bg-muted/50">
                      <tr>
                        <th className="p-2.5 text-left font-medium">{t("deviceAudit.devColName")}</th>
                        <th className="p-2.5 text-left font-medium">{t("deviceAudit.devColApp")}</th>
                        <th className="p-2.5 text-left font-medium">{t("deviceAudit.devColIp")}</th>
                        <th className="p-2.5 text-left font-medium">{t("deviceAudit.devColLastActivity")}</th>
                        <th className="p-2.5 text-left font-medium">{t("deviceAudit.devColStatus")}</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y">
                      {devices.map((d, di) => (
                        <tr key={d.device_id || `idx:${di}`} className="hover:bg-muted/30">
                          <td className="p-2.5">
                            <div className="font-medium">{d.device_name || "—"}</div>
                            <div className="font-mono text-[11px] text-muted-foreground">
                              {d.device_id ? (d.device_id.length > 12 ? `${d.device_id.slice(0, 12)}…` : d.device_id) : "—"}
                            </div>
                          </td>
                          <td className="p-2.5">
                            <ClientCell device={d} />
                          </td>
                          <td className="p-2.5">
                            <IpCell device={d} t={t} />
                          </td>
                          <td className="p-2.5 text-xs text-muted-foreground">
                            {formatIso(d.last_activity, locale)}
                          </td>
                          <td className="p-2.5">
                            <StatusCell online={d.online} t={t} />
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">—</p>
            )}
          </div>
        </div>
      )}
    </Card>
  );
}

// ---------------------------------------------------------------------------
// 设备视图：拍平的设备表（客户端筛选在此为行级精确，不存在嵌套歧义）
// ---------------------------------------------------------------------------

function DeviceTableView({
  rows,
  t,
  locale,
}: {
  rows: { user: EmbyAuditUser; device: EmbyAuditDevice; rowKey: string }[];
  t: TFunc;
  locale: string;
}) {
  if (rows.length === 0) {
    return (
      <Card>
        <CardContent className="p-6 text-center text-sm text-muted-foreground">
          {t("deviceAudit.emptyDevices")}
        </CardContent>
      </Card>
    );
  }
  return (
    <Card>
      <CardContent className="p-0">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-muted/50">
              <tr>
                <th className="p-2.5 text-left font-medium">{t("deviceAudit.devColUser")}</th>
                <th className="p-2.5 text-left font-medium">{t("deviceAudit.devColName")}</th>
                <th className="p-2.5 text-left font-medium">{t("deviceAudit.devColApp")}</th>
                <th className="p-2.5 text-left font-medium">{t("deviceAudit.devColIp")}</th>
                <th className="p-2.5 text-left font-medium">{t("deviceAudit.devColLastActivity")}</th>
                <th className="p-2.5 text-left font-medium">{t("deviceAudit.devColStatus")}</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {rows.map(({ user: u, device: d, rowKey }) => {
                const name = u.emby_user_name || u.local_user?.username || t("deviceAudit.unknownUser");
                const count = (d as any).count || 1;
                return (
                  <tr key={rowKey} className="hover:bg-muted/30">
                    <td className="p-2.5">
                      <div className="flex items-center gap-1.5">
                        <span className="font-medium">{name}</span>
                        {u.local_user && u.local_user.role === 0 && (
                          <Shield className="h-3 w-3 text-purple-500" />
                        )}
                      </div>
                      {u.local_user ? (
                        <div className="text-[11px] text-muted-foreground">
                          {u.local_user.username} · UID {u.local_user.uid}
                        </div>
                      ) : (
                        <Badge variant="secondary" className="mt-0.5 text-[10px]">
                          {t("deviceAudit.unlinked")}
                        </Badge>
                      )}
                    </td>
                    <td className="p-2.5">
                      <div className="font-medium">{d.device_name || "—"}</div>
                      {count > 1 && (
                        <Badge variant="secondary" className="mt-0.5 text-[10px]">
                          ×{count}
                        </Badge>
                      )}
                    </td>
                    <td className="p-2.5">
                      <ClientCell device={d} />
                    </td>
                    <td className="p-2.5">
                      <IpCell device={d} t={t} />
                    </td>
                    <td className="p-2.5 text-xs text-muted-foreground">
                      {formatIso(d.last_activity, locale)}
                    </td>
                    <td className="p-2.5">
                      <StatusCell online={d.online} t={t} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// 共享小组件
// ---------------------------------------------------------------------------

function AccountPanel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border bg-background p-3">
      <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {title}
      </div>
      {children}
    </div>
  );
}

function ClientCell({ device: d }: { device: EmbyAuditDevice }) {
  return (
    <span className="inline-flex items-center gap-1">
      <span>{d.app_name || "—"}</span>
      {d.app_version && <span className="text-xs text-muted-foreground">{d.app_version}</span>}
    </span>
  );
}

function IpCell({ device: d, t }: { device: EmbyAuditDevice; t: TFunc }) {
  if (!d.ip) return <span className="font-mono text-muted-foreground">—</span>;
  if (d.ip_approx) {
    return (
      <span
        className="inline-flex items-center gap-1 font-mono text-muted-foreground"
        title={t("deviceAudit.ipApproxTooltip")}
      >
        <span>~{d.ip}</span>
        <span className="rounded bg-amber-500/10 px-1 py-0.5 text-[9px] not-italic text-amber-600">
          {t("deviceAudit.ipApproxTag")}
        </span>
      </span>
    );
  }
  return <span className="font-mono">{d.ip}</span>;
}

function StatusCell({ online, t }: { online: boolean; t: TFunc }) {
  return online ? (
    <Badge className="border-emerald-500/20 bg-emerald-500/10 text-emerald-500">
      {t("deviceAudit.online")}
    </Badge>
  ) : (
    <Badge variant="secondary">{t("deviceAudit.offline")}</Badge>
  );
}

function Row({
  label,
  value,
  mono,
}: {
  label: string;
  value: React.ReactNode;
  mono?: boolean;
}) {
  return (
    <div className="flex justify-between gap-3">
      <dt className="shrink-0 text-muted-foreground">{label}</dt>
      <dd className={`text-right ${mono ? "font-mono text-xs" : ""}`}>{value}</dd>
    </div>
  );
}
