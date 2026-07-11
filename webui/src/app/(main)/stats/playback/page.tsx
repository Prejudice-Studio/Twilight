"use client";

import { useCallback, useEffect, useState } from "react";
import { motion } from "framer-motion";
import { Activity, BarChart3, CalendarDays, Clock, Loader2, Play, RefreshCw, Tv, Users } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";
import type { EmbyPlaybackStats } from "@/lib/api-types";
import { useI18n } from "@/lib/i18n";
import { useAuthStore } from "@/store/auth";
import { useSystemStore } from "@/store/system";

type StatsScope = "self" | "user" | "global";
type StatsSort = "plays" | "name";

function autoIntervalMs(value: string): number {
  const seconds = Number(value);
  return Number.isFinite(seconds) && seconds > 0 ? seconds * 1000 : 0;
}

export default function PlaybackStatsPage() {
  const { toast } = useToast();
  const { locale, t } = useI18n();
  const { user } = useAuthStore();
  const { info: systemInfo } = useSystemStore();
  const isAdmin = user?.role === 0;
  const [scope, setScope] = useState<StatsScope>(isAdmin ? "global" : "self");
  const [days, setDays] = useState("30");
  const [todayOnly, setTodayOnly] = useState(false);
  const [uid, setUid] = useState("");
  const [limit, setLimit] = useState("20");
  const [sort, setSort] = useState<StatsSort>("plays");
  const [autoRefresh, setAutoRefresh] = useState("60");
  const [stats, setStats] = useState<EmbyPlaybackStats | null>(null);
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [lastUpdated, setLastUpdated] = useState<number | null>(null);

  const formatDuration = useCallback((sec: number): string => {
    if (!sec || sec < 0) return t("playbackStats.durationMinutes", { minutes: 0 });
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    if (h > 0) return t("playbackStats.durationHoursMinutes", { hours: h, minutes: m });
    return t("playbackStats.durationMinutes", { minutes: m });
  }, [t]);

  useEffect(() => {
    if (!isAdmin && scope !== "self") {
      setScope("self");
      setUid("");
    }
  }, [isAdmin, scope]);

  const loadStats = useCallback(async (refresh = false) => {
    setLoading(!refresh);
    setRefreshing(refresh);
    try {
      const numericDays = Number(days) || 30;
      const numericLimit = Number(limit) || 20;
      const numericUID = Number(uid) || undefined;
      if (scope === "user" && !numericUID) {
        toast({ title: t("playbackStats.requireUid"), variant: "destructive" });
        return;
      }
      const params = {
        scope,
        uid: scope === "user" ? numericUID : undefined,
        days: numericDays,
        today: todayOnly,
        limit: numericLimit,
        sort,
        refresh,
      };
      const res = isAdmin && scope !== "self"
        ? await api.adminGetEmbyPlaybackStats(params)
        : await api.getEmbyPlaybackStats(params);
      if (res.success && res.data) {
        setStats(res.data);
        setLastUpdated(Date.now());
      } else {
        toast({ title: t("playbackStats.loadingFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({ title: t("playbackStats.loadingError"), description: err instanceof Error ? err.message : undefined, variant: "destructive" });
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [days, isAdmin, limit, scope, sort, t, todayOnly, toast, uid]);

  const enabled = systemInfo?.features?.emby_playback_stats === true;

  useEffect(() => {
    if (enabled) {
      void loadStats(false);
    }
  }, [enabled, loadStats]);

  useEffect(() => {
    if (!enabled) return;
    const ms = autoIntervalMs(autoRefresh);
    if (!ms) return;
    const timer = window.setInterval(() => {
      if (document.visibilityState !== "visible") return;
      void loadStats(false);
    }, ms);
    return () => window.clearInterval(timer);
  }, [autoRefresh, enabled, loadStats]);

  if (!enabled) {
    return (
      <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }}>
        <Card>
          <CardContent className="p-8 text-center text-sm text-muted-foreground">
            {t("playbackStats.disabled")}
          </CardContent>
        </Card>
      </motion.div>
    );
  }

  return (
    <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="flex items-center gap-2 text-2xl font-bold">
            <Activity className="h-6 w-6" />
            {t("playbackStats.title")}
          </h1>
          <p className="i18n-label mt-1 text-sm text-muted-foreground">{t("playbackStats.description")}</p>
          <p className="mt-1 text-xs text-muted-foreground">{t("playbackStats.autoRefreshHint")}</p>
          {lastUpdated && (
            <p className="mt-1 text-xs text-muted-foreground">
              {t("playbackStats.lastUpdated", { time: new Date(lastUpdated).toLocaleTimeString(locale) })}
            </p>
          )}
        </div>
        <Badge variant="outline">
          {stats?.period === "today"
            ? t("playbackStats.periodBadgeToday")
            : t("playbackStats.periodBadgeDays", { days: stats?.days ?? Number(days) })}
        </Badge>
      </div>

      <Card>
        <CardContent className="pt-4">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-7">
            <div className="space-y-1">
              <Label className="text-xs">{t("playbackStats.scopeLabel")}</Label>
              <Select value={scope} onValueChange={(value) => setScope(value as StatsScope)}>
                <SelectTrigger className="i18n-control"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="self">{t("playbackStats.scopeSelf")}</SelectItem>
                  {isAdmin && <SelectItem value="global">{t("playbackStats.scopeGlobal")}</SelectItem>}
                  {isAdmin && <SelectItem value="user">{t("playbackStats.scopeUser")}</SelectItem>}
                </SelectContent>
              </Select>
            </div>
            {isAdmin && scope === "user" && (
              <div className="space-y-1">
                <Label className="text-xs">{t("playbackStats.userUid")}</Label>
                <Input value={uid} onChange={(e) => setUid(e.target.value)} placeholder={t("playbackStats.userUidPlaceholder")} className="h-9" />
              </div>
            )}
            <div className="space-y-1">
              <Label className="text-xs">{t("playbackStats.periodLabel")}</Label>
              <Select value={todayOnly ? "today" : days} onValueChange={(value) => {
                setTodayOnly(value === "today");
                if (value !== "today") setDays(value);
              }}>
                <SelectTrigger className="i18n-control"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="today">{t("playbackStats.periodToday")}</SelectItem>
                  {[7, 30, 90, 365].map((item) => (
                    <SelectItem key={item} value={String(item)}>{t("playbackStats.periodDays", { days: item })}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label className="text-xs">{t("playbackStats.sortLabel")}</Label>
              <Select value={sort} onValueChange={(value) => setSort(value as StatsSort)}>
                <SelectTrigger className="i18n-control"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="plays">{t("playbackStats.sortPlays")}</SelectItem>
                  <SelectItem value="name">{t("playbackStats.sortName")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label className="text-xs">{t("playbackStats.limitLabel")}</Label>
              <Select value={limit} onValueChange={setLimit}>
                <SelectTrigger className="i18n-control"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {[10, 20, 50, 100].map((item) => (
                    <SelectItem key={item} value={String(item)}>{item}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label className="text-xs">{t("playbackStats.autoRefreshLabel")}</Label>
              <Select value={autoRefresh} onValueChange={setAutoRefresh}>
                <SelectTrigger className="i18n-control"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="0">{t("playbackStats.autoRefreshOff")}</SelectItem>
                  <SelectItem value="30">{t("playbackStats.autoRefresh30s")}</SelectItem>
                  <SelectItem value="60">{t("playbackStats.autoRefresh60s")}</SelectItem>
                  <SelectItem value="300">{t("playbackStats.autoRefresh300s")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-end gap-2">
              <Button onClick={() => loadStats(false)} disabled={loading || refreshing} className="flex-1">
                {loading ? <Loader2 className="mr-1.5 h-4 w-4 animate-spin" /> : <BarChart3 className="mr-1.5 h-4 w-4" />}
                {t("playbackStats.query")}
              </Button>
              <Button variant="outline" size="icon" onClick={() => loadStats(true)} disabled={loading || refreshing} title={t("playbackStats.refreshLogs")}>
                {refreshing ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      {stats && (
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <Card>
            <CardContent className="p-4 text-center">
              <Play className="mx-auto mb-1 h-5 w-5 text-blue-500" />
              <p className="text-2xl font-bold">{stats.total_plays ?? 0}</p>
              <p className="text-[10px] text-muted-foreground">{t("playbackStats.statPlayEvents")}</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4 text-center">
              <Clock className="mx-auto mb-1 h-5 w-5 text-green-500" />
              <p className="text-2xl font-bold">{formatDuration(stats.total_duration ?? 0)}</p>
              <p className="text-[10px] text-muted-foreground">{t("playbackStats.statDuration")}</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4 text-center">
              <Tv className="mx-auto mb-1 h-5 w-5 text-purple-500" />
              <p className="text-2xl font-bold">{stats.unique_items ?? 0}</p>
              <p className="text-[10px] text-muted-foreground">{t("playbackStats.statItems")}</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4 text-center">
              <CalendarDays className="mx-auto mb-1 h-5 w-5 text-orange-500" />
              <p className="text-2xl font-bold">{stats.period === "today" ? t("playbackStats.periodToday") : stats.days}</p>
              <p className="text-[10px] text-muted-foreground">{t("playbackStats.statPeriod")}</p>
            </CardContent>
          </Card>
        </div>
      )}

      {stats && (
        <div className="grid gap-4 lg:grid-cols-2">
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="flex items-center gap-2 text-base"><Users className="h-4 w-4" />{t("playbackStats.userRanking")}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="max-h-96 space-y-1 overflow-y-auto">
                {stats.user_rankings.length === 0 ? (
                  <p className="p-4 text-center text-sm text-muted-foreground">{t("playbackStats.noRankings")}</p>
                ) : stats.user_rankings.map((entry, index) => (
                  <div key={entry.uid || index} className="flex items-center justify-between rounded bg-accent/20 p-2 text-sm">
                    <span className="min-w-0 truncate">
                      <Badge variant="outline" className="mr-2 text-[10px]">{index + 1}</Badge>
                      {entry.username || `UID:${entry.uid}`}
                    </span>
                    <span className="shrink-0 text-xs text-muted-foreground">{t("playbackStats.playsCount", { count: entry.plays })}</span>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="flex items-center gap-2 text-base"><Tv className="h-4 w-4" />{t("playbackStats.itemRanking")}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="max-h-96 space-y-1 overflow-y-auto">
                {stats.top_items.length === 0 ? (
                  <p className="p-4 text-center text-sm text-muted-foreground">{t("playbackStats.noItems")}</p>
                ) : stats.top_items.map((entry, index) => (
                  <div key={`${entry.name}-${index}`} className="flex items-center justify-between gap-3 rounded bg-accent/20 p-2 text-sm">
                    <span className="min-w-0 truncate">{entry.name || t("playbackStats.unnamedItem")}</span>
                    <span className="shrink-0 text-xs text-muted-foreground">{t("playbackStats.playsCount", { count: entry.plays })}</span>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </div>
      )}

      {stats && stats.daily_breakdown.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">{t("playbackStats.dailyBreakdown")}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
              {stats.daily_breakdown.map((entry) => (
                <div key={entry.date} className="rounded border border-border/50 p-3">
                  <p className="text-xs text-muted-foreground">{entry.date}</p>
                  <p className="text-lg font-bold">{t("playbackStats.playsCount", { count: entry.plays })}</p>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {!stats && !loading && (
        <Card>
          <CardContent className="p-8 text-center text-sm text-muted-foreground">{t("playbackStats.emptyHint")}</CardContent>
        </Card>
      )}
    </motion.div>
  );
}
