"use client";

import React, { useCallback, useState } from "react";
import Image from "next/image";
import { motion } from "framer-motion";
import {
  Palette,
  Upload,
  Loader2,
  Trash2,
  Eye,
  Download,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useToast } from "@/hooks/use-toast";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError, PageLoading } from "@/components/layout/page-state";
import { useAuthStore } from "@/store/auth";
import { api } from "@/lib/api";
import { emitRegionRefresh, RegionRefreshKeys } from "@/lib/region-refresh";
import { normalizeBackgroundImageValue } from "@/lib/safe-url";

const container = {
  hidden: { opacity: 0 },
  show: { opacity: 1, transition: { staggerChildren: 0.1 } },
};

const item = {
  hidden: { opacity: 0, y: 10 },
  show: { opacity: 1, y: 0 },
};

interface BackgroundConfig {
  lightBg: string;
  darkBg: string;
  lightBgImage: string;
  darkBgImage: string;
  lightFlow: boolean;
  darkFlow: boolean;
  lightBlur: number;
  darkBlur: number;
  lightOpacity: number;
  darkOpacity: number;
}

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
  {
    name: "海洋蓝",
    value: "linear-gradient(135deg, #2f4f91 0%, #5a85d6 100%)",
  },
  {
    name: "樱花粉",
    value: "linear-gradient(135deg, #ffd3e9 0%, #ffc0df 100%)",
  },
];

export default function AppearanceSettingsPage() {
  const { user } = useAuthStore();
  const { toast } = useToast();

  // 背景配置
  const [bgConfig, setBgConfig] = useState<BackgroundConfig>({
    lightBg: "",
    darkBg: "",
    lightBgImage: "",
    darkBgImage: "",
    lightFlow: false,
    darkFlow: false,
    lightBlur: 0,
    darkBlur: 0,
    lightOpacity: 100,
    darkOpacity: 100,
  });

  // 头像
  const [avatar, setAvatar] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const [bgUploading, setBgUploading] = useState<"light" | "dark" | null>(null);
  const [saving, setSaving] = useState(false);

  // 预览
  const [lightPreview, setLightPreview] = useState("");
  const [darkPreview, setDarkPreview] = useState("");

  // 分发标签页
  const [activeTab, setActiveTab] = useState<"background" | "avatar">("background");

  const updatePreview = useCallback((css: string, img: string, type: "light" | "dark") => {
    const combined = normalizeBackgroundImageValue(img) || normalizeBackgroundImageValue(css);

    if (type === "light") {
      setLightPreview(combined);
    } else {
      setDarkPreview(combined);
    }
  }, []);

  const loadAppearanceResource = useCallback(async () => {
    if (!user?.uid) return true;

    const [bgRes, avatarRes] = await Promise.all([
      api.getUserBackground(user.uid),
      api.getUserAvatar(user.uid),
    ]);

    if (bgRes.success && bgRes.data?.background) {
      const config = JSON.parse(bgRes.data.background);
      setBgConfig({
        lightBg: config.lightBg || "",
        darkBg: config.darkBg || "",
        lightBgImage: config.lightBgImage || "",
        darkBgImage: config.darkBgImage || "",
        lightFlow: Boolean(config.lightFlow),
        darkFlow: Boolean(config.darkFlow),
        lightBlur: Number(config.lightBlur ?? 0),
        darkBlur: Number(config.darkBlur ?? 0),
        lightOpacity: Number(config.lightOpacity ?? 100),
        darkOpacity: Number(config.darkOpacity ?? 100),
      });
      updatePreview(config.lightBg || "", config.lightBgImage || "", "light");
      updatePreview(config.darkBg || "", config.darkBgImage || "", "dark");
    }

    if (avatarRes.success && avatarRes.data?.avatar) {
      setAvatar(avatarRes.data.avatar);
    }

    return true;
  }, [user?.uid, updatePreview]);

  const {
    isLoading: loading,
    error,
    execute: loadAppearanceConfig,
  } = useAsyncResource(loadAppearanceResource, { immediate: true });

  const handleBgChange = (field: keyof BackgroundConfig, value: string) => {
    const newConfig = { ...bgConfig, [field]: value };
    setBgConfig(newConfig);

    if (field === "lightBg" || field === "lightBgImage") {
      updatePreview(
        field === "lightBg" ? value : newConfig.lightBg,
        field === "lightBgImage" ? value : newConfig.lightBgImage,
        "light"
      );
    } else {
      updatePreview(
        field === "darkBg" ? value : newConfig.darkBg,
        field === "darkBgImage" ? value : newConfig.darkBgImage,
        "dark"
      );
    }
  };

  const applyPreset = (type: "light" | "dark", value: string) => {
    if (type === "light") {
      const newConfig = {
        ...bgConfig,
        lightBg: value,
        lightBgImage: "",
      };
      setBgConfig(newConfig);
      updatePreview(newConfig.lightBg, newConfig.lightBgImage, "light");
    } else {
      const newConfig = {
        ...bgConfig,
        darkBg: value,
        darkBgImage: "",
      };
      setBgConfig(newConfig);
      updatePreview(newConfig.darkBg, newConfig.darkBgImage, "dark");
    }
  };

  const handleFlowToggle = (type: "light" | "dark", checked: boolean) => {
    setBgConfig((prev) => {
      if (type === "light") {
        const next = {
          ...prev,
          lightFlow: checked,
          lightBgImage: checked ? "" : prev.lightBgImage,
        };
        updatePreview(next.lightBg, next.lightBgImage, "light");
        return next;
      }

      const next = {
        ...prev,
        darkFlow: checked,
        darkBgImage: checked ? "" : prev.darkBgImage,
      };
      updatePreview(next.darkBg, next.darkBgImage, "dark");
      return next;
    });
  };

  const handleVisualChange = (
    field: "lightBlur" | "darkBlur" | "lightOpacity" | "darkOpacity",
    value: number
  ) => {
    setBgConfig((prev) => ({
      ...prev,
      [field]: value,
    }));
  };

  const handleAvatarUpload = async (file: File) => {
    if (!file) return;

    const allowedTypes = ["image/jpeg", "image/png", "image/gif", "image/webp"];
    if (!allowedTypes.includes(file.type)) {
      toast({
        title: "错误",
        description: "只支持 JPG、PNG、GIF、WebP 格式的图片",
        variant: "destructive",
      });
      return;
    }

    if (file.size > 2 * 1024 * 1024) {
      toast({
        title: "错误",
        description: "文件大小不能超过 2MB",
        variant: "destructive",
      });
      return;
    }

    setUploading(true);
    try {
      const res = await api.uploadAvatar(file);

      if (res.success && res.data?.avatar_url) {
        setAvatar(res.data.avatar_url);
        emitRegionRefresh(RegionRefreshKeys.UserProfile);
        toast({
          title: "成功",
          description: "头像上传成功",
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
      setUploading(false);
    }
  };

  const handleBgFileUpload = async (file: File, type: "light" | "dark") => {
    if (!file) return;

    const allowedTypes = ["image/jpeg", "image/png", "image/gif", "image/webp"];
    if (!allowedTypes.includes(file.type)) {
      toast({
        title: "错误",
        description: "只支持 JPG、PNG、GIF、WebP 格式的图片",
        variant: "destructive",
      });
      return;
    }

    if (file.size > 5 * 1024 * 1024) {
      toast({
        title: "错误",
        description: "背景图大小不能超过 5MB",
        variant: "destructive",
      });
      return;
    }

    setBgUploading(type);
    try {
      const res = await api.uploadBackgroundImage(file, type);
      if (res.success && res.data?.url) {
        const cssUrl = `url("${res.data.url}")`;
        if (type === "light") {
          handleBgChange("lightBgImage", cssUrl);
        } else {
          handleBgChange("darkBgImage", cssUrl);
        }
        toast({
          title: "成功",
          description: `${type === "light" ? "浅色" : "暗色"}背景上传成功，请点击保存背景`,
        });
      } else {
        toast({
          title: "错误",
          description: res.message || "上传失败",
          variant: "destructive",
        });
      }
    } catch {
      toast({
        title: "错误",
        description: "上传失败，请重试",
        variant: "destructive",
      });
    } finally {
      setBgUploading(null);
    }
  };

  const handleDeleteAvatar = async () => {
    if (!avatar) return;

    setSaving(true);
    try {
      const res = await api.deleteAvatar();

      if (res.success) {
        setAvatar(null);
        emitRegionRefresh(RegionRefreshKeys.UserProfile);
        toast({
          title: "成功",
          description: "头像已删除",
        });
      } else {
        toast({
          title: "错误",
          description: "删除失败",
          variant: "destructive",
        });
      }
    } catch (error) {
      toast({
        title: "错误",
        description: "删除失败，请重试",
        variant: "destructive",
      });
    } finally {
      setSaving(false);
    }
  };

  const handleSaveBg = async () => {
    if (
      !bgConfig.lightBg &&
      !bgConfig.darkBg &&
      !bgConfig.lightBgImage &&
      !bgConfig.darkBgImage
    ) {
      toast({
        title: "错误",
        description: "至少需要配置一个背景",
        variant: "destructive",
      });
      return;
    }

    setSaving(true);
    try {
      const res = await api.updateUserBackground(bgConfig);

      if (res.success) {
        emitRegionRefresh(RegionRefreshKeys.UserBackground);
        toast({
          title: "成功",
          description: "背景已保存",
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
      setSaving(false);
    }
  };

  const handleResetBg = async () => {
    setSaving(true);
    try {
      const res = await api.deleteUserBackground();

      if (res.success) {
        setBgConfig({
          lightBg: "",
          darkBg: "",
          lightBgImage: "",
          darkBgImage: "",
          lightFlow: false,
          darkFlow: false,
          lightBlur: 0,
          darkBlur: 0,
          lightOpacity: 100,
          darkOpacity: 100,
        });
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
      setSaving(false);
    }
  };

  if (error) {
    return <PageError message={error} onRetry={() => void loadAppearanceConfig()} />;
  }

  if (loading) {
    return <PageLoading message="正在加载外观设置..." />;
  }

  const renderBackgroundPanel = (theme: "light" | "dark") => {
    const isLight = theme === "light";
    const title = isLight ? "浅色主题背景" : "暗色主题背景";
    const description = isLight ? "用于白天或浅色模式下的页面底图" : "用于夜间或暗色模式下的页面底图";
    const bgField = isLight ? "lightBg" : "darkBg";
    const imageField = isLight ? "lightBgImage" : "darkBgImage";
    const blurField = isLight ? "lightBlur" : "darkBlur";
    const opacityField = isLight ? "lightOpacity" : "darkOpacity";
    const bgValue = bgConfig[bgField];
    const imageValue = bgConfig[imageField];
    const flow = isLight ? bgConfig.lightFlow : bgConfig.darkFlow;
    const blur = bgConfig[blurField];
    const opacity = bgConfig[opacityField];
    const preview = isLight ? lightPreview : darkPreview;
    const uploadId = `${theme}-bg-upload`;
    const isUploading = bgUploading === theme;
    const fallback = isLight
      ? "linear-gradient(135deg, #f8fafc 0%, #dbeafe 52%, #f5d0fe 100%)"
      : "linear-gradient(135deg, #111827 0%, #312e81 52%, #0f172a 100%)";

    return (
      <Card key={theme} className="overflow-hidden border-border/80 bg-card/70 backdrop-blur-sm">
        <CardHeader className="border-b border-border/60 bg-muted/20">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0 space-y-1">
              <CardTitle className="flex items-center gap-2 text-lg">
                <Palette className={`h-5 w-5 ${isLight ? "text-amber-500" : "text-slate-400"}`} />
                {title}
              </CardTitle>
              <CardDescription>{description}</CardDescription>
            </div>
            <Badge variant="outline" className="w-fit text-[11px]">
              {imageValue ? "图片优先" : bgValue ? "CSS 背景" : "使用默认"}
            </Badge>
          </div>
        </CardHeader>

        <CardContent className="grid gap-5 p-4 lg:grid-cols-[minmax(240px,0.9fr)_minmax(0,1.35fr)] lg:p-6">
          <div className="space-y-3">
            <div className="flex items-center justify-between gap-3">
              <Label className="flex items-center gap-2 text-sm">
                <Eye className="h-4 w-4 text-muted-foreground" />
                实时预览
              </Label>
              <span className="text-xs text-muted-foreground">
                模糊 {blur}px · 透明度 {opacity}%
              </span>
            </div>
            <div className="relative min-h-[260px] overflow-hidden rounded-2xl border border-border bg-muted shadow-inner">
              <div
                className="absolute inset-0"
                style={{
                  background: preview || fallback,
                  backgroundPosition: "center",
                  backgroundSize: flow ? "220% 220%" : "cover",
                  animation: flow ? "twilight-gradient-flow 14s ease infinite" : undefined,
                  filter: `blur(${blur}px)`,
                  opacity: opacity / 100,
                  transform: blur > 0 ? "scale(1.06)" : undefined,
                }}
              />
              <div className="absolute inset-0 bg-gradient-to-br from-black/35 via-black/10 to-black/45" />
              <div className="relative z-10 flex min-h-[260px] flex-col justify-between p-4 text-white">
                <div>
                  <p className="text-xs uppercase tracking-[0.25em] text-white/70">Twilight</p>
                  <p className="mt-2 text-lg font-semibold">{isLight ? "Light Mode" : "Dark Mode"}</p>
                </div>
                <div className="rounded-xl border border-white/20 bg-white/15 p-3 backdrop-blur-md">
                  <p className="text-sm font-medium">背景预览区域</p>
                  <p className="mt-1 text-xs text-white/75">保存后将在主页和侧边区域刷新显示</p>
                </div>
              </div>
            </div>
          </div>

          <div className="space-y-5">
            <div className="space-y-2">
              <Label className="text-sm">快速应用预设梯度</Label>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                {gradientPresets.map((preset) => {
                  const selected = bgValue === preset.value && !imageValue;
                  return (
                    <motion.button
                      key={preset.name}
                      type="button"
                      whileHover={{ scale: 1.03 }}
                      whileTap={{ scale: 0.98 }}
                      className={`group overflow-hidden rounded-xl border bg-background text-left transition-all ${
                        selected ? "border-primary shadow-sm" : "border-border/70 hover:border-primary/50"
                      }`}
                      onClick={() => applyPreset(theme, preset.value)}
                    >
                      <div className="h-12" style={{ background: preset.value }} />
                      <div className="px-2 py-1.5 text-xs text-muted-foreground group-hover:text-foreground">
                        {preset.name}
                      </div>
                    </motion.button>
                  );
                })}
              </div>
            </div>

            <div className="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-center">
              <div className="rounded-xl border border-border/70 bg-muted/20 px-3 py-2">
                <Label htmlFor={`${theme}-flow`}>流光渐变</Label>
                <p className="text-xs text-muted-foreground">开启后预设渐变会缓慢流动，并自动清空该主题的图片 URL。</p>
              </div>
              <Switch
                id={`${theme}-flow`}
                checked={flow}
                onCheckedChange={(checked) => handleFlowToggle(theme, checked)}
                className="justify-self-start sm:justify-self-end"
              />
            </div>

            <Separator />

            <div className="grid gap-4 xl:grid-cols-2">
              <div className="space-y-2 xl:col-span-2">
                <Label htmlFor={`${theme}-bg-css`}>CSS 梯度或颜色</Label>
                <Textarea
                  id={`${theme}-bg-css`}
                  placeholder="例如: linear-gradient(135deg, #667eea 0%, #764ba2 100%)"
                  value={bgValue}
                  onChange={(e) => handleBgChange(bgField, e.target.value)}
                  rows={3}
                  className="min-h-[92px] font-mono text-sm"
                />
              </div>

              <div className="space-y-2 xl:col-span-2">
                <Label htmlFor={`${theme}-bg-url`}>背景图片 URL</Label>
                <Input
                  id={`${theme}-bg-url`}
                  placeholder="https://example.com/image.jpg 或 url(https://...)"
                  value={imageValue}
                  onChange={(e) => handleBgChange(imageField, e.target.value)}
                  className="font-mono text-sm"
                />
              </div>

              <div className="space-y-2 xl:col-span-2">
                <Label htmlFor={uploadId}>本地上传背景图</Label>
                <label
                  htmlFor={uploadId}
                  className={`flex min-h-11 cursor-pointer items-center justify-between gap-3 rounded-xl border border-dashed border-border bg-muted/20 px-3 py-2 text-sm transition-colors hover:border-primary/50 hover:bg-muted/35 ${
                    isUploading ? "pointer-events-none opacity-60" : ""
                  }`}
                >
                  <span className="min-w-0">
                    <span className="block font-medium">选择 JPG / PNG / GIF / WebP</span>
                    <span className="block text-xs text-muted-foreground">最大 5MB，上传后仍需点击保存背景</span>
                  </span>
                  {isUploading ? <Loader2 className="h-4 w-4 shrink-0 animate-spin text-primary" /> : <Upload className="h-4 w-4 shrink-0 text-muted-foreground" />}
                  <Input
                    id={uploadId}
                    type="file"
                    accept="image/*"
                    disabled={isUploading}
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) {
                        void handleBgFileUpload(file, theme);
                        e.currentTarget.value = "";
                      }
                    }}
                    className="sr-only"
                  />
                </label>
              </div>

              <div className="space-y-2 rounded-xl border border-border/60 bg-muted/20 p-3">
                <div className="flex items-center justify-between gap-3">
                  <Label htmlFor={`${theme}-blur`}>模糊程度</Label>
                  <span className="text-xs tabular-nums text-muted-foreground">{blur}px</span>
                </div>
                <Input
                  id={`${theme}-blur`}
                  type="range"
                  min={0}
                  max={30}
                  step={1}
                  value={blur}
                  onChange={(e) => handleVisualChange(blurField, Number(e.target.value))}
                />
              </div>

              <div className="space-y-2 rounded-xl border border-border/60 bg-muted/20 p-3">
                <div className="flex items-center justify-between gap-3">
                  <Label htmlFor={`${theme}-opacity`}>透明度</Label>
                  <span className="text-xs tabular-nums text-muted-foreground">{opacity}%</span>
                </div>
                <Input
                  id={`${theme}-opacity`}
                  type="range"
                  min={10}
                  max={100}
                  step={1}
                  value={opacity}
                  onChange={(e) => handleVisualChange(opacityField, Number(e.target.value))}
                />
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    );
  };

  return (
    <motion.div
      variants={container}
      initial="hidden"
      animate="show"
      className="space-y-6"
    >
      <Tabs value={activeTab} onValueChange={(value) => setActiveTab(value as "background" | "avatar")} className="space-y-5">
        <motion.div variants={item} className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div className="min-w-0 space-y-1">
            <h1 className="text-2xl font-bold sm:text-3xl">外观设置</h1>
            <p className="text-sm text-muted-foreground">统一管理个人背景与头像，保存后会同步刷新相关区域。</p>
          </div>
          <TabsList className="grid h-auto w-full grid-cols-2 sm:w-auto">
            <TabsTrigger value="background" className="gap-2">
              <Palette className="h-4 w-4" />
              背景主题
            </TabsTrigger>
            <TabsTrigger value="avatar" className="gap-2">
              <Upload className="h-4 w-4" />
              用户头像
            </TabsTrigger>
          </TabsList>
        </motion.div>

        <TabsContent value="background" className="mt-0 space-y-5">
          <motion.div variants={item} className="grid gap-5">
            {renderBackgroundPanel("light")}
            {renderBackgroundPanel("dark")}

            <div className="flex flex-col gap-3 rounded-2xl border border-border/80 bg-card/70 p-4 shadow-sm sm:flex-row sm:items-center sm:justify-between">
              <div className="min-w-0">
                <p className="text-sm font-medium">保存背景配置</p>
                <p className="text-xs text-muted-foreground">预览和表单变更只保存在当前页面，点击保存后才会应用。</p>
              </div>
              <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
                <Button onClick={handleResetBg} variant="outline" disabled={saving} className="sm:w-auto">
                  <Trash2 className="mr-2 h-4 w-4" />
                  重置为默认
                </Button>
                <Button onClick={handleSaveBg} disabled={saving} className="sm:w-auto">
                  {saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Download className="mr-2 h-4 w-4" />}
                  保存背景
                </Button>
              </div>
            </div>
          </motion.div>
        </TabsContent>

        <TabsContent value="avatar" className="mt-0">
          <motion.div variants={item} className="grid gap-5 lg:grid-cols-[320px_minmax(0,1fr)]">
            <Card className="border-border/80 bg-card/70 backdrop-blur-sm">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-lg">
                  <Upload className="h-5 w-5 text-blue-500" />
                  当前头像
                </CardTitle>
                <CardDescription>头像会在导航、个人信息和管理列表中展示。</CardDescription>
              </CardHeader>
              <CardContent className="flex flex-col items-center gap-4">
                <div className="relative rounded-full bg-gradient-to-br from-primary/30 via-sky-500/20 to-fuchsia-500/20 p-1.5">
                  <div className="flex h-40 w-40 items-center justify-center overflow-hidden rounded-full border border-border bg-muted shadow-inner">
                    {avatar ? (
                      <Image
                        src={avatar}
                        alt="用户头像"
                        width={160}
                        height={160}
                        unoptimized
                        className="h-full w-full object-cover"
                      />
                    ) : (
                      <div className="text-center">
                        <Upload className="mx-auto h-9 w-9 text-muted-foreground" />
                        <p className="mt-2 text-xs text-muted-foreground">未设置</p>
                      </div>
                    )}
                  </div>
                </div>
                <Badge variant={avatar ? "success" : "outline"} className="text-xs">
                  {avatar ? "已设置个人头像" : "当前使用默认头像"}
                </Badge>
                {avatar && (
                  <Button onClick={handleDeleteAvatar} variant="destructive" disabled={saving || uploading} className="w-full">
                    {saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Trash2 className="mr-2 h-4 w-4" />}
                    删除头像
                  </Button>
                )}
              </CardContent>
            </Card>

            <Card className="border-border/80 bg-card/70 backdrop-blur-sm">
              <CardHeader>
                <CardTitle>上传新头像</CardTitle>
                <CardDescription>推荐尺寸 200x200px 或更高，最大 2MB。</CardDescription>
              </CardHeader>
              <CardContent className="space-y-5">
                <label
                  className={`flex min-h-[220px] cursor-pointer flex-col items-center justify-center rounded-2xl border-2 border-dashed border-primary/25 bg-primary/5 p-6 text-center transition-colors hover:border-primary/60 hover:bg-primary/10 ${
                    uploading ? "pointer-events-none opacity-60" : ""
                  }`}
                >
                  <input
                    type="file"
                    accept="image/*"
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) {
                        void handleAvatarUpload(file);
                        e.currentTarget.value = "";
                      }
                    }}
                    disabled={uploading}
                    className="sr-only"
                  />
                  <div className="mb-4 rounded-full bg-background p-4 shadow-sm">
                    {uploading ? <Loader2 className="h-8 w-8 animate-spin text-primary" /> : <Upload className="h-8 w-8 text-primary" />}
                  </div>
                  <p className="text-base font-semibold">点击选择头像图片</p>
                  <p className="mt-1 text-sm text-muted-foreground">支持 JPG、PNG、GIF、WebP，上传成功后立即生效。</p>
                </label>

                {avatar && (
                  <div className="rounded-xl border border-border/70 bg-muted/20 p-3">
                    <p className="text-xs font-medium text-muted-foreground">当前头像地址</p>
                    <p className="mt-1 truncate font-mono text-xs">{avatar}</p>
                  </div>
                )}

                <Alert className="border-blue-500/20 bg-blue-500/10">
                  <AlertDescription>
                    头像会在全站展示。为避免裁切失真，建议使用主体居中的正方形图片。
                  </AlertDescription>
                </Alert>
              </CardContent>
            </Card>
          </motion.div>
        </TabsContent>
      </Tabs>
    </motion.div>
  );
}
