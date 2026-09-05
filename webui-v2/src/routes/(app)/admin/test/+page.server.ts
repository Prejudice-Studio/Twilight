import { redirect } from "@sveltejs/kit";
import type { PageServerLoad } from "./$types";

// Keep old bookmarks working after server information and health checks merged.
export const load: PageServerLoad = () => {
  throw redirect(308, "/admin/status");
};
