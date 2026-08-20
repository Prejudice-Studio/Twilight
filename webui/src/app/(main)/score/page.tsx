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
import { Switch } from "@/components/ui/switch";
import { useToast } from "@/hooks/use-toast";
import { useAuthStore } from "@/store/auth";
import { useI18n } from "@/lib/i18n";
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

function formatRelative(ts: number, locale: string): string {
  if (!ts || ts <= 0) return "—";
  try {
    return new Date(ts * 1000).toLocaleString(locale);
  } catch {
    return "—";
  }
}

export default function ScorePage() {
  const { toast } = useToast();
  const { user, fetchUser, setUser } = useAuthStore();
  const { locale, t } = useI18n();
  const [summary, setSummary] = useState<SigninSummary | null>(null);
  const [config, setConfig] = useState<SigninPublicConfig | null>(null);
  const [history, setHistory] = useState<SigninHistoryRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [signing, setSigning] = useState(false);
  const [renewing, setRenewing] = useState(false);
  const [updatingAutoRenewal, setUpdatingAutoRenewal] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const currencyName = summary?.currency_name || config?.currency_name || t("score.defaultCurrency");
  const renewal = summary?.renewal || config?.renewal;
  const hasEmbyBinding = Boolean(user?.emby_bound || user?.emby_id);

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
      setError(err instanceof Error ? err.message : t("score.loadFailed"));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const handleSignin = async () => {
    if (signing) return;
    setSigning(true);
    try {
      const res = await api.signinNow();
      if (res.success && res.data) {
        const bonus = res.data.bonus_points > 0 ? t("score.streakBonus", { points: res.data.bonus_points }) : "";
        if (res.data.created === false || res.data.total_today <= 0) {
          toast({
            title: t("score.alreadySigned"),
            description: t("score.currentStreak", { days: res.data.current_streak }),
          });
          await reload();
          return;
        }
        toast({
          title: t("score.signinSuccess", { points: res.data.total_today, currency: res.data.currency_name }),
          description: t("score.signinSuccessDescription", { days: res.data.current_streak, bonus }),
          variant: "success",
        });
        await reload();
      } else {
        toast({
          title: t("score.signinFailed"),
          description: res.message,
          variant: "destructive",
        });
        await reload();
      }
    } catch (err) {
      toast({
        title: t("score.signinFailed"),
        description: err instanceof Error ? err.message : t("common.networkError"),
        variant: "destructive",
      });
    } finally {
      setSigning(false);
    }
  };

  const handleRenewal = async () => {
    if (renewing || !renewal?.enabled) return;
    setRenewing(true);
    try {
      const res = await api.renewWithSigninCurrency();
      if (res.success && res.data) {
        toast({
          title: t("signinRenewal.successTitle"),
          description: t("signinRenewal.successDescription", { spent: res.data.spent_points, currencyName: res.data.currency_name, expireStatus: res.data.expire_status }),
          variant: "success",
        });
        if (res.data.user) setUser(res.data.user);
        else await fetchUser();
        await reload();
      } else {
        toast({ title: t("signinRenewal.failureTitle"), description: res.message, variant: "destructive" });
        await reload();
      }
    } catch (err) {
      toast({
        title: t("signinRenewal.failureTitle"),
        description: err instanceof Error ? err.message : t("common.networkError"),
        variant: "destructive",
      });
    } finally {
      setRenewing(false);
    }
  };

  const handleAutoRenewalChange = async (enabled: boolean) => {
    if (updatingAutoRenewal || !renewal?.auto_renewal_enabled || !renewal.auto_renewal_available) return;
    setUpdatingAutoRenewal(true);
    try {
      const res = await api.updateSigninAutoRenewal(enabled);
      if (!res.success) {
        toast({ title: t("signinRenewal.autoUpdateFailed"), description: res.message, variant: "destructive" });
        return;
      }
      setSummary((current) => current?.renewal ? {
        ...current,
        renewal: { ...current.renewal, auto_renewal_user_enabled: enabled },
      } : current);
      if (res.data) setUser(res.data);
      toast({
        title: enabled ? t("signinRenewal.autoEnabledTitle") : t("signinRenewal.autoDisabledTitle"),
        description: enabled
          ? t("signinRenewal.autoEnabledDescription", { cost: renewal.cost, currencyName, days: renewal.days })
          : t("signinRenewal.autoDisabledDescription"),
        variant: "success",
      });
    } catch (err) {
      toast({
        title: t("signinRenewal.autoUpdateFailed"),
        description: err instanceof Error ? err.message : t("common.networkError"),
        variant: "destructive",
      });
    } finally {
      setUpdatingAutoRenewal(false);
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

  if (!loading && disabledByConfig) {
    return (
      <Card className="border-border/60">
        <CardContent className="flex min-h-[320px] flex-col items-center justify-center gap-3 p-8 text-center">
          <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
            <Coins className="h-7 w-7" />
          </div>
          <div>
            <h1 className="text-xl font-semibold">{t("score.disabledTitle")}</h1>
            <p className="mt-2 text-sm text-muted-foreground">{t("score.disabledDescription")}</p>
          </div>
        </CardContent>
      </Card>
    );
  }

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
              <div className="min-w-0">
                <p className="text-sm text-muted-foreground whitespace-nowrap">{t("score.myCurrency", { currency: currencyName })}</p>
                <div className="flex items-baseline gap-2 whitespace-nowrap">
                  <p className="text-4xl font-bold tracking-tight">
                    {summary?.current_points ?? 0}
                  </p>
                  <span className="text-base text-muted-foreground">{currencyName}</span>
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t("score.totalEarned", { points: summary?.total_points ?? 0, currency: currencyName })}
                </p>
              </div>
            </div>

            <div className="flex flex-col items-stretch gap-2 md:items-end">
              {renewal?.enabled && (
                <Button
                  variant="secondary"
                  size="lg"
                  onClick={handleRenewal}
                  disabled={renewing || !renewal.affordable || !hasEmbyBinding}
                  title={!hasEmbyBinding ? t("signinRenewal.requiresEmby") : undefined}
                  className="min-w-[180px]"
                >
                  {renewing ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <CalendarCheck className="mr-2 h-4 w-4" />
                  )}
                  {t("signinRenewal.actionLabel", { cost: renewal.cost, currencyName, days: renewal.days })}
                </Button>
              )}
              {renewal?.enabled && !hasEmbyBinding && (
                <p className="max-w-[280px] text-xs text-muted-foreground md:text-right">
                  {t("signinRenewal.requiresEmby")}
                </p>
              )}
              {renewal?.auto_renewal_enabled && (
                <div className="flex w-full min-w-0 items-center justify-between gap-3 rounded-md border border-border/60 bg-muted/30 px-3 py-2 md:w-[320px]">
                  <label htmlFor="signin-auto-renewal" className="min-w-0 flex-1 cursor-pointer">
                    <span className="block text-sm font-medium leading-5">{t("signinRenewal.autoLabel")}</span>
                    <span className="mt-0.5 block text-xs leading-4 text-muted-foreground">
                      {renewal.auto_renewal_available
                        ? t("signinRenewal.autoDescription", { cost: renewal.cost, currencyName, days: renewal.days })
                        : t("signinRenewal.autoUnavailable")}
                    </span>
                  </label>
                  <Switch
                    id="signin-auto-renewal"
                    checked={renewal.auto_renewal_user_enabled === true}
                    onCheckedChange={handleAutoRenewalChange}
                    disabled={updatingAutoRenewal || !renewal.auto_renewal_available}
                    aria-label={t("signinRenewal.autoLabel")}
                    className="shrink-0"
                  />
                </div>
              )}
              <Button
                size="lg"
                onClick={handleSignin}
                disabled={signing || todaySigned || disabledByConfig}
                className="min-w-[180px]"
              >
                {signing ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" /> {t("score.signing")}
                  </>
                ) : todaySigned ? (
                  <>
                    <CalendarCheck className="mr-2 h-4 w-4" /> {t("score.alreadySigned")}
                  </>
                ) : (
                  <>
                    <Sparkles className="mr-2 h-4 w-4" /> {t("score.signinNow")}
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
                {t("common.refresh")}
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
              <p className="text-xs text-muted-foreground">{t("score.currentStreakTitle")}</p>
              <p className="text-2xl font-semibold">{t("score.days", { days: summary?.current_streak ?? 0 })}</p>
              {config?.streak_bonus_enabled === false ? (
                <p className="mt-1 text-xs text-muted-foreground">{t("score.bonusDisabled")}</p>
              ) : summary?.next_bonus_in_days && summary.next_bonus_points ? (
                <p className="mt-1 text-xs text-muted-foreground">
                  {t("score.nextBonus", { days: summary.next_bonus_in_days, points: summary.next_bonus_points, currency: currencyName })}
                </p>
              ) : (
                <p className="mt-1 text-xs text-muted-foreground">{t("score.noMoreBonus")}</p>
              )}
            </div>
          </CardContent>
        </Card>

        <Card className="border-border/60">
          <CardContent className="flex items-center gap-4 p-5">
            <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-info/10 text-info">
              <Trophy className="h-5 w-5" />
            </div>
            <div>
              <p className="text-xs text-muted-foreground">{t("score.longestStreak")}</p>
              <p className="text-2xl font-semibold">{t("score.days", { days: summary?.longest_streak ?? 0 })}</p>
              <p className="mt-1 text-xs text-muted-foreground">{t("score.lastSignin", { date: formatDate(summary?.last_signin_date) })}</p>
            </div>
          </CardContent>
        </Card>

        <Card className="border-border/60">
          <CardContent className="flex items-center gap-4 p-5">
            <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-sky-500/15 text-sky-500">
              <Sparkles className="h-5 w-5" />
            </div>
            <div>
              <p className="text-xs text-muted-foreground">{t("score.dailyReward")}</p>
              <p className="text-2xl font-semibold">{dailyRange} {currencyName}</p>
              <p className="mt-1 text-xs text-muted-foreground">
                {config?.reset_after_miss ? t("score.missReset") : t("score.missKeep")}
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
              <h3 className="text-base font-semibold">{t("score.bonusTitle")}</h3>
              {config && config.streak_bonus_enabled === false && (
                <Badge variant="outline" className="ml-1 text-[10px]">{t("score.closed")}</Badge>
              )}
            </div>
            {config && config.streak_bonus_enabled === false ? (
              <p className="text-sm text-muted-foreground">
                {t("score.bonusDisabledDescription", { currency: currencyName })}
              </p>
            ) : bonusTable.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t("score.bonusEmpty")}</p>
            ) : (
              <div className="space-y-2">
                {bonusTable.map((rule) => {
                  const reached = (summary?.current_streak || 0) >= rule.streak_days;
                  return (
                    <div
                      key={rule.streak_days}
                      className="flex items-center justify-between rounded-lg border border-border/60 px-3 py-2"
                    >
                      <span className="text-sm">{t("score.streakDays", { days: rule.streak_days })}</span>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-amber-500">
                          +{rule.bonus_points} {currencyName}
                        </span>
                        {reached && <Badge variant="secondary">{t("score.achieved")}</Badge>}
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
                <h3 className="text-base font-semibold">{t("score.historyTitle")}</h3>
              </div>
              <span className="text-xs text-muted-foreground">{t("score.historyCount", { count: history.length })}</span>
            </div>

            {loading ? (
              <div className="flex h-32 items-center justify-center text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
              </div>
            ) : history.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t("score.historyEmpty")}</p>
            ) : (
              <div className="space-y-1">
                {history.map((row, idx) => (
                  <div
                    key={`${row.date}-${idx}`}
                    className="flex items-center justify-between rounded-md px-3 py-2 hover:bg-muted/40"
                  >
                    <div className="flex flex-col">
                      <span className="text-sm font-medium">{row.date}</span>
                      <span className="text-xs text-muted-foreground">{formatRelative(row.created_at, locale)}</span>
                    </div>
                    <div className="flex items-center gap-3">
                      <span className="text-xs text-muted-foreground">{t("score.streakDays", { days: row.streak })}</span>
                      <span className="text-sm font-semibold text-amber-500">
                        +{row.total} {currencyName}
                      </span>
                      {row.bonus_points > 0 && (
                        <Badge variant="outline" className="text-[10px]">
                          {t("score.includesBonus", { points: row.bonus_points })}
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
          {t("score.disabledFooter")}
        </p>
      )}
    </div>
  );
}
