"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { motion } from "framer-motion";
import { Sparkles, Eye, EyeOff, UserPlus, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";

export default function RegisterPage() {
  const router = useRouter();
  const { toast } = useToast();
  
  const [formData, setFormData] = useState({
    username: "",
    password: "",
    confirmPassword: "",
    email: "",
    regCode: "",
  });
  const [showPassword, setShowPassword] = useState(false);
  const [isLoading, setIsLoading] = useState(false);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setFormData({ ...formData, [e.target.name]: e.target.value });
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!formData.username || !formData.password) {
      toast({
        title: "请填写完整信息",
        variant: "destructive",
      });
      return;
    }

    if (formData.password !== formData.confirmPassword) {
      toast({
        title: "密码不一致",
        description: "请确认两次输入的密码相同",
        variant: "destructive",
      });
      return;
    }

    if (formData.password.length < 6) {
      toast({
        title: "密码太短",
        description: "密码至少需要 6 位",
        variant: "destructive",
      });
      return;
    }

    setIsLoading(true);
    try {
      const res = await api.register({
        username: formData.username,
        password: formData.password,
        email: formData.email || undefined,
        reg_code: formData.regCode || undefined,
      });
      
      if (res.success) {
        toast({
          title: "注册成功",
          description: "请登录您的账号",
          variant: "success",
        });
        router.push("/login");
      } else {
        toast({
          title: "注册失败",
          description: res.message,
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: "注册失败",
        description: error.message || "请检查网络连接",
        variant: "destructive",
      });
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <main className="relative flex min-h-screen w-full items-center justify-center overflow-hidden bg-background p-4">
      <motion.div
        initial={{ opacity: 0, scale: 0.95, y: 20 }}
        animate={{ opacity: 1, scale: 1, y: 0 }}
        transition={{ duration: 0.8, ease: "easeOut" }}
        className="relative z-10 w-full max-w-[480px]"
      >
        <Card className="overflow-hidden border-border bg-card/50 shadow-2xl backdrop-blur-3xl">
          <CardHeader className="space-y-2 pb-6 pt-10 text-center">
            <motion.div 
              initial={{ rotate: 10, scale: 0.8 }}
              animate={{ rotate: 0, scale: 1 }}
              transition={{ 
                type: "spring",
                stiffness: 260,
                damping: 20,
                delay: 0.2
              }}
              className="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-primary shadow-lg shadow-primary/20"
            >
              <Sparkles className="h-7 w-7 text-primary-foreground" />
            </motion.div>
            
            <CardTitle className="text-3xl font-black tracking-tight text-foreground">
              加入 Twilight
            </CardTitle>
            <CardDescription className="text-muted-foreground">
              开启全新的私人影音管理体验
            </CardDescription>
          </CardHeader>
          
          <CardContent className="px-8 pb-10">
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="username" className="text-foreground/70 ml-1">用户名 *</Label>
                  <Input
                    id="username"
                    name="username"
                    placeholder="Username"
                    value={formData.username}
                    onChange={handleChange}
                    className="h-11 border-border bg-muted/50 text-foreground placeholder:text-muted-foreground/50 focus:ring-primary/20"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="email" className="text-foreground/70 ml-1">邮箱</Label>
                  <Input
                    id="email"
                    name="email"
                    type="email"
                    placeholder="Email (Optional)"
                    value={formData.email}
                    onChange={handleChange}
                    className="h-11 border-border bg-muted/50 text-foreground placeholder:text-muted-foreground/50 focus:ring-primary/20"
                  />
                </div>
              </div>
              
              <div className="space-y-2">
                <Label htmlFor="password" className="text-foreground/70 ml-1">设置密码 *</Label>
                <div className="relative">
                  <Input
                    id="password"
                    name="password"
                    type={showPassword ? "text" : "password"}
                    placeholder="Password (Min 6 chars)"
                    value={formData.password}
                    onChange={handleChange}
                    className="h-11 border-border bg-muted/50 text-foreground placeholder:text-muted-foreground/50 focus:ring-primary/20 pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground/60 hover:text-foreground transition-colors"
                  >
                    {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
              </div>

              <div className="space-y-2">
                <Label htmlFor="confirmPassword" className="text-foreground/70 ml-1">确认密码 *</Label>
                <Input
                  id="confirmPassword"
                  name="confirmPassword"
                  type="password"
                  placeholder="Confirm Password"
                  value={formData.confirmPassword}
                  onChange={handleChange}
                  className="h-11 border-border bg-muted/50 text-foreground placeholder:text-muted-foreground/50 focus:ring-primary/20"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="regCode" className="text-foreground/70 ml-1 text-xs">注册码 / 邀请码</Label>
                <Input
                  id="regCode"
                  name="regCode"
                  placeholder="Registration Code"
                  value={formData.regCode}
                  onChange={handleChange}
                  className="h-11 border-border bg-muted/50 text-foreground placeholder:text-muted-foreground/50 focus:ring-primary/20"
                />
              </div>

              <div className="pt-4">
                <Button
                  type="submit"
                  className="h-12 w-full bg-primary font-bold text-primary-foreground shadow-lg shadow-primary/20 hover:bg-primary/90 transition-all active:scale-[0.98]"
                  disabled={isLoading}
                >
                  {isLoading ? (
                    <Loader2 className="mr-2 h-5 w-5 animate-spin" />
                  ) : (
                    <UserPlus className="mr-2 h-5 w-5" />
                  )}
                  开启旅程
                </Button>
              </div>
            </form>

            <div className="mt-8 flex items-center justify-center gap-2 text-sm">
              <span className="text-muted-foreground">已有账号？</span>
              <Link
                href="/login"
                className="font-bold text-primary hover:underline transition-colors"
              >
                立即登录
              </Link>
            </div>
          </CardContent>
        </Card>
      </motion.div>
    </main>
  );
}

