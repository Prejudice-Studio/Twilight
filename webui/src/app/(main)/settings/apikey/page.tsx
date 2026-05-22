"use client";

import { useCallback, useState } from "react";
import {
  Key,
  Copy,
  Trash2,
  Plus,
  Edit2,
  Loader2,
  Eye,
  EyeOff,
  CheckCircle2,
  XCircle,
  Shield,
  RefreshCw,
  AlertTriangle,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Separator } from "@/components/ui/separator";
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { api } from "@/lib/api";
import { useAsyncResource } from "@/hooks/use-async-resource";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert";

interface ApiKey {
  id: number;
  name: string;
  key: string;            // masked, e.g. "key-xxxxxxxx…yyyyyyyy"
  key_prefix: string;
  key_suffix: string;
  enabled: boolean;
  allow_query: boolean;
  permissions?: string[];
  rate_limit: number;
  request_count: number;
  last_used: number | null;
  created_at: number;
  expired_at: number | null;
}

function isNormalOps(k: { allow_query: boolean }) {
  return k.allow_query;
}

export default function ApiKeyPage() {
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const [apiKeys, setApiKeys] = useState<ApiKey[]>([]);
  const [isSaving, setIsSaving] = useState(false);

  const [openGenerate, setOpenGenerate] = useState(false);
  const [newKeyForm, setNewKeyForm] = useState({
    name: "",
    normalOps: true,
    rate_limit: 100,
  });
  const [generatedKey, setGeneratedKey] = useState<string | null>(null);

  const [openEdit, setOpenEdit] = useState(false);
  const [editingKey, setEditingKey] = useState<ApiKey | null>(null);
  const [editForm, setEditForm] = useState({
    name: "",
    enabled: false,
    normalOps: true,
    rate_limit: 100,
  });

  // 创建后展示明文 key（id -> plaintext），用户离开对话框后清空
  const [revealedKeys, setRevealedKeys] = useState<Record<number, string>>({});
  const [showKeyId, setShowKeyId] = useState<number | null>(null);

  const loadApiKeysResource = useCallback(async () => {
    const res = await api.getMyApiKeys();
    if (!res.success) {
      throw new Error(res.message || "无法加载 API Keys");
    }
    const keys = Array.isArray(res.data?.keys) ? (res.data!.keys as ApiKey[]) : [];
    setApiKeys(keys);
    return keys;
  }, []);

  const {
    isLoading,
    error,
    execute: loadApiKeys,
  } = useAsyncResource(loadApiKeysResource, { immediate: true });

  const handleGenerateKey = async () => {
    if (!newKeyForm.name.trim()) {
      toast({ title: "错误", description: "请输入 Key 名称", variant: "destructive" });
      return;
    }
    setIsSaving(true);
    try {
      const res = await api.createMyApiKey({
        name: newKeyForm.name.trim(),
        allow_query: newKeyForm.normalOps,
        rate_limit: newKeyForm.rate_limit,
      });
      if (res.success && res.data?.key) {
        setGeneratedKey(res.data.key);
        if (res.data.id) {
          setRevealedKeys((prev) => ({ ...prev, [res.data!.id]: res.data!.key }));
        }
        setNewKeyForm({ name: "", normalOps: true, rate_limit: 100 });
        await loadApiKeys();
      } else {
        toast({ title: "创建失败", description: res.message || "无法创建 API Key", variant: "destructive" });
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : "网络错误";
      toast({ title: "创建失败", description: msg, variant: "destructive" });
    } finally {
      setIsSaving(false);
    }
  };

  const handleUpdateKey = async () => {
    if (!editingKey) return;
    setIsSaving(true);
    try {
      const res = await api.updateMyApiKey(editingKey.id, {
        name: editForm.name,
        enabled: editForm.enabled,
        allow_query: editForm.normalOps,
        rate_limit: editForm.rate_limit,
      });
      if (res.success) {
        toast({ title: "成功", description: "API Key 已更新" });
        setOpenEdit(false);
        await loadApiKeys();
      } else {
        toast({ title: "更新失败", description: res.message || "无法更新", variant: "destructive" });
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : "网络错误";
      toast({ title: "更新失败", description: msg, variant: "destructive" });
    } finally {
      setIsSaving(false);
    }
  };

  const handleDeleteKey = async (keyId: number) => {
    const ok = await confirm({
      title: "删除 API Key？",
      description: "删除后该 Key 将立即失效，且无法恢复。",
      tone: "danger",
      confirmLabel: "删除",
    });
    if (!ok) return;
    setIsSaving(true);
    try {
      const res = await api.deleteMyApiKey(keyId);
      if (res.success) {
        toast({ title: "成功", description: "API Key 已删除" });
        await loadApiKeys();
      } else {
        toast({ title: "删除失败", description: res.message || "无法删除", variant: "destructive" });
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : "网络错误";
      toast({ title: "删除失败", description: msg, variant: "destructive" });
    } finally {
      setIsSaving(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text).catch(() => {
      toast({ title: "复制失败", description: "浏览器拒绝访问剪贴板", variant: "destructive" });
      return;
    });
    toast({ title: "已复制", description: "已复制到剪贴板" });
  };

  const formatDate = (timestamp: number | null) => {
    if (!timestamp) return "-";
    return new Date(timestamp * 1000).toLocaleString("zh-CN");
  };

  const handleRefresh = () => {
    void loadApiKeys();
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">API Key 管理</h1>
          <p className="text-sm text-muted-foreground">创建和管理 API Keys，用于外部系统调用</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={handleRefresh} disabled={isLoading}>
            <RefreshCw className={`h-4 w-4 mr-1 ${isLoading ? "animate-spin" : ""}`} />
            刷新
          </Button>
          <Button onClick={() => setOpenGenerate(true)} size="sm">
            <Plus className="h-4 w-4 mr-1" />
            新建
          </Button>
        </div>
      </div>

      <Alert className="bg-blue-500/10 border-blue-500/20">
        <Key className="h-4 w-4" />
        <AlertTitle>使用说明</AlertTitle>
        <AlertDescription className="text-xs mt-1">
          API Key 用于外部系统访问你的账号。请勿泄露给他人，删除后无法恢复。
        </AlertDescription>
      </Alert>

      <div className="space-y-3">
        {error ? (
          <Card className="border-destructive/40">
            <CardContent className="p-6 text-center space-y-3">
              <AlertTriangle className="h-8 w-8 mx-auto text-destructive" />
              <p className="text-sm text-foreground font-medium">无法加载 API Keys</p>
              <p className="text-xs text-muted-foreground break-words">{error}</p>
              <p className="text-[11px] text-muted-foreground">
                若后端返回 500，请确认 Go 后端已完成数据库迁移并已重启
              </p>
              <Button variant="outline" size="sm" onClick={handleRefresh}>
                <RefreshCw className="h-4 w-4 mr-1" />
                重试
              </Button>
            </CardContent>
          </Card>
        ) : isLoading && apiKeys.length === 0 ? (
          <Card className="border-dashed">
            <CardContent className="p-8 text-center">
              <Loader2 className="h-8 w-8 mx-auto animate-spin text-muted-foreground mb-2" />
              <p className="text-sm text-muted-foreground">正在加载 API Keys...</p>
            </CardContent>
          </Card>
        ) : apiKeys.length === 0 ? (
          <Card className="border-dashed">
            <CardContent className="p-8 text-center">
              <Key className="h-10 w-10 mx-auto text-muted-foreground mb-2 opacity-40" />
              <p className="font-medium">暂无 API Keys</p>
              <p className="text-xs text-muted-foreground mt-1">点击右上角“新建”按钮来创建你的第一个 API Key</p>
            </CardContent>
          </Card>
        ) : (
          apiKeys.map((apiKey) => (
            <Card key={apiKey.id}>
              <CardContent className="p-4">
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2 min-w-0">
                      <h3 className="font-medium truncate">{apiKey.name}</h3>
                      <Badge variant={apiKey.enabled ? "default" : "secondary"} className="shrink-0 text-xs">
                        {apiKey.enabled ? "启用" : "停用"}
                      </Badge>
                    </div>
                    <div className="flex gap-1 shrink-0">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        onClick={() => {
                          setEditingKey(apiKey);
                          setEditForm({
                            name: apiKey.name,
                            enabled: apiKey.enabled,
                            normalOps: isNormalOps(apiKey),
                            rate_limit: apiKey.rate_limit,
                          });
                          setOpenEdit(true);
                        }}
                      >
                        <Edit2 className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-destructive hover:text-destructive"
                        onClick={() => handleDeleteKey(apiKey.id)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>

                  <div className="flex items-center gap-2">
                    <code className="flex-1 text-xs bg-muted px-3 py-1.5 rounded truncate font-mono">
                      {showKeyId === apiKey.id && revealedKeys[apiKey.id]
                        ? revealedKeys[apiKey.id]
                        : apiKey.key}
                    </code>
                    {revealedKeys[apiKey.id] && (
                      <Button variant="ghost" size="icon" className="h-8 w-8 shrink-0"
                        onClick={() => setShowKeyId(showKeyId === apiKey.id ? null : apiKey.id)}
                        title={showKeyId === apiKey.id ? "隐藏明文" : "显示明文（仅本次会话）"}>
                        {showKeyId === apiKey.id ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                      </Button>
                    )}
                    <Button variant="ghost" size="icon" className="h-8 w-8 shrink-0"
                      onClick={() => copyToClipboard(revealedKeys[apiKey.id] || apiKey.key)}
                      title={revealedKeys[apiKey.id] ? "复制明文" : "明文已不可见，仅复制掩码"}>
                      <Copy className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                  {!revealedKeys[apiKey.id] && (
                    <p className="text-[10px] text-muted-foreground -mt-1">
                      明文仅在创建时显示一次，如已遗失请删除后重新创建
                    </p>
                  )}

                  <div className="flex items-center gap-3 text-xs text-muted-foreground flex-wrap">
                    <span className="flex items-center gap-1">
                      <Shield className="h-3 w-3" />
                      普通操作: {isNormalOps(apiKey) ? <CheckCircle2 className="h-3 w-3 text-green-500" /> : <XCircle className="h-3 w-3 text-red-500" />}
                    </span>
                    <Separator orientation="vertical" className="h-3" />
                    <span>{apiKey.request_count} 次请求</span>
                    <span>限制 {apiKey.rate_limit}/h</span>
                    {apiKey.last_used && <span>最后使用 {formatDate(apiKey.last_used)}</span>}
                  </div>
                </div>
              </CardContent>
            </Card>
          ))
        )}
      </div>

      <Dialog open={openGenerate} onOpenChange={(open) => { setOpenGenerate(open); if (!open) setGeneratedKey(null); }}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>新建 API Key</DialogTitle>
            <DialogDescription>设置权限和限制</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            {generatedKey ? (
              <Alert className="bg-green-500/10 border-green-500/20">
                <CheckCircle2 className="h-4 w-4 text-green-500" />
                <AlertTitle>生成成功</AlertTitle>
                <AlertDescription className="mt-2">
                  <p className="text-xs mb-2">请立即复制保存，关闭后无法再次查看。</p>
                  <div className="flex gap-2">
                    <Input value={generatedKey} readOnly className="font-mono text-xs" />
                    <Button size="sm" variant="outline" onClick={() => copyToClipboard(generatedKey)}>
                      <Copy className="h-4 w-4" />
                    </Button>
                  </div>
                </AlertDescription>
              </Alert>
            ) : (
              <>
                <div className="space-y-2">
                  <Label>名称</Label>
                  <Input placeholder="例如: 自动查询脚本" value={newKeyForm.name}
                    onChange={(e) => setNewKeyForm({ ...newKeyForm, name: e.target.value })} />
                </div>
                <div className="space-y-3">
                  <Label>权限</Label>
                  <div className="flex items-center justify-between p-3 border rounded-md">
                    <div>
                      <div className="flex items-center gap-1.5">
                        <Shield className="h-4 w-4 text-blue-500" />
                        <span className="text-sm font-medium">普通操作</span>
                      </div>
                      <p className="text-xs text-muted-foreground mt-0.5">查询信息等只读操作</p>
                    </div>
                    <Switch checked={newKeyForm.normalOps}
                      onCheckedChange={(v) => setNewKeyForm({ ...newKeyForm, normalOps: v })} />
                  </div>
                </div>
                <div className="space-y-2">
                  <Label>速率限制（请求/小时）</Label>
                  <Input type="number" min="0" value={newKeyForm.rate_limit}
                    onChange={(e) => setNewKeyForm({ ...newKeyForm, rate_limit: parseInt(e.target.value) || 0 })} />
                </div>
              </>
            )}
          </div>
          <div className="flex justify-end gap-2 mt-2">
            <Button variant="outline" onClick={() => { setOpenGenerate(false); setGeneratedKey(null); }}>
              {generatedKey ? "关闭" : "取消"}
            </Button>
            {!generatedKey && (
              <Button onClick={handleGenerateKey} disabled={isSaving}>
                {isSaving ? <><Loader2 className="mr-2 h-4 w-4 animate-spin" />生成中...</> : "生成"}
              </Button>
            )}
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={openEdit} onOpenChange={setOpenEdit}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>编辑 API Key</DialogTitle>
            <DialogDescription>修改名称和权限</DialogDescription>
          </DialogHeader>
          {editingKey && (
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>名称</Label>
                <Input value={editForm.name}
                  onChange={(e) => setEditForm({ ...editForm, name: e.target.value })} />
              </div>
              <div className="flex items-center justify-between p-3 border rounded-md">
                <span className="text-sm font-medium">启用</span>
                <Switch checked={editForm.enabled}
                  onCheckedChange={(v) => setEditForm({ ...editForm, enabled: v })} />
              </div>
              <div className="space-y-3">
                <Label>权限</Label>
                <div className="flex items-center justify-between p-3 border rounded-md">
                  <div>
                    <div className="flex items-center gap-1.5">
                      <Shield className="h-4 w-4 text-blue-500" />
                      <span className="text-sm font-medium">普通操作</span>
                    </div>
                    <p className="text-xs text-muted-foreground mt-0.5">查询信息等只读操作</p>
                  </div>
                  <Switch checked={editForm.normalOps}
                    onCheckedChange={(v) => setEditForm({ ...editForm, normalOps: v })} />
                </div>
              </div>
              <div className="space-y-2">
                <Label>速率限制（请求/小时）</Label>
                <Input type="number" min="0" value={editForm.rate_limit}
                  onChange={(e) => setEditForm({ ...editForm, rate_limit: parseInt(e.target.value) || 0 })} />
              </div>
            </div>
          )}
          <div className="flex justify-end gap-2 mt-2">
            <Button variant="outline" onClick={() => setOpenEdit(false)}>取消</Button>
            <Button onClick={handleUpdateKey} disabled={isSaving}>
              {isSaving ? <><Loader2 className="mr-2 h-4 w-4 animate-spin" />保存中...</> : "保存"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
