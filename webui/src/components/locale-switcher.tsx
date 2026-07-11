"use client";

import { Check, Globe2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import {
  localeLabels,
  localeShortLabels,
  supportedLocales,
  useI18n,
  type Locale,
} from "@/lib/i18n";

interface LocaleSwitcherProps {
  className?: string;
  align?: "start" | "center" | "end";
  showLabel?: boolean;
  onLocaleChange?: (locale: Locale) => void;
}

export function LocaleSwitcher({
  className,
  align = "end",
  showLabel = true,
  onLocaleChange,
}: LocaleSwitcherProps) {
  const { locale, setLocale, t } = useI18n();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className={cn("i18n-control min-w-0 gap-2", className)}
          title={`${t("common.currentLanguage")}: ${localeLabels[locale]}`}
          aria-label={t("common.switchLanguage")}
        >
          <Globe2 className="h-4 w-4 shrink-0" />
          {showLabel && (
            <span className="truncate text-xs font-medium">
              {localeShortLabels[locale]}
            </span>
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align={align} className="w-48">
        <DropdownMenuLabel>{t("common.switchLanguage")}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {supportedLocales.map((item) => (
          <DropdownMenuItem
            key={item}
            onSelect={() => {
              setLocale(item);
              onLocaleChange?.(item);
            }}
            className="gap-2"
          >
            <Check className={cn("h-4 w-4", item === locale ? "opacity-100" : "opacity-0")} />
            <span className="flex-1">{localeLabels[item]}</span>
            <span className="text-[11px] text-muted-foreground">{item}</span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
