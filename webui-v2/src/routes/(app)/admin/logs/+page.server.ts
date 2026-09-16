import { apiJSON } from "$lib/server/api";
import type { PageServerLoad } from "./$types";
import type { AdminRuntimeLogsPageData, RuntimeLogsResponse, RuntimeStatus } from "$lib/types";
import { t } from "$lib/i18n";

function limitValue(value: string | null): number {
  const parsed = Number(value || "500");
  return [100, 200, 500, 1000, 2000].includes(parsed) ? parsed : 500;
}

export const load: PageServerLoad = async (event): Promise<AdminRuntimeLogsPageData> => {
  const limit = limitValue(event.url.searchParams.get("limit"));
  const results = await Promise.allSettled([
    apiJSON<RuntimeStatus>(event, "/api/v2/admin/runtime/status", { cache: "no-store" }),
    apiJSON<RuntimeLogsResponse>(event, `/api/v2/admin/runtime/logs?limit=${limit}`, { cache: "no-store" })
  ]);
  const statusResult = results[0].status === "fulfilled" ? results[0].value : null;
  const logsResult = results[1].status === "fulfilled" ? results[1].value : null;
  const status = statusResult?.success ? statusResult.data || null : null;
  const logs = logsResult?.success ? logsResult.data || null : null;
  return {
    status,
    logs,
    limit,
    loadError: status && logs ? null : t.adminRuntimeLogsReadFailed
  };
};
