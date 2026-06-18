"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import Image from "next/image";
import { useRouter } from "next/navigation";
import { Eye, EyeOff, ArrowRight, Loader2, Send } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useToast } from "@/hooks/use-toast";
import { useAuthStore } from "@/store/auth";
import { useSystemStore } from "@/store/system";
import { SITE_NAME } from "@/lib/site-config";
import { sanitizeExternalUrl, sanitizeImageUrl } from "@/lib/safe-url";
import { API_BASE } from "@/lib/api-request";
import { friendlyError } from "@/lib/validators";
import { safeProtectedRedirectTarget } from "@/lib/auth-routes";
import { useI18n } from "@/lib/i18n";

function loginRedirectTarget(): string {
  if (typeof window === "undefined") return "/dashboard";
  const next = new URLSearchParams(window.location.search).get("next");
  return safeProtectedRedirectTarget(next);
}

function serverIconUrl(icon?: string | null): string | undefined {
  if (!icon) return undefined;
  if (icon.startsWith("http")) return sanitizeImageUrl(icon);
  if (icon.startsWith("/")) return sanitizeImageUrl(`${API_BASE}/api/v1${icon}`);
  return sanitizeImageUrl(icon);
}

export default function LoginPage() {
  const router = useRouter();
  const { toast } = useToast();
  const { t } = useI18n();
  const { login } = useAuthStore();
  const { info: systemInfo, fetchInfo: fetchSystemInfo } = useSystemStore();
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
    ...(requiredTelegramLinks.length > 0 ? requiredTelegramLinks : [
      ...(systemInfo?.telegram_links?.groups || []),
      ...(systemInfo?.telegram_links?.channels || []),
    ]),
  ].map((item) => ({ ...item, url: sanitizeExternalUrl(item.url) })).filter((item): item is { label: string; url: string } => Boolean(item.url));

  useEffect(() => {
    // 不在未登录状态预取受保护路由：middleware 会把预取重定向到 /login，
    // App Router 之后可能复用这份旧结果，让登录成功后的跳转看起来失效。
    void fetchSystemInfo();
  }, [fetchSystemInfo]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!username || !password) {
      toast({
        title: t("auth.login.incomplete"),
        variant: "destructive",
      });
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
        // 用稳定的 error_code 决定 UI 分支，避免 /禁用/.test(message) 这种
        // 文案级匹配在后端切英文 / 改文案时炸掉。
        // 区分两类失败：
        //   - AUTH_ACCOUNT_DISABLED：管理员主动禁用，引导联系管理员；
        //   - AUTH_ACCOUNT_EXPIRED：entitlement 到期，引导续费而非申诉。
        // 后端在 R62-6 起把这两种 Active=false 状态拆开返回，前端原本只会
        // 提示"账户被禁用"让到期用户也跑去找管理员，UX 错位。
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
        toast({
          title,
          description: body,
          variant: "destructive",
        });
      }
    } catch (error) {
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
    <main className="relative flex min-h-screen w-full items-center justify-center p-4">
      <div className="relative z-10 w-full max-w-[440px] animate-auth-enter">
        <Card className="auth-card-inner border-border/70 bg-card/78 shadow-2xl backdrop-blur-xl">
          <CardHeader className="space-y-2 pb-6 pt-8 text-center">
            {systemInfo?.server_icon ? (
              <div className="mx-auto mb-3 flex h-14 w-14 items-center justify-center rounded-2xl bg-muted overflow-hidden relative">
                <Image
                  src={serverIconUrl(systemInfo.server_icon) || ""}
                  alt={systemInfo.name || SITE_NAME}
                  fill
                  className="object-cover"
                  onError={(e) => {
                    e.currentTarget.style.display = "none";
                  }}
                />
              </div>
            ) : null}

            <CardTitle className="text-2xl font-semibold tracking-tight">
              {t("auth.login.title", { site: systemInfo?.name || SITE_NAME })}
            </CardTitle>
            <CardDescription className="text-sm text-foreground">
              {t("auth.login.description")}
            </CardDescription>
          </CardHeader>

          <CardContent className="px-6 pb-7 md:px-8">
            {telegramLinks.length > 0 && (
              <div className="mb-5 rounded-xl border border-border/70 bg-muted/40 px-4 py-3 text-sm">
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
                <Label htmlFor="username" className="ml-1">{t("common.username")}</Label>
                <Input
                  id="username"
                  placeholder="Username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
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
                  {t("auth.login.submit")}
                </Button>
              </div>
            </form>
 
            {forgotPasswordEnabled && (
            <div className="mt-5 text-center text-sm">
              <Link href="/forgot-password" className="font-medium text-foreground/80 hover:text-foreground hover:underline">
                {t("auth.login.forgotPassword")}
              </Link>
            </div>
            )}

            <div className="mt-5 flex items-center justify-center gap-2 text-sm">
              <span className="text-foreground">{t("auth.login.noAccount")}</span>
              <Link
                href="/register"
                className="font-medium text-foreground/80 hover:text-foreground hover:underline"
              >
                {t("auth.login.createAccount")}
              </Link>
            </div>
          </CardContent>
        </Card>
      </div>
    </main>
  );
}
