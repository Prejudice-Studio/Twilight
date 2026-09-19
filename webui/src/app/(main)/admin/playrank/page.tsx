"use client";

import { useCallback, useEffect, useState } from "react";
import { BarChart3, Trophy, Film, Clock, Users, RefreshCw, Loader2, DownloadCloud } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useToast } from "@/hooks/use-toast";
import { useI18n } from "@/lib/i18n";
import { api, type PlayRankGroupBy, type PlayRankRange, type PlayRankResponse, type PlayRankSortBy } from "@/lib/api";
import { PlayRankMediaLabel } from "@/components/play-rank-media-label";

const rankRanges: PlayRankRange[] = ["day", "week", "month", "all"];
const rankGroups: PlayRankGroupBy[] = ["item", "series"];
const rankSorts: PlayRankSortBy[] = ["plays", "duration"];

// 当前排序指标用正常字重显示，另一个压暗。不这么做的话，两列数字并排、
// 看的人不知道这一屏究竟是照哪一列排的。
function metricClass(active: boolean): string {
  return active ? "text-sm font-medium text-foreground" : "text-xs text-muted-foreground";
}

// 同步窗口：活动日志只能按"过去 N 小时"回拉，默认 24 小时。想让榜单覆盖更久的
// 历史，得先按更长的窗口把日志拉回来——否则库里没有数据，切到总榜也是空的。
const syncWindows = [24, 72, 168, 720];

function rangeHintKey(range: PlayRankRange): "playRank.dayHint" | "playRank.weekHint" | "playRank.monthHint" | "playRank.allHint" {
  if (range === "week") return "playRank.weekHint";
  if (range === "month") return "playRank.monthHint";
  if (range === "all") return "playRank.allHint";
  return "playRank.dayHint";
}

function formatDay(unix: number): string {
  if (!unix || unix <= 0) return "-";
  return new Date(unix * 1000).toLocaleDateString();
}

export default function AdminPlayRankPage() {
  const { t } = useI18n();
  const { toast } = useToast();

  const [range, setRange] = useState<PlayRankRange>("day");
  const [groupBy, setGroupBy] = useState<PlayRankGroupBy>("item");
  const [sortBy, setSortBy] = useState<PlayRankSortBy>("plays");
  const [data, setData] = useState<PlayRankResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [syncHours, setSyncHours] = useState(24);
  const [error, setError] = useState<string | null>(null);

  const formatDuration = (seconds: number) => {
    const total = Math.max(0, Math.floor(Number.isFinite(seconds) ? seconds : 0));
    const hours = Math.floor(total / 3600);
    const minutes = Math.round((total % 3600) / 60);
    if (hours > 0) return t("playRank.unitHours", { value: hours });
    return t("playRank.unitMinutes", { value: total > 0 ? Math.max(1, minutes) : 0 });
  };

  const load = useCallback(async (target: PlayRankRange, group: PlayRankGroupBy, sort: PlayRankSortBy, force = false) => {
    if (force) setRefreshing(true);
    else setLoading(true);
    setError(null);
    try {
      const res = await api.getAdminPlayRank(target, { groupBy: group, sortBy: sort, refresh: force });
      if (res.success && res.data) {
        setData(res.data);
      } else {
        setError(res.message || t("playRank.empty"));
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t("common.networkError"));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [t]);

  useEffect(() => {
    void load(range, groupBy, sortBy);
  }, [load, range, groupBy, sortBy]);

  // 榜单数据来自 Emby 活动日志同步：手动同步一次可以让刚发生的播放立刻入榜，
  // 不必等定时任务或 60 秒缓存过期。
  const handleSync = async () => {
    if (syncing) return;
    setSyncing(true);
    try {
      const res = await api.adminGetEmbyActivityLogs(1, true, syncHours);
      if (res.success && res.data) {
        toast({
          title: t("playRank.syncDone", { count: res.data.new_entries ?? 0 }),
          variant: "success",
        });
        await load(range, groupBy, sortBy, true);
      } else {
        toast({
          title: t("playRank.syncFailed", { message: res.message || t("common.networkError") }),
          variant: "destructive",
        });
      }
    } catch (err) {
      toast({
        title: t("playRank.syncFailed", { message: err instanceof Error ? err.message : t("common.networkError") }),
        variant: "destructive",
      });
    } finally {
      setSyncing(false);
    }
  };

  const summary = data?.summary;
  const recorded = data?.recorded;
  const media = data?.media || [];
  const users = data?.users || [];
  const emptyHint =
    range !== "all" && (recorded?.total ?? 0) > 0 ? t("playRank.emptyHintSwitchRange") : t("playRank.emptyHint");

  return (
    <div className="page-enter space-y-6">
      <Card className="border-border/60">
        <CardContent className="flex flex-col gap-4 p-6">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="flex items-center gap-3">
              <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-primary/10 text-primary">
                <BarChart3 className="h-6 w-6" />
              </div>
              <div className="min-w-0">
                <h1 className="text-xl font-semibold">{t("playRank.adminTitle")}</h1>
                <p className="mt-0.5 text-sm text-muted-foreground">{t("playRank.adminDescription")}</p>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Select value={String(syncHours)} onValueChange={(value) => setSyncHours(Number(value))}>
                <SelectTrigger className="h-8 w-[7.5rem] text-xs" aria-label={t("playRank.syncWindow")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {syncWindows.map((hours) => (
                    <SelectItem key={hours} value={String(hours)} className="text-xs">
                      {hours < 168
                        ? t("playRank.syncWindowHours", { hours })
                        : t("playRank.syncWindowDays", { days: hours / 24 })}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button variant="secondary" size="sm" onClick={() => void handleSync()} disabled={syncing}>
                {syncing ? <Loader2 className="h-4 w-4 animate-spin" /> : <DownloadCloud className="h-4 w-4" />}
                {syncing ? t("playRank.syncing") : t("playRank.sync")}
              </Button>
              <Button variant="secondary" size="sm" onClick={() => void load(range, groupBy, sortBy, true)} disabled={refreshing || loading}>
                {refreshing ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
                {t("playRank.refresh")}
              </Button>
            </div>
          </div>

          <div className="flex flex-wrap gap-2">
            {rankRanges.map((item) => (
              <button
                key={item}
                type="button"
                onClick={() => setRange(item)}
                className={`rounded-lg px-3 py-1.5 text-sm font-medium transition-colors ${
                  range === item
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted text-muted-foreground hover:text-foreground"
                }`}
              >
                {item === "day"
                  ? t("playRank.day")
                  : item === "week"
                    ? t("playRank.week")
                    : item === "month"
                      ? t("playRank.month")
                      : t("playRank.all")}
              </button>
            ))}
            <span className="self-center text-xs text-muted-foreground">{t(rangeHintKey(range))}</span>
            {data?.enabled === false && (
              <Badge variant="destructive" className="self-center">{t("playRank.disabled")}</Badge>
            )}
            {data?.enabled !== false && data?.user_visible === false && (
              <Badge variant="secondary" className="self-center">{t("playRank.userVisibleOff")}</Badge>
            )}
            {data?.playback_reporting?.available ? (
              <Badge variant="success" className="self-center">{t("playRank.sourcePlugin")}</Badge>
            ) : (
              <Badge variant="secondary" className="self-center">{t("playRank.sourceActivityLog")}</Badge>
            )}
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <span className="text-xs text-muted-foreground">{t("playRank.groupBy")}</span>
            {rankGroups.map((item) => (
              <button
                key={item}
                type="button"
                onClick={() => setGroupBy(item)}
                className={`rounded-lg px-3 py-1 text-xs font-medium transition-colors ${
                  groupBy === item
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted text-muted-foreground hover:text-foreground"
                }`}
              >
                {item === "item" ? t("playRank.groupItem") : t("playRank.groupSeries")}
              </button>
            ))}
            <span className="self-center text-xs text-muted-foreground">
              {groupBy === "item" ? t("playRank.groupItemHint") : t("playRank.groupSeriesHint")}
            </span>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <span className="text-xs text-muted-foreground">{t("playRank.sortBy")}</span>
            {rankSorts.map((item) => (
              <button
                key={item}
                type="button"
                onClick={() => setSortBy(item)}
                className={`rounded-lg px-3 py-1 text-xs font-medium transition-colors ${
                  sortBy === item
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted text-muted-foreground hover:text-foreground"
                }`}
              >
                {item === "plays" ? t("playRank.sortPlays") : t("playRank.sortDuration")}
              </button>
            ))}
            <span className="self-center text-xs text-muted-foreground">
              {sortBy === "plays" ? t("playRank.sortPlaysHint") : t("playRank.sortDurationHint")}
            </span>
          </div>

          {data?.enabled === false && (
            <p className="rounded-lg border border-border/60 bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
              {t("playRank.disabledAdminHint")}
            </p>
          )}

          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <SummaryCard icon={Trophy} label={t("playRank.summaryPlays")} value={summary?.plays ?? 0} />
            <SummaryCard icon={Clock} label={t("playRank.summaryDuration")} value={formatDuration(summary?.duration ?? 0)} />
            <SummaryCard icon={Users} label={t("playRank.summaryViewers")} value={summary?.viewers ?? 0} />
            <SummaryCard icon={Film} label={t("playRank.summaryItems")} value={summary?.items ?? 0} />
          </div>

          {recorded && recorded.total > 0 ? (
            <p className="text-xs text-muted-foreground">
              {t("playRank.recorded", { total: recorded.total })}
              {recorded.earliest > 0
                ? ` · ${t("playRank.recordedSince", { date: formatDay(recorded.earliest) })}`
                : ""}
              {recorded.latest > 0
                ? ` · ${t("playRank.recordedUntil", { date: formatDay(recorded.latest) })}`
                : ""}
            </p>
          ) : (
            <p className="text-xs text-muted-foreground">{t("playRank.recordedEmpty")}</p>
          )}

          <p className="text-xs text-muted-foreground">
            {data?.playback_reporting?.available
              ? t("playRank.sourcePluginHint")
              : t("playRank.sourceActivityLogHint")}
          </p>

          {/* 探测没过时把原因直接摆出来：插件确实装了却用不上是最难自查的情况，
              只显示"活动日志"会让人以为没装。 */}
          {data?.playback_reporting?.enabled !== false &&
            !data?.playback_reporting?.available &&
            data?.playback_reporting?.last_error && (
              <p className="text-xs text-destructive">
                {t("playRank.sourceProbeFailed", { message: data.playback_reporting.last_error })}
              </p>
            )}
        </CardContent>
      </Card>

      {loading && !data ? (
        <Card className="border-border/60">
          <CardContent className="flex min-h-[200px] items-center justify-center">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </CardContent>
        </Card>
      ) : error ? (
        <Card className="border-border/60">
          <CardContent className="flex min-h-[200px] flex-col items-center justify-center gap-3 text-center">
            <p className="text-sm text-muted-foreground">{error}</p>
            <Button variant="secondary" size="sm" onClick={() => void load(range, groupBy, sortBy)}>
              {t("common.retry")}
            </Button>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-6 lg:grid-cols-2">
          <Card className="border-border/60">
            <CardContent className="p-0">
              <div className="flex items-center gap-2 border-b border-border/60 px-4 py-3">
                <Trophy className="h-4 w-4 text-primary" />
                <h2 className="text-sm font-semibold">
                  {groupBy === "series" ? t("playRank.mediaTitleSeries") : t("playRank.mediaTitle")}
                </h2>
                <Badge variant="secondary" className="ml-auto text-xs">{media.length}</Badge>
              </div>
              <div className="flex items-center gap-3 px-3 py-2 text-xs font-medium text-muted-foreground">
                <span className="w-6 shrink-0 text-center">{t("playRank.rank")}</span>
                <span className="min-w-0 flex-1">{t("playRank.media")}</span>
                <span className="w-14 shrink-0 text-right">{t("playRank.viewers")}</span>
                <span className={`shrink-0 ${sortBy === "plays" ? "text-foreground" : ""}`}>{t("playRank.plays")}</span>
                <span className={`w-20 shrink-0 text-right ${sortBy === "duration" ? "text-foreground" : ""}`}>{t("playRank.duration")}</span>
              </div>
              {media.length === 0 ? (
                <Empty hint={emptyHint} />
              ) : (
                media.map((item, index) => (
                  <div key={`${item.item_id || item.title}-${index}`} className="flex items-center gap-3 border-t border-border/50 px-3 py-2.5">
                    <span className="w-6 shrink-0 text-center text-sm font-semibold text-muted-foreground">{index + 1}</span>
                    <PlayRankMediaLabel item={item} groupBy={groupBy} />
                    <span className="w-14 shrink-0 text-right text-xs tabular-nums text-muted-foreground">{item.viewers}</span>
                    <span className={`shrink-0 tabular-nums ${metricClass(sortBy === "plays")}`}>{item.plays}</span>
                    <span className={`w-20 shrink-0 text-right tabular-nums ${metricClass(sortBy === "duration")}`}>{formatDuration(item.duration)}</span>
                  </div>
                ))
              )}
            </CardContent>
          </Card>

          <Card className="border-border/60">
            <CardContent className="p-0">
              <div className="flex items-center gap-2 border-b border-border/60 px-4 py-3">
                <Trophy className="h-4 w-4 text-primary" />
                <h2 className="text-sm font-semibold">{t("playRank.userTitle")}</h2>
                <Badge variant="secondary" className="ml-auto text-xs">{users.length}</Badge>
              </div>
              <div className="flex items-center gap-3 px-3 py-2 text-xs font-medium text-muted-foreground">
                <span className="w-6 shrink-0 text-center">{t("playRank.rank")}</span>
                <span className="min-w-0 flex-1">{t("playRank.username")}</span>
                <span className="w-14 shrink-0 text-right">{t("playRank.titles")}</span>
                <span className={`shrink-0 ${sortBy === "plays" ? "text-foreground" : ""}`}>{t("playRank.plays")}</span>
                <span className={`w-20 shrink-0 text-right ${sortBy === "duration" ? "text-foreground" : ""}`}>{t("playRank.duration")}</span>
              </div>
              {users.length === 0 ? (
                <Empty hint={emptyHint} />
              ) : (
                users.map((item, index) => (
                  <div key={`${item.uid ?? item.user_name}-${index}`} className="flex items-center gap-3 border-t border-border/50 px-3 py-2.5">
                    <span className="w-6 shrink-0 text-center text-sm font-semibold text-muted-foreground">{index + 1}</span>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{item.username || item.user_name || t("playRank.unknown")}</p>
                      <p className="truncate text-xs text-muted-foreground">
                        {t("playRank.uid")} {item.uid ?? "—"}
                      </p>
                    </div>
                    <span className="w-14 shrink-0 text-right text-xs tabular-nums text-muted-foreground">{item.items}</span>
                    <span className={`shrink-0 tabular-nums ${metricClass(sortBy === "plays")}`}>{item.plays}</span>
                    <span className={`w-20 shrink-0 text-right tabular-nums ${metricClass(sortBy === "duration")}`}>{formatDuration(item.duration)}</span>
                  </div>
                ))
              )}
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}

function SummaryCard({ icon: Icon, label, value }: { icon: typeof Trophy; label: string; value: string | number }) {
  return (
    <div className="rounded-xl border border-border/60 bg-muted/20 px-3 py-2.5">
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <Icon className="h-3.5 w-3.5" />
        {label}
      </div>
      <p className="mt-1 truncate text-lg font-semibold tabular-nums">{value}</p>
    </div>
  );
}

function Empty({ hint }: { hint: string }) {
  return (
    <div className="flex min-h-[120px] flex-col items-center justify-center gap-1 px-4 py-6 text-center">
      <p className="text-sm text-muted-foreground">{hint}</p>
    </div>
  );
}
