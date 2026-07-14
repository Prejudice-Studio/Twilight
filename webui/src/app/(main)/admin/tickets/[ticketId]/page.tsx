"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type ClipboardEvent } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  AlertCircle,
  Archive,
  ArrowLeft,
  ArrowRight,
  CheckCircle2,
  Clock,
  ImagePlus,
  Loader2,
  MessageSquareMore,
  PlayCircle,
  RefreshCw,
  Send,
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

  const [ticket, setTicket] = useState<Ticket | null>(null);
  const [types, setTypes] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reply, setReply] = useState("");
  const [sending, setSending] = useState(false);
  const [saving, setSaving] = useState(false);
  const [uploadingPaste, setUploadingPaste] = useState(false);
  const [deletingReplyImage, setDeletingReplyImage] = useState<string | null>(null);
  const [replyAttachments, setReplyAttachments] = useState<TicketAttachment[]>([]);
  const [previewSrc, setPreviewSrc] = useState<string | null>(null);
  const [jumpId, setJumpId] = useState("");
  const [statusDraft, setStatusDraft] = useState("");
  const [priorityDraft, setPriorityDraft] = useState("");
  const [typeDraft, setTypeDraft] = useState("");
  const [noteDraft, setNoteDraft] = useState("");

  const loadTicket = useCallback(async () => {
    if (!Number.isInteger(id) || id <= 0) {
      setError(t("adminTickets.invalidTicketId"));
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const res = await api.adminGetTicket(id);
      if (res.success && res.data) {
        setTicket(res.data.ticket);
        setTypes(res.data.ticket_types || []);
        setReplyAttachments([]);
      } else {
        throw new Error(res.message || t("adminTickets.loadFailed"));
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t("adminTickets.loadFailed"));
    } finally {
      setLoading(false);
    }
  }, [id, t]);

  useEffect(() => {
    void loadTicket();
  }, [loadTicket]);

  useEffect(() => {
    if (!ticket) return;
    setStatusDraft(ticket.status);
    setPriorityDraft(ticket.priority);
    setTypeDraft(ticket.type);
    setNoteDraft(ticket.admin_note || "");
  }, [ticket]);

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

  const goToTicket = () => {
    const nextId = Number(jumpId.trim());
    if (!Number.isInteger(nextId) || nextId <= 0) {
      toast({ title: t("adminTickets.invalidTicketId"), variant: "destructive" });
      return;
    }
    router.push(`/admin/tickets/${nextId}`);
  };

  const handleSaveMeta = async () => {
    if (!ticket) return;
    setSaving(true);
    try {
      const res = await api.adminUpdateTicket(ticket.id, {
        status: statusDraft,
        priority: priorityDraft,
        type: typeDraft,
        admin_note: noteDraft.trim(),
      });
      if (res.success && res.data) {
        setTicket(res.data);
        toast({ title: t("adminTickets.updated") });
      } else {
        toast({ title: res.message || t("common.updateFailed"), variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: friendlyError(err?.errorCode, err?.message), variant: "destructive" });
    } finally {
      setSaving(false);
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

  return (
    <div className="space-y-4">
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
              <Badge variant="secondary" className="font-mono">#{ticket.id}</Badge>
            </div>
            <h1 className="break-words text-2xl font-bold">{ticket.title}</h1>
            <p className="mt-1 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
              <span className="inline-flex items-center gap-1"><User className="h-3.5 w-3.5" />{ticket.username} (UID: {ticket.uid})</span>
              <span className="inline-flex items-center gap-1"><Clock className="h-3.5 w-3.5" />{toDateTime(ticket.created_at)}</span>
            </p>
          </div>
        </div>
        <Button variant="outline" onClick={() => void loadTicket()} disabled={loading}>
          <RefreshCw className={`mr-2 h-4 w-4 ${loading ? "animate-spin" : ""}`} />
          {t("common.refresh")}
        </Button>
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]">
        <Card className="overflow-hidden">
          <CardContent className="flex min-h-[65vh] flex-col p-0">
            <div className="border-b px-4 py-3">
              <div className="flex items-center gap-2 text-sm font-semibold">
                <MessageSquareMore className="h-4 w-4 text-primary" />
                {t("tickets.conversation")}
              </div>
            </div>
            <div ref={conversationRef} className="flex-1 space-y-4 overflow-y-auto bg-muted/20 p-4">
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
                <span>{t("adminTickets.pasteImageHint")}</span>
                {replyAttachments.length > 0 && (
                  <span className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-1 font-medium text-foreground">
                    <ImagePlus className="h-3.5 w-3.5" />
                    {t("adminTickets.replyImagesCount", { count: replyAttachments.length, max: imageMaxCount })}
                  </span>
                )}
              </div>
              {replyAttachments.length > 0 && (
                <div className="mb-3 rounded-lg border bg-muted/20 p-2">
                  <div className="flex gap-2 overflow-x-auto pb-1">
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
            </div>
          </CardContent>
        </Card>

        <div className="space-y-4">
          <Card>
            <CardContent className="space-y-4 p-4">
              <div className="space-y-2">
                <Label>{t("adminTickets.jump")}</Label>
                <form
                  className="flex gap-2"
                  onSubmit={(event) => {
                    event.preventDefault();
                    goToTicket();
                  }}
                >
                  <Input value={jumpId} onChange={(event) => setJumpId(event.target.value)} inputMode="numeric" placeholder={t("adminTickets.jumpPlaceholder")} />
                  <Button type="submit" variant="outline" size="icon" className="shrink-0">
                    <ArrowRight className="h-4 w-4" />
                  </Button>
                </form>
              </div>
              <div className="grid gap-3">
                <div className="space-y-2">
                  <Label>{t("adminTickets.changeStatus")}</Label>
                  <Select value={statusDraft} onValueChange={setStatusDraft}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>{Object.entries(STATUS_MAP).map(([value, item]) => <SelectItem key={value} value={value}>{t(item.labelKey as any)}</SelectItem>)}</SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>{t("tickets.priority")}</Label>
                  <Select value={priorityDraft} onValueChange={setPriorityDraft}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>{Object.entries(PRIORITY_MAP).map(([value, item]) => <SelectItem key={value} value={value}>{t(item.labelKey as any)}</SelectItem>)}</SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>{t("tickets.type")}</Label>
                  <Select value={typeDraft} onValueChange={setTypeDraft}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>{typeOptions.map((value) => <SelectItem key={value} value={value}>{value === "all" ? t("tickets.typeAll") : value}</SelectItem>)}</SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>{t("adminTickets.adminNote")}</Label>
                  <Textarea value={noteDraft} onChange={(event) => setNoteDraft(event.target.value)} maxLength={5000} rows={4} />
                </div>
                <Button onClick={() => void handleSaveMeta()} disabled={saving}>
                  {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  {t("adminTickets.saveMetadata")}
                </Button>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardContent className="space-y-4 p-4">
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
              <div className="flex justify-between gap-3">
                <span className="text-muted-foreground">{t("tickets.createdAt", { time: "" }).replace(/\s*\{time\}\s*/g, "").trim() || t("tickets.createdAt", { time: toDateTime(ticket.created_at) })}</span>
                <span>{toDateTime(ticket.created_at)}</span>
              </div>
              <div className="flex justify-between gap-3">
                <span className="text-muted-foreground">{t("tickets.updatedAt", { time: "" }).replace(/\s*\{time\}\s*/g, "").trim() || t("tickets.updatedAt", { time: toDateTime(ticket.updated_at) })}</span>
                <span>{toDateTime(ticket.updated_at)}</span>
              </div>
              {ticket.resolved_at && ticket.resolved_at > 0 && (
                <div className="flex justify-between gap-3">
                  <span className="text-muted-foreground">{t("tickets.statusResolved")}</span>
                  <span>{toDateTime(ticket.resolved_at)}</span>
                </div>
              )}
              {ticket.closed_at && ticket.closed_at > 0 && (
                <div className="flex justify-between gap-3">
                  <span className="text-muted-foreground">{t("tickets.statusClosed")}</span>
                  <span>{toDateTime(ticket.closed_at)}</span>
                </div>
              )}
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
