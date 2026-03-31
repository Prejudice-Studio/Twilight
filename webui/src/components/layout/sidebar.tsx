"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { motion } from "framer-motion";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/store/auth";
import {
  LayoutDashboard,
  Film,
  Coins,
  Settings,
  Users,
  FileText,
  LogOut,
  Moon,
  Sun,
  TestTube,
  FileCode,
  Library,
} from "lucide-react";
import { useTheme } from "next-themes";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { api } from "@/lib/api";
import { useRegionRefresh } from "@/hooks/use-region-refresh";
import { RegionRefreshKeys } from "@/lib/region-refresh";
import { useSystemStore } from "@/store/system";

const userNavItems = [
  { href: "/dashboard", label: "仪表盘", icon: LayoutDashboard },
  { href: "/media", label: "媒体搜索", icon: Film },
  { href: "/score", label: "积分中心", icon: Coins },
  { href: "/settings", label: "个人设置", icon: Settings },
];

const adminNavItems = [
  { href: "/admin/users", label: "用户管理", icon: Users },
  { href: "/admin/regcodes", label: "注册码", icon: FileText },
  { href: "/admin/requests", label: "求片审核", icon: Film },
  { href: "/admin/config", label: "配置管理", icon: FileCode },
  { href: "/admin/nsfw", label: "NSFW 库管理", icon: Library },
  { href: "/admin/test", label: "API 测试", icon: TestTube },
];

export function Sidebar() {
  const pathname = usePathname();
  const { user, logout } = useAuthStore();
  const { setTheme, theme: currentTheme } = useTheme();
  const isAdmin = user?.role === 0;
  const [profileAvatar, setProfileAvatar] = useState<string | null>(user?.avatar || null);
  const { info: systemInfo, fetchInfo: fetchSystemInfo } = useSystemStore();

  useEffect(() => {
    void fetchSystemInfo();
  }, [fetchSystemInfo]);

  const loadProfileAvatar = useCallback(async () => {
    if (!user?.uid) {
      setProfileAvatar(null);
      return;
    }

    try {
      const res = await api.getUserAvatar(user.uid);
      if (res.success) {
        setProfileAvatar(res.data?.avatar || user.avatar || null);
      } else {
        setProfileAvatar(user.avatar || null);
      }
    } catch {
      setProfileAvatar(user.avatar || null);
    }
  }, [user?.uid, user?.avatar]);

  useEffect(() => {
    setProfileAvatar(user?.avatar || null);
    void loadProfileAvatar();
  }, [user?.avatar, loadProfileAvatar]);

  useRegionRefresh(
    RegionRefreshKeys.UserProfile,
    useCallback(() => {
      void loadProfileAvatar();
    }, [loadProfileAvatar])
  );

  const toggleTheme = (event: React.MouseEvent) => {
    const x = event.clientX;
    const y = event.clientY;

    const themeOrder = ["light", "dark"];
    const currentIndex = themeOrder.indexOf(currentTheme || "light");
    const nextTheme = themeOrder[(currentIndex + 1) % themeOrder.length];

    if (!(document as any).startViewTransition) {
      setTheme(nextTheme);
      return;
    }

    const transition = (document as any).startViewTransition(async () => {
      setTheme(nextTheme);
    });

    transition.ready.then(() => {
      const radius = Math.hypot(
        Math.max(x, window.innerWidth - x),
        Math.max(y, window.innerHeight - y)
      );

      document.documentElement.animate(
        {
          clipPath: [
            `circle(0px at ${x}px ${y}px)`,
            `circle(${radius}px at ${x}px ${y}px)`,
          ],
        },
        {
          duration: 500,
          easing: "ease-in-out",
          pseudoElement: "::view-transition-new(root)",
        }
      );
    });
  };

  return (
    <aside className="fixed inset-y-0 left-0 z-40 hidden w-72 p-4 lg:block">
      <div className="sidebar-surface h-full">
        <div className="sidebar-brand">
          {systemInfo?.icon ? (
            <img src={systemInfo.icon} alt={systemInfo?.name || "Twilight"} className="h-10 w-10 rounded-xl object-cover" />
          ) : (
            <div className="brand-logo">{(systemInfo?.name || "TW").slice(0, 2).toUpperCase()}</div>
          )}
          <div>
            <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">Media OPS</p>
            <h2 className="text-lg font-semibold">{systemInfo?.name || "Twilight"}</h2>
          </div>
        </div>

        <nav className="sidebar-nav">
          <p className="sidebar-label">用户菜单</p>
          {userNavItems.map((item) => {
            const active = pathname === item.href;
            return (
              <Link
                key={item.href}
                href={item.href}
                className={cn("sidebar-link", active && "sidebar-link-active")}
              >
                <item.icon className="h-4 w-4" />
                <span>{item.label}</span>
                {active && <motion.div layoutId="sidebar-active" className="sidebar-dot" />}
              </Link>
            );
          })}

          {isAdmin && (
            <>
              <p className="sidebar-label mt-5">管理菜单</p>
              {adminNavItems.map((item) => {
                const active = pathname.startsWith(item.href);
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    className={cn("sidebar-link", active && "sidebar-link-active")}
                  >
                    <item.icon className="h-4 w-4" />
                    <span>{item.label}</span>
                    {active && <motion.div layoutId="sidebar-active-admin" className="sidebar-dot" />}
                  </Link>
                );
              })}
            </>
          )}
        </nav>

        <div className="sidebar-footer">
          <div className="profile-card">
            <Avatar className="h-10 w-10 border border-border/60">
              {profileAvatar && <AvatarImage src={profileAvatar} alt={user?.username} />}
              <AvatarFallback className="bg-primary/15 text-primary text-xs font-semibold">
                {user?.username?.slice(0, 2).toUpperCase() || "U"}
              </AvatarFallback>
            </Avatar>
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">{user?.username}</p>
              <p className="truncate text-xs text-muted-foreground">{user?.role_name}</p>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-2">
            <Button
              variant="outline"
              className="h-10"
              onClick={toggleTheme}
              title={`当前主题: ${currentTheme || "light"}`}
            >
              {currentTheme === "dark" ? <Moon className="h-4 w-4" /> : <Sun className="h-4 w-4" />}
            </Button>
            <Button variant="outline" className="h-10" onClick={logout}>
              <LogOut className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>
    </aside>
  );
}

