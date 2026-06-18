"use client";

import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useToast } from "@/hooks/use-toast";
import { useI18n } from "@/lib/i18n";
import { api } from "@/lib/api";
import { friendlyError, isThrottleErrorCode, throttleCooldownSeconds } from "@/lib/validators";

interface EmailCodeInputProps {
  purpose: "bind" | "change_password" | "change_emby_password";
  // bind 用途需要目标邮箱；change_* 用途由后端用已绑定邮箱，可省略。
  email?: string;
  code: string;
  onCodeChange: (code: string) => void;
  // 发码成功后把 verification_id 回传给父组件，提交时一并带上。
  onSent: (verificationId: string) => void;
  disabled?: boolean;
}

// EmailCodeInput 封装「获取验证码 + 输入验证码」的复用 UI（含重发冷却）。
// 绑定邮箱与强制改密两条链路共用：父组件保存 verification_id + code，提交时使用。
export function EmailCodeInput({ purpose, email, code, onCodeChange, onSent, disabled }: EmailCodeInputProps) {
  const { t } = useI18n();
  const { toast } = useToast();
  const [sending, setSending] = useState(false);
  const [cooldown, setCooldown] = useState(0);

  useEffect(() => {
    if (cooldown <= 0) return;
    const timer = setInterval(() => setCooldown((s) => (s > 0 ? s - 1 : 0)), 1000);
    return () => clearInterval(timer);
  }, [cooldown]);

  const send = async () => {
    if (purpose === "bind" && !email?.trim()) {
      toast({ title: t("email.enterEmailFirst"), variant: "destructive" });
      return;
    }
    setSending(true);
    try {
      const res = await api.sendEmailCode({ purpose, email: email?.trim() });
      if (res.success && res.data) {
        onSent(res.data.verification_id);
        setCooldown(res.data.resend_after || 60);
        toast({ title: t("email.codeSentTo", { email: res.data.email }), variant: "success" });
      } else if (isThrottleErrorCode(res.error_code)) {
        // 被限流或发送失败：本地起冷却让按钮自禁，避免继续无效点击。
        // EMAIL_SEND_FAILED 通常是 SMTP 服务商限流（如每小时发件上限），
        // 给予较长冷却（120s）减少无效重试。
        const cd = throttleCooldownSeconds(res.error_code);
        setCooldown((c) => (c > 0 ? c : cd));
        toast({ title: friendlyError(res.error_code, res.message), variant: "destructive" });
      } else {
        toast({ title: friendlyError(res.error_code, res.message), variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: error?.message || t("common.networkError"), variant: "destructive" });
    } finally {
      setSending(false);
    }
  };

  return (
    <div className="space-y-2">
      <Label>{t("email.codeLabel")}</Label>
      <div className="flex items-center gap-2">
        <Input
          value={code}
          onChange={(e) => onCodeChange(e.target.value)}
          placeholder={t("email.codePlaceholder")}
          inputMode="numeric"
          autoComplete="one-time-code"
          disabled={disabled}
        />
        <Button type="button" variant="outline" className="shrink-0" onClick={send} disabled={disabled || sending || cooldown > 0}>
          {sending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          {cooldown > 0 ? t("email.resendIn", { seconds: cooldown }) : t("email.sendCode")}
        </Button>
      </div>
    </div>
  );
}
