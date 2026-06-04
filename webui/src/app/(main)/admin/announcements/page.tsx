"use client";

import { useCallback, useState } from "react";
import { motion } from "framer-motion";
import {
  Megaphone,
  Plus,
  Edit2,
  Trash2,
  Loader2,
  Pin,
  EyeOff,
  Eye,
  AlertOctagon,
  AlertTriangle,
  Info,
  Clock,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
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
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { api, type Announcement, type AnnouncementRenderMode } from "@/lib/api";
import { SafeAnnouncementContent } from "@/lib/safe-render";
import { useI18n, type MessageKey } from "@/lib/i18n";

type Level = Announcement["level"];

const RENDER_OPTIONS: Array<{ value: AnnouncementRenderMode; label: string | MessageKey; hintKey: MessageKey }> = [
  { value: "plain", label: "adminAnnouncements.renderPlain", hintKey: "adminAnnouncements.renderPlainHint" },
  { value: "markdown", label: "Markdown", hintKey: "adminAnnouncements.renderMarkdownHint" },
  { value: "bbcode", label: "BBCode", hintKey: "adminAnnouncements.renderBBCodeHint" },
];

const LEVEL_OPTIONS: Array<{ value: Level; labelKey: MessageKey }> = [
  { value: "info", labelKey: "adminAnnouncements.levelInfo" },
  { value: "notice", labelKey: "adminAnnouncements.levelNotice" },
  { value: "warning", labelKey: "adminAnnouncements.levelWarning" },
  { value: "critical", labelKey: "adminAnnouncements.levelCritical" },
];

const LEVEL_BADGES: Record<Level, { className: string; icon: typeof Info; labelKey: MessageKey }> = {
  // 使用语义令牌：bg-info/10 + text-info 等。
  info: { className: "bg-info/10 text-info border-info/30", icon: Info, labelKey: "announcements.levelInfo" },
  notice: { className: "bg-success/10 text-success border-success/30", icon: Megaphone, labelKey: "announcements.levelNotice" },
  warning: { className: "bg-warning/15 text-warning border-warning/35", icon: AlertTriangle, labelKey: "announcements.levelWarning" },
  critical: { className: "bg-destructive/15 text-destructive border-destructive/40", icon: AlertOctagon, labelKey: "announcements.levelCritical" },
};

interface FormState {
  title: string;
  content: string;
  level: Level;
  renderMode: AnnouncementRenderMode;
  pinned: boolean;
  visible: boolean;
  expiresAtLocal: string; // datetime-local input value; empty = never expires
}

const emptyForm = (): FormState => ({
  title: "",
  content: "",
  level: "info",
  renderMode: "plain",
  pinned: false,
  visible: true,
  expiresAtLocal: "",
});

function formatTime(unix: number, locale: string): string {
  if (!unix) return "";
  return new Date(unix * 1000).toLocaleString(locale);
}

function unixToLocalInput(unix: number): string {
  if (!unix || unix <= 0) return "";
  const d = new Date(unix * 1000);
  const pad = (n: number) => n.toString().padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function localInputToUnix(value: string): number {
  if (!value) return -1;
  const t = new Date(value).getTime();
  return Number.isNaN(t) ? -1 : Math.floor(t / 1000);
}

export default function AdminAnnouncementsPage() {
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const { locale, t } = useI18n();
  const [items, setItems] = useState<Announcement[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [includeInvisible, setIncludeInvisible] = useState(true);
  const [includeExpired, setIncludeExpired] = useState(true);

  const [createOpen, setCreateOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm());
  const [saving, setSaving] = useState(false);

  const loadResource = useCallback(async () => {
    const res = await api.adminListAnnouncements({
      page,
      per_page: 20,
      include_invisible: includeInvisible,
      include_expired: includeExpired,
    });
    if (res.success && res.data) {
      setItems(res.data.announcements || []);
      setTotal(res.data.total || 0);
    } else {
      throw new Error(res.message || t("adminAnnouncements.loadFailed"));
    }
    return true;
  }, [includeExpired, includeInvisible, page, t]);

  const {
    isLoading,
    error,
    execute: reload,
  } = useAsyncResource(loadResource, { immediate: true });

  const openCreate = () => {
    setEditingId(null);
    setForm(emptyForm());
    setCreateOpen(true);
  };

  const openEdit = (ann: Announcement) => {
    setEditingId(ann.id);
    setForm({
      title: ann.title || "",
      content: ann.content,
      level: ann.level,
      renderMode: (ann.render_mode as AnnouncementRenderMode) || "plain",
      pinned: ann.pinned,
      visible: ann.visible,
      expiresAtLocal: unixToLocalInput(ann.expires_at),
    });
    setCreateOpen(true);
  };

  const handleSave = async () => {
    const content = form.content.trim();
    if (!content) {
      toast({ title: t("adminAnnouncements.contentRequired"), variant: "destructive" });
      return;
    }
    setSaving(true);
    try {
      const payload = {
        title: form.title.trim() || undefined,
        content,
        level: form.level,
        render_mode: form.renderMode,
        pinned: form.pinned,
        visible: form.visible,
        expires_at: localInputToUnix(form.expiresAtLocal),
      };
      const res = editingId
        ? await api.adminUpdateAnnouncement(editingId, payload)
        : await api.adminCreateAnnouncement(payload);
      if (res.success) {
        toast({ title: editingId ? t("adminAnnouncements.updated") : t("adminAnnouncements.published") });
        setCreateOpen(false);
        await reload();
      } else {
        toast({ title: t("adminConfig.saveFailureTitle"), description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({
        title: t("adminConfig.saveFailureTitle"),
        description: err instanceof Error ? err.message : t("adminAnnouncements.requestError"),
        variant: "destructive",
      });
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: number) => {
    const ok = await confirm({
      title: t("adminAnnouncements.deleteConfirmTitle"),
      description: t("adminAnnouncements.deleteConfirmDescription"),
      tone: "danger",
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    try {
      const res = await api.adminDeleteAnnouncement(id);
      if (res.success) {
        toast({ title: t("adminAnnouncements.deleted") });
        await reload();
      } else {
        toast({ title: t("common.deleteFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({
        title: t("common.deleteFailed"),
        description: err instanceof Error ? err.message : t("adminAnnouncements.requestError"),
        variant: "destructive",
      });
    }
  };

  const toggleVisible = async (ann: Announcement) => {
    try {
      const res = await api.adminUpdateAnnouncement(ann.id, { visible: !ann.visible });
      if (res.success) {
        toast({ title: ann.visible ? t("adminAnnouncements.hidden") : t("adminAnnouncements.shown") });
        await reload();
      } else {
        toast({ title: t("common.operationFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({
        title: t("common.operationFailed"),
        description: err instanceof Error ? err.message : t("adminAnnouncements.requestError"),
        variant: "destructive",
      });
    }
  };

  const togglePinned = async (ann: Announcement) => {
    try {
      const res = await api.adminUpdateAnnouncement(ann.id, { pinned: !ann.pinned });
      if (res.success) {
        toast({ title: ann.pinned ? t("adminAnnouncements.unpinned") : t("adminAnnouncements.pinned") });
        await reload();
      } else {
        toast({ title: t("common.operationFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({
        title: t("common.operationFailed"),
        description: err instanceof Error ? err.message : t("adminAnnouncements.requestError"),
        variant: "destructive",
      });
    }
  };

  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      className="space-y-6"
    >
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Megaphone className="h-5 w-5" />
            {t("adminAnnouncements.title")}
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            {t("adminAnnouncements.description")}
          </p>
        </div>
        <Button onClick={openCreate} size="sm">
          <Plus className="h-4 w-4 mr-1" />
          {t("adminAnnouncements.new")}
        </Button>
      </div>

      <div className="flex items-center gap-4 text-xs flex-wrap">
        <label className="flex items-center gap-2 cursor-pointer">
          <Switch checked={includeInvisible} onCheckedChange={setIncludeInvisible} />
          <span>{t("adminAnnouncements.showHidden")}</span>
        </label>
        <label className="flex items-center gap-2 cursor-pointer">
          <Switch checked={includeExpired} onCheckedChange={setIncludeExpired} />
          <span>{t("adminAnnouncements.showExpired")}</span>
        </label>
        <span className="text-muted-foreground ml-auto">{t("adminAnnouncements.total", { count: total })}</span>
      </div>

      {error ? (
        <Card className="border-destructive/40">
          <CardContent className="p-6 text-center space-y-3">
            <AlertTriangle className="h-8 w-8 mx-auto text-destructive" />
            <p className="text-sm">{error}</p>
            <Button variant="outline" size="sm" onClick={() => void reload()}>
              {t("common.retry")}
            </Button>
          </CardContent>
        </Card>
      ) : isLoading && items.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="p-8 text-center">
            <Loader2 className="h-6 w-6 mx-auto animate-spin text-muted-foreground" />
          </CardContent>
        </Card>
      ) : items.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="p-8 text-center">
            <Megaphone className="h-10 w-10 mx-auto text-muted-foreground mb-2 opacity-40" />
            <p className="font-medium">{t("announcements.empty")}</p>
            <p className="text-xs text-muted-foreground mt-1">
              {t("adminAnnouncements.emptyHint")}
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {items.map((ann) => {
            const levelStyle = LEVEL_BADGES[ann.level] || LEVEL_BADGES.info;
            const LevelIcon = levelStyle.icon;
            const isExpired = ann.expires_at > 0 && ann.expires_at * 1000 < Date.now();
            return (
              <Card key={ann.id} className={!ann.visible || isExpired ? "opacity-70" : ""}>
                <CardContent className="p-4 space-y-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        {ann.pinned && (
                          <Pin className="h-3.5 w-3.5 text-primary shrink-0" />
                        )}
                        <Badge variant="outline" className={`text-[10px] ${levelStyle.className}`}>
                          <LevelIcon className="h-3 w-3 mr-1" />
                          {t(levelStyle.labelKey)}
                        </Badge>
                        {!ann.visible && (
                          <Badge variant="secondary" className="text-[10px]">
                            <EyeOff className="h-3 w-3 mr-1" />
                            {t("adminAnnouncements.hidden")}
                          </Badge>
                        )}
                        {isExpired && (
                          <Badge variant="secondary" className="text-[10px]">
                            <Clock className="h-3 w-3 mr-1" />
                            {t("dashboard.expired")}
                          </Badge>
                        )}
                        {ann.title && (
                          <h3 className="font-bold text-sm">{ann.title}</h3>
                        )}
                      </div>
                      <p className="text-[11px] text-muted-foreground mt-1">
                        {t("adminAnnouncements.publishedAt", { id: ann.id, time: formatTime(ann.created_at, locale) })}
                        {ann.updated_at && ann.updated_at !== ann.created_at && (
                          <>{t("adminAnnouncements.updatedAt", { time: formatTime(ann.updated_at, locale) })}</>
                        )}
                        {ann.expires_at > 0 && (
                          <>{t("adminAnnouncements.expiresAt", { time: formatTime(ann.expires_at, locale) })}</>
                        )}
                      </p>
                    </div>
                    <div className="flex gap-1 shrink-0">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        onClick={() => togglePinned(ann)}
                        title={ann.pinned ? t("adminAnnouncements.unpin") : t("adminAnnouncements.pin")}
                      >
                        <Pin className={`h-4 w-4 ${ann.pinned ? "text-primary" : ""}`} />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        onClick={() => toggleVisible(ann)}
                        title={ann.visible ? t("common.hide") : t("common.show")}
                      >
                        {ann.visible ? <Eye className="h-4 w-4" /> : <EyeOff className="h-4 w-4" />}
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        onClick={() => openEdit(ann)}
                      >
                        <Edit2 className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-destructive hover:text-destructive"
                        onClick={() => handleDelete(ann.id)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                  <div className="text-sm break-words bg-muted/40 rounded-md p-3">
                    <SafeAnnouncementContent content={ann.content} mode={ann.render_mode} />
                    {ann.render_mode && ann.render_mode !== "plain" && (
                      <p className="mt-2 text-[10px] text-muted-foreground/80 font-mono">
                        {t("adminAnnouncements.renderMode", { mode: ann.render_mode })}
                      </p>
                    )}
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      {total > 20 && (
        <div className="flex items-center justify-center gap-2 text-sm">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            disabled={page <= 1}
          >
            {t("common.previousPage")}
          </Button>
          <span className="text-muted-foreground">
            {t("common.pageStatus", { page, pages: Math.ceil(total / 20) })}
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setPage((p) => p + 1)}
            disabled={page * 20 >= total}
          >
            {t("common.nextPage")}
          </Button>
        </div>
      )}

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{editingId ? t("adminAnnouncements.edit") : t("adminAnnouncements.new")}</DialogTitle>
            <DialogDescription>
              {t("adminAnnouncements.dialogDescription")}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("adminAnnouncements.titleOptional")}</Label>
              <Input
                value={form.title}
                onChange={(e) => setForm({ ...form, title: e.target.value })}
                placeholder={t("adminAnnouncements.titlePlaceholder")}
                maxLength={200}
              />
            </div>
            <div className="space-y-2">
              <Label>{t("adminAnnouncements.content")}</Label>
              <Textarea
                value={form.content}
                onChange={(e) => setForm({ ...form, content: e.target.value })}
                placeholder={t("adminAnnouncements.contentPlaceholder")}
                rows={6}
                maxLength={10000}
                className="resize-y"
              />
              <p className="text-[10px] text-muted-foreground">
                {t("adminAnnouncements.charCount", { count: form.content.length })}
              </p>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div className="space-y-2">
                <Label>{t("adminAnnouncements.level")}</Label>
                <Select
                  value={form.level}
                  onValueChange={(v) => setForm({ ...form, level: v as Level })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {LEVEL_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {t(opt.labelKey)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>{t("adminAnnouncements.renderType")}</Label>
                <Select
                  value={form.renderMode}
                  onValueChange={(v) => setForm({ ...form, renderMode: v as AnnouncementRenderMode })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {RENDER_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label.includes(".") ? t(opt.label as MessageKey) : opt.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-[10px] text-muted-foreground">
                  {t(RENDER_OPTIONS.find((o) => o.value === form.renderMode)?.hintKey || "adminAnnouncements.renderPlainHint")}
                </p>
              </div>
              <div className="space-y-2">
                <Label>{t("adminAnnouncements.expires")}</Label>
                <Input
                  type="datetime-local"
                  value={form.expiresAtLocal}
                  onChange={(e) => setForm({ ...form, expiresAtLocal: e.target.value })}
                />
              </div>
            </div>
            {form.content.trim() && (
              <div className="space-y-1">
                <Label className="text-[11px] text-muted-foreground">{t("adminAnnouncements.preview")}</Label>
                <div className="rounded-md border bg-muted/30 p-3 max-h-48 overflow-y-auto">
                  <SafeAnnouncementContent content={form.content} mode={form.renderMode} />
                </div>
              </div>
            )}
            <div className="flex items-center justify-between p-3 border rounded-md">
              <div>
                <p className="text-sm font-medium">{t("adminAnnouncements.pin")}</p>
                <p className="text-xs text-muted-foreground">{t("adminAnnouncements.pinDescription")}</p>
              </div>
              <Switch
                checked={form.pinned}
                onCheckedChange={(v) => setForm({ ...form, pinned: v })}
              />
            </div>
            <div className="flex items-center justify-between p-3 border rounded-md">
              <div>
                <p className="text-sm font-medium">{t("adminAnnouncements.visible")}</p>
                <p className="text-xs text-muted-foreground">{t("adminAnnouncements.visibleDescription")}</p>
              </div>
              <Switch
                checked={form.visible}
                onCheckedChange={(v) => setForm({ ...form, visible: v })}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button onClick={handleSave} disabled={saving}>
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {editingId ? t("adminAnnouncements.saveChanges") : t("adminAnnouncements.publish")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}
