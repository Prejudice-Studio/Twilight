"use client";

import { useCallback, useEffect, useState } from "react";
import { Activity, Loader2, LogIn, LogOut, Pause, Play, RefreshCw } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

interface ActivityLogEntry {
  id: number;
  emby_log_id: number;
  type: string;
  name: string;
  user_id: string;
  user_name: string;
  overview: string;
  date: number;
}

function activityIcon(type: string) {
  const t = type.toLowerCase();
  if (t.includes("playback") && !t.includes("complete")) return <Play className="h-3.5 w-3.5 text-blue-500" />;
  if (t.includes("playback")) return <Pause className="h-3.5 w-3.5 text-green-500" />;
  if (t.includes("auth") || t.includes("login")) return <LogIn className="h-3.5 w-3.5 text-yellow-500" />;
  if (t.includes("session") && t.includes("end")) return <LogOut className="h-3.5 w-3.5 text-muted-foreground" />;
  if (t.includes("session")) return <LogIn className="h-3.5 w-3.5 text-yellow-500" />;
  return <Activity className="h-3.5 w-3.5 text-muted-foreground" />;
}

function activityLabelKey(type: string) {
  const t = type.toLowerCase();
  if (t === "videoplayback") return "embyActivityLogs.labelVideoPlayback" as const;
  if (t === "videoplaybackcomplete") return "embyActivityLogs.labelVideoPlaybackComplete" as const;
  if (t === "authenticationsucceeded") return "embyActivityLogs.labelAuthenticationSucceeded" as const;
  if (t === "authenticationfailure") return "embyActivityLogs.labelAuthenticationFailure" as const;
  if (t === "sessionstarted") return "embyActivityLogs.labelSessionStarted" as const;
  if (t === "sessionended") return "embyActivityLogs.labelSessionEnded" as const;
  return null;
}

export default function EmbyActivityLogs() {
  const { toast } = useToast();
  const { locale, t } = useI18n();
  const [refreshing, setRefreshing] = useState(false);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [lastUpdated, setLastUpdated] = useState<number | null>(null);
  const [lastNewEntries, setLastNewEntries] = useState<number | null>(null);

  const formatDate = useCallback((unix: number) => {
    if (!unix) return "";
    return new Date(unix * 1000).toLocaleString(locale);
  }, [locale]);

  const applyMeta = useCallback((data: { new_entries?: number }) => {
    setLastUpdated(Date.now());
    if (typeof data.new_entries === "number") setLastNewEntries(data.new_entries);
  }, []);

  const load = useCallback(async () => {
    const res = await api.adminGetEmbyActivityLogs(100, false, true);
    if (res.success && res.data) {
      applyMeta(res.data);
      return res.data.entries || [];
    }
    throw new Error(res.message || t("embyActivityLogs.loadFailed"));
  }, [applyMeta, t]);

  const { data: logs, isLoading, error, execute: reload, setData } = useAsyncResource(load, { immediate: true });

  const handleRefresh = async () => {
    setRefreshing(true);
    try {
      const res = await api.adminGetEmbyActivityLogs(100, true, false);
      if (!res.success || !res.data) {
        throw new Error(res.message || t("embyActivityLogs.refreshFailed"));
      }
      applyMeta(res.data);
      setData(res.data.entries || []);
      toast({
        title: t("embyActivityLogs.refreshed"),
        description: t("embyActivityLogs.newEntries", { count: res.data.new_entries ?? 0 }),
        variant: "success",
      });
    } catch (err) {
      toast({
        title: t("embyActivityLogs.refreshFailed"),
        description: err instanceof Error ? err.message : undefined,
        variant: "destructive",
      });
    } finally {
      setRefreshing(false);
    }
  };

  useEffect(() => {
    if (!autoRefresh) return;
    const timer = window.setInterval(() => {
      if (document.visibilityState !== "visible") return;
      void reload();
    }, 30000);
    return () => window.clearInterval(timer);
  }, [autoRefresh, reload]);

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <h3 className="flex items-center gap-2 text-lg font-bold">
            <Activity className="h-5 w-5 text-primary" />
            {t("embyActivityLogs.title")}
          </h3>
          <p className="i18n-label text-sm text-muted-foreground">{t("embyActivityLogs.description")}</p>
          <div className="mt-2 flex flex-wrap gap-2 text-[11px] text-muted-foreground">
            {lastUpdated && <span>{t("embyActivityLogs.lastUpdated", { time: new Date(lastUpdated).toLocaleTimeString(locale) })}</span>}
            {lastNewEntries !== null && <span>{t("embyActivityLogs.newEntries", { count: lastNewEntries })}</span>}
          </div>
        </div>
        <div className="i18n-toolbar flex flex-wrap items-center gap-2">
          <label className="flex min-h-9 items-center gap-2 rounded-md border border-border/60 px-3 text-xs">
            <Switch checked={autoRefresh} onCheckedChange={setAutoRefresh} />
            <span>{t("embyActivityLogs.autoRefresh")}</span>
          </label>
          <Button variant="outline" size="sm" onClick={handleRefresh} disabled={refreshing || isLoading}>
            {refreshing || isLoading ? <Loader2 className="mr-1.5 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-1.5 h-4 w-4" />}
            {t("embyActivityLogs.refresh")}
          </Button>
        </div>
      </div>

      {autoRefresh && <p className="text-xs text-muted-foreground">{t("embyActivityLogs.autoRefreshHint", { seconds: 30 })}</p>}

      {error ? (
        <Card><CardContent className="p-6 text-center text-destructive">{String(error)}</CardContent></Card>
      ) : isLoading && !logs ? (
        <Card><CardContent className="flex p-8 justify-center"><Loader2 className="h-6 w-6 animate-spin" /></CardContent></Card>
      ) : !logs || logs.length === 0 ? (
        <Card><CardContent className="p-8 text-center text-sm text-muted-foreground">{t("embyActivityLogs.empty")}</CardContent></Card>
      ) : (
        <div className="space-y-1.5">
          {logs.map((log: ActivityLogEntry, idx: number) => {
            const labelKey = activityLabelKey(log.type);
            return (
              <div key={log.id || idx} className="flex items-start gap-3 rounded-lg border border-border/40 bg-accent/10 p-3 text-sm transition-colors hover:bg-accent/20">
                <div className="mt-0.5 shrink-0">{activityIcon(log.type)}</div>
                <div className="min-w-0 flex-1 space-y-0.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="outline" className="text-[10px]">{labelKey ? t(labelKey) : log.type}</Badge>
                    {log.user_name && <span className="truncate text-xs font-medium">{log.user_name}</span>}
                  </div>
                  <p className="truncate text-xs text-muted-foreground">{log.name}</p>
                  {log.overview && <p className="line-clamp-1 text-[11px] text-muted-foreground/70">{log.overview}</p>}
                </div>
                <div className="shrink-0 whitespace-nowrap text-[10px] text-muted-foreground/50">
                  {formatDate(log.date)}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
