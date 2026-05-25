"use client";

import { useState } from "react";
import Link from "next/link";
import { Copy, KeyRound, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";
import { validateEmbyUsername } from "@/lib/validators";

export default function ForgotPasswordPage() {
  const { toast } = useToast();
  const [embyUsername, setEmbyUsername] = useState("");
  const [embyPassword, setEmbyPassword] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [result, setResult] = useState<{ username: string; new_password: string } | null>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const usernameCheck = validateEmbyUsername(embyUsername);
    if (!usernameCheck.ok) {
      toast({ title: usernameCheck.message, variant: "destructive" });
      return;
    }
    if (!embyPassword) {
      toast({ title: "请填写 Emby 密码", variant: "destructive" });
      return;
    }
    setIsLoading(true);
    setResult(null);
    try {
      const res = await api.forgotPasswordByEmby({ emby_username: embyUsername.trim(), emby_password: embyPassword });
      if (res.success && res.data) {
        setResult(res.data);
        setEmbyPassword("");
        toast({ title: "密码已重置", description: "新密码只显示一次，请立即保存", variant: "success" });
      } else {
        toast({ title: "找回失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "找回失败", description: error.message || "网络异常", variant: "destructive" });
    } finally {
      setIsLoading(false);
    }
  };

  const copyPassword = () => {
    if (!result?.new_password) return;
    navigator.clipboard.writeText(result.new_password);
    toast({ title: "已复制新密码" });
  };

  return (
    <main className="relative flex min-h-screen w-full items-center justify-center p-4">
      <Card className="w-full max-w-[460px] border-border/70 bg-card/78 shadow-2xl backdrop-blur-xl">
        <CardHeader className="space-y-2 text-center">
          <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/14 text-primary">
            <KeyRound className="h-7 w-7" />
          </div>
          <CardTitle className="text-2xl">找回 Web 登录密码</CardTitle>
          <CardDescription>验证你已绑定的 Emby 账号后，系统会生成一个新的 Web 登录密码。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <form onSubmit={submit} className="space-y-4">
            <div className="space-y-2">
              <Label>Emby 用户名</Label>
              <Input value={embyUsername} onChange={(e) => setEmbyUsername(e.target.value)} autoComplete="username" />
            </div>
            <div className="space-y-2">
              <Label>Emby 密码</Label>
              <Input type="password" value={embyPassword} onChange={(e) => setEmbyPassword(e.target.value)} autoComplete="current-password" />
            </div>
            <Button type="submit" className="w-full" disabled={isLoading}>
              {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              验证并重置密码
            </Button>
          </form>

          {result && (
            <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-4">
              <p className="text-sm font-semibold">Web 用户名：{result.username}</p>
              <p className="mt-2 text-xs text-muted-foreground">新密码只显示一次，请立即复制并登录后修改。</p>
              <div className="mt-3 flex items-center gap-2">
                <code className="min-w-0 flex-1 break-all rounded bg-background px-3 py-2 text-sm">{result.new_password}</code>
                <Button
                  type="button"
                  size="icon"
                  variant="outline"
                  onClick={copyPassword}
                  aria-label="复制新密码"
                >
                  <Copy className="h-4 w-4" aria-hidden="true" />
                </Button>
              </div>
            </div>
          )}

          <div className="text-center text-sm">
            <Link href="/login" className="font-medium text-primary hover:underline">返回登录</Link>
          </div>
        </CardContent>
      </Card>
    </main>
  );
}
