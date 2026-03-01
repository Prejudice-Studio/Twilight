"use client";

import { useAuthStore } from "@/store/auth";
import { Badge } from "@/components/ui/badge";
import { formatRelativeTime } from "@/lib/utils";
import { Sparkles } from "lucide-react";

export function Header() {
  const { user } = useAuthStore();

  // 判断账号状态
  const isUnregistered = user?.role === -1; // UNRECOGNIZED
  const isDisabled = user?.active === false;
  
  // 处理时间戳（秒转毫秒）
  const expiredTimeMs = typeof user?.expired_at === 'number' 
    ? (user.expired_at < 10000000000 ? user.expired_at * 1000 : user.expired_at)
    : (user?.expired_at ? new Date(user.expired_at).getTime() : -1);

  const isExpired = expiredTimeMs !== -1 && 
    expiredTimeMs !== 0 &&
    expiredTimeMs < Date.now();

  const isPermanent = !user?.expired_at || 
    user.expired_at === -1 || 
    user.expired_at === "-1";

  // 获取状态标签
  const getStatusBadge = () => {
    if (user?.role === 0) {
      return <Badge variant="default" className="bg-emerald-500/10 text-emerald-500 border-emerald-500/20">永久有效</Badge>;
    }
    if (isUnregistered) {
      return <Badge variant="secondary">未注册</Badge>;
    }
    if (isDisabled) {
      return <Badge variant="destructive">已禁用</Badge>;
    }
    if (isExpired) {
      return <Badge variant="destructive">已过期</Badge>;
    }
    if (isPermanent) {
      return <Badge variant="default" className="bg-emerald-500/10 text-emerald-500 border-emerald-500/20">永久有效</Badge>;
    }
    if (user?.expired_at) {
      return (
        <Badge variant="outline">
          {formatRelativeTime(user.expired_at)}
        </Badge>
      );
    }
    return null;
  };

  return (
    <header className="sticky top-0 z-30 px-4 pt-4 md:px-6 md:pt-6 xl:px-8">
      <div className="header-surface">
        <div className="flex min-w-0 items-center gap-4">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl bg-primary/15 text-primary">
            <Sparkles className="h-5 w-5" />
          </div>
          <div className="min-w-0">
            <p className="text-xs uppercase tracking-[0.16em] text-muted-foreground">Twilight Control</p>
            <h1 className="truncate text-base font-semibold md:text-lg">
              欢迎回来，{user?.username}
            </h1>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <Badge variant="outline" className="hidden md:inline-flex">
            {user?.role_name}
          </Badge>
          {getStatusBadge()}
        </div>
      </div>
    </header>
  );
}

