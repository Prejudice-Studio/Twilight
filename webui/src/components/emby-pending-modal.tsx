"use client";

import { useEffect, useState } from "react";
import { Eye, EyeOff, Loader2, ShieldPlus } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useToast } from "@/hooks/use-toast";
import { useAuthStore } from "@/store/auth";
import { api } from "@/lib/api";
import { passwordStrengthLabel, validatePasswordStrength } from "@/lib/password";

/**
 * 全站挂载的 Modal：当用户系统账号已建但 Emby 未绑定时（pending_emby=true）
 * 每次进入受认证页面都会弹出引导用户补建 Emby 账号。可以暂时关闭逛网站。
 */
export function EmbyPendingModal() {
  const { user, fetchUser } = useAuthStore();
  const { toast } = useToast();

  const [open, setOpen] = useState(false);
  const [embyUsername, setEmbyUsername] = useState("");
  const [embyPassword, setEmbyPassword] = useState("");
  const [showPwd, setShowPwd] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const isPending = Boolean(user?.pending_emby) && !user?.emby_id;

  useEffect(() => {
    if (isPending) {
      // 每次 user 变更且仍处于 pending 时，把 modal 打开
      setOpen(true);
      setEmbyUsername(user?.username || "");
    } else {
      setOpen(false);
    }
  }, [isPending, user?.uid, user?.username]);

  if (!isPending) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!embyUsername.trim()) {
      toast({ title: "请填写 Emby 用户名", variant: "destructive" });
      return;
    }
    const strength = validatePasswordStrength(embyPassword, "Emby 密码");
    if (!strength.ok) {
      toast({ title: "密码强度不足", description: strength.message, variant: "destructive" });
      return;
    }

    setSubmitting(true);
    try {
      const res = await api.completeEmbyRegistration(embyUsername.trim(), embyPassword);
      if (res.success) {
        toast({
          title: "Emby 账号已创建并绑定",
          description: res.message,
          variant: "success",
        });
        await fetchUser({ silent: true });
        setOpen(false);
        setEmbyPassword("");
      } else {
        toast({
          title: "创建 Emby 账号失败",
          description: res.message,
          variant: "destructive",
        });
      }
    } catch (err: any) {
      toast({
        title: "创建 Emby 账号失败",
        description: err?.message || "请稍后重试",
        variant: "destructive",
      });
    } finally {
      setSubmitting(false);
    }
  };

  const pendingDays = user?.pending_emby_days;
  const daysText = pendingDays === null || pendingDays === undefined
    ? "由管理员默认配置决定"
    : pendingDays <= 0
      ? "永久"
      : `${pendingDays} 天`;

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldPlus className="h-5 w-5 text-primary" />
            完成 Emby 账号注册
          </DialogTitle>
          <DialogDescription>
            你的系统账号已建好，但还没有绑定 Emby 账号。补建一次即可使用媒体服务，
            如果失败可下次登录再试。
            <span className="mt-1 block text-xs text-muted-foreground">
              开通时长：{daysText}
            </span>
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-3 py-2">
          <div className="space-y-2">
            <Label htmlFor="pending-emby-name">Emby 用户名</Label>
            <Input
              id="pending-emby-name"
              value={embyUsername}
              onChange={(e) => setEmbyUsername(e.target.value)}
              placeholder="3-20 位字母数字下划线"
              disabled={submitting}
              autoComplete="off"
            />
            <p className="text-xs text-muted-foreground">
              建议与系统用户名保持一致；若已被占用可换个名字。
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="pending-emby-pwd">Emby 密码</Label>
            <div className="relative">
              <Input
                id="pending-emby-pwd"
                type={showPwd ? "text" : "password"}
                value={embyPassword}
                onChange={(e) => setEmbyPassword(e.target.value)}
                placeholder="至少 8 位，含大小写和数字"
                disabled={submitting}
                className="pr-10"
                autoComplete="new-password"
              />
              <button
                type="button"
                onClick={() => setShowPwd((v) => !v)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                aria-label="切换密码可见"
              >
                {showPwd ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            {embyPassword && (() => {
              const s = validatePasswordStrength(embyPassword, "Emby 密码");
              return (
                <p className={`text-xs ${s.ok ? passwordStrengthLabel(s.score).className : "text-destructive"}`}>
                  {s.ok ? `强度：${passwordStrengthLabel(s.score).label}` : s.message}
                </p>
              );
            })()}
          </div>

          <DialogFooter className="gap-2 sm:gap-2">
            <Button
              type="button"
              variant="ghost"
              onClick={() => setOpen(false)}
              disabled={submitting}
            >
              暂不补充
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              创建并绑定
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
