"use client";

import { useCallback, useState } from "react";
import {
  AlertTriangle,
  Trash2,
  Loader2,
  ChevronLeft,
  ChevronRight,
  Search,
  ShieldAlert,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
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
import { PageError } from "@/components/layout/page-state";
import { api, type ViolationLog } from "@/lib/api";
import { formatDate } from "@/lib/utils";

const CODE_TYPE_LABELS: Record<string, string> = {
  regcode_decoy: "诱饵注册码",
  regcode_target_mismatch: "指名注册码越权",
};

const ACTION_LABELS: Record<string, string> = {
  disable_user: "已禁用账号",
  disable_emby: "已禁用 Emby",
  log_only: "仅记录",
};

export default function AdminViolationsPage() {
  const { toast } = useToast();
  const [violations, setViolations] = useState<ViolationLog[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [typeFilter, setTypeFilter] = useState("all");
  const [search, setSearch] = useState("");
  const [searchInput, setSearchInput] = useState("");
  const [clearOpen, setClearOpen] = useState(false);
  const [isClearing, setIsClearing] = useState(false);

  const loadViolations = useCallback(
    async (signal?: AbortSignal) => {
      const res = await api.getViolations(page, {
        type: typeFilter !== "all" ? typeFilter : undefined,
        search: search || undefined,
      });
      if (res.success && res.data) {
        setViolations(res.data.violations || []);
        setTotal(res.data.total || 0);
      }
      return true;
    },
    [page, typeFilter, search]
  );

  const { isLoading, error, execute: reload } = useAsyncResource(
    loadViolations,
    { immediate: true }
  );

  const handleDelete = async (id: number) => {
    try {
      const res = await api.deleteViolation(id);
      if (res.success) {
        toast({ title: "已删除", variant: "success" });
        reload();
      } else {
        toast({ title: "删除失败", description: res.message, variant: "destructive" });
      }
    } catch (err: unknown) {
      toast({ title: "删除失败", description: (err as Error).message, variant: "destructive" });
    }
  };

  const handleClearAll = async () => {
    setIsClearing(true);
    try {
      const res = await api.clearViolations();
      if (res.success) {
        toast({ title: "已清除所有违规记录", variant: "success" });
        setClearOpen(false);
        reload();
      } else {
        toast({ title: "清除失败", description: res.message, variant: "destructive" });
      }
    } catch (err: unknown) {
      toast({ title: "清除失败", description: (err as Error).message, variant: "destructive" });
    } finally {
      setIsClearing(false);
    }
  };

  const handleSearch = () => {
    setSearch(searchInput);
    setPage(1);
  };

  const totalPages = Math.ceil(total / 20);

  if (error) return <PageError message={error.message} />;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <ShieldAlert className="h-6 w-6 text-destructive" />
          <h1 className="text-2xl font-bold">违规审计</h1>
          {total > 0 && (
            <Badge variant="destructive" className="ml-2">
              {total}
            </Badge>
          )}
        </div>
        {total > 0 && (
          <Button
            variant="destructive"
            size="sm"
            onClick={() => setClearOpen(true)}
          >
            清除全部
          </Button>
        )}
      </div>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">筛选</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col sm:flex-row gap-3">
            <Select value={typeFilter} onValueChange={(v) => { setTypeFilter(v); setPage(1); }}>
              <SelectTrigger className="w-full sm:w-[200px]">
                <SelectValue placeholder="违规类型" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部类型</SelectItem>
                <SelectItem value="regcode_decoy">诱饵注册码</SelectItem>
                <SelectItem value="regcode_target_mismatch">指名码越权</SelectItem>
              </SelectContent>
            </Select>
            <div className="flex gap-2 flex-1">
              <Input
                placeholder="搜索用户名或卡码..."
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && handleSearch()}
              />
              <Button variant="outline" size="icon" onClick={handleSearch}>
                <Search className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : violations.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
              <AlertTriangle className="h-10 w-10 mb-3 opacity-40" />
              <p>暂无违规记录</p>
            </div>
          ) : (
            <div className="divide-y">
              {violations.map((v) => (
                <div
                  key={v.id}
                  className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-4 p-4"
                >
                  <div className="flex-1 min-w-0 space-y-1">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="font-medium">{v.username}</span>
                      <Badge variant="outline" className="text-xs">
                        UID: {v.uid}
                      </Badge>
                      {v.telegram_id && (
                        <Badge variant="secondary" className="text-xs">
                          TG: {v.telegram_id}
                        </Badge>
                      )}
                    </div>
                    <div className="text-sm text-muted-foreground">
                      <span className="font-mono text-xs bg-muted px-1.5 py-0.5 rounded">
                        {v.code}
                      </span>
                      <span className="mx-2">·</span>
                      <span>{CODE_TYPE_LABELS[v.code_type] || v.code_type}</span>
                    </div>
                    <div className="text-xs text-muted-foreground">
                      {v.reason}
                    </div>
                  </div>
                  <div className="flex items-center gap-3 sm:flex-shrink-0">
                    <Badge
                      variant={v.action === "log_only" ? "secondary" : "destructive"}
                      className="text-xs"
                    >
                      {ACTION_LABELS[v.action] || v.action}
                    </Badge>
                    <span className="text-xs text-muted-foreground whitespace-nowrap">
                      {formatDate(v.created_at)}
                    </span>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8"
                      onClick={() => handleDelete(v.id)}
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

      {totalPages > 1 && (
        <div className="flex items-center justify-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => setPage(page - 1)}
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <span className="text-sm text-muted-foreground">
            {page} / {totalPages}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= totalPages}
            onClick={() => setPage(page + 1)}
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      )}

      <Dialog open={clearOpen} onOpenChange={setClearOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认清除所有违规记录</DialogTitle>
            <DialogDescription>
              此操作不可撤销，将删除所有 {total} 条违规审计记录。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setClearOpen(false)}>
              取消
            </Button>
            <Button
              variant="destructive"
              onClick={handleClearAll}
              disabled={isClearing}
            >
              {isClearing && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
              确认清除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
