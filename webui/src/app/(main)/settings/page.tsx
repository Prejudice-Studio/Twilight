"use client";

import React, { useCallback, useEffect, useState } from "react";
import { motion } from "framer-motion";
import {
  User,
  Shield,
  Bell,
  RefreshCw,
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
import { api, type UserSettings, type TelegramStatus, type NsfwStatus } from "@/lib/api";
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

  // Telegram dialogs
  const [bindTgOpen, setBindTgOpen] = useState(false);
  const [changeTgOpen, setChangeTgOpen] = useState(false);
  const [newTelegramId, setNewTelegramId] = useState("");
  const [isTgLoading, setIsTgLoading] = useState(false);

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

  const loadSettingsResource = useCallback(async () => {
    const [settingsRes, tgRes, nsfwRes] = await Promise.all([
      api.getMySettings(),
      api.getTelegramStatus(),
      api.getNsfwStatus(),
    ]);
    if (settingsRes.success && settingsRes.data) {
      setSettings(settingsRes.data);
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

  const handleToggleNsfw = async (enabled: boolean) => {
    try {
      const res = await api.toggleNsfw(enabled);
      if (res.success) {
        setNsfwStatus((prev) => prev ? { ...prev, enabled } : null);
        toast({
          title: enabled ? "已开启 NSFW 内容" : "已关闭 NSFW 内容",
          variant: "success",
        });
      } else {
        toast({ title: "操作失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "操作失败", description: error.message, variant: "destructive" });
    }
  };

  const handleBindTelegram = async () => {
    if (!newTelegramId) {
      toast({ title: "请输入 Telegram ID", variant: "destructive" });
      return;
    }

    setIsTgLoading(true);
    try {
      const res = await api.bindTelegram(parseInt(newTelegramId));
      if (res.success) {
        toast({ title: "绑定成功", variant: "success" });
        setBindTgOpen(false);
        setNewTelegramId("");
        loadData();
        fetchUser();
      } else {
        toast({ title: "绑定失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "绑定失败", description: error.message, variant: "destructive" });
    } finally {
      setIsTgLoading(false);
    }
  };

  const handleUnbindTelegram = async () => {
    setIsTgLoading(true);
    try {
      const res = await api.unbindTelegram();
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

  const handleChangeTelegram = async () => {
    if (!newTelegramId) {
      toast({ title: "请输入新的 Telegram ID", variant: "destructive" });
      return;
    }

    setIsTgLoading(true);
    try {
      const res = await api.changeTelegram(parseInt(newTelegramId));
      if (res.success) {
        toast({ title: "换绑成功", variant: "success" });
        setChangeTgOpen(false);
        setNewTelegramId("");
        loadData();
        fetchUser();
      } else {
        toast({ title: "换绑失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "换绑失败", description: error.message, variant: "destructive" });
    } finally {
      setIsTgLoading(false);
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
                  <Button onClick={() => setBindTgOpen(true)}>
                    <LinkIcon className="mr-2 h-4 w-4" />
                    绑定
                  </Button>
                ) : (
                  <>
                    <Button variant="outline" onClick={() => setChangeTgOpen(true)}>
                      换绑
                    </Button>
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
                  </>
                )}
              </div>
            </div>
            {telegramStatus?.force_bind && (
              <p className="text-sm text-amber-500">
                ⚠️ 系统要求必须绑定 Telegram，无法解绑
              </p>
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

            {/* NSFW Management */}
            {settings?.system_config.nsfw_library_configured && (
              <div className="space-y-3">
                <div className="flex items-center justify-between">
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
                      {nsfwStatus?.enabled && (
                        <Badge variant="secondary" className="text-xs">
                          已启用
                        </Badge>
                      )}
                    </Label>
                    <p className="text-sm text-muted-foreground">
                      {nsfwStatus?.message || "控制是否在媒体库中显示 NSFW 内容"}
                    </p>
                    {nsfwStatus?.has_permission && (
                      <div className="mt-2 space-y-1">
                        <p className="text-xs text-muted-foreground">
                          当前状态: <span className="font-medium">{nsfwStatus?.enabled ? "已启用" : "已禁用"}</span>
                        </p>
                        <p className="text-xs text-muted-foreground">
                          说明: {nsfwStatus?.enabled ? "NSFW 内容将在媒体库中显示" : "NSFW 内容将被隐藏"}
                        </p>
                      </div>
                    )}
                  </div>
                  <Switch
                    checked={nsfwStatus?.enabled || false}
                    onCheckedChange={handleToggleNsfw}
                    disabled={!nsfwStatus?.can_toggle}
                  />
                </div>
                {!nsfwStatus?.has_permission ? (
                  <Alert>
                    <AlertTriangle className="h-4 w-4" />
                    <AlertDescription>
                      您没有 NSFW 库的访问权限，请联系管理员授予权限
                    </AlertDescription>
                  </Alert>
                ) : (
                  <Alert>
                    <AlertDescription className="text-xs">
                      <strong>提示：</strong>此设置仅控制 NSFW 内容的显示状态，不影响您的访问权限。权限由管理员在 Emby 中管理。
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

      {/* Bind Telegram Dialog */}
      <Dialog open={bindTgOpen} onOpenChange={setBindTgOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>绑定 Telegram</DialogTitle>
            <DialogDescription>
              输入您的 Telegram 用户 ID（可通过 @userinfobot 获取）
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Telegram ID</Label>
              <Input
                type="number"
                placeholder="例如：123456789"
                value={newTelegramId}
                onChange={(e) => setNewTelegramId(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setBindTgOpen(false)}>
              取消
            </Button>
            <Button onClick={handleBindTelegram} disabled={isTgLoading}>
              {isTgLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认绑定
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Change Telegram Dialog */}
      <Dialog open={changeTgOpen} onOpenChange={setChangeTgOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>换绑 Telegram</DialogTitle>
            <DialogDescription>
              输入新的 Telegram 用户 ID
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>新 Telegram ID</Label>
              <Input
                type="number"
                placeholder="例如：123456789"
                value={newTelegramId}
                onChange={(e) => setNewTelegramId(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setChangeTgOpen(false)}>
              取消
            </Button>
            <Button onClick={handleChangeTelegram} disabled={isTgLoading}>
              {isTgLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认换绑
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

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
    </motion.div>
  );
}

