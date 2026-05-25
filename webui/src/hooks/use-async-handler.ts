"use client";

import { useCallback } from "react";
import { useToast } from "@/hooks/use-toast";
import type { ApiResponse } from "@/lib/api-types";
import { ApiError } from "@/lib/api-request";
import { friendlyError } from "@/lib/validators";
import { isKnownErrCode, type ErrCode } from "@/lib/errcode";

/**
 * useAsyncHandler 用于把"调用 API → 错误处理 → toast → 成功回调"这一三段式样板
 * 集中到一个 Hook 中，避免每个页面 try/catch + toast 重复 30+ 处。
 *
 * 用法（最常见，调用 api.* 返回 ApiResponse 包裹）：
 *
 *   const handle = useAsyncHandler();
 *   const onSubmit = () => handle(() => api.updateMe({...}), {
 *     successTitle: "已保存",
 *     errorTitle: "保存失败",
 *     onSuccess: (data) => setUser(data),
 *   });
 *
 * 错误处理优先级：
 *   1. errorCodeMap[res.error_code]                 — 调用方传入的局部覆盖
 *   2. friendlyError(error_code)                    — 全局 lib/validators.ts 映射
 *   3. ApiError.status 自动归类（401/403/429/5xx）  — 用户友好兜底
 *   4. res.message / error.message                  — 后端中文文案
 *   5. errorTitle                                   — 兜底
 *
 * 这样所有调用 useAsyncHandler 的页面都自动获得 error_code → 友好文案的映射，
 * 不需要逐页改 catch 分支；后端切英文 / 加新错误码时也只在 validators.ts 修。
 *
 * 返回值：成功且有 data 时返回 data；失败/异常时返回 null。
 * 不会重新抛出异常 —— 调用方根据返回值是否为 null 判断成败。
 */
export interface AsyncHandlerOptions<T> {
  successTitle?: string;
  successDescription?: string;
  errorTitle?: string;
  silent?: boolean;
  silentSuccess?: boolean;
  /**
   * 局部错误码覆盖：仅当 error_code 命中时显示自定义文案。
   * 类型从宽松 `Record<string, string>`
   * 收紧到 `Partial<Record<ErrCode, string>>`，让 TS 能在编译期发现拼写
   * 错误，并和 `lib/validators.ts` 的 ERROR_CODE_FRIENDLY 类型保持一致。
   * 后端新增码时由 `errcode.ts` 的镜像枚举强制提示前端补 friendly 文案。
   */
  errorCodeMap?: Partial<Record<ErrCode, string>>;
  onSuccess?: (data: T) => void | Promise<void>;
  onError?: (error: Error | ApiResponse<unknown>) => void;
}

/** 把 ApiError.status 归类为用户友好的 fallback 文案。 */
function statusFallback(status: number | undefined): string | null {
  if (status === 401) return "登录态已失效，请重新登录";
  if (status === 403) return "权限不足";
  if (status === 404) return "请求的资源不存在";
  if (status === 409) return "操作冲突，请刷新后重试";
  if (status === 413) return "上传内容过大";
  if (status === 429) return "请求过于频繁，请稍后再试";
  if (typeof status === "number" && status >= 500) {
    return "服务器开小差了，请稍后再试";
  }
  return null;
}

/**
 * 把 errorCodeMap / friendlyError 的查询路径
 * 都通过 isKnownErrCode 类型守卫窄化。
 *   - errorCodeMap[code] 仅在 code ∈ ErrCode 时有意义；
 *   - friendlyError 也仅当 code 是已知 ErrCode 时才尝试，未知码直接走兜底，
 *     避免运行时偶发匹配到无意义 key。
 */
function resolveCodeText(
  code: string | undefined,
  errorCodeMap: AsyncHandlerOptions<unknown>["errorCodeMap"],
): string | null {
  if (!code || !isKnownErrCode(code)) return null;
  if (errorCodeMap?.[code]) return errorCodeMap[code] ?? null;
  const friendly = friendlyError(code, "");
  if (friendly && friendly !== "操作失败") return friendly;
  return null;
}

function describeError(
  err: Error | ApiResponse<unknown>,
  options: AsyncHandlerOptions<unknown>,
  defaultMsg: string,
): string {
  // ApiError 路径：status + error_code 都齐全
  if (err instanceof ApiError) {
    const friendly = resolveCodeText(err.errorCode, options.errorCodeMap);
    if (friendly) return friendly;
    const fallback = statusFallback(err.status);
    if (fallback) return fallback;
    return err.backendMessage || err.message || defaultMsg;
  }
  // ApiResponse envelope 路径：res.success === false
  if (typeof err === "object" && err !== null && "success" in err) {
    const env = err as ApiResponse<unknown>;
    const friendly = resolveCodeText(env.error_code, options.errorCodeMap);
    if (friendly) return friendly;
    return env.message || defaultMsg;
  }
  // 普通 Error / 网络错误
  if (err instanceof Error && err.message) return err.message;
  return defaultMsg;
}

export function useAsyncHandler() {
  const { toast } = useToast();

  return useCallback(
    async <T>(
      fn: () => Promise<ApiResponse<T>>,
      options: AsyncHandlerOptions<T> = {},
    ): Promise<T | null> => {
      const {
        successTitle,
        successDescription,
        errorTitle = "操作失败",
        silent = false,
        silentSuccess = false,
        onSuccess,
        onError,
      } = options;
      try {
        const res = await fn();
        if (res.success) {
          if (!silent && !silentSuccess && successTitle) {
            toast({ title: successTitle, description: successDescription });
          }
          if (res.data !== undefined && onSuccess) {
            await onSuccess(res.data as T);
          }
          return (res.data as T | undefined) ?? null;
        }
        if (!silent) {
          toast({
            title: errorTitle,
            description: describeError(res, options as AsyncHandlerOptions<unknown>, "请求失败"),
            variant: "destructive",
          });
        }
        onError?.(res);
        return null;
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        if (!silent) {
          toast({
            title: errorTitle,
            description: describeError(error, options as AsyncHandlerOptions<unknown>, "网络异常"),
            variant: "destructive",
          });
        }
        onError?.(error);
        return null;
      }
    },
    [toast],
  );
}
