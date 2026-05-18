"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { motion } from "framer-motion";
import { Eye, EyeOff, Loader2, ShieldPlus, UserPlus, Bot } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardTitle } from "@/components/ui/card";
import { useToast } from "@/hooks/use-toast";
import { api, type RegisterAvailability, type RegisterData } from "@/lib/api";
import { SITE_NAME } from "@/lib/site-config";
import { useSystemStore } from "@/store/system";
import { passwordStrengthLabel, validatePasswordStrength } from "@/lib/password";

export default function RegisterPage() {
  const router = useRouter();
  const { toast } = useToast();
  const { info: systemInfo, fetchInfo: fetchSystemInfo } = useSystemStore();

  const [formData, setFormData] = useState({
    username: "",
    password: "",
    confirmPassword: "",
    email: "",
    regCode: "",
  });

  const [registerAvailability, setRegisterAvailability] = useState<RegisterAvailability | null>(null);
  const [bindCode, setBindCode] = useState("");
  const [bindCodeExpiry, setBindCodeExpiry] = useState(0);
  const [bindConfirmed, setBindConfirmed] = useState(false);

  const [isRegisterLoading, setIsRegisterLoading] = useState(false);
  const [isBindCodeLoading, setIsBindCodeLoading] = useState(false);
  const [showPassword, setShowPassword] = useState(false);

  useEffect(() => {
    void fetchSystemInfo();
    void refreshRegisterAvailability();
  }, [fetchSystemInfo]);

  const forceBindTelegram = Boolean(systemInfo?.features?.force_bind_telegram);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setFormData({ ...formData, [e.target.name]: e.target.value });
  };

  const refreshRegisterAvailability = async () => {
    try {
      const res = await api.getRegisterAvailability();
      if (res.success && res.data) {
        setRegisterAvailability(res.data);
      }
    } catch {
      // ignore
    }
  };

  const handleGetTelegramBindCode = async () => {
    setIsBindCodeLoading(true);
    try {
      const res = await api.getRegisterBindCode();
      setBindCode(res.data?.bind_code || "");
      setBindCodeExpiry(res.data?.expires_in ?? 0);
      setBindConfirmed(false);
      toast({
        title: "已生成绑定码",
        description: "请在 Telegram Bot 私聊中发送 /bind <绑定码> 完成验证",
        variant: "success",
      });
    } catch (error: any) {
      toast({
        title: "获取绑定码失败",
        description: error.message || "请检查 API 服务可达性与 Telegram Bot 配置",
        variant: "destructive",
      });
    } finally {
      setIsBindCodeLoading(false);
    }
  };

  // 拿到绑定码后开始轮询，直到 Bot 端确认或绑定码过期。
  useEffect(() => {
    if (!bindCode || bindConfirmed) return;

    let cancelled = false;
    let toastedConfirmed = false;
    const controller = new AbortController();

    const tick = async () => {
      try {
        const res = await api.getRegisterBindCodeStatus(bindCode, controller.signal);
        if (cancelled) return;
        if (res.success && res.data) {
          if (typeof res.data.expires_in === "number") {
            setBindCodeExpiry(res.data.expires_in);
          }
          if (res.data.confirmed && !toastedConfirmed) {
            toastedConfirmed = true;
            setBindConfirmed(true);
            toast({
              title: "Telegram 绑定成功",
              description: "点击下方「注册」按钮即可进入系统",
              variant: "success",
            });
          }
        } else if (res.message && /无效|过期/.test(res.message)) {
          if (!cancelled) {
            setBindCode("");
            setBindCodeExpiry(0);
            setBindConfirmed(false);
            toast({
              title: "绑定码已过期",
              description: "请重新获取绑定码",
              variant: "destructive",
            });
          }
        }
      } catch {
        // 静默重试
      }
    };

    void tick();
    const handle = window.setInterval(tick, 2000);

    return () => {
      cancelled = true;
      controller.abort();
      window.clearInterval(handle);
    };
  }, [bindCode, bindConfirmed, toast]);

  const validateRegisterForm = (): boolean => {
    if (!formData.username) {
      toast({ title: "请填写用户名", variant: "destructive" });
      return false;
    }

    if (!formData.password) {
      toast({ title: "请设置密码", variant: "destructive" });
      return false;
    }

    if (formData.password !== formData.confirmPassword) {
      toast({ title: "密码不一致", description: "请确认两次输入的密码相同", variant: "destructive" });
      return false;
    }

    const strength = validatePasswordStrength(formData.password, "密码");
    if (!strength.ok) {
      toast({ title: "密码强度不足", description: strength.message, variant: "destructive" });
      return false;
    }

    if (forceBindTelegram && !bindCode) {
      toast({
        title: "请先完成 Telegram 绑定验证",
        description: "点击获取绑定码后，在 Bot 私聊发送 /bind <绑定码>",
        variant: "destructive",
      });
      return false;
    }

    if (forceBindTelegram && bindCode && !bindConfirmed) {
      toast({
        title: "请先在 Telegram 完成绑定验证",
        description: `请去 Bot 私聊发送 /bind ${bindCode}`,
        variant: "destructive",
      });
      return false;
    }

    return true;
  };

  const handleRegisterSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!validateRegisterForm()) {
      return;
    }

    setIsRegisterLoading(true);
    try {
      const payload: RegisterData = {
        username: formData.username,
        email: formData.email || undefined,
        telegram_bind_code: bindCode || undefined,
        password: formData.password,
      };

      if (formData.regCode.trim()) {
        payload.reg_code = formData.regCode.trim();
      }

      const res = await api.register(payload);

      if (!res.success) {
        toast({ title: "注册失败", description: res.message, variant: "destructive" });
        return;
      }

      toast({
        title: "注册成功",
        description: "请使用系统账号登录，登录后将引导你补建 Emby 账号",
        variant: "success",
      });
      router.push("/login");
    } catch (error: any) {
      toast({
        title: "注册失败",
        description: error.message || "请检查网络连接",
        variant: "destructive",
      });
    } finally {
      setIsRegisterLoading(false);
      void refreshRegisterAvailability();
    }
  };

  return (
    <main className="relative flex min-h-screen w-full items-center justify-center p-4">
      <motion.div
        initial={{ opacity: 0, scale: 0.95, y: 20 }}
        animate={{ opacity: 1, scale: 1, y: 0 }}
        transition={{ duration: 0.35, ease: "easeOut" }}
        className="relative z-10 w-full max-w-[1100px]"
      >
        <Card className="grid gap-6 overflow-hidden border-border/70 bg-card/78 shadow-2xl backdrop-blur-xl lg:grid-cols-[300px_minmax(0,1fr)]">
          <div className="space-y-6 border-b border-border/70 p-6 lg:border-b-0 lg:border-r lg:p-8">
            <div className="space-y-2">
              <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/14 text-primary">
                <ShieldPlus className="h-7 w-7" />
              </div>
              <div>
                <h2 className="text-xl font-semibold">欢迎来到 {systemInfo?.name || SITE_NAME}</h2>
                <p className="text-sm text-muted-foreground">
                  使用注册码注册系统账号；登录后将自动引导你完成 Emby 账号补建。
                </p>
              </div>
            </div>

            <div className="rounded-2xl border border-border/70 bg-muted/40 p-4 text-sm text-muted-foreground">
              <p className="font-semibold text-foreground">关于注册码</p>
              <p className="mt-2 leading-relaxed">
                注册码即 Emby 注册码：使用它创建账号后，首次登录时会自动弹出 Emby 账号注册流程，
                如失败可在后续登录时反复重试，直到绑定成功。
              </p>
              {systemInfo?.telegram_bot?.username ? (
                <p className="mt-2 inline-flex items-center gap-1.5 text-xs">
                  <Bot className="h-3.5 w-3.5" />
                  <span>绑定 Bot：</span>
                  <a
                    href={systemInfo.telegram_bot.url ?? `https://t.me/${systemInfo.telegram_bot.username}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="font-medium text-primary hover:underline"
                  >
                    @{systemInfo.telegram_bot.username}
                  </a>
                </p>
              ) : null}
              {registerAvailability ? (
                <p className="mt-2 text-xs text-muted-foreground">
                  当前注册配额: {registerAvailability.current_users} / {registerAvailability.max_users}
                </p>
              ) : null}
            </div>
          </div>

          <div className="space-y-6 p-6 sm:p-8">
            <div className="space-y-3">
              <CardTitle className="text-2xl font-semibold tracking-tight">创建账号</CardTitle>
              <p className="text-sm text-muted-foreground">
                填写下面的信息即可注册。注册码如有可直接填入，将影响 Emby 账号的开通时长。
              </p>
            </div>

            <form onSubmit={handleRegisterSubmit} className="space-y-4">
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="username" className="ml-1">用户名 *</Label>
                  <Input
                    id="username"
                    name="username"
                    placeholder="Username"
                    value={formData.username}
                    onChange={handleChange}
                    className="h-11"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="email" className="ml-1">邮箱</Label>
                  <Input
                    id="email"
                    name="email"
                    type="email"
                    placeholder="Email (Optional)"
                    value={formData.email}
                    onChange={handleChange}
                    className="h-11"
                  />
                </div>
              </div>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="password" className="ml-1">设置密码 *</Label>
                  <div className="relative">
                    <Input
                      id="password"
                      name="password"
                      type={showPassword ? "text" : "password"}
                      placeholder="至少 8 位，含大小写字母和数字"
                      value={formData.password}
                      onChange={handleChange}
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
                  {formData.password && (() => {
                    const s = validatePasswordStrength(formData.password, "密码");
                    return (
                      <p className={`text-xs ${s.ok ? passwordStrengthLabel(s.score).className : "text-destructive"}`}>
                        {s.ok ? `强度：${passwordStrengthLabel(s.score).label}` : s.message}
                      </p>
                    );
                  })()}
                </div>
                <div className="space-y-2">
                  <Label htmlFor="confirmPassword" className="ml-1">确认密码 *</Label>
                  <Input
                    id="confirmPassword"
                    name="confirmPassword"
                    type="password"
                    placeholder="Confirm Password"
                    value={formData.confirmPassword}
                    onChange={handleChange}
                    className="h-11"
                  />
                </div>
              </div>

              <div className="space-y-2">
                <Label htmlFor="regCode" className="ml-1 text-xs">注册码（可选）</Label>
                <Input
                  id="regCode"
                  name="regCode"
                  placeholder="Registration Code"
                  value={formData.regCode}
                  onChange={handleChange}
                  className="h-11"
                />
                <p className="text-xs text-muted-foreground">
                  使用注册码注册可在首次登录时获得对应的 Emby 开通天数；不填则按管理员设置的默认时长。
                </p>
              </div>

              {(forceBindTelegram || systemInfo?.features?.telegram) && (
                <div className="space-y-2">
                  <Label className="ml-1">
                    Telegram 绑定{forceBindTelegram ? " *" : "（可选）"}
                  </Label>
                  <div className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
                    <p className="font-medium">在 Telegram 中打开服务 Bot 的私聊窗口。</p>
                    <p className="mt-1 leading-relaxed">
                      点击"获取绑定码"后，在 Bot 私聊中发送 /bind &lt;绑定码&gt; 完成验证。
                    </p>
                    {systemInfo?.telegram_bot?.username ? (
                      <p className="mt-2 inline-flex items-center gap-1.5 text-xs text-amber-900">
                        <Bot className="h-3.5 w-3.5" />
                        <span>本站 Bot：</span>
                        <a
                          href={systemInfo.telegram_bot.url ?? `https://t.me/${systemInfo.telegram_bot.username}`}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="font-medium underline-offset-2 hover:underline"
                        >
                          @{systemInfo.telegram_bot.username}
                        </a>
                      </p>
                    ) : (
                      <p className="mt-2 text-xs text-amber-700">
                        管理员尚未配置可识别的 Bot 账号，如无法获取绑定码请联系管理员。
                      </p>
                    )}
                  </div>
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:flex-wrap">
                    <Button
                      type="button"
                      onClick={handleGetTelegramBindCode}
                      disabled={isBindCodeLoading}
                    >
                      {isBindCodeLoading ? (
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      ) : (
                        <ShieldPlus className="mr-2 h-4 w-4" />
                      )}
                      获取绑定码
                    </Button>
                    {systemInfo?.telegram_bot?.url ? (
                      <Button asChild type="button" variant="outline">
                        <a
                          href={systemInfo.telegram_bot.url}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          <Bot className="mr-2 h-4 w-4" />
                          打开 @{systemInfo.telegram_bot.username}
                        </a>
                      </Button>
                    ) : null}
                    {bindCode && !bindConfirmed ? (
                      <div className="basis-full space-y-2 rounded-lg border border-border/70 bg-muted/50 px-3 py-3 text-sm text-muted-foreground">
                        <p>请到 Bot 私聊发送下面这条命令：</p>
                        <div className="flex flex-wrap items-center gap-2">
                          <code className="rounded bg-background px-2 py-1 font-mono text-base text-foreground select-all">
                            /bind {bindCode}
                          </code>
                          <Button
                            type="button"
                            size="sm"
                            variant="outline"
                            onClick={() => {
                              navigator.clipboard.writeText(`/bind ${bindCode}`).then(
                                () => toast({ title: "已复制到剪贴板", variant: "success" }),
                                () => toast({ title: "复制失败", variant: "destructive" }),
                              );
                            }}
                          >
                            复制命令
                          </Button>
                          {systemInfo?.telegram_bot?.url ? (
                            <Button asChild type="button" size="sm">
                              <a
                                href={systemInfo.telegram_bot.url}
                                target="_blank"
                                rel="noopener noreferrer"
                              >
                                <Bot className="mr-2 h-4 w-4" />
                                打开 @{systemInfo.telegram_bot.username}
                              </a>
                            </Button>
                          ) : null}
                        </div>
                        <p className="flex items-center gap-1 text-xs">
                          <Loader2 className="h-3 w-3 animate-spin" />
                          等待 Bot 端验证…（剩余 {Math.max(0, Math.floor(bindCodeExpiry / 60))} 分钟）
                        </p>
                      </div>
                    ) : null}
                    {bindCode && bindConfirmed ? (
                      <div className="rounded-lg border border-emerald-300/60 bg-emerald-50 px-3 py-2 text-sm dark:border-emerald-700/60 dark:bg-emerald-900/30">
                        <p className="font-semibold text-emerald-700 dark:text-emerald-300">
                          ✅ Telegram 绑定成功
                        </p>
                        <p className="text-xs text-emerald-700/80 dark:text-emerald-300/80">
                          点击下方「注册」按钮即可进入系统。
                        </p>
                      </div>
                    ) : null}
                  </div>
                </div>
              )}

              <div className="pt-2">
                <Button
                  type="submit"
                  className="h-11 w-full"
                  disabled={
                    isRegisterLoading ||
                    (forceBindTelegram && !!bindCode && !bindConfirmed)
                  }
                >
                  {isRegisterLoading ? (
                    <Loader2 className="mr-2 h-5 w-5 animate-spin" />
                  ) : (
                    <UserPlus className="mr-2 h-5 w-5" />
                  )}
                  注册账号
                </Button>
              </div>

              <div className="pt-1 text-center">
                <Button asChild variant="link" className="h-auto px-1 text-sm">
                  <Link href="/login">已有账号？返回登录页</Link>
                </Button>
              </div>
            </form>
          </div>
        </Card>
      </motion.div>
    </main>
  );
}
