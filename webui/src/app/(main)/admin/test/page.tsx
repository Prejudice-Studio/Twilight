"use client";

import { useEffect, useState } from "react";
import { Activity, Database, Loader2, RefreshCw, Server, ShieldCheck, XCircle } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { api, type SystemStats } from "@/lib/api";
import { useToast } from "@/hooks/use-toast";

const FEATURE_LABELS: Record<string, string> = {
  register: "开放注册",
  score_register: "积分注册",
  telegram: "Telegram Bot",
  webhook: "Webhook 通知",
  red_packet: "红包功能",
  transfer: "转账功能",
  auto_renew: "自动续期",
  invite: "邀请系统",
  force_bind_telegram: "强制绑定 Telegram",
};

const LIMIT_LABELS: Record<string, string> = {
  user_limit: "用户上限",
  device_limit: "设备上限",
  stream_limit: "同时播放上限",
};

interface SystemHealthInfo {
  api: boolean;
  database: boolean;
  emby: boolean;
}

interface SystemStatsInfo {
  timestamp: number;
  cpu_count: number | null;
  cpu_percent?: number | null;
  memory?: {
    total: number;
    available: number;
    percent: number;
    used: number;
  } | null;
  disk?: {
    total: number;
    free: number;
    percent: number;
  } | null;
}

interface ExtendedSystemStats extends SystemStats {
  emby?: {
    active_sessions?: number;
    online?: boolean;
    operating_system?: string;
    server_name?: string;
    total_sessions?: number;
    version?: string;
  };
  regcodes?: {
    active?: number;
    total?: number;
  };
  users?: {
    active?: number;
    limit?: number | null;
    total?: number;
    usage_percent?: number;
  };
}

export default function AdminStatusPage() {
  const { toast } = useToast();
  const [health, setHealth] = useState<SystemHealthInfo | null>(null);
  const [info, setInfo] = useState<any>(null);
  const [stats, setStats] = useState<ExtendedSystemStats | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadStatus = async () => {
    setLoading(true);
    setError(null);
    try {
      const [healthRes, infoRes, statsRes] = await Promise.all([
        api.getSystemHealth(),
        api.getSystemInfo(),
        api.getSystemStats(),
      ]);

      if (!healthRes.success) {
        throw new Error(healthRes.message || "健康检查失败");
      }
      if (!infoRes.success) {
        throw new Error(infoRes.message || "系统信息获取失败");
      }
      if (!statsRes.success) {
        throw new Error(statsRes.message || "系统统计获取失败");
      }

      setHealth(healthRes.data || null);
      setInfo(infoRes.data || null);
      setStats(statsRes.data || null);
      toast({ title: "系统状态已更新", variant: "success" });
    } catch (err: any) {
      setError(err?.message || "加载系统状态时出错");
      toast({ title: "加载失败", description: err?.message || String(err), variant: "destructive" });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadStatus();
  }, []);

  const renderStatusItem = (flag: boolean | undefined, label: string) => {
    const healthy = Boolean(flag);
    return (
      <div className="flex items-center justify-between gap-3 rounded-xl border p-4">
        <div className="flex items-center gap-3">
          <div className={`grid h-10 w-10 place-items-center rounded-xl ${healthy ? "bg-emerald-500/10" : "bg-destructive/10"}`}>
            {healthy ? <ShieldCheck className="h-5 w-5 text-emerald-500" /> : <XCircle className="h-5 w-5 text-destructive" />}
          </div>
          <div>
            <p className="font-medium">{label}</p>
            <p className="text-sm text-muted-foreground">{healthy ? "正常" : "异常"}</p>
          </div>
        </div>
        <Badge variant={healthy ? "success" : "destructive"}>{healthy ? "OK" : "FAIL"}</Badge>
      </div>
    );
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-3xl font-bold">服务器状态面板</h1>
          <p className="text-muted-foreground">展示系统健康、运行统计和公开信息。</p>
        </div>
        <Button onClick={loadStatus} disabled={loading}>
          {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
          刷新状态
        </Button>
      </div>

      {error && (
        <Card className="border border-destructive/30 bg-destructive/5">
          <CardContent>
            <p className="text-destructive font-medium">{error}</p>
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 xl:grid-cols-3">
        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Activity className="h-5 w-5" />
              健康检查
            </CardTitle>
            <CardDescription>检查后端服务、数据库和 Emby 连接是否正常。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {health ? (
              <div className="space-y-3">
                {renderStatusItem(health.api, "API 服务")}
                {renderStatusItem(health.database, "数据库")}
                {renderStatusItem(health.emby, "Emby 服务")}
              </div>
            ) : (
              <div className="flex h-40 items-center justify-center">
                <Loader2 className="h-8 w-8 animate-spin text-primary" />
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Server className="h-5 w-5" />
              系统与架构
            </CardTitle>
            <CardDescription>展示当前服务版本、功能开关、计费策略与运行限制。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {info ? (
              <div className="space-y-4 text-sm text-muted-foreground">
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                    <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">服务名称</p>
                    <p className="mt-2 text-base font-semibold text-foreground">{info.name ?? "未知"}</p>
                  </div>
                  <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                    <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">版本</p>
                    <p className="mt-2 text-base font-semibold text-foreground">{info.version ?? "未知"}</p>
                  </div>
                  <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                    <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">图标</p>
                    <p className="mt-2 text-base font-semibold text-foreground">{info.icon || "无"}</p>
                  </div>
                  {info.score && (
                    <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                      <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">计费策略</p>
                      <div className="mt-2 space-y-2 text-sm text-muted-foreground">
                        <div className="flex items-center justify-between gap-2">
                          <span>名称</span>
                          <span>{info.score.name ?? "未知"}</span>
                        </div>
                        <div className="flex items-center justify-between gap-2">
                          <span>自动续期费用</span>
                          <span>{info.score.auto_renew_cost ?? "未知"}</span>
                        </div>
                        <div className="flex items-center justify-between gap-2">
                          <span>注册需积分</span>
                          <span>{info.score.register_need ?? "未知"}</span>
                        </div>
                      </div>
                    </div>
                  )}
                </div>

                <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                  <p className="text-sm font-medium text-foreground">启用功能</p>
                  <div className="mt-3 grid gap-2">
                    {Object.entries(info.features || {}).map(([name, enabled]) => (
                      <div key={name} className="flex items-center justify-between rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                        <span className="text-sm">{FEATURE_LABELS[name] ?? name}</span>
                        <Badge variant={enabled ? "success" : "outline"}>
                          {enabled ? "开启" : "关闭"}
                        </Badge>
                      </div>
                    ))}
                  </div>
                </div>

                {info.limits && (
                  <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                    <p className="text-sm font-medium text-foreground">系统限制</p>
                    <div className="mt-3 grid gap-2">
                      {Object.entries(info.limits).map(([name, value]) => (
                        <div key={name} className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                          <span className="text-sm">{LIMIT_LABELS[name] ?? name}</span>
                          <span>{value == null ? "无限" : String(value)}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            ) : (
              <div className="flex h-40 items-center justify-center">
                <Loader2 className="h-8 w-8 animate-spin text-primary" />
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="glass-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Database className="h-5 w-5" />
              系统统计
            </CardTitle>
            <CardDescription>展示 Emby、注册码与用户总览数据。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {stats ? (
              <div className="space-y-4 text-sm text-muted-foreground">
                <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                  <p className="text-sm font-medium text-foreground">Emby 服务器</p>
                  <div className="mt-3 grid gap-2">
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>服务器名称</span>
                      <span>{stats.emby?.server_name ?? "未知"}</span>
                    </div>
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>版本</span>
                      <span>{stats.emby?.version ?? "未知"}</span>
                    </div>
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>在线状态</span>
                      <Badge variant={stats.emby?.online ? "success" : "destructive"}>
                        {stats.emby?.online ? "在线" : "离线"}
                      </Badge>
                    </div>
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>活跃会话</span>
                      <span>{stats.emby?.active_sessions ?? 0}</span>
                    </div>
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>总会话</span>
                      <span>{stats.emby?.total_sessions ?? 0}</span>
                    </div>
                    <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                      <span>系统平台</span>
                      <span>{stats.emby?.operating_system ?? "未知"}</span>
                    </div>
                  </div>
                </div>

                <div className="grid gap-4 lg:grid-cols-2">
                  <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                    <p className="text-sm font-medium text-foreground">注册码统计</p>
                    <div className="mt-3 space-y-2 text-sm text-muted-foreground">
                      <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                        <span>激活</span>
                        <span>{stats.regcodes?.active ?? 0}</span>
                      </div>
                      <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                        <span>总计</span>
                        <span>{stats.regcodes?.total ?? 0}</span>
                      </div>
                    </div>
                  </div>
                  <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                    <p className="text-sm font-medium text-foreground">用户统计</p>
                    <div className="mt-3 space-y-2 text-sm text-muted-foreground">
                      <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                        <span>活跃</span>
                        <span>{stats.users?.active ?? 0}</span>
                      </div>
                      <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                        <span>总用户</span>
                        <span>{stats.users?.total ?? 0}</span>
                      </div>
                      <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                        <span>上限</span>
                        <span>{stats.users?.limit == null ? "无限" : stats.users.limit}</span>
                      </div>
                      <div className="flex items-center justify-between gap-2 rounded-lg border border-muted/20 bg-muted/20 px-3 py-2">
                        <span>使用率</span>
                        <span>{stats.users?.usage_percent != null ? `${stats.users.usage_percent}%` : "未知"}</span>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            ) : (
              <div className="flex h-40 items-center justify-center">
                <Loader2 className="h-8 w-8 animate-spin text-primary" />
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <Card className="glass-card">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Database className="h-5 w-5" />
            详细调试数据
          </CardTitle>
          <CardDescription>以可视化方式展示系统健康与原始调试 JSON。</CardDescription>
        </CardHeader>
        <CardContent>
          {health && info && stats ? (
            <div className="space-y-4">
              <div className="rounded-xl border border-muted/70 bg-background/80 p-4">
                <p className="text-sm font-medium text-foreground">健康状态</p>
                <div className="mt-3 space-y-2">
                  {([['API 服务', health.api], ['数据库', health.database], ['Emby 服务', health.emby]] as const).map(([label, ok]) => (
                    <div key={label} className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2 bg-muted/40">
                      <span className="text-sm text-muted-foreground">{label}</span>
                      <Badge variant={ok ? 'success' : 'destructive'}>{ok ? '正常' : '异常'}</Badge>
                    </div>
                  ))}
                </div>
              </div>

              <details className="rounded-xl border border-muted/70 bg-muted/40 p-4">
                <summary className="cursor-pointer text-sm font-medium text-foreground">查看原始 JSON</summary>
                <pre className="mt-3 max-h-[320px] overflow-auto whitespace-pre-wrap text-xs text-muted-foreground">
                  {JSON.stringify({ health, info, stats }, null, 2)}
                </pre>
              </details>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">正在加载详细信息...</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

