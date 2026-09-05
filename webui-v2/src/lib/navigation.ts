import type { t } from "$lib/i18n";

export type NavigationItem = {
  href: string;
  label: keyof typeof t;
  match?: "exact" | "prefix";
};

export type NavigationGroup = {
  label: keyof typeof t;
  items: NavigationItem[];
};

export const primaryNavigation: NavigationItem[] = [
  { href: "/dashboard", label: "dashboard" },
  { href: "/announcements", label: "announcements" },
  { href: "/score", label: "signin" },
  { href: "/media", label: "media" },
  { href: "/tickets", label: "tickets", match: "prefix" }
];

export const accountNavigation: NavigationItem[] = [
  { href: "/settings", label: "settings" },
  { href: "/settings/appearance", label: "settingsAppearance", match: "prefix" },
  { href: "/settings/apikey", label: "apiKeyTitle", match: "prefix" },
  { href: "/invite", label: "inviteTitle" },
  { href: "/bangumi", label: "bangumiTitle", match: "prefix" },
  { href: "/wiki", label: "wikiTitle" }
];

export const adminNavigation: NavigationGroup[] = [
  {
    label: "adminHomeCategoryUser",
    items: [
      { href: "/admin", label: "adminHomeTitle" },
      { href: "/admin/users", label: "adminUsersTitle", match: "prefix" },
      { href: "/admin/regcodes", label: "adminRegcodesTitle", match: "prefix" },
      { href: "/admin/invite", label: "adminInviteTitle", match: "prefix" }
    ]
  },
  {
    label: "adminHomeCategoryContent",
    items: [
      { href: "/admin/tickets", label: "adminTicketsTitle", match: "prefix" },
      { href: "/admin/requests", label: "adminRequestsTitle", match: "prefix" },
      { href: "/admin/announcements", label: "adminAnnouncementsTitle", match: "prefix" },
      { href: "/admin/bangumi", label: "adminBangumiTitle", match: "prefix" }
    ]
  },
  {
    label: "adminHomeCategorySecurity",
    items: [
      { href: "/admin/security", label: "adminSecurityTitle", match: "prefix" },
      { href: "/admin/audit-logs", label: "adminAuditLogTitle", match: "prefix" },
      { href: "/admin/violations", label: "adminViolationsTitle", match: "prefix" },
      { href: "/admin/developer", label: "adminDeveloperTitle", match: "prefix" }
    ]
  },
  {
    label: "adminHomeCategoryOperations",
    items: [
      { href: "/admin/status", label: "adminStatusTitle", match: "prefix" },
      { href: "/admin/scheduler", label: "adminSchedulerTitle", match: "prefix" },
      { href: "/admin/logs", label: "adminRuntimeLogsTitle", match: "prefix" },
      { href: "/admin/config", label: "adminConfigTitle", match: "prefix" },
      { href: "/admin/database", label: "adminDatabaseTitle", match: "prefix" }
    ]
  },
  {
    label: "adminHomeCategoryIntegration",
    items: [
      { href: "/admin/emby", label: "adminEmbyTitle", match: "prefix" },
      { href: "/admin/telegram", label: "adminTelegramTitle", match: "exact" },
      { href: "/admin/telegram-rebind-requests", label: "adminTelegramRebind", match: "prefix" },
      { href: "/admin/email", label: "adminEmailTitle", match: "prefix" }
    ]
  }
];

export function isActivePath(pathname: string, item: NavigationItem): boolean {
  if (pathname === item.href) return true;
  return item.match === "prefix" && pathname.startsWith(`${item.href}/`);
}
