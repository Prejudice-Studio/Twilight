import type { MessageKey } from "@/lib/i18n";

// 主题三态：浅色 / 暗色 / 跟随系统。
// `system` 交给 next-themes 依据 `prefers-color-scheme` 解析为浅色或暗色。
export type ThemeMode = "light" | "dark" | "system";

export const THEME_MODES: readonly ThemeMode[] = ["light", "dark", "system"] as const;

// 把 next-themes 的原始 `theme` 值收敛为三态之一；未知 / 未挂载（undefined）回退浅色。
export function normalizeThemeMode(raw: string | undefined | null): ThemeMode {
  if (raw === "dark" || raw === "system") return raw;
  return "light";
}

// 三态循环顺序：浅色 → 暗色 → 跟随系统 → 浅色。
// 保留「浅→暗」的既有肌肉记忆，把「跟随系统」接在末尾作为新增项。
export function nextThemeMode(raw: string | undefined | null): ThemeMode {
  switch (normalizeThemeMode(raw)) {
    case "light":
      return "dark";
    case "dark":
      return "system";
    default:
      return "light";
  }
}

// 每个模式对应的 i18n 文案键。
export function themeModeLabelKey(mode: ThemeMode): MessageKey {
  switch (mode) {
    case "dark":
      return "common.themeDark";
    case "system":
      return "common.themeSystem";
    default:
      return "common.themeLight";
  }
}
