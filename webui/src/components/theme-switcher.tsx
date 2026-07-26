"use client";

import { useRef } from "react";
import type { ComponentType } from "react";
import { flushSync } from "react-dom";
import { useTheme } from "next-themes";
import { Check, Monitor, Moon, Sun } from "lucide-react";
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
import { useI18n } from "@/lib/i18n";
import {
  THEME_MODES,
  normalizeThemeMode,
  themeModeLabelKey,
  type ThemeMode,
} from "@/lib/theme-mode";

type ViewTransitionLike = { ready: Promise<void> };
type MaybeStartViewTransition = {
  startViewTransition?: (updateCallback: () => void | Promise<void>) => ViewTransitionLike;
};

const MODE_ICON: Record<ThemeMode, ComponentType<{ className?: string }>> = {
  light: Sun,
  dark: Moon,
  system: Monitor,
};

interface ThemeSwitcherProps {
  className?: string;
  align?: "start" | "center" | "end";
  showLabel?: boolean;
  onModeChange?: (mode: ThemeMode) => void;
}

export function ThemeSwitcher({
  className,
  align = "start",
  showLabel = true,
  onModeChange,
}: ThemeSwitcherProps) {
  const { t } = useI18n();
  const { theme, setTheme } = useTheme();
  // 以「用户所选设置值」驱动展示，未挂载时归一化回退浅色，与 SSR 首帧一致，避免水合抖动。
  const mode = normalizeThemeMode(theme);
  const TriggerIcon = MODE_ICON[mode];
  // 记录最近一次指针坐标，供圆形揭示动画定位；键盘选择时为 null，回退屏幕中心。
  const pointerRef = useRef<{ x: number; y: number } | null>(null);

  const applyMode = (next: ThemeMode) => {
    onModeChange?.(next);
    if (next === mode) return;

    const startViewTransition = (document as unknown as MaybeStartViewTransition).startViewTransition;
    if (!startViewTransition) {
      setTheme(next);
      return;
    }

    const point = pointerRef.current;
    const x = point?.x ?? window.innerWidth / 2;
    const y = point?.y ?? window.innerHeight / 2;

    // 必须在回调里 flushSync 同步提交，否则 React 18 批处理会让 DOM 更新晚于浏览器拍快照。
    let didCommit = false;
    try {
      const transition = startViewTransition(() => {
        flushSync(() => setTheme(next));
        didCommit = true;
      });
      void transition.ready
        .then(() => {
          const radius = Math.hypot(
            Math.max(x, window.innerWidth - x),
            Math.max(y, window.innerHeight - y),
          );
          document.documentElement.animate(
            {
              clipPath: [
                `circle(0px at ${x}px ${y}px)`,
                `circle(${radius}px at ${x}px ${y}px)`,
              ],
            },
            {
              duration: 500,
              easing: "ease-in-out",
              pseudoElement: "::view-transition-new(root)",
            } as KeyframeAnimationOptions,
          );
        })
        .catch(() => undefined);
    } catch {
      if (!didCommit) setTheme(next);
    }
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className={cn("i18n-control min-w-0 gap-2", className)}
          title={`${t(themeModeLabelKey(mode))} · ${t("common.switchTheme")}`}
          aria-label={t("common.switchTheme")}
        >
          <TriggerIcon className="h-4 w-4 shrink-0" />
          {showLabel && (
            <span className="truncate text-xs font-medium">{t(themeModeLabelKey(mode))}</span>
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align={align} className="w-44">
        <DropdownMenuLabel>{t("common.switchTheme")}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {THEME_MODES.map((item) => {
          const ItemIcon = MODE_ICON[item];
          return (
            <DropdownMenuItem
              key={item}
              // onClick 抓取真实鼠标坐标；键盘触发不带坐标，pointerRef 复位为 null。
              onClick={(e) => {
                pointerRef.current = e.clientX && e.clientY ? { x: e.clientX, y: e.clientY } : null;
              }}
              onSelect={() => applyMode(item)}
              className="gap-2"
            >
              <Check className={cn("h-4 w-4 shrink-0", item === mode ? "opacity-100" : "opacity-0")} />
              <ItemIcon className="h-4 w-4 shrink-0" />
              <span className="flex-1">{t(themeModeLabelKey(item))}</span>
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
