"use client";

import React, { createContext, useCallback, useContext, useEffect, useState } from "react";
import basicMessages from "@/locales/basic.json";

const localeStorageKey = "twilight:locale";

export const supportedLocales = ["zh-Hans", "zh-Hant", "en-US"] as const;
export type Locale = (typeof supportedLocales)[number];

export const defaultLocale: Locale = "zh-Hans";

export const localeLabels: Record<Locale, string> = {
  "zh-Hans": "\u7b80\u4f53\u4e2d\u6587",
  "zh-Hant": "\u7e41\u9ad4\u4e2d\u6587",
  "en-US": "English",
};

export const localeShortLabels: Record<Locale, string> = {
  "zh-Hans": "\u7b80\u4e2d",
  "zh-Hant": "\u7e41\u4e2d",
  "en-US": "EN",
};

type MessageCatalog = typeof basicMessages;
type MessageOverlay<T> = {
  [K in keyof T]?: T[K] extends string ? string : MessageOverlay<T[K]>;
};

const localeOverlays: Record<string, Record<string, unknown> | undefined> = {};

async function preloadLocale(locale: Locale): Promise<void> {
  if (localeOverlays[locale]) return;
  try {
    let mod: { default: Record<string, unknown> };
    if (locale === "zh-Hans") {
      mod = await import("@/locales/zh-Hans.json");
    } else if (locale === "zh-Hant") {
      mod = await import("@/locales/zh-Hant.json");
    } else {
      mod = await import("@/locales/en-US.json");
    }
    localeOverlays[locale] = mod.default;
  } catch {
    // 加载失败静默降级到 basic.json 兜底
  }
}

type DotKeys<T> = {
  [K in keyof T & string]: T[K] extends string ? K : `${K}.${DotKeys<T[K]>}`;
}[keyof T & string];

export type MessageKey = DotKeys<MessageCatalog>;
export type MessageParams = Record<string, string | number | null | undefined>;

interface LocaleContextValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: MessageKey, params?: MessageParams) => string;
}

const LocaleContext = createContext<LocaleContextValue | null>(null);

function normalizeLocale(value?: string | null): Locale | null {
  if (!value) return null;
  const normalized = value.trim().replace(/_/g, "-").toLowerCase();
  if (!normalized) return null;
  const exact = supportedLocales.find((item) => item.toLowerCase() === normalized);
  if (exact) return exact;
  if (normalized === "zh" || normalized === "zh-cn" || normalized === "zh-sg" || normalized.startsWith("zh-hans")) return "zh-Hans";
  if (["zh-tw", "zh-hk", "zh-mo"].includes(normalized) || normalized.startsWith("zh-hant")) return "zh-Hant";
  if (normalized === "en" || normalized === "en-us" || normalized.startsWith("en-")) return "en-US";
  return null;
}

function preferredBrowserLocale(): Locale {
  if (typeof navigator === "undefined") return defaultLocale;
  for (const item of navigator.languages || []) {
    const matched = normalizeLocale(item);
    if (matched) return matched;
  }
  return normalizeLocale(navigator.language) || defaultLocale;
}

function lookupMessage(locale: Locale, key: MessageKey): string {
	const parts = key.split(".");
	const overlay = localeOverlays[locale];
	if (overlay) {
		let current: unknown = overlay;
		for (const part of parts) {
			if (!current || typeof current !== "object" || !(part in current)) {
				current = null;
				break;
			}
			current = (current as Record<string, unknown>)[part];
		}
		if (typeof current === "string") return current;
	}
	let current: unknown = basicMessages;
	for (const part of parts) {
		if (!current || typeof current !== "object" || !(part in current)) {
			current = null;
			break;
		}
		current = (current as Record<string, unknown>)[part];
	}
	if (typeof current === "string") return current;
	return key;
}

function formatMessage(template: string, params?: MessageParams): string {
  if (!params) return template;
  return template.replace(/\{([a-zA-Z0-9_]+)\}/g, (match, name: string) => {
    const value = params[name];
    if (value === null || value === undefined) return "";
    return String(value);
  });
}

// 模块级"当前语言"，由 LocaleProvider 保持同步。用于 React 组件树之外（lib/*、
// hooks、store 等无法调用 useI18n 的纯函数）做本地化。初始值是默认 locale，
// Provider 挂载后会立即用 localStorage / 浏览器偏好覆盖。
let activeLocale: Locale = defaultLocale;

export function getActiveLocale(): Locale {
  return activeLocale;
}

/**
 * 非 React 上下文的翻译函数。语义与 useI18n().t 相同：按当前 activeLocale 查
 * 字典，缺失回退到 basic.json，再缺失返回 key 原文。
 */
export function translate(key: MessageKey, params?: MessageParams): string {
  return formatMessage(lookupMessage(activeLocale, key), params);
}

export function LocaleProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(defaultLocale);
  const [initialized, setInitialized] = useState(false);

	useEffect(() => {
		const stored = normalizeLocale(window.localStorage.getItem(localeStorageKey));
		const next = stored || preferredBrowserLocale();
		activeLocale = next;
		setLocaleState(next);
		setInitialized(true);
		void preloadLocale(next);
	}, []);

	const setLocale = (nextLocale: Locale) => {
		activeLocale = nextLocale;
		setLocaleState(nextLocale);
		void preloadLocale(nextLocale);
	};

  useEffect(() => {
    if (!initialized) return;
    activeLocale = locale;
    document.documentElement.lang = locale;
    window.localStorage.setItem(localeStorageKey, locale);
  }, [initialized, locale]);

  const t = useCallback((key: MessageKey, params?: MessageParams) => {
    return formatMessage(lookupMessage(locale, key), params);
  }, [locale]);

  return <LocaleContext.Provider value={{ locale, setLocale, t }}>{children}</LocaleContext.Provider>;
}

export function useI18n() {
  const context = useContext(LocaleContext);
  if (!context) {
    throw new Error("useI18n must be used within LocaleProvider");
  }
  return context;
}
