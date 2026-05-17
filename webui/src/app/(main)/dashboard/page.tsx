"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { motion } from "framer-motion";
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
  Bot,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Hourglass,
  Download,
  Globe,
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
import { api, type EmbyInfo, type MediaRequest, type TelegramStatus } from "@/lib/api";
import { AnnouncementBoard } from "@/components/announcement-board";

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

export default function DashboardPage() {
  const { user, fetchUser } = useAuthStore();
  const { info: systemInfo, fetchInfo: fetchSystemInfo } = useSystemStore();
  const { toast } = useToast();
  const [regCode, setRegCode] = useState("");
  const [regCodeInfo, setRegCodeInfo] = useState<{ type: number; type_name: string; days: number } | null>(null);
  const [showConfirm, setShowConfirm] = useState(false);
  const [isUsingCode, setIsUsingCode] = useState(false);
  const [embyUsername, setEmbyUsername] = useState("");
  const [embyPassword, setEmbyPassword] = useState("");
  const [showEmbyPassword, setShowEmbyPassword] = useState(false);

  const [telegramStatus, setTelegramStatus] = useState<TelegramStatus | null>(null);
  const [embyInfo, setEmbyInfo] = useState<EmbyInfo | null>(null);
  const [myRequests, setMyRequests] = useState<MediaRequest[]>([]);
  const [lineSlots, setLineSlots] = useState<LineSlot[]>([]);
  const [linesRequireEmby, setLinesRequireEmby] = useState(false);
  const [lineLatencyMap, setLineLatencyMap] = useState<Record<string, LineLatencyInfo>>({});
  const [isLatencyTesting, setIsLatencyTesting] = useState(false);

  useEffect(() => {
    void fetchSystemInfo();
  }, [fetchSystemInfo]);

  const loadDashboardData = useCallback(async (signal?: AbortSignal) => {
    const [tgRes, embyRes, urlsRes, reqRes] = await Promise.all([
      api.getTelegramStatus().catch(() => null),
      api.getEmbyInfo().catch(() => null),
      api.getEmbyUrls().catch(() => null),
      api.getMyRequests(signal).catch(() => null),
    ]);

    if (tgRes && tgRes.success && tgRes.data) {
      setTelegramStatus(tgRes.data);
    }
    if (embyRes && embyRes.success && embyRes.data) {
      setEmbyInfo(embyRes.data);
    }
    if (urlsRes && urlsRes.success && urlsRes.data) {
      const lines = (urlsRes.data.lines || []).map((line, index) => ({
        key: `line:${index}:${line.url}`,
        name: line.name || `线路 ${index + 1}`,
        url: line.url,
        scope: "line" as const,
      }));
      const wl = (urlsRes.data.whitelist_lines || []).map((line, index) => ({
        key: `wl:${index}:${line.url}`,
        name: line.name || `专属线路 ${index + 1}`,
        url: line.url,
        scope: "wl" as const,
      }));
      setLineSlots([...lines, ...wl]);
      setLinesRequireEmby(Boolean(urlsRes.data.requires_emby_account));
    }
    if (reqRes && reqRes.success && Array.isArray(reqRes.data)) {
      setMyRequests(reqRes.data);
    }
    return true;
  }, []);

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

  let expiredTimestamp: number | null = null;
  if (user?.expired_at) {
    if (typeof user.expired_at === "number") {
      if (user.expired_at !== -1) {
        expiredTimestamp = user.expired_at < 10000000000 ? user.expired_at * 1000 : user.expired_at;
      }
    } else if (typeof user.expired_at === "string" && user.expired_at !== "-1") {
      const parsed = new Date(user.expired_at).getTime();
      expiredTimestamp = Number.isNaN(parsed) ? null : parsed;
    }
  }

  const isPermanent = isAdmin || expiredTimestamp === null;
  const safeExpiredTimestamp = expiredTimestamp ?? 0;
  const isExpired = !isPermanent && safeExpiredTimestamp < Date.now();
  const daysLeft = !isPermanent ? Math.max(0, Math.ceil((safeExpiredTimestamp - Date.now()) / (1000 * 60 * 60 * 24))) : 0;

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

  // ============== Regcode 流程 ==============
  const handleCheckRegcode = async () => {
    if (!regCode.trim()) {
      toast({ title: "请输入注册码/续期码", variant: "destructive" });
      return;
    }
    try {
      const res = await api.checkRegcode(regCode.trim());
      if (res.success && res.data) {
        setRegCodeInfo(res.data);
        setEmbyUsername("");
        setEmbyPassword("");
        setShowEmbyPassword(false);
        setShowConfirm(true);
      } else {
        toast({ title: "注册码/续期码无效", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "检查失败", description: err.message || "网络异常", variant: "destructive" });
    }
  };

  const handleUseRegcode = async () => {
    if (!regCodeInfo || !regCode.trim()) return;
    const requiresEmbyRegister = !user?.emby_id && (regCodeInfo.type === 1 || regCodeInfo.type === 3);
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
        toast({ title: "注册码/续期码使用成功", description: regCodeInfo.type_name, variant: "success" });
        setRegCode("");
        setRegCodeInfo(null);
        setShowConfirm(false);
        setEmbyUsername("");
        setEmbyPassword("");
        await fetchUser();
      } else {
        toast({ title: "使用失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "使用失败", description: err.message || "网络异常", variant: "destructive" });
    } finally {
      setIsUsingCode(false);
    }
  };

  if (error) {
    return <PageError message={error} />;
  }

  const botUsername = systemInfo?.telegram_bot?.username || null;
  const botUrl =
    systemInfo?.telegram_bot?.url ||
    (botUsername ? `https://t.me/${botUsername}` : null);

  return (
    <motion.div variants={container} initial="hidden" animate="show" className="space-y-6 pb-10">
      <div className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div className="min-w-0">
          <h1 className="text-3xl font-black tracking-tighter sm:text-4xl">
            {getGreeting()}，{user?.username}
          </h1>
          <p className="text-muted-foreground font-medium mt-1 text-sm sm:text-base">
            {isPending ? "当前账号可登录，若需媒体服务请联系管理员开通 Emby 账号" : "欢迎回来，当前账号状态正常"}
          </p>
        </div>
        <Badge className="bg-primary/10 text-primary border-primary/20 px-4 py-1.5 rounded-full font-black text-xs uppercase tracking-widest w-fit">
          {user?.role_name}
        </Badge>
      </div>

      <motion.div variants={item}>
        <AnnouncementBoard splitPinned />
      </motion.div>

      {/* 顶部三块: 到期 / 状态 / Emby 绑定 */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <motion.div variants={item} className="premium-card p-5 sm:p-6">
          <div className="flex items-start justify-between gap-2">
            <div className="p-3 w-fit bg-amber-500/10 text-amber-500 rounded-2xl">
              <Calendar className="h-5 w-5" />
            </div>
            {!isPermanent && (
              <Badge variant={isExpired ? "destructive" : daysLeft <= 7 ? "warning" : "outline"} className="text-[10px]">
                {isExpired ? "已过期" : `剩 ${daysLeft} 天`}
              </Badge>
            )}
          </div>
          <p className="mt-4 text-[10px] font-black uppercase tracking-widest text-muted-foreground">到期倒计时</p>
          <h3 className="text-2xl sm:text-3xl font-black mt-1 break-all">
            {isPermanent ? "∞ 永久" : `${daysLeft} 天`}
          </h3>
        </motion.div>

        <motion.div variants={item} className="premium-card p-5 sm:p-6">
          <div className="p-3 w-fit bg-purple-500/10 text-purple-500 rounded-2xl">
            <Clock className="h-5 w-5" />
          </div>
          <p className="mt-4 text-[10px] font-black uppercase tracking-widest text-muted-foreground">账号状态</p>
          <h3 className="text-2xl sm:text-3xl font-black mt-1">
            {isPending ? "待开通 Emby" : isExpired ? "已过期" : "正常"}
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
                  {telegramStatus.telegram_username ? `@${telegramStatus.telegram_username}` : `ID ${telegramStatus.telegram_id}`}
                </p>
                {telegramStatus.telegram_id && telegramStatus.telegram_username && (
                  <p className="text-xs text-muted-foreground truncate">ID: {telegramStatus.telegram_id}</p>
                )}
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

          {botUsername && botUrl && (
            <div className="mt-auto flex flex-wrap items-center gap-2 pt-2">
              <Badge variant="outline" className="gap-1 truncate max-w-full">
                <Bot className="h-3 w-3 shrink-0" />
                <span className="truncate">@{botUsername}</span>
              </Badge>
              <Button asChild size="sm" variant="outline" className="ml-auto">
                <a href={botUrl} target="_blank" rel="noopener noreferrer">
                  打开 Bot
                </a>
              </Button>
            </div>
          )}
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
              <p className="text-xs text-muted-foreground break-all" title={user.emby_username}>
                我的 Emby：<span className="font-mono">{user.emby_username}</span>
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

      {/* 线路延迟 —— 仅在用户已绑定 Emby 时显示，避免预先泄露线路 */}
      {!linesRequireEmby && (
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

      {/* 注册码/续期码 */}
      <motion.div variants={item} className="premium-card p-5 sm:p-6">
        <div className="flex items-center gap-3 mb-5">
          <div className="p-2 bg-primary/10 rounded-xl text-primary">
            <Key className="h-5 w-5" />
          </div>
          <div>
            <h3 className="text-base font-black tracking-tight">注册码/续期码使用</h3>
            <p className="text-[11px] text-muted-foreground font-bold uppercase tracking-tighter">Code Use</p>
          </div>
        </div>

        <div className="flex flex-col gap-3 md:flex-row">
          <Input
            placeholder="请输入注册码或续期码"
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

      <Dialog open={showConfirm} onOpenChange={setShowConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认使用注册码/续期码</DialogTitle>
          </DialogHeader>
          {regCodeInfo && (
            <div className="space-y-2 text-sm text-muted-foreground">
              <p>类型: {regCodeInfo.type_name}</p>
              <p>
                {regCodeInfo.type === 2
                  ? regCodeInfo.days <= 0
                    ? "续期时长: 永久"
                    : `续期时长: ${regCodeInfo.days} 天`
                  : regCodeInfo.days <= 0
                    ? "有效期: 永久"
                    : `增加时长: ${regCodeInfo.days} 天`}
              </p>
            </div>
          )}
          {regCodeInfo && !user?.emby_id && (regCodeInfo.type === 1 || regCodeInfo.type === 3) && (
            <div className="space-y-3 rounded-lg border border-border p-3">
              <p className="text-sm font-medium">该卡码将创建 Emby 账号，请填写以下信息</p>
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
              {isUsingCode ? <Loader2 className="h-4 w-4 animate-spin" /> : "确认"}
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
