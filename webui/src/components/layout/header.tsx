"use client";

import Link from "next/link";
import Image from "next/image";
import { usePathname } from "next/navigation";
import { useMemo, useState } from "react";
import { useTheme } from "next-themes";
import { useAuthStore } from "@/store/auth";
import { useSystemStore } from "@/store/system";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { sanitizeImageUrl } from "@/lib/safe-url";
import { adminNavItems, filterNavItems, userNavItems } from "@/components/layout/sidebar";
import { Menu, Moon, Sparkles, Sun } from "lucide-react";
import { GithubProjectLink } from "@/components/github-project-link";
import { LocaleSwitcher } from "@/components/locale-switcher";
import { useI18n } from "@/lib/i18n";

export function Header() {
  const pathname = usePathname();
  const { user, logout } = useAuthStore();
  const { t } = useI18n();
  const { info: systemInfo } = useSystemStore();
  const { theme, resolvedTheme, setTheme } = useTheme();
  const [mobileOpen, setMobileOpen] = useState(false);
  const isAdmin = user?.role === 0;
  const activeTheme = resolvedTheme || theme || "light";
  const isDark = activeTheme === "dark";
  const themeLabel = isDark ? t("common.themeDark") : t("common.themeLight");
  const envIcon = process.env.NEXT_PUBLIC_AUTH_ICON_URL?.trim();
  const systemIcon = useMemo(() => sanitizeImageUrl(envIcon || systemInfo?.icon), [envIcon, systemInfo?.icon]);
  const displaySiteName = systemInfo?.name || "Twilight";
  const visibleUserNavItems = useMemo(
    () => filterNavItems(userNavItems, systemInfo?.features),
    [systemInfo?.features],
  );
  const visibleAdminNavItems = useMemo(
    () => filterNavItems(adminNavItems, systemInfo?.features),
    [systemInfo?.features],
  );

  return (
    <header className="sticky top-0 z-30 mx-auto w-full max-w-[1680px] px-2 pt-3 sm:px-4 sm:pt-4 md:px-6 md:pt-6 xl:px-8">
      <div className="header-surface">
        <div className="flex min-w-0 items-center gap-4">
          <Dialog open={mobileOpen} onOpenChange={setMobileOpen}>
            <DialogTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                className="lg:hidden"
                aria-label={t("navigation.openMenu")}
              >
                <Menu className="h-5 w-5" aria-hidden="true" />
              </Button>
            </DialogTrigger>
            <DialogContent className="left-auto right-0 top-0 h-[100dvh] w-[min(92vw,24rem)] max-w-none translate-x-0 translate-y-0 grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden rounded-none border-y-0 border-r-0 p-0 sm:max-h-[100dvh] sm:rounded-none sm:p-0">
              <DialogHeader className="border-b px-5 py-4 pr-12 text-left">
                <DialogTitle>{t("navigation.mobileMenuTitle")}</DialogTitle>
                <DialogDescription>{t("navigation.mobileMenuDescription")}</DialogDescription>
              </DialogHeader>

              <nav className="min-h-0 space-y-2 overflow-y-auto overscroll-contain px-3 py-4">
                <p className="px-2 text-xs uppercase tracking-[0.14em] text-muted-foreground">{t("navigation.userMenu")}</p>
                {visibleUserNavItems.map((item) => {
                  const active = pathname === item.href;
                  return (
                    <Link
                      key={item.href}
                      href={item.href}
                      prefetch={false}
                      onClick={() => setMobileOpen(false)}
                      aria-current={active ? "page" : undefined}
                      className={cn(
                        "flex min-w-0 items-center gap-3 rounded-lg px-3 py-3 text-sm",
                        active ? "bg-primary/12 text-primary" : "hover:bg-muted"
                      )}
                      >
                        <item.icon className="h-4 w-4 shrink-0" />
                      <span className="truncate">{t(item.labelKey)}</span>
                    </Link>
                  );
                })}

                {isAdmin && (
                  <>
                    <p className="px-2 pt-2 text-xs uppercase tracking-[0.14em] text-muted-foreground">{t("navigation.adminMenu")}</p>
                    {visibleAdminNavItems.map((item) => {
                      const active = pathname.startsWith(item.href);
                      return (
                        <Link
                          key={item.href}
                          href={item.href}
                          prefetch={false}
                          onClick={() => setMobileOpen(false)}
                          aria-current={active ? "page" : undefined}
                          className={cn(
                            "flex min-w-0 items-center gap-3 rounded-lg px-3 py-3 text-sm",
                            active ? "bg-primary/12 text-primary" : "hover:bg-muted"
                          )}
                          >
                            <item.icon className="h-4 w-4 shrink-0" />
                          <span className="truncate">{t(item.labelKey)}</span>
                        </Link>
                      );
                    })}
                  </>
                )}
              </nav>

              <div className="grid grid-cols-3 gap-2 border-t bg-background/95 p-4">
                <GithubProjectLink className="col-span-3" />
                <Button
                  variant="outline"
                  className="h-11 w-full min-w-0"
                  onClick={() => setTheme(isDark ? "light" : "dark")}
                  title={`${themeLabel} · ${t("common.switchTheme")}`}
                  aria-label={t("common.switchTheme")}
                >
                  {isDark ? <Moon className="mr-2 h-4 w-4 shrink-0" /> : <Sun className="mr-2 h-4 w-4 shrink-0" />}
                  <span className="truncate">{themeLabel}</span>
                </Button>
                <LocaleSwitcher
                  align="center"
                  className="h-11 w-full justify-center px-2"
                  onLocaleChange={() => setMobileOpen(false)}
                />
                <Button
                  variant="outline"
                  className="h-11 w-full min-w-0"
                  aria-label={t("common.logout")}
                  onClick={() => {
                    setMobileOpen(false);
                    void logout();
                  }}
                >
                  <span className="truncate">{t("common.logout")}</span>
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
            <p className="text-xs uppercase tracking-[0.16em] text-muted-foreground">{t("navigation.dashboardLabel")}</p>
            <h1 className="truncate text-base font-semibold md:text-lg">
              {t("navigation.welcomeBack", { username: user?.username || "" })}
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
