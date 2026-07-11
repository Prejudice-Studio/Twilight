"use client";

import { useCallback, useState } from "react";
import { motion } from "framer-motion";
import { Activity, RefreshCw, Loader2, Tv, LogIn, LogOut, Play, Pause, Filter } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";

interface ActivityLogEntry {
  id: number;
  emby_log_id: number;
  type: string;
  name: string;
  user_id: string;
  user_name: string;
  overview: string;
  date: number;
}

function formatDate(unix: number) {
  if (!unix) return "";
  return new Date(unix * 1000).toLocaleString();
}

function activityIcon(type: string) {
  const t = type.toLowerCase();
  if (t.includes("playback") && !t.includes("complete")) return <Play className="h-3.5 w-3.5 text-blue-500" />;
  if (t.includes("playback")) return <Pause className="h-3.5 w-3.5 text-green-500" />;
  if (t.includes("auth") || t.includes("login")) return <LogIn className="h-3.5 w-3.5 text-yellow-500" />;
  if (t.includes("session") && t.includes("end")) return <LogOut className="h-3.5 w-3.5 text-muted-foreground" />;
  if (t.includes("session")) return <LogIn className="h-3.5 w-3.5 text-yellow-500" />;
  return <Activity className="h-3.5 w-3.5 text-muted-foreground" />;
}

function activityLabel(type: string) {
  const t = type.toLowerCase();
  if (t === "videoplayback") return "开始播放";
  if (t === "videoplaybackcomplete") return "播放完成";
  if (t === "authenticationsucceeded") return "登录成功";
  if (t === "authenticationfailure") return "登录失败";
  if (t === "sessionstarted") return "会话开始";
  if (t === "sessionended") return "会话结束";
  return type;
}

export default function EmbyActivityLogs() {
  const { toast } = useToast();
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(async () => {
    const res = await api.adminGetEmbyActivityLogs(100);
    if (res.success && res.data) return res.data.entries || [];
    throw new Error(res.message || "加载失败");
  }, []);

  const { data: logs, isLoading, error, execute: reload } = useAsyncResource(load, { immediate: true });

  const handleRefresh = async () => {
    setRefreshing(true);
    try {
      const res = await api.adminGetEmbyActivityLogs(100, true);
      if (res.success) {
        await reload();
        toast({ title: "日志已刷新" });
      }
    } catch {
      toast({ title: "刷新失败", variant: "destructive" });
    } finally {
      setRefreshing(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-bold flex items-center gap-2">
            <Activity className="h-5 w-5 text-primary" />
            Emby 活动日志
          </h3>
          <p className="text-sm text-muted-foreground">来自 Emby 服务器的活动记录（登录、播放、停止等）</p>
        </div>
        <Button variant="outline" size="sm" onClick={handleRefresh} disabled={refreshing || isLoading}>
          {refreshing || isLoading ? <Loader2 className="mr-1.5 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-1.5 h-4 w-4" />}
          刷新
        </Button>
      </div>

      {error ? (
        <Card><CardContent className="p-6 text-center text-destructive">{String(error)}</CardContent></Card>
      ) : isLoading && !logs ? (
        <Card><CardContent className="p-8 flex justify-center"><Loader2 className="h-6 w-6 animate-spin" /></CardContent></Card>
      ) : !logs || logs.length === 0 ? (
        <Card><CardContent className="p-8 text-center text-sm text-muted-foreground">暂无活动日志，点击刷新从 Emby 拉取</CardContent></Card>
      ) : (
        <div className="space-y-1.5">
          {logs.map((log: ActivityLogEntry, idx: number) => (
            <div key={log.id || idx} className="flex items-start gap-3 rounded-lg border border-border/40 bg-accent/10 hover:bg-accent/20 transition-colors p-3 text-sm">
              <div className="mt-0.5 flex-shrink-0">{activityIcon(log.type)}</div>
              <div className="min-w-0 flex-1 space-y-0.5">
                <div className="flex items-center gap-2 flex-wrap">
                  <Badge variant="outline" className="text-[10px]">{activityLabel(log.type)}</Badge>
                  {log.user_name && <span className="font-medium text-xs truncate">{log.user_name}</span>}
                </div>
                <p className="text-xs text-muted-foreground truncate">{log.name}</p>
                {log.overview && <p className="text-[11px] text-muted-foreground/70 line-clamp-1">{log.overview}</p>}
              </div>
              <div className="text-[10px] text-muted-foreground/50 whitespace-nowrap flex-shrink-0">
                {formatDate(log.date)}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
