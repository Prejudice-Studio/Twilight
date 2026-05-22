"use client";

import type { ComponentType } from "react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Activity,
  CirclePause,
  CirclePlay,
  Cpu,
  Database,
  Loader2,
  MemoryStick,
  RefreshCw,
  Server,
  Trash2,
  Wifi,
  WifiOff,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { api, type RuntimeLogEntry, type RuntimeStatus } from "@/lib/api";
import { useToast } from "@/hooks/use-toast";

function formatTime(seconds?: number) {
  if (!seconds) return "未知";
  return new Date(seconds * 1000).toLocaleString();
}

function formatDuration(total?: number) {
  if (!total || total < 0) return "0 秒";
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = Math.floor(total % 60);
  return [
    days ? `${days} 天` : "",
    hours ? `${hours} 小时` : "",
    minutes ? `${minutes} 分钟` : "",
    !days && !hours ? `${seconds} 秒` : "",
  ].filter(Boolean).join(" ");
}

function formatBytes(value?: number) {
  if (!value || value < 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = value;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index++;
  }
  return `${size.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function levelVariant(level: string): "default" | "secondary" | "outline" | "destructive" | "success" {
  const value = level.toLowerCase();
  if (value.includes("error")) return "destructive";
  if (value.includes("warn")) return "secondary";
  if (value.includes("info")) return "success";
  return "outline";
}

function RuntimeStat({
  icon: Icon,
  label,
  value,
}: {
  icon: ComponentType<{ className?: string }>;
  label: string;
  value: string | number;
}) {
  return (
    <div className="flex min-w-0 items-center gap-3 rounded-lg border bg-background/80 p-3">
      <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary">
        <Icon className="h-4 w-4" />
      </div>
      <div className="min-w-0">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="truncate text-sm font-semibold">{value}</p>
      </div>
    </div>
  );
}

export default function AdminRuntimeLogsPage() {
  const { toast } = useToast();
  const [status, setStatus] = useState<RuntimeStatus | null>(null);
  const [logs, setLogs] = useState<RuntimeLogEntry[]>([]);
  const [cursor, setCursor] = useState(0);
  const [loading, setLoading] = useState(true);
  const [connected, setConnected] = useState(false);
  const [paused, setPaused] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const eventRef = useRef<EventSource | null>(null);
  const cursorRef = useRef(0);
  const bottomRef = useRef<HTMLDivElement | null>(null);

  const setNextCursor = useCallback((nextCursor?: number) => {
    if (!nextCursor || nextCursor < cursorRef.current) return;
    cursorRef.current = nextCursor;
    setCursor(nextCursor);
  }, []);

  const appendLogs = useCallback((entries: RuntimeLogEntry[], nextCursor?: number) => {
    if (entries.length > 0) {
      setLogs((current) => {
        const seen = new Set(current.map((entry) => entry.id));
        const merged = [...current];
        for (const entry of entries) {
          if (!seen.has(entry.id)) merged.push(entry);
        }
        return merged.slice(-500);
      });
    }
    setNextCursor(nextCursor);
  }, [setNextCursor]);

  const loadStatus = useCallback(async () => {
    const res = await api.getRuntimeStatus();
    if (res.success) setStatus(res.data || null);
  }, []);

  const loadSnapshot = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [statusRes, logsRes] = await Promise.all([
        api.getRuntimeStatus(),
        api.getRuntimeLogs(200),
      ]);
      if (statusRes.success) setStatus(statusRes.data || null);
      if (logsRes.success && logsRes.data) {
        setLogs(logsRes.data.entries || []);
        setNextCursor(logsRes.data.next_cursor || 0);
      }
    } catch (err: any) {
      const message = err?.message || "加载运行状态失败";
      setError(message);
      toast({ title: "加载失败", description: message, variant: "destructive" });
    } finally {
      setLoading(false);
    }
  }, [setNextCursor, toast]);

  useEffect(() => {
    void loadSnapshot();
  }, [loadSnapshot]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      void loadStatus().catch(() => undefined);
    }, 15000);
    return () => window.clearInterval(timer);
  }, [loadStatus]);

  useEffect(() => {
    if (paused) {
      eventRef.current?.close();
      eventRef.current = null;
      setConnected(false);
      return;
    }

    eventRef.current?.close();
    const source = new EventSource(api.runtimeLogStreamURL(100, cursorRef.current), { withCredentials: true });
    eventRef.current = source;

    const handlePayload = (event: MessageEvent) => {
      try {
        const payload = JSON.parse(event.data) as { entries?: RuntimeLogEntry[]; next_cursor?: number };
        appendLogs(payload.entries || [], payload.next_cursor);
      } catch {
        // Broken SSE frames are ignored; the connection will keep streaming valid frames.
      }
    };

    source.addEventListener("snapshot", handlePayload);
    source.addEventListener("logs", handlePayload);
    source.addEventListener("ping", () => setConnected(true));
    source.onopen = () => {
      setConnected(true);
      setError(null);
    };
    source.onerror = () => {
      setConnected(false);
    };

    return () => {
      source.close();
    };
  }, [appendLogs, paused]);

  useEffect(() => {
    if (!paused) bottomRef.current?.scrollIntoView({ block: "end" });
  }, [logs, paused]);

  const latestStatus = useMemo(() => {
    if (!status) return [];
    return [
      { icon: Server, label: "主机", value: status.hostname || "未知" },
      { icon: Activity, label: "进程运行", value: formatDuration(status.uptime_seconds) },
      { icon: MemoryStick, label: "堆内存", value: formatBytes(status.memory?.heap_alloc) },
      { icon: Database, label: "数据库", value: `${status.active_database || "unknown"} / ${status.users} 用户` },
    ];
  }, [status]);

  return (
    <div className="space-y-5">
      <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0">
          <h1 className="text-2xl font-bold sm:text-3xl">实时日志</h1>
          <p className="break-words text-sm text-muted-foreground">后端运行日志、进程状态和主机状态。</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Badge variant={connected ? "success" : "secondary"} className="h-9 gap-1.5 px-3">
            {connected ? <Wifi className="h-3.5 w-3.5" /> : <WifiOff className="h-3.5 w-3.5" />}
            {connected ? "实时连接" : "未连接"}
          </Badge>
          <Button variant="outline" onClick={() => setPaused((value) => !value)}>
            {paused ? <CirclePlay className="mr-2 h-4 w-4" /> : <CirclePause className="mr-2 h-4 w-4" />}
            {paused ? "继续" : "暂停"}
          </Button>
          <Button variant="outline" onClick={loadSnapshot} disabled={loading}>
            {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
            刷新
          </Button>
          <Button variant="outline" onClick={() => setLogs([])}>
            <Trash2 className="mr-2 h-4 w-4" />
            清屏
          </Button>
        </div>
      </div>

      {error && (
        <Card className="border-destructive/40 bg-destructive/5">
          <CardContent className="p-4 text-sm text-destructive">{error}</CardContent>
        </Card>
      )}

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {latestStatus.length > 0 ? latestStatus.map((item) => (
          <RuntimeStat key={item.label} icon={item.icon} label={item.label} value={item.value} />
        )) : (
          <Card className="sm:col-span-2 xl:col-span-4">
            <CardContent className="flex h-24 items-center justify-center">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </CardContent>
          </Card>
        )}
      </div>

      <Card className="overflow-hidden">
        <CardHeader className="flex flex-row items-center justify-between gap-3 border-b p-4">
          <CardTitle className="text-base">日志流</CardTitle>
          <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
            <span className="whitespace-nowrap">{logs.length} 行</span>
            <span className="truncate">游标 {cursor}</span>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <div className="h-[min(62vh,42rem)] overflow-y-auto overflow-x-hidden bg-zinc-950 p-3 text-xs text-zinc-100">
            {logs.length === 0 ? (
              <div className="flex h-full items-center justify-center text-zinc-500">暂无日志</div>
            ) : (
              <div className="space-y-1 font-mono">
                {logs.map((entry) => (
                  <div key={entry.id} className="grid min-w-0 gap-2 rounded px-2 py-1 hover:bg-white/5 md:grid-cols-[8rem_5rem_minmax(0,1fr)]">
                    <span className="break-all text-zinc-500">{formatTime(entry.time)}</span>
                    <Badge variant={levelVariant(entry.level)} className="h-5 w-fit rounded px-1.5 py-0 text-[10px] uppercase">
                      {entry.level}
                    </Badge>
                    <span className="min-w-0 break-all text-zinc-100">
                      {entry.message}
                      {entry.attrs && Object.keys(entry.attrs).length > 0 && (
                        <span className="ml-2 text-zinc-400">
                          {Object.entries(entry.attrs).map(([key, value]) => `${key}=${value}`).join(" ")}
                        </span>
                      )}
                    </span>
                  </div>
                ))}
                <div ref={bottomRef} />
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {status && (
        <div className="grid gap-3 lg:grid-cols-3">
          <Card>
            <CardHeader><CardTitle className="flex items-center gap-2 text-base"><Cpu className="h-4 w-4" />Go 运行时</CardTitle></CardHeader>
            <CardContent className="space-y-2 text-sm text-muted-foreground">
              <p className="break-all">版本：{status.go_version}</p>
              <p>平台：{status.goos}/{status.goarch}</p>
              <p>协程：{status.goroutines}</p>
              <p>CPU：{status.cpu_count}</p>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle className="text-base">服务状态</CardTitle></CardHeader>
            <CardContent className="space-y-2 text-sm text-muted-foreground">
              <p>启动时间：{formatTime(status.started_at)}</p>
              <p>Redis：{status.redis_enabled ? "启用" : "未启用"}</p>
              <p>路由数：{status.routes}</p>
              <p>主机运行：{formatDuration(status.host_uptime_seconds)}</p>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle className="text-base">主机负载</CardTitle></CardHeader>
            <CardContent className="space-y-2 text-sm text-muted-foreground">
              <p>负载：{status.load_average?.join(" / ") || "不可用"}</p>
              <p>总内存：{formatBytes((status.host_memory?.total_kb || 0) * 1024)}</p>
              <p>可用内存：{formatBytes((status.host_memory?.available_kb || 0) * 1024)}</p>
              <p>缓存：{formatBytes((status.host_memory?.cached_kb || 0) * 1024)}</p>
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}
