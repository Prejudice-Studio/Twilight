"use client";

import { useCallback, useRef, useState } from "react";
import {
  Check,
  X,
  Clock,
  Loader2,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { useToast } from "@/hooks/use-toast";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError } from "@/components/layout/page-state";
import { api, type TelegramRebindRequest } from "@/lib/api";
import { formatDate } from "@/lib/utils";

export default function AdminTelegramRebindRequestsPage() {
  const { toast } = useToast();
  const [requests, setRequests] = useState<TelegramRebindRequest[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState("pending");

  const [actionOpen, setActionOpen] = useState(false);
  const [selectedRequest, setSelectedRequest] = useState<TelegramRebindRequest | null>(null);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [selectedAction, setSelectedAction] = useState<"approve" | "reject">("approve");
  const [adminNote, setAdminNote] = useState("");
  const [isActioning, setIsActioning] = useState(false);

  const requestsCacheRef = useRef(new Map<string, { requests: TelegramRebindRequest[]; total: number }>());

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

    const res = await api.getTelegramRebindRequests({ page, per_page: 20, status }, signal);
    if (res.success && res.data) {
      setRequests(res.data.requests);
      setTotal(res.data.total);
      requestsCacheRef.current.set(cacheKey, { requests: res.data.requests, total: res.data.total });
    }
    return true;
  }, [page, status]);

  const { isLoading, error, execute: loadRequests } = useAsyncResource(loadRequestsResource, { immediate: true });

  const openActionDialog = (request: TelegramRebindRequest, action: "approve" | "reject") => {
    setSelectedRequest(request);
    setSelectedAction(action);
    setAdminNote(request.admin_note || "");
    setActionOpen(true);
  };

  const openBulkActionDialog = (action: "approve" | "reject") => {
    if (selectedIds.size === 0) {
      toast({ title: "请先选择待处理请求", variant: "destructive" });
      return;
    }
    setSelectedRequest(null);
    setSelectedAction(action);
    setAdminNote("");
    setActionOpen(true);
  };

  const toggleSelect = (id: number, checked: boolean) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  };

  const selectCurrentPendingPage = () => {
    setSelectedIds(new Set(requests.filter((item) => item.status === "pending").map((item) => item.id)));
  };

  const handleAction = async () => {
    const request = selectedRequest;
    const isBulk = !request;
    const ids = Array.from(selectedIds);
    if (!request && ids.length === 0) return;
    setIsActioning(true);
    try {
      const res = isBulk
        ? await api.batchReviewTelegramRebindRequests(ids, selectedAction, adminNote)
        : selectedAction === "approve"
          ? await api.approveTelegramRebindRequest(request!.id, adminNote)
          : await api.rejectTelegramRebindRequest(request!.id, adminNote);
      if (res.success) {
        const bulkData = isBulk ? res.data as { success: number; failed: number } | undefined : undefined;
        toast({
          title: isBulk ? "批量操作完成" : "操作成功",
          description: bulkData ? `成功 ${bulkData.success}，失败 ${bulkData.failed}` : undefined,
          variant: "success",
        });
        setActionOpen(false);
        setSelectedRequest(null);
        setSelectedIds(new Set());
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

  const getStatusBadge = (status: string) => {
    switch (status) {
      case "pending":
        return (
          <Badge variant="warning">
            <Clock className="mr-1 h-3 w-3" />
            待处理
          </Badge>
        );
      case "approved":
        return (
          <Badge variant="success">
            <Check className="mr-1 h-3 w-3" />
            已批准
          </Badge>
        );
      case "rejected":
        return (
          <Badge variant="destructive">
            <X className="mr-1 h-3 w-3" />
            已拒绝
          </Badge>
        );
      default:
        return <Badge variant="secondary">{status}</Badge>;
    }
  };

  const pages = Math.ceil(total / 20);
  const pendingOnPage = requests.filter((item) => item.status === "pending");

  if (error) {
    return <PageError message={error} onRetry={() => void loadRequests()} />;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">Telegram 换绑审核</h1>
          <p className="text-muted-foreground">审核用户提交的 Telegram 换绑请求</p>
        </div>
        <Badge variant="outline" className="text-lg px-4 py-2">
          共 {total} 条请求
        </Badge>
      </div>

      <Card>
        <CardContent className="p-4">
          <div className="mb-4 flex flex-wrap items-center gap-2">
            {['pending', 'approved', 'rejected'].map((value) => (
              <Button
                key={value}
                variant={status === value ? 'secondary' : 'outline'}
                size="sm"
                onClick={() => { setStatus(value); setPage(1); setSelectedIds(new Set()); }}
              >
                {value === 'pending' ? '待处理' : value === 'approved' ? '已批准' : '已拒绝'}
              </Button>
            ))}
            {status === "pending" && pendingOnPage.length > 0 && (
              <>
                <Button variant="outline" size="sm" onClick={selectCurrentPendingPage}>
                  全选当前页
                </Button>
                <Button variant="outline" size="sm" onClick={() => setSelectedIds(new Set())} disabled={selectedIds.size === 0}>
                  清空选择
                </Button>
                <Button size="sm" onClick={() => openBulkActionDialog("approve")} disabled={selectedIds.size === 0}>
                  <Check className="mr-1 h-4 w-4" />
                  批量批准 ({selectedIds.size})
                </Button>
                <Button size="sm" variant="destructive" onClick={() => openBulkActionDialog("reject")} disabled={selectedIds.size === 0}>
                  <X className="mr-1 h-4 w-4" />
                  批量拒绝 ({selectedIds.size})
                </Button>
              </>
            )}
          </div>

          {isLoading ? (
            <div className="flex h-64 items-center justify-center">
              <Loader2 className="h-8 w-8 animate-spin text-primary" />
            </div>
          ) : requests.length === 0 ? (
            <div className="flex h-64 items-center justify-center text-muted-foreground">
              暂无{status === 'pending' ? '待处理的' : ''}请求
            </div>
          ) : (
            <div className="space-y-4">
              {requests.map((request) => (
                <Card key={request.id} className="border">
                  {/* 内层卡没有 CardHeader，shadcn 默认 `p-6 pt-0` 会把内容顶到 0px 边缘，
                      显式补 `pt-6` 再用 `items-center` 让按钮和文字块垂直居中 */}
                  <CardContent className="pt-6">
                    <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
                      <div className="space-y-2 min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-3">
                          {request.status === 'pending' && (
                            <input
                              type="checkbox"
                              className="h-4 w-4 rounded border-border"
                              checked={selectedIds.has(request.id)}
                              onChange={(event) => toggleSelect(request.id, event.target.checked)}
                              aria-label={`选择 ${request.username || `UID ${request.uid}`} 的换绑请求`}
                            />
                          )}
                          <p className="text-lg font-medium">{request.username || `UID ${request.uid}`}</p>
                          {getStatusBadge(request.status)}
                        </div>
                        <p className="text-sm text-muted-foreground">
                          提交时间：{formatDate(request.created_at)}
                        </p>
                        <p className="text-sm">
                          当前 Telegram ID：{request.old_telegram_id ?? '无'}
                        </p>
                        {request.reason && (
                          <p className="text-sm text-muted-foreground">原因：{request.reason}</p>
                        )}
                        {request.admin_note && (
                          <p className="text-sm text-primary">管理员备注：{request.admin_note}</p>
                        )}
                        {request.reviewed_at && (
                          <p className="text-sm text-muted-foreground">处理时间：{formatDate(request.reviewed_at)}</p>
                        )}
                      </div>
                      {request.status === 'pending' && (
                        <div className="flex shrink-0 flex-wrap items-center gap-2 md:flex-nowrap">
                          <Button
                            size="sm"
                            className="h-9 min-w-20 justify-center"
                            onClick={() => openActionDialog(request, 'approve')}
                          >
                            <Check className="mr-1 h-4 w-4" />
                            批准
                          </Button>
                          <Button
                            size="sm"
                            variant="destructive"
                            className="h-9 min-w-20 justify-center"
                            onClick={() => openActionDialog(request, 'reject')}
                          >
                            <X className="mr-1 h-4 w-4" />
                            拒绝
                          </Button>
                        </div>
                      )}
                    </div>
                  </CardContent>
                </Card>
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
            aria-label="上一页"
          >
            <ChevronLeft className="h-4 w-4" aria-hidden="true" />
          </Button>
          <span className="text-sm text-muted-foreground">第 {page} / {pages} 页</span>
          <Button
            variant="outline"
            size="icon"
            onClick={() => setPage((p) => Math.min(pages, p + 1))}
            disabled={page === pages}
            aria-label="下一页"
          >
            <ChevronRight className="h-4 w-4" aria-hidden="true" />
          </Button>
        </div>
      )}

      <Dialog open={actionOpen} onOpenChange={setActionOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {selectedRequest
                ? selectedAction === 'approve' ? '批准换绑请求' : '拒绝换绑请求'
                : selectedAction === 'approve' ? '批量批准换绑请求' : '批量拒绝换绑请求'}
            </DialogTitle>
            <DialogDescription>
              {selectedAction === 'approve'
                ? '批准后会解绑用户当前的 Telegram 绑定，用户可重新绑定新的 Telegram 账号。'
                : '拒绝后用户将收到管理员拒绝结果。'}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 pt-4">
            <div>
              <Label>管理员备注 (可选)</Label>
              <Input
                value={adminNote}
                onChange={(event) => setAdminNote(event.target.value)}
                placeholder="填写处理说明"
              />
            </div>
            <div className="rounded-lg border border-muted p-3 text-sm text-muted-foreground">
              {selectedRequest ? (
                <>
                  <p>用户：{selectedRequest.username || `UID ${selectedRequest.uid}`}</p>
                  <p>旧 Telegram ID：{selectedRequest.old_telegram_id ?? '无'}</p>
                  <p>提交时间：{formatDate(selectedRequest.created_at)}</p>
                </>
              ) : (
                <p>将处理已选择的 {selectedIds.size} 条待处理请求。</p>
              )}
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setActionOpen(false)} disabled={isActioning}>
              取消
            </Button>
            <Button onClick={handleAction} disabled={isActioning}>
              {isActioning ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : selectedAction === 'approve' ? '批准' : '拒绝'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
