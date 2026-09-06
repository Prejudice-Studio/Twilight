import { readdir, readFile } from "node:fs/promises";
import { join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

const projectRoot = join(fileURLToPath(new URL("..", import.meta.url)));
const legacyRoot = join(projectRoot, "..", "webui", "src", "app");
const v2Root = join(projectRoot, "src");
const packageJSON = JSON.parse(await readFile(join(projectRoot, "package.json"), "utf8"));

const requiredScripts = ["check", "build", "verify"];
for (const script of requiredScripts) {
  if (typeof packageJSON.scripts?.[script] !== "string") {
    throw new Error(`webui-v2 package.json is missing the ${script} script`);
  }
}
if (!packageJSON.devDependencies?.["@sveltejs/adapter-node"]) {
  throw new Error("webui-v2 must use @sveltejs/adapter-node for the production SSR server");
}

async function filesUnder(root) {
  const result = [];
  async function visit(directory) {
    for (const entry of await readdir(directory, { withFileTypes: true })) {
      const path = join(directory, entry.name);
      if (entry.isDirectory()) await visit(path);
      else result.push(path);
    }
  }
  await visit(root);
  return result;
}

function routeFromPath(path, root, suffix) {
  let route = relative(root, path).split(sep).join("/");
  route = route.slice(0, -suffix.length);
  route = route.split("/").filter((segment) => !/^\([^/]+\)$/.test(segment)).join("/");
  return route === "." ? "" : route.replace(/^\/+|\/+$/g, "");
}

const legacyFiles = await filesUnder(legacyRoot);
const v2Files = await filesUnder(v2Root);
const legacyRoutes = new Set(
  legacyFiles
    .filter((path) => path.endsWith("page.tsx"))
    .map((path) => routeFromPath(path, legacyRoot, "page.tsx"))
);
const v2RouteRoot = join(projectRoot, "src", "routes");
const v2PageRoutes = new Set(
  v2Files
    .filter((path) => path.endsWith("+page.svelte"))
    .map((path) => routeFromPath(path, v2RouteRoot, "+page.svelte"))
);
const v2ServerRoutes = new Set(
  v2Files
    .filter((path) => path.endsWith("+page.server.ts"))
    .map((path) => routeFromPath(path, v2RouteRoot, "+page.server.ts"))
);
const v2Routes = new Set([...v2PageRoutes, ...v2ServerRoutes]);

const missing = [...legacyRoutes].filter((route) => !v2Routes.has(route)).sort();
if (missing.length) {
  console.error(`V2 route coverage failed; missing ${missing.length} legacy route(s):`);
  for (const route of missing) console.error(`  /${route}`);
  process.exit(1);
}

const compatibilityRoutes = new Map([
  ["admin/stats", "/admin/status"],
  ["admin/test", "/admin/status"],
  ["admin/device-audit", "/admin/emby?tab=devices"],
  ["admin/telegram/commands", "/admin/telegram"],
  ["admin/developer/js-docs", "/admin/developer"],
  ["settings/background", "/settings/appearance"]
]);
const redirectOnlyRoutes = new Set([""]);
const staticSSRRoutes = new Set(["wiki", "admin/security"]);

// A route is only migrated when it has a real Svelte page. Server-only files
// are reserved for explicit compatibility redirects; otherwise a file-only
// route can pass coverage while rendering no user-facing page at all.
const missingPages = [...legacyRoutes]
  .filter((route) => !compatibilityRoutes.has(route) && !redirectOnlyRoutes.has(route) && !v2PageRoutes.has(route))
  .sort();
if (missingPages.length) {
  console.error("V2 page implementation coverage failed; missing real Svelte page(s):");
  for (const route of missingPages) console.error(`  /${route}`);
  process.exit(1);
}

// Every non-static page must have a server boundary. This keeps session reads
// and mutations out of browser-only components while allowing public Wiki and
// the admin security navigation hub to remain static under their layouts.
const missingServerBoundaries = [...v2PageRoutes]
  .filter((route) => !staticSSRRoutes.has(route) && !v2ServerRoutes.has(route))
  .sort();
if (missingServerBoundaries.length) {
  console.error("V2 SSR boundary failed; page(s) missing +page.server.ts:");
  for (const route of missingServerBoundaries) console.error(`  /${route}`);
  process.exit(1);
}

for (const [route, target] of compatibilityRoutes) {
  const routeParts = route.split("/");
  const candidates = [
    join(v2Root, "routes", ...routeParts, "+page.server.ts"),
    join(v2Root, "routes", "(app)", ...routeParts, "+page.server.ts")
  ];
  let source = "";
  for (const candidate of candidates) {
    source = await readFile(candidate, "utf8").catch(() => "");
    if (source) break;
  }
  if (!source || !/\bredirect\(/.test(source)) {
    throw new Error(`compatibility route /${route} must be a server redirect to ${target}`);
  }
}

for (const route of redirectOnlyRoutes) {
  const candidate = join(v2Root, "routes", ...(route ? route.split("/") : []), "+page.server.ts");
  const source = await readFile(candidate, "utf8").catch(() => "");
  if (!source || !/\bredirect\(/.test(source)) {
    throw new Error(`redirect-only route /${route} must be implemented by a server redirect`);
  }
}

const rootLayout = await readFile(join(v2Root, "routes", "+layout.ts"), "utf8");
for (const setting of ["ssr = true", "csr = true", "prerender = false"]) {
  if (!rootLayout.includes(setting)) throw new Error(`root SSR layout must keep ${setting}`);
}
const svelteConfig = await readFile(join(projectRoot, "svelte.config.js"), "utf8");
if (!svelteConfig.includes("@sveltejs/adapter-node") || !/adapter\(\)/.test(svelteConfig)) {
  throw new Error("svelte.config.js must configure adapter-node");
}

const productionEntrypoints = [
  ["README.md", join(projectRoot, "..", "README.md"), /webui-v2/],
  ["docker-compose.yml", join(projectRoot, "..", "docker-compose.yml"), /context:\s+\.[\\/]webui-v2/],
  ["deploy/twilight-webui-v2.service", join(projectRoot, "..", "deploy", "twilight-webui-v2.service"), /webui-v2[\\/]+build/],
  ["deploy/setup-systemd.sh", join(projectRoot, "..", "deploy", "setup-systemd.sh"), /webui-v2/],
  ["deploy/nginx-twilight.conf", join(projectRoot, "..", "deploy", "nginx-twilight.conf"), /127\.0\.0\.1:3001/]
];
for (const [label, path, expected] of productionEntrypoints) {
  const source = await readFile(path, "utf8");
  if (!expected.test(source)) {
    throw new Error(label + " must reference the V2 SSR frontend entrypoint");
  }
  if (/webui\/(?:build|\.next|src[\\/])/.test(source)) {
    throw new Error(label + " still references the legacy frontend runtime");
  }
}

const forbidden = /(?:from\s*["'](?:react|next|zustand)(?:\/|["'])|require\(\s*["'](?:react|next|zustand))/;
const forbiddenClientRuntime = /\b(?:onMount|EventSource|WebSocket|setInterval)\b/;
const directFetch = /\bfetch\s*\(/;
const forbiddenBrowserState = /(?:\{@html\}|\b(?:localStorage|sessionStorage|indexedDB)\b|\bdocument\.cookie\b|\b(?:innerHTML|outerHTML)\b)/;
const violations = [];
for (const path of v2Files) {
  if (!/\.(?:svelte|ts)$/.test(path)) continue;
  const source = await readFile(path, "utf8");
  const label = relative(projectRoot, path).split(sep).join("/");
  if (forbiddenBrowserState.test(source)) violations.push(label + ": unsafe HTML or browser-owned state boundary");
  if (forbidden.test(source)) violations.push(`${label}: legacy frontend dependency`);
  if (directFetch.test(source) && !label.endsWith("src/lib/server/api.ts")) violations.push(`${label}: direct fetch outside SSR API boundary`);
  if (forbiddenClientRuntime.test(source) && !label.endsWith("src/lib/server/api.ts")) {
    violations.push(`${label}: client polling or browser-owned runtime state is forbidden in V2`);
  }
}
if (violations.length) {
  console.error("V2 architecture boundary failed:");
  for (const violation of violations) console.error(`  ${violation}`);
  process.exit(1);
}

console.log(`OK: ${legacyRoutes.size} legacy route(s) covered by ${v2PageRoutes.size} Svelte page(s), ${compatibilityRoutes.size} compatibility redirect(s), and ${redirectOnlyRoutes.size} redirect-only entrypoint(s); SSR, adapter-node, API boundary, and no-polling rules passed.`);
