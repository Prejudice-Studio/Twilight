import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/**
 * 格式化日期/时间戳为中文日期字符串。
 * 支持 Unix 秒级/毫秒级时间戳、Date 对象、ISO 字符串。
 * 特殊值: -1 = 永久, 0 = 未开通
 */
export function formatDate(date: string | Date | number | null | undefined): string {
  if (date == null || date === undefined || date === -1 || date === "-1") {
    return "永久";
  }
  // 0 是 "未开通 Emby" 的 sentinel，避免被格式化成 1970 年
  if (date === 0 || date === "0") {
    return "未开通";
  }

  let d: Date;
  if (typeof date === 'number') {
    if (!isFinite(date) || isNaN(date)) {
      return "无效日期";
    }
    // 秒级时间戳 (< 10^10) 转换为毫秒
    d = new Date(date < 10000000000 ? date * 1000 : date);
  } else {
    d = new Date(date);
  }

  if (isNaN(d.getTime())) {
    return "无效日期";
  }

  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(d);
}

export function formatNumber(num: number): string {
  return new Intl.NumberFormat("zh-CN").format(num);
}

/**
 * 将到期时间格式化为相对时间描述（如 "3 天后到期"）。
 * 特殊值: -1 = 永久, 0 = 未开通, >100年 = 永久
 */
export function formatRelativeTime(date: string | Date | number): string {
  if (!date || date === -1 || date === "-1") return "永久";
  if (date === 0 || date === "0") return "未开通";

  const now = new Date();
  let target: Date;

  if (typeof date === 'number') {
    target = new Date(date < 10000000000 ? date * 1000 : date);
  } else {
    target = new Date(date);
  }

  const diff = target.getTime() - now.getTime();
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24));

  if (days < 0) return `已过期 ${Math.abs(days)} 天`;
  if (days === 0) return "今天到期";
  if (days === 1) return "明天到期";
  if (days <= 7) return `${days} 天后到期`;
  if (days <= 30) return `${Math.ceil(days / 7)} 周后到期`;
  if (days <= 365) return `${Math.ceil(days / 30)} 个月后到期`;
  if (days > 365 * 100) return "永久";
  return `${Math.ceil(days / 365)} 年后到期`;
}

