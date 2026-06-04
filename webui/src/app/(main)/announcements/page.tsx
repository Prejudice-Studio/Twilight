"use client";

import { motion } from "framer-motion";
import { Megaphone } from "lucide-react";
import { AnnouncementBoard } from "@/components/announcement-board";
import { useI18n } from "@/lib/i18n";

export default function UserAnnouncementsPage() {
  const { t } = useI18n();

  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      className="space-y-6"
    >
      <div>
        <h1 className="text-2xl font-bold flex items-center gap-2">
          <Megaphone className="h-5 w-5" />
          {t("announcements.pageTitle")}
        </h1>
        <p className="text-sm text-muted-foreground mt-1">
          {t("announcements.pageDescription")}
        </p>
      </div>

      <AnnouncementBoard
        title={null}
        limit={200}
        collapseAfter={200}
        showEmptyState
      />
    </motion.div>
  );
}
