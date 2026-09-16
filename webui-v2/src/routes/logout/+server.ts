import { redirect } from "@sveltejs/kit";
import { apiRequest, copySetCookies } from "$lib/server/api";
import type { RequestHandler } from "./$types";

export const POST: RequestHandler = async (event) => {
  const response = await apiRequest(event, "/api/v2/auth/logout", { method: "POST" });
  copySetCookies(event, response);
  throw redirect(303, "/login");
};
