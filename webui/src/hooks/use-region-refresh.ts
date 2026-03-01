import { useEffect } from "react";
import { subscribeRegionRefresh } from "@/lib/region-refresh";

export function useRegionRefresh(region: string, handler: (payload?: unknown) => void) {
  useEffect(() => {
    return subscribeRegionRefresh(region, handler);
  }, [region, handler]);
}
