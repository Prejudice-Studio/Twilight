import { create } from "zustand";
import { persist } from "zustand/middleware";
import { api, type UserInfo } from "@/lib/api";
import { ApiError, clearApiRequestCaches } from "@/lib/api-request";

/**
 * login() 后端响应轻量校验。
 * 之前 `(res.data.user || {}) as Partial<UserInfo>` 直接 spread —— 后端
 * 偶发返回 null / 字符串 / 数组等异常 shape 时会静默生成一个全空 quickUser，
 * 用户体验是 "登录成功但页面卡 Loading 永远等不到 fetchUser"。
 *
 * 这里不引第三方 schema 库（保持 0 新增依赖），只做最关键字段的形状检查：
 *   - 必须是 plain object；
 *   - 至少有 uid（number）+ username（string）这两个核心标识。
 * 校验失败按登录失败处理，并通过 errorCode = "AUTH_USER_PAYLOAD_INVALID"
 * 让前端可以分支提示（即使后端没返回这个码也不会冲突）。
 */
function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function validateUserPayload(raw: unknown): Partial<UserInfo> | null {
  if (!isPlainObject(raw)) return null;
  // uid 与 username 是后端 envelope 的稳定字段，缺任何一项都视作不可用。
  if (typeof raw.uid !== "number" || !Number.isFinite(raw.uid)) return null;
  if (typeof raw.username !== "string" || raw.username === "") return null;
  // 进入这一步时已确认 uid/username 合法；其余字段交给 UserInfo 默认值兜底。
  return raw as Partial<UserInfo>;
}

export interface LoginResult {
  ok: boolean;
  message?: string;
  /**
   * 后端 envelope.error_code，让登录页能基于稳定错误码分支：
   *   - AUTH_ACCOUNT_DISABLED → "账户已禁用"
   *   - AUTH_LOGIN_RATE_LIMITED → "请稍后再试"
   *   - AUTH_LOGIN_INVALID → "用户名或密码错误"
   * 不再依赖 /禁用/.test(message) 这种文案匹配。
   */
  errorCode?: string;
}

/**
 * fetchUser 不再静默吞错。
 * 旧调用方 `void fetchUser()` 完全兼容（直接丢弃返回值），
 * 但需要分支处理网络错 vs 后端 401 的页面（layout / region-refresh）
 * 现在能拿到 errorCode 决定是否触发 logout / 重试。
 */
export interface FetchUserResult {
  success: boolean;
  errorCode?: string;
  /** fetch 抛 TypeError 等没有 errorCode 的网络异常 */
  networkError?: boolean;
}

interface AuthState {
  user: UserInfo | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  /**
   * Zustand `persist` 在客户端是异步从 localStorage 还原的；
   * 该字段用于让布局组件等待还原完成后再判定登录态，
   * 避免出现 "已登录闪烁未登录态" 或反之的水合错位。
   */
  isHydrated: boolean;
  initialize: () => Promise<void>;
  login: (username: string, password: string) => Promise<LoginResult>;
  logout: () => Promise<void>;
  fetchUser: (options?: { silent?: boolean }) => Promise<FetchUserResult>;
  setUser: (user: UserInfo | null) => void;
  /** 由 persist 中间件 onRehydrateStorage 内部调用 */
  setHydrated: () => void;
}

/**
 * In-flight 请求合流锁。
 * 多个 layout effect / region-refresh 会同时触发 initialize() / fetchUser()，
 * 之前会并发打 /users/me，响应乱序导致状态翻车。
 * 这里用共享 Promise 做合流：第二个调用直接复用第一个的 in-flight，任一调用
 * 结束后清空槽位，下一次重新发起。
 *
 * 早期版本把这两个 Promise 放在模块顶层 (`let inFlight* = null`)。模块级状态
 * 在 Vitest 等测试 runner 中跨用例残留 —— 上一个 case 抛异常没 settle，下个
 * case 走 initialize() 直接复用一个永远 pending 的 Promise 整体卡死；同时
 * Next.js RSC 服务端预渲染时模块也被求值，server 上的 Promise 永远无法在
 * client 上 settle。挪进 zustand store 的内部 ref 槽位后：
 *   - 每个 createAuthStore() 创建独立 ref，测试可通过重建 store 重置；
 *   - SSR 阶段不会被求值（store 工厂只在 client 第一次 useAuthStore 时跑）；
 *   - 不进入 partialize，不会被 persist 写到 localStorage。
 */
interface AuthInFlightSlots {
  initialize: Promise<void> | null;
  fetchUser: Promise<FetchUserResult> | null;
  generation: number;
}

const inFlight: AuthInFlightSlots = { initialize: null, fetchUser: null, generation: 0 };

function bumpAuthGeneration(): number {
  inFlight.generation += 1;
  inFlight.initialize = null;
  inFlight.fetchUser = null;
  clearApiRequestCaches();
  return inFlight.generation;
}

/**
 * resetAuthInFlight 仅供测试 setup/teardown 使用：把 in-flight 槽位清空，确保
 * 一个测试用例的悬挂 Promise 不会污染下一个用例的 initialize / fetchUser。
 */
export function resetAuthInFlight(): void {
  inFlight.initialize = null;
  inFlight.fetchUser = null;
  inFlight.generation = 0;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      user: null,
      isAuthenticated: false,
      isLoading: true,
      isHydrated: false,

      initialize: async () => {
        // persist 中间件的 onRehydrateStorage
        // 是异步触发的 —— layout 首个 effect 可能比 setHydrated() 先跑，
        // 此时 get().isAuthenticated 还是 create() 时的初始值 false，
        // 即使 localStorage 里实际为 true 也会被错判为未登录。
        // 这里在入口守 isHydrated，等持久化还原完毕再做决策。
        if (!get().isHydrated) {
          return;
        }
        // 已有 in-flight 初始化：直接复用，避免重复 /users/me。
        if (inFlight.initialize) {
          return inFlight.initialize;
        }
        const run = (async () => {
          // 进入受保护 layout 时总是向后端确认一次会话。不要依赖 localStorage
          // 里的 isAuthenticated 快照：它可能尚未写入、被浏览器禁用或被清理，
          // 但 HttpOnly session cookie 仍然有效。
          await get().fetchUser();
        })();
        let tracked: Promise<void>;
        tracked = run.finally(() => {
          if (inFlight.initialize === tracked) {
            inFlight.initialize = null;
          }
        });
        inFlight.initialize = tracked;
        return tracked;
      },

      login: async (username: string, password: string) => {
        const generation = bumpAuthGeneration();
        try {
          const res = await api.login(username, password);
          if (generation !== inFlight.generation) {
            return { ok: false, message: "登录状态已变化，请重新确认", errorCode: "AUTH_SESSION_CHANGED" };
          }
          if (res.success && res.data) {
            // 校验后端 user payload 的最小形状，
            // 失败时回退为登录失败 + 自定义 errorCode，避免污染 store。
            const baseUser = validateUserPayload((res.data as { user?: unknown }).user);
            if (!baseUser) {
              return {
                ok: false,
                message: "服务器返回的用户信息格式异常",
                errorCode: "AUTH_USER_PAYLOAD_INVALID",
              };
            }
            const quickUser: UserInfo = {
              uid: baseUser.uid as number,
              username: baseUser.username as string,
              email: baseUser.email,
              role: baseUser.role ?? 1,
              role_name: baseUser.role_name || "普通用户",
              active: baseUser.active ?? true,
              expired_at: baseUser.expired_at,
              emby_id: baseUser.emby_id,
              avatar: baseUser.avatar,
              bgm_mode: baseUser.bgm_mode ?? false,
              created_at: baseUser.created_at || new Date().toISOString(),
              telegram_id: baseUser.telegram_id,
              telegram_username: baseUser.telegram_username,
              is_pending: baseUser.is_pending,
              pending_emby: baseUser.pending_emby,
              pending_emby_days: baseUser.pending_emby_days,
            };

            set({ user: quickUser, isAuthenticated: true, isLoading: false });
            void get().fetchUser({ silent: true });
            return { ok: true };
          }
          return { ok: false, message: res.message, errorCode: res.error_code };
        } catch (error: any) {
          // ApiError 经 lib/api-request.ts 抛出，携带 errorCode/backendMessage。
          return {
            ok: false,
            message: error?.backendMessage || error?.message,
            errorCode: error?.errorCode,
          };
        }
      },

      logout: async () => {
        bumpAuthGeneration();
        try {
          await api.logout();
        } finally {
          set({ user: null, isAuthenticated: false, isLoading: false });
          // 清掉持久化快照，防止下一个用本机的人看到上一个账户的状态。
          try {
            useAuthStore.persist.clearStorage();
          } catch {
            // 浏览器禁用 localStorage 时静默失败
          }
        }
      },

      fetchUser: async (options) => {
        const silent = options?.silent ?? false;
        const generation = inFlight.generation;
        // 已有 in-flight /users/me：合流复用。
        // 注意 silent 语义：即使本次是 silent，若已有 non-silent 在跑也直接复用结果。
        if (inFlight.fetchUser) {
          return inFlight.fetchUser;
        }
        const run = (async (): Promise<FetchUserResult> => {
          try {
            if (!silent) {
              set({ isLoading: true });
            }
            const userRes = await api.getMe();
            if (generation !== inFlight.generation) {
              return { success: false, errorCode: "AUTH_SESSION_CHANGED" };
            }
            if (userRes.success && userRes.data) {
              set({ user: userRes.data, isAuthenticated: true, isLoading: false });
              return { success: true };
            }
            // 后端 200 但 envelope.success=false 的极少数路径：保守按未鉴权处理。
            set({ user: null, isAuthenticated: false, isLoading: false });
            return { success: false, errorCode: userRes.error_code };
          } catch (err: unknown) {
            // 关键不变量：只有 server 明确返回 401 才能把 isAuthenticated 翻成 false。
            // fetch 抛 TypeError（DNS / 离线 / CORS preflight 失败）、超时、502/503
            // 这些都是临时故障 —— 用户不应该因为路由器抽筋就被静默踢出登录。
            // 之前一律清空 user/isAuthenticated 会让 layout 立刻跳转 /signin，
            // 用户回来还得重新输密码，体验上比"先卡一下再恢复"差得多。
            const apiErr = err instanceof ApiError ? err : null;
            const isAuthFailure = apiErr?.isAuth() ?? false;
            if (generation !== inFlight.generation) {
              return {
                success: false,
                errorCode: "AUTH_SESSION_CHANGED",
                networkError: apiErr === null,
              };
            }
            if (isAuthFailure) {
              // 服务端权威 401：会话真的失效了，清持久化 + 内存态。
              set({ user: null, isAuthenticated: false, isLoading: false });
              try {
                useAuthStore.persist.clearStorage();
              } catch {
                // 浏览器禁用 localStorage 时静默失败
              }
            } else {
              // 非 401：保留既有 isAuthenticated，只是结束 loading。
              set({ isLoading: false });
            }
            const failure: FetchUserResult = {
              success: false,
              errorCode: apiErr?.errorCode,
              networkError: apiErr === null,
            };
            if (process.env.NODE_ENV !== "production") {
              // dev only：生产环境此处仍静默，避免控制台噪声。
              // eslint-disable-next-line no-console
              console.warn("[auth] fetchUser failed", failure, err);
            }
            return failure;
          }
        })();
        let tracked: Promise<FetchUserResult>;
        tracked = run.finally(() => {
          if (inFlight.fetchUser === tracked) {
            inFlight.fetchUser = null;
          }
        }) as Promise<FetchUserResult>;
        inFlight.fetchUser = tracked;
        return tracked;
      },

      setUser: (user) => {
        set({ user, isAuthenticated: !!user });
      },

      setHydrated: () => {
        set({ isHydrated: true });
      },
    }),
    {
      name: "twilight-auth",
      // 仅持久化登录标志，UserInfo（含 email/telegram_id 等 PII）
      // 永远从 /users/me 拉取，避免落到 localStorage 被本机其他用户/扩展读取。
      partialize: (state) => ({
        isAuthenticated: state.isAuthenticated,
      }),
      onRehydrateStorage: () => (state) => {
        state?.setHydrated();
      },
    }
  )
);

