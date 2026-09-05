import { redirect } from "@sveltejs/kit";
import type { PageServerLoad } from "./$types";

// The former stats screen is now part of the single SSR status surface.
export const load: PageServerLoad = () => {
  throw redirect(308, "/admin/status");
};
