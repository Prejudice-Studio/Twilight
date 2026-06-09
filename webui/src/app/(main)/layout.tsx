"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useAuthStore } from "@/store/auth";
import { Sidebar } from "@/components/layout/sidebar";
import { Header } from "@/components/layout/header";
import { EmbyPendingModal } from "@/components/emby-pending-modal";
import { ErrorBoundary } from "@/components/error-boundary";
import RebindGuard from "@/components/rebind-guard";
import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { useTheme } from "next-themes";
import { api } from "@/lib/api";
import { RegionRefreshKeys } from "@/lib/region-refresh";
import { useRegionRefresh } from "@/hooks/use-region-refresh";
import { normalizeBackgroundImageValue } from "@/lib/safe-url";
import { useI18n } from "@/lib/i18n";

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

function buildBgStyleFromConfig(
  bgConfig: BackgroundConfig,
  isDark: boolean,
): Record<string, string> {
  const css = normalizeBackgroundImageValue((isDark ? bgConfig.darkBg : bgConfig.lightBg) || "");
  const imgRaw = (isDark ? bgConfig.darkBgImage : bgConfig.lightBgImage) || "";
  const flow = Boolean(isDark ? bgConfig.darkFlow : bgConfig.lightFlow);
  const blur = Number((isDark ? bgConfig.darkBlur : bgConfig.lightBlur) ?? 0);
  const opacity = Number((isDark ? bgConfig.darkOpacity : bgConfig.lightOpacity) ?? 100);

  const img = normalizeBackgroundImageValue(imgRaw);
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
  const { t } = useI18n();
  const { user, isAuthenticated, isLoading, isHydrated, initialize, fetchUser } = useAuthStore();
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
    // persist 还原是异步的，
    // 在 isHydrated 翻为 true 之前 store 里的 isAuthenticated 还是默认 false，
    // 直接 initialize() 会被错判为未登录、跳掉 fetchUser。
    if (!isHydrated) return;
    void initialize();
  }, [isHydrated, initialize]);

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
  // 同时等待 persist 还原（isHydrated）避免登录态闪烁。
  //
  // pathname.startsWith('/admin') && !isAdmin 也走 loader：上方 useEffect
  // 用 router.push('/dashboard') 把普通用户从 /admin/* 踢回 dashboard，但
  // router.push 是异步的——在它真正完成前，老逻辑会先把 admin 侧边栏 / 头部
  // chrome 渲染出来再被替换。这是 BATCH_11 R54 已经处理过、又被新一轮重构
  // 写回去的"管理面板闪一帧"回归。把 admin 路径下非管理员也并入 loader 分
  // 支，admin chrome 永远不会出现在非 admin 用户的视口里。
  const isPendingAdminRedirect = pathname.startsWith("/admin") && !isAdmin;
  if (!isHydrated || isLoading || !isAuthenticated || isPendingAdminRedirect) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  if (user?.rebinding_in_progress) {
    return <RebindGuard />;
  }

  return (
    <div className={cn("app-shell min-h-screen", !isAdmin && "hide-dev-tools")}>
      {/* Skip-to-content：键盘第一个 Tab 焦点直达主内容，
          满足 WCAG 2.4.1 Bypass Blocks。
          默认 sr-only 视觉隐藏，focus-visible 时显示。 */}
      <a
        href="#main-content"
        className="sr-only focus-visible:not-sr-only focus-visible:fixed focus-visible:left-4 focus-visible:top-4 focus-visible:z-[1000] focus-visible:rounded-md focus-visible:bg-background focus-visible:px-4 focus-visible:py-2 focus-visible:text-sm focus-visible:font-medium focus-visible:text-foreground focus-visible:shadow-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
      >
        {t("accessibility.skipToContent")}
      </a>
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
          <main id="main-content" tabIndex={-1} className="mx-auto w-full min-w-0 max-w-[1680px] flex-1 px-2 py-3 sm:p-4 md:p-6 xl:p-8">
            <div className="section-surface">
              <ErrorBoundary>{children}</ErrorBoundary>
            </div>
          </main>
        </div>
      </div>
      <EmbyPendingModal />
    </div>
  );
}
