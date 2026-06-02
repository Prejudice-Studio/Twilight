import { create } from "zustand";
import { api, type SystemInfo } from "@/lib/api";

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

/**
 * SystemInfo TTL（毫秒）。loaded=true 之后默认 5 分钟内复用缓存；
 * 超过 TTL 的下一次 fetchInfo() 会自动重新拉取。
 *
 * 主动失效路径走 invalidate()（admin 保存配置 / 上传 server-icon 后调用），
 * TTL 作为兜底，避免 admin 忘了调 invalidate 的场景。
 */
const SYSTEM_INFO_TTL_MS = 5 * 60 * 1000;

interface SystemStore {
  info: SystemInfo | null;
  loaded: boolean;
  /**
   * 上次 fetchInfo 的失败信息：成功后会被清空。
   * 区分 loaded（成功拉过一次）vs lastError（拉取过但失败），见 41.10 / batch 09。
   */
  lastError: SystemFetchResult | null;
  /** 上次成功 fetch 的时间戳；TTL 过期后强制重拉 */
  fetchedAt: number;
  /**
   * 当前正在飞行的 fetchInfo Promise。两个组件同帧 mount 都会调用 fetchInfo()，
   * 没有这把锁就会触发两次完全相同的 GET /api/system/info：浪费请求 +
   * 任意一次失败都会污染 lastError。拿到 promise 后 awaiter 共用同一个结果。
   */
  inflight: Promise<SystemFetchResult> | null;
  fetchInfo: (force?: boolean) => Promise<SystemFetchResult>;
  /**
   * 主动失效：admin 修改可能影响 systemInfo（server_icon、registration_enabled
   * 等公开字段）的设置后调用。
   * 不直接 fetch，下一次 fetchInfo() 会重新走网络。
   */
  invalidate: () => void;
}

export const useSystemStore = create<SystemStore>((set, get) => ({
  info: null,
  loaded: false,
  lastError: null,
  fetchedAt: 0,
  inflight: null,
  invalidate: () => {
    set({ loaded: false, fetchedAt: 0 });
  },
  fetchInfo: async (force = false) => {
    const { loaded, fetchedAt, inflight } = get();
    const fresh = loaded && Date.now() - fetchedAt < SYSTEM_INFO_TTL_MS;
    if (fresh && !force) {
      return { success: true };
    }
    // force=true 仍然要让出给已经在飞的请求 —— 重复 force 调用是 admin
    // 在配置页连点保存按钮时常见的边界，复用同一次拉取避免雪崩。
    if (inflight) {
      return inflight;
    }
    const promise = (async (): Promise<SystemFetchResult> => {
      try {
		const res = await api.getSystemInfo();
		if (res.success && res.data) {
		  set({
            info: res.data,
            loaded: true,
            lastError: null,
            fetchedAt: Date.now(),
          });
          return { success: true };
        }
		// 后端 200 但 envelope.success=false 的极少数路径：保留失败状态。
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
      } finally {
        // 必须放 finally：无论 success 还是 throw，都必须把 inflight slot 让出来，
        // 否则下次 fetchInfo 永远拿到同一个旧 promise，TTL 也救不回来。
        set({ inflight: null });
      }
    })();
    set({ inflight: promise });
    return promise;
  },
}));
