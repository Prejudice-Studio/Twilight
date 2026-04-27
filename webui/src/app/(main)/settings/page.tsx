"use client";

import React, { useCallback, useEffect, useState } from "react";
import { motion } from "framer-motion";
import {
  User,
  Shield,
  Bell,
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
  AlertTriangle,
  Palette,
  Lock,
  Globe,
  Star,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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
import { api, type UserSettings, type TelegramStatus, type NsfwStatus, type EmbyStatus } from "@/lib/api";
import {
  Alert,
  AlertDescription,
} from "@/components/ui/alert";

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
  const [settings, setSettings] = useState<UserSettings | null>(null);
  const [telegramStatus, setTelegramStatus] = useState<TelegramStatus | null>(null);
  const [nsfwStatus, setNsfwStatus] = useState<NsfwStatus | null>(null);
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

  // Emby dialogs
  const [bindEmbyOpen, setBindEmbyOpen] = useState(false);
  const [embyUsername, setEmbyUsername] = useState("");
  const [embyPassword, setEmbyPassword] = useState("");
  const [showEmbyPassword, setShowEmbyPassword] = useState(false);
  const [isEmbyLoading, setIsEmbyLoading] = useState(false);

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

  // Emby URLs
  const [embyLines, setEmbyLines] = useState<Array<{ name: string; url: string }>>([]);
  const [whitelistLines, setWhitelistLines] = useState<Array<{ name: string; url: string }>>([]);
  const [copiedIndex, setCopiedIndex] = useState<string | null>(null);

  const loadSettingsResource = useCallback(async () => {
    const [settingsRes, tgRes, nsfwRes] = await Promise.all([
      api.getMySettings(),
      api.getTelegramStatus(),
      api.getNsfwStatus(),
    ]);
    if (settingsRes.success && settingsRes.data) {
      setSettings(settingsRes.data);
      setBgmMode(settingsRes.data.bgm_mode);
      setBgmTokenSet(settingsRes.data.bgm_token_set ?? false);
      setEmbyStatus(settingsRes.data.emby_status ?? null);
    }
    if (tgRes.success && tgRes.data) {
      setTelegramStatus(tgRes.data);
    }
    if (nsfwRes.success && nsfwRes.data) {
      setNsfwStatus(nsfwRes.data);
    }
    return true;
  }, []);

  const {
    isLoading,
    error,
    execute: loadData,
  } = useAsyncResource(loadSettingsResource, { immediate: true });

  const handleToggleAutoRenew = async (enabled: boolean) => {
    try {
      const res = await api.updateAutoRenew(enabled);
      if (res.success) {
        setSettings((prev) => prev ? { ...prev, auto_renew: enabled } : null);
        toast({
          title: enabled ? "已开启自动续期" : "已关闭自动续期",
          variant: "success",
        });
      } else {
        toast({ title: "操作失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "操作失败", description: error.message, variant: "destructive" });
    }
  };

  const handleSaveBgmSettings = async () => {
    if (bgmMode && !bgmToken && !bgmTokenSet) {
      toast({ title: "请输入 Bangumi Token", description: "启用 BGM 同步前需要填写个人 Token", variant: "destructive" });
      return;
    }

    setIsBgmLoading(true);
    try {
      const res = await api.updateMySettings({
        bgm_mode: bgmMode,
        bgm_token: bgmToken || undefined,
      });

      if (res.success) {
        setBgmToken("");
        setBgmTokenSet(bgmTokenSet || Boolean(bgmToken));
        setSettings((prev) => prev ? { ...prev, bgm_mode: bgmMode, bgm_token_set: bgmTokenSet || Boolean(bgmToken) } : prev);
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

  const handleToggleNsfwLibrary = async (libraryName: string, enabled: boolean) => {
    try {
      const res = await api.toggleNsfw(enabled, [libraryName]);
      if (res.success) {
        setNsfwStatus((prev) => {
          if (!prev) return null;
          const updatedLibraries = prev.libraries.map((lib) =>
            lib.name === libraryName ? { ...lib, enabled } : lib
          );
          const anyEnabled = updatedLibraries.some((lib) => lib.enabled);
          return { ...prev, enabled: anyEnabled, libraries: updatedLibraries };
        });
        toast({
          title: enabled ? `已开启「${libraryName}」` : `已关闭「${libraryName}」`,
          variant: "success",
        });
      } else {
        toast({ title: "操作失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "操作失败", description: error.message, variant: "destructive" });
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
    setIsRebindLoading(true);
    try {
      const res = await api.requestTelegramRebind();
      if (res.success) {
        toast({ title: "换绑请求已提交", description: res.message, variant: "success" });
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
    const password = embyPassword.trim();
    
    if (!username) {
      toast({ title: "请输入 Emby 用户名", variant: "destructive" });
      return;
    }

    if (!password) {
      toast({ title: "请输入 Emby 密码", variant: "destructive" });
      return;
    }

    setIsEmbyLoading(true);
    try {
      // 确保密码被正确传递
      console.log("绑定 Emby 账号:", { username, passwordLength: password.length });
      const res = await api.bindEmbyAccount(username, password);
      if (res.success) {
        toast({ title: "绑定成功", variant: "success" });
        setBindEmbyOpen(false);
        setEmbyUsername("");
        setEmbyPassword("");
        loadData();
        fetchUser();
      } else {
        toast({ title: "绑定失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      console.error("绑定失败:", error);
      toast({ title: "绑定失败", description: error.message, variant: "destructive" });
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
        loadData();
        fetchUser();
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
    if (newPassword.length < 6) {
      toast({ title: "新密码长度至少 6 位", variant: "destructive" });
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
    if (newEmbyPassword.length < 6) {
      toast({ title: "新密码长度至少 6 位", variant: "destructive" });
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

  // 初始加载线路
  useEffect(() => {
    api.getEmbyUrls().then((res) => {
      if (res.success && res.data) {
        setEmbyLines(res.data.lines);
        setWhitelistLines(res.data.whitelist_lines || []);
      }
    });
  }, []);

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
                        onClick={handleRequestTelegramRebind}
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
                <div className="flex items-center gap-3">
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
                </div>
                <p className="text-sm text-muted-foreground">
                  请在 {Math.floor(bindCodeExpiry / 60)} 分钟内向 Telegram Bot 发送：<code className="bg-background/50 px-1.5 py-0.5 rounded">/bind {bindCode}</code>
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
            <div className="flex items-center justify-between rounded-lg bg-accent/50 p-4">
              <div className="flex items-center gap-3">
                <div className={`flex h-10 w-10 items-center justify-center rounded-full ${user?.emby_id ? 'bg-emerald-500/20' : 'bg-muted'}`}>
                  {user?.emby_id ? (
                    <Check className="h-5 w-5 text-emerald-500" />
                  ) : (
                    <X className="h-5 w-5 text-muted-foreground" />
                  )}
                </div>
                <div>
                  <p className="font-medium">
                    {user?.emby_id ? "已绑定" : "未绑定"}
                  </p>
                  {user?.emby_id && (
                    <p className="text-sm text-muted-foreground">
                      Emby ID: {user.emby_id}
                    </p>
                  )}
                  {user?.username && user?.emby_id && (
                    <p className="text-sm text-muted-foreground">
                      用户名: {user.username}
                    </p>
                  )}
                </div>
              </div>
              <div className="flex gap-2">
                {!user?.emby_id ? (
                  <Button onClick={() => setBindEmbyOpen(true)}>
                    <LinkIcon className="mr-2 h-4 w-4" />
                    绑定
                  </Button>
                ) : (
                  <Button
                    variant="destructive"
                    onClick={handleUnbindEmby}
                    disabled={isEmbyLoading}
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
                如果您在 Emby 服务器中已有账号，可以在此绑定。绑定后即可使用该账号访问媒体内容。
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

      {/* 服务器线路 */}
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
            {embyLines.length > 0 ? (
              <div className="grid gap-3 sm:grid-cols-2">
                {embyLines.map((line, i) => {
                  const key = `line-${i}`;
                  return (
                    <div
                      key={key}
                      className="group relative h-full rounded-xl border bg-card p-4 transition-colors hover:bg-accent/50 dark:border-slate-700/70 dark:bg-slate-950/60 dark:hover:bg-slate-900/80"
                    >
                      <div className="flex items-start justify-between gap-2">
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-semibold">{line.name || `线路 ${i + 1}`}</p>
                          <p className="mt-1 break-all truncate font-mono text-xs text-muted-foreground">
                            {line.url}
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

            {whitelistLines.length > 0 && (
              <>
                <Separator />
                <div>
                  <p className="mb-3 flex items-center gap-1.5 text-sm font-medium">
                    <Star className="h-4 w-4 text-yellow-500" />
                    专属线路
                  </p>
                  <div className="grid gap-3 sm:grid-cols-2">
                    {whitelistLines.map((line, i) => {
                      const key = `wl-${i}`;
                      return (
                        <div
                          key={key}
                          className="group relative rounded-xl border border-yellow-500/20 bg-yellow-500/5 p-4 transition-colors hover:bg-yellow-500/10 dark:border-yellow-500/30 dark:bg-yellow-500/10 dark:hover:bg-yellow-500/20"
                        >
                          <div className="flex items-start justify-between gap-2">
                            <div className="min-w-0 flex-1">
                              <p className="text-sm font-semibold">{line.name || `专属线路 ${i + 1}`}</p>
                              <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
                                {line.url}
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

      {/* Preferences */}
      <motion.div variants={item}>
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Shield className="h-5 w-5" />
              偏好设置
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-6">
            {/* Auto Renew */}
            {settings?.system_config.auto_renew_enabled && (
              <div className="flex items-center justify-between">
                <div className="space-y-0.5">
                  <Label>自动续期</Label>
                  <p className="text-sm text-muted-foreground">
                    余额充足时自动使用积分续期（{settings.system_config.auto_renew_cost} 积分/{settings.system_config.auto_renew_days} 天）
                  </p>
                </div>
                <Switch
                  checked={settings?.auto_renew}
                  onCheckedChange={handleToggleAutoRenew}
                />
              </div>
            )}

            <Separator />

            {/* Bangumi Sync */}
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <div className="space-y-0.5">
                  <Label>Bangumi 同步</Label>
                  <p className="text-sm text-muted-foreground">
                    启用后会将观看记录同步到 Bangumi，需要填写个人 Token。
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
                  <Button
                    onClick={handleSaveBgmSettings}
                    disabled={isBgmLoading}
                  >
                    {isBgmLoading ? "保存中..." : "保存 Bangumi 设置"}
                  </Button>
                  {bgmTokenSet && (
                    <p className="text-sm text-muted-foreground">
                      当前已配置 Bangumi Token，可直接启用同步。
                    </p>
                  )}
                </div>
              </div>
            </div>

            <Separator />

            {/* NSFW Management */}
            {settings?.system_config.nsfw_library_configured && (
              <div className="space-y-3">
                <div className="space-y-0.5">
                  <Label className="flex items-center gap-2">
                    NSFW 内容显示
                    {nsfwStatus?.has_permission ? (
                      <Badge variant="default" className="text-xs">
                        有权限
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="text-xs">
                        无权限
                      </Badge>
                    )}
                  </Label>
                  <p className="text-sm text-muted-foreground">
                    {nsfwStatus?.message || "控制是否在媒体库中显示 NSFW 内容"}
                  </p>
                </div>
                {!nsfwStatus?.has_permission ? (
                  <Alert>
                    <AlertTriangle className="h-4 w-4" />
                    <AlertDescription>
                      您没有 NSFW 库的访问权限，请联系管理员授予权限
                    </AlertDescription>
                  </Alert>
                ) : nsfwStatus?.libraries && nsfwStatus.libraries.length > 0 ? (
                  <div className="space-y-2">
                    {nsfwStatus.libraries.map((lib) => (
                      <div
                        key={lib.name}
                        className="flex items-center justify-between rounded-lg border p-3"
                      >
                        <div>
                          <p className="text-sm font-medium">{lib.name}</p>
                          <p className="text-xs text-muted-foreground">
                            {lib.enabled ? "已启用" : "已禁用"}
                          </p>
                        </div>
                        <Switch
                          checked={lib.enabled}
                          onCheckedChange={(checked) =>
                            handleToggleNsfwLibrary(lib.name, checked)
                          }
                          disabled={!nsfwStatus?.can_toggle}
                        />
                      </div>
                    ))}
                    <Alert>
                      <AlertDescription className="text-xs">
                        <strong>提示：</strong>此设置仅控制各 NSFW 库内容的显示状态，不影响您的访问权限。权限由管理员在 Emby 中管理。
                      </AlertDescription>
                    </Alert>
                  </div>
                ) : (
                  <Alert>
                    <AlertDescription className="text-xs">
                      暂无可用的 NSFW 库
                    </AlertDescription>
                  </Alert>
                )}
              </div>
            )}

            <Separator />

            {/* Device Limit Info */}
            {settings?.system_config.device_limit_enabled && (
              <div className="rounded-lg bg-accent/50 p-4">
                <p className="font-medium">设备限制</p>
                <p className="text-sm text-muted-foreground">
                  最多 {settings.system_config.max_devices} 台设备，
                  同时 {settings.system_config.max_streams} 路播放
                </p>
              </div>
            )}
          </CardContent>
        </Card>
      </motion.div>

      {/* Bind Emby Dialog */}
      <Dialog open={bindEmbyOpen} onOpenChange={setBindEmbyOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>绑定 Emby 账号</DialogTitle>
            <DialogDescription>
              输入您在 Emby 服务器中的用户名和密码以验证身份
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
                  if (e.key === "Enter" && embyUsername.trim() && embyPassword.trim()) {
                    handleBindEmby();
                  }
                }}
              />
            </div>
            <div className="space-y-2">
              <Label>Emby 密码</Label>
              <div className="relative">
                <Input
                  type={showEmbyPassword ? "text" : "password"}
                  placeholder="请输入 Emby 密码"
                  value={embyPassword}
                  onChange={(e) => setEmbyPassword(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && embyUsername.trim() && embyPassword.trim()) {
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
                需要验证您的 Emby 账号凭据才能绑定
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button 
              variant="outline" 
              onClick={() => {
                setBindEmbyOpen(false);
                setEmbyUsername("");
                setEmbyPassword("");
              }}
            >
              取消
            </Button>
            <Button 
              onClick={handleBindEmby} 
              disabled={isEmbyLoading || !embyUsername.trim() || !embyPassword.trim()}
            >
              {isEmbyLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认绑定
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
                  placeholder="至少 6 位"
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
              disabled={isSystemPwdLoading || !oldPassword || !newPassword || newPassword !== confirmPassword}
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
                  placeholder="至少 6 位"
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
              disabled={isEmbyPwdLoading || !newEmbyPassword || newEmbyPassword !== confirmEmbyPassword}
            >
              {isEmbyPwdLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认修改
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}