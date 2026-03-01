import { create } from "zustand";
import { persist } from "zustand/middleware";
import { api, type UserInfo } from "@/lib/api";

interface AuthState {
  user: UserInfo | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  initialize: () => Promise<void>;
  login: (username: string, password: string) => Promise<boolean>;
  logout: () => void;
  fetchUser: (options?: { silent?: boolean }) => Promise<void>;
  setUser: (user: UserInfo | null) => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      user: null,
      isAuthenticated: false,
      isLoading: true,

      initialize: async () => {
        if (!api.hasToken()) {
          set({ user: null, isAuthenticated: false, isLoading: false });
          return;
        }
        await get().fetchUser();
      },

      login: async (username: string, password: string) => {
        try {
          const res = await api.login(username, password);
          if (res.success && res.data) {
            const baseUser = (res.data.user || {}) as Partial<UserInfo>;
            const quickUser: UserInfo = {
              uid: baseUser.uid || 0,
              username: baseUser.username || username,
              email: baseUser.email,
              role: baseUser.role ?? 1,
              role_name: baseUser.role_name || "普通用户",
              active: baseUser.active ?? true,
              expired_at: baseUser.expired_at,
              emby_id: baseUser.emby_id,
              avatar: baseUser.avatar,
              score: baseUser.score ?? 0,
              auto_renew: baseUser.auto_renew ?? false,
              bgm_mode: baseUser.bgm_mode ?? false,
              nsfw: baseUser.nsfw ?? false,
              created_at: baseUser.created_at || new Date().toISOString(),
              telegram_id: baseUser.telegram_id,
              telegram_username: baseUser.telegram_username,
              is_pending: baseUser.is_pending,
            };

            set({ user: quickUser, isAuthenticated: true, isLoading: false });
            void get().fetchUser({ silent: true });
            return true;
          }
          return false;
        } catch {
          return false;
        }
      },

      logout: () => {
        api.logout();
        set({ user: null, isAuthenticated: false, isLoading: false });
      },

      fetchUser: async (options) => {
        const silent = options?.silent ?? false;
        try {
          if (!silent) {
            set({ isLoading: true });
          }
          const [userRes, scoreRes] = await Promise.all([
            api.getMe(),
            api.getScoreInfo().catch(() => ({ success: false, data: null }))
          ]);
          
          if (userRes.success && userRes.data) {
            const userData = userRes.data;
            // 如果获取到了积分信息，则合并到用户数据中
            if (scoreRes.success && scoreRes.data) {
              userData.score = scoreRes.data.balance;
            }
            set({ user: userData, isAuthenticated: true, isLoading: false });
          } else {
            set({ user: null, isAuthenticated: false, isLoading: false });
          }
        } catch {
          set({ user: null, isAuthenticated: false, isLoading: false });
        }
      },

      setUser: (user) => {
        set({ user, isAuthenticated: !!user });
      },
    }),
    {
      name: "twilight-auth",
      partialize: (state) => ({
        isAuthenticated: state.isAuthenticated,
        user: state.user,
      }),
    }
  )
);

