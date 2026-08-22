"use client";

import Link from "next/link";
import Image from "next/image";
import { usePathname } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";
import { useAuthStore } from "@/store/auth";
import { useSystemStore } from "@/store/system";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { sanitizeImageUrl } from "@/lib/safe-url";
import {
  adminCategoryLabelKeys,
  adminNavItems,
  filterNavItems,
  groupNavItems,
  isActivePath,
  userNavItems,
} from "@/components/layout/sidebar";
import { Menu, Sparkles } from "lucide-react";
import { GithubProjectLink } from "@/components/github-project-link";
import { LocaleSwitcher } from "@/components/locale-switcher";
import { ThemeSwitcher } from "@/components/theme-switcher";
import { useI18n } from "@/lib/i18n";

export function Header() {
  const pathname = usePathname();
  const { user, logout } = useAuthStore();
  const { t } = useI18n();
  const { info: systemInfo } = useSystemStore();
  const [mobileOpen, setMobileOpen] = useState(false);
  const mobileNavRef = useRef<HTMLElement | null>(null);
  const isAdmin = user?.role === 0;
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
  const visibleAdminNavGroups = useMemo(
    () => groupNavItems(visibleAdminNavItems),
    [visibleAdminNavItems],
  );

  useEffect(() => {
    if (!mobileOpen) return;
    const frame = window.requestAnimationFrame(() => {
      mobileNavRef.current
        ?.querySelector<HTMLElement>('[aria-current="page"]')
        ?.scrollIntoView({ block: "nearest", inline: "nearest" });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [mobileOpen, pathname]);

  return (
    <header className="app-header sticky top-0 z-30 mx-auto w-full max-w-[1680px] px-2 sm:px-4 md:px-6 xl:px-8">
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

              <nav ref={mobileNavRef} className="custom-scrollbar min-h-0 space-y-3 overflow-y-auto overscroll-contain px-3 py-4 [scrollbar-gutter:stable]">
                <p className="px-2 text-xs uppercase tracking-[0.14em] text-muted-foreground">{t("navigation.userMenu")}</p>
                {visibleUserNavItems.map((item) => {
                  const active = isActivePath(pathname, item.href);
                  return (
                    <Link
                      key={item.href}
                      href={item.href}
                      prefetch={false}
                      onClick={() => setMobileOpen(false)}
                      aria-current={active ? "page" : undefined}
                      className={cn(
                        "flex min-h-11 min-w-0 items-center gap-3 rounded-lg px-3 py-2.5 text-sm",
                        active ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"
                      )}
                      >
                        <item.icon className="h-4 w-4 shrink-0" />
                      <span className="min-w-0 break-words">{item.label || (item.labelKey ? t(item.labelKey) : "")}</span>
                    </Link>
                  );
                })}

                {isAdmin && (
                  <>
                    <p className="px-2 pt-2 text-xs uppercase tracking-[0.14em] text-muted-foreground">{t("navigation.adminMenu")}</p>
                    {visibleAdminNavGroups.map((group) => (
                      <div key={group.category || "account"} className="space-y-1">
                        {group.category && (
                          <p className="px-3 pt-2 text-[11px] font-medium text-muted-foreground/70">
                            {adminCategoryLabelKeys[group.category]
                              ? t(adminCategoryLabelKeys[group.category])
                              : group.category}
                          </p>
                        )}
                        {group.items.map((item) => {
                          const active = isActivePath(pathname, item.href);
                          return (
                            <Link
                              key={item.href}
                              href={item.href}
                              prefetch={false}
                              onClick={() => setMobileOpen(false)}
                              aria-current={active ? "page" : undefined}
                              className={cn(
                                "flex min-h-11 min-w-0 items-center gap-3 rounded-lg px-3 py-2.5 text-sm",
                                active ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"
                              )}
                            >
                              <item.icon className="h-4 w-4 shrink-0" />
                              <span className="min-w-0 break-words">{item.label || (item.labelKey ? t(item.labelKey) : "")}</span>
                            </Link>
                          );
                        })}
                      </div>
                    ))}
                  </>
                )}
              </nav>

              <div className="mobile-nav-footer grid grid-cols-2 gap-2 border-t bg-background/95 px-4 pt-4 sm:grid-cols-3">
                <GithubProjectLink className="col-span-2 sm:col-span-3" />
                <ThemeSwitcher
                  align="center"
                  showLabel
                  className="min-h-11 w-full min-w-0 justify-center"
                  onModeChange={() => setMobileOpen(false)}
                />
                <LocaleSwitcher
                  align="center"
                  className="min-h-11 w-full min-w-0 justify-center px-2"
                  onLocaleChange={() => setMobileOpen(false)}
                />
                <Button
                  variant="outline"
                  className="col-span-2 min-h-11 w-full min-w-0 sm:col-span-1"
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
              className="hidden h-10 w-10 shrink-0 rounded-2xl border border-border/70 object-cover shadow-sm sm:block"
              unoptimized
              referrerPolicy="no-referrer"
            />
          ) : (
            <div className="hidden h-10 w-10 shrink-0 items-center justify-center rounded-2xl bg-primary/15 text-primary sm:flex">
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
