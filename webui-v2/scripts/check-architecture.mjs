import { readdir, readFile } from "node:fs/promises";
import { join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

const projectRoot = join(fileURLToPath(new URL("..", import.meta.url)));
const legacyRoot = join(projectRoot, "..", "webui", "src", "app");
const v2Root = join(projectRoot, "src");
const packageJSON = JSON.parse(await readFile(join(projectRoot, "package.json"), "utf8"));

const requiredScripts = ["check", "build"];
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
const v2Routes = new Set(
  v2Files
    .filter((path) => path.endsWith("+page.svelte") || path.endsWith("+page.server.ts"))
    .map((path) => routeFromPath(path, join(projectRoot, "src", "routes"), path.endsWith("+page.svelte") ? "+page.svelte" : "+page.server.ts"))
);

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

const rootLayout = await readFile(join(v2Root, "routes", "+layout.ts"), "utf8");
for (const setting of ["ssr = true", "csr = true", "prerender = false"]) {
  if (!rootLayout.includes(setting)) throw new Error(`root SSR layout must keep ${setting}`);
}
const svelteConfig = await readFile(join(projectRoot, "svelte.config.js"), "utf8");
if (!svelteConfig.includes("@sveltejs/adapter-node") || !/adapter\(\)/.test(svelteConfig)) {
  throw new Error("svelte.config.js must configure adapter-node");
}

const forbidden = /(?:from\s*["'](?:react|next|zustand)(?:\/|["'])|require\(\s*["'](?:react|next|zustand))/;
const forbiddenClientRuntime = /\b(?:onMount|EventSource|WebSocket|setInterval)\b/;
const directFetch = /\bfetch\s*\(/;
const violations = [];
for (const path of v2Files) {
  if (!/\.(?:svelte|ts)$/.test(path)) continue;
  const source = await readFile(path, "utf8");
  const label = relative(projectRoot, path).split(sep).join("/");
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

console.log(`OK: ${legacyRoutes.size} legacy route(s) covered; SSR, adapter-node, compatibility redirects, API boundary, and no-polling rules passed.`);
