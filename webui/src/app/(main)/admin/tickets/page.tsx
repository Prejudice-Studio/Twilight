"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { motion } from "framer-motion";
import {
  MessageSquareMore, Loader2, Trash2, Edit2, AlertCircle, Clock, User,
  CheckCircle2, Archive, RotateCcw, PlayCircle, Plus, Pencil, Settings2, RefreshCw,
  ArrowRight,
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
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { api, type Ticket } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { useSystemStore } from "@/store/system";
import { TicketImages } from "@/components/ticket-images";

const DEFAULT_TICKET_IMAGE_MAX_SIZE = 5 * 1024 * 1024;
const DEFAULT_TICKET_IMAGE_MAX_COUNT = 5;

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

const DEFAULT_TYPES = [
  { value: "all", labelKey: "tickets.typeAll" },
];

export default function AdminTicketsPage() {
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const { t } = useI18n();
  const router = useRouter();
  const { info: systemInfo } = useSystemStore();
  const imageMaxSize = Number(systemInfo?.limits?.ticket_image_max_size) || DEFAULT_TICKET_IMAGE_MAX_SIZE;
  const imageMaxCount = Number(systemInfo?.limits?.ticket_image_max_count) || DEFAULT_TICKET_IMAGE_MAX_COUNT;

  const [statusFilter, setStatusFilter] = useState("all");
  const [typeFilter, setTypeFilter] = useState("all");
  const [priorityFilter, setPriorityFilter] = useState("all");
  const [jumpId, setJumpId] = useState("");

  const [editOpen, setEditOpen] = useState(false);
  const [editingTicket, setEditingTicket] = useState<Ticket | null>(null);
  const [editStatus, setEditStatus] = useState("");
  const [editPriority, setEditPriority] = useState("");
  const [editType, setEditType] = useState("");
  const [editNote, setEditNote] = useState("");
  const [saving, setSaving] = useState(false);

  // 工单类型管理
  const [typeMgmtOpen, setTypeMgmtOpen] = useState(false);
  const [typeMgmtTypes, setTypeMgmtTypes] = useState<string[]>([]);
  const [newTypeName, setNewTypeName] = useState("");
  const [editingTypeName, setEditingTypeName] = useState<string | null>(null);
  const [editTypeValue, setEditTypeValue] = useState("");
  const [typeMgmtSaving, setTypeMgmtSaving] = useState(false);

  const refreshTypeMgmtTypes = async () => {
    try {
      const res = await api.adminGetTicketTypes();
      if (res.success && res.data) setTypeMgmtTypes(res.data.types);
    } catch { /* ignore */ }
  };

  const handleAddType = async () => {
    const name = newTypeName.trim();
    if (!name) return;
    setTypeMgmtSaving(true);
    try {
      const res = await api.adminAddTicketType(name);
      if (res.success) { toast({ title: t("adminTickets.typeAdded") }); setNewTypeName(""); await reload(); await refreshTypeMgmtTypes(); }
      else toast({ title: res.message, variant: "destructive" });
    } catch (err: any) { toast({ title: err?.message || t("common.networkError"), variant: "destructive" }); }
    finally { setTypeMgmtSaving(false); }
  };

  const handleDeleteType = async (name: string) => {
    const ok = await confirm({ title: t("adminTickets.deleteTypeTitle"), description: t("adminTickets.deleteTypeDesc", { name }), tone: "danger", confirmLabel: t("common.delete") });
    if (!ok) return;
    try {
      const res = await api.adminDeleteTicketType(name);
      if (res.success) { toast({ title: t("adminTickets.typeDeleted") }); await reload(); await refreshTypeMgmtTypes(); }
      else toast({ title: res.message, variant: "destructive" });
    } catch (err: any) { toast({ title: err?.message || t("common.networkError"), variant: "destructive" }); }
  };

  const handleRenameType = async () => {
    if (!editingTypeName || !editTypeValue.trim()) return;
    setTypeMgmtSaving(true);
    try {
      const res = await api.adminRenameTicketType(editingTypeName, editTypeValue.trim());
      if (res.success) { toast({ title: t("adminTickets.typeRenamed") }); setEditingTypeName(null); setEditTypeValue(""); await reload(); await refreshTypeMgmtTypes(); }
      else toast({ title: res.message, variant: "destructive" });
    } catch (err: any) { toast({ title: err?.message || t("common.networkError"), variant: "destructive" }); }
    finally { setTypeMgmtSaving(false); }
  };

  const loadTickets = useCallback(async () => {
    const res = await api.adminListTickets({
      all: statusFilter === "all",
      status: statusFilter !== "all" ? statusFilter : undefined,
      type: typeFilter !== "all" ? typeFilter : undefined,
      priority: priorityFilter !== "all" ? priorityFilter : undefined,
    });
    if (res.success && res.data) return { tickets: res.data.tickets, types: res.data.ticket_types || [] };
    throw new Error(res.message || t("common.networkError"));
  }, [statusFilter, typeFilter, priorityFilter, t]);

  const { data, isLoading, error, execute: reload } = useAsyncResource(loadTickets, { immediate: true });

  const openEdit = (ticket: Ticket) => { setEditingTicket(ticket); setEditStatus(ticket.status); setEditPriority(ticket.priority); setEditType(ticket.type); setEditNote(ticket.admin_note || ""); setEditOpen(true); };
  const openConversation = (id: number) => router.push(`/admin/tickets/${id}`);

  const handleJump = () => {
    const id = Number(jumpId.trim());
    if (!Number.isInteger(id) || id <= 0) {
      toast({ title: t("adminTickets.invalidTicketId"), variant: "destructive" });
      return;
    }
    openConversation(id);
  };

  const handleSave = async () => {
    if (!editingTicket) return;
    setSaving(true);
    try {
      const res = await api.adminUpdateTicket(editingTicket.id, { status: editStatus, priority: editPriority, type: editType, admin_note: editNote.trim() });
      if (res.success) { toast({ title: t("adminTickets.updated") }); setEditOpen(false); await reload(); }
      else toast({ title: res.message, variant: "destructive" });
    } catch (err: any) { toast({ title: err?.message || t("common.networkError"), variant: "destructive" }); }
    finally { setSaving(false); }
  };

  const quickStatus = async (ticket: Ticket, status: string) => {
    try {
      const res = await api.adminUpdateTicket(ticket.id, { status, admin_note: ticket.admin_note || "" });
      if (res.success) { toast({ title: t("adminTickets.updated") }); await reload(); }
      else toast({ title: res.message, variant: "destructive" });
    } catch (err: any) { toast({ title: err?.message || t("common.networkError"), variant: "destructive" }); }
  };

  const handleDelete = async (id: number) => {
    const ok = await confirm({ title: t("adminTickets.deleteConfirmTitle"), description: t("adminTickets.deleteConfirmDescription"), tone: "danger", confirmLabel: t("common.delete") });
    if (!ok) return;
    try { const res = await api.adminDeleteTicket(id); if (res.success) { toast({ title: t("adminTickets.deleted") }); await reload(); } else toast({ title: res.message, variant: "destructive" }); }
    catch (err: any) { toast({ title: err?.message || t("common.networkError"), variant: "destructive" }); }
  };

  const types = Array.isArray(data?.types) && data.types.length ? data.types : DEFAULT_TYPES.map((d) => d.value);
  const tickets = Array.isArray(data?.tickets) ? data.tickets : [];
  const typeLabelFor = (value: string) => {
    const known = DEFAULT_TYPES.find((d) => d.value === value);
    return known ? t(known.labelKey as any) : value;
  };

  return (
    <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold flex items-center gap-2"><MessageSquareMore className="h-5 w-5" />{t("adminTickets.title")}</h1>
        <p className="text-sm text-muted-foreground mt-1">{t("adminTickets.description")}</p>
      </div>

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-[repeat(3,minmax(0,12rem))_minmax(13rem,18rem)_1fr_auto_auto] xl:items-end">
        <Select value={statusFilter} onValueChange={setStatusFilter}>
          <SelectTrigger className="w-full"><SelectValue placeholder={t("adminTickets.filterAll")} /></SelectTrigger>
          <SelectContent>{Object.entries(STATUS_MAP).map(([v, s]) => <SelectItem key={v} value={v}>{t(s.labelKey as any)}</SelectItem>)}<SelectItem value="all">{t("adminTickets.filterAll")}</SelectItem></SelectContent>
        </Select>
        <Select value={typeFilter} onValueChange={setTypeFilter}>
          <SelectTrigger className="w-full"><SelectValue placeholder={t("adminTickets.filterAllTypes")} /></SelectTrigger>
          <SelectContent><SelectItem value="all">{t("adminTickets.filterAllTypes")}</SelectItem>
            {types.map((tp: string) => <SelectItem key={tp} value={tp}>{typeLabelFor(tp)}</SelectItem>)}
          </SelectContent>
        </Select>
        <Select value={priorityFilter} onValueChange={setPriorityFilter}>
          <SelectTrigger className="w-full"><SelectValue placeholder={t("adminTickets.filterAllPriorities")} /></SelectTrigger>
          <SelectContent><SelectItem value="all">{t("adminTickets.filterAllPriorities")}</SelectItem>
            {Object.entries(PRIORITY_MAP).map(([v, p]) => <SelectItem key={v} value={v}>{t(p.labelKey as any)}</SelectItem>)}
          </SelectContent>
        </Select>
        <form
          className="flex min-w-0 gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            handleJump();
          }}
        >
          <Input
            value={jumpId}
            onChange={(event) => setJumpId(event.target.value)}
            inputMode="numeric"
            placeholder={t("adminTickets.jumpPlaceholder")}
            className="min-w-0"
          />
          <Button type="submit" variant="outline" size="icon" className="h-10 w-10 shrink-0" title={t("adminTickets.jump")}>
            <ArrowRight className="h-4 w-4" />
          </Button>
        </form>
        <span className="text-xs text-muted-foreground xl:ml-auto">{t("adminTickets.total", { count: data?.tickets?.length ?? 0 })}</span>
        <Button variant="outline" size="sm" onClick={async () => {
          try {
            const res = await api.adminGetTicketTypes();
            setTypeMgmtTypes(res.success && res.data ? res.data.types : []);
          } catch { setTypeMgmtTypes([]); }
          setTypeMgmtOpen(true);
        }} className="w-full sm:w-auto"><Settings2 className="mr-1 h-3.5 w-3.5" />{t("adminTickets.manageTypes")}</Button>
        <Button variant="outline" size="sm" onClick={() => void reload()} disabled={isLoading} className="w-full sm:w-auto">
          <RefreshCw className={`mr-1 h-3.5 w-3.5 ${isLoading ? "animate-spin" : ""}`} />{t("common.refresh")}
        </Button>
      </div>

      {error ? (
        <Card className="border-destructive/40"><CardContent className="p-6 text-center space-y-3"><AlertCircle className="h-8 w-8 mx-auto text-destructive" /><p className="text-sm">{error}</p><Button variant="outline" size="sm" onClick={() => void reload()}>{t("common.retry")}</Button></CardContent></Card>
      ) : isLoading && !data ? (
        <Card className="border-dashed"><CardContent className="p-8 text-center"><Loader2 className="h-6 w-6 mx-auto animate-spin text-muted-foreground" /></CardContent></Card>
      ) : tickets.length === 0 ? (
        <Card className="border-dashed"><CardContent className="p-8 text-center"><MessageSquareMore className="h-10 w-10 mx-auto text-muted-foreground mb-2 opacity-40" /><p className="font-medium">{t("adminTickets.noTickets")}</p><p className="text-xs text-muted-foreground mt-1">{t("adminTickets.noTicketsHint")}</p></CardContent></Card>
      ) : (
        <div className="space-y-4">
          {tickets.map((ticket: Ticket) => {
            const s = STATUS_MAP[ticket.status] || STATUS_MAP.open;
            const p = PRIORITY_MAP[ticket.priority] || PRIORITY_MAP.medium;
            const typeLabel = ticket.type ? typeLabelFor(ticket.type) : "";
            const SI = s.icon;
            const isClosed = ticket.status === "closed";
            return (
              <Card key={ticket.id} className={isClosed ? "opacity-70" : ""}>
                <CardContent className="p-5 space-y-4">
                  <div className="flex items-start justify-between gap-3 flex-wrap">
                    <div className="flex-1 min-w-0 space-y-2">
                      <div className="flex items-center gap-2 flex-wrap">
                        <Badge variant="outline" className={`text-[10px] gap-1 ${s.className}`}><SI className="h-3 w-3" />{t(s.labelKey as any)}</Badge>
                        <Badge variant="outline" className={`text-[10px] ${p.className}`}>{t(p.labelKey as any)}</Badge>
                        {typeLabel && <Badge variant="secondary" className="text-[10px]">{typeLabel}</Badge>}
                        <Badge variant="secondary" className="text-[10px] font-mono">#{ticket.id}</Badge>
                      </div>
                      <p className="text-xs text-muted-foreground flex items-center gap-1"><User className="h-3 w-3" />{ticket.username} (UID: {ticket.uid})</p>
                      <button className="block min-w-0 text-left" onClick={() => openConversation(ticket.id)}>
                        <h3 className="font-bold text-base hover:underline">{ticket.title}</h3>
                      </button>
                    </div>
                    <div className="flex gap-1 shrink-0 flex-wrap">
                      <Button variant="default" size="sm" className="h-8 text-xs" onClick={() => openConversation(ticket.id)}>
                        <MessageSquareMore className="h-3.5 w-3.5 mr-1" />{t("adminTickets.openConversation")}
                      </Button>
                      {ticket.status === "open" && (
                        <Button variant="outline" size="sm" className="h-8 text-xs" onClick={() => quickStatus(ticket, "in_progress")}><PlayCircle className="h-3.5 w-3.5 mr-1" />{t("adminTickets.markInProgress")}</Button>
                      )}
                      {(ticket.status === "open" || ticket.status === "in_progress") && (
                        <Button variant="outline" size="sm" className="h-8 text-xs" onClick={() => quickStatus(ticket, "resolved")}><CheckCircle2 className="h-3.5 w-3.5 mr-1" />{t("adminTickets.markResolved")}</Button>
                      )}
                      {ticket.status !== "closed" && (
                        <Button variant="ghost" size="sm" className="h-8 text-xs text-muted-foreground" onClick={() => quickStatus(ticket, "closed")}><Archive className="h-3.5 w-3.5 mr-1" />{t("adminTickets.closeTicket")}</Button>
                      )}
                      {ticket.status === "closed" && (
                        <Button variant="ghost" size="sm" className="h-8 text-xs" onClick={() => quickStatus(ticket, "open")}><RotateCcw className="h-3.5 w-3.5 mr-1" />{t("adminTickets.reopenTicket")}</Button>
                      )}
                      <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => openEdit(ticket)}><Edit2 className="h-4 w-4" /></Button>
                      <Button variant="ghost" size="icon" className="h-8 w-8 text-destructive hover:text-destructive" onClick={() => handleDelete(ticket.id)}><Trash2 className="h-4 w-4" /></Button>
                    </div>
                  </div>

                  <div className="rounded-lg bg-muted/30 p-4 text-sm whitespace-pre-wrap break-words border border-border/50">
                    {ticket.content}
                  </div>

                  <TicketImages
                    ticketId={ticket.id}
                    attachments={ticket.attachments || []}
                    editable={!isClosed}
                    canDelete
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
                          </div>
                          <p className="whitespace-pre-wrap break-words text-sm">{ticket.admin_note}</p>
                        </div>
                      )}
                    </div>
                  )}

                  <div className="flex items-center gap-3 text-[11px] text-muted-foreground flex-wrap">
                    <span className="flex items-center gap-1"><Clock className="h-3 w-3" />{new Date(ticket.created_at * 1000).toLocaleString()}</span>
                    {ticket.updated_at !== ticket.created_at && (<span>· {new Date(ticket.updated_at * 1000).toLocaleString()}</span>)}
                    {ticket.resolved_at && ticket.resolved_at > 0 && <span className="text-success">· {t("tickets.resolvedAt", { time: new Date(ticket.resolved_at * 1000).toLocaleString() })}</span>}
                    {isClosed && ticket.closed_at && ticket.closed_at > 0 && <span>· {t("tickets.closedAt", { time: new Date(ticket.closed_at * 1000).toLocaleString() })}</span>}
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      {/* Edit Dialog */}
      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("adminTickets.editTitle")}</DialogTitle>
            <DialogDescription>
              <span className="block font-medium text-sm text-foreground">{editingTicket?.title}</span>
              <span className="text-xs text-muted-foreground">{editingTicket?.username} · #{editingTicket?.id}</span>
            </DialogDescription>
          </DialogHeader>
          {editingTicket && (
            <div className="space-y-4">
              <div className="rounded-lg bg-muted/30 p-3 text-sm whitespace-pre-wrap break-words border max-h-32 overflow-y-auto">
                {editingTicket.content}
              </div>
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                <div className="space-y-2"><Label>{t("adminTickets.changeStatus")}</Label>
                  <Select value={editStatus} onValueChange={setEditStatus}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>{Object.entries(STATUS_MAP).map(([v, st]) => <SelectItem key={v} value={v}>{t(st.labelKey as any)}</SelectItem>)}</SelectContent>
                  </Select>
                </div>
                <div className="space-y-2"><Label>{t("tickets.priority")}</Label>
                  <Select value={editPriority} onValueChange={setEditPriority}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>{Object.entries(PRIORITY_MAP).map(([v, pr]) => <SelectItem key={v} value={v}>{t(pr.labelKey as any)}</SelectItem>)}</SelectContent>
                  </Select>
                </div>
                <div className="space-y-2"><Label>{t("tickets.type")}</Label>
                  <Select value={editType} onValueChange={setEditType}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>{types.map((tp: string) => <SelectItem key={tp} value={tp}>{typeLabelFor(tp)}</SelectItem>)}</SelectContent>
                  </Select>
                </div>
              </div>
              <div className="space-y-2">
                <Label className="flex items-center gap-1"><MessageSquareMore className="h-3.5 w-3.5" />{t("tickets.adminReply")}</Label>
                <Textarea value={editNote} onChange={(e) => setEditNote(e.target.value)} placeholder={t("adminTickets.adminNotePlaceholder")} rows={4} maxLength={5000} className="resize-y" />
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditOpen(false)}>{t("common.cancel")}</Button>
            <Button onClick={handleSave} disabled={saving}>{saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}{t("adminTickets.save")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 工单类型管理 */}
      <Dialog open={typeMgmtOpen} onOpenChange={setTypeMgmtOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{t("adminTickets.manageTypes")}</DialogTitle>
            <DialogDescription>{t("adminTickets.manageTypesDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            {/* 添加新类型 */}
            <div className="flex gap-2">
              <Input
                value={newTypeName}
                onChange={(e) => setNewTypeName(e.target.value)}
                placeholder={t("adminTickets.newTypePlaceholder")}
                maxLength={50}
                className="flex-1"
                onKeyDown={(e) => { if (e.key === "Enter") void handleAddType(); }}
              />
              <Button size="sm" onClick={() => void handleAddType()} disabled={typeMgmtSaving || !newTypeName.trim()}>
                <Plus className="mr-1 h-3.5 w-3.5" />{t("adminTickets.addType")}
              </Button>
            </div>
            {/* 已有类型列表 */}
            <div className="max-h-60 space-y-1 overflow-y-auto">
              {typeMgmtTypes.map((tp: string) => (
                <div key={tp} className="flex items-center gap-2 rounded-md border px-3 py-2">
                  {editingTypeName === tp ? (
                    <>
                      <Input
                        value={editTypeValue}
                        onChange={(e) => setEditTypeValue(e.target.value)}
                        maxLength={50}
                        className="flex-1 h-8 text-sm"
                        onKeyDown={(e) => { if (e.key === "Enter") void handleRenameType(); if (e.key === "Escape") setEditingTypeName(null); }}
                        autoFocus
                      />
                      <Button size="sm" variant="ghost" className="h-8 px-2" onClick={() => void handleRenameType()} disabled={typeMgmtSaving}>
                        {t("common.save")}
                      </Button>
                      <Button size="sm" variant="ghost" className="h-8 px-2" onClick={() => setEditingTypeName(null)}>
                        {t("common.cancel")}
                      </Button>
                    </>
                  ) : (
                    <>
                      <span className="flex-1 text-sm">{DEFAULT_TYPES.find((d) => d.value === tp) ? t(DEFAULT_TYPES.find((d) => d.value === tp)!.labelKey as any) : tp}</span>
                      <Button
                        size="icon" variant="ghost" className="h-7 w-7"
                        onClick={() => { setEditingTypeName(tp); setEditTypeValue(tp); }}
                        title="Rename"
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        size="icon" variant="ghost" className="h-7 w-7 text-destructive hover:text-destructive"
                        onClick={() => void handleDeleteType(tp)}
                        disabled={typeMgmtTypes.length <= 1}
                        title={typeMgmtTypes.length <= 1 ? "Cannot delete last type" : "Delete"}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </>
                  )}
                </div>
              ))}
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setTypeMgmtOpen(false)}>{t("common.close")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}
