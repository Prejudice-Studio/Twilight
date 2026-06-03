"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { motion } from "framer-motion";
import {
  AlertTriangle,
  ArrowUpRight,
  CalendarClock,
  Copy,
  Crown,
  GitBranch,
  KeyRound,
  Loader2,
  Plus,
  RefreshCw,
  ShieldCheck,
  Trash2,
  Users,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/hooks/use-toast";
import { api, type InviteCodeItem, type InviteConfig, type InviteMyStatus, type InviteTreeNode } from "@/lib/api";

function formatExpires(unix?: number | null): string {
  if (!unix || unix <= 0) return "永不过期";
  return new Date(unix * 1000).toLocaleString("zh-CN");
}

function InviteTreeNodeList({ nodes }: { nodes: InviteTreeNode[] }) {
  if (!nodes.length) return null;
  return (
    <div className="space-y-2">
      {nodes.map((node) => (
        <div key={node.uid} className="space-y-2">
          <div className="rounded-lg border bg-card/70 px-3 py-2">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{node.username}</p>
                <p className="text-[11px] text-muted-foreground">
                  UID #{node.uid} · 第 {node.depth} 层
                </p>
              </div>
              <div className="flex flex-wrap justify-end gap-1">
                <Badge variant={node.active ? "success" : "secondary"} className="text-[10px]">
                  {node.active ? "启用" : "禁用"}
                </Badge>
                <Badge variant={node.has_emby ? "outline" : "secondary"} className="text-[10px]">
                  {node.has_emby ? "Emby" : "无 Emby"}
                </Badge>
                {node.emby_expired && (
                  <Badge variant="destructive" className="text-[10px]">
                    已到期
                  </Badge>
                )}
              </div>
            </div>
            <p className="mt-1 text-[11px] text-muted-foreground">{node.expire_status || "-"}</p>
          </div>
          {node.children?.length ? (
            <div className="ml-4 border-l pl-3">
              <InviteTreeNodeList nodes={node.children} />
            </div>
          ) : null}
        </div>
      ))}
    </div>
  );
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
  const [days, setDays] = useState("30");
  const [note, setNote] = useState("");
  const [targetUsername, setTargetUsername] = useState("");
  const [renewOpen, setRenewOpen] = useState(false);
  const [renewTarget, setRenewTarget] = useState<InviteMyStatus["children"][number] | null>(null);
  const [renewDays, setRenewDays] = useState("30");
  const [renewNote, setRenewNote] = useState("");
  const [renewing, setRenewing] = useState(false);
  const [generatedRenewCode, setGeneratedRenewCode] = useState<null | { code: string; target_username: string; days: number; validity_hours: number }>(null);
  const [detachingChildUid, setDetachingChildUid] = useState<number | null>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const cfg = await api.getInviteConfig();
      if (cfg.success && cfg.data) setConfig(cfg.data);
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

  const maxCodeDays = status?.max_code_days ?? 0;
  const inviteTree = status?.tree && !Array.isArray(status.tree) && status.tree.self ? status.tree : null;
  const activeCount = useMemo(
    () => codes.filter((code) => code.active && code.use_count < (code.use_count_limit === -1 ? Infinity : code.use_count_limit)).length,
    [codes],
  );

  const handleCreate = async () => {
    setCreating(true);
    try {
      const parsedDays = Number(days);
      if (!config?.enabled) {
        toast({ title: "邀请系统已关闭", description: "当前不能生成新的邀请码，但仍可为已有直属下级生成专属续期码。", variant: "destructive" });
        return;
      }
      if (maxCodeDays <= 0) {
        toast({ title: "当前不能生成邀请码", description: status?.max_code_days_reason, variant: "destructive" });
        return;
      }
      if (Number.isNaN(parsedDays) || parsedDays <= 0) {
        toast({ title: "请输入有效天数", description: "邀请树邀请码不能设置为永久", variant: "destructive" });
        return;
      }
      if (parsedDays > maxCodeDays) {
        toast({ title: "天数超出上限", description: `最多可生成 ${maxCodeDays} 天`, variant: "destructive" });
        return;
      }
      const res = await api.createInviteCode({
        days: parsedDays,
        note: note.trim() || undefined,
        target_username: targetUsername.trim() || undefined,
      });
      if (res.success) {
        toast({ title: "邀请码已生成", variant: "success" });
        setCreateOpen(false);
        setNote("");
        setTargetUsername("");
        await reload();
      } else {
        toast({ title: "生成失败", description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({ title: "生成失败", description: err instanceof Error ? err.message : "网络异常", variant: "destructive" });
    } finally {
      setCreating(false);
    }
  };

  const canCreateInviteCode = Boolean(config?.enabled && status?.can_invite);

  const openRenewDialog = (child: InviteMyStatus["children"][number]) => {
    const maxDays = status?.max_code_days ?? 30;
    setRenewTarget(child);
    setRenewDays(String(Math.min(30, Math.max(1, maxDays))));
    setRenewNote("");
    setGeneratedRenewCode(null);
    setRenewOpen(true);
  };

  const handleCreateRenewCode = async () => {
    if (!renewTarget) return;
    const parsedDays = Number(renewDays);
    if (Number.isNaN(parsedDays) || parsedDays <= 0) {
      toast({ title: "请输入有效续期天数", variant: "destructive" });
      return;
    }
    if (maxCodeDays <= 0 || parsedDays > maxCodeDays) {
      toast({ title: "续期天数超出上限", description: `最多可生成 ${maxCodeDays} 天`, variant: "destructive" });
      return;
    }
    setRenewing(true);
    try {
      const res = await api.createInviteRenewCode({
        target_uid: renewTarget.uid,
        days: parsedDays,
        note: renewNote.trim() || undefined,
      });
      if (res.success && res.data) {
        setGeneratedRenewCode({
          code: res.data.code,
          target_username: res.data.target_username,
          days: res.data.days,
          validity_hours: res.data.validity_hours,
        });
        toast({ title: "专属续期码已生成", variant: "success" });
      } else {
        toast({ title: "生成失败", description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({ title: "生成失败", description: err instanceof Error ? err.message : "网络异常", variant: "destructive" });
    } finally {
      setRenewing(false);
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
      description: used ? "邀请码已被使用，仅会停用，无法物理删除。" : "未使用的邀请码将被永久删除，无法恢复。",
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

  const handleDetachExpiredChild = async (child: InviteMyStatus["children"][number]) => {
    const reason = !child.active ? "该下级 Web 账号已被禁用，且仍绑定 Emby。" : "该下级 Emby 权益已到期。";
    const ok = await confirm({
      title: "删除 Emby 并断开关系？",
      description: `${reason}\n\n操作会删除他的远端 Emby 账号，清空本地 Emby 绑定，并断开邀请关系。Web 账号当前启用/禁用状态会保持不变。`,
      tone: "danger",
      confirmLabel: "删除 Emby 并断开",
    });
    if (!ok) return;
    setDetachingChildUid(child.uid);
    const res = await api.detachExpiredInviteChild(child.uid).catch((err) => ({
      success: false,
      message: err instanceof Error ? err.message : "请求异常",
    }));
    setDetachingChildUid(null);
    if (res.success) {
      toast({ title: "已断开下级关系", variant: "success" });
      await reload();
    } else {
      toast({ title: "操作失败", description: res.message, variant: "destructive" });
    }
  };

  if (loading && !config) {
    return (
      <div className="flex h-60 items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (config && !config.enabled && !status) {
    return (
      <Card className="border-dashed">
        <CardContent className="space-y-2 p-10 text-center">
          <GitBranch className="mx-auto h-10 w-10 text-muted-foreground" />
          <p className="font-medium">邀请系统未开启</p>
          <p className="text-xs text-muted-foreground">不能生成新的邀请码；如果你已有直属下级，可刷新后继续为下级生成专属续期码。</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold">
            <GitBranch className="h-5 w-5" />
            邀请中心
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">生成邀请码邀请新用户注册 Emby；下级注册成功后会与你建立树状关系。</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => void reload()} disabled={loading}>
            <RefreshCw className={`mr-1 h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
            刷新
          </Button>
          <Button
            size="sm"
            onClick={() => {
              const defaultDays = config?.default_days && config.default_days > 0 ? config.default_days : maxCodeDays || 30;
              setDays(String(Math.min(defaultDays, maxCodeDays || defaultDays)));
              setTargetUsername("");
              setCreateOpen(true);
            }}
            disabled={!canCreateInviteCode}
          >
            <Plus className="mr-1 h-4 w-4" />
            生成邀请码
          </Button>
        </div>
      </div>

      {error && (
        <Card className="border-destructive/40">
          <CardContent className="flex items-start gap-2 p-4 text-sm">
            <AlertTriangle className="mt-0.5 h-4 w-4 text-destructive" />
            <span>{error}</span>
          </CardContent>
        </Card>
      )}

      {config && !config.enabled && (
        <Card className="border-amber-500/30 bg-amber-500/5">
          <CardContent className="flex items-start gap-2 p-4 text-sm text-amber-700 dark:text-amber-300">
            <AlertTriangle className="mt-0.5 h-4 w-4" />
            <div className="space-y-1">
              <p className="font-medium">邀请系统已关闭，不能生成新的邀请码。</p>
              <p className="text-xs opacity-90">已有直属下级仍可生成专属续期码；Emby 已到期或 Web 已禁用且仍绑定 Emby 的下级，可删除 Emby 并断开关系。</p>
            </div>
          </CardContent>
        </Card>
      )}

      {status && (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Card>
            <CardContent className="space-y-1 p-4">
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
            <CardContent className="space-y-1 p-4">
              <p className="text-[11px] uppercase tracking-widest text-muted-foreground">下级数</p>
              <p className="text-2xl font-bold">{status.children.length}</p>
              <p className="text-xs text-muted-foreground">直接邀请的人数</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="space-y-1 p-4">
              <p className="text-[11px] uppercase tracking-widest text-muted-foreground">未使用邀请码</p>
              <p className="text-2xl font-bold">{activeCount}</p>
              <p className="text-xs text-muted-foreground">上限 {config?.invite_limit === -1 ? "无限" : config?.invite_limit ?? "-"}</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="space-y-1 p-4">
              <p className="text-[11px] uppercase tracking-widest text-muted-foreground">能否邀请</p>
              <p className="flex items-center gap-1 text-2xl font-bold">
                {canCreateInviteCode ? (
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
                {canCreateInviteCode ? `最多可授权 ${status.max_code_days ?? "-"} 天` : status.invite_block_reason || status.max_code_days_reason || "条件不满足"}
              </p>
            </CardContent>
          </Card>
        </div>
      )}

      {inviteTree && (
        <Card>
          <CardContent className="space-y-4 p-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <h3 className="flex items-center gap-2 text-sm font-semibold">
                <GitBranch className="h-4 w-4 text-primary" />
                我的上下级树
              </h3>
              <Badge variant="outline" className="text-[10px]">
                下级总数 {inviteTree.descendant_count}
              </Badge>
            </div>

            <div className="rounded-lg border bg-muted/30 p-3">
              <p className="mb-2 text-[11px] uppercase tracking-widest text-muted-foreground">直属上级</p>
              <div className="flex flex-wrap items-center gap-2 text-sm">
                {status?.parent ? (
                  <Badge variant="secondary" className="max-w-[220px] truncate">
                    {status.parent.username} · UID #{status.parent.uid}
                  </Badge>
                ) : (
                  <span className="text-muted-foreground">你当前没有上级，是邀请树根节点。</span>
                )}
                <Badge variant="default" className="max-w-[180px] truncate">
                  我 · 第 {inviteTree.self.depth} 层
                </Badge>
              </div>
            </div>

            <div className="space-y-2">
              <p className="text-[11px] uppercase tracking-widest text-muted-foreground">下级树</p>
              {inviteTree.descendants.length ? (
                <InviteTreeNodeList nodes={inviteTree.descendants} />
              ) : (
                <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
                  暂无下级。{config?.enabled ? "生成邀请码并被使用后，这里会展示完整下级树。" : "邀请系统已关闭，暂不能新增下级。"}
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      {status && status.children.length > 0 && (
        <Card>
          <CardContent className="space-y-3 p-4">
            <h3 className="flex items-center gap-2 text-sm font-semibold">
              <Users className="h-4 w-4 text-primary" />
              我邀请的人
            </h3>
            <div className="grid gap-2 sm:grid-cols-2">
              {status.children.map((child) => (
                <div key={child.uid} className="rounded-lg border bg-card/60 px-3 py-2">
                  <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{child.username}</p>
                      <p className="text-[11px] text-muted-foreground">UID #{child.uid}</p>
                    </div>
                    <div className="flex shrink-0 flex-wrap justify-end gap-1">
                      <Badge variant={child.active ? "success" : "secondary"} className="text-[10px]">
                        {child.active ? "启用" : "禁用"}
                      </Badge>
                      <Badge variant={child.has_emby ? "outline" : "secondary"} className="text-[10px]">
                        {child.has_emby ? "Emby" : "无 Emby"}
                      </Badge>
                      {child.emby_expired && (
                        <Badge variant="destructive" className="text-[10px]">
                          已到期
                        </Badge>
                      )}
                    </div>
                  </div>
                  <div className="mt-2 flex flex-wrap items-center justify-between gap-2 border-t pt-2 text-[11px] text-muted-foreground">
                    <span>{child.expire_status || "-"}</span>
                    {child.can_generate_renew_code && (
                      <Button size="sm" variant="outline" className="h-7 px-2 text-[11px]" onClick={() => openRenewDialog(child)}>
                        <KeyRound className="mr-1 h-3 w-3" />
                        生成专属续期码
                      </Button>
                    )}
                    {child.can_delete_emby_and_detach && (
                      <Button
                        size="sm"
                        variant="destructive"
                        className="h-7 px-2 text-[11px]"
                        onClick={() => void handleDetachExpiredChild(child)}
                        disabled={detachingChildUid === child.uid}
                      >
                        {detachingChildUid === child.uid ? <Loader2 className="mr-1 h-3 w-3 animate-spin" /> : <Trash2 className="mr-1 h-3 w-3" />}
                        删除 Emby 并断开
                      </Button>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent className="overflow-hidden p-0">
          <div className="flex items-center justify-between border-b px-4 py-3">
            <h3 className="flex items-center gap-2 text-sm font-semibold">
              <Crown className="h-4 w-4 text-primary" />
              我的邀请码
            </h3>
            <Badge variant="secondary" className="text-[10px]">
              {codes.length}
            </Badge>
          </div>
          {codes.length === 0 ? (
            <div className="p-8 text-center text-sm text-muted-foreground">暂无邀请码，点击右上方按钮生成第一张。</div>
          ) : (
            <div className="divide-y">
              {codes.map((code) => {
                const usedUp = code.use_count_limit !== -1 && code.use_count >= code.use_count_limit;
                return (
                  <div key={code.code} className="flex flex-wrap items-start gap-3 px-4 py-3">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <code className="break-all rounded bg-muted px-2 py-1 font-mono text-xs">{code.code}</code>
                        {code.active && !usedUp ? (
                          <Badge variant="success" className="text-[10px]">
                            可用
                          </Badge>
                        ) : usedUp ? (
                          <Badge variant="secondary" className="text-[10px]">
                            已使用
                          </Badge>
                        ) : (
                          <Badge variant="secondary" className="text-[10px]">
                            已停用
                          </Badge>
                        )}
                        <Badge variant="outline" className="text-[10px]">
                          {code.days <= 0 ? "按上限" : `${code.days} 天`}
                        </Badge>
                      </div>
                      <p className="mt-1 text-[11px] text-muted-foreground">
                        {new Date(code.created_at * 1000).toLocaleString("zh-CN")} · 截止 {formatExpires(code.expires_at)}
                        {code.target_username ? ` · 指定 ${code.target_username}` : ""}
                        {code.used_by_username ? ` · 使用者 ${code.used_by_username}` : code.used_by_uid ? ` · 使用者 UID ${code.used_by_uid}` : ""}
                        {code.note ? ` · ${code.note}` : ""}
                      </p>
                    </div>
                    <div className="flex shrink-0 gap-1">
                      <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => handleCopy(code.code)} title="复制邀请码">
                        <Copy className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-destructive hover:text-destructive"
                        onClick={() => handleRevoke(code)}
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

      <Dialog open={createOpen} onOpenChange={(open) => {
        setCreateOpen(open);
        if (!open) setTargetUsername("");
      }}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>生成邀请码</DialogTitle>
            <DialogDescription>将邀请码分享给好友。对方注册并登录后，在仪表盘的卡码使用区填写即可。</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label>Emby 默认开通天数</Label>
              <Input value={days} onChange={(event) => setDays(event.target.value)} inputMode="numeric" placeholder="例如 30" />
              <p className="text-[10px] text-muted-foreground">
                被邀请人通过该码创建 Emby 账号后的有效期；不能超过你的可授权上限 {maxCodeDays || "-"} 天，不能设置为永久。
              </p>
            </div>
            <div className="space-y-1.5">
              <Label>备注（可选）</Label>
              <Textarea rows={2} maxLength={255} value={note} onChange={(event) => setNote(event.target.value)} placeholder="送给谁、什么场景使用..." />
            </div>
            <div className="space-y-1.5">
              <Label>指定使用用户名（可选）</Label>
              <Input value={targetUsername} onChange={(event) => setTargetUsername(event.target.value)} placeholder="留空则任何符合条件的用户都可使用" />
              <p className="text-[10px] text-muted-foreground">
                填写后仅该 Web 用户名可使用此邀请码，比较时不区分大小写。适合把邀请码提前分配给指定成员。
              </p>
            </div>
            {config?.code_format && (
              <div className="rounded-lg border bg-muted/30 px-3 py-2 text-[10px] text-muted-foreground">
                当前邀请码格式：<code>{config.code_format}</code>
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              取消
            </Button>
            <Button onClick={handleCreate} disabled={creating}>
              {creating ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <ArrowUpRight className="mr-2 h-4 w-4" />}
              生成
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={renewOpen} onOpenChange={setRenewOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>生成专属续期码</DialogTitle>
            <DialogDescription>续期码只允许 {renewTarget?.username || "该下级"} 使用。其他账号使用会被拒绝。</DialogDescription>
          </DialogHeader>
          {generatedRenewCode ? (
            <div className="space-y-4">
              <div className="rounded-lg border bg-muted/40 p-3">
                <p className="text-xs text-muted-foreground">专属续期码</p>
                <code className="mt-1 block break-all rounded bg-background px-2 py-2 font-mono text-sm">{generatedRenewCode.code}</code>
                <p className="mt-2 text-xs text-muted-foreground">
                  目标：{generatedRenewCode.target_username} · 续期 {generatedRenewCode.days} 天 · 卡码有效 {generatedRenewCode.validity_hours} 小时
                </p>
              </div>
              <DialogFooter>
                <Button variant="outline" onClick={() => void handleCopy(generatedRenewCode.code)}>
                  <Copy className="mr-2 h-4 w-4" />
                  复制
                </Button>
                <Button onClick={() => setRenewOpen(false)}>完成</Button>
              </DialogFooter>
            </div>
          ) : (
            <div className="space-y-3">
              <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-700 dark:text-amber-300">
                <CalendarClock className="mr-1 inline h-3.5 w-3.5" />
                该码绑定目标 UID #{renewTarget?.uid}，不要发给其他人。
              </div>
              <div className="space-y-1.5">
                <Label>续期天数</Label>
                <Input value={renewDays} onChange={(event) => setRenewDays(event.target.value)} inputMode="numeric" />
                <p className="text-[10px] text-muted-foreground">最多 {maxCodeDays || "-"} 天，由你的剩余有效期决定。</p>
              </div>
              <div className="space-y-1.5">
                <Label>备注（可选）</Label>
                <Textarea rows={2} maxLength={120} value={renewNote} onChange={(event) => setRenewNote(event.target.value)} />
              </div>
              <DialogFooter>
                <Button variant="outline" onClick={() => setRenewOpen(false)}>
                  取消
                </Button>
                <Button onClick={handleCreateRenewCode} disabled={renewing}>
                  {renewing ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <KeyRound className="mr-2 h-4 w-4" />}
                  生成续期码
                </Button>
              </DialogFooter>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}
