"use client";

import { useEffect, useState } from "react";
import { motion } from "framer-motion";
import {
  Coins,
  Send,
  ArrowDownLeft,
  ArrowUpRight,
  Clock,
  Gift,
  RefreshCw,
  Loader2,
  Trophy,
  Ticket,
  Plus,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { useToast } from "@/hooks/use-toast";
import { useAuthStore } from "@/store/auth";
import { api, type ScoreInfo, type ScoreRecord, type ScoreRankingItem } from "@/lib/api";
import { formatNumber, formatDate } from "@/lib/utils";

const container = {
  hidden: { opacity: 0 },
  show: {
    opacity: 1,
    transition: { staggerChildren: 0.1 },
  },
};

const item = {
  hidden: { opacity: 0, y: 20 },
  show: { opacity: 1, y: 0 },
};

export default function ScorePage() {
  const { toast } = useToast();
  const { fetchUser } = useAuthStore();
  const [scoreInfo, setScoreInfo] = useState<ScoreInfo | null>(null);
  const [history, setHistory] = useState<ScoreRecord[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  // Transfer state
  const [transferOpen, setTransferOpen] = useState(false);
  const [transferTo, setTransferTo] = useState("");
  const [transferAmount, setTransferAmount] = useState("");
  const [transferNote, setTransferNote] = useState("");
  const [isTransferring, setIsTransferring] = useState(false);

  // Renew state
  const [renewOpen, setRenewOpen] = useState(false);
  const [renewDays, setRenewDays] = useState("30");
  const [isRenewing, setIsRenewing] = useState(false);
  
  // Ranking state
  const [ranking, setRanking] = useState<ScoreRankingItem[]>([]);
  const [isRankingLoading, setIsRankingLoading] = useState(false);

  // Red Packet state
  const [redPacketOpen, setRedPacketOpen] = useState(false);
  const [rpAmount, setRpAmount] = useState("");
  const [rpCount, setRpCount] = useState("");
  const [rpType, setRpType] = useState("1"); // 1: Random, 2: Equal
  const [isCreatingRP, setIsCreatingRP] = useState(false);
  const [grabRPKey, setGrabRPKey] = useState("");
  const [isGrabbingRP, setIsGrabbingRP] = useState(false);

  useEffect(() => {
    loadData();
  }, []);

  const loadData = async () => {
    try {
      const [scoreRes, historyRes] = await Promise.all([
        api.getScoreInfo(),
        api.getScoreHistory(),
      ]);
      if (scoreRes.success && scoreRes.data) {
        setScoreInfo(scoreRes.data);
      }
      if (historyRes.success && historyRes.data) {
        setHistory(historyRes.data.records);
      }
    } catch (error) {
      console.error(error);
    } finally {
      setIsLoading(false);
    }
  };

  const handleTransfer = async () => {
    if (!transferTo || !transferAmount) {
      toast({ title: "请填写完整信息", variant: "destructive" });
      return;
    }

    setIsTransferring(true);
    try {
      const res = await api.transferScore(
        parseInt(transferTo),
        parseInt(transferAmount),
        transferNote || undefined
      );

      if (res.success) {
        toast({ title: "转账成功", variant: "success" });
        setTransferOpen(false);
        setTransferTo("");
        setTransferAmount("");
        setTransferNote("");
        loadData();
        fetchUser();
      } else {
        toast({ title: "转账失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "转账失败", description: error.message, variant: "destructive" });
    } finally {
      setIsTransferring(false);
    }
  };

  const handleRenew = async () => {
    if (!renewDays) {
      toast({ title: "请输入续期天数", variant: "destructive" });
      return;
    }

    setIsRenewing(true);
    try {
      const res = await api.renewWithScore(parseInt(renewDays));

      if (res.success) {
        toast({ title: "续期成功", variant: "success" });
        setRenewOpen(false);
        setRenewDays("30");
        loadData();
        fetchUser();
      } else {
        toast({ title: "续期失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "续期失败", description: error.message, variant: "destructive" });
    } finally {
      setIsRenewing(false);
    }
  };

  const loadRanking = async () => {
    setIsRankingLoading(true);
    try {
      const res = await api.getScoreRanking();
      if (res.success && res.data) {
        setRanking(res.data.ranking);
      }
    } catch (error) {
      console.error(error);
    } finally {
      setIsRankingLoading(false);
    }
  };

  const handleCreateRedPacket = async () => {
    if (!rpAmount || !rpCount) {
      toast({ title: "请填写完整信息", variant: "destructive" });
      return;
    }

    setIsCreatingRP(true);
    try {
      const res = await api.createRedPacket(
        parseInt(rpAmount),
        parseInt(rpCount),
        parseInt(rpType)
      );

      if (res.success) {
        toast({ 
          title: "红包创建成功", 
          description: `Key: ${res.data?.rp_key} (已复制到剪贴板)`,
          variant: "success" 
        });
        if (res.data?.rp_key) {
          navigator.clipboard.writeText(res.data.rp_key);
        }
        setRedPacketOpen(false);
        setRpAmount("");
        setRpCount("");
        loadData();
        fetchUser();
      } else {
        toast({ title: "创建失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "创建失败", description: error.message, variant: "destructive" });
    } finally {
      setIsCreatingRP(false);
    }
  };

  const handleGrabRedPacket = async () => {
    if (!grabRPKey) {
      toast({ title: "请输入红包口令", variant: "destructive" });
      return;
    }

    setIsGrabbingRP(true);
    try {
      const res = await api.grabRedPacket(grabRPKey);
      if (res.success) {
        toast({ 
          title: "抢红包成功", 
          description: `恭喜获得 ${res.data?.amount} ${scoreInfo?.score_name || '积分'}`,
          variant: "success" 
        });
        setGrabRPKey("");
        loadData();
        fetchUser();
      } else {
        toast({ title: "抢红包失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "抢红包失败", description: error.message, variant: "destructive" });
    } finally {
      setIsGrabbingRP(false);
    }
  };

  const getRecordIcon = (type: string) => {
    switch (type) {
      case "checkin":
        return <Gift className="h-4 w-4 text-emerald-500" />;
      case "transfer_in":
        return <ArrowDownLeft className="h-4 w-4 text-blue-500" />;
      case "transfer_out":
        return <ArrowUpRight className="h-4 w-4 text-orange-500" />;
      case "renew":
        return <RefreshCw className="h-4 w-4 text-purple-500" />;
      default:
        return <Coins className="h-4 w-4 text-primary" />;
    }
  };

  const getRecordLabel = (type: string) => {
    switch (type) {
      case "checkin":
        return "签到奖励";
      case "transfer_in":
        return "收到转账";
      case "transfer_out":
        return "转出积分";
      case "renew":
        return "积分续期";
      case "red_packet":
        return "红包";
      default:
        return type;
    }
  };

  return (
    <motion.div
      variants={container}
      initial="hidden"
      animate="show"
      className="space-y-6"
    >
      <div>
        <h1 className="text-3xl font-bold">积分中心</h1>
        <p className="text-muted-foreground">管理您的{scoreInfo?.score_name || '积分'}，转账、续期</p>
      </div>

      {/* Stats */}
      <div className="grid gap-4 md:grid-cols-3">
        <motion.div variants={item}>
          <Card className="relative overflow-hidden">
            <div className="absolute right-0 top-0 h-32 w-32 translate-x-8 translate-y-[-50%] rounded-full bg-gradient-to-br from-twilight-500/20 to-transparent" />
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                当前余额
              </CardTitle>
              <Coins className="h-4 w-4 text-twilight-500" />
            </CardHeader>
            <CardContent>
              <div className="text-3xl font-bold">
                {formatNumber(scoreInfo?.balance || 0)}
              </div>
              <p className="text-xs text-muted-foreground">
                {scoreInfo?.score_name || '积分'}
              </p>
            </CardContent>
          </Card>
        </motion.div>

        <motion.div variants={item}>
          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                累计获得
              </CardTitle>
              <ArrowDownLeft className="h-4 w-4 text-emerald-500" />
            </CardHeader>
            <CardContent>
              <div className="text-3xl font-bold text-emerald-500">
                +{formatNumber(scoreInfo?.total_earned || 0)}
              </div>
            </CardContent>
          </Card>
        </motion.div>

        <motion.div variants={item}>
          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                累计消费
              </CardTitle>
              <ArrowUpRight className="h-4 w-4 text-orange-500" />
            </CardHeader>
            <CardContent>
              <div className="text-3xl font-bold text-orange-500">
                -{formatNumber(scoreInfo?.total_spent || 0)}
              </div>
            </CardContent>
          </Card>
        </motion.div>
      </div>

      <Tabs defaultValue="history" className="w-full">
        <TabsList className="grid w-full grid-cols-3 mb-6">
          <TabsTrigger value="history">积分历史</TabsTrigger>
          <TabsTrigger value="ranking" onClick={loadRanking}>榜单</TabsTrigger>
          <TabsTrigger value="redpacket">红包</TabsTrigger>
        </TabsList>

        <TabsContent value="history">
          {/* Actions */}
          <motion.div variants={item} className="mb-6">
            <Card>
              <CardHeader>
                <CardTitle>快捷操作</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-wrap gap-3">
                <Dialog open={transferOpen} onOpenChange={setTransferOpen}>
                  <DialogTrigger asChild>
                    <Button variant="outline">
                      <Send className="mr-2 h-4 w-4" />
                      转账
                    </Button>
                  </DialogTrigger>
                  <DialogContent>
                    <DialogHeader>
                      <DialogTitle>积分转账</DialogTitle>
                      <DialogDescription>
                        将积分转给其他用户
                      </DialogDescription>
                    </DialogHeader>
                    <div className="space-y-4">
                      <div className="space-y-2">
                        <Label>对方 UID</Label>
                        <Input
                          type="number"
                          placeholder="输入对方的 UID"
                          value={transferTo}
                          onChange={(e) => setTransferTo(e.target.value)}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label>转账金额</Label>
                        <Input
                          type="number"
                          placeholder="输入转账金额"
                          value={transferAmount}
                          onChange={(e) => setTransferAmount(e.target.value)}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label>备注（可选）</Label>
                        <Input
                          placeholder="转账备注"
                          value={transferNote}
                          onChange={(e) => setTransferNote(e.target.value)}
                        />
                      </div>
                    </div>
                    <DialogFooter>
                      <Button variant="outline" onClick={() => setTransferOpen(false)}>
                        取消
                      </Button>
                      <Button onClick={handleTransfer} disabled={isTransferring} variant="gradient">
                        {isTransferring && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                        确认转账
                      </Button>
                    </DialogFooter>
                  </DialogContent>
                </Dialog>

                <Dialog open={renewOpen} onOpenChange={setRenewOpen}>
                  <DialogTrigger asChild>
                    <Button variant="outline">
                      <RefreshCw className="mr-2 h-4 w-4" />
                      积分续期
                    </Button>
                  </DialogTrigger>
                  <DialogContent>
                    <DialogHeader>
                      <DialogTitle>积分续期</DialogTitle>
                      <DialogDescription>
                        使用积分延长账号时间
                      </DialogDescription>
                    </DialogHeader>
                    <div className="space-y-4">
                      <div className="space-y-2">
                        <Label>续期天数</Label>
                        <Input
                          type="number"
                          placeholder="输入续期天数"
                          value={renewDays}
                          onChange={(e) => setRenewDays(e.target.value)}
                        />
                      </div>
                      <p className="text-sm text-muted-foreground">
                        预计消耗: {parseInt(renewDays || "0") * 10} {scoreInfo?.score_name || '积分'}
                      </p>
                    </div>
                    <DialogFooter>
                      <Button variant="outline" onClick={() => setRenewOpen(false)}>
                        取消
                      </Button>
                      <Button onClick={handleRenew} disabled={isRenewing} variant="gradient">
                        {isRenewing && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                        确认续期
                      </Button>
                    </DialogFooter>
                  </DialogContent>
                </Dialog>
              </CardContent>
            </Card>
          </motion.div>

          {/* History */}
          <motion.div variants={item}>
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Clock className="h-5 w-5" />
                  积分记录
                </CardTitle>
              </CardHeader>
              <CardContent>
                {isLoading ? (
                  <div className="flex h-32 items-center justify-center">
                    <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                  </div>
                ) : history.length === 0 ? (
                  <div className="flex h-32 items-center justify-center text-muted-foreground">
                    暂无积分记录
                  </div>
                ) : (
                  <div className="space-y-3">
                    {history.map((record) => (
                      <div
                        key={record.id}
                        className="flex items-center justify-between rounded-lg bg-accent/30 p-3"
                      >
                        <div className="flex items-center gap-3">
                          <div className="flex h-10 w-10 items-center justify-center rounded-full bg-background">
                            {getRecordIcon(record.type)}
                          </div>
                          <div>
                            <p className="font-medium">{getRecordLabel(record.type)}</p>
                            <p className="text-xs text-muted-foreground">
                              {record.note || formatDate(record.created_at)}
                            </p>
                          </div>
                        </div>
                        <div className="text-right">
                          <p className={`font-bold ${record.amount > 0 ? 'text-emerald-500' : 'text-orange-500'}`}>
                            {record.amount > 0 ? '+' : ''}{formatNumber(record.amount)}
                          </p>
                          <p className="text-xs text-muted-foreground">
                            余额: {formatNumber(record.balance_after)}
                          </p>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </motion.div>
        </TabsContent>

        <TabsContent value="ranking">
          <motion.div variants={item}>
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Trophy className="h-5 w-5 text-yellow-500" />
                  积分排行榜
                </CardTitle>
                <CardDescription>看看谁才是暮光之巅</CardDescription>
              </CardHeader>
              <CardContent>
                {isRankingLoading ? (
                  <div className="flex h-32 items-center justify-center">
                    <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                  </div>
                ) : ranking.length === 0 ? (
                  <div className="flex h-32 items-center justify-center text-muted-foreground">
                    排行榜加载失败
                  </div>
                ) : (
                  <div className="space-y-2">
                    {ranking.map((row, idx) => (
                      <div
                        key={row.uid}
                        className={`flex items-center justify-between rounded-lg p-3 ${
                          idx === 0
                            ? "bg-yellow-500/10 border border-yellow-500/20"
                            : idx === 1
                            ? "bg-slate-400/10 border border-slate-400/20"
                            : idx === 2
                            ? "bg-amber-600/10 border border-amber-600/20"
                            : "bg-accent/30"
                        }`}
                      >
                        <div className="flex items-center gap-4">
                          <div className={`flex h-8 w-8 items-center justify-center rounded-full font-bold ${
                            idx === 0 ? "text-yellow-600" : idx === 1 ? "text-slate-500" : idx === 2 ? "text-amber-700" : "text-muted-foreground"
                          }`}>
                            {idx + 1}
                          </div>
                          <div>
                            <p className="font-medium">{row.username}</p>
                            <p className="text-xs text-muted-foreground">UID: {row.uid}</p>
                          </div>
                        </div>
                        <div className="text-right">
                          <p className="font-bold text-primary">{formatNumber(row.score)}</p>
                          <p className="text-xs text-muted-foreground">连签 {row.checkin_days} 天</p>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </motion.div>
        </TabsContent>

        <TabsContent value="redpacket">
          <div className="grid gap-6 md:grid-cols-2">
            <motion.div variants={item}>
              <Card className="h-full">
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <Plus className="h-5 w-5 text-primary" />
                    发红包
                  </CardTitle>
                  <CardDescription>独乐乐不如众乐乐</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="space-y-2">
                    <Label>总金额</Label>
                    <Input 
                      type="number" 
                      placeholder="总积分金额" 
                      value={rpAmount}
                      onChange={(e) => setRpAmount(e.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label>红包个数</Label>
                    <Input 
                      type="number" 
                      placeholder="分几个红包" 
                      value={rpCount}
                      onChange={(e) => setRpCount(e.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label>类型</Label>
                    <div className="flex gap-2">
                      <Button 
                        variant={rpType === "1" ? "default" : "outline"} 
                        className="flex-1"
                        onClick={() => setRpType("1")}
                      >
                        拼手气
                      </Button>
                      <Button 
                        variant={rpType === "2" ? "default" : "outline"} 
                        className="flex-1"
                        onClick={() => setRpType("2")}
                      >
                        普通
                      </Button>
                    </div>
                  </div>
                  <Button 
                    className="w-full" 
                    variant="gradient"
                    onClick={handleCreateRedPacket}
                    disabled={isCreatingRP}
                  >
                    {isCreatingRP && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    塞进红包
                  </Button>
                </CardContent>
              </Card>
            </motion.div>

            <motion.div variants={item}>
              <Card className="h-full">
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <Ticket className="h-5 w-5 text-orange-500" />
                    抢红包
                  </CardTitle>
                  <CardDescription>输入红包口令领取惊喜</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="space-y-2">
                    <Label>红包口令</Label>
                    <Input 
                      placeholder="rp-xxxxxx" 
                      value={grabRPKey}
                      onChange={(e) => setGrabRPKey(e.target.value)}
                    />
                  </div>
                  <Button 
                    className="w-full" 
                    variant="gradient"
                    onClick={handleGrabRedPacket}
                    disabled={isGrabbingRP}
                  >
                    {isGrabbingRP && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    立即领取
                  </Button>
                </CardContent>
              </Card>
            </motion.div>
          </div>
        </TabsContent>
      </Tabs>
    </motion.div>
  );
}

