"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  AlertCircle,
  Archive,
  Bell,
  BellOff,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Clock,
  Image as ImageIcon,
  Loader2,
  MessageSquareMore,
  Plus,
  RefreshCw,
  RotateCcw,
  Send,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { useToast } from "@/hooks/use-toast";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { api, type Ticket, type TicketAttachment, type UserTicketListItem } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { useSystemStore } from "@/store/system";
import { TicketImages } from "@/components/ticket-images";

const PAGE_SIZE = 20;
const DEFAULT_TICKET_IMAGE_MAX_SIZE = 5 * 1024 * 1024;
const DEFAULT_TICKET_IMAGE_MAX_COUNT = 5;

const STATUS_MAP: Record<string, { labelKey: string; className: string; icon: typeof AlertCircle }> = {
  open: { labelKey: "tickets.statusOpen", className: "bg-warning/10 text-warning border-warning/30", icon: AlertCircle },
  in_progress: { labelKey: "tickets.statusInProgress", className: "bg-info/10 text-info border-info/30", icon: Loader2 },
  resolved: { labelKey: "tickets.statusResolved", className: "bg-success/10 text-success border-success/30", icon: CheckCircle2 },
  closed: { labelKey: "tickets.statusClosed", className: "bg-muted text-muted-foreground border-muted", icon: Archive },
};

const PRIORITY_MAP: Record<string, { labelKey: string; className: string }> = {
  low: { labelKey: "tickets.priorityLow", className: "bg-muted text-muted-foreground" },
  medium: { labelKey: "tickets.priorityMedium", className: "bg-info/10 text-info" },
  high: { labelKey: "tickets.priorityHigh", className: "bg-warning/10 text-warning" },
  urgent: { labelKey: "tickets.priorityUrgent", className: "bg-destructive/10 text-destructive" },
};

const DEFAULT_TYPES = [{ value: "all", labelKey: "tickets.typeAll" }];

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

export default function UserTicketsPage() {
  const { toast } = useToast();
  const { t } = useI18n();
  const { info: systemInfo } = useSystemStore();
  const ticketEnabled = Boolean(systemInfo?.features?.ticket_system);
  const imageMaxSize = Number(systemInfo?.limits?.ticket_image_max_size) || DEFAULT_TICKET_IMAGE_MAX_SIZE;
  const imageMaxCount = Number(systemInfo?.limits?.ticket_image_max_count) || DEFAULT_TICKET_IMAGE_MAX_COUNT;

  const [page, setPage] = useState(1);
  const [createOpen, setCreateOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  const [ticketType, setTicketType] = useState("all");
  const [priority, setPriority] = useState("medium");
  const [notifyTelegram, setNotifyTelegram] = useState(true);
  const [saving, setSaving] = useState(false);
  const [selectedTicketID, setSelectedTicketID] = useState<number | null>(null);
  const [selectedTicket, setSelectedTicket] = useState<Ticket | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [replyDraft, setReplyDraft] = useState("");
  const [replying, setReplying] = useState(false);
  const [mutatingTicketID, setMutatingTicketID] = useState<number | null>(null);
  const detailAbortRef = useRef<AbortController | null>(null);

  const loadTickets = useCallback(async (signal?: AbortSignal) => {
    const res = await api.getMyTickets({ page, per_page: PAGE_SIZE }, signal);
    if (!res.success || !res.data) throw new Error(res.message || t("common.networkError"));
    return res.data;
  }, [page, t]);

  const { data, isLoading, error, execute: reload } = useAsyncResource(loadTickets, { immediate: true });
  const types = Array.isArray(data?.ticket_types) && data.ticket_types.length
    ? data.ticket_types
    : DEFAULT_TYPES.map((item) => item.value);
  const totalPages = Math.max(1, Math.ceil((data?.total || 0) / PAGE_SIZE));

  const typeLabelFor = (value: string) => {
    const known = DEFAULT_TYPES.find((item) => item.value === value);
    return known ? t(known.labelKey as never) : value;
  };

  const refreshCurrentPage = useCallback(() => {
    void reload().catch(() => undefined);
  }, [reload]);

  const closeTicketDetail = useCallback(() => {
    detailAbortRef.current?.abort();
    detailAbortRef.current = null;
    setSelectedTicketID(null);
    setSelectedTicket(null);
    setDetailLoading(false);
    setReplyDraft("");
  }, []);

  useEffect(() => () => detailAbortRef.current?.abort(), []);

  const openTicketDetail = async (ticketID: number) => {
    detailAbortRef.current?.abort();
    const controller = new AbortController();
    detailAbortRef.current = controller;
    setSelectedTicketID(ticketID);
    setSelectedTicket(null);
    setDetailLoading(true);
    setReplyDraft("");
    try {
      const response = await api.getMyTicket(ticketID, controller.signal);
      if (!response.success || !response.data) throw new Error(response.message || t("common.networkError"));
      if (!controller.signal.aborted) setSelectedTicket(response.data.ticket);
    } catch (error) {
      if (!isAbortError(error)) {
        toast({ title: t("common.error"), description: error instanceof Error ? error.message : t("common.networkError"), variant: "destructive" });
        closeTicketDetail();
      }
    } finally {
      if (detailAbortRef.current === controller) {
        detailAbortRef.current = null;
        setDetailLoading(false);
      }
    }
  };

  const replaceSelectedTicket = (ticket: Ticket) => {
    if (selectedTicketID === ticket.id) setSelectedTicket(ticket);
    refreshCurrentPage();
  };

  const handleCreate = async () => {
    if (!title.trim()) {
      toast({ title: t("tickets.titleRequired"), variant: "destructive" });
      return;
    }
    if (!content.trim()) {
      toast({ title: t("tickets.contentRequired"), variant: "destructive" });
      return;
    }
    setSaving(true);
    try {
      const response = await api.createTicket({
        title: title.trim(),
        content: content.trim(),
        type: ticketType,
        priority,
        notify_telegram: notifyTelegram,
      });
      if (!response.success) throw new Error(response.message || t("common.networkError"));
      toast({ title: t("tickets.submitted"), variant: "success" });
      setCreateOpen(false);
      setTitle("");
      setContent("");
      if (page === 1) refreshCurrentPage();
      else setPage(1);
    } catch (error) {
      toast({ title: t("common.error"), description: error instanceof Error ? error.message : t("common.networkError"), variant: "destructive" });
    } finally {
      setSaving(false);
    }
  };

  const handleTicketMutation = async (
    ticketID: number,
    operation: () => Promise<{ success: boolean; message?: string; data?: Ticket }>,
    successMessage: string,
  ) => {
    setMutatingTicketID(ticketID);
    try {
      const response = await operation();
      if (!response.success || !response.data) throw new Error(response.message || t("common.networkError"));
      replaceSelectedTicket(response.data);
      toast({ title: successMessage, variant: "success" });
    } catch (error) {
      toast({ title: t("common.error"), description: error instanceof Error ? error.message : t("common.networkError"), variant: "destructive" });
    } finally {
      setMutatingTicketID(null);
    }
  };

  const handleReply = async () => {
    if (!selectedTicket) return;
    const reply = replyDraft.trim();
    if (!reply) {
      toast({ title: t("tickets.replyRequired"), variant: "destructive" });
      return;
    }
    setReplying(true);
    try {
      const response = await api.replyTicket(selectedTicket.id, reply);
      if (!response.success || !response.data) throw new Error(response.message || t("common.networkError"));
      setSelectedTicket(response.data.ticket);
      setReplyDraft("");
      refreshCurrentPage();
      toast({ title: t("tickets.replySent"), variant: "success" });
    } catch (error) {
      toast({ title: t("common.error"), description: error instanceof Error ? error.message : t("common.networkError"), variant: "destructive" });
    } finally {
      setReplying(false);
    }
  };

  const updateSelectedAttachments = (attachments: TicketAttachment[]) => {
    setSelectedTicket((current) => current ? { ...current, attachments } : current);
    refreshCurrentPage();
  };

  if (!ticketEnabled) {
    return (
      <div className="space-y-6">
        <Card className="border-dashed"><CardContent className="p-8 text-center">
          <AlertCircle className="mx-auto mb-2 h-10 w-10 text-muted-foreground/40" />
          <p className="font-medium">{t("tickets.disabled")}</p>
        </CardContent></Card>
      </div>
    );
  }

  return (
    <div className="min-w-0 space-y-5 pb-8 sm:space-y-6">
      <header className="flex min-w-0 flex-col gap-3 border-b pb-5 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <h1 className="flex items-center gap-2 text-2xl font-bold"><MessageSquareMore className="h-5 w-5 shrink-0" />{t("tickets.pageTitle")}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t("tickets.pageDescription")}</p>
        </div>
        <div className="grid grid-cols-2 gap-2 sm:flex">
          <Button variant="outline" size="sm" onClick={refreshCurrentPage} disabled={isLoading}>
            <RefreshCw className={`mr-1.5 h-4 w-4 ${isLoading ? "animate-spin" : ""}`} />{t("common.refresh")}
          </Button>
          <Button onClick={() => { setTitle(""); setContent(""); setTicketType(types[0] || "all"); setNotifyTelegram(true); setCreateOpen(true); }} size="sm">
            <Plus className="mr-1.5 h-4 w-4" />{t("tickets.submit")}
          </Button>
        </div>
      </header>

      {error ? (
        <Card className="border-destructive/40"><CardContent className="space-y-3 p-6 text-center">
          <AlertCircle className="mx-auto h-8 w-8 text-destructive" />
          <p className="text-sm">{error}</p>
          <Button variant="outline" size="sm" onClick={refreshCurrentPage}>{t("common.retry")}</Button>
        </CardContent></Card>
      ) : isLoading && !data ? (
        <Card className="border-dashed"><CardContent className="p-8 text-center"><Loader2 className="mx-auto h-6 w-6 animate-spin text-muted-foreground" /></CardContent></Card>
      ) : !data?.tickets.length ? (
        <Card className="border-dashed"><CardContent className="p-8 text-center">
          <MessageSquareMore className="mx-auto mb-2 h-10 w-10 text-muted-foreground/40" />
          <p className="font-medium">{t("tickets.noTickets")}</p>
          <p className="mt-1 text-xs text-muted-foreground">{t("tickets.noTicketsHint")}</p>
        </CardContent></Card>
      ) : (
        <div className="space-y-3">
          {data.tickets.map((ticket) => <TicketListRow
            key={ticket.id}
            ticket={ticket}
            typeLabel={typeLabelFor(ticket.type)}
            busy={mutatingTicketID === ticket.id}
            onOpen={() => void openTicketDetail(ticket.id)}
            onToggleNotify={() => void handleTicketMutation(ticket.id, () => api.toggleTicketNotify(ticket.id, !ticket.notify_telegram), ticket.notify_telegram ? t("tickets.notifyOff") : t("tickets.notifyOn"))}
            onClose={() => void handleTicketMutation(ticket.id, () => api.closeOwnTicket(ticket.id), t("tickets.closed"))}
            onReopen={() => void handleTicketMutation(ticket.id, () => api.reopenOwnTicket(ticket.id), t("tickets.reopened"))}
          />)}
        </div>
      )}

      {data && data.total > 0 ? (
        <div className="flex flex-wrap items-center justify-end gap-2 border-t pt-4">
          <Button variant="outline" size="sm" aria-label={t("common.previousPage")} onClick={() => setPage((current) => Math.max(1, current - 1))} disabled={page <= 1 || isLoading}><ChevronLeft className="h-4 w-4" /></Button>
          <p className="min-w-0 text-center text-xs text-muted-foreground">{t("tickets.pageSummary", { page, total: totalPages, count: data.total })}</p>
          <Button variant="outline" size="sm" aria-label={t("common.nextPage")} onClick={() => setPage((current) => Math.min(totalPages, current + 1))} disabled={page >= totalPages || isLoading}><ChevronRight className="h-4 w-4" /></Button>
        </div>
      ) : null}

      <TicketDetailDialog
        ticket={selectedTicket}
        loading={detailLoading}
        open={selectedTicketID !== null}
        replyDraft={replyDraft}
        replying={replying}
        mutating={mutatingTicketID === selectedTicket?.id}
        imageMaxSize={imageMaxSize}
        imageMaxCount={imageMaxCount}
        typeLabelFor={typeLabelFor}
        onClose={closeTicketDetail}
        onReply={() => void handleReply()}
        onReplyDraftChange={setReplyDraft}
        onAttachmentsChange={updateSelectedAttachments}
        onToggleNotify={() => selectedTicket && void handleTicketMutation(selectedTicket.id, () => api.toggleTicketNotify(selectedTicket.id, !selectedTicket.notify_telegram), selectedTicket.notify_telegram ? t("tickets.notifyOff") : t("tickets.notifyOn"))}
        onCloseTicket={() => selectedTicket && void handleTicketMutation(selectedTicket.id, () => api.closeOwnTicket(selectedTicket.id), t("tickets.closed"))}
        onReopenTicket={() => selectedTicket && void handleTicketMutation(selectedTicket.id, () => api.reopenOwnTicket(selectedTicket.id), t("tickets.reopened"))}
      />

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader><DialogTitle>{t("tickets.createTitle")}</DialogTitle><DialogDescription>{t("tickets.createDescription")}</DialogDescription></DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2"><Label>{t("tickets.title")}</Label><Input value={title} onChange={(event) => setTitle(event.target.value)} placeholder={t("tickets.titlePlaceholder")} maxLength={200} /></div>
            <div className="space-y-2"><Label>{t("tickets.content")}</Label><Textarea value={content} onChange={(event) => setContent(event.target.value)} placeholder={t("tickets.contentPlaceholder")} rows={5} maxLength={10000} className="resize-y" /></div>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="space-y-2"><Label>{t("tickets.type")}</Label><Select value={ticketType} onValueChange={setTicketType}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{types.map((type) => <SelectItem key={type} value={type}>{typeLabelFor(type)}</SelectItem>)}</SelectContent></Select></div>
              <div className="space-y-2"><Label>{t("tickets.priority")}</Label><Select value={priority} onValueChange={setPriority}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{Object.entries(PRIORITY_MAP).map(([value, option]) => <SelectItem key={value} value={value}>{t(option.labelKey as never)}</SelectItem>)}</SelectContent></Select></div>
            </div>
            <div className="flex min-w-0 items-center justify-between gap-4 rounded-lg border px-3 py-2">
              <div className="min-w-0 space-y-0.5"><Label className="text-sm">{t("tickets.notifyTelegram")}</Label><p className="text-xs text-muted-foreground">{t("tickets.notifyTelegramDesc")}</p></div>
              <Switch checked={notifyTelegram} onCheckedChange={setNotifyTelegram} />
            </div>
          </div>
          <DialogFooter><Button variant="outline" onClick={() => setCreateOpen(false)}>{t("common.cancel")}</Button><Button onClick={() => void handleCreate()} disabled={saving}>{saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}{t("tickets.submit")}</Button></DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function TicketListRow({
  ticket,
  typeLabel,
  busy,
  onOpen,
  onToggleNotify,
  onClose,
  onReopen,
}: {
  ticket: UserTicketListItem;
  typeLabel: string;
  busy: boolean;
  onOpen: () => void;
  onToggleNotify: () => void;
  onClose: () => void;
  onReopen: () => void;
}) {
  const { t } = useI18n();
  const status = STATUS_MAP[ticket.status] || STATUS_MAP.open;
  const priority = PRIORITY_MAP[ticket.priority] || PRIORITY_MAP.medium;
  const StatusIcon = status.icon;
  const closed = ticket.status === "closed";

  return (
    <Card className={closed ? "opacity-75" : ""}>
      <CardContent className="flex min-w-0 flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5">
        <div className="min-w-0 space-y-2">
          <div className="flex flex-wrap items-center gap-1.5">
            <Badge variant="outline" className={`gap-1 text-[10px] ${status.className}`}><StatusIcon className="h-3 w-3" />{t(status.labelKey as never)}</Badge>
            <Badge variant="outline" className={`text-[10px] ${priority.className}`}>{t(priority.labelKey as never)}</Badge>
            {typeLabel ? <Badge variant="secondary" className="text-[10px]">{typeLabel}</Badge> : null}
            <Badge variant="secondary" className="font-mono text-[10px]">#{ticket.id}</Badge>
          </div>
          <h2 className="break-words text-base font-semibold">{ticket.title}</h2>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span className="flex items-center gap-1"><Clock className="h-3.5 w-3.5" />{t("tickets.updatedAt", { time: new Date(ticket.updated_at * 1000).toLocaleString() })}</span>
            <span className="flex items-center gap-1"><MessageSquareMore className="h-3.5 w-3.5" />{ticket.reply_count}</span>
            <span className="flex items-center gap-1"><ImageIcon className="h-3.5 w-3.5" />{ticket.attachment_count}</span>
          </div>
        </div>
        {/* 三个操作按钮此前排在两列网格里，第三个会掉到第二行第一列、和铃铛图标
            对齐，看着像两对不同功能。改成自由换行的一排。 */}
        <div className="flex shrink-0 flex-wrap items-center gap-2 sm:justify-end">
          <Button variant="ghost" size="icon" aria-label={ticket.notify_telegram ? t("tickets.notifyOn") : t("tickets.notifyOff")} title={ticket.notify_telegram ? t("tickets.notifyOn") : t("tickets.notifyOff")} onClick={onToggleNotify} disabled={busy}>{ticket.notify_telegram ? <Bell className="h-4 w-4 text-info" /> : <BellOff className="h-4 w-4 text-muted-foreground" />}</Button>
          <Button variant="outline" size="sm" onClick={onOpen} disabled={busy}>{t("common.view")}</Button>
          {!closed ? <Button variant="ghost" size="sm" className="text-muted-foreground hover:text-destructive" onClick={onClose} disabled={busy}><Archive className="mr-1.5 h-3.5 w-3.5" />{t("tickets.closeTicket")}</Button> : <Button variant="ghost" size="sm" onClick={onReopen} disabled={busy}><RotateCcw className="mr-1.5 h-3.5 w-3.5" />{t("tickets.reopenTicket")}</Button>}
        </div>
      </CardContent>
    </Card>
  );
}

function TicketDetailDialog({
  ticket,
  loading,
  open,
  replyDraft,
  replying,
  mutating,
  imageMaxSize,
  imageMaxCount,
  typeLabelFor,
  onClose,
  onReply,
  onReplyDraftChange,
  onAttachmentsChange,
  onToggleNotify,
  onCloseTicket,
  onReopenTicket,
}: {
  ticket: Ticket | null;
  loading: boolean;
  open: boolean;
  replyDraft: string;
  replying: boolean;
  mutating: boolean;
  imageMaxSize: number;
  imageMaxCount: number;
  typeLabelFor: (value: string) => string;
  onClose: () => void;
  onReply: () => void;
  onReplyDraftChange: (value: string) => void;
  onAttachmentsChange: (attachments: TicketAttachment[]) => void;
  onToggleNotify: () => void;
  onCloseTicket: () => void;
  onReopenTicket: () => void;
}) {
  const { t } = useI18n();
  const status = ticket ? STATUS_MAP[ticket.status] || STATUS_MAP.open : STATUS_MAP.open;
  const priority = ticket ? PRIORITY_MAP[ticket.priority] || PRIORITY_MAP.medium : PRIORITY_MAP.medium;
  const StatusIcon = status.icon;
  const closed = ticket?.status === "closed";

  return (
    <Dialog open={open} onOpenChange={(next) => { if (!next) onClose(); }}>
      <DialogContent className="max-w-4xl overflow-hidden p-0 sm:max-h-[calc(100dvh-2rem)]">
        {loading || !ticket ? <div className="flex min-h-56 items-center justify-center"><Loader2 className="h-6 w-6 animate-spin text-muted-foreground" /></div> : (
          <div className="flex min-h-0 max-h-[calc(100dvh-1.5rem)] flex-col sm:max-h-[calc(100dvh-2rem)]">
            <DialogHeader className="shrink-0 border-b px-4 pb-3 pt-4 sm:px-6 sm:pb-4 sm:pt-6">
              <div className="flex min-w-0 flex-wrap items-center gap-1.5 pr-8">
                <Badge variant="outline" className={`gap-1 text-[10px] ${status.className}`}><StatusIcon className="h-3 w-3" />{t(status.labelKey as never)}</Badge>
                <Badge variant="outline" className={`text-[10px] ${priority.className}`}>{t(priority.labelKey as never)}</Badge>
                {ticket.type ? <Badge variant="secondary" className="text-[10px]">{typeLabelFor(ticket.type)}</Badge> : null}
                <Badge variant="secondary" className="font-mono text-[10px]">#{ticket.id}</Badge>
              </div>
              <DialogTitle>{ticket.title}</DialogTitle>
              <DialogDescription>{t("tickets.createdAt", { time: new Date(ticket.created_at * 1000).toLocaleString() })}</DialogDescription>
            </DialogHeader>
            <div className="custom-scrollbar min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4 sm:px-6">
              <div className="whitespace-pre-wrap break-words rounded-lg border border-border/60 bg-muted/30 p-4 text-sm">{ticket.content}</div>
              <TicketImages ticketId={ticket.id} attachments={ticket.attachments || []} editable={!closed} maxSize={imageMaxSize} maxCount={imageMaxCount} onChange={onAttachmentsChange} />
              {ticket.replies?.length ? <section className="space-y-3"><h3 className="flex items-center gap-2 text-sm font-semibold"><MessageSquareMore className="h-4 w-4 text-info" />{t("tickets.conversation")}</h3>{ticket.replies.map((reply, index) => {
                const adminReply = reply.author === "admin" || reply.role === 0;
                return <div key={`${reply.created_at}-${reply.uid}-${index}`} className={`rounded-lg border p-3 ${adminReply ? "border-info/20 bg-info/5" : "border-border bg-background"}`}><div className="mb-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground"><span className={adminReply ? "font-semibold text-info" : "font-semibold text-foreground"}>{adminReply ? t("tickets.adminReply") : t("tickets.userReply")}</span><span>{reply.username}</span><span className="sm:ml-auto">{new Date(reply.created_at * 1000).toLocaleString()}</span></div><p className="whitespace-pre-wrap break-words text-sm">{reply.content}</p></div>;
              })}</section> : null}
            </div>
            <div className="shrink-0 space-y-3 border-t bg-background px-4 py-3 sm:px-6 sm:py-4">
              {!closed ? <><Textarea value={replyDraft} onChange={(event) => onReplyDraftChange(event.target.value)} placeholder={t("tickets.replyPlaceholder")} rows={3} maxLength={5000} className="resize-y" /><div className="grid grid-cols-1 gap-2 sm:flex sm:justify-between"><div className="grid grid-cols-2 gap-2 sm:flex"><Button variant="outline" size="sm" onClick={onToggleNotify} disabled={mutating}>{ticket.notify_telegram ? <Bell className="mr-1.5 h-3.5 w-3.5" /> : <BellOff className="mr-1.5 h-3.5 w-3.5" />}{ticket.notify_telegram ? t("tickets.notifyOn") : t("tickets.notifyOff")}</Button><Button variant="ghost" size="sm" className="text-muted-foreground hover:text-destructive" onClick={onCloseTicket} disabled={mutating}><Archive className="mr-1.5 h-3.5 w-3.5" />{t("tickets.closeTicket")}</Button></div><Button size="sm" onClick={onReply} disabled={replying}>{replying ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : <Send className="mr-1.5 h-3.5 w-3.5" />}{t("tickets.replySubmit")}</Button></div></> : <div className="flex justify-end"><Button size="sm" onClick={onReopenTicket} disabled={mutating}><RotateCcw className="mr-1.5 h-3.5 w-3.5" />{t("tickets.reopenTicket")}</Button></div>}
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
