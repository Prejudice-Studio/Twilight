"use client";

const STORAGE_KEY = "twilight:theme-custom";
const CSS_VAR_PREFIX = "--tw-custom";
const LEGACY_DEFAULT_PRIMARY = "#7c3aed";
const LEGACY_DEFAULT_ACCENT = "#ede9fe";

export interface ThemeCustom {
  /** 主色，使用 #RRGGBB 存储 */
  primaryColor: string;
  /** 强调色，使用 #RRGGBB 存储 */
  accentColor: string;
  /** 旧版 primary 色相偏移。仅用于 localStorage 迁移。 */
  primaryHueShift?: number;
  /** 圆角基准值 rem（0.25 ~ 2.0，默认 1.0） */
  radius: number;
  /** 玻璃态模糊强度 px（0 ~ 32，默认 12） */
  glassBlur: number;
  /** 紧凑模式 */
  compact: boolean;
  reduceMotion: boolean;
}

const DEFAULTS: ThemeCustom = {
  primaryColor: "#334155",
  accentColor: "#e2e8f0",
  radius: 1.0,
  glassBlur: 12,
  compact: false,
  reduceMotion: false,
};

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

function normalizeHexColor(value: unknown, fallback: string): string {
  if (typeof value !== "string") return fallback;
  const trimmed = value.trim();
  const short = /^#([0-9a-fA-F]{3})$/.exec(trimmed);
  if (short) {
    const [r, g, b] = short[1].split("");
    return `#${r}${r}${g}${g}${b}${b}`.toLowerCase();
  }
  if (/^#[0-9a-fA-F]{6}$/.test(trimmed)) return trimmed.toLowerCase();
  return fallback;
}

function hslToHex(h: number, s: number, l: number): string {
  const hue = (((h % 360) + 360) % 360) / 360;
  const sat = clamp(s, 0, 100) / 100;
  const light = clamp(l, 0, 100) / 100;
  const q = light < 0.5 ? light * (1 + sat) : light + sat - light * sat;
  const p = 2 * light - q;
  const hueToRgb = (t: number) => {
    let next = t;
    if (next < 0) next += 1;
    if (next > 1) next -= 1;
    if (next < 1 / 6) return p + (q - p) * 6 * next;
    if (next < 1 / 2) return q;
    if (next < 2 / 3) return p + (q - p) * (2 / 3 - next) * 6;
    return p;
  };
  const toHex = (n: number) => Math.round(n * 255).toString(16).padStart(2, "0");
  return `#${toHex(hueToRgb(hue + 1 / 3))}${toHex(hueToRgb(hue))}${toHex(hueToRgb(hue - 1 / 3))}`;
}

function hexToRgb(hex: string): { r: number; g: number; b: number } {
  const normalized = normalizeHexColor(hex, DEFAULTS.primaryColor);
  return {
    r: Number.parseInt(normalized.slice(1, 3), 16),
    g: Number.parseInt(normalized.slice(3, 5), 16),
    b: Number.parseInt(normalized.slice(5, 7), 16),
  };
}

function rgbToHsl({ r, g, b }: { r: number; g: number; b: number }): { h: number; s: number; l: number } {
  const red = r / 255;
  const green = g / 255;
  const blue = b / 255;
  const max = Math.max(red, green, blue);
  const min = Math.min(red, green, blue);
  const light = (max + min) / 2;
  const delta = max - min;
  if (delta === 0) return { h: 0, s: 0, l: Math.round(light * 100) };

  const sat = delta / (1 - Math.abs(2 * light - 1));
  let hue = 0;
  if (max === red) hue = 60 * (((green - blue) / delta) % 6);
  else if (max === green) hue = 60 * ((blue - red) / delta + 2);
  else hue = 60 * ((red - green) / delta + 4);
  return {
    h: Math.round((hue + 360) % 360),
    s: Math.round(sat * 100),
    l: Math.round(light * 100),
  };
}

function contrastForeground(hex: string): string {
  const { r, g, b } = hexToRgb(hex);
  const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
  return luminance > 0.58 ? "224 36% 10%" : "0 0% 100%";
}

function hslVar(hex: string): string {
  const { h, s, l } = rgbToHsl(hexToRgb(hex));
  return `${h} ${s}% ${l}%`;
}

function normalizeThemeCustom(partial: Partial<ThemeCustom>): ThemeCustom {
  const migratedPrimary = typeof partial.primaryColor === "string"
    ? partial.primaryColor
    : typeof partial.primaryHueShift === "number"
      ? hslToHex(257 + partial.primaryHueShift, 90, 58)
      : DEFAULTS.primaryColor;

  const primaryColor = normalizeHexColor(migratedPrimary, DEFAULTS.primaryColor);
  const accentColor = normalizeHexColor(partial.accentColor, DEFAULTS.accentColor);
  const isLegacyDefault =
    primaryColor === LEGACY_DEFAULT_PRIMARY && accentColor === LEGACY_DEFAULT_ACCENT;
  const isLegacyHueDefault =
    typeof partial.primaryColor !== "string" && partial.primaryHueShift === 0;

  return {
    ...DEFAULTS,
    ...partial,
    primaryColor: isLegacyDefault || isLegacyHueDefault ? DEFAULTS.primaryColor : primaryColor,
    accentColor: isLegacyDefault ? DEFAULTS.accentColor : accentColor,
    radius: clamp(Number(partial.radius ?? DEFAULTS.radius), 0.25, 2),
    glassBlur: clamp(Number(partial.glassBlur ?? DEFAULTS.glassBlur), 0, 32),
    compact: Boolean(partial.compact),
    reduceMotion: Boolean(partial.reduceMotion),
  };
}

export function getThemeCustom(): ThemeCustom {
  if (typeof window === "undefined") return { ...DEFAULTS };
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { ...DEFAULTS };
    const parsed = JSON.parse(raw) as Partial<ThemeCustom>;
    return normalizeThemeCustom(parsed);
  } catch {
    return { ...DEFAULTS };
  }
}

export function setThemeCustom(partial: Partial<ThemeCustom>): ThemeCustom {
  const current = getThemeCustom();
  const next = normalizeThemeCustom({ ...current, ...partial });
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  } catch { /* quota exceeded etc */ }
  applyThemeCustom(next);
  return next;
}

export function resetThemeCustom(): ThemeCustom {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch { /* ignore */ }
  applyThemeCustom(DEFAULTS);
  return { ...DEFAULTS };
}

/** 把主题自定义值写入 document.documentElement.style */
export function applyThemeCustom(tc: ThemeCustom): void {
  if (typeof window === "undefined") return;
  const root = document.documentElement;
  const normalized = normalizeThemeCustom(tc);

  const primaryHsl = hslVar(normalized.primaryColor);
  const accentHsl = hslVar(normalized.accentColor);
  const { h, s, l } = rgbToHsl(hexToRgb(normalized.primaryColor));

  root.style.setProperty("--primary", primaryHsl);
  root.style.setProperty("--primary-foreground", contrastForeground(normalized.primaryColor));
  root.style.setProperty("--ring", primaryHsl);
  root.style.setProperty("--accent", accentHsl);
  root.style.setProperty("--accent-foreground", contrastForeground(normalized.accentColor));
  root.style.setProperty("--shell-glow", `hsla(${h}, ${s}%, ${l}%, 0.16)`);

  // radius：覆盖 --radius
  root.style.setProperty("--radius", `${normalized.radius}rem`);

  // glass blur：自定义变量，被 .section-surface 等引用
  root.style.setProperty("--tw-glass-blur", `${normalized.glassBlur}px`);

  // compact
  if (normalized.compact) {
    root.classList.add("tw-compact");
  } else {
    root.classList.remove("tw-compact");
  }

  if (normalized.reduceMotion) {
    root.classList.add("tw-reduce-motion");
  } else {
    root.classList.remove("tw-reduce-motion");
  }
}

export function initThemeCustom(): ThemeCustom {
  const tc = getThemeCustom();
  applyThemeCustom(tc);
  return tc;
}
