"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useToast } from "@/hooks/use-toast";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError, PageLoading } from "@/components/layout/page-state";
import { useAuthStore } from "@/store/auth";
import { useSystemStore } from "@/store/system";
import { api, type UserSettings, type TelegramStatus, type EmbyStatus, type EmbyLibraryAccess } from "@/lib/api";
import { passwordStrengthLabel, validatePasswordStrength } from "@/lib/password";

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
  const { user, fetchUser } = useAuthStore();
  const { info: systemInfo, fetchInfo: fetchSystemInfo } = useSystemStore();
  const [settings, setSettings] = useState<UserSettings | null>(null);
  const [telegramStatus, setTelegramStatus] = useState<TelegramStatus | null>(null);
  const [bgmTokenSet, setBgmTokenSet] = useState(false);
  const [bgmMode, setBgmMode] = useState(false);
  const [bgmToken, setBgmToken] = useState("");
  const [isBgmLoading, setIsBgmLoading] = useState(false);
  const [embyStatus, setEmbyStatus] = useState<EmbyStatus | null>(null);

  // Telegram bind code
  const [bindCode, setBindCode] = useState<string | null>(null);
  const [bindCodeExpiry, setBindCodeExpiry] = useState<number>(0);
  const [isTgLoading, setIsTgLoading] = useState(false);
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
  const [libraryAccess, setLibraryAccess] = useState<EmbyLibraryAccess | null>(null);
  const [isLibraryLoading, setIsLibraryLoading] = useState(false);

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

  const systemPwdStrength = useMemo(
    () => validatePasswordStrength(newPassword),
    [newPassword]
  );
  const embyPwdStrength = useMemo(
    () => validatePasswordStrength(newEmbyPassword),
    [newEmbyPassword]
  );
  const bangumiSyncEnabled = Boolean(settings?.system_config?.bangumi_sync_enabled);

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
    if (!info || info.status === "idle") return "待测速";
    if (info.status === "testing") return "测速中...";
    if (info.status === "ok") return `${info.latencyMs} ms`;
    if (info.status === "timeout") return "超时";
    return "不可达";
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

  const testSingleLineLatency = useCallback(async (rawUrl: string) => {
    const url = rawUrl.trim();
    if (!url) {
      return { status: "error" as const };
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
        status: "ok" as const,
        latencyMs: Math.max(1, Math.round(performance.now() - startedAt)),
      };
    } catch (error: any) {
      if (error?.name === "AbortError") {
        return { status: "timeout" as const };
      }
      return { status: "error" as const };
    } finally {
      window.clearTimeout(timeout);
    }
  }, []);

  const runLineLatencyTests = useCallback(async () => {
    const allItems = [...embyLineItems, ...whitelistLineItems];
    if (allItems.length === 0) {
      setLineLatencyMap({});
      return;
    }

    setIsLatencyTesting(true);
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
    const [settingsRes, librariesRes] = await Promise.all([
      api.getMySettings(),
      api.getMyLibraries().catch(() => null),
    ]);
    if (settingsRes.success && settingsRes.data) {
      setSettings(settingsRes.data);
      setBgmMode(settingsRes.data.bgm_mode);
      setBgmTokenSet(settingsRes.data.bgm_token_set ?? false);
      setEmbyStatus(settingsRes.data.emby_status ?? null);
      setTelegramStatus(settingsRes.data.telegram as TelegramStatus);
    }
    if (librariesRes?.success && librariesRes.data) {
      setLibraryAccess(librariesRes.data);
    }
    return true;
  }, []);

  const isSelfServiceLibraryVisible = useCallback((name: string) => {
    const key = name.trim().toLowerCase();
    return Boolean(libraryAccess?.libraries?.some((lib) => lib.name.trim().toLowerCase() === key));
  }, [libraryAccess]);

  const handleLibraryVisibility = async (name: string, action: "show" | "hide") => {
    if (!libraryAccess?.self_service_enabled) {
      toast({ title: "无权操作", description: "管理员尚未为你开启媒体库自助显隐权限", variant: "destructive" });
      return;
    }
    setIsLibraryLoading(true);
    try {
      const res = await api.updateMyLibraryVisibility(action, [name]);
      if (res.success && res.data) {
        setLibraryAccess(res.data);
        toast({ title: action === "show" ? "媒体库已显示" : "媒体库已隐藏", variant: "success" });
      } else {
        toast({ title: "更新失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "更新失败", description: error.message, variant: "destructive" });
    } finally {
      setIsLibraryLoading(false);
    }
  };

  const {
    isLoading,
    error,
    execute: loadData,
  } = useAsyncResource(loadSettingsResource, { immediate: true });

  const handleSaveBgmSettings = async () => {
    const nextBgmToken = bgmToken.trim();
    if (!bangumiSyncEnabled) {
      toast({ title: "功能未启用", description: "管理员尚未开启 Bangumi 点格子", variant: "destructive" });
      return;
    }
    if (bgmMode && !nextBgmToken && !bgmTokenSet) {
      toast({ title: "请输入 Bangumi Token", description: "启用 BGM 同步前需要填写个人 Token", variant: "destructive" });
      return;
    }

    setIsBgmLoading(true);
    try {
      const res = await api.updateMySettings({
        bgm_mode: bgmMode,
        bgm_token: nextBgmToken || undefined,
      });

      if (res.success) {
        const nextTokenSet = bgmTokenSet || Boolean(nextBgmToken);
        setBgmToken("");
        setBgmTokenSet(nextTokenSet);
        setSettings((prev) => prev ? { ...prev, bgm_mode: bgmMode, bgm_token_set: nextTokenSet } : prev);
        toast({ title: "保存成功", description: "Bangumi 同步设置已更新", variant: "success" });
      } else {
        toast({ title: "保存失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "保存失败", description: error.message, variant: "destructive" });
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
        bgm_token: "",
      });

      if (res.success) {
        setBgmMode(false);
        setBgmToken("");
        setBgmTokenSet(false);
        setSettings((prev) => prev ? { ...prev, bgm_mode: false, bgm_token_set: false } : prev);
        toast({ title: "已清除", description: "Bangumi Token 已移除，同步已关闭", variant: "success" });
      } else {
        toast({ title: "清除失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "清除失败", description: error.message, variant: "destructive" });
    } finally {
      setIsBgmLoading(false);
    }
  };

  const handleGetBindCode = async () => {
    setIsTgLoading(true);
    try {
      const res = await api.getBindCode();
      if (res.success && res.data?.bind_code) {
        setBindCode(res.data.bind_code);
        setBindCodeExpiry(res.data.expires_in);
        toast({
          title: "绑定码已生成",
          description: `请在 ${Math.floor(res.data.expires_in / 60)} 分钟内向 Bot 发送 /bind ${res.data.bind_code}`,
          variant: "success",
        });
      } else {
        toast({ title: "获取绑定码失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "获取绑定码失败", description: error.message, variant: "destructive" });
    } finally {
      setIsTgLoading(false);
    }
  };

  const handleRequestTelegramRebind = async () => {
    if (rebindReason.length > 500) {
      toast({ title: "备注过长", description: "最多 500 字符", variant: "destructive" });
      return;
    }
    setIsRebindLoading(true);
    try {
      const res = await api.requestTelegramRebind(rebindReason.trim() || undefined);
      if (res.success) {
        toast({ title: "换绑请求已提交", description: res.message, variant: "success" });
        setRebindDialogOpen(false);
        setRebindReason("");
        loadData();
      } else {
        toast({ title: "换绑请求提交失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "换绑请求提交失败", description: error.message, variant: "destructive" });
    } finally {
      setIsRebindLoading(false);
    }
  };

  const handleUnbindTelegram = async () => {
    setIsTgLoading(true);
    try {
      const res = await api.unbindTelegram();
      if (res.success) {
        toast({ title: "解绑成功", variant: "success" });
        setBindCode(null);
        loadData();
        fetchUser();
      } else {
        toast({ title: "解绑失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "解绑失败", description: error.message, variant: "destructive" });
    } finally {
      setIsTgLoading(false);
    }
  };

  const handleBindEmby = async () => {
    const username = embyUsername.trim();
    // Emby 允许空密码账号，这里保留原样（不再 trim 密码两端，但用户输入框已限制空白）
    const password = embyPassword;

    if (!username) {
      toast({ title: "请输入 Emby 用户名", variant: "destructive" });
      return;
    }

    setIsEmbyLoading(true);
    try {
      const res = completePendingEmby
        ? await api.completeEmbyRegistration(username, password)
        : await api.bindEmbyAccount(username, password);
      if (res.success) {
        toast({ title: completePendingEmby ? "Emby 账号已开通" : "绑定成功", variant: "success" });
        setBindEmbyOpen(false);
        setCompletePendingEmby(false);
        setEmbyUsername("");
        setEmbyPassword("");
        await loadData();
        await fetchUser();
        await loadEmbyUrls();
      } else {
        toast({ title: completePendingEmby ? "开通失败" : "绑定失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      console.error("绑定失败:", error);
      toast({ title: completePendingEmby ? "开通失败" : "绑定失败", description: error.message, variant: "destructive" });
    } finally {
      setIsEmbyLoading(false);
    }
  };

  const handleUnbindEmby = async () => {
    setIsEmbyLoading(true);
    try {
      const res = await api.unbindEmbyAccount();
      if (res.success) {
        toast({ title: "解绑成功", variant: "success" });
        await loadData();
        await fetchUser();
        await loadEmbyUrls();
      } else {
        toast({ title: "解绑失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "解绑失败", description: error.message, variant: "destructive" });
    } finally {
      setIsEmbyLoading(false);
    }
  };

  const handleUpdateEmail = async () => {
    if (!emailValue) {
      toast({ title: "请输入邮箱地址", variant: "destructive" });
      return;
    }

    setIsEmailLoading(true);
    try {
      const res = await api.updateMe({ email: emailValue });
      if (res.success) {
        toast({ title: "邮箱更新成功", variant: "success" });
        setEditEmailOpen(false);
        fetchUser();
      } else {
        toast({ title: "更新失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "更新失败", description: error.message, variant: "destructive" });
    } finally {
      setIsEmailLoading(false);
    }
  };

  const handleChangeSystemPassword = async () => {
    if (!oldPassword || !newPassword) {
      toast({ title: "请填写当前密码和新密码", variant: "destructive" });
      return;
    }
    const strength = validatePasswordStrength(newPassword);
    if (!strength.ok) {
      toast({ title: "密码强度不足", description: strength.message, variant: "destructive" });
      return;
    }
    if (newPassword !== confirmPassword) {
      toast({ title: "两次输入的新密码不一致", variant: "destructive" });
      return;
    }

    setIsSystemPwdLoading(true);
    try {
      const res = await api.changeSystemPassword(oldPassword, newPassword);
      if (res.success) {
        toast({ title: "系统密码修改成功", description: "仅系统登录密码已更新", variant: "success" });
        setChangeSystemPwdOpen(false);
        setOldPassword("");
        setNewPassword("");
        setConfirmPassword("");
      } else {
        toast({ title: "修改失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "修改失败", description: error.message, variant: "destructive" });
    } finally {
      setIsSystemPwdLoading(false);
    }
  };

  const handleChangeEmbyPassword = async () => {
    if (!newEmbyPassword) {
      toast({ title: "请填写新的 Emby 密码", variant: "destructive" });
      return;
    }
    const strength = validatePasswordStrength(newEmbyPassword);
    if (!strength.ok) {
      toast({ title: "密码强度不足", description: strength.message, variant: "destructive" });
      return;
    }
    if (newEmbyPassword !== confirmEmbyPassword) {
      toast({ title: "两次输入的新密码不一致", variant: "destructive" });
      return;
    }

    setIsEmbyPwdLoading(true);
    try {
      const res = await api.changeEmbyPassword(newEmbyPassword);
      if (res.success) {
        toast({ title: "Emby 密码修改成功", description: "仅 Emby 密码已更新", variant: "success" });
        setChangeEmbyPwdOpen(false);
        setNewEmbyPassword("");
        setConfirmEmbyPassword("");
      } else {
        toast({ title: "修改失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "修改失败", description: error.message, variant: "destructive" });
    } finally {
      setIsEmbyPwdLoading(false);
    }
  };

  const handleCopyUrl = (url: string, key: string) => {
    navigator.clipboard.writeText(url).then(() => {
      setCopiedIndex(key);
      toast({ title: "已复制", description: "线路地址已复制到剪贴板" });
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
    return <PageLoading message="正在加载设置..." />;
  }

  return (
    <motion.div
      variants={container}
      initial="hidden"
      animate="show"
      className="space-y-6"
    >
      <div>
        <h1 className="text-3xl font-bold">个人设置</h1>
        <p className="text-muted-foreground">管理您的账户设置和偏好</p>
      </div>

      {/* 快速导航 */}
      <motion.div variants={item}>
        <div className="grid gap-4 sm:grid-cols-3">
          <a href="/settings/appearance" className="group">
            <Card className="glass-card cursor-pointer hover:shadow-lg transition-all h-full">
              <CardContent className="p-6 flex flex-col items-center justify-center text-center gap-3 h-full">
                <div className="p-3 rounded-lg bg-primary/10 group-hover:bg-primary/20 transition-colors">
                  <Palette className="h-6 w-6 text-primary" />
                </div>
                <div>
                  <h3 className="font-semibold">外观设置</h3>
                  <p className="text-sm text-muted-foreground">背景和头像</p>
                </div>
              </CardContent>
            </Card>
          </a>
        </div>
      </motion.div>

      {/* Account Info */}
      <motion.div variants={item}>
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <User className="h-5 w-5" />
              账户信息
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <Label className="text-muted-foreground">用户名</Label>
                <p className="mt-1 font-medium">{user?.username}</p>
              </div>
              <div>
                <Label className="text-muted-foreground">UID</Label>
                <p className="mt-1 font-medium">{user?.uid}</p>
              </div>
              <div>
                <Label className="text-muted-foreground">角色</Label>
                <div className="mt-1">
                  <Badge variant={user?.role === 0 ? "gradient" : "secondary"}>
                    {user?.role_name}
                  </Badge>
                </div>
              </div>
              <div>
                <Label className="text-muted-foreground font-medium">邮箱</Label>
                <div className="mt-1 flex items-center justify-between">
                  <p className="font-medium">{user?.email || "未设置"}</p>
                  <Button 
                    variant="outline" 
                    size="sm"
                    onClick={() => {
                      setEmailValue(user?.email || "");
                      setEditEmailOpen(true);
                    }}
                  >
                    修改
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
              Telegram 绑定
            </CardTitle>
            <CardDescription>
              绑定 Telegram 账号以便接收通知和使用机器人功能
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {systemInfo?.telegram_bot?.username && (
              <div className="flex items-center justify-between gap-3 rounded-lg border border-primary/20 bg-primary/5 px-4 py-3 text-sm">
                <div className="flex items-center gap-2 min-w-0">
                  <Bot className="h-4 w-4 text-primary shrink-0" />
                  <div className="min-w-0">
                    <p className="font-medium">本站 Bot</p>
                    <a
                      href={systemInfo.telegram_bot.url ?? `https://t.me/${systemInfo.telegram_bot.username}`}
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
                    href={systemInfo.telegram_bot.url ?? `https://t.me/${systemInfo.telegram_bot.username}`}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    打开 Bot
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
                    {telegramStatus?.bound ? "已绑定" : "未绑定"}
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
                    获取绑定码
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
                        解绑
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
                        提交换绑请求
                      </Button>
                    )}
                    {!telegramStatus.can_change && telegramStatus.pending_rebind_request && (
                      <Badge variant="outline" className="self-center">
                        换绑请求已提交，等待管理员处理
                      </Badge>
                    )}
                  </>
                )}
              </div>
            </div>
            {telegramStatus?.force_bind && (
              <p className="text-sm text-amber-500">
                ⚠️ 系统要求必须绑定 Telegram，无法解绑
              </p>
            )}
            {bindCode && !telegramStatus?.bound && (
              <div className="rounded-lg bg-blue-500/10 p-4 space-y-2">
                <p className="font-medium text-blue-500">绑定码已生成</p>
                <div className="flex flex-wrap items-center gap-2">
                  <code className="text-2xl font-mono font-bold tracking-widest bg-background/50 px-4 py-2 rounded-lg">
                    {bindCode}
                  </code>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      navigator.clipboard.writeText(`/bind ${bindCode}`);
                      toast({ title: "已复制到剪贴板", variant: "success" });
                    }}
                  >
                    复制命令
                  </Button>
                  {systemInfo?.telegram_bot?.url && (
                    <Button asChild variant="outline" size="sm">
                      <a
                        href={systemInfo.telegram_bot.url}
                        target="_blank"
                        rel="noopener noreferrer"
                      >
                        <Bot className="mr-2 h-4 w-4" />
                        打开 @{systemInfo.telegram_bot.username}
                      </a>
                    </Button>
                  )}
                </div>
                <p className="text-sm text-muted-foreground">
                  请在 {Math.floor(bindCodeExpiry / 60)} 分钟内向{" "}
                  {systemInfo?.telegram_bot?.username ? (
                    <a
                      href={systemInfo.telegram_bot.url ?? `https://t.me/${systemInfo.telegram_bot.username}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-primary hover:underline"
                    >
                      @{systemInfo.telegram_bot.username}
                    </a>
                  ) : (
                    "Telegram Bot"
                  )}{" "}
                  发送：<code className="bg-background/50 px-1.5 py-0.5 rounded">/bind {bindCode}</code>
                </p>
                <p className="text-xs text-muted-foreground">
                  绑定完成后请刷新此页面确认。
                </p>
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
              Emby 账号绑定
            </CardTitle>
            <CardDescription>
              绑定已有的 Emby 账号以使用媒体服务
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
                    {user?.emby_id ? "已绑定" : "未绑定"}
                  </p>
                  {user?.emby_id && (
                    <p className="text-sm text-muted-foreground break-all">
                      Emby ID: {user.emby_id}
                    </p>
                  )}
                  {user?.emby_id && (user.emby_username || user.username) && (
                    <p className="text-sm text-muted-foreground break-all">
                      Emby 用户名: {user.emby_username || user.username}
                    </p>
                  )}
                </div>
              </div>
              <div className="flex gap-2 shrink-0 sm:justify-end">
                {!user?.emby_id ? (
                  <Button
                    onClick={() => {
                      setCompletePendingEmby(Boolean(user?.pending_emby));
                      setEmbyUsername(user?.username || "");
                      setBindEmbyOpen(true);
                    }}
                    className="w-full sm:w-auto"
                  >
                    <LinkIcon className="mr-2 h-4 w-4" />
                    {user?.pending_emby ? "继续开通" : "绑定"}
                  </Button>
                ) : (
                  <Button
                    variant="destructive"
                    onClick={handleUnbindEmby}
                    disabled={isEmbyLoading}
                    className="w-full sm:w-auto"
                  >
                    {isEmbyLoading ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <Unlink className="mr-2 h-4 w-4" />
                    )}
                    解绑
                  </Button>
                )}
              </div>
            </div>
            {!user?.emby_id && (
              <p className="text-sm text-muted-foreground">
                {user?.pending_emby
                  ? "你已拥有 Emby 开通资格但尚未创建账号，可点击“继续开通”完成注册。"
                  : "如果您在 Emby 服务器中已有账号，可以在此绑定。绑定后即可使用该账号访问媒体内容。"}
              </p>
            )}
          </CardContent>
        </Card>
      </motion.div>

      {/* Media Library Visibility */}
      {user?.emby_id && (
        <motion.div variants={item}>
          <Card className="glass-card">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Eye className="h-5 w-5" />
                媒体库显隐
              </CardTitle>
              <CardDescription>
                管理员开放的指定媒体库，可在这里自行显示或隐藏
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {!libraryAccess ? (
                <div className="flex items-center gap-2 rounded-lg bg-accent/50 p-4 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  正在加载媒体库权限
                </div>
              ) : !libraryAccess.self_service_enabled ? (
                <div className="flex items-start gap-3 rounded-lg bg-accent/50 p-4 text-sm text-muted-foreground">
                  <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
                  管理员尚未为你开启媒体库自助显隐权限。
                </div>
              ) : libraryAccess.self_service_libraries.length === 0 ? (
                <div className="flex items-start gap-3 rounded-lg bg-accent/50 p-4 text-sm text-muted-foreground">
                  <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
                  管理员尚未开放可自助显隐的媒体库。
                </div>
              ) : (
                <div className="grid gap-3 md:grid-cols-2">
                  {libraryAccess.self_service_libraries.map((name) => {
                    const visible = isSelfServiceLibraryVisible(name);
                    return (
                      <div key={name} className="flex items-center justify-between gap-3 rounded-lg border bg-card/60 p-4">
                        <div className="min-w-0">
                          <p className="truncate font-medium">{name}</p>
                          <p className={visible ? "text-sm text-emerald-500" : "text-sm text-muted-foreground"}>
                            {visible ? "当前显示" : "当前隐藏"}
                          </p>
                        </div>
                        <Button
                          variant={visible ? "outline" : "default"}
                          size="sm"
                          disabled={isLibraryLoading}
                          onClick={() => void handleLibraryVisibility(name, visible ? "hide" : "show")}
                        >
                          {isLibraryLoading ? (
                            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                          ) : visible ? (
                            <EyeOff className="mr-2 h-4 w-4" />
                          ) : (
                            <Eye className="mr-2 h-4 w-4" />
                          )}
                          {visible ? "隐藏" : "显示"}
                        </Button>
                      </div>
                    );
                  })}
                </div>
              )}
              {libraryAccess?.default_hidden_libraries?.length ? (
                <p className="text-xs text-muted-foreground">
                  默认隐藏库：{libraryAccess.default_hidden_libraries.join("、")}
                </p>
              ) : null}
            </CardContent>
          </Card>
        </motion.div>
      )}

      {/* API Key Management */}
      {user?.emby_id && (
        <motion.div variants={item}>
          <Card className="glass-card">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Key className="h-5 w-5" />
                API Key 管理
              </CardTitle>
              <CardDescription>
                管理您的API Key用于外部接口控制账号
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex items-center justify-between rounded-lg bg-accent/50 p-4">
                <div>
                  <p className="font-medium">API Key 管理</p>
                  <p className="text-sm text-muted-foreground">
                    生成、启用、禁用或刷新您的API Key
                  </p>
                </div>
                <Button asChild>
                  <a href="/settings/apikey">
                    <Key className="mr-2 h-4 w-4" />
                    管理 API Key
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
              密码管理
            </CardTitle>
            <CardDescription>
              分别修改系统登录密码和绑定 Emby 密码。
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="rounded-xl border border-border p-4">
                <p className="text-sm font-medium">系统密码</p>
                <p className="text-sm text-muted-foreground mt-1">
                  修改网站登录密码，不会更改 Emby 账户密码。
                </p>
                <Button className="mt-4" onClick={() => setChangeSystemPwdOpen(true)}>
                  <Lock className="mr-2 h-4 w-4" />
                  修改系统密码
                </Button>
              </div>
              <div className="rounded-xl border border-border p-4">
                <p className="text-sm font-medium">Emby 密码</p>
                <p className="text-sm text-muted-foreground mt-1">
                  只更新当前绑定的 Emby 账号密码。
                </p>
                <Button className="mt-4" onClick={() => setChangeEmbyPwdOpen(true)} disabled={!user?.emby_id}>
                  <Lock className="mr-2 h-4 w-4" />
                  修改 Emby 密码
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
                Emby 已到期
              </CardTitle>
              <CardDescription>
                系统账号仍可登录；Emby 已禁用，续期后恢复线路访问。
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
              服务器线路
            </CardTitle>
            <CardDescription>
              选择延迟最低的线路连接 Emby，点击地址可复制
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex justify-end">
              <Button variant="outline" size="sm" onClick={() => void runLineLatencyTests()} disabled={isLatencyTesting}>
                {isLatencyTesting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
                重新测速
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
                          <p className="text-sm font-semibold">{line.name || `线路 ${index + 1}`}</p>
                          <p className="mt-1 break-all truncate font-mono text-xs text-muted-foreground">
                            {line.url}
                          </p>
                          <p className={`mt-1 ${renderLatencyClassName(key)}`}>
                            延迟：{renderLatencyText(key)}
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
              <p className="text-sm text-muted-foreground">暂无可用线路</p>
            )}

            {sortedWhitelistLineItems.length > 0 && (
              <>
                <Separator />
                <div>
                  <p className="mb-3 flex items-center gap-1.5 text-sm font-medium">
                    <Star className="h-4 w-4 text-yellow-500" />
                    专属线路
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
                              <p className="text-sm font-semibold">{line.name || `专属线路 ${index + 1}`}</p>
                              <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
                                {line.url}
                              </p>
                              <p className={`mt-1 ${renderLatencyClassName(key)}`}>
                                延迟：{renderLatencyText(key)}
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

      {/* Bangumi Sync */}
      {bangumiSyncEnabled && (
      <motion.div variants={item}>
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Key className="h-5 w-5" />
              Bangumi 点格子
            </CardTitle>
            <CardDescription>
              使用你自己的 Bangumi Access Token 同步观看进度，系统不会使用管理员全局 Token 代同步。
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <div className="space-y-0.5">
                  <Label>Bangumi 同步</Label>
                  <p className="text-sm text-muted-foreground">
                    启用后会在 Emby 看完番剧时自动同步 Bangumi 点格子。Token 只保存在服务端，不会回显明文。
                  </p>
                </div>
                <Switch checked={bgmMode} onCheckedChange={setBgmMode} />
              </div>
              <div className="space-y-2">
                <div className="space-y-2">
                  <Label>Bangumi Token</Label>
                  <Input
                    type="password"
                    placeholder={bgmTokenSet ? "已配置 Token，留空则保留当前值" : "请输入 Bangumi Token"}
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
                      {isBgmLoading ? "保存中..." : "保存 Bangumi 设置"}
                    </Button>
                    {bgmTokenSet && (
                      <Button
                        variant="outline"
                        onClick={handleClearBgmToken}
                        disabled={isBgmLoading}
                      >
                        清除 Token
                      </Button>
                    )}
                  </div>
                  {bgmTokenSet && (
                    <p className="text-sm text-muted-foreground">
                      当前已配置 Bangumi Token，可直接启用同步。
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

      {/* Bind Emby Dialog */}
      <Dialog open={bindEmbyOpen} onOpenChange={(open) => {
        setBindEmbyOpen(open);
        if (!open) setCompletePendingEmby(false);
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{completePendingEmby ? "开通 Emby 账号" : "绑定 Emby 账号"}</DialogTitle>
            <DialogDescription>
              {completePendingEmby ? "填写要创建的 Emby 用户名和密码" : "输入您在 Emby 服务器中的用户名和密码以验证身份"}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Emby 用户名</Label>
              <Input
                placeholder="例如：myembyuser"
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
              <Label>Emby 密码 {!completePendingEmby && <span className="text-xs text-muted-foreground">（如账号无密码请留空）</span>}</Label>
              <div className="relative">
                <Input
                  type={showEmbyPassword ? "text" : "password"}
                  placeholder="请输入 Emby 密码（可留空）"
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
                需要验证您的 Emby 账号凭据才能绑定（无密码账号请留空）
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
              取消
            </Button>
            <Button
              onClick={handleBindEmby}
              disabled={isEmbyLoading || !embyUsername.trim()}
            >
              {isEmbyLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {completePendingEmby ? "确认开通" : "确认绑定"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Email Dialog */}
      <Dialog open={editEmailOpen} onOpenChange={setEditEmailOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>设置/修改邮箱</DialogTitle>
            <DialogDescription>
              请输入您的邮箱地址，用于找回密码或接收重要通知
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>邮箱地址</Label>
              <Input
                type="email"
                placeholder="例如：example@gmail.com"
                value={emailValue}
                onChange={(e) => setEmailValue(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditEmailOpen(false)}>
              取消
            </Button>
            <Button onClick={handleUpdateEmail} disabled={isEmailLoading}>
              {isEmailLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认保存
            </Button>
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
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>修改系统密码</DialogTitle>
            <DialogDescription>
              修改系统登录密码，不会同步更改 Emby 密码。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>当前密码</Label>
              <div className="relative">
                <Input
                  type={showOldPwd ? "text" : "password"}
                  placeholder="请输入当前密码"
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
              <Label>新密码</Label>
              <div className="relative">
                <Input
                  type={showNewPwd ? "text" : "password"}
                  placeholder="至少 8 位，含大小写字母和数字"
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
                    ? `强度：${passwordStrengthLabel(systemPwdStrength.score).label}`
                    : systemPwdStrength.message}
                </p>
              )}
            </div>
            <div className="space-y-2">
              <Label>确认新密码</Label>
              <Input
                type="password"
                placeholder="再次输入新密码"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && oldPassword && newPassword && confirmPassword) {
                    handleChangeSystemPassword();
                  }
                }}
              />
              {confirmPassword && newPassword !== confirmPassword && (
                <p className="text-xs text-destructive">两次输入的密码不一致</p>
              )}
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setChangeSystemPwdOpen(false)}>
              取消
            </Button>
            <Button
              onClick={handleChangeSystemPassword}
              disabled={isSystemPwdLoading || !oldPassword || !systemPwdStrength.ok || newPassword !== confirmPassword}
            >
              {isSystemPwdLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认修改
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={changeEmbyPwdOpen} onOpenChange={(open) => {
        setChangeEmbyPwdOpen(open);
        if (!open) {
          setNewEmbyPassword("");
          setConfirmEmbyPassword("");
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>修改 Emby 密码</DialogTitle>
            <DialogDescription>
              只更新当前绑定的 Emby 账号密码。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>新密码</Label>
              <div className="relative">
                <Input
                  type={showNewEmbyPwd ? "text" : "password"}
                  placeholder="至少 8 位，含大小写字母和数字"
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
                    ? `强度：${passwordStrengthLabel(embyPwdStrength.score).label}`
                    : embyPwdStrength.message}
                </p>
              )}
            </div>
            <div className="space-y-2">
              <Label>确认新密码</Label>
              <div className="relative">
                <Input
                  type={showConfirmEmbyPwd ? "text" : "password"}
                  placeholder="再次输入新密码"
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
                <p className="text-xs text-destructive">两次输入的密码不一致</p>
              )}
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setChangeEmbyPwdOpen(false)}>
              取消
            </Button>
            <Button
              onClick={handleChangeEmbyPassword}
              disabled={isEmbyPwdLoading || !embyPwdStrength.ok || newEmbyPassword !== confirmEmbyPassword}
            >
              {isEmbyPwdLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认修改
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={rebindDialogOpen} onOpenChange={setRebindDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>提交 Telegram 换绑请求</DialogTitle>
            <DialogDescription>
              管理员批准后会解绑当前 Telegram，你可以重新绑定新的 Telegram 账号。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label>备注给管理员（可选）</Label>
            <Textarea
              value={rebindReason}
              onChange={(event) => setRebindReason(event.target.value.slice(0, 500))}
              placeholder="例如：原 Telegram 账号无法登录，需要换绑到新账号"
              rows={4}
            />
            <p className="text-xs text-muted-foreground">{rebindReason.length}/500</p>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRebindDialogOpen(false)} disabled={isRebindLoading}>
              取消
            </Button>
            <Button onClick={handleRequestTelegramRebind} disabled={isRebindLoading}>
              {isRebindLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              提交申请
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}
