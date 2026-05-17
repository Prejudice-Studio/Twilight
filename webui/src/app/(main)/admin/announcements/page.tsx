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

type Level = Announcement["level"];

const RENDER_OPTIONS: Array<{ value: AnnouncementRenderMode; label: string; hint: string }> = [
  { value: "plain", label: "纯文本", hint: "保留换行，原样展示，所有标签都按字面显示" },
  { value: "markdown", label: "Markdown", hint: "支持 # 标题、**粗体**、`代码`、列表、引用、[链接](URL) 等" },
  { value: "bbcode", label: "BBCode", hint: "支持 [b][i][u][code][quote][url=...][color=#fff][size=14] 等白名单标签" },
];

const LEVEL_OPTIONS: Array<{ value: Level; label: string }> = [
  { value: "info", label: "通知 (info)" },
  { value: "notice", label: "公告 (notice)" },
  { value: "warning", label: "注意 (warning)" },
  { value: "critical", label: "重要 (critical)" },
];

const LEVEL_BADGES: Record<Level, { className: string; icon: typeof Info; label: string }> = {
  info: { className: "bg-blue-500/10 text-blue-600 border-blue-500/30 dark:text-blue-300", icon: Info, label: "通知" },
  notice: { className: "bg-emerald-500/10 text-emerald-600 border-emerald-500/30 dark:text-emerald-300", icon: Megaphone, label: "公告" },
  warning: { className: "bg-amber-500/15 text-amber-600 border-amber-500/35 dark:text-amber-300", icon: AlertTriangle, label: "注意" },
  critical: { className: "bg-destructive/15 text-destructive border-destructive/40", icon: AlertOctagon, label: "重要" },
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

function formatTime(unix: number): string {
  if (!unix) return "";
  return new Date(unix * 1000).toLocaleString("zh-CN");
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
      throw new Error(res.message || "加载公告失败");
    }
    return true;
  }, [page, includeInvisible, includeExpired]);

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
      toast({ title: "请填写公告内容", variant: "destructive" });
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
        toast({ title: editingId ? "公告已更新" : "公告已发布" });
        setCreateOpen(false);
        await reload();
      } else {
        toast({ title: "保存失败", description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({
        title: "保存失败",
        description: err instanceof Error ? err.message : "请求异常",
        variant: "destructive",
      });
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: number) => {
    const ok = await confirm({
      title: "删除公告？",
      description: "公告会被永久删除，且无法恢复。",
      tone: "danger",
      confirmLabel: "删除",
    });
    if (!ok) return;
    try {
      const res = await api.adminDeleteAnnouncement(id);
      if (res.success) {
        toast({ title: "公告已删除" });
        await reload();
      } else {
        toast({ title: "删除失败", description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({
        title: "删除失败",
        description: err instanceof Error ? err.message : "请求异常",
        variant: "destructive",
      });
    }
  };

  const toggleVisible = async (ann: Announcement) => {
    try {
      const res = await api.adminUpdateAnnouncement(ann.id, { visible: !ann.visible });
      if (res.success) {
        toast({ title: ann.visible ? "已隐藏" : "已显示" });
        await reload();
      } else {
        toast({ title: "操作失败", description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({
        title: "操作失败",
        description: err instanceof Error ? err.message : "请求异常",
        variant: "destructive",
      });
    }
  };

  const togglePinned = async (ann: Announcement) => {
    try {
      const res = await api.adminUpdateAnnouncement(ann.id, { pinned: !ann.pinned });
      if (res.success) {
        toast({ title: ann.pinned ? "已取消置顶" : "已置顶" });
        await reload();
      } else {
        toast({ title: "操作失败", description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({
        title: "操作失败",
        description: err instanceof Error ? err.message : "请求异常",
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
            公告管理
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            发布全站公告，可设置级别、置顶、过期时间。隐藏的公告不会显示给终端用户。
          </p>
        </div>
        <Button onClick={openCreate} size="sm">
          <Plus className="h-4 w-4 mr-1" />
          新建公告
        </Button>
      </div>

      <div className="flex items-center gap-4 text-xs flex-wrap">
        <label className="flex items-center gap-2 cursor-pointer">
          <Switch checked={includeInvisible} onCheckedChange={setIncludeInvisible} />
          <span>显示已隐藏</span>
        </label>
        <label className="flex items-center gap-2 cursor-pointer">
          <Switch checked={includeExpired} onCheckedChange={setIncludeExpired} />
          <span>显示已过期</span>
        </label>
        <span className="text-muted-foreground ml-auto">共 {total} 条</span>
      </div>

      {error ? (
        <Card className="border-destructive/40">
          <CardContent className="p-6 text-center space-y-3">
            <AlertTriangle className="h-8 w-8 mx-auto text-destructive" />
            <p className="text-sm">{error}</p>
            <Button variant="outline" size="sm" onClick={() => void reload()}>
              重试
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
            <p className="font-medium">暂无公告</p>
            <p className="text-xs text-muted-foreground mt-1">
              点击右上角"新建公告"发布第一条公告
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
                          {levelStyle.label}
                        </Badge>
                        {!ann.visible && (
                          <Badge variant="secondary" className="text-[10px]">
                            <EyeOff className="h-3 w-3 mr-1" />
                            已隐藏
                          </Badge>
                        )}
                        {isExpired && (
                          <Badge variant="secondary" className="text-[10px]">
                            <Clock className="h-3 w-3 mr-1" />
                            已过期
                          </Badge>
                        )}
                        {ann.title && (
                          <h3 className="font-bold text-sm">{ann.title}</h3>
                        )}
                      </div>
                      <p className="text-[11px] text-muted-foreground mt-1">
                        #{ann.id} · 发布于 {formatTime(ann.created_at)}
                        {ann.updated_at && ann.updated_at !== ann.created_at && (
                          <> · 更新于 {formatTime(ann.updated_at)}</>
                        )}
                        {ann.expires_at > 0 && (
                          <> · 截止 {formatTime(ann.expires_at)}</>
                        )}
                      </p>
                    </div>
                    <div className="flex gap-1 shrink-0">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        onClick={() => togglePinned(ann)}
                        title={ann.pinned ? "取消置顶" : "置顶"}
                      >
                        <Pin className={`h-4 w-4 ${ann.pinned ? "text-primary" : ""}`} />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        onClick={() => toggleVisible(ann)}
                        title={ann.visible ? "隐藏" : "显示"}
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
                        渲染：{ann.render_mode}
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
            上一页
          </Button>
          <span className="text-muted-foreground">
            第 {page} / {Math.ceil(total / 20)} 页
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setPage((p) => p + 1)}
            disabled={page * 20 >= total}
          >
            下一页
          </Button>
        </div>
      )}

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{editingId ? "编辑公告" : "新建公告"}</DialogTitle>
            <DialogDescription>
              公告将面向全站用户展示，可设置级别、置顶、过期时间。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>标题（可选）</Label>
              <Input
                value={form.title}
                onChange={(e) => setForm({ ...form, title: e.target.value })}
                placeholder="例如：维护通知"
                maxLength={200}
              />
            </div>
            <div className="space-y-2">
              <Label>内容</Label>
              <Textarea
                value={form.content}
                onChange={(e) => setForm({ ...form, content: e.target.value })}
                placeholder="公告正文，支持换行..."
                rows={6}
                maxLength={10000}
                className="resize-y"
              />
              <p className="text-[10px] text-muted-foreground">
                {form.content.length} / 10000 字
              </p>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div className="space-y-2">
                <Label>级别</Label>
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
                        {opt.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>渲染方式</Label>
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
                        {opt.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-[10px] text-muted-foreground">
                  {RENDER_OPTIONS.find((o) => o.value === form.renderMode)?.hint}
                </p>
              </div>
              <div className="space-y-2">
                <Label>截止时间（留空 = 永久）</Label>
                <Input
                  type="datetime-local"
                  value={form.expiresAtLocal}
                  onChange={(e) => setForm({ ...form, expiresAtLocal: e.target.value })}
                />
              </div>
            </div>
            {form.content.trim() && (
              <div className="space-y-1">
                <Label className="text-[11px] text-muted-foreground">预览</Label>
                <div className="rounded-md border bg-muted/30 p-3 max-h-48 overflow-y-auto">
                  <SafeAnnouncementContent content={form.content} mode={form.renderMode} />
                </div>
              </div>
            )}
            <div className="flex items-center justify-between p-3 border rounded-md">
              <div>
                <p className="text-sm font-medium">置顶</p>
                <p className="text-xs text-muted-foreground">置顶公告会显示在列表最前</p>
              </div>
              <Switch
                checked={form.pinned}
                onCheckedChange={(v) => setForm({ ...form, pinned: v })}
              />
            </div>
            <div className="flex items-center justify-between p-3 border rounded-md">
              <div>
                <p className="text-sm font-medium">立即可见</p>
                <p className="text-xs text-muted-foreground">关闭则保存为草稿，不展示给用户</p>
              </div>
              <Switch
                checked={form.visible}
                onCheckedChange={(v) => setForm({ ...form, visible: v })}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              取消
            </Button>
            <Button onClick={handleSave} disabled={saving}>
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {editingId ? "保存修改" : "发布公告"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}
