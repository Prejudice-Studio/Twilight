"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { motion } from "framer-motion";
import { Loader2, Mail, ShieldCheck, X } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useToast } from "@/hooks/use-toast";
import { useAuthStore } from "@/store/auth";
import { useSystemStore } from "@/store/system";
import { useI18n } from "@/lib/i18n";
import { api } from "@/lib/api";
import { validateEmailOptional, friendlyError, isThrottleErrorCode, throttleCooldownSeconds } from "@/lib/validators";

// EmailVerifyGuard 在开启「强制绑定邮箱」时介入仪表盘：
//   - 普通用户 / 白名单且未验证邮箱 → 全屏接管，必须完成绑定+验证才能继续；
//   - 管理员未验证 → 仅底部可关闭横幅提示，不阻断；
//   - 其它情况（未开启 / 已验证 / 邮箱功能关闭）→ 不渲染。
// 单一挂载点（(main)/layout.tsx）内部按角色自决模式，保持与 RebindGuard /
// EmbyPendingModal 一致的"守卫组件读 store 自行判定"风格。
export default function EmailVerifyGuard() {
  const { t } = useI18n();
  const { toast } = useToast();
  const router = useRouter();
  const { user, fetchUser } = useAuthStore();
  const { info } = useSystemStore();

  const [dismissed, setDismissed] = useState(false);
  const [email, setEmail] = useState("");
  const [stage, setStage] = useState<"email" | "code">("email");
  const [verificationId, setVerificationId] = useState("");
  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(false);
  const [cooldown, setCooldown] = useState(0);

  useEffect(() => {
    if (cooldown <= 0) return;
    const timer = setInterval(() => setCooldown((s) => (s > 0 ? s - 1 : 0)), 1000);
    return () => clearInterval(timer);
  }, [cooldown]);

  const emailEnabled = Boolean(info?.features?.email_enabled);
  const forceBind = Boolean(info?.features?.force_bind_email);
  const verified = Boolean(user?.email_verified);
  const applicable = Boolean(user) && emailEnabled && forceBind && !verified;
  const isAdmin = user?.role === 0;
  const mode: "required" | "nudge" | "none" = !applicable ? "none" : isAdmin ? "nudge" : "required";

  const sendCode = async () => {
    const check = validateEmailOptional(email);
    if (!email || !check.ok) {
      toast({ title: check.message || t("email.enterEmailFirst"), variant: "destructive" });
      return;
    }
    setLoading(true);
    try {
      const res = await api.sendEmailCode({ purpose: "bind", email: email.trim() });
      if (res.success && res.data) {
        setVerificationId(res.data.verification_id);
        setStage("code");
        setCooldown(res.data.resend_after || 60);
        toast({ title: t("email.codeSentTo", { email: res.data.email }), variant: "success" });
      } else if (isThrottleErrorCode(res.error_code)) {
        const cd = throttleCooldownSeconds(res.error_code);
        setCooldown((c) => (c > 0 ? c : cd));
        toast({ title: friendlyError(res.error_code, res.message), variant: "destructive" });
      } else {
        toast({ title: friendlyError(res.error_code, res.message), variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: error?.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setLoading(false);
    }
  };

  const verify = async () => {
    if (!code.trim()) {
      toast({ title: t("email.enterCodeFirst"), variant: "destructive" });
      return;
    }
    setLoading(true);
    try {
      const res = await api.verifyEmailCode({ verification_id: verificationId, code: code.trim() });
      if (res.success) {
        toast({ title: t("email.bindSuccess"), variant: "success" });
        await fetchUser();
      } else {
        toast({ title: friendlyError(res.error_code, res.message), variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: error?.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setLoading(false);
    }
  };

  if (mode === "none") return null;

  if (mode === "nudge") {
    if (dismissed) return null;
    return (
      <div className="fixed inset-x-0 bottom-0 z-50 px-4 pb-4">
        <div className="mx-auto flex max-w-2xl items-center gap-3 rounded-xl border border-amber-500/30 bg-amber-500/10 p-3 shadow-lg backdrop-blur">
          <Mail className="h-5 w-5 shrink-0 text-amber-500" />
          <div className="min-w-0 flex-1 text-sm">
            <p className="font-medium">{t("email.guard.adminTitle")}</p>
            <p className="text-xs text-muted-foreground">{t("email.guard.adminDescription")}</p>
          </div>
          <Button size="sm" variant="outline" onClick={() => router.push("/settings")}>{t("email.bindTitle")}</Button>
          <Button size="icon" variant="ghost" aria-label={t("email.guard.dismiss")} onClick={() => setDismissed(true)}>
            <X className="h-4 w-4" />
          </Button>
        </div>
      </div>
    );
  }

  // mode === "required"：全屏接管
  return (
    <div className="fixed inset-0 z-[100] flex min-h-screen items-center justify-center bg-background p-4">
      <motion.div initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} className="w-full max-w-md">
        <Card className="border-primary/20 bg-card/90 backdrop-blur-sm">
          <CardHeader className="pb-2 text-center">
            <div className="mx-auto mb-3 flex h-14 w-14 items-center justify-center rounded-full bg-primary/10">
              <ShieldCheck className="h-7 w-7 text-primary" />
            </div>
            <CardTitle className="text-xl">{t("email.guard.title")}</CardTitle>
            <CardDescription>{t("email.guard.description")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {stage === "email" ? (
              <div className="space-y-3">
                <div className="space-y-2">
                  <Label>{t("email.emailLabel")}</Label>
                  <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder={t("email.emailPlaceholder")} autoComplete="email" />
                </div>
                <Button className="w-full" size="lg" onClick={sendCode} disabled={loading || cooldown > 0}>
                  {loading ? <Loader2 className="mr-2 h-5 w-5 animate-spin" /> : <Mail className="mr-2 h-5 w-5" />}
                  {cooldown > 0 ? t("email.resendIn", { seconds: cooldown }) : t("email.sendCode")}
                </Button>
              </div>
            ) : (
              <div className="space-y-3">
                <p className="text-sm text-muted-foreground">{t("email.codeSentTo", { email })}</p>
                <div className="space-y-2">
                  <Label>{t("email.codeLabel")}</Label>
                  <Input value={code} onChange={(e) => setCode(e.target.value)} placeholder={t("email.codePlaceholder")} inputMode="numeric" autoComplete="one-time-code" />
                </div>
                <div className="flex items-center gap-2">
                  <Button className="flex-1" onClick={verify} disabled={loading}>
                    {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    {t("email.verify")}
                  </Button>
                  <Button variant="outline" disabled={cooldown > 0 || loading} onClick={sendCode}>
                    {cooldown > 0 ? t("email.resendIn", { seconds: cooldown }) : t("email.resend")}
                  </Button>
                </div>
                <Button variant="ghost" className="w-full text-xs" onClick={() => { setStage("email"); setCode(""); }}>
                  {t("email.changeTitle")}
                </Button>
              </div>
            )}
          </CardContent>
        </Card>
      </motion.div>
    </div>
  );
}
