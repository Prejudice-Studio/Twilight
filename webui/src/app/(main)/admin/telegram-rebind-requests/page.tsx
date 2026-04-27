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

  const handleAction = async () => {
    if (!selectedRequest) return;
    setIsActioning(true);
    try {
      const res = selectedAction === "approve"
        ? await api.approveTelegramRebindRequest(selectedRequest.id, adminNote)
        : await api.rejectTelegramRebindRequest(selectedRequest.id, adminNote);
      if (res.success) {
        toast({ title: "操作成功", variant: "success" });
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
                onClick={() => { setStatus(value); setPage(1); }}
              >
                {value === 'pending' ? '待处理' : value === 'approved' ? '已批准' : '已拒绝'}
              </Button>
            ))}
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
                  <CardContent>
                    <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                      <div className="space-y-2">
                        <div className="flex flex-wrap items-center gap-3">
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
                      <div className="flex flex-wrap items-center gap-2">
                        {request.status === 'pending' && (
                          <>
                            <Button size="sm" onClick={() => openActionDialog(request, 'approve')}>
                              批准
                            </Button>
                            <Button size="sm" variant="destructive" onClick={() => openActionDialog(request, 'reject')}>
                              拒绝
                            </Button>
                          </>
                        )}
                      </div>
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
          <Button variant="outline" size="icon" onClick={() => setPage((p) => Math.max(1, p - 1))} disabled={page === 1}>
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <span className="text-sm text-muted-foreground">第 {page} / {pages} 页</span>
          <Button variant="outline" size="icon" onClick={() => setPage((p) => Math.min(pages, p + 1))} disabled={page === pages}>
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      )}

      <Dialog open={actionOpen} onOpenChange={setActionOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{selectedAction === 'approve' ? '批准换绑请求' : '拒绝换绑请求'}</DialogTitle>
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
              <p>用户：{selectedRequest?.username || `UID ${selectedRequest?.uid}`}</p>
              <p>旧 Telegram ID：{selectedRequest?.old_telegram_id ?? '无'}</p>
              <p>提交时间：{selectedRequest ? formatDate(selectedRequest.created_at) : '-'}</p>
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
