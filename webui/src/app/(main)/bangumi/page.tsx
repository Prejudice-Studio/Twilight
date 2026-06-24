"use client";

import { useCallback, useState } from "react";
import { motion } from "framer-motion";
import { BookOpen, RefreshCw, Trash2, Loader2, CheckCircle2, XCircle, Clock, AlertCircle, Heart, Tv, ExternalLink, Edit, User as UserIcon, Star } from "lucide-react";
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
import { useAuthStore } from "@/store/auth";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter, DialogClose } from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

function formatTime(unix: number): string {
  return new Date(unix * 1000).toLocaleString();
}

function StarRating({ value }: { value: number }) {
  if (value <= 0) return null;
  return (
    <span className="inline-flex items-center gap-0.5 text-yellow-500 text-[10px]">
      <Star className="h-3 w-3 fill-current" />
      {value}
    </span>
  );
}

function RateInput({ value, onChange }: { value: number; onChange: (v: number) => void }) {
  return (
    <div className="flex gap-1">
      {[0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((v) => (
        <button
          key={v}
          type="button"
          onClick={() => onChange(value === v ? 0 : v)}
          className={`w-7 h-7 rounded text-[10px] font-bold transition-colors ${
            value === v
              ? "bg-yellow-500 text-black"
              : "bg-accent/30 text-muted-foreground hover:bg-accent/60"
          }`}
        >
          {v}
        </button>
      ))}
    </div>
  );
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

  // "查看全部" 状态以及分页数据
  const [viewAllOpen, setViewAllOpen] = useState(false);
  const [viewAllType, setViewAllType] = useState<number>(3);
  const [viewAllTitle, setViewAllTitle] = useState("");
  const [viewAllItems, setViewAllItems] = useState<any[]>([]);
  const [viewAllTotal, setViewAllTotal] = useState(0);
  const [viewAllOffset, setViewAllOffset] = useState(0);
  const [viewAllLoading, setViewAllLoading] = useState(false);

  // "编辑进度" 状态
  const [editOpen, setEditOpen] = useState(false);
  const [editingItem, setEditingItem] = useState<any>(null);
  const [editType, setEditType] = useState<number>(3);
  const [editEpStatus, setEditEpStatus] = useState<number>(0);
  const [editRate, setEditRate] = useState<number>(0);
  const [updating, setUpdating] = useState(false);

  const loadViewAll = async (type: number, offset: number, reset = false) => {
    setViewAllLoading(true);
    try {
      const res = await api.getBangumiCollections(type, 12, offset);
      if (res.success && res.data) {
        const entries = res.data.entries || [];
        const total = res.data.total ?? 0;
        if (reset) {
          setViewAllItems(entries);
        } else {
          setViewAllItems((prev) => [...prev, ...entries]);
        }
        setViewAllTotal(total);
        setViewAllOffset(offset);
      }
    } catch (e) {
      console.error(e);
      toast({ title: "加载失败", variant: "destructive" });
    } finally {
      setViewAllLoading(false);
    }
  };

  const handleViewAllPage = async (dir: number) => {
    const nextOffset = viewAllOffset + dir * 12;
    if (nextOffset < 0 || nextOffset >= viewAllTotal) return;
    await loadViewAll(viewAllType, nextOffset, true);
  };

  const handleOpenViewAll = (type: number, title: string) => {
    setViewAllType(type);
    setViewAllTitle(title);
    setViewAllItems([]);
    setViewAllTotal(0);
    setViewAllOffset(0);
    setViewAllOpen(true);
    void loadViewAll(type, 0, true);
  };

  const handleOpenEdit = (item: any) => {
    setEditingItem(item);
    setEditType(item.type ?? 3);
    setEditEpStatus(item.ep_status ?? 0);
    setEditRate(item.rate ?? 0);
    setEditOpen(true);
  };

  const handleEditTypeChange = (val: string) => {
    const t = Number(val);
    setEditType(t);
    // 非在看/看过状态下清除 ep_status，避免 Bangumi API 拒绝不一致请求
    if (t !== 2 && t !== 3) {
      setEditEpStatus(0);
    }
  };

  const handleSaveProgress = async () => {
    if (!editingItem) return;
    setUpdating(true);
    try {
      const res = await api.updateBangumiCollection(editingItem.subject_id, editType, editEpStatus, editRate);
      if (res.success) {
        toast({ title: "更新成功" });
        setEditOpen(false);
        await reload();
        if (viewAllOpen) {
          void loadViewAll(viewAllType, 0, true);
        }
      } else {
        toast({ title: "更新失败", description: res.message, variant: "destructive" });
      }
    } catch {
      toast({ title: "更新发生错误", variant: "destructive" });
    } finally {
      setUpdating(false);
    }
  };

  const handleSync = async () => {
    setSyncing(true);
    try {
      const res = await api.triggerBangumiSync();
      if (res.success && res.data) {
        toast({ title: t("bangumi.syncCompleted"), description: `${t("bangumi.syncedCount")}: ${res.data.synced}, ${t("bangumi.skippedCount")}: ${res.data.skipped}, ${t("bangumi.failedCount")}: ${res.data.failed}` });
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
    if (bgmMode && !bgmToken && !status?.bgm_token_set) {
      toast({ title: t("bangumi.tokenRequired"), variant: "destructive" });
      return;
    }
    setSaving(true);
    try {
      const res = await api.updateMySettings({ bgm_mode: bgmMode, bgm_manage_mode: bgmManageMode, bgm_token: bgmToken || undefined });
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

  const statusIcon = (s: string) => {
    switch (s) {
      case "success": return <CheckCircle2 className="h-4 w-4 text-green-500" />;
      case "failed": return <XCircle className="h-4 w-4 text-red-500" />;
      case "skipped": return <Clock className="h-4 w-4 text-yellow-500" />;
      default: return <AlertCircle className="h-4 w-4 text-muted-foreground" />;
    }
  };

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
        <p className="text-sm text-muted-foreground mt-1">
          {t("bangumi.description")}
        </p>
      </div>

      {bgmMe?.expired && (
        <Alert variant="destructive" className="border-red-500 bg-red-500/10">
          <AlertCircle className="h-5 w-5 text-red-600 animate-pulse" />
          <AlertTitle className="font-extrabold text-red-600 dark:text-red-400">
            您的 Bangumi 访问令牌已过期 / Access Token Expired
          </AlertTitle>
          <AlertDescription className="text-xs text-muted-foreground leading-relaxed mt-1">
            检测到当前绑定的 Bangumi (BGM) Token 凭证已失效、到期或已被其官方注销。由于 Token 失效，目前的点格子自动同步动作将被服务端暂时阻断。请在下方【设置】面板中填入重新申请的有效 Token。
          </AlertDescription>
        </Alert>
      )}

      {bgmMe && !bgmMe.expired && bgmMe.me && (
        <motion.div variants={{ hidden: {}, show: {} }} className="grid grid-cols-1 md:grid-cols-3 gap-6">
          <Card className="glass-card md:col-span-1">
            <CardHeader className="pb-4">
              <CardTitle className="text-lg font-bold flex items-center gap-2">
                <UserIcon className="h-4 w-4 text-primary" />
                Bangumi 账号关联
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center gap-4">
                {bgmMe.me.avatar?.large ? (
                  // eslint-disable-next-line @next/next/no-img-element -- User-provided avatar URLs
                  <img
                    src={bgmMe.me.avatar.large}
                    className="h-16 w-16 rounded-full border-2 border-primary object-cover"
                    alt={bgmMe.me.nickname || bgmMe.me.username}
                    referrerPolicy="no-referrer"
                  />
                ) : (
                  <div className="h-16 w-16 rounded-full border-2 border-primary bg-muted flex items-center justify-center">
                    <UserIcon className="h-8 w-8 text-muted-foreground" />
                  </div>
                )}
                <div className="min-w-0 flex-1">
                  <h3 className="font-bold text-foreground truncate text-base">
                    {bgmMe.me.nickname || "神秘用户"}
                  </h3>
                  <p className="text-xs text-muted-foreground truncate">
                    @{bgmMe.me.username || "unknown"}
                  </p>
                  <p className="text-xs text-muted-foreground mt-0.5">
                    UID: {bgmMe.me.id}
                  </p>
                </div>
              </div>

              {bgmMe.me.sign ? (
                <div className="rounded-lg bg-accent/20 p-3 text-xs italic text-muted-foreground line-clamp-3">
                  “ {bgmMe.me.sign} ”
                </div>
              ) : null}

              <div className="grid grid-cols-3 gap-2 py-3 border-t border-b border-border/40 text-center">
                <div className="space-y-0.5 pointer-events-none">
                  <p className="text-[11px] text-muted-foreground">在看</p>
                  <p className="text-base font-extrabold text-foreground">{bgmMe.watching_total ?? 0}</p>
                </div>
                <div className="space-y-0.5 pointer-events-none">
                  <p className="text-[11px] text-muted-foreground">想看</p>
                  <p className="text-base font-extrabold text-foreground">{bgmMe.wishlist_total ?? 0}</p>
                </div>
                <div className="space-y-0.5 pointer-events-none">
                  <p className="text-[11px] text-muted-foreground">看过</p>
                  <p className="text-base font-extrabold text-foreground">{bgmMe.collected_total ?? 0}</p>
                </div>
              </div>

              <div className="pt-2 flex items-center justify-between text-xs text-muted-foreground">
                <span>用户组</span>
                <Badge variant="outline" className="text-[10px]">
                  {bgmMe.me.user_group === 1 ? "普通成员" : bgmMe.me.user_group === 2 ? "管理员" : "BGM 用户"}
                </Badge>
              </div>

              <div className="flex justify-end pt-1">
                <a
                  href={`https://bgm.tv/user/${bgmMe.me.username}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-xs text-primary flex items-center gap-1 hover:underline"
                >
                  前往 Bangumi 主页
                  <ExternalLink className="h-3 w-3" />
                </a>
              </div>
            </CardContent>
          </Card>

          <Card className="glass-card md:col-span-2">
            <CardHeader className="pb-2">
              <CardTitle className="text-lg font-bold">Bangumi 个人娱乐视图</CardTitle>
              <CardDescription>您的 Bangumi 实时精选番剧同步与收藏视图</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="space-y-3">
                <div className="flex items-center justify-between border-b border-border/30 pb-1.5">
                  <h3 className="text-xs font-bold text-muted-foreground uppercase tracking-wide flex items-center gap-1.5">
                    <Tv className="h-3.5 w-3.5 text-primary" />
                    在看动画 ({bgmMe.watching_total ?? bgmMe.watching?.length ?? 0})
                  </h3>
                  <Button variant="link" className="text-xs h-auto p-0 text-primary font-bold hover:underline" onClick={() => handleOpenViewAll(3, "精选在看番剧")}>
                    查看全部 ({bgmMe.watching_total ?? bgmMe.watching?.length ?? 0}) →
                  </Button>
                </div>
                {bgmMe.watching && bgmMe.watching.length > 0 ? (
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    {bgmMe.watching.map((item: any) => {
                      const name = item.subject?.name_cn || item.subject?.name || "未知番剧";
                      const poster = item.subject?.images?.medium || item.subject?.images?.common || item.subject?.images?.small;
                      return (
                        <div key={item.subject_id} className="flex gap-3 bg-accent/20 border border-border/20 rounded-lg p-3 hover:bg-accent/30 transition shadow-sm">
                          {poster ? (
                            // eslint-disable-next-line @next/next/no-img-element -- User-provided poster URLs
                            <img src={poster} className="h-28 w-20 rounded-md object-cover flex-shrink-0 shadow-sm border border-border/40" alt={name} referrerPolicy="no-referrer" />
                          ) : (
                            <div className="h-28 w-20 rounded-md bg-muted flex items-center justify-center text-[10px] text-muted-foreground flex-shrink-0 border border-border/40">无封面</div>
                          )}
                          <div className="min-w-0 flex-1 flex flex-col justify-between py-0.5">
                            <div>
                              <h4 className="text-xs font-bold truncate text-foreground" title={name}>{name}</h4>
                              <p className="text-[10px] text-muted-foreground truncate">{item.subject?.name}</p>
                              {item.ep_status ? (
                                <Badge variant="secondary" className="text-[10px] mt-2 px-1.5 py-0.5">看到 {item.ep_status} 话</Badge>
                              ) : <Badge variant="outline" className="text-[10px] mt-2 px-1.5 py-0.5 text-muted-foreground">尚无进度</Badge>}
                              {item.rate > 0 && (
                                <div className="mt-1"><StarRating value={item.rate} /></div>
                              )}
                            </div>
                            <div className="flex items-center justify-between gap-1.5 mt-2 border-t border-border/20 pt-1.5">
                              <Button variant="ghost" size="sm" className="h-6 px-1.5 text-[10px] text-muted-foreground hover:text-primary flex items-center gap-1" onClick={() => handleOpenEdit(item)}>
                                <Edit className="h-3 w-3" />
                                进度/状态
                              </Button>
                              <a
                                href={`https://bgm.tv/subject/${item.subject_id}`}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="text-[10px] text-primary flex items-center gap-0.5 hover:underline px-1.5 py-1"
                              >
                                详情
                                <ExternalLink className="h-3 w-3" />
                              </a>
                            </div>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground italic py-2">目前没有正在观看的在看动画...</p>
                )}
              </div>

              <div className="space-y-3">
                <div className="flex items-center justify-between border-b border-border/30 pb-1.5">
                  <h3 className="text-xs font-bold text-muted-foreground uppercase tracking-wide flex items-center gap-1.5">
                    <CheckCircle2 className="h-3.5 w-3.5 text-green-500" />
                    最近看过 ({bgmMe.collected_total ?? bgmMe.collected?.length ?? 0})
                  </h3>
                  <Button variant="link" className="text-xs h-auto p-0 text-primary font-bold hover:underline" onClick={() => handleOpenViewAll(2, "精选看过番剧")}>
                    查看全部 ({bgmMe.collected_total ?? bgmMe.collected?.length ?? 0}) →
                  </Button>
                </div>
                {bgmMe.collected && bgmMe.collected.length > 0 ? (
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    {bgmMe.collected.map((item: any) => {
                      const name = item.subject?.name_cn || item.subject?.name || "看过动画";
                      const poster = item.subject?.images?.medium || item.subject?.images?.common || item.subject?.images?.small;
                      return (
                        <div key={item.subject_id} className="flex gap-3 bg-accent/20 border border-border/20 rounded-lg p-3 hover:bg-accent/30 transition shadow-sm">
                          {poster ? (
                            // eslint-disable-next-line @next/next/no-img-element -- User-provided poster URLs
                            <img src={poster} className="h-28 w-20 rounded-md object-cover flex-shrink-0 shadow-sm border border-border/40" alt={name} referrerPolicy="no-referrer" />
                          ) : (
                            <div className="h-28 w-20 rounded-md bg-muted flex items-center justify-center text-[10px] text-muted-foreground flex-shrink-0 border border-border/40">无封面</div>
                          )}
                          <div className="min-w-0 flex-1 flex flex-col justify-between py-0.5">
                            <div>
                              <h4 className="text-xs font-bold truncate text-foreground" title={name}>{name}</h4>
                              <p className="text-[10px] text-muted-foreground truncate">{item.subject?.name}</p>
                              <Badge variant="outline" className="text-[10px] mt-2 px-1.5 py-0.5 text-green-600 bg-green-500/5 border-green-500/20">已看完</Badge>
                              {item.rate > 0 && (
                                <div className="mt-1"><StarRating value={item.rate} /></div>
                              )}
                            </div>
                            <div className="flex items-center justify-between gap-1.5 mt-2 border-t border-border/20 pt-1.5">
                              <Button variant="ghost" size="sm" className="h-6 px-1.5 text-[10px] text-muted-foreground hover:text-primary flex items-center gap-1" onClick={() => handleOpenEdit(item)}>
                                <Edit className="h-3 w-3" />
                                进度/状态
                              </Button>
                              <a
                                href={`https://bgm.tv/subject/${item.subject_id}`}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="text-[10px] text-primary flex items-center gap-0.5 hover:underline px-1.5 py-1"
                              >
                                详情
                                <ExternalLink className="h-3 w-3" />
                              </a>
                            </div>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground italic py-2">目前还没有看过的动画记录呢...</p>
                )}
              </div>

              <div className="space-y-3">
                <div className="flex items-center justify-between border-b border-border/30 pb-1.5">
                  <h3 className="text-xs font-bold text-muted-foreground uppercase tracking-wide flex items-center gap-1.5">
                    <Heart className="h-3.5 w-3.5 text-red-500" />
                    想看动画 ({bgmMe.wishlist_total ?? bgmMe.wishlist?.length ?? 0})
                  </h3>
                  <Button variant="link" className="text-xs h-auto p-0 text-primary font-bold hover:underline" onClick={() => handleOpenViewAll(1, "精选想看番剧")}>
                    查看全部 ({bgmMe.wishlist_total ?? bgmMe.wishlist?.length ?? 0}) →
                  </Button>
                </div>
                {bgmMe.wishlist && bgmMe.wishlist.length > 0 ? (
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    {bgmMe.wishlist.map((item: any) => {
                      const name = item.subject?.name_cn || item.subject?.name || "想看动画";
                      const poster = item.subject?.images?.medium || item.subject?.images?.common || item.subject?.images?.small;
                      return (
                        <div key={item.subject_id} className="flex gap-3 bg-accent/20 border border-border/20 rounded-lg p-3 hover:bg-accent/30 transition shadow-sm">
                          {poster ? (
                            // eslint-disable-next-line @next/next/no-img-element -- User-provided poster URLs
                            <img src={poster} className="h-28 w-20 rounded-md object-cover flex-shrink-0 shadow-sm border border-border/40" alt={name} referrerPolicy="no-referrer" />
                          ) : (
                            <div className="h-28 w-20 rounded-md bg-muted flex items-center justify-center text-[10px] text-muted-foreground flex-shrink-0 border border-border/40">无封面</div>
                          )}
                          <div className="min-w-0 flex-1 flex flex-col justify-between py-0.5">
                            <div>
                              <h4 className="text-xs font-bold truncate text-foreground" title={name}>{name}</h4>
                              <p className="text-[10px] text-muted-foreground truncate">{item.subject?.name}</p>
                              <Badge variant="outline" className="text-[10px] mt-2 px-1.5 py-0.5 text-red-500 bg-red-500/5 border-red-500/20">想看</Badge>
                              {item.rate > 0 && (
                                <div className="mt-1"><StarRating value={item.rate} /></div>
                              )}
                            </div>
                            <div className="flex items-center justify-between gap-1.5 mt-2 border-t border-border/20 pt-1.5">
                              <Button variant="ghost" size="sm" className="h-6 px-1.5 text-[10px] text-muted-foreground hover:text-primary flex items-center gap-1" onClick={() => handleOpenEdit(item)}>
                                <Edit className="h-3 w-3" />
                                进度/状态
                              </Button>
                              <a
                                href={`https://bgm.tv/subject/${item.subject_id}`}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="text-[10px] text-primary flex items-center gap-0.5 hover:underline px-1.5 py-1"
                              >
                                详情
                                <ExternalLink className="h-3 w-3" />
                              </a>
                            </div>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground italic py-2">目前想看列表里还没有动画呢...</p>
                )}
              </div>
            </CardContent>
          </Card>
        </motion.div>
      )}

      <motion.div variants={{ hidden: {}, show: {} }}>
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <BookOpen className="h-4 w-4" />
              {t("bangumi.syncStatus")}
            </CardTitle>
            <CardDescription>{t("bangumi.syncStatusDescription")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
              <div className="rounded-lg bg-accent/50 p-3 text-center">
                <div className="text-2xl font-bold">{status?.total_records ?? 0}</div>
                <div className="text-xs text-muted-foreground">{t("bangumi.totalRecords")}</div>
              </div>
              <div className="rounded-lg bg-accent/50 p-3 text-center">
                <div className="text-2xl font-bold text-green-500">{status?.synced_count ?? 0}</div>
                <div className="text-xs text-muted-foreground">{t("bangumi.synced")}</div>
              </div>
              <div className="rounded-lg bg-accent/50 p-3 text-center">
                <div className="text-2xl font-bold">{status?.sync_ready ? t("bangumi.ready") : t("bangumi.notReady")}</div>
                <div className="text-xs text-muted-foreground">{t("bangumi.status")}</div>
              </div>
              <div className="rounded-lg bg-accent/50 p-3 text-center">
                <div className="text-2xl font-bold">{status?.bgm_token_set ? t("bangumi.configured") : t("bangumi.notConfigured")}</div>
                <div className="text-xs text-muted-foreground">{t("bangumi.token")}</div>
              </div>
            </div>
            <div className="flex gap-2">
              <Button onClick={handleSync} disabled={syncing || !status?.sync_ready}>
                {syncing ? <Loader2 className="h-4 w-4 animate-spin mr-1" /> : <RefreshCw className="h-4 w-4 mr-1" />}
                {t("bangumi.startSync")}
              </Button>
              {logs.length > 0 && (
                <Button variant="outline" onClick={handleClearHistory}>
                  <Trash2 className="h-4 w-4 mr-1" />
                  {t("bangumi.clearHistory")}
                </Button>
              )}
            </div>
          </CardContent>
        </Card>
      </motion.div>

      <motion.div variants={{ hidden: {}, show: {} }}>
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <BookOpen className="h-4 w-4" />
              {t("bangumi.settings")}
            </CardTitle>
            <CardDescription>{t("settings.bangumiDescription")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between border-b border-border/40 pb-4">
              <div>
                <Label className="text-sm font-bold">{t("settings.bangumiSync")}</Label>
                <p className="text-xs text-muted-foreground">{t("settings.bangumiSyncDescription")}</p>
              </div>
              <Switch checked={bgmMode} onCheckedChange={setBgmMode} disabled={saving} />
            </div>

            <div className="flex items-center justify-between border-b border-border/40 pb-4">
              <div>
                <Label className="text-sm font-bold">{t("settings.bangumiManage")}</Label>
                <p className="text-xs text-muted-foreground">{t("settings.bangumiManageDescription")}</p>
              </div>
              <Switch checked={bgmManageMode} onCheckedChange={setBgmManageMode} disabled={saving} />
            </div>

            <div className="space-y-2">
              <Label className="text-sm font-bold">{t("bangumi.accessToken")}</Label>
              <Input
                type="password"
                placeholder={status?.bgm_token_set ? t("settings.bangumiTokenConfiguredPlaceholder") : t("settings.bangumiTokenPlaceholder")}
                value={bgmToken}
                onChange={(e) => setBgmToken(e.target.value)}
                disabled={saving}
              />
              <p className="text-xs text-muted-foreground">
                {t("bangumi.tokenHint")}{" "}
                <a href="https://next.bgm.tv/demo/access-token" target="_blank" rel="noopener noreferrer" className="text-primary underline">
                  https://next.bgm.tv/demo/access-token
                </a>
              </p>
            </div>
            <div className="flex gap-2">
              <Button onClick={handleSaveSettings} disabled={saving}>
                {saving ? <Loader2 className="h-4 w-4 animate-spin mr-1" /> : null}
                {t("settings.saveBangumiSettings")}
              </Button>
              {status?.bgm_token_set && (
                <Button variant="outline" onClick={handleClearToken} disabled={saving}>
                  {t("settings.clearToken")}
                </Button>
              )}
            </div>
          </CardContent>
        </Card>
      </motion.div>

      {logs.length > 0 && (
        <motion.div variants={{ hidden: {}, show: {} }}>
          <Card className="glass-card">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Clock className="h-4 w-4" />
                {t("bangumi.syncHistory")}
              </CardTitle>
              <CardDescription>{t("bangumi.syncHistoryDescription")}</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-2 max-h-96 overflow-y-auto">
                {logs.map((log) => (
                  <div key={log.id} className="flex items-start gap-2 rounded-lg bg-accent/30 p-2 text-sm">
                    {statusIcon(log.status)}
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-1 flex-wrap">
                        {log.subject_name && (
                          <span className="font-medium truncate">{log.subject_name}</span>
                        )}
                        {log.episode ? (
                          <span className="text-muted-foreground">#{log.episode}</span>
                        ) : null}
                      </div>
                      <div className="flex items-center gap-2 text-xs text-muted-foreground mt-0.5">
                        <Badge variant="outline" className="text-xs">
                          {log.status === "success" ? t("bangumi.success") : log.status === "failed" ? t("bangumi.failed") : t("bangumi.pending")}
                        </Badge>
                        <span>{formatTime(log.created_at)}</span>
                      </div>
                      {log.message && (
                        <p className="text-xs text-muted-foreground mt-0.5 truncate">{log.message}</p>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </motion.div>
      )}

      {/* Edit subject status/progress Dialog */}
      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="sm:max-w-[425px]">
          <DialogHeader>
            <DialogTitle>编辑收藏状态</DialogTitle>
            <DialogDescription>
              更新您在 Bangumi 上的番剧观看状态及看到第几集。
            </DialogDescription>
          </DialogHeader>
          {editingItem && (
            <div className="grid gap-4 py-4">
              <div className="flex gap-4 items-start bg-accent/20 p-3 rounded-lg border border-border/30">
                {editingItem.subject?.images?.medium || editingItem.subject?.images?.common ? (
                  <img
                    src={editingItem.subject?.images?.medium || editingItem.subject?.images?.common}
                    className="h-24 w-16 rounded object-cover shadow-sm border border-border/40"
                    alt={editingItem.subject?.name_cn || editingItem.subject?.name}
                    referrerPolicy="no-referrer"
                  />
                ) : (
                  <div className="h-24 w-16 bg-muted flex items-center justify-center text-xs text-muted-foreground rounded">无封面</div>
                )}
                <div>
                  <h4 className="text-sm font-bold text-foreground">
                    {editingItem.subject?.name_cn || editingItem.subject?.name || "未知番剧"}
                  </h4>
                  <p className="text-xs text-muted-foreground">{editingItem.subject?.name}</p>
                </div>
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="status_type">观看状态</Label>
                <Select
                  value={String(editType)}
                  onValueChange={handleEditTypeChange}
                >
                  <SelectTrigger id="status_type">
                    <SelectValue placeholder="选择状态" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="1">想看</SelectItem>
                    <SelectItem value="2">看过</SelectItem>
                    <SelectItem value="3">在看</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {editType === 3 && (
                <div className="space-y-1.5">
                  <Label htmlFor="status_episode">看到第几集</Label>
                  <div className="flex gap-2">
                    <Input
                      id="status_episode"
                      type="number"
                      min={0}
                      value={editEpStatus}
                      onChange={(e) => setEditEpStatus(parseInt(e.target.value) || 0)}
                    />
                    <Button
                      variant="outline"
                      type="button"
                      onClick={() => setEditEpStatus((prev) => Math.max(0, prev - 1))}
                    >
                      -1
                    </Button>
                    <Button
                      variant="outline"
                      type="button"
                      onClick={() => setEditEpStatus((prev) => prev + 1)}
                    >
                      +1
                    </Button>
                  </div>
                </div>
              )}

              <div className="space-y-1.5">
                <Label>评分</Label>
                <RateInput value={editRate} onChange={setEditRate} />
                <p className="text-[10px] text-muted-foreground">点击数字设置评分（0=不评分），再点击取消评分</p>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditOpen(false)}>
              取消
            </Button>
            <Button onClick={handleSaveProgress} disabled={updating}>
              {updating && <Loader2 className="h-4 w-4 animate-spin mr-1" />}
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* View-All Dialog */}
      <Dialog open={viewAllOpen} onOpenChange={setViewAllOpen}>
        <DialogContent className="sm:max-w-[700px] max-h-[85vh] flex flex-col p-6">
          <DialogHeader className="pb-2 border-b border-border/40">
            <DialogTitle className="flex justify-between items-center pr-6">
              <span>{viewAllTitle}</span>
              <Badge variant="secondary" className="text-xs font-semibold ml-2">全部项目数: {viewAllTotal}</Badge>
            </DialogTitle>
            <DialogDescription>
              浏览您在 Bangumi 下的所有该分类下的番剧条目。
            </DialogDescription>
          </DialogHeader>

          <div className="flex-1 overflow-y-auto py-4 pr-1 min-h-[300px]">
            {viewAllLoading ? (
              <div className="flex h-48 w-full items-center justify-center">
                <Loader2 className="h-8 w-8 animate-spin text-primary" />
              </div>
            ) : viewAllItems && viewAllItems.length > 0 ? (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                {viewAllItems.map((item: any) => {
                  const name = item.subject?.name_cn || item.subject?.name || "未知番剧";
                  const poster = item.subject?.images?.medium || item.subject?.images?.common || item.subject?.images?.small;
                  return (
                    <div key={item.subject_id} className="flex gap-3 bg-accent/20 border border-border/20 rounded-lg p-3 hover:bg-accent/30 transition shadow-sm">
                      {poster ? (
                        <img src={poster} className="h-28 w-20 rounded-md object-cover flex-shrink-0 shadow-sm border border-border/40" alt={name} referrerPolicy="no-referrer" />
                      ) : (
                        <div className="h-28 w-20 rounded-md bg-muted flex items-center justify-center text-[10px] text-muted-foreground flex-shrink-0 border border-border/40">无封面</div>
                      )}
                      <div className="min-w-0 flex-1 flex flex-col justify-between py-0.5">
                        <div>
                          <h4 className="text-xs font-bold truncate text-foreground" title={name}>{name}</h4>
                          <p className="text-[10px] text-muted-foreground truncate">{item.subject?.name}</p>
                          {item.type === 3 ? (
                            item.ep_status ? (
                              <Badge variant="secondary" className="text-[10px] mt-2 px-1.5 py-0.5">看到 {item.ep_status} 话</Badge>
                            ) : <Badge variant="outline" className="text-[10px] mt-2 px-1.5 py-0.5 text-muted-foreground">尚无进度</Badge>
                          ) : item.type === 2 ? (
                            <Badge variant="outline" className="text-[10px] mt-2 px-1.5 py-0.5 text-green-600 bg-green-500/5 border-green-500/20">已看完</Badge>
                          ) : (
                            <Badge variant="outline" className="text-[10px] mt-2 px-1.5 py-0.5 text-red-500 bg-red-500/5 border-red-500/20">想看</Badge>
                          )}
                          {item.rate > 0 && (
                            <div className="mt-1"><StarRating value={item.rate} /></div>
                          )}
                        </div>
                        <div className="flex items-center justify-between gap-1.5 mt-2 border-t border-border/20 pt-1.5">
                          <Button variant="ghost" size="sm" className="h-6 px-1.5 text-[10px] text-muted-foreground hover:text-primary flex items-center gap-1" onClick={() => {
                            setViewAllOpen(false);
                            handleOpenEdit(item);
                          }}>
                            <Edit className="h-3 w-3" />
                            进度/状态
                          </Button>
                          <a
                            href={`https://bgm.tv/subject/${item.subject_id}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-[10px] text-primary flex items-center gap-0.5 hover:underline px-1.5 py-1"
                          >
                            详情
                            <ExternalLink className="h-3 w-3" />
                          </a>
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground text-center py-8">暂无条目</p>
            )}
          </div>

          <div className="flex justify-between items-center pt-4 border-t border-border/40 bg-background">
            <Button
              variant="outline"
              size="sm"
              disabled={viewAllOffset === 0 || viewAllLoading}
              onClick={() => handleViewAllPage(-1)}
            >
              上一页
            </Button>
            <span className="text-xs text-muted-foreground">
              第 {Math.floor(viewAllOffset / 12) + 1} 页
            </span>
            <Button
              variant="outline"
              size="sm"
              disabled={viewAllOffset + 12 >= viewAllTotal || viewAllLoading}
              onClick={() => handleViewAllPage(1)}
            >
              下一页
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}
