import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { Actions, PageServerLoad } from "./$types";
import type { InventoryCheckResult, MediaDetail, MediaItem, MediaRequest } from "$lib/types";
import { t } from "$lib/i18n";
import { safeImageURL } from "$lib/media";

type SearchData = { results: MediaItem[]; total?: number; warnings?: Record<string, string> };
type FormState = { action?: string; error?: string };

function text(form: FormData, name: string): string {
  const value = form.get(name);
  return typeof value === "string" ? value.trim() : "";
}

function cleanText(value: string, maxLength: number): string {
  return value.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g, "").slice(0, maxLength);
}

function positiveID(value: string): string {
  return /^\d{1,18}$/.test(value) && Number(value) > 0 ? value : "";
}

function positiveSeason(value: string): number {
  return /^\d{1,3}$/.test(value) && Number(value) > 0 ? Number(value) : 0;
}

function sourceValue(value: string): string {
  return value === "tmdb" || value === "bangumi" ? value : "all";
}

function typeValue(value: string): string {
  return value === "tv" ? "tv" : "movie";
}

function requestMediaType(source: string, value: string): boolean {
  if (!value || value.length > 40 || /[\u0000-\u001F\u007F]/.test(value)) return false;
  return source === "tmdb" ? value === "movie" || value === "tv" : true;
}

function searchURL(query: string, source: string, mediaType: string): string {
  const params = new URLSearchParams({ q: query, source, limit: "20" });
  if (source !== "bangumi") params.set("type", mediaType);
  return `/api/v1/media/search?${params.toString()}`;
}

async function mutation(
  event: Parameters<NonNullable<Actions["create"]>>[0],
  path: string,
  method: "POST" | "DELETE",
  body: Record<string, unknown> | undefined,
  action: string,
  fallback: string
) {
  const result = await apiJSONWithResponse<unknown>(event, path, {
    method,
    ...(body ? { headers: { "content-type": "application/json" }, body: JSON.stringify(body) } : {})
  });
  const response = result?.response;
  const envelope = result?.envelope;
  if (!response || !envelope || !response.ok || !envelope.success) {
    return fail(response?.status || 503, { action, error: envelope?.message || fallback } satisfies FormState);
  }
  return null;
}

export const load: PageServerLoad = async (event) => {
  const query = event.url.searchParams.get("q")?.trim().slice(0, 120) || "";
  const source = sourceValue(event.url.searchParams.get("source") || "all");
  const mediaType = typeValue(event.url.searchParams.get("type") || "movie");
  const selectedID = positiveID(event.url.searchParams.get("media_id") || "");
  const selectedSeason = positiveSeason(event.url.searchParams.get("season") || "");
  const tab = event.url.searchParams.get("tab") === "requests" ? "requests" : "search";

  let results: MediaItem[] = [];
  let searchWarning: string | null = null;
  let searchFailed = false;
  if (tab === "search" && query) {
    const search = await apiJSON<SearchData>(event, searchURL(query, source, mediaType), { cache: "no-store" });
    if (search?.success && search.data) {
      results = Array.isArray(search.data.results) ? search.data.results : [];
      searchWarning = Object.keys(search.data.warnings || {}).length ? t.mediaSearchFailed : null;
    } else {
      searchFailed = true;
    }
  }

  let detail: MediaDetail | null = null;
  let inventory: InventoryCheckResult | null = null;
  if (tab === "search" && selectedID) {
    const selected = results.find((item) => String(item.id) === selectedID && (source === "all" || item.source === source));
    const selectedSource = selected?.source || (source === "all" ? "tmdb" : source);
    const selectedType = selected?.media_type || mediaType;
    const detailPath = `/api/v1/media/detail?source=${encodeURIComponent(selectedSource)}&media_id=${selectedID}&media_type=${encodeURIComponent(selectedType)}`;
    const inventoryBody = {
      source: selectedSource,
      media_id: Number(selectedID),
      media_type: selectedType,
      ...(selected?.title ? { title: selected.title } : {}),
      ...(selected?.original_title ? { original_title: selected.original_title } : {}),
      ...(selected?.year && Number(selected.year) > 0 ? { year: Number(selected.year) } : {}),
      ...(selectedSeason ? { season: selectedSeason } : {})
    };
    const [detailResponse, inventoryResponse] = await Promise.all([
      apiJSON<MediaDetail>(event, detailPath, { cache: "no-store" }),
      apiJSON<InventoryCheckResult>(event, "/api/v1/media/inventory/check", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(inventoryBody),
        cache: "no-store"
      })
    ]);
    if (detailResponse?.success && detailResponse.data) detail = detailResponse.data;
    else if (selected) detail = selected as MediaDetail;
    if (inventoryResponse?.success && inventoryResponse.data) inventory = inventoryResponse.data;
  }

  let requests: MediaRequest[] | null = null;
  let requestsFailed = false;
  if (tab === "requests") {
    const response = await apiJSON<MediaRequest[]>(event, "/api/v1/media/request/my", { cache: "no-store" });
    if (response?.success && Array.isArray(response.data)) requests = response.data;
    else requestsFailed = true;
  }

  return {
    query,
    source,
    mediaType,
    tab,
    selectedID,
    selectedSeason,
    results,
    detail,
    inventory,
    searchWarning,
    searchFailed,
    requests,
    requestsFailed,
    result: event.url.searchParams.get("result") || ""
  };
};

export const actions: Actions = {
  create: async (event) => {
    const form = await event.request.formData();
    const source = text(form, "source");
    const mediaID = positiveID(text(form, "media_id"));
    const mediaType = text(form, "media_type") || "movie";
    const title = cleanText(text(form, "title"), 200);
    const note = cleanText(text(form, "note"), 500);
    if ((source !== "tmdb" && source !== "bangumi") || !mediaID || !title || !requestMediaType(source, mediaType)) {
      return fail(400, { action: "create", error: t.mediaRequestFailed } satisfies FormState);
    }
    const seasonText = text(form, "season");
    const yearText = text(form, "year");
    const body: Record<string, unknown> = {
      source,
      media_id: Number(mediaID),
      media_type: mediaType,
      title,
      original_title: cleanText(text(form, "original_title"), 200),
      poster: safeImageURL(text(form, "poster").slice(0, 2048)),
      poster_url: safeImageURL(text(form, "poster_url").slice(0, 2048)),
      overview: cleanText(text(form, "overview"), 5000),
      note,
      ...(seasonText && /^\d{1,3}$/.test(seasonText) ? { season: Number(seasonText) } : {}),
      ...(yearText && /^\d{4}$/.test(yearText) ? { year: Number(yearText) } : {})
    };
    const error = await mutation(event, "/api/v1/media/request", "POST", body, "create", t.mediaRequestFailed);
    if (error) return error;
    throw redirect(303, "/media?tab=requests&result=created");
  },

  deleteRequest: async (event) => {
    const form = await event.request.formData();
    const key = text(form, "require_key");
    if (!key || key.length > 128 || /[\\/]/.test(key)) {
      return fail(400, { action: "delete", error: t.mediaRequestDeleteFailed } satisfies FormState);
    }
    const error = await mutation(event, `/api/v1/media/request/by-key/${encodeURIComponent(key)}`, "DELETE", undefined, "delete", t.mediaRequestDeleteFailed);
    if (error) return error;
    throw redirect(303, "/media?tab=requests&result=deleted");
  }
};
