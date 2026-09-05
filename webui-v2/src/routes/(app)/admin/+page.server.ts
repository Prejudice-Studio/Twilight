import { apiJSON } from "$lib/server/api";
import type { PageServerLoad } from "./$types";
import type { AdminHomePageData, SystemInfo, SystemStats } from "$lib/types";
import { t } from "$lib/i18n";

export const load: PageServerLoad = async (event): Promise<AdminHomePageData> => {
  const results = await Promise.allSettled([
    apiJSON<SystemInfo>(event, "/api/v1/system/info", { cache: "no-store" }),
    apiJSON<SystemStats>(event, "/api/v1/system/admin/stats", { cache: "no-store" })
  ]);
  const infoResult = results[0].status === "fulfilled" ? results[0].value : null;
  const statsResult = results[1].status === "fulfilled" ? results[1].value : null;
  const info = infoResult?.success ? infoResult.data || null : null;
  const stats = statsResult?.success ? statsResult.data || null : null;
  return {
    info,
    stats,
    loadError: info && stats ? null : t.adminHomeLoadFailed
  };
};
