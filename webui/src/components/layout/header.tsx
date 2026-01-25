"use client";

import { useAuthStore } from "@/store/auth";
import { Badge } from "@/components/ui/badge";
import { formatRelativeTime } from "@/lib/utils";

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
      return <Badge variant="gradient">永久有效</Badge>;
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
    <header className="sticky top-0 z-30 h-16 border-b bg-background/80 backdrop-blur-xl">
      <div className="flex h-full items-center justify-between px-6">
        <div>
          <h1 className="text-lg font-semibold">
            欢迎回来, <span className="gradient-text">{user?.username}</span>
          </h1>
        </div>

        <div className="flex items-center gap-6">
          <div className="flex items-center gap-2 text-sm">
            <span className="text-muted-foreground font-medium">积分:</span>
            <span className="font-bold text-primary">{user?.score || 0}</span>
          </div>
        </div>
      </div>
    </header>
  );
}

