import { create } from "zustand";
import { persist } from "zustand/middleware";
import { api, type UserInfo } from "@/lib/api";

export interface LoginResult {
  ok: boolean;
  message?: string;
}

interface AuthState {
  user: UserInfo | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  initialize: () => Promise<void>;
  login: (username: string, password: string) => Promise<LoginResult>;
  logout: () => Promise<void>;
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
        // 仅在本地有登录态快照时探测会话，避免未登录场景请求 /users/me
        const { isAuthenticated, user } = get();
        if (isAuthenticated || !!user?.uid) {
          await get().fetchUser();
          return;
        }

        set({ user: null, isAuthenticated: false, isLoading: false });
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
          return { ok: false, message: res.message };
        } catch (error: any) {
          return { ok: false, message: error?.message };
        }
      },

      logout: async () => {
        await api.logout();
        set({ user: null, isAuthenticated: false, isLoading: false });
      },

      fetchUser: async (options) => {
        const silent = options?.silent ?? false;
        try {
          if (!silent) {
            set({ isLoading: true });
          }
          const userRes = await api.getMe();
          
          if (userRes.success && userRes.data) {
            set({ user: userRes.data, isAuthenticated: true, isLoading: false });
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

