"use client";

import { useCallback, useEffect, useState } from "react";
import { Activity, Database, Loader2, RefreshCw, Server, ShieldCheck, XCircle } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { api, type SystemStats } from "@/lib/api";
import { useToast } from "@/hooks/use-toast";
import { useI18n, type MessageKey } from "@/lib/i18n";

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
  api: boolean;
  database: boolean;
  emby: boolean;
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

  const loadStatus = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const [healthRes, infoRes, statsRes] = await Promise.all([
        api.getSystemHealth(),
        api.getSystemInfo(),
        api.getSystemStats(),
      ]);

      if (!healthRes.success) {
        throw new Error(healthRes.message || t("adminStatus.healthFailed"));
      }
      if (!infoRes.success) {
        throw new Error(infoRes.message || t("adminStatus.infoFailed"));
      }
      if (!statsRes.success) {
        throw new Error(statsRes.message || t("adminStatus.statsFailed"));
      }

      setHealth(healthRes.data || null);
      setInfo(infoRes.data || null);
      setStats(statsRes.data || null);
      toast({ title: t("adminStatus.updated"), variant: "success" });
    } catch (err: any) {
      setError(err?.message || t("adminStatus.loadError"));
      toast({ title: t("adminLogs.loadFailed"), description: err?.message || String(err), variant: "destructive" });
    } finally {
      setLoading(false);
    }
  }, [t, toast]);

  useEffect(() => {
    void loadStatus();
  }, [loadStatus]);

  const renderStatusItem = (flag: boolean | undefined, label: string) => {
    const healthy = Boolean(flag);
    return (
      <div className="flex items-center justify-between gap-3 rounded-xl border p-4">
        <div className="flex items-center gap-3">
          <div className={`grid h-10 w-10 place-items-center rounded-xl ${healthy ? "bg-emerald-500/10" : "bg-destructive/10"}`}>
            {healthy ? <ShieldCheck className="h-5 w-5 text-emerald-500" /> : <XCircle className="h-5 w-5 text-destructive" />}
          </div>
          <div>
            <p className="font-medium">{label}</p>
            <p className="text-sm text-muted-foreground">{healthy ? t("adminStatus.healthy") : t("adminStatus.unhealthy")}</p>
          </div>
        </div>
        <Badge variant={healthy ? "success" : "destructive"}>{healthy ? "OK" : "FAIL"}</Badge>
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
        <Button onClick={loadStatus} disabled={loading}>
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
                {renderStatusItem(health.api, t("adminStatus.apiService"))}
                {renderStatusItem(health.database, t("adminLogs.database"))}
                {renderStatusItem(health.emby, t("adminStatus.embyService"))}
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
                      <span>{stats.emby?.server_name ?? t("adminStats.unknown")}</span>
                    </div>
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>{t("adminStatus.version")}</span>
                      <span>{stats.emby?.version ?? t("adminStats.unknown")}</span>
                    </div>
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>{t("adminStatus.onlineStatus")}</span>
                      <Badge variant={stats.emby?.online ? "success" : "destructive"}>
                        {stats.emby?.online ? t("dashboard.online") : t("dashboard.offline")}
                      </Badge>
                    </div>
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>{t("adminStatus.activeSessions")}</span>
                      <span>{stats.emby?.active_sessions ?? 0}</span>
                    </div>
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>{t("adminStatus.totalSessions")}</span>
                      <span>{stats.emby?.total_sessions ?? 0}</span>
                    </div>
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>{t("adminStatus.platform")}</span>
                      <span>{stats.emby?.operating_system ?? t("adminStats.unknown")}</span>
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
