"use client";

import { motion } from "framer-motion";
import Link from "next/link";
import {
  Users, Megaphone, MessageSquareMore, FileText, Network, Film, ShieldAlert,
  ClipboardList, BookOpen, Mail, MessageSquare, Server,
  TimerReset, Database, FileCode, ScrollText, TestTube,
  AlertTriangle, Settings, Shield, Code2,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { useI18n } from "@/lib/i18n";
import type { LucideIcon } from "lucide-react";

interface AdminNavEntry {
  href: string;
  labelKey: string;
  icon: LucideIcon;
  category: string;
}

const adminPages: AdminNavEntry[] = [
  { href: "/admin/users", labelKey: "navigation.users", icon: Users, category: "user" },
  { href: "/admin/regcodes", labelKey: "navigation.regcodes", icon: FileText, category: "user" },
  { href: "/admin/invite", labelKey: "navigation.inviteForest", icon: Network, category: "user" },
  { href: "/admin/violations", labelKey: "navigation.violations", icon: ShieldAlert, category: "user" },
  { href: "/admin/requests", labelKey: "navigation.requestReview", icon: Film, category: "content" },
  { href: "/admin/announcements", labelKey: "navigation.adminAnnouncements", icon: Megaphone, category: "content" },
  { href: "/admin/tickets", labelKey: "navigation.adminTickets", icon: MessageSquareMore, category: "content" },
  { href: "/admin/bangumi", labelKey: "navigation.bangumiAdmin", icon: BookOpen, category: "content" },
  { href: "/admin/security", labelKey: "navigation.securityCenter", icon: Shield, category: "security" },
  { href: "/admin/audit-logs", labelKey: "navigation.auditLogs", icon: ClipboardList, category: "security" },
  { href: "/admin/scheduler", labelKey: "navigation.scheduler", icon: TimerReset, category: "ops" },
  { href: "/admin/database", labelKey: "navigation.databaseBackup", icon: Database, category: "ops" },
  { href: "/admin/config", labelKey: "navigation.configAdmin", icon: FileCode, category: "ops" },
  { href: "/admin/logs", labelKey: "navigation.runtimeLogs", icon: ScrollText, category: "ops" },
  { href: "/admin/developer", labelKey: "navigation.developerMode", icon: Code2, category: "ops" },
  { href: "/admin/test", labelKey: "navigation.serverInfo", icon: TestTube, category: "ops" },
  { href: "/admin/emby", labelKey: "navigation.embyAdmin", icon: Server, category: "integration" },
  { href: "/admin/email", labelKey: "navigation.emailAdmin", icon: Mail, category: "integration" },
  { href: "/admin/telegram", labelKey: "navigation.telegramAdmin", icon: MessageSquare, category: "integration" },
];

const categoryConfig: Record<string, { labelKey: string; icon: LucideIcon }> = {
  user: { labelKey: "adminNav.categoryUser", icon: Shield },
  content: { labelKey: "adminNav.categoryContent", icon: Megaphone },
  security: { labelKey: "adminNav.categorySecurity", icon: AlertTriangle },
  ops: { labelKey: "adminNav.categoryOps", icon: Settings },
  integration: { labelKey: "adminNav.categoryIntegration", icon: Server },
};

const container = {
  hidden: { opacity: 0 },
  show: { opacity: 1, transition: { staggerChildren: 0.05 } },
};

const item = {
  hidden: { opacity: 0, y: 12 },
  show: { opacity: 1, y: 0 },
};

export default function AdminIndexPage() {
  const { t } = useI18n();

  return (
    <motion.div variants={container} initial="hidden" animate="show" className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold">{t("adminNav.title")}</h1>
        <p className="text-sm text-muted-foreground mt-1">{t("adminNav.description")}</p>
      </div>

      {Object.entries(categoryConfig).map(([catKey, config]) => {
        const pages = adminPages.filter((p) => p.category === catKey);
        if (pages.length === 0) return null;
        const Icon = config.icon;
        return (
          <section key={catKey}>
            <div className="flex items-center gap-2 mb-3">
              <Icon className="h-4 w-4 text-muted-foreground" />
              <h2 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
                {t(config.labelKey as any)}
              </h2>
            </div>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
              {pages.map((page) => {
                const Icon = page.icon;
                return (
                  <motion.div key={page.href} variants={item}>
                    <Link href={page.href} prefetch={false}>
                      <Card className="glass-card cursor-pointer transition-colors hover:border-primary/50 hover:bg-accent/30">
                        <CardContent className="flex items-center gap-3 p-4">
                          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                            <Icon className="h-5 w-5" />
                          </div>
                          <div className="min-w-0">
                            <p className="text-sm font-medium truncate">{t(page.labelKey as any)}</p>
                            <p className="text-xs text-muted-foreground truncate">{page.href}</p>
                          </div>
                        </CardContent>
                      </Card>
                    </Link>
                  </motion.div>
                );
              })}
            </div>
          </section>
        );
      })}
    </motion.div>
  );
}
