"use client";

import Image from "next/image";
import { cn } from "@/lib/utils";
import { useSystemStore } from "@/store/system";
import { SITE_NAME } from "@/lib/site-config";
import { sanitizeImageUrl } from "@/lib/safe-url";
import { API_BASE } from "@/lib/api-request";

// 认证页主操作按钮统一风格：中性深/浅底（非品牌紫），与重写后的右侧面板
// 视觉一致。次要操作仍用 <Button variant="outline" /> 等既有变体。
export const AUTH_PRIMARY_BTN =
  "h-11 w-full bg-foreground text-background hover:bg-foreground/90 disabled:opacity-50";

export const AUTH_GHOST_LINK =
  "font-medium text-foreground/80 underline-offset-4 hover:text-foreground hover:underline";

function serverIconUrl(icon?: string | null): string | undefined {
  if (!icon) return undefined;
  if (icon.startsWith("http")) return sanitizeImageUrl(icon);
  if (icon.startsWith("/")) return sanitizeImageUrl(`${API_BASE}/api/v1${icon}`);
  return sanitizeImageUrl(icon);
}

// AuthBrand 渲染站点图标 + 名称，作为右侧面板顶部的统一品牌头。
export function AuthBrand({ subtitle }: { subtitle?: string }) {
  const { info } = useSystemStore();
  const name = info?.name || SITE_NAME;
  const icon = serverIconUrl(info?.server_icon);

  return (
    <div className="flex flex-col items-center gap-3 text-center">
      {icon ? (
        <div className="relative h-14 w-14 overflow-hidden rounded-2xl bg-muted">
          <Image
            src={icon}
            alt={name}
            fill
            className="object-cover"
            onError={(e) => {
              e.currentTarget.style.display = "none";
            }}
          />
        </div>
      ) : (
        <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-foreground text-lg font-bold text-background">
          {name.slice(0, 2).toUpperCase()}
        </div>
      )}
      <div className="space-y-1">
        <h1 className="text-xl font-semibold tracking-tight">{name}</h1>
        {subtitle ? <p className="text-sm text-foreground/70">{subtitle}</p> : null}
      </div>
    </div>
  );
}

// AuthPanel 是右侧固定面板的外壳：桌面端贴右、移动端全宽，内部滚动。
// children 由各页面提供具体表单内容。
export function AuthPanel({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <main className="relative flex min-h-dvh w-full">
      <section className={cn("auth-panel animate-auth-enter", className)}>
        <div className="auth-panel-inner space-y-7">{children}</div>
      </section>
    </main>
  );
}

// AuthStepDots 渲染注册向导的步骤进度点。
export function AuthStepDots({ total, current }: { total: number; current: number }) {
  return (
    <div className="flex items-center justify-center gap-1.5" aria-hidden="true">
      {Array.from({ length: total }).map((_, i) => (
        <span
          key={i}
          className={cn(
            "auth-step-dot",
            i === current && "auth-step-dot-active",
            i < current && "auth-step-dot-done",
          )}
        />
      ))}
    </div>
  );
}
