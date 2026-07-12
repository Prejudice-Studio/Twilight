"use client";

import { useState, useCallback, useEffect } from "react";
import { motion } from "framer-motion";
import {
  MessageSquareMore, Plus, Loader2, Clock, AlertCircle,
  CheckCircle2, RotateCcw, Archive, Bell, BellOff,
  Send,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { useToast } from "@/hooks/use-toast";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { api, type Ticket } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { useSystemStore } from "@/store/system";
import { TicketImages } from "@/components/ticket-images";

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

const DEFAULT_TYPES = [
  { value: "all", labelKey: "tickets.typeAll" },
];

export default function UserTicketsPage() {
  const { toast } = useToast();
  const { t } = useI18n();
  const { info: systemInfo } = useSystemStore();
  const ticketEnabled = Boolean(systemInfo?.features?.ticket_system);
  const imageMaxSize = Number(systemInfo?.limits?.ticket_image_max_size) || DEFAULT_TICKET_IMAGE_MAX_SIZE;
  const imageMaxCount = Number(systemInfo?.limits?.ticket_image_max_count) || DEFAULT_TICKET_IMAGE_MAX_COUNT;

  const [createOpen, setCreateOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  const [ticketType, setTicketType] = useState("all");
  const [priority, setPriority] = useState("medium");
  const [notifyTelegram, setNotifyTelegram] = useState(true);
  const [saving, setSaving] = useState(false);
  const [replyDrafts, setReplyDrafts] = useState<Record<number, string>>({});
  const [replyingId, setReplyingId] = useState<number | null>(null);

  const loadTickets = useCallback(async () => {
    const res = await api.getMyTickets();
    if (res.success && res.data) return { tickets: res.data.tickets, types: res.data.ticket_types || [] };
    throw new Error(res.message || t("common.networkError"));
  }, [t]);

  const { data, isLoading, error, execute: reload } = useAsyncResource(loadTickets, { immediate: true });

  const types = Array.isArray(data?.types) && data.types.length ? data.types : DEFAULT_TYPES.map((d) => d.value);
  const typeLabelFor = (value: string) => {
    const known = DEFAULT_TYPES.find((d) => d.value === value);
    return known ? t(known.labelKey as any) : value;
  };

  const handleCreate = async () => {
    if (!title.trim()) { toast({ title: t("tickets.titleRequired"), variant: "destructive" }); return; }
    if (!content.trim()) { toast({ title: t("tickets.contentRequired"), variant: "destructive" }); return; }
    setSaving(true);
    try {
      const res = await api.createTicket({ title: title.trim(), content: content.trim(), type: ticketType, priority, notify_telegram: notifyTelegram });
      if (res.success) { toast({ title: t("tickets.submitted") }); setCreateOpen(false); setTitle(""); setContent(""); await reload(); }
      else toast({ title: res.message, variant: "destructive" });
    } catch (err: any) { toast({ title: err?.message || t("common.networkError"), variant: "destructive" }); }
    finally { setSaving(false); }
  };

  const handleClose = async (id: number) => {
    try {
      const res = await api.closeOwnTicket(id);
      if (res.success) { toast({ title: t("tickets.closed") }); await reload(); }
      else toast({ title: res.message, variant: "destructive" });
    } catch (err: any) { toast({ title: err?.message || t("common.networkError"), variant: "destructive" }); }
  };

  const handleReopen = async (id: number) => {
    try {
      const res = await api.reopenOwnTicket(id);
      if (res.success) { toast({ title: t("tickets.reopened") }); await reload(); }
      else toast({ title: res.message, variant: "destructive" });
    } catch (err: any) { toast({ title: err?.message || t("common.networkError"), variant: "destructive" }); }
  };

  const handleReply = async (id: number) => {
    const text = (replyDrafts[id] || "").trim();
    if (!text) { toast({ title: t("tickets.replyRequired"), variant: "destructive" }); return; }
    setReplyingId(id);
    try {
      const res = await api.replyTicket(id, text);
      if (res.success) {
        toast({ title: t("tickets.replySent") });
        setReplyDrafts((current) => ({ ...current, [id]: "" }));
        await reload();
      } else {
        toast({ title: res.message, variant: "destructive" });
      }
    } catch (err: any) { toast({ title: err?.message || t("common.networkError"), variant: "destructive" }); }
    finally { setReplyingId(null); }
  };

  if (!ticketEnabled) {
    return (
      <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
        <Card className="border-dashed"><CardContent className="p-8 text-center">
          <AlertCircle className="h-10 w-10 mx-auto text-muted-foreground mb-2 opacity-40" />
          <p className="font-medium">{t("tickets.disabled")}</p>
        </CardContent></Card>
      </motion.div>
    );
  }

  return (
    <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <MessageSquareMore className="h-5 w-5" />{t("tickets.pageTitle")}
          </h1>
          <p className="text-sm text-muted-foreground mt-1">{t("tickets.pageDescription")}</p>
        </div>
        <Button onClick={() => { setTitle(""); setContent(""); setTicketType(types[0] || "all"); setNotifyTelegram(true); setCreateOpen(true); }} size="sm">
          <Plus className="h-4 w-4 mr-1" />{t("tickets.submit")}
        </Button>
      </div>

      {error ? (
        <Card className="border-destructive/40"><CardContent className="p-6 text-center space-y-3">
          <AlertCircle className="h-8 w-8 mx-auto text-destructive" />
          <p className="text-sm">{error}</p>
          <Button variant="outline" size="sm" onClick={() => void reload()}>{t("common.retry")}</Button>
        </CardContent></Card>
      ) : isLoading ? (
        <Card className="border-dashed"><CardContent className="p-8 text-center">
          <Loader2 className="h-6 w-6 mx-auto animate-spin text-muted-foreground" />
        </CardContent></Card>
      ) : !data?.tickets?.length ? (
        <Card className="border-dashed"><CardContent className="p-8 text-center">
          <MessageSquareMore className="h-10 w-10 mx-auto text-muted-foreground mb-2 opacity-40" />
          <p className="font-medium">{t("tickets.noTickets")}</p>
          <p className="text-xs text-muted-foreground mt-1">{t("tickets.noTicketsHint")}</p>
        </CardContent></Card>
      ) : (
        <div className="space-y-4">
          {data.tickets.map((ticket: Ticket) => {
            const s = STATUS_MAP[ticket.status] || STATUS_MAP.open;
            const p = PRIORITY_MAP[ticket.priority] || PRIORITY_MAP.medium;
            const typeLabel = ticket.type ? typeLabelFor(ticket.type) : "";
            const SI = s.icon;
            const isClosed = ticket.status === "closed";
            const isResolved = ticket.status === "resolved";
            return (
              <Card key={ticket.id} className={isClosed ? "opacity-70" : ""}>
                <CardContent className="p-5 space-y-4">
                  <div className="flex items-start justify-between gap-3 flex-wrap">
                    <div className="flex-1 min-w-0 space-y-2">
                      <div className="flex items-center gap-2 flex-wrap">
                        <Badge variant="outline" className={`text-[10px] gap-1 ${s.className}`}>
                          <SI className="h-3 w-3" />{t(s.labelKey as any)}
                        </Badge>
                        <Badge variant="outline" className={`text-[10px] ${p.className}`}>
                          {t(p.labelKey as any)}
                        </Badge>
                        {typeLabel && <Badge variant="secondary" className="text-[10px]">{typeLabel}</Badge>}
                        <Badge variant="secondary" className="text-[10px] font-mono">#{ticket.id}</Badge>
                      </div>
                      <h3 className="font-bold text-base">{ticket.title}</h3>
                    </div>
                    <div className="flex gap-1 shrink-0">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7"
                        title={ticket.notify_telegram ? t("tickets.notifyOn") : t("tickets.notifyOff")}
                        onClick={async () => {
                          try {
                            const res = await api.toggleTicketNotify(ticket.id, !ticket.notify_telegram);
                            if (res.success) await reload();
                            else toast({ title: res.message, variant: "destructive" });
                          } catch (err: any) {
                            toast({ title: err?.message || t("common.networkError"), variant: "destructive" });
                          }
                        }}
                      >
                        {ticket.notify_telegram ? <Bell className="h-3.5 w-3.5 text-info" /> : <BellOff className="h-3.5 w-3.5 text-muted-foreground" />}
                      </Button>
                      {!isClosed && (
                        <Button variant="ghost" size="sm" className="h-8 text-xs text-muted-foreground hover:text-destructive" onClick={() => handleClose(ticket.id)}>
                          <Archive className="h-3.5 w-3.5 mr-1" />{t("tickets.closeTicket")}
                        </Button>
                      )}
                      {isClosed && (
                        <Button variant="ghost" size="sm" className="h-8 text-xs" onClick={() => handleReopen(ticket.id)}>
                          <RotateCcw className="h-3.5 w-3.5 mr-1" />{t("tickets.reopenTicket")}
                        </Button>
                      )}
                    </div>
                  </div>

                  <div className="rounded-lg bg-muted/30 p-4 text-sm whitespace-pre-wrap break-words border border-border/50">
                    {ticket.content}
                  </div>

                  <TicketImages
                    ticketId={ticket.id}
                    attachments={ticket.attachments || []}
                    editable={!isClosed}
                    maxSize={imageMaxSize}
                    maxCount={imageMaxCount}
                    onChange={() => void reload()}
                  />

                  {((ticket.replies && ticket.replies.length > 0) || ticket.admin_note) && (
                    <div className="rounded-lg border border-border/60 bg-muted/20 p-4 space-y-3">
                      <div className="flex items-center gap-2 text-xs font-semibold">
                        <MessageSquareMore className="h-3.5 w-3.5 text-info" />
                        {t("tickets.conversation")}
                      </div>
                      {ticket.replies && ticket.replies.length > 0 ? (
                        <div className="space-y-3">
                          {ticket.replies.map((reply, index) => {
                            const isAdminReply = reply.author === "admin" || reply.role === 0;
                            return (
                              <div key={`${reply.created_at}-${reply.uid}-${index}`} className={`rounded-md border p-3 ${isAdminReply ? "border-info/20 bg-info/5" : "border-border bg-background/70"}`}>
                                <div className="mb-1 flex items-center gap-2 text-[11px] text-muted-foreground">
                                  <span className={isAdminReply ? "font-semibold text-info" : "font-semibold text-foreground"}>
                                    {isAdminReply ? t("tickets.adminReply") : t("tickets.userReply")}
                                  </span>
                                  <span>{reply.username}</span>
                                  <span className="ml-auto">{new Date(reply.created_at * 1000).toLocaleString()}</span>
                                </div>
                                <p className="whitespace-pre-wrap break-words text-sm">{reply.content}</p>
                              </div>
                            );
                          })}
                        </div>
                      ) : (
                        <div className="rounded-md border border-info/20 bg-info/5 p-3">
                          <div className="mb-1 flex items-center gap-2 text-[11px] text-muted-foreground">
                            <span className="font-semibold text-info">{t("tickets.adminReply")}</span>
                            <span className="ml-auto">{t("tickets.updatedAt", { time: new Date(ticket.updated_at * 1000).toLocaleString() })}</span>
                          </div>
                          <p className="whitespace-pre-wrap break-words text-sm">{ticket.admin_note}</p>
                        </div>
                      )}
                    </div>
                  )}

                  {!isClosed && (
                    <div className="space-y-2 rounded-lg border border-border/60 bg-background p-3">
                      <Textarea
                        value={replyDrafts[ticket.id] || ""}
                        onChange={(event) => setReplyDrafts((current) => ({ ...current, [ticket.id]: event.target.value }))}
                        placeholder={t("tickets.replyPlaceholder")}
                        rows={3}
                        maxLength={5000}
                        className="resize-y"
                      />
                      <div className="flex justify-end">
                        <Button size="sm" onClick={() => void handleReply(ticket.id)} disabled={replyingId === ticket.id}>
                          {replyingId === ticket.id ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : <Send className="mr-1.5 h-3.5 w-3.5" />}
                          {t("tickets.replySubmit")}
                        </Button>
                      </div>
                    </div>
                  )}

                  <div className="flex items-center gap-3 text-[11px] text-muted-foreground flex-wrap">
                    <span className="flex items-center gap-1">
                      <Clock className="h-3 w-3" />
                      {t("tickets.createdAt", { time: new Date(ticket.created_at * 1000).toLocaleString() })}
                    </span>
                    {ticket.resolved_at && ticket.resolved_at > 0 && (
                      <span className="flex items-center gap-1 text-success">
                        <CheckCircle2 className="h-3 w-3" />
                        {t("tickets.resolvedAt", { time: new Date(ticket.resolved_at * 1000).toLocaleString() })}
                      </span>
                    )}
                    {isClosed && ticket.closed_at && ticket.closed_at > 0 && (
                      <span className="flex items-center gap-1">
                        <Archive className="h-3 w-3" />
                        {t("tickets.closedAt", { time: new Date(ticket.closed_at * 1000).toLocaleString() })}
                      </span>
                    )}
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      {/* Create Dialog — unchanged */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("tickets.createTitle")}</DialogTitle>
            <DialogDescription>{t("tickets.createDescription")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2"><Label>{t("tickets.title")}</Label>
              <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder={t("tickets.titlePlaceholder")} maxLength={200} />
            </div>
            <div className="space-y-2"><Label>{t("tickets.content")}</Label>
              <Textarea value={content} onChange={(e) => setContent(e.target.value)} placeholder={t("tickets.contentPlaceholder")} rows={5} maxLength={10000} className="resize-y" />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2"><Label>{t("tickets.type")}</Label>
                <Select value={ticketType} onValueChange={setTicketType}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {types.map((tp: string) => <SelectItem key={tp} value={tp}>{typeLabelFor(tp)}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2"><Label>{t("tickets.priority")}</Label>
                <Select value={priority} onValueChange={setPriority}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {Object.entries(PRIORITY_MAP).map(([v, o]) => <SelectItem key={v} value={v}>{t(o.labelKey as any)}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="flex items-center justify-between rounded-lg border px-3 py-2">
              <div className="space-y-0.5">
                <Label className="text-sm">{t("tickets.notifyTelegram")}</Label>
                <p className="text-xs text-muted-foreground">{t("tickets.notifyTelegramDesc")}</p>
              </div>
              <Switch
                checked={notifyTelegram}
                onCheckedChange={setNotifyTelegram}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>{t("common.cancel")}</Button>
            <Button onClick={handleCreate} disabled={saving}>{saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}{t("tickets.submit")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}
