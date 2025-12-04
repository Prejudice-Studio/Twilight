"use client";

import { useEffect, useState } from "react";
import {
  Film,
  Check,
  X,
  Clock,
  Loader2,
  ChevronLeft,
  ChevronRight,
  MessageSquare,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useToast } from "@/hooks/use-toast";
import { api, type MediaRequest } from "@/lib/api";
import { formatDate } from "@/lib/utils";

export default function AdminRequestsPage() {
  const { toast } = useToast();
  const [requests, setRequests] = useState<MediaRequest[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState("pending");
  const [isLoading, setIsLoading] = useState(true);

  // Action dialog
  const [actionOpen, setActionOpen] = useState(false);
  const [selectedRequest, setSelectedRequest] = useState<MediaRequest | null>(null);
  const [actionType, setActionType] = useState<"approve" | "reject">("approve");
  const [adminNote, setAdminNote] = useState("");
  const [isActioning, setIsActioning] = useState(false);

  useEffect(() => {
    loadRequests();
  }, [page, status]);

  const loadRequests = async () => {
    setIsLoading(true);
    try {
      const res = await api.getMediaRequests({ page, status });
      if (res.success && res.data) {
        setRequests(res.data.requests);
        setTotal(res.data.total);
      }
    } catch (error) {
      console.error(error);
    } finally {
      setIsLoading(false);
    }
  };

  const handleAction = async () => {
    if (!selectedRequest) return;

    setIsActioning(true);
    try {
      const newStatus = actionType === "approve" ? "approved" : "rejected";
      const res = await api.updateMediaRequest(selectedRequest.id, newStatus, adminNote);

      if (res.success) {
        toast({
          title: actionType === "approve" ? "已批准" : "已拒绝",
          variant: "success",
        });
        setActionOpen(false);
        setSelectedRequest(null);
        setAdminNote("");
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

  const openActionDialog = (request: MediaRequest, type: "approve" | "reject") => {
    setSelectedRequest(request);
    setActionType(type);
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
      case "completed":
        return (
          <Badge variant="info">
            <Check className="mr-1 h-3 w-3" />
            已完成
          </Badge>
        );
      default:
        return <Badge variant="secondary">{status}</Badge>;
    }
  };

  const pages = Math.ceil(total / 20);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">求片审核</h1>
          <p className="text-muted-foreground">处理用户的媒体请求</p>
        </div>
        <Badge variant="outline" className="text-lg px-4 py-2">
          共 {total} 条请求
        </Badge>
      </div>

      {/* Status Filter */}
      <Tabs value={status} onValueChange={(v) => { setStatus(v); setPage(1); }}>
        <TabsList>
          <TabsTrigger value="pending">待处理</TabsTrigger>
          <TabsTrigger value="approved">已批准</TabsTrigger>
          <TabsTrigger value="rejected">已拒绝</TabsTrigger>
          <TabsTrigger value="completed">已完成</TabsTrigger>
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
                <div key={request.id} className="flex items-center justify-between p-4 hover:bg-muted/30">
                  <div className="flex items-center gap-4">
                    <div className="flex h-12 w-12 items-center justify-center rounded-lg bg-primary/10">
                      <Film className="h-6 w-6 text-primary" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <p className="font-medium">{request.title}</p>
                        {request.season && (
                          <Badge variant="outline" className="text-xs">
                            第 {request.season} 季
                          </Badge>
                        )}
                      </div>
                      <div className="mt-1 flex items-center gap-2 text-xs text-muted-foreground">
                        <Badge variant="secondary" className="text-xs">
                          {request.source.toUpperCase()}
                        </Badge>
                        <span>•</span>
                        <span>{request.media_type === "movie" ? "电影" : "剧集"}</span>
                        <span>•</span>
                        <span>{formatDate(request.created_at)}</span>
                      </div>
                      {request.note && (
                        <p className="mt-1 text-xs text-muted-foreground">
                          <MessageSquare className="mr-1 inline h-3 w-3" />
                          {request.note}
                        </p>
                      )}
                      {request.admin_note && (
                        <p className="mt-1 text-xs text-primary">
                          管理员备注: {request.admin_note}
                        </p>
                      )}
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    {getStatusBadge(request.status)}
                    {request.status === "pending" && (
                      <div className="flex gap-2">
                        <Button
                          size="sm"
                          variant="outline"
                          className="text-emerald-500 hover:text-emerald-600"
                          onClick={() => openActionDialog(request, "approve")}
                        >
                          <Check className="mr-1 h-4 w-4" />
                          批准
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          className="text-destructive hover:text-destructive"
                          onClick={() => openActionDialog(request, "reject")}
                        >
                          <X className="mr-1 h-4 w-4" />
                          拒绝
                        </Button>
                      </div>
                    )}
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
            <DialogTitle>
              {actionType === "approve" ? "批准请求" : "拒绝请求"}
            </DialogTitle>
            <DialogDescription>
              {selectedRequest?.title}
              {selectedRequest?.season && ` - 第 ${selectedRequest.season} 季`}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>管理员备注（可选）</Label>
              <Input
                placeholder={actionType === "approve" ? "例如：已添加到下载队列" : "例如：版权原因无法添加"}
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
              variant={actionType === "approve" ? "default" : "destructive"}
              onClick={handleAction}
              disabled={isActioning}
            >
              {isActioning && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认{actionType === "approve" ? "批准" : "拒绝"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

