"use client";

import { useCallback, useState } from "react";
import { motion } from "framer-motion";
import { BookOpen, RefreshCw, Trash2, Loader2, CheckCircle2, XCircle, Clock, AlertCircle, Eye, Search } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { api, type BangumiUserInfo, type BangumiSyncLog, type PlaybackRecordWithSync } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

function formatTime(unix: number): string {
  return new Date(unix * 1000).toLocaleString();
}

function formatDuration(seconds: number): string {
  if (seconds <= 0) return "";
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h}h${m}m`;
  return `${m}m`;
}

export default function AdminBangumiPage() {
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const { t } = useI18n();

  const [users, setUsers] = useState<BangumiUserInfo[]>([]);
  const [syncingUID, setSyncingUID] = useState<number | null>(null);
  const [selectedUser, setSelectedUser] = useState<BangumiUserInfo | null>(null);
  const [recordOpen, setRecordOpen] = useState(false);
  const [records, setRecords] = useState<PlaybackRecordWithSync[]>([]);
  const [loadingRecords, setLoadingRecords] = useState(false);
  const [logsOpen, setLogsOpen] = useState(false);
  const [logs, setLogs] = useState<BangumiSyncLog[]>([]);
  const [loadingLogs, setLoadingLogs] = useState(false);

  // 分页与搜索状态
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const [totalPages, setTotalPages] = useState(1);
  const [totalUsers, setTotalUsers] = useState(0);

  const loadResource = useCallback(async () => {
    const res = await api.adminBangumiUsers(page, 20, search);
    if (res.success && res.data) {
      setUsers(res.data.users || []);
      setTotalUsers(res.data.total ?? 0);
      setTotalPages(res.data.pages ?? 1);
      return true;
    }
    throw new Error(res.message || "加载失败");
  }, [page, search]);

  const { isLoading, error, execute: reload } = useAsyncResource(loadResource, { immediate: true });

  const handleSearchSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setSearch(searchQuery);
    setPage(1);
  };

  const handleSyncUser = async (uid: number) => {
    setSyncingUID(uid);
    try {
      const res = await api.adminBangumiSyncUser(uid);
      if (res.success && res.data) {
        toast({ title: t("bangumi.syncCompleted"), description: `${t("bangumi.syncedCount")}: ${res.data.synced}, ${t("bangumi.skippedCount")}: ${res.data.skipped}, ${t("bangumi.failedCount")}: ${res.data.failed}` });
        await reload();
      } else {
        toast({ title: t("bangumi.syncFailed"), description: res.message, variant: "destructive" });
      }
    } catch {
      toast({ title: t("bangumi.syncError"), variant: "destructive" });
    } finally {
      setSyncingUID(null);
    }
  };

  const handleViewRecords = async (u: BangumiUserInfo) => {
    setSelectedUser(u);
    setRecordOpen(true);
    setLoadingRecords(true);
    try {
      const res = await api.adminBangumiRecords(u.uid);
      if (res.success && res.data) {
        setRecords(res.data.records || []);
      }
    } catch {
      toast({ title: t("bangumi.loadFailed"), variant: "destructive" });
    } finally {
      setLoadingRecords(false);
    }
  };

  const handleViewLogs = async (u: BangumiUserInfo) => {
    setSelectedUser(u);
    setLogsOpen(true);
    setLoadingLogs(true);
    try {
      const res = await api.adminBangumiSyncLogs(u.uid);
      if (res.success && res.data) {
        setLogs(res.data.logs || []);
      }
    } catch {
      toast({ title: t("bangumi.loadFailed"), variant: "destructive" });
    } finally {
      setLoadingLogs(false);
    }
  };

  const handleClearLogs = async (uid: number) => {
    const ok = await confirm({
      title: t("bangumi.clearConfirmTitle"),
      description: t("bangumi.clearConfirmDescription"),
      tone: "danger",
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    try {
      const res = await api.adminBangumiClearLogs(uid);
      if (res.success) {
        toast({ title: t("bangumi.cleared") });
        await reload();
      }
    } catch {
      toast({ title: t("common.deleteFailed"), variant: "destructive" });
    }
  };

  return (
    <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold flex items-center gap-2">
          <BookOpen className="h-6 w-6" />
          {t("bangumi.adminTitle")}
        </h1>
        <p className="text-sm text-muted-foreground mt-1">
          {t("bangumi.adminDescription")}
        </p>
      </div>

      <div className="flex items-center gap-2">
        <form onSubmit={handleSearchSubmit} className="flex flex-1 max-w-sm items-center gap-2">
          <Input
            placeholder="搜索用户名..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="h-9"
          />
          <Button type="submit" size="sm" className="h-9">
            <Search className="h-4 w-4 mr-1" />
            搜索
          </Button>
          {(searchQuery || search) && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => {
                setSearchQuery("");
                setSearch("");
                setPage(1);
              }}
              className="h-9 px-2"
            >
              清除
            </Button>
          )}
        </form>
      </div>

      {error ? (
        <Card>
          <CardContent className="pt-6 flex flex-col items-center gap-3">
            <AlertCircle className="h-8 w-8 text-destructive" />
            <p className="text-sm text-muted-foreground">{String(error)}</p>
            <Button variant="outline" onClick={() => { void reload(); }}>{t("common.retry")}</Button>
          </CardContent>
        </Card>
      ) : isLoading ? (
        <Card>
          <CardContent className="pt-6 flex items-center justify-center">
            <Loader2 className="h-6 w-6 animate-spin text-primary" />
          </CardContent>
        </Card>
      ) : users.length === 0 ? (
        <Card>
          <CardContent className="pt-6 flex flex-col items-center gap-3">
            <BookOpen className="h-8 w-8 text-muted-foreground" />
            <p className="text-sm text-muted-foreground">没有找到 Bangumi 用户</p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {users.map((u) => (
            <Card key={u.uid} className="glass-card">
              <CardContent className="pt-4">
                <div className="flex items-center justify-between flex-wrap gap-3">
                  <div className="flex items-center gap-3 min-w-0">
                    <div className="text-sm font-medium truncate">{u.username}</div>
                    <div className="flex items-center gap-1.5 flex-wrap">
                      <Badge variant="outline" className="text-xs">
                        {u.bgm_mode ? "同步:开" : "同步:关"}
                      </Badge>
                      <Badge variant={u.bgm_manage_mode ? "default" : "secondary"} className="text-xs">
                        {u.bgm_manage_mode ? "管理:开" : "管理:关"}
                      </Badge>
                      <Badge variant={u.token_set ? "default" : "secondary"} className="text-xs">
                        {u.token_set ? "Token 已设" : "Token 未设"}
                      </Badge>
                      {u.sync_ready && (
                        <Badge variant="default" className="text-xs bg-green-600">
                          就绪
                        </Badge>
                      )}
                    </div>
                  </div>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <span>{t("bangumi.records")}: {u.record_count}</span>
                    <span className="text-green-500">{t("bangumi.synced")}: {u.sync_count}</span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => handleViewRecords(u)}
                      disabled={u.record_count === 0}
                    >
                      <Eye className="h-3.5 w-3.5 mr-1" />
                      {t("bangumi.viewRecords")}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => handleViewLogs(u)}
                    >
                      <Clock className="h-3.5 w-3.5 mr-1" />
                      {t("bangumi.logs")}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => handleSyncUser(u.uid)}
                      disabled={syncingUID === u.uid || !u.sync_ready}
                    >
                      {syncingUID === u.uid ? (
                        <Loader2 className="h-3.5 w-3.5 animate-spin mr-1" />
                      ) : (
                        <RefreshCw className="h-3.5 w-3.5 mr-1" />
                      )}
                      {t("bangumi.syncNow")}
                    </Button>
                    {u.sync_count > 0 && (
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => handleClearLogs(u.uid)}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    )}
                  </div>
                </div>
              </CardContent>
            </Card>
          ))}

          {/* Pagination Controls */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between border-t border-border/40 pt-4 mt-4 bg-transparent">
              <Button
                variant="outline"
                size="sm"
                disabled={page <= 1}
                onClick={() => setPage((prev) => Math.max(1, prev - 1))}
              >
                上一页
              </Button>
              <span className="text-xs text-muted-foreground">
                第 {page} / {totalPages} 页 (共 {totalUsers} 个用户)
              </span>
              <Button
                variant="outline"
                size="sm"
                disabled={page >= totalPages}
                onClick={() => setPage((prev) => Math.min(totalPages, prev + 1))}
              >
                下一页
              </Button>
            </div>
          )}
        </div>
      )}

      <Dialog open={recordOpen} onOpenChange={setRecordOpen}>
        <DialogContent className="max-w-2xl max-h-[80vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{t("bangumi.playbackRecords")} - {selectedUser?.username}</DialogTitle>
            <DialogDescription>{t("bangumi.playbackRecordsDescription")}</DialogDescription>
          </DialogHeader>
          {loadingRecords ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin" />
            </div>
          ) : records.length === 0 ? (
            <div className="text-center py-8 text-sm text-muted-foreground">{t("bangumi.noRecords")}</div>
          ) : (
            <div className="space-y-2">
              {records.map((rec, idx) => (
                <div key={`${rec.item_id}-${idx}`} className="rounded-lg bg-accent/30 p-2 text-sm">
                  <div className="flex items-center justify-between">
                    <span className="font-medium truncate">{rec.title}</span>
                    {rec.synced_name && (
                      <Badge variant="default" className="text-xs bg-green-600 shrink-0 ml-2">
                        {rec.synced_name}
                      </Badge>
                    )}
                  </div>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground mt-1">
                    {rec.series_name && <span>{rec.series_name}</span>}
                    {rec.media_type && <span>{rec.media_type}</span>}
                    {rec.index_number ? <span>#{rec.index_number}</span> : null}
                    <span>{formatTime(rec.played_at)}</span>
                    <span>{formatDuration(rec.duration)}</span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={logsOpen} onOpenChange={setLogsOpen}>
        <DialogContent className="max-w-2xl max-h-[80vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{t("bangumi.syncLogs")} - {selectedUser?.username}</DialogTitle>
            <DialogDescription>{t("bangumi.syncLogsDescription")}</DialogDescription>
          </DialogHeader>
          {loadingLogs ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin" />
            </div>
          ) : logs.length === 0 ? (
            <div className="text-center py-8 text-sm text-muted-foreground">{t("bangumi.noLogs")}</div>
          ) : (
            <div className="space-y-2">
              {logs.map((log) => (
                <div key={log.id} className="flex items-start gap-2 rounded-lg bg-accent/30 p-2 text-sm">
                  {log.status === "success" ? (
                    <CheckCircle2 className="h-4 w-4 text-green-500 mt-0.5" />
                  ) : log.status === "failed" ? (
                    <XCircle className="h-4 w-4 text-red-500 mt-0.5" />
                  ) : (
                    <Clock className="h-4 w-4 text-yellow-500 mt-0.5" />
                  )}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-1 flex-wrap">
                      {log.subject_name && <span className="font-medium">{log.subject_name}</span>}
                      {log.episode ? <span className="text-muted-foreground">#{log.episode}</span> : null}
                    </div>
                    {log.message && (
                      <p className="text-xs text-muted-foreground mt-0.5 truncate">{log.message}</p>
                    )}
                    <div className="text-xs text-muted-foreground mt-0.5">
                      {formatTime(log.created_at)}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}
