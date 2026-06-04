"use client";

import { ExternalLink } from "lucide-react";
import { GithubIcon } from "@/components/icons/github-icon";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";

export const GITHUB_PROJECT_URL = "https://github.com/Prejudice-Studio/Twilight";

interface GithubProjectLinkProps {
  className?: string;
  compact?: boolean;
}

export function GithubProjectLink({ className, compact = false }: GithubProjectLinkProps) {
  const { t } = useI18n();

  return (
    <a
      href={GITHUB_PROJECT_URL}
      target="_blank"
      rel="noreferrer"
      className={cn(
        "group inline-flex min-w-0 items-center gap-3 rounded-2xl border border-border/70 bg-background/70 px-3 py-2 text-sm font-medium text-foreground shadow-sm transition-all hover:-translate-y-0.5 hover:border-primary/40 hover:bg-primary/10 hover:text-primary hover:shadow-md",
        compact && "h-10 rounded-md px-4 py-2",
        className,
      )}
      title={t("common.externalLink")}
    >
      <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-foreground text-background transition-colors group-hover:bg-primary group-hover:text-primary-foreground">
        <GithubIcon className="h-4 w-4" />
      </span>
      <span className="min-w-0 flex-1 text-left">
        <span className="block truncate leading-tight">{t("common.githubProject")}</span>
        {!compact && (
          <span className="block truncate text-[11px] font-normal text-muted-foreground group-hover:text-primary/80">
            {t("common.projectSource")}
          </span>
        )}
      </span>
      <ExternalLink className="h-3.5 w-3.5 shrink-0 opacity-50 transition-opacity group-hover:opacity-100" />
    </a>
  );
}
