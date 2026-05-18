"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { motion } from "framer-motion";
import {
  Coins,
  Flame,
  Trophy,
  CalendarCheck,
  Sparkles,
  RefreshCw,
  Loader2,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/hooks/use-toast";
import {
  api,
  type SigninSummary,
  type SigninPublicConfig,
  type SigninHistoryRecord,
} from "@/lib/api";

function formatDate(date: string | null | undefined): string {
  if (!date) return "—";
  return date;
}

function formatRelative(ts: number): string {
  if (!ts || ts <= 0) return "—";
  try {
    return new Date(ts * 1000).toLocaleString("zh-CN");
  } catch {
    return "—";
  }
}

export default function ScorePage() {
  const { toast } = useToast();
  const [summary, setSummary] = useState<SigninSummary | null>(null);
  const [config, setConfig] = useState<SigninPublicConfig | null>(null);
  const [history, setHistory] = useState<SigninHistoryRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [signing, setSigning] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const currencyName = summary?.currency_name || config?.currency_name || "积分";

  const reload = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [summaryRes, configRes, historyRes] = await Promise.all([
        api.getSigninSummary(),
        api.getSigninPublicConfig(),
        api.getSigninHistory(30),
      ]);
      if (summaryRes.success && summaryRes.data) setSummary(summaryRes.data);
      if (configRes.success && configRes.data) setConfig(configRes.data);
      if (historyRes.success && historyRes.data) setHistory(historyRes.data.records);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  const handleSignin = async () => {
    if (signing) return;
    setSigning(true);
    try {
      const res = await api.signinNow();
      if (res.success && res.data) {
        const bonus = res.data.bonus_points > 0 ? `（含连签加成 +${res.data.bonus_points}）` : "";
        toast({
          title: `签到成功 +${res.data.total_today} ${res.data.currency_name}`,
          description: `当前连签 ${res.data.current_streak} 天 ${bonus}`,
          variant: "success",
        });
        await reload();
      } else {
        toast({
          title: "签到失败",
          description: res.message,
          variant: "destructive",
        });
        await reload();
      }
    } catch (err) {
      toast({
        title: "签到失败",
        description: err instanceof Error ? err.message : "网络错误",
        variant: "destructive",
      });
    } finally {
      setSigning(false);
    }
  };

  const disabledByConfig = config?.enabled === false || summary?.enabled === false;
  const todaySigned = summary?.today_signed === true;

  const bonusTable = useMemo(() => config?.bonus_table || [], [config?.bonus_table]);
  const dailyRange = useMemo(() => {
    if (!config) return "—";
    if (config.daily_min === config.daily_max) return String(config.daily_min);
    return `${config.daily_min} - ${config.daily_max}`;
  }, [config]);

  return (
    <div className="space-y-6">
      {/* 头部：余额 + 签到按钮 */}
      <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }}>
        <Card className="overflow-hidden border-border/60">
          <CardContent className="relative flex flex-col gap-6 p-6 md:flex-row md:items-center md:justify-between">
            <div className="flex items-center gap-5">
              <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-amber-500/15 text-amber-500">
                <Coins className="h-7 w-7" />
              </div>
              <div>
                <p className="text-sm text-muted-foreground">我的{currencyName}</p>
                <div className="flex items-baseline gap-2">
                  <p className="text-4xl font-bold tracking-tight">
                    {summary?.current_points ?? 0}
                  </p>
                  <span className="text-base text-muted-foreground">{currencyName}</span>
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  累计获得 {summary?.total_points ?? 0} {currencyName}
                </p>
              </div>
            </div>

            <div className="flex flex-col items-stretch gap-2 md:items-end">
              <Button
                size="lg"
                onClick={handleSignin}
                disabled={signing || todaySigned || disabledByConfig}
                className="min-w-[180px]"
              >
                {signing ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" /> 签到中…
                  </>
                ) : todaySigned ? (
                  <>
                    <CalendarCheck className="mr-2 h-4 w-4" /> 今日已签到
                  </>
                ) : (
                  <>
                    <Sparkles className="mr-2 h-4 w-4" /> 立即签到
                  </>
                )}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => void reload()}
                disabled={loading}
              >
                <RefreshCw className={`mr-2 h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
                刷新
              </Button>
            </div>
          </CardContent>
        </Card>
      </motion.div>

      {/* 三个统计卡 */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <Card className="border-border/60">
          <CardContent className="flex items-center gap-4 p-5">
            <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-orange-500/15 text-orange-500">
              <Flame className="h-5 w-5" />
            </div>
            <div>
              <p className="text-xs text-muted-foreground">当前连签</p>
              <p className="text-2xl font-semibold">{summary?.current_streak ?? 0} 天</p>
              {summary?.next_bonus_in_days && summary.next_bonus_points ? (
                <p className="mt-1 text-xs text-muted-foreground">
                  再签 {summary.next_bonus_in_days} 天可获 +{summary.next_bonus_points} {currencyName}
                </p>
              ) : (
                <p className="mt-1 text-xs text-muted-foreground">无更多加成档位</p>
              )}
            </div>
          </CardContent>
        </Card>

        <Card className="border-border/60">
          <CardContent className="flex items-center gap-4 p-5">
            <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-purple-500/15 text-purple-500">
              <Trophy className="h-5 w-5" />
            </div>
            <div>
              <p className="text-xs text-muted-foreground">历史最长连签</p>
              <p className="text-2xl font-semibold">{summary?.longest_streak ?? 0} 天</p>
              <p className="mt-1 text-xs text-muted-foreground">上次签到 {formatDate(summary?.last_signin_date)}</p>
            </div>
          </CardContent>
        </Card>

        <Card className="border-border/60">
          <CardContent className="flex items-center gap-4 p-5">
            <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-sky-500/15 text-sky-500">
              <Sparkles className="h-5 w-5" />
            </div>
            <div>
              <p className="text-xs text-muted-foreground">每日奖励</p>
              <p className="text-2xl font-semibold">{dailyRange} {currencyName}</p>
              <p className="mt-1 text-xs text-muted-foreground">
                {config?.reset_after_miss ? "漏签会清零连签" : "漏签也会保留连签"}
              </p>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* 加成表 + 历史 */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-5">
        <Card className="border-border/60 lg:col-span-2">
          <CardContent className="p-6">
            <div className="mb-4 flex items-center gap-2">
              <Flame className="h-4 w-4 text-orange-500" />
              <h3 className="text-base font-semibold">连签加成</h3>
            </div>
            {bonusTable.length === 0 ? (
              <p className="text-sm text-muted-foreground">未配置连签加成。</p>
            ) : (
              <div className="space-y-2">
                {bonusTable.map((rule) => {
                  const reached = (summary?.current_streak || 0) >= rule.streak_days;
                  return (
                    <div
                      key={rule.streak_days}
                      className="flex items-center justify-between rounded-lg border border-border/60 px-3 py-2"
                    >
                      <span className="text-sm">连签 {rule.streak_days} 天</span>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-amber-500">
                          +{rule.bonus_points} {currencyName}
                        </span>
                        {reached && <Badge variant="secondary">已达成</Badge>}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="border-border/60 lg:col-span-3">
          <CardContent className="p-6">
            <div className="mb-4 flex items-center justify-between">
              <div className="flex items-center gap-2">
                <CalendarCheck className="h-4 w-4 text-sky-500" />
                <h3 className="text-base font-semibold">最近 30 天签到</h3>
              </div>
              <span className="text-xs text-muted-foreground">共 {history.length} 条</span>
            </div>

            {loading ? (
              <div className="flex h-32 items-center justify-center text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
              </div>
            ) : history.length === 0 ? (
              <p className="text-sm text-muted-foreground">还没有签到记录，先签一次试试吧。</p>
            ) : (
              <div className="space-y-1">
                {history.map((row, idx) => (
                  <div
                    key={`${row.date}-${idx}`}
                    className="flex items-center justify-between rounded-md px-3 py-2 hover:bg-muted/40"
                  >
                    <div className="flex flex-col">
                      <span className="text-sm font-medium">{row.date}</span>
                      <span className="text-xs text-muted-foreground">{formatRelative(row.created_at)}</span>
                    </div>
                    <div className="flex items-center gap-3">
                      <span className="text-xs text-muted-foreground">连签 {row.streak} 天</span>
                      <span className="text-sm font-semibold text-amber-500">
                        +{row.total} {currencyName}
                      </span>
                      {row.bonus_points > 0 && (
                        <Badge variant="outline" className="text-[10px]">
                          含加成 +{row.bonus_points}
                        </Badge>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {error && (
        <p className="text-sm text-destructive">{error}</p>
      )}

      {disabledByConfig && (
        <p className="text-sm text-muted-foreground">
          管理员当前已关闭签到功能。
        </p>
      )}
    </div>
  );
}
