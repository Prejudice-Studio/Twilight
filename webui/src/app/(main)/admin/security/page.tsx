"use client";

import Link from "next/link";
import { ClipboardList, ScrollText, ShieldAlert, MonitorSmartphone } from "lucide-react";
import { AdminConfigSections } from "@/components/admin/config-section-editor";
import { Card, CardContent } from "@/components/ui/card";
import { useI18n } from "@/lib/i18n";

const securityLinks = [
  { href: "/admin/audit-logs", titleKey: "adminSecurity.auditTitle", descriptionKey: "adminSecurity.auditDescription", icon: ClipboardList },
  { href: "/admin/logs", titleKey: "adminSecurity.logsTitle", descriptionKey: "adminSecurity.logsDescription", icon: ScrollText },
  { href: "/admin/violations", titleKey: "adminSecurity.riskTitle", descriptionKey: "adminSecurity.riskDescription", icon: ShieldAlert },
  { href: "/admin/emby?tab=devices", titleKey: "adminSecurity.deviceTitle", descriptionKey: "adminSecurity.deviceDescription", icon: MonitorSmartphone },
] as const;

export default function AdminSecurityCenterPage() {
  const { t } = useI18n();

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("adminSecurity.title")}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t("adminSecurity.description")}</p>
      </div>

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        {securityLinks.map((item) => {
          const Icon = item.icon;
          return (
            <Link key={item.href} href={item.href} prefetch={false}>
              <Card className="h-full transition-colors hover:border-primary/50 hover:bg-accent/30">
                <CardContent className="flex min-h-[112px] gap-3 p-4">
                  <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                    <Icon className="h-5 w-5" />
                  </div>
                  <div className="min-w-0">
                    <p className="font-medium leading-tight">{t(item.titleKey)}</p>
                    <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{t(item.descriptionKey)}</p>
                  </div>
                </CardContent>
              </Card>
            </Link>
          );
        })}
      </div>

      <AdminConfigSections
        sectionKeys={["Security", "RateLimit", "AuditLog", "DeviceLimit"]}
        title={t("adminSecurity.configTitle")}
        description={t("adminSecurity.configDescription")}
        notice={t("adminSecurity.configNotice")}
      />
    </div>
  );
}
