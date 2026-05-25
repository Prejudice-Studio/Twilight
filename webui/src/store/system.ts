import { create } from "zustand";
import { api, type SystemInfo } from "@/lib/api";
import { setCsrfCookieName } from "@/lib/api-request";

/**
 * fetchInfo 不再静默吞错，
 * 给调用方一个 {success, errorCode?} 形状以便：
 *   - dev 环境定位 backend 异常 vs 网络失败；
 *   - 关键页面（如 /admin/config）可以决定是否重试；
 *   - 旧调用方 `void fetchInfo()` 完全兼容（直接丢弃返回值）。
 */
export interface SystemFetchResult {
  success: boolean;
  errorCode?: string;
  /** 标记是否是网络异常（区别于后端 4xx），便于上层做退避策略 */
  networkError?: boolean;
}

interface SystemStore {
  info: SystemInfo | null;
  loaded: boolean;
  /**
   * 上次 fetchInfo 的失败信息：成功后会被清空。
   * 区分 loaded（成功拉过一次）vs lastError（拉取过但失败），见 41.10 / batch 09。
   */
  lastError: SystemFetchResult | null;
  fetchInfo: (force?: boolean) => Promise<SystemFetchResult>;
}

export const useSystemStore = create<SystemStore>((set, get) => ({
  info: null,
  loaded: false,
  lastError: null,
  fetchInfo: async (force = false) => {
    if (get().loaded && !force) {
      return { success: true };
    }
    try {
      const res = await api.getSystemInfo();
      if (res.success && res.data) {
        // 把后端公开的 csrf cookie 名注入到 api-request 模块缓存里。
        // 之后 readCSRFCookie() 会按精确名匹配，避免同域 / 父域第三方
        // 应用下发的其它 *_csrf cookie 把 token 取错。
        setCsrfCookieName(res.data.csrf_cookie_name ?? null);
        set({ info: res.data, loaded: true, lastError: null });
        return { success: true };
      }
      const failure: SystemFetchResult = {
        success: false,
        errorCode: res.error_code,
      };
      set({ lastError: failure });
      return failure;
    } catch (err: unknown) {
      // ApiError 由 lib/api-request.ts 抛出，携带 errorCode/backendMessage；
      // 网络错误（fetch 抛 TypeError）则没有 errorCode。
      const apiErr = err as { errorCode?: string } | null;
      const failure: SystemFetchResult = {
        success: false,
        errorCode: apiErr?.errorCode,
        networkError: !apiErr?.errorCode,
      };
      set({ lastError: failure });
      if (process.env.NODE_ENV !== "production") {
        // dev only：在生产构建里 zap 日志走后端，避免噪声。
        // eslint-disable-next-line no-console
        console.warn("[system] fetchInfo failed", failure, err);
      }
      return failure;
    }
  },
}));
