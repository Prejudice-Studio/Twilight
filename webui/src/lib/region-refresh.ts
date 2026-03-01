type RefreshPayload = unknown;
type RefreshHandler = (payload?: RefreshPayload) => void;

const handlers = new Map<string, Set<RefreshHandler>>();

export function emitRegionRefresh(region: string, payload?: RefreshPayload) {
  const listeners = handlers.get(region);
  if (!listeners || listeners.size === 0) {
    return;
  }
  listeners.forEach((handler) => {
    try {
      handler(payload);
    } catch (error) {
      console.error("region refresh handler error:", error);
    }
  });
}

export function subscribeRegionRefresh(region: string, handler: RefreshHandler) {
  if (!handlers.has(region)) {
    handlers.set(region, new Set());
  }
  handlers.get(region)!.add(handler);

  return () => {
    const listeners = handlers.get(region);
    if (!listeners) return;
    listeners.delete(handler);
    if (listeners.size === 0) {
      handlers.delete(region);
    }
  };
}

export const RegionRefreshKeys = {
  UserProfile: "user:profile",
  UserBackground: "user:background",
  DashboardData: "dashboard:data",
  MediaRequests: "media:requests",
} as const;
