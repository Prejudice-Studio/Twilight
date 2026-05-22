"use client";

import { useCallback, useEffect, useState } from "react";
import { motion } from "framer-motion";
import {
  FileText,
  Plus,
  Copy,
  Trash2,
  Check,
  Loader2,
  ChevronLeft,
  ChevronRight,
  Download,
  Save,
  Users,
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError } from "@/components/layout/page-state";
import { api, type Regcode, type UserInfo } from "@/lib/api";
import { formatDate } from "@/lib/utils";

export default function AdminRegcodesPage() {
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const [regcodes, setRegcodes] = useState<Regcode[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [selectedCodes, setSelectedCodes] = useState<Set<string>>(new Set());
  const [isBatchDeleting, setIsBatchDeleting] = useState(false);
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
  });
  const [createDecoy, setCreateDecoy] = useState(false);
  const [isPermanentDays, setIsPermanentDays] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const [createdCodes, setCreatedCodes] = useState<string[]>([]);

  const loadRegcodesResource = useCallback(async () => {
    const res = await api.getRegcodes(page, { type: filterType, status: filterStatus, search, sort, order });
    if (res.success && res.data) {
      const regcodesList = Array.isArray(res.data.regcodes)
        ? res.data.regcodes
        : Array.isArray(res.data)
          ? res.data
          : [];
      setRegcodes(regcodesList);
      setNoteDrafts(Object.fromEntries(regcodesList.map((item) => [item.code, item.note || ""])));
      setTotal(res.data.total || regcodesList.length);
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
        setCreateData({ days: "30", validityTime: "-1", useCountLimit: "1", count: "1", format: "", randomAlgorithm: "" });
      } else if (activeTab === "2") {
        setIsPermanentDays(false);
        setCreateData({ days: "30", validityTime: "72", useCountLimit: "1", count: "1", format: "", randomAlgorithm: "" });
      } else {
        setIsPermanentDays(true);
        setCreateData({ days: "-1", validityTime: "-1", useCountLimit: "-1", count: "1", format: "", randomAlgorithm: "" });
      }
  }, [activeTab, createOpen]);

  const handleCreate = async () => {
    const count = parseInt(createData.count, 10);
    if (Number.isNaN(count) || count < 1 || count > 100) {
      toast({ title: "参数错误", description: "生成数量必须在 1-100 之间", variant: "destructive" });
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
      });

      if (res.success && res.data) {
        toast({ title: "生成成功", variant: "success" });
        setCreatedCodes(res.data.codes || []);
        loadRegcodes();
      } else {
        toast({ title: "生成失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "生成失败", description: error.message, variant: "destructive" });
    } finally {
      setIsCreating(false);
    }
  };

  const handleDelete = async (code: string) => {
    const ok = await confirm({
      title: "删除卡码？",
      description: `卡码 ${code} 将被立即删除，且无法恢复。`,
      tone: "danger",
      confirmLabel: "删除",
    });
    if (!ok) return;

    try {
      const res = await api.deleteRegcode(code);
      if (res.success) {
        toast({ title: "删除成功", variant: "success" });
        loadRegcodes();
      } else {
        toast({ title: "删除失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "删除失败", description: error.message, variant: "destructive" });
    }
  };

  const handleBatchDelete = async () => {
    const codes = Array.from(selectedCodes);
    if (codes.length === 0) {
      toast({ title: "请先选择要删除的注册码", variant: "destructive" });
      return;
    }
    const preview = codes.slice(0, 6).join("\n");
    const ok = await confirm({
      title: `批量删除 ${codes.length} 个注册码？`,
      description: `${preview}${codes.length > 6 ? `\n... 另有 ${codes.length - 6} 个` : ""}\n\n删除后无法恢复。`,
      tone: "danger",
      confirmLabel: "批量删除",
    });
    if (!ok) return;

    setIsBatchDeleting(true);
    try {
      const res = await api.batchDeleteRegcodes(codes);
      if (res.success && res.data) {
        toast({
          title: "批量删除完成",
          description: `已删除 ${res.data.deleted} 个，未找到 ${res.data.missing} 个`,
          variant: "success",
        });
        setSelectedCodes(new Set());
        await loadRegcodes();
      } else {
        toast({ title: "批量删除失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "批量删除失败", description: error.message, variant: "destructive" });
    } finally {
      setIsBatchDeleting(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    toast({ title: "已复制到剪贴板" });
  };

  const copyRegcodes = (items: Regcode[], emptyMessage = "没有可复制的卡码") => {
    if (items.length === 0) {
      toast({ title: emptyMessage, variant: "destructive" });
      return;
    }
    navigator.clipboard.writeText(items.map((item) => item.code).join("\n"));
    toast({ title: `已复制 ${items.length} 个卡码` });
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
        toast({ title: "备注已保存", variant: "success" });
        setRegcodes((prev) => prev.map((item) => item.code === code ? { ...item, note } : item));
      } else {
        toast({ title: "保存失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "保存失败", description: error.message, variant: "destructive" });
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
        toast({ title: "加载使用者失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "加载使用者失败", description: error.message, variant: "destructive" });
    } finally {
      setUsageLoading(false);
    }
  };

  const selectedRegcodes = regcodes.filter((item) => selectedCodes.has(item.code));

  const toggleSelectAll = (checked: boolean) => {
    setSelectedCodes(checked ? new Set(regcodes.map((item) => item.code)) : new Set());
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
    switch (type) {
      case 1:
        return <Badge variant="secondary" className="bg-blue-500/10 text-blue-500 border-blue-500/20">注册码</Badge>;
      case 2:
        return <Badge variant="default" className="bg-orange-500/10 text-orange-500 border-orange-500/20">续期码</Badge>;
      case 3:
        return <Badge variant="success" className="bg-emerald-500/10 text-emerald-500 border-emerald-500/20">白名单</Badge>;
      default:
        return <Badge variant="secondary">未知</Badge>;
    }
  };

  const getStatusBadge = (code: Regcode) => {
    const status = code.status || (code.active === false ? "disabled" : "available");
    if (code.is_decoy) return <Badge variant="destructive">诱饵码</Badge>;
    if (status === "disabled") return <Badge variant="destructive">已禁用</Badge>;
    if (status === "used_up") return <Badge variant="warning">已用完</Badge>;
    if (status === "expired") return <Badge variant="secondary">已过期</Badge>;
    return <Badge variant="success">可用</Badge>;
  };

  const typeDescriptions: Record<string, { title: string; description: string; defaults: string }> = {
    "1": {
      title: "注册码",
      description: "用于新用户注册系统账号，并授予首次补建 Emby 账号的开通天数。旧版已生成注册码仍按数据库中的类型、天数和使用次数生效。",
      defaults: "默认：账号 30 天、卡码永久有效、使用 1 次。",
    },
    "2": {
      title: "续期码",
      description: "仅已绑定 Emby 的用户可使用，用于延长账号有效期；永久天数会把到期设置为永久。",
      defaults: "默认：增加 30 天、卡码 72 小时有效、使用 1 次。",
    },
    "3": {
      title: "白名单码",
      description: "将用户升级为白名单并设置永久有效；未绑定 Emby 时需同时填写 Emby 用户名和密码完成创建。",
      defaults: "默认：永久账号、卡码永久有效、不限制使用次数。",
    },
  };

  const activeTypeDescription = typeDescriptions[activeTab];

  const randomAlgorithmDescriptions: Record<string, string> = {
    "base32-20": "20 位易抄写大写 Base32 风格字符，去掉易混淆字符，默认推荐。",
    "base32-24": "24 位易抄写大写 Base32 风格字符，更高强度，适合公开发放。",
    hex32: "32 位十六进制随机串，约 128-bit 强度，适合系统间导入导出。",
    hex20: "20 位十六进制随机串，旧默认格式，保留兼容。",
    "base32-16": "16 位大写 Base32 风格字符，去掉易混淆字符，适合人工抄写。",
    "alnum-24": "24 位大写字母 + 数字，更高强度，兼顾可读性和随机性。",
    "alnum-16": "16 位大写字母 + 数字，兼顾可读性和随机性。",
    "urlsafe-24": "24 位 URL 安全字符，包含大小写字母、数字、- 和 _。",
    "digits-16": "16 位纯数字，比 12 位更安全，但仍低于字母数字混合。",
    "digits-12": "12 位纯数字，便于口头传递，但安全性低于字母数字混合。",
    uuid: "标准 UUID v4，长度较长，唯一性强，适合系统间导入导出。",
    "legacy-sha1": "旧版 SHA1 截断格式，用于兼容历史卡码风格。",
  };

  const selectedAlgorithmDescription = createData.randomAlgorithm
    ? randomAlgorithmDescriptions[createData.randomAlgorithm]
    : "使用配置管理中的默认随机算法。";

  const pages = Math.ceil(total / 20);

  const usedCount = (code: Regcode) => {
    const byUid = code.used_by_uids?.length || 0;
    const byTg = code.used_by_telegram_ids?.length || 0;
    return Math.max(byUid, byTg, code.use_count || 0);
  };

  if (error) {
    return <PageError message={error} onRetry={() => void loadRegcodes()} />;
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl sm:text-3xl font-bold">卡码管理</h1>
          <p className="text-sm text-muted-foreground">批量生成注册码、续期码和白名单码</p>
        </div>
        <Dialog open={createOpen} onOpenChange={(open) => {
          setCreateOpen(open);
          if (!open) {
            setCreatedCodes([]);
          }
        }}>
          <DialogTrigger asChild>
            <Button variant="default" className="rounded-xl shadow-lg shadow-primary/20">
              <Plus className="mr-2 h-4 w-4" />
              生成卡码
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-md">
            <DialogHeader>
              <DialogTitle>生成注册码/续期码</DialogTitle>
              <DialogDescription>请选择卡码类型并配置参数</DialogDescription>
            </DialogHeader>
            
            <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
              <TabsList className="grid w-full grid-cols-3 mb-4">
                <TabsTrigger value="1">注册</TabsTrigger>
                <TabsTrigger value="2">续期</TabsTrigger>
                <TabsTrigger value="3">白名单</TabsTrigger>
              </TabsList>

                <div className="space-y-4 py-2">
                <div className="rounded-xl border border-primary/20 bg-primary/5 p-3 text-xs text-muted-foreground">
                  <div className="font-medium text-foreground">{activeTypeDescription.title}</div>
                  <p className="mt-1">{activeTypeDescription.description}</p>
                  <p className="mt-1">{activeTypeDescription.defaults}</p>
                  <p className="mt-1">类型值：注册=1、续期=2、白名单=3；0 或 -1 天数均表示永久。</p>
                </div>
                <div className="space-y-2">
                  <Label>{activeTab === "3" ? "白名单有效天数" : "账号天数"}</Label>
                  <div className="flex items-center justify-between rounded-md border border-border/80 bg-muted/40 px-3 py-2">
                    <span className="text-xs text-muted-foreground">设为永久（0 或 -1 都视为永久）</span>
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
                    {activeTab === "3" ? "白名单用户的有效时长，0 和 -1 均为永久" : "使用此码后账号增加的有效天数，0 和 -1 均为永久"}
                  </p>
                </div>
                
                <div className="space-y-2">
                  <Label>卡码本身的有效期 (小时)</Label>
                  <Input
                    type="number"
                    value={createData.validityTime}
                    onChange={(e) => setCreateData({ ...createData, validityTime: e.target.value })}
                  />
                  <p className="text-[11px] text-muted-foreground">
                    在此时间内不使用则卡码失效，-1 为永久有效
                  </p>
                </div>

                <div className="space-y-2">
                  <Label>使用次数上限</Label>
                  <Input
                    type="number"
                    value={createData.useCountLimit}
                    onChange={(e) => setCreateData({ ...createData, useCountLimit: e.target.value })}
                  />
                  <p className="text-[11px] text-muted-foreground">
                    该卡码可以被使用的总次数，-1 为无限制
                  </p>
                </div>

                <div className="space-y-2">
                  <Label>生成数量</Label>
                  <Input
                    type="number"
                    value={createData.count}
                    onChange={(e) => setCreateData({ ...createData, count: e.target.value })}
                    min="1"
                  />
                </div>

                <div className="space-y-2 rounded-xl border border-border/80 bg-muted/30 p-3">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <Label>生成假卡码 / 诱饵码</Label>
                      <p className="mt-1 text-[11px] text-muted-foreground">
                        用户使用后会按 [SAR].regcode_decoy_action 执行安全动作。
                      </p>
                    </div>
                    <Switch checked={createDecoy} onCheckedChange={setCreateDecoy} />
                  </div>
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label>随机算法</Label>
                    <Select value={createData.randomAlgorithm || "default"} onValueChange={(v) => setCreateData({ ...createData, randomAlgorithm: v === "default" ? "" : v })}>
                      <SelectTrigger><SelectValue placeholder="使用配置默认" /></SelectTrigger>
                      <SelectContent>
                        <SelectItem value="default">使用配置默认</SelectItem>
                        <SelectItem value="base32-20">base32-20 推荐</SelectItem>
                        <SelectItem value="base32-24">base32-24 高强度</SelectItem>
                        <SelectItem value="hex32">hex32 128-bit</SelectItem>
                        <SelectItem value="hex20">hex20 旧默认</SelectItem>
                        <SelectItem value="base32-16">base32-16 短码</SelectItem>
                        <SelectItem value="alnum-24">alnum-24 高强度</SelectItem>
                        <SelectItem value="alnum-16">alnum-16</SelectItem>
                        <SelectItem value="urlsafe-24">urlsafe-24</SelectItem>
                        <SelectItem value="digits-16">digits-16</SelectItem>
                        <SelectItem value="digits-12">digits-12</SelectItem>
                        <SelectItem value="uuid">uuid</SelectItem>
                        <SelectItem value="legacy-sha1">legacy-sha1</SelectItem>
                      </SelectContent>
                    </Select>
                    <p className="text-[11px] text-muted-foreground">{selectedAlgorithmDescription}</p>
                  </div>
                  <div className="space-y-2">
                    <Label>自定义格式</Label>
                    <Input
                      value={createData.format}
                      onChange={(e) => setCreateData({ ...createData, format: e.target.value })}
                      placeholder="如 TW-{type}-{random}"
                    />
                  </div>
                </div>
                <div className="rounded-xl border bg-muted/30 p-3 text-[11px] text-muted-foreground">
                  <p className="font-medium text-foreground">格式说明</p>
                  <p className="mt-1">可直接写自定义文本，例如 <code>VIP-{'{random}'}</code>、<code>银河列车-{'{type}'}-{'{random}'}</code>。</p>
                  <p><code>{'{random}'}</code>：按所选随机算法生成的随机部分。</p>
                  <p><code>{'{type}'}</code>：卡码类型，register / renew / whitelist。</p>
                  <p><code>{'{days}'}</code>：账号有效天数，-1 表示永久。</p>
                  <p><code>{'{index}'}</code>：本次批量生成序号，从 1 开始。</p>
                  <p><code>{'{validity}'}</code>：卡码自身有效期小时数；<code>{'{limit}'}</code>：使用次数上限。</p>
                  <p className="mt-1">留空则使用配置管理中的默认格式。格式中没有 <code>{'{random}'}</code> 时，后端会自动追加随机部分。</p>
                </div>

                {createdCodes.length > 0 && (
                  <div className="mt-4 space-y-2 p-3 bg-muted/50 rounded-xl border border-border">
                    <div className="flex items-center justify-between gap-2">
                      <Label className="text-xs">已生成的代码</Label>
                      <Button size="sm" variant="outline" className="h-7 px-2 text-xs" onClick={() => {
                        navigator.clipboard.writeText(createdCodes.join("\n"));
                        toast({ title: `已复制 ${createdCodes.length} 个卡码` });
                      }}>
                        <Copy className="mr-1 h-3.5 w-3.5" /> 复制全部
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
                取消
              </Button>
              <Button onClick={handleCreate} disabled={isCreating} className="min-w-[80px]">
                {isCreating ? <Loader2 className="h-4 w-4 animate-spin" /> : "立即生成"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      <div className="flex flex-col gap-2 rounded-xl border bg-muted/30 p-3 text-sm sm:flex-row sm:items-center sm:justify-between">
        <span className="text-muted-foreground">
          已选择 {selectedCodes.size} 个；未选择时导出当前页全部 {regcodes.length} 个。
        </span>
        <div className="flex w-full flex-wrap gap-2 sm:w-auto">
          <Button className="flex-1 sm:flex-none" variant="outline" size="sm" onClick={() => copyRegcodes(selectedRegcodes.length > 0 ? selectedRegcodes : regcodes)} disabled={regcodes.length === 0}>
            <Copy className="mr-2 h-4 w-4" /> 复制卡码
          </Button>
          <Button className="flex-1 sm:flex-none" variant="outline" size="sm" onClick={() => exportSelected("txt")} disabled={regcodes.length === 0}>
            <Download className="mr-2 h-4 w-4" /> 导出 TXT
          </Button>
          <Button className="flex-1 sm:flex-none" variant="outline" size="sm" onClick={() => exportSelected("json")} disabled={regcodes.length === 0}>
            <Download className="mr-2 h-4 w-4" /> 导出 JSON
          </Button>
          <Button
            className="flex-1 sm:flex-none"
            variant="destructive"
            size="sm"
            onClick={() => void handleBatchDelete()}
            disabled={selectedCodes.size === 0 || isBatchDeleting}
          >
            {isBatchDeleting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Trash2 className="mr-2 h-4 w-4" />}
            批量删除
          </Button>
        </div>
      </div>

      <Card>
        <CardContent className="grid gap-3 p-4 md:grid-cols-[1.2fr_0.8fr_0.8fr_0.8fr_0.7fr_auto]">
          <Input
            placeholder="搜索卡码 / 备注 / 使用 UID"
            value={search}
            onChange={(e) => { setSearch(e.target.value); setPage(1); }}
          />
          <Select value={filterType} onValueChange={(v) => { setFilterType(v); setPage(1); }}>
            <SelectTrigger><SelectValue placeholder="类型" /></SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部类型</SelectItem>
              <SelectItem value="1">注册码</SelectItem>
              <SelectItem value="2">续期码</SelectItem>
              <SelectItem value="3">白名单码</SelectItem>
            </SelectContent>
          </Select>
          <Select value={filterStatus} onValueChange={(v) => { setFilterStatus(v); setPage(1); }}>
            <SelectTrigger><SelectValue placeholder="状态" /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部状态</SelectItem>
                <SelectItem value="available">可用</SelectItem>
                <SelectItem value="active">启用中</SelectItem>
                <SelectItem value="decoy">诱饵码</SelectItem>
                <SelectItem value="used_up">已用完</SelectItem>
              <SelectItem value="expired">已过期</SelectItem>
              <SelectItem value="disabled">已禁用</SelectItem>
            </SelectContent>
          </Select>
          <Select value={sort} onValueChange={setSort}>
            <SelectTrigger><SelectValue placeholder="排序" /></SelectTrigger>
            <SelectContent>
              <SelectItem value="created_time">创建时间</SelectItem>
              <SelectItem value="code">卡码</SelectItem>
              <SelectItem value="type">类型</SelectItem>
              <SelectItem value="days">天数</SelectItem>
              <SelectItem value="use_count">使用次数</SelectItem>
              <SelectItem value="note">备注</SelectItem>
            </SelectContent>
          </Select>
          <Select value={order} onValueChange={setOrder}>
            <SelectTrigger><SelectValue placeholder="顺序" /></SelectTrigger>
            <SelectContent>
              <SelectItem value="desc">降序</SelectItem>
              <SelectItem value="asc">升序</SelectItem>
            </SelectContent>
          </Select>
          <div className="flex gap-2 md:min-w-0">
            <Button className="flex-1 md:flex-none" variant="outline" onClick={clearFilters}>重置</Button>
            <Button className="flex-1 md:flex-none" variant="outline" onClick={() => void loadRegcodes()}>刷新</Button>
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
              暂无注册码
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
                          {code.is_decoy ? <Badge variant="destructive">假卡码</Badge> : <Badge variant="outline">正常卡码</Badge>}
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
                        onClick={() => handleDelete(code.code)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>

                  <div className="mt-3 grid grid-cols-2 gap-3 text-sm">
                    <div>
                      <p className="text-xs text-muted-foreground">账号有效</p>
                      <p className="mt-1">{code.days <= 0 ? "永久" : `${code.days} 天`}</p>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">卡码有效</p>
                      <p className="mt-1">
                        {code.validity_time === -1 || code.validity_time === undefined
                          ? "永久有效"
                          : `${code.validity_time} 小时`}
                      </p>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">使用次数</p>
                      <p className="mt-1">{code.use_count || 0} / {code.use_count_limit === -1 ? "∞" : code.use_count_limit || "∞"}</p>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">创建时间</p>
                      <p className="mt-1">{formatDate(code.created_time || code.created_at)}</p>
                    </div>
                  </div>

                  <div className="mt-3 flex gap-2 border-t pt-3">
                    <Input
                      value={noteDrafts[code.code] ?? code.note ?? ""}
                      maxLength={120}
                      placeholder="备注 / 名称"
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
                        title={`${usedCount(code)} 人使用`}
                      >
                        <Users className="h-4 w-4" />
                      </Button>
                    )}
                  </div>
                </div>
              ))}
            </div>

            <div className="hidden overflow-x-auto md:block">
              <table className="w-full min-w-[980px]">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left text-sm font-medium">
                      <input
                        type="checkbox"
                        checked={regcodes.length > 0 && selectedCodes.size === regcodes.length}
                        onChange={(e) => toggleSelectAll(e.target.checked)}
                      />
                    </th>
                    <th className="px-4 py-3 text-left text-sm font-medium">注册码</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">类型 / 备注</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">账号有效天数</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">注册码有效期</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">使用次数</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">状态 / 使用用户</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">创建时间</th>
                    <th className="px-4 py-3 text-right text-sm font-medium">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {regcodes.map((code) => (
                    <tr key={code.code} className="border-b hover:bg-muted/30">
                      <td className="px-4 py-3">
                        <input
                          type="checkbox"
                          checked={selectedCodes.has(code.code)}
                          onChange={(e) => toggleSelectCode(code.code, e.target.checked)}
                        />
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <code className="rounded bg-muted px-2 py-1 text-sm">
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
                      <td className="px-4 py-3 min-w-[220px]">
                        <div className="space-y-2">
                          <div className="flex flex-wrap gap-1">
                            {getTypeBadge(code.type)}
                            {code.is_decoy ? <Badge variant="destructive">假卡码</Badge> : <Badge variant="outline">正常卡码</Badge>}
                          </div>
                          <div className="flex gap-1">
                            <Input
                              value={noteDrafts[code.code] ?? code.note ?? ""}
                              maxLength={120}
                              placeholder="备注 / 名称"
                              className="h-8 text-xs"
                              onChange={(e) => setNoteDrafts((prev) => ({ ...prev, [code.code]: e.target.value }))}
                            />
                            <Button size="icon" variant="ghost" className="h-8 w-8" disabled={savingNote === code.code} onClick={() => void handleSaveNote(code.code)}>
                              {savingNote === code.code ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
                            </Button>
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3">{code.days <= 0 ? "永久" : `${code.days} 天`}</td>
                      <td className="px-4 py-3 text-sm">
                        {code.validity_time === -1 || code.validity_time === undefined 
                          ? '永久有效' 
                          : `${code.validity_time} 小时`}
                      </td>
                      <td className="px-4 py-3 text-sm">
                        {code.use_count || 0} / {code.use_count_limit === -1 ? '∞' : code.use_count_limit || '∞'}
                      </td>
                      <td className="px-4 py-3">
                        {getStatusBadge(code)}
                        {usedCount(code) > 0 && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="mt-1 h-7 px-2 text-xs text-primary"
                            onClick={() => void openUsageDialog(code)}
                          >
                            <Users className="mr-1 h-3.5 w-3.5" />
                            {usedCount(code)} 人使用
                          </Button>
                        )}
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        {formatDate(code.created_time || code.created_at)}
                      </td>
                      <td className="px-4 py-3 text-right">
                        <Button
                          size="icon"
                          variant="ghost"
                          className="text-destructive hover:text-destructive"
                          onClick={() => handleDelete(code.code)}
                        >
                          <Trash2 className="h-4 w-4" />
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

      <Dialog open={usageOpen} onOpenChange={setUsageOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>卡码使用者</DialogTitle>
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
              暂无使用者记录
            </div>
          ) : (
            <div className="max-h-[60vh] space-y-3 overflow-y-auto pr-1">
              {usageUsers.map((user, index) => (
                <div key={`${user.uid || "missing"}-${index}`} className="rounded-xl border bg-muted/20 p-4">
                  {user.found ? (
                    <div className="grid gap-2 text-sm sm:grid-cols-2">
                      <div className="font-medium">{user.username || "未知用户"}</div>
                      <div className="text-muted-foreground">UID: {user.uid}</div>
                      <div>角色: {user.role_name || user.role}</div>
                      <div>状态: {user.active ? "启用" : "禁用"}</div>
                      <div>Telegram: {user.telegram_id ? "已绑定" : "未绑定"}</div>
                      <div>Emby: {user.emby_id ? "已绑定" : user.pending_emby ? "待补建" : "未绑定"}</div>
                      <div className="sm:col-span-2 text-muted-foreground">到期: {user.expire_status || "-"}</div>
                    </div>
                  ) : (
                    <div className="text-sm text-muted-foreground">UID {user.uid} 的本地用户已不存在</div>
                  )}
                </div>
              ))}
              {usageTelegramOnly.map((item) => (
                <div key={`tg-${item.telegram_id}`} className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-4 text-sm">
                  <div className="font-medium">仅记录到 Telegram 使用者</div>
                  <div className="mt-1 text-muted-foreground">TGID: {item.telegram_id}</div>
                  <div className="mt-1 text-muted-foreground">当前没有本地用户绑定该 Telegram ID。</div>
                </div>
              ))}
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setUsageOpen(false)}>关闭</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

