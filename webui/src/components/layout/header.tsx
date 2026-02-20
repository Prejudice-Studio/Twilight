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
    <header className="sticky top-0 z-30 h-20 border-b border-border bg-card/40 backdrop-blur-3xl px-8">
      <div className="flex h-full items-center justify-between">
        <div className="flex flex-col">
          <h1 className="text-xl font-bold tracking-tight text-foreground">
            你好, <span className="text-primary">{user?.username}</span>
          </h1>
          <div className="mt-1 flex items-center gap-2">
            {getStatusBadge()}
          </div>
        </div>

        <div className="flex items-center gap-4">
          <div className="flex items-center gap-3 rounded-2xl bg-muted border border-border px-4 py-2">
            <div className="flex flex-col items-end">
              <span className="text-[10px] font-black uppercase tracking-widest text-muted-foreground">Balance</span>
              <div className="flex items-center gap-1.5">
                <span className="text-sm font-black text-foreground">{user?.score || 0}</span>
                <span className="text-[10px] font-bold text-primary">PTS</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </header>
  );
}

