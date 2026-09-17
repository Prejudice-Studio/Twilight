"use client";

import { useCallback, useEffect, useState } from "react";
import { BarChart3, Trophy, Film, Clock, Users, RefreshCw, Loader2 } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useAuthStore } from "@/store/auth";
import { useSystemStore } from "@/store/system";
import { useI18n } from "@/lib/i18n";
import { api, type PlayRankResponse } from "@/lib/api";

type RankRange = "day" | "week";

export default function PlayRankPage() {
  const { t } = useI18n();
  const { user } = useAuthStore();
  const { info: systemInfo } = useSystemStore();
  const isAdmin = user?.role === 0;

  const [range, setRange] = useState<RankRange>("day");
  const [data, setData] = useState<PlayRankResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // 排行榜入口由后台开关控制：总开关关闭时任何人都不看，普通用户开关关闭时
  // 只有管理员还能预览（管理后台另有完整榜单页）。
  const playRankEnabled = systemInfo?.features?.play_rank !== false;
  const userVisible = systemInfo?.features?.play_rank_user !== false;
  const canView = playRankEnabled && (userVisible || isAdmin);

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
      const res = await api.getPlayRank(target, { refresh: force });
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
    if (!canView) return;
    void load(range);
  }, [canView, load, range]);

  const handleRefresh = () => {
    if (refreshing) return;
    void load(range, true);
  };

  if (!canView) {
    return (
      <div className="page-enter">
        <Card className="border-border/60">
          <CardContent className="flex min-h-[320px] flex-col items-center justify-center gap-3 p-8 text-center">
            <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
              <BarChart3 className="h-7 w-7" />
            </div>
            <h1 className="text-xl font-semibold">
              {playRankEnabled ? t("playRank.userVisibleOff") : t("playRank.disabled")}
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">{t("playRank.emptyHint")}</p>
          </CardContent>
        </Card>
      </div>
    );
  }

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
              <div>
                <h1 className="text-xl font-semibold">{t("playRank.title")}</h1>
                <p className="mt-0.5 text-sm text-muted-foreground">{t("playRank.description")}</p>
              </div>
            </div>
            <Button variant="secondary" size="sm" onClick={handleRefresh} disabled={refreshing || loading}>
              {refreshing ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
              {t("playRank.refresh")}
            </Button>
          </div>

          <div className="flex gap-2">
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
          </div>

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
          <RankCard
            title={t("playRank.mediaTitle")}
            empty={media.length === 0}
            emptyHint={t("playRank.emptyHint")}
            header={[t("playRank.rank"), t("playRank.media"), t("playRank.plays"), t("playRank.duration")]}
          >
            {media.map((item, index) => (
              <div key={`${item.item_id}-${index}`} className="flex items-center gap-3 border-b border-border/50 px-3 py-2.5 last:border-0">
                <span className="w-6 shrink-0 text-center text-sm font-semibold text-muted-foreground">{index + 1}</span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium" title={item.title}>{item.title || t("playRank.unknown")}</p>
                  <p className="truncate text-xs text-muted-foreground">
                    {item.series_name ? `${item.series_name} · ` : ""}
                    {t("playRank.viewers")} {item.viewers}
                  </p>
                </div>
                <span className="shrink-0 text-sm tabular-nums">{item.plays}</span>
                <span className="w-20 shrink-0 text-right text-xs tabular-nums text-muted-foreground">{formatDuration(item.duration)}</span>
              </div>
            ))}
          </RankCard>

          <RankCard
            title={t("playRank.userTitle")}
            empty={users.length === 0}
            emptyHint={t("playRank.emptyHint")}
            header={[t("playRank.rank"), t("playRank.user"), t("playRank.plays"), t("playRank.duration")]}
          >
            {users.map((item, index) => (
              <div key={`${item.user_name}-${index}`} className="flex items-center gap-3 border-b border-border/50 px-3 py-2.5 last:border-0">
                <span className="w-6 shrink-0 text-center text-sm font-semibold text-muted-foreground">{index + 1}</span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{item.user_name || t("playRank.unknown")}</p>
                  <p className="truncate text-xs text-muted-foreground">
                    {t("playRank.titles")} {item.items}
                  </p>
                </div>
                <span className="shrink-0 text-sm tabular-nums">{item.plays}</span>
                <span className="w-20 shrink-0 text-right text-xs tabular-nums text-muted-foreground">{formatDuration(item.duration)}</span>
              </div>
            ))}
          </RankCard>
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

function RankCard({
  title,
  header,
  empty,
  emptyHint,
  children,
}: {
  title: string;
  header: string[];
  empty: boolean;
  emptyHint: string;
  children: React.ReactNode;
}) {
  return (
    <Card className="border-border/60">
      <CardContent className="p-0">
        <div className="flex items-center gap-2 border-b border-border/60 px-4 py-3">
          <Trophy className="h-4 w-4 text-primary" />
          <h2 className="text-sm font-semibold">{title}</h2>
          {empty && <Badge variant="secondary" className="ml-auto text-xs">0</Badge>}
        </div>
        <div className="flex items-center gap-3 px-3 py-2 text-xs font-medium text-muted-foreground">
          <span className="w-6 shrink-0 text-center">{header[0]}</span>
          <span className="min-w-0 flex-1">{header[1]}</span>
          <span className="shrink-0">{header[2]}</span>
          <span className="w-20 shrink-0 text-right">{header[3]}</span>
        </div>
        {empty ? (
          <div className="flex min-h-[120px] flex-col items-center justify-center gap-1 px-4 py-6 text-center">
            <p className="text-sm text-muted-foreground">{emptyHint}</p>
          </div>
        ) : (
          children
        )}
      </CardContent>
    </Card>
  );
}
