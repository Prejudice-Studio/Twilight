"use client";

import { useCallback, useEffect, useState } from "react";
import { BarChart3, Trophy, Film, Clock, Users, RefreshCw, Loader2, DownloadCloud } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/hooks/use-toast";
import { useI18n } from "@/lib/i18n";
import { api, type PlayRankResponse } from "@/lib/api";

type RankRange = "day" | "week";

export default function AdminPlayRankPage() {
  const { t } = useI18n();
  const { toast } = useToast();

  const [range, setRange] = useState<RankRange>("day");
  const [data, setData] = useState<PlayRankResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const formatDuration = (seconds: number) => {
    const total = Math.max(0, Math.floor(Number.isFinite(seconds) ? seconds : 0));
    const hours = Math.floor(total / 3600);
    const minutes = Math.round((total % 3600) / 60);
    if (hours > 0) return t("playRank.unitHours", { value: hours });
    return t("playRank.unitMinutes", { value: total > 0 ? Math.max(1, minutes) : 0 });
  };

  const load = useCallback(async (target: RankRange, force = false) => {
    if (force) setRefreshing(true);
    else setLoading(true);
    setError(null);
    try {
      const res = await api.getAdminPlayRank(target, { refresh: force });
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
    void load(range);
  }, [load, range]);

  // 榜单数据来自 Emby 活动日志同步：手动同步一次可以让刚发生的播放立刻入榜，
  // 不必等定时任务或 60 秒缓存过期。
  const handleSync = async () => {
    if (syncing) return;
    setSyncing(true);
    try {
      const res = await api.adminGetEmbyActivityLogs(1, true, 24);
      if (res.success && res.data) {
        toast({
          title: t("playRank.syncDone", { count: res.data.new_entries ?? 0 }),
          variant: "success",
        });
        await load(range, true);
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
  const media = data?.media || [];
  const users = data?.users || [];

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
            <div className="flex flex-wrap gap-2">
              <Button variant="secondary" size="sm" onClick={() => void handleSync()} disabled={syncing}>
                {syncing ? <Loader2 className="h-4 w-4 animate-spin" /> : <DownloadCloud className="h-4 w-4" />}
                {syncing ? t("playRank.syncing") : t("playRank.sync")}
              </Button>
              <Button variant="secondary" size="sm" onClick={() => void load(range, true)} disabled={refreshing || loading}>
                {refreshing ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
                {t("playRank.refresh")}
              </Button>
            </div>
          </div>

          <div className="flex flex-wrap gap-2">
            {(["day", "week"] as RankRange[]).map((item) => (
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
                {item === "day" ? t("playRank.day") : t("playRank.week")}
              </button>
            ))}
            <span className="self-center text-xs text-muted-foreground">
              {range === "day" ? t("playRank.dayHint") : t("playRank.weekHint")}
            </span>
            {data?.enabled === false && (
              <Badge variant="destructive" className="self-center">{t("playRank.disabled")}</Badge>
            )}
            {data?.enabled !== false && data?.user_visible === false && (
              <Badge variant="secondary" className="self-center">{t("playRank.userVisibleOff")}</Badge>
            )}
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
            <Button variant="secondary" size="sm" onClick={() => void load(range)}>
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
                <h2 className="text-sm font-semibold">{t("playRank.mediaTitle")}</h2>
                <Badge variant="secondary" className="ml-auto text-xs">{media.length}</Badge>
              </div>
              <div className="flex items-center gap-3 px-3 py-2 text-xs font-medium text-muted-foreground">
                <span className="w-6 shrink-0 text-center">{t("playRank.rank")}</span>
                <span className="min-w-0 flex-1">{t("playRank.media")}</span>
                <span className="w-14 shrink-0 text-right">{t("playRank.viewers")}</span>
                <span className="shrink-0">{t("playRank.plays")}</span>
                <span className="w-20 shrink-0 text-right">{t("playRank.duration")}</span>
              </div>
              {media.length === 0 ? (
                <Empty hint={t("playRank.emptyHint")} />
              ) : (
                media.map((item, index) => (
                  <div key={`${item.item_id}-${index}`} className="flex items-center gap-3 border-t border-border/50 px-3 py-2.5">
                    <span className="w-6 shrink-0 text-center text-sm font-semibold text-muted-foreground">{index + 1}</span>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium" title={item.title}>{item.title || t("playRank.unknown")}</p>
                      <p className="truncate text-xs text-muted-foreground">
                        {item.series_name || item.media_type || t("playRank.unknown")}
                      </p>
                    </div>
                    <span className="w-14 shrink-0 text-right text-xs tabular-nums text-muted-foreground">{item.viewers}</span>
                    <span className="shrink-0 text-sm tabular-nums">{item.plays}</span>
                    <span className="w-20 shrink-0 text-right text-xs tabular-nums text-muted-foreground">{formatDuration(item.duration)}</span>
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
                <span className="shrink-0">{t("playRank.plays")}</span>
                <span className="w-20 shrink-0 text-right">{t("playRank.duration")}</span>
              </div>
              {users.length === 0 ? (
                <Empty hint={t("playRank.emptyHint")} />
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
                    <span className="shrink-0 text-sm tabular-nums">{item.plays}</span>
                    <span className="w-20 shrink-0 text-right text-xs tabular-nums text-muted-foreground">{formatDuration(item.duration)}</span>
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
