"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Activity,
  AlertTriangle,
  CalendarClock,
  CheckCircle2,
  Clock3,
  FileText,
  Loader2,
  PlayCircle,
  RefreshCw,
  RotateCcw,
  Settings2,
  Square,
  TimerReset,
  XCircle,
  type LucideIcon,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useToast } from "@/hooks/use-toast";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError } from "@/components/layout/page-state";
import { useI18n, type MessageKey, type MessageParams } from "@/lib/i18n";
import {
  api,
  type SchedulerJobItem,
  type SchedulerJobRun,
  type SchedulerSchedulePayload,
  type SchedulerTriggerSpec,
} from "@/lib/api";

type TFunc = (key: MessageKey, params?: MessageParams) => string;

function formatTimestamp(seconds: number | null | undefined): string {
  if (!seconds) return "—";
  const d = new Date(seconds * 1000);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleString();
}

function formatDuration(startSec: number, endSec: number | null): string {
  if (!endSec) return "—";
  const ms = (endSec - startSec) * 1000;
  if (ms < 1000) return `${ms} ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`;
  return `${Math.round(ms / 60_000)} min`;
}

// 后端 summary 字段名 → i18n key 后缀，已知的列在前面以固定展示顺序
const SUMMARY_LABEL_KEYS = [
  "scanned", "disabled", "deleted", "cleared", "failed", "sent", "success",
  "active", "total", "registered", "user_limit", "available_regcodes", "in_group",
  "active_sessions", "emby_online", "enabled", "days_threshold", "preserve_tg_bound",
  "pending_register_excluded", "skipped_pending_emby", "rejoined_pending_review",
  "rejoin_scanned", "rejoin_candidates", "rejoin_expired_skipped", "rejoin_auto_enabled",
  "rejoin_auto_failed", "auto_enable_rejoined", "rejoin_uids", "ban_on_leave",
  "reason_no_account", "reason_no_emby", "reason_disabled", "preserved_bound",
  "roster_size", "bots_in_roster", "admins_excluded", "excluded_total", "targets",
  "dry_run", "not_in_group", "kicked", "skipped", "telegram_bound", "invalid_telegram_id",
  "duplicate_telegram_ids", "rebind_state_mismatch",
  "current", "removed", "reason",
] as const;

const SUMMARY_LABEL_KEY_SET = new Set<string>(SUMMARY_LABEL_KEYS);

function summaryLabel(t: TFunc, key: string): string {
  if (SUMMARY_LABEL_KEY_SET.has(key)) {
    return t(`adminScheduler.summary.${key}` as MessageKey);
  }
  return key;
}

function formatSummaryValue(t: TFunc, value: unknown): string {
  if (value === null || value === undefined) return "—";
  if (typeof value === "boolean") return value ? t("adminScheduler.boolYes") : t("adminScheduler.boolNo");
  if (typeof value === "number") return value.toLocaleString();
  if (Array.isArray(value) || typeof value === "object") return "";
  return String(value);
}

function formatRunType(t: TFunc, run?: SchedulerJobRun | null): string {
  const type = run?.type || (run?.trigger === "manual" ? "manual" : "auto");
  return type === "manual" ? t("adminScheduler.runTypeManual") : t("adminScheduler.runTypeAuto");
}

function renderSummaryChips(t: TFunc, summary: SchedulerJobRun["summary"]) {
  if (!summary || typeof summary !== "object") return null;
  const entries = Object.entries(summary).filter(([, value]) => !Array.isArray(value) && (value === null || typeof value !== "object"));
  if (entries.length === 0) return null;

  // 按已知键的顺序排（其余追加）
  entries.sort(([a], [b]) => {
    const ia = SUMMARY_LABEL_KEYS.indexOf(a as (typeof SUMMARY_LABEL_KEYS)[number]);
    const ib = SUMMARY_LABEL_KEYS.indexOf(b as (typeof SUMMARY_LABEL_KEYS)[number]);
    if (ia === -1 && ib === -1) return a.localeCompare(b);
    if (ia === -1) return 1;
    if (ib === -1) return -1;
    return ia - ib;
  });

  return (
    <div className="mt-2 flex flex-wrap gap-1.5">
      {entries.map(([key, value]) => (
        <Badge key={key} variant="outline" className="text-[10px] font-normal">
          {summaryLabel(t, key)}：{formatSummaryValue(t, value)}
        </Badge>
      ))}
    </div>
  );
}

function describeTriggerSpec(t: TFunc, spec: SchedulerTriggerSpec | undefined | null): string {
  if (!spec) return "—";
  if (spec.type === "manual") return t("adminScheduler.triggerManualOnly");
  if (spec.type === "cron_daily") {
    const hh = String(spec.hour).padStart(2, "0");
    const mm = String(spec.minute).padStart(2, "0");
    return t("adminScheduler.triggerDaily", { time: `${hh}:${mm}` });
  }
  const s = spec.seconds;
  if (s % 3600 === 0) return t("adminScheduler.triggerEveryHours", { count: s / 3600 });
  if (s % 60 === 0) return t("adminScheduler.triggerEveryMinutes", { count: s / 60 });
  return t("adminScheduler.triggerEverySeconds", { count: s });
}

function intervalUnits(t: TFunc) {
  return [
    { value: "minutes", label: t("adminScheduler.unitMinutes"), multiplier: 60 },
    { value: "hours", label: t("adminScheduler.unitHours"), multiplier: 3600 },
  ] as const;
}
type IntervalUnit = "minutes" | "hours";
const INTERVAL_MULTIPLIER: Record<IntervalUnit, number> = { minutes: 60, hours: 3600 };

function secondsToUnit(seconds: number): { value: number; unit: IntervalUnit } {
  if (seconds > 0 && seconds % 3600 === 0) return { value: seconds / 3600, unit: "hours" };
  return { value: Math.max(1, Math.round(seconds / 60)), unit: "minutes" };
}

interface ScheduleEditorProps {
  job: SchedulerJobItem | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSaved: () => Promise<unknown> | unknown;
}

function cleanupSchedulerRuntimeConfig(t: TFunc, jobID: string) {
  if (jobID === "cleanup_no_emby") {
    return {
      hasDays: true,
      title: t("adminScheduler.cleanupNoEmbyTitle"),
      description: t("adminScheduler.cleanupNoEmbyDescription"),
    };
  }
  if (jobID === "cleanup_pending_emby_entitlements") {
    return {
      hasDays: false,
      title: t("adminScheduler.cleanupPendingTitle"),
      description: t("adminScheduler.cleanupPendingDescription"),
    };
  }
  if (jobID === "enforce_group_membership") {
    return {
      hasDays: false,
      hasAutoEnableRejoined: true,
      title: t("adminScheduler.enforceGroupTitle"),
      description: t("adminScheduler.enforceGroupDescription"),
    };
  }
  if (jobID === "cleanup_audit_logs") {
    return {
      hasDays: true,
      hasRetentionDays: true,
      hasMaxEntries: true,
      title: t("adminScheduler.cleanupAuditLogsTitle"),
      description: t("adminScheduler.cleanupAuditLogsDescription"),
    };
  }
  return null;
}

function ScheduleEditor({ job, open, onOpenChange, onSaved }: ScheduleEditorProps) {
  const { toast } = useToast();
  const { t } = useI18n();
  const [type, setType] = useState<SchedulerTriggerSpec["type"]>("cron_daily");
  const [hour, setHour] = useState(0);
  const [minute, setMinute] = useState(0);
  const [intervalValue, setIntervalValue] = useState(1);
  const [intervalUnit, setIntervalUnit] = useState<IntervalUnit>("hours");
  const [cleanupDays, setCleanupDays] = useState("7");
  const [cleanupEnabled, setCleanupEnabled] = useState(false);
  const [saving, setSaving] = useState(false);
  const [resetting, setResetting] = useState(false);

  // 打开时把当前值填进表单
  useEffect(() => {
    if (!open || !job) return;
    const spec = job.trigger_spec;
    const rp = job.runtime_params || {};
    if (spec.type === "manual") {
      setType("manual");
      setHour(0);
      setMinute(0);
      setIntervalValue(1);
      setIntervalUnit("hours");
      setCleanupDays(String(Number(rp.days ?? 7) || 7));
      setCleanupEnabled(job.id === "enforce_group_membership" ? Boolean(rp.auto_enable_rejoined) : Boolean(rp.enabled ?? rp.auto_enabled));
      return;
    }
    setType(spec.type);
    setCleanupDays(String(Number(rp.days ?? 7) || 7));
    setCleanupEnabled(job.id === "enforce_group_membership" ? Boolean(rp.auto_enable_rejoined) : Boolean(rp.enabled ?? rp.auto_enabled));
    if (spec.type === "cron_daily") {
      setHour(spec.hour);
      setMinute(spec.minute);
      const { value, unit } = secondsToUnit(3600);
      setIntervalValue(value);
      setIntervalUnit(unit);
    } else {
      const { value, unit } = secondsToUnit(spec.seconds);
      setIntervalValue(value);
      setIntervalUnit(unit);
      setHour(0);
      setMinute(0);
    }
  }, [open, job]);

  if (!job) return null;
  const cleanupConfig = cleanupSchedulerRuntimeConfig(t, job.id);

  const handleSave = async () => {
    setSaving(true);
    try {
      let payload: SchedulerSchedulePayload;
      if (type === "manual") {
        payload = { type: "manual" };
      } else if (type === "cron_daily") {
        if (hour < 0 || hour > 23 || minute < 0 || minute > 59) {
          toast({ title: t("adminScheduler.invalidTimeTitle"), description: t("adminScheduler.invalidTimeDescription"), variant: "destructive" });
          return;
        }
        payload = { type: "cron_daily", hour: Math.trunc(hour), minute: Math.trunc(minute) };
      } else {
        const multiplier = INTERVAL_MULTIPLIER[intervalUnit];
        const seconds = Math.trunc(intervalValue * multiplier);
        if (seconds < 60) {
          toast({ title: t("adminScheduler.intervalTooShortTitle"), description: t("adminScheduler.intervalTooShortDescription"), variant: "destructive" });
          return;
        }
        if (seconds > 7 * 86400) {
          toast({ title: t("adminScheduler.intervalTooLongTitle"), description: t("adminScheduler.intervalTooLongDescription"), variant: "destructive" });
          return;
        }
        payload = { type: "interval", seconds };
      }
      if (cleanupConfig) {
        const runtimeParams: Record<string, unknown> = {};
        if ("hasAutoEnableRejoined" in cleanupConfig && cleanupConfig.hasAutoEnableRejoined) {
          runtimeParams.auto_enable_rejoined = cleanupEnabled;
        } else {
          runtimeParams.enabled = cleanupEnabled;
        }
        if (cleanupConfig.hasDays) {
          const days = Number(cleanupDays);
          if (!Number.isFinite(days) || days < 1 || days > 3650) {
            toast({ title: t("adminScheduler.cleanupParamInvalidTitle"), description: t("adminScheduler.cleanupParamInvalidDescription"), variant: "destructive" });
            return;
          }
          runtimeParams.days = Math.trunc(days);
        }
        payload.runtime_params = runtimeParams;
      }
      const res = await api.setSchedulerJobSchedule(job.id, payload);
      if (res.success) {
        toast({ title: t("adminScheduler.updatedTitle"), description: cleanupConfig ? t("adminScheduler.triggerAndCleanupSaved") : describeTriggerSpec(t, res.data?.trigger_spec), variant: "success" });
        onOpenChange(false);
        await onSaved();
      } else {
        toast({ title: t("adminScheduler.updateFailedTitle"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("adminScheduler.updateFailedTitle"), description: err.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setSaving(false);
    }
  };

  const handleReset = async () => {
    setResetting(true);
    try {
      const res = await api.resetSchedulerJobSchedule(job.id);
      if (res.success) {
        toast({ title: t("adminScheduler.resetDoneTitle"), description: describeTriggerSpec(t, res.data?.trigger_spec), variant: "success" });
        onOpenChange(false);
        await onSaved();
      } else {
        toast({ title: t("adminScheduler.resetFailedTitle"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("adminScheduler.resetFailedTitle"), description: err.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setResetting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("adminScheduler.editorTitle", { name: job.name })}</DialogTitle>
          <DialogDescription>
            {t("adminScheduler.editorCurrent", { spec: describeTriggerSpec(t, job.trigger_spec) })}
            {job.is_custom
              ? t("adminScheduler.editorCustomSuffix")
              : t("adminScheduler.editorDefaultSuffix", { spec: describeTriggerSpec(t, job.default_trigger_spec) })}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label>{t("adminScheduler.triggerMode")}</Label>
            <Select value={type} onValueChange={(v) => setType(v as SchedulerTriggerSpec["type"])}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="manual">{t("adminScheduler.modeManual")}</SelectItem>
                <SelectItem value="cron_daily">{t("adminScheduler.modeDaily")}</SelectItem>
                <SelectItem value="interval">{t("adminScheduler.modeInterval")}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {type === "manual" ? (
            <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-sm text-amber-700 dark:text-amber-300">
              {t("adminScheduler.manualModeHint")}
            </div>
          ) : type === "cron_daily" ? (
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2">
                <Label>{t("adminScheduler.hourLabel")}</Label>
                <Input
                  type="number"
                  min={0}
                  max={23}
                  value={hour}
                  onChange={(e) => setHour(Number(e.target.value) || 0)}
                />
              </div>
              <div className="space-y-2">
                <Label>{t("adminScheduler.minuteLabel")}</Label>
                <Input
                  type="number"
                  min={0}
                  max={59}
                  value={minute}
                  onChange={(e) => setMinute(Number(e.target.value) || 0)}
                />
              </div>
            </div>
          ) : (
            <div className="grid grid-cols-[1fr_120px] gap-3">
              <div className="space-y-2">
                <Label>{t("adminScheduler.everyLabel")}</Label>
                <Input
                  type="number"
                  min={1}
                  value={intervalValue}
                  onChange={(e) => setIntervalValue(Number(e.target.value) || 1)}
                />
              </div>
              <div className="space-y-2">
                <Label>{t("adminScheduler.unitLabel")}</Label>
                <Select value={intervalUnit} onValueChange={(v) => setIntervalUnit(v as IntervalUnit)}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {intervalUnits(t).map((u) => (
                      <SelectItem key={u.value} value={u.value}>
                        {u.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}

          <p className="text-xs text-muted-foreground">
            {t("adminScheduler.editorFootnote")}
          </p>

          {cleanupConfig && (
            <div className="space-y-3 rounded-xl border border-border/70 bg-muted/30 p-3">
              <div className="space-y-2">
                <Label>{cleanupConfig.title}</Label>
                {cleanupConfig.hasDays && (
                  <Input
                    type="number"
                    min={1}
                    max={3650}
                    value={cleanupDays}
                    onChange={(e) => setCleanupDays(e.target.value)}
                  />
                )}
                <p className="text-xs text-muted-foreground">
                  {cleanupConfig.description}
                </p>
              </div>
              <label className="flex items-start gap-2 rounded-lg border border-border/60 bg-background/60 p-2 text-sm">
                <input
                  type="checkbox"
                  checked={cleanupEnabled}
                  onChange={(e) => setCleanupEnabled(e.target.checked)}
                  className="mt-0.5 h-4 w-4 rounded border-border accent-primary"
                />
                <span>
                  {"hasAutoEnableRejoined" in cleanupConfig && cleanupConfig.hasAutoEnableRejoined
                    ? t("adminScheduler.autoUnbanLabel")
                    : t("adminScheduler.enableAutoCleanupLabel")}
                  <span className="block text-xs text-muted-foreground">
                    {"hasAutoEnableRejoined" in cleanupConfig && cleanupConfig.hasAutoEnableRejoined
                      ? t("adminScheduler.autoUnbanHint")
                      : t("adminScheduler.enableAutoCleanupHint")}
                  </span>
                </span>
              </label>
            </div>
          )}
        </div>

        <DialogFooter className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-between">
          <Button
            variant="ghost"
            size="sm"
            onClick={handleReset}
            disabled={resetting || !job.is_custom}
            title={job.is_custom ? t("adminScheduler.resetTitleCustom") : t("adminScheduler.resetTitleDefault")}
          >
            {resetting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RotateCcw className="mr-2 h-4 w-4" />}
            {t("adminScheduler.resetDefault")}
          </Button>
          <div className="flex gap-2 sm:justify-end">
            <Button variant="outline" onClick={() => onOpenChange(false)}>{t("common.cancel")}</Button>
            <Button onClick={handleSave} disabled={saving}>
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t("common.save")}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function StatusBadge({ t, job, isRunning }: { t: TFunc; job: SchedulerJobItem; isRunning?: boolean }) {
  if (isRunning ?? job.is_running) {
    return (
      <Badge variant="outline" className="text-[10px] border-sky-500/40 text-sky-600 dark:text-sky-400">
        <Loader2 className="mr-1 h-3 w-3 animate-spin" />
        {t("adminScheduler.statusRunning")}
      </Badge>
    );
  }
  if (!job.last_run) {
    return (
      <Badge variant="outline" className="text-[10px] text-muted-foreground">
        {t("adminScheduler.statusNeverRun")}
      </Badge>
    );
  }
  if (job.last_run.status === "success") {
    return (
      <Badge variant="success" className="text-[10px]">
        <CheckCircle2 className="mr-1 h-3 w-3" />
        {t("adminScheduler.statusLastSuccess")}
      </Badge>
    );
  }
  return (
    <Badge variant="destructive" className="text-[10px]">
      <XCircle className="mr-1 h-3 w-3" />
      {t("adminScheduler.statusLastFailed")}
    </Badge>
  );
}

type JobView = "all" | "running" | "failed" | "custom" | "manual";

function MetricCard({
  icon: Icon,
  label,
  value,
  hint,
  tone = "default",
}: {
  icon: LucideIcon;
  label: string;
  value: string | number;
  hint: string;
  tone?: "default" | "sky" | "amber" | "destructive";
}) {
  const toneClass = {
    default: "bg-muted text-muted-foreground",
    sky: "bg-sky-500/10 text-sky-600 dark:text-sky-400",
    amber: "bg-amber-500/10 text-amber-700 dark:text-amber-300",
    destructive: "bg-destructive/10 text-destructive",
  }[tone];

  return (
    <Card className="border-border/80 bg-card/70 backdrop-blur-sm">
      <CardContent className="flex items-start gap-3 p-4">
        <div className={`rounded-2xl p-2 ${toneClass}`}>
          <Icon className="h-4 w-4" />
        </div>
        <div className="min-w-0">
          <p className="text-xs text-muted-foreground">{label}</p>
          <p className="mt-1 truncate text-xl font-semibold tabular-nums">{value}</p>
          <p className="mt-1 truncate text-xs text-muted-foreground">{hint}</p>
        </div>
      </CardContent>
    </Card>
  );
}

function jobIsRunning(job: SchedulerJobItem, running: Record<string, boolean>) {
  return job.is_running || Boolean(running[job.id]);
}

// 哪些任务在手动触发时支持参数面板
const PARAMETERIZED_JOBS = new Set(["cleanup_no_emby", "cleanup_pending_emby_entitlements", "cleanup_audit_logs", "kick_unknown_group_members"]);

export default function AdminSchedulerPage() {
  const { toast } = useToast();
  const { t } = useI18n();
  const [jobs, setJobs] = useState<SchedulerJobItem[]>([]);
  const [running, setRunning] = useState<Record<string, boolean>>({});
  const [terminating, setTerminating] = useState<Record<string, boolean>>({});
  const [rejoinEnabling, setRejoinEnabling] = useState(false);
  const [jobView, setJobView] = useState<JobView>("all");
  const pollTimerRef = useRef<number | null>(null);

  // 日志/历史弹窗
  const [logsJob, setLogsJob] = useState<SchedulerJobItem | null>(null);
  const [logsDetail, setLogsDetail] = useState<SchedulerJobRun | null>(null);
  const [logsHistory, setLogsHistory] = useState<SchedulerJobRun[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);

  // 触发器编辑器
  const [scheduleJob, setScheduleJob] = useState<SchedulerJobItem | null>(null);

  // 参数化触发弹窗
  const [paramJob, setParamJob] = useState<SchedulerJobItem | null>(null);
  const [paramDays, setParamDays] = useState("7");
  const [paramPreserveTg, setParamPreserveTg] = useState(true);
  const [paramIgnoreEnabled, setParamIgnoreEnabled] = useState(true);
  const [paramKickDryRun, setParamKickDryRun] = useState(true);
  const [paramKickMaxPerRun, setParamKickMaxPerRun] = useState("200");

  const loadJobs = useCallback(async () => {
    const res = await api.listSchedulerJobs();
    if (res.success && res.data) {
      setJobs(res.data.jobs || []);
      setRunning({});
    }
    return true;
  }, []);

  const {
    isLoading,
    error,
    execute: refresh,
  } = useAsyncResource(loadJobs, { immediate: true });

  const anyRunning = useMemo(
    () => jobs.some((j) => j.is_running) || Object.values(running).some(Boolean),
    [jobs, running]
  );

  const schedulerHasTimedJobs = useMemo(
    () => jobs.some((j) => !j.manual_only && j.enabled && j.trigger_spec?.type !== "manual"),
    [jobs]
  );

  const schedulerStats = useMemo(() => {
    const runningCount = jobs.filter((job) => jobIsRunning(job, running)).length;
    const failedCount = jobs.filter((job) => job.last_run?.status === "failed").length;
    const customCount = jobs.filter((job) => job.is_custom).length;
    const timedCount = jobs.filter((job) => !job.manual_only && job.enabled && job.trigger_spec?.type !== "manual").length;
    const manualCount = jobs.filter((job) => job.manual_only || job.trigger_spec?.type === "manual").length;
    const nextJob = jobs
      .filter((job) => !job.manual_only && job.enabled && job.next_run_at && job.trigger_spec?.type !== "manual")
      .sort((a, b) => Number(a.next_run_at || 0) - Number(b.next_run_at || 0))[0] || null;
    return { runningCount, failedCount, customCount, timedCount, manualCount, nextJob };
  }, [jobs, running]);

  const filteredJobs = useMemo(() => {
    if (jobView === "running") return jobs.filter((job) => jobIsRunning(job, running));
    if (jobView === "failed") return jobs.filter((job) => job.last_run?.status === "failed");
    if (jobView === "custom") return jobs.filter((job) => job.is_custom);
    if (jobView === "manual") return jobs.filter((job) => job.manual_only || job.trigger_spec?.type === "manual");
    return jobs;
  }, [jobView, jobs, running]);

  useEffect(() => {
    if (!anyRunning) {
      if (pollTimerRef.current) {
        window.clearInterval(pollTimerRef.current);
        pollTimerRef.current = null;
      }
      return;
    }
    if (pollTimerRef.current) return;
    pollTimerRef.current = window.setInterval(() => {
      if (document.visibilityState !== "visible") return;
      void refresh();
    }, 2000);
    return () => {
      if (pollTimerRef.current) {
        window.clearInterval(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    };
  }, [anyRunning, refresh]);

  const runJob = useCallback(
    async (job: SchedulerJobItem, params?: Record<string, unknown>) => {
      setRunning((p) => ({ ...p, [job.id]: true }));
      try {
        const res = await api.triggerSchedulerJob(job.id, params);
        if (res.success) {
          toast({
            title: t("adminScheduler.triggeredTitle", { name: job.name }),
            description: t("adminScheduler.triggeredDescription"),
            variant: "success",
          });
          await refresh();
        } else {
          toast({ title: t("adminScheduler.triggerFailedTitle"), description: res.message, variant: "destructive" });
        }
      } catch (err: any) {
        toast({ title: t("adminScheduler.triggerFailedTitle"), description: err.message || t("common.networkError"), variant: "destructive" });
      } finally {
        setRunning((p) => ({ ...p, [job.id]: false }));
      }
    },
    [refresh, toast, t],
  );

  const handleTrigger = (job: SchedulerJobItem) => {
    if (PARAMETERIZED_JOBS.has(job.id)) {
      // 弹窗收集参数后再发起
      const lastDays = Number(
        (job.runtime_params as Record<string, unknown> | undefined)?.["days"] ??
          (job.last_run?.summary as Record<string, unknown> | undefined)?.["days_threshold"] ??
          7,
      );
      setParamDays(Number.isFinite(lastDays) && lastDays > 0 ? String(Math.trunc(lastDays)) : "7");
      const lastPreserveTg = (job.last_run?.summary as Record<string, unknown> | undefined)?.[
        "preserve_tg_bound"
      ];
      setParamPreserveTg(lastPreserveTg === undefined ? true : Boolean(lastPreserveTg));
      setParamIgnoreEnabled(Boolean((job.runtime_params as Record<string, unknown> | undefined)?.["enabled"] ?? (job.runtime_params as Record<string, unknown> | undefined)?.["auto_enabled"] ?? true));
      setParamKickDryRun(true);
      setParamKickMaxPerRun("200");
      setParamJob(job);
      return;
    }
    void runJob(job);
  };

  const handleTerminate = useCallback(
    async (job: SchedulerJobItem) => {
      setTerminating((p) => ({ ...p, [job.id]: true }));
      try {
        const res = await api.terminateSchedulerJob(job.id);
        if (res.success) {
          toast({ title: t("adminScheduler.terminateRequestedTitle"), description: t("adminScheduler.terminateRequestedDescription", { name: job.name }), variant: "success" });
          await refresh();
        } else {
          toast({ title: t("adminScheduler.terminateFailedTitle"), description: res.message, variant: "destructive" });
        }
      } catch (err: any) {
        toast({ title: t("adminScheduler.terminateFailedTitle"), description: err.message || t("common.networkError"), variant: "destructive" });
      } finally {
        setTerminating((p) => ({ ...p, [job.id]: false }));
      }
    },
    [refresh, toast, t],
  );

  const handleParamConfirm = async () => {
    if (!paramJob) return;
    let params: Record<string, unknown> = {};
    const cleanupConfig = cleanupSchedulerRuntimeConfig(t, paramJob.id);
    if (cleanupConfig) {
      params = {
        ignore_enabled_flag: paramIgnoreEnabled,
      };
      if (cleanupConfig.hasDays) {
        const days = Number(paramDays);
        if (!Number.isFinite(days) || days < 1) {
          toast({ title: t("adminScheduler.daysInvalidTitle"), description: t("adminScheduler.daysInvalidDescription"), variant: "destructive" });
          return;
        }
        params.days = Math.trunc(days);
      }
      if (paramJob.id === "cleanup_no_emby") {
        params.preserve_tg_bound = paramPreserveTg;
      }
    } else if (paramJob.id === "kick_unknown_group_members") {
      const mpr = Number(paramKickMaxPerRun);
      params = {
        dry_run: paramKickDryRun,
        max_per_run: Number.isFinite(mpr) && mpr > 0 ? Math.trunc(mpr) : 200,
      };
    }
    const job = paramJob;
    setParamJob(null);
    await runJob(job, params);
  };

  const handleEnableRejoinedUsers = useCallback(async () => {
    setRejoinEnabling(true);
    try {
      const res = await api.enableRejoinedTelegramUsers();
      if (res.success && res.data) {
        const failed = res.data.failed.length;
        toast({
          title: t("adminScheduler.rejoinEnabledTitle", { count: res.data.enabled }),
          description: failed ? t("adminScheduler.rejoinEnabledFailed", { count: failed }) : t("adminScheduler.rejoinEnabledRevalidated"),
          variant: failed ? "default" : "success",
        });
        await refresh();
      } else {
        toast({ title: t("adminScheduler.enableFailedTitle"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("adminScheduler.enableFailedTitle"), description: err.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setRejoinEnabling(false);
    }
  }, [refresh, toast, t]);

  const openLogs = async (job: SchedulerJobItem) => {
    setLogsJob(job);
    setLogsDetail(null);
    setLogsHistory([]);
    setLogsLoading(true);
    try {
      const [detailRes, historyRes] = await Promise.all([
        api.getSchedulerJobLastRun(job.id),
        api.getSchedulerJobHistory(job.id, 20),
      ]);
      if (detailRes.success) {
        setLogsDetail(detailRes.data?.last_run || null);
      }
      if (historyRes.success) {
        setLogsHistory(historyRes.data?.history || []);
      }
    } catch (err: any) {
      toast({ title: t("adminScheduler.loadLogsFailedTitle"), description: err.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setLogsLoading(false);
    }
  };

  if (error) {
    return <PageError message={error} onRetry={() => void refresh()} />;
  }

  return (
    <div className="space-y-6">
      <Card className="overflow-hidden border-primary/20 bg-gradient-to-br from-primary/10 via-card to-sky-500/10">
        <CardContent className="p-5 sm:p-6">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
            <div className="min-w-0 space-y-3">
              <Badge variant="outline" className="w-fit border-primary/30 bg-background/70 text-primary">
                <CalendarClock className="mr-1 h-3.5 w-3.5" />
                Scheduler Console
              </Badge>
              <div className="space-y-1">
                <h1 className="text-2xl font-bold tracking-tight sm:text-3xl">{t("adminScheduler.title")}</h1>
                <p className="max-w-3xl text-sm leading-relaxed text-muted-foreground">
                  {t("adminScheduler.description")}
                </p>
              </div>
            </div>
            <div className="flex flex-col gap-2 sm:flex-row lg:items-center">
              <Badge variant={anyRunning ? "outline" : "secondary"} className="justify-center py-2 text-xs">
                {anyRunning ? t("adminScheduler.pollingActive") : t("adminScheduler.noRunning")}
              </Badge>
              <Button variant="outline" onClick={() => void refresh()} disabled={isLoading} className="bg-background/70 sm:w-auto">
                {isLoading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
                {t("adminScheduler.refreshJobs")}
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          icon={Activity}
          label={t("adminScheduler.metricRunning")}
          value={schedulerStats.runningCount}
          hint={schedulerStats.runningCount > 0 ? t("adminScheduler.metricRunningHintActive") : t("adminScheduler.metricRunningHintIdle")}
          tone="sky"
        />
        <MetricCard
          icon={CalendarClock}
          label={t("adminScheduler.metricTimed")}
          value={schedulerStats.timedCount}
          hint={t("adminScheduler.metricTimedHint", { count: schedulerStats.manualCount })}
        />
        <MetricCard
          icon={AlertTriangle}
          label={t("adminScheduler.metricFailed")}
          value={schedulerStats.failedCount}
          hint={schedulerStats.failedCount > 0 ? t("adminScheduler.metricFailedHintBad") : t("adminScheduler.metricFailedHintGood")}
          tone={schedulerStats.failedCount > 0 ? "destructive" : "default"}
        />
        <MetricCard
          icon={Clock3}
          label={t("adminScheduler.metricNext")}
          value={schedulerStats.nextJob ? formatTimestamp(schedulerStats.nextJob.next_run_at) : "—"}
          hint={schedulerStats.nextJob ? schedulerStats.nextJob.name : t("adminScheduler.metricNextHintEmpty")}
          tone="amber"
        />
      </div>

      {jobs.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            {isLoading ? t("adminScheduler.loadingShort") : t("adminScheduler.noJobs")}
          </CardContent>
        </Card>
      ) : (
        <>
          {!schedulerHasTimedJobs && (
            <Card className="border-amber-500/30 bg-amber-500/5">
              <CardContent className="flex gap-3 py-4 text-sm text-amber-700 dark:text-amber-300">
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                <span>
                  {t("adminScheduler.noTimedJobsWarning")}
                </span>
              </CardContent>
            </Card>
          )}
          <div className="flex flex-col gap-3 rounded-2xl border border-border/80 bg-card/70 p-3 backdrop-blur-sm sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0 px-1">
              <p className="text-sm font-medium">{t("adminScheduler.jobListTitle")}</p>
              <p className="text-xs text-muted-foreground">{t("adminScheduler.jobListHint")}</p>
            </div>
            <Tabs value={jobView} onValueChange={(value) => setJobView(value as JobView)}>
              <TabsList className="grid h-auto w-full grid-cols-2 gap-1 sm:inline-flex sm:w-auto sm:grid-cols-none">
                <TabsTrigger value="all" className="gap-1.5 px-3">
                  {t("adminScheduler.filterAll")}
                  <span className="rounded-full bg-muted px-1.5 text-[10px] text-muted-foreground">{jobs.length}</span>
                </TabsTrigger>
                <TabsTrigger value="running" className="gap-1.5 px-3">
                  {t("adminScheduler.filterRunning")}
                  <span className="rounded-full bg-muted px-1.5 text-[10px] text-muted-foreground">{schedulerStats.runningCount}</span>
                </TabsTrigger>
                <TabsTrigger value="failed" className="gap-1.5 px-3">
                  {t("adminScheduler.filterFailed")}
                  <span className="rounded-full bg-muted px-1.5 text-[10px] text-muted-foreground">{schedulerStats.failedCount}</span>
                </TabsTrigger>
                <TabsTrigger value="custom" className="gap-1.5 px-3">
                  {t("adminScheduler.filterCustom")}
                  <span className="rounded-full bg-muted px-1.5 text-[10px] text-muted-foreground">{schedulerStats.customCount}</span>
                </TabsTrigger>
                <TabsTrigger value="manual" className="gap-1.5 px-3">
                  {t("adminScheduler.filterManual")}
                  <span className="rounded-full bg-muted px-1.5 text-[10px] text-muted-foreground">{schedulerStats.manualCount}</span>
                </TabsTrigger>
              </TabsList>
            </Tabs>
          </div>

          {filteredJobs.length === 0 ? (
            <Card className="border-dashed">
              <CardContent className="py-10 text-center text-sm text-muted-foreground">
                {t("adminScheduler.noJobsInFilter")}
              </CardContent>
            </Card>
          ) : (
            <div className="grid gap-4 xl:grid-cols-2 2xl:grid-cols-3">
              {filteredJobs.map((job) => {
                const lr = job.last_run;
                const isRunning = jobIsRunning(job, running);
                const rejoinCandidates = Number((lr?.summary as Record<string, unknown> | null | undefined)?.["rejoin_candidates"] ?? 0);
                const cardClass = isRunning
                  ? "border-sky-500/35 ring-1 ring-sky-500/20"
                  : lr?.status === "failed"
                    ? "border-destructive/35"
                    : "border-border/80";
                return (
                  <Card key={job.id} className={`flex flex-col overflow-hidden bg-card/70 backdrop-blur-sm ${cardClass}`}>
                    <CardHeader className="space-y-3 border-b border-border/60 bg-muted/15 pb-4">
                      <div className="flex flex-wrap items-center gap-2">
                        <StatusBadge t={t} job={job} isRunning={isRunning} />
                        {job.manual_only && (
                          <Badge variant="outline" className="border-amber-500/40 text-[10px] text-amber-600 dark:text-amber-400">
                            {t("adminScheduler.manualOnly")}
                          </Badge>
                        )}
                        {!job.manual_only && job.is_custom && (
                          <Badge variant="outline" className="text-[10px]">{t("adminScheduler.customTrigger")}</Badge>
                        )}
                        {job.auto_disabled && (
                          <Badge variant="outline" className="border-muted-foreground/30 text-[10px] text-muted-foreground">
                            {t("adminScheduler.autoDisabled")}
                          </Badge>
                        )}
                      </div>
                      <div className="space-y-1">
                        <CardTitle className="text-base leading-snug">{job.name}</CardTitle>
                        <CardDescription className="break-words leading-relaxed">{job.description}</CardDescription>
                      </div>
                    </CardHeader>

                    <CardContent className="flex flex-1 flex-col gap-4 p-4">
                      <div className="grid gap-2 rounded-2xl border border-border/60 bg-muted/20 p-3 text-xs sm:grid-cols-2">
                        <div className="flex min-w-0 items-center gap-2">
                          <CalendarClock className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                          <span className="truncate">{t("adminScheduler.triggerLabel")}{describeTriggerSpec(t, job.trigger_spec)}</span>
                        </div>
                        <div className="flex min-w-0 items-center gap-2">
                          <TimerReset className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                          <span className="truncate">{t("adminScheduler.nextLabel")}{job.manual_only ? t("adminScheduler.triggerManualOnly") : formatTimestamp(job.next_run_at)}</span>
                        </div>
                        <div className="flex min-w-0 items-center gap-2">
                          <Clock3 className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                          <span className="truncate">{t("adminScheduler.autoLabel")}{formatTimestamp(job.last_auto_run_at)}</span>
                        </div>
                        <div className="flex min-w-0 items-center gap-2">
                          <Activity className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                          <span className="truncate">{t("adminScheduler.manualLabel")}{formatTimestamp(job.last_manual_run_at)}</span>
                        </div>
                      </div>

                      <div className="rounded-2xl border border-border/60 bg-background/55 p-3 text-xs">
                        <div className="flex flex-wrap items-center justify-between gap-2">
                          <p className="font-medium">{t("adminScheduler.lastRunTitle")}</p>
                          {lr ? (
                            <Badge variant={lr.status === "success" ? "success" : lr.status === "failed" ? "destructive" : "outline"} className="text-[10px]">
                              {lr.status}
                            </Badge>
                          ) : (
                            <Badge variant="outline" className="text-[10px] text-muted-foreground">{t("adminScheduler.none")}</Badge>
                          )}
                        </div>
                        {lr ? (
                          <div className="mt-3 space-y-1">
                            <p><span className="text-muted-foreground">{t("adminScheduler.startedLabel")}</span>{formatTimestamp(lr.started_at)}</p>
                            <p><span className="text-muted-foreground">{t("adminScheduler.finishedLabel")}</span>{formatTimestamp(lr.finished_at)}</p>
                            <p><span className="text-muted-foreground">{t("adminScheduler.durationLabel")}</span>{formatDuration(lr.started_at, lr.finished_at)}</p>
                            <p><span className="text-muted-foreground">{t("adminScheduler.typeLabel")}</span>{formatRunType(t, lr)}{lr.trigger === "startup" ? t("adminScheduler.atStartup") : ""}</p>
                            {lr.error && <p className="break-words text-destructive">{t("adminScheduler.errorLabel")}{lr.error}</p>}
                            {renderSummaryChips(t, lr.summary)}
                          </div>
                        ) : (
                          <p className="mt-3 text-muted-foreground">{t("adminScheduler.noRunRecord")}</p>
                        )}
                      </div>

                      {job.id === "enforce_group_membership" && rejoinCandidates > 0 && (
                        <div className="space-y-2 rounded-2xl border border-amber-500/30 bg-amber-500/5 p-3 text-xs">
                          <p className="text-amber-700 dark:text-amber-300">
                            {t("adminScheduler.rejoinDetected", { count: rejoinCandidates })}
                          </p>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => void handleEnableRejoinedUsers()}
                            disabled={rejoinEnabling || isRunning}
                            className="h-8 border-amber-500/40 text-amber-700 hover:text-amber-800 dark:text-amber-300 dark:hover:text-amber-200"
                          >
                            {rejoinEnabling ? <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" /> : <CheckCircle2 className="mr-2 h-3.5 w-3.5" />}
                            {t("adminScheduler.enableRejoinedButton")}
                          </Button>
                        </div>
                      )}

                      <div className="mt-auto flex flex-col gap-2 sm:flex-row">
                        <Button onClick={() => void handleTrigger(job)} disabled={isRunning} className="sm:flex-1">
                          {isRunning ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <PlayCircle className="mr-2 h-4 w-4" />}
                          {isRunning ? t("adminScheduler.running") : t("adminScheduler.runNow")}
                        </Button>
                        <div className={`grid gap-2 ${job.manual_only ? "grid-cols-2" : "grid-cols-3"} sm:flex`}>
                          <Button
                            variant="destructive"
                            size="sm"
                            onClick={() => void handleTerminate(job)}
                            disabled={!isRunning || Boolean(terminating[job.id])}
                          >
                            {terminating[job.id] ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Square className="mr-2 h-4 w-4" />}
                            {t("adminScheduler.terminate")}
                          </Button>
                          {!job.manual_only && (
                            <Button variant="outline" size="sm" onClick={() => setScheduleJob(job)}>
                              <Settings2 className="mr-2 h-4 w-4" />
                              {t("adminScheduler.edit")}
                            </Button>
                          )}
                          <Button variant="outline" size="sm" onClick={() => void openLogs(job)}>
                            <FileText className="mr-2 h-4 w-4" />
                            {t("adminScheduler.logs")}
                          </Button>
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                );
              })}
            </div>
          )}
        </>
      )}

      <ScheduleEditor
        job={scheduleJob}
        open={Boolean(scheduleJob)}
        onOpenChange={(open) => { if (!open) setScheduleJob(null); }}
        onSaved={refresh}
      />

      {/* 参数化手动触发：cleanup jobs / kick_unknown_group_members */}
      <Dialog
        open={Boolean(paramJob)}
        onOpenChange={(open) => { if (!open) setParamJob(null); }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("adminScheduler.manualTriggerTitle", { name: paramJob?.name ?? "" })}</DialogTitle>
            <DialogDescription>
              {t("adminScheduler.manualTriggerDescription")}
            </DialogDescription>
          </DialogHeader>

          {paramJob && cleanupSchedulerRuntimeConfig(t, paramJob.id) && (
            <div className="space-y-4">
              {cleanupSchedulerRuntimeConfig(t, paramJob.id)?.hasDays ? (
                <div className="space-y-2">
                  <Label>{cleanupSchedulerRuntimeConfig(t, paramJob.id)?.title}</Label>
                  <Input
                    type="number"
                    min={1}
                    value={paramDays}
                    onChange={(e) => setParamDays(e.target.value)}
                  />
                  <p className="text-xs text-muted-foreground">
                    {cleanupSchedulerRuntimeConfig(t, paramJob.id)?.description}
                  </p>
                </div>
              ) : (
                <div className="space-y-2">
                  <Label>{cleanupSchedulerRuntimeConfig(t, paramJob.id)?.title}</Label>
                  <p className="text-xs text-muted-foreground">
                    {cleanupSchedulerRuntimeConfig(t, paramJob.id)?.description}
                  </p>
                </div>
              )}
              {paramJob.id === "cleanup_no_emby" && (
                <label className="flex items-start gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={paramPreserveTg}
                    onChange={(e) => setParamPreserveTg(e.target.checked)}
                    className="mt-0.5 h-4 w-4 rounded border-border accent-primary"
                  />
                  <span>
                    {t("adminScheduler.preserveTgLabel")}
                    <span className="block text-xs text-muted-foreground">
                      {t("adminScheduler.preserveTgHint")}
                    </span>
                  </span>
                </label>
              )}
              <label className="flex items-start gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={paramIgnoreEnabled}
                  onChange={(e) => setParamIgnoreEnabled(e.target.checked)}
                  className="mt-0.5 h-4 w-4 rounded border-border accent-primary"
                />
                <span>
                  {t("adminScheduler.ignoreEnabledLabel")}
                  <span className="block text-xs text-muted-foreground">
                    {t("adminScheduler.ignoreEnabledHint")}
                  </span>
                </span>
              </label>
            </div>
          )}

          {paramJob?.id === "kick_unknown_group_members" && (
            <div className="space-y-4">
              <label className="flex items-start gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={paramKickDryRun}
                  onChange={(e) => setParamKickDryRun(e.target.checked)}
                  className="mt-0.5 h-4 w-4 rounded border-border accent-primary"
                />
                <span>
                  {t("adminScheduler.dryRunLabel")}
                  <span className="block text-xs text-muted-foreground">
                    {t("adminScheduler.dryRunHint")}
                  </span>
                </span>
              </label>
              <div className="space-y-2">
                <Label>{t("adminScheduler.maxPerRunLabel")}</Label>
                <Input
                  type="number"
                  min={1}
                  max={500}
                  value={paramKickMaxPerRun}
                  onChange={(e) => setParamKickMaxPerRun(e.target.value)}
                />
                <p className="text-xs text-muted-foreground">
                  {t("adminScheduler.maxPerRunHint")}
                </p>
              </div>
              <p className="rounded-md border border-amber-500/40 bg-amber-500/5 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
                {t("adminScheduler.kickWarning")}
              </p>
            </div>
          )}

          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={() => setParamJob(null)}>{t("common.cancel")}</Button>
            <Button onClick={handleParamConfirm}>{t("adminScheduler.executeNow")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(logsJob)} onOpenChange={(open) => { if (!open) { setLogsJob(null); setLogsDetail(null); setLogsHistory([]); } }}>
        <DialogContent className="max-h-[85vh] w-[92vw] max-w-3xl overflow-hidden p-0 sm:max-w-3xl">
          <DialogHeader className="border-b p-4">
            <DialogTitle>{t("adminScheduler.runLogsTitle", { name: logsJob?.name ?? "" })}</DialogTitle>
            <DialogDescription className="break-words">
              {logsJob?.description}
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-[70vh] space-y-4 overflow-y-auto p-4">
            {logsLoading ? (
              <div className="flex items-center justify-center py-10">
                <Loader2 className="h-6 w-6 animate-spin text-primary" />
              </div>
            ) : !logsDetail ? (
              <p className="text-center text-sm text-muted-foreground">{t("adminScheduler.noRunRecord")}</p>
            ) : (
              <>
                <div className="rounded-md border border-border/60 bg-muted/30 p-3 text-xs space-y-1">
                  <p><span className="text-muted-foreground">{t("adminScheduler.statusLabel")}</span>{logsDetail.status}</p>
                  <p><span className="text-muted-foreground">{t("adminScheduler.startLabel")}</span>{formatTimestamp(logsDetail.started_at)}</p>
                  <p><span className="text-muted-foreground">{t("adminScheduler.endLabel")}</span>{formatTimestamp(logsDetail.finished_at)}</p>
                  <p><span className="text-muted-foreground">{t("adminScheduler.durationLabel")}</span>{formatDuration(logsDetail.started_at, logsDetail.finished_at)}</p>
                  <p><span className="text-muted-foreground">{t("adminScheduler.typeLabel")}</span>{formatRunType(t, logsDetail)}</p>
                  {logsDetail.error && (
                    <p className="break-words text-destructive">{t("adminScheduler.errorLabel")}{logsDetail.error}</p>
                  )}
                  {renderSummaryChips(t, logsDetail.summary)}
                </div>

                {logsDetail.logs && logsDetail.logs.length > 0 ? (
                  <div>
                    <p className="mb-1 text-xs font-medium text-muted-foreground">{t("adminScheduler.lastLog")}</p>
                    <pre className="max-h-72 overflow-auto rounded-md border border-border/60 bg-background p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-words">
                      {logsDetail.logs.join("\n")}
                    </pre>
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground">{t("adminScheduler.noLogOutput")}</p>
                )}

                {logsHistory.length > 0 && (
                  <div>
                    <p className="mb-2 text-xs font-medium text-muted-foreground">{t("adminScheduler.historyRuns", { count: logsHistory.length })}</p>
                    <div className="space-y-2">
                      {logsHistory.map((run) => (
                        <details
                          key={run.id || `${run.started_at}-${run.status}`}
                          className="rounded-md border border-border/40 px-3 py-2 text-xs"
                        >
                          <summary className="flex cursor-pointer list-none flex-wrap items-center justify-between gap-2">
                            <span className="font-mono text-muted-foreground">
                              {formatTimestamp(run.started_at)}
                            </span>
                            <span className="flex items-center gap-2">
                              <Badge
                                variant={run.status === "success" ? "success" : run.status === "failed" ? "destructive" : "outline"}
                                className="text-[10px]"
                              >
                                {run.status}
                              </Badge>
                              <span className="text-muted-foreground">{formatDuration(run.started_at, run.finished_at)}</span>
                              <span className="text-muted-foreground">[{formatRunType(t, run)}]</span>
                              <span className="text-muted-foreground">{t("adminScheduler.logLineCount", { count: run.logs?.length || 0 })}</span>
                            </span>
                          </summary>
                          <div className="mt-2 space-y-2 border-t border-border/40 pt-2">
                            {run.error && <p className="break-words text-destructive">{t("adminScheduler.errorLabel")}{run.error}</p>}
                            {renderSummaryChips(t, run.summary)}
                            {run.logs && run.logs.length > 0 ? (
                              <pre className="max-h-60 overflow-auto rounded-md border border-border/60 bg-background p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-words">
                                {run.logs.join("\n")}
                              </pre>
                            ) : (
                              <p className="text-muted-foreground">{t("adminScheduler.noLogOutput")}</p>
                            )}
                          </div>
                        </details>
                      ))}
                    </div>
                  </div>
                )}
              </>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
