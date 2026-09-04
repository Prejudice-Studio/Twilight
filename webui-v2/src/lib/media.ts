import type { MediaDetail, MediaItem, MediaRequest } from "$lib/types";

export function safeImageURL(value: unknown): string {
  if (typeof value !== "string" || value.length === 0 || value.length > 2048) return "";
  try {
    const url = new URL(value);
    if ((url.protocol !== "http:" && url.protocol !== "https:") || url.username || url.password) return "";
    return url.toString();
  } catch {
    return "";
  }
}

export function safeExternalURL(value: unknown): string {
  return safeImageURL(value);
}

export function mediaPoster(item: MediaItem | MediaRequest): string {
  if ("media_info" in item) {
    const info = item.media_info || {};
    return safeImageURL(info["poster_url"] || info["poster"]);
  }
  const media = item as MediaItem;
  return safeImageURL(media.poster_url || media.poster);
}

export function mediaTitle(item: MediaItem | MediaRequest): string {
  if ("media_info" in item) {
    const info = item.media_info || {};
    const value = info["title"];
    return typeof value === "string" && value.trim() ? value : item.title || "";
  }
  return item.title || "";
}

export function mediaStatusLabel(status: string): string {
  switch (status.toLowerCase()) {
    case "unhandled":
    case "pending": return "待处理";
    case "accepted": return "已接受";
    case "downloading": return "正在下载";
    case "rejected": return "已拒绝";
    case "completed": return "已完成";
    default: return status;
  }
}

export function mediaID(item: MediaItem | MediaDetail): string {
  return `${item.source}:${item.id}:${item.media_type}`;
}
