import { apiJSON } from "$lib/server/api";
import type { PageServerLoad } from "./$types";
import type { V2Capabilities, ViewerCount } from "$lib/types";

export const load: PageServerLoad = async (event) => {
  const [capabilities, viewers] = await Promise.all([
    apiJSON<V2Capabilities>(event, "/api/v2/system/capabilities", { cache: "no-store" }),
    apiJSON<ViewerCount>(event, "/api/v1/system/emby-viewers", { cache: "no-store" })
  ]);
  return {
    capabilities: capabilities?.success ? capabilities.data : null,
    viewers: viewers?.success ? viewers.data?.viewers ?? null : null
  };
};
