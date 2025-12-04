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
    <div className="flex min-h-screen items-center justify-center p-4">
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.5 }}
      >
        <Card className="w-full max-w-md glass-card border-white/10">
          <CardHeader className="space-y-1 text-center">
            <div className="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-gradient-to-br from-twilight-500 to-sunset-500 shadow-lg shadow-twilight-500/30">
              <Sparkles className="h-7 w-7 text-white" />
            </div>
            <CardTitle className="text-2xl font-bold gradient-text">
              创建账号
            </CardTitle>
            <CardDescription>
              注册新的 Twilight 账号
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="username">用户名 *</Label>
                <Input
                  id="username"
                  name="username"
                  type="text"
                  placeholder="请输入用户名"
                  value={formData.username}
                  onChange={handleChange}
                  className="bg-background/50"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="email">邮箱</Label>
                <Input
                  id="email"
                  name="email"
                  type="email"
                  placeholder="选填"
                  value={formData.email}
                  onChange={handleChange}
                  className="bg-background/50"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="password">密码 *</Label>
                <div className="relative">
                  <Input
                    id="password"
                    name="password"
                    type={showPassword ? "text" : "password"}
                    placeholder="至少 6 位"
                    value={formData.password}
                    onChange={handleChange}
                    className="bg-background/50 pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  >
                    {showPassword ? (
                      <EyeOff className="h-4 w-4" />
                    ) : (
                      <Eye className="h-4 w-4" />
                    )}
                  </button>
                </div>
              </div>

              <div className="space-y-2">
                <Label htmlFor="confirmPassword">确认密码 *</Label>
                <Input
                  id="confirmPassword"
                  name="confirmPassword"
                  type="password"
                  placeholder="再次输入密码"
                  value={formData.confirmPassword}
                  onChange={handleChange}
                  className="bg-background/50"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="regCode">注册码</Label>
                <Input
                  id="regCode"
                  name="regCode"
                  type="text"
                  placeholder="如有注册码请输入"
                  value={formData.regCode}
                  onChange={handleChange}
                  className="bg-background/50"
                />
              </div>

              <Button
                type="submit"
                variant="gradient"
                className="w-full"
                disabled={isLoading}
              >
                {isLoading ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <UserPlus className="mr-2 h-4 w-4" />
                )}
                注册
              </Button>
            </form>

            <div className="mt-6 text-center text-sm">
              <span className="text-muted-foreground">已有账号？</span>{" "}
              <Link
                href="/login"
                className="font-medium text-primary hover:underline"
              >
                立即登录
              </Link>
            </div>
          </CardContent>
        </Card>
      </motion.div>
    </div>
  );
}

