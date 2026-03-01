"use client";

import { Loader2, RefreshCw, AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";

export function PageLoading({ message = "加载中..." }: { message?: string }) {
  return (
    <div className="flex min-h-[260px] items-center justify-center">
      <div className="flex items-center gap-3 rounded-xl border border-border/70 bg-card/70 px-4 py-3 text-muted-foreground">
        <Loader2 className="h-5 w-5 animate-spin" />
        <span>{message}</span>
      </div>
    </div>
  );
}

export function PageError({
  message,
  onRetry,
}: {
  message: string;
  onRetry?: () => void;
}) {
  return (
    <div className="flex min-h-[260px] items-center justify-center">
      <div className="max-w-md rounded-xl border border-destructive/35 bg-card/78 p-6 text-center shadow-sm">
        <AlertTriangle className="h-8 w-8 mx-auto text-destructive mb-3" />
        <p className="text-sm text-foreground mb-4">{message}</p>
        {onRetry && (
          <Button variant="outline" onClick={onRetry}>
            <RefreshCw className="h-4 w-4 mr-2" />
            重试
          </Button>
        )}
      </div>
    </div>
  );
}
