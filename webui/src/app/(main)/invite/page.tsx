"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { motion } from "framer-motion";
import {
  GitBranch,
  Loader2,
  Plus,
  RefreshCw,
  Copy,
  Trash2,
  Users,
  ArrowUpRight,
  Crown,
  AlertTriangle,
  ShieldCheck,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { api, type InviteCodeItem, type InviteMyStatus, type InviteConfig } from "@/lib/api";

function formatExpires(unix: number): string {
  if (!unix || unix <= 0) return "永不过期";
  return new Date(unix * 1000).toLocaleString("zh-CN");
}

export default function InviteCenterPage() {
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const [config, setConfig] = useState<InviteConfig | null>(null);
  const [status, setStatus] = useState<InviteMyStatus | null>(null);
  const [codes, setCodes] = useState<InviteCodeItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  const [createOpen, setCreateOpen] = useState(false);
  const [days, setDays] = useState<string>("30");
  const [note, setNote] = useState("");

  const reload = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const cfg = await api.getInviteConfig();
      if (cfg.success && cfg.data) setConfig(cfg.data);
      if (!cfg.data?.enabled) {
        setStatus(null);
        setCodes([]);
        return;
      }
      const [me, list] = await Promise.all([
        api.getMyInviteStatus().catch(() => null),
        api.getMyInviteCodes().catch(() => null),
      ]);
      if (me?.success && me.data) setStatus(me.data);
      if (list?.success && list.data) setCodes(list.data.codes || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  const handleCreate = async () => {
    setCreating(true);
    try {
      const parsedDays = Number(days);
      const payload: { days?: number; note?: string } = {};
      if (!Number.isNaN(parsedDays)) {
        payload.days = parsedDays;
      }
      if (note.trim()) payload.note = note.trim();
      const res = await api.createInviteCode(payload);
      if (res.success) {
        toast({ title: "邀请码已生成", variant: "success" });
        setCreateOpen(false);
        setNote("");
        await reload();
      } else {
        toast({ title: "生成失败", description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({
        title: "生成失败",
        description: err instanceof Error ? err.message : "网络异常",
        variant: "destructive",
      });
    } finally {
      setCreating(false);
    }
  };

  const handleCopy = async (code: string) => {
    try {
      await navigator.clipboard.writeText(code);
      toast({ title: "已复制到剪贴板" });
    } catch {
      toast({ title: "复制失败，请手动选中复制", variant: "destructive" });
    }
  };

  const handleRevoke = async (code: InviteCodeItem) => {
    const used = code.use_count > 0;
    const ok = await confirm({
      title: used ? "停用该邀请码？" : "删除该邀请码？",
      description: used
        ? "邀请码已被使用，仅会停用，无法物理删除。"
        : "未使用的邀请码将被永久删除，无法恢复。",
      tone: "danger",
      confirmLabel: used ? "停用" : "删除",
    });
    if (!ok) return;
    const res = await api.revokeInviteCode(code.code).catch((err) => ({
      success: false,
      message: err instanceof Error ? err.message : "请求异常",
    }));
    if (res.success) {
      toast({ title: used ? "已停用" : "已删除" });
      await reload();
    } else {
      toast({ title: "操作失败", description: res.message, variant: "destructive" });
    }
  };

  const activeCount = useMemo(
    () => codes.filter((c) => c.active && c.use_count < (c.use_count_limit === -1 ? Infinity : c.use_count_limit)).length,
    [codes],
  );

  if (loading && !config) {
    return (
      <div className="flex h-60 items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (config && !config.enabled) {
    return (
      <Card className="border-dashed">
        <CardContent className="p-10 text-center space-y-2">
          <GitBranch className="h-10 w-10 mx-auto text-muted-foreground" />
          <p className="font-medium">邀请系统未开启</p>
          <p className="text-xs text-muted-foreground">
            如需启用，请联系管理员在「配置 → 注册与用户策略」中打开「启用邀请树」。
          </p>
        </CardContent>
      </Card>
    );
  }

  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      className="space-y-6"
    >
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <GitBranch className="h-5 w-5" />
            邀请中心
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            生成邀请码邀请新用户注册 Emby；下级注册成功后将与你建立树状关系。
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => void reload()} disabled={loading}>
            <RefreshCw className={`mr-1 h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
            刷新
          </Button>
          <Button
            size="sm"
            onClick={() => setCreateOpen(true)}
            disabled={!status?.can_invite}
          >
            <Plus className="mr-1 h-4 w-4" />
            生成邀请码
          </Button>
        </div>
      </div>

      {error && (
        <Card className="border-destructive/40">
          <CardContent className="p-4 text-sm flex items-start gap-2">
            <AlertTriangle className="h-4 w-4 text-destructive mt-0.5" />
            <span>{error}</span>
          </CardContent>
        </Card>
      )}

      {/* 状态卡 */}
      {status && (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Card>
            <CardContent className="p-4 space-y-1">
              <p className="text-[11px] uppercase tracking-widest text-muted-foreground">当前层级</p>
              <p className="text-2xl font-bold">
                {status.depth} / {status.max_depth}
              </p>
              <p className="text-xs text-muted-foreground">
                {status.is_root ? "你是树根" : status.parent ? `上级：${status.parent.username}` : "未绑定"}
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4 space-y-1">
              <p className="text-[11px] uppercase tracking-widest text-muted-foreground">下级数</p>
              <p className="text-2xl font-bold">{status.children.length}</p>
              <p className="text-xs text-muted-foreground">直接邀请的人数</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4 space-y-1">
              <p className="text-[11px] uppercase tracking-widest text-muted-foreground">未使用邀请码</p>
              <p className="text-2xl font-bold">{activeCount}</p>
              <p className="text-xs text-muted-foreground">
                上限 {config?.invite_limit === -1 ? "无限" : config?.invite_limit ?? "-"}
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4 space-y-1">
              <p className="text-[11px] uppercase tracking-widest text-muted-foreground">能否邀请</p>
              <p className="text-2xl font-bold flex items-center gap-1">
                {status.can_invite ? (
                  <>
                    <ShieldCheck className="h-5 w-5 text-emerald-500" />
                    <span>可邀请</span>
                  </>
                ) : (
                  <>
                    <AlertTriangle className="h-5 w-5 text-amber-500" />
                    <span>暂不可</span>
                  </>
                )}
              </p>
              <p className="text-xs text-muted-foreground">
                {status.can_invite ? "条件已满足" : status.invite_block_reason || "条件不满足"}
              </p>
            </CardContent>
          </Card>
        </div>
      )}

      {/* 下级名单 */}
      {status && status.children.length > 0 && (
        <Card>
          <CardContent className="p-4 space-y-3">
            <h3 className="text-sm font-semibold flex items-center gap-2">
              <Users className="h-4 w-4 text-primary" />
              我邀请的人
            </h3>
            <div className="grid gap-2 sm:grid-cols-2">
              {status.children.map((c) => (
                <div
                  key={c.uid}
                  className="flex items-center justify-between gap-3 rounded-lg border bg-card/60 px-3 py-2"
                >
                  <div className="min-w-0">
                    <p className="text-sm font-medium truncate">{c.username}</p>
                    <p className="text-[11px] text-muted-foreground">UID #{c.uid}</p>
                  </div>
                  <div className="flex shrink-0 gap-1">
                    <Badge variant={c.active ? "success" : "secondary"} className="text-[10px]">
                      {c.active ? "启用" : "禁用"}
                    </Badge>
                    <Badge variant={c.has_emby ? "outline" : "secondary"} className="text-[10px]">
                      {c.has_emby ? "Emby ✓" : "无 Emby"}
                    </Badge>
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* 邀请码列表 */}
      <Card>
        <CardContent className="p-0 overflow-hidden">
          <div className="flex items-center justify-between px-4 py-3 border-b">
            <h3 className="text-sm font-semibold flex items-center gap-2">
              <Crown className="h-4 w-4 text-primary" />
              我的邀请码
            </h3>
            <Badge variant="secondary" className="text-[10px]">{codes.length}</Badge>
          </div>
          {codes.length === 0 ? (
            <div className="p-8 text-center text-sm text-muted-foreground">
              暂无邀请码，点击右上方按钮生成第一张。
            </div>
          ) : (
            <div className="divide-y">
              {codes.map((c) => {
                const usedUp =
                  c.use_count_limit !== -1 && c.use_count >= c.use_count_limit;
                return (
                  <div key={c.code} className="px-4 py-3 flex items-start gap-3 flex-wrap">
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2 flex-wrap">
                        <code className="font-mono text-xs break-all bg-muted px-2 py-1 rounded">{c.code}</code>
                        {c.active && !usedUp ? (
                          <Badge variant="success" className="text-[10px]">可用</Badge>
                        ) : usedUp ? (
                          <Badge variant="secondary" className="text-[10px]">已使用</Badge>
                        ) : (
                          <Badge variant="secondary" className="text-[10px]">已停用</Badge>
                        )}
                        <Badge variant="outline" className="text-[10px]">
                          {c.days <= 0 ? "永久" : `${c.days} 天`}
                        </Badge>
                      </div>
                      <p className="text-[11px] text-muted-foreground mt-1">
                        {new Date(c.created_at * 1000).toLocaleString("zh-CN")} · 截止 {formatExpires(c.expires_at)}
                        {c.note ? ` · ${c.note}` : ""}
                      </p>
                    </div>
                    <div className="flex shrink-0 gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        onClick={() => handleCopy(c.code)}
                        title="复制邀请码"
                      >
                        <Copy className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-destructive hover:text-destructive"
                        onClick={() => handleRevoke(c)}
                        title={usedUp ? "停用" : "删除"}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>生成邀请码</DialogTitle>
            <DialogDescription>
              将邀请码分享给好友，让对方在「注册」/「邀请使用」页填写即可。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label>Emby 默认开通天数</Label>
              <Input
                value={days}
                onChange={(e) => setDays(e.target.value)}
                inputMode="numeric"
                placeholder="例如 30；填 0 或 -1 表示永久"
              />
              <p className="text-[10px] text-muted-foreground">
                被邀请人通过该码创建 Emby 账号后的有效期；管理员/白名单不受影响。
              </p>
            </div>
            <div className="space-y-1.5">
              <Label>备注（可选）</Label>
              <Textarea
                rows={2}
                maxLength={255}
                value={note}
                onChange={(e) => setNote(e.target.value)}
                placeholder="送给谁的、什么场景使用..."
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>取消</Button>
            <Button onClick={handleCreate} disabled={creating}>
              {creating ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <ArrowUpRight className="mr-2 h-4 w-4" />}
              生成
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}
