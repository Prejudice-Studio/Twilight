"use client";

import { useState, useCallback } from "react";
import { motion } from "framer-motion";
import {
  MessageSquareMore,
  Plus,
  Loader2,
  Clock,
  AlertCircle,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
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
import { useToast } from "@/hooks/use-toast";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { api, type Ticket } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { useSystemStore } from "@/store/system";

const STATUS_OPTIONS: Array<{ value: string; labelKey: string; className: string }> = [
  { value: "open", labelKey: "tickets.statusOpen", className: "bg-warning/10 text-warning border-warning/30" },
  { value: "in_progress", labelKey: "tickets.statusInProgress", className: "bg-info/10 text-info border-info/30" },
  { value: "resolved", labelKey: "tickets.statusResolved", className: "bg-success/10 text-success border-success/30" },
  { value: "closed", labelKey: "tickets.statusClosed", className: "bg-muted text-muted-foreground border-muted" },
];

const PRIORITY_OPTIONS = [
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

export default function UserTicketsPage() {
  const { toast } = useToast();
  const { t } = useI18n();
  const { info: systemInfo } = useSystemStore();
  const ticketEnabled = Boolean(systemInfo?.features?.ticket_system);

  const [createOpen, setCreateOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  const [ticketType, setTicketType] = useState("bug");
  const [priority, setPriority] = useState("medium");
  const [saving, setSaving] = useState(false);

  const loadTickets = useCallback(async () => {
    const res = await api.getMyTickets();
    if (res.success && res.data) {
      return { tickets: res.data.tickets, types: res.data.ticket_types || [] };
    }
    throw new Error(res.message || t("common.networkError"));
  }, [t]);

  const { data, isLoading, error, execute: reload } = useAsyncResource(loadTickets, { immediate: true });

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
      const res = await api.createTicket({ title: title.trim(), content: content.trim(), type: ticketType, priority });
      if (res.success) {
        toast({ title: t("tickets.submitted") });
        setCreateOpen(false);
        setTitle("");
        setContent("");
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

  if (!ticketEnabled) {
    return (
      <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
        <Card className="border-dashed">
          <CardContent className="p-8 text-center">
            <AlertCircle className="h-10 w-10 mx-auto text-muted-foreground mb-2 opacity-40" />
            <p className="font-medium">{t("tickets.disabled")}</p>
          </CardContent>
        </Card>
      </motion.div>
    );
  }

  const types = data?.types?.length ? data.types : DEFAULT_TYPES.map((t) => t.value);

  return (
    <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <MessageSquareMore className="h-5 w-5" />
            {t("tickets.pageTitle")}
          </h1>
          <p className="text-sm text-muted-foreground mt-1">{t("tickets.pageDescription")}</p>
        </div>
        <Button onClick={() => { setTitle(""); setContent(""); setCreateOpen(true); }} size="sm">
          <Plus className="h-4 w-4 mr-1" />
          {t("tickets.submit")}
        </Button>
      </div>

      {error ? (
        <Card className="border-destructive/40">
          <CardContent className="p-6 text-center space-y-3">
            <AlertCircle className="h-8 w-8 mx-auto text-destructive" />
            <p className="text-sm">{error}</p>
            <Button variant="outline" size="sm" onClick={() => void reload()}>{t("common.retry")}</Button>
          </CardContent>
        </Card>
      ) : isLoading ? (
        <Card className="border-dashed">
          <CardContent className="p-8 text-center">
            <Loader2 className="h-6 w-6 mx-auto animate-spin text-muted-foreground" />
          </CardContent>
        </Card>
      ) : !data?.tickets?.length ? (
        <Card className="border-dashed">
          <CardContent className="p-8 text-center">
            <MessageSquareMore className="h-10 w-10 mx-auto text-muted-foreground mb-2 opacity-40" />
            <p className="font-medium">{t("tickets.noTickets")}</p>
            <p className="text-xs text-muted-foreground mt-1">{t("tickets.noTicketsHint")}</p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {data.tickets.map((ticket: Ticket) => {
            const status = STATUS_OPTIONS.find((s) => s.value === ticket.status) || STATUS_OPTIONS[0];
            const typeLabel = DEFAULT_TYPES.find((dt) => dt.value === ticket.type)?.labelKey;
            return (
              <Card key={ticket.id}>
                <CardContent className="p-4 space-y-2">
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        <h3 className="font-bold text-sm">{ticket.title}</h3>
                        <Badge variant="outline" className={`text-[10px] ${status.className}`}>
                          {t(status.labelKey as any)}
                        </Badge>
                        <Badge variant="secondary" className="text-[10px]">{t(PRIORITY_OPTIONS.find((p) => p.value === ticket.priority)?.labelKey as any || "tickets.priorityMedium")}</Badge>
                        {typeLabel && <Badge variant="secondary" className="text-[10px]">{t(typeLabel as any)}</Badge>}
                      </div>
                      <p className="text-sm mt-2 whitespace-pre-wrap break-words">{ticket.content}</p>
                      {ticket.admin_note && (
                        <div className="mt-2 rounded-md bg-muted/60 p-3 text-sm">
                          <span className="font-medium text-xs text-muted-foreground">{t("adminTickets.adminNote")}: </span>
                          {ticket.admin_note}
                        </div>
                      )}
                      <p className="text-[11px] text-muted-foreground mt-2 flex items-center gap-1">
                        <Clock className="h-3 w-3" />
                        {t("tickets.createdAt", { time: new Date(ticket.created_at * 1000).toLocaleString() })}
                        {ticket.updated_at !== ticket.created_at && (
                          <> · {t("tickets.updatedAt", { time: new Date(ticket.updated_at * 1000).toLocaleString() })}</>
                        )}
                      </p>
                    </div>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("tickets.createTitle")}</DialogTitle>
            <DialogDescription>{t("tickets.createDescription")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("tickets.title")}</Label>
              <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder={t("tickets.titlePlaceholder")} maxLength={200} />
            </div>
            <div className="space-y-2">
              <Label>{t("tickets.content")}</Label>
              <Textarea value={content} onChange={(e) => setContent(e.target.value)} placeholder={t("tickets.contentPlaceholder")} rows={5} maxLength={10000} className="resize-y" />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2">
                <Label>{t("tickets.type")}</Label>
                <Select value={ticketType} onValueChange={setTicketType}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {DEFAULT_TYPES.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>{t(opt.labelKey as any)}</SelectItem>
                    ))}
                    {data?.types?.filter((tp: string) => !DEFAULT_TYPES.find((d) => d.value === tp)).map((tp: string) => (
                      <SelectItem key={tp} value={tp}>{tp}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>{t("tickets.priority")}</Label>
                <Select value={priority} onValueChange={setPriority}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {PRIORITY_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>{t(opt.labelKey as any)}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>{t("common.cancel")}</Button>
            <Button onClick={handleCreate} disabled={saving}>
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t("tickets.submit")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}
