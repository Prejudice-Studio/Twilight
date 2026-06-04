"use client";

import React, { createContext, useCallback, useContext, useEffect, useState } from "react";
import basicMessages from "@/locales/basic.json";
import zhHansMessages from "@/locales/zh-Hans.json";
import zhHantMessages from "@/locales/zh-Hant.json";
import enUSMessages from "@/locales/en-US.json";

const localeStorageKey = "twilight:locale";

export const supportedLocales = ["zh-Hans", "zh-Hant", "en-US"] as const;
export type Locale = (typeof supportedLocales)[number];

export const defaultLocale: Locale = "zh-Hans";

export const localeLabels: Record<Locale, string> = {
  "zh-Hans": "简体中文",
  "zh-Hant": "繁體中文",
  "en-US": "English",
};

export const localeShortLabels: Record<Locale, string> = {
  "zh-Hans": "简中",
  "zh-Hant": "繁中",
  "en-US": "EN",
};

type MessageCatalog = typeof basicMessages;
type MessageOverlay<T> = {
  [K in keyof T]?: T[K] extends string ? string : MessageOverlay<T[K]>;
};

const messages: Record<Locale, MessageOverlay<MessageCatalog>> = {
  "zh-Hans": zhHansMessages,
  "zh-Hant": zhHantMessages,
  "en-US": enUSMessages,
};

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
  let current: unknown = messages[locale];
  for (const part of parts) {
    if (!current || typeof current !== "object" || !(part in current)) {
      current = null;
      break;
    }
    current = (current as Record<string, unknown>)[part];
  }
  if (typeof current === "string") return current;
  current = basicMessages;
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

export function LocaleProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(defaultLocale);
  const [initialized, setInitialized] = useState(false);

  useEffect(() => {
    const stored = normalizeLocale(window.localStorage.getItem(localeStorageKey));
    setLocaleState(stored || preferredBrowserLocale());
    setInitialized(true);
  }, []);

  const setLocale = (nextLocale: Locale) => {
    setLocaleState(nextLocale);
  };

  useEffect(() => {
    if (!initialized) return;
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
