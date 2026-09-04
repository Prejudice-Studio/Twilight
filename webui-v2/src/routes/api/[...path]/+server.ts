import type { RequestHandler } from "./$types";
import { proxyAPIRequest } from "$lib/server/api";

const handle: RequestHandler = (event) => proxyAPIRequest(event);

export const GET = handle;
export const HEAD = handle;
export const POST = handle;
export const PUT = handle;
export const PATCH = handle;
export const DELETE = handle;
