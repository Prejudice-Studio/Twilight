import { create } from "zustand";
import { api, type SystemInfo } from "@/lib/api";

interface SystemStore {
  info: SystemInfo | null;
  loaded: boolean;
  fetchInfo: () => Promise<void>;
}

export const useSystemStore = create<SystemStore>((set, get) => ({
  info: null,
  loaded: false,
  fetchInfo: async () => {
    if (get().loaded) return;
    try {
      const res = await api.getSystemInfo();
      if (res.success && res.data) {
        set({ info: res.data, loaded: true });
      }
    } catch {
      // ignore - use defaults
    }
  },
}));
