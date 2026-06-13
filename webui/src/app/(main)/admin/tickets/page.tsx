"use client";

import { useState, useCallback } from "react";
import { motion } from "framer-motion";
import {
  MessageSquareMore,
  Loader2,
  Trash2,
  Edit2,
  AlertCircle,
  Clock,
  User,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
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
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { api, type Ticket } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

const STATUS_OPTIONS: Array<{ value: string; labelKey: string; className: string }> = [
  { value: "", labelKey: "adminTickets.filterAll", className: "" },
  { value: "open", labelKey: "adminTickets.filterOpen", className: "bg-warning/10 text-warning border-warning/30" },
  { value: "in_progress", labelKey: "adminTickets.filterInProgress", className: "bg-info/10 text-info border-info/30" },
  { value: "resolved", labelKey: "adminTickets.filterResolved", className: "bg-success/10 text-success border-success/30" },
  { value: "closed", labelKey: "adminTickets.filterClosed", className: "bg-muted text-muted-foreground" },
];

const PRIORITY_OPTIONS = [
  { value: "", labelKey: "adminTickets.filterAllPriorities" },
  { value: "low", labelKey: "tickets.priorityLow" },
  { value: "medium", labelKey: "tickets.priorityMedium" },
  { value: "high", labelKey: "tickets.priorityHigh" },
  { value: "urgent", labelKey: "tickets.priorityUrgent" },
];

const DEFAULT_TYPES = [
  { value: "bug", labelKey: "tickets.typeBug" },
  { value: "feature", labelKey: "tickets.typeFeature" },
  { value: "question", labelKey: "tickets.typeQuestion" },
  { value: "account", labelKey: "tickets.typeAccount" },
  { value: "other", labelKey: "tickets.typeOther" },
];

export default function AdminTicketsPage() {
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const { t } = useI18n();

  const [statusFilter, setStatusFilter] = useState("");
  const [typeFilter, setTypeFilter] = useState("");
  const [priorityFilter, setPriorityFilter] = useState("");

  const [editOpen, setEditOpen] = useState(false);
  const [editingTicket, setEditingTicket] = useState<Ticket | null>(null);
  const [editStatus, setEditStatus] = useState("");
  const [editPriority, setEditPriority] = useState("");
  const [editType, setEditType] = useState("");
  const [editNote, setEditNote] = useState("");
  const [saving, setSaving] = useState(false);

  const loadTickets = useCallback(async () => {
    const res = await api.adminListTickets({
      status: statusFilter || undefined,
      type: typeFilter || undefined,
      priority: priorityFilter || undefined,
    });
    if (res.success && res.data) {
      return { tickets: res.data.tickets, types: res.data.ticket_types || [] };
    }
    throw new Error(res.message || t("common.networkError"));
  }, [statusFilter, typeFilter, priorityFilter, t]);

  const { data, isLoading, error, execute: reload } = useAsyncResource(loadTickets, { immediate: true });

  const openEdit = (ticket: Ticket) => {
    setEditingTicket(ticket);
    setEditStatus(ticket.status);
    setEditPriority(ticket.priority);
    setEditType(ticket.type);
    setEditNote(ticket.admin_note || "");
    setEditOpen(true);
  };

  const handleSave = async () => {
    if (!editingTicket) return;
    setSaving(true);
    try {
      const res = await api.adminUpdateTicket(editingTicket.id, {
        status: editStatus,
        priority: editPriority,
        type: editType,
        admin_note: editNote.trim(),
      });
      if (res.success) {
        toast({ title: t("adminTickets.updated") });
        setEditOpen(false);
        await reload();
      } else {
        toast({ title: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({ title: err instanceof Error ? err.message : t("common.networkError"), variant: "destructive" });
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: number) => {
    const ok = await confirm({
      title: t("adminTickets.deleteConfirmTitle"),
      description: t("adminTickets.deleteConfirmDescription"),
      tone: "danger",
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    try {
      const res = await api.adminDeleteTicket(id);
      if (res.success) {
        toast({ title: t("adminTickets.deleted") });
        await reload();
      } else {
        toast({ title: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({ title: err instanceof Error ? err.message : t("common.networkError"), variant: "destructive" });
    }
  };

  const types = data?.types?.length ? data.types : DEFAULT_TYPES.map((t) => t.value);

  return (
    <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold flex items-center gap-2">
          <MessageSquareMore className="h-5 w-5" />
          {t("adminTickets.title")}
        </h1>
        <p className="text-sm text-muted-foreground mt-1">{t("adminTickets.description")}</p>
      </div>

      <div className="flex items-center gap-3 flex-wrap">
        <Select value={statusFilter} onValueChange={setStatusFilter}>
          <SelectTrigger className="w-32"><SelectValue placeholder={t("adminTickets.filterAll")} /></SelectTrigger>
          <SelectContent>
            {STATUS_OPTIONS.map((s) => (
              <SelectItem key={s.value} value={s.value}>{t(s.labelKey as any)}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={typeFilter} onValueChange={setTypeFilter}>
          <SelectTrigger className="w-32"><SelectValue placeholder={t("adminTickets.filterAllTypes")} /></SelectTrigger>
          <SelectContent>
            <SelectItem value="">{t("adminTickets.filterAllTypes")}</SelectItem>
            {DEFAULT_TYPES.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>{t(opt.labelKey as any)}</SelectItem>
            ))}
            {data?.types?.filter((tp: string) => !DEFAULT_TYPES.find((d) => d.value === tp)).map((tp: string) => (
              <SelectItem key={tp} value={tp}>{tp}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={priorityFilter} onValueChange={setPriorityFilter}>
          <SelectTrigger className="w-32"><SelectValue placeholder={t("adminTickets.filterAllPriorities")} /></SelectTrigger>
          <SelectContent>
            {PRIORITY_OPTIONS.map((p) => (
              <SelectItem key={p.value} value={p.value}>{t(p.labelKey as any)}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span className="text-xs text-muted-foreground ml-auto">
          {t("adminTickets.total", { count: data?.tickets?.length ?? 0 })}
        </span>
      </div>

      {error ? (
        <Card className="border-destructive/40">
          <CardContent className="p-6 text-center space-y-3">
            <AlertCircle className="h-8 w-8 mx-auto text-destructive" />
            <p className="text-sm">{error}</p>
            <Button variant="outline" size="sm" onClick={() => void reload()}>{t("common.retry")}</Button>
          </CardContent>
        </Card>
      ) : isLoading && !data ? (
        <Card className="border-dashed">
          <CardContent className="p-8 text-center">
            <Loader2 className="h-6 w-6 mx-auto animate-spin text-muted-foreground" />
          </CardContent>
        </Card>
      ) : !data?.tickets?.length ? (
        <Card className="border-dashed">
          <CardContent className="p-8 text-center">
            <MessageSquareMore className="h-10 w-10 mx-auto text-muted-foreground mb-2 opacity-40" />
            <p className="font-medium">{t("adminTickets.noTickets")}</p>
            <p className="text-xs text-muted-foreground mt-1">{t("adminTickets.noTicketsHint")}</p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {data.tickets.map((ticket: Ticket) => {
            const status = STATUS_OPTIONS.find((s) => s.value === ticket.status) || STATUS_OPTIONS[1];
            const typeLabel = DEFAULT_TYPES.find((dt) => dt.value === ticket.type)?.labelKey;
            return (
              <Card key={ticket.id} className={ticket.status === "closed" ? "opacity-70" : ""}>
                <CardContent className="p-4 space-y-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        <h3 className="font-bold text-sm">{ticket.title}</h3>
                        <Badge variant="outline" className={`text-[10px] ${status.className}`}>
                          {t(status.labelKey as any)}
                        </Badge>
                        <Badge variant="secondary" className="text-[10px]">
                          {t(PRIORITY_OPTIONS.find((p) => p.value === ticket.priority)?.labelKey as any || "tickets.priorityMedium")}
                        </Badge>
                        {typeLabel && <Badge variant="secondary" className="text-[10px]">{t(typeLabel as any)}</Badge>}
                      </div>
                      <p className="text-xs text-muted-foreground mt-1 flex items-center gap-1">
                        <User className="h-3 w-3" />
                        {ticket.username} (UID: {ticket.uid})
                      </p>
                      <p className="text-sm mt-2 whitespace-pre-wrap break-words">{ticket.content}</p>
                      {ticket.admin_note && (
                        <div className="mt-2 rounded-md bg-muted/60 p-3 text-sm">
                          <span className="font-medium text-xs text-muted-foreground">{t("adminTickets.adminNote")}: </span>
                          {ticket.admin_note}
                        </div>
                      )}
                      <p className="text-[11px] text-muted-foreground mt-2 flex items-center gap-1">
                        <Clock className="h-3 w-3" />
                        {new Date(ticket.created_at * 1000).toLocaleString()}
                        {ticket.updated_at !== ticket.created_at && (
                          <> · {new Date(ticket.updated_at * 1000).toLocaleString()}</>
                        )}
                      </p>
                    </div>
                    <div className="flex gap-1 shrink-0">
                      <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => openEdit(ticket)}>
                        <Edit2 className="h-4 w-4" />
                      </Button>
                      <Button variant="ghost" size="icon" className="h-8 w-8 text-destructive hover:text-destructive" onClick={() => handleDelete(ticket.id)}>
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("adminTickets.title")}</DialogTitle>
            <DialogDescription>{editingTicket?.title}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2">
                <Label>{t("adminTickets.changeStatus")}</Label>
                <Select value={editStatus} onValueChange={setEditStatus}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {STATUS_OPTIONS.filter((s) => s.value !== "").map((s) => (
                      <SelectItem key={s.value} value={s.value}>{t(s.labelKey as any)}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>{t("tickets.priority")}</Label>
                <Select value={editPriority} onValueChange={setEditPriority}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {PRIORITY_OPTIONS.filter((p) => p.value !== "").map((p) => (
                      <SelectItem key={p.value} value={p.value}>{t(p.labelKey as any)}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="space-y-2">
              <Label>{t("tickets.type")}</Label>
              <Select value={editType} onValueChange={setEditType}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {DEFAULT_TYPES.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>{t(opt.labelKey as any)}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>{t("adminTickets.adminNote")}</Label>
              <Textarea value={editNote} onChange={(e) => setEditNote(e.target.value)} placeholder={t("adminTickets.adminNotePlaceholder")} rows={3} maxLength={5000} className="resize-y" />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditOpen(false)}>{t("common.cancel")}</Button>
            <Button onClick={handleSave} disabled={saving}>
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t("adminTickets.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}
