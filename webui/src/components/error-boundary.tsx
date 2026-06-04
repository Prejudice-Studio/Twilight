"use client";

import React, { Component, type ErrorInfo, type ReactNode } from "react";
import { AlertTriangle, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useI18n } from "@/lib/i18n";

/**
 * ErrorBoundary 给 React 渲染期抛错兜底。
 * 没有边界时一处子树抛错会把整棵 React 树打回白屏，用户只能 F5；
 * 有了边界，渲染异常被截在最近的边界内，其他区域仍可用，
 * 同时给一个明显的"重试 / 回首页"出口，避免用户看到 Next.js 默认 dev overlay
 * 或纯白屏。
 *
 * 仅捕获渲染期错误（render / lifecycle / hooks 的同步抛错）。
 * 异步 / 事件回调里的错误不会进这里，那条路径走 useAsyncHandler。
 */
interface ErrorBoundaryProps {
  children: ReactNode;
  /** 自定义降级 UI；不传则用内置卡片。 */
  fallback?: (error: Error, reset: () => void) => ReactNode;
  /** 出错时上报回调，比如埋点。 */
  onError?: (error: Error, info: ErrorInfo) => void;
}

interface ErrorBoundaryState {
  error: Error | null;
}

export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    // 控制台打印保留给开发；生产环境可由 onError 转发到日志后端。
    if (process.env.NODE_ENV !== "production") {
      // eslint-disable-next-line no-console
      console.error("[ErrorBoundary]", error, info.componentStack);
    }
    this.props.onError?.(error, info);
  }

  reset = (): void => {
    this.setState({ error: null });
  };

  render(): ReactNode {
    const { error } = this.state;
    if (!error) return this.props.children;

    if (this.props.fallback) {
      return this.props.fallback(error, this.reset);
    }

    return <DefaultErrorFallback error={error} reset={this.reset} />;
  }
}

function DefaultErrorFallback({ error, reset }: { error: Error; reset: () => void }) {
  const { t } = useI18n();

  return (
    <div
      role="alert"
      aria-live="assertive"
      className="flex min-h-[60vh] items-center justify-center p-4"
    >
      <div className="w-full max-w-md rounded-lg border border-destructive/40 bg-card p-6 shadow-sm">
        <div className="mb-3 flex items-center gap-2 text-destructive">
          <AlertTriangle className="h-5 w-5" aria-hidden="true" />
          <h2 className="text-base font-semibold">{t("errorBoundary.title")}</h2>
        </div>
        <p className="mb-4 text-sm text-muted-foreground">
          {t("errorBoundary.description")}
        </p>
        {process.env.NODE_ENV !== "production" && (
          <pre className="mb-4 max-h-40 overflow-auto rounded bg-muted px-3 py-2 text-xs text-muted-foreground">
            {error.message}
          </pre>
        )}
        <div className="flex flex-wrap gap-2">
          <Button size="sm" onClick={reset}>
            <RefreshCw className="mr-1.5 h-4 w-4" aria-hidden="true" />
            {t("common.retry")}
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
            {t("errorBoundary.home")}
          </Button>
        </div>
      </div>
    </div>
  );
}
