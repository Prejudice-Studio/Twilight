"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { motion } from "framer-motion";
import { Activity, BarChart3, CalendarDays, Clock, Download, ImageIcon, Loader2, Play, RefreshCw, Search, Tv, Users } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useToast } from "@/hooks/use-toast";
import { useVisiblePolling } from "@/hooks/use-visible-polling";
import { api } from "@/lib/api";
import type { EmbyPlaybackStats } from "@/lib/api-types";
import { useI18n } from "@/lib/i18n";
import { useAuthStore } from "@/store/auth";
import { useSystemStore } from "@/store/system";

type StatsScope = "self" | "user" | "global";
type StatsPeriod = "today" | "week" | "month" | "days" | "range";
type StatsGroupBy = "day" | "week" | "month";
type StatsMediaType = "all" | "movie" | "series" | "other";
type StatsSort = "plays" | "duration" | "name";

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
  const [period, setPeriod] = useState<StatsPeriod>("today");
  const [days, setDays] = useState("30");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [groupBy, setGroupBy] = useState<StatsGroupBy>("day");
  const [mediaType, setMediaType] = useState<StatsMediaType>("all");
  const [queryText, setQueryText] = useState("");
  const [minDuration, setMinDuration] = useState("0");
  const [uid, setUid] = useState("");
  const [limit, setLimit] = useState("20");
  const [sort, setSort] = useState<StatsSort>("plays");
  const [autoRefresh, setAutoRefresh] = useState("300");
  const [stats, setStats] = useState<EmbyPlaybackStats | null>(null);
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [lastUpdated, setLastUpdated] = useState<number | null>(null);
  const requestRef = useRef<AbortController | null>(null);

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
    requestRef.current?.abort();
    const controller = new AbortController();
    requestRef.current = controller;
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
      if (period === "range" && (!dateFrom || !dateTo)) {
        toast({ title: t("playbackStats.requireDateRange"), variant: "destructive" });
        return;
      }
      const params = {
        scope,
        uid: scope === "user" ? numericUID : undefined,
        period: period === "today" ? "today" as const : period === "week" ? "week" as const : period === "month" ? "month" as const : period === "days" ? "custom" as const : undefined,
        days: period === "days" ? numericDays : undefined,
        from: period === "range" ? dateFrom : undefined,
        to: period === "range" ? dateTo : undefined,
        group_by: groupBy,
        media_type: mediaType,
        query: queryText.trim() || undefined,
        min_duration: Number(minDuration) || undefined,
        limit: numericLimit,
        sort,
        refresh,
      };
      const res = isAdmin && scope !== "self"
        ? await api.adminGetEmbyPlaybackStats(params, controller.signal)
        : await api.getEmbyPlaybackStats(params, controller.signal);
      if (res.success && res.data) {
        setStats(res.data);
        setLastUpdated(Date.now());
      } else {
        toast({ title: t("playbackStats.loadingFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      if (err instanceof Error && err.name === "AbortError") return;
      toast({ title: t("playbackStats.loadingError"), description: err instanceof Error ? err.message : undefined, variant: "destructive" });
    } finally {
      if (requestRef.current === controller) {
        requestRef.current = null;
        setLoading(false);
        setRefreshing(false);
      }
    }
  }, [dateFrom, dateTo, days, groupBy, isAdmin, limit, mediaType, minDuration, period, queryText, scope, sort, t, toast, uid]);

  const handleDownloadCSV = useCallback(() => {
    const params = new URLSearchParams();
    if (days) params.set("days", days);
    if (dateFrom) params.set("from", dateFrom);
    if (dateTo) params.set("to", dateTo);
    window.open(`/api/v1/batch/export/playback?${params.toString()}`, "_blank");
  }, [dateFrom, dateTo, days]);

  const enabled = systemInfo?.features?.emby_playback_stats === true;

  useEffect(() => {
    if (enabled) {
      void loadStats(false);
    }
  }, [enabled, loadStats]);

  useEffect(() => {
    return () => requestRef.current?.abort();
  }, []);

  const pollingMs = autoIntervalMs(autoRefresh);
  useVisiblePolling(() => loadStats(false), pollingMs, enabled && pollingMs > 0);

  const policy = stats?.policy;
  const periodAllowed = (kind: "day" | "week" | "month" | "custom") => isAdmin || !policy || policy.allowed_periods.includes(kind);
  const groupingAllowed = (kind: StatsGroupBy) => isAdmin || !policy || policy.allowed_groupings.includes(kind);
  const showUserRankings = isAdmin || policy?.show_user_rankings !== false;
  const showItemRankings = isAdmin || policy?.show_item_rankings !== false;
  const showDailySummary = isAdmin || policy?.show_daily_summary !== false;
  const periodLabel = stats?.period === "today"
    ? t("playbackStats.periodToday")
    : stats?.period === "week"
      ? t("playbackStats.periodWeek")
      : stats?.period === "month"
        ? t("playbackStats.periodMonth")
        : t("playbackStats.periodDays", { days: stats?.days ?? Number(days) });

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
        <div className="flex flex-wrap gap-2">
          <Badge variant="outline">{periodLabel}</Badge>
          {stats?.from && stats?.to && <Badge variant="secondary">{stats.from} - {stats.to}</Badge>}
          {stats?.source && <Badge variant="outline">{stats.source === "emby_activity_log" ? "Emby ActivityLog" : t("playbackStats.localFallback")}</Badge>}
          {isAdmin && (
            <Button size="icon" variant="outline" onClick={handleDownloadCSV} title={t("playbackStats.exportCSV")}>
              <Download className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>

      <Card>
        <CardContent className="pt-4">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-6">
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
              <Select value={period} onValueChange={(value) => setPeriod(value as StatsPeriod)}>
                <SelectTrigger className="i18n-control"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="today" disabled={!periodAllowed("day")}>{t("playbackStats.periodToday")}</SelectItem>
                  <SelectItem value="week" disabled={!periodAllowed("week")}>{t("playbackStats.periodWeek")}</SelectItem>
                  <SelectItem value="month" disabled={!periodAllowed("month")}>{t("playbackStats.periodMonth")}</SelectItem>
                  <SelectItem value="days" disabled={!periodAllowed("custom")}>{t("playbackStats.periodCustomDays")}</SelectItem>
                  <SelectItem value="range" disabled={!periodAllowed("custom")}>{t("playbackStats.periodCustomRange")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {period === "days" && (
              <div className="space-y-1">
                <Label className="text-xs">{t("playbackStats.daysLabel")}</Label>
                <Input value={days} onChange={(e) => setDays(e.target.value)} type="number" min={1} max={policy?.max_days ?? 365} className="h-9" />
              </div>
            )}
            {period === "range" && (
              <>
                <div className="space-y-1">
                  <Label className="text-xs">{t("playbackStats.fromLabel")}</Label>
                  <Input value={dateFrom} onChange={(e) => setDateFrom(e.target.value)} type="date" className="h-9" />
                </div>
                <div className="space-y-1">
                  <Label className="text-xs">{t("playbackStats.toLabel")}</Label>
                  <Input value={dateTo} onChange={(e) => setDateTo(e.target.value)} type="date" className="h-9" />
                </div>
              </>
            )}
            <div className="space-y-1">
              <Label className="text-xs">{t("playbackStats.groupByLabel")}</Label>
              <Select value={groupBy} onValueChange={(value) => setGroupBy(value as StatsGroupBy)}>
                <SelectTrigger className="i18n-control"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="day" disabled={!groupingAllowed("day")}>{t("playbackStats.groupDay")}</SelectItem>
                  <SelectItem value="week" disabled={!groupingAllowed("week")}>{t("playbackStats.groupWeek")}</SelectItem>
                  <SelectItem value="month" disabled={!groupingAllowed("month")}>{t("playbackStats.groupMonth")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label className="text-xs">{t("playbackStats.mediaTypeLabel")}</Label>
              <Select value={mediaType} onValueChange={(value) => setMediaType(value as StatsMediaType)}>
                <SelectTrigger className="i18n-control"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">{t("playbackStats.mediaAll")}</SelectItem>
                  <SelectItem value="movie">{t("playbackStats.mediaMovie")}</SelectItem>
                  <SelectItem value="series">{t("playbackStats.mediaSeries")}</SelectItem>
                  <SelectItem value="other">{t("playbackStats.mediaOther")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label className="text-xs">{t("playbackStats.sortLabel")}</Label>
              <Select value={sort} onValueChange={(value) => setSort(value as StatsSort)}>
                <SelectTrigger className="i18n-control"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="plays">{t("playbackStats.sortPlays")}</SelectItem>
                  <SelectItem value="duration">{t("playbackStats.sortDuration")}</SelectItem>
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
              <Label className="text-xs">{t("playbackStats.minDurationLabel")}</Label>
              <Select value={minDuration} onValueChange={setMinDuration}>
                <SelectTrigger className="i18n-control"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="0">{t("playbackStats.minDurationAny")}</SelectItem>
                  <SelectItem value="300">{t("playbackStats.minDuration5m")}</SelectItem>
                  <SelectItem value="900">{t("playbackStats.minDuration15m")}</SelectItem>
                  <SelectItem value="1800">{t("playbackStats.minDuration30m")}</SelectItem>
                  <SelectItem value="3600">{t("playbackStats.minDuration60m")}</SelectItem>
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
            <div className="space-y-1 md:col-span-2">
              <Label className="text-xs">{t("playbackStats.queryLabel")}</Label>
              <div className="relative">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input value={queryText} onChange={(e) => setQueryText(e.target.value)} placeholder={t("playbackStats.queryPlaceholder")} className="h-9 pl-9" />
              </div>
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
              <p className="truncate text-xl font-bold" title={periodLabel}>{periodLabel}</p>
              <p className="text-[10px] text-muted-foreground">{t("playbackStats.statPeriod")}</p>
            </CardContent>
          </Card>
        </div>
      )}

      {stats && (showUserRankings || showItemRankings) && (
        <div className="grid gap-4 lg:grid-cols-2">
          {showUserRankings && <Card>
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
                    <span className="shrink-0 text-right text-xs text-muted-foreground">
                      <span className="block">{t("playbackStats.playsCount", { count: entry.plays })}</span>
                      <span className="block">{formatDuration(entry.duration)}</span>
                    </span>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>}

          {showItemRankings && <Card>
            <CardHeader className="pb-2">
              <CardTitle className="flex items-center gap-2 text-base"><Tv className="h-4 w-4" />{t("playbackStats.itemRanking")}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="max-h-96 space-y-1 overflow-y-auto">
                {stats.top_items.length === 0 ? (
                  <p className="p-4 text-center text-sm text-muted-foreground">{t("playbackStats.noItems")}</p>
                ) : stats.top_items.map((entry, index) => (
                  <div key={`${entry.id || entry.name}-${index}`} className="flex items-center justify-between gap-3 rounded bg-accent/20 p-2 text-sm">
                    <div className="flex min-w-0 items-center gap-3">
                      <div className="relative grid h-12 w-9 shrink-0 place-items-center overflow-hidden rounded bg-muted text-muted-foreground">
                        <ImageIcon className="h-4 w-4" />
                        {entry.image_url ? (
                          // eslint-disable-next-line @next/next/no-img-element
                          <img
                            src={entry.image_url}
                            alt=""
                            className="absolute inset-0 h-full w-full object-cover"
                            loading="lazy"
                            onError={(event) => {
                              event.currentTarget.style.display = "none";
                            }}
                          />
                        ) : null}
                      </div>
                      <div className="min-w-0">
                        <p className="truncate font-medium">{entry.name || t("playbackStats.unnamedItem")}</p>
                        {entry.media_type && <p className="text-[10px] uppercase text-muted-foreground">{entry.media_type}</p>}
                      </div>
                    </div>
                    <span className="shrink-0 text-right text-xs text-muted-foreground">
                      <span className="block">{t("playbackStats.playsCount", { count: entry.plays })}</span>
                      <span className="block">{formatDuration(entry.duration)}</span>
                    </span>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>}
        </div>
      )}

      {stats && showDailySummary && stats.daily_breakdown.length > 0 && (
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
                  <p className="text-xs text-muted-foreground">{formatDuration(entry.duration)}</p>
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
