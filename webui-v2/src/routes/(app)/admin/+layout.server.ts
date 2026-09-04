import { error, redirect } from "@sveltejs/kit";
import type { LayoutServerLoad } from "./$types";

export const load: LayoutServerLoad = ({ locals }) => {
  if (!locals.user) throw redirect(303, "/login");
  if (locals.user.role !== 0) throw error(403, "需要管理员权限");
  return { user: locals.user };
};
