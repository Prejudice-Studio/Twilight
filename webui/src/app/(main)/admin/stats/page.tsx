"use client";

import { useCallback, useState } from "react";
import {
  Users,
  UserCheck,
  UserX,
  Coins,
  FileText,
  Clock,
  Loader2,
  TrendingUp,
  Activity,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError, PageLoading } from "@/components/layout/page-state";
import { api, type SystemStats } from "@/lib/api";
import { formatNumber } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";

const formatBytes = (bytes: number | undefined, unknown: string) => {
  if (bytes == null) return unknown;
  const mb = bytes / 1024 / 1024;
  return `${formatNumber(Math.round(mb))} MB`;
};

export default function AdminStatsPage() {
  const { t } = useI18n();
  const [stats, setStats] = useState<SystemStats | null>(null);

  const loadStatsResource = useCallback(async () => {
    const res = await api.getSystemStats();
    if (res.success && res.data) {
      setStats(res.data);
    }
    return true;
  }, []);

  const {
    isLoading,
    error,
    execute: loadStats,
  } = useAsyncResource(loadStatsResource, { immediate: true });

  if (error) {
    return <PageError message={error} onRetry={() => void loadStats()} />;
  }

  if (isLoading) {
    return <PageLoading message={t("adminStats.loading")} />;
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold">{t("adminStats.title")}</h1>
        <p className="text-muted-foreground">{t("adminStats.description")}</p>
      </div>

      {/* Stats Grid */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        <div>
          <Card className="relative overflow-hidden">
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {t("adminStats.cpuCount")}
              </CardTitle>
              <Users className="h-4 w-4 text-blue-500" />
            </CardHeader>
            <CardContent>
              <div className="text-3xl font-bold">
                {stats?.cpu_count ?? 0}
              </div>
            </CardContent>
          </Card>
        </div>

        <div>
          <Card className="relative overflow-hidden">
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {t("adminStats.cpuUsage")}
              </CardTitle>
              <TrendingUp className="h-4 w-4 text-emerald-500" />
            </CardHeader>
            <CardContent>
              <div className="text-3xl font-bold text-emerald-500">
                {stats?.cpu_percent != null ? `${stats.cpu_percent}%` : t("adminStats.unknown")}
              </div>
            </CardContent>
          </Card>
        </div>

        <div>
          <Card className="relative overflow-hidden">
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {t("adminStats.memoryUsage")}
              </CardTitle>
              <FileText className="h-4 w-4 text-orange-500" />
            </CardHeader>
            <CardContent>
              <div className="text-3xl font-bold text-orange-500">
                {stats?.memory?.percent != null ? `${stats.memory.percent}%` : t("adminStats.unknown")}
              </div>
            </CardContent>
          </Card>
        </div>

        <div>
          <Card className="relative overflow-hidden">
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {t("adminStats.memoryAvailable")}
              </CardTitle>
              <Coins className="h-4 w-4 text-info" />
            </CardHeader>
            <CardContent>
              <div className="text-3xl font-bold">
                {formatBytes(stats?.memory?.available, t("adminStats.unknown"))}
              </div>
            </CardContent>
          </Card>
        </div>

        <div>
          <Card className="relative overflow-hidden">
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {t("adminStats.diskUsage")}
              </CardTitle>
              <Clock className="h-4 w-4 text-info" />
            </CardHeader>
            <CardContent>
              <div className="text-3xl font-bold">
                {stats?.disk?.percent != null ? `${stats.disk.percent}%` : t("adminStats.unknown")}
              </div>
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Quick Overview */}
      <div>
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Activity className="h-5 w-5" />
              {t("adminStats.systemStatus")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-3">
                <div className="flex items-center justify-between rounded-lg bg-accent/50 p-3">
                  <span className="text-sm">{t("adminStats.lastUpdated")}</span>
                  <Badge variant="secondary">
                    {stats?.timestamp ? new Date(stats.timestamp * 1000).toLocaleString() : t("adminStats.unknown")}
                  </Badge>
                </div>
                <div className="flex items-center justify-between rounded-lg bg-accent/50 p-3">
                  <span className="text-sm">{t("adminStats.memoryTotal")}</span>
                  <Badge variant="secondary">
                    {formatBytes(stats?.memory?.total, t("adminStats.unknown"))}
                  </Badge>
                </div>
              </div>
              <div className="space-y-3">
                <div className="flex items-center justify-between rounded-lg bg-accent/50 p-3">
                  <span className="text-sm">{t("adminStats.memoryAvailable")}</span>
                  <Badge variant={stats?.memory?.available && stats.memory.available > 0 ? "success" : "secondary"}>
                    {formatBytes(stats?.memory?.available, t("adminStats.unknown"))}
                  </Badge>
                </div>
                <div className="flex items-center justify-between rounded-lg bg-accent/50 p-3">
                  <span className="text-sm">{t("adminStats.diskAvailable")}</span>
                  <Badge variant={stats?.disk?.free && stats.disk.free > 0 ? "success" : "secondary"}>
                    {formatBytes(stats?.disk?.free, t("adminStats.unknown"))}
                  </Badge>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

