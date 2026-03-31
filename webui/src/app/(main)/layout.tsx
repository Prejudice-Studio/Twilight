"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useAuthStore } from "@/store/auth";
import { Sidebar } from "@/components/layout/sidebar";
import { Header } from "@/components/layout/header";
import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { useTheme } from "next-themes";
import { api } from "@/lib/api";
import { RegionRefreshKeys } from "@/lib/region-refresh";
import { useRegionRefresh } from "@/hooks/use-region-refresh";

export default function MainLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const { user, isAuthenticated, isLoading, initialize, fetchUser } = useAuthStore();
  const { theme } = useTheme();
  const isAdmin = user?.role === 0;
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

  const loadUserBg = useCallback(async () => {
    if (!isAuthenticated || !user?.uid) {
      applyBackgroundStyle({});
      return;
    }

    try {
      const res = await api.getUserBackground(user.uid);
      if (!res.success || !res.data?.background) {
        setBgStyle({});
        return;
      }

      const bgConfig = JSON.parse(res.data.background);
      const isDark = theme === "dark";
      const bgKey = isDark ? "darkBg" : "lightBg";
      const imgKey = isDark ? "darkBgImage" : "lightBgImage";
      const flowKey = isDark ? "darkFlow" : "lightFlow";
      const blurKey = isDark ? "darkBlur" : "lightBlur";
      const opacityKey = isDark ? "darkOpacity" : "lightOpacity";
      const css = bgConfig[bgKey] || "";
      const img = bgConfig[imgKey] || "";
      const flow = Boolean(bgConfig[flowKey]);
      const blur = Number(bgConfig[blurKey] ?? 0);
      const opacity = Number(bgConfig[opacityKey] ?? 100);

      const effectiveBackground = img || css;

      if (effectiveBackground) {
        const safeBlur = Number.isFinite(blur) ? Math.min(30, Math.max(0, blur)) : 0;
        const safeOpacity = Number.isFinite(opacity) ? Math.min(100, Math.max(10, opacity)) : 100;

        const nextStyle: Record<string, string> = {
          backgroundImage: effectiveBackground,
          backgroundAttachment: "fixed",
          backgroundSize: "cover",
          backgroundPosition: "center",
          filter: `blur(${safeBlur}px)`,
          opacity: `${safeOpacity / 100}`,
          transform: safeBlur > 0 ? "scale(1.04)" : "scale(1)",
          transformOrigin: "center",
        };

        if (!img && flow && css.includes("gradient")) {
          nextStyle.backgroundSize = "220% 220%";
          nextStyle.animation = "twilight-gradient-flow 14s ease infinite";
        }

        applyBackgroundStyle(nextStyle);
      } else {
        applyBackgroundStyle({});
      }
    } catch {
      applyBackgroundStyle({});
    }
  }, [applyBackgroundStyle, isAuthenticated, user?.uid, theme]);

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

  if (isLoading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  if (!isAuthenticated) {
    return null;
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
          <main className="mx-auto w-full max-w-[1680px] flex-1 p-4 md:p-6 xl:p-8">
            <div className="section-surface">{children}</div>
          </main>
        </div>
      </div>
    </div>
  );
}

