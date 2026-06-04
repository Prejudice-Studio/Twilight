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
import { useI18n } from "@/lib/i18n";

export default function ForgotPasswordPage() {
  const { toast } = useToast();
  const { t } = useI18n();
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
      toast({ title: t("auth.forgotPassword.embyPasswordRequired"), variant: "destructive" });
      return;
    }
    setIsLoading(true);
    setResult(null);
    try {
      const res = await api.forgotPasswordByEmby({ emby_username: embyUsername.trim(), emby_password: embyPassword });
      if (res.success && res.data) {
        setResult(res.data);
        setEmbyPassword("");
        toast({ title: t("auth.forgotPassword.resetSuccess"), description: t("auth.forgotPassword.oneTimePassword"), variant: "success" });
      } else {
        toast({ title: t("auth.forgotPassword.failed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("auth.forgotPassword.failed"), description: error.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setIsLoading(false);
    }
  };

  const copyPassword = () => {
    if (!result?.new_password) return;
    navigator.clipboard.writeText(result.new_password);
    toast({ title: t("auth.forgotPassword.copied") });
  };

  return (
    <main className="relative flex min-h-screen w-full items-center justify-center p-4">
      <Card className="w-full max-w-[460px] border-border/70 bg-card/78 shadow-2xl backdrop-blur-xl">
        <CardHeader className="space-y-2 text-center">
          <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/14 text-primary">
            <KeyRound className="h-7 w-7" />
          </div>
          <CardTitle className="text-2xl">{t("auth.forgotPassword.title")}</CardTitle>
          <CardDescription>{t("auth.forgotPassword.description")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <form onSubmit={submit} className="space-y-4">
            <div className="space-y-2">
              <Label>{t("auth.forgotPassword.embyUsername")}</Label>
              <Input value={embyUsername} onChange={(e) => setEmbyUsername(e.target.value)} autoComplete="username" />
            </div>
            <div className="space-y-2">
              <Label>{t("auth.forgotPassword.embyPassword")}</Label>
              <Input type="password" value={embyPassword} onChange={(e) => setEmbyPassword(e.target.value)} autoComplete="current-password" />
            </div>
            <Button type="submit" className="w-full" disabled={isLoading}>
              {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t("auth.forgotPassword.submit")}
            </Button>
          </form>

          {result && (
            <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-4">
              <p className="text-sm font-semibold">{t("auth.forgotPassword.webUsername", { username: result.username })}</p>
              <p className="mt-2 text-xs text-muted-foreground">{t("auth.forgotPassword.copyHint")}</p>
              <div className="mt-3 flex items-center gap-2">
                <code className="min-w-0 flex-1 break-all rounded bg-background px-3 py-2 text-sm">{result.new_password}</code>
                <Button
                  type="button"
                  size="icon"
                  variant="outline"
                  onClick={copyPassword}
                  aria-label={t("auth.forgotPassword.copyPassword")}
                >
                  <Copy className="h-4 w-4" aria-hidden="true" />
                </Button>
              </div>
            </div>
          )}

          <div className="text-center text-sm">
            <Link href="/login" className="font-medium text-primary hover:underline">{t("auth.forgotPassword.backToLogin")}</Link>
          </div>
        </CardContent>
      </Card>
    </main>
  );
}
