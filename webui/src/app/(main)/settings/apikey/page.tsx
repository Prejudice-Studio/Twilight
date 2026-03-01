"use client";

import { useCallback, useState } from "react";
import { motion } from "framer-motion";
import {
  Key,
  Copy,
  Trash2,
  Plus,
  Edit2,
  Loader2,
  Eye,
  EyeOff,
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Clock,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Separator } from "@/components/ui/separator";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError, PageLoading } from "@/components/layout/page-state";
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

const container = {
  hidden: { opacity: 0 },
  show: {
    opacity: 1,
    transition: {
      staggerChildren: 0.1,
    },
  },
};

const item = {
  hidden: { opacity: 0, y: 20 },
  show: { opacity: 1, y: 0 },
};

interface ApiKey {
  id: number;
  name: string;
  key: string;
  key_full: string;
  enabled: boolean;
  allow_checkin: boolean;
  allow_transfer: boolean;
  allow_query: boolean;
  rate_limit: number;
  request_count: number;
  last_used: number | null;
  created_at: number;
  expired_at: number | null;
}

export default function ApiKeyPage() {
  const { toast } = useToast();
  const [apiKeys, setApiKeys] = useState<ApiKey[]>([]);
  const [isSaving, setIsSaving] = useState(false);

  // 生成新 Key 对话框
  const [openGenerate, setOpenGenerate] = useState(false);
  const [newKeyForm, setNewKeyForm] = useState({
    name: "",
    allow_checkin: true,
    allow_transfer: false,
    allow_query: true,
    rate_limit: 100,
  });
  const [generatedKey, setGeneratedKey] = useState<string | null>(null);

  // 编辑 Key 对话框
  const [openEdit, setOpenEdit] = useState(false);
  const [editingKey, setEditingKey] = useState<ApiKey | null>(null);
  const [editForm, setEditForm] = useState({
    name: "",
    enabled: false,
    allow_checkin: false,
    allow_transfer: false,
    allow_query: false,
    rate_limit: 100,
  });

  // 查看完整 Key
  const [showKeyId, setShowKeyId] = useState<number | null>(null);

  const loadApiKeysResource = useCallback(async () => {
    const res = await api.getMyApiKeys();
    if (res.success && res.data?.keys) {
      setApiKeys(res.data.keys);
    } else {
      throw new Error(res.message || "无法加载 API Keys");
    }
    return true;
  }, []);

  const {
    isLoading,
    error,
    execute: loadApiKeys,
  } = useAsyncResource(loadApiKeysResource, { immediate: true });

  const handleGenerateKey = async () => {
    if (!newKeyForm.name) {
      toast({
        title: "错误",
        description: "请输入 Key 名称",
        variant: "destructive",
      });
      return;
    }

    setIsSaving(true);
    try {
      const res = await api.createMyApiKey(newKeyForm);

      if (res.success && res.data?.key) {
        setGeneratedKey(res.data.key);
        setNewKeyForm({
          name: "",
          allow_checkin: true,
          allow_transfer: false,
          allow_query: true,
          rate_limit: 100,
        });
        await loadApiKeys();
      } else {
        toast({
          title: "生成失败",
          description: res.message || "无法生成 API Key",
          variant: "destructive",
        });
      }
    } catch (error) {
      toast({
        title: "生成失败",
        description: "请重试",
        variant: "destructive",
      });
    } finally {
      setIsSaving(false);
    }
  };

  const handleUpdateKey = async () => {
    if (!editingKey) return;

    setIsSaving(true);
    try {
      const res = await api.updateMyApiKey(editingKey.id, editForm);

      if (res.success) {
        toast({
          title: "成功",
          description: "API Key 已更新",
        });
        setOpenEdit(false);
        await loadApiKeys();
      } else {
        toast({
          title: "更新失败",
          description: res.message || "无法更新 API Key",
          variant: "destructive",
        });
      }
    } catch (error) {
      toast({
        title: "更新失败",
        description: "请重试",
        variant: "destructive",
      });
    } finally {
      setIsSaving(false);
    }
  };

  const handleDeleteKey = async (keyId: number) => {
    if (!window.confirm("确定要删除此 API Key 吗？删除后无法恢复。")) {
      return;
    }

    setIsSaving(true);
    try {
      const res = await api.deleteMyApiKey(keyId);

      if (res.success) {
        toast({
          title: "成功",
          description: "API Key 已删除",
        });
        await loadApiKeys();
      } else {
        toast({
          title: "删除失败",
          description: "无法删除 API Key",
          variant: "destructive",
        });
      }
    } catch (error) {
      toast({
        title: "删除失败",
        description: "请重试",
        variant: "destructive",
      });
    } finally {
      setIsSaving(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    toast({
      title: "已复制",
      description: "已复制到剪贴板",
    });
  };

  const formatDate = (timestamp: number | null) => {
    if (!timestamp) return "-";
    return new Date(timestamp * 1000).toLocaleString("zh-CN");
  };

  if (error) {
    return <PageError message={error} onRetry={() => void loadApiKeys()} />;
  }

  if (isLoading) {
    return <PageLoading message="正在加载 API Keys..." />;
  }

  return (
    <motion.div
      variants={container}
      initial="hidden"
      animate="show"
      className="space-y-6"
    >
      {/* 标题和按钮 */}
      <motion.div variants={item} className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">API Key 管理</h1>
          <p className="text-muted-foreground">创建和管理多个 API Keys</p>
        </div>
        <Button onClick={() => setOpenGenerate(true)} className="gap-2">
          <Plus className="h-4 w-4" />
          生成新 Key
        </Button>
      </motion.div>

      {/* 提示 */}
      <motion.div variants={item}>
        <Alert className="bg-blue-500/10 border-blue-500/20">
          <Key className="h-4 w-4" />
          <AlertTitle>API Key 说明</AlertTitle>
          <AlertDescription>
            <ul className="list-disc list-inside space-y-1 mt-2">
              <li>可以生成多个 API Keys，每个都可以独立配置权限</li>
              <li>API Key 具有与您的账号相同的权限，请勿泄露给他人</li>
              <li>禁用 Key 后，使用该 Key 的应用将无法访问</li>
              <li>删除 Key 后无法恢复，请谨慎操作</li>
            </ul>
          </AlertDescription>
        </Alert>
      </motion.div>

      {/* API Keys 列表 */}
      <div className="space-y-3">
        {apiKeys.length === 0 ? (
          <motion.div variants={item}>
            <Card className="glass-card border-dashed">
              <CardContent className="p-8 text-center">
                <Key className="h-12 w-12 mx-auto text-muted-foreground mb-2 opacity-50" />
                <h3 className="font-semibold">暂无 API Keys</h3>
                <p className="text-sm text-muted-foreground mt-1">点击按钮生成您的第一个 API Key</p>
              </CardContent>
            </Card>
          </motion.div>
        ) : (
          apiKeys.map((apiKey) => (
            <motion.div key={apiKey.id} variants={item}>
              <Card className="glass-card">
                <CardContent className="p-6">
                  <div className="space-y-4">
                    {/* 头部信息 */}
                    <div className="flex items-center justify-between">
                      <div className="flex-1">
                        <div className="flex items-center gap-3">
                          <h3 className="font-semibold">{apiKey.name}</h3>
                          <Badge
                            variant={apiKey.enabled ? "default" : "secondary"}
                          >
                            {apiKey.enabled ? (
                              <>
                                <CheckCircle2 className="h-3 w-3 mr-1" />
                                启用
                              </>
                            ) : (
                              <>
                                <XCircle className="h-3 w-3 mr-1" />
                                禁用
                              </>
                            )}
                          </Badge>
                        </div>
                        <p className="text-xs text-muted-foreground mt-1">
                          创建于 {formatDate(apiKey.created_at)}
                        </p>
                      </div>
                      <div className="flex gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => {
                            setEditingKey(apiKey);
                            setEditForm({
                              name: apiKey.name,
                              enabled: apiKey.enabled,
                              allow_checkin: apiKey.allow_checkin,
                              allow_transfer: apiKey.allow_transfer,
                              allow_query: apiKey.allow_query,
                              rate_limit: apiKey.rate_limit,
                            });
                            setOpenEdit(true);
                          }}
                        >
                          <Edit2 className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="destructive"
                          size="sm"
                          onClick={() => handleDeleteKey(apiKey.id)}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>

                    <Separator />

                    {/* Key 值 */}
                    <div className="space-y-2">
                      <Label className="text-xs">API Key</Label>
                      <div className="flex items-center gap-2">
                        <Input
                          value={showKeyId === apiKey.id ? apiKey.key_full : apiKey.key}
                          readOnly
                          className="font-mono text-xs"
                        />
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setShowKeyId(showKeyId === apiKey.id ? null : apiKey.id)}
                        >
                          {showKeyId === apiKey.id ? (
                            <EyeOff className="h-4 w-4" />
                          ) : (
                            <Eye className="h-4 w-4" />
                          )}
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => copyToClipboard(apiKey.key_full)}
                        >
                          <Copy className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>

                    {/* 权限信息 */}
                    <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 p-3 bg-muted/50 rounded-lg">
                      <div className="text-center">
                        <p className="text-xs text-muted-foreground">签到权限</p>
                        <Badge variant={apiKey.allow_checkin ? "default" : "outline"} className="mt-1">
                          {apiKey.allow_checkin ? "✓ 允许" : "✗ 禁止"}
                        </Badge>
                      </div>
                      <div className="text-center">
                        <p className="text-xs text-muted-foreground">转账权限</p>
                        <Badge variant={apiKey.allow_transfer ? "default" : "outline"} className="mt-1">
                          {apiKey.allow_transfer ? "✓ 允许" : "✗ 禁止"}
                        </Badge>
                      </div>
                      <div className="text-center">
                        <p className="text-xs text-muted-foreground">查询权限</p>
                        <Badge variant={apiKey.allow_query ? "default" : "outline"} className="mt-1">
                          {apiKey.allow_query ? "✓ 允许" : "✗ 禁止"}
                        </Badge>
                      </div>
                      <div className="text-center">
                        <p className="text-xs text-muted-foreground">速率限制</p>
                        <p className="text-sm font-semibold mt-1">{apiKey.rate_limit}/h</p>
                      </div>
                    </div>

                    {/* 使用情况 */}
                    <div className="grid grid-cols-2 gap-3 text-sm">
                      <div>
                        <p className="text-xs text-muted-foreground">请求次数</p>
                        <p className="font-semibold">{apiKey.request_count}</p>
                      </div>
                      <div>
                        <p className="text-xs text-muted-foreground">最后使用</p>
                        <p className="font-semibold text-xs">
                          {apiKey.last_used ? formatDate(apiKey.last_used) : "未使用"}
                        </p>
                      </div>
                    </div>
                  </div>
                </CardContent>
              </Card>
            </motion.div>
          ))
        )}
      </div>

      {/* 生成新 Key 对话框 */}
      <Dialog open={openGenerate} onOpenChange={setOpenGenerate}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>生成新 API Key</DialogTitle>
            <DialogDescription>
              创建一个新的 API Key 并配置其权限
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            {/* 已生成的 Key 显示 */}
            {generatedKey && (
              <Alert className="bg-green-500/10 border-green-500/20">
                <CheckCircle2 className="h-4 w-4 text-green-500" />
                <AlertTitle>API Key 生成成功</AlertTitle>
                <AlertDescription className="mt-2">
                  <p className="text-xs mb-2">请妥善保管此 Key，关闭此对话框后将无法再次显示：</p>
                  <div className="flex gap-2">
                    <Input
                      value={generatedKey}
                      readOnly
                      className="font-mono text-xs"
                    />
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => copyToClipboard(generatedKey)}
                    >
                      <Copy className="h-4 w-4" />
                    </Button>
                  </div>
                </AlertDescription>
              </Alert>
            )}

            {!generatedKey && (
              <>
                {/* Key 名称 */}
                <div className="space-y-2">
                  <Label>Key 名称</Label>
                  <Input
                    placeholder="例如: 移动应用、Web API 等"
                    value={newKeyForm.name}
                    onChange={(e) =>
                      setNewKeyForm({ ...newKeyForm, name: e.target.value })
                    }
                  />
                </div>

                {/* 权限配置 */}
                <div className="space-y-3">
                  <Label>权限配置</Label>

                  <div className="flex items-center justify-between p-3 border border-border rounded">
                    <div>
                      <p className="font-medium text-sm">允许签到</p>
                      <p className="text-xs text-muted-foreground">
                        可以使用此 Key 进行每日签到
                      </p>
                    </div>
                    <Switch
                      checked={newKeyForm.allow_checkin}
                      onCheckedChange={(checked) =>
                        setNewKeyForm({
                          ...newKeyForm,
                          allow_checkin: checked,
                        })
                      }
                    />
                  </div>

                  <div className="flex items-center justify-between p-3 border border-border rounded">
                    <div>
                      <p className="font-medium text-sm">允许转账</p>
                      <p className="text-xs text-muted-foreground">
                        可以使用此 Key 进行积分转账
                      </p>
                    </div>
                    <Switch
                      checked={newKeyForm.allow_transfer}
                      onCheckedChange={(checked) =>
                        setNewKeyForm({
                          ...newKeyForm,
                          allow_transfer: checked,
                        })
                      }
                    />
                  </div>

                  <div className="flex items-center justify-between p-3 border border-border rounded">
                    <div>
                      <p className="font-medium text-sm">允许查询</p>
                      <p className="text-xs text-muted-foreground">
                        可以使用此 Key 查询账户信息
                      </p>
                    </div>
                    <Switch
                      checked={newKeyForm.allow_query}
                      onCheckedChange={(checked) =>
                        setNewKeyForm({
                          ...newKeyForm,
                          allow_query: checked,
                        })
                      }
                    />
                  </div>
                </div>

                {/* 速率限制 */}
                <div className="space-y-2">
                  <Label htmlFor="rate-limit">速率限制（请求/小时）</Label>
                  <Input
                    id="rate-limit"
                    type="number"
                    min="0"
                    value={newKeyForm.rate_limit}
                    onChange={(e) =>
                      setNewKeyForm({
                        ...newKeyForm,
                        rate_limit: parseInt(e.target.value) || 0,
                      })
                    }
                  />
                </div>
              </>
            )}
          </div>

          <div className="flex justify-end gap-2 mt-4">
            <Button
              variant="outline"
              onClick={() => {
                setOpenGenerate(false);
                setGeneratedKey(null);
              }}
            >
              {generatedKey ? "关闭" : "取消"}
            </Button>
            {!generatedKey && (
              <Button onClick={handleGenerateKey} disabled={isSaving}>
                {isSaving ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    生成中...
                  </>
                ) : (
                  "生成 Key"
                )}
              </Button>
            )}
          </div>
        </DialogContent>
      </Dialog>

      {/* 编辑 Key 对话框 */}
      <Dialog open={openEdit} onOpenChange={setOpenEdit}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>编辑 API Key</DialogTitle>
            <DialogDescription>
              修改 Key 的名称和权限配置
            </DialogDescription>
          </DialogHeader>

          {editingKey && (
            <div className="space-y-4">
              {/* Key 名称 */}
              <div className="space-y-2">
                <Label>Key 名称</Label>
                <Input
                  value={editForm.name}
                  onChange={(e) =>
                    setEditForm({ ...editForm, name: e.target.value })
                  }
                />
              </div>

              {/* 启用/禁用 */}
              <div className="flex items-center justify-between p-3 border border-border rounded">
                <p className="font-medium text-sm">启用此 Key</p>
                <Switch
                  checked={editForm.enabled}
                  onCheckedChange={(checked) =>
                    setEditForm({ ...editForm, enabled: checked })
                  }
                />
              </div>

              {/* 权限配置 */}
              <div className="space-y-3">
                <Label>权限配置</Label>

                <div className="flex items-center justify-between p-3 border border-border rounded">
                  <div>
                    <p className="font-medium text-sm">允许签到</p>
                  </div>
                  <Switch
                    checked={editForm.allow_checkin}
                    onCheckedChange={(checked) =>
                      setEditForm({
                        ...editForm,
                        allow_checkin: checked,
                      })
                    }
                  />
                </div>

                <div className="flex items-center justify-between p-3 border border-border rounded">
                  <div>
                    <p className="font-medium text-sm">允许转账</p>
                  </div>
                  <Switch
                    checked={editForm.allow_transfer}
                    onCheckedChange={(checked) =>
                      setEditForm({
                        ...editForm,
                        allow_transfer: checked,
                      })
                    }
                  />
                </div>

                <div className="flex items-center justify-between p-3 border border-border rounded">
                  <div>
                    <p className="font-medium text-sm">允许查询</p>
                  </div>
                  <Switch
                    checked={editForm.allow_query}
                    onCheckedChange={(checked) =>
                      setEditForm({
                        ...editForm,
                        allow_query: checked,
                      })
                    }
                  />
                </div>
              </div>

              {/* 速率限制 */}
              <div className="space-y-2">
                <Label htmlFor="edit-rate-limit">速率限制（请求/小时）</Label>
                <Input
                  id="edit-rate-limit"
                  type="number"
                  min="0"
                  value={editForm.rate_limit}
                  onChange={(e) =>
                    setEditForm({
                      ...editForm,
                      rate_limit: parseInt(e.target.value) || 0,
                    })
                  }
                />
              </div>
            </div>
          )}

          <div className="flex justify-end gap-2 mt-4">
            <Button variant="outline" onClick={() => setOpenEdit(false)}>
              取消
            </Button>
            <Button onClick={handleUpdateKey} disabled={isSaving}>
              {isSaving ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  保存中...
                </>
              ) : (
                "保存"
              )}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}

