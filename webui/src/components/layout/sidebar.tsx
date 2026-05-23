"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { flushSync } from "react-dom";
import type { ComponentType } from "react";
import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { motion } from "framer-motion";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/store/auth";
import {
  LayoutDashboard,
  Film,
  Settings,
  Users,
  FileText,
  MessageSquare,
  LogOut,
  Moon,
  Sun,
  TestTube,
  FileCode,
  Database,
  Server,
  Megaphone,
  TimerReset,
  GitBranch,
  Network,
  Coins,
  Github,
  ScrollText,
} from "lucide-react";
import { useTheme } from "next-themes";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { api } from "@/lib/api";
import { SITE_NAME } from "@/lib/site-config";
import { useRegionRefresh } from "@/hooks/use-region-refresh";
import { RegionRefreshKeys } from "@/lib/region-refresh";
import { useSystemStore } from "@/store/system";

export interface SidebarNavItem {
  href: string;
  label: string;
  icon: ComponentType<{ className?: string }>;
}

type ViewTransitionLike = {
  ready: Promise<void>;
};

type MaybeStartViewTransition = {
  startViewTransition?: (updateCallback: () => void | Promise<void>) => ViewTransitionLike;
};

const SAFE_IMAGE_URL = /^(https?:\/\/|\/|data:image\/(png|jpe?g|gif|webp|avif|bmp)(;|,)|[a-zA-Z]:[\\/])/i;

function sanitizeImageUrl(url?: string | null): string | undefined {
  if (!url) return undefined;
  const value = url.trim();
  if (!value) return undefined;
  if (!SAFE_IMAGE_URL.test(value)) return undefined;
  if (/^[a-zA-Z]:[\\/]/.test(value)) return `/api/v1/system/server-icon`;
  return value;
}

export const userNavItems: SidebarNavItem[] = [
  { href: "/dashboard", label: "仪表盘", icon: LayoutDashboard },
  { href: "/announcements", label: "公告", icon: Megaphone },
  { href: "/media", label: "求片中心", icon: Film },
  { href: "/score", label: "签到", icon: Coins },
  { href: "/invite", label: "邀请中心", icon: GitBranch },
  { href: "/settings", label: "个人设置", icon: Settings },
];

export const adminNavItems: SidebarNavItem[] = [
  { href: "/admin/users", label: "用户管理", icon: Users },
  { href: "/admin/announcements", label: "公告管理", icon: Megaphone },
  { href: "/admin/regcodes", label: "注册码", icon: FileText },
  { href: "/admin/invite", label: "邀请森林", icon: Network },
  { href: "/admin/requests", label: "求片审核", icon: Film },
  { href: "/admin/telegram-rebind-requests", label: "Telegram 换绑", icon: MessageSquare },
  { href: "/admin/emby", label: "Emby 管理", icon: Server },
  { href: "/admin/scheduler", label: "定时任务", icon: TimerReset },
  { href: "/admin/database", label: "数据库迁移", icon: Database },
  { href: "/admin/config", label: "配置管理", icon: FileCode },
  { href: "/admin/logs", label: "实时日志", icon: ScrollText },
  { href: "/admin/test", label: "服务器信息", icon: TestTube },
];

export function Sidebar() {
  const pathname = usePathname();
  const { user, logout } = useAuthStore();
  const { setTheme, theme: rawTheme, resolvedTheme } = useTheme();
  // resolvedTheme 反映真实生效主题（含 SSR -> CSR 后的 hydration），用于图标显示
  const currentTheme = resolvedTheme || rawTheme || "light";
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

  const displaySiteName = systemInfo?.name || SITE_NAME;
  const safeSystemIcon = useMemo(() => sanitizeImageUrl(systemInfo?.icon), [systemInfo?.icon]);
  const safeProfileAvatar = useMemo(() => sanitizeImageUrl(profileAvatar), [profileAvatar]);

  const toggleTheme = (event: React.MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    // 把 light <-> dark 翻转。currentTheme 在 enableSystem={false} 下只会是 "light" / "dark"。
    const nextTheme = currentTheme === "dark" ? "light" : "dark";

    const startViewTransition = (document as unknown as MaybeStartViewTransition).startViewTransition;

    // 没有 View Transition API（Firefox / Safari / 旧 Chrome）：直接切换
    if (!startViewTransition) {
      setTheme(nextTheme);
      return;
    }

    // 关键：必须在 startViewTransition 回调里用 ``flushSync`` 同步提交 React 状态，
    // 否则 React 18 的批处理会让 DOM 更新晚于浏览器拍快照，结果「主题没变」或者
    // 「拍了两张相同快照、没有动画且看似失效」。
    let didCommit = false;
    try {
      const x = event.clientX;
      const y = event.clientY;
      const transition = startViewTransition(() => {
        flushSync(() => {
          setTheme(nextTheme);
        });
        didCommit = true;
      });

      void transition.ready
        .then(() => {
          const radius = Math.hypot(
            Math.max(x, window.innerWidth - x),
            Math.max(y, window.innerHeight - y),
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
            } as KeyframeAnimationOptions,
          );
        })
        .catch(() => undefined);
    } catch {
      // View Transition 内部抛错时（例如旧版 Chrome 的边界情况）回落到直接切换。
      if (!didCommit) {
        setTheme(nextTheme);
      }
    }
  };

  return (
    <aside className="fixed inset-y-0 left-0 z-40 hidden w-72 p-4 lg:block">
      <div className="sidebar-surface h-full">
        <div className="sidebar-brand">
          {safeSystemIcon ? (
            <Image
              src={safeSystemIcon}
              alt={displaySiteName}
              width={40}
              height={40}
              className="h-10 w-10 rounded-xl object-cover"
              unoptimized
              referrerPolicy="no-referrer"
            />
          ) : (
            <div className="brand-logo">{displaySiteName.slice(0, 2).toUpperCase()}</div>
          )}
          <div>
            <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">Media OPS</p>
            <h2 className="text-lg font-semibold">{displaySiteName}</h2>
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
          <a
            href="https://github.com/Prejudice-Studio/Twilight"
            target="_blank"
            rel="noreferrer"
            className="flex items-center justify-center gap-2 rounded-full border border-border/70 bg-background/60 px-3 py-2 text-xs font-medium text-muted-foreground transition-all hover:bg-primary/10 hover:text-primary"
          >
            <Github className="h-4 w-4" />
            GitHub Project
          </a>

          <div className="profile-card">
            <Avatar className="h-10 w-10 border border-border/60">
              {safeProfileAvatar && <AvatarImage src={safeProfileAvatar} alt={user?.username} />}
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
              type="button"
              variant="outline"
              className="h-10 justify-start gap-2 overflow-hidden rounded-full border-border/70 bg-background/60 px-3 transition-all hover:bg-primary/10 hover:text-primary"
              onClick={toggleTheme}
              title={`当前主题：${currentTheme === "dark" ? "暗色" : "浅色"} · 点击切换`}
              aria-label="切换暗色 / 浅色主题"
            >
              {currentTheme === "dark" ? <Moon className="h-4 w-4" /> : <Sun className="h-4 w-4" />}
              <span className="text-xs font-medium">{currentTheme === "dark" ? "暗色" : "浅色"}</span>
            </Button>
            <Button type="button" variant="outline" className="h-10" onClick={() => void logout()}>
              <LogOut className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>
    </aside>
  );
}
