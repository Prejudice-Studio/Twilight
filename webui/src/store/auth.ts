import { create } from "zustand";
import { persist } from "zustand/middleware";
import { api, type User, type UserInfo } from "@/lib/api";

interface AuthState {
  user: UserInfo | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (username: string, password: string) => Promise<boolean>;
  logout: () => void;
  fetchUser: () => Promise<void>;
  setUser: (user: UserInfo | null) => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      user: null,
      isAuthenticated: false,
      isLoading: true,

      login: async (username: string, password: string) => {
        try {
          const res = await api.login(username, password);
          if (res.success && res.data) {
            await get().fetchUser();
            return true;
          }
          return false;
        } catch {
          return false;
        }
      },

      logout: () => {
        api.logout();
        set({ user: null, isAuthenticated: false });
      },

      fetchUser: async () => {
        try {
          set({ isLoading: true });
          const res = await api.getMe();
          if (res.success && res.data) {
            set({ user: res.data, isAuthenticated: true, isLoading: false });
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
      partialize: (state) => ({ isAuthenticated: state.isAuthenticated }),
    }
  )
);

