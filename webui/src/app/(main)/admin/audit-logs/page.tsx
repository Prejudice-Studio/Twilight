"use client";

import { useCallback, useState } from "react";
import {
  Bot,
  CalendarDays,
  ChevronLeft,
  ChevronRight,
  Filter,
  Loader2,
  RotateCcw,
  ScrollText,
  Search,
  Shield,
  Trash2,
  User,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
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
import { api, type AuditLog } from "@/lib/api";
import { formatDate } from "@/lib/utils";
import { useI18n, type MessageKey } from "@/lib/i18n";

const CATEGORY_ICONS: Record<string, React.ReactNode> = {
  admin: <Shield className="h-4 w-4" />,
  user: <User className="h-4 w-4" />,
  system: <Bot className="h-4 w-4" />,
};

const ACTION_LABELS: Record<string, MessageKey> = {
  create_regcode: "adminAuditLog.actionCreateRegcode",
  update_regcode: "adminAuditLog.actionUpdateRegcode",
  delete_regcode: "adminAuditLog.actionDeleteRegcode",
  batch_delete_regcode: "adminAuditLog.actionBatchDeleteRegcode",
  clear_regcode_usage: "adminAuditLog.actionClearRegcodeUsage",
  create_invite_code: "adminAuditLog.actionCreateInviteCode",
  create_renew_code: "adminAuditLog.actionCreateRenewCode",
  use_code: "adminAuditLog.actionUseCode",
  update_user: "adminAuditLog.actionUpdateUser",
  set_role: "adminAuditLog.actionSetRole",
  enable_user: "adminAuditLog.actionEnableUser",
  disable_user: "adminAuditLog.actionDisableUser",
  delete_user: "adminAuditLog.actionDeleteUser",
  batch_enable_users: "adminAuditLog.actionBatchEnableUsers",
  batch_disable_users: "adminAuditLog.actionBatchDisableUsers",
  batch_renew_users: "adminAuditLog.actionBatchRenewUsers",
  batch_delete_users: "adminAuditLog.actionBatchDeleteUsers",
};

const SORT_MAP: Record<string, { sort: string; order: string }> = {
  created_desc: { sort: "created_at", order: "desc" },
  created_asc: { sort: "created_at", order: "asc" },
  action_asc: { sort: "action", order: "asc" },
  action_desc: { sort: "action", order: "desc" },
  user_asc: { sort: "username", order: "asc" },
  user_desc: { sort: "username", order: "desc" },
  category_asc: { sort: "category", order: "asc" },
  uid_asc: { sort: "uid", order: "asc" },
};

function timeRangeBounds(value: string): { from?: number; to?: number } {
  const now = Math.floor(Date.now() / 1000);
  if (value === "today") {
    const start = new Date();
    start.setHours(0, 0, 0, 0);
    return { from: Math.floor(start.getTime() / 1000), to: now };
  }
  if (value === "24h") return { from: now - 24 * 60 * 60, to: now };
  if (value === "7d") return { from: now - 7 * 24 * 60 * 60, to: now };
  if (value === "30d") return { from: now - 30 * 24 * 60 * 60, to: now };
  return {};
}

export default function AdminAuditLogsPage() {
  const { toast } = useToast();
  const { t } = useI18n();
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [presetFilter, setPresetFilter] = useState("all");
  const [categoryFilter, setCategoryFilter] = useState("all");
  const [actionFilter, setActionFilter] = useState("all");
  const [timeRange, setTimeRange] = useState("all");
  const [sortMode, setSortMode] = useState("created_desc");
  const [perPage, setPerPage] = useState(50);
  const [uidFilter, setUidFilter] = useState("");
  const [targetUidFilter, setTargetUidFilter] = useState("");
  const [search, setSearch] = useState("");
  const [searchInput, setSearchInput] = useState("");
  const [clearOpen, setClearOpen] = useState(false);
  const [isClearing, setIsClearing] = useState(false);
  const [pruneOpen, setPruneOpen] = useState(false);
  const [pruneMode, setPruneMode] = useState<"entries" | "days">("days");
  const [pruneEntries, setPruneEntries] = useState("1000");
  const [pruneDays, setPruneDays] = useState("90");
  const [prunePreserveAdmin, setPrunePreserveAdmin] = useState(true);
  const [isPruning, setIsPruning] = useState(false);

  const sortQuery = SORT_MAP[sortMode] || SORT_MAP.created_desc;

  const loadLogs = useCallback(
    async (signal?: AbortSignal) => {
      const bounds = timeRangeBounds(timeRange);
      const res = await api.getAuditLogs(page, {
        preset: presetFilter !== "all" ? presetFilter : undefined,
        category: categoryFilter !== "all" ? categoryFilter : undefined,
        action: actionFilter !== "all" ? actionFilter : undefined,
        uid: uidFilter.trim() || undefined,
        target_uid: targetUidFilter.trim() || undefined,
        search: search || undefined,
        from: bounds.from,
        to: bounds.to,
        sort: sortQuery.sort,
        order: sortQuery.order,
        per_page: perPage,
        signal,
      });
      if (res.success && res.data) {
        setLogs(res.data.logs || []);
        setTotal(res.data.total || 0);
      }
      return true;
    },
    [
      page,
      presetFilter,
      categoryFilter,
      actionFilter,
      uidFilter,
      targetUidFilter,
      search,
      timeRange,
      sortQuery.sort,
      sortQuery.order,
      perPage,
    ]
  );

  const { isLoading, error, execute: reload } = useAsyncResource(
    loadLogs,
    { immediate: true }
  );

  const handlePresetChange = (value: string) => {
    setPresetFilter(value);
    setPage(1);
    if (value === "admin" || value === "user" || value === "system") {
      setCategoryFilter(value);
      setActionFilter("all");
      return;
    }
    if (value === "today") {
      setCategoryFilter("all");
      setActionFilter("all");
      setTimeRange("today");
      return;
    }
    if (value === "week") {
      setCategoryFilter("all");
      setActionFilter("all");
      setTimeRange("7d");
      return;
    }
    setCategoryFilter("all");
    setActionFilter("all");
    if (value === "all") setTimeRange("all");
  };

  const resetFilters = () => {
    setPresetFilter("all");
    setCategoryFilter("all");
    setActionFilter("all");
    setTimeRange("all");
    setSortMode("created_desc");
    setPerPage(50);
    setUidFilter("");
    setTargetUidFilter("");
    setSearch("");
    setSearchInput("");
    setPage(1);
  };

  const handleDelete = async (id: number) => {
    try {
      const res = await api.deleteAuditLog(id);
      if (res.success) {
        toast({ title: t("adminAuditLog.deleted"), variant: "success" });
        reload();
      } else {
        toast({ title: t("common.deleteFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err: unknown) {
      toast({ title: t("common.deleteFailed"), description: (err as Error).message, variant: "destructive" });
    }
  };

  const handleClearAll = async () => {
    setIsClearing(true);
    try {
      const res = await api.clearAuditLogs();
      if (res.success) {
        toast({ title: t("adminAuditLog.clearedAll"), variant: "success" });
        setClearOpen(false);
        reload();
      } else {
        toast({ title: t("adminAuditLog.clearFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err: unknown) {
      toast({ title: t("adminAuditLog.clearFailed"), description: (err as Error).message, variant: "destructive" });
    } finally {
      setIsClearing(false);
    }
  };

  const handlePrune = async () => {
    setIsPruning(true);
    try {
      const res = await api.pruneAuditLogs({
        maxEntries: pruneMode === "entries" ? Math.max(1, parseInt(pruneEntries, 10) || 1000) : undefined,
        retentionDays: pruneMode === "days" ? Math.max(1, parseInt(pruneDays, 10) || 90) : undefined,
        preserveAdmin: prunePreserveAdmin,
      });
      if (res.success) {
        toast({ title: t("adminAuditLog.pruneDone"), description: res.data?.logs?.join("; "), variant: "success" });
        setPruneOpen(false);
        reload();
      } else {
        toast({ title: t("adminAuditLog.pruneFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err: unknown) {
      toast({ title: t("adminAuditLog.pruneFailed"), description: (err as Error).message, variant: "destructive" });
    } finally {
      setIsPruning(false);
    }
  };

  const handleSearch = () => {
    setSearch(searchInput.trim());
    setPage(1);
  };

  const totalPages = Math.ceil(total / perPage);

  if (error) return <PageError message={error} />;

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-2">
          <ScrollText className="h-6 w-6 text-primary" />
          <div>
            <h1 className="text-2xl font-bold">{t("adminAuditLog.title")}</h1>
            <p className="text-sm text-muted-foreground">{t("adminAuditLog.totalCount", { count: total })}</p>
          </div>
        </div>
        {total > 0 && (
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setPruneMode("days");
                setPruneDays("90");
                setPruneEntries("1000");
                setPrunePreserveAdmin(true);
                setPruneOpen(true);
              }}
            >
              <Filter className="mr-1 h-4 w-4" />
              {t("adminAuditLog.prune")}
            </Button>
            <Button variant="destructive" size="sm" onClick={() => setClearOpen(true)}>
              {t("adminAuditLog.clearAll")}
            </Button>
          </div>
        )}
      </div>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">{t("adminAuditLog.filter")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <Select value={presetFilter} onValueChange={handlePresetChange}>
              <SelectTrigger>
                <SelectValue placeholder={t("adminAuditLog.preset")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("adminAuditLog.presetAll")}</SelectItem>
                <SelectItem value="admin">{t("adminAuditLog.presetAdmin")}</SelectItem>
                <SelectItem value="user">{t("adminAuditLog.presetUser")}</SelectItem>
                <SelectItem value="system">{t("adminAuditLog.presetSystem")}</SelectItem>
                <SelectItem value="destructive">{t("adminAuditLog.presetDestructive")}</SelectItem>
                <SelectItem value="security">{t("adminAuditLog.presetSecurity")}</SelectItem>
                <SelectItem value="today">{t("adminAuditLog.presetToday")}</SelectItem>
                <SelectItem value="week">{t("adminAuditLog.presetWeek")}</SelectItem>
              </SelectContent>
            </Select>

            <Select value={categoryFilter} onValueChange={(value) => { setCategoryFilter(value); setPage(1); }}>
              <SelectTrigger>
                <SelectValue placeholder={t("adminAuditLog.filterCategory")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("adminAuditLog.allCategories")}</SelectItem>
                <SelectItem value="admin">{t("adminAuditLog.categoryAdmin")}</SelectItem>
                <SelectItem value="user">{t("adminAuditLog.categoryUser")}</SelectItem>
                <SelectItem value="system">{t("adminAuditLog.categorySystem")}</SelectItem>
              </SelectContent>
            </Select>

            <Select value={actionFilter} onValueChange={(value) => { setActionFilter(value); setPage(1); }}>
              <SelectTrigger>
                <SelectValue placeholder={t("adminAuditLog.filterAction")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("adminAuditLog.allActions")}</SelectItem>
                <SelectItem value="create_regcode">{t("adminAuditLog.actionCreateRegcode")}</SelectItem>
                <SelectItem value="update_regcode">{t("adminAuditLog.actionUpdateRegcode")}</SelectItem>
                <SelectItem value="delete_regcode">{t("adminAuditLog.actionDeleteRegcode")}</SelectItem>
                <SelectItem value="batch_delete_regcode">{t("adminAuditLog.actionBatchDeleteRegcode")}</SelectItem>
                <SelectItem value="clear_regcode_usage">{t("adminAuditLog.actionClearRegcodeUsage")}</SelectItem>
                <SelectItem value="create_invite_code">{t("adminAuditLog.actionCreateInviteCode")}</SelectItem>
                <SelectItem value="create_renew_code">{t("adminAuditLog.actionCreateRenewCode")}</SelectItem>
                <SelectItem value="use_code">{t("adminAuditLog.actionUseCode")}</SelectItem>
                <SelectItem value="update_user">{t("adminAuditLog.actionUpdateUser")}</SelectItem>
                <SelectItem value="set_role">{t("adminAuditLog.actionSetRole")}</SelectItem>
                <SelectItem value="enable_user">{t("adminAuditLog.actionEnableUser")}</SelectItem>
                <SelectItem value="disable_user">{t("adminAuditLog.actionDisableUser")}</SelectItem>
                <SelectItem value="delete_user">{t("adminAuditLog.actionDeleteUser")}</SelectItem>
                <SelectItem value="batch_enable_users">{t("adminAuditLog.actionBatchEnableUsers")}</SelectItem>
                <SelectItem value="batch_disable_users">{t("adminAuditLog.actionBatchDisableUsers")}</SelectItem>
                <SelectItem value="batch_renew_users">{t("adminAuditLog.actionBatchRenewUsers")}</SelectItem>
                <SelectItem value="batch_delete_users">{t("adminAuditLog.actionBatchDeleteUsers")}</SelectItem>
              </SelectContent>
            </Select>

            <Select value={timeRange} onValueChange={(value) => { setTimeRange(value); setPage(1); }}>
              <SelectTrigger>
                <CalendarDays className="mr-2 h-4 w-4 text-muted-foreground" />
                <SelectValue placeholder={t("adminAuditLog.timeRange")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("adminAuditLog.timeAll")}</SelectItem>
                <SelectItem value="today">{t("adminAuditLog.timeToday")}</SelectItem>
                <SelectItem value="24h">{t("adminAuditLog.time24h")}</SelectItem>
                <SelectItem value="7d">{t("adminAuditLog.time7d")}</SelectItem>
                <SelectItem value="30d">{t("adminAuditLog.time30d")}</SelectItem>
              </SelectContent>
            </Select>

            <Select value={sortMode} onValueChange={(value) => { setSortMode(value); setPage(1); }}>
              <SelectTrigger>
                <SelectValue placeholder={t("adminAuditLog.sortBy")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="created_desc">{t("adminAuditLog.sortNewest")}</SelectItem>
                <SelectItem value="created_asc">{t("adminAuditLog.sortOldest")}</SelectItem>
                <SelectItem value="action_asc">{t("adminAuditLog.sortActionAsc")}</SelectItem>
                <SelectItem value="action_desc">{t("adminAuditLog.sortActionDesc")}</SelectItem>
                <SelectItem value="user_asc">{t("adminAuditLog.sortUserAsc")}</SelectItem>
                <SelectItem value="user_desc">{t("adminAuditLog.sortUserDesc")}</SelectItem>
                <SelectItem value="category_asc">{t("adminAuditLog.sortCategoryAsc")}</SelectItem>
                <SelectItem value="uid_asc">{t("adminAuditLog.sortUidAsc")}</SelectItem>
              </SelectContent>
            </Select>

            <Select value={String(perPage)} onValueChange={(value) => { setPerPage(Number(value)); setPage(1); }}>
              <SelectTrigger>
                <SelectValue placeholder={t("adminAuditLog.perPage")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="25">25 / {t("adminAuditLog.perPage")}</SelectItem>
                <SelectItem value="50">50 / {t("adminAuditLog.perPage")}</SelectItem>
                <SelectItem value="100">100 / {t("adminAuditLog.perPage")}</SelectItem>
                <SelectItem value="200">200 / {t("adminAuditLog.perPage")}</SelectItem>
              </SelectContent>
            </Select>

            <Input
              inputMode="numeric"
              placeholder={t("adminAuditLog.uidPlaceholder")}
              value={uidFilter}
              onChange={(event) => {
                setUidFilter(event.target.value.replace(/[^\d]/g, ""));
                setPage(1);
              }}
            />

            <Input
              inputMode="numeric"
              placeholder={t("adminAuditLog.targetUidPlaceholder")}
              value={targetUidFilter}
              onChange={(event) => {
                setTargetUidFilter(event.target.value.replace(/[^\d]/g, ""));
                setPage(1);
              }}
            />
          </div>

          <div className="flex flex-col gap-2 sm:flex-row">
            <div className="flex min-w-0 flex-1 gap-2">
              <Input
                placeholder={t("adminAuditLog.searchPlaceholder")}
                value={searchInput}
                onChange={(event) => setSearchInput(event.target.value)}
                onKeyDown={(event) => event.key === "Enter" && handleSearch()}
              />
              <Button variant="outline" size="icon" onClick={handleSearch} aria-label={t("common.search")}>
                <Search className="h-4 w-4" />
              </Button>
            </div>
            <Button variant="outline" onClick={resetFilters}>
              <RotateCcw className="mr-2 h-4 w-4" />
              {t("adminAuditLog.resetFilters")}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : logs.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
              <ScrollText className="mb-3 h-10 w-10 opacity-40" />
              <p>{t("adminAuditLog.empty")}</p>
            </div>
          ) : (
            <div className="divide-y">
              {logs.map((log) => (
                <div
                  key={log.id}
                  className="flex flex-col gap-2 p-4 sm:flex-row sm:items-center sm:gap-4"
                >
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      {CATEGORY_ICONS[log.category] && (
                        <span className="text-muted-foreground">
                          {CATEGORY_ICONS[log.category]}
                        </span>
                      )}
                      <span className="font-medium">{log.username}</span>
                      <Badge variant="outline" className="text-xs">
                        UID: {log.uid}
                      </Badge>
                      <Badge
                        variant={log.category === "admin" ? "default" : log.category === "system" ? "secondary" : "outline"}
                        className="text-xs"
                      >
                        {log.category === "admin"
                          ? t("adminAuditLog.categoryAdmin")
                          : log.category === "system"
                            ? t("adminAuditLog.categorySystem")
                            : t("adminAuditLog.categoryUser")}
                      </Badge>
                    </div>
                    <div className="text-sm">
                      <span className="font-medium">
                        {ACTION_LABELS[log.action] ? t(ACTION_LABELS[log.action]) : log.action}
                      </span>
                      {log.target_uid != null && log.target_uid > 0 && (
                        <span className="ml-2 text-muted-foreground">
                          -&gt; UID: {log.target_uid}
                        </span>
                      )}
                    </div>
                    {log.detail && Object.keys(log.detail).length > 0 && (
                      <div className="break-all text-xs text-muted-foreground">
                        {JSON.stringify(log.detail)}
                      </div>
                    )}
                    {log.ip && (
                      <div className="text-xs text-muted-foreground">
                        IP: {log.ip}
                      </div>
                    )}
                  </div>
                  <div className="flex items-center gap-3 sm:flex-shrink-0">
                    <span className="whitespace-nowrap text-xs text-muted-foreground">
                      {formatDate(log.created_at)}
                    </span>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8"
                      onClick={() => handleDelete(log.id)}
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
            <DialogTitle>{t("adminAuditLog.clearConfirmTitle")}</DialogTitle>
            <DialogDescription>
              {t("adminAuditLog.clearConfirmDescription", { count: total })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setClearOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={handleClearAll}
              disabled={isClearing}
            >
              {isClearing && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t("adminAuditLog.clearConfirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={pruneOpen} onOpenChange={setPruneOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("adminAuditLog.pruneTitle")}</DialogTitle>
            <DialogDescription>{t("adminAuditLog.pruneDescription")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="flex gap-3">
              <Button type="button" variant={pruneMode === "days" ? "default" : "outline"} size="sm" onClick={() => setPruneMode("days")}>
                {t("adminAuditLog.pruneByDays")}
              </Button>
              <Button type="button" variant={pruneMode === "entries" ? "default" : "outline"} size="sm" onClick={() => setPruneMode("entries")}>
                {t("adminAuditLog.pruneByEntries")}
              </Button>
            </div>
            {pruneMode === "days" ? (
              <div className="space-y-2">
                <Label>{t("adminAuditLog.pruneDaysLabel")}</Label>
                <Input type="number" min={1} max={3650} value={pruneDays} onChange={(e) => setPruneDays(e.target.value)} />
                <p className="text-xs text-muted-foreground">{t("adminAuditLog.pruneDaysHint")}</p>
              </div>
            ) : (
              <div className="space-y-2">
                <Label>{t("adminAuditLog.pruneEntriesLabel")}</Label>
                <Input type="number" min={1} max={100000} value={pruneEntries} onChange={(e) => setPruneEntries(e.target.value)} />
                <p className="text-xs text-muted-foreground">{t("adminAuditLog.pruneEntriesHint")}</p>
              </div>
            )}
            <div className="flex items-center gap-2">
              <Checkbox id="preserve-admin" checked={prunePreserveAdmin} onCheckedChange={(v) => setPrunePreserveAdmin(v === true)} />
              <Label htmlFor="preserve-admin" className="text-sm">{t("adminAuditLog.prunePreserveAdmin")}</Label>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPruneOpen(false)} disabled={isPruning}>
              {t("common.cancel")}
            </Button>
            <Button variant="destructive" onClick={handlePrune} disabled={isPruning}>
              {isPruning && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t("adminAuditLog.pruneConfirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
