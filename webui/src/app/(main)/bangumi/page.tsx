"use client";

import { useCallback, useMemo, useState } from "react";
import { motion } from "framer-motion";
import Link from "next/link";
import { BookOpen, RefreshCw, Trash2, Loader2, CheckCircle2, XCircle, Clock, AlertCircle, Heart, Tv, ExternalLink, User as UserIcon, Star, ListChecks, Eye, EyeOff, BookmarkX } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { api, type BangumiSyncStatus, type BangumiSyncLog } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { API_BASE } from "@/lib/api-request";
import { useAuthStore } from "@/store/auth";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";

function formatTime(unix: number): string {
  if (!unix) return "";
  return new Date(unix * 1000).toLocaleString();
}

const NAV_ITEMS = [
  { type: 3, label: "在看", icon: Tv, color: "text-blue-500 bg-blue-500/10", href: "/bangumi/collections/3" },
  { type: 2, label: "看过", icon: CheckCircle2, color: "text-green-500 bg-green-500/10", href: "/bangumi/collections/2" },
  { type: 1, label: "想看", icon: Heart, color: "text-red-500 bg-red-500/10", href: "/bangumi/collections/1" },
  { type: 4, label: "搁置", icon: BookmarkX, color: "text-yellow-500 bg-yellow-500/10", href: "/bangumi/collections/4" },
  { type: 5, label: "抛弃", icon: EyeOff, color: "text-muted-foreground bg-accent/20", href: "/bangumi/collections/5" },
];

function activityText(item: any) {
  const name = item.subject?.name_cn || item.subject?.name || "未知条目";
  switch (item.type) {
    case 1: return `想看了 ${name}`;
    case 2: return `看过了 ${name}`;
    case 3: return item.ep_status ? `更新 ${name} 进度到第 ${item.ep_status} 话` : `开始看 ${name}`;
    case 4: return `搁置了 ${name}`;
    case 5: return `抛弃了 ${name}`;
    default: return `更新了 ${name}`;
  }
}

function activityIcon(type: number) {
  switch (type) {
    case 1: return <Heart className="h-3 w-3 text-red-500" />;
    case 2: return <CheckCircle2 className="h-3 w-3 text-green-500" />;
    case 3: return <Tv className="h-3 w-3 text-blue-500" />;
    case 4: return <BookmarkX className="h-3 w-3 text-yellow-500" />;
    case 5: return <EyeOff className="h-3 w-3 text-muted-foreground" />;
    default: return <Clock className="h-3 w-3 text-muted-foreground" />;
  }
}

export default function BangumiPage() {
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const { t } = useI18n();
  const { user, fetchUser } = useAuthStore();

  const [status, setStatus] = useState<BangumiSyncStatus | null>(null);
  const [syncing, setSyncing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [bgmMode, setBgmMode] = useState(false);
  const [bgmManageMode, setBgmManageMode] = useState(false);
  const [bgmToken, setBgmToken] = useState("");
  const [logs, setLogs] = useState<BangumiSyncLog[]>([]);
  const [bgmMe, setBgmMe] = useState<any>(null);

  const loadResource = useCallback(async () => {
    const res = await api.getBangumiSyncStatus();
    if (res.success && res.data) {
      setStatus(res.data);
      setBgmMode(res.data.bgm_mode);
      setBgmManageMode(res.data.bgm_manage_mode);
      setLogs(res.data.recent_logs || []);

      if (res.data.bgm_token_set) {
        try {
          const meRes = await api.getBangumiMe();
          if (meRes.success && meRes.data) {
            setBgmMe(meRes.data);
          } else {
            setBgmMe(null);
          }
        } catch (e) {
          console.error("加载 Bangumi 用户数据失败", e);
        }
      } else {
        setBgmMe(null);
      }
      return true;
    }
    throw new Error(res.message || "加载失败");
  }, []);

  const { isLoading, error, execute: reload } = useAsyncResource(loadResource, { immediate: true });

  const recentActivity = useMemo(() => {
    if (!bgmMe) return [];
    const all = [
      ...(bgmMe.watching || []),
      ...(bgmMe.collected || []),
      ...(bgmMe.wishlist || []),
      ...(bgmMe.on_hold || []),
      ...(bgmMe.dropped || []),
    ];
    all.sort((a: any, b: any) => (b.updated_at || 0) - (a.updated_at || 0));
    return all.slice(0, 8);
  }, [bgmMe]);

  const handleSync = async () => {
    setSyncing(true);
    try {
      const res = await api.triggerBangumiSync();
      if (res.success && res.data) {
        toast({ title: t("bangumi.syncCompleted"), description: `${t("bangumi.syncedCount")}: ${res.data.synced}` });
        await reload();
      } else {
        toast({ title: t("bangumi.syncFailed"), description: res.message, variant: "destructive" });
      }
    } catch {
      toast({ title: t("bangumi.syncError"), variant: "destructive" });
    } finally {
      setSyncing(false);
    }
  };

  const handleSaveSettings = async () => {
    const syncEnabled = status?.sync_enabled === true;
    const manageEnabled = status?.manage_enabled === true;
    if ((syncEnabled && bgmMode || manageEnabled && bgmManageMode) && !bgmToken && !status?.bgm_token_set) {
      toast({ title: t("bangumi.tokenRequired"), variant: "destructive" });
      return;
    }
    setSaving(true);
    try {
      const payload: Parameters<typeof api.updateMySettings>[0] = { bgm_token: bgmToken || undefined };
      if (syncEnabled) payload.bgm_mode = bgmMode;
      if (manageEnabled) payload.bgm_manage_mode = bgmManageMode;
      const res = await api.updateMySettings(payload);
      if (res.success) {
        toast({ title: t("bangumi.settingsSaved") });
        await fetchUser();
        await reload();
      } else {
        toast({ title: t("bangumi.saveFailed"), description: res.message, variant: "destructive" });
      }
    } catch {
      toast({ title: t("bangumi.saveError"), variant: "destructive" });
    } finally {
      setSaving(false);
    }
  };

  const handleClearHistory = async () => {
    const ok = await confirm({
      title: t("bangumi.clearConfirmTitle"),
      description: t("bangumi.clearConfirmDescription"),
      tone: "danger",
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    try {
      const res = await api.clearBangumiSyncHistory();
      if (res.success) {
        toast({ title: t("bangumi.cleared") });
        await reload();
      } else {
        toast({ title: t("common.deleteFailed"), description: res.message, variant: "destructive" });
      }
    } catch {
      toast({ title: t("common.deleteFailed"), variant: "destructive" });
    }
  };

  const handleClearToken = async () => {
    try {
      const res = await api.updateMySettings({ bgm_mode: false, bgm_manage_mode: false, bgm_token: "" });
      if (res.success) {
        toast({ title: t("bangumi.tokenCleared") });
        setBgmMode(false);
        setBgmManageMode(false);
        setBgmToken("");
        await fetchUser();
        await reload();
      }
    } catch {
      toast({ title: t("bangumi.clearFailed"), variant: "destructive" });
    }
  };

  const syncEnabled = status?.sync_enabled === true;
  const manageEnabled = status?.manage_enabled === true;

  if (error) {
    return (
      <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
        <Card>
          <CardContent className="pt-6 flex flex-col items-center gap-3">
            <AlertCircle className="h-8 w-8 text-destructive" />
            <p className="text-sm text-muted-foreground">{String(error)}</p>
            <Button variant="outline" onClick={() => { void reload(); }}>{t("common.retry")}</Button>
          </CardContent>
        </Card>
      </motion.div>
    );
  }

  if (isLoading) {
    return (
      <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
        <Card>
          <CardContent className="pt-6 flex items-center justify-center">
            <Loader2 className="h-6 w-6 animate-spin text-primary" />
          </CardContent>
        </Card>
      </motion.div>
    );
  }

  return (
    <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold flex items-center gap-2">
          <BookOpen className="h-6 w-6" />
          {t("bangumi.title")}
        </h1>
        <p className="text-sm text-muted-foreground mt-1">{t("bangumi.description")}</p>
      </div>

      {bgmMe?.expired && (
        <Alert variant="destructive" className="border-red-500 bg-red-500/10">
          <AlertCircle className="h-5 w-5 text-red-600 animate-pulse" />
          <AlertTitle className="font-extrabold text-red-600 dark:text-red-400">
            您的 Bangumi 访问令牌已过期 / Access Token Expired
          </AlertTitle>
          <AlertDescription className="text-xs text-muted-foreground leading-relaxed mt-1">
            请在下方设置面板中填入重新申请的有效 Token。
          </AlertDescription>
        </Alert>
      )}

      {bgmMe && !bgmMe.expired && bgmMe.me && (
        <>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            <Card className="glass-card md:col-span-1">
              <CardHeader className="pb-3">
                <CardTitle className="text-base font-bold flex items-center gap-2">
                  <UserIcon className="h-4 w-4 text-primary" />
                  Bangumi 账号
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <div className="flex items-center gap-3">
                  {bgmMe.me.avatar?.large ? (
                    // eslint-disable-next-line @next/next/no-img-element -- BGM user avatar URL
                    <img src={bgmMe.me.avatar.large} className="h-14 w-14 rounded-full border-2 border-primary object-cover" alt={bgmMe.me.nickname} loading="lazy" referrerPolicy="no-referrer" />
                  ) : (
                    <div className="h-14 w-14 rounded-full border-2 border-primary bg-muted flex items-center justify-center"><UserIcon className="h-6 w-6 text-muted-foreground" /></div>
                  )}
                  <div className="min-w-0 flex-1">
                    <h3 className="font-bold truncate text-sm">{bgmMe.me.nickname || "神秘用户"}</h3>
                    <p className="text-xs text-muted-foreground truncate">@{bgmMe.me.username || "unknown"}</p>
                    <p className="text-[10px] text-muted-foreground">BGM UID: {bgmMe.me.id}</p>
                  </div>
                </div>
                {bgmMe.me.sign ? (
                  <div className="rounded-lg bg-accent/20 p-2.5 text-xs italic text-muted-foreground line-clamp-2">&ldquo;{bgmMe.me.sign}&rdquo;</div>
                ) : null}
                <a href={`https://bgm.tv/user/${bgmMe.me.username}`} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 text-xs text-primary hover:underline">
                  前往 Bangumi 主页 <ExternalLink className="h-3 w-3" />
                </a>
              </CardContent>
            </Card>

            <Card className="glass-card md:col-span-2">
              <CardHeader className="pb-3">
                <CardTitle className="text-base font-bold flex items-center gap-2">
                  <Clock className="h-4 w-4 text-primary" />
                  最近动态
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-0">
                {recentActivity.length === 0 ? (
                  <p className="text-xs text-muted-foreground py-4">暂无动态</p>
                ) : (
                  <div className="space-y-2 max-h-[420px] overflow-y-auto pr-1">
                    {recentActivity.map((item: any, idx: number) => (
                      <div key={`${item.subject_id}-${idx}`} className="flex gap-2.5 text-xs relative">
                        <div className="relative flex flex-col items-center flex-shrink-0 pt-1">
                          {activityIcon(item.type)}
                          {idx < recentActivity.length - 1 && <div className="w-px flex-1 bg-border/20 mt-0.5" />}
                        </div>
                        <div className="min-w-0 flex-1 pb-2 border-b border-border/10 last:border-0">
                          <Link href={`/bangumi/collections/${item.type > 0 && item.type <= 5 ? item.type : 3}`} className="text-muted-foreground leading-relaxed line-clamp-1 hover:text-primary transition-colors">
                            {activityText(item)}
                          </Link>
                          {item.updated_at ? (
                            <p className="text-[10px] text-muted-foreground/50 mt-0.5">{formatTime(item.updated_at)}</p>
                          ) : null}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </div>

          <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
            {NAV_ITEMS.map((nav) => {
              const total = nav.type === 1 ? bgmMe.wishlist_total
                : nav.type === 2 ? bgmMe.collected_total
                : nav.type === 3 ? bgmMe.watching_total
                : nav.type === 4 ? (bgmMe.on_hold_total ?? 0)
                : (bgmMe.dropped_total ?? 0);
              return (
                <Link key={nav.type} href={nav.href} prefetch={false}>
                  <Card className="hover:bg-accent/30 transition-colors cursor-pointer border-border/40">
                    <CardContent className="p-4 flex flex-col items-center gap-2 text-center">
                      <div className={`h-10 w-10 rounded-full flex items-center justify-center ${nav.color}`}>
                        <nav.icon className="h-5 w-5" />
                      </div>
                      <div>
                        <p className="text-xs font-bold text-foreground">{nav.label}</p>
                        <p className="text-lg font-extrabold text-foreground">{total ?? 0}</p>
                      </div>
                    </CardContent>
                  </Card>
                </Link>
              );
            })}
          </div>
        </>
      )}

      {syncEnabled && (
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <RefreshCw className="h-4 w-4" />
              {t("bangumi.syncStatus")}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
              <div className="rounded-lg bg-accent/50 p-3 text-center">
                <div className="text-xl font-bold">{status?.total_records ?? 0}</div>
                <div className="text-xs text-muted-foreground">总记录</div>
              </div>
              <div className="rounded-lg bg-accent/50 p-3 text-center">
                <div className="text-xl font-bold text-green-500">{status?.synced_count ?? 0}</div>
                <div className="text-xs text-muted-foreground">已同步</div>
              </div>
              <div className="rounded-lg bg-accent/50 p-3 text-center">
                <div className="text-xl font-bold">{status?.sync_ready ? "就绪" : "未就绪"}</div>
                <div className="text-xs text-muted-foreground">状态</div>
              </div>
              <div className="rounded-lg bg-accent/50 p-3 text-center">
                <div className="text-xl font-bold">{status?.bgm_token_set ? "已配置" : "未配置"}</div>
                <div className="text-xs text-muted-foreground">Token</div>
              </div>
            </div>
            <div className="flex gap-2">
              <Button onClick={handleSync} disabled={syncing || !status?.sync_ready}>
                {syncing ? <Loader2 className="h-4 w-4 animate-spin mr-1" /> : <RefreshCw className="h-4 w-4 mr-1" />}
                开始同步
              </Button>
              {logs.length > 0 && (
                <Button variant="outline" onClick={handleClearHistory}>
                  <Trash2 className="h-4 w-4 mr-1" />清除历史
                </Button>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      <Card className="glass-card">
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <BookOpen className="h-4 w-4" />
            {t("bangumi.settings")}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {syncEnabled && (
            <div className="flex items-center justify-between border-b border-border/40 pb-4">
              <div>
                <Label className="text-sm font-bold">同步开关</Label>
                <p className="text-xs text-muted-foreground">启用后自动将 Emby 播放记录同步到 Bangumi</p>
              </div>
              <Switch checked={bgmMode} onCheckedChange={setBgmMode} disabled={saving} />
            </div>
          )}
          {manageEnabled && (
            <div className="flex items-center justify-between border-b border-border/40 pb-4">
              <div>
                <Label className="text-sm font-bold">收藏管理</Label>
                <p className="text-xs text-muted-foreground">启用后可在面板管理 Bangumi 收藏状态</p>
              </div>
              <Switch checked={bgmManageMode} onCheckedChange={setBgmManageMode} disabled={saving} />
            </div>
          )}
          <div className="space-y-2">
            <Label className="text-sm font-bold">Access Token</Label>
            <Input
              type="password"
              placeholder={status?.bgm_token_set ? "已配置（留空不修改）" : "粘贴 Bangumi Access Token"}
              value={bgmToken}
              onChange={(e) => setBgmToken(e.target.value)}
              disabled={saving}
            />
            <p className="text-xs text-muted-foreground">
              从 <a href="https://next.bgm.tv/demo/access-token" target="_blank" rel="noopener noreferrer" className="text-primary underline">https://next.bgm.tv/demo/access-token</a> 获取
            </p>
          </div>
          <div className="flex gap-2">
            <Button onClick={handleSaveSettings} disabled={saving}>
              {saving ? <Loader2 className="h-4 w-4 animate-spin mr-1" /> : null}
              保存设置
            </Button>
            {status?.bgm_token_set && (
              <Button variant="outline" onClick={handleClearToken} disabled={saving}>
                清除 Token
              </Button>
            )}
          </div>
        </CardContent>
      </Card>

      {logs.length > 0 && (
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <ListChecks className="h-4 w-4" />
              同步历史
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2 max-h-80 overflow-y-auto">
              {logs.map((log) => (
                <div key={log.id} className="flex items-start gap-2 rounded-lg bg-accent/30 p-2 text-sm">
                  {log.status === "success" ? <CheckCircle2 className="h-4 w-4 text-green-500" /> : log.status === "failed" ? <XCircle className="h-4 w-4 text-red-500" /> : <Clock className="h-4 w-4 text-yellow-500" />}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-1 flex-wrap">
                      {log.subject_name ? <span className="font-medium truncate">{log.subject_name}</span> : null}
                      {log.episode ? <span className="text-muted-foreground">#{log.episode}</span> : null}
                    </div>
                    <div className="flex items-center gap-2 text-xs text-muted-foreground mt-0.5">
                      <Badge variant="outline" className="text-xs">{log.status === "success" ? "成功" : log.status === "failed" ? "失败" : "待处理"}</Badge>
                      <span>{formatTime(log.created_at)}</span>
                    </div>
                    {log.message ? <p className="text-xs text-muted-foreground mt-0.5 truncate">{log.message}</p> : null}
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
    </motion.div>
  );
}
