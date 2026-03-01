"use client";

import { useCallback, useState } from "react";
import { motion } from "framer-motion";
import {
  Coins,
  Calendar,
  Clock,
  Play,
  Tv,
  Flame,
  Gift,
  TrendingUp,
  CheckCircle2,
  XCircle,
  Key,
  Loader2,
  Edit,
  AlertTriangle,
  Sparkles,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { useToast } from "@/hooks/use-toast";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { useRegionRefresh } from "@/hooks/use-region-refresh";
import { PageError } from "@/components/layout/page-state";
import { useAuthStore } from "@/store/auth";
import { api, type ScoreInfo, type PlaybackStats, type TopMediaItem } from "@/lib/api";
import { formatRelativeTime, formatNumber, cn } from "@/lib/utils";
import { RegionRefreshKeys } from "@/lib/region-refresh";

const container = {
  hidden: { opacity: 0 },
  show: {
    opacity: 1,
    transition: {
      staggerChildren: 0.1,
    },
  },
};

const item = {
  hidden: { opacity: 0, y: 20 },
  show: { opacity: 1, y: 0 },
};

export default function DashboardPage() {
  const { user, fetchUser } = useAuthStore();
  const { toast } = useToast();
  const [scoreInfo, setScoreInfo] = useState<ScoreInfo | null>(null);
  const [stats, setStats] = useState<PlaybackStats | null>(null);
  const [isCheckinLoading, setIsCheckinLoading] = useState(false);
  const [regCode, setRegCode] = useState("");
  const [isRenewing, setIsRenewing] = useState(false);
  const [topMedia, setTopMedia] = useState<TopMediaItem[]>([]);
  const [regCodeInfo, setRegCodeInfo] = useState<{ type: number; type_name: string; days: number } | null>(null);
  const [showConfirm, setShowConfirm] = useState(false);
  
  // Admin edit score
  const [showEditScore, setShowEditScore] = useState(false);
  const [editScoreValue, setEditScoreValue] = useState(0);
  const [isEditingScore, setIsEditingScore] = useState(false);

  const loadDashboardData = useCallback(async () => {
    const [scoreRes, statsRes, topMediaRes] = await Promise.all([
      api.getScoreInfo(),
      api.getMyStats().catch(() => ({ success: false, data: null })),
      api.getTopMedia("week", 5).catch(() => ({ success: false, data: { ranking: [] } })),
    ]);

    if (scoreRes.success && scoreRes.data) {
      setScoreInfo(scoreRes.data);
    }
    if (statsRes.success && statsRes.data) {
      setStats(statsRes.data);
    }
    if (topMediaRes.success && topMediaRes.data) {
      setTopMedia(topMediaRes.data.ranking);
    }

    return true;
  }, []);

  const {
    isLoading,
    error,
    execute: loadData,
  } = useAsyncResource(loadDashboardData, { immediate: true });

  useRegionRefresh(RegionRefreshKeys.DashboardData, useCallback(() => {
    void loadData();
  }, [loadData]));

  const handleCheckRegcode = async () => {
    if (!regCode.trim()) {
      toast({
        title: "请输入注册码",
        variant: "destructive",
      });
      return;
    }

    try {
      const res = await api.checkRegcode(regCode.trim());
      if (res.success && res.data) {
        setRegCodeInfo(res.data);
        
        // 如果是注册类型，提示用户去注册页面
        if (res.data.type === 1) {
          toast({
            title: "这是注册码",
            description: "请前往注册页面使用此注册码进行注册",
            variant: "default",
          });
          // 跳转到注册页面
          window.location.href = "/register?regcode=" + encodeURIComponent(regCode.trim());
          return;
        }
        
        // 其他类型显示确认对话框
        setShowConfirm(true);
      } else {
        toast({
          title: "注册码无效",
          description: res.message || "请检查注册码是否正确",
          variant: "destructive",
        });
        setRegCodeInfo(null);
      }
    } catch (error: any) {
      toast({
        title: "检查失败",
        description: error.message || "请检查网络连接",
        variant: "destructive",
      });
      setRegCodeInfo(null);
    }
  };

  const handleConfirmUseRegcode = async () => {
    if (!regCode.trim() || !regCodeInfo) return;

    setIsRenewing(true);
    setShowConfirm(false);
    try {
      const res = await api.renewWithRegcode(regCode.trim());
      if (res.success) {
        toast({
          title: `${regCodeInfo.type_name}成功`,
          description: `账号已成功${regCodeInfo.type_name}`,
          variant: "success",
        });
        setRegCode("");
        setRegCodeInfo(null);
        await fetchUser();
        await loadData();
      } else {
        toast({
          title: `${regCodeInfo.type_name}失败`,
          description: res.message,
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: `${regCodeInfo.type_name}失败`,
        description: error.message || "请检查网络连接",
        variant: "destructive",
      });
    } finally {
      setIsRenewing(false);
    }
  };

  const handleEditScore = async () => {
    setIsEditingScore(true);
    try {
      const res = await api.updateMyAdminInfo({ score: editScoreValue });
      if (res.success) {
        toast({
          title: "更新成功",
          description: "积分已更新",
          variant: "success",
        });
        setShowEditScore(false);
        await fetchUser();
        await loadData();
      } else {
        toast({
          title: "更新失败",
          description: res.message,
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: "更新失败",
        description: error.message,
        variant: "destructive",
      });
    } finally {
      setIsEditingScore(false);
    }
  };

  const handleCheckin = async () => {
    // 前端也检查是否已签到
    if (scoreInfo?.today_checkin) {
      toast({
        title: "今日已签到",
        description: "明天再来吧~",
        variant: "destructive",
      });
      return;
    }

    setIsCheckinLoading(true);
    try {
      const res = await api.checkin();
      if (res.success && res.data) {
        toast({
          title: "签到成功！",
          description: `获得 ${res.data.score} ${scoreInfo?.score_name || '积分'}，连续签到 ${res.data.streak} 天`,
          variant: "success",
        });
        // 更新本地状态
        setScoreInfo((prev) => prev ? {
          ...prev,
          balance: res.data!.balance,
          today_checkin: true,
          checkin_streak: res.data!.streak,
        } : null);
        // 刷新用户数据
        await fetchUser();
      } else {
        toast({
          title: "签到失败",
          description: res.message,
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: "签到失败",
        description: error.message,
        variant: "destructive",
      });
    } finally {
      setIsCheckinLoading(false);
    }
  };

  // 计算到期状态
  const isPending = !user?.emby_id && !user?.active;  // 待激活用户
  
  // 处理到期时间（可能是时间戳或字符串）
  let expiredTimestamp: number | null = null;
  if (user?.expired_at) {
    if (typeof user.expired_at === 'number') {
      // 如果是 -1，表示永久
      if (user.expired_at === -1) {
        expiredTimestamp = null;
      } else {
        // 如果是秒级时间戳（小于 10 位），转换为毫秒
        expiredTimestamp = user.expired_at < 10000000000 ? user.expired_at * 1000 : user.expired_at;
        // 检查是否为异常值（如 1978 年等明显错误的日期）
        const date = new Date(expiredTimestamp);
        // 如果日期早于 2000 年，认为是永久
        if (date.getFullYear() < 2000) {
          expiredTimestamp = null;
        }
      }
    } else if (typeof user.expired_at === 'string') {
      // 如果是字符串，尝试解析
      if (user.expired_at === '-1' || user.expired_at === '-1') {
        expiredTimestamp = null; // 永久
      } else {
        const parsed = new Date(user.expired_at).getTime();
        // 检查是否为异常值
        const date = new Date(parsed);
        if (date.getFullYear() < 2000) {
          expiredTimestamp = null;
        } else {
          expiredTimestamp = parsed;
        }
      }
    }
  }
  
  const isAdmin = user?.role === 0;
  const isExpired = !isAdmin && expiredTimestamp !== null && expiredTimestamp !== -1 && expiredTimestamp < Date.now();
  const isPermanent = isAdmin || !expiredTimestamp || expiredTimestamp === -1;
  
  const daysLeft = (!isPending && !isPermanent && expiredTimestamp)
    ? Math.max(0, Math.ceil((expiredTimestamp - Date.now()) / (1000 * 60 * 60 * 24)))
    : 0;

  // 获取问候语
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

  if (error) {
    return <PageError message={error} onRetry={() => void loadData()} />;
  }

  return (
    <motion.div
      variants={container}
      initial="hidden"
      animate="show"
      className="space-y-8 pb-10"
    >
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
        <div>
          <h1 className="text-4xl font-black tracking-tighter text-foreground">
            {getGreeting()}，{user?.username}
          </h1>
          <p className="text-muted-foreground font-medium mt-1">
            {isPending ? "激活账号，开启您的星光之旅" : "发现新鲜事，品味好作品"}
          </p>
        </div>
        <div className="flex items-center gap-3">
           <Badge className="bg-primary/10 text-primary border-primary/20 px-4 py-1.5 rounded-full font-black text-xs uppercase tracking-widest">
             {user?.role_name}
           </Badge>
        </div>
      </div>

      {/* Stats Overview */}
      <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-4">
        <motion.div variants={item}>
          <div className="premium-card p-6 h-full flex flex-col justify-between group">
            <div className="flex items-center justify-between">
              <div className="p-3 bg-blue-500/10 text-blue-500 rounded-2xl group-hover:bg-blue-500 group-hover:text-white transition-all duration-500 shadow-sm">
                <Coins className="h-5 w-5" />
              </div>
              {user?.role === 0 && (
                <Button variant="ghost" size="icon" className="h-8 w-8 rounded-lg" onClick={() => {
                  setEditScoreValue(scoreInfo?.balance || 0);
                  setShowEditScore(true);
                }}>
                  <Edit className="h-3.5 w-3.5" />
                </Button>
              )}
            </div>
            <div className="mt-6">
              <p className="text-[10px] font-black uppercase tracking-widest text-muted-foreground">{scoreInfo?.score_name || "积分"}余额</p>
              <h3 className="text-3xl font-black mt-1">
                {isLoading ? <Skeleton className="h-8 w-24" /> : formatNumber(scoreInfo?.balance || 0)}
              </h3>
            </div>
          </div>
        </motion.div>

        <motion.div variants={item}>
          <div className="premium-card p-6 h-full flex flex-col justify-between group">
            <div className="p-3 w-fit bg-amber-500/10 text-amber-500 rounded-2xl group-hover:bg-amber-500 group-hover:text-white transition-all duration-500 shadow-sm">
              <Calendar className="h-5 w-5" />
            </div>
            <div className="mt-6">
              <p className="text-[10px] font-black uppercase tracking-widest text-muted-foreground">到期倒计时</p>
              <h3 className="text-3xl font-black mt-1">
                {isPermanent ? "∞ 永恒" : `${daysLeft} 天`}
              </h3>
            </div>
          </div>
        </motion.div>

        <motion.div variants={item}>
          <div className="premium-card p-6 h-full flex flex-col justify-between group">
            <div className="p-3 w-fit bg-emerald-500/10 text-emerald-500 rounded-2xl group-hover:bg-emerald-500 group-hover:text-white transition-all duration-500 shadow-sm">
              <Flame className="h-5 w-5" />
            </div>
            <div className="mt-6">
              <p className="text-[10px] font-black uppercase tracking-widest text-muted-foreground">连续签到</p>
              <h3 className="text-3xl font-black mt-1">
                {isLoading ? <Skeleton className="h-8 w-24" /> : `${scoreInfo?.checkin_streak || 0} DAY`}
              </h3>
            </div>
          </div>
        </motion.div>

        <motion.div variants={item}>
          <div className="premium-card p-6 h-full flex flex-col justify-between group">
            <div className="p-3 w-fit bg-purple-500/10 text-purple-500 rounded-2xl group-hover:bg-purple-500 group-hover:text-white transition-all duration-500 shadow-sm">
              <Clock className="h-5 w-5" />
            </div>
            <div className="mt-6">
              <p className="text-[10px] font-black uppercase tracking-widest text-muted-foreground">累计观影</p>
              <h3 className="text-3xl font-black mt-1">
                {isLoading ? <Skeleton className="h-8 w-24" /> : `${Math.floor((stats?.total_time || 0) / 60)} HR`}
              </h3>
            </div>
          </div>
        </motion.div>
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        {/* Main Interaction Area */}
        <motion.div variants={item} className="lg:col-span-2 space-y-6">
          {/* Checkin / Welcome Card */}
          <div className="premium-card p-1 shadow-2xl">
            <div className="glass-liquid p-8 rounded-[1.75rem] flex flex-col md:flex-row items-center gap-8 border-0 overflow-hidden relative">
               <div className="absolute -top-12 -right-12 w-48 h-48 bg-primary/10 blur-[80px] rounded-full" />
               <div className="relative group shrink-0">
                  <div className="absolute -inset-4 bg-primary/20 rounded-full blur-2xl group-hover:bg-primary/30 transition-all duration-500" />
                  <div className="relative bg-white/60 p-6 rounded-3xl border border-white shadow-xl">
                    <Gift className="h-12 w-12 text-primary animate-bounce-slow" />
                  </div>
               </div>
               <div className="flex-1 text-center md:text-left z-10">
                  <h2 className="text-2xl font-black tracking-tight">每日福利时刻</h2>
                  <p className="text-muted-foreground font-medium mt-1">
                    {scoreInfo?.today_checkin 
                      ? "任务已达成！明天再来领奖励吧" 
                      : `快来领取今日奖励，已连签 ${scoreInfo?.checkin_streak || 0} 天`}
                  </p>
                  <div className="mt-8 flex flex-col sm:flex-row items-center gap-4">
                    {scoreInfo?.today_checkin ? (
                      <div className="inline-flex items-center gap-2 px-6 h-12 bg-emerald-500/10 text-emerald-600 rounded-2xl border border-emerald-200 font-black text-xs uppercase tracking-widest">
                        <CheckCircle2 className="h-4 w-4" />
                        MISSION COMPLETED
                      </div>
                    ) : (
                      <Button 
                        size="lg" 
                        onClick={handleCheckin} 
                        disabled={isCheckinLoading}
                        className="rounded-2xl px-10 h-14 font-black text-base shadow-2xl shadow-primary/30 hover:scale-105 active:scale-95 transition-all"
                      >
                        {isCheckinLoading ? <Loader2 className="h-5 w-5 animate-spin mr-2" /> : <Sparkles className="h-5 w-5 mr-2" />}
                        立即签到
                      </Button>
                    )}
                    
                    <div className="flex-1 max-w-[200px]">
                       <div className="flex justify-between text-[10px] font-black uppercase mb-1.5 opacity-60">
                         <span>连签进度</span>
                         <span>{scoreInfo?.checkin_streak || 0}/7</span>
                       </div>
                       <Progress value={((scoreInfo?.checkin_streak || 0) % 7) / 7 * 100} className="h-1.5 bg-white/40" />
                    </div>
                  </div>
               </div>
            </div>
          </div>

          <div className="grid gap-6 md:grid-cols-2">
            {/* Activation / Renewal */}
            <div className="premium-card p-6 flex flex-col border-white/40">
              <div className="flex items-center gap-3 mb-6">
                <div className="p-2 bg-primary/10 rounded-xl text-primary">
                  <Key className="h-5 w-5" />
                </div>
                <div>
                  <h3 className="text-base font-black tracking-tight">账户激活与续期</h3>
                  <p className="text-[11px] text-muted-foreground font-bold uppercase tracking-tighter">Activate & Renew</p>
                </div>
              </div>
              <div className="flex flex-col gap-3">
                <Input
                  placeholder="输入授权码以增加效期..."
                  value={regCode}
                  onChange={(e) => setRegCode(e.target.value)}
                  className="h-12 rounded-xl border-white/60 bg-white/40 shadow-inner focus:bg-white transition-all font-medium"
                />
                <Button 
                  onClick={handleCheckRegcode} 
                  disabled={isRenewing}
                  className="h-12 rounded-xl font-black bg-secondary text-foreground hover:bg-secondary/70 shadow-lg border-white transition-all"
                >
                  {isRenewing ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : "验证并使用"}
                </Button>
              </div>
              <p className="mt-4 text-[11px] text-center text-muted-foreground font-bold">
                没有授权码？请联系管理员或在群组获取
              </p>
            </div>

            {/* Favorite Genres / Quick Stats */}
            <div className="premium-card p-6 border-white/40">
               <div className="flex items-center gap-3 mb-6">
                <div className="p-2 bg-orange-500/10 rounded-xl text-orange-500">
                  <TrendingUp className="h-5 w-5" />
                </div>
                <div>
                  <h3 className="text-base font-black tracking-tight">观影口味分布</h3>
                  <p className="text-[11px] text-muted-foreground font-bold uppercase tracking-tighter">Favorite Genres</p>
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                {stats?.favorite_genres && stats.favorite_genres.length > 0 ? (
                  stats.favorite_genres.slice(0, 8).map((genre) => (
                    <Badge key={genre} variant="secondary" className="bg-white/60 border border-white/40 text-muted-foreground font-bold rounded-lg px-2.5 py-1">
                      {genre}
                    </Badge>
                  ))
                ) : (
                  <p className="text-xs text-muted-foreground italic py-4">正在分析您的观影爱好...</p>
                )}
              </div>
            </div>
          </div>
        </motion.div>

        {/* Sidebar Widgets */}
        <motion.div variants={item} className="space-y-6">
           {/* Top Ranking */}
           <div className="premium-card p-6">
              <h3 className="text-lg font-black tracking-tight mb-6 flex items-center gap-2">
                <TrendingUp className="h-5 w-5 text-primary" />
                全站实时热门
              </h3>
              <div className="space-y-5">
                {topMedia && topMedia.length > 0 ? (
                  topMedia.map((item, index) => (
                    <div key={index} className="flex items-center justify-between group">
                      <div className="flex items-center gap-3">
                        <span className={cn(
                          "flex h-7 w-7 items-center justify-center rounded-xl text-xs font-black transition-all",
                          index === 0 ? "bg-amber-400 text-amber-900 shadow-lg shadow-amber-400/20" : "bg-secondary text-muted-foreground"
                        )}>
                          {index + 1}
                        </span>
                        <div className="max-w-[140px]">
                           <p className="text-sm font-bold truncate group-hover:text-primary transition-colors">{item.item_name}</p>
                           <p className="text-[10px] text-muted-foreground/60 uppercase tracking-tighter">{item.item_type}</p>
                        </div>
                      </div>
                      <div className="text-right shrink-0">
                         <span className="text-[11px] font-black text-primary">{item.play_count}</span>
                         <span className="text-[9px] text-muted-foreground/40 ml-1 font-black">PLAYS</span>
                      </div>
                    </div>
                  ))
                ) : (
                  <div className="py-10 text-center">
                    <Loader2 className="h-6 w-6 animate-spin text-muted-foreground mx-auto opacity-20" />
                  </div>
                )}
              </div>
           </div>

           {/* Recent Activity */}
           <div className="premium-card p-6">
              <h3 className="text-lg font-black tracking-tight mb-6 flex items-center gap-2">
                <Play className="h-5 w-5 text-primary" />
                最近观看历史
              </h3>
              <div className="space-y-5">
                {stats?.recent_items && stats.recent_items.length > 0 ? (
                  stats.recent_items.slice(0, 4).map((item, idx) => (
                    <div key={idx} className="flex items-center gap-3 group">
                       <div className="h-10 w-10 shrink-0 flex items-center justify-center rounded-2xl bg-primary/5 text-primary group-hover:bg-primary group-hover:text-white transition-all shadow-inner">
                         <Tv className="h-4 w-4" />
                       </div>
                       <div className="min-w-0">
                         <p className="text-sm font-black truncate group-hover:text-primary transition-colors">{item.name}</p>
                         <p className="text-[9px] font-black text-muted-foreground uppercase opacity-60">
                           {formatRelativeTime(new Date(item.played_at).getTime())}
                         </p>
                       </div>
                    </div>
                  ))
                ) : (
                  <p className="text-xs text-muted-foreground italic text-center py-4">暂入播放记录</p>
                )}
              </div>
           </div>
        </motion.div>
      </div>

      {/* Dialogs */}
      <Dialog open={showConfirm} onOpenChange={setShowConfirm}>
        <DialogContent className="max-w-md glass-acrylic border-0 rounded-[2.5rem] p-0 overflow-hidden shadow-2xl">
          <div className="p-8">
            <div className="flex items-center gap-4 mb-6">
               <div className="h-12 w-12 rounded-2xl bg-primary/10 flex items-center justify-center text-primary">
                  <CheckCircle2 className="h-6 w-6" />
               </div>
               <div>
                  <DialogTitle className="text-xl font-black">确认使用授权码</DialogTitle>
                  <p className="text-sm text-muted-foreground font-medium">请确认以下授权信息</p>
               </div>
            </div>
            
            {regCodeInfo && (
              <div className="p-4 rounded-2xl bg-secondary/50 border border-white mb-8 space-y-3">
                <div className="flex justify-between items-center">
                  <span className="text-xs font-bold text-muted-foreground">授权类型</span>
                  <Badge className="bg-primary/20 text-primary border-0 rounded-lg">{regCodeInfo.type_name}</Badge>
                </div>
                <div className="flex justify-between items-center">
                  <span className="text-xs font-bold text-muted-foreground">增加时长</span>
                  <span className="text-sm font-black">{regCodeInfo.days} 天</span>
                </div>
              </div>
            )}

            <div className="flex gap-3">
              <Button variant="outline" className="flex-1 h-12 rounded-2xl font-black border-white bg-white/40" onClick={() => setShowConfirm(false)}>
                取消
              </Button>
              <Button className="flex-[2] h-12 rounded-2xl font-black shadow-xl shadow-primary/20" onClick={handleConfirmUseRegcode}>
                立即激活
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={showEditScore} onOpenChange={setShowEditScore}>
        <DialogContent className="max-w-md glass-acrylic border-0 rounded-[2.5rem] p-0 overflow-hidden shadow-2xl">
          <div className="p-8">
            <div className="flex items-center gap-4 mb-6">
               <div className="h-12 w-12 rounded-2xl bg-amber-500/10 flex items-center justify-center text-amber-500">
                  <AlertTriangle className="h-6 w-6" />
               </div>
               <div>
                  <DialogTitle className="text-xl font-black">修改管理员积分</DialogTitle>
                  <p className="text-sm text-muted-foreground font-medium">此操作仅限管理员调试</p>
               </div>
            </div>
            
            <div className="space-y-4 mb-8">
              <div className="space-y-2">
                <Label className="text-xs font-black uppercase tracking-widest ml-1">New Balance</Label>
                <Input
                  type="number"
                  value={editScoreValue}
                  onChange={(e) => setEditScoreValue(Number(e.target.value))}
                  className="h-12 rounded-2xl border-white bg-white/40 shadow-inner"
                />
              </div>
            </div>

            <div className="flex gap-3">
              <Button variant="outline" className="flex-1 h-12 rounded-2xl font-black border-white bg-white/40" onClick={() => setShowEditScore(false)}>
                取消
              </Button>
              <Button 
                className="flex-[2] h-12 rounded-2xl font-black shadow-xl shadow-amber-500/20 bg-amber-500 text-white hover:bg-amber-600"
                onClick={handleEditScore}
                disabled={isEditingScore}
              >
                {isEditingScore ? <Loader2 className="h-5 w-5 animate-spin" /> : "更新积分"}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}
