"use client";

/**
 * 自带的最小化侧栏抽屉
 * --------------------------------------------------
 * 没有引入 shadcn 的 Sheet 组件；这里手写一个超小的覆盖式面板，
 * 用 framer-motion 做入场即可。点击遮罩 / Esc / 右上角 X 都会关闭。
 */

import { ReactNode, useCallback, useEffect } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { X } from "lucide-react";
import { useI18n } from "@/lib/i18n";

export function Sheet({ onClose, children }: { onClose: () => void; children: ReactNode }) {
  const { t } = useI18n();
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [onClose]);

  return (
    <AnimatePresence>
      <motion.div
        className="fixed inset-0 z-50 flex justify-end"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
      >
        <button
          type="button"
          data-sheet-overlay
          aria-label={t("common.close")}
          className="absolute inset-0 bg-background/60 backdrop-blur-[2px]"
          onClick={onClose}
        />
        <motion.div
          initial={{ x: "100%" }}
          animate={{ x: 0 }}
          exit={{ x: "100%" }}
          transition={{ type: "spring", stiffness: 300, damping: 30 }}
          className="relative ml-auto h-full w-full max-w-sm overflow-y-auto border-l bg-background p-5 shadow-2xl"
        >
          {children}
        </motion.div>
      </motion.div>
    </AnimatePresence>
  );
}

export function SheetClose() {
  const { t } = useI18n();
  // 由父级捕获 onClose；这里用一个小按钮触发 propagate up
  // 实际上 Sheet 内的关闭逻辑由父级 onClose 完成，所以这里发一个 click 到背景遮罩。
  // 用稳定的 data-sheet-overlay 属性定位遮罩，避免与可本地化的 aria-label 文案耦合。
  const handler = useCallback(() => {
    const overlay = document.querySelector("[data-sheet-overlay]") as HTMLButtonElement | null;
    overlay?.click();
  }, []);
  return (
    <button
      type="button"
      onClick={handler}
      className="rounded-md p-1.5 text-muted-foreground transition hover:bg-muted hover:text-foreground"
      aria-label={t("common.close")}
    >
      <X className="h-4 w-4" />
    </button>
  );
}
