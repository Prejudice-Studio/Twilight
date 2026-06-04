"use client";

import { useEffect, useRef, useState } from "react";
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
import { useAsyncHandler } from "@/hooks/use-async-handler";
import { useAuthStore } from "@/store/auth";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { passwordStrengthLabel, validatePasswordStrength } from "@/lib/password";

/**
 * 全站挂载的 Modal：仅引导已使用注册码但 Emby 补建未完成的用户继续补建。
 * 自由注册用户改为在仪表盘手动点击入口开通，避免未开启自由注册时自动弹窗。
 *
 * 关闭逻辑：用户点"暂不补充"或 ESC 关闭后，本次会话内不再自动弹窗，
 * 必须用户主动行为（重新登录、user 变更到非 pending → pending）才会再次打开。
 * 旧实现在每次 user 变化时无脑 setOpen(true)，导致用户无法 dismiss。
 */
export function EmbyPendingModal() {
  const { user, fetchUser } = useAuthStore();
  const { toast } = useToast();
  const { t } = useI18n();
  const handle = useAsyncHandler();

  const [open, setOpen] = useState(false);
  const [embyUsername, setEmbyUsername] = useState("");
  const [embyPassword, setEmbyPassword] = useState("");
  const [showPwd, setShowPwd] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  // 记录用户最近一次主动 dismiss 对应的 uid；下次同一用户再次进入 pending 状态
  // 不再自动弹窗，避免"无法关闭"的体感。重新登录会清掉。
  const dismissedUidRef = useRef<number | null>(null);

  const isPendingFromRegcode =
    Boolean(user?.pending_emby) &&
    !user?.emby_id &&
    user?.pending_emby_days !== null &&
    user?.pending_emby_days !== undefined;

  useEffect(() => {
    if (!isPendingFromRegcode) {
      setOpen(false);
      return;
    }
    // 用户切换登录态后允许重新弹一次
    if (dismissedUidRef.current !== null && dismissedUidRef.current !== user?.uid) {
      dismissedUidRef.current = null;
    }
    if (dismissedUidRef.current === user?.uid) return;
    setOpen(true);
    setEmbyUsername((prev) => prev || user?.username || "");
  }, [isPendingFromRegcode, user?.uid, user?.username]);

  if (!isPendingFromRegcode) return null;

  const handleClose = () => {
    if (submitting) return;
    if (user?.uid) dismissedUidRef.current = user.uid;
    setOpen(false);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    const trimmedName = embyUsername.trim();
    if (!trimmedName) {
      toast({ title: t("embyPending.usernameRequired"), variant: "destructive" });
      return;
    }
    const strength = validatePasswordStrength(embyPassword, t("embyPending.passwordLabel"));
    if (!strength.ok) {
      toast({ title: t("settings.passwordWeak"), description: strength.message, variant: "destructive" });
      return;
    }

    setSubmitting(true);
    const result = await handle(
      () => api.completeEmbyRegistration(trimmedName, embyPassword),
      {
        successTitle: t("embyPending.createdTitle"),
        errorTitle: t("embyPending.createFailedTitle"),
        onSuccess: async () => {
          await fetchUser({ silent: true });
          setEmbyPassword("");
          dismissedUidRef.current = null;
          setOpen(false);
        },
      },
    );
    setSubmitting(false);
    if (result === null) {
      // 失败时清空密码框，避免重复提交时把不合规密码再发一次。
      setEmbyPassword("");
    }
  };

  const pendingDays = user?.pending_emby_days;
  const daysText = pendingDays === null || pendingDays === undefined
    ? t("embyPending.defaultDays")
    : pendingDays <= 0
      ? t("embyPending.permanent")
      : t("embyPending.days", { days: pendingDays });

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) handleClose();
        else setOpen(true);
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldPlus className="h-5 w-5 text-primary" />
            {t("embyPending.title")}
          </DialogTitle>
          <DialogDescription>
            {t("embyPending.description")}
            <span className="mt-1 block text-xs text-muted-foreground">
              {t("embyPending.duration", { days: daysText })}
            </span>
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-3 py-2">
          <div className="space-y-2">
            <Label htmlFor="pending-emby-name">{t("embyPending.usernameLabel")}</Label>
            <Input
              id="pending-emby-name"
              value={embyUsername}
              onChange={(e) => setEmbyUsername(e.target.value)}
              placeholder={t("embyPending.usernamePlaceholder")}
              disabled={submitting}
              autoComplete="off"
            />
            <p className="text-xs text-muted-foreground">
              {t("embyPending.usernameHelp")}
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="pending-emby-pwd">{t("embyPending.passwordLabel")}</Label>
            <div className="relative">
              <Input
                id="pending-emby-pwd"
                type={showPwd ? "text" : "password"}
                value={embyPassword}
                onChange={(e) => setEmbyPassword(e.target.value)}
                placeholder={t("embyPending.passwordPlaceholder")}
                disabled={submitting}
                className="pr-10"
                autoComplete="new-password"
              />
              <button
                type="button"
                onClick={() => setShowPwd((v) => !v)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                aria-label={t("common.showPassword")}
              >
                {showPwd ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            {embyPassword && (() => {
              const s = validatePasswordStrength(embyPassword, t("embyPending.passwordLabel"));
              return (
                <p className={`text-xs ${s.ok ? passwordStrengthLabel(s.score).className : "text-destructive"}`}>
                  {s.ok ? t("settings.passwordStrength", { label: passwordStrengthLabel(s.score).label }) : s.message}
                </p>
              );
            })()}
          </div>

          <DialogFooter className="gap-2 sm:gap-2">
            <Button
              type="button"
              variant="ghost"
              onClick={handleClose}
              disabled={submitting}
            >
              {t("embyPending.skip")}
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t("embyPending.createAndBind")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
