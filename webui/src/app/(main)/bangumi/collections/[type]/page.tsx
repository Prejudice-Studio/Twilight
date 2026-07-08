"use client";

import { useCallback, useMemo, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { motion } from "framer-motion";
import { ArrowLeft, BookOpen, CalendarDays, Edit, ExternalLink, Loader2, RefreshCw, Star } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";

const PAGE_SIZE = 24;

const COLLECTION_META: Record<number, { title: string; badge: string }> = {
  1: { title: "想看列表", badge: "想看" },
  2: { title: "看过列表", badge: "看过" },
  3: { title: "在看列表", badge: "在看" },
  4: { title: "搁置列表", badge: "搁置" },
  5: { title: "抛弃列表", badge: "抛弃" },
};

function itemTitle(item: any) {
  return item?.subject?.name_cn || item?.subject?.name || "未知条目";
}

function posterUrl(item: any) {
  return `/api/v1/bangumi/cover/${item.subject_id}`;
}

function hasPoster(item: any) {
  return Boolean(item?.subject?.images?.large || item?.subject?.images?.medium || item?.subject?.images?.common || item?.subject?.images?.small);
}

function subjectTags(item: any) {
  const tags = Array.isArray(item?.subject?.tags) ? item.subject.tags : [];
  return tags.slice(0, 5).map((tag: any) => String(tag?.name || "")).filter(Boolean);
}

function RateInput({ value, onChange }: { value: number; onChange: (v: number) => void }) {
  return (
    <div className="flex flex-wrap gap-1">
      {[0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((v) => (
        <button
          key={v}
          type="button"
          onClick={() => onChange(value === v ? 0 : v)}
          className={`h-8 w-8 rounded text-xs font-semibold transition-colors ${
            value === v ? "bg-yellow-500 text-black" : "bg-accent/40 text-muted-foreground hover:bg-accent/70"
          }`}
        >
          {v}
        </button>
      ))}
    </div>
  );
}

export default function BangumiCollectionPage() {
  const params = useParams<{ type: string }>();
  const { toast } = useToast();
  const type = Number(params.type || 3);
  const meta = COLLECTION_META[type] || COLLECTION_META[3];
  const [page, setPage] = useState(1);
  const [items, setItems] = useState<any[]>([]);
  const [total, setTotal] = useState(0);
  const [cached, setCached] = useState(false);
  const [cacheUpdatedAt, setCacheUpdatedAt] = useState<number | null>(null);
  const [editingItem, setEditingItem] = useState<any | null>(null);
  const [editType, setEditType] = useState(3);
  const [editEpStatus, setEditEpStatus] = useState(0);
  const [editRate, setEditRate] = useState(0);
  const [refreshing, setRefreshing] = useState(false);
  const [saving, setSaving] = useState(false);

  const offset = (page - 1) * PAGE_SIZE;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const fetchCollections = useCallback(async (refresh = false) => {
    const res = await api.getBangumiCollections(type, PAGE_SIZE, offset, refresh);
    if (!res.success || !res.data) throw new Error(res.message || "加载失败");
    setItems(res.data.entries || []);
    setTotal(res.data.total || 0);
    setCached(Boolean(res.data.cached));
    setCacheUpdatedAt(res.data.cache_updated_at || null);
    return true;
  }, [offset, type]);

  const loadCollections = useCallback(() => fetchCollections(false), [fetchCollections]);

  const { isLoading, error, execute: reload } = useAsyncResource(loadCollections, { immediate: true });

  const handleRefresh = async () => {
    setRefreshing(true);
    try {
      await fetchCollections(true);
      toast({ title: "已刷新收藏列表" });
    } catch (err: any) {
      toast({ title: "刷新失败", description: err?.message || "网络错误", variant: "destructive" });
    } finally {
      setRefreshing(false);
    }
  };

  const pageSummary = useMemo(() => {
    if (total === 0) return "暂无条目";
    return `第 ${offset + 1}-${Math.min(offset + items.length, total)} 条，共 ${total} 条`;
  }, [items.length, offset, total]);

  const openEdit = (item: any) => {
    setEditingItem(item);
    setEditType(item.type ?? type);
    setEditEpStatus(item.ep_status ?? 0);
    setEditRate(item.rate ?? 0);
  };

  const saveProgress = async () => {
    if (!editingItem) return;
    setSaving(true);
    try {
      const payload: { type: number; ep_status?: number; rate: number } = { type: editType, rate: editRate };
      if (editType === 3) payload.ep_status = editEpStatus;
      const res = await api.updateBangumiCollection(String(editingItem.subject_id), payload);
      if (res.success) {
        toast({ title: "已更新收藏状态" });
        setEditingItem(null);
        await reload();
      } else {
        toast({ title: "更新失败", description: res.message, variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: "更新失败", description: err?.message || "网络错误", variant: "destructive" });
    } finally {
      setSaving(false);
    }
  };

  return (
    <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <Button asChild variant="ghost" size="sm" className="h-8 px-0 text-muted-foreground">
            <Link href="/bangumi" prefetch={false}>
              <ArrowLeft className="mr-1 h-4 w-4" />
              返回 Bangumi
            </Link>
          </Button>
          <h1 className="flex items-center gap-2 text-2xl font-bold">
            <BookOpen className="h-6 w-6" />
            {meta.title}
          </h1>
          <p className="text-sm text-muted-foreground">{pageSummary}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={cached ? "default" : "secondary"}>
            {cached ? "缓存加载" : "实时加载"}
          </Badge>
          {cacheUpdatedAt ? (
            <span className="text-xs text-muted-foreground">
              缓存于 {new Date(cacheUpdatedAt * 1000).toLocaleString()}
            </span>
          ) : null}
          <Button variant="outline" size="sm" onClick={() => void handleRefresh()} disabled={isLoading || refreshing}>
            {isLoading || refreshing ? <Loader2 className="mr-1 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-1 h-4 w-4" />}
            刷新
          </Button>
        </div>
      </div>

      {error ? (
        <Alert variant="destructive">
          <AlertTitle>加载失败</AlertTitle>
          <AlertDescription>{String(error)}</AlertDescription>
        </Alert>
      ) : isLoading && items.length === 0 ? (
        <Card>
          <CardContent className="flex justify-center p-10">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </CardContent>
        </Card>
      ) : items.length === 0 ? (
        <Card>
          <CardContent className="p-10 text-center text-sm text-muted-foreground">暂无条目</CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {items.map((item) => {
            const subject = item.subject || {};
            const rating = subject.rating || {};
            const tags = subjectTags(item);
            return (
              <Card key={item.subject_id} className="overflow-hidden">
                <CardContent className="grid h-full grid-cols-[96px_1fr] gap-4 p-4">
                  {hasPoster(item) ? (
                    // eslint-disable-next-line @next/next/no-img-element -- Bangumi poster served locally
                    <img src={posterUrl(item)} alt={itemTitle(item)} className="h-36 w-24 rounded-md object-cover shadow-sm" loading="lazy" referrerPolicy="no-referrer" />
                  ) : (
                    <div className="flex h-36 w-24 items-center justify-center rounded-md bg-muted text-xs text-muted-foreground">无封面</div>
                  )}
                  <div className="min-w-0 space-y-2">
                    <div>
                      <div className="flex items-start justify-between gap-2">
                        <h2 className="line-clamp-2 text-sm font-bold" title={itemTitle(item)}>{itemTitle(item)}</h2>
                        <Badge variant="outline" className="shrink-0 text-[10px]">{meta.badge}</Badge>
                      </div>
                      {subject.name && subject.name !== itemTitle(item) ? (
                        <p className="truncate text-xs text-muted-foreground" title={subject.name}>{subject.name}</p>
                      ) : null}
                    </div>
                    <div className="flex flex-wrap gap-1 text-[11px] text-muted-foreground">
                      {subject.date ? <span className="inline-flex items-center gap-1"><CalendarDays className="h-3 w-3" />{subject.date}</span> : null}
                      {subject.eps ? <Badge variant="secondary" className="text-[10px]">{subject.eps} 话</Badge> : null}
                      {rating.score ? <Badge variant="outline" className="gap-1 text-[10px]"><Star className="h-3 w-3 fill-current text-yellow-500" />{rating.score}</Badge> : null}
                      {rating.rank ? <Badge variant="outline" className="text-[10px]">Rank #{rating.rank}</Badge> : null}
                    </div>
                    <div className="flex flex-wrap gap-1">
                      {tags.map((tag: string) => <Badge key={tag} variant="secondary" className="text-[10px]">{tag}</Badge>)}
                    </div>
                    <div className="flex flex-wrap items-center gap-1 text-xs">
                      {item.ep_status ? <Badge variant="default" className="text-[10px]">进度 {item.ep_status}</Badge> : <Badge variant="outline" className="text-[10px]">无进度</Badge>}
                      {item.rate ? <Badge variant="outline" className="text-[10px]">我的评分 {item.rate}</Badge> : null}
                    </div>
                    <div className="flex items-center justify-between border-t border-border/40 pt-2">
                      <Button variant="ghost" size="sm" className="h-7 px-2 text-xs" onClick={() => openEdit(item)}>
                        <Edit className="mr-1 h-3.5 w-3.5" />
                        状态
                      </Button>
                      <a href={`https://bgm.tv/subject/${item.subject_id}`} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 text-xs text-primary hover:underline">
                        Bangumi
                        <ExternalLink className="h-3 w-3" />
                      </a>
                    </div>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      <div className="flex items-center justify-center gap-3">
        <Button variant="outline" disabled={page <= 1 || isLoading} onClick={() => setPage((prev) => Math.max(1, prev - 1))}>
          上一页
        </Button>
        <span className="text-sm text-muted-foreground">第 {page} / {totalPages} 页</span>
        <Button variant="outline" disabled={page >= totalPages || isLoading} onClick={() => setPage((prev) => Math.min(totalPages, prev + 1))}>
          下一页
        </Button>
      </div>

      <Dialog open={Boolean(editingItem)} onOpenChange={(open) => { if (!open) setEditingItem(null); }}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>编辑收藏状态</DialogTitle>
            <DialogDescription>{editingItem ? itemTitle(editingItem) : ""}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>观看状态</Label>
              <Select value={String(editType)} onValueChange={(value) => setEditType(Number(value))}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="1">想看</SelectItem>
                  <SelectItem value="2">看过</SelectItem>
                  <SelectItem value="3">在看</SelectItem>
                  <SelectItem value="4">搁置</SelectItem>
                  <SelectItem value="5">抛弃</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {editType === 3 && (
              <div className="space-y-2">
                <Label>看到第几集</Label>
                <Input type="number" min={0} value={editEpStatus} onChange={(e) => setEditEpStatus(parseInt(e.target.value, 10) || 0)} />
              </div>
            )}
            {editType === 2 && (
              <p className="rounded-md border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
                保存为「看过」时，后端会自动读取 Bangumi 本篇章节总数并把进度写满。
              </p>
            )}
            <div className="space-y-2">
              <Label>评分</Label>
              <RateInput value={editRate} onChange={setEditRate} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditingItem(null)}>取消</Button>
            <Button onClick={saveProgress} disabled={saving}>
              {saving && <Loader2 className="mr-1 h-4 w-4 animate-spin" />}
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}
