"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";
import {
  Film,
  Check,
  X,
  Clock,
  Loader2,
  ChevronLeft,
  ChevronRight,
  MessageSquare,
  Hash,
  Fingerprint,
  Trash2,
  ExternalLink,
  Copy,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError } from "@/components/layout/page-state";
import { api, type MediaRequest } from "@/lib/api";
import { formatDate } from "@/lib/utils";
import { mediaRequestExternalUrl } from "@/lib/media-external-url";

export default function AdminRequestsPage() {
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const [requests, setRequests] = useState<MediaRequest[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState("pending");

  // Action dialog
  const [actionOpen, setActionOpen] = useState(false);
  const [selectedRequest, setSelectedRequest] = useState<MediaRequest | null>(null);
  const [selectedStatus, setSelectedStatus] = useState("accepted");
  const [adminNote, setAdminNote] = useState("");
  const [isActioning, setIsActioning] = useState(false);
  const requestsCacheRef = useRef<Map<string, { requests: MediaRequest[]; total: number }>>(
    new Map()
  );

  const invalidateRequestsCache = () => {
    requestsCacheRef.current.clear();
  };

  const loadRequestsResource = useCallback(async (signal?: AbortSignal) => {
    const cacheKey = `${page}-${status}`;
    const cached = requestsCacheRef.current.get(cacheKey);
    if (cached) {
      setRequests(cached.requests);
      setTotal(cached.total);
      return true;
    }

    const res = await api.getMediaRequests({ page, status }, signal);
    if (res.success && res.data) {
      setRequests(res.data.requests);
      setTotal(res.data.total);
      requestsCacheRef.current.set(cacheKey, {
        requests: res.data.requests,
        total: res.data.total,
      });
    }
    return true;
  }, [page, status]);

  const {
    isLoading,
    error,
    execute: loadRequests,
  } = useAsyncResource(loadRequestsResource, { immediate: false });

  useEffect(() => {
    void loadRequests();
  }, [loadRequests]);

  const handleAction = async () => {
    if (!selectedRequest) return;
    if (!selectedRequest.require_key) {
      toast({ title: "该请求缺少 require_key，无法操作", variant: "destructive" });
      return;
    }

    setIsActioning(true);
    try {
      const res = await api.updateMediaRequest(selectedRequest.require_key, selectedStatus, adminNote);

      if (res.success) {
        toast({
          title: "操作成功",
          variant: "success",
        });
        setActionOpen(false);
        setSelectedRequest(null);
        setAdminNote("");
        invalidateRequestsCache();
        loadRequests();
      } else {
        toast({ title: "操作失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "操作失败", description: error.message, variant: "destructive" });
    } finally {
      setIsActioning(false);
    }
  };

  const handleDelete = async (requireKey: string) => {
    if (!requireKey) {
      toast({ title: "缺少 require_key，无法删除", variant: "destructive" });
      return;
    }
    const ok = await confirm({
      title: "删除求片请求？",
      description: "该操作不可恢复。",
      tone: "danger",
      confirmLabel: "删除",
    });
    if (!ok) return;

    try {
      const res = await api.deleteMediaRequest(requireKey);
      if (res.success) {
        toast({ title: "删除成功", variant: "success" });
        invalidateRequestsCache();
        loadRequests();
      } else {
        toast({ title: "删除失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "删除失败", description: error.message, variant: "destructive" });
    }
  };

  const openActionDialog = (request: MediaRequest) => {
    setSelectedRequest(request);
    setSelectedStatus(request.status === "pending" ? "accepted" : request.status);
    setAdminNote(request.admin_note || "");
    setActionOpen(true);
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case "pending":
        return (
          <Badge variant="warning">
            <Clock className="mr-1 h-3 w-3" />
            待处理
          </Badge>
        );
      case "accepted":
        return (
          <Badge variant="success">
            <Check className="mr-1 h-3 w-3" />
            已接受
          </Badge>
        );
      case "rejected":
        return (
          <Badge variant="destructive">
            <X className="mr-1 h-3 w-3" />
            已拒绝
          </Badge>
        );
      case "downloading":
        return (
          <Badge variant="gradient">
            <Loader2 className="mr-1 h-3 w-3 animate-spin" />
            下载中
          </Badge>
        );
      case "completed":
        return (
          <Badge variant="success">
            <Check className="mr-1 h-3 w-3" />
            已完成
          </Badge>
        );
      default:
        return <Badge variant="secondary">{status}</Badge>;
    }
  };

  const pages = Math.ceil(total / 20);

  if (error) {
    return <PageError message={error} onRetry={() => void loadRequests()} />;
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <h1 className="text-2xl font-bold sm:text-3xl">求片审核</h1>
          <p className="text-sm text-muted-foreground">处理用户的媒体请求</p>
        </div>
        <Badge variant="outline" className="self-start px-3 py-1.5 text-sm sm:self-auto sm:px-4 sm:py-2 sm:text-lg">
          共 {total} 条请求
        </Badge>
      </div>

      {/* Status Filter */}
      <Tabs value={status} onValueChange={(v) => { setStatus(v); setPage(1); }}>
        <TabsList className="flex w-full overflow-x-auto sm:inline-flex sm:w-auto">
          <TabsTrigger value="pending">待处理</TabsTrigger>
          <TabsTrigger value="accepted">已接受</TabsTrigger>
          <TabsTrigger value="downloading">下载中</TabsTrigger>
          <TabsTrigger value="rejected">已拒绝</TabsTrigger>
          <TabsTrigger value="completed">已完成</TabsTrigger>
          <TabsTrigger value="all">全部</TabsTrigger>
        </TabsList>
      </Tabs>

      {/* Requests List */}
      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="flex h-64 items-center justify-center">
              <Loader2 className="h-8 w-8 animate-spin text-primary" />
            </div>
          ) : requests.length === 0 ? (
            <div className="flex h-64 items-center justify-center text-muted-foreground">
              暂无{status === "pending" ? "待处理的" : ""}请求
            </div>
          ) : (
            <div className="divide-y">
              {requests.map((request) => (
                <div
                  key={request.require_key || `${request.source}-${request.id}`}
                  className="flex flex-col gap-3 p-4 hover:bg-muted/30 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="flex min-w-0 flex-1 items-start gap-4">
                    <div className="relative flex h-20 w-14 shrink-0 items-center justify-center rounded-lg bg-primary/5 overflow-hidden border border-primary/10">
                      {request.media_info?.poster || request.media_info?.poster_url ? (
                        <Image
                          src={request.media_info.poster || request.media_info.poster_url || ""}
                          alt={request.media_info.title}
                          fill
                          unoptimized
                          sizes="56px"
                          className="h-full w-full object-cover"
                        />
                      ) : (
                        <Film className="h-6 w-6 text-primary/50" />
                      )}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        {(() => {
                        const url = mediaRequestExternalUrl(request);
                          const title = request.media_info?.title || request.title;
                          if (!url) return <p className="break-words font-medium">{title}</p>;
                          return (
                            <a
                              href={url}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="break-words font-medium underline decoration-dotted underline-offset-2 hover:text-primary inline-flex items-center gap-1"
                              title={`在 ${request.source.toUpperCase()} 上查看`}
                            >
                              {title}
                              <ExternalLink className="h-3 w-3 shrink-0 opacity-70" />
                            </a>
                          );
                        })()}
                        {request.media_info?.season && (
                          <Badge variant="outline" className="text-xs">
                            第 {request.media_info.season} 季
                          </Badge>
                        )}
                        {request.media_info?.vote_average && (
                          <Badge variant="outline" className="text-xs border-amber-500/20 text-amber-500">
                             ★ {request.media_info.vote_average.toFixed(1)}
                          </Badge>
                        )}
                        {request.media_info?.rating && (
                          <Badge variant="outline" className="text-xs border-amber-500/20 text-amber-500">
                             ★ {request.media_info.rating.toFixed(1)}
                          </Badge>
                        )}
                      </div>
                      <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                        <div className="flex items-center gap-1">
                          {(() => {
                                const url = mediaRequestExternalUrl(request);
                            const inner = request.source.toLowerCase() === "tmdb" ? (
                              <Image
                                src="https://www.themoviedb.org/assets/2/v4/logos/v2/blue_short-8e7b30f73a4020692ccca9c88bafe5dcb6f8a62a4c6bc55cd9ba82bb2cd95f6c.svg"
                                alt="TMDB"
                                width={42}
                                height={12}
                                unoptimized
                                className="h-3 w-auto"
                              />
                            ) : request.source.toLowerCase() === "bangumi" ? (
                              <div className="flex items-center gap-1 bg-[#f09199]/10 dark:bg-[#f09199]/20 px-1.5 py-0.5 rounded text-[10px] font-bold text-[#d95b67] dark:text-[#ffb3bc] border border-[#f09199]/20 dark:border-[#f09199]/40">
                                <Image
                                  src="https://bangumi.tv/img/favicon.ico"
                                  alt="Bangumi"
                                  width={12}
                                  height={12}
                                  unoptimized
                                  className="h-3 w-3"
                                />
                                Bangumi
                              </div>
                            ) : (
                              <Badge variant="secondary" className="text-[10px] h-4">
                                {request.source.toUpperCase()}
                              </Badge>
                            );
                            return url ? (
                              <a
                                href={url}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="inline-flex items-center gap-1 hover:opacity-80"
                                title={`在 ${request.source.toUpperCase()} 上查看`}
                              >
                                {inner}
                              </a>
                            ) : inner;
                          })()}
                        </div>
                        <span className="hidden sm:inline">•</span>
                        <span className="flex items-center gap-0.5" title={`内部 ID（${request.source}）`}>
                          <Hash className="h-3 w-3" />{request.id}
                        </span>
                        {request.media_id !== undefined && request.media_id !== null && request.media_id !== "" && (
                          <>
                            <span className="hidden sm:inline">•</span>
                            <span className="flex items-center gap-0.5" title={`${request.source.toUpperCase()} ID`}>
                              {request.source.toUpperCase()}#{String(request.media_id)}
                            </span>
                          </>
                        )}
                        <span className="hidden sm:inline">•</span>
                        <span className="flex min-w-0 items-center gap-0.5" title="External Update Key（点击复制）">
                          <Fingerprint className="h-3 w-3 shrink-0" />
                          <code className="max-w-[10rem] truncate rounded bg-muted px-1 text-foreground sm:max-w-[16rem]">
                            {request.require_key}
                          </code>
                          <Button
                            type="button"
                            size="icon"
                            variant="ghost"
                            className="h-5 w-5 text-muted-foreground hover:text-foreground"
                            title="复制 Key"
                            onClick={() => {
                              navigator.clipboard.writeText(request.require_key).then(
                                () => toast({ title: "已复制 Key 到剪贴板", variant: "success" }),
                                () => toast({ title: "复制失败", variant: "destructive" }),
                              );
                            }}
                          >
                            <Copy className="h-3 w-3" />
                          </Button>
                        </span>
                        <span className="hidden sm:inline">•</span>
                        <span>{request.media_info?.media_type === "movie" ? "电影" : "剧集"}</span>
                        <span className="hidden sm:inline">•</span>
                        <span>{formatDate(request.timestamp)}</span>
                        {request.user && (
                          <>
                            <span className="hidden sm:inline">•</span>
                            <span className="truncate">用户: {request.user.username || request.user.telegram_id}</span>
                          </>
                        )}
                      </div>
                      {request.media_info?.overview && (
                        <p className="mt-2 line-clamp-2 max-w-full break-words text-xs text-muted-foreground sm:max-w-2xl">
                          {request.media_info.overview}
                        </p>
                      )}
                      {request.media_info?.note && (
                        <p className="mt-1 break-words text-xs text-muted-foreground">
                          <MessageSquare className="mr-1 inline h-3 w-3" />
                          {request.media_info.note}
                        </p>
                      )}
                      {request.admin_note && (
                        <p className="mt-1 break-words text-xs text-primary">
                          管理员备注: {request.admin_note}
                        </p>
                      )}
                    </div>
                  </div>
                  <div className="flex shrink-0 flex-wrap items-center gap-2 sm:gap-3 sm:self-center">
                    {getStatusBadge(request.status)}
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => openActionDialog(request)}
                    >
                      处理
                    </Button>
                    <Button
                      size="icon"
                      variant="ghost"
                      className="h-8 w-8 text-muted-foreground hover:text-destructive dark:hover:bg-destructive/15"
                      onClick={() => handleDelete(request.require_key)}
                      title="删除请求"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {pages > 1 && (
        <div className="flex items-center justify-center gap-2">
          <Button
            variant="outline"
            size="icon"
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            disabled={page === 1}
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <span className="text-sm">
            第 {page} 页，共 {pages} 页
          </span>
          <Button
            variant="outline"
            size="icon"
            onClick={() => setPage((p) => Math.min(pages, p + 1))}
            disabled={page === pages}
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      )}

      {/* Action Dialog */}
      <Dialog open={actionOpen} onOpenChange={setActionOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>处理媒体请求</DialogTitle>
            <DialogDescription>
              ID: {selectedRequest?.id} | 媒体 ID: {selectedRequest?.media_id}
              <br />
              {selectedRequest?.media_info?.title}
              {selectedRequest?.media_info?.season && ` - 第 ${selectedRequest.media_info.season} 季`}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>修改状态</Label>
              <Select value={selectedStatus} onValueChange={setSelectedStatus}>
                <SelectTrigger>
                  <SelectValue placeholder="选择状态" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="pending">待处理</SelectItem>
                  <SelectItem value="accepted">已接受</SelectItem>
                  <SelectItem value="downloading">下载中</SelectItem>
                  <SelectItem value="rejected">已拒绝</SelectItem>
                  <SelectItem value="completed">已完成</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>管理员备注（可选）</Label>
              <Input
                placeholder="在此输入备注信息..."
                value={adminNote}
                onChange={(e) => setAdminNote(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setActionOpen(false)}>
              取消
            </Button>
            <Button
              onClick={handleAction}
              disabled={isActioning}
            >
              {isActioning && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
