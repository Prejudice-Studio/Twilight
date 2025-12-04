"use client";

import { useEffect, useState } from "react";
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
} from "@/components/ui/dialog";
import { useToast } from "@/hooks/use-toast";
import { useAuthStore } from "@/store/auth";
import { api, type ScoreInfo, type PlaybackStats } from "@/lib/api";
import { formatRelativeTime, formatNumber } from "@/lib/utils";

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
  const [isLoading, setIsLoading] = useState(true);
  const [regCode, setRegCode] = useState("");
  const [isRenewing, setIsRenewing] = useState(false);
  const [regCodeInfo, setRegCodeInfo] = useState<{ type: number; type_name: string; days: number } | null>(null);
  const [showConfirm, setShowConfirm] = useState(false);
  
  // Admin edit score
  const [showEditScore, setShowEditScore] = useState(false);
  const [editScoreValue, setEditScoreValue] = useState(0);
  const [isEditingScore, setIsEditingScore] = useState(false);

  // 加载数据函数
  const loadData = async () => {
    try {
      const [scoreRes, statsRes] = await Promise.all([
        api.getScoreInfo(),
        api.getMyStats().catch(() => ({ success: false, data: null })),  // 待激活用户可能没有统计数据
      ]);
      if (scoreRes.success && scoreRes.data) {
        setScoreInfo(scoreRes.data);
      }
      if (statsRes.success && statsRes.data) {
        setStats(statsRes.data);
      }
    } catch (error) {
      console.error(error);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

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
  
  const isExpired = expiredTimestamp !== null && expiredTimestamp !== -1 && expiredTimestamp < Date.now();
  const isPermanent = !expiredTimestamp || expiredTimestamp === -1;
  
  const daysLeft = (!isPending && !isPermanent && expiredTimestamp)
    ? Math.max(0, Math.ceil((expiredTimestamp - Date.now()) / (1000 * 60 * 60 * 24)))
    : 0;

  // 获取问候语
  const getGreeting = () => {
    const hour = new Date().getHours();
    if (hour < 6) return "夜深了";
    if (hour < 9) return "早上好";
    if (hour < 12) return "上午好";
    if (hour < 14) return "中午好";
    if (hour < 18) return "下午好";
    if (hour < 22) return "晚上好";
    return "夜深了";
  };

  return (
    <motion.div
      variants={container}
      initial="hidden"
      animate="show"
      className="space-y-6"
    >
      {/* Welcome Banner */}
      <motion.div variants={item}>
        <Card className={`overflow-hidden border-0 ${isPending ? 'bg-gradient-to-r from-amber-600 via-amber-500 to-orange-500' : 'bg-gradient-to-r from-twilight-600 via-twilight-500 to-sunset-500'}`}>
          <CardContent className="flex items-center justify-between p-6">
            <div>
              <h2 className="text-2xl font-bold text-white">
                {getGreeting()}，{user?.username}！
              </h2>
              <p className="mt-1 text-white/80">
                {isPending 
                  ? "您的账户待激活，签到赚积分后可激活 Emby 账户" 
                  : "今天也是美好的一天，来看点什么吧"}
              </p>
            </div>
            <div className="hidden sm:block">
              <Gift className="h-20 w-20 text-white/20" />
            </div>
          </CardContent>
        </Card>
      </motion.div>

      {/* Stats Grid */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        {/* Score Card */}
        <motion.div variants={item}>
          <Card className="relative overflow-hidden">
            <div className="absolute right-0 top-0 h-32 w-32 translate-x-8 translate-y-[-50%] rounded-full bg-gradient-to-br from-twilight-500/20 to-transparent" />
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <div className="flex items-center gap-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {scoreInfo?.score_name || "积分"}余额
              </CardTitle>
                {user?.role === 0 && (
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-6 w-6 p-0"
                    onClick={() => {
                      setEditScoreValue(user?.score || 0);
                      setShowEditScore(true);
                    }}
                  >
                    <Edit className="h-3 w-3" />
                  </Button>
                )}
              </div>
              <Coins className="h-4 w-4 text-twilight-500" />
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <Skeleton className="h-8 w-24" />
              ) : (
                <div className="text-3xl font-bold">
                  {formatNumber(scoreInfo?.balance || 0)}
                </div>
              )}
            </CardContent>
          </Card>
        </motion.div>

        {/* Expiry Card */}
        <motion.div variants={item}>
          <Card className="relative overflow-hidden">
            <div className="absolute right-0 top-0 h-32 w-32 translate-x-8 translate-y-[-50%] rounded-full bg-gradient-to-br from-sunset-500/20 to-transparent" />
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {isPending ? "账户状态" : "会员到期"}
              </CardTitle>
              <Calendar className="h-4 w-4 text-sunset-500" />
            </CardHeader>
            <CardContent>
              {isPending ? (
                <>
                  <div className="text-xl font-bold text-amber-500">待激活</div>
                  <p className="text-xs text-muted-foreground">
                    签到赚积分后可激活
                  </p>
                </>
              ) : isPermanent ? (
                <div className="text-xl font-bold text-emerald-500">永久</div>
              ) : isExpired ? (
                <>
                  <div className="text-xl font-bold text-destructive">已过期</div>
                  <p className="text-xs text-muted-foreground">
                    请续期以继续使用
                  </p>
                </>
              ) : (
                <>
                  <div className="text-3xl font-bold">{daysLeft} 天</div>
                  <p className="text-xs text-muted-foreground">
                    {expiredTimestamp ? formatRelativeTime(expiredTimestamp) : '永久'}
                  </p>
                </>
              )}
            </CardContent>
          </Card>
        </motion.div>

        {/* Checkin Streak */}
        <motion.div variants={item}>
          <Card className="relative overflow-hidden">
            <div className="absolute right-0 top-0 h-32 w-32 translate-x-8 translate-y-[-50%] rounded-full bg-gradient-to-br from-emerald-500/20 to-transparent" />
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                连续签到
              </CardTitle>
              <Flame className="h-4 w-4 text-emerald-500" />
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <Skeleton className="h-8 w-16" />
              ) : (
                <div className="text-3xl font-bold">
                  {scoreInfo?.checkin_streak || 0} 天
                </div>
              )}
            </CardContent>
          </Card>
        </motion.div>

        {/* Watch Time */}
        <motion.div variants={item}>
          <Card className="relative overflow-hidden">
            <div className="absolute right-0 top-0 h-32 w-32 translate-x-8 translate-y-[-50%] rounded-full bg-gradient-to-br from-blue-500/20 to-transparent" />
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                观看时长
              </CardTitle>
              <Clock className="h-4 w-4 text-blue-500" />
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <Skeleton className="h-8 w-20" />
              ) : (
                <div className="text-3xl font-bold">
                  {Math.floor((stats?.total_time || 0) / 60)} 小时
                </div>
              )}
            </CardContent>
          </Card>
        </motion.div>
      </div>

      {/* Main Content */}
      <div className="grid gap-6 lg:grid-cols-3">
        {/* Checkin Card */}
        <motion.div variants={item} className="lg:col-span-2">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Gift className="h-5 w-5 text-primary" />
                每日签到
              </CardTitle>
              <CardDescription>
                连续签到可获得额外奖励
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between rounded-lg bg-accent/50 p-4">
                <div className="flex items-center gap-4">
                  <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
                    {scoreInfo?.today_checkin ? (
                      <CheckCircle2 className="h-6 w-6 text-emerald-500" />
                    ) : (
                      <Gift className="h-6 w-6 text-primary" />
                    )}
                  </div>
                  <div>
                    <p className="font-medium">
                      {scoreInfo?.today_checkin ? "今日已签到" : "今日未签到"}
                    </p>
                    <p className="text-sm text-muted-foreground">
                      累计获得 {formatNumber(scoreInfo?.total_earned || 0)} {scoreInfo?.score_name || '积分'}
                    </p>
                  </div>
                </div>
                <Button
                  variant={scoreInfo?.today_checkin ? "outline" : "gradient"}
                  disabled={scoreInfo?.today_checkin || isCheckinLoading}
                  onClick={handleCheckin}
                >
                  {isCheckinLoading ? "签到中..." : scoreInfo?.today_checkin ? "已签到" : "立即签到"}
                </Button>
              </div>

              {/* Streak Progress */}
              <div className="space-y-2">
                <div className="flex justify-between text-sm">
                  <span className="text-muted-foreground">连签进度</span>
                  <span className="font-medium">{scoreInfo?.checkin_streak || 0} / 7 天</span>
                </div>
                <Progress value={((scoreInfo?.checkin_streak || 0) % 7) / 7 * 100} className="h-2" />
                <p className="text-xs text-muted-foreground">
                  连续签到 7 天可获得额外奖励！
                </p>
              </div>
            </CardContent>
          </Card>
        </motion.div>

        {/* Quick Stats */}
        <motion.div variants={item}>
          <Card className="h-full">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <TrendingUp className="h-5 w-5 text-primary" />
                观看统计
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Play className="h-4 w-4 text-muted-foreground" />
                  <span className="text-sm">总播放次数</span>
                </div>
                <span className="font-bold">{stats?.total_plays || 0}</span>
              </div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Clock className="h-4 w-4 text-muted-foreground" />
                  <span className="text-sm">总观看时长</span>
                </div>
                <span className="font-bold">{Math.floor((stats?.total_time || 0) / 60)} 小时</span>
              </div>

              {stats?.favorite_genres && stats.favorite_genres.length > 0 && (
                <div className="pt-4">
                  <p className="mb-2 text-sm text-muted-foreground">喜欢的类型</p>
                  <div className="flex flex-wrap gap-2">
                    {stats.favorite_genres.slice(0, 5).map((genre) => (
                      <Badge key={genre} variant="secondary">
                        {genre}
                      </Badge>
                    ))}
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </motion.div>
      </div>

      {/* Recent Activity */}
      {stats?.recent_items && stats.recent_items.length > 0 && (
        <motion.div variants={item}>
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Tv className="h-5 w-5 text-primary" />
                最近观看
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-3">
                {stats.recent_items.slice(0, 5).map((item, index) => (
                  <div
                    key={index}
                    className="flex items-center justify-between rounded-lg bg-accent/30 p-3"
                  >
                    <div className="flex items-center gap-3">
                      <div className="flex h-10 w-10 items-center justify-center rounded-md bg-primary/10">
                        <Play className="h-4 w-4 text-primary" />
                      </div>
                      <div>
                        <p className="font-medium">{item.name}</p>
                        <p className="text-xs text-muted-foreground">{item.type}</p>
                      </div>
                    </div>
                    <span className="text-xs text-muted-foreground">
                      {new Date(item.played_at).toLocaleDateString()}
                    </span>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </motion.div>
      )}

      {/* Regcode Renew */}
      <motion.div variants={item}>
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Key className="h-5 w-5 text-primary" />
              使用注册码续期
            </CardTitle>
            <CardDescription>
              输入续期码来延长账号有效期
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="regcode">注册码</Label>
              <div className="flex gap-2">
                <Input
                  id="regcode"
                  placeholder="请输入续期码"
                  value={regCode}
                  onChange={(e: React.ChangeEvent<HTMLInputElement>) => setRegCode(e.target.value)}
                  onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) => {
                    if (e.key === "Enter" && regCode.trim()) {
                      handleCheckRegcode();
                    }
                  }}
                />
                <Button
                  variant="gradient"
                  onClick={handleCheckRegcode}
                  disabled={!regCode.trim() || isRenewing}
                >
                  {isRenewing ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      处理中...
                    </>
                  ) : (
                    "检查并使用"
                  )}
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </motion.div>

      {/* Confirm Dialog */}
      {showConfirm && regCodeInfo && (
        <Dialog open={showConfirm} onOpenChange={setShowConfirm}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <AlertTriangle className="h-5 w-5 text-amber-500" />
                确认使用注册码
              </DialogTitle>
              <DialogDescription>
                <div className="space-y-2 mt-2">
                  <p>注册码类型：<strong>{regCodeInfo.type_name}</strong></p>
                  <p>有效天数：<strong>{regCodeInfo.days} 天</strong></p>
                  <p className="text-sm text-muted-foreground mt-4">
                    确定要使用此注册码吗？使用后将无法撤销。
                  </p>
                </div>
              </DialogDescription>
            </DialogHeader>
            <div className="flex justify-end gap-2 mt-4">
              <Button variant="outline" onClick={() => setShowConfirm(false)}>
                取消
              </Button>
              <Button variant="gradient" onClick={handleConfirmUseRegcode}>
                确认使用
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      )}

      {/* Edit Score Dialog (Admin only) */}
      {user?.role === 0 && (
        <Dialog open={showEditScore} onOpenChange={setShowEditScore}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>修改积分</DialogTitle>
              <DialogDescription>
                管理员可以直接修改自己的积分
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>积分值</Label>
                <Input
                  type="number"
                  value={editScoreValue}
                  onChange={(e) => setEditScoreValue(parseInt(e.target.value) || 0)}
                  placeholder="输入新的积分值"
                />
              </div>
            </div>
            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={() => setShowEditScore(false)}>
                取消
              </Button>
              <Button onClick={handleEditScore} disabled={isEditingScore}>
                {isEditingScore && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                确认修改
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      )}
    </motion.div>
  );
}

