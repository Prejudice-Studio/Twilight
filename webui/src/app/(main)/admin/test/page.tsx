"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Activity, Database, Loader2, RefreshCw, Server, ShieldCheck, XCircle } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { api, type SystemStats } from "@/lib/api";
import { useToast } from "@/hooks/use-toast";
import { useI18n, type MessageKey } from "@/lib/i18n";
import type { SystemHealthDetail } from "@/lib/api-types";

const FEATURE_LABELS: Record<string, string | MessageKey> = {
  register: "adminStatus.featureRegister",
  telegram: "Telegram Bot",
  force_bind_telegram: "adminStatus.featureForceTelegram",
};

const LIMIT_LABELS: Record<string, MessageKey> = {
  user_limit: "adminStatus.limitUsers",
  stream_limit: "adminStatus.limitStreams",
};

interface SystemHealthInfo {
  api?: SystemHealthDetail;
  database?: SystemHealthDetail;
  emby?: SystemHealthDetail;
}

interface ExtendedSystemStats extends SystemStats {
  emby?: {
    active_sessions?: number;
    online?: boolean;
    operating_system?: string;
    server_name?: string;
    total_sessions?: number;
    version?: string;
  };
  regcodes?: {
    active?: number;
    total?: number;
  };
  users?: {
    active?: number;
    limit?: number | null;
    total?: number;
    usage_percent?: number;
  };
}

export default function AdminStatusPage() {
  const { toast } = useToast();
  const { t } = useI18n();
  const [health, setHealth] = useState<SystemHealthInfo | null>(null);
  const [info, setInfo] = useState<any>(null);
  const [stats, setStats] = useState<ExtendedSystemStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const loadAbortRef = useRef<AbortController | null>(null);
  const loadSequenceRef = useRef(0);

  const loadStatus = useCallback(async () => {
    loadAbortRef.current?.abort();
    const controller = new AbortController();
    loadAbortRef.current = controller;
    const sequence = ++loadSequenceRef.current;
    setLoading(true);
    setError(null);

    try {
      const [apiHealthRes, databaseHealthRes, embyHealthRes, infoRes, statsRes] = await Promise.allSettled([
        api.getSystemHealthApi(controller.signal),
        api.getSystemHealthDatabase(controller.signal),
        api.getSystemHealthEmby(controller.signal),
        api.getSystemInfo(controller.signal),
        api.getSystemStats(controller.signal),
      ]);

      if (controller.signal.aborted || sequence !== loadSequenceRef.current) return;
      const errors: string[] = [];
      const read = <T,>(result: PromiseSettledResult<{ success: boolean; message: string; data?: T }>, label: string) => {
        if (result.status === "rejected") {
          errors.push(`${label}: ${result.reason instanceof Error ? result.reason.message : t("adminStatus.loadError")}`);
          return undefined;
        }
        if (!result.value.success || !result.value.data) {
          errors.push(`${label}: ${result.value.message || t("adminStatus.loadError")}`);
          return undefined;
        }
        return result.value.data;
      };
      const nextApiHealth = read(apiHealthRes, t("adminStatus.apiService"));
      const nextDatabaseHealth = read(databaseHealthRes, t("adminLogs.database"));
      const nextEmbyHealth = read(embyHealthRes, t("adminStatus.embyService"));
      const nextInfo = read(infoRes, t("adminStatus.systemArchitecture"));
      const nextStats = read(statsRes, t("adminStatus.systemStats"));

      setHealth({
        api: nextApiHealth,
        database: nextDatabaseHealth,
        emby: nextEmbyHealth,
      });
      setInfo(nextInfo || null);
      setStats(nextStats || null);
      if (errors.length > 0) {
        const summary = errors.join("；");
        setError(summary);
        toast({ title: t("adminStatus.partialUpdate"), description: summary, variant: "destructive" });
      } else {
        toast({ title: t("adminStatus.updated"), variant: "success" });
      }
    } catch (err: unknown) {
      if (controller.signal.aborted || sequence !== loadSequenceRef.current) return;
      const message = err instanceof Error ? err.message : t("adminStatus.loadError");
      setError(message);
      toast({ title: t("adminStatus.loadError"), description: message, variant: "destructive" });
    } finally {
      if (loadAbortRef.current === controller) {
        loadAbortRef.current = null;
        if (sequence === loadSequenceRef.current) setLoading(false);
      }
    }
  }, [t, toast]);

  const embyHealth = health?.emby;

  useEffect(() => {
    void loadStatus();
    return () => loadAbortRef.current?.abort();
  }, [loadStatus]);

  const renderStatusItem = (flag: boolean | undefined, label: string) => {
    const available = typeof flag === "boolean";
    const healthy = flag === true;
    return (
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border p-4">
        <div className="flex items-center gap-3">
          <div className={`grid h-10 w-10 place-items-center rounded-xl ${healthy ? "bg-emerald-500/10" : available ? "bg-destructive/10" : "bg-muted"}`}>
            {healthy ? <ShieldCheck className="h-5 w-5 text-emerald-500" /> : available ? <XCircle className="h-5 w-5 text-destructive" /> : <Activity className="h-5 w-5 text-muted-foreground" />}
          </div>
          <div>
            <p className="font-medium">{label}</p>
            <p className="text-sm text-muted-foreground">{!available ? t("adminStatus.unavailable") : healthy ? t("adminStatus.healthy") : t("adminStatus.unhealthy")}</p>
          </div>
        </div>
        <Badge variant={!available ? "outline" : healthy ? "success" : "destructive"}>{!available ? "N/A" : healthy ? "OK" : "FAIL"}</Badge>
      </div>
    );
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-3xl font-bold">{t("adminStatus.title")}</h1>
          <p className="text-muted-foreground">{t("adminStatus.description")}</p>
        </div>
        <Button className="w-full sm:w-auto" onClick={() => void loadStatus()} disabled={loading}>
          {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
          {t("adminStatus.refresh")}
        </Button>
      </div>

      {error && (
        <Card className="border border-destructive/30 bg-destructive/5">
          <CardContent>
            <p className="text-destructive font-medium">{error}</p>
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 xl:grid-cols-3">
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Activity className="h-5 w-5" />
              {t("adminStatus.healthTitle")}
            </CardTitle>
            <CardDescription>{t("adminStatus.healthDescription")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {health ? (
              <div className="space-y-3">
                {renderStatusItem(health.api?.ok, t("adminStatus.apiService"))}
                {renderStatusItem(health.database?.ok, t("adminLogs.database"))}
                {renderStatusItem(health.emby?.online, t("adminStatus.embyService"))}
              </div>
            ) : (
              <div className="flex h-40 items-center justify-center">
                <Loader2 className="h-8 w-8 animate-spin text-primary" />
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Server className="h-5 w-5" />
              {t("adminStatus.systemArchitecture")}
            </CardTitle>
            <CardDescription>{t("adminStatus.systemArchitectureDescription")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {info ? (
              <div className="space-y-4 text-sm text-muted-foreground">
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                    <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">{t("adminStatus.serviceName")}</p>
                    <p className="mt-2 text-base font-semibold text-foreground">{info.name ?? t("adminStats.unknown")}</p>
                  </div>
                  <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                    <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">{t("adminStatus.version")}</p>
                    <p className="mt-2 text-base font-semibold text-foreground">{info.version ?? t("adminStats.unknown")}</p>
                  </div>
                  <div className="rounded-xl border border-muted/70 bg-background/80 p-4 sm:col-span-2">
                    <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">{t("adminStatus.icon")}</p>
                    <p className="mt-2 text-base font-semibold text-foreground">{info.icon || t("adminStatus.none")}</p>
                  </div>
                </div>

                <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                  <p className="text-sm font-medium text-foreground">{t("adminStatus.features")}</p>
                  <div className="mt-3 grid gap-2">
                    {Object.entries(info.features || {}).map(([name, enabled]) => (
                      <div key={name} className="flex items-center justify-between rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                        <span className="text-sm">{FEATURE_LABELS[name]?.includes(".") ? t(FEATURE_LABELS[name] as MessageKey) : FEATURE_LABELS[name] ?? name}</span>
                        <Badge variant={enabled ? "success" : "outline"}>
                          {enabled ? t("adminStatus.on") : t("adminStatus.off")}
                        </Badge>
                      </div>
                    ))}
                  </div>
                </div>

                {info.limits && (
                  <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                    <p className="text-sm font-medium text-foreground">{t("adminStatus.limits")}</p>
                    <div className="mt-3 grid gap-2">
                      {Object.entries(info.limits).map(([name, value]) => (
                        <div key={name} className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                          <span className="text-sm">{LIMIT_LABELS[name] ? t(LIMIT_LABELS[name]) : name}</span>
                          <span>{value == null ? t("invite.unlimited") : String(value)}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            ) : (
              <div className="flex h-40 items-center justify-center">
                <Loader2 className="h-8 w-8 animate-spin text-primary" />
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Database className="h-5 w-5" />
              {t("adminStatus.systemStats")}
            </CardTitle>
            <CardDescription>{t("adminStatus.systemStatsDescription")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {stats ? (
              <div className="space-y-4 text-sm text-muted-foreground">
                <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                  <p className="text-sm font-medium text-foreground">{t("dashboard.embyServer")}</p>
                  <div className="mt-3 grid gap-2">
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>{t("adminStatus.serverName")}</span>
                      <span>{embyHealth?.server_name ?? t("adminStats.unknown")}</span>
                    </div>
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>{t("adminStatus.version")}</span>
                      <span>{embyHealth?.version ?? t("adminStats.unknown")}</span>
                    </div>
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>{t("adminStatus.onlineStatus")}</span>
                      <Badge variant={embyHealth?.online ? "success" : "destructive"}>
                        {embyHealth?.online ? t("dashboard.online") : t("dashboard.offline")}
                      </Badge>
                    </div>
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>{t("adminStatus.activeSessions")}</span>
                      <span>{embyHealth?.active_sessions ?? 0}</span>
                    </div>
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>{t("adminStatus.totalSessions")}</span>
                      <span>{embyHealth?.total_sessions ?? 0}</span>
                    </div>
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>{t("adminStatus.platform")}</span>
                      <span>{embyHealth?.operating_system ?? t("adminStats.unknown")}</span>
                    </div>
                  </div>
                </div>

                <div className="grid gap-4 lg:grid-cols-2">
                  <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                    <p className="text-sm font-medium text-foreground">{t("adminStatus.regcodeStats")}</p>
                    <div className="mt-3 space-y-2 text-sm text-muted-foreground">
                      <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                        <span>{t("adminStatus.active")}</span>
                        <span>{stats.regcodes?.active ?? 0}</span>
                      </div>
                      <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                        <span>{t("adminStatus.total")}</span>
                        <span>{stats.regcodes?.total ?? 0}</span>
                      </div>
                    </div>
                  </div>
                  <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                    <p className="text-sm font-medium text-foreground">{t("adminStatus.userStats")}</p>
                    <div className="mt-3 space-y-2 text-sm text-muted-foreground">
                      <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                        <span>{t("adminStatus.activeUsers")}</span>
                        <span>{stats.users?.active ?? 0}</span>
                      </div>
                      <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                        <span>{t("adminStatus.totalUsers")}</span>
                        <span>{stats.users?.total ?? 0}</span>
                      </div>
                      <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                        <span>{t("adminStatus.limit")}</span>
                        <span>{stats.users?.limit == null ? t("invite.unlimited") : stats.users.limit}</span>
                      </div>
                      <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                        <span>{t("adminStatus.usage")}</span>
                        <span>{stats.users?.usage_percent != null ? `${stats.users.usage_percent}%` : t("adminStats.unknown")}</span>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            ) : (
              <div className="flex h-40 items-center justify-center">
                <Loader2 className="h-8 w-8 animate-spin text-primary" />
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
