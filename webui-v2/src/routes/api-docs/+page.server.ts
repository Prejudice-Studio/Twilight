import { apiJSON } from "$lib/server/api";
import type { ApiDocRoute, ApiDocsPageData } from "$lib/types";
import type { PageServerLoad } from "./$types";

type OpenAPIDocument = {
  paths?: Record<string, Record<string, unknown>>;
};

type AdminRoutePayload = {
  apis?: Array<Partial<ApiDocRoute>>;
};

const methods = new Set(["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"]);

function versionOf(path: string): ApiDocRoute["version"] {
  if (path === "/api/v2" || path.startsWith("/api/v2/")) return "v2";
  if (path === "/api/v1" || path.startsWith("/api/v1/")) return "v1";
  return "other";
}

function publicRoutes(document: OpenAPIDocument | undefined): ApiDocRoute[] {
  return Object.entries(document?.paths || {}).flatMap(([path, operations]) =>
    Object.keys(operations || {})
      .filter((method) => methods.has(method.toUpperCase()))
      .map((method) => ({ method: method.toUpperCase(), path, auth: "Public", version: versionOf(path) }))
  );
}

function adminRoutes(payload: AdminRoutePayload | undefined): ApiDocRoute[] {
  return (payload?.apis || []).flatMap((item) => {
    const path = typeof item.path === "string" ? item.path : "";
    const method = typeof item.method === "string" ? item.method.toUpperCase() : "";
    if (!path || !methods.has(method)) return [];
    return [{ method, path, auth: item.auth || "Public", version: item.version || versionOf(path) }];
  });
}

function filterRoutes(routes: ApiDocRoute[], search: string, method: string, auth: string, version: string): ApiDocRoute[] {
  const needle = search.toLocaleLowerCase("zh-CN");
  return routes
    .filter((route) => !method || route.method === method)
    .filter((route) => !auth || route.auth === auth)
    .filter((route) => !version || route.version === version)
    .filter((route) => {
      if (!needle) return true;
      return `${route.method} ${route.path} ${route.auth} ${route.version}`.toLocaleLowerCase("zh-CN").includes(needle);
    })
    .sort((left, right) => left.path.localeCompare(right.path) || left.method.localeCompare(right.method));
}

export const load: PageServerLoad = async (event): Promise<ApiDocsPageData> => {
  const search = event.url.searchParams.get("q")?.trim().slice(0, 120) || "";
  const method = event.url.searchParams.get("method")?.trim().toUpperCase() || "";
  const auth = event.url.searchParams.get("auth")?.trim() || "";
  const version = event.url.searchParams.get("version")?.trim() || "";

  const publicEnvelope = await apiJSON<OpenAPIDocument>(event, "/api/v2/openapi.json", { cache: "no-store" });
  const publicList = publicRoutes(publicEnvelope?.success ? publicEnvelope.data : undefined);
  if (!event.locals.user || event.locals.user.role !== 0) {
    return {
      routes: filterRoutes(publicList, search, method, auth, version),
      source: "public",
      total: publicList.length,
      query: { search, method, auth, version },
      loadError: publicEnvelope?.success ? null : "API 文档暂时无法读取，请稍后重试。"
    };
  }

  const adminEnvelope = await apiJSON<AdminRoutePayload>(event, "/api/v2/admin/docs/routes", { cache: "no-store" });
  const allRoutes = adminEnvelope?.success ? adminRoutes(adminEnvelope.data) : [];
  if (!allRoutes.length) {
    return {
      routes: filterRoutes(publicList, search, method, auth, version),
      source: "public",
      total: publicList.length,
      query: { search, method, auth, version },
      loadError: "完整接口清单暂时无法读取，当前显示公开接口。"
    };
  }

  return {
    routes: filterRoutes(allRoutes, search, method, auth, version),
    source: "admin",
    total: allRoutes.length,
    query: { search, method, auth, version },
    loadError: null
  };
};
