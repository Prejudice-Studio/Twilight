// V2 is an authenticated SSR application. Keep this explicit so an adapter
// or route change cannot silently turn session pages into static HTML.
export const ssr = true;
export const csr = true;
export const prerender = false;
export const trailingSlash = "ignore" as const;
