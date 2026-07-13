"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { motion } from "framer-motion";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  Calendar,
  Clock,
  Eye,
  EyeOff,
  Gift,
  Key,
  Loader2,
  MessageCircle,
  Play,
  Tv,
  Activity,
  Film,
  RefreshCw,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Hourglass,
  Download,
  Globe,
  Coins,
  Flame,
  Sparkles,
  UserPlus,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useToast } from "@/hooks/use-toast";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { useVisiblePolling } from "@/hooks/use-visible-polling";
import { PageError } from "@/components/layout/page-state";
import { useAuthStore } from "@/store/auth";
import { useSystemStore } from "@/store/system";
import { api, type CodeUsePreview, type EmbyInfo, type MediaRequest, type TelegramStatus, type SigninSummary, type RegisterAvailability, type EmbyRegisterStatus } from "@/lib/api";
import { AnnouncementBoard } from "@/components/announcement-board";
import { useI18n } from "@/lib/i18n";
import { validatePasswordStrength } from "@/lib/password";

const container = {
  hidden: { opacity: 0 },
  show: {
    opacity: 1,
    transition: {
      staggerChildren: 0.08,
    },
  },
};

const item = {
  hidden: { opacity: 0, y: 16 },
  show: { opacity: 1, y: 0 },
};

type LineLatencyStatus = "idle" | "testing" | "ok" | "timeout" | "error";
interface LineLatencyInfo {
  status: LineLatencyStatus;
  latencyMs?: number;
}
interface LineSlot {
  key: string;
  name: string;
  url: string;
  scope: "line" | "wl";
}

interface StoredEmbyRegisterRequest {
  requestId: string;
  statusToken: string;
  savedAt: number;
}

type CodeCheckInfo = CodeUsePreview;

export default function DashboardPage() {
  const router = useRouter();
  const { user, fetchUser } = useAuthStore();
  const { info: systemInfo, fetchInfo: fetchSystemInfo, invalidate: invalidateSystemStore } = useSystemStore();
  const { toast } = useToast();
  const { t } = useI18n();
  const [regCode, setRegCode] = useState("");
  const [regCodeInfo, setRegCodeInfo] = useState<CodeCheckInfo | null>(null);
  const [showConfirm, setShowConfirm] = useState(false);
  const [isUsingCode, setIsUsingCode] = useState(false);
  const [showDeveloperDialog, setShowDeveloperDialog] = useState(false);
  const [developerPassword, setDeveloperPassword] = useState("");
  const [activatingDeveloper, setActivatingDeveloper] = useState(false);
  const [embyUsername, setEmbyUsername] = useState("");
  const [registerAvailability, setRegisterAvailability] = useState<RegisterAvailability | null>(null);
  const [showDirectRegisterDialog, setShowDirectRegisterDialog] = useState(false);
  const [isSubmittingDirectRegister, setIsSubmittingDirectRegister] = useState(false);
  const [directEmbyUsername, setDirectEmbyUsername] = useState("");
  const [directEmbyPassword, setDirectEmbyPassword] = useState("");
  const [showDirectEmbyPassword, setShowDirectEmbyPassword] = useState(false);
  const [embyRegisterStatus, setEmbyRegisterStatus] = useState<EmbyRegisterStatus | null>(null);
  const [embyRegisterStored, setEmbyRegisterStored] = useState<StoredEmbyRegisterRequest | null>(null);
  // 自由注册天数由管理员单值固定，前端不再让用户挑套餐或自定义

  const [telegramStatus, setTelegramStatus] = useState<TelegramStatus | null>(null);
  const [embyInfo, setEmbyInfo] = useState<EmbyInfo | null>(null);
  const [embyStats, setEmbyStats] = useState<any>(null);
  const [embyViewers, setEmbyViewers] = useState(0);
  const [embyStatsRefreshing, setEmbyStatsRefreshing] = useState(false);
  const [myRequests, setMyRequests] = useState<MediaRequest[]>([]);
  const [lineSlots, setLineSlots] = useState<LineSlot[]>([]);
  const [linesRequireEmby, setLinesRequireEmby] = useState(false);
  const [linesRequireRenewal, setLinesRequireRenewal] = useState(false);
  const [lineLatencyMap, setLineLatencyMap] = useState<Record<string, LineLatencyInfo>>({});
  const [isLatencyTesting, setIsLatencyTesting] = useState(false);
  const [signinSummary, setSigninSummary] = useState<SigninSummary | null>(null);
  const [signingIn, setSigningIn] = useState(false);
  const [renewingWithPoints, setRenewingWithPoints] = useState(false);
  const mediaRequestEnabled = systemInfo?.features?.media_request !== false;

  const embyRegisterStorageKey = user?.uid ? `twilight:emby-register:${user.uid}` : null;

  const saveEmbyRegisterRequest = useCallback((requestId: string, statusToken: string) => {
    if (!embyRegisterStorageKey) return;
    const item = { requestId, statusToken, savedAt: Date.now() };
    localStorage.setItem(embyRegisterStorageKey, JSON.stringify(item));
    setEmbyRegisterStored(item);
  }, [embyRegisterStorageKey]);

  const clearEmbyRegisterRequest = useCallback(() => {
    if (embyRegisterStorageKey) localStorage.removeItem(embyRegisterStorageKey);
    setEmbyRegisterStored(null);
  }, [embyRegisterStorageKey]);

  useEffect(() => {
    void fetchSystemInfo();
  }, [fetchSystemInfo]);

  const applyEmbyUrls = useCallback((data: {
    lines?: Array<{ name: string; url: string }>;
    whitelist_lines?: Array<{ name: string; url: string }>;
    requires_emby_account?: boolean;
    requires_renewal?: boolean;
    emby_disabled_by_expiry?: boolean;
  }) => {
    const lines = (data.lines || []).map((line, index) => ({
      key: `line:${index}:${line.url}`,
      name: line.name || t("settings.lineName", { index: index + 1 }),
      url: line.url,
      scope: "line" as const,
    }));
    const wl = (data.whitelist_lines || []).map((line, index) => ({
      key: `wl:${index}:${line.url}`,
      name: line.name || t("settings.dedicatedLineName", { index: index + 1 }),
      url: line.url,
      scope: "wl" as const,
    }));
    setLineSlots([...lines, ...wl]);
    setLinesRequireEmby(Boolean(data.requires_emby_account));
    setLinesRequireRenewal(Boolean(data.requires_renewal || data.emby_disabled_by_expiry));
  }, [t]);

  const loadEmbyUrls = useCallback(async () => {
    const res = await api.getEmbyUrls();
    if (res.success && res.data) {
      applyEmbyUrls(res.data);
    }
  }, [applyEmbyUrls]);

  useEffect(() => {
    if (!embyRegisterStorageKey) return;
    try {
      const raw = localStorage.getItem(embyRegisterStorageKey);
      if (!raw) return;
      const parsed = JSON.parse(raw) as StoredEmbyRegisterRequest;
      if (!parsed.requestId || !parsed.statusToken) return;
      if (Date.now() - Number(parsed.savedAt || 0) > 15 * 60 * 1000) {
        localStorage.removeItem(embyRegisterStorageKey);
        return;
      }
      setEmbyRegisterStored(parsed);
    } catch {
      localStorage.removeItem(embyRegisterStorageKey);
    }
  }, [embyRegisterStorageKey]);

  const refreshStoredEmbyRegisterStatus = useCallback(async () => {
    if (!embyRegisterStored) return;
    try {
      const res = await api.getEmbyRegisterStatus(embyRegisterStored.requestId, embyRegisterStored.statusToken);
      if (res.success && res.data) {
        setEmbyRegisterStatus(res.data);
        if (res.data.status === "success") {
          await fetchUser();
          await loadEmbyUrls();
          clearEmbyRegisterRequest();
        }
      } else if (!res.success) {
        clearEmbyRegisterRequest();
      }
    } catch {
      // Keep the local request for 15 minutes; transient errors should not hide status.
    }
  }, [clearEmbyRegisterRequest, embyRegisterStored, fetchUser, loadEmbyUrls]);

  useEffect(() => {
    if (embyRegisterStored) void refreshStoredEmbyRegisterStatus();
  }, [embyRegisterStored, refreshStoredEmbyRegisterStatus]);
  useVisiblePolling(refreshStoredEmbyRegisterStatus, 5000, Boolean(embyRegisterStored));

  const loadDashboardData = useCallback(async (signal?: AbortSignal) => {
    // 之前每个子接口都包了 .catch(() => null) + Promise.all：任何失败都会被
    // 静默丢弃，dashboard 显示一切正常但数据缺一块（典型表现是 emby 卡片
    // 空白、签到状态错位）。改用 allSettled + 统计失败数量：
    //   - 网络偶发失败：toast 提示并允许用户重试，不像之前那样静默；
    //   - signal aborted：是 useAsyncResource 卸载触发的正常路径，跳过提示;
    //   - dev 环境额外打 console.warn 方便定位是哪一路接口挂了。
    // 注意：必须按位置展开调用，不能 tasks.map(t => t.run())，否则 TS 会把
    // 各路返回类型联合起来，导致 setState 的入参类型推断丢精度。
    //
    // 求片功能关闭时 /media/request/my 会返回 403 (MEDIA_REQUEST_DISABLED)，
    // 之前没 gating 直接落到 reqSettled.rejected 触发"my_requests 加载失败"
    // toast。先确保 systemInfo 加载完成再决定要不要发这一路请求。
    const infoState = useSystemStore.getState();
    let mediaEnabled = infoState.info?.features?.media_request !== false;
    if (!infoState.loaded) {
      const res = await infoState.fetchInfo();
      if (res.success) {
        mediaEnabled = useSystemStore.getState().info?.features?.media_request !== false;
      }
    }
    const myRequestsPromise: Promise<{ success: boolean; data?: MediaRequest[] } | null> = mediaEnabled
      ? api.getMyRequests(signal)
      : Promise.resolve(null);
    const [tgSettled, embySettled, urlsSettled, reqSettled, signinSettled, registerSettled] = await Promise.allSettled([
      api.getTelegramStatus(),
      api.getEmbyInfo(),
      api.getEmbyUrls(),
      myRequestsPromise,
      api.getSigninSummary(),
      api.getRegisterAvailability(),
    ]);

    const failed: string[] = [];
    const inspect = (name: string, settled: PromiseSettledResult<unknown>) => {
      if (settled.status !== "rejected") return;
      const reason = settled.reason as { name?: string } | undefined;
      // AbortError 是组件卸载 / 路由切换时正常的取消信号，不算失败。
      if (reason?.name === "AbortError") return;
      failed.push(name);
      if (process.env.NODE_ENV !== "production") {
        // eslint-disable-next-line no-console
        console.warn(`[dashboard] sub-resource ${name} failed`, settled.reason);
      }
    };
    inspect("telegram_status", tgSettled);
    inspect("emby_info", embySettled);
    inspect("emby_urls", urlsSettled);
    if (mediaEnabled) inspect("my_requests", reqSettled);
    inspect("signin_summary", signinSettled);
    inspect("register_availability", registerSettled);

    if (signinSettled.status === "fulfilled" && signinSettled.value.success && signinSettled.value.data) {
      setSigninSummary(signinSettled.value.data);
    }
    if (tgSettled.status === "fulfilled" && tgSettled.value.success && tgSettled.value.data) {
      setTelegramStatus(tgSettled.value.data);
    }
    if (embySettled.status === "fulfilled" && embySettled.value.success && embySettled.value.data) {
      setEmbyInfo(embySettled.value.data);
      setEmbyViewers(embySettled.value.data.active_sessions ?? 0);
      const ed = embySettled.value.data;
      if (ed.movie_count !== undefined || ed.series_count !== undefined || ed.episode_count !== undefined) {
        setEmbyStats({ enabled: true, configured: true, movie_count: ed.movie_count ?? 0, series_count: ed.series_count ?? 0, episode_count: ed.episode_count ?? 0 });
      }
    }
    if (urlsSettled.status === "fulfilled" && urlsSettled.value.success && urlsSettled.value.data) {
      applyEmbyUrls(urlsSettled.value.data);
    }
    if (mediaEnabled && reqSettled.status === "fulfilled" && reqSettled.value && reqSettled.value.success && Array.isArray(reqSettled.value.data)) {
      setMyRequests(reqSettled.value.data);
    } else if (!mediaEnabled) {
      setMyRequests([]);
    }
    if (registerSettled.status === "fulfilled" && registerSettled.value.success && registerSettled.value.data) {
      setRegisterAvailability(registerSettled.value.data);
    }

    if (failed.length > 0 && !signal?.aborted) {
      toast({
        title: t("dashboard.partialLoadFailed"),
        description: t("dashboard.partialLoadFailedDescription", { modules: failed.join(", ") }),
        variant: "destructive",
      });
    }
    return true;
  }, [applyEmbyUrls, t, toast]);

  const {
    isLoading,
    error,
  } = useAsyncResource(loadDashboardData, { immediate: true });

  const loadEmbyLibraryStats = useCallback(async (notify = false, signal?: AbortSignal) => {
    setEmbyStatsRefreshing(true);
    try {
      const result = await api.getEmbyLibraryStats(signal);
      if (signal?.aborted) return;
      if (result.success && result.data) {
        setEmbyStats(result.data);
        if (notify) toast({ title: t("adminStatus.updated"), variant: "success" });
      } else if (notify) {
        toast({ title: t("dashboard.libraryStatsUnavailable"), description: result.message, variant: "destructive" });
      }
    } finally {
      if (!signal?.aborted) setEmbyStatsRefreshing(false);
    }
  }, [t, toast]);

  const loadEmbyViewers = useCallback(async (signal?: AbortSignal) => {
    const result = await api.getEmbyViewerCount(signal);
    if (signal?.aborted) return;
    if (result.success) setEmbyViewers(result.data?.viewers ?? 0);
  }, []);

  // 独立拉取 Emby 统计（功能开关控制，避免污染主数据加载流程）
  useEffect(() => {
    if (!systemInfo?.features?.emby_stats) {
      setEmbyStats(null);
      setEmbyViewers(0);
      return;
    }
    if (isLoading || embyStats) return;
    const controller = new AbortController();
    void loadEmbyLibraryStats(false, controller.signal);
    return () => controller.abort();
  }, [embyStats, isLoading, loadEmbyLibraryStats, systemInfo?.features?.emby_stats]);

  const embyStatsEnabled = systemInfo?.features?.emby_stats === true;
  useVisiblePolling(loadEmbyViewers, 60000, embyStatsEnabled);

  // ============== 线路延迟测试 ==============
  // 由后端 /system/emby-urls/probe 代发请求测速。前端直连 Emby 会被 CORS /
  // 私网混合内容 / 浏览器 CORP 拦截，换成后端代理后这些问题一次解决；URL 在
  // 后端会做白名单校验，不接受任意 URL，避免 SSRF。
  const testSingleLineLatency = useCallback(async (rawUrl: string): Promise<LineLatencyInfo> => {
    const url = rawUrl.trim();
    if (!url) {
      return { status: "error" };
    }
    try {
      const res = await api.probeEmbyUrl(url);
      if (res.success && res.data) {
        if (res.data.status === "ok") {
          return { status: "ok", latencyMs: Math.max(1, Math.round(res.data.latency_ms || 0)) };
        }
        return { status: res.data.status };
      }
      return { status: "error" };
    } catch {
      return { status: "error" };
    }
  }, []);

  const runLineLatencyTests = useCallback(async () => {
    if (lineSlots.length === 0) {
      setLineLatencyMap({});
      return;
    }
    setIsLatencyTesting(true);
    // 每次测速前先检测 Emby 在线状态
    let embyOnline = false;
    try {
      const embyRes = await api.getEmbyInfo();
      embyOnline = embyRes.success && !!embyRes.data?.online;
    } catch {
      // 状态接口异常，视为不可达
    }
    if (!embyOnline) {
      const offlineMap: Record<string, LineLatencyInfo> = {};
      for (const slot of lineSlots) offlineMap[slot.key] = { status: "error" };
      setLineLatencyMap(offlineMap);
      setIsLatencyTesting(false);
      return;
    }
    setLineLatencyMap((prev) => {
      const next = { ...prev };
      for (const slot of lineSlots) next[slot.key] = { status: "testing" };
      return next;
    });
    const entries = await Promise.all(
      lineSlots.map(async (slot) => [slot.key, await testSingleLineLatency(slot.url)] as const)
    );
    setLineLatencyMap((prev) => {
      const next = { ...prev };
      for (const [k, v] of entries) next[k] = v;
      return next;
    });
    setIsLatencyTesting(false);
  }, [lineSlots, testSingleLineLatency]);

  useEffect(() => {
    if (lineSlots.length > 0) void runLineLatencyTests();
  }, [lineSlots, runLineLatencyTests]);

  useEffect(() => {
    setDirectEmbyUsername(user?.username || "");
    setDirectEmbyPassword("");
    setShowDirectEmbyPassword(false);
  }, [user?.uid, user?.username]);

  const renderLatencyText = (key: string) => {
    const info = lineLatencyMap[key];
    if (!info || info.status === "idle") return t("settings.latencyIdle");
    if (info.status === "testing") return t("settings.latencyTesting");
    if (info.status === "ok") return `${info.latencyMs} ms`;
    if (info.status === "timeout") return t("settings.latencyTimeout");
    return t("settings.latencyUnreachable");
  };

  const renderLatencyToneClass = (key: string) => {
    const info = lineLatencyMap[key];
    if (!info || info.status === "idle" || info.status === "testing") {
      return "bg-muted text-muted-foreground";
    }
    if (info.status === "ok") {
      if ((info.latencyMs ?? 0) <= 150) return "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400";
      if ((info.latencyMs ?? 0) <= 400) return "bg-amber-500/10 text-amber-600 dark:text-amber-400";
      return "bg-orange-500/10 text-orange-600 dark:text-orange-400";
    }
    return "bg-destructive/10 text-destructive";
  };

  const getLatencyRank = useCallback((key: string) => {
    const info = lineLatencyMap[key];
    if (!info) return 8_000_000;
    if (info.status === "ok") return info.latencyMs ?? 8_000_000;
    if (info.status === "testing") return 9_000_000;
    if (info.status === "timeout") return 10_000_000;
    if (info.status === "error") return 11_000_000;
    return 12_000_000;
  }, [lineLatencyMap]);

  const sortedLineSlots = useMemo(
    () => [...lineSlots].sort((a, b) => getLatencyRank(a.key) - getLatencyRank(b.key)),
    [lineSlots, getLatencyRank]
  );

  // ============== 求片状态汇总 ==============
  const requestStats = useMemo(() => {
    const counts = {
      pending: 0,
      accepted: 0,
      downloading: 0,
      completed: 0,
      rejected: 0,
    };
    for (const req of myRequests) {
      const s = (req.status || "").toUpperCase();
      if (s === "UNHANDLED" || s === "PENDING") counts.pending++;
      else if (s === "ACCEPTED") counts.accepted++;
      else if (s === "DOWNLOADING") counts.downloading++;
      else if (s === "COMPLETED") counts.completed++;
      else if (s === "REJECTED") counts.rejected++;
    }
    return counts;
  }, [myRequests]);

  const latestRequests = useMemo(() => {
    const sorted = [...myRequests].sort((a, b) => (b.timestamp || 0) - (a.timestamp || 0));
    return sorted.slice(0, 3);
  }, [myRequests]);

  // ============== 用户/到期信息 ==============
  const isAdmin = user?.role === 0;
  const isPending = !user?.emby_id && !user?.active;
  const isPendingEmby = Boolean(user?.pending_emby) && !user?.emby_id;
  const isPendingEmbyFromRegcode = isPendingEmby && user?.pending_emby_days !== null && user?.pending_emby_days !== undefined;
  const hasGrantedEmbyRegisterEntitlement = isPendingEmbyFromRegcode;

  let expiredTimestamp: number | null = null;
  if (user?.expired_at !== undefined && user.expired_at !== null) {
    if (typeof user.expired_at === "number") {
      // -1 永久；0 未开通 sentinel；都不映射为真实时间戳
      if (user.expired_at !== -1 && user.expired_at !== 0) {
        expiredTimestamp = user.expired_at < 10000000000 ? user.expired_at * 1000 : user.expired_at;
      }
    } else if (typeof user.expired_at === "string" && user.expired_at !== "-1" && user.expired_at !== "0") {
      const parsed = new Date(user.expired_at).getTime();
      expiredTimestamp = Number.isNaN(parsed) ? null : parsed;
    }
  }

  // 未开通 Emby 时既不算"永久"也不算"已过期"，前端单独展示。
  const isPermanent = !isPendingEmby && (isAdmin || expiredTimestamp === null);
  const safeExpiredTimestamp = expiredTimestamp ?? 0;
  const isExpired = !isPermanent && !isPendingEmby && safeExpiredTimestamp < Date.now();
  const embyDisabledByExpiry = Boolean(user?.emby_disabled_by_expiry || isExpired);
  const daysLeft = !isPermanent && !isPendingEmby
    ? Math.max(0, Math.ceil((safeExpiredTimestamp - Date.now()) / (1000 * 60 * 60 * 24)))
    : 0;
  const signinRenewal = signinSummary?.renewal;

  const getGreeting = () => {
    const hour = new Date().getHours();
    if (hour < 6) return t("dashboard.greetingEarlyMorning");
    if (hour < 9) return t("dashboard.greetingMorning");
    if (hour < 12) return t("dashboard.greetingLateMorning");
    if (hour < 14) return t("dashboard.greetingNoon");
    if (hour < 18) return t("dashboard.greetingAfternoon");
    if (hour < 22) return t("dashboard.greetingEvening");
    return t("dashboard.greetingLateNight");
  };

  // ============== 卡码/邀请码流程 ==============
  const handleCheckRegcode = async () => {
    const code = regCode.trim();
    if (!code) {
      toast({ title: t("dashboard.codeRequired"), variant: "destructive" });
      return;
    }
    if (code.toUpperCase() === "DEBUGMODE") {
      if (!isAdmin) {
        toast({ title: t("dashboard.developerAdminOnly"), variant: "destructive" });
        return;
      }
      setDeveloperPassword("");
      setShowDeveloperDialog(true);
      return;
    }
    try {
      const res = await api.useCode(code, { checkOnly: true });
      if (res.success && res.data?.source && res.data.type_name) {
        setRegCodeInfo(res.data as CodeUsePreview);
        setEmbyUsername("");
        setShowConfirm(true);
      } else {
        toast({ title: t("dashboard.codeInvalid"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("dashboard.checkFailed"), description: err.message || t("common.networkError"), variant: "destructive" });
    }
  };

  const handleUseRegcode = async () => {
    if (!regCodeInfo || !regCode.trim()) return;
    const requiresEmbyUsername = Boolean(regCodeInfo.requires_emby_credentials);
    if (requiresEmbyUsername) {
      if (!embyUsername.trim()) {
        toast({ title: t("dashboard.embyUsernameRequired"), variant: "destructive" });
        return;
      }
    }
    setIsUsingCode(true);
    try {
      const res = await api.useCode(
        regCode.trim(),
        requiresEmbyUsername
          ? { embyUsername: embyUsername.trim() }
          : undefined
      );
      if (res.success) {
        if (res.data?.pending && res.data.request_id && res.data.status_token) {
          toast({
            title: res.data.reused ? t("dashboard.codeRequestExisting") : t("dashboard.codeRequestQueued"),
            description: res.data.queue_position ? t("dashboard.queuePosition", { position: res.data.queue_position }) : t("dashboard.autoCompleteLater"),
          });
          setShowConfirm(false);

          for (let i = 0; i < 90; i++) {
            await new Promise((resolve) => setTimeout(resolve, 2000));
            const statusRes = await api.getUseCodeStatus(res.data.request_id, res.data.status_token);
            if (!statusRes.success || !statusRes.data) continue;
            if (statusRes.data.status === "success") {
              toast({ title: t("dashboard.codeUseSuccess"), description: statusRes.data.message || regCodeInfo.type_name, variant: "success" });
              setRegCode("");
              setRegCodeInfo(null);
              setEmbyUsername("");
              await fetchUser();
              await loadEmbyUrls();
              return;
            }
            if (statusRes.data.status === "failed") {
              toast({ title: t("dashboard.useFailed"), description: statusRes.data.message || t("dashboard.codeProcessFailed"), variant: "destructive" });
              return;
            }
          }
          toast({ title: t("dashboard.codeStillQueued"), description: t("dashboard.refreshLater") });
        } else {
          toast({ title: t("dashboard.codeUseSuccess"), description: res.message || regCodeInfo.type_name, variant: "success" });
          setRegCode("");
          setRegCodeInfo(null);
          setShowConfirm(false);
          setEmbyUsername("");
          await fetchUser();
          await loadEmbyUrls();
        }
      } else {
        toast({ title: t("dashboard.useFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("dashboard.useFailed"), description: err.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setIsUsingCode(false);
    }
  };

  // ============== Emby 自由注册（登录后从仪表盘开通） ==============
  // 天数由管理员在配置里固定，前端只读取并展示
  const directRegisterDays = useMemo<number>(() => {
    if (hasGrantedEmbyRegisterEntitlement && user?.pending_emby_days !== null && user?.pending_emby_days !== undefined) {
      return Number(user.pending_emby_days);
    }
    const raw = Number(registerAvailability?.emby_direct_register_days);
    if (!Number.isFinite(raw) || raw === 0) return 30;
    return raw;
  }, [hasGrantedEmbyRegisterEntitlement, registerAvailability?.emby_direct_register_days, user?.pending_emby_days]);

  const directRegisterDaysLabel = directRegisterDays < 0 ? t("dashboard.permanent") : t("score.days", { days: directRegisterDays });

  const directRegisterBlockedReason = useMemo<string | null>(() => {
    if (!registerAvailability) return null;
    if (hasGrantedEmbyRegisterEntitlement) return null;
    if (!registerAvailability.emby_direct_register_enabled) return t("dashboard.directRegisterDisabled");
    if (user?.emby_id) return t("dashboard.alreadyBoundEmby");
    if (!user?.telegram_id) return t("dashboard.bindTelegramFirst");
    const limit = Number(registerAvailability.emby_user_limit ?? -1);
    const used = Number(registerAvailability.emby_bound_users ?? 0);
    if (limit > 0 && used >= limit) return t("dashboard.embyLimitReached", { used, limit });
    return null;
  }, [hasGrantedEmbyRegisterEntitlement, registerAvailability, t, user?.emby_id, user?.telegram_id]);

  const showEmbyDirectRegisterCard = Boolean(
    !user?.emby_id && (registerAvailability?.emby_direct_register_enabled || hasGrantedEmbyRegisterEntitlement),
  );

  const handleSubmitDirectRegister = async () => {
    if (directRegisterBlockedReason) {
      toast({ title: t("dashboard.temporarilyUnavailable"), description: directRegisterBlockedReason, variant: "destructive" });
      return;
    }
    const username = directEmbyUsername.trim();
    if (!username) {
      toast({ title: t("dashboard.embyUsernameRequired"), variant: "destructive" });
      return;
    }
    if (!/^[A-Za-z_][A-Za-z0-9_]{2,19}$/.test(username)) {
      toast({
        title: t("dashboard.invalidEmbyUsername"),
        description: t("dashboard.embyUsernameRule"),
        variant: "destructive",
      });
      return;
    }
    const password = directEmbyPassword;
    if (password.length < 8 || !/[a-z]/.test(password) || !/[A-Z]/.test(password) || !/\d/.test(password)) {
      toast({
        title: t("dashboard.passwordWeak"),
        description: t("dashboard.passwordRule"),
        variant: "destructive",
      });
      return;
    }

    setIsSubmittingDirectRegister(true);
    try {
      // 不再传 days：后端按 RegisterConfig.EMBY_DIRECT_REGISTER_DAYS 单值落库
      const res = await api.completeEmbyRegistration(username, password);
      if (res.success) {
        // 注册队列高峰期可能 60s 内还没跑完，后端会返回 success+pending=true 让前端轮询。
        // 这里只在终态时关掉弹窗 + 刷新；pending 状态保留窗口，再触发一次轮询。
        const pending = Boolean((res.data as any)?.pending);
        if (pending) {
          const data = res.data as any;
          if (data?.request_id && data?.status_token) {
            saveEmbyRegisterRequest(data.request_id, data.status_token);
            setEmbyRegisterStatus({
              request_id: data.request_id,
              status: data.status || "queued",
              queue_position: data.queue_position,
              message: res.message,
              updated_at: Math.floor(Date.now() / 1000),
            });
          }
          toast({
            title: t("dashboard.embyRegisterQueued"),
            description: data?.queue_position ? t("dashboard.queuePositionAutoComplete", { position: data.queue_position }) : t("dashboard.refreshToView"),
            variant: "default",
          });
          // 异步轮询，最多 60s
          void (async () => {
            const reqId = data?.request_id;
            const token = data?.status_token;
            if (!reqId || !token) return;
            for (let i = 0; i < 30; i++) {
              await new Promise((r) => setTimeout(r, 2000));
              try {
                const s = await api.getEmbyRegisterStatus(reqId, token);
                if (s.success && s.data) {
                  const st = s.data.status;
                  if (st === "success") {
                    clearEmbyRegisterRequest();
                    toast({
                      title: t("dashboard.embyOpened"),
                      description: directRegisterDays < 0 ? t("dashboard.permanent") : t("dashboard.openDuration", { days: directRegisterDays }),
                      variant: "success",
                    });
                    await fetchUser();
                    await loadEmbyUrls();
                    return;
                  }
                  if (st === "failed") {
                    toast({
                      title: t("dashboard.embyRegisterFailed"),
                      description: s.data.message || t("dashboard.retryLater"),
                      variant: "destructive",
                    });
                    return;
                  }
                }
              } catch (_) {
                // 忽略瞬时网络抖动
              }
            }
            toast({
              title: t("dashboard.queueTimeout"),
              description: t("dashboard.refreshLatest"),
              variant: "default",
            });
          })();
        } else {
          clearEmbyRegisterRequest();
          toast({
            title: t("dashboard.embyOpened"),
            description: directRegisterDays < 0 ? t("dashboard.permanent") : t("dashboard.openDuration", { days: directRegisterDays }),
            variant: "success",
          });
          setShowDirectRegisterDialog(false);
          setDirectEmbyPassword("");
          await fetchUser();
          await loadEmbyUrls();
        }
      } else {
        toast({ title: t("dashboard.openFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("dashboard.openFailed"), description: err.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setIsSubmittingDirectRegister(false);
    }
  };

  const handleQuickSignin = async () => {
    if (signingIn) return;
    setSigningIn(true);
    try {
      const res = await api.signinNow();
      if (res.success && res.data) {
        const bonus = res.data.bonus_points > 0 ? t("score.streakBonus", { points: res.data.bonus_points }) : "";
        if (res.data.created === false || res.data.total_today <= 0) {
          toast({
            title: t("score.alreadySigned"),
            description: t("score.currentStreak", { days: res.data.current_streak }),
          });
        } else {
          toast({
            title: t("score.signinSuccess", { points: res.data.total_today, currency: res.data.currency_name }),
            description: t("score.signinSuccessDescription", { days: res.data.current_streak, bonus }),
            variant: "success",
          });
        }
      } else {
        toast({ title: t("score.signinFailed"), description: res.message, variant: "destructive" });
      }
      const summary = await api.getSigninSummary().catch(() => null);
      if (summary?.success && summary.data) setSigninSummary(summary.data);
    } catch (err: any) {
      toast({ title: t("score.signinFailed"), description: err?.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setSigningIn(false);
    }
  };

  const handleActivateDeveloperMode = async () => {
    if (!developerPassword) {
      toast({ title: t("dashboard.developerPasswordRequired"), variant: "destructive" });
      return;
    }
    setActivatingDeveloper(true);
    try {
      const res = await api.activateDeveloperMode({ code: "DEBUGMODE", password: developerPassword });
      if (res.success) {
        invalidateSystemStore();
        await fetchSystemInfo(true).catch(() => null);
        if (res.data?.enabled) {
          sessionStorage.setItem("twilight:developer-mode", "1");
          toast({ title: t("dashboard.developerEnabled"), variant: "success" });
        } else {
          sessionStorage.removeItem("twilight:developer-mode");
          toast({ title: t("dashboard.developerDisabled"), variant: "success" });
        }
        setShowDeveloperDialog(false);
        setDeveloperPassword("");
        setRegCode("");
        if (res.data?.enabled) router.push("/admin/developer");
      } else {
        toast({ title: t("dashboard.developerEnableFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({ title: t("dashboard.developerEnableFailed"), description: err instanceof Error ? err.message : undefined, variant: "destructive" });
    } finally {
      setActivatingDeveloper(false);
    }
  };

  const handleSigninRenewal = async () => {
    if (renewingWithPoints || !signinRenewal?.enabled) return;
    setRenewingWithPoints(true);
    try {
      const res = await api.renewWithSigninCurrency();
      if (res.success && res.data) {
        setSigninSummary((prev) => prev
          ? {
              ...prev,
              current_points: res.data!.remaining_points,
              total_points: res.data!.remaining_points,
              renewal: res.data!.renewal,
            }
          : prev);
        toast({
          title: t("signinRenewal.successTitle"),
          description: t("signinRenewal.successDescription", { spent: res.data.spent_points, currencyName: res.data.currency_name, expireStatus: res.data.expire_status }),
          variant: "success",
        });
        await fetchUser();
        await loadEmbyUrls();
        const summary = await api.getSigninSummary().catch(() => null);
        if (summary?.success && summary.data) setSigninSummary(summary.data);
      } else {
        toast({ title: t("signinRenewal.failureTitle"), description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: t("signinRenewal.failureTitle"), description: err?.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setRenewingWithPoints(false);
    }
  };

  if (error) {
    return <PageError message={error} />;
  }

  return (
    <motion.div variants={container} initial="hidden" animate="show" className="space-y-6 pb-10">
      <div className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div className="min-w-0">
          <h1 className="text-3xl font-black tracking-tighter sm:text-4xl">
            {getGreeting()}，{user?.username}
          </h1>
          <p className="text-muted-foreground font-medium mt-1 text-sm sm:text-base">
            {isPendingEmby
              ? isPendingEmbyFromRegcode
                ? t("dashboard.pendingRegcodeDescription")
                : t("dashboard.pendingDirectDescription")
              : isPending
                ? t("dashboard.pendingAdminDescription")
                : t("dashboard.normalDescription")}
          </p>
        </div>
        <div className="flex shrink-0 flex-col items-start gap-1 md:items-end">
          <Badge className="w-fit border-primary/20 bg-primary/10 px-4 py-1.5 text-xs font-black uppercase tracking-widest text-primary md:hidden">
            {user?.role_name}
          </Badge>
        </div>
      </div>

      {/* 顶部三块: 到期 / 状态 / Emby 绑定 */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <motion.div variants={item} className="premium-card p-5 sm:p-6">
          <div className="flex items-start justify-between gap-2">
            <div className="p-3 w-fit bg-amber-500/10 text-amber-500 rounded-2xl">
              <Calendar className="h-5 w-5" />
            </div>
            {!isPermanent && !isPendingEmby && (
              <Badge variant={isExpired ? "destructive" : daysLeft <= 7 ? "warning" : "outline"} className="text-[10px]">
                {isExpired ? t("dashboard.expired") : t("dashboard.daysLeft", { days: daysLeft })}
              </Badge>
            )}
            {isPendingEmby && (
              <Badge variant="outline" className="text-[10px] text-muted-foreground">
                {t("dashboard.notOpened")}
              </Badge>
            )}
          </div>
          <p className="mt-4 text-[10px] font-black uppercase tracking-widest text-muted-foreground">{t("dashboard.expiryCountdown")}</p>
          <h3 className="text-2xl sm:text-3xl font-black mt-1 break-all">
            {isPendingEmby ? "—" : isPermanent ? t("dashboard.permanentDisplay") : t("score.days", { days: daysLeft })}
          </h3>
        </motion.div>

        <motion.div variants={item} className="premium-card p-5 sm:p-6">
          <div className="p-3 w-fit bg-purple-500/10 text-purple-500 rounded-2xl">
            <Clock className="h-5 w-5" />
          </div>
          <p className="mt-4 text-[10px] font-black uppercase tracking-widest text-muted-foreground">{t("dashboard.accountStatus")}</p>
          <h3 className="text-2xl sm:text-3xl font-black mt-1">
            {isPendingEmby ? t("dashboard.embyNotOpened") : isPending ? t("dashboard.embyPending") : isExpired ? t("dashboard.expired") : t("dashboard.normal")}
          </h3>
        </motion.div>

        <motion.div variants={item} className="premium-card p-5 sm:p-6 sm:col-span-2 lg:col-span-1">
          <div className="p-3 w-fit bg-emerald-500/10 text-emerald-500 rounded-2xl">
            <Gift className="h-5 w-5" />
          </div>
          <p className="mt-4 text-[10px] font-black uppercase tracking-widest text-muted-foreground">{t("dashboard.embyBinding")}</p>
          <h3 className="text-2xl sm:text-3xl font-black mt-1 truncate">
            {!user?.emby_id ? t("dashboard.unbound") : user?.active ? t("dashboard.normal") : t("dashboard.disabled")}
          </h3>
        </motion.div>
      </div>

      {/* 第二行: Telegram / Emby 服务器 / 我的求片 */}
      <div className="grid gap-4 lg:grid-cols-4">
        {/* Telegram */}
        <motion.div variants={item} className="premium-card p-5 sm:p-6 flex flex-col gap-3">
          <div className="flex items-start justify-between gap-2">
            <div className="flex items-center gap-2 min-w-0">
              <div className="p-2 bg-sky-500/10 text-sky-500 rounded-xl">
                <MessageCircle className="h-4 w-4" />
              </div>
              <h3 className="text-sm font-black uppercase tracking-widest text-muted-foreground">Telegram</h3>
            </div>
            <Badge
              variant={telegramStatus?.bound ? "success" : "secondary"}
              className="text-[10px] shrink-0"
            >
              {telegramStatus?.bound ? t("dashboard.bound") : t("dashboard.unbound")}
            </Badge>
          </div>

          <div className="min-w-0 space-y-1.5">
            {telegramStatus?.bound ? (
              <>
                <p className="text-base font-bold truncate">
                  {t("dashboard.telegramBound")}
                </p>
                <p className="text-xs text-muted-foreground truncate">{t("dashboard.telegramManageHint")}</p>
                {telegramStatus.pending_rebind_request && (
                  <p className="text-xs text-amber-500">{t("dashboard.telegramRebindPending")}</p>
                )}
              </>
            ) : (
              <p className="text-sm text-muted-foreground">
                {telegramStatus?.force_bind
                  ? t("dashboard.telegramRequired")
                  : t("dashboard.telegramBindHint")}
              </p>
            )}
          </div>
        </motion.div>

        {/* Emby 服务器 */}
        <motion.div variants={item} className="premium-card p-5 sm:p-6 flex flex-col gap-3">
          <div className="flex items-start justify-between gap-2">
            <div className="flex items-center gap-2 min-w-0">
              <div className="p-2 bg-emerald-500/10 text-emerald-500 rounded-xl">
                <Tv className="h-4 w-4" />
              </div>
              <h3 className="text-sm font-black uppercase tracking-widest text-muted-foreground">{t("dashboard.embyServer")}</h3>
            </div>
            <Badge
              variant={embyInfo?.online ? "success" : "destructive"}
              className="text-[10px] shrink-0"
            >
              {embyInfo?.online ? t("dashboard.online") : t("dashboard.offline")}
            </Badge>
          </div>

          <div className="min-w-0 space-y-1.5">
            <p className="text-base font-bold truncate">
              {embyInfo?.server_name || "Emby"}
            </p>
            <p className="text-xs text-muted-foreground truncate">
              {t("dashboard.version", { version: embyInfo?.version || "--" })}
            </p>
            {user?.emby_id ? (
              <p className="text-xs text-muted-foreground break-all" title={user.emby_id}>
                {t("dashboard.myEmby")}<span className="font-mono">{user.emby_id}</span>
              </p>
            ) : (
              <p className="text-xs text-amber-500">{t("dashboard.embyUnbound")}</p>
            )}
          </div>
        </motion.div>

        {/* 媒体概览 */}
        <motion.div variants={item} className="premium-card p-5 sm:p-6 flex flex-col gap-3">
          <div className="flex items-start justify-between gap-2">
            <div className="flex items-center gap-2 min-w-0">
              <div className="p-2 bg-indigo-500/10 text-indigo-500 rounded-xl">
                <Activity className="h-4 w-4" />
              </div>
              <h3 className="text-sm font-black uppercase tracking-widest text-muted-foreground">{t("dashboard.mediaOverview")}</h3>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <Badge variant={embyViewers > 0 ? "success" : "outline"} className="text-[10px]">
                {t("dashboard.liveNow", { count: embyViewers })}
              </Badge>
              <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => void loadEmbyLibraryStats(true)} disabled={embyStatsRefreshing}>
                {embyStatsRefreshing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
                <span className="sr-only">{t("common.refresh")}</span>
              </Button>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-2 text-xs">
            <StatPill icon={Activity} tone="success" label={t("dashboard.onlineViewers")} value={embyViewers} />
            <StatPill icon={Film} tone="primary" label={t("dashboard.movieCount")} value={embyStats?.movie_count ?? 0} />
            <StatPill icon={Tv} tone="info" label={t("dashboard.seriesCount")} value={embyStats?.series_count ?? 0} />
            <StatPill icon={Play} tone="warning" label={t("dashboard.episodeCount")} value={embyStats?.episode_count ?? 0} />
          </div>
          {(!embyStats?.enabled || !embyStats?.configured) && (
            <p className="text-xs text-muted-foreground">{t("dashboard.libraryStatsUnavailable")}</p>
          )}
        </motion.div>

        {/* 求片状态 */}
        {mediaRequestEnabled && (
        <motion.div variants={item} className="premium-card p-5 sm:p-6 flex flex-col gap-3">
          <div className="flex items-start justify-between gap-2">
            <div className="flex items-center gap-2 min-w-0">
              <div className="p-2 bg-primary/10 text-primary rounded-xl">
                <Film className="h-4 w-4" />
              </div>
              <h3 className="text-sm font-black uppercase tracking-widest text-muted-foreground">{t("dashboard.myRequests")}</h3>
            </div>
            <Badge variant="outline" className="text-[10px] shrink-0">
              {t("dashboard.totalCount", { count: myRequests.length })}
            </Badge>
          </div>

          <div className="grid grid-cols-2 gap-2 text-xs sm:grid-cols-3">
            <StatPill icon={Hourglass} tone="warning" label={t("media.statusUnhandled")} value={requestStats.pending} />
            <StatPill icon={CheckCircle2} tone="primary" label={t("media.statusAccepted")} value={requestStats.accepted} />
            <StatPill icon={Download} tone="info" label={t("media.statusDownloading")} value={requestStats.downloading} />
            <StatPill icon={CheckCircle2} tone="success" label={t("media.statusCompleted")} value={requestStats.completed} />
            <StatPill icon={XCircle} tone="destructive" label={t("media.statusRejected")} value={requestStats.rejected} />
          </div>

          {latestRequests.length > 0 ? (
            <div className="mt-auto space-y-1.5 pt-1 text-xs">
              <p className="text-[10px] font-black uppercase tracking-widest text-muted-foreground">{t("dashboard.latestRequests")}</p>
              {latestRequests.map((req) => (
                <div key={req.id} className="flex items-center justify-between gap-2 min-w-0">
                  <span className="truncate">
                    {req.media_info?.title || req.title}
                    {req.media_info?.season ? t("dashboard.requestSeason", { season: req.media_info.season }) : ""}
                  </span>
                  <RequestStatusBadge status={req.status} />
                </div>
              ))}
            </div>
          ) : (
            <p className="text-xs text-muted-foreground mt-auto pt-1">{t("dashboard.noRequests")}</p>
          )}
        </motion.div>
        )}
      </div>

      {/* 签到 / 积分 快捷区 */}
      {signinSummary?.enabled && (
        <motion.div variants={item} className="premium-card p-5 sm:p-6">
          <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
            <div className="flex items-center gap-4 min-w-0">
              <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-amber-500/15 text-amber-500">
                <Coins className="h-6 w-6" />
              </div>
              <div className="min-w-0">
                <p
                  className="text-[10px] font-black uppercase tracking-widest text-muted-foreground whitespace-nowrap truncate"
                  title={t("dashboard.myCurrency", { currency: signinSummary.currency_name })}
                >
                  {t("dashboard.myCurrency", { currency: signinSummary.currency_name })}
                </p>
                <div className="flex items-baseline gap-2 whitespace-nowrap">
                  <h3 className="text-2xl font-black tracking-tight">{signinSummary.current_points}</h3>
                  <span className="text-sm text-muted-foreground truncate">{signinSummary.currency_name}</span>
                </div>
                <p className="mt-0.5 text-xs text-muted-foreground inline-flex items-center gap-1">
                  <Flame className="h-3 w-3 text-orange-500" />
                  {t("dashboard.streak", { days: signinSummary.current_streak })}
                  {signinSummary.next_bonus_in_days && signinSummary.next_bonus_points ? (
                    <span className="ml-1">{t("dashboard.nextBonus", { days: signinSummary.next_bonus_in_days, points: signinSummary.next_bonus_points })}</span>
                  ) : null}
                </p>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2 md:justify-end">
              {signinRenewal?.enabled && (
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={handleSigninRenewal}
                  disabled={renewingWithPoints || !signinRenewal.affordable || isPermanent || isPendingEmby}
                  title={signinRenewal.affordable ? undefined : t("signinRenewal.actionTitle", { cost: signinRenewal.cost, currencyName: signinSummary.currency_name, days: signinRenewal.days })}
                >
                  {renewingWithPoints ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Calendar className="mr-2 h-4 w-4" />
                  )}
                  {t("signinRenewal.actionLabel", { cost: signinRenewal.cost, currencyName: signinSummary.currency_name, days: signinRenewal.days })}
                </Button>
              )}
              <Button
                size="sm"
                onClick={handleQuickSignin}
                disabled={signingIn || signinSummary.today_signed}
              >
                {signingIn ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : signinSummary.today_signed ? (
                  <CheckCircle2 className="mr-2 h-4 w-4" />
                ) : (
                  <Sparkles className="mr-2 h-4 w-4" />
                )}
                {signinSummary.today_signed ? t("dashboard.signedToday") : t("dashboard.signinNow")}
              </Button>
              <Button asChild size="sm" variant="outline">
                <Link href="/score">{t("dashboard.signinCenter")}</Link>
              </Button>
            </div>
          </div>
        </motion.div>
      )}

      {linesRequireRenewal || embyDisabledByExpiry ? (
        <motion.div variants={item} className="premium-card border-destructive/30 p-5 sm:p-6">
          <div className="flex items-start gap-3">
            <div className="rounded-xl bg-destructive/10 p-2 text-destructive">
              <AlertCircle className="h-5 w-5" />
            </div>
            <div>
              <h3 className="text-base font-black tracking-tight">{t("dashboard.embyExpired")}</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                {t("dashboard.embyExpiredDescription")}
              </p>
            </div>
          </div>
        </motion.div>
      ) : !linesRequireEmby && (
      <motion.div variants={item} className="premium-card p-5 sm:p-6">
        <div className="flex flex-wrap items-center justify-between gap-3 mb-4">
          <div className="flex items-center gap-3 min-w-0">
            <div className="p-2 bg-primary/10 rounded-xl text-primary">
              <Globe className="h-5 w-5" />
            </div>
            <div className="min-w-0">
              <h3 className="text-base font-black tracking-tight">{t("dashboard.serverLatency")}</h3>
              <p className="text-[11px] text-muted-foreground font-bold uppercase tracking-tighter">
                {t("dashboard.serverNetworkStatus")}
              </p>
            </div>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void runLineLatencyTests()}
            disabled={isLatencyTesting || lineSlots.length === 0}
          >
            {isLatencyTesting ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="mr-2 h-4 w-4" />
            )}
            {t("dashboard.retest")}
          </Button>
        </div>

        {lineSlots.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {isLoading ? t("dashboard.loadingLines") : t("dashboard.noLines")}
          </p>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {sortedLineSlots.map((slot) => (
              <div
                key={slot.key}
                className="flex items-center justify-between gap-3 rounded-xl border border-border/60 bg-card/60 px-4 py-3"
              >
                <div className="flex items-center gap-2 min-w-0">
                  {slot.scope === "wl" ? (
                    <Badge variant="outline" className="border-yellow-500/30 text-yellow-600 dark:text-yellow-400 text-[10px] shrink-0">
                      {t("dashboard.dedicated")}
                    </Badge>
                  ) : (
                    <Badge variant="outline" className="text-[10px] shrink-0">
                      {t("dashboard.public")}
                    </Badge>
                  )}
                  <span className="text-sm font-semibold truncate" title={slot.name}>
                    {slot.name}
                  </span>
                </div>
                <span
                  className={`shrink-0 rounded-full px-2.5 py-1 text-[11px] font-bold tabular-nums ${renderLatencyToneClass(slot.key)}`}
                >
                  {renderLatencyText(slot.key)}
                </span>
              </div>
            ))}
          </div>
        )}
      </motion.div>
      )}

      {/* 注册码/续期码/邀请码 */}
      <motion.div variants={item} className="premium-card p-5 sm:p-6">
        <div className="flex items-center gap-3 mb-5">
          <div className="p-2 bg-primary/10 rounded-xl text-primary">
            <Key className="h-5 w-5" />
          </div>
          <div>
            <h3 className="text-base font-black tracking-tight">{t("dashboard.codeUseTitle")}</h3>
            <p className="text-[11px] text-muted-foreground font-bold uppercase tracking-tighter">{t("dashboard.codeUseSubtitle")}</p>
          </div>
        </div>

        <div className="flex flex-col gap-3 md:flex-row">
          <Input
            placeholder={t("dashboard.codePlaceholder")}
            value={regCode}
            onChange={(e) => setRegCode(e.target.value)}
            className="h-12 rounded-xl border-white/60 bg-white/40 shadow-inner"
          />
          <Button
            onClick={handleCheckRegcode}
            disabled={isLoading || isUsingCode}
            className="h-12 rounded-xl font-black md:w-auto w-full"
          >
            {isUsingCode ? <Loader2 className="h-4 w-4 animate-spin" /> : t("dashboard.verifyAndUse")}
          </Button>
        </div>
      </motion.div>

      {/* Emby 自由注册 / 管理员授予资格：登录后可在仪表盘开通 */}
      {showEmbyDirectRegisterCard && (
        <motion.div variants={item} className="premium-card p-5 sm:p-6">
          <div className="flex items-center gap-3 mb-4">
            <div className="p-2 bg-emerald-500/10 rounded-xl text-emerald-500">
              <Gift className="h-5 w-5" />
            </div>
            <div>
              <h3 className="text-base font-black tracking-tight">
                {hasGrantedEmbyRegisterEntitlement ? t("dashboard.grantedEmbyTitle") : t("dashboard.directRegisterTitle")}
              </h3>
              <p className="text-[11px] text-muted-foreground font-bold uppercase tracking-tighter">
                {t("dashboard.directRegisterSubtitle")}
              </p>
            </div>
          </div>

          <div className="space-y-3 text-sm">
            <p className="text-muted-foreground">
              {hasGrantedEmbyRegisterEntitlement
                ? t("dashboard.grantedEmbyDescription")
                : directRegisterBlockedReason
                ? directRegisterBlockedReason
                : t("dashboard.directRegisterDescription")}
            </p>

            <Button
              className="rounded-xl font-black"
              disabled={Boolean(directRegisterBlockedReason)}
              onClick={() => {
                setShowDirectRegisterDialog(true);
              }}
            >
              <UserPlus className="mr-2 h-4 w-4" />
              {t("dashboard.openEmbyNow")}
            </Button>
          </div>
        </motion.div>
      )}

      {embyRegisterStored && embyRegisterStatus && (
        <motion.div variants={item} className="premium-card p-5 sm:p-6 border-emerald-500/20 bg-emerald-500/5">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <h3 className="text-base font-black tracking-tight">{t("dashboard.registerQueueStatus")}</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                {embyRegisterStatus.status === "queued"
                  ? t("dashboard.queued", { position: embyRegisterStatus.queue_position ? t("dashboard.queuePositionSuffix", { position: embyRegisterStatus.queue_position }) : "" })
                  : embyRegisterStatus.status === "processing"
                    ? t("dashboard.processing")
                    : embyRegisterStatus.status === "success"
                      ? t("dashboard.registerComplete")
                      : embyRegisterStatus.message || t("dashboard.registerFailed")}
              </p>
              {embyRegisterStatus.message && (
                <p className="mt-1 text-xs text-muted-foreground">{embyRegisterStatus.message}</p>
              )}
            </div>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={() => void refreshStoredEmbyRegisterStatus()}>
                <RefreshCw className="mr-2 h-4 w-4" />
                {t("common.refresh")}
              </Button>
              {(["success", "failed", "rejected"] as const).includes(embyRegisterStatus.status as any) && (
                <Button variant="ghost" size="sm" onClick={clearEmbyRegisterRequest}>
                  {t("common.close")}
                </Button>
              )}
            </div>
          </div>
        </motion.div>
      )}

      {/* 公告板 —— 仪表盘最下方 */}
      <motion.div variants={item}>
        <AnnouncementBoard splitPinned />
      </motion.div>

      {/* Emby 自由注册对话框 */}
      <Dialog open={showDirectRegisterDialog} onOpenChange={setShowDirectRegisterDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("dashboard.openEmbyTitle")}</DialogTitle>
          </DialogHeader>

          <div className="space-y-4 py-2 text-sm">
            <div className="space-y-2">
              <Label className="text-xs uppercase tracking-wider text-muted-foreground">{t("dashboard.duration")}</Label>
              <p className="rounded-md border bg-muted/40 px-3 py-2 text-sm font-medium">
                {directRegisterDaysLabel}
                <span className="ml-2 text-xs font-normal text-muted-foreground">
                  {t("dashboard.durationConfigured")}
                </span>
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="directEmbyUser">{t("dashboard.embyUsername")}</Label>
              <Input
                id="directEmbyUser"
                value={directEmbyUsername}
                onChange={(e) => setDirectEmbyUsername(e.target.value)}
                placeholder={t("dashboard.embyUsernameRule")}
                disabled={isSubmittingDirectRegister}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="directEmbyPwd">{t("dashboard.embyPassword")}</Label>
              <div className="relative">
                <Input
                  id="directEmbyPwd"
                  type={showDirectEmbyPassword ? "text" : "password"}
                  value={directEmbyPassword}
                  onChange={(e) => setDirectEmbyPassword(e.target.value)}
                  placeholder={t("auth.register.passwordPlaceholder")}
                  className="pr-10"
                  disabled={isSubmittingDirectRegister}
                />
                <button
                  type="button"
                  onClick={() => setShowDirectEmbyPassword((v) => !v)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  aria-label={t("common.showPassword")}
                >
                  {showDirectEmbyPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
            </div>

            {directRegisterBlockedReason && (
              <p className="rounded-md border border-destructive/40 bg-destructive/5 p-2 text-xs text-destructive">
                {directRegisterBlockedReason}
              </p>
            )}
          </div>

          <div className="flex gap-3 justify-end">
            <Button variant="outline" onClick={() => setShowDirectRegisterDialog(false)}>{t("common.cancel")}</Button>
            <Button
              onClick={handleSubmitDirectRegister}
              disabled={isSubmittingDirectRegister || Boolean(directRegisterBlockedReason)}
            >
              {isSubmittingDirectRegister && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t("dashboard.confirmOpen")}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={showConfirm} onOpenChange={setShowConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{regCodeInfo?.confirm_title || t("dashboard.confirmUseCode")}</DialogTitle>
          </DialogHeader>
          {regCodeInfo && (
            <div className="space-y-2 text-sm text-muted-foreground">
              <p>{t("dashboard.type", { type: regCodeInfo.type_name })}</p>
              {regCodeInfo.source === "invite" && regCodeInfo.inviter && (
                <p>{t("dashboard.inviter", { inviter: regCodeInfo.inviter })}</p>
              )}
              <p>{regCodeInfo.duration_label}</p>
              {!regCodeInfo.requires_emby_credentials && <p>{regCodeInfo.description}</p>}
            </div>
          )}
          {regCodeInfo?.requires_emby_credentials && (
            <div className="space-y-3 rounded-lg border border-border p-3">
              <p className="text-sm font-medium">
                {regCodeInfo.description}
              </p>
              <div className="space-y-2">
                <Label htmlFor="embyUsername">{t("dashboard.embyUsername")}</Label>
                <Input
                  id="embyUsername"
                  value={embyUsername}
                  onChange={(e) => setEmbyUsername(e.target.value)}
                  placeholder={t("dashboard.embyUsernameRequired")}
                />
              </div>
              <p className="text-xs text-muted-foreground">
                {t("dashboard.codeEmbyHint")}
              </p>
            </div>
          )}
          <div className="flex gap-3 justify-end">
            <Button variant="outline" onClick={() => setShowConfirm(false)}>{t("common.cancel")}</Button>
            <Button onClick={handleUseRegcode} disabled={isUsingCode}>
              {isUsingCode ? <Loader2 className="h-4 w-4 animate-spin" /> : regCodeInfo?.submit_label || t("common.confirm")}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={showDeveloperDialog} onOpenChange={setShowDeveloperDialog}>
        <DialogContent className="sm:max-w-[480px]">
          <DialogHeader>
            <DialogTitle>
              {systemInfo?.features?.developer_mode 
                ? "关闭开发者模式" 
                : "启用开发者模式"}
            </DialogTitle>
            <DialogDescription>
              {systemInfo?.features?.developer_mode
                ? "输入当前管理员密码验证，以全面关闭服务端开发者 JS 运行沙箱。"
                : "输入当前管理员密码验证，以开启开发预编译沙箱及 Telegram 自定义脚本配置。"}
            </DialogDescription>
          </DialogHeader>

          <div className="rounded-lg border-2 border-red-500/50 bg-red-500/5 p-4 text-xs font-semibold leading-relaxed text-red-500 space-y-2">
            <p className="text-sm font-black uppercase tracking-wider text-red-600 dark:text-red-400">
              ⚠️ 免责声明与最高安全警告 / DISCLAIMER & WARNING
            </p>
            <p className="font-extrabold text-xs">
              {systemInfo?.features?.developer_mode
                ? "警告：关闭开发者模式后，全站所有「js:」前缀的自定义指令脚本、辅助 sandbox 运行、inline callback 等 JS 交互都会立即被服务端全面阻断。由此带来的线上交互故障需要管理员自行检查，关闭操作产生的最终后果由系统管理员承担。"
                : "重要声明：启用开发者模式（自定义 JS 沙箱）允许管理员部署和预览动态脚本。在此期间的所有代码编写、脚本执行以及与 Telegram 交互所产生的一切权限穿透、配置使用、异常流、运行行为与数据变动等，以及导致的一切安全防线穿透或数据风险，均由系统管理员自愿承担全部技术和法律风险与最终后果！"}
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="developerPassword">{t("dashboard.developerPasswordLabel")}</Label>
            <Input
              id="developerPassword"
              type="password"
              placeholder="请输入管理员密码完成验证"
              value={developerPassword}
              onChange={(event) => setDeveloperPassword(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") void handleActivateDeveloperMode();
              }}
              disabled={activatingDeveloper}
            />
            <p className="text-xs text-muted-foreground">
              {t("dashboard.developerRiskHint")}
            </p>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDeveloperDialog(false)} disabled={activatingDeveloper}>
              {t("common.cancel")}
            </Button>
            <Button
              variant={systemInfo?.features?.developer_mode ? "destructive" : "default"}
              onClick={() => void handleActivateDeveloperMode()}
              disabled={activatingDeveloper}
            >
              {activatingDeveloper && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {systemInfo?.features?.developer_mode ? "立即关闭 (Deactivate)" : "立即启用 (Activate)"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}

function StatPill({
  icon: Icon,
  tone,
  label,
  value,
}: {
  icon: React.ComponentType<{ className?: string }>;
  tone: "warning" | "primary" | "info" | "success" | "destructive";
  label: string;
  value: number;
}) {
  const toneClass: Record<string, string> = {
    warning: "bg-amber-500/10 text-amber-600 dark:text-amber-400",
    primary: "bg-primary/10 text-primary",
    info: "bg-sky-500/10 text-sky-600 dark:text-sky-400",
    success: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
    destructive: "bg-destructive/10 text-destructive",
  };
  return (
    <div className={`flex items-center justify-between gap-2 rounded-lg px-2.5 py-1.5 ${toneClass[tone]}`}>
      <span className="flex items-center gap-1.5 truncate">
        <Icon className="h-3.5 w-3.5 shrink-0" />
        <span className="truncate">{label}</span>
      </span>
      <span className="font-bold tabular-nums">{value}</span>
    </div>
  );
}

function RequestStatusBadge({ status }: { status?: string }) {
  const { t } = useI18n();
  const s = (status || "").toUpperCase();
  if (s === "UNHANDLED" || s === "PENDING") {
    return <Badge variant="warning" className="shrink-0 text-[10px]">{t("media.statusUnhandled")}</Badge>;
  }
  if (s === "ACCEPTED") {
    return <Badge variant="outline" className="shrink-0 text-[10px] border-primary/40 text-primary">{t("media.statusAccepted")}</Badge>;
  }
  if (s === "DOWNLOADING") {
    return (
      <Badge variant="outline" className="shrink-0 text-[10px] border-sky-500/40 text-sky-600 dark:text-sky-400">
        {t("media.statusDownloading")}
      </Badge>
    );
  }
  if (s === "COMPLETED") {
    return <Badge variant="success" className="shrink-0 text-[10px]">{t("media.statusCompleted")}</Badge>;
  }
  if (s === "REJECTED") {
    return <Badge variant="destructive" className="shrink-0 text-[10px]">{t("media.statusRejected")}</Badge>;
  }
  return <Badge variant="secondary" className="shrink-0 text-[10px]">{status}</Badge>;
}
