"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Eye, EyeOff, ArrowRight, Loader2, Send } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useToast } from "@/hooks/use-toast";
import { useAuthStore } from "@/store/auth";
import { useSystemStore } from "@/store/system";
import { sanitizeExternalUrl } from "@/lib/safe-url";
import { friendlyError } from "@/lib/validators";
import { safeProtectedRedirectTarget } from "@/lib/auth-routes";
import { useI18n } from "@/lib/i18n";
import { AuthBrand, AUTH_PRIMARY_BTN, AUTH_GHOST_LINK } from "../auth-ui";

function loginRedirectTarget(): string {
  if (typeof window === "undefined") return "/dashboard";
  const next = new URLSearchParams(window.location.search).get("next");
  return safeProtectedRedirectTarget(next);
}

export default function LoginPage() {
  const router = useRouter();
  const { toast } = useToast();
  const { t } = useI18n();
  const { login } = useAuthStore();
  const { info: systemInfo } = useSystemStore();
  const forgotPasswordEnabled = Boolean(systemInfo?.features?.forgot_password_enabled);

  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [isLoading, setIsLoading] = useState(false);

  const requiredTelegramLinks = [
    ...(systemInfo?.required_telegram_links?.groups || []),
    ...(systemInfo?.required_telegram_links?.channels || []),
  ];
  const telegramLinks = [
    ...(requiredTelegramLinks.length > 0
      ? requiredTelegramLinks
      : [
          ...(systemInfo?.telegram_links?.groups || []),
          ...(systemInfo?.telegram_links?.channels || []),
        ]),
  ]
    .map((item) => ({ ...item, url: sanitizeExternalUrl(item.url) }))
    .filter((item): item is { label: string; url: string } => Boolean(item.url));

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!username || !password) {
      toast({ title: t("auth.login.incomplete"), variant: "destructive" });
      return;
    }

    setIsLoading(true);
    try {
      const result = await login(username, password);
      if (result.ok) {
        toast({
          title: t("auth.login.successTitle"),
          description: t("auth.login.successDescription"),
          variant: "success",
        });
        router.replace(loginRedirectTarget());
      } else {
        // 用稳定的 error_code 决定 UI 分支，避免文案级匹配在后端切英文时炸掉。
        const code = result.errorCode;
        const disabled = code === "AUTH_ACCOUNT_DISABLED";
        const expired = code === "AUTH_ACCOUNT_EXPIRED";
        const description = code
          ? friendlyError(code, result.message)
          : result.message || t("auth.login.invalidCredentials");
        let title = t("auth.login.failed");
        let body = description;
        if (disabled) {
          title = t("auth.login.accountDisabled");
          body = t("auth.login.contactAdmin");
        } else if (expired) {
          title = t("auth.login.accountExpired");
          body = t("auth.login.renewBeforeLogin");
        }
        toast({ title, description: body, variant: "destructive" });
      }
    } catch {
      toast({
        title: t("auth.login.failed"),
        description: t("common.checkNetwork"),
        variant: "destructive",
      });
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <>
      <AuthBrand subtitle={t("auth.login.description")} />

      {telegramLinks.length > 0 && (
        <div className="rounded-xl border border-border/70 bg-muted/40 px-4 py-3 text-sm">
          <div className="mb-2 flex items-center gap-2 font-medium text-foreground">
            <Send className="h-4 w-4 text-muted-foreground" />
            {t("auth.login.telegramCommunity")}
          </div>
          <div className="flex flex-wrap gap-2">
            {telegramLinks.map((item) => (
              <a
                key={item.url}
                href={item.url}
                target="_blank"
                rel="noopener noreferrer"
                className="rounded-md border border-border/70 bg-background px-2.5 py-1 text-xs font-medium text-foreground hover:bg-muted"
              >
                {item.label}
              </a>
            ))}
          </div>
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-5">
        <div className="space-y-2">
          <Label htmlFor="username" className="ml-1">{t("common.username")} / {t("common.email")}</Label>
          <Input
            id="username"
            placeholder="Username / Email"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
            className="h-11"
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="password" className="ml-1">{t("common.password")}</Label>
          <div className="relative">
            <Input
              id="password"
              type={showPassword ? "text" : "password"}
              placeholder="Password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              className="h-11 pr-10"
            />
            <button
              type="button"
              onClick={() => setShowPassword(!showPassword)}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
              aria-label={t("common.showPassword")}
            >
              {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </button>
          </div>
        </div>

        <Button type="submit" className={AUTH_PRIMARY_BTN} disabled={isLoading}>
          {isLoading ? (
            <Loader2 className="mr-2 h-5 w-5 animate-spin" />
          ) : (
            <ArrowRight className="mr-2 h-5 w-5" />
          )}
          {t("auth.login.submit")}
        </Button>
      </form>

      {forgotPasswordEnabled && (
        <div className="text-center text-sm">
          <Link href="/forgot-password" className={AUTH_GHOST_LINK}>
            {t("auth.login.forgotPassword")}
          </Link>
        </div>
      )}

      <div className="flex items-center justify-center gap-2 text-sm">
        <span className="text-foreground">{t("auth.login.noAccount")}</span>
        <Link href="/register" className={AUTH_GHOST_LINK}>
          {t("auth.login.createAccount")}
        </Link>
      </div>
    </>
  );
}
