"use client";

import { useCallback, useEffect, useState } from "react";
import {
  FileText,
  Plus,
  Copy,
  Trash2,
  Loader2,
  ChevronLeft,
  ChevronRight,
  Download,
  Save,
  Users,
  RotateCcw,
  Link2,
  RefreshCw,
  Search,
  FlipHorizontal2,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError } from "@/components/layout/page-state";
import { api, type InviteCodeItem, type Regcode, type UserInfo } from "@/lib/api";
import { formatDate } from "@/lib/utils";
import { useI18n, type MessageKey } from "@/lib/i18n";

export default function AdminRegcodesPage() {
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const { t } = useI18n();
  const [regcodes, setRegcodes] = useState<Regcode[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [selectedCodes, setSelectedCodes] = useState<Set<string>>(new Set());
  const [isBatchDeleting, setIsBatchDeleting] = useState(false);
  const [deletingCode, setDeletingCode] = useState<string | null>(null);
  const [filterType, setFilterType] = useState("all");
  const [filterStatus, setFilterStatus] = useState("all");
  const [search, setSearch] = useState("");
  const [sort, setSort] = useState("created_time");
  const [order, setOrder] = useState("desc");
  const [noteDrafts, setNoteDrafts] = useState<Record<string, string>>({});
  const [savingNote, setSavingNote] = useState<string | null>(null);
  const [usageOpen, setUsageOpen] = useState(false);
  const [usageCode, setUsageCode] = useState<Regcode | null>(null);
  const [usageLoading, setUsageLoading] = useState(false);
  const [usageUsers, setUsageUsers] = useState<Array<Partial<UserInfo> & { found: boolean; source: "uid" | "telegram" }>>([]);
  const [usageTelegramOnly, setUsageTelegramOnly] = useState<Array<{ telegram_id: number; found: false; source: "telegram" }>>([]);
  const [clearingUsage, setClearingUsage] = useState(false);

  // Invite codes section
  const [viewMode, setViewMode] = useState<"regcodes" | "invitecodes">("regcodes");
  const [inviteCodes, setInviteCodes] = useState<InviteCodeItem[]>([]);
  const [inviteCodesLoading, setInviteCodesLoading] = useState(false);
  const [inviteCodesTotal, setInviteCodesTotal] = useState(0);
  const [inviteCodesLoaded, setInviteCodesLoaded] = useState(false);
  const [inviteSearch, setInviteSearch] = useState("");

  // Create dialog
  const [createOpen, setCreateOpen] = useState(false);
  const [activeTab, setActiveTab] = useState("1"); // 1: 注册, 2: 续期, 3: 白名单
  const [createData, setCreateData] = useState({
    days: "30",
    validityTime: "-1",
    useCountLimit: "1",
    count: "1",
    format: "",
    randomAlgorithm: "",
    targetUsername: "",
    targetTelegramUsername: "",
    targetTelegramId: "",
  });
  const [createDecoy, setCreateDecoy] = useState(false);
  const [isPermanentDays, setIsPermanentDays] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const [createdCodes, setCreatedCodes] = useState<string[]>([]);

  const formatRegcodeDays = (days: number) => (days < 0 ? t("adminRegcodes.permanent") : t("adminRegcodes.daysValue", { days: days || 30 }));

  const loadRegcodesResource = useCallback(async () => {
    const res = await api.getRegcodes(page, { type: filterType, status: filterStatus, search, sort, order });
    if (res.success && res.data) {
      const regcodesList = Array.isArray(res.data.regcodes)
        ? res.data.regcodes
        : Array.isArray(res.data)
          ? res.data
          : [];
      const totalItems = res.data.total || regcodesList.length;
      if (totalItems > 0 && regcodesList.length === 0 && page > 1) {
        setPage(Math.max(1, Math.ceil(totalItems / 20)));
        return true;
      }
      setRegcodes(regcodesList);
      setNoteDrafts(Object.fromEntries(regcodesList.map((item) => [item.code, item.note || ""])));
      setTotal(totalItems);
    } else {
      setRegcodes([]);
      setTotal(0);
    }
    return true;
  }, [page, filterType, filterStatus, search, sort, order]);

  const {
    isLoading,
    error,
    execute: loadRegcodes,
  } = useAsyncResource(loadRegcodesResource, { immediate: true });

  // 监听 Tab 切换，重置数据
  useEffect(() => {
    if (!createOpen) return;
    setCreatedCodes([]);
    setCreateDecoy(false);
    if (activeTab === "1") {
      setIsPermanentDays(false);
      setCreateData({ days: "30", validityTime: "-1", useCountLimit: "1", count: "1", format: "", randomAlgorithm: "", targetUsername: "", targetTelegramUsername: "", targetTelegramId: "" });
    } else if (activeTab === "2") {
      setIsPermanentDays(false);
      setCreateData({ days: "30", validityTime: "72", useCountLimit: "1", count: "1", format: "", randomAlgorithm: "", targetUsername: "", targetTelegramUsername: "", targetTelegramId: "" });
    } else {
      setIsPermanentDays(true);
      setCreateData({ days: "-1", validityTime: "-1", useCountLimit: "-1", count: "1", format: "", randomAlgorithm: "", targetUsername: "", targetTelegramUsername: "", targetTelegramId: "" });
    }
  }, [activeTab, createOpen]);

  const handleCreate = async () => {
    const count = parseInt(createData.count, 10);
    if (Number.isNaN(count) || count < 1 || count > 100) {
      toast({ title: t("adminRegcodes.paramError"), description: t("adminRegcodes.countRange"), variant: "destructive" });
      return;
    }
    const targetUsername = createData.targetUsername.trim();
    const targetTelegramUsername = createData.targetTelegramUsername.trim().replace(/^@+/, "");
    const targetTelegramIdText = createData.targetTelegramId.trim();
    const targetCount = [targetUsername, targetTelegramUsername, targetTelegramIdText].filter(Boolean).length;
    if (targetCount > 1) {
      toast({ title: t("adminRegcodes.targetConflict"), description: t("adminRegcodes.targetConflictDesc"), variant: "destructive" });
      return;
    }
    const targetTelegramId = targetTelegramIdText ? Number.parseInt(targetTelegramIdText, 10) : undefined;
    if (targetTelegramIdText && (!/^\d+$/.test(targetTelegramIdText) || !targetTelegramId || targetTelegramId <= 0)) {
      toast({ title: t("adminRegcodes.paramError"), description: t("adminRegcodes.tgIdMustBePositive"), variant: "destructive" });
      return;
    }
    setIsCreating(true);
    try {
      const parsedDays = parseInt(createData.days, 10);
      const normalizedDays = isPermanentDays || Number.isNaN(parsedDays) || parsedDays <= 0 ? -1 : parsedDays;

      const res = await api.createRegcode({
        type: parseInt(activeTab),
        days: normalizedDays,
        validity_time: parseInt(createData.validityTime) || -1,
        use_count_limit: parseInt(createData.useCountLimit) || 1,
        count,
        decoy: createDecoy,
        format: createData.format.trim() || undefined,
        random_algorithm: createData.randomAlgorithm || undefined,
        target_username: targetUsername || undefined,
        target_telegram_username: targetTelegramUsername || undefined,
        target_telegram_id: targetTelegramId,
      });

      if (res.success && res.data) {
        toast({ title: t("adminRegcodes.generateSuccess"), variant: "success" });
        setCreatedCodes(res.data.codes || []);
        loadRegcodes();
      } else {
        toast({ title: t("adminRegcodes.generateFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminRegcodes.generateFailed"), description: error.message, variant: "destructive" });
    } finally {
      setIsCreating(false);
    }
  };

  const handleDelete = async (code: Regcode) => {
    const ok = await confirm({
      title: t("adminRegcodes.deleteCodeTitle"),
      description: t("adminRegcodes.deleteCodeDesc", { code: code.code }),
      tone: "danger",
      confirmLabel: t("adminRegcodes.deleteLabel"),
    });
    if (!ok) return;

    setDeletingCode(code.code);
    try {
      const res = await api.deleteRegcode(code.code);
      if (res.success && (!res.data || res.data.deleted > 0)) {
        toast({ title: t("adminRegcodes.deleteSuccess"), variant: "success" });
        setSelectedCodes((prev) => {
          const next = new Set(prev);
          next.delete(code.code);
          return next;
        });
        await loadRegcodes();
      } else {
        toast({ title: t("adminRegcodes.deleteFailed"), description: res.data?.missing_codes?.length ? t("adminRegcodes.codeNotFound") : res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminRegcodes.deleteFailed"), description: error.message, variant: "destructive" });
    } finally {
      setDeletingCode(null);
    }
  };

  const handleBatchDelete = async () => {
    const selectedItems = regcodes.filter((item) => selectedCodes.has(item.code));
    const codes = selectedItems.map((item) => item.code);
    if (codes.length === 0) {
      toast({ title: t("adminRegcodes.selectToDelete"), variant: "destructive" });
      return;
    }
    const preview = codes.slice(0, 6).join("\n");
    const ok = await confirm({
      title: t("adminRegcodes.batchDeleteTitle", { count: codes.length }),
      description: t("adminRegcodes.batchDeleteDesc", { preview, more: codes.length > 6 ? t("adminRegcodes.batchDeleteMore", { count: codes.length - 6 }) : "" }),
      tone: "danger",
      confirmLabel: t("adminRegcodes.batchDelete"),
    });
    if (!ok) return;

    setIsBatchDeleting(true);
    try {
      const res = await api.batchDeleteRegcodes(codes);
      if (res.success && res.data) {
        toast({
          title: t("adminRegcodes.batchDeleteComplete"),
          description: t("adminRegcodes.batchDeleteResult", { deleted: res.data.deleted, missing: res.data.missing }),
          variant: "success",
        });
        setSelectedCodes(new Set());
        await loadRegcodes();
      } else {
        toast({ title: t("adminRegcodes.batchDeleteFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminRegcodes.batchDeleteFailed"), description: error.message, variant: "destructive" });
    } finally {
      setIsBatchDeleting(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    toast({ title: t("common.copiedToClipboard") });
  };

  const copyRegcodes = (items: Regcode[], emptyMessage?: string) => {
    if (items.length === 0) {
      toast({ title: emptyMessage ?? t("adminRegcodes.noCodesToCopy"), variant: "destructive" });
      return;
    }
    navigator.clipboard.writeText(items.map((item) => item.code).join("\n"));
    toast({ title: t("adminRegcodes.copiedCount", { count: items.length }) });
  };

  const clearFilters = () => {
    setFilterType("all");
    setFilterStatus("all");
    setSearch("");
    setSort("created_time");
    setOrder("desc");
    setPage(1);
  };

  const handleSaveNote = async (code: string) => {
    setSavingNote(code);
    try {
      const note = (noteDrafts[code] || "").trim();
      const res = await api.updateRegcode(code, { note });
      if (res.success) {
        toast({ title: t("adminRegcodes.noteSaved"), variant: "success" });
        setRegcodes((prev) => prev.map((item) => item.code === code ? { ...item, note } : item));
      } else {
        toast({ title: t("adminRegcodes.saveFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminRegcodes.saveFailed"), description: error.message, variant: "destructive" });
    } finally {
      setSavingNote(null);
    }
  };

  const openUsageDialog = async (code: Regcode) => {
    setUsageCode(code);
    setUsageOpen(true);
    setUsageUsers([]);
    setUsageTelegramOnly([]);
    setUsageLoading(true);
    try {
      const res = await api.getRegcodeUsers(code.code);
      if (res.success && res.data) {
        setUsageUsers(res.data.users || []);
        setUsageTelegramOnly(res.data.telegram_only || []);
      } else {
        toast({ title: t("adminRegcodes.loadUsersFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminRegcodes.loadUsersFailed"), description: error.message, variant: "destructive" });
    } finally {
      setUsageLoading(false);
    }
  };

  const handleClearUsage = async (code: string) => {
    const ok = await confirm({
      title: t("adminRegcodes.clearUsageTitle"),
      description: t("adminRegcodes.clearUsageDesc", { code }),
      tone: "danger",
      confirmLabel: t("adminRegcodes.clearUsageConfirm"),
    });
    if (!ok) return;
    setClearingUsage(true);
    try {
      const res = await api.clearRegcodeUsage(code);
      if (res.success) {
        toast({ title: t("adminRegcodes.usageCleared"), description: t("adminRegcodes.usageClearedDesc", { count: res.data?.cleared_use_count || 0 }), variant: "success" });
        setUsageOpen(false);
        loadRegcodes();
      } else {
        toast({ title: t("adminRegcodes.clearFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminRegcodes.clearFailed"), description: error.message, variant: "destructive" });
    } finally {
      setClearingUsage(false);
    }
  };

  const loadInviteCodes = useCallback(async () => {
    setInviteCodesLoading(true);
    try {
      const res = await api.getAdminInviteCodes();
      if (res.success && res.data) {
        setInviteCodes(res.data.codes || []);
        setInviteCodesTotal(res.data.total || 0);
        setInviteCodesLoaded(true);
      } else {
        toast({ title: t("adminRegcodes.loadInviteFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminRegcodes.loadInviteFailed"), description: error.message, variant: "destructive" });
    } finally {
      setInviteCodesLoading(false);
    }
  }, [toast, t]);

  useEffect(() => {
    if (viewMode === "invitecodes" && !inviteCodesLoaded && !inviteCodesLoading) {
      void loadInviteCodes();
    }
  }, [viewMode, inviteCodesLoaded, inviteCodesLoading, loadInviteCodes]);

  useEffect(() => {
    const visibleCodes = new Set(regcodes.map((item) => item.code));
    setSelectedCodes((prev) => {
      const next = new Set(Array.from(prev).filter((code) => visibleCodes.has(code)));
      return next.size === prev.size ? prev : next;
    });
  }, [regcodes]);

  const selectedRegcodes = regcodes.filter((item) => selectedCodes.has(item.code));

  const toggleSelectAll = (checked: boolean) => {
    setSelectedCodes(checked ? new Set(regcodes.map((item) => item.code)) : new Set());
  };

  // 反选：对当前页可见卡码逐个翻转选中态（选中列表已被 useEffect 收窄到当前页）。
  const invertSelection = () => {
    setSelectedCodes((prev) => {
      const next = new Set<string>();
      regcodes.forEach((item) => {
        if (!prev.has(item.code)) next.add(item.code);
      });
      return next;
    });
  };

  const toggleSelectCode = (code: string, checked: boolean) => {
    setSelectedCodes((prev) => {
      const next = new Set(prev);
      if (checked) next.add(code);
      else next.delete(code);
      return next;
    });
  };

  const downloadText = (filename: string, content: string, type: string) => {
    const blob = new Blob([content], { type });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
  };

  const exportSelected = (format: "json" | "txt") => {
    const items = selectedRegcodes.length > 0 ? selectedRegcodes : regcodes;
    if (items.length === 0) return;
    const stamp = new Date().toISOString().replace(/[:.]/g, "-");
    if (format === "json") {
      downloadText(`regcodes-${stamp}.json`, JSON.stringify(items, null, 2), "application/json;charset=utf-8");
    } else {
      downloadText(`regcodes-${stamp}.txt`, items.map((item) => item.code).join("\n"), "text/plain;charset=utf-8");
    }
  };

  const getTypeBadge = (type: number) => {
    // 改用语义令牌，方便统一主题切换。
    switch (type) {
      case 1:
        return <Badge variant="secondary" className="bg-info/10 text-info border-info/20">{t("adminRegcodes.badgeRegcode")}</Badge>;
      case 2:
        return <Badge variant="default" className="bg-warning/10 text-warning border-warning/20">{t("adminRegcodes.badgeRenewcode")}</Badge>;
      case 3:
        return <Badge variant="success" className="bg-success/10 text-success border-success/20">{t("adminRegcodes.badgeWhitelist")}</Badge>;
      default:
        return <Badge variant="secondary">{t("adminRegcodes.badgeUnknown")}</Badge>;
    }
  };

  const getStatusBadge = (code: Regcode) => {
    const status = code.status || (code.active === false ? "disabled" : "available");
    if (code.is_decoy) return <Badge variant="destructive">{t("adminRegcodes.badgeDecoy")}</Badge>;
    if (status === "disabled") return <Badge variant="destructive">{t("adminRegcodes.statusDisabled")}</Badge>;
    if (status === "used_up") return <Badge variant="warning">{t("adminRegcodes.badgeUsedUp")}</Badge>;
    if (status === "expired") return <Badge variant="secondary">{t("adminRegcodes.badgeExpired")}</Badge>;
    return <Badge variant="success">{t("adminRegcodes.badgeAvailable")}</Badge>;
  };

  const activeTypeDescription = {
    title: t(`adminRegcodes.type${activeTab}Title` as MessageKey),
    description: t(`adminRegcodes.type${activeTab}Desc` as MessageKey),
    defaults: t(`adminRegcodes.type${activeTab}Defaults` as MessageKey),
  };

  const selectedAlgorithmDescription = createData.randomAlgorithm
    ? t(`adminRegcodes.algoDesc_${createData.randomAlgorithm}` as MessageKey)
    : t("adminRegcodes.algoDescDefault");

  const pages = Math.ceil(total / 20);

  const usedCount = (code: Regcode) => {
    const byUid = code.used_by_uids?.length || 0;
    const byTg = code.used_by_telegram_ids?.length || 0;
    return Math.max(byUid, byTg, code.use_count || 0);
  };

  const userLabel = (username?: string | null, uid?: number | null) => {
    if (username && uid) return `${username} · UID ${uid}`;
    if (username) return username;
    if (uid) return `UID ${uid}`;
    return "-";
  };

  const resolvedTargetSuffix = (code: Regcode) => {
    if (code.target_resolved_username && code.target_uid) return ` · ${code.target_resolved_username} / UID ${code.target_uid}`;
    if (code.target_resolved_username) return ` · ${code.target_resolved_username}`;
    if (code.target_uid) return ` · UID ${code.target_uid}`;
    return "";
  };

  const regcodeTargetLabel = (code: Regcode) => {
    if (code.target_username) return `Web: ${userLabel(code.target_username, code.target_uid)}`;
    if (code.target_telegram_username) return `TG: @${code.target_telegram_username}${resolvedTargetSuffix(code)}`;
    if (code.target_telegram_id) return `TG ID: ${code.target_telegram_id}${resolvedTargetSuffix(code)}`;
    return "";
  };

  const regcodeUsedByLabel = (code: Regcode) => {
    if (code.used_by_usernames?.length) return code.used_by_usernames.join("、");
    if (code.used_by_uids?.length) return code.used_by_uids.map((uid) => `UID ${uid}`).join("、");
    return "";
  };

  if (error) {
    return <PageError message={error} onRetry={() => void loadRegcodes()} />;
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl sm:text-3xl font-bold">{t("adminRegcodes.pageTitle")}</h1>
          <p className="text-sm text-muted-foreground">{t("adminRegcodes.pageSubtitle")}</p>
        </div>
        <div className="flex gap-2">
          <div className="flex rounded-lg border bg-muted/30 p-0.5">
            <Button
              variant={viewMode === "regcodes" ? "default" : "ghost"}
              size="sm"
              className="rounded-md px-3"
              onClick={() => setViewMode("regcodes")}
            >
              <FileText className="mr-1.5 h-3.5 w-3.5" />
              {t("adminRegcodes.tabRegcodes")}
            </Button>
            <Button
              variant={viewMode === "invitecodes" ? "default" : "ghost"}
              size="sm"
              className="rounded-md px-3"
              onClick={() => setViewMode("invitecodes")}
            >
              <Link2 className="mr-1.5 h-3.5 w-3.5" />
              {t("adminRegcodes.tabInviteCodes")}
            </Button>
          </div>
          {viewMode === "regcodes" && (
            <Dialog open={createOpen} onOpenChange={(open) => {
              setCreateOpen(open);
              if (!open) {
                setCreatedCodes([]);
              }
            }}>
              <DialogTrigger asChild>
                <Button variant="default" className="rounded-xl shadow-lg shadow-primary/20">
                  <Plus className="mr-2 h-4 w-4" />
                  {t("adminRegcodes.generateCard")}
                </Button>
              </DialogTrigger>
          <DialogContent className="max-w-md">
            <DialogHeader>
              <DialogTitle>{t("adminRegcodes.createTitle")}</DialogTitle>
              <DialogDescription>{t("adminRegcodes.createDesc")}</DialogDescription>
            </DialogHeader>
            
            <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
              <TabsList className="grid w-full grid-cols-3 mb-4">
                <TabsTrigger value="1">{t("adminRegcodes.tabRegister")}</TabsTrigger>
                <TabsTrigger value="2">{t("adminRegcodes.tabRenew")}</TabsTrigger>
                <TabsTrigger value="3">{t("adminRegcodes.tabWhitelist")}</TabsTrigger>
              </TabsList>

                <div className="space-y-4 py-2">
                <div className="rounded-xl border border-primary/20 bg-primary/5 p-3 text-xs text-muted-foreground">
                  <div className="font-medium text-foreground">{activeTypeDescription.title}</div>
                  <p className="mt-1">{activeTypeDescription.description}</p>
                  <p className="mt-1">{activeTypeDescription.defaults}</p>
                  <p className="mt-1">{t("adminRegcodes.typeValueHint")}</p>
                </div>
                <div className="space-y-2">
                  <Label>{activeTab === "3" ? t("adminRegcodes.daysLabelWhitelist") : t("adminRegcodes.daysLabelAccount")}</Label>
                  <div className="flex items-center justify-between rounded-md border border-border/80 bg-muted/40 px-3 py-2">
                    <span className="text-xs text-muted-foreground">{t("adminRegcodes.permanentSwitchLabel")}</span>
                    <Switch
                      checked={isPermanentDays}
                      onCheckedChange={(checked) => {
                        setIsPermanentDays(checked);
                        if (checked) {
                          setCreateData((prev) => ({ ...prev, days: "-1" }));
                        }
                      }}
                    />
                  </div>
                  <Input
                    type="number"
                    value={createData.days}
                    onChange={(e) => setCreateData({ ...createData, days: e.target.value })}
                    disabled={isPermanentDays}
                  />
                  <p className="text-[11px] text-muted-foreground">
                    {activeTab === "3" ? t("adminRegcodes.daysHelpWhitelist") : t("adminRegcodes.daysHelpAccount")}
                  </p>
                </div>
                
                <div className="space-y-2">
                  <Label>{t("adminRegcodes.validityLabel")}</Label>
                  <Input
                    type="number"
                    value={createData.validityTime}
                    onChange={(e) => setCreateData({ ...createData, validityTime: e.target.value })}
                  />
                  <p className="text-[11px] text-muted-foreground">
                    {t("adminRegcodes.validityHelp")}
                  </p>
                </div>

                <div className="space-y-2">
                  <Label>{t("adminRegcodes.useCountLimitLabel")}</Label>
                  <Input
                    type="number"
                    value={createData.useCountLimit}
                    onChange={(e) => setCreateData({ ...createData, useCountLimit: e.target.value })}
                  />
                  <p className="text-[11px] text-muted-foreground">
                    {t("adminRegcodes.useCountLimitHelp")}
                  </p>
                </div>

                <div className="space-y-2">
                  <Label>{t("adminRegcodes.countLabel")}</Label>
                  <Input
                    type="number"
                    value={createData.count}
                    onChange={(e) => setCreateData({ ...createData, count: e.target.value })}
                    min="1"
                  />
                </div>

                <div className="space-y-2">
                  <Label>{t("adminRegcodes.targetUsernameLabel")}</Label>
                  <Input
                    value={createData.targetUsername}
                    onChange={(e) => setCreateData({ ...createData, targetUsername: e.target.value })}
                    placeholder={t("adminRegcodes.targetUsernamePlaceholder")}
                  />
                  <p className="text-[11px] text-muted-foreground">
                    {t("adminRegcodes.targetUsernameHelp")}
                  </p>
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label>{t("adminRegcodes.targetTgUsernameLabel")}</Label>
                    <Input
                      value={createData.targetTelegramUsername}
                      onChange={(e) => setCreateData({ ...createData, targetTelegramUsername: e.target.value })}
                      placeholder={t("adminRegcodes.targetTgUsernamePlaceholder")}
                    />
                    <p className="text-[11px] text-muted-foreground">{t("adminRegcodes.targetTgUsernameHelp")}</p>
                  </div>
                  <div className="space-y-2">
                    <Label>{t("adminRegcodes.targetTgIdLabel")}</Label>
                    <Input
                      value={createData.targetTelegramId}
                      onChange={(e) => setCreateData({ ...createData, targetTelegramId: e.target.value })}
                      placeholder={t("adminRegcodes.targetTgIdPlaceholder")}
                      inputMode="numeric"
                    />
                    <p className="text-[11px] text-muted-foreground">{t("adminRegcodes.targetTgIdHelp")}</p>
                  </div>
                </div>

                <div className="space-y-2 rounded-xl border border-border/80 bg-muted/30 p-3">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <Label>{t("adminRegcodes.decoyLabel")}</Label>
                      <p className="mt-1 text-[11px] text-muted-foreground">
                        {t("adminRegcodes.decoyHelp")}
                      </p>
                    </div>
                    <Switch checked={createDecoy} onCheckedChange={setCreateDecoy} />
                  </div>
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label>{t("adminRegcodes.randomAlgoLabel")}</Label>
                    <Select value={createData.randomAlgorithm || "default"} onValueChange={(v) => setCreateData({ ...createData, randomAlgorithm: v === "default" ? "" : v })}>
                      <SelectTrigger><SelectValue placeholder={t("adminRegcodes.randomAlgoPlaceholder")} /></SelectTrigger>
                      <SelectContent>
                        <SelectItem value="default">{t("adminRegcodes.algoDefault")}</SelectItem>
                        <SelectItem value="base32-20">{t("adminRegcodes.algoBase32_20")}</SelectItem>
                        <SelectItem value="base32-24">{t("adminRegcodes.algoBase32_24")}</SelectItem>
                        <SelectItem value="base32-32">{t("adminRegcodes.algoBase32_32")}</SelectItem>
                        <SelectItem value="hex32">{t("adminRegcodes.algoHex32")}</SelectItem>
                        <SelectItem value="hex40">{t("adminRegcodes.algoHex40")}</SelectItem>
                        <SelectItem value="hex20">{t("adminRegcodes.algoHex20")}</SelectItem>
                        <SelectItem value="base32-16">{t("adminRegcodes.algoBase32_16")}</SelectItem>
                        <SelectItem value="alnum-24">{t("adminRegcodes.algoAlnum24")}</SelectItem>
                        <SelectItem value="alnum-16">{t("adminRegcodes.algoAlnum16")}</SelectItem>
                        <SelectItem value="alnum-32">{t("adminRegcodes.algoAlnum32")}</SelectItem>
                        <SelectItem value="urlsafe-24">{t("adminRegcodes.algoUrlsafe24")}</SelectItem>
                        <SelectItem value="urlsafe-32">{t("adminRegcodes.algoUrlsafe32")}</SelectItem>
                        <SelectItem value="digits-16">{t("adminRegcodes.algoDigits16")}</SelectItem>
                        <SelectItem value="digits-12">{t("adminRegcodes.algoDigits12")}</SelectItem>
                        <SelectItem value="symbols-16">{t("adminRegcodes.algoSymbols16")}</SelectItem>
                        <SelectItem value="symbols-24">{t("adminRegcodes.algoSymbols24")}</SelectItem>
                        <SelectItem value="uuid">{t("adminRegcodes.algoUuid")}</SelectItem>
                        <SelectItem value="legacy-sha1">{t("adminRegcodes.algoLegacySha1")}</SelectItem>
                      </SelectContent>
                    </Select>
                    <p className="text-[11px] text-muted-foreground">{selectedAlgorithmDescription}</p>
                  </div>
                  <div className="space-y-2">
                    <Label>{t("adminRegcodes.customFormatLabel")}</Label>
                    <Input
                      value={createData.format}
                      onChange={(e) => setCreateData({ ...createData, format: e.target.value })}
                      placeholder={t("adminRegcodes.customFormatPlaceholder")}
                    />
                  </div>
                </div>
                <div className="rounded-xl border bg-muted/30 p-3 text-[11px] text-muted-foreground">
                  <p className="font-medium text-foreground">{t("adminRegcodes.formatHintTitle")}</p>
                  <p className="mt-1">{t("adminRegcodes.formatHintRandom")}</p>
                  <p>{t("adminRegcodes.formatHintType")}</p>
                  <p>{t("adminRegcodes.formatHintDays")}</p>
                  <p>{t("adminRegcodes.formatHintIndex")}</p>
                  <p>{t("adminRegcodes.formatHintValidityLimit")}</p>
                  <p className="mt-1">{t("adminRegcodes.formatHintFallback")}</p>
                </div>

                {createdCodes.length > 0 && (
                  <div className="mt-4 space-y-2 p-3 bg-muted/50 rounded-xl border border-border">
                    <div className="flex items-center justify-between gap-2">
                      <Label className="text-xs">{t("adminRegcodes.generatedCodesLabel")}</Label>
                      <Button size="sm" variant="outline" className="h-7 px-2 text-xs" onClick={() => {
                        navigator.clipboard.writeText(createdCodes.join("\n"));
                        toast({ title: t("adminRegcodes.copiedCount", { count: createdCodes.length }) });
                      }}>
                        <Copy className="mr-1 h-3.5 w-3.5" /> {t("adminRegcodes.copyAll")}
                      </Button>
                    </div>
                    <div className="max-h-40 overflow-y-auto space-y-2 pr-1">
                      {createdCodes.map((code) => (
                        <div key={code} className="flex items-center gap-2 group">
                          <code className="flex-1 text-[12px] font-mono bg-background px-2 py-1.5 rounded-lg border border-border group-hover:border-primary/50 transition-colors">
                            {code}
                          </code>
                          <Button size="icon" variant="ghost" className="h-8 w-8 hover:bg-primary/10 hover:text-primary" onClick={() => copyToClipboard(code)}>
                            <Copy className="h-3.5 w-3.5" />
                          </Button>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            </Tabs>

            <DialogFooter className="mt-4">
              <Button variant="outline" onClick={() => setCreateOpen(false)}>
                {t("common.cancel")}
              </Button>
              <Button onClick={handleCreate} disabled={isCreating} className="min-w-[80px]">
                {isCreating ? <Loader2 className="h-4 w-4 animate-spin" /> : t("adminRegcodes.generateNow")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
          )}
        </div>
      </div>

      {viewMode === "invitecodes" ? (
        <Card>
          <CardHeader className="flex flex-row items-center justify-between gap-3 pb-3">
            <CardTitle className="text-lg">{t("adminRegcodes.inviteListTitle")}</CardTitle>
            <div className="flex items-center gap-2">
              <div className="relative">
                <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder={t("adminRegcodes.inviteSearchPlaceholder")}
                  value={inviteSearch}
                  onChange={(e) => setInviteSearch(e.target.value)}
                  className="h-8 w-48 pl-8 text-xs"
                />
              </div>
              <Button variant="outline" size="sm" onClick={() => void loadInviteCodes()} disabled={inviteCodesLoading}>
                {inviteCodesLoading ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="mr-1.5 h-3.5 w-3.5" />}
                {t("adminRegcodes.refresh")}
              </Button>
            </div>
          </CardHeader>
          <CardContent className="p-0">
            {inviteCodesLoading && !inviteCodesLoaded ? (
              <div className="flex h-64 items-center justify-center">
                <Loader2 className="h-8 w-8 animate-spin text-primary" />
              </div>
            ) : inviteCodes.length === 0 ? (
              <div className="flex h-64 items-center justify-center text-muted-foreground">
                {t("adminRegcodes.noInviteCodes")}
              </div>
            ) : (() => {
              const lowerSearch = inviteSearch.toLowerCase().trim();
              const filtered = lowerSearch
                ? inviteCodes.filter((c) =>
                    c.code.toLowerCase().includes(lowerSearch) ||
                    String(c.inviter_uid).includes(lowerSearch) ||
                    (c.inviter_username && c.inviter_username.toLowerCase().includes(lowerSearch)) ||
                    (c.target_username && c.target_username.toLowerCase().includes(lowerSearch)) ||
                    (c.target_uid && String(c.target_uid).includes(lowerSearch)) ||
                    (c.used_by_uid && String(c.used_by_uid).includes(lowerSearch)) ||
                    (c.used_by_username && c.used_by_username.toLowerCase().includes(lowerSearch))
                  )
                : inviteCodes;
              if (filtered.length === 0) {
                return (
                  <div className="flex h-40 items-center justify-center text-muted-foreground">
                    {t("adminRegcodes.noMatchInviteCodes")}
                  </div>
                );
              }
              return (
              <>
                <div className="space-y-3 p-3 md:hidden">
                  {filtered.map((code) => (
                    <div key={code.code} className="rounded-xl border bg-background p-4 shadow-sm">
                      <div className="flex items-center gap-2">
                        <code className="flex-1 truncate rounded bg-muted px-2 py-1 text-sm">{code.code}</code>
                        <Button size="icon" variant="ghost" className="h-7 w-7 shrink-0" onClick={() => copyToClipboard(code.code)}>
                          <Copy className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                      <div className="mt-3 grid grid-cols-2 gap-3 text-sm">
                        <div>
                          <p className="text-xs text-muted-foreground">{t("adminRegcodes.colInviter")}</p>
                          <p className="mt-1">{userLabel(code.inviter_username, code.inviter_uid)}</p>
                        </div>
                        <div>
                          <p className="text-xs text-muted-foreground">{t("adminRegcodes.colDays")}</p>
                          <p className="mt-1">{formatRegcodeDays(code.days)}</p>
                        </div>
                        <div>
                          <p className="text-xs text-muted-foreground">{t("adminRegcodes.colUseCount")}</p>
                          <p className="mt-1">{code.use_count} / {code.use_count_limit === -1 ? "∞" : code.use_count_limit}</p>
                        </div>
                        <div>
                          <p className="text-xs text-muted-foreground">{t("adminRegcodes.colStatus")}</p>
                          <p className="mt-1">
                            {code.active ? <Badge variant="success">{t("adminRegcodes.statusAvailable")}</Badge> : <Badge variant="destructive">{t("adminRegcodes.statusDisabled")}</Badge>}
                          </p>
                        </div>
                        {code.target_username && (
                          <div className="col-span-2">
                            <p className="text-xs text-muted-foreground">{t("adminRegcodes.colTargetUser")}</p>
                            <p className="mt-1">{userLabel(code.target_username, code.target_uid)}</p>
                          </div>
                        )}
                        {code.used_by_uid && (
                          <div>
                            <p className="text-xs text-muted-foreground">{t("adminRegcodes.colUsedBy")}</p>
                            <p className="mt-1">{userLabel(code.used_by_username, code.used_by_uid)}</p>
                          </div>
                        )}
                        <div>
                          <p className="text-xs text-muted-foreground">{t("adminRegcodes.createdAt")}</p>
                          <p className="mt-1">{formatDate(code.created_at)}</p>
                        </div>
                        {code.expires_at && (
                          <div>
                            <p className="text-xs text-muted-foreground">{t("adminRegcodes.colExpiresAt")}</p>
                            <p className="mt-1">{formatDate(code.expires_at)}</p>
                          </div>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
                <div className="hidden overflow-x-auto md:block">
                  <table className="w-full min-w-[900px]">
                    <thead>
                      <tr className="border-b bg-muted/50">
                        <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colInviteCode")}</th>
                        <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colInviter")}</th>
                        <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colDays")}</th>
                        <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colUseCount")}</th>
                        <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colTargetUser")}</th>
                        <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colUsedBy")}</th>
                        <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colStatus")}</th>
                        <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colExpiresAt")}</th>
                        <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colCreatedAt")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filtered.map((code) => (
                        <tr key={code.code} className="border-b hover:bg-muted/30">
                          <td className="px-4 py-3">
                            <div className="flex items-center gap-2">
                              <code className="rounded bg-muted px-2 py-1 text-sm">{code.code}</code>
                              <Button size="icon" variant="ghost" className="h-6 w-6" onClick={() => copyToClipboard(code.code)}>
                                <Copy className="h-3 w-3" />
                              </Button>
                            </div>
                          </td>
                          <td className="px-4 py-3 text-sm">{userLabel(code.inviter_username, code.inviter_uid)}</td>
                          <td className="px-4 py-3 text-sm">{formatRegcodeDays(code.days)}</td>
                          <td className="px-4 py-3 text-sm">{code.use_count} / {code.use_count_limit === -1 ? "∞" : code.use_count_limit}</td>
                          <td className="px-4 py-3 text-sm">{userLabel(code.target_username, code.target_uid)}</td>
                          <td className="px-4 py-3 text-sm">{userLabel(code.used_by_username, code.used_by_uid)}</td>
                          <td className="px-4 py-3">
                            {code.active ? <Badge variant="success">{t("adminRegcodes.statusAvailable")}</Badge> : <Badge variant="destructive">{t("adminRegcodes.statusDisabled")}</Badge>}
                          </td>
                          <td className="px-4 py-3 text-sm text-muted-foreground">{code.expires_at ? formatDate(code.expires_at) : t("adminRegcodes.permanent")}</td>
                          <td className="px-4 py-3 text-sm text-muted-foreground">{formatDate(code.created_at)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <div className="p-3 text-center text-sm text-muted-foreground">
                  {lowerSearch ? t("adminRegcodes.matchInviteCount", { count: filtered.length, total: inviteCodesTotal }) : t("adminRegcodes.totalInviteCount", { total: inviteCodesTotal })}
                </div>
              </>
              );
            })()}
          </CardContent>
        </Card>
      ) : (
      <>
      <div className="flex flex-col gap-2 rounded-xl border bg-muted/30 p-3 text-sm sm:flex-row sm:items-center sm:justify-between">
        <span className="text-muted-foreground">
          {t("adminRegcodes.selectionSummary", { selected: selectedRegcodes.length, total: regcodes.length })}
        </span>
        <div className="flex w-full flex-wrap gap-2 sm:w-auto">
          <Button className="flex-1 sm:flex-none" variant="outline" size="sm" onClick={invertSelection} disabled={regcodes.length === 0}>
            <FlipHorizontal2 className="mr-2 h-4 w-4" /> {t("adminRegcodes.invertSelection")}
          </Button>
          <Button className="flex-1 sm:flex-none" variant="outline" size="sm" onClick={() => copyRegcodes(selectedRegcodes.length > 0 ? selectedRegcodes : regcodes)} disabled={regcodes.length === 0}>
            <Copy className="mr-2 h-4 w-4" /> {t("adminRegcodes.copyCodes")}
          </Button>
          <Button className="flex-1 sm:flex-none" variant="outline" size="sm" onClick={() => exportSelected("txt")} disabled={regcodes.length === 0}>
            <Download className="mr-2 h-4 w-4" /> {t("adminRegcodes.exportTxt")}
          </Button>
          <Button className="flex-1 sm:flex-none" variant="outline" size="sm" onClick={() => exportSelected("json")} disabled={regcodes.length === 0}>
            <Download className="mr-2 h-4 w-4" /> {t("adminRegcodes.exportJson")}
          </Button>
          <Button
            className="flex-1 sm:flex-none"
            variant="destructive"
            size="sm"
            onClick={() => void handleBatchDelete()}
            disabled={selectedRegcodes.length === 0 || isBatchDeleting}
          >
            {isBatchDeleting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Trash2 className="mr-2 h-4 w-4" />}
            {t("adminRegcodes.batchDelete")}
          </Button>
        </div>
      </div>

      <Card>
        <CardContent className="grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-[minmax(240px,1.2fr)_0.8fr_0.8fr_0.8fr_0.7fr_auto]">
          <Input
            placeholder={t("adminRegcodes.searchRegcodePlaceholder")}
            value={search}
            onChange={(e) => { setSearch(e.target.value); setPage(1); }}
          />
          <Select value={filterType} onValueChange={(v) => { setFilterType(v); setPage(1); }}>
            <SelectTrigger><SelectValue placeholder={t("adminRegcodes.filterTypePlaceholder")} /></SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("adminRegcodes.allTypes")}</SelectItem>
              <SelectItem value="1">{t("adminRegcodes.typeRegcode")}</SelectItem>
              <SelectItem value="2">{t("adminRegcodes.typeRenewcode")}</SelectItem>
              <SelectItem value="3">{t("adminRegcodes.typeWhitelistcode")}</SelectItem>
            </SelectContent>
          </Select>
          <Select value={filterStatus} onValueChange={(v) => { setFilterStatus(v); setPage(1); }}>
            <SelectTrigger><SelectValue placeholder={t("adminRegcodes.filterStatusPlaceholder")} /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("adminRegcodes.allStatus")}</SelectItem>
                <SelectItem value="available">{t("adminRegcodes.statusAvailable")}</SelectItem>
                <SelectItem value="active">{t("adminRegcodes.statusActive")}</SelectItem>
                <SelectItem value="decoy">{t("adminRegcodes.statusDecoy")}</SelectItem>
                <SelectItem value="used_up">{t("adminRegcodes.statusUsedUp")}</SelectItem>
              <SelectItem value="expired">{t("adminRegcodes.statusExpired")}</SelectItem>
              <SelectItem value="disabled">{t("adminRegcodes.statusDisabled")}</SelectItem>
            </SelectContent>
          </Select>
          <Select value={sort} onValueChange={setSort}>
            <SelectTrigger><SelectValue placeholder={t("adminRegcodes.sortPlaceholder")} /></SelectTrigger>
            <SelectContent>
              <SelectItem value="created_time">{t("adminRegcodes.sortCreatedTime")}</SelectItem>
              <SelectItem value="code">{t("adminRegcodes.sortCode")}</SelectItem>
              <SelectItem value="type">{t("adminRegcodes.sortType")}</SelectItem>
              <SelectItem value="days">{t("adminRegcodes.sortDays")}</SelectItem>
              <SelectItem value="use_count">{t("adminRegcodes.sortUseCount")}</SelectItem>
              <SelectItem value="note">{t("adminRegcodes.sortNote")}</SelectItem>
            </SelectContent>
          </Select>
          <Select value={order} onValueChange={setOrder}>
            <SelectTrigger><SelectValue placeholder={t("adminRegcodes.orderPlaceholder")} /></SelectTrigger>
            <SelectContent>
              <SelectItem value="desc">{t("adminRegcodes.orderDesc")}</SelectItem>
              <SelectItem value="asc">{t("adminRegcodes.orderAsc")}</SelectItem>
            </SelectContent>
          </Select>
          <div className="flex gap-2 sm:col-span-2 lg:col-span-3 xl:col-span-1 xl:min-w-0">
            <Button className="flex-1 md:flex-none" variant="outline" onClick={clearFilters}>{t("adminRegcodes.reset")}</Button>
            <Button className="flex-1 md:flex-none" variant="outline" onClick={() => void loadRegcodes()}>{t("adminRegcodes.refresh")}</Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="flex h-64 items-center justify-center">
              <Loader2 className="h-8 w-8 animate-spin text-primary" />
            </div>
          ) : !regcodes || regcodes.length === 0 ? (
            <div className="flex h-64 items-center justify-center text-muted-foreground">
              {t("adminRegcodes.noRegcodes")}
            </div>
          ) : (
            <>
            <div className="space-y-3 p-3 md:hidden">
              {regcodes.map((code) => (
                <div key={code.code} className="rounded-xl border bg-background p-4 shadow-sm">
                  <div className="flex items-start justify-between gap-3">
                    <label className="flex min-w-0 flex-1 items-start gap-3">
                      <input
                        type="checkbox"
                        checked={selectedCodes.has(code.code)}
                        onChange={(e) => toggleSelectCode(code.code, e.target.checked)}
                        className="mt-1 shrink-0"
                      />
                      <span className="min-w-0">
                        <code className="block truncate rounded bg-muted px-2 py-1 text-sm" title={code.code}>
                          {code.code}
                        </code>
                        <span className="mt-2 flex flex-wrap gap-1">
                          {getTypeBadge(code.type)}
                          {getStatusBadge(code)}
                          {regcodeTargetLabel(code) ? <Badge variant="secondary">{t("adminRegcodes.onlyTargetPrefix")}{regcodeTargetLabel(code)}</Badge> : null}
                          {code.is_decoy ? <Badge variant="destructive">{t("adminRegcodes.badgeFake")}</Badge> : <Badge variant="outline">{t("adminRegcodes.badgeNormal")}</Badge>}
                        </span>
                      </span>
                    </label>
                    <div className="flex shrink-0 items-center gap-1">
                      <Button
                        size="icon"
                        variant="ghost"
                        className="h-8 w-8"
                        onClick={() => copyToClipboard(code.code)}
                      >
                        <Copy className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        size="icon"
                        variant="ghost"
                        className="h-8 w-8 text-destructive hover:text-destructive"
                        onClick={() => void handleDelete(code)}
                        disabled={deletingCode === code.code}
                      >
                        {deletingCode === code.code ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
                      </Button>
                    </div>
                  </div>

                  <div className="mt-3 grid grid-cols-2 gap-3 text-sm">
                    <div>
                      <p className="text-xs text-muted-foreground">{t("adminRegcodes.accountValid")}</p>
                      <p className="mt-1">{formatRegcodeDays(code.days)}</p>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">{t("adminRegcodes.codeValid")}</p>
                      <p className="mt-1">
                        {code.validity_time === -1 || code.validity_time === undefined
                          ? t("adminRegcodes.permanentValid")
                          : t("adminRegcodes.hoursValue", { hours: code.validity_time })}
                      </p>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">{t("adminRegcodes.usageCount")}</p>
                      <p className="mt-1">{code.use_count || 0} / {code.use_count_limit === -1 ? "∞" : code.use_count_limit || "∞"}</p>
                    </div>
                    {regcodeUsedByLabel(code) && (
                      <div className="col-span-2">
                        <p className="text-xs text-muted-foreground">{t("adminRegcodes.usedByLabel")}</p>
                        <p className="mt-1 break-all">{regcodeUsedByLabel(code)}</p>
                      </div>
                    )}
                    <div>
                      <p className="text-xs text-muted-foreground">{t("adminRegcodes.createdAt")}</p>
                      <p className="mt-1">{formatDate(code.created_time || code.created_at)}</p>
                    </div>
                  </div>

                  <div className="mt-3 flex gap-2 border-t pt-3">
                    <Input
                      value={noteDrafts[code.code] ?? code.note ?? ""}
                      maxLength={120}
                      placeholder={t("adminRegcodes.notePlaceholder")}
                      className="h-9 min-w-0 text-xs"
                      onChange={(e) => setNoteDrafts((prev) => ({ ...prev, [code.code]: e.target.value }))}
                    />
                    <Button size="icon" variant="outline" className="h-9 w-9 shrink-0" disabled={savingNote === code.code} onClick={() => void handleSaveNote(code.code)}>
                      {savingNote === code.code ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
                    </Button>
                    {usedCount(code) > 0 && (
                      <Button
                        variant="outline"
                        size="icon"
                        className="h-9 w-9 shrink-0"
                        onClick={() => void openUsageDialog(code)}
                        title={t("adminRegcodes.peopleUsed", { count: usedCount(code) })}
                      >
                        <Users className="h-4 w-4" />
                      </Button>
                    )}
                  </div>
                </div>
              ))}
            </div>

            <div className="hidden overflow-x-auto md:block">
              <table className="w-full min-w-[1160px] table-fixed">
                <colgroup>
                  <col className="w-12" />
                  <col className="w-[260px]" />
                  <col className="w-[260px]" />
                  <col className="w-[120px]" />
                  <col className="w-[130px]" />
                  <col className="w-[115px]" />
                  <col className="w-[190px]" />
                  <col className="w-[150px]" />
                  <col className="w-20" />
                </colgroup>
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left text-sm font-medium">
                      <input
                        type="checkbox"
                        checked={regcodes.length > 0 && regcodes.every((item) => selectedCodes.has(item.code))}
                        onChange={(e) => toggleSelectAll(e.target.checked)}
                      />
                    </th>
                    <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colRegcode")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colTypeNote")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colAccountDays")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colCodeValidity")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colUseCount")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colStatusUser")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">{t("adminRegcodes.colCreatedAt")}</th>
                    <th className="px-4 py-3 text-right text-sm font-medium">{t("adminRegcodes.colActions")}</th>
                  </tr>
                </thead>
                <tbody>
                  {regcodes.map((code) => (
                    <tr key={code.code} className="border-b hover:bg-muted/30">
                      <td className="px-4 py-3 align-top">
                        <input
                          type="checkbox"
                          checked={selectedCodes.has(code.code)}
                          onChange={(e) => toggleSelectCode(code.code, e.target.checked)}
                        />
                      </td>
                      <td className="px-4 py-3 align-top">
                        <div className="flex min-w-0 items-center gap-2">
                          <code className="block max-w-[210px] truncate rounded bg-muted px-2 py-1 text-sm" title={code.code}>
                            {code.code}
                          </code>
                          <Button
                            size="icon"
                            variant="ghost"
                            className="h-6 w-6"
                            onClick={() => copyToClipboard(code.code)}
                          >
                            <Copy className="h-3 w-3" />
                          </Button>
                        </div>
                      </td>
                      <td className="px-4 py-3 align-top">
                        <div className="space-y-2">
                          <div className="flex flex-wrap gap-1">
                            {getTypeBadge(code.type)}
                            {code.is_decoy ? <Badge variant="destructive">{t("adminRegcodes.badgeFake")}</Badge> : <Badge variant="outline">{t("adminRegcodes.badgeNormal")}</Badge>}
                          </div>
                          {regcodeTargetLabel(code) ? <div className="truncate text-xs text-muted-foreground" title={regcodeTargetLabel(code)}>{t("adminRegcodes.onlyLimitPrefix")}{regcodeTargetLabel(code)}</div> : null}
                          <div className="flex gap-1">
                            <Input
                              value={noteDrafts[code.code] ?? code.note ?? ""}
                              maxLength={120}
                              placeholder={t("adminRegcodes.notePlaceholder")}
                              className="h-8 text-xs"
                              onChange={(e) => setNoteDrafts((prev) => ({ ...prev, [code.code]: e.target.value }))}
                            />
                            <Button size="icon" variant="ghost" className="h-8 w-8" disabled={savingNote === code.code} onClick={() => void handleSaveNote(code.code)}>
                              {savingNote === code.code ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
                            </Button>
                          </div>
                        </div>
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 align-top text-sm">{formatRegcodeDays(code.days)}</td>
                      <td className="whitespace-nowrap px-4 py-3 align-top text-sm">
                        {code.validity_time === -1 || code.validity_time === undefined 
                          ? t("adminRegcodes.permanentValid") 
                          : t("adminRegcodes.hoursValue", { hours: code.validity_time })}
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 align-top text-sm">
                        {code.use_count || 0} / {code.use_count_limit === -1 ? '∞' : code.use_count_limit || '∞'}
                      </td>
                      <td className="px-4 py-3 align-top">
                        {getStatusBadge(code)}
                        {usedCount(code) > 0 && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="mt-1 h-7 px-2 text-xs text-primary"
                            onClick={() => void openUsageDialog(code)}
                          >
                            <Users className="mr-1 h-3.5 w-3.5" />
                            {t("adminRegcodes.peopleUsed", { count: usedCount(code) })}
                          </Button>
                        )}
                        {regcodeUsedByLabel(code) && (
                          <div className="mt-1 max-w-[180px] truncate text-xs text-muted-foreground" title={regcodeUsedByLabel(code)}>
                            {regcodeUsedByLabel(code)}
                          </div>
                        )}
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 align-top text-sm text-muted-foreground">
                        {formatDate(code.created_time || code.created_at)}
                      </td>
                      <td className="px-4 py-3 text-right align-top">
                        <Button
                          size="icon"
                          variant="ghost"
                          className="text-destructive hover:text-destructive"
                          onClick={() => void handleDelete(code)}
                          disabled={deletingCode === code.code}
                        >
                          {deletingCode === code.code ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            </>
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
            {t("adminRegcodes.pageStatus", { page, pages })}
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
      </>
      )}

      <Dialog open={usageOpen} onOpenChange={setUsageOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("adminRegcodes.usageTitle")}</DialogTitle>
            <DialogDescription>
              {usageCode?.code}
            </DialogDescription>
          </DialogHeader>
          {usageLoading ? (
            <div className="flex h-32 items-center justify-center">
              <Loader2 className="h-6 w-6 animate-spin text-primary" />
            </div>
          ) : usageUsers.length === 0 && usageTelegramOnly.length === 0 ? (
            <div className="rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground">
              {t("adminRegcodes.noUsageRecords")}
            </div>
          ) : (
            <div className="max-h-[60vh] space-y-3 overflow-y-auto pr-1">
              {usageUsers.map((user, index) => (
                <div key={`${user.uid || "missing"}-${index}`} className="rounded-xl border bg-muted/20 p-4">
                  {user.found ? (
                    <div className="grid gap-2 text-sm sm:grid-cols-2">
                      <div className="font-medium">{user.username || t("adminRegcodes.unknownUser")}</div>
                      <div className="text-muted-foreground">UID: {user.uid}</div>
                      <div>{t("adminRegcodes.roleLabel")}{user.role_name || user.role}</div>
                      <div>{t("adminRegcodes.statusLabelColon")}{user.active ? t("adminRegcodes.enabled") : t("adminRegcodes.disabled")}</div>
                      <div>{t("adminRegcodes.telegramColon")}{user.telegram_id ? t("adminRegcodes.bound") : t("adminRegcodes.unbound")}</div>
                      <div>{t("adminRegcodes.embyColon")}{user.emby_id ? t("adminRegcodes.bound") : user.pending_emby ? t("adminRegcodes.pendingEmby") : t("adminRegcodes.unbound")}</div>
                      <div className="sm:col-span-2 text-muted-foreground">{t("adminRegcodes.expireColon")}{user.expire_status || "-"}</div>
                    </div>
                  ) : (
                    <div className="text-sm text-muted-foreground">{t("adminRegcodes.localUserGone", { uid: user.uid })}</div>
                  )}
                </div>
              ))}
              {usageTelegramOnly.map((item) => (
                <div key={`tg-${item.telegram_id}`} className="rounded-xl border border-warning/30 bg-warning/10 p-4 text-sm">
                  <div className="font-medium">{t("adminRegcodes.tgOnlyTitle")}</div>
                  <div className="mt-1 text-muted-foreground">TGID: {item.telegram_id}</div>
                  <div className="mt-1 text-muted-foreground">{t("adminRegcodes.tgOnlyHint")}</div>
                </div>
              ))}
            </div>
          )}
          <DialogFooter className="flex-col gap-2 sm:flex-row">
            {usageCode && (usageUsers.length > 0 || usageTelegramOnly.length > 0) && (
              <Button
                variant="destructive"
                size="sm"
                onClick={() => void handleClearUsage(usageCode.code)}
                disabled={clearingUsage}
              >
                {clearingUsage ? <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" /> : <RotateCcw className="mr-2 h-3.5 w-3.5" />}
                {t("adminRegcodes.clearUsageBtn")}
              </Button>
            )}
            <Button variant="outline" onClick={() => setUsageOpen(false)}>{t("common.close")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

