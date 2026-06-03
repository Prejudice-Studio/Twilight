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
import {
  api,
  type SchedulerJobItem,
  type SchedulerJobRun,
  type SchedulerSchedulePayload,
  type SchedulerTriggerSpec,
} from "@/lib/api";

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

// 把后端的 summary 字段名翻译成中文，已知的列在前面
const SUMMARY_LABELS: Record<string, string> = {
  scanned: "扫描",
  disabled: "禁用",
  deleted: "删除",
  cleared: "清理资格",
  failed: "失败",
  sent: "发送",
  success: "成功",
  active: "活跃",
  total: "总数",
  registered: "注册",
  user_limit: "上限",
  available_regcodes: "可用注册码",
  in_group: "仍在群",
  active_sessions: "活跃会话",
  emby_online: "Emby 在线",
  enabled: "启用",
  days_threshold: "阈值(天)",
  preserve_tg_bound: "保留TG绑定",
  pending_register_excluded: "注册队列排除",
  skipped_pending_emby: "跳过开通资格",
  rejoined_pending_review: "回群待复核",
  rejoin_scanned: "复核扫描",
  rejoin_candidates: "待复核恢复",
  rejoin_expired_skipped: "过期跳过",
  rejoin_auto_enabled: "自动恢复",
  rejoin_auto_failed: "自动恢复失败",
  auto_enable_rejoined: "回群自动启用",
  rejoin_uids: "待复核 UID",
  ban_on_leave: "退群永封",
  reason_no_account: "无系统账号",
  reason_no_emby: "无 Emby",
  reason_disabled: "已禁用",
  preserved_bound: "已绑 Emby 保留",
  roster_size: "花名册总数",
  bots_in_roster: "花名册 Bot",
  admins_excluded: "排除管理员",
  excluded_total: "排除合计",
  targets: "目标数",
  dry_run: "干跑",
  not_in_group: "已不在群",
  kicked: "已踢出",
  skipped: "跳过",
  telegram_bound: "TG绑定",
  invalid_telegram_id: "非法TGID",
  duplicate_telegram_ids: "重复TGID",
  rebind_state_mismatch: "换绑状态异常",
};

function formatSummaryValue(value: unknown): string {
  if (value === null || value === undefined) return "—";
  if (typeof value === "boolean") return value ? "是" : "否";
  if (typeof value === "number") return value.toLocaleString();
  if (Array.isArray(value) || typeof value === "object") return "";
  return String(value);
}

function formatRunType(run?: SchedulerJobRun | null): string {
  const type = run?.type || (run?.trigger === "manual" ? "manual" : "auto");
  return type === "manual" ? "手动" : "自动";
}

function renderSummaryChips(summary: SchedulerJobRun["summary"]) {
  if (!summary || typeof summary !== "object") return null;
  const entries = Object.entries(summary).filter(([, value]) => !Array.isArray(value) && (value === null || typeof value !== "object"));
  if (entries.length === 0) return null;

  // 按已知键的顺序排（其余追加）
  const knownOrder = Object.keys(SUMMARY_LABELS);
  entries.sort(([a], [b]) => {
    const ia = knownOrder.indexOf(a);
    const ib = knownOrder.indexOf(b);
    if (ia === -1 && ib === -1) return a.localeCompare(b);
    if (ia === -1) return 1;
    if (ib === -1) return -1;
    return ia - ib;
  });

  return (
    <div className="mt-2 flex flex-wrap gap-1.5">
      {entries.map(([key, value]) => (
        <Badge key={key} variant="outline" className="text-[10px] font-normal">
          {SUMMARY_LABELS[key] || key}：{formatSummaryValue(value)}
        </Badge>
      ))}
    </div>
  );
}

function describeTriggerSpec(spec: SchedulerTriggerSpec | undefined | null): string {
  if (!spec) return "—";
  if (spec.type === "manual") return "仅手动触发";
  if (spec.type === "cron_daily") {
    const hh = String(spec.hour).padStart(2, "0");
    const mm = String(spec.minute).padStart(2, "0");
    return `每日 ${hh}:${mm}`;
  }
  const s = spec.seconds;
  if (s % 3600 === 0) return `每 ${s / 3600} 小时`;
  if (s % 60 === 0) return `每 ${s / 60} 分钟`;
  return `每 ${s} 秒`;
}

const INTERVAL_UNITS = [
  { value: "minutes", label: "分钟", multiplier: 60 },
  { value: "hours", label: "小时", multiplier: 3600 },
] as const;
type IntervalUnit = (typeof INTERVAL_UNITS)[number]["value"];

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

function cleanupSchedulerRuntimeConfig(jobID: string) {
  if (jobID === "cleanup_no_emby") {
    return {
      hasDays: true,
      title: "Web 账号清理参数",
      description: "注册超过该天数、没有 Emby 且没有 Emby 开通资格的 Web 账号会被清理。",
    };
  }
  if (jobID === "cleanup_pending_emby_entitlements") {
    return {
      hasDays: false,
      title: "Emby 开通资格清理参数",
      description: "每次执行会收回所有尚未创建 Emby 的开通资格，不删除 Web 账号。",
    };
  }
  if (jobID === "enforce_group_membership") {
    return {
      hasDays: false,
      hasAutoEnableRejoined: true,
      title: "群成员校验参数",
      description: "检测到用户回到群里且账号未过期时，是否自动解禁并启用其 Emby 和系统账号。关闭时仅标记为待审核。注意：ban_on_leave 开启时自动解禁不会生效。",
    };
  }
  return null;
}

function ScheduleEditor({ job, open, onOpenChange, onSaved }: ScheduleEditorProps) {
  const { toast } = useToast();
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
  const cleanupConfig = cleanupSchedulerRuntimeConfig(job.id);

  const handleSave = async () => {
    setSaving(true);
    try {
      let payload: SchedulerSchedulePayload;
      if (type === "manual") {
        payload = { type: "manual" };
      } else if (type === "cron_daily") {
        if (hour < 0 || hour > 23 || minute < 0 || minute > 59) {
          toast({ title: "时间不合法", description: "小时 0-23 / 分钟 0-59", variant: "destructive" });
          return;
        }
        payload = { type: "cron_daily", hour: Math.trunc(hour), minute: Math.trunc(minute) };
      } else {
        const multiplier = INTERVAL_UNITS.find((u) => u.value === intervalUnit)!.multiplier;
        const seconds = Math.trunc(intervalValue * multiplier);
        if (seconds < 60) {
          toast({ title: "间隔过短", description: "最小 1 分钟", variant: "destructive" });
          return;
        }
        if (seconds > 7 * 86400) {
          toast({ title: "间隔过长", description: "最长 7 天", variant: "destructive" });
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
            toast({ title: "清理参数不合法", description: "天数必须在 1-3650 之间", variant: "destructive" });
            return;
          }
          runtimeParams.days = Math.trunc(days);
        }
        payload.runtime_params = runtimeParams;
      }
      const res = await api.setSchedulerJobSchedule(job.id, payload);
      if (res.success) {
        toast({ title: "已更新", description: cleanupConfig ? "触发器与清理阈值已保存" : describeTriggerSpec(res.data?.trigger_spec), variant: "success" });
        onOpenChange(false);
        await onSaved();
      } else {
        toast({ title: "更新失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "更新失败", description: err.message || "网络异常", variant: "destructive" });
    } finally {
      setSaving(false);
    }
  };

  const handleReset = async () => {
    setResetting(true);
    try {
      const res = await api.resetSchedulerJobSchedule(job.id);
      if (res.success) {
        toast({ title: "已恢复默认", description: describeTriggerSpec(res.data?.trigger_spec), variant: "success" });
        onOpenChange(false);
        await onSaved();
      } else {
        toast({ title: "恢复失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "恢复失败", description: err.message || "网络异常", variant: "destructive" });
    } finally {
      setResetting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>编辑触发器 · {job.name}</DialogTitle>
          <DialogDescription>
            当前：{describeTriggerSpec(job.trigger_spec)}
            {job.is_custom ? " · 已自定义" : ` · 默认（${describeTriggerSpec(job.default_trigger_spec)}）`}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label>触发模式</Label>
            <Select value={type} onValueChange={(v) => setType(v as SchedulerTriggerSpec["type"])}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="manual">关闭自动执行，仅手动</SelectItem>
                <SelectItem value="cron_daily">每日固定时间</SelectItem>
                <SelectItem value="interval">固定间隔</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {type === "manual" ? (
            <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-sm text-amber-700 dark:text-amber-300">
              保存后该任务不会被调度器自动执行，管理员仍可点击“立即运行”手动触发。
            </div>
          ) : type === "cron_daily" ? (
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2">
                <Label>小时 (0-23)</Label>
                <Input
                  type="number"
                  min={0}
                  max={23}
                  value={hour}
                  onChange={(e) => setHour(Number(e.target.value) || 0)}
                />
              </div>
              <div className="space-y-2">
                <Label>分钟 (0-59)</Label>
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
                <Label>每</Label>
                <Input
                  type="number"
                  min={1}
                  value={intervalValue}
                  onChange={(e) => setIntervalValue(Number(e.target.value) || 1)}
                />
              </div>
              <div className="space-y-2">
                <Label>单位</Label>
                <Select value={intervalUnit} onValueChange={(v) => setIntervalUnit(v as IntervalUnit)}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {INTERVAL_UNITS.map((u) => (
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
            修改后立即生效并落库，重启进程后仍保留。选择“关闭自动执行”可完全停止该任务的自动调度；可点击「恢复默认」清除覆盖。
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
                    ? "检测到用户回群时自动解禁"
                    : "启用自动清理"}
                  <span className="block text-xs text-muted-foreground">
                    {"hasAutoEnableRejoined" in cleanupConfig && cleanupConfig.hasAutoEnableRejoined
                      ? "开启后，巡检发现用户已回到群里且账号未过期时，自动恢复其系统账号和 Emby 访问权限。关闭时仅标记为待审核，需管理员手动启用。"
                      : "该开关保存到调度器数据库参数；关闭后定时自动执行会跳过，手动运行仍可临时强制执行。"}
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
            title={job.is_custom ? "清除自定义，恢复 config.toml 默认值" : "当前已是默认值"}
          >
            {resetting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RotateCcw className="mr-2 h-4 w-4" />}
            恢复默认
          </Button>
          <div className="flex gap-2 sm:justify-end">
            <Button variant="outline" onClick={() => onOpenChange(false)}>取消</Button>
            <Button onClick={handleSave} disabled={saving}>
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              保存
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function StatusBadge({ job, isRunning }: { job: SchedulerJobItem; isRunning?: boolean }) {
  if (isRunning ?? job.is_running) {
    return (
      <Badge variant="outline" className="text-[10px] border-sky-500/40 text-sky-600 dark:text-sky-400">
        <Loader2 className="mr-1 h-3 w-3 animate-spin" />
        运行中
      </Badge>
    );
  }
  if (!job.last_run) {
    return (
      <Badge variant="outline" className="text-[10px] text-muted-foreground">
        未运行
      </Badge>
    );
  }
  if (job.last_run.status === "success") {
    return (
      <Badge variant="success" className="text-[10px]">
        <CheckCircle2 className="mr-1 h-3 w-3" />
        上次成功
      </Badge>
    );
  }
  return (
    <Badge variant="destructive" className="text-[10px]">
      <XCircle className="mr-1 h-3 w-3" />
      上次失败
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
const PARAMETERIZED_JOBS = new Set(["cleanup_no_emby", "cleanup_pending_emby_entitlements", "kick_unknown_group_members"]);

export default function AdminSchedulerPage() {
  const { toast } = useToast();
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
            title: `已触发：${job.name}`,
            description: "任务在后台执行，可在卡片中查看状态",
            variant: "success",
          });
          await refresh();
        } else {
          toast({ title: "触发失败", description: res.message, variant: "destructive" });
        }
      } catch (err: any) {
        toast({ title: "触发失败", description: err.message || "网络异常", variant: "destructive" });
      } finally {
        setRunning((p) => ({ ...p, [job.id]: false }));
      }
    },
    [refresh, toast],
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
          toast({ title: "已请求终止", description: `${job.name} 正在停止，请稍后刷新状态。`, variant: "success" });
          await refresh();
        } else {
          toast({ title: "终止失败", description: res.message, variant: "destructive" });
        }
      } catch (err: any) {
        toast({ title: "终止失败", description: err.message || "网络异常", variant: "destructive" });
      } finally {
        setTerminating((p) => ({ ...p, [job.id]: false }));
      }
    },
    [refresh, toast],
  );

  const handleParamConfirm = async () => {
    if (!paramJob) return;
    let params: Record<string, unknown> = {};
    const cleanupConfig = cleanupSchedulerRuntimeConfig(paramJob.id);
    if (cleanupConfig) {
      params = {
        ignore_enabled_flag: paramIgnoreEnabled,
      };
      if (cleanupConfig.hasDays) {
        const days = Number(paramDays);
        if (!Number.isFinite(days) || days < 1) {
          toast({ title: "天数不合法", description: "必须 >= 1 天", variant: "destructive" });
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
          title: `已启用 ${res.data.enabled} 个回群用户`,
          description: failed ? `失败 ${failed} 个，请查看后端日志` : "已重新校验 Telegram 群成员状态",
          variant: failed ? "default" : "success",
        });
        await refresh();
      } else {
        toast({ title: "启用失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "启用失败", description: err.message || "网络异常", variant: "destructive" });
    } finally {
      setRejoinEnabling(false);
    }
  }, [refresh, toast]);

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
      toast({ title: "加载日志失败", description: err.message || "网络异常", variant: "destructive" });
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
                <h1 className="text-2xl font-bold tracking-tight sm:text-3xl">定时任务</h1>
                <p className="max-w-3xl text-sm leading-relaxed text-muted-foreground">
                  管理后台巡检、清理和同步任务。任务异步执行，运行中会自动轮询；手动触发不会阻塞当前页面。
                </p>
              </div>
            </div>
            <div className="flex flex-col gap-2 sm:flex-row lg:items-center">
              <Badge variant={anyRunning ? "outline" : "secondary"} className="justify-center py-2 text-xs">
                {anyRunning ? "正在轮询运行状态" : "当前无运行任务"}
              </Badge>
              <Button variant="outline" onClick={() => void refresh()} disabled={isLoading} className="bg-background/70 sm:w-auto">
                {isLoading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
                刷新任务
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          icon={Activity}
          label="运行中"
          value={schedulerStats.runningCount}
          hint={schedulerStats.runningCount > 0 ? "页面每 2 秒刷新状态" : "无后台任务占用中"}
          tone="sky"
        />
        <MetricCard
          icon={CalendarClock}
          label="自动任务"
          value={schedulerStats.timedCount}
          hint={`手动任务 ${schedulerStats.manualCount} 个`}
        />
        <MetricCard
          icon={AlertTriangle}
          label="上次失败"
          value={schedulerStats.failedCount}
          hint={schedulerStats.failedCount > 0 ? "建议查看运行日志" : "最近状态正常"}
          tone={schedulerStats.failedCount > 0 ? "destructive" : "default"}
        />
        <MetricCard
          icon={Clock3}
          label="下次执行"
          value={schedulerStats.nextJob ? formatTimestamp(schedulerStats.nextJob.next_run_at) : "—"}
          hint={schedulerStats.nextJob ? schedulerStats.nextJob.name : "暂无自动计划"}
          tone="amber"
        />
      </div>

      {jobs.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            {isLoading ? "加载中..." : "没有可用的定时任务"}
          </CardContent>
        </Card>
      ) : (
        <>
          {!schedulerHasTimedJobs && (
            <Card className="border-amber-500/30 bg-amber-500/5">
              <CardContent className="flex gap-3 py-4 text-sm text-amber-700 dark:text-amber-300">
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                <span>
                  当前未发现已注册的自动定时任务。请确认调度器进程已启动，否则任务只会在手动点击“立即运行”时执行。
                </span>
              </CardContent>
            </Card>
          )}
          <div className="flex flex-col gap-3 rounded-2xl border border-border/80 bg-card/70 p-3 backdrop-blur-sm sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0 px-1">
              <p className="text-sm font-medium">任务列表</p>
              <p className="text-xs text-muted-foreground">按运行状态快速筛选；操作按钮位于每个任务卡片底部。</p>
            </div>
            <Tabs value={jobView} onValueChange={(value) => setJobView(value as JobView)}>
              <TabsList className="grid h-auto w-full grid-cols-2 gap-1 sm:inline-flex sm:w-auto sm:grid-cols-none">
                <TabsTrigger value="all" className="gap-1.5 px-3">
                  全部
                  <span className="rounded-full bg-muted px-1.5 text-[10px] text-muted-foreground">{jobs.length}</span>
                </TabsTrigger>
                <TabsTrigger value="running" className="gap-1.5 px-3">
                  运行中
                  <span className="rounded-full bg-muted px-1.5 text-[10px] text-muted-foreground">{schedulerStats.runningCount}</span>
                </TabsTrigger>
                <TabsTrigger value="failed" className="gap-1.5 px-3">
                  失败
                  <span className="rounded-full bg-muted px-1.5 text-[10px] text-muted-foreground">{schedulerStats.failedCount}</span>
                </TabsTrigger>
                <TabsTrigger value="custom" className="gap-1.5 px-3">
                  自定义
                  <span className="rounded-full bg-muted px-1.5 text-[10px] text-muted-foreground">{schedulerStats.customCount}</span>
                </TabsTrigger>
                <TabsTrigger value="manual" className="gap-1.5 px-3">
                  手动
                  <span className="rounded-full bg-muted px-1.5 text-[10px] text-muted-foreground">{schedulerStats.manualCount}</span>
                </TabsTrigger>
              </TabsList>
            </Tabs>
          </div>

          {filteredJobs.length === 0 ? (
            <Card className="border-dashed">
              <CardContent className="py-10 text-center text-sm text-muted-foreground">
                当前筛选下没有任务
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
                        <StatusBadge job={job} isRunning={isRunning} />
                        {job.manual_only && (
                          <Badge variant="outline" className="border-amber-500/40 text-[10px] text-amber-600 dark:text-amber-400">
                            仅手动
                          </Badge>
                        )}
                        {!job.manual_only && job.is_custom && (
                          <Badge variant="outline" className="text-[10px]">自定义触发器</Badge>
                        )}
                        {job.auto_disabled && (
                          <Badge variant="outline" className="border-muted-foreground/30 text-[10px] text-muted-foreground">
                            自动已停用
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
                          <span className="truncate">触发：{describeTriggerSpec(job.trigger_spec)}</span>
                        </div>
                        <div className="flex min-w-0 items-center gap-2">
                          <TimerReset className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                          <span className="truncate">下次：{job.manual_only ? "仅手动" : formatTimestamp(job.next_run_at)}</span>
                        </div>
                        <div className="flex min-w-0 items-center gap-2">
                          <Clock3 className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                          <span className="truncate">自动：{formatTimestamp(job.last_auto_run_at)}</span>
                        </div>
                        <div className="flex min-w-0 items-center gap-2">
                          <Activity className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                          <span className="truncate">手动：{formatTimestamp(job.last_manual_run_at)}</span>
                        </div>
                      </div>

                      <div className="rounded-2xl border border-border/60 bg-background/55 p-3 text-xs">
                        <div className="flex flex-wrap items-center justify-between gap-2">
                          <p className="font-medium">最近一次运行</p>
                          {lr ? (
                            <Badge variant={lr.status === "success" ? "success" : lr.status === "failed" ? "destructive" : "outline"} className="text-[10px]">
                              {lr.status}
                            </Badge>
                          ) : (
                            <Badge variant="outline" className="text-[10px] text-muted-foreground">暂无</Badge>
                          )}
                        </div>
                        {lr ? (
                          <div className="mt-3 space-y-1">
                            <p><span className="text-muted-foreground">开始：</span>{formatTimestamp(lr.started_at)}</p>
                            <p><span className="text-muted-foreground">结束：</span>{formatTimestamp(lr.finished_at)}</p>
                            <p><span className="text-muted-foreground">耗时：</span>{formatDuration(lr.started_at, lr.finished_at)}</p>
                            <p><span className="text-muted-foreground">类型：</span>{formatRunType(lr)}{lr.trigger === "startup" ? "（启动时）" : ""}</p>
                            {lr.error && <p className="break-words text-destructive">错误：{lr.error}</p>}
                            {renderSummaryChips(lr.summary)}
                          </div>
                        ) : (
                          <p className="mt-3 text-muted-foreground">该任务还没有运行记录。</p>
                        )}
                      </div>

                      {job.id === "enforce_group_membership" && rejoinCandidates > 0 && (
                        <div className="space-y-2 rounded-2xl border border-amber-500/30 bg-amber-500/5 p-3 text-xs">
                          <p className="text-amber-700 dark:text-amber-300">
                            检测到 {rejoinCandidates} 个已禁用但重新入群用户，可重新校验后批量启用。
                          </p>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => void handleEnableRejoinedUsers()}
                            disabled={rejoinEnabling || isRunning}
                            className="h-8 border-amber-500/40 text-amber-700 hover:text-amber-800 dark:text-amber-300 dark:hover:text-amber-200"
                          >
                            {rejoinEnabling ? <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" /> : <CheckCircle2 className="mr-2 h-3.5 w-3.5" />}
                            一键启用回群用户
                          </Button>
                        </div>
                      )}

                      <div className="mt-auto flex flex-col gap-2 sm:flex-row">
                        <Button onClick={() => void handleTrigger(job)} disabled={isRunning} className="sm:flex-1">
                          {isRunning ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <PlayCircle className="mr-2 h-4 w-4" />}
                          {isRunning ? "运行中" : "立即运行"}
                        </Button>
                        <div className={`grid gap-2 ${job.manual_only ? "grid-cols-2" : "grid-cols-3"} sm:flex`}>
                          <Button
                            variant="destructive"
                            size="sm"
                            onClick={() => void handleTerminate(job)}
                            disabled={!isRunning || Boolean(terminating[job.id])}
                          >
                            {terminating[job.id] ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Square className="mr-2 h-4 w-4" />}
                            终止
                          </Button>
                          {!job.manual_only && (
                            <Button variant="outline" size="sm" onClick={() => setScheduleJob(job)}>
                              <Settings2 className="mr-2 h-4 w-4" />
                              编辑
                            </Button>
                          )}
                          <Button variant="outline" size="sm" onClick={() => void openLogs(job)}>
                            <FileText className="mr-2 h-4 w-4" />
                            日志
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
            <DialogTitle>手动触发 · {paramJob?.name}</DialogTitle>
            <DialogDescription>
              本次仅作用于这一次手动触发，不会修改 config.toml 默认值。
            </DialogDescription>
          </DialogHeader>

          {paramJob && cleanupSchedulerRuntimeConfig(paramJob.id) && (
            <div className="space-y-4">
              {cleanupSchedulerRuntimeConfig(paramJob.id)?.hasDays ? (
                <div className="space-y-2">
                  <Label>{cleanupSchedulerRuntimeConfig(paramJob.id)?.title}</Label>
                  <Input
                    type="number"
                    min={1}
                    value={paramDays}
                    onChange={(e) => setParamDays(e.target.value)}
                  />
                  <p className="text-xs text-muted-foreground">
                    {cleanupSchedulerRuntimeConfig(paramJob.id)?.description}
                  </p>
                </div>
              ) : (
                <div className="space-y-2">
                  <Label>{cleanupSchedulerRuntimeConfig(paramJob.id)?.title}</Label>
                  <p className="text-xs text-muted-foreground">
                    {cleanupSchedulerRuntimeConfig(paramJob.id)?.description}
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
                    保留已绑定 Telegram 的账号
                    <span className="block text-xs text-muted-foreground">
                      勾选后已绑定 Telegram 但没有 Emby 的 Web 账号会被保留。
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
                  忽略自动清理总开关
                  <span className="block text-xs text-muted-foreground">
                    勾选后即便 config 里没启用自动清理，也能手动跑一次。
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
                  仅试运行 (dry_run)
                  <span className="block text-xs text-muted-foreground">
                    勾选后只统计待踢的目标，不会真正踢人。结果在 summary 中查看。
                  </span>
                </span>
              </label>
              <div className="space-y-2">
                <Label>单次最多踢出</Label>
                <Input
                  type="number"
                  min={1}
                  max={500}
                  value={paramKickMaxPerRun}
                  onChange={(e) => setParamKickMaxPerRun(e.target.value)}
                />
                <p className="text-xs text-muted-foreground">
                  避免触发 Telegram 限流；默认 200，上限 500。
                </p>
              </div>
              <p className="rounded-md border border-amber-500/40 bg-amber-500/5 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
                ⚠️ 按 Sakura_EmbyBoss 思路：以群花名册为基准反查 users 表，踢 无系统账号 / 已禁用 / 未绑 Emby 的成员。
              </p>
            </div>
          )}

          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={() => setParamJob(null)}>取消</Button>
            <Button onClick={handleParamConfirm}>立即执行</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(logsJob)} onOpenChange={(open) => { if (!open) { setLogsJob(null); setLogsDetail(null); setLogsHistory([]); } }}>
        <DialogContent className="max-h-[85vh] w-[92vw] max-w-3xl overflow-hidden p-0 sm:max-w-3xl">
          <DialogHeader className="border-b p-4">
            <DialogTitle>{logsJob?.name} · 运行日志</DialogTitle>
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
              <p className="text-center text-sm text-muted-foreground">暂无运行记录</p>
            ) : (
              <>
                <div className="rounded-md border border-border/60 bg-muted/30 p-3 text-xs space-y-1">
                  <p><span className="text-muted-foreground">状态：</span>{logsDetail.status}</p>
                  <p><span className="text-muted-foreground">开始：</span>{formatTimestamp(logsDetail.started_at)}</p>
                  <p><span className="text-muted-foreground">结束：</span>{formatTimestamp(logsDetail.finished_at)}</p>
                  <p><span className="text-muted-foreground">耗时：</span>{formatDuration(logsDetail.started_at, logsDetail.finished_at)}</p>
                  <p><span className="text-muted-foreground">类型：</span>{formatRunType(logsDetail)}</p>
                  {logsDetail.error && (
                    <p className="break-words text-destructive">错误：{logsDetail.error}</p>
                  )}
                  {renderSummaryChips(logsDetail.summary)}
                </div>

                {logsDetail.logs && logsDetail.logs.length > 0 ? (
                  <div>
                    <p className="mb-1 text-xs font-medium text-muted-foreground">最近一次日志</p>
                    <pre className="max-h-72 overflow-auto rounded-md border border-border/60 bg-background p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-words">
                      {logsDetail.logs.join("\n")}
                    </pre>
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground">本次未产生日志输出</p>
                )}

                {logsHistory.length > 0 && (
                  <div>
                    <p className="mb-2 text-xs font-medium text-muted-foreground">历史运行（最近 {logsHistory.length} 次）</p>
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
                              <span className="text-muted-foreground">[{formatRunType(run)}]</span>
                              <span className="text-muted-foreground">日志 {run.logs?.length || 0} 行</span>
                            </span>
                          </summary>
                          <div className="mt-2 space-y-2 border-t border-border/40 pt-2">
                            {run.error && <p className="break-words text-destructive">错误：{run.error}</p>}
                            {renderSummaryChips(run.summary)}
                            {run.logs && run.logs.length > 0 ? (
                              <pre className="max-h-60 overflow-auto rounded-md border border-border/60 bg-background p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-words">
                                {run.logs.join("\n")}
                              </pre>
                            ) : (
                              <p className="text-muted-foreground">本次未产生日志输出</p>
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
