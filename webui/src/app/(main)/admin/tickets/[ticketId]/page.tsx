"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type ClipboardEvent } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  AlertCircle,
  Archive,
  ArrowLeft,
  ArrowRight,
  CheckCircle2,
  Clock,
  ExternalLink,
  ImagePlus,
  Loader2,
  MessageSquareMore,
  PlayCircle,
  RefreshCw,
  Send,
  ShieldAlert,
  Trash2,
  User,
  X,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogOverlay } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { TicketImages } from "@/components/ticket-images";
import { useToast } from "@/hooks/use-toast";
import { api, type Ticket, type TicketAttachment, type TicketReply } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { friendlyError } from "@/lib/validators";
import { useSystemStore } from "@/store/system";

const DEFAULT_TICKET_IMAGE_MAX_SIZE = 5 * 1024 * 1024;
const DEFAULT_TICKET_IMAGE_MAX_COUNT = 5;
const ALLOWED_IMAGE_TYPES = ["image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp"];

// 状态是工单处理里最高频的操作，做成一排分段按钮放在处理面板最上方：点一下就
// 保存，不用再去找"保存"按钮——之前改了状态忘记点保存，改动就静默丢了。
const STATUS_FLOW = ["open", "in_progress", "resolved", "closed"] as const;

const STATUS_MAP: Record<string, { labelKey: string; className: string; icon: typeof AlertCircle }> = {
  open: { labelKey: "tickets.statusOpen", className: "bg-warning/10 text-warning border-warning/30", icon: AlertCircle },
  in_progress: { labelKey: "tickets.statusInProgress", className: "bg-info/10 text-info border-info/30", icon: PlayCircle },
  resolved: { labelKey: "tickets.statusResolved", className: "bg-success/10 text-success border-success/30", icon: CheckCircle2 },
  closed: { labelKey: "tickets.statusClosed", className: "bg-muted text-muted-foreground border-muted", icon: Archive },
};

const PRIORITY_MAP: Record<string, { labelKey: string; className: string }> = {
  low: { labelKey: "tickets.priorityLow", className: "bg-muted text-muted-foreground" },
  medium: { labelKey: "tickets.priorityMedium", className: "bg-info/10 text-info" },
  high: { labelKey: "tickets.priorityHigh", className: "bg-warning/10 text-warning" },
  urgent: { labelKey: "tickets.priorityUrgent", className: "bg-destructive/10 text-destructive" },
};

type ConversationMessage = {
  key: string;
  author: "admin" | "user";
  username: string;
  content: string;
  createdAt: number;
};

function messageFromReply(reply: TicketReply, index: number): ConversationMessage {
  const isAdmin = reply.author === "admin" || reply.role === 0;
  return {
    key: `${reply.created_at}-${reply.uid}-${index}`,
    author: isAdmin ? "admin" : "user",
    username: reply.username,
    content: reply.content,
    createdAt: reply.created_at,
  };
}

function toDateTime(seconds?: number | null) {
  if (!seconds || seconds <= 0) return "-";
  return new Date(seconds * 1000).toLocaleString();
}

export default function AdminTicketDetailPage() {
  const { ticketId } = useParams<{ ticketId: string }>();
  const router = useRouter();
  const { t } = useI18n();
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const { info: systemInfo } = useSystemStore();
  const imageMaxSize = Number(systemInfo?.limits?.ticket_image_max_size) || DEFAULT_TICKET_IMAGE_MAX_SIZE;
  const imageMaxCount = Number(systemInfo?.limits?.ticket_image_max_count) || DEFAULT_TICKET_IMAGE_MAX_COUNT;
  const id = Number(ticketId);
  const conversationRef = useRef<HTMLDivElement>(null);
  const loadAbortRef = useRef<AbortController | null>(null);
  const loadSequenceRef = useRef(0);

  const [ticket, setTicket] = useState<Ticket | null>(null);
  const [types, setTypes] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reply, setReply] = useState("");
  const [sending, setSending] = useState(false);
  const [savingNote, setSavingNote] = useState(false);
  const [uploadingPaste, setUploadingPaste] = useState(false);
  const [deletingReplyImage, setDeletingReplyImage] = useState<string | null>(null);
  const [replyAttachments, setReplyAttachments] = useState<TicketAttachment[]>([]);
  const [previewSrc, setPreviewSrc] = useState<string | null>(null);
  const [jumpId, setJumpId] = useState("");
  // 只有内部备注还需要草稿——状态/优先级/类型改成即时保存，不再有草稿与服务端值
  // 打架的问题（原先回复一次会把未保存的草稿冲掉）。
  const [noteDraft, setNoteDraft] = useState("");
  const [patchingField, setPatchingField] = useState<string | null>(null);

  const loadTicket = useCallback(async () => {
    loadAbortRef.current?.abort();
    const controller = new AbortController();
    loadAbortRef.current = controller;
    const sequence = ++loadSequenceRef.current;
    if (!Number.isInteger(id) || id <= 0) {
      setError(t("adminTickets.invalidTicketId"));
      setLoading(false);
      loadAbortRef.current = null;
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const res = await api.adminGetTicket(id, controller.signal);
      if (controller.signal.aborted || sequence !== loadSequenceRef.current) return;
      if (res.success && res.data) {
        setTicket(res.data.ticket);
        setTypes(res.data.ticket_types || []);
        setReplyAttachments([]);
      } else {
        throw new Error(res.message || t("adminTickets.loadFailed"));
      }
    } catch (err) {
      if (controller.signal.aborted || sequence !== loadSequenceRef.current) return;
      setError(err instanceof Error ? err.message : t("adminTickets.loadFailed"));
    } finally {
      if (sequence === loadSequenceRef.current) setLoading(false);
      if (loadAbortRef.current === controller) loadAbortRef.current = null;
    }
  }, [id, t]);

  useEffect(() => {
    void loadTicket();
    return () => loadAbortRef.current?.abort();
  }, [loadTicket]);

  // 仅在「切换到另一张工单」时用服务端值初始化备注草稿。依赖整个 ticket 对象的
  // 话，发送回复 / 保存元数据后的 setTicket 都会重跑本 effect，把管理员正在编辑
  // 但尚未保存的备注冲掉。
  useEffect(() => {
    if (!ticket) return;
    setNoteDraft(ticket.admin_note || "");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ticket?.id]);

  const messages = useMemo<ConversationMessage[]>(() => {
    if (!ticket) return [];
    return [
      {
        key: "initial",
        author: "user",
        username: ticket.username,
        content: ticket.content,
        createdAt: ticket.created_at,
      },
      ...(ticket.replies || []).map(messageFromReply),
    ];
  }, [ticket]);

  const typeOptions = useMemo(() => {
    const list = types.length > 0 ? [...types] : [];
    if (ticket?.type && !list.includes(ticket.type)) list.push(ticket.type);
    return list;
  }, [ticket?.type, types]);

  const syncTicketAttachments = useCallback((attachments: TicketAttachment[]) => {
    setTicket((current) => current ? { ...current, attachments } : current);
    const existing = new Set(attachments.map((item) => item.filename));
    setReplyAttachments((current) => current.filter((item) => existing.has(item.filename)));
  }, []);

  useEffect(() => {
    const node = conversationRef.current;
    if (!node) return;
    node.scrollTo({ top: node.scrollHeight, behavior: "smooth" });
  }, [messages.length, ticket?.id]);

  // patchTicket 只提交一个字段。后端按 patch 语义处理，未提供即"不动此字段"，
  // 两个管理员并发各改一处时不会用陈旧快照回退对方的改动。
  const patchTicket = useCallback(async (field: string, payload: { status?: string; priority?: string; type?: string }) => {
    if (!ticket) return;
    setPatchingField(field);
    try {
      const res = await api.adminUpdateTicket(ticket.id, payload);
      if (res.success && res.data) {
        setTicket(res.data);
      } else {
        toast({ title: res.message || t("common.updateFailed"), variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: friendlyError(err?.errorCode, err?.message), variant: "destructive" });
    } finally {
      setPatchingField(null);
    }
  }, [ticket, t, toast]);

  const handleSaveNote = async () => {
    if (!ticket) return;
    const note = noteDraft.trim();
    if (note === (ticket.admin_note || "")) return;
    setSavingNote(true);
    try {
      const res = await api.adminUpdateTicket(ticket.id, { admin_note: note });
      if (res.success && res.data) {
        setTicket(res.data);
        toast({ title: t("adminTickets.updated") });
      } else {
        toast({ title: res.message || t("common.updateFailed"), variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: friendlyError(err?.errorCode, err?.message), variant: "destructive" });
    } finally {
      setSavingNote(false);
    }
  };

  const handleSend = async () => {
    if (!ticket) return;
    const content = reply.trim();
    if (!content) {
      toast({ title: t("tickets.replyRequired"), variant: "destructive" });
      return;
    }
    setSending(true);
    try {
      const res = await api.adminReplyTicket(ticket.id, content);
      if (res.success && res.data?.ticket) {
        // 后端在管理员回复 open 工单时会自动流转 open→in_progress，这里直接采信
        // 服务端返回的整张工单——状态已经没有本地草稿了，不会互相打架。
        setTicket(res.data.ticket);
        setReply("");
        setReplyAttachments([]);
        toast({ title: t("tickets.replySent") });
      } else {
        toast({ title: res.message || t("common.operationFailed"), variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: friendlyError(err?.errorCode, err?.message), variant: "destructive" });
    } finally {
      setSending(false);
    }
  };

  const handlePaste = async (event: ClipboardEvent<HTMLTextAreaElement>) => {
    if (!ticket || uploadingPaste) return;
    const files = Array.from(event.clipboardData.files).filter((file) => ALLOWED_IMAGE_TYPES.includes(file.type));
    if (files.length === 0) return;
    event.preventDefault();
    const existingCount = ticket.attachments?.length || 0;
    const remaining = imageMaxCount - existingCount;
    if (remaining <= 0) {
      toast({ title: t("tickets.imageTooMany", { count: imageMaxCount }), variant: "destructive" });
      return;
    }
    const pendingFiles = files.slice(0, remaining);
    setUploadingPaste(true);
    let uploaded = 0;
    try {
      for (const file of pendingFiles) {
        if (file.size > imageMaxSize) {
          toast({ title: t("tickets.imageTooLarge", { size: Math.round((imageMaxSize / (1024 * 1024)) * 10) / 10 }), variant: "destructive" });
          continue;
        }
        const res = await api.uploadTicketImage(ticket.id, file);
        if (res.success && res.data) {
          uploaded++;
          syncTicketAttachments(res.data.attachments);
          setReplyAttachments((current) => {
            if (current.some((item) => item.filename === res.data!.attachment.filename)) return current;
            return [...current, res.data!.attachment];
          });
        } else {
          toast({ title: friendlyError(res.error_code, res.message), variant: "destructive" });
        }
      }
      if (files.length > pendingFiles.length) {
        toast({ title: t("tickets.imageTooMany", { count: imageMaxCount }), variant: "destructive" });
      }
      if (uploaded > 0) {
        toast({ title: t("adminTickets.pasteImageUploaded") });
      }
    } catch (err: any) {
      toast({ title: friendlyError(err?.errorCode, err?.message), variant: "destructive" });
    } finally {
      setUploadingPaste(false);
    }
  };

  const handleDeleteReplyImage = async (attachment: TicketAttachment) => {
    if (!ticket) return;
    setDeletingReplyImage(attachment.filename);
    try {
      const res = await api.deleteTicketImage(ticket.id, attachment.filename);
      if (res.success && res.data) {
        syncTicketAttachments(res.data.attachments);
        if (previewSrc === api.ticketImageSrc(attachment.url)) setPreviewSrc(null);
        toast({ title: t("tickets.imageDeleted") });
      } else {
        toast({ title: friendlyError(res.error_code, res.message), variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: friendlyError(err?.errorCode, err?.message), variant: "destructive" });
    } finally {
      setDeletingReplyImage(null);
    }
  };

  const handleDelete = async () => {
    if (!ticket) return;
    const ok = await confirm({
      title: t("adminTickets.deleteConfirmTitle"),
      description: t("adminTickets.deleteConfirmDescription"),
      tone: "danger",
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    try {
      const res = await api.adminDeleteTicket(ticket.id);
      if (res.success) {
        toast({ title: t("adminTickets.deleted") });
        router.push("/admin/tickets");
      } else {
        toast({ title: res.message || t("common.deleteFailed"), variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: friendlyError(err?.errorCode, err?.message), variant: "destructive" });
    }
  };

  const goToTicket = () => {
    const nextId = Number(jumpId.trim());
    if (!Number.isInteger(nextId) || nextId <= 0) {
      toast({ title: t("adminTickets.invalidTicketId"), variant: "destructive" });
      return;
    }
    router.push(`/admin/tickets/${nextId}`);
  };

  if (loading && !ticket) {
    return (
      <div className="flex min-h-[50vh] items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error || !ticket) {
    return (
      <Card className="border-destructive/40">
        <CardContent className="space-y-4 p-6 text-center">
          <AlertCircle className="mx-auto h-8 w-8 text-destructive" />
          <p className="text-sm">{error || t("adminTickets.loadFailed")}</p>
          <div className="flex justify-center gap-2">
            <Button variant="outline" onClick={() => router.push("/admin/tickets")}>
              <ArrowLeft className="mr-2 h-4 w-4" />
              {t("adminTickets.backToList")}
            </Button>
            <Button onClick={() => void loadTicket()}>{t("common.retry")}</Button>
          </div>
        </CardContent>
      </Card>
    );
  }

  const status = STATUS_MAP[ticket.status] || STATUS_MAP.open;
  const priority = PRIORITY_MAP[ticket.priority] || PRIORITY_MAP.medium;
  const StatusIcon = status.icon;
  const noteDirty = noteDraft.trim() !== (ticket.admin_note || "");

  return (
    <div className="space-y-4">
      {/* 头部：返回 + 标题 + 状态概览 + 刷新。跳转放在最右，不占主要视线。 */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-2">
          <Button variant="ghost" size="sm" className="-ml-2" onClick={() => router.push("/admin/tickets")}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            {t("adminTickets.backToList")}
          </Button>
          <div className="min-w-0">
            <div className="mb-2 flex flex-wrap items-center gap-2">
              <Badge variant="outline" className={`gap-1 ${status.className}`}>
                <StatusIcon className="h-3 w-3" />
                {t(status.labelKey as any)}
              </Badge>
              <Badge variant="outline" className={priority.className}>{t(priority.labelKey as any)}</Badge>
              {ticket.type ? <Badge variant="secondary">{ticket.type === "all" ? t("tickets.typeAll") : ticket.type}</Badge> : null}
              <Badge variant="secondary" className="font-mono">#{ticket.id}</Badge>
            </div>
            <h1 className="break-words text-2xl font-bold">{ticket.title}</h1>
            <p className="mt-1 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
              <span className="inline-flex items-center gap-1"><User className="h-3.5 w-3.5" />{ticket.username} (UID: {ticket.uid})</span>
              <span className="inline-flex items-center gap-1"><Clock className="h-3.5 w-3.5" />{toDateTime(ticket.created_at)}</span>
            </p>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <form
            className="flex min-w-0 gap-2"
            onSubmit={(event) => {
              event.preventDefault();
              goToTicket();
            }}
          >
            <Input
              value={jumpId}
              onChange={(event) => setJumpId(event.target.value)}
              inputMode="numeric"
              placeholder={t("adminTickets.jumpPlaceholder")}
              className="w-32"
            />
            <Button type="submit" variant="outline" size="icon" className="shrink-0" title={t("adminTickets.jump")} aria-label={t("adminTickets.jump")}>
              <ArrowRight className="h-4 w-4" />
            </Button>
          </form>
          <Button variant="outline" onClick={() => void loadTicket()} disabled={loading}>
            <RefreshCw className={`mr-2 h-4 w-4 ${loading ? "animate-spin" : ""}`} />
            {t("common.refresh")}
          </Button>
        </div>
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_21rem]">
        {/* 左：会话。回复框明确标注"用户可见"，与右侧的内部备注形成对照。 */}
        <Card className="overflow-hidden">
          <CardContent className="flex min-h-[65dvh] min-w-0 flex-col p-0">
            <div className="flex items-center gap-2 border-b px-4 py-3">
              <MessageSquareMore className="h-4 w-4 text-primary" />
              <span className="text-sm font-semibold">{t("tickets.conversation")}</span>
              <Badge variant="secondary" className="ml-auto text-xs">{messages.length}</Badge>
            </div>
            <div ref={conversationRef} className="custom-scrollbar min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain bg-muted/20 p-4">
              {messages.map((message) => {
                const isAdmin = message.author === "admin";
                return (
                  <div key={message.key} className={`flex ${isAdmin ? "justify-end" : "justify-start"}`}>
                    <div className={`max-w-[min(44rem,86%)] rounded-2xl px-4 py-3 shadow-sm ${isAdmin ? "rounded-br-sm bg-primary text-primary-foreground" : "rounded-bl-sm border bg-background"}`}>
                      <div className={`mb-1 flex items-center gap-2 text-[11px] ${isAdmin ? "text-primary-foreground/75" : "text-muted-foreground"}`}>
                        <span className="font-semibold">{isAdmin ? t("tickets.adminReply") : message.username}</span>
                        <span>{toDateTime(message.createdAt)}</span>
                      </div>
                      <p className="whitespace-pre-wrap break-words text-sm leading-relaxed">{message.content}</p>
                    </div>
                  </div>
                );
              })}
            </div>
            <div className="border-t bg-background p-4">
              <div className="mb-2 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
                <span className="font-medium text-foreground">{t("adminTickets.replyToUser")}</span>
                {replyAttachments.length > 0 && (
                  <span className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-1 font-medium text-foreground">
                    <ImagePlus className="h-3.5 w-3.5" />
                    {t("adminTickets.replyImagesCount", { count: replyAttachments.length, max: imageMaxCount })}
                  </span>
                )}
              </div>
              {replyAttachments.length > 0 && (
                <div className="mb-3 rounded-lg border bg-muted/20 p-2">
                  <div className="custom-scrollbar flex gap-2 overflow-x-auto overscroll-x-contain pb-1">
                    {replyAttachments.map((attachment) => {
                      const src = api.ticketImageSrc(attachment.url);
                      if (!src) return null;
                      return (
                        <div
                          key={attachment.filename}
                          className="group relative h-20 w-20 shrink-0 overflow-hidden rounded-md border border-border/70 bg-background"
                        >
                          <button
                            type="button"
                            onClick={() => setPreviewSrc(src)}
                            className="block h-full w-full text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
                            title={t("adminTickets.previewReplyImage")}
                          >
                            {/* eslint-disable-next-line @next/next/no-img-element */}
                            <img src={src} alt={attachment.filename} className="h-full w-full object-cover" loading="lazy" />
                          </button>
                          <button
                            type="button"
                            onClick={() => void handleDeleteReplyImage(attachment)}
                            disabled={deletingReplyImage === attachment.filename}
                            title={t("adminTickets.removeReplyImage")}
                            className="absolute right-1 top-1 flex h-6 w-6 items-center justify-center rounded-full bg-black/70 text-white opacity-100 transition hover:bg-destructive sm:opacity-0 sm:group-hover:opacity-100"
                          >
                            {deletingReplyImage === attachment.filename ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <X className="h-3.5 w-3.5" />}
                          </button>
                        </div>
                      );
                    })}
                  </div>
                  <p className="mt-1 text-[11px] text-muted-foreground">{t("adminTickets.replyImagesHint")}</p>
                </div>
              )}
              <div className="flex flex-col gap-2 sm:flex-row">
                <Textarea
                  value={reply}
                  onChange={(event) => setReply(event.target.value)}
                  onPaste={handlePaste}
                  placeholder={t("tickets.replyPlaceholder")}
                  maxLength={5000}
                  rows={3}
                  className="min-h-[5.5rem] flex-1 resize-y"
                />
                <Button onClick={() => void handleSend()} disabled={sending || uploadingPaste || !reply.trim()} className="min-h-10 sm:self-end">
                  {sending || uploadingPaste ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Send className="mr-2 h-4 w-4" />}
                  {uploadingPaste ? t("adminTickets.pasteImageUploading") : t("tickets.replySubmit")}
                </Button>
              </div>
              <p className="mt-2 text-[11px] text-muted-foreground">{t("adminTickets.pasteImageHint")}</p>
            </div>
          </CardContent>
        </Card>

        {/* 右：处理面板。顺序按使用频率排，移动端堆叠后也是这个顺序。 */}
        <div className="space-y-4">
          <Card>
            <CardContent className="space-y-4 p-4">
              <div className="space-y-2">
                <Label>{t("adminTickets.changeStatus")}</Label>
                {/* 分段按钮：点即保存，不用再找保存按钮。 */}
                <div className="grid grid-cols-2 gap-2">
                  {STATUS_FLOW.map((value) => {
                    const option = STATUS_MAP[value];
                    const Icon = option.icon;
                    const active = ticket.status === value;
                    return (
                      <Button
                        key={value}
                        type="button"
                        size="sm"
                        variant={active ? "default" : "outline"}
                        className="justify-start text-xs"
                        disabled={patchingField !== null}
                        onClick={() => void patchTicket("status", { status: value })}
                      >
                        {patchingField === "status" && active ? (
                          <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                        ) : (
                          <Icon className="mr-1.5 h-3.5 w-3.5" />
                        )}
                        {t(option.labelKey as any)}
                      </Button>
                    );
                  })}
                </div>
              </div>

              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-1">
                <div className="space-y-2">
                  <Label>{t("tickets.priority")}</Label>
                  <Select
                    value={ticket.priority}
                    disabled={patchingField !== null}
                    onValueChange={(value) => void patchTicket("priority", { priority: value })}
                  >
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>{Object.entries(PRIORITY_MAP).map(([value, item]) => <SelectItem key={value} value={value}>{t(item.labelKey as any)}</SelectItem>)}</SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>{t("tickets.type")}</Label>
                  <Select
                    value={ticket.type}
                    disabled={patchingField !== null || typeOptions.length === 0}
                    onValueChange={(value) => void patchTicket("type", { type: value })}
                  >
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>{typeOptions.map((value) => <SelectItem key={value} value={value}>{value === "all" ? t("tickets.typeAll") : value}</SelectItem>)}</SelectContent>
                  </Select>
                </div>
              </div>

              <Separator />

              {/* 内部备注单独一块并显式标注"用户看不到"：它和左边的回复框长得很像，
                  不隔开的话很容易把内部备注当回复发出去，或者反过来。 */}
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <Label className="flex items-center gap-1">{t("adminTickets.adminNote")}</Label>
                  {noteDirty ? (
                    <Badge variant="outline" className="text-[10px] text-warning border-warning/40">{t("adminTickets.unsavedNote")}</Badge>
                  ) : null}
                </div>
                <Textarea
                  value={noteDraft}
                  onChange={(event) => setNoteDraft(event.target.value)}
                  maxLength={5000}
                  rows={4}
                  placeholder={t("adminTickets.adminNotePlaceholder")}
                  className="resize-y bg-muted/30"
                />
                <p className="flex items-start gap-1 text-[11px] text-muted-foreground">
                  <ShieldAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  {t("adminTickets.adminNoteNotVisible")}
                </p>
                <Button size="sm" className="w-full" onClick={() => void handleSaveNote()} disabled={savingNote || !noteDirty}>
                  {savingNote && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  {t("adminTickets.saveNote")}
                </Button>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardContent className="space-y-3 p-4">
              <TicketImages
                ticketId={ticket.id}
                attachments={ticket.attachments || []}
                editable
                canDelete
                maxSize={imageMaxSize}
                maxCount={imageMaxCount}
                onChange={syncTicketAttachments}
              />
            </CardContent>
          </Card>

          <Card>
            <CardContent className="space-y-3 p-4 text-sm">
              <div className="flex items-center justify-between gap-2">
                <span className="font-medium">{t("adminTickets.submitter")}</span>
                <Link
                  href="/admin/users"
                  className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
                >
                  {t("adminTickets.viewInUserAdmin")}
                  <ExternalLink className="h-3 w-3" />
                </Link>
              </div>
              <p className="break-words">
                {ticket.username} <span className="text-muted-foreground">(UID: {ticket.uid})</span>
              </p>

              <Separator />

              <div className="space-y-1.5 text-xs">
                <TimelineRow label={t("adminTickets.labelCreated")} value={toDateTime(ticket.created_at)} />
                <TimelineRow label={t("adminTickets.labelUpdated")} value={toDateTime(ticket.updated_at)} />
                {ticket.resolved_at && ticket.resolved_at > 0 && (
                  <TimelineRow label={t("adminTickets.labelResolved")} value={toDateTime(ticket.resolved_at)} />
                )}
                {ticket.closed_at && ticket.closed_at > 0 && (
                  <TimelineRow label={t("adminTickets.labelClosed")} value={toDateTime(ticket.closed_at)} />
                )}
              </div>
            </CardContent>
          </Card>

          {/* 危险区独立成块：原先删除按钮挤在一堆只读时间信息里，很容易误点。 */}
          <Card className="border-destructive/40">
            <CardContent className="space-y-2 p-4">
              <p className="flex items-center gap-1.5 text-sm font-medium text-destructive">
                <ShieldAlert className="h-4 w-4" />
                {t("adminTickets.dangerZone")}
              </p>
              <p className="text-xs text-muted-foreground">{t("adminTickets.dangerZoneHint")}</p>
              <Button variant="destructive" className="w-full" onClick={() => void handleDelete()}>
                <Trash2 className="mr-2 h-4 w-4" />
                {t("adminTickets.deleteAndBack")}
              </Button>
            </CardContent>
          </Card>
        </div>
      </div>

      <Dialog open={!!previewSrc} onOpenChange={(open) => { if (!open) setPreviewSrc(null); }}>
        <DialogOverlay className="bg-black/70" />
        <DialogContent className="max-h-[90dvh] max-w-[90vw] border-0 bg-transparent p-0 shadow-none">
          <button
            type="button"
            onClick={() => setPreviewSrc(null)}
            className="absolute right-0 top-0 z-50 flex h-9 w-9 -translate-y-12 items-center justify-center rounded-full bg-black/60 text-white transition hover:bg-black/80"
            aria-label={t("common.close")}
          >
            <X className="h-5 w-5" />
          </button>
          {/* eslint-disable-next-line @next/next/no-img-element */}
          {previewSrc && <img src={previewSrc} alt="" className="mx-auto max-h-[85dvh] w-auto rounded-lg object-contain" />}
        </DialogContent>
      </Dialog>
    </div>
  );
}

function TimelineRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-3">
      <span className="shrink-0 text-muted-foreground">{label}</span>
      <span className="text-right">{value}</span>
    </div>
  );
}
