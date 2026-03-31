"use client";

import { useAuthStore } from "@/store/auth";
import { Badge } from "@/components/ui/badge";
import { Sparkles } from "lucide-react";

export function Header() {
  const { user } = useAuthStore();

  return (
    <header className="sticky top-0 z-30 px-4 pt-4 md:px-6 md:pt-6 xl:px-8">
      <div className="header-surface">
        <div className="flex min-w-0 items-center gap-4">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl bg-primary/15 text-primary">
            <Sparkles className="h-5 w-5" />
          </div>
          <div className="min-w-0">
            <p className="text-xs uppercase tracking-[0.16em] text-muted-foreground">DashBoard</p>
            <h1 className="truncate text-base font-semibold md:text-lg">
              欢迎回来，{user?.username}
            </h1>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <Badge variant="outline" className="hidden md:inline-flex">
            {user?.role_name}
          </Badge>
        </div>
      </div>
    </header>
  );
}

