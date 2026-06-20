"use client";

import Link from "next/link";
import { ArrowLeft, BookOpen } from "lucide-react";
import { Button } from "@/components/ui/button";
import { DeveloperJSDocsPanel } from "@/components/developer-js-docs-panel";
import { useI18n } from "@/lib/i18n";

export default function DeveloperJSDocsPage() {
  const { t } = useI18n();

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <h1 className="flex items-center gap-2 text-2xl font-bold">
            <BookOpen className="h-6 w-6" />
            {t("adminDeveloper.docsPageTitle")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">{t("adminDeveloper.docsPageDescription")}</p>
        </div>
        <Button asChild variant="outline">
          <Link href="/admin/developer">
            <ArrowLeft className="mr-2 h-4 w-4" />
            {t("adminDeveloper.backToDeveloper")}
          </Link>
        </Button>
      </div>

      <DeveloperJSDocsPanel />
    </div>
  );
}
