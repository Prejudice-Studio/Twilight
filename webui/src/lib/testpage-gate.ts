/**
 * testpage-gate 用于把 /testweb /testwebuser /testwebadmin 三个 demo 页
 * 在生产构建中强制屏蔽，避免示例页污染线上路由。
 *
 * 启用条件（任一即可）：
 *   - NODE_ENV !== "production"（本地开发自动开放）
 *   - NEXT_PUBLIC_ENABLE_TESTPAGES === "1" / "true" / "yes"
 *
 * 使用：
 *   import { ensureTestPageEnabled } from "@/lib/testpage-gate";
 *   ...
 *   ensureTestPageEnabled();   // 早期调用，未开启时抛 notFound()
 *
 *
 */
import { notFound } from "next/navigation";

const ENABLED_VALUES = new Set(["1", "true", "yes", "on"]);

export function isTestPageEnabled(): boolean {
  if (process.env.NODE_ENV !== "production") {
    return true;
  }
  const flag = (process.env.NEXT_PUBLIC_ENABLE_TESTPAGES ?? "").trim().toLowerCase();
  return ENABLED_VALUES.has(flag);
}

export function ensureTestPageEnabled(): void {
  if (!isTestPageEnabled()) {
    notFound();
  }
}
