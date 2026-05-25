"use client";

import { useCallback, useState } from "react";

/**
 * useDialogState 把 Dialog 常见的四元组状态合并为单一对象：
 *   - open: 是否展开
 *   - data: 上下文数据（编辑目标、确认对象等）
 *   - loading: 提交中标志
 *   - error: 上次错误消息
 *
 * 替代以下重复模式：
 *   const [open, setOpen] = useState(false);
 *   const [target, setTarget] = useState<X | null>(null);
 *   const [loading, setLoading] = useState(false);
 *   const [error, setError] = useState<string>("");
 *
 * 用法：
 *   const dialog = useDialogState<User>();
 *   dialog.open(user);              // 打开并带数据
 *   dialog.close();                 // 关闭并清空
 *   dialog.setLoading(true);        // 提交中
 *   dialog.setError("保存失败");
 *
 *   <Dialog open={dialog.isOpen} onOpenChange={(v) => !v && dialog.close()}>
 *     {dialog.data && <Form value={dialog.data} ... />}
 *   </Dialog>
 */
export interface DialogState<T> {
  isOpen: boolean;
  data: T | null;
  loading: boolean;
  error: string;
  open: (data?: T | null) => void;
  close: () => void;
  setData: (data: T | null) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string) => void;
  reset: () => void;
}

export function useDialogState<T = unknown>(initialData: T | null = null): DialogState<T> {
  const [isOpen, setIsOpen] = useState(false);
  const [data, setData] = useState<T | null>(initialData);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const open = useCallback((next?: T | null) => {
    if (next !== undefined) {
      setData(next);
    }
    setError("");
    setIsOpen(true);
  }, []);

  const close = useCallback(() => {
    setIsOpen(false);
    setLoading(false);
    setError("");
  }, []);

  const reset = useCallback(() => {
    setIsOpen(false);
    setLoading(false);
    setError("");
    setData(initialData);
  }, [initialData]);

  return {
    isOpen,
    data,
    loading,
    error,
    open,
    close,
    setData,
    setLoading,
    setError,
    reset,
  };
}
