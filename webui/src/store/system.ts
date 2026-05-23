import { create } from "zustand";
import { api, type SystemInfo } from "@/lib/api";

interface SystemStore {
  info: SystemInfo | null;
  loaded: boolean;
  fetchInfo: (force?: boolean) => Promise<void>;
}

export const useSystemStore = create<SystemStore>((set, get) => ({
  info: null,
  loaded: false,
  fetchInfo: async (force = false) => {
    if (get().loaded && !force) return;
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
