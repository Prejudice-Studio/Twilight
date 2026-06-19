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
  ScrollText,
  ShieldAlert,
  MonitorSmartphone,
  Mail,
  ClipboardList,
  BookOpen,
  MessageSquareMore,
} from "lucide-react";
import { useTheme } from "next-themes";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { GithubProjectLink } from "@/components/github-project-link";
import { LocaleSwitcher } from "@/components/locale-switcher";
import { api } from "@/lib/api";
import { useI18n, type MessageKey } from "@/lib/i18n";
import { SITE_NAME } from "@/lib/site-config";
import { useRegionRefresh } from "@/hooks/use-region-refresh";
import { RegionRefreshKeys } from "@/lib/region-refresh";
import { useSystemStore } from "@/store/system";
import { sanitizeImageUrl } from "@/lib/safe-url";

export interface SidebarNavItem {
  href: string;
  labelKey: MessageKey;
  icon: ComponentType<{ className?: string }>;
}

type ViewTransitionLike = {
  ready: Promise<void>;
};

type MaybeStartViewTransition = {
  startViewTransition?: (updateCallback: () => void | Promise<void>) => ViewTransitionLike;
};

export const userNavItems: SidebarNavItem[] = [
  { href: "/dashboard", labelKey: "navigation.dashboard", icon: LayoutDashboard },
  { href: "/announcements", labelKey: "navigation.announcements", icon: Megaphone },
  { href: "/tickets", labelKey: "navigation.tickets", icon: MessageSquareMore },
  { href: "/media", labelKey: "navigation.mediaRequest", icon: Film },
  { href: "/bangumi", labelKey: "navigation.bangumi", icon: BookOpen },
  { href: "/score", labelKey: "navigation.signin", icon: Coins },
  { href: "/invite", labelKey: "navigation.inviteCenter", icon: GitBranch },
  { href: "/settings", labelKey: "navigation.settings", icon: Settings },
];

export const adminNavItems: SidebarNavItem[] = [
  { href: "/admin/users", labelKey: "navigation.users", icon: Users },
  { href: "/admin/announcements", labelKey: "navigation.adminAnnouncements", icon: Megaphone },
  { href: "/admin/tickets", labelKey: "navigation.adminTickets", icon: MessageSquareMore },
  { href: "/admin/regcodes", labelKey: "navigation.regcodes", icon: FileText },
  { href: "/admin/invite", labelKey: "navigation.inviteForest", icon: Network },
  { href: "/admin/requests", labelKey: "navigation.requestReview", icon: Film },
  { href: "/admin/violations", labelKey: "navigation.violations", icon: ShieldAlert },
  { href: "/admin/audit-logs", labelKey: "navigation.auditLogs", icon: ClipboardList },
  { href: "/admin/bangumi", labelKey: "navigation.bangumiAdmin", icon: BookOpen },
  { href: "/admin/email", labelKey: "navigation.emailAdmin", icon: Mail },
  { href: "/admin/telegram-rebind-requests", labelKey: "navigation.telegramRebind", icon: MessageSquare },
  { href: "/admin/emby", labelKey: "navigation.embyAdmin", icon: Server },
  { href: "/admin/device-audit", labelKey: "navigation.embyDeviceAudit", icon: MonitorSmartphone },
  { href: "/admin/scheduler", labelKey: "navigation.scheduler", icon: TimerReset },
  { href: "/admin/database", labelKey: "navigation.databaseBackup", icon: Database },
  { href: "/admin/config", labelKey: "navigation.configAdmin", icon: FileCode },
  { href: "/admin/logs", labelKey: "navigation.runtimeLogs", icon: ScrollText },
  { href: "/admin/test", labelKey: "navigation.serverInfo", icon: TestTube },
];

export function filterNavItems(
  items: SidebarNavItem[],
  features?: Record<string, boolean> | null,
) {
  return items.filter((item) => {
    if (features?.media_request === false && item.href === "/media") {
      return false;
    }
    if (features?.signin === false && item.href === "/score") {
      return false;
    }
    if (features?.bangumi_sync === false && item.href === "/bangumi") {
      return false;
    }
    if (features?.ticket_system === false && (item.href === "/tickets" || item.href === "/admin/tickets")) {
      return false;
    }
    return true;
  });
}

export function Sidebar() {
  const pathname = usePathname();
  const { t } = useI18n();
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
  const envIcon = process.env.NEXT_PUBLIC_AUTH_ICON_URL?.trim();
  const safeSystemIcon = useMemo(() => sanitizeImageUrl(envIcon || systemInfo?.icon), [envIcon, systemInfo?.icon]);
  const safeProfileAvatar = useMemo(() => sanitizeImageUrl(profileAvatar), [profileAvatar]);
  const visibleUserNavItems = useMemo(
    () => filterNavItems(userNavItems, systemInfo?.features),
    [systemInfo?.features],
  );
  const visibleAdminNavItems = useMemo(
    () => filterNavItems(adminNavItems, systemInfo?.features),
    [systemInfo?.features],
  );
  const themeLabel = currentTheme === "dark" ? t("common.themeDark") : t("common.themeLight");

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
            <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">{t("navigation.mediaOps")}</p>
            <h2 className="text-lg font-semibold">{displaySiteName}</h2>
          </div>
        </div>

        <nav className="sidebar-nav">
          <p className="sidebar-label">{t("navigation.userMenu")}</p>
          {visibleUserNavItems.map((item) => {
            const active = pathname === item.href;
            return (
              <Link
                key={item.href}
                href={item.href}
                className={cn("sidebar-link", active && "sidebar-link-active")}
              >
                <item.icon className="h-4 w-4" />
                <span>{t(item.labelKey)}</span>
                {active && <span className="sidebar-dot" />}
              </Link>
            );
          })}
          {isAdmin && (
            <>
              <p className="sidebar-label mt-5">{t("navigation.adminMenu")}</p>
              {visibleAdminNavItems.map((item) => {
                const active = pathname.startsWith(item.href);
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    className={cn("sidebar-link", active && "sidebar-link-active")}
                  >
                    <item.icon className="h-4 w-4" />
                    <span>{t(item.labelKey)}</span>
                    {active && <span className="sidebar-dot" />}
                  </Link>
                );
              })}
            </>
          )}
        </nav>

        <div className="sidebar-footer">
          <GithubProjectLink className="w-full" />

          <div className="profile-card">
            <Avatar className="h-10 w-10 border border-border/60">
              {safeProfileAvatar && <AvatarImage src={safeProfileAvatar} alt={user?.username} referrerPolicy="no-referrer" />}
              <AvatarFallback className="bg-primary/15 text-primary text-xs font-semibold">
                {user?.username?.slice(0, 2).toUpperCase() || "U"}
              </AvatarFallback>
            </Avatar>
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">{user?.username}</p>
              <p className="truncate text-xs text-muted-foreground">{user?.role_name}</p>
            </div>
          </div>

          <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_2.5rem] gap-2">
            <Button
              type="button"
              variant="outline"
              className="h-10 justify-start gap-2 overflow-hidden rounded-full border-border/70 bg-background/60 px-3 transition-all hover:bg-primary/10 hover:text-primary"
              onClick={toggleTheme}
              title={`${themeLabel} · ${t("common.switchTheme")}`}
              aria-label={t("common.switchTheme")}
            >
              {currentTheme === "dark" ? <Moon className="h-4 w-4" /> : <Sun className="h-4 w-4" />}
              <span className="truncate text-xs font-medium">{themeLabel}</span>
            </Button>
            <LocaleSwitcher
              align="start"
              className="h-10 justify-start overflow-hidden rounded-full border-border/70 bg-background/60 px-3 transition-all hover:bg-primary/10 hover:text-primary"
            />
            <Button
              type="button"
              variant="outline"
              className="h-10 rounded-full border-border/70 bg-background/60 transition-all hover:bg-destructive/10 hover:text-destructive"
              onClick={() => void logout()}
              title={t("common.logout")}
              aria-label={t("common.logout")}
            >
              <LogOut className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>
    </aside>
  );
}
