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
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
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
import { useI18n, type MessageKey } from "@/lib/i18n";
import { api } from "@/lib/api";
import type { EmbyAuditUser, EmbyDeviceAuditData } from "@/lib/api-types";

type SortKey = "devices" | "ips" | "online" | "activity" | "name";
type CategoryKey = "all" | "linked" | "unlinked" | "online" | "multiDevice" | "multiIp";

// 与后端 permanentExpiryUnix 同量级：超过该阈值视为「永久有效」。
const PERMANENT_THRESHOLD = 253402214400;

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

export default function AdminDeviceAuditPage() {
  const { t, locale } = useI18n();
  const { toast } = useToast();

  const [data, setData] = useState<EmbyDeviceAuditData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState("");
  const [category, setCategory] = useState<CategoryKey>("all");
  const [sortKey, setSortKey] = useState<SortKey>("devices");
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  const reload = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.adminGetEmbyDeviceAudit();
      if (res.success && res.data) {
        setData(res.data);
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

  const toggleExpand = useCallback((id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const visibleUsers = useMemo<EmbyAuditUser[]>(() => {
    if (!data) return [];
    const q = search.trim().toLowerCase();
    const filtered = data.users.filter((u) => {
      // 归类
      switch (category) {
        case "linked":
          if (!u.local_user) return false;
          break;
        case "unlinked":
          if (u.local_user) return false;
          break;
        case "online":
          if (u.online_count <= 0) return false;
          break;
        case "multiDevice":
          if (u.device_count < 2) return false;
          break;
        case "multiIp":
          if (u.ip_count < 2) return false;
          break;
        default:
          break;
      }
      // 搜索：Emby 名 / EmbyID / 本地用户名 / UID / 邮箱 / Telegram / 任意 IP
      if (q) {
        const haystacks = [
          u.emby_user_name,
          u.emby_user_id,
          u.local_user?.username ?? "",
          u.local_user ? String(u.local_user.uid) : "",
          u.local_user?.email ?? "",
          u.local_user?.telegram_username ?? "",
          u.local_user?.telegram_id != null ? String(u.local_user.telegram_id) : "",
          ...u.ips,
        ];
        if (!haystacks.some((h) => h.toLowerCase().includes(q))) return false;
      }
      return true;
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
  }, [data, search, category, sortKey]);

  const summary = data?.summary;
  const embyConfigured = data?.emby_configured ?? true;

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold">
            <MonitorSmartphone className="h-5 w-5" />
            {t("deviceAudit.title")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">{t("deviceAudit.description")}</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void reload()} disabled={loading}>
          {loading ? (
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <RefreshCw className="mr-2 h-4 w-4" />
          )}
          {t("common.refresh")}
        </Button>
      </div>

      {/* Toolbar: search + categorize + sort + summary */}
      <Card>
        <CardContent className="flex flex-col gap-3 p-4">
          <div className="grid gap-2 lg:grid-cols-[minmax(220px,1fr)_200px_200px]">
            <div className="relative">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder={t("deviceAudit.searchPlaceholder")}
                className="pl-9"
              />
            </div>
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
        </CardContent>
      </Card>

      {/* States */}
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
      ) : visibleUsers.length === 0 ? (
        <Card>
          <CardContent className="p-6 text-center text-sm text-muted-foreground">
            {t("deviceAudit.empty")}
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-2">
          {visibleUsers.map((u, index) => {
            const key = userKey(u, index);
            const isOpen = expanded.has(key);
            const displayName = u.emby_user_name || u.local_user?.username || t("deviceAudit.unknownUser");
            return (
              <Card key={key} className="overflow-hidden">
                {/* Collapsed header (clickable) */}
                <button
                  type="button"
                  onClick={() => toggleExpand(key)}
                  className="flex w-full items-center gap-3 p-4 text-left transition-colors hover:bg-muted/40"
                  aria-expanded={isOpen}
                >
                  {isOpen ? (
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
                    <Badge variant="outline" className="text-xs">
                      {t("deviceAudit.deviceCountBadge", { count: u.device_count })}
                    </Badge>
                    <Badge variant="outline" className="text-xs">
                      {t("deviceAudit.ipCountBadge", { count: u.ip_count })}
                    </Badge>
                    {u.online_count > 0 && (
                      <Badge className="border-emerald-500/20 bg-emerald-500/10 text-xs text-emerald-500">
                        {t("deviceAudit.onlineBadge", { count: u.online_count })}
                      </Badge>
                    )}
                  </div>
                </button>

                {/* Expanded details */}
                {isOpen && (
                  <div className="border-t bg-muted/20 p-4 space-y-4">
                    {/* Account info grids */}
                    <div className="grid gap-4 md:grid-cols-3">
                      {/* Web account */}
                      <div className="rounded-lg border bg-background p-3">
                        <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                          {t("deviceAudit.webAccount")}
                        </div>
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
                      </div>

                      {/* Emby account */}
                      <div className="rounded-lg border bg-background p-3">
                        <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                          {t("deviceAudit.embyAccount")}
                        </div>
                        <dl className="space-y-1 text-sm">
                          <Row label={t("deviceAudit.fieldEmbyName")} value={u.emby_user_name || "—"} />
                          <Row
                            label={t("deviceAudit.fieldEmbyId")}
                            value={u.emby_user_id || "—"}
                            mono
                          />
                          {u.local_user?.emby_username && (
                            <Row label={t("deviceAudit.fieldLocalEmbyName")} value={u.local_user.emby_username} />
                          )}
                          <Row
                            label={t("deviceAudit.lastActivity")}
                            value={formatIso(u.last_activity, locale)}
                          />
                        </dl>
                      </div>

                      {/* Telegram */}
                      <div className="rounded-lg border bg-background p-3">
                        <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                          {t("deviceAudit.telegram")}
                        </div>
                        {u.local_user && (u.local_user.telegram_id != null || u.local_user.telegram_username) ? (
                          <dl className="space-y-1 text-sm">
                            <Row
                              label={t("deviceAudit.fieldTgId")}
                              value={u.local_user.telegram_id != null ? String(u.local_user.telegram_id) : "—"}
                              mono
                            />
                            <Row
                              label={t("deviceAudit.fieldTgUser")}
                              value={
                                u.local_user.telegram_username ? `@${u.local_user.telegram_username}` : "—"
                              }
                            />
                          </dl>
                        ) : (
                          <p className="text-sm text-muted-foreground">{t("deviceAudit.notBound")}</p>
                        )}
                      </div>
                    </div>

                    {/* Login IPs */}
                    <div>
                      <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                        {t("deviceAudit.ipsTitle")}
                      </div>
                      {u.ips.length > 0 ? (
                        <div className="flex flex-wrap gap-1.5">
                          {u.ips.map((ip) => (
                            <Badge key={ip} variant="secondary" className="font-mono text-xs">
                              {ip}
                            </Badge>
                          ))}
                        </div>
                      ) : (
                        <p className="text-sm text-muted-foreground">{t("deviceAudit.noIps")}</p>
                      )}
                    </div>

                    {/* Devices */}
                    <div>
                      <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                        {t("deviceAudit.devicesTitle")}
                      </div>
                      {u.devices.length > 0 ? (
                        <div className="overflow-hidden rounded-lg border bg-background">
                          <div className="overflow-x-auto">
                            <table className="w-full text-sm">
                              <thead className="bg-muted/50">
                                <tr>
                                  <th className="p-2 text-left font-medium">{t("deviceAudit.devColName")}</th>
                                  <th className="p-2 text-left font-medium">{t("deviceAudit.devColApp")}</th>
                                  <th className="p-2 text-left font-medium">{t("deviceAudit.devColIp")}</th>
                                  <th className="p-2 text-left font-medium">{t("deviceAudit.devColLastActivity")}</th>
                                  <th className="p-2 text-left font-medium">{t("deviceAudit.devColStatus")}</th>
                                </tr>
                              </thead>
                              <tbody className="divide-y">
                                {u.devices.map((d) => (
                                  <tr key={d.device_id} className="hover:bg-muted/30">
                                    <td className="p-2">
                                      <div className="font-medium">{d.device_name || "—"}</div>
                                      <div className="font-mono text-[11px] text-muted-foreground">
                                        {d.device_id ? `${d.device_id.slice(0, 12)}…` : "—"}
                                      </div>
                                    </td>
                                    <td className="p-2">
                                      <span>{d.app_name || "—"}</span>
                                      {d.app_version && (
                                        <span className="ml-1 text-xs text-muted-foreground">{d.app_version}</span>
                                      )}
                                    </td>
                                    <td className="p-2 font-mono">{d.ip || "—"}</td>
                                    <td className="p-2 text-xs text-muted-foreground">
                                      {formatIso(d.last_activity, locale)}
                                    </td>
                                    <td className="p-2">
                                      {d.online ? (
                                        <Badge className="border-emerald-500/20 bg-emerald-500/10 text-emerald-500">
                                          {t("deviceAudit.online")}
                                        </Badge>
                                      ) : (
                                        <Badge variant="secondary">{t("deviceAudit.offline")}</Badge>
                                      )}
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
          })}
        </div>
      )}
    </div>
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
