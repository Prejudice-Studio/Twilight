"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useAuthStore } from "@/store/auth";
import { Sidebar } from "@/components/layout/sidebar";
import { Header } from "@/components/layout/header";
import { EmbyPendingModal } from "@/components/emby-pending-modal";
import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { useTheme } from "next-themes";
import { api } from "@/lib/api";
import { RegionRefreshKeys } from "@/lib/region-refresh";
import { useRegionRefresh } from "@/hooks/use-region-refresh";

interface BackgroundConfig {
  lightBg?: string;
  darkBg?: string;
  lightBgImage?: string;
  darkBgImage?: string;
  lightFlow?: boolean;
  darkFlow?: boolean;
  lightBlur?: number;
  darkBlur?: number;
  lightOpacity?: number;
  darkOpacity?: number;
}

const SAFE_BG_CSS_FUNCTION = /^(linear-gradient|radial-gradient|conic-gradient|repeating-linear-gradient|repeating-radial-gradient)\s*\(/i;
const SAFE_BG_DATA_IMAGE = /^data:image\/(png|jpe?g|gif|webp|avif|bmp)(;|,)/i;

function escapeCssUrlValue(value: string): string {
  return value.replace(/"/g, '\\"');
}

function sanitizeBgUrl(raw: string): string {
  const value = raw.trim();
  if (!value || /[\u0000-\u001F\u007F]/.test(value) || value.startsWith("//")) {
    return "";
  }
  if (/^https?:\/\//i.test(value) || value.startsWith("/") || value.startsWith("./") || value.startsWith("../")) {
    return escapeCssUrlValue(value);
  }
  if (value.startsWith("blob:") || SAFE_BG_DATA_IMAGE.test(value)) {
    return escapeCssUrlValue(value);
  }
  if (/^[a-z][a-z0-9+.-]*:/i.test(value)) {
    return "";
  }
  return escapeCssUrlValue(value);
}

// 把可能是裸 URL/路径的图片值规范化为合法且受限的 CSS background-image 值
function normalizeBgImageValue(raw: string): string {
  const value = (raw || "").trim();
  if (!value) return "";
  // 渐变类 CSS 函数不涉及外部资源，可直接使用。
  if (SAFE_BG_CSS_FUNCTION.test(value)) {
    return value;
  }
  const urlMatch = value.match(/^url\(\s*(['"]?)(.*?)\1\s*\)$/i);
  const safeUrl = sanitizeBgUrl(urlMatch ? urlMatch[2] : value);
  return safeUrl ? `url("${safeUrl}")` : "";
}

function buildBgStyleFromConfig(
  bgConfig: BackgroundConfig,
  isDark: boolean,
): Record<string, string> {
  const css = normalizeBgImageValue((isDark ? bgConfig.darkBg : bgConfig.lightBg) || "");
  const imgRaw = (isDark ? bgConfig.darkBgImage : bgConfig.lightBgImage) || "";
  const flow = Boolean(isDark ? bgConfig.darkFlow : bgConfig.lightFlow);
  const blur = Number((isDark ? bgConfig.darkBlur : bgConfig.lightBlur) ?? 0);
  const opacity = Number((isDark ? bgConfig.darkOpacity : bgConfig.lightOpacity) ?? 100);

  const img = normalizeBgImageValue(imgRaw);
  const effective = img || css;

  if (!effective) return {};

  const safeBlur = Number.isFinite(blur) ? Math.min(30, Math.max(0, blur)) : 0;
  const safeOpacity = Number.isFinite(opacity) ? Math.min(100, Math.max(10, opacity)) : 100;

  const style: Record<string, string> = {
    backgroundImage: effective,
    backgroundAttachment: "fixed",
    backgroundSize: "cover",
    backgroundPosition: "center",
    filter: `blur(${safeBlur}px)`,
    opacity: `${safeOpacity / 100}`,
    transform: safeBlur > 0 ? "scale(1.04)" : "scale(1)",
    transformOrigin: "center",
  };

  if (!img && flow && css.includes("gradient")) {
    style.backgroundSize = "220% 220%";
    style.animation = "twilight-gradient-flow 14s ease infinite";
  }

  return style;
}

export default function MainLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const { user, isAuthenticated, isLoading, initialize, fetchUser } = useAuthStore();
  const { resolvedTheme, theme } = useTheme();
  const activeTheme = resolvedTheme || theme;
  const isAdmin = user?.role === 0;
  const [bgConfig, setBgConfig] = useState<BackgroundConfig | null>(null);
  const [bgStyle, setBgStyle] = useState<Record<string, string>>({});
  const [nextBgStyle, setNextBgStyle] = useState<Record<string, string> | null>(null);
  const [bgRevealActive, setBgRevealActive] = useState(false);
  const bgTransitionTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const initialBgResolvedRef = useRef(false);
  const bgStyleRef = useRef<Record<string, string>>({});

  const clearBgTransitionTimer = () => {
    if (bgTransitionTimerRef.current) {
      clearTimeout(bgTransitionTimerRef.current);
      bgTransitionTimerRef.current = null;
    }
  };

  const applyBackgroundStyle = useCallback((style: Record<string, string>) => {
    if (!initialBgResolvedRef.current) {
      initialBgResolvedRef.current = true;
      bgStyleRef.current = style;
      setBgStyle(style);
      setNextBgStyle(null);
      setBgRevealActive(false);
      return;
    }

    const currentSerialized = JSON.stringify(bgStyleRef.current);
    const nextSerialized = JSON.stringify(style);
    if (currentSerialized === nextSerialized) {
      return;
    }

    clearBgTransitionTimer();
    setNextBgStyle(style);
    setBgRevealActive(false);

    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        setBgRevealActive(true);
      });
    });

    bgTransitionTimerRef.current = setTimeout(() => {
      bgStyleRef.current = style;
      setBgStyle(style);
      setNextBgStyle(null);
      setBgRevealActive(false);
    }, 520);
  }, []);

  // 仅在登录态变化时重新拉取背景配置；主题切换不再触发网络请求
  const loadUserBg = useCallback(async () => {
    if (!isAuthenticated || !user?.uid) {
      setBgConfig(null);
      applyBackgroundStyle({});
      return;
    }

    try {
      const res = await api.getUserBackground(user.uid);
      if (!res.success || !res.data?.background) {
        setBgConfig(null);
        applyBackgroundStyle({});
        return;
      }
      setBgConfig(JSON.parse(res.data.background) as BackgroundConfig);
    } catch {
      setBgConfig(null);
      applyBackgroundStyle({});
    }
  }, [applyBackgroundStyle, isAuthenticated, user?.uid]);

  // 主题或 bgConfig 变化时，仅纯前端重算样式（不再网络请求）
  const computedBgStyle = useMemo(() => {
    if (!bgConfig) return {};
    return buildBgStyleFromConfig(bgConfig, activeTheme === "dark");
  }, [bgConfig, activeTheme]);

  useEffect(() => {
    applyBackgroundStyle(computedBgStyle);
  }, [computedBgStyle, applyBackgroundStyle]);

  useEffect(() => {
    void initialize();
  }, [initialize]);

  useEffect(() => {
    return () => {
      clearBgTransitionTimer();
    };
  }, []);

  useEffect(() => {
    void loadUserBg();
  }, [loadUserBg]);

  useRegionRefresh(RegionRefreshKeys.UserProfile, useCallback(() => {
    void fetchUser({ silent: true });
  }, [fetchUser]));

  useRegionRefresh(RegionRefreshKeys.UserBackground, useCallback(() => {
    void loadUserBg();
  }, [loadUserBg]));

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      router.push("/login");
    }
  }, [isAuthenticated, isLoading, router]);

  useEffect(() => {
    if (!isLoading && isAuthenticated && pathname.startsWith('/admin') && !isAdmin) {
      router.push('/dashboard');
    }
  }, [isAuthenticated, isLoading, isAdmin, pathname, router]);

  // 加载中 / 未登录都保持 loader，等到 router.push 真的导航走再卸载，
  // 避免出现"白屏一帧 → 跳转"的肉眼可见闪烁。
  if (isLoading || !isAuthenticated) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  return (
    <div className={cn("app-shell min-h-screen", !isAdmin && "hide-dev-tools")}>
      <div className="fixed inset-0 -z-10 pointer-events-none twilight-bg-layer" style={bgStyle} />
      {nextBgStyle && (
        <div
          className={cn(
            "fixed inset-0 -z-10 pointer-events-none twilight-bg-layer twilight-bg-wipe",
            bgRevealActive && "twilight-bg-wipe-active"
          )}
          style={nextBgStyle}
        />
      )}
      <div className="shell-glow shell-glow-left" />
      <div className="shell-glow shell-glow-right" />
      <div className="relative z-10 flex min-h-screen">
        <Sidebar />
        <div className="flex min-h-screen min-w-0 flex-1 flex-col lg:pl-72">
          <Header />
          <main className="mx-auto w-full min-w-0 max-w-[1680px] flex-1 px-2 py-3 sm:p-4 md:p-6 xl:p-8">
            <div className="section-surface">{children}</div>
          </main>
        </div>
      </div>
      <EmbyPendingModal />
    </div>
  );
}

