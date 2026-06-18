"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Copy, KeyRound, Loader2, Mail } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";
import { validateEmbyUsername, validateEmailOptional, friendlyError, isThrottleErrorCode, throttleCooldownSeconds } from "@/lib/validators";
import { validatePasswordStrength } from "@/lib/password";
import { useI18n } from "@/lib/i18n";
import { useSystemStore } from "@/store/system";

export default function ForgotPasswordPage() {
  const { toast } = useToast();
  const { t } = useI18n();
  const { info: systemInfo, fetchInfo } = useSystemStore();
  const features = systemInfo?.features;
  const forgotPasswordEnabled = Boolean(features?.forgot_password_enabled);
  const forgotPasswordEmbyEnabled = Boolean(features?.forgot_password_emby_enabled);
  const forgotPasswordEmailEnabled = Boolean(features?.forgot_password_email_enabled);
  const emailEnabled = Boolean(features?.email_enabled) && forgotPasswordEmailEnabled;
  const embyAvailable = forgotPasswordEmbyEnabled;
  const emailAvailable = emailEnabled;

  // Emby 重置（保留原有流程）
  const [embyUsername, setEmbyUsername] = useState("");
  const [embyPassword, setEmbyPassword] = useState("");
  const [embyLoading, setEmbyLoading] = useState(false);
  const [embyResult, setEmbyResult] = useState<{ username: string; new_password: string } | null>(null);

  // 邮箱验证码重置
  const [emailStage, setEmailStage] = useState<"request" | "reset">("request");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [emailLoading, setEmailLoading] = useState(false);
  const [cooldown, setCooldown] = useState(0);
  const [emailDone, setEmailDone] = useState(false);

  useEffect(() => {
    void fetchInfo();
  }, [fetchInfo]);

  // 重发冷却倒计时
  useEffect(() => {
    if (cooldown <= 0) return;
    const timer = setInterval(() => setCooldown((s) => (s > 0 ? s - 1 : 0)), 1000);
    return () => clearInterval(timer);
  }, [cooldown]);

  const submitEmby = async (e: React.FormEvent) => {
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
    setEmbyLoading(true);
    setEmbyResult(null);
    try {
      const res = await api.forgotPasswordByEmby({ emby_username: embyUsername.trim(), emby_password: embyPassword });
      if (res.success && res.data) {
        setEmbyResult(res.data);
        setEmbyPassword("");
        toast({ title: t("auth.forgotPassword.resetSuccess"), description: t("auth.forgotPassword.oneTimePassword"), variant: "success" });
      } else {
        toast({ title: t("auth.forgotPassword.failed"), description: friendlyError(res.error_code, res.message), variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("auth.forgotPassword.failed"), description: error?.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setEmbyLoading(false);
    }
  };

  const requestEmailCode = async (e?: React.FormEvent) => {
    e?.preventDefault();
    const emailCheck = validateEmailOptional(email);
    if (!email || !emailCheck.ok) {
      toast({ title: emailCheck.message || t("email.enterEmailFirst"), variant: "destructive" });
      return;
    }
    setEmailLoading(true);
    try {
      const res = await api.requestPasswordResetEmail(email.trim());
      if (res.success) {
        setEmailStage("reset");
        setCooldown(res.data?.resend_after || 60);
        toast({ title: t("email.forgot.requestSent"), variant: "success" });
      } else if (isThrottleErrorCode(res.error_code)) {
        // 被限流或 SMTP 服务商限制：按错误码决定冷却时长，减少无效重试。
        const cd = throttleCooldownSeconds(res.error_code);
        setCooldown((c) => (c > 0 ? c : cd));
        toast({ title: friendlyError(res.error_code, res.message), variant: "destructive" });
      } else {
        toast({ title: friendlyError(res.error_code, res.message), variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: error?.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setEmailLoading(false);
    }
  };

  const submitEmailReset = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!code.trim()) {
      toast({ title: t("email.enterCodeFirst"), variant: "destructive" });
      return;
    }
    const strength = validatePasswordStrength(newPassword, t("common.password"));
    if (!strength.ok) {
      toast({ title: strength.message, variant: "destructive" });
      return;
    }
    setEmailLoading(true);
    try {
      const res = await api.resetPasswordByEmail({ email: email.trim(), code: code.trim(), new_password: newPassword });
      if (res.success) {
        setEmailDone(true);
        toast({ title: t("email.forgot.resetSuccess"), variant: "success" });
      } else {
        toast({ title: friendlyError(res.error_code, res.message), variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: error?.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setEmailLoading(false);
    }
  };

  const copyPassword = () => {
    if (!embyResult?.new_password) return;
    navigator.clipboard.writeText(embyResult.new_password);
    toast({ title: t("auth.forgotPassword.copied") });
  };

  const embyForm = (
    <div className="space-y-5">
      <form onSubmit={submitEmby} className="space-y-4">
        <div className="space-y-2">
          <Label>{t("auth.forgotPassword.embyUsername")}</Label>
          <Input value={embyUsername} onChange={(e) => setEmbyUsername(e.target.value)} autoComplete="username" />
        </div>
        <div className="space-y-2">
          <Label>{t("auth.forgotPassword.embyPassword")}</Label>
          <Input type="password" value={embyPassword} onChange={(e) => setEmbyPassword(e.target.value)} autoComplete="current-password" />
        </div>
        <Button type="submit" className="w-full" disabled={embyLoading}>
          {embyLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          {t("auth.forgotPassword.submit")}
        </Button>
      </form>
      {embyResult && (
        <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-4">
          <p className="text-sm font-semibold">{t("auth.forgotPassword.webUsername", { username: embyResult.username })}</p>
          <p className="mt-2 text-xs text-muted-foreground">{t("auth.forgotPassword.copyHint")}</p>
          <div className="mt-3 flex items-center gap-2">
            <code className="min-w-0 flex-1 break-all rounded bg-background px-3 py-2 text-sm">{embyResult.new_password}</code>
            <Button type="button" size="icon" variant="outline" onClick={copyPassword} aria-label={t("auth.forgotPassword.copyPassword")}>
              <Copy className="h-4 w-4" aria-hidden="true" />
            </Button>
          </div>
        </div>
      )}
    </div>
  );

  const emailForm = emailDone ? (
    <div className="rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-4 text-center text-sm">
      <p className="font-semibold">{t("email.forgot.resetSuccess")}</p>
      <Link href="/login" className="mt-3 inline-block font-medium text-emerald-700 hover:underline dark:text-emerald-300">
        {t("auth.forgotPassword.backToLogin")}
      </Link>
    </div>
  ) : emailStage === "request" ? (
    <form onSubmit={requestEmailCode} className="space-y-4">
      <p className="text-sm text-foreground">{t("email.forgot.description")}</p>
      <div className="space-y-2">
        <Label>{t("email.emailLabel")}</Label>
        <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder={t("email.emailPlaceholder")} autoComplete="email" />
      </div>
      <Button type="submit" className="w-full" disabled={emailLoading || cooldown > 0}>
        {emailLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
        {cooldown > 0 ? t("email.resendIn", { seconds: cooldown }) : t("email.sendCode")}
      </Button>
    </form>
  ) : (
    <form onSubmit={submitEmailReset} className="space-y-4">
      <p className="text-sm text-foreground">{t("email.codeSentTo", { email })}</p>
      <div className="space-y-2">
        <Label>{t("email.codeLabel")}</Label>
        <Input value={code} onChange={(e) => setCode(e.target.value)} placeholder={t("email.codePlaceholder")} inputMode="numeric" autoComplete="one-time-code" />
      </div>
      <div className="space-y-2">
        <Label>{t("email.newPassword")}</Label>
        <Input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} placeholder={t("email.newPasswordPlaceholder")} autoComplete="new-password" />
      </div>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
        <Button type="submit" className="flex-1" disabled={emailLoading}>
          {emailLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          {t("email.forgot.submitReset")}
        </Button>
        <Button type="button" variant="outline" disabled={cooldown > 0 || emailLoading} onClick={() => requestEmailCode()}>
          {cooldown > 0 ? t("email.resendIn", { seconds: cooldown }) : t("email.resend")}
        </Button>
      </div>
    </form>
  );

  const showTabs = emailAvailable && embyAvailable;
  const showEmbyOnly = embyAvailable && !emailAvailable;
  const showEmailOnly = !embyAvailable && emailAvailable;
  const nothingAvailable = !embyAvailable && !emailAvailable;

  return (
    <main className="relative flex min-h-screen w-full items-center justify-center p-4">
      <Card className="auth-card-inner w-full max-w-[460px] animate-auth-enter border-border/70 bg-card/78 shadow-2xl backdrop-blur-xl">
        <CardHeader className="space-y-2 text-center">
          <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-muted text-foreground">
            <KeyRound className="h-7 w-7" />
          </div>
          <CardTitle className="text-2xl">{t("auth.forgotPassword.title")}</CardTitle>
          <CardDescription className="text-foreground">{t("auth.forgotPassword.description")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          {!forgotPasswordEnabled || nothingAvailable ? (
            <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-4 text-center text-sm">
              <p className="font-semibold">{t("auth.forgotPassword.adminDisabled")}</p>
            </div>
          ) : showTabs ? (
            <Tabs defaultValue="email">
              <TabsList className="grid w-full grid-cols-2">
                <TabsTrigger value="email">
                  <Mail className="mr-1.5 h-4 w-4" />
                  {t("email.forgot.emailTab")}
                </TabsTrigger>
                <TabsTrigger value="emby">{t("email.forgot.embyTab")}</TabsTrigger>
              </TabsList>
              <TabsContent value="email" className="mt-4">{emailForm}</TabsContent>
              <TabsContent value="emby" className="mt-4">{embyForm}</TabsContent>
            </Tabs>
          ) : showEmbyOnly ? (
            embyForm
          ) : showEmailOnly ? (
            emailForm
          ) : null}

          <div className="text-center text-sm">
            <Link href="/login" className="font-medium text-foreground hover:underline">{t("auth.forgotPassword.backToLogin")}</Link>
          </div>
        </CardContent>
      </Card>
    </main>
  );
}
