"use client";

import Link from "next/link";
import Image from "next/image";
import { usePathname } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { useTheme } from "next-themes";
import { useAuthStore } from "@/store/auth";
import { useSystemStore } from "@/store/system";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { adminNavItems, userNavItems } from "@/components/layout/sidebar";
import { Github, Menu, Moon, Sparkles, Sun } from "lucide-react";

const SAFE_IMAGE_URL = /^(https?:\/\/|\/|data:image\/(png|jpe?g|gif|webp|avif|bmp)(;|,))/i;

function sanitizeImageUrl(url?: string | null): string | undefined {
  const value = (url || "").trim();
  if (!value || !SAFE_IMAGE_URL.test(value)) return undefined;
  return value;
}

export function Header() {
  const pathname = usePathname();
  const { user, logout } = useAuthStore();
  const { info: systemInfo, fetchInfo: fetchSystemInfo } = useSystemStore();
  const { theme, resolvedTheme, setTheme } = useTheme();
  const [mobileOpen, setMobileOpen] = useState(false);
  const isAdmin = user?.role === 0;
  const activeTheme = resolvedTheme || theme || "light";
  const isDark = activeTheme === "dark";
  const systemIcon = useMemo(() => sanitizeImageUrl(systemInfo?.icon), [systemInfo?.icon]);
  const displaySiteName = systemInfo?.name || "Twilight";

  useEffect(() => {
    void fetchSystemInfo();
  }, [fetchSystemInfo]);

  return (
    <header className="sticky top-0 z-30 px-2 pt-3 sm:px-4 sm:pt-4 md:px-6 md:pt-6 xl:px-8">
      <div className="header-surface">
        <div className="flex min-w-0 items-center gap-4">
          <Dialog open={mobileOpen} onOpenChange={setMobileOpen}>
            <DialogTrigger asChild>
              <Button variant="outline" size="icon" className="lg:hidden">
                <Menu className="h-5 w-5" />
                <span className="sr-only">打开菜单</span>
              </Button>
            </DialogTrigger>
            <DialogContent className="left-auto right-0 top-0 h-[100dvh] w-[min(92vw,24rem)] max-w-none translate-x-0 translate-y-0 grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden rounded-none border-y-0 border-r-0 p-0 sm:max-h-[100dvh] sm:rounded-none sm:p-0">
              <DialogHeader className="border-b px-5 py-4 pr-12 text-left">
                <DialogTitle>导航菜单</DialogTitle>
                <DialogDescription>快速切换页面</DialogDescription>
              </DialogHeader>

              <nav className="min-h-0 space-y-2 overflow-y-auto overscroll-contain px-3 py-4">
                <p className="px-2 text-xs uppercase tracking-[0.14em] text-muted-foreground">用户菜单</p>
                {userNavItems.map((item) => {
                  const active = pathname === item.href;
                  return (
                    <Link
                      key={item.href}
                      href={item.href}
                      onClick={() => setMobileOpen(false)}
                      aria-current={active ? "page" : undefined}
                      className={cn(
                        "flex min-w-0 items-center gap-3 rounded-lg px-3 py-3 text-sm",
                        active ? "bg-primary/12 text-primary" : "hover:bg-muted"
                      )}
                    >
                      <item.icon className="h-4 w-4 shrink-0" />
                      <span className="truncate">{item.label}</span>
                    </Link>
                  );
                })}

                {isAdmin && (
                  <>
                    <p className="px-2 pt-2 text-xs uppercase tracking-[0.14em] text-muted-foreground">管理菜单</p>
                    {adminNavItems.map((item) => {
                      const active = pathname.startsWith(item.href);
                      return (
                        <Link
                          key={item.href}
                          href={item.href}
                          onClick={() => setMobileOpen(false)}
                          aria-current={active ? "page" : undefined}
                          className={cn(
                            "flex min-w-0 items-center gap-3 rounded-lg px-3 py-3 text-sm",
                            active ? "bg-primary/12 text-primary" : "hover:bg-muted"
                          )}
                        >
                          <item.icon className="h-4 w-4 shrink-0" />
                          <span className="truncate">{item.label}</span>
                        </Link>
                      );
                    })}
                  </>
                )}
              </nav>

              <div className="grid grid-cols-2 gap-2 border-t bg-background/95 p-4">
                <a
                  href="https://github.com/Prejudice-Studio/Twilight"
                  target="_blank"
                  rel="noreferrer"
                  className="col-span-2 flex h-11 min-w-0 items-center justify-center gap-2 rounded-md border border-input bg-background px-3 py-2 text-sm font-medium hover:bg-accent hover:text-accent-foreground"
                >
                  <Github className="h-4 w-4 shrink-0" />
                  <span className="truncate">GitHub Project</span>
                </a>
                <Button
                  variant="outline"
                  className="h-11 w-full min-w-0"
                  onClick={() => setTheme(isDark ? "light" : "dark")}
                  title={`当前主题：${isDark ? "暗色" : "浅色"}`}
                >
                  {isDark ? <Moon className="mr-2 h-4 w-4 shrink-0" /> : <Sun className="mr-2 h-4 w-4 shrink-0" />}
                  <span className="truncate">{isDark ? "暗色" : "浅色"}</span>
                </Button>
                <Button
                  variant="outline"
                  className="h-11 w-full min-w-0"
                  onClick={() => {
                    setMobileOpen(false);
                    void logout();
                  }}
                >
                  <span className="truncate">退出登录</span>
                </Button>
              </div>
            </DialogContent>
          </Dialog>

          {systemIcon ? (
            <Image
              src={systemIcon}
              alt={displaySiteName}
              width={40}
              height={40}
              className="h-10 w-10 shrink-0 rounded-2xl border border-border/70 object-cover shadow-sm"
              unoptimized
              referrerPolicy="no-referrer"
            />
          ) : (
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl bg-primary/15 text-primary">
              <Sparkles className="h-5 w-5" />
            </div>
          )}
          <div className="min-w-0">
            <p className="text-xs uppercase tracking-[0.16em] text-muted-foreground">DashBoard</p>
            <h1 className="truncate text-base font-semibold md:text-lg">
              欢迎回来，{user?.username}
            </h1>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <Badge variant="outline" className="hidden md:inline-flex">
            {user?.role_name}
          </Badge>
        </div>
      </div>
    </header>
  );
}

