import { currentUser } from "$lib/server/api";
import type { Handle } from "@sveltejs/kit";

export const handle: Handle = async ({ event, resolve }) => {
  if (event.url.pathname.startsWith("/api/") || event.url.pathname.startsWith("/_app/")) {
    event.locals.user = null;
    return withSecurityHeaders(await resolve(event), event.url.pathname);
  }
  event.locals.user = await currentUser(event);
  return withSecurityHeaders(await resolve(event), event.url.pathname);
};

function withSecurityHeaders(response: Response, pathname: string): Response {
  const headers = new Headers(response.headers);
  const isImmutableAsset = pathname.startsWith("/_app/immutable/");
  headers.set("x-content-type-options", "nosniff");
  headers.set("x-frame-options", "DENY");
  headers.set("referrer-policy", "strict-origin-when-cross-origin");
  headers.set("permissions-policy", "camera=(), microphone=(), geolocation=()");
  headers.set("cross-origin-opener-policy", "same-origin");
  headers.set("cross-origin-resource-policy", "same-origin");
  headers.set("cache-control", isImmutableAsset ? "public, max-age=31536000, immutable" : "no-store");
  return new Response(response.body, { status: response.status, statusText: response.statusText, headers });
}
