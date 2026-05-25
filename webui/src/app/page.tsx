"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useAuthStore } from "@/store/auth";
import { useSystemStore } from "@/store/system";
import { SITE_NAME } from "@/lib/site-config";
import { Sparkles } from "lucide-react";

export default function Home() {
  const router = useRouter();
  const { isAuthenticated, isLoading, isHydrated, initialize } = useAuthStore();
  const { info: systemInfo, fetchInfo: fetchSystemInfo } = useSystemStore();

  useEffect(() => {
    // 必须等 persist 把 isAuthenticated 从
    // localStorage 还原回来再 initialize，否则首次渲染拿到的是默认值 false，
    // 已登录用户会被瞬间踢回 /login。
    if (!isHydrated) return;
    void initialize();
    void fetchSystemInfo();
  }, [isHydrated, initialize, fetchSystemInfo]);

  useEffect(() => {
    if (!isLoading) {
      if (isAuthenticated) {
        router.replace("/dashboard");
      } else {
        router.replace("/login");
      }
    }
  }, [isAuthenticated, isLoading, router]);

  return (
    <main className="flex min-h-screen items-center justify-center bg-aurora-gradient">
      <div className="flex flex-col items-center gap-4">
        <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-white/10 backdrop-blur-xl animate-pulse">
          <Sparkles className="h-8 w-8 text-white" />
        </div>
        <h1 className="text-2xl font-bold text-white">{systemInfo?.name || SITE_NAME}</h1>
        <p className="text-white/60">正在加载...</p>
      </div>
    </main>
  );
}

