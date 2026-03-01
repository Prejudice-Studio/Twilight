"use client";

import React, { useCallback, useState } from "react";
import { motion } from "framer-motion";
import { Library, Loader2, Save, AlertCircle } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { useToast } from "@/hooks/use-toast";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError, PageLoading } from "@/components/layout/page-state";
import { api } from "@/lib/api";
import {
  Alert,
  AlertDescription,
} from "@/components/ui/alert";
import { Label } from "@/components/ui/label";

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

interface LibraryItem {
  id: string;
  name: string;
  type: string;
  is_nsfw: boolean;
}

export default function NsfwLibraryPage() {
  const { toast } = useToast();
  const [libraries, setLibraries] = useState<LibraryItem[]>([]);
  const [selectedLibraryId, setSelectedLibraryId] = useState<string>("");
  const [isSaving, setIsSaving] = useState(false);

  const loadLibrariesResource = useCallback(async () => {
    const res = await api.getEmbyLibraries();
    if (res.success && res.data) {
      setLibraries(res.data);
      const nsfwLib = res.data.find((lib) => lib.is_nsfw);
      setSelectedLibraryId(nsfwLib?.id || "");
    } else {
      throw new Error(res.message || "无法加载媒体库列表");
    }
    return true;
  }, []);

  const {
    isLoading,
    error,
    execute: loadLibraries,
  } = useAsyncResource(loadLibrariesResource, { immediate: true });

  const handleSave = async () => {
    setIsSaving(true);
    try {
      const res = await api.updateNsfwLibrary(selectedLibraryId);
      if (res.success) {
        toast({
          title: "保存成功",
          description: `NSFW 库已更新为: ${libraries.find((lib) => lib.id === selectedLibraryId)?.name || "未选择"}`,
        });
        // 重新加载以更新状态
        await loadLibraries();
      } else {
        toast({
          title: "保存失败",
          description: res.message || "无法更新 NSFW 库配置",
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
    return <PageError message={error} onRetry={() => void loadLibraries()} />;
  }

  if (isLoading) {
    return <PageLoading message="正在加载媒体库..." />;
  }

  return (
    <motion.div
      variants={container}
      initial="hidden"
      animate="show"
      className="space-y-6"
    >
      <div>
        <h1 className="text-3xl font-bold tracking-tight">NSFW 库管理</h1>
        <p className="text-muted-foreground">
          选择 Emby 媒体库作为 NSFW（成人内容）库，用户需要特殊权限才能访问
        </p>
      </div>

      <motion.div variants={item}>
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Library className="h-5 w-5" />
              选择 NSFW 库
            </CardTitle>
            <CardDescription>
              从 Emby 服务器中选择一个媒体库作为 NSFW 库。选择后，只有拥有 NSFW 权限的用户才能访问该库。
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            {libraries.length === 0 ? (
              <Alert>
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>
                  未找到任何媒体库。请检查 Emby 服务器连接配置。
                </AlertDescription>
              </Alert>
            ) : (
              <>
                <div className="space-y-3">
                  <div
                    onClick={() => setSelectedLibraryId("")}
                    className={`flex cursor-pointer items-center space-x-3 rounded-lg border p-3 transition-all ${
                      selectedLibraryId === ""
                        ? "border-primary bg-primary/5"
                        : "hover:bg-accent"
                    }`}
                  >
                    <div
                      className={`h-4 w-4 rounded-full border-2 ${
                        selectedLibraryId === ""
                          ? "border-primary bg-primary"
                          : "border-muted-foreground"
                      }`}
                    >
                      {selectedLibraryId === "" && (
                        <div className="h-full w-full rounded-full bg-primary" />
                      )}
                    </div>
                    <Label className="flex-1 cursor-pointer font-normal">
                      <div className="flex items-center justify-between">
                        <span>不设置 NSFW 库</span>
                        <span className="text-xs text-muted-foreground">
                          禁用 NSFW 功能
                        </span>
                      </div>
                    </Label>
                  </div>
                  {libraries.map((library) => (
                    <div
                      key={library.id}
                      onClick={() => setSelectedLibraryId(library.id)}
                      className={`flex cursor-pointer items-center space-x-3 rounded-lg border p-3 transition-all ${
                        selectedLibraryId === library.id
                          ? "border-primary bg-primary/5"
                          : "hover:bg-accent"
                      }`}
                    >
                      <div
                        className={`h-4 w-4 rounded-full border-2 ${
                          selectedLibraryId === library.id
                            ? "border-primary bg-primary"
                            : "border-muted-foreground"
                        }`}
                      >
                        {selectedLibraryId === library.id && (
                          <div className="h-full w-full rounded-full bg-primary" />
                        )}
                      </div>
                      <Label className="flex-1 cursor-pointer font-normal">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-2">
                            <span className="font-medium">{library.name}</span>
                            <span className="rounded bg-muted px-2 py-0.5 text-xs text-muted-foreground">
                              {library.type || "未知"}
                            </span>
                            {library.is_nsfw && (
                              <span className="rounded bg-primary/10 px-2 py-0.5 text-xs text-primary">
                                当前 NSFW 库
                              </span>
                            )}
                          </div>
                          <span className="text-xs text-muted-foreground">
                            ID: {library.id}
                          </span>
                        </div>
                      </Label>
                    </div>
                  ))}
                </div>

                <div className="flex items-center justify-between rounded-lg bg-accent/50 p-4">
                  <div>
                    <p className="font-medium">当前选择</p>
                    <p className="text-sm text-muted-foreground">
                      {selectedLibraryId
                        ? libraries.find((lib) => lib.id === selectedLibraryId)
                            ?.name || "未知"
                        : "未选择（禁用 NSFW 功能）"}
                    </p>
                  </div>
                  <Button
                    onClick={handleSave}
                    disabled={isSaving}
                    className="min-w-[100px]"
                  >
                    {isSaving ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        保存中...
                      </>
                    ) : (
                      <>
                        <Save className="mr-2 h-4 w-4" />
                        保存配置
                      </>
                    )}
                  </Button>
                </div>

                <Alert>
                  <AlertCircle className="h-4 w-4" />
                  <AlertDescription>
                    <strong>注意：</strong>保存后，配置将立即更新到{" "}
                    <code className="rounded bg-muted px-1 py-0.5 text-xs">
                      config.toml
                    </code>
                    ，并自动重新加载。只有拥有 NSFW 权限的用户才能访问选中的媒体库。
                  </AlertDescription>
                </Alert>
              </>
            )}
          </CardContent>
        </Card>
      </motion.div>
    </motion.div>
  );
}

