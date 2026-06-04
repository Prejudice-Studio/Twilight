"use client";

import * as React from "react";
import { AlertTriangle, Loader2 } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button, type ButtonProps } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";

export type ConfirmTone = "default" | "danger" | "warning";

export interface ConfirmDialogAction {
  label: string;
  variant?: ButtonProps["variant"];
  /** 返回该 action 的标识；若 onConfirm 返回 false，弹窗保持打开 */
  value?: string;
  loading?: boolean;
}

export interface ConfirmOptions {
  title: string;
  description?: React.ReactNode;
  /** 详情区，会渲染在描述下方（如要列出注意事项可传 ReactNode） */
  body?: React.ReactNode;
  tone?: ConfirmTone;
  confirmLabel?: string;
  cancelLabel?: string;
  /** 按钮 variant，默认根据 tone 推断 */
  confirmVariant?: ButtonProps["variant"];
  /** 多动作模式：传入按钮列表覆盖默认的"确认/取消"按钮 */
  actions?: ConfirmDialogAction[];
}

interface PendingPrompt extends ConfirmOptions {
  resolve: (value: string | null) => void;
}

interface ConfirmContextValue {
  confirm: (options: ConfirmOptions) => Promise<boolean>;
  /** actions 模式：返回选中按钮的 value（取消返回 null） */
  confirmAction: (options: ConfirmOptions) => Promise<string | null>;
}

const ConfirmContext = React.createContext<ConfirmContextValue | null>(null);

const TONE_ICON: Record<ConfirmTone, React.ReactNode> = {
  default: null,
  warning: <AlertTriangle className="h-5 w-5 text-amber-500" />,
  danger: <AlertTriangle className="h-5 w-5 text-destructive" />,
};

const TONE_TITLE_CLASS: Record<ConfirmTone, string> = {
  default: "",
  warning: "text-amber-600 dark:text-amber-400",
  danger: "text-destructive",
};

export function ConfirmDialogProvider({ children }: { children: React.ReactNode }) {
  const { t } = useI18n();
  const [pending, setPending] = React.useState<PendingPrompt | null>(null);
  const [busyKey, setBusyKey] = React.useState<string | null>(null);

  const close = React.useCallback((value: string | null) => {
    setPending((current) => {
      if (current) current.resolve(value);
      return null;
    });
    setBusyKey(null);
  }, []);

  const confirmAction = React.useCallback(
    (options: ConfirmOptions) =>
      new Promise<string | null>((resolve) => {
        setPending({ ...options, resolve });
      }),
    []
  );

  const confirm = React.useCallback(
    async (options: ConfirmOptions) => {
      const result = await confirmAction(options);
      return result !== null;
    },
    [confirmAction]
  );

  const value = React.useMemo<ConfirmContextValue>(
    () => ({ confirm, confirmAction }),
    [confirm, confirmAction]
  );

  const tone: ConfirmTone = pending?.tone ?? "default";
  const icon = TONE_ICON[tone];

  const actions: ConfirmDialogAction[] = React.useMemo(() => {
    if (!pending) return [];
    if (pending.actions && pending.actions.length > 0) {
      return pending.actions;
    }
    const confirmVariant: ButtonProps["variant"] =
      pending.confirmVariant ?? (tone === "danger" ? "destructive" : "default");
    return [
      {
        label: pending.confirmLabel ?? t("common.confirm"),
        variant: confirmVariant,
        value: "confirm",
      },
    ];
  }, [pending, t, tone]);

  return (
    <ConfirmContext.Provider value={value}>
      {children}
      <Dialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open && !busyKey) close(null);
        }}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle className={cn("flex items-center gap-2", TONE_TITLE_CLASS[tone])}>
              {icon}
              <span>{pending?.title}</span>
            </DialogTitle>
            {pending?.description && (
              <DialogDescription className="pt-1 whitespace-pre-line">
                {pending.description}
              </DialogDescription>
            )}
          </DialogHeader>
          {pending?.body && <div className="text-sm">{pending.body}</div>}
          <DialogFooter className="gap-2 pt-2">
            <Button
              variant="outline"
              onClick={() => close(null)}
              disabled={busyKey !== null}
            >
              {pending?.cancelLabel ?? t("common.cancel")}
            </Button>
            {actions.map((action, index) => {
              const key = action.value ?? `action-${index}`;
              const isBusy = busyKey === key;
              return (
                <Button
                  key={key}
                  variant={action.variant ?? "default"}
                  onClick={() => {
                    setBusyKey(key);
                    close(key);
                  }}
                  disabled={busyKey !== null}
                >
                  {isBusy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  {action.label}
                </Button>
              );
            })}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </ConfirmContext.Provider>
  );
}

export function useConfirm() {
  const ctx = React.useContext(ConfirmContext);
  if (!ctx) {
    throw new Error("useConfirm must be used inside <ConfirmDialogProvider>");
  }
  return ctx;
}
