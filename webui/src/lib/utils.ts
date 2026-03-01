import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatDate(date: string | Date | number | null | undefined): string {
  // 处理无效值
  if (date == null || date === undefined || date === -1 || date === "-1") {
    return "永久";
  }
  
  // 处理时间戳（秒或毫秒）
  let d: Date;
  if (typeof date === 'number') {
    // 检查是否为有效数字
    if (!isFinite(date) || isNaN(date)) {
      return "无效日期";
    }
    // 如果是秒级时间戳（小于 10 位），转换为毫秒
    d = new Date(date < 10000000000 ? date * 1000 : date);
  } else {
    d = new Date(date);
  }
  
  // 检查日期是否有效
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

export function formatRelativeTime(date: string | Date | number): string {
  if (!date || date === -1 || date === "-1") return "永久";
  
  const now = new Date();
  let target: Date;
  
  if (typeof date === 'number') {
    // 如果是秒级时间戳（小于 10 位），转换为毫秒
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
  if (days > 365 * 100) return "永久";  // 超过 100 年视为永久
  return `${Math.ceil(days / 365)} 年后到期`;
}

