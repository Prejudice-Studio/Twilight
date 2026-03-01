"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { motion } from "framer-motion";
import { Eye, EyeOff, ArrowRight, Loader2, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useToast } from "@/hooks/use-toast";
import { useAuthStore } from "@/store/auth";

export default function LoginPage() {
  const router = useRouter();
  const { toast } = useToast();
  const { login } = useAuthStore();
  
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    router.prefetch("/dashboard");
  }, [router]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!username || !password) {
      toast({
        title: "请填写完整信息",
        variant: "destructive",
      });
      return;
    }

    setIsLoading(true);
    try {
      const success = await login(username, password);
      if (success) {
        toast({
          title: "登录成功",
          description: "欢迎回来！",
          variant: "success",
        });
        router.replace("/dashboard");
      } else {
        toast({
          title: "登录失败",
          description: "用户名或密码错误",
          variant: "destructive",
        });
      }
    } catch (error) {
      toast({
        title: "登录失败",
        description: "请检查网络连接",
        variant: "destructive",
      });
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <main className="relative flex min-h-screen w-full items-center justify-center p-4">
      <motion.div
        initial={{ opacity: 0, scale: 0.95, y: 20 }}
        animate={{ opacity: 1, scale: 1, y: 0 }}
        transition={{ duration: 0.35, ease: "easeOut" }}
        className="relative z-10 w-full max-w-[440px]"
      >
        <Card className="border-border/70 bg-card/78 shadow-2xl backdrop-blur-xl">
          <CardHeader className="space-y-2 pb-6 pt-8 text-center">
            <div className="mx-auto mb-3 flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/14 text-primary">
              <ShieldCheck className="h-7 w-7" />
            </div>

            <CardTitle className="text-2xl font-semibold tracking-tight">
              登录 Twilight
            </CardTitle>
            <CardDescription className="text-sm">
              访问你的媒体控制台
            </CardDescription>
          </CardHeader>

          <CardContent className="px-6 pb-7 md:px-8">
            <form onSubmit={handleSubmit} className="space-y-5">
              <div className="space-y-2">
                <Label htmlFor="username" className="ml-1">用户名</Label>
                <Input
                  id="username"
                  placeholder="Username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  className="h-11"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="password" className="ml-1">密码</Label>
                <div className="relative">
                  <Input
                    id="password"
                    type={showPassword ? "text" : "password"}
                    placeholder="Password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className="h-11 pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  >
                    {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
              </div>

              <div className="pt-2">
                <Button
                  type="submit"
                  className="h-11 w-full"
                  disabled={isLoading}
                >
                  {isLoading ? (
                    <Loader2 className="mr-2 h-5 w-5 animate-spin" />
                  ) : (
                    <ArrowRight className="mr-2 h-5 w-5" />
                  )}
                  立即登入
                </Button>
              </div>
            </form>
 
            <div className="mt-8 flex items-center justify-center gap-2 text-sm">
              <span className="text-muted-foreground">还没有账号？</span>
              <Link
                href="/register"
                className="font-medium text-primary hover:underline"
              >
                创建新账户
              </Link>
            </div>
          </CardContent>
        </Card>
      </motion.div>
    </main>
  );
}

