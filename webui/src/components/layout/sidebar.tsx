"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/store/auth";
import {
  LayoutDashboard,
  Film,
  Coins,
  Settings,
  Users,
  FileText,
  Shield,
  BarChart3,
  LogOut,
  Moon,
  Sun,
  Sparkles,
  TestTube,
  FileCode,
  Library,
} from "lucide-react";
import { useTheme } from "next-themes";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Separator } from "@/components/ui/separator";

const userNavItems = [
  { href: "/dashboard", label: "仪表盘", icon: LayoutDashboard },
  { href: "/media", label: "媒体搜索", icon: Film },
  { href: "/score", label: "积分中心", icon: Coins },
  { href: "/settings", label: "个人设置", icon: Settings },
];

const adminNavItems = [
  { href: "/admin/users", label: "用户管理", icon: Users },
  { href: "/admin/regcodes", label: "注册码", icon: FileText },
  { href: "/admin/requests", label: "求片审核", icon: Film },
  { href: "/admin/config", label: "配置管理", icon: FileCode },
  { href: "/admin/nsfw", label: "NSFW 库管理", icon: Library },
  { href: "/admin/test", label: "API 测试", icon: TestTube },
];

export function Sidebar() {
  const pathname = usePathname();
  const { user, logout } = useAuthStore();
  const { theme, setTheme } = useTheme();
  const isAdmin = user?.role === 0;

  return (
    <aside className="fixed left-0 top-0 z-40 h-screen w-64 border-r bg-card/40 backdrop-blur-2xl transition-all duration-300">
      <div className="flex h-full flex-col">
        {/* Logo */}
        <div className="flex h-16 items-center gap-2 border-b px-6">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-gradient-to-br from-twilight-500 to-sunset-500 shadow-lg shadow-primary/20 animate-pulse-slow">
            <Sparkles className="h-5 w-5 text-white" />
          </div>
          <span className="text-xl font-bold tracking-tighter gradient-text">Twilight</span>
        </div>

        {/* User Nav */}
        <nav className="flex-1 space-y-1.5 px-4 py-6 overflow-y-auto custom-scrollbar">
          <div className="mb-4 px-3">
            <p className="text-[10px] font-bold uppercase tracking-[0.2em] text-muted-foreground/60">
              User Menu
            </p>
          </div>
          {userNavItems.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "group relative flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-all duration-200 overflow-hidden",
                pathname === item.href
                  ? "bg-primary/10 text-primary shadow-[inset_0_0_20px_rgba(var(--primary-rgb),0.05)]"
                  : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
              )}
            >
              <item.icon className={cn(
                "h-4 w-4 transition-transform group-hover:scale-110",
                pathname === item.href ? "text-primary" : "text-muted-foreground"
              )} />
              {item.label}
              {pathname === item.href && (
                <div className="absolute right-0 h-8 w-1 rounded-l-full bg-primary" />
              )}
            </Link>
          ))}

          {isAdmin && (
            <>
              <div className="mt-8 mb-4 px-3">
                <Separator className="mb-4 opacity-50" />
                <p className="text-[10px] font-bold uppercase tracking-[0.2em] text-muted-foreground/60">
                  Management
                </p>
              </div>
              {adminNavItems.map((item) => (
                <Link
                  key={item.href}
                  href={item.href}
                  className={cn(
                    "group relative flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-all duration-200",
                    pathname.startsWith(item.href)
                      ? "bg-primary/10 text-primary"
                      : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
                  )}
                >
                  <item.icon className="h-4 w-4 transition-transform group-hover:rotate-12" />
                  {item.label}
                  {pathname.startsWith(item.href) && (
                    <div className="absolute right-0 h-8 w-1 rounded-l-full bg-primary" />
                  )}
                </Link>
              ))}
            </>
          )}
        </nav>

        {/* User Profile & Actions */}
        <div className="border-t bg-accent/20 p-4">
          <div className="flex items-center gap-3 rounded-xl bg-card border border-border/50 p-3 shadow-sm">
            <Avatar className="h-9 w-9 border border-primary/20">
              <AvatarFallback className="bg-primary/5 text-primary text-xs font-bold">
                {user?.username?.slice(0, 2).toUpperCase() || "U"}
              </AvatarFallback>
            </Avatar>
            <div className="flex-1 truncate">
              <p className="text-sm font-bold text-foreground truncate">{user?.username}</p>
              <p className="text-[10px] text-muted-foreground uppercase font-semibold">{user?.role_name}</p>
            </div>
          </div>

          <div className="mt-4 flex gap-2">
            <Button
              variant="secondary"
              size="sm"
              onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
              className="flex-1 h-9 rounded-lg glass shadow-sm"
            >
              <Sun className="h-4 w-4 rotate-0 scale-100 transition-all dark:-rotate-90 dark:scale-0" />
              <Moon className="absolute h-4 w-4 rotate-90 scale-0 transition-all dark:rotate-0 dark:scale-100" />
            </Button>
            <Button
              variant="secondary"
              size="sm"
              onClick={logout}
              className="flex-1 h-9 rounded-lg glass shadow-sm hover:text-destructive hover:bg-destructive/10"
            >
              <LogOut className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>
    </aside>
  );
}

