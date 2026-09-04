import { currentUser } from "$lib/server/api";
import type { Handle } from "@sveltejs/kit";

export const handle: Handle = async ({ event, resolve }) => {
  if (event.url.pathname.startsWith("/api/") || event.url.pathname.startsWith("/_app/")) {
    event.locals.user = null;
    return withSecurityHeaders(await resolve(event));
  }
  event.locals.user = await currentUser(event);
  return withSecurityHeaders(await resolve(event));
};

function withSecurityHeaders(response: Response): Response {
  const headers = new Headers(response.headers);
  headers.set("x-content-type-options", "nosniff");
  headers.set("x-frame-options", "DENY");
  headers.set("referrer-policy", "strict-origin-when-cross-origin");
  headers.set("permissions-policy", "camera=(), microphone=(), geolocation=()");
  headers.set("cross-origin-opener-policy", "same-origin");
  headers.set("cross-origin-resource-policy", "same-origin");
  return new Response(response.body, { status: response.status, statusText: response.statusText, headers });
}
