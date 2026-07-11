"use client";

import { useCallback, useState } from "react";
import { motion } from "framer-motion";
import { Activity, Tv, Clock, Users, Eye, EyeOff, Play, StopCircle } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import { useSystemStore } from "@/store/system";

function formatDuration(sec: number): string {
  if (!sec || sec < 0) return "0 分钟";
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  return `${m} 分钟`;
}

export default function PlaybackStatsPage() {
  const { toast } = useToast();
  const { user } = useAuthStore();
  const { info: systemInfo } = useSystemStore();
  const isAdmin = user?.role === 0;
  const [days, setDays] = useState("30");
  const [uid, setUid] = useState("");
  const [stats, setStats] = useState<any>(null);
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);

  const loadStats = useCallback(async () => {
    setLoading(true);
    try {
      const params: any = { days: Number(days) || 30 };
      if (uid && isAdmin) params.uid = Number(uid);
      const url = uid && isAdmin ? `/admin/emby/playback-stats/${uid}?days=${days}` : `/admin/emby/playback-stats?days=${days}`;
      const res = await fetch(`/api/v1${url}`, { credentials: "include" }).then(r => r.json());
      if (res.success && res.data) setStats(res.data);
      else toast({ title: "加载失败", description: res.message, variant: "destructive" });
    } catch {
      toast({ title: "加载异常", variant: "destructive" });
    } finally { setLoading(false); }
  }, [days, uid, isAdmin, toast]);

  const handleRefresh = async () => {
    setRefreshing(true);
    try {
      await api.adminGetEmbyActivityLogs(10, true);
      await loadStats();
      toast({ title: "已刷新" });
    } catch { toast({ title: "刷新失败", variant: "destructive" }); }
    finally { setRefreshing(false); }
  };

  const enabled = systemInfo?.features?.emby_playback_stats === true;

  if (!enabled) {
    return (
      <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }}>
        <Card><CardContent className="p-8 text-center text-sm text-muted-foreground">播放统计功能未启用，请联系管理员开启。</CardContent></Card>
      </motion.div>
    );
  }

  return (
    <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold flex items-center gap-2"><Activity className="h-6 w-6" />播放统计</h1>
        <p className="text-sm text-muted-foreground mt-1">独立于 Bangumi 同步的播放次数与时长统计</p>
      </div>

      <Card>
        <CardContent className="pt-4 space-y-3">
          <div className="flex flex-wrap items-end gap-3">
            <div className="space-y-1"><Label className="text-xs">统计天数</Label>
              <Select value={days} onValueChange={setDays}>
                <SelectTrigger className="w-24"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="7">7 天</SelectItem>
                  <SelectItem value="30">30 天</SelectItem>
                  <SelectItem value="90">90 天</SelectItem>
                  <SelectItem value="365">365 天</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {isAdmin && (
              <div className="space-y-1"><Label className="text-xs">用户 UID (可选)</Label>
                <Input value={uid} onChange={(e) => setUid(e.target.value)} placeholder="留空=全部" className="w-28 h-9 text-sm" />
              </div>
            )}
            <Button onClick={loadStats} disabled={loading}>{loading ? "加载中..." : "查询"}</Button>
            <Button variant="outline" onClick={handleRefresh} disabled={refreshing}>刷新活动日志</Button>
          </div>
        </CardContent>
      </Card>

      {stats && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <Card>
            <CardContent className="p-4 text-center">
              <Play className="h-5 w-5 mx-auto text-blue-500 mb-1" />
              <p className="text-2xl font-bold">{stats.total_plays ?? 0}</p>
              <p className="text-[10px] text-muted-foreground">播放次数</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4 text-center">
              <Clock className="h-5 w-5 mx-auto text-green-500 mb-1" />
              <p className="text-2xl font-bold">{formatDuration(stats.total_duration ?? 0)}</p>
              <p className="text-[10px] text-muted-foreground">总时长</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4 text-center">
              <Tv className="h-5 w-5 mx-auto text-purple-500 mb-1" />
              <p className="text-2xl font-bold">{stats.unique_items ?? 0}</p>
              <p className="text-[10px] text-muted-foreground">不同节目</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4 text-center">
              <Users className="h-5 w-5 mx-auto text-orange-500 mb-1" />
              <p className="text-2xl font-bold">{stats.days ?? "-"}</p>
              <p className="text-[10px] text-muted-foreground">统计天数</p>
            </CardContent>
          </Card>
        </div>
      )}

      {stats && stats.user_rankings && (
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-base flex items-center gap-2"><Users className="h-4 w-4" />播放排行</CardTitle></CardHeader>
          <CardContent>
            <div className="space-y-1 max-h-96 overflow-y-auto">
              {stats.user_rankings.map((u: any, i: number) => (
                <div key={i} className="flex items-center justify-between p-2 rounded bg-accent/20 text-sm">
                  <span><Badge variant="outline" className="text-[10px] mr-2">{i + 1}</Badge>{u.username || `UID:${u.uid}`}</span>
                  <span className="text-xs text-muted-foreground">{u.plays} 次 · {formatDuration(u.duration)}</span>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {!stats && !loading && (
        <Card><CardContent className="p-8 text-center text-sm text-muted-foreground">点击上方「查询」加载统计数据</CardContent></Card>
      )}
    </motion.div>
  );
}
