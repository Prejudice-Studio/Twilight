import type { MediaRequest } from "@/lib/api-types";

export function mediaRequestExternalUrl(req: MediaRequest): string | null {
  const source = (req.source || "").toLowerCase();
  const rawId = req.media_id;
  if (rawId === undefined || rawId === null || rawId === "") return null;

  if (source === "bangumi") {
    const id = String(rawId).replace(/[^0-9]/g, "");
    return id ? `https://bgm.tv/subject/${id}` : null;
  }

  if (source === "tmdb") {
    const text = String(rawId);
    const match = text.match(/^(?:(movie|tv):)?(\d+)$/i);
    const id = match ? match[2] : text.replace(/[^0-9]/g, "");
    if (!id) return null;
    const declaredType = (req.media_info?.media_type || req.media_type || "").toLowerCase();
    const prefixType = match?.[1]?.toLowerCase() || null;
    const tmdbType = prefixType || (declaredType === "tv" || declaredType === "anime" ? "tv" : "movie");
    return `https://www.themoviedb.org/${tmdbType}/${id}`;
  }

  return null;
}
