import { apiJSON } from "$lib/server/api";
import type { PageServerLoad } from "./$types";
import type { DashboardSummary } from "$lib/types";

export const load: PageServerLoad = async (event) => {
  const result = await apiJSON<DashboardSummary>(event, "/api/v2/dashboard/summary", { cache: "no-store" });
  const summary = result?.success ? result.data || null : null;
  return {
    capabilities: summary?.capabilities || null,
    viewers: summary?.viewers || null,
    user: summary?.user || event.locals.user
  };
};
