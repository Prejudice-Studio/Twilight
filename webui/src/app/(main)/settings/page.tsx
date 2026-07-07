"use client";

import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { motion } from "framer-motion";
import {
  User,
  RefreshCw,
  Copy,
  Eye,
  EyeOff,
  MessageCircle,
  Link as LinkIcon,
  Unlink,
  Loader2,
  Check,
  X,
  Tv,
  Key,
  Palette,
  Lock,
  Globe,
  Star,
  Bot,
  AlertCircle,
  Bell,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
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
import { useToast } from "@/hooks/use-toast";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { useBindCodeStatus } from "@/hooks/use-bind-code-status";
import { PageError, PageLoading } from "@/components/layout/page-state";
import { useAuthStore } from "@/store/auth";
import { useSystemStore } from "@/store/system";
import { api, type UserSettings, type TelegramStatus, type EmbyStatus } from "@/lib/api";
import { ApiError } from "@/lib/api-request";
import { ErrCodes } from "@/lib/errcode";
import { localeLabels, supportedLocales, useI18n, type Locale } from "@/lib/i18n";
import { passwordStrengthLabel, validatePasswordStrength } from "@/lib/password";
import { friendlyError } from "@/lib/validators";
import { EmailCodeInput } from "@/components/email-code-input";
import { telegramBotUrl } from "@/lib/safe-url";

const container = {
  hidden: { opacity: 0 },
  show: { opacity: 1, transition: { staggerChildren: 0.1 } },
};

const item = {
  hidden: { opacity: 0, y: 20 },
  show: { opacity: 1, y: 0 },
};

export default function SettingsPage() {
  const { toast } = useToast();
  const { locale, setLocale, t } = useI18n();
  const { user, fetchUser } = useAuthStore();
  const { info: systemInfo, fetchInfo: fetchSystemInfo } = useSystemStore();
  const [settings, setSettings] = useState<UserSettings | null>(null);
  const [telegramStatus, setTelegramStatus] = useState<TelegramStatus | null>(null);
  const [bgmTokenSet, setBgmTokenSet] = useState(false);
  const [bgmMode, setBgmMode] = useState(false);
  const [bgmManageMode, setBgmManageMode] = useState(false);
  const [bgmToken, setBgmToken] = useState("");
  const [isBgmLoading, setIsBgmLoading] = useState(false);
  const [embyStatus, setEmbyStatus] = useState<EmbyStatus | null>(null);
  const [notifyLoginTelegram, setNotifyLoginTelegram] = useState(false);
  const [notifyLoginEmail, setNotifyLoginEmail] = useState(false);
  const [notifyTicketTelegram, setNotifyTicketTelegram] = useState(false);

  // 仅首次挂载播放进场动画；切语言不再重播
  const [hasAppeared, setHasAppeared] = useState(false);
  useEffect(() => { setHasAppeared(true); }, []);

  // Telegram bind code
  const [bindCode, setBindCode] = useState<string | null>(null);
  const [bindCodeExpiry, setBindCodeExpiry] = useState<number>(0);
  const [isTgLoading, setIsTgLoading] = useState(false);
  const bindCodeRequestId = useRef(0);
  const botUsername = systemInfo?.telegram_bot?.username;
  const botUrl = telegramBotUrl(systemInfo?.telegram_bot?.username, systemInfo?.telegram_bot?.url);
  const [isRebindLoading, setIsRebindLoading] = useState(false);
  const [rebindDialogOpen, setRebindDialogOpen] = useState(false);
  const [rebindReason, setRebindReason] = useState("");

  // Emby dialogs
  const [bindEmbyOpen, setBindEmbyOpen] = useState(false);
  const [completePendingEmby, setCompletePendingEmby] = useState(false);
  const [embyUsername, setEmbyUsername] = useState("");
  const [embyPassword, setEmbyPassword] = useState("");
  const [showEmbyPassword, setShowEmbyPassword] = useState(false);
  const [isEmbyLoading, setIsEmbyLoading] = useState(false);
  const hasEmbyRegistrationEntitlement =
    Boolean(user?.pending_emby) &&
    !user?.emby_id &&
    user?.pending_emby_days !== null &&
    user?.pending_emby_days !== undefined;
  const canUnbindEmby =
    embyStatus?.can_unbind ?? !user?.emby_grant_locked;

  // Email dialog
  const [editEmailOpen, setEditEmailOpen] = useState(false);
  const [emailValue, setEmailValue] = useState("");
  const [isEmailLoading, setIsEmailLoading] = useState(false);

  // Password change
  const [changeSystemPwdOpen, setChangeSystemPwdOpen] = useState(false);
  const [changeEmbyPwdOpen, setChangeEmbyPwdOpen] = useState(false);

  const [oldPassword, setOldPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [showOldPwd, setShowOldPwd] = useState(false);
  const [showNewPwd, setShowNewPwd] = useState(false);
  const [isSystemPwdLoading, setIsSystemPwdLoading] = useState(false);

  const [newEmbyPassword, setNewEmbyPassword] = useState("");
  const [confirmEmbyPassword, setConfirmEmbyPassword] = useState("");
  const [showNewEmbyPwd, setShowNewEmbyPwd] = useState(false);
  const [showConfirmEmbyPwd, setShowConfirmEmbyPwd] = useState(false);
  const [isEmbyPwdLoading, setIsEmbyPwdLoading] = useState(false);

  // 邮箱验证 / 强制改密
  const emailEnabled = Boolean(systemInfo?.features?.email_enabled);
  const forceBindEmail = Boolean(systemInfo?.features?.force_bind_email);
  const emailVerified = Boolean(user?.email_verified);
  const hasTelegramBinding = Boolean(telegramStatus?.bound);
  const hasVerifiedEmail = emailVerified;
  // 普通/白名单且开启强制绑定时，改密需要邮箱验证码（管理员不强制）。
  const passwordEmailGate = emailEnabled && forceBindEmail && user?.role !== 0;
  const [emailBindStage, setEmailBindStage] = useState<"email" | "code">("email");
  const [emailBindVerifId, setEmailBindVerifId] = useState("");
  const [emailBindCode, setEmailBindCode] = useState("");
  const [sysPwdVerifId, setSysPwdVerifId] = useState("");
  const [sysPwdCode, setSysPwdCode] = useState("");
  const [embyPwdVerifId, setEmbyPwdVerifId] = useState("");
  const [embyPwdCode, setEmbyPwdCode] = useState("");

  const resetEmailBind = () => {
    setEmailBindStage("email");
    setEmailBindVerifId("");
    setEmailBindCode("");
  };

  const systemPwdStrength = useMemo(
    () => validatePasswordStrength(newPassword),
    [newPassword]
  );
  const embyPwdStrength = useMemo(
    () => validatePasswordStrength(newEmbyPassword),
    [newEmbyPassword]
  );
  const bangumiSyncEnabled = Boolean(settings?.system_config?.bangumi_sync_enabled);
  const bangumiManageEnabled = Boolean(settings?.system_config?.bangumi_manage_enabled);
  const bangumiAnyEnabled = bangumiSyncEnabled || bangumiManageEnabled;

  // Emby URLs
  const [embyLines, setEmbyLines] = useState<Array<{ name: string; url: string }>>([]);
  const [whitelistLines, setWhitelistLines] = useState<Array<{ name: string; url: string }>>([]);
  const [linesRequireEmby, setLinesRequireEmby] = useState(false);
  const [linesRequireRenewal, setLinesRequireRenewal] = useState(false);
  const [copiedIndex, setCopiedIndex] = useState<string | null>(null);
  const [lineLatencyMap, setLineLatencyMap] = useState<Record<string, { status: "idle" | "testing" | "ok" | "timeout" | "error"; latencyMs?: number }>>({});
  const [isLatencyTesting, setIsLatencyTesting] = useState(false);

  const makeLineKey = useCallback((scope: "line" | "wl", index: number, url: string) => `${scope}:${index}:${url}`, []);

  const embyLineItems = useMemo(
    () => embyLines.map((line, index) => ({ line, index, key: makeLineKey("line", index, line.url) })),
    [embyLines, makeLineKey]
  );

  const whitelistLineItems = useMemo(
    () => whitelistLines.map((line, index) => ({ line, index, key: makeLineKey("wl", index, line.url) })),
    [whitelistLines, makeLineKey]
  );

  const getLatencyRank = useCallback((key: string) => {
    const info = lineLatencyMap[key];
    if (!info) return 8_000_000;
    if (info.status === "ok") return info.latencyMs ?? 8_000_000;
    if (info.status === "testing") return 9_000_000;
    if (info.status === "timeout") return 10_000_000;
    if (info.status === "error") return 11_000_000;
    return 12_000_000;
  }, [lineLatencyMap]);

  const sortedEmbyLineItems = useMemo(
    () => [...embyLineItems].sort((a, b) => getLatencyRank(a.key) - getLatencyRank(b.key)),
    [embyLineItems, getLatencyRank]
  );

  const sortedWhitelistLineItems = useMemo(
    () => [...whitelistLineItems].sort((a, b) => getLatencyRank(a.key) - getLatencyRank(b.key)),
    [whitelistLineItems, getLatencyRank]
  );

  const renderLatencyText = (key: string) => {
    const info = lineLatencyMap[key];
    if (!info || info.status === "idle") return t("settings.latencyIdle");
    if (info.status === "testing") return t("settings.latencyTesting");
    if (info.status === "ok") return `${info.latencyMs} ms`;
    if (info.status === "timeout") return t("settings.latencyTimeout");
    return t("settings.latencyUnreachable");
  };

  const renderLatencyClassName = (key: string) => {
    const info = lineLatencyMap[key];
    if (!info || info.status === "idle" || info.status === "testing") {
      return "text-xs text-muted-foreground";
    }
    if (info.status === "ok") {
      if ((info.latencyMs ?? 0) <= 150) return "text-xs text-emerald-600";
      if ((info.latencyMs ?? 0) <= 400) return "text-xs text-amber-600";
      return "text-xs text-orange-600";
    }
    return "text-xs text-destructive";
  };

  // 由后端代理测速，避开浏览器 CORS / 私网混合内容拦截。
  const testSingleLineLatency = useCallback(async (rawUrl: string) => {
    const url = rawUrl.trim();
    if (!url) {
      return { status: "error" as const };
    }
    try {
      const res = await api.probeEmbyUrl(url);
      if (res.success && res.data) {
        if (res.data.status === "ok") {
          return { status: "ok" as const, latencyMs: Math.max(1, Math.round(res.data.latency_ms || 0)) };
        }
        if (res.data.status === "timeout") {
          return { status: "timeout" as const };
        }
        return { status: "error" as const };
      }
      return { status: "error" as const };
    } catch {
      return { status: "error" as const };
    }
  }, []);

  const runLineLatencyTests = useCallback(async () => {
    const allItems = [...embyLineItems, ...whitelistLineItems];
    if (allItems.length === 0) {
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
      const offlineMap: Record<string, { status: "error" }> = {};
      for (const item of allItems) offlineMap[item.key] = { status: "error" };
      setLineLatencyMap(offlineMap);
      setIsLatencyTesting(false);
      return;
    }
    setLineLatencyMap((prev) => {
      const next = { ...prev };
      for (const item of allItems) {
        next[item.key] = { status: "testing" };
      }
      return next;
    });

    const entries = await Promise.all(
      allItems.map(async (item) => [item.key, await testSingleLineLatency(item.line.url)] as const)
    );

    setLineLatencyMap((prev) => {
      const next = { ...prev };
      for (const [key, result] of entries) {
        next[key] = result;
      }
      return next;
    });
    setIsLatencyTesting(false);
  }, [embyLineItems, whitelistLineItems, testSingleLineLatency]);

  const loadEmbyUrls = useCallback(async () => {
    const res = await api.getEmbyUrls();
    if (res.success && res.data) {
      setEmbyLines(res.data.lines || []);
      setWhitelistLines(res.data.whitelist_lines || []);
      setLinesRequireEmby(Boolean(res.data.requires_emby_account));
      setLinesRequireRenewal(Boolean(res.data.requires_renewal || res.data.emby_disabled_by_expiry));
    }
  }, []);

  const loadSettingsResource = useCallback(async () => {
    const settingsRes = await api.getMySettings();
    if (settingsRes.success && settingsRes.data) {
      setSettings(settingsRes.data);
      setBgmMode(settingsRes.data.bgm_mode);
      setBgmManageMode(Boolean(settingsRes.data.bgm_manage_mode));
      setBgmTokenSet(settingsRes.data.bgm_token_set ?? false);
      setEmbyStatus(settingsRes.data.emby_status ?? null);
      setTelegramStatus(settingsRes.data.telegram as TelegramStatus);
      setNotifyLoginTelegram(Boolean(settingsRes.data.notify_on_login_telegram));
      setNotifyLoginEmail(Boolean(settingsRes.data.notify_on_login_email));
      setNotifyTicketTelegram(Boolean(settingsRes.data.notify_on_ticket_telegram));
    }
    return true;
  }, []);

  const {
    isLoading,
    error,
    execute: loadData,
  } = useAsyncResource(loadSettingsResource, { immediate: true });

  // 生成绑定码后自动轮询，不再要求用户手动刷新页面（刷新会丢掉 bindCode 这块
  // React 状态、销毁整个绑定流程）。带超时中断 + 请求中断，见 useBindCodeStatus。
  useBindCodeStatus({
    code: bindCode,
    scene: "user",
    expiresIn: bindCodeExpiry,
    enabled: Boolean(bindCode) && !telegramStatus?.bound,
    onBound: () => {
      setBindCode(null);
      setBindCodeExpiry(0);
      toast({ title: t("settings.rebindCompleteTitle"), variant: "success" });
      void loadData();
      void fetchUser();
    },
    onTerminalError: (data) => {
      setBindCode(null);
      setBindCodeExpiry(0);
      toast({ title: t("settings.getBindCodeFailed"), description: data.message, variant: "destructive" });
    },
    // 绑定码 TTL 到期仍未确认：静默清掉过期码（与旧的本地过期计时器行为一致）。
    onTimeout: () => {
      setBindCode(null);
      setBindCodeExpiry(0);
    },
  });

  const handleSaveBgmSettings = async () => {
    const nextBgmToken = bgmToken.trim();
    if (!bangumiAnyEnabled) {
      toast({ title: t("settings.bangumiDisabledTitle"), description: t("settings.bangumiDisabledDescription"), variant: "destructive" });
      return;
    }
    if ((bgmMode || bgmManageMode) && !nextBgmToken && !bgmTokenSet) {
      toast({ title: t("settings.bangumiTokenRequiredTitle"), description: t("settings.bangumiTokenRequiredDescription"), variant: "destructive" });
      return;
    }

    setIsBgmLoading(true);
    try {
      const payload: Parameters<typeof api.updateMySettings>[0] = {
        bgm_token: nextBgmToken || undefined,
      };
      if (bangumiSyncEnabled) payload.bgm_mode = bgmMode;
      if (bangumiManageEnabled) payload.bgm_manage_mode = bgmManageMode;
      const res = await api.updateMySettings(payload);

      if (res.success) {
        const nextTokenSet = bgmTokenSet || Boolean(nextBgmToken);
        setBgmToken("");
        setBgmTokenSet(nextTokenSet);
        setSettings((prev) => prev ? { ...prev, bgm_mode: bgmMode, bgm_manage_mode: bgmManageMode, bgm_token_set: nextTokenSet } : prev);
        toast({ title: t("adminConfig.saveSuccessTitle"), description: t("settings.bangumiSavedDescription"), variant: "success" });
      } else {
        toast({ title: t("adminConfig.saveFailureTitle"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminConfig.saveFailureTitle"), description: error.message, variant: "destructive" });
    } finally {
      setIsBgmLoading(false);
    }
  };

  const handleClearBgmToken = async () => {
    if (!bgmTokenSet) return;

    setIsBgmLoading(true);
    try {
      const res = await api.updateMySettings({
        bgm_mode: false,
        bgm_manage_mode: false,
        bgm_token: "",
      });

      if (res.success) {
        setBgmMode(false);
        setBgmManageMode(false);
        setBgmToken("");
        setBgmTokenSet(false);
        setSettings((prev) => prev ? { ...prev, bgm_mode: false, bgm_manage_mode: false, bgm_token_set: false } : prev);
        toast({ title: t("settings.bangumiClearedTitle"), description: t("settings.bangumiClearedDescription"), variant: "success" });
      } else {
        toast({ title: t("settings.clearFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("settings.clearFailed"), description: error.message, variant: "destructive" });
    } finally {
      setIsBgmLoading(false);
    }
  };

  const handleGetBindCode = async () => {
    const requestId = ++bindCodeRequestId.current;
    setIsTgLoading(true);
    try {
      const res = await api.getBindCode();
      if (requestId !== bindCodeRequestId.current) return;
      if (res.success && res.data?.bind_code) {
        setBindCode(res.data.bind_code);
        setBindCodeExpiry(res.data.expires_in);
        toast({
          title: t("settings.bindCodeGenerated"),
          description: t("settings.bindCodeToastDescription", { minutes: Math.floor(res.data.expires_in / 60), code: res.data.bind_code }),
          variant: "success",
        });
      } else {
        toast({ title: t("settings.getBindCodeFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      if (requestId !== bindCodeRequestId.current) return;
      if (error instanceof ApiError && error.errorCode === ErrCodes.BindCodeRateLimited) {
        toast({ title: t("settings.getBindCodeFailed"), description: t("settings.bindCodeRateLimited"), variant: "destructive" });
        return;
      }
      toast({ title: t("settings.getBindCodeFailed"), description: error.message, variant: "destructive" });
    } finally {
      if (requestId === bindCodeRequestId.current) {
        setIsTgLoading(false);
      }
    }
  };

  const handleRequestTelegramRebind = async () => {
    if (rebindReason.length > 500) {
      toast({ title: t("settings.noteTooLong"), description: t("settings.max500Chars"), variant: "destructive" });
      return;
    }
    setIsRebindLoading(true);
    try {
      const res = await api.requestTelegramRebind(rebindReason.trim() || undefined);
      if (res.success) {
        toast({ title: t("settings.rebindSubmitSuccess"), description: res.message, variant: "success" });
        setRebindDialogOpen(false);
        setRebindReason("");
        loadData();
      } else {
        toast({ title: t("settings.rebindSubmitFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("settings.rebindSubmitFailed"), description: error.message, variant: "destructive" });
    } finally {
      setIsRebindLoading(false);
    }
  };

  const handleUnbindTelegram = async () => {
    setIsTgLoading(true);
    try {
      const res = await api.unbindTelegram();
      if (res.success) {
        toast({ title: t("settings.unbindSuccess"), variant: "success" });
        setBindCode(null);
        setBindCodeExpiry(0);
        loadData();
        fetchUser();
      } else {
        toast({ title: t("settings.unbindFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      if (error instanceof ApiError && error.errorCode === ErrCodes.TGUnbindForbidden) {
        toast({ title: t("settings.unbindFailed"), description: t("settings.telegramRequired"), variant: "destructive" });
        return;
      }
      toast({ title: t("settings.unbindFailed"), description: error.message, variant: "destructive" });
    } finally {
      setIsTgLoading(false);
    }
  };

  const handleBindEmby = async () => {
    const username = embyUsername.trim();
    // Emby 允许空密码账号，这里保留原样（不再 trim 密码两端，但用户输入框已限制空白）
    const password = embyPassword;

    if (!username) {
      toast({ title: t("settings.embyUsernameRequired"), variant: "destructive" });
      return;
    }

    setIsEmbyLoading(true);
    try {
      const res = completePendingEmby
        ? await api.completeEmbyRegistration(username, password)
        : await api.bindEmbyAccount(username, password);
      if (res.success) {
        toast({ title: completePendingEmby ? t("settings.embyOpenedTitle") : t("settings.bindSuccess"), variant: "success" });
        setBindEmbyOpen(false);
        setCompletePendingEmby(false);
        setEmbyUsername("");
        setEmbyPassword("");
        await loadData();
        await fetchUser();
        await loadEmbyUrls();
      } else {
        toast({ title: completePendingEmby ? t("settings.openFailed") : t("settings.bindFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      console.error("bind failed:", error);
      toast({ title: completePendingEmby ? t("settings.openFailed") : t("settings.bindFailed"), description: error.message, variant: "destructive" });
    } finally {
      setIsEmbyLoading(false);
    }
  };

  const handleUnbindEmby = async () => {
    setIsEmbyLoading(true);
    try {
      const res = await api.unbindEmbyAccount();
      if (res.success) {
        toast({
          title: t("settings.unbindSuccess"),
          description: res.data?.remote_emby_disabled ? t("settings.remoteEmbyDisabledUnbound") : t("settings.remoteEmbyNotFoundUnbound"),
          variant: "success",
        });
        await loadData();
        await fetchUser();
        await loadEmbyUrls();
      } else {
        toast({ title: t("settings.unbindFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("settings.unbindFailed"), description: error.message, variant: "destructive" });
    } finally {
      setIsEmbyLoading(false);
    }
  };

  const handleUpdateEmail = async () => {
    if (!emailValue) {
      toast({ title: t("settings.emailRequired"), variant: "destructive" });
      return;
    }

    setIsEmailLoading(true);
    try {
      const res = await api.updateMe({ email: emailValue });
      if (res.success) {
        toast({ title: t("settings.emailUpdated"), variant: "success" });
        setEditEmailOpen(false);
        fetchUser();
      } else {
        toast({ title: t("common.updateFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("common.updateFailed"), description: error.message, variant: "destructive" });
    } finally {
      setIsEmailLoading(false);
    }
  };

  // 邮箱启用时，绑定/换绑走「发码 → 验证」，验证通过即标记已验证。
  const handleVerifyBindEmail = async () => {
    if (!emailBindCode.trim()) {
      toast({ title: t("email.enterCodeFirst"), variant: "destructive" });
      return;
    }
    setIsEmailLoading(true);
    try {
      const res = await api.verifyEmailCode({ verification_id: emailBindVerifId, code: emailBindCode.trim() });
      if (res.success) {
        toast({ title: t("email.bindSuccess"), variant: "success" });
        setEditEmailOpen(false);
        resetEmailBind();
        fetchUser();
      } else {
        toast({ title: friendlyError(res.error_code, res.message), variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: error?.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setIsEmailLoading(false);
    }
  };

  const handleChangeSystemPassword = async () => {
    if (!oldPassword || !newPassword) {
      toast({ title: t("settings.passwordFieldsRequired"), variant: "destructive" });
      return;
    }
    const strength = validatePasswordStrength(newPassword);
    if (!strength.ok) {
      toast({ title: t("settings.passwordWeak"), description: strength.message, variant: "destructive" });
      return;
    }
    if (newPassword !== confirmPassword) {
      toast({ title: t("settings.passwordMismatch"), variant: "destructive" });
      return;
    }
    if (passwordEmailGate && !sysPwdCode.trim()) {
      toast({ title: t("email.enterCodeFirst"), variant: "destructive" });
      return;
    }

    setIsSystemPwdLoading(true);
    try {
      const proof = passwordEmailGate ? { verification_id: sysPwdVerifId, code: sysPwdCode.trim() } : undefined;
      const res = await api.changeSystemPassword(oldPassword, newPassword, proof);
      if (res.success) {
        toast({ title: t("settings.systemPasswordUpdated"), description: t("settings.systemPasswordUpdatedDescription"), variant: "success" });
        setChangeSystemPwdOpen(false);
        setOldPassword("");
        setNewPassword("");
        setConfirmPassword("");
        setSysPwdCode("");
        setSysPwdVerifId("");
      } else {
        toast({ title: friendlyError(res.error_code, res.message), variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("common.modifyFailed"), description: error.message, variant: "destructive" });
    } finally {
      setIsSystemPwdLoading(false);
    }
  };

  const handleChangeEmbyPassword = async () => {
    if (!newEmbyPassword) {
      toast({ title: t("settings.embyPasswordRequired"), variant: "destructive" });
      return;
    }
    const strength = validatePasswordStrength(newEmbyPassword);
    if (!strength.ok) {
      toast({ title: t("settings.passwordWeak"), description: strength.message, variant: "destructive" });
      return;
    }
    if (newEmbyPassword !== confirmEmbyPassword) {
      toast({ title: t("settings.passwordMismatch"), variant: "destructive" });
      return;
    }
    if (passwordEmailGate && !embyPwdCode.trim()) {
      toast({ title: t("email.enterCodeFirst"), variant: "destructive" });
      return;
    }

    setIsEmbyPwdLoading(true);
    try {
      const proof = passwordEmailGate ? { verification_id: embyPwdVerifId, code: embyPwdCode.trim() } : undefined;
      const res = await api.changeEmbyPassword(newEmbyPassword, proof);
      if (res.success) {
        toast({ title: t("settings.embyPasswordUpdated"), description: t("settings.embyPasswordUpdatedDescription"), variant: "success" });
        setChangeEmbyPwdOpen(false);
        setNewEmbyPassword("");
        setConfirmEmbyPassword("");
        setEmbyPwdCode("");
        setEmbyPwdVerifId("");
      } else {
        toast({ title: friendlyError(res.error_code, res.message), variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("common.modifyFailed"), description: error.message, variant: "destructive" });
    } finally {
      setIsEmbyPwdLoading(false);
    }
  };

  const handleCopyUrl = (url: string, key: string) => {
    navigator.clipboard.writeText(url).then(() => {
      setCopiedIndex(key);
      toast({ title: t("common.copied"), description: t("settings.lineCopiedDescription") });
      setTimeout(() => setCopiedIndex(null), 2000);
    });
  };

  // 初始加载线路；绑定/解绑后也会因 user.emby_id 变化重新拉取，避免保留旧的“需绑定”状态。
  useEffect(() => {
    void loadEmbyUrls();
  }, [loadEmbyUrls, user?.emby_id, user?.role]);

  useEffect(() => {
    void fetchSystemInfo();
  }, [fetchSystemInfo]);

  useEffect(() => {
    void runLineLatencyTests();
  }, [runLineLatencyTests]);

  if (error) {
    return <PageError message={error} onRetry={() => void loadData()} />;
  }

  if (isLoading) {
    return <PageLoading message={t("settings.loading")} />;
  }

  return (
    <motion.div
      variants={container}
      initial={hasAppeared ? false : "hidden"}
      animate="show"
      className="space-y-6"
    >
      <div>
        <h1 className="text-3xl font-bold">{t("settings.title")}</h1>
        <p className="text-muted-foreground">{t("settings.description")}</p>
      </div>

      {/* 快速导航 */}
      <motion.div variants={item}>
        <div className="grid gap-4 sm:grid-cols-3">
          <Link href="/settings/appearance" className="group">
            <Card className="glass-card cursor-pointer hover:shadow-lg transition-all h-full">
              <CardContent className="p-6 flex flex-col items-center justify-center text-center gap-3 h-full">
                <div className="p-3 rounded-lg bg-primary/10 group-hover:bg-primary/20 transition-colors">
                  <Palette className="h-6 w-6 text-primary" />
                </div>
                <div>
                  <h3 className="font-semibold">{t("settings.appearanceTitle")}</h3>
                  <p className="text-sm text-muted-foreground">{t("settings.appearanceDescription")}</p>
                </div>
              </CardContent>
            </Card>
          </Link>
        </div>
      </motion.div>

      <motion.div variants={item}>
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Globe className="h-5 w-5" />
              {t("settings.localeTitle")}
            </CardTitle>
            <CardDescription>
              {t("settings.localeDescription")}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            <Label>{t("settings.localeLabel")}</Label>
            <Select value={locale} onValueChange={(value) => setLocale(value as Locale)}>
              <SelectTrigger className="w-full sm:w-[240px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {supportedLocales.map((item) => (
                  <SelectItem key={item} value={item}>
                    {localeLabels[item]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">{t("settings.localeHelp")}</p>
          </CardContent>
        </Card>
      </motion.div>

      {/* Account Info */}
      <motion.div variants={item}>
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <User className="h-5 w-5" />
              {t("settings.accountInfo")}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <Label className="text-muted-foreground">{t("settings.username")}</Label>
                <p className="mt-1 font-medium">{user?.username}</p>
              </div>
              <div>
                <Label className="text-muted-foreground">UID</Label>
                <p className="mt-1 font-medium">{user?.uid}</p>
              </div>
              <div>
                <Label className="text-muted-foreground">{t("settings.role")}</Label>
                <div className="mt-1">
                  <Badge variant={user?.role === 0 ? "gradient" : "secondary"}>
                    {user?.role_name}
                  </Badge>
                </div>
              </div>
              <div>
                <Label className="text-muted-foreground font-medium">{t("settings.email")}</Label>
                <div className="mt-1 flex items-center justify-between gap-2">
                  <p className="font-medium flex min-w-0 items-center gap-2">
                    <span className="truncate">{user?.email || t("settings.notSet")}</span>
                    {user?.email && emailEnabled && (
                      <Badge variant="outline" className={emailVerified ? "shrink-0 border-emerald-500/40 text-emerald-600" : "shrink-0 border-amber-500/40 text-amber-600"}>
                        {emailVerified ? t("email.verified") : t("email.unverified")}
                      </Badge>
                    )}
                  </p>
                  <Button
                    variant="outline"
                    size="sm"
                    className="shrink-0"
                    onClick={() => {
                      setEmailValue(user?.email || "");
                      resetEmailBind();
                      setEditEmailOpen(true);
                    }}
                  >
                    {emailEnabled ? (user?.email ? t("email.changeTitle") : t("email.bindTitle")) : t("settings.edit")}
                  </Button>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </motion.div>

      {/* Telegram Binding */}
      <motion.div variants={item}>
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <MessageCircle className="h-5 w-5" />
              {t("settings.telegramTitle")}
            </CardTitle>
            <CardDescription>
              {t("settings.telegramDescription")}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {systemInfo?.telegram_bot?.username && (
              <div className="flex items-center justify-between gap-3 rounded-lg border border-primary/20 bg-primary/5 px-4 py-3 text-sm">
                <div className="flex items-center gap-2 min-w-0">
                  <Bot className="h-4 w-4 text-primary shrink-0" />
                  <div className="min-w-0">
                    <p className="font-medium">{t("settings.siteBot")}</p>
                    <a
                      href={botUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="block truncate text-primary hover:underline"
                    >
                      @{systemInfo.telegram_bot.username}
                    </a>
                  </div>
                </div>
                <Button asChild size="sm" variant="outline">
                  <a
                    href={botUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {t("settings.openBot")}
                  </a>
                </Button>
              </div>
            )}
            <div className="flex items-center justify-between rounded-lg bg-accent/50 p-4">
              <div className="flex items-center gap-3">
                <div className={`flex h-10 w-10 items-center justify-center rounded-full ${telegramStatus?.bound ? 'bg-emerald-500/20' : 'bg-muted'}`}>
                  {telegramStatus?.bound ? (
                    <Check className="h-5 w-5 text-emerald-500" />
                  ) : (
                    <X className="h-5 w-5 text-muted-foreground" />
                  )}
                </div>
                <div>
                  <p className="font-medium">
                    {telegramStatus?.bound ? t("settings.bound") : t("settings.unbound")}
                  </p>
                  {telegramStatus?.telegram_id && (
                    <p className="text-sm text-muted-foreground">
                      {telegramStatus.telegram_username ? (
                        <>@{telegramStatus.telegram_username} ({telegramStatus.telegram_id})</>
                      ) : (
                        <>ID: {telegramStatus.telegram_id}</>
                      )}
                    </p>
                  )}
                </div>
              </div>
              <div className="flex gap-2">
                {!telegramStatus?.bound ? (
                  <Button
                    onClick={handleGetBindCode}
                    disabled={isTgLoading}
                  >
                    {isTgLoading ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <LinkIcon className="mr-2 h-4 w-4" />
                    )}
                    {t("settings.getBindCode")}
                  </Button>
                ) : (
                  <>
                    {telegramStatus.rebind_approved ? (
                      <Button
                        variant="default"
                        onClick={handleUnbindTelegram}
                        disabled={isTgLoading}
                      >
                        {isTgLoading ? (
                          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        ) : (
                          <RefreshCw className="mr-2 h-4 w-4" />
                        )}
                        {t("settings.rebindApprovedUnbind")}
                      </Button>
                    ) : (
                      <>
                        {telegramStatus.can_unbind && (
                          <Button
                            variant="destructive"
                            onClick={handleUnbindTelegram}
                            disabled={isTgLoading}
                          >
                            <Unlink className="mr-2 h-4 w-4" />
                            {t("settings.unbind")}
                          </Button>
                        )}
                        {telegramStatus.can_change && (
                          <Button
                            variant="outline"
                            onClick={() => setRebindDialogOpen(true)}
                            disabled={isRebindLoading}
                          >
                            {isRebindLoading ? (
                              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                            ) : (
                              <LinkIcon className="mr-2 h-4 w-4" />
                            )}
                            {t("settings.submitRebindRequest")}
                          </Button>
                        )}
                        {!telegramStatus.can_change && telegramStatus.pending_rebind_request && (
                          <Badge variant="outline" className="self-center">
                            {t("settings.rebindSubmitted")}
                          </Badge>
                        )}
                      </>
                    )}
                  </>
                )}
              </div>
            </div>
            {telegramStatus?.force_bind && !telegramStatus?.rebind_approved && (
              <p className="text-sm text-amber-500">
                {t("settings.telegramRequired")}
              </p>
            )}
            {telegramStatus?.rebind_approved && telegramStatus?.bound && (
              <p className="text-sm text-emerald-600">
                {t("settings.rebindApprovedHint")}
              </p>
            )}
            {bindCode && !telegramStatus?.bound && (
              <div className="rounded-lg bg-blue-500/10 p-4 space-y-2">
                <p className="font-medium text-blue-500">{t("settings.bindCodeGenerated")}</p>
                <div className="flex flex-wrap items-center gap-2">
                  <code className="text-2xl font-mono font-bold tracking-widest bg-background/50 px-4 py-2 rounded-lg">
                    {bindCode}
                  </code>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      navigator.clipboard.writeText(`/bind ${bindCode}`);
                      toast({ title: t("settings.copyCommand"), variant: "success" });
                    }}
                  >
                    {t("settings.copyCommand")}
                  </Button>
                  {botUrl && (
                    <Button asChild variant="outline" size="sm">
                      <a
                        href={botUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                      >
                        <Bot className="mr-2 h-4 w-4" />
                        {t("settings.openBotUsername", { username: botUsername || "" })}
                      </a>
                    </Button>
                  )}
                </div>
                <p className="text-sm text-muted-foreground">
                  {t("settings.sendBindWithin", { minutes: Math.floor(bindCodeExpiry / 60), bot: systemInfo?.telegram_bot?.username ? `@${systemInfo.telegram_bot.username}` : "Telegram Bot" })}{" "}
                  <code className="bg-background/50 px-1.5 py-0.5 rounded">/bind {bindCode}</code>
                </p>
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  {t("settings.waitingForBind")}
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </motion.div>

      {/* Emby Binding */}
      <motion.div variants={item}>
        <Card className="glass-card border-primary/20 bg-primary/5">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Tv className="h-5 w-5" />
              {t("settings.embyTitle")}
            </CardTitle>
            <CardDescription>
              {t("settings.embyDescription")}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex flex-col gap-3 rounded-lg bg-accent/50 p-4 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
              <div className="flex items-center gap-3 min-w-0">
                <div className={`flex h-10 w-10 items-center justify-center rounded-full shrink-0 ${user?.emby_id ? 'bg-emerald-500/20' : 'bg-muted'}`}>
                  {user?.emby_id ? (
                    <Check className="h-5 w-5 text-emerald-500" />
                  ) : (
                    <X className="h-5 w-5 text-muted-foreground" />
                  )}
                </div>
                <div className="min-w-0 flex-1">
                  <p className="font-medium">
                    {user?.emby_id ? t("settings.bound") : t("settings.unbound")}
                  </p>
                  {user?.emby_id && (
                    <p className="text-sm text-muted-foreground break-all">
                      Emby ID: {user.emby_id}
                    </p>
                  )}
                  {user?.emby_id && (user.emby_username || user.username) && (
                    <p className="text-sm text-muted-foreground break-all">
                      {t("settings.embyUsername", { username: user.emby_username || user.username || "" })}
                    </p>
                  )}
                </div>
              </div>
              <div className="flex gap-2 shrink-0 sm:justify-end">
                {!user?.emby_id ? (
                  <Button
                    onClick={() => {
                      setCompletePendingEmby(hasEmbyRegistrationEntitlement);
                      setEmbyUsername(user?.username || "");
                      setBindEmbyOpen(true);
                    }}
                    className="w-full sm:w-auto"
                  >
                    <LinkIcon className="mr-2 h-4 w-4" />
                    {hasEmbyRegistrationEntitlement ? t("settings.continueSetup") : t("settings.bind")}
                  </Button>
                ) : (
                  <Button
                    variant="destructive"
                    onClick={handleUnbindEmby}
                    disabled={isEmbyLoading || !canUnbindEmby}
                    className="w-full sm:w-auto"
                  >
                    {isEmbyLoading ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <Unlink className="mr-2 h-4 w-4" />
                    )}
                    {t("settings.unbind")}
                  </Button>
                )}
              </div>
            </div>
            {!user?.emby_id && (
              <p className="text-sm text-muted-foreground">
                {hasEmbyRegistrationEntitlement
                  ? t("settings.pendingEmbyHint")
                  : t("settings.existingEmbyHint")}
              </p>
            )}
            {user?.emby_id && !canUnbindEmby && (
              <p className="text-sm text-muted-foreground">
                {t("settings.embyGrantLocked")}
              </p>
            )}
            {user?.emby_id && canUnbindEmby && (
              <p className="text-sm text-muted-foreground">
                {t("settings.embyUnbindWarning")}
              </p>
            )}
          </CardContent>
        </Card>
      </motion.div>

      {/* API Key Management */}
      {user?.emby_id && (
        <motion.div variants={item}>
          <Card className="glass-card">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Key className="h-5 w-5" />
                {t("settings.apiKeyTitle")}
              </CardTitle>
              <CardDescription>
                {t("settings.apiKeyDescription")}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex items-center justify-between rounded-lg bg-accent/50 p-4">
                <div>
                  <p className="font-medium">{t("settings.apiKeyTitle")}</p>
                  <p className="text-sm text-muted-foreground">
                    {t("settings.apiKeyCardDescription")}
                  </p>
                </div>
                <Button asChild>
                  <a href="/settings/apikey">
                    <Key className="mr-2 h-4 w-4" />
                    {t("settings.manageApiKey")}
                  </a>
                </Button>
              </div>
            </CardContent>
          </Card>
        </motion.div>
      )}

      {/* Password Change & Emby URLs */}
      <motion.div variants={item}>
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Lock className="h-5 w-5" />
              {t("settings.passwordTitle")}
            </CardTitle>
            <CardDescription>
              {t("settings.passwordDescription")}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="rounded-xl border border-border p-4">
                <p className="text-sm font-medium">{t("settings.systemPassword")}</p>
                <p className="text-sm text-muted-foreground mt-1">
                  {t("settings.systemPasswordDescription")}
                </p>
                <Button className="mt-4" onClick={() => setChangeSystemPwdOpen(true)}>
                  <Lock className="mr-2 h-4 w-4" />
                  {t("settings.changeSystemPassword")}
                </Button>
              </div>
              <div className="rounded-xl border border-border p-4">
                <p className="text-sm font-medium">{t("settings.embyPassword")}</p>
                <p className="text-sm text-muted-foreground mt-1">
                  {t("settings.embyPasswordDescription")}
                </p>
                <Button className="mt-4" onClick={() => setChangeEmbyPwdOpen(true)} disabled={!user?.emby_id}>
                  <Lock className="mr-2 h-4 w-4" />
                  {t("settings.changeEmbyPassword")}
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </motion.div>

      {linesRequireRenewal ? (
        <motion.div variants={item}>
          <Card className="glass-card border-destructive/30">
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-destructive">
                <AlertCircle className="h-5 w-5" />
                {t("settings.embyExpiredTitle")}
              </CardTitle>
              <CardDescription>
                {t("settings.embyExpiredDescription")}
              </CardDescription>
            </CardHeader>
          </Card>
        </motion.div>
      ) : !linesRequireEmby && (
      <motion.div variants={item}>
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Globe className="h-5 w-5" />
              {t("settings.serverLinesTitle")}
            </CardTitle>
            <CardDescription>
              {t("settings.serverLinesDescription")}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex justify-end">
              <Button variant="outline" size="sm" onClick={() => void runLineLatencyTests()} disabled={isLatencyTesting}>
                {isLatencyTesting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
                {t("settings.testLatencyAgain")}
              </Button>
            </div>

            {sortedEmbyLineItems.length > 0 ? (
              <div className="grid gap-3 sm:grid-cols-2">
                {sortedEmbyLineItems.map(({ line, index, key }) => {
                  return (
                    <div
                      key={key}
                      className="group relative h-full rounded-xl border border-border bg-card p-4 transition-colors hover:bg-accent/50"
                    >
                      <div className="flex items-start justify-between gap-2">
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-semibold">{line.name || t("settings.lineName", { index: index + 1 })}</p>
                          <p className="mt-1 break-all truncate font-mono text-xs text-muted-foreground">
                            {line.url}
                          </p>
                          <p className={`mt-1 ${renderLatencyClassName(key)}`}>
                            {t("settings.latencyLabel", { value: renderLatencyText(key) })}
                          </p>
                        </div>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8 shrink-0 opacity-0 transition-opacity group-hover:opacity-100"
                          onClick={() => handleCopyUrl(line.url, key)}
                        >
                          {copiedIndex === key ? (
                            <Check className="h-4 w-4 text-green-500" />
                          ) : (
                            <Copy className="h-4 w-4" />
                          )}
                        </Button>
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">{t("settings.noLines")}</p>
            )}

            {sortedWhitelistLineItems.length > 0 && (
              <>
                <Separator />
                <div>
                  <p className="mb-3 flex items-center gap-1.5 text-sm font-medium">
                    <Star className="h-4 w-4 text-yellow-500" />
                    {t("settings.dedicatedLines")}
                  </p>
                  <div className="grid gap-3 sm:grid-cols-2">
                    {sortedWhitelistLineItems.map(({ line, index, key }) => {
                      return (
                        <div
                          key={key}
                          className="group relative rounded-xl border border-yellow-500/20 bg-yellow-500/5 p-4 transition-colors hover:bg-yellow-500/10 dark:border-yellow-500/30 dark:bg-yellow-500/10 dark:hover:bg-yellow-500/20"
                        >
                          <div className="flex items-start justify-between gap-2">
                            <div className="min-w-0 flex-1">
                              <p className="text-sm font-semibold">{line.name || t("settings.dedicatedLineName", { index: index + 1 })}</p>
                              <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
                                {line.url}
                              </p>
                              <p className={`mt-1 ${renderLatencyClassName(key)}`}>
                                {t("settings.latencyLabel", { value: renderLatencyText(key) })}
                              </p>
                            </div>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8 shrink-0 opacity-0 transition-opacity group-hover:opacity-100"
                              onClick={() => handleCopyUrl(line.url, key)}
                            >
                              {copiedIndex === key ? (
                                <Check className="h-4 w-4 text-green-500" />
                              ) : (
                                <Copy className="h-4 w-4" />
                              )}
                            </Button>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              </>
            )}
          </CardContent>
        </Card>
      </motion.div>
      )}

      {/* Bangumi */}
      {bangumiAnyEnabled && (
      <motion.div variants={item}>
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Key className="h-5 w-5" />
              {t("settings.bangumiTitle")}
            </CardTitle>
            <CardDescription>
              {t("settings.bangumiDescription")}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="space-y-3">
              {bangumiSyncEnabled && (
                <div className="flex items-center justify-between gap-4">
                  <div className="space-y-0.5">
                    <Label>{t("settings.bangumiSync")}</Label>
                    <p className="text-sm text-muted-foreground">
                      {t("settings.bangumiSyncDescription")}
                    </p>
                  </div>
                  <Switch checked={bgmMode} onCheckedChange={setBgmMode} />
                </div>
              )}
              {bangumiManageEnabled && (
                <div className="flex items-center justify-between gap-4">
                  <div className="space-y-0.5">
                    <Label>{t("settings.bangumiManage")}</Label>
                    <p className="text-sm text-muted-foreground">
                      {t("settings.bangumiManageDescription")}
                    </p>
                  </div>
                  <Switch checked={bgmManageMode} onCheckedChange={setBgmManageMode} />
                </div>
              )}
              <div className="space-y-2">
                <div className="space-y-2">
                  <Label>Bangumi Token</Label>
                  <Input
                    type="password"
                    placeholder={bgmTokenSet ? t("settings.bangumiTokenConfiguredPlaceholder") : t("settings.bangumiTokenPlaceholder")}
                    value={bgmToken}
                    onChange={(e) => setBgmToken(e.target.value)}
                  />
                </div>
                <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                  <div className="flex flex-col gap-2 sm:flex-row">
                    <Button
                      onClick={handleSaveBgmSettings}
                      disabled={isBgmLoading}
                    >
                      {isBgmLoading ? t("common.saving") : t("settings.saveBangumiSettings")}
                    </Button>
                    {bgmTokenSet && (
                      <Button
                        variant="outline"
                        onClick={handleClearBgmToken}
                        disabled={isBgmLoading}
                      >
                        {t("settings.clearToken")}
                      </Button>
                    )}
                  </div>
                  {bgmTokenSet && (
                    <p className="text-sm text-muted-foreground">
                      {t("settings.bangumiTokenConfigured")}
                    </p>
                  )}
                </div>
              </div>
            </div>

            <Separator />
          </CardContent>
        </Card>
      </motion.div>
      )}

      {/* 登录通知 */}
      <motion.div variants={item}>
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Bell className="h-5 w-5" />
              {t("settings.loginNotifyTitle")}
            </CardTitle>
            <CardDescription>
              {t("settings.loginNotifyDescription")}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label>{t("settings.loginNotifyTelegram")}</Label>
                <p className="text-sm text-muted-foreground">
                  {t("settings.loginNotifyTelegramDesc")}
                </p>
              </div>
              <Switch
                checked={notifyLoginTelegram}
                onCheckedChange={(v) => {
                  setNotifyLoginTelegram(v);
                  api.updateMySettings({ notify_on_login_telegram: v });
                }}
                disabled={!hasTelegramBinding}
                aria-readonly={!hasTelegramBinding}
              />
            </div>
            <Separator />
            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label>{t("settings.loginNotifyEmail")}</Label>
                <p className="text-sm text-muted-foreground">
                  {t("settings.loginNotifyEmailDesc")}
                </p>
              </div>
              <Switch
                checked={notifyLoginEmail}
                onCheckedChange={(v) => {
                  setNotifyLoginEmail(v);
                  api.updateMySettings({ notify_on_login_email: v });
                }}
                disabled={!hasVerifiedEmail}
                aria-readonly={!hasVerifiedEmail}
              />
            </div>
            <Separator />
            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label>{t("settings.ticketNotifyTelegram")}</Label>
                <p className="text-sm text-muted-foreground">
                  {t("settings.ticketNotifyTelegramDesc")}
                </p>
              </div>
              <Switch
                checked={notifyTicketTelegram}
                onCheckedChange={(v) => {
                  setNotifyTicketTelegram(v);
                  api.updateMySettings({ notify_on_ticket_telegram: v });
                }}
                disabled={!hasTelegramBinding}
                aria-readonly={!hasTelegramBinding}
              />
            </div>
          </CardContent>
        </Card>
      </motion.div>

      {/* Bind Emby Dialog */}
      <Dialog open={bindEmbyOpen} onOpenChange={(open) => {
        setBindEmbyOpen(open);
        if (!open) setCompletePendingEmby(false);
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{completePendingEmby ? t("settings.embyBindDialogTitleCreate") : t("settings.embyBindDialogTitleBind")}</DialogTitle>
            <DialogDescription>
              {completePendingEmby ? t("settings.embyBindDialogDescriptionCreate") : t("settings.embyBindDialogDescriptionBind")}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("settings.embyUsername")}</Label>
              <Input
                placeholder={t("settings.embyUsernamePlaceholder")}
                value={embyUsername}
                onChange={(e) => setEmbyUsername(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && embyUsername.trim()) {
                    handleBindEmby();
                  }
                }}
              />
            </div>
            <div className="space-y-2">
              <Label>{t("settings.embyPassword")} {!completePendingEmby && <span className="text-xs text-muted-foreground">{t("settings.embyPasswordOptionalHint")}</span>}</Label>
              <div className="relative">
                <Input
                  type={showEmbyPassword ? "text" : "password"}
                  placeholder={t("settings.embyPasswordPlaceholder")}
                  value={embyPassword}
                  onChange={(e) => setEmbyPassword(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && embyUsername.trim()) {
                      handleBindEmby();
                    }
                  }}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="absolute right-0 top-0 h-full px-3 py-2 hover:bg-transparent"
                  onClick={() => setShowEmbyPassword(!showEmbyPassword)}
                >
                  {showEmbyPassword ? (
                    <EyeOff className="h-4 w-4 text-muted-foreground" />
                  ) : (
                    <Eye className="h-4 w-4 text-muted-foreground" />
                  )}
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">
                {t("settings.embyCredentialsHint")}
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                    setBindEmbyOpen(false);
                    setCompletePendingEmby(false);
                setEmbyUsername("");
                setEmbyPassword("");
              }}
            >
              {t("common.cancel")}
            </Button>
            <Button
              onClick={handleBindEmby}
              disabled={isEmbyLoading || !embyUsername.trim()}
            >
              {isEmbyLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {completePendingEmby ? t("settings.confirmOpen") : t("settings.confirmBind")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Email Dialog */}
      <Dialog open={editEmailOpen} onOpenChange={(open) => { setEditEmailOpen(open); if (!open) resetEmailBind(); }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{emailEnabled ? t("email.bindTitle") : t("settings.emailDialogTitle")}</DialogTitle>
            <DialogDescription>
              {emailEnabled ? t("email.bindDescription") : t("settings.emailDialogDescription")}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("settings.emailAddress")}</Label>
              <Input
                type="email"
                placeholder={t("settings.emailPlaceholder")}
                value={emailValue}
                onChange={(e) => setEmailValue(e.target.value)}
              />
            </div>
            {emailEnabled && (
              <EmailCodeInput
                purpose="bind"
                email={emailValue}
                code={emailBindCode}
                onCodeChange={setEmailBindCode}
                onSent={(id) => setEmailBindVerifId(id)}
              />
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditEmailOpen(false)}>
              {t("common.cancel")}
            </Button>
            {emailEnabled ? (
              <Button onClick={handleVerifyBindEmail} disabled={isEmailLoading || !emailBindVerifId || !emailBindCode.trim()}>
                {isEmailLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {t("email.verify")}
              </Button>
            ) : (
              <Button onClick={handleUpdateEmail} disabled={isEmailLoading}>
                {isEmailLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {t("adminConfig.confirmSave")}
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Change Password Dialog */}
      <Dialog open={changeSystemPwdOpen} onOpenChange={(open) => {
        setChangeSystemPwdOpen(open);
        if (!open) {
          setOldPassword("");
          setNewPassword("");
          setConfirmPassword("");
          setSysPwdCode("");
          setSysPwdVerifId("");
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("settings.systemPasswordDialogTitle")}</DialogTitle>
            <DialogDescription>
              {t("settings.systemPasswordDialogDescription")}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("settings.currentPassword")}</Label>
              <div className="relative">
                <Input
                  type={showOldPwd ? "text" : "password"}
                  placeholder={t("settings.currentPasswordPlaceholder")}
                  value={oldPassword}
                  onChange={(e) => setOldPassword(e.target.value)}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="absolute right-0 top-0 h-full px-3 py-2 hover:bg-transparent"
                  onClick={() => setShowOldPwd(!showOldPwd)}
                >
                  {showOldPwd ? <EyeOff className="h-4 w-4 text-muted-foreground" /> : <Eye className="h-4 w-4 text-muted-foreground" />}
                </Button>
              </div>
            </div>
            <div className="space-y-2">
              <Label>{t("settings.newPassword")}</Label>
              <div className="relative">
                <Input
                  type={showNewPwd ? "text" : "password"}
                  placeholder={t("settings.newPasswordPlaceholder")}
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="absolute right-0 top-0 h-full px-3 py-2 hover:bg-transparent"
                  onClick={() => setShowNewPwd(!showNewPwd)}
                >
                  {showNewPwd ? <EyeOff className="h-4 w-4 text-muted-foreground" /> : <Eye className="h-4 w-4 text-muted-foreground" />}
                </Button>
              </div>
              {newPassword && (
                <p className={`text-xs ${systemPwdStrength.ok ? passwordStrengthLabel(systemPwdStrength.score).className : "text-destructive"}`}>
                  {systemPwdStrength.ok
                    ? t("settings.passwordStrength", { label: passwordStrengthLabel(systemPwdStrength.score).label })
                    : systemPwdStrength.message}
                </p>
              )}
            </div>
            <div className="space-y-2">
              <Label>{t("settings.confirmNewPassword")}</Label>
              <Input
                type="password"
                placeholder={t("settings.confirmNewPasswordPlaceholder")}
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && oldPassword && newPassword && confirmPassword) {
                    handleChangeSystemPassword();
                  }
                }}
              />
              {confirmPassword && newPassword !== confirmPassword && (
                <p className="text-xs text-destructive">{t("settings.passwordMismatch")}</p>
              )}
            </div>
            {passwordEmailGate && (
              <div className="space-y-2 rounded-lg border border-amber-500/30 bg-amber-500/5 p-3">
                <p className="text-xs text-muted-foreground">{t("email.changePasswordNotice")}</p>
                <EmailCodeInput purpose="change_password" code={sysPwdCode} onCodeChange={setSysPwdCode} onSent={(id) => setSysPwdVerifId(id)} />
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setChangeSystemPwdOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={handleChangeSystemPassword}
              disabled={isSystemPwdLoading || !oldPassword || !systemPwdStrength.ok || newPassword !== confirmPassword || (passwordEmailGate && (!sysPwdCode.trim() || !sysPwdVerifId))}
            >
              {isSystemPwdLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t("settings.confirmModify")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={changeEmbyPwdOpen} onOpenChange={(open) => {
        setChangeEmbyPwdOpen(open);
        if (!open) {
          setNewEmbyPassword("");
          setConfirmEmbyPassword("");
          setEmbyPwdCode("");
          setEmbyPwdVerifId("");
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("settings.embyPasswordDialogTitle")}</DialogTitle>
            <DialogDescription>
              {t("settings.embyPasswordDescription")}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("settings.newPassword")}</Label>
              <div className="relative">
                <Input
                  type={showNewEmbyPwd ? "text" : "password"}
                  placeholder={t("settings.newPasswordPlaceholder")}
                  value={newEmbyPassword}
                  onChange={(e) => setNewEmbyPassword(e.target.value)}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="absolute right-0 top-0 h-full px-3 py-2 hover:bg-transparent"
                  onClick={() => setShowNewEmbyPwd(!showNewEmbyPwd)}
                >
                  {showNewEmbyPwd ? <EyeOff className="h-4 w-4 text-muted-foreground" /> : <Eye className="h-4 w-4 text-muted-foreground" />}
                </Button>
              </div>
              {newEmbyPassword && (
                <p className={`text-xs ${embyPwdStrength.ok ? passwordStrengthLabel(embyPwdStrength.score).className : "text-destructive"}`}>
                  {embyPwdStrength.ok
                    ? t("settings.passwordStrength", { label: passwordStrengthLabel(embyPwdStrength.score).label })
                    : embyPwdStrength.message}
                </p>
              )}
            </div>
            <div className="space-y-2">
              <Label>{t("settings.confirmNewPassword")}</Label>
              <div className="relative">
                <Input
                  type={showConfirmEmbyPwd ? "text" : "password"}
                  placeholder={t("settings.confirmNewPasswordPlaceholder")}
                  value={confirmEmbyPassword}
                  onChange={(e) => setConfirmEmbyPassword(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && newEmbyPassword && confirmEmbyPassword) {
                      handleChangeEmbyPassword();
                    }
                  }}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="absolute right-0 top-0 h-full px-3 py-2 hover:bg-transparent"
                  onClick={() => setShowConfirmEmbyPwd(!showConfirmEmbyPwd)}
                >
                  {showConfirmEmbyPwd ? <EyeOff className="h-4 w-4 text-muted-foreground" /> : <Eye className="h-4 w-4 text-muted-foreground" />}
                </Button>
              </div>
              {confirmEmbyPassword && newEmbyPassword !== confirmEmbyPassword && (
                <p className="text-xs text-destructive">{t("settings.passwordMismatch")}</p>
              )}
            </div>
            {passwordEmailGate && (
              <div className="space-y-2 rounded-lg border border-amber-500/30 bg-amber-500/5 p-3">
                <p className="text-xs text-muted-foreground">{t("email.changePasswordNotice")}</p>
                <EmailCodeInput purpose="change_emby_password" code={embyPwdCode} onCodeChange={setEmbyPwdCode} onSent={(id) => setEmbyPwdVerifId(id)} />
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setChangeEmbyPwdOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={handleChangeEmbyPassword}
              disabled={isEmbyPwdLoading || !embyPwdStrength.ok || newEmbyPassword !== confirmEmbyPassword || (passwordEmailGate && (!embyPwdCode.trim() || !embyPwdVerifId))}
            >
              {isEmbyPwdLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t("settings.confirmModify")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={rebindDialogOpen} onOpenChange={setRebindDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("settings.telegramRebindDialogTitle")}</DialogTitle>
            <DialogDescription>
              {t("settings.telegramRebindDialogDescription")}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label>{t("settings.rebindReasonLabel")}</Label>
            <Textarea
              value={rebindReason}
              onChange={(event) => setRebindReason(event.target.value.slice(0, 500))}
              placeholder={t("settings.rebindReasonPlaceholder")}
              rows={4}
            />
            <p className="text-xs text-muted-foreground">{rebindReason.length}/500</p>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => { setRebindDialogOpen(false); setRebindReason(""); }} disabled={isRebindLoading}>
              {t("common.cancel")}
            </Button>
            <Button onClick={handleRequestTelegramRebind} disabled={isRebindLoading}>
              {isRebindLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t("settings.submitApplication")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}
