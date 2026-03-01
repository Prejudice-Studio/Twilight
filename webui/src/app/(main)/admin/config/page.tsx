"use client";

import { useCallback, useEffect, useState } from "react";
import { motion } from "framer-motion";
import { Settings, Save, Loader2, AlertTriangle, CheckCircle2 } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError } from "@/components/layout/page-state";
import { api } from "@/lib/api";
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

export default function AdminConfigPage() {
  const { toast } = useToast();
  const [configContent, setConfigContent] = useState("");
  const [originalContent, setOriginalContent] = useState("");
  const [isSaving, setIsSaving] = useState(false);
  const [configPath, setConfigPath] = useState("");
  const [hasChanges, setHasChanges] = useState(false);

  useEffect(() => {
    setHasChanges(configContent !== originalContent);
  }, [configContent, originalContent]);

  const loadConfigResource = useCallback(async () => {
    const res = await api.getConfigToml();
    if (res.success && res.data) {
      setConfigContent(res.data.content);
      setOriginalContent(res.data.content);
      setConfigPath(res.data.path);
    } else {
      throw new Error(res.message || "无法加载配置文件");
    }
    return true;
  }, []);

  const {
    isLoading,
    error,
    execute: loadConfig,
  } = useAsyncResource(loadConfigResource, { immediate: true });

  const handleSave = async () => {
    if (!hasChanges) {
      toast({
        title: "没有更改",
        description: "配置文件未修改",
        variant: "default",
      });
      return;
    }

    setIsSaving(true);
    try {
      const res = await api.updateConfigToml(configContent);
      if (res.success) {
        setOriginalContent(configContent);
        setHasChanges(false);
        toast({
          title: "保存成功",
          description: "配置文件已更新并重新加载",
          variant: "success",
        });
      } else {
        toast({
          title: "保存失败",
          description: res.message || "无法保存配置文件",
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: "保存失败",
        description: error.message || "请检查网络连接",
        variant: "destructive",
      });
    } finally {
      setIsSaving(false);
    }
  };

  if (error) {
    return <PageError message={error} onRetry={() => void loadConfig()} />;
  }

  return (
    <motion.div
      variants={container}
      initial="hidden"
      animate="show"
      className="space-y-6"
    >
      <div>
        <h1 className="text-3xl font-bold">配置管理</h1>
        <p className="text-muted-foreground">查看和修改项目配置文件</p>
      </div>

      {configPath && (
        <Alert>
          <AlertTriangle className="h-4 w-4" />
          <AlertTitle>配置文件路径</AlertTitle>
          <AlertDescription>
            {configPath}
            <br />
            <span className="text-xs text-muted-foreground">
              修改前会自动备份原文件为 config.toml.backup
            </span>
          </AlertDescription>
        </Alert>
      )}

      <motion.div variants={item}>
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle className="flex items-center gap-2">
                  <Settings className="h-5 w-5" />
                  config.toml
                </CardTitle>
                <CardDescription>
                  编辑配置文件内容，保存后会自动重新加载配置
                </CardDescription>
              </div>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  onClick={loadConfig}
                  disabled={isLoading || isSaving}
                >
                  {isLoading ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      加载中...
                    </>
                  ) : (
                    "重新加载"
                  )}
                </Button>
                <Button
                  onClick={handleSave}
                  disabled={isLoading || isSaving || !hasChanges}
                >
                  {isSaving ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      保存中...
                    </>
                  ) : (
                    <>
                      <Save className="mr-2 h-4 w-4" />
                      保存
                    </>
                  )}
                </Button>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <div className="flex items-center justify-center h-96">
                <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
              </div>
            ) : (
              <div className="space-y-4">
                {hasChanges && (
                  <Alert>
                    <AlertTriangle className="h-4 w-4" />
                    <AlertDescription>
                      检测到未保存的更改，请点击保存按钮应用更改
                    </AlertDescription>
                  </Alert>
                )}
                <Textarea
                  value={configContent}
                  onChange={(e) => setConfigContent(e.target.value)}
                  className="font-mono text-sm min-h-[600px]"
                  placeholder="配置文件内容..."
                />
                <div className="flex items-center justify-between text-sm text-muted-foreground">
                  <span>行数: {configContent.split("\n").length}</span>
                  <span>字符数: {configContent.length}</span>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </motion.div>
    </motion.div>
  );
}

