"use client";

import { useEffect, useState } from "react";
import { motion } from "framer-motion";
import {
  Key,
  Loader2,
  CheckCircle2,
  XCircle,
  Eye,
  EyeOff,
  Copy,
  RefreshCw,
  AlertTriangle,
  Shield,
  ShieldOff,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

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

export default function ApiKeyPage() {
  const { toast } = useToast();
  const { user, fetchUser } = useAuthStore();
  const [apiKeyStatus, setApiKeyStatus] = useState<{
    enabled: boolean;
    apikey: string | null;
  } | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isActionLoading, setIsActionLoading] = useState(false);
  const [showApiKey, setShowApiKey] = useState(false);
  const [showConfirmDialog, setShowConfirmDialog] = useState(false);
  const [confirmAction, setConfirmAction] = useState<"generate" | "disable" | "refresh" | null>(null);

  useEffect(() => {
    loadApiKeyStatus();
  }, []);

  const loadApiKeyStatus = async () => {
    setIsLoading(true);
    try {
      const res = await api.getApiKeyStatus();
      if (res.success && res.data) {
        setApiKeyStatus(res.data);
      } else {
        toast({
          title: "加载失败",
          description: res.message || "无法加载API Key状态",
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: "加载失败",
        description: error.message || "请检查网络连接",
        variant: "destructive",
      });
    } finally {
      setIsLoading(false);
    }
  };

  const handleGenerate = async () => {
    setIsActionLoading(true);
    try {
      const res = await api.generateApiKey();
      if (res.success && res.data) {
        setApiKeyStatus({
          enabled: res.data.enabled,
          apikey: res.data.apikey,
        });
        setShowApiKey(true);
        toast({
          title: "生成成功",
          description: "API Key 已生成",
          variant: "success",
        });
      } else {
        toast({
          title: "生成失败",
          description: res.message || "无法生成API Key",
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: "生成失败",
        description: error.message || "请检查网络连接",
        variant: "destructive",
      });
    } finally {
      setIsActionLoading(false);
      setShowConfirmDialog(false);
    }
  };

  const handleDisable = async () => {
    setIsActionLoading(true);
    try {
      const res = await api.disableApiKey();
      if (res.success) {
        setApiKeyStatus({
          enabled: false,
          apikey: apiKeyStatus?.apikey || null,
        });
        setShowApiKey(false);
        toast({
          title: "已禁用",
          description: "API Key 已禁用",
          variant: "success",
        });
      } else {
        toast({
          title: "禁用失败",
          description: res.message || "无法禁用API Key",
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: "禁用失败",
        description: error.message || "请检查网络连接",
        variant: "destructive",
      });
    } finally {
      setIsActionLoading(false);
      setShowConfirmDialog(false);
    }
  };

  const handleEnable = async () => {
    setIsActionLoading(true);
    try {
      const res = await api.enableApiKey();
      if (res.success && res.data) {
        setApiKeyStatus({
          enabled: res.data.enabled,
          apikey: res.data.apikey,
        });
        setShowApiKey(true);
        toast({
          title: "已启用",
          description: "API Key 已启用",
          variant: "success",
        });
      } else {
        toast({
          title: "启用失败",
          description: res.message || "无法启用API Key",
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: "启用失败",
        description: error.message || "请检查网络连接",
        variant: "destructive",
      });
    } finally {
      setIsActionLoading(false);
    }
  };

  const handleRefresh = async () => {
    setIsActionLoading(true);
    try {
      const res = await api.refreshApiKey();
      if (res.success && res.data) {
        setApiKeyStatus({
          enabled: res.data.enabled,
          apikey: res.data.apikey,
        });
        setShowApiKey(true);
        toast({
          title: "刷新成功",
          description: "API Key 已重新生成",
          variant: "success",
        });
      } else {
        toast({
          title: "刷新失败",
          description: res.message || "无法刷新API Key",
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: "刷新失败",
        description: error.message || "请检查网络连接",
        variant: "destructive",
      });
    } finally {
      setIsActionLoading(false);
      setShowConfirmDialog(false);
    }
  };

  const handleCopyApiKey = () => {
    if (apiKeyStatus?.apikey) {
      navigator.clipboard.writeText(apiKeyStatus.apikey);
      toast({
        title: "已复制",
        description: "API Key 已复制到剪贴板",
        variant: "success",
      });
    }
  };

  const openConfirmDialog = (action: "generate" | "disable" | "refresh") => {
    setConfirmAction(action);
    setShowConfirmDialog(true);
  };

  const executeAction = () => {
    if (confirmAction === "generate") {
      handleGenerate();
    } else if (confirmAction === "disable") {
      handleDisable();
    } else if (confirmAction === "refresh") {
      handleRefresh();
    }
  };

  // 检查用户是否有Emby账号
  const hasEmbyAccount = user?.emby_id && user.emby_id.trim() !== "";

  if (!hasEmbyAccount) {
    return (
      <motion.div
        variants={container}
        initial="hidden"
        animate="show"
        className="space-y-6"
      >
        <div>
          <h1 className="text-3xl font-bold">API Key 管理</h1>
          <p className="text-muted-foreground">管理您的API Key用于外部接口控制</p>
        </div>

        <motion.div variants={item}>
          <Alert>
            <AlertTriangle className="h-4 w-4" />
            <AlertTitle>需要绑定Emby账号</AlertTitle>
            <AlertDescription>
              您需要先绑定Emby账号才能使用API Key功能。请前往个人设置页面绑定Emby账号。
            </AlertDescription>
          </Alert>
        </motion.div>
      </motion.div>
    );
  }

  return (
    <motion.div
      variants={container}
      initial="hidden"
      animate="show"
      className="space-y-6"
    >
      <div>
        <h1 className="text-3xl font-bold">API Key 管理</h1>
        <p className="text-muted-foreground">管理您的API Key用于外部接口控制账号</p>
      </div>

      <motion.div variants={item}>
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Key className="h-5 w-5" />
              API Key 状态
            </CardTitle>
            <CardDescription>
              API Key 可用于外部接口控制您的账号，请妥善保管
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            {isLoading ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
              </div>
            ) : (
              <>
                <div className="flex items-center justify-between p-4 rounded-lg bg-accent/50">
                  <div className="flex items-center gap-3">
                    {apiKeyStatus?.enabled ? (
                      <CheckCircle2 className="h-5 w-5 text-emerald-500" />
                    ) : (
                      <XCircle className="h-5 w-5 text-muted-foreground" />
                    )}
                    <div>
                      <div className="font-medium">
                        {apiKeyStatus?.enabled ? "已启用" : "已禁用"}
                      </div>
                      <div className="text-sm text-muted-foreground">
                        {apiKeyStatus?.enabled
                          ? "API Key 当前可用"
                          : "API Key 当前不可用"}
                      </div>
                    </div>
                  </div>
                  <Badge variant={apiKeyStatus?.enabled ? "default" : "secondary"}>
                    {apiKeyStatus?.enabled ? "启用" : "禁用"}
                  </Badge>
                </div>

                {apiKeyStatus?.apikey && (
                  <div className="space-y-2">
                    <Label>API Key</Label>
                    <div className="flex gap-2">
                      <Input
                        type={showApiKey ? "text" : "password"}
                        value={apiKeyStatus.apikey}
                        readOnly
                        className="font-mono"
                      />
                      <Button
                        variant="outline"
                        size="icon"
                        onClick={() => setShowApiKey(!showApiKey)}
                      >
                        {showApiKey ? (
                          <EyeOff className="h-4 w-4" />
                        ) : (
                          <Eye className="h-4 w-4" />
                        )}
                      </Button>
                      <Button
                        variant="outline"
                        size="icon"
                        onClick={handleCopyApiKey}
                      >
                        <Copy className="h-4 w-4" />
                      </Button>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      点击复制按钮将API Key复制到剪贴板
                    </p>
                  </div>
                )}

                <div className="flex flex-wrap gap-2">
                  {!apiKeyStatus?.apikey ? (
                    <Button
                      onClick={() => openConfirmDialog("generate")}
                      disabled={isActionLoading}
                    >
                      {isActionLoading ? (
                        <>
                          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                          生成中...
                        </>
                      ) : (
                        <>
                          <Key className="mr-2 h-4 w-4" />
                          生成 API Key
                        </>
                      )}
                    </Button>
                  ) : (
                    <>
                      {apiKeyStatus.enabled ? (
                        <>
                          <Button
                            variant="outline"
                            onClick={() => openConfirmDialog("disable")}
                            disabled={isActionLoading}
                          >
                            {isActionLoading ? (
                              <>
                                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                禁用中...
                              </>
                            ) : (
                              <>
                                <ShieldOff className="mr-2 h-4 w-4" />
                                禁用
                              </>
                            )}
                          </Button>
                          <Button
                            variant="outline"
                            onClick={() => openConfirmDialog("refresh")}
                            disabled={isActionLoading}
                          >
                            {isActionLoading ? (
                              <>
                                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                刷新中...
                              </>
                            ) : (
                              <>
                                <RefreshCw className="mr-2 h-4 w-4" />
                                刷新/重新生成
                              </>
                            )}
                          </Button>
                        </>
                      ) : (
                        <Button
                          onClick={handleEnable}
                          disabled={isActionLoading}
                        >
                          {isActionLoading ? (
                            <>
                              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                              启用中...
                            </>
                          ) : (
                            <>
                              <Shield className="mr-2 h-4 w-4" />
                              启用
                            </>
                          )}
                        </Button>
                      )}
                    </>
                  )}
                </div>

                <Alert>
                  <AlertTriangle className="h-4 w-4" />
                  <AlertTitle>安全提示</AlertTitle>
                  <AlertDescription>
                    <ul className="list-disc list-inside space-y-1 mt-2">
                      <li>API Key 具有与您的账号相同的权限，请勿泄露给他人</li>
                      <li>如果怀疑API Key泄露，请立即刷新生成新的API Key</li>
                      <li>禁用API Key后，使用该Key的外部接口将无法访问</li>
                      <li>刷新API Key会生成新的Key，旧的Key将立即失效</li>
                    </ul>
                  </AlertDescription>
                </Alert>
              </>
            )}
          </CardContent>
        </Card>
      </motion.div>

      {/* Confirm Dialog */}
      <Dialog open={showConfirmDialog} onOpenChange={setShowConfirmDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认操作</DialogTitle>
            <DialogDescription>
              {confirmAction === "generate" && "确定要生成新的API Key吗？"}
              {confirmAction === "disable" && "确定要禁用API Key吗？禁用后使用该Key的外部接口将无法访问。"}
              {confirmAction === "refresh" && "确定要刷新API Key吗？旧的Key将立即失效，需要更新所有使用该Key的外部接口。"}
            </DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2 mt-4">
            <Button variant="outline" onClick={() => setShowConfirmDialog(false)}>
              取消
            </Button>
            <Button onClick={executeAction} disabled={isActionLoading}>
              {isActionLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  处理中...
                </>
              ) : (
                "确认"
              )}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </motion.div>
  );
}

