import { create } from "zustand";
import { persist } from "zustand/middleware";
import { api, type UserInfo } from "@/lib/api";

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
 * 模块级 in-flight 锁。
 * 多个 layout effect / region-refresh 会同时触发 initialize() / fetchUser()，
 * 之前会并发打 /users/me，响应乱序导致状态翻车。
 * 这里用共享 Promise 做合流：第二个调用直接 await 第一个的 in-flight，
 * 不再独立发请求。任一调用结束后清空槽位，下一次重新发起。
 */
let inFlightInitialize: Promise<void> | null = null;
let inFlightFetchUser: Promise<FetchUserResult> | null = null;

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
        if (inFlightInitialize) {
          return inFlightInitialize;
        }
        const run = (async () => {
          // 仅在本地有登录态快照时探测会话，避免未登录场景请求 /users/me。
          // user 已不再持久化（避免 PII 落 localStorage），仅靠 isAuthenticated 判定。
          const { isAuthenticated } = get();
          if (isAuthenticated) {
            await get().fetchUser();
            return;
          }
          set({ user: null, isAuthenticated: false, isLoading: false });
        })();
        inFlightInitialize = run.finally(() => {
          inFlightInitialize = null;
        });
        return inFlightInitialize;
      },

      login: async (username: string, password: string) => {
        try {
          const res = await api.login(username, password);
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
        // 已有 in-flight /users/me：合流复用。
        // 注意 silent 语义：即使本次是 silent，若已有 non-silent 在跑也直接复用结果。
        if (inFlightFetchUser) {
          return inFlightFetchUser;
        }
        const run = (async (): Promise<FetchUserResult> => {
          try {
            if (!silent) {
              set({ isLoading: true });
            }
            const userRes = await api.getMe();
            if (userRes.success && userRes.data) {
              set({ user: userRes.data, isAuthenticated: true, isLoading: false });
              return { success: true };
            }
            set({ user: null, isAuthenticated: false, isLoading: false });
            return { success: false, errorCode: userRes.error_code };
          } catch (err: unknown) {
            // ApiError 由 lib/api-request.ts 抛出，携带 errorCode/backendMessage；
            // 网络错误（fetch 抛 TypeError）则没有 errorCode。
            const apiErr = err as { errorCode?: string } | null;
            set({ user: null, isAuthenticated: false, isLoading: false });
            const failure: FetchUserResult = {
              success: false,
              errorCode: apiErr?.errorCode,
              networkError: !apiErr?.errorCode,
            };
            if (process.env.NODE_ENV !== "production") {
              // dev only：生产环境此处仍静默，避免控制台噪声。
              // eslint-disable-next-line no-console
              console.warn("[auth] fetchUser failed", failure, err);
            }
            return failure;
          }
        })();
        inFlightFetchUser = run.finally(() => {
          inFlightFetchUser = null;
        }) as Promise<FetchUserResult>;
        return inFlightFetchUser;
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

