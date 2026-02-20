"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { motion } from "framer-motion";
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
  const { setTheme, themes, theme: currentTheme } = useTheme();
  const isAdmin = user?.role === 0;

  const toggleTheme = (event: React.MouseEvent) => {
    const x = event.clientX;
    const y = event.clientY;

    const nextTheme = currentTheme === "light" ? "bloom" : "light";

    if (!(document as any).startViewTransition) {
      setTheme(nextTheme);
      return;
    }

    const transition = (document as any).startViewTransition(async () => {
      setTheme(nextTheme);
    });

    transition.ready.then(() => {
      const radius = Math.hypot(
        Math.max(x, window.innerWidth - x),
        Math.max(y, window.innerHeight - y)
      );

      document.documentElement.animate(
        {
          clipPath: [
            `circle(0px at ${x}px ${y}px)`,
            `circle(${radius}px at ${x}px ${y}px)`,
          ],
        },
        {
          duration: 500,
          easing: "ease-in-out",
          pseudoElement: "::view-transition-new(root)",
        }
      );
    });
  };

  return (
    <aside className="fixed left-0 top-0 z-40 h-screen w-64 border-r border-white/40 glass-acrylic transition-all duration-500">
      <div className="flex h-full flex-col">
        {/* Logo */}
        <div className="flex h-20 items-center gap-3 border-b border-white/20 px-6">
          <div className="group relative flex h-10 w-10 items-center justify-center rounded-2xl bg-primary shadow-lg shadow-primary/20 transition-all hover:scale-110 active:scale-95">
            <Sparkles className="h-6 w-6 text-primary-foreground" />
          </div>
          <div className="flex flex-col">
            <span className="text-xl font-black tracking-tight text-foreground bg-clip-text text-transparent bg-gradient-to-br from-foreground to-foreground/70">Twilight</span>
            <span className="text-[9px] font-bold text-primary/60 uppercase tracking-widest leading-none">Studio</span>
          </div>
        </div>

        {/* User Nav */}
        <nav className="flex-1 space-y-1.5 px-3 py-6 overflow-y-auto custom-scrollbar">
          <div className="mb-4 px-3">
            <p className="text-[10px] font-black uppercase tracking-[0.3em] text-muted-foreground/60">
              Menu
            </p>
          </div>
          {userNavItems.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "group relative flex items-center gap-3 rounded-2xl px-3 py-2.5 text-sm font-bold transition-all duration-500",
                pathname === item.href
                  ? "glass-frosted text-primary shadow-lg shadow-primary/5"
                  : "text-muted-foreground/80 hover:bg-white/40 hover:text-foreground"
              )}
            >
              <div className={cn(
                "flex h-8 w-8 items-center justify-center rounded-xl transition-all duration-500",
                pathname === item.href ? "bg-primary text-primary-foreground shadow-md shadow-primary/20" : "bg-white/50 text-muted-foreground group-hover:bg-primary group-hover:text-primary-foreground group-hover:shadow-md"
              )}>
                <item.icon className="h-4 w-4" />
              </div>
              {item.label}
              {pathname === item.href && (
                <motion.div 
                  layoutId="sidebar-active"
                  className="absolute left-0 h-6 w-1 rounded-r-full bg-primary" 
                />
              )}
            </Link>
          ))}

          {isAdmin && (
            <>
              <div className="mt-10 mb-4 px-3">
                <p className="text-[10px] font-black uppercase tracking-[0.3em] text-muted-foreground/60">
                  Admin Control
                </p>
              </div>
              {adminNavItems.map((item) => (
                <Link
                  key={item.href}
                  href={item.href}
                  className={cn(
                    "group relative flex items-center gap-3 rounded-2xl px-3 py-2.5 text-sm font-bold transition-all duration-500",
                    pathname.startsWith(item.href)
                      ? "glass-frosted text-primary shadow-lg shadow-primary/5"
                      : "text-muted-foreground/80 hover:bg-white/40 hover:text-foreground"
                  )}
                >
                  <div className={cn(
                    "flex h-8 w-8 items-center justify-center rounded-xl transition-all duration-500",
                    pathname.startsWith(item.href) ? "bg-primary text-primary-foreground shadow-md shadow-primary/20" : "bg-white/50 text-muted-foreground group-hover:bg-primary group-hover:text-primary-foreground group-hover:shadow-md"
                  )}>
                    <item.icon className="h-4 w-4" />
                  </div>
                  {item.label}
                  {pathname.startsWith(item.href) && (
                    <motion.div 
                      layoutId="sidebar-active-admin"
                      className="absolute left-0 h-6 w-1 rounded-r-full bg-primary" 
                    />
                  )}
                </Link>
              ))}
            </>
          )}
        </nav>

        {/* Action Buttons */}
        <div className="mt-auto p-4 space-y-3">
          <div className="flex items-center gap-3 rounded-[1.5rem] border border-white/50 bg-white/40 p-3 shadow-inner">
            <Avatar className="h-10 w-10 border-2 border-white shadow-sm">
              <AvatarFallback className="bg-primary text-primary-foreground text-xs font-black">
                {user?.username?.slice(0, 2).toUpperCase() || "U"}
              </AvatarFallback>
            </Avatar>
            <div className="flex-1 truncate">
              <p className="text-sm font-black text-foreground truncate">{user?.username}</p>
              <p className="text-[9px] text-primary/70 uppercase font-black tracking-widest leading-none mt-0.5">{user?.role_name}</p>
            </div>
          </div>

          <div className="flex gap-2">
            <Button
              variant="outline"
              size="icon"
              onClick={toggleTheme}
              className="flex-1 h-11 rounded-2xl bg-white/60 hover:bg-white border-white/40 shadow-sm"
            >
              <Sun className={cn("h-4 w-4 transition-all", currentTheme === "bloom" ? "text-orange-500" : "text-amber-500")} />
            </Button>
            <Button
              variant="outline"
              size="icon"
              onClick={logout}
              className="flex-1 h-11 rounded-2xl bg-white/60 hover:bg-red-50 hover:text-red-500 border-white/40 shadow-sm"
            >
              <LogOut className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>
    </aside>
  );
}

