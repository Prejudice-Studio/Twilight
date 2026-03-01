"use client";

import React, { useCallback, useState } from "react";
import { motion } from "framer-motion";
import { Palette, RefreshCw, Loader2, Check, X, Upload } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { useToast } from "@/hooks/use-toast";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError, PageLoading } from "@/components/layout/page-state";
import { useAuthStore } from "@/store/auth";
import { api } from "@/lib/api";
import { emitRegionRefresh, RegionRefreshKeys } from "@/lib/region-refresh";

const container = {
  hidden: { opacity: 0 },
  show: { opacity: 1, transition: { staggerChildren: 0.1 } },
};

const item = {
  hidden: { opacity: 0, y: 10 },
  show: { opacity: 1, y: 0 },
};

const gradientPresets = [
  {
    name: "蓝色渐变",
    value: "linear-gradient(135deg, #9db7ff 0%, #d3b1ff 100%)",
  },
  {
    name: "紫粉渐变",
    value: "linear-gradient(135deg, #f5b8ff 0%, #ffb8c9 100%)",
  },
  {
    name: "绿清渐变",
    value: "linear-gradient(135deg, #9fd9ff 0%, #9ff6f8 100%)",
  },
  {
    name: "金橙渐变",
    value: "linear-gradient(135deg, #ffc0d5 0%, #ffe9a6 100%)",
  },
  {
    name: "深紫渐变",
    value: "linear-gradient(135deg, #3b3b64 0%, #585884 100%)",
  },
  {
    name: "午夜黑",
    value: "linear-gradient(135deg, #27234a 0%, #4a4585 50%, #3f3f6b 100%)",
  },
];

export default function BackgroundSettingsPage() {
  const { user } = useAuthStore();
  const { toast } = useToast();
  const [loading, setLoading] = useState(false);
  const [uploading, setUploading] = useState<"light" | "dark" | null>(null);
  const [lightBg, setLightBg] = useState("");
  const [darkBg, setDarkBg] = useState("");
  const [lightBgImage, setLightBgImage] = useState("");
  const [darkBgImage, setDarkBgImage] = useState("");
  const [lightPreview, setLightPreview] = useState("");
  const [darkPreview, setDarkPreview] = useState("");

  const loadBackgroundResource = useCallback(async () => {
    if (!user?.uid) return true;
    const res = await api.getUserBackground(user.uid);
    if (res.success && res.data?.background) {
      const config = JSON.parse(res.data.background);
      setLightBg(config.lightBg || "");
      setDarkBg(config.darkBg || "");
      setLightBgImage(config.lightBgImage || "");
      setDarkBgImage(config.darkBgImage || "");
      updatePreview(config.lightBg || "", config.lightBgImage || "", "light");
      updatePreview(config.darkBg || "", config.darkBgImage || "", "dark");
    }
    return true;
  }, [user?.uid]);

  const {
    isLoading: pageLoading,
    error,
    execute: loadBackgroundConfig,
  } = useAsyncResource(loadBackgroundResource, { immediate: true });

  const updatePreview = (css: string, img: string, type: "light" | "dark") => {
    const combined = img || css;
    
    if (type === "light") {
      setLightPreview(combined);
    } else {
      setDarkPreview(combined);
    }
  };

  const handleLightBgChange = (value: string) => {
    setLightBg(value);
    updatePreview(value, lightBgImage, "light");
  };

  const handleDarkBgChange = (value: string) => {
    setDarkBg(value);
    updatePreview(value, darkBgImage, "dark");
  };

  const handleLightImageChange = (value: string) => {
    setLightBgImage(value);
    updatePreview(lightBg, value, "light");
  };

  const handleDarkImageChange = (value: string) => {
    setDarkBgImage(value);
    updatePreview(darkBg, value, "dark");
  };

  const handleFileUpload = async (file: File, type: "light" | "dark") => {
    if (!file) return;

    // 验证文件类型
    const allowedTypes = ["image/jpeg", "image/png", "image/gif", "image/webp"];
    if (!allowedTypes.includes(file.type)) {
      toast({
        title: "错误",
        description: "只支持 JPG、PNG、GIF、WebP 格式的图片",
        variant: "destructive",
      });
      return;
    }

    // 验证文件大小
    if (file.size > 5 * 1024 * 1024) {
      toast({
        title: "错误",
        description: "文件大小不能超过 5MB",
        variant: "destructive",
      });
      return;
    }

    setUploading(type);
    try {
      const res = await api.uploadBackgroundImage(file, type);

      if (res.success && res.data?.url) {
        const imageUrl = `url(${res.data.url})`;
        
        if (type === "light") {
          setLightBgImage(imageUrl);
          updatePreview(lightBg, imageUrl, "light");
        } else {
          setDarkBgImage(imageUrl);
          updatePreview(darkBg, imageUrl, "dark");
        }

        toast({
          title: "成功",
          description: "图片上传成功，点击保存应用",
        });
      } else {
        toast({
          title: "错误",
          description: res.message || "上传失败",
          variant: "destructive",
        });
      }
    } catch (error) {
      toast({
        title: "错误",
        description: "上传失败，请重试",
        variant: "destructive",
      });
    } finally {
      setUploading(null);
    }
  };

  const handleSave = async () => {
    if (!lightBg && !darkBg && !lightBgImage && !darkBgImage) {
      toast({
        title: "错误",
        description: "至少需要配置一个背景",
        variant: "destructive",
      });
      return;
    }

    setLoading(true);
    try {
      const res = await api.updateUserBackground({
        lightBg,
        darkBg,
        lightBgImage,
        darkBgImage,
      });

      if (res.success) {
        emitRegionRefresh(RegionRefreshKeys.UserBackground);
        toast({
          title: "成功",
          description: "背景已更新，刷新页面查看效果",
        });
      } else {
        toast({
          title: "错误",
          description: res.message || "保存失败",
          variant: "destructive",
        });
      }
    } catch (error) {
      toast({
        title: "错误",
        description: "保存失败，请重试",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  };

  const handleReset = async () => {
    setLoading(true);
    try {
      const res = await api.deleteUserBackground();

      if (res.success) {
        setLightBg("");
        setDarkBg("");
        setLightBgImage("");
        setDarkBgImage("");
        setLightPreview("");
        setDarkPreview("");
        emitRegionRefresh(RegionRefreshKeys.UserBackground);
        toast({
          title: "成功",
          description: "背景已重置为默认",
        });
      } else {
        toast({
          title: "错误",
          description: "重置失败",
          variant: "destructive",
        });
      }
    } catch (error) {
      toast({
        title: "错误",
        description: "重置失败，请重试",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  };

  const applyPreset = (type: "light" | "dark", value: string) => {
    if (type === "light") {
      setLightBg(value);
      setLightBgImage("");
      updatePreview(value, "", "light");
    } else {
      setDarkBg(value);
      setDarkBgImage("");
      updatePreview(value, "", "dark");
    }
  };

  if (error) {
    return <PageError message={error} onRetry={() => void loadBackgroundConfig()} />;
  }

  if (pageLoading) {
    return <PageLoading message="正在加载背景设置..." />;
  }

  const BackgroundSection = ({ type, bgCss, bgImage, preview, onBgChange, onImageChange, onUpload }: any) => {
    const title = type === "light" ? "浅色主题背景" : "暗色主题背景";
    const desc = type === "light" ? "自定义浅色主题下的背景样式" : "自定义暗色主题下的背景样式";
    const presets = type === "light" ? gradientPresets.slice(0, 4) : gradientPresets.slice(4);
    const bgKey = type === "light" ? "light" : "dark";
    const isUploading = uploading === bgKey;

    return (
      <motion.div variants={item}>
        <Card>
          <CardHeader>
            <CardTitle>{title}</CardTitle>
            <CardDescription>{desc}（支持 CSS 渐变、图片 URL、文件上传）</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {/* 预览 */}
            <div 
              className={`relative h-48 rounded-lg border-2 border-primary/20 overflow-hidden ${type === "dark" ? "bg-slate-950" : ""}`}
              style={{ backgroundImage: preview }}
            />

            {/* CSS 输入 */}
            <div className="space-y-2">
              <Label>CSS 背景样式（可选）</Label>
              <Textarea
                value={bgCss}
                onChange={(e) => onBgChange(e.target.value)}
                placeholder="例如: linear-gradient(135deg, #667eea 0%, #764ba2 100%)"
                className="font-mono text-sm h-20"
              />
              <p className="text-xs text-muted-foreground">
                支持任何有效的 CSS background-image 属性值
              </p>
            </div>

            {/* 图片URL输入 */}
            <div className="space-y-2">
              <Label>背景图片 URL（可选）</Label>
              <Input
                value={bgImage}
                onChange={(e) => onImageChange(e.target.value)}
                placeholder="例如: url(https://example.com/image.jpg)"
                className="font-mono text-sm"
              />
              <p className="text-xs text-muted-foreground">
                输入完整的 URL，格式: url(https://......)
              </p>
            </div>

            {/* 文件上传 */}
            <div className="space-y-2">
              <Label>或上传背景图片</Label>
              <div className="flex gap-2">
                <Input
                  id={`file-${bgKey}`}
                  type="file"
                  accept="image/*"
                  onChange={(e) => {
                    const file = e.currentTarget.files?.[0];
                    if (file) {
                      onUpload(file);
                      e.currentTarget.value = "";
                    }
                  }}
                  disabled={isUploading}
                  className="flex-1"
                />
                <Button
                  size="sm"
                  disabled={isUploading}
                  variant="outline"
                  onClick={() => document.getElementById(`file-${bgKey}`)?.click()}
                >
                  {isUploading ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Upload className="mr-2 h-4 w-4" />
                  )}
                  {isUploading ? "上传中..." : "选择文件"}
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">
                最大 5MB，支持 JPG、PNG、GIF、WebP
              </p>
            </div>

            {/* 预设 */}
            <div className="space-y-2">
              <Label>快速预设</Label>
              <div className="grid grid-cols-2 gap-2">
                {presets.map((preset) => (
                  <button
                    key={preset.name}
                    onClick={() => applyPreset(type as any, preset.value)}
                    className="relative h-12 rounded-lg border border-primary/20 hover:border-primary/50 transition-colors overflow-hidden group"
                    style={{ backgroundImage: preset.value }}
                  >
                    <div className="absolute inset-0 bg-black/40 group-hover:bg-black/50 transition-colors flex items-center justify-center">
                      <span className="text-white text-xs font-medium">{preset.name}</span>
                    </div>
                  </button>
                ))}
              </div>
            </div>
          </CardContent>
        </Card>
      </motion.div>
    );
  };

  return (
    <div className="space-y-6">
      <motion.div
        variants={container}
        initial="hidden"
        animate="show"
        className="space-y-4"
      >
        {/* 标题 */}
        <motion.div variants={item}>
          <div className="flex items-center gap-2">
            <Palette className="h-8 w-8 text-primary" />
            <div>
              <h1 className="text-3xl font-bold">背景设置</h1>
              <p className="text-muted-foreground">自定义您的主页背景（支持 CSS、URL、文件上传）</p>
            </div>
          </div>
        </motion.div>

        {/* 浅色背景 */}
        <BackgroundSection
          type="light"
          bgCss={lightBg}
          bgImage={lightBgImage}
          preview={lightPreview}
          onBgChange={handleLightBgChange}
          onImageChange={handleLightImageChange}
          onUpload={(file: File) => handleFileUpload(file, "light")}
        />

        {/* 暗色背景 */}
        <BackgroundSection
          type="dark"
          bgCss={darkBg}
          bgImage={darkBgImage}
          preview={darkPreview}
          onBgChange={handleDarkBgChange}
          onImageChange={handleDarkImageChange}
          onUpload={(file: File) => handleFileUpload(file, "dark")}
        />

        {/* 警告 */}
        <motion.div variants={item}>
          <Alert>
            <AlertDescription>
              💡 提示：支持 CSS 背景、图片 URL 和文件上传的组合。如果同时设置，图片会显示在CSS背景下面。修改后需要刷新页面才能看到效果。跨域图片需要支持 CORS。
            </AlertDescription>
          </Alert>
        </motion.div>

        {/* 操作按钮 */}
        <motion.div variants={item} className="flex gap-3">
          <Button
            onClick={handleSave}
            disabled={loading || uploading !== null}
            className="flex-1"
          >
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            保存设置
          </Button>
          <Button
            onClick={handleReset}
            disabled={loading || uploading !== null}
            variant="outline"
            className="flex-1"
          >
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            重置为默认
          </Button>
        </motion.div>
      </motion.div>
    </div>
  );
}
