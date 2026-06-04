import { API_BASE } from "./api-request";

const SAFE_IMAGE_URL = /^(https?:\/\/|\/|data:image\/(png|jpe?g|gif|webp|avif|bmp)(;|,)|[a-zA-Z]:[\\/])/i;
const SAFE_BG_CSS_FUNCTION = /^(linear-gradient|radial-gradient|conic-gradient|repeating-linear-gradient|repeating-radial-gradient)\s*\(/i;
const SAFE_BACKGROUND_ASSET_PATH = /^\/api\/v1\/users\/assets\/background\/[a-f0-9]{16}\.(jpg|png|gif|webp|bmp)$/i;

export function sanitizeImageUrl(url?: string | null): string | undefined {
  if (!url) return undefined;
  const value = url.trim();
  if (!value || !SAFE_IMAGE_URL.test(value)) return undefined;
  if (/^[a-zA-Z]:[\\/]/.test(value)) return "/api/v1/system/server-icon";
  return value;
}

export function sanitizeExternalUrl(url?: string | null): string | undefined {
  if (!url) return undefined;
  const value = url.trim();
  if (!value || /[\u0000-\u001F\u007F]/.test(value)) return undefined;
  try {
    const parsed = new URL(value);
    return parsed.protocol === "https:" || parsed.protocol === "http:" ? parsed.toString() : undefined;
  } catch {
    return undefined;
  }
}

export function telegramBotUrl(username?: string | null, url?: string | null): string | undefined {
  const safeUrl = sanitizeExternalUrl(url);
  if (safeUrl) return safeUrl;
  const name = (username || "").trim().replace(/^@/, "");
  if (!/^[A-Za-z0-9_]{5,32}$/.test(name)) return undefined;
  return `https://t.me/${name}`;
}

function escapeCssUrlValue(value: string): string {
  return value.replace(/"/g, '\\"');
}

function sanitizeCssUrl(raw: string): string {
  const value = raw.trim();
  if (!value || /[\u0000-\u001F\u007F]/.test(value) || value.startsWith("//")) {
    return "";
  }
  let path = value;
  let origin = "";
  if (/^https?:\/\//i.test(value)) {
    try {
      const parsed = new URL(value);
      if (parsed.search || parsed.hash) return "";
      const allowedOrigins = new Set<string>();
      if (typeof window !== "undefined") allowedOrigins.add(window.location.origin);
      if (API_BASE) allowedOrigins.add(new URL(API_BASE, typeof window === "undefined" ? "http://localhost" : window.location.origin).origin);
      if (!allowedOrigins.has(parsed.origin)) return "";
      origin = parsed.origin;
      path = parsed.pathname;
    } catch {
      return "";
    }
  }
  if (SAFE_BACKGROUND_ASSET_PATH.test(path)) {
    return escapeCssUrlValue(`${origin}${path}`);
  }
  if (/^[a-z][a-z0-9+.-]*:/i.test(value)) {
    return "";
  }
  return "";
}

function sanitizeGradientCss(raw: string): string {
  const value = raw.trim();
  if (!SAFE_BG_CSS_FUNCTION.test(value)) return "";
  if (value.length > 2000 || /[\u0000-\u001F\u007F<>;{}]/.test(value) || /url\s*\(/i.test(value) || value.includes("@")) {
    return "";
  }
  return value;
}

export function normalizeBackgroundImageValue(raw?: string | null): string {
  const value = (raw || "").trim();
  if (!value) return "";
  const gradient = sanitizeGradientCss(value);
  if (gradient) {
    return gradient;
  }
  const urlMatch = value.match(/^url\(\s*(['"]?)(.*?)\1\s*\)$/i);
  const safeUrl = sanitizeCssUrl(urlMatch ? urlMatch[2] : value);
  return safeUrl ? `url("${safeUrl}")` : "";
}
