const SAFE_IMAGE_URL = /^(https?:\/\/|\/|data:image\/(png|jpe?g|gif|webp|avif|bmp)(;|,)|[a-zA-Z]:[\\/])/i;
const SAFE_BG_CSS_FUNCTION = /^(linear-gradient|radial-gradient|conic-gradient|repeating-linear-gradient|repeating-radial-gradient)\s*\(/i;
const SAFE_BG_DATA_IMAGE = /^data:image\/(png|jpe?g|gif|webp|avif|bmp)(;|,)/i;

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
  if (/^https?:\/\//i.test(value) || value.startsWith("/") || value.startsWith("./") || value.startsWith("../")) {
    return escapeCssUrlValue(value);
  }
  if (value.startsWith("blob:") || SAFE_BG_DATA_IMAGE.test(value)) {
    return escapeCssUrlValue(value);
  }
  if (/^[a-z][a-z0-9+.-]*:/i.test(value)) {
    return "";
  }
  return escapeCssUrlValue(value);
}

export function normalizeBackgroundImageValue(raw?: string | null): string {
  const value = (raw || "").trim();
  if (!value) return "";
  if (SAFE_BG_CSS_FUNCTION.test(value)) {
    return value;
  }
  const urlMatch = value.match(/^url\(\s*(['"]?)(.*?)\1\s*\)$/i);
  const safeUrl = sanitizeCssUrl(urlMatch ? urlMatch[2] : value);
  return safeUrl ? `url("${safeUrl}")` : "";
}
