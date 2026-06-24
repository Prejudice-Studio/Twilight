"use client";

import { useEffect } from "react";
import { AlertTriangle, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";

/**
 * Next.js App Router 路由级错误边界。
 * 仅处理"逃出 ErrorBoundary 的渲染期错误"以及 server component 抛错——
 * 例如 layout/page 里 throw new Error 时 RootLayout 仍然渲染，避免白屏到 Next.js dev overlay。
 *
 * 注意：global-error.tsx 才能截 RootLayout 自身的错误，那里不能依赖 ThemeProvider 等
 * client provider；因此留一份内联的最小样式。
 */
export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    if (process.env.NODE_ENV !== "production") {
      // eslint-disable-next-line no-console
      console.error("[app/error]", error);
    }
  }, [error]);

  return (
    <div
      role="alert"
      aria-live="assertive"
      className="flex min-h-screen items-center justify-center p-4"
    >
      <div className="w-full max-w-md rounded-lg border border-destructive/40 bg-card p-6 shadow-sm">
        <div className="mb-3 flex items-center gap-2 text-destructive">
          <AlertTriangle className="h-5 w-5" aria-hidden="true" />
          <h1 className="text-base font-semibold">页面崩溃了</h1>
        </div>
        <p className="mb-4 text-sm text-muted-foreground">
          抱歉，页面遇到未预期错误。可以先重试；如果反复失败，请刷新或回到首页。
        </p>
        {error.digest && (
          <p className="mb-3 text-xs text-muted-foreground">
            错误 ID：<code className="rounded bg-muted px-1 py-0.5">{error.digest}</code>
          </p>
        )}
        {process.env.NODE_ENV !== "production" && error.message && (
          <pre className="mb-4 max-h-40 overflow-auto rounded bg-muted px-3 py-2 text-xs text-muted-foreground">
            {error.message}
          </pre>
        )}
        <div className="flex flex-wrap gap-2">
          <Button size="sm" onClick={reset}>
            <RefreshCw className="mr-1.5 h-4 w-4" aria-hidden="true" />
            重试
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              if (typeof window !== "undefined") {
                window.location.href = "/dashboard";
              }
            }}
          >
            回到首页
          </Button>
        </div>
      </div>
    </div>
  );
}
