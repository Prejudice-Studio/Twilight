"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { motion } from "framer-motion";
import Link from "next/link";
import {
  Calendar,
  Clock,
  Eye,
  EyeOff,
  Gift,
  Key,
  Loader2,
  MessageCircle,
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
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useToast } from "@/hooks/use-toast";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError } from "@/components/layout/page-state";
import { useAuthStore } from "@/store/auth";
import { useSystemStore } from "@/store/system";
import { api, type CodeUsePreview, type EmbyInfo, type MediaRequest, type TelegramStatus, type SigninSummary, type RegisterAvailability, type EmbyRegisterStatus } from "@/lib/api";
import { AnnouncementBoard } from "@/components/announcement-board";
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
  const { user, fetchUser } = useAuthStore();
  const { fetchInfo: fetchSystemInfo } = useSystemStore();
  const { toast } = useToast();
  const [regCode, setRegCode] = useState("");
  const [regCodeInfo, setRegCodeInfo] = useState<CodeCheckInfo | null>(null);
  const [showConfirm, setShowConfirm] = useState(false);
  const [isUsingCode, setIsUsingCode] = useState(false);
  const [embyUsername, setEmbyUsername] = useState("");
  const [embyPassword, setEmbyPassword] = useState("");
  const [showEmbyPassword, setShowEmbyPassword] = useState(false);
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
  const [myRequests, setMyRequests] = useState<MediaRequest[]>([]);
  const [lineSlots, setLineSlots] = useState<LineSlot[]>([]);
  const [linesRequireEmby, setLinesRequireEmby] = useState(false);
  const [linesRequireRenewal, setLinesRequireRenewal] = useState(false);
  const [lineLatencyMap, setLineLatencyMap] = useState<Record<string, LineLatencyInfo>>({});
  const [isLatencyTesting, setIsLatencyTesting] = useState(false);
  const [signinSummary, setSigninSummary] = useState<SigninSummary | null>(null);
  const [signingIn, setSigningIn] = useState(false);

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
      name: line.name || `线路 ${index + 1}`,
      url: line.url,
      scope: "line" as const,
    }));
    const wl = (data.whitelist_lines || []).map((line, index) => ({
      key: `wl:${index}:${line.url}`,
      name: line.name || `专属线路 ${index + 1}`,
      url: line.url,
      scope: "wl" as const,
    }));
    setLineSlots([...lines, ...wl]);
    setLinesRequireEmby(Boolean(data.requires_emby_account));
    setLinesRequireRenewal(Boolean(data.requires_renewal || data.emby_disabled_by_expiry));
  }, []);

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
        }
      } else if (!res.success) {
        clearEmbyRegisterRequest();
      }
    } catch {
      // Keep the local request for 15 minutes; transient errors should not hide status.
    }
  }, [clearEmbyRegisterRequest, embyRegisterStored, fetchUser, loadEmbyUrls]);

  useEffect(() => {
    if (!embyRegisterStored) return;
    void refreshStoredEmbyRegisterStatus();
    const timer = window.setInterval(() => {
      void refreshStoredEmbyRegisterStatus();
    }, 5000);
    return () => window.clearInterval(timer);
  }, [embyRegisterStored, refreshStoredEmbyRegisterStatus]);

  const loadDashboardData = useCallback(async (signal?: AbortSignal) => {
    const [tgRes, embyRes, urlsRes, reqRes, signinRes, registerRes] = await Promise.all([
      api.getTelegramStatus().catch(() => null),
      api.getEmbyInfo().catch(() => null),
      api.getEmbyUrls().catch(() => null),
      api.getMyRequests(signal).catch(() => null),
      api.getSigninSummary().catch(() => null),
      api.getRegisterAvailability().catch(() => null),
    ]);

    if (signinRes && signinRes.success && signinRes.data) {
      setSigninSummary(signinRes.data);
    }

    if (tgRes && tgRes.success && tgRes.data) {
      setTelegramStatus(tgRes.data);
    }
    if (embyRes && embyRes.success && embyRes.data) {
      setEmbyInfo(embyRes.data);
    }
    if (urlsRes && urlsRes.success && urlsRes.data) {
      applyEmbyUrls(urlsRes.data);
    }
    if (reqRes && reqRes.success && Array.isArray(reqRes.data)) {
      setMyRequests(reqRes.data);
    }
    if (registerRes && registerRes.success && registerRes.data) {
      setRegisterAvailability(registerRes.data);
    }
    return true;
  }, [applyEmbyUrls]);

  const {
    isLoading,
    error,
  } = useAsyncResource(loadDashboardData, { immediate: true });

  // ============== 线路延迟测试 ==============
  const testSingleLineLatency = useCallback(async (rawUrl: string): Promise<LineLatencyInfo> => {
    const url = rawUrl.trim();
    if (!url) {
      return { status: "error" };
    }
    const normalizedUrl = /^https?:\/\//i.test(url) ? url : `https://${url.replace(/^\/+/, "")}`;
    const probeUrl = `${normalizedUrl}${normalizedUrl.includes("?") ? "&" : "?"}tw_ping=${Date.now()}`;
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), 8000);
    const startedAt = performance.now();
    try {
      await fetch(probeUrl, {
        method: "GET",
        mode: "no-cors",
        cache: "no-store",
        signal: controller.signal,
      });
      return {
        status: "ok",
        latencyMs: Math.max(1, Math.round(performance.now() - startedAt)),
      };
    } catch (err: any) {
      if (err?.name === "AbortError") return { status: "timeout" };
      return { status: "error" };
    } finally {
      window.clearTimeout(timeout);
    }
  }, []);

  const runLineLatencyTests = useCallback(async () => {
    if (lineSlots.length === 0) {
      setLineLatencyMap({});
      return;
    }
    setIsLatencyTesting(true);
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
    if (!info || info.status === "idle") return "待测速";
    if (info.status === "testing") return "测速中...";
    if (info.status === "ok") return `${info.latencyMs} ms`;
    if (info.status === "timeout") return "超时";
    return "不可达";
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

  const getGreeting = () => {
    const hour = new Date().getHours();
    if (hour < 6) return "凌晨好";
    if (hour < 9) return "早上好";
    if (hour < 12) return "上午好";
    if (hour < 14) return "中午好";
    if (hour < 18) return "下午好";
    if (hour < 22) return "晚上好";
    return "夜深了";
  };

  // ============== 卡码/邀请码流程 ==============
  const handleCheckRegcode = async () => {
    const code = regCode.trim();
    if (!code) {
      toast({ title: "请输入注册码/续期码/邀请码", variant: "destructive" });
      return;
    }
    try {
      const res = await api.useCode(code, { checkOnly: true });
      if (res.success && res.data?.source && res.data.type_name) {
        setRegCodeInfo(res.data as CodeUsePreview);
        setEmbyUsername("");
        setEmbyPassword("");
        setShowEmbyPassword(false);
        setShowConfirm(true);
      } else {
        toast({ title: "卡码无效", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "检查失败", description: err.message || "网络异常", variant: "destructive" });
    }
  };

  const handleUseRegcode = async () => {
    if (!regCodeInfo || !regCode.trim()) return;
    const requiresEmbyRegister = Boolean(regCodeInfo.requires_emby_credentials);
    const validateEmbyPassword = (pwd: string) => {
      if (pwd.length < 8) return "Emby 密码至少 8 位";
      if (!/[a-z]/.test(pwd)) return "Emby 密码至少包含一个小写字母";
      if (!/[A-Z]/.test(pwd)) return "Emby 密码至少包含一个大写字母";
      if (!/\d/.test(pwd)) return "Emby 密码至少包含一个数字";
      return "";
    };
    if (requiresEmbyRegister) {
      if (!embyUsername.trim()) {
        toast({ title: "请输入 Emby 用户名", variant: "destructive" });
        return;
      }
      const pwdErr = validateEmbyPassword(embyPassword);
      if (pwdErr) {
        toast({ title: "Emby 密码强度不足", description: pwdErr, variant: "destructive" });
        return;
      }
    }
    setIsUsingCode(true);
    try {
      const res = await api.useCode(
        regCode.trim(),
        requiresEmbyRegister
          ? { embyUsername: embyUsername.trim(), embyPassword }
          : undefined
      );
      if (res.success) {
        if (res.data?.pending && res.data.request_id && res.data.status_token) {
          toast({
            title: res.data.reused ? "已有卡码请求处理中" : "卡码已加入处理队列",
            description: res.data.queue_position ? `排队中（第 ${res.data.queue_position} 位）` : "稍后会自动完成",
          });
          setShowConfirm(false);

          for (let i = 0; i < 90; i++) {
            await new Promise((resolve) => setTimeout(resolve, 2000));
            const statusRes = await api.getUseCodeStatus(res.data.request_id, res.data.status_token);
            if (!statusRes.success || !statusRes.data) continue;
            if (statusRes.data.status === "success") {
              toast({ title: "卡码使用成功", description: statusRes.data.message || regCodeInfo.type_name, variant: "success" });
              setRegCode("");
              setRegCodeInfo(null);
              setEmbyUsername("");
              setEmbyPassword("");
              await fetchUser();
              await loadEmbyUrls();
              return;
            }
            if (statusRes.data.status === "failed") {
              toast({ title: "使用失败", description: statusRes.data.message || "卡码处理失败", variant: "destructive" });
              return;
            }
          }
          toast({ title: "卡码仍在队列中", description: "请稍后刷新页面查看结果" });
        } else {
          toast({ title: "卡码使用成功", description: res.message || regCodeInfo.type_name, variant: "success" });
          setRegCode("");
          setRegCodeInfo(null);
          setShowConfirm(false);
          setEmbyUsername("");
          setEmbyPassword("");
          await fetchUser();
          await loadEmbyUrls();
        }
      } else {
        toast({ title: "使用失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "使用失败", description: err.message || "网络异常", variant: "destructive" });
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

  const directRegisterDaysLabel = directRegisterDays <= 0 ? "永久" : `${directRegisterDays} 天`;

  const directRegisterBlockedReason = useMemo<string | null>(() => {
    if (!registerAvailability) return null;
    if (hasGrantedEmbyRegisterEntitlement) return null;
    if (!registerAvailability.emby_direct_register_enabled) return "管理员尚未开启自由注册";
    if (user?.emby_id) return "您已经绑定了 Emby 账号";
    if (!user?.telegram_id) return "请先在「设置 → Telegram」中完成 Telegram 绑定";
    const limit = Number(registerAvailability.emby_user_limit ?? -1);
    const used = Number(registerAvailability.emby_bound_users ?? 0);
    if (limit > 0 && used >= limit) return `Emby 已绑定用户数已达上限（${used}/${limit}）`;
    return null;
  }, [hasGrantedEmbyRegisterEntitlement, registerAvailability, user?.emby_id, user?.telegram_id]);

  const showEmbyDirectRegisterCard = Boolean(
    !user?.emby_id && (registerAvailability?.emby_direct_register_enabled || hasGrantedEmbyRegisterEntitlement),
  );

  const handleSubmitDirectRegister = async () => {
    if (directRegisterBlockedReason) {
      toast({ title: "暂时无法开通", description: directRegisterBlockedReason, variant: "destructive" });
      return;
    }
    const username = directEmbyUsername.trim();
    if (!username) {
      toast({ title: "请输入 Emby 用户名", variant: "destructive" });
      return;
    }
    if (!/^[A-Za-z_][A-Za-z0-9_]{2,19}$/.test(username)) {
      toast({
        title: "Emby 用户名格式不正确",
        description: "3-20 位字母数字下划线，不能以数字开头",
        variant: "destructive",
      });
      return;
    }
    const password = directEmbyPassword;
    if (password.length < 8 || !/[a-z]/.test(password) || !/[A-Z]/.test(password) || !/\d/.test(password)) {
      toast({
        title: "密码强度不足",
        description: "至少 8 位，且包含大小写字母和数字",
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
            title: "Emby 注册已加入队列",
            description: data?.queue_position ? `排队中（第 ${data.queue_position} 位），稍后会自动完成` : "稍后会自动完成，请刷新页面查看",
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
                      title: "Emby 账号已开通",
                      description: directRegisterDays <= 0 ? "永久" : `开通时长 ${directRegisterDays} 天`,
                      variant: "success",
                    });
                    await fetchUser();
                    await loadEmbyUrls();
                    return;
                  }
                  if (st === "failed") {
                    toast({
                      title: "Emby 注册失败",
                      description: s.data.message || "请稍后重试",
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
              title: "队列处理超时",
              description: "请刷新页面查看最新状态",
              variant: "default",
            });
          })();
        } else {
          clearEmbyRegisterRequest();
          toast({
            title: "Emby 账号已开通",
            description: directRegisterDays <= 0 ? "永久" : `开通时长 ${directRegisterDays} 天`,
            variant: "success",
          });
          setShowDirectRegisterDialog(false);
          setDirectEmbyPassword("");
          await fetchUser();
          await loadEmbyUrls();
        }
      } else {
        toast({ title: "开通失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "开通失败", description: err.message || "网络异常", variant: "destructive" });
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
        const bonus = res.data.bonus_points > 0 ? `（含连签加成 +${res.data.bonus_points}）` : "";
        toast({
          title: `签到成功 +${res.data.total_today} ${res.data.currency_name}`,
          description: `当前连签 ${res.data.current_streak} 天 ${bonus}`,
          variant: "success",
        });
      } else {
        toast({ title: "签到失败", description: res.message, variant: "destructive" });
      }
      const summary = await api.getSigninSummary().catch(() => null);
      if (summary?.success && summary.data) setSigninSummary(summary.data);
    } catch (err: any) {
      toast({ title: "签到失败", description: err?.message || "网络异常", variant: "destructive" });
    } finally {
      setSigningIn(false);
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
                ? "你的系统账号已建好，但还没绑定 Emby 账号，可通过补建弹窗完成注册"
                : "你的系统账号已建好，但还没绑定 Emby 账号，可在下方自由注册入口开通"
              : isPending
                ? "当前账号可登录，若需媒体服务请联系管理员开通 Emby 账号"
                : "欢迎回来，当前账号状态正常"}
          </p>
        </div>
        <Badge className="bg-primary/10 text-primary border-primary/20 px-4 py-1.5 rounded-full font-black text-xs uppercase tracking-widest w-fit">
          {user?.role_name}
        </Badge>
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
                {isExpired ? "已过期" : `剩 ${daysLeft} 天`}
              </Badge>
            )}
            {isPendingEmby && (
              <Badge variant="outline" className="text-[10px] text-muted-foreground">
                未开通
              </Badge>
            )}
          </div>
          <p className="mt-4 text-[10px] font-black uppercase tracking-widest text-muted-foreground">到期倒计时</p>
          <h3 className="text-2xl sm:text-3xl font-black mt-1 break-all">
            {isPendingEmby ? "—" : isPermanent ? "∞ 永久" : `${daysLeft} 天`}
          </h3>
        </motion.div>

        <motion.div variants={item} className="premium-card p-5 sm:p-6">
          <div className="p-3 w-fit bg-purple-500/10 text-purple-500 rounded-2xl">
            <Clock className="h-5 w-5" />
          </div>
          <p className="mt-4 text-[10px] font-black uppercase tracking-widest text-muted-foreground">账号状态</p>
          <h3 className="text-2xl sm:text-3xl font-black mt-1">
            {isPendingEmby ? "未开通 Emby" : isPending ? "待开通 Emby" : isExpired ? "已过期" : "正常"}
          </h3>
        </motion.div>

        <motion.div variants={item} className="premium-card p-5 sm:p-6 sm:col-span-2 lg:col-span-1">
          <div className="p-3 w-fit bg-emerald-500/10 text-emerald-500 rounded-2xl">
            <Gift className="h-5 w-5" />
          </div>
          <p className="mt-4 text-[10px] font-black uppercase tracking-widest text-muted-foreground">Emby 绑定</p>
          <h3 className="text-2xl sm:text-3xl font-black mt-1 truncate">
            {!user?.emby_id ? "未绑定" : user?.active ? "正常" : "禁用"}
          </h3>
        </motion.div>
      </div>

      {/* 第二行: Telegram / Emby 服务器 / 我的求片 */}
      <div className="grid gap-4 lg:grid-cols-3">
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
              {telegramStatus?.bound ? "已绑定" : "未绑定"}
            </Badge>
          </div>

          <div className="min-w-0 space-y-1.5">
            {telegramStatus?.bound ? (
              <>
                <p className="text-base font-bold truncate">
                  Telegram 已绑定
                </p>
                <p className="text-xs text-muted-foreground truncate">可在个人设置中管理绑定状态</p>
                {telegramStatus.pending_rebind_request && (
                  <p className="text-xs text-amber-500">已提交换绑请求，等待管理员处理</p>
                )}
              </>
            ) : (
              <p className="text-sm text-muted-foreground">
                {telegramStatus?.force_bind
                  ? "系统要求绑定 Telegram 才能完整使用"
                  : "前往个人设置生成绑定码后即可联动"}
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
              <h3 className="text-sm font-black uppercase tracking-widest text-muted-foreground">Emby 服务器</h3>
            </div>
            <Badge
              variant={embyInfo?.online ? "success" : "destructive"}
              className="text-[10px] shrink-0"
            >
              {embyInfo?.online ? "在线" : "离线"}
            </Badge>
          </div>

          <div className="min-w-0 space-y-1.5">
            <p className="text-base font-bold truncate">
              {embyInfo?.server_name || "Emby"}
            </p>
            <p className="text-xs text-muted-foreground truncate">
              版本：{embyInfo?.version || "--"}
            </p>
            {typeof embyInfo?.active_sessions === "number" && (
              <p className="text-xs text-muted-foreground inline-flex items-center gap-1">
                <Activity className="h-3 w-3" />
                当前活跃会话：{embyInfo.active_sessions}
              </p>
            )}
            {user?.emby_id ? (
              <p className="text-xs text-muted-foreground break-all" title={user.emby_id}>
                我的 Emby：<span className="font-mono">{user.emby_id}</span>
              </p>
            ) : (
              <p className="text-xs text-amber-500">尚未绑定 Emby 账号</p>
            )}
          </div>
        </motion.div>

        {/* 求片状态 */}
        <motion.div variants={item} className="premium-card p-5 sm:p-6 flex flex-col gap-3">
          <div className="flex items-start justify-between gap-2">
            <div className="flex items-center gap-2 min-w-0">
              <div className="p-2 bg-primary/10 text-primary rounded-xl">
                <Film className="h-4 w-4" />
              </div>
              <h3 className="text-sm font-black uppercase tracking-widest text-muted-foreground">我的求片</h3>
            </div>
            <Badge variant="outline" className="text-[10px] shrink-0">
              共 {myRequests.length}
            </Badge>
          </div>

          <div className="grid grid-cols-2 gap-2 text-xs sm:grid-cols-3">
            <StatPill icon={Hourglass} tone="warning" label="待处理" value={requestStats.pending} />
            <StatPill icon={CheckCircle2} tone="primary" label="已接受" value={requestStats.accepted} />
            <StatPill icon={Download} tone="info" label="下载中" value={requestStats.downloading} />
            <StatPill icon={CheckCircle2} tone="success" label="已完成" value={requestStats.completed} />
            <StatPill icon={XCircle} tone="destructive" label="已拒绝" value={requestStats.rejected} />
          </div>

          {latestRequests.length > 0 ? (
            <div className="mt-auto space-y-1.5 pt-1 text-xs">
              <p className="text-[10px] font-black uppercase tracking-widest text-muted-foreground">最近请求</p>
              {latestRequests.map((req) => (
                <div key={req.id} className="flex items-center justify-between gap-2 min-w-0">
                  <span className="truncate">
                    {req.media_info?.title || req.title}
                    {req.media_info?.season ? ` · 第 ${req.media_info.season} 季` : ""}
                  </span>
                  <RequestStatusBadge status={req.status} />
                </div>
              ))}
            </div>
          ) : (
            <p className="text-xs text-muted-foreground mt-auto pt-1">还没有求片记录</p>
          )}
        </motion.div>
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
                  title={`我的${signinSummary.currency_name}`}
                >
                  我的{signinSummary.currency_name}
                </p>
                <div className="flex items-baseline gap-2 whitespace-nowrap">
                  <h3 className="text-2xl font-black tracking-tight">{signinSummary.current_points}</h3>
                  <span className="text-sm text-muted-foreground truncate">{signinSummary.currency_name}</span>
                </div>
                <p className="mt-0.5 text-xs text-muted-foreground inline-flex items-center gap-1">
                  <Flame className="h-3 w-3 text-orange-500" />
                  连签 {signinSummary.current_streak} 天
                  {signinSummary.next_bonus_in_days && signinSummary.next_bonus_points ? (
                    <span className="ml-1">· 再签 {signinSummary.next_bonus_in_days} 天 +{signinSummary.next_bonus_points}</span>
                  ) : null}
                </p>
              </div>
            </div>
            <div className="flex items-center gap-2 md:justify-end">
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
                {signinSummary.today_signed ? "今日已签" : "立即签到"}
              </Button>
              <Button asChild size="sm" variant="outline">
                <Link href="/score">前往签到中心</Link>
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
              <h3 className="text-base font-black tracking-tight">Emby 已到期</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                系统账号仍可登录查看信息，但 Emby 账号已被禁用，续期后会自动恢复线路访问。
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
              <h3 className="text-base font-black tracking-tight">服务器线路延迟</h3>
              <p className="text-[11px] text-muted-foreground font-bold uppercase tracking-tighter">
                Server Network Status
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
            重新测速
          </Button>
        </div>

        {lineSlots.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {isLoading ? "正在加载线路信息..." : "暂无可用线路"}
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
                      专属
                    </Badge>
                  ) : (
                    <Badge variant="outline" className="text-[10px] shrink-0">
                      公共
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
            <h3 className="text-base font-black tracking-tight">注册码/续期码/邀请码使用</h3>
            <p className="text-[11px] text-muted-foreground font-bold uppercase tracking-tighter">Code Use</p>
          </div>
        </div>

        <div className="flex flex-col gap-3 md:flex-row">
          <Input
            placeholder="请输入注册码、续期码或邀请码"
            value={regCode}
            onChange={(e) => setRegCode(e.target.value)}
            className="h-12 rounded-xl border-white/60 bg-white/40 shadow-inner"
          />
          <Button
            onClick={handleCheckRegcode}
            disabled={isLoading || isUsingCode}
            className="h-12 rounded-xl font-black md:w-auto w-full"
          >
            {isUsingCode ? <Loader2 className="h-4 w-4 animate-spin" /> : "验证并使用"}
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
                {hasGrantedEmbyRegisterEntitlement ? "你已获得 Emby 注册权利" : "当前已开启 Emby 自由注册"}
              </h3>
              <p className="text-[11px] text-muted-foreground font-bold uppercase tracking-tighter">
                Direct Emby Registration
              </p>
            </div>
          </div>

          <div className="space-y-3 text-sm">
            <p className="text-muted-foreground">
              {hasGrantedEmbyRegisterEntitlement
                ? "管理员已为你清理注册队列并授予补建 Emby 资格。请点击下方按钮填写 Emby 用户名和密码完成开通。"
                : directRegisterBlockedReason
                ? directRegisterBlockedReason
                : "当前已开启自由注册，点击按钮填写 Emby 用户名和密码即可开通。"}
            </p>

            <Button
              className="rounded-xl font-black"
              disabled={Boolean(directRegisterBlockedReason)}
              onClick={() => {
                setShowDirectRegisterDialog(true);
              }}
            >
              <UserPlus className="mr-2 h-4 w-4" />
              立即开通 Emby
            </Button>
          </div>
        </motion.div>
      )}

      {embyRegisterStored && embyRegisterStatus && (
        <motion.div variants={item} className="premium-card p-5 sm:p-6 border-emerald-500/20 bg-emerald-500/5">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <h3 className="text-base font-black tracking-tight">Emby 注册队列状态</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                {embyRegisterStatus.status === "queued"
                  ? `排队中${embyRegisterStatus.queue_position ? `（第 ${embyRegisterStatus.queue_position} 位）` : ""}`
                  : embyRegisterStatus.status === "processing"
                    ? "正在创建 Emby 账号"
                    : embyRegisterStatus.status === "success"
                      ? "注册完成，状态将保留 15 分钟"
                      : embyRegisterStatus.message || "注册失败"}
              </p>
              {embyRegisterStatus.message && (
                <p className="mt-1 text-xs text-muted-foreground">{embyRegisterStatus.message}</p>
              )}
            </div>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={() => void refreshStoredEmbyRegisterStatus()}>
                <RefreshCw className="mr-2 h-4 w-4" />
                刷新
              </Button>
              {(["success", "failed", "rejected"] as const).includes(embyRegisterStatus.status as any) && (
                <Button variant="ghost" size="sm" onClick={clearEmbyRegisterRequest}>
                  关闭
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
            <DialogTitle>开通 Emby 账号</DialogTitle>
          </DialogHeader>

          <div className="space-y-4 py-2 text-sm">
            <div className="space-y-2">
              <Label className="text-xs uppercase tracking-wider text-muted-foreground">开通时长</Label>
              <p className="rounded-md border bg-muted/40 px-3 py-2 text-sm font-medium">
                {directRegisterDaysLabel}
                <span className="ml-2 text-xs font-normal text-muted-foreground">
                  （由管理员统一配置，本次开通后将自动生效）
                </span>
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="directEmbyUser">Emby 用户名</Label>
              <Input
                id="directEmbyUser"
                value={directEmbyUsername}
                onChange={(e) => setDirectEmbyUsername(e.target.value)}
                placeholder="3-20 位字母数字下划线，不能以数字开头"
                disabled={isSubmittingDirectRegister}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="directEmbyPwd">Emby 密码</Label>
              <div className="relative">
                <Input
                  id="directEmbyPwd"
                  type={showDirectEmbyPassword ? "text" : "password"}
                  value={directEmbyPassword}
                  onChange={(e) => setDirectEmbyPassword(e.target.value)}
                  placeholder="至少 8 位，含大小写字母和数字"
                  className="pr-10"
                  disabled={isSubmittingDirectRegister}
                />
                <button
                  type="button"
                  onClick={() => setShowDirectEmbyPassword((v) => !v)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
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
            <Button variant="outline" onClick={() => setShowDirectRegisterDialog(false)}>取消</Button>
            <Button
              onClick={handleSubmitDirectRegister}
              disabled={isSubmittingDirectRegister || Boolean(directRegisterBlockedReason)}
            >
              {isSubmittingDirectRegister && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认开通
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={showConfirm} onOpenChange={setShowConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{regCodeInfo?.confirm_title || "确认使用卡码"}</DialogTitle>
          </DialogHeader>
          {regCodeInfo && (
            <div className="space-y-2 text-sm text-muted-foreground">
              <p>类型: {regCodeInfo.type_name}</p>
              {regCodeInfo.source === "invite" && regCodeInfo.inviter && (
                <p>邀请人: {regCodeInfo.inviter}</p>
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
                <Label htmlFor="embyUsername">Emby 用户名</Label>
                <Input
                  id="embyUsername"
                  value={embyUsername}
                  onChange={(e) => setEmbyUsername(e.target.value)}
                  placeholder="请输入 Emby 用户名"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="embyPassword">Emby 密码</Label>
                <div className="relative">
                  <Input
                    id="embyPassword"
                    type={showEmbyPassword ? "text" : "password"}
                    value={embyPassword}
                    onChange={(e) => setEmbyPassword(e.target.value)}
                    placeholder="至少8位，含大小写字母和数字"
                    className="pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowEmbyPassword((v) => !v)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  >
                    {showEmbyPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
              </div>
            </div>
          )}
          <div className="flex gap-3 justify-end">
            <Button variant="outline" onClick={() => setShowConfirm(false)}>取消</Button>
            <Button onClick={handleUseRegcode} disabled={isUsingCode}>
              {isUsingCode ? <Loader2 className="h-4 w-4 animate-spin" /> : regCodeInfo?.submit_label || "确认"}
            </Button>
          </div>
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
  const s = (status || "").toUpperCase();
  if (s === "UNHANDLED" || s === "PENDING") {
    return <Badge variant="warning" className="shrink-0 text-[10px]">待处理</Badge>;
  }
  if (s === "ACCEPTED") {
    return <Badge variant="outline" className="shrink-0 text-[10px] border-primary/40 text-primary">已接受</Badge>;
  }
  if (s === "DOWNLOADING") {
    return (
      <Badge variant="outline" className="shrink-0 text-[10px] border-sky-500/40 text-sky-600 dark:text-sky-400">
        下载中
      </Badge>
    );
  }
  if (s === "COMPLETED") {
    return <Badge variant="success" className="shrink-0 text-[10px]">已完成</Badge>;
  }
  if (s === "REJECTED") {
    return <Badge variant="destructive" className="shrink-0 text-[10px]">已拒绝</Badge>;
  }
  return <Badge variant="secondary" className="shrink-0 text-[10px]">{status}</Badge>;
}
