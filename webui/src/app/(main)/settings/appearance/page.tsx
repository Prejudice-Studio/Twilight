"use client";

import React, { useCallback, useState } from "react";
import Image from "next/image";
import { motion } from "framer-motion";
import {
  Palette,
  Upload,
  Loader2,
  Trash2,
  Copy,
  Check,
  X,
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
  const [copied, setCopied] = useState<string | null>(null);

  // 预览
  const [lightPreview, setLightPreview] = useState("");
  const [darkPreview, setDarkPreview] = useState("");
  const [showLightPreview, setShowLightPreview] = useState(false);
  const [showDarkPreview, setShowDarkPreview] = useState(false);

  // 分发标签页
  const [activeTab, setActiveTab] = useState<"background" | "avatar">("background");

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
  }, [user?.uid]);

  const {
    isLoading: loading,
    error,
    execute: loadAppearanceConfig,
  } = useAsyncResource(loadAppearanceResource, { immediate: true });

  const updatePreview = (css: string, img: string, type: "light" | "dark") => {
    const combined = img || css;

    if (type === "light") {
      setLightPreview(combined);
    } else {
      setDarkPreview(combined);
    }
  };

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

  return (
    <motion.div
      variants={container}
      initial="hidden"
      animate="show"
      className="space-y-6"
    >
      {/* 标签页 */}
      <motion.div variants={item} className="flex gap-2 border-b border-border">
        <button
          onClick={() => setActiveTab("background")}
          className={`pb-3 px-4 font-medium transition-colors ${
            activeTab === "background"
              ? "border-b-2 border-primary text-primary"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          <Palette className="inline mr-2 h-4 w-4" />
          背景主题
        </button>
        <button
          onClick={() => setActiveTab("avatar")}
          className={`pb-3 px-4 font-medium transition-colors ${
            activeTab === "avatar"
              ? "border-b-2 border-primary text-primary"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          <Upload className="inline mr-2 h-4 w-4" />
          用户头像
        </button>
      </motion.div>

      {/* 背景主题标签页 */}
      {activeTab === "background" && (
        <motion.div variants={item} className="space-y-6">
          {/* 浅色主题背景 */}
          <Card className="border-border bg-card/50 backdrop-blur-sm">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Palette className="h-5 w-5 text-amber-500" />
                浅色主题背景
              </CardTitle>
              <CardDescription>
                自定义浅色模式下的页面背景
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {/* 预设梯度 */}
              <div className="space-y-2">
                <Label className="text-sm">快速应用预设梯度</Label>
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
                  {gradientPresets.map((preset) => (
                    <motion.button
                      key={preset.name}
                      whileHover={{ scale: 1.05 }}
                      whileTap={{ scale: 0.95 }}
                      className="relative group"
                      onClick={() => applyPreset("light", preset.value)}
                    >
                      <div
                        className="h-16 rounded-lg border-2 border-border transition-all group-hover:border-primary"
                        style={{ background: preset.value }}
                      />
                      <span className="absolute -bottom-6 left-0 right-0 text-center text-xs text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap">
                        {preset.name}
                      </span>
                    </motion.button>
                  ))}
                </div>
              </div>

              <Separator />

              <div className="flex items-center justify-between rounded-lg border border-border/70 px-3 py-2">
                <div>
                  <Label htmlFor="light-flow">流光渐变</Label>
                  <p className="text-xs text-muted-foreground">开启后预设渐变会缓慢流动</p>
                </div>
                <Switch
                  id="light-flow"
                  checked={bgConfig.lightFlow}
                  onCheckedChange={(checked) => handleFlowToggle("light", checked)}
                />
              </div>

              {/* 自定义 CSS */}
              <div className="space-y-2">
                <Label htmlFor="light-bg-css">CSS 梯度或颜色</Label>
                <Textarea
                  id="light-bg-css"
                  placeholder="例如: linear-gradient(135deg, #667eea 0%, #764ba2 100%)"
                  value={bgConfig.lightBg}
                  onChange={(e) => handleBgChange("lightBg", e.target.value)}
                  rows={3}
                  className="font-mono text-sm"
                />
              </div>

              {/* 背景图片 */}
              <div className="space-y-2">
                <Label htmlFor="light-bg-url">背景图片 URL</Label>
                <Input
                  id="light-bg-url"
                  placeholder="https://example.com/image.jpg 或 url(https://...)"
                  value={bgConfig.lightBgImage}
                  onChange={(e) => handleBgChange("lightBgImage", e.target.value)}
                  className="font-mono text-sm"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="light-bg-upload">本地上传背景图</Label>
                <div className="flex items-center gap-2">
                  <Input
                    id="light-bg-upload"
                    type="file"
                    accept="image/*"
                    disabled={bgUploading === "light"}
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) {
                        void handleBgFileUpload(file, "light");
                        e.currentTarget.value = "";
                      }
                    }}
                  />
                  {bgUploading === "light" && <Loader2 className="h-4 w-4 animate-spin text-primary" />}
                </div>
                <p className="text-xs text-muted-foreground">支持 JPG/PNG/GIF/WebP，最大 5MB</p>
              </div>

              <div className="grid gap-3 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="light-blur">模糊程度 ({bgConfig.lightBlur}px)</Label>
                  <Input
                    id="light-blur"
                    type="range"
                    min={0}
                    max={30}
                    step={1}
                    value={bgConfig.lightBlur}
                    onChange={(e) => handleVisualChange("lightBlur", Number(e.target.value))}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="light-opacity">透明度 ({bgConfig.lightOpacity}%)</Label>
                  <Input
                    id="light-opacity"
                    type="range"
                    min={10}
                    max={100}
                    step={1}
                    value={bgConfig.lightOpacity}
                    onChange={(e) => handleVisualChange("lightOpacity", Number(e.target.value))}
                  />
                </div>
              </div>

              {/* 预览 */}
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label>预览</Label>
                  <button
                    onClick={() => setShowLightPreview(!showLightPreview)}
                    className="flex items-center gap-1 text-xs text-primary hover:underline"
                  >
                    <Eye className="h-3 w-3" />
                    {showLightPreview ? "隐藏" : "显示"}
                  </button>
                </div>
                {showLightPreview && (
                  <div
                    className="h-32 rounded-lg border-2 border-border"
                    style={{
                      background: lightPreview,
                      backgroundSize: bgConfig.lightFlow ? "220% 220%" : undefined,
                      animation: bgConfig.lightFlow ? "twilight-gradient-flow 14s ease infinite" : undefined,
                      filter: `blur(${bgConfig.lightBlur}px)`,
                      opacity: bgConfig.lightOpacity / 100,
                    }}
                  />
                )}
              </div>
            </CardContent>
          </Card>

          {/* 暗色主题背景 */}
          <Card className="border-border bg-card/50 backdrop-blur-sm">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Palette className="h-5 w-5 text-slate-400" />
                暗色主题背景
              </CardTitle>
              <CardDescription>
                自定义暗色模式下的页面背景
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {/* 预设梯度 */}
              <div className="space-y-2">
                <Label className="text-sm">快速应用预设梯度</Label>
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
                  {gradientPresets.map((preset) => (
                    <motion.button
                      key={preset.name}
                      whileHover={{ scale: 1.05 }}
                      whileTap={{ scale: 0.95 }}
                      className="relative group"
                      onClick={() => applyPreset("dark", preset.value)}
                    >
                      <div
                        className="h-16 rounded-lg border-2 border-border transition-all group-hover:border-primary"
                        style={{ background: preset.value }}
                      />
                      <span className="absolute -bottom-6 left-0 right-0 text-center text-xs text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap">
                        {preset.name}
                      </span>
                    </motion.button>
                  ))}
                </div>
              </div>

              <Separator />

              <div className="flex items-center justify-between rounded-lg border border-border/70 px-3 py-2">
                <div>
                  <Label htmlFor="dark-flow">流光渐变</Label>
                  <p className="text-xs text-muted-foreground">开启后预设渐变会缓慢流动</p>
                </div>
                <Switch
                  id="dark-flow"
                  checked={bgConfig.darkFlow}
                  onCheckedChange={(checked) => handleFlowToggle("dark", checked)}
                />
              </div>

              {/* 自定义 CSS */}
              <div className="space-y-2">
                <Label htmlFor="dark-bg-css">CSS 梯度或颜色</Label>
                <Textarea
                  id="dark-bg-css"
                  placeholder="例如: linear-gradient(135deg, #1e1e1e 0%, #2d2d2d 100%)"
                  value={bgConfig.darkBg}
                  onChange={(e) => handleBgChange("darkBg", e.target.value)}
                  rows={3}
                  className="font-mono text-sm"
                />
              </div>

              {/* 背景图片 */}
              <div className="space-y-2">
                <Label htmlFor="dark-bg-url">背景图片 URL</Label>
                <Input
                  id="dark-bg-url"
                  placeholder="https://example.com/image.jpg 或 url(https://...)"
                  value={bgConfig.darkBgImage}
                  onChange={(e) => handleBgChange("darkBgImage", e.target.value)}
                  className="font-mono text-sm"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="dark-bg-upload">本地上传背景图</Label>
                <div className="flex items-center gap-2">
                  <Input
                    id="dark-bg-upload"
                    type="file"
                    accept="image/*"
                    disabled={bgUploading === "dark"}
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) {
                        void handleBgFileUpload(file, "dark");
                        e.currentTarget.value = "";
                      }
                    }}
                  />
                  {bgUploading === "dark" && <Loader2 className="h-4 w-4 animate-spin text-primary" />}
                </div>
                <p className="text-xs text-muted-foreground">支持 JPG/PNG/GIF/WebP，最大 5MB</p>
              </div>

              <div className="grid gap-3 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="dark-blur">模糊程度 ({bgConfig.darkBlur}px)</Label>
                  <Input
                    id="dark-blur"
                    type="range"
                    min={0}
                    max={30}
                    step={1}
                    value={bgConfig.darkBlur}
                    onChange={(e) => handleVisualChange("darkBlur", Number(e.target.value))}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="dark-opacity">透明度 ({bgConfig.darkOpacity}%)</Label>
                  <Input
                    id="dark-opacity"
                    type="range"
                    min={10}
                    max={100}
                    step={1}
                    value={bgConfig.darkOpacity}
                    onChange={(e) => handleVisualChange("darkOpacity", Number(e.target.value))}
                  />
                </div>
              </div>

              {/* 预览 */}
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label>预览</Label>
                  <button
                    onClick={() => setShowDarkPreview(!showDarkPreview)}
                    className="flex items-center gap-1 text-xs text-primary hover:underline"
                  >
                    <Eye className="h-3 w-3" />
                    {showDarkPreview ? "隐藏" : "显示"}
                  </button>
                </div>
                {showDarkPreview && (
                  <div
                    className="h-32 rounded-lg border-2 border-border"
                    style={{
                      background: darkPreview,
                      backgroundSize: bgConfig.darkFlow ? "220% 220%" : undefined,
                      animation: bgConfig.darkFlow ? "twilight-gradient-flow 14s ease infinite" : undefined,
                      filter: `blur(${bgConfig.darkBlur}px)`,
                      opacity: bgConfig.darkOpacity / 100,
                    }}
                  />
                )}
              </div>
            </CardContent>
          </Card>

          {/* 操作按钮 */}
          <div className="flex gap-3">
            <Button
              onClick={handleSaveBg}
              disabled={saving}
              className="flex-1 sm:flex-none"
            >
              {saving ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Download className="mr-2 h-4 w-4" />
              )}
              保存背景
            </Button>
            <Button
              onClick={handleResetBg}
              variant="outline"
              disabled={saving}
            >
              <Trash2 className="mr-2 h-4 w-4" />
              重置为默认
            </Button>
          </div>
        </motion.div>
      )}

      {/* 用户头像标签页 */}
      {activeTab === "avatar" && (
        <motion.div variants={item}>
          <Card className="border-border bg-card/50 backdrop-blur-sm">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Upload className="h-5 w-5 text-blue-500" />
                用户头像
              </CardTitle>
              <CardDescription>
                上传一张个人头像图片，推荐尺寸 200x200px，最大 2MB
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* 头像预览 */}
              <div className="flex flex-col items-center gap-4">
                <div className="relative">
                  <div
                    className="h-32 w-32 rounded-full border-4 border-primary/20 bg-muted flex items-center justify-center overflow-hidden"
                  >
                    {avatar ? (
                      <Image
                        src={avatar}
                        alt="用户头像"
                        width={128}
                        height={128}
                        unoptimized
                        className="w-full h-full object-cover"
                      />
                    ) : (
                      <div className="text-center">
                        <Upload className="h-8 w-8 mx-auto text-muted-foreground" />
                        <p className="text-xs text-muted-foreground mt-2">
                          未设置
                        </p>
                      </div>
                    )}
                  </div>
                </div>

                {/* 上传区域 */}
                <label className="w-full">
                  <div className="border-2 border-dashed border-primary/30 rounded-lg p-6 text-center cursor-pointer hover:border-primary/60 transition-colors">
                    <input
                      type="file"
                      accept="image/*"
                      onChange={(e) => {
                        const file = e.target.files?.[0];
                        if (file) handleAvatarUpload(file);
                      }}
                      disabled={uploading}
                      className="hidden"
                    />
                    <Upload className="h-8 w-8 mx-auto mb-2 text-muted-foreground" />
                    <p className="text-sm font-medium">点击选择或拖放上传</p>
                    <p className="text-xs text-muted-foreground mt-1">
                      支持 JPG, PNG, GIF, WebP（最大 2MB）
                    </p>
                  </div>
                </label>

                {/* 删除按钮 */}
                {avatar && (
                  <Button
                    onClick={handleDeleteAvatar}
                    variant="destructive"
                    disabled={saving || uploading}
                  >
                    {saving ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <Trash2 className="mr-2 h-4 w-4" />
                    )}
                    删除头像
                  </Button>
                )}
              </div>

              {/* 信息提示 */}
              <Alert className="bg-blue-500/10 border-blue-500/20">
                <AlertDescription>
                  💡 头像将在全站展示，建议选择高质量的个人头像
                </AlertDescription>
              </Alert>
            </CardContent>
          </Card>
        </motion.div>
      )}
    </motion.div>
  );
}
