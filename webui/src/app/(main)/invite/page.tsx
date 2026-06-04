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
import { useI18n } from "@/lib/i18n";

function formatExpires(unix: number | null | undefined, locale: string, neverExpires: string): string {
  if (!unix || unix <= 0) return neverExpires;
  return new Date(unix * 1000).toLocaleString(locale);
}

function InviteTreeNodeList({ nodes }: { nodes: InviteTreeNode[] }) {
  const { t } = useI18n();
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
                  {t("invite.treeNodeMeta", { uid: node.uid, depth: node.depth })}
                </p>
              </div>
              <div className="flex flex-wrap justify-end gap-1">
                <Badge variant={node.active ? "success" : "secondary"} className="text-[10px]">
                  {node.active ? t("invite.active") : t("invite.inactive")}
                </Badge>
                <Badge variant={node.has_emby ? "outline" : "secondary"} className="text-[10px]">
                  {node.has_emby ? "Emby" : t("invite.noEmby")}
                </Badge>
                {node.emby_expired && (
                  <Badge variant="destructive" className="text-[10px]">
                    {t("invite.expired")}
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
  const { locale, t } = useI18n();
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
      setError(err instanceof Error ? err.message : t("invite.loadFailed"));
    } finally {
      setLoading(false);
    }
  }, [t]);

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
        toast({ title: t("invite.systemClosed"), description: t("invite.systemClosedCreateDescription"), variant: "destructive" });
        return;
      }
      if (maxCodeDays <= 0) {
        toast({ title: t("invite.cannotCreate"), description: status?.max_code_days_reason, variant: "destructive" });
        return;
      }
      if (Number.isNaN(parsedDays) || parsedDays <= 0) {
        toast({ title: t("invite.invalidDays"), description: t("invite.cannotBePermanent"), variant: "destructive" });
        return;
      }
      if (parsedDays > maxCodeDays) {
        toast({ title: t("invite.daysExceeded"), description: t("invite.maxDays", { days: maxCodeDays }), variant: "destructive" });
        return;
      }
      const res = await api.createInviteCode({
        days: parsedDays,
        note: note.trim() || undefined,
        target_username: targetUsername.trim() || undefined,
      });
      if (res.success) {
        toast({ title: t("invite.created"), variant: "success" });
        setCreateOpen(false);
        setNote("");
        setTargetUsername("");
        await reload();
      } else {
        toast({ title: t("invite.generateFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({ title: t("invite.generateFailed"), description: err instanceof Error ? err.message : t("common.networkError"), variant: "destructive" });
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
      toast({ title: t("invite.invalidRenewDays"), variant: "destructive" });
      return;
    }
    if (maxCodeDays <= 0 || parsedDays > maxCodeDays) {
      toast({ title: t("invite.renewDaysExceeded"), description: t("invite.maxDays", { days: maxCodeDays }), variant: "destructive" });
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
        toast({ title: t("invite.renewCreated"), variant: "success" });
      } else {
        toast({ title: t("invite.generateFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({ title: t("invite.generateFailed"), description: err instanceof Error ? err.message : t("common.networkError"), variant: "destructive" });
    } finally {
      setRenewing(false);
    }
  };

  const handleCopy = async (code: string) => {
    try {
      await navigator.clipboard.writeText(code);
      toast({ title: t("common.copiedToClipboard") });
    } catch {
      toast({ title: t("invite.copyManual"), variant: "destructive" });
    }
  };

  const handleRevoke = async (code: InviteCodeItem) => {
    const used = code.use_count > 0;
    const ok = await confirm({
      title: used ? t("invite.disableConfirmTitle") : t("invite.deleteConfirmTitle"),
      description: used ? t("invite.disableConfirmDescription") : t("invite.deleteConfirmDescription"),
      tone: "danger",
      confirmLabel: used ? t("invite.disable") : t("common.delete"),
    });
    if (!ok) return;
    const res = await api.revokeInviteCode(code.code).catch((err) => ({
      success: false,
      message: err instanceof Error ? err.message : t("invite.requestError"),
    }));
    if (res.success) {
      toast({ title: used ? t("invite.disabled") : t("invite.deleted") });
      await reload();
    } else {
      toast({ title: t("invite.operationFailed"), description: res.message, variant: "destructive" });
    }
  };

  const handleDetachExpiredChild = async (child: InviteMyStatus["children"][number]) => {
    const reason = !child.active ? t("invite.childWebDisabledReason") : t("invite.childEmbyExpiredReason");
    const ok = await confirm({
      title: t("invite.detachConfirmTitle"),
      description: t("invite.detachConfirmDescription", { reason }),
      tone: "danger",
      confirmLabel: t("invite.deleteEmbyDetach"),
    });
    if (!ok) return;
    setDetachingChildUid(child.uid);
    const res = await api.detachExpiredInviteChild(child.uid).catch((err) => ({
      success: false,
      message: err instanceof Error ? err.message : t("invite.requestError"),
    }));
    setDetachingChildUid(null);
    if (res.success) {
      toast({ title: t("invite.detached"), variant: "success" });
      await reload();
    } else {
      toast({ title: t("invite.operationFailed"), description: res.message, variant: "destructive" });
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
          <p className="font-medium">{t("invite.systemNotOpen")}</p>
          <p className="text-xs text-muted-foreground">{t("invite.systemNotOpenDescription")}</p>
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
            {t("invite.title")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">{t("invite.description")}</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => void reload()} disabled={loading}>
            <RefreshCw className={`mr-1 h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
            {t("common.refresh")}
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
            {t("invite.generateInvite")}
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
              <p className="font-medium">{t("invite.closedBanner")}</p>
              <p className="text-xs opacity-90">{t("invite.closedBannerDescription")}</p>
            </div>
          </CardContent>
        </Card>
      )}

      {status && (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Card>
            <CardContent className="space-y-1 p-4">
              <p className="text-[11px] uppercase tracking-widest text-muted-foreground">{t("invite.currentDepth")}</p>
              <p className="text-2xl font-bold">
                {status.depth} / {status.max_depth}
              </p>
              <p className="text-xs text-muted-foreground">
                {status.is_root ? t("invite.treeRoot") : status.parent ? t("invite.parent", { username: status.parent.username }) : t("invite.unbound")}
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="space-y-1 p-4">
              <p className="text-[11px] uppercase tracking-widest text-muted-foreground">{t("invite.childCount")}</p>
              <p className="text-2xl font-bold">{status.children.length}</p>
              <p className="text-xs text-muted-foreground">{t("invite.directInviteCount")}</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="space-y-1 p-4">
              <p className="text-[11px] uppercase tracking-widest text-muted-foreground">{t("invite.unusedCodes")}</p>
              <p className="text-2xl font-bold">{activeCount}</p>
              <p className="text-xs text-muted-foreground">{t("invite.limit", { limit: config?.invite_limit === -1 ? t("invite.unlimited") : config?.invite_limit ?? "-" })}</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="space-y-1 p-4">
              <p className="text-[11px] uppercase tracking-widest text-muted-foreground">{t("invite.canInviteTitle")}</p>
              <p className="flex items-center gap-1 text-2xl font-bold">
                {canCreateInviteCode ? (
                  <>
                    <ShieldCheck className="h-5 w-5 text-emerald-500" />
                    <span>{t("invite.canInvite")}</span>
                  </>
                ) : (
                  <>
                    <AlertTriangle className="h-5 w-5 text-amber-500" />
                    <span>{t("invite.cannotInvite")}</span>
                  </>
                )}
              </p>
              <p className="text-xs text-muted-foreground">
                {canCreateInviteCode ? t("invite.maxGrantDays", { days: status.max_code_days ?? "-" }) : status.invite_block_reason || status.max_code_days_reason || t("invite.conditionsNotMet")}
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
                {t("invite.myTree")}
              </h3>
              <Badge variant="outline" className="text-[10px]">
                {t("invite.descendantCount", { count: inviteTree.descendant_count })}
              </Badge>
            </div>

            <div className="rounded-lg border bg-muted/30 p-3">
              <p className="mb-2 text-[11px] uppercase tracking-widest text-muted-foreground">{t("invite.directParent")}</p>
              <div className="flex flex-wrap items-center gap-2 text-sm">
                {status?.parent ? (
                  <Badge variant="secondary" className="max-w-[220px] truncate">
                    {status.parent.username} · UID #{status.parent.uid}
                  </Badge>
                ) : (
                  <span className="text-muted-foreground">{t("invite.noParent")}</span>
                )}
                <Badge variant="default" className="max-w-[180px] truncate">
                  {t("invite.selfDepth", { depth: inviteTree.self.depth })}
                </Badge>
              </div>
            </div>

            <div className="space-y-2">
              <p className="text-[11px] uppercase tracking-widest text-muted-foreground">{t("invite.childTree")}</p>
              {inviteTree.descendants.length ? (
                <InviteTreeNodeList nodes={inviteTree.descendants} />
              ) : (
                <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
                  {config?.enabled ? t("invite.noChildrenWithInvite") : t("invite.noChildrenClosed")}
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
              {t("invite.myInvitees")}
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
                        {child.active ? t("invite.active") : t("invite.inactive")}
                      </Badge>
                      <Badge variant={child.has_emby ? "outline" : "secondary"} className="text-[10px]">
                        {child.has_emby ? "Emby" : t("invite.noEmby")}
                      </Badge>
                      {child.emby_expired && (
                        <Badge variant="destructive" className="text-[10px]">
                          {t("invite.expired")}
                        </Badge>
                      )}
                    </div>
                  </div>
                  <div className="mt-2 flex flex-wrap items-center justify-between gap-2 border-t pt-2 text-[11px] text-muted-foreground">
                    <span>{child.expire_status || "-"}</span>
                    {child.can_generate_renew_code && (
                      <Button size="sm" variant="outline" className="h-7 px-2 text-[11px]" onClick={() => openRenewDialog(child)}>
                        <KeyRound className="mr-1 h-3 w-3" />
                        {t("invite.generateRenewCode")}
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
                        {t("invite.deleteEmbyDetach")}
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
              {t("invite.myCodes")}
            </h3>
            <Badge variant="secondary" className="text-[10px]">
              {codes.length}
            </Badge>
          </div>
          {codes.length === 0 ? (
            <div className="p-8 text-center text-sm text-muted-foreground">{t("invite.emptyCodes")}</div>
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
                            {t("invite.available")}
                          </Badge>
                        ) : usedUp ? (
                          <Badge variant="secondary" className="text-[10px]">
                            {t("invite.used")}
                          </Badge>
                        ) : (
                          <Badge variant="secondary" className="text-[10px]">
                            {t("invite.disabled")}
                          </Badge>
                        )}
                        <Badge variant="outline" className="text-[10px]">
                          {code.days <= 0 ? t("invite.byLimit") : t("score.days", { days: code.days })}
                        </Badge>
                      </div>
                      <p className="mt-1 text-[11px] text-muted-foreground">
                        {t("invite.createdExpires", { created: new Date(code.created_at * 1000).toLocaleString(locale), expires: formatExpires(code.expires_at, locale, t("invite.neverExpires")) })}
                        {code.target_username ? t("invite.specifiedUser", { username: code.target_username }) : ""}
                        {code.used_by_username ? t("invite.usedBy", { username: code.used_by_username }) : code.used_by_uid ? t("invite.usedByUid", { uid: code.used_by_uid }) : ""}
                        {code.note ? ` · ${code.note}` : ""}
                      </p>
                    </div>
                    <div className="flex shrink-0 gap-1">
                      <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => handleCopy(code.code)} title={t("invite.copyInvite")}>
                        <Copy className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-destructive hover:text-destructive"
                        onClick={() => handleRevoke(code)}
                        title={usedUp ? t("invite.disable") : t("common.delete")}
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
            <DialogTitle>{t("invite.createDialogTitle")}</DialogTitle>
            <DialogDescription>{t("invite.createDialogDescription")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label>{t("invite.defaultEmbyDays")}</Label>
              <Input value={days} onChange={(event) => setDays(event.target.value)} inputMode="numeric" placeholder={t("invite.daysPlaceholder")} />
              <p className="text-[10px] text-muted-foreground">
                {t("invite.daysHelp", { days: maxCodeDays || "-" })}
              </p>
            </div>
            <div className="space-y-1.5">
              <Label>{t("invite.noteOptional")}</Label>
              <Textarea rows={2} maxLength={255} value={note} onChange={(event) => setNote(event.target.value)} placeholder={t("invite.notePlaceholder")} />
            </div>
            <div className="space-y-1.5">
              <Label>{t("invite.targetUsernameOptional")}</Label>
              <Input value={targetUsername} onChange={(event) => setTargetUsername(event.target.value)} placeholder={t("invite.targetUsernamePlaceholder")} />
              <p className="text-[10px] text-muted-foreground">
                {t("invite.targetUsernameHelp")}
              </p>
            </div>
            {config?.code_format && (
              <div className="rounded-lg border bg-muted/30 px-3 py-2 text-[10px] text-muted-foreground">
                {t("invite.codeFormat", { format: config.code_format })}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button onClick={handleCreate} disabled={creating}>
              {creating ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <ArrowUpRight className="mr-2 h-4 w-4" />}
              {t("invite.generate")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={renewOpen} onOpenChange={setRenewOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("invite.renewDialogTitle")}</DialogTitle>
            <DialogDescription>{t("invite.renewDialogDescription", { username: renewTarget?.username || t("invite.thisChild") })}</DialogDescription>
          </DialogHeader>
          {generatedRenewCode ? (
            <div className="space-y-4">
              <div className="rounded-lg border bg-muted/40 p-3">
                <p className="text-xs text-muted-foreground">{t("invite.dedicatedRenewCode")}</p>
                <code className="mt-1 block break-all rounded bg-background px-2 py-2 font-mono text-sm">{generatedRenewCode.code}</code>
                <p className="mt-2 text-xs text-muted-foreground">
                  {t("invite.renewCodeMeta", { username: generatedRenewCode.target_username, days: generatedRenewCode.days, hours: generatedRenewCode.validity_hours })}
                </p>
              </div>
              <DialogFooter>
                <Button variant="outline" onClick={() => void handleCopy(generatedRenewCode.code)}>
                  <Copy className="mr-2 h-4 w-4" />
                  {t("invite.copy")}
                </Button>
                <Button onClick={() => setRenewOpen(false)}>{t("invite.done")}</Button>
              </DialogFooter>
            </div>
          ) : (
            <div className="space-y-3">
              <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-700 dark:text-amber-300">
                <CalendarClock className="mr-1 inline h-3.5 w-3.5" />
                {t("invite.renewUidWarning", { uid: renewTarget?.uid })}
              </div>
              <div className="space-y-1.5">
                <Label>{t("invite.renewDays")}</Label>
                <Input value={renewDays} onChange={(event) => setRenewDays(event.target.value)} inputMode="numeric" />
                <p className="text-[10px] text-muted-foreground">{t("invite.renewDaysHelp", { days: maxCodeDays || "-" })}</p>
              </div>
              <div className="space-y-1.5">
                <Label>{t("invite.noteOptional")}</Label>
                <Textarea rows={2} maxLength={120} value={renewNote} onChange={(event) => setRenewNote(event.target.value)} />
              </div>
              <DialogFooter>
                <Button variant="outline" onClick={() => setRenewOpen(false)}>
                  {t("common.cancel")}
                </Button>
                <Button onClick={handleCreateRenewCode} disabled={renewing}>
                  {renewing ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <KeyRound className="mr-2 h-4 w-4" />}
                  {t("invite.generateRenew")}
                </Button>
              </DialogFooter>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}
