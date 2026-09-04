import { apiJSONWithResponse } from "$lib/server/api";
import type { AdminStatusPageData, HealthProbe, SystemHealthDetail, SystemInfo, SystemStats } from "$lib/types";
import type { PageServerLoad } from "./$types";

interface ReadResult<T> extends HealthProbe<T> {}

async function read<T>(event: Parameters<PageServerLoad>[0], path: string): Promise<ReadResult<T>> {
  try {
    const result = await apiJSONWithResponse<T>(event, path, { cache: "no-store" });
    if (!result || !result.response.ok || !result.envelope?.success || result.envelope.data === undefined) {
      return { available: false, data: null };
    }
    return { available: true, data: result.envelope.data };
  } catch {
    return { available: false, data: null };
  }
}

function settled<T>(result: PromiseSettledResult<ReadResult<T>>): ReadResult<T> {
  return result.status === "fulfilled" ? result.value : { available: false, data: null };
}

export const load: PageServerLoad = async (event): Promise<AdminStatusPageData> => {
  const results = await Promise.allSettled([
    read<SystemHealthDetail>(event, "/api/v1/system/health/api"),
    read<SystemHealthDetail>(event, "/api/v1/system/health/database"),
    read<SystemHealthDetail>(event, "/api/v1/system/health/emby"),
    read<SystemInfo>(event, "/api/v1/system/info"),
    read<SystemStats>(event, "/api/v1/system/admin/stats")
  ]);

  return {
    health: {
      api: settled(results[0] as PromiseSettledResult<ReadResult<SystemHealthDetail>>),
      database: settled(results[1] as PromiseSettledResult<ReadResult<SystemHealthDetail>>),
      emby: settled(results[2] as PromiseSettledResult<ReadResult<SystemHealthDetail>>)
    },
    info: settled(results[3] as PromiseSettledResult<ReadResult<SystemInfo>>).data,
    stats: settled(results[4] as PromiseSettledResult<ReadResult<SystemStats>>).data,
    refreshed_at: Math.floor(Date.now() / 1000)
  };
};
