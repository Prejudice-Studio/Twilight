import { redirect } from "@sveltejs/kit";
import type { PageServerLoad } from "./$types";

export const load: PageServerLoad = ({ locals }) => {
  throw redirect(303, locals.user ? "/dashboard" : "/login");
};
