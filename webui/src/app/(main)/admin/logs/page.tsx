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
import { useI18n } from "@/lib/i18n";
import { useVisiblePolling } from "@/hooks/use-visible-polling";

function formatTime(seconds: number | undefined, unknown: string) {
  if (!seconds) return unknown;
  return new Date(seconds * 1000).toLocaleString();
}

function formatDuration(total: number | undefined, units: { second: string; minute: string; hour: string; day: string }) {
  if (!total || total < 0) return units.second.replace("{value}", "0");
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = Math.floor(total % 60);
  return [
    days ? units.day.replace("{value}", String(days)) : "",
    hours ? units.hour.replace("{value}", String(hours)) : "",
    minutes ? units.minute.replace("{value}", String(minutes)) : "",
    !days && !hours ? units.second.replace("{value}", String(seconds)) : "",
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

interface RuntimeLogState {
  entries: RuntimeLogEntry[];
  ids: Set<number>;
}

function runtimeLogState(entries: RuntimeLogEntry[], limit: number): RuntimeLogState {
  const ordered = [...entries].sort((a, b) => a.id - b.id).slice(-limit);
  return {
    entries: ordered,
    ids: new Set(ordered.map((entry) => entry.id)),
  };
}

function appendRuntimeLogs(current: RuntimeLogState, entries: RuntimeLogEntry[], limit: number): RuntimeLogState {
  if (entries.length === 0) return current;
  const ids = new Set(current.ids);
  const merged = [...current.entries];
  let changed = false;
  let ordered = true;
  let lastId = merged.length > 0 ? merged[merged.length - 1].id : Number.NEGATIVE_INFINITY;

  for (const entry of entries) {
    if (ids.has(entry.id)) continue;
    ids.add(entry.id);
    merged.push(entry);
    changed = true;
    if (entry.id < lastId) ordered = false;
    if (entry.id > lastId) lastId = entry.id;
  }
  if (!changed) return current;
  if (!ordered) merged.sort((a, b) => a.id - b.id);
  if (merged.length <= limit) return { entries: merged, ids };

  const trimmed = merged.slice(-limit);
  return {
    entries: trimmed,
    ids: new Set(trimmed.map((entry) => entry.id)),
  };
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
  const { t } = useI18n();
  const durationUnits = useMemo(() => ({
    second: t("adminLogs.seconds", { value: "{value}" }),
    minute: t("adminLogs.minutes", { value: "{value}" }),
    hour: t("adminLogs.hours", { value: "{value}" }),
    day: t("adminLogs.days", { value: "{value}" }),
  }), [t]);
  const [status, setStatus] = useState<RuntimeStatus | null>(null);
  const [logState, setLogState] = useState<RuntimeLogState>(() => runtimeLogState([], 500));
  const logs = logState.entries;
  const [cursor, setCursor] = useState(0);
  const [loading, setLoading] = useState(true);
  const [connected, setConnected] = useState(false);
  const [paused, setPaused] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [logLimit, setLogLimit] = useState(500);
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
      setLogState((current) => appendRuntimeLogs(current, entries, logLimit));
    }
    setNextCursor(nextCursor);
  }, [logLimit, setNextCursor]);

  const loadStatus = useCallback(async (signal?: AbortSignal) => {
    const res = await api.getRuntimeStatus(signal);
    if (res.success) setStatus(res.data || null);
  }, []);

  const loadSnapshot = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    setError(null);
    try {
      const [statusRes, logsRes] = await Promise.all([
        api.getRuntimeStatus(signal),
        api.getRuntimeLogs(logLimit, undefined, signal),
      ]);
      if (signal?.aborted) return;
      if (statusRes.success) setStatus(statusRes.data || null);
      if (logsRes.success && logsRes.data) {
        setLogState(runtimeLogState(logsRes.data.entries || [], logLimit));
        cursorRef.current = logsRes.data.next_cursor || 0;
        setCursor(logsRes.data.next_cursor || 0);
        setNextCursor(logsRes.data.next_cursor || 0);
      }
    } catch (err: any) {
      if (signal?.aborted || err?.name === "AbortError") return;
      const message = err?.message || t("adminLogs.loadStatusFailed");
      setError(message);
      toast({ title: t("adminLogs.loadFailed"), description: message, variant: "destructive" });
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, [logLimit, setNextCursor, t, toast]);

  const loadMore = useCallback(async () => {
    const nextLimit = Math.min(status?.runtime_log_limit || 5000, logLimit + 500);
    setLogLimit(nextLimit);
    const res = await api.getRuntimeLogs(nextLimit);
    if (res.success && res.data) {
      setLogState(runtimeLogState(res.data.entries || [], nextLimit));
      setNextCursor(res.data.next_cursor || 0);
    }
  }, [logLimit, setNextCursor, status?.runtime_log_limit]);

  useEffect(() => {
    const controller = new AbortController();
    void loadSnapshot(controller.signal);
    return () => controller.abort();
  }, [loadSnapshot]);

  useVisiblePolling(loadStatus, 15000);

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

    const handlePing = () => setConnected(true);

    source.addEventListener("snapshot", handlePayload);
    source.addEventListener("logs", handlePayload);
    source.addEventListener("ping", handlePing);
    source.onopen = () => {
      setConnected(true);
      setError(null);
    };
    source.onerror = () => {
      setConnected(false);
      setError(t("adminLogs.streamDisconnected"));
    };

    return () => {
      source.removeEventListener("snapshot", handlePayload);
      source.removeEventListener("logs", handlePayload);
      source.removeEventListener("ping", handlePing);
      source.close();
      if (eventRef.current === source) {
        eventRef.current = null;
      }
    };
  }, [appendLogs, paused, t]);

  useVisiblePolling(
    async (signal?: AbortSignal) => {
      if (eventRef.current?.readyState === EventSource.CONNECTING) return;
      try {
        const res = await api.getRuntimeLogs(200, cursorRef.current, signal);
        if (res.success && res.data) {
          appendLogs(res.data.entries || [], res.data.next_cursor);
          setError(null);
        }
      } catch {
        if (!signal?.aborted) setError(t("adminLogs.pollFailed"));
      }
    },
    2500,
    !paused && !connected,
  );

  useEffect(() => {
    if (!paused) bottomRef.current?.scrollIntoView({ block: "end" });
  }, [logs, paused]);

  const latestStatus = useMemo(() => {
    if (!status) return [];
    return [
      { icon: Server, label: t("adminLogs.host"), value: status.hostname || t("adminLogs.unknown") },
      { icon: Activity, label: t("adminLogs.processUptime"), value: formatDuration(status.uptime_seconds, durationUnits) },
      { icon: MemoryStick, label: t("adminLogs.heapMemory"), value: formatBytes(status.memory?.heap_alloc) },
      { icon: Database, label: t("adminLogs.database"), value: t("adminLogs.databaseUsers", { database: status.active_database || "unknown", users: status.users }) },
    ];
  }, [durationUnits, status, t]);

  return (
    <div className="space-y-5">
      <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0">
          <h1 className="text-2xl font-bold sm:text-3xl">{t("adminLogs.title")}</h1>
          <p className="break-words text-sm text-muted-foreground">{t("adminLogs.description")}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Badge variant={connected ? "success" : "secondary"} className="h-9 gap-1.5 px-3">
            {connected ? <Wifi className="h-3.5 w-3.5" /> : <WifiOff className="h-3.5 w-3.5" />}
            {connected ? t("adminLogs.connected") : t("adminLogs.disconnected")}
          </Badge>
          <Button variant="outline" onClick={() => setPaused((value) => !value)}>
            {paused ? <CirclePlay className="mr-2 h-4 w-4" /> : <CirclePause className="mr-2 h-4 w-4" />}
            {paused ? t("adminLogs.resume") : t("adminLogs.pause")}
          </Button>
          <Button variant="outline" onClick={() => void loadSnapshot()} disabled={loading}>
            {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
            {t("common.refresh")}
          </Button>
          <Button variant="outline" onClick={loadMore} disabled={loading || (status?.runtime_log_limit ? logLimit >= status.runtime_log_limit : false)}>
            {t("adminLogs.more")}
          </Button>
          <Button variant="outline" onClick={() => setLogState(runtimeLogState([], logLimit))}>
            <Trash2 className="mr-2 h-4 w-4" />
            {t("adminLogs.clearScreen")}
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
          <CardTitle className="text-base">{t("adminLogs.stream")}</CardTitle>
          <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
            <span className="whitespace-nowrap">{t("adminLogs.lineCount", { current: logs.length, limit: status?.runtime_log_limit || logLimit })}</span>
            <span className="truncate">{t("adminLogs.cursor", { cursor })}</span>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <div className="h-[min(62vh,42rem)] overflow-y-auto overflow-x-hidden bg-zinc-950 p-3 text-xs text-zinc-100">
            {logs.length === 0 ? (
              <div className="flex h-full items-center justify-center text-zinc-500">{t("adminLogs.empty")}</div>
            ) : (
              <div className="space-y-1 font-mono">
                {logs.map((entry) => (
                  <div key={entry.id} className="grid min-w-0 gap-2 rounded px-2 py-1 hover:bg-white/5 md:grid-cols-[8rem_5rem_minmax(0,1fr)]">
                    <span className="break-all text-zinc-500">{formatTime(entry.time, t("adminLogs.unknown"))}</span>
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
            <CardHeader><CardTitle className="flex items-center gap-2 text-base"><Cpu className="h-4 w-4" />{t("adminLogs.goRuntime")}</CardTitle></CardHeader>
            <CardContent className="space-y-2 text-sm text-muted-foreground">
              <p className="break-all">{t("adminLogs.version", { value: status.go_version })}</p>
              <p>{t("adminLogs.platform", { value: `${status.goos}/${status.goarch}` })}</p>
              <p>{t("adminLogs.goroutines", { value: status.goroutines })}</p>
              <p>{t("adminLogs.cpu", { value: status.cpu_count })}</p>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle className="text-base">{t("adminLogs.serviceStatus")}</CardTitle></CardHeader>
            <CardContent className="space-y-2 text-sm text-muted-foreground">
              <p>{t("adminLogs.startedAt", { value: formatTime(status.started_at, t("adminLogs.unknown")) })}</p>
              <p>{t("adminLogs.redis", { value: status.redis_enabled ? t("adminLogs.enabled") : t("adminLogs.notEnabled") })}</p>
              <p>{t("adminLogs.logLevel", { value: status.log_level || "info" })}</p>
              <p>{t("adminLogs.logBackend", { value: status.runtime_log_backend || status.active_database || "unknown" })}</p>
              <p>{t("adminLogs.logBuffer", { current: status.runtime_log_entries ?? logs.length, limit: status.runtime_log_limit ?? logLimit })}</p>
              <p>{t("adminLogs.routes", { value: status.routes })}</p>
              <p>{t("adminLogs.hostUptime", { value: formatDuration(status.host_uptime_seconds, durationUnits) })}</p>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle className="text-base">{t("adminLogs.hostLoad")}</CardTitle></CardHeader>
            <CardContent className="space-y-2 text-sm text-muted-foreground">
              <p>{t("adminLogs.load", { value: status.load_average?.join(" / ") || t("adminLogs.unavailable") })}</p>
              <p>{t("adminLogs.totalMemory", { value: formatBytes((status.host_memory?.total_kb || 0) * 1024) })}</p>
              <p>{t("adminLogs.availableMemory", { value: formatBytes((status.host_memory?.available_kb || 0) * 1024) })}</p>
              <p>{t("adminLogs.cachedMemory", { value: formatBytes((status.host_memory?.cached_kb || 0) * 1024) })}</p>
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}
