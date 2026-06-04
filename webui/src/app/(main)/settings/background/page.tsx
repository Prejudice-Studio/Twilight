"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { PageLoading } from "@/components/layout/page-state";
import { useI18n } from "@/lib/i18n";

export default function BackgroundSettingsRedirectPage() {
  const router = useRouter();
  const { t } = useI18n();

  useEffect(() => {
    router.replace("/settings/appearance");
  }, [router]);

  return <PageLoading message={t("appearance.loading")} />;
}
