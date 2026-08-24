import type { ApiResponse, MediaDetail, MediaItem } from "@/lib/api";
import { sanitizeExternalUrl, sanitizeImageUrl } from "@/lib/safe-url";

export type MediaSearchMode = "name" | "id";
export type MediaSearchSource = "all" | "tmdb" | "bangumi";
export type TMDBMediaType = "movie" | "tv";
export type MediaCoverOrientation = "landscape" | "portrait";

export function mediaCoverOrientationFromDimensions(width: number, height: number): MediaCoverOrientation {
  return Number.isFinite(width) && Number.isFinite(height) && width > height
    ? "landscape"
    : "portrait";
}

export interface ResponseOutcome<T> {
  data?: T;
  message?: string;
  error?: unknown;
}

export async function settleResponse<T>(request: Promise<ApiResponse<T>>): Promise<ResponseOutcome<T>> {
  try {
    const response = await request;
    if (response.success && response.data !== undefined && response.data !== null) {
      return { data: response.data };
    }
    return { message: response.message };
  } catch (error) {
    return { error };
  }
}

export function rememberCache<K, V>(cache: Map<K, V>, key: K, value: V, limit: number): void {
  cache.delete(key);
  cache.set(key, value);
  while (cache.size > limit) {
    const oldestKey = cache.keys().next().value as K | undefined;
    if (oldestKey === undefined) return;
    cache.delete(oldestKey);
  }
}

export function normalizeMediaYear(value: MediaItem["year"]): number | undefined {
  const parsed = typeof value === "number" ? value : Number.parseInt(String(value || ""), 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
}

export function normalizeMediaRating(value: MediaItem["rating"] | MediaItem["vote_average"]): number | undefined {
  const parsed = typeof value === "number" ? value : Number.parseFloat(String(value ?? ""));
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
}

export function normalizeMediaItem(item: MediaItem): MediaItem {
  const source = String(item.source || "tmdb").toLowerCase();
  const mediaType = String(item.media_type || "movie").toLowerCase();
  const poster = sanitizeImageUrl(item.poster || item.poster_url);
  const logo = sanitizeImageUrl(item.logo || item.logo_url);
  return {
    ...item,
    source,
    media_type: mediaType,
    title: String(item.title || ""),
    poster,
    poster_url: poster,
    logo,
    logo_url: logo,
    source_url: sanitizeExternalUrl(item.source_url),
    year: normalizeMediaYear(item.year),
    rating: normalizeMediaRating(item.rating ?? item.vote_average),
  };
}

export function normalizeMediaDetail(detail: MediaDetail): MediaDetail {
  const item = normalizeMediaItem(detail);
  const backdrop = sanitizeImageUrl(detail.backdrop || detail.backdrop_url);
  return {
    ...detail,
    ...item,
    backdrop,
    backdrop_url: backdrop,
    official_url: sanitizeExternalUrl(detail.official_url),
    trailer_url: sanitizeExternalUrl(detail.trailer_url),
  };
}

export function mergeMediaItems(current: MediaItem, incoming: MediaItem): MediaItem {
  const first = normalizeMediaItem(current);
  const next = normalizeMediaItem(incoming);
  return normalizeMediaItem({
    ...next,
    ...first,
    title: first.title || next.title,
    original_title: first.original_title || next.original_title,
    overview: first.overview || next.overview,
    poster: first.poster || next.poster,
    poster_url: first.poster_url || next.poster_url,
    logo: first.logo || next.logo,
    logo_url: first.logo_url || next.logo_url,
    logo_language: first.logo_language || next.logo_language,
    release_date: first.release_date || next.release_date,
    year: first.year ?? next.year,
    source_url: first.source_url || next.source_url,
    rating: first.rating ?? next.rating,
    vote_average: first.vote_average ?? next.vote_average,
  });
}

export function mergeMediaDetail(base: MediaItem, detail: MediaDetail): MediaDetail {
  const fallback = normalizeMediaItem(base);
  const resolved = normalizeMediaDetail(detail);
  return normalizeMediaDetail({
    ...fallback,
    ...resolved,
    title: resolved.title || fallback.title,
    original_title: resolved.original_title || fallback.original_title,
    overview: resolved.overview || fallback.overview,
    poster: resolved.poster || fallback.poster,
    poster_url: resolved.poster_url || fallback.poster_url,
    logo: resolved.logo || fallback.logo,
    logo_url: resolved.logo_url || fallback.logo_url,
    logo_language: resolved.logo_language || fallback.logo_language,
    release_date: resolved.release_date || fallback.release_date,
    year: resolved.year ?? fallback.year,
    source_url: resolved.source_url || fallback.source_url,
    rating: resolved.rating ?? fallback.rating,
    vote_average: resolved.vote_average ?? fallback.vote_average,
  });
}

export function mediaCacheKey(media: MediaItem): string {
  return `${String(media.source).toLowerCase()}-${media.id}-${String(media.media_type).toLowerCase()}`;
}

export function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === "AbortError";
}

export function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}
