package api

import (
	"net/http"
	"strings"
)

// handleV2OpenAPI keeps the public specification under the versioned V2
// contract. The shared generator intentionally exposes only public routes;
// administrator routes are available through the authenticated inventory
// below so an anonymous visitor cannot enumerate the private attack surface.
func (a *App) handleV2OpenAPI(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleOpenAPI(w, r, nil)
}

// handleV2AdminAPIRoutes is the SSR documentation page's private route
// inventory. It returns metadata only: no handlers, source locations, config,
// or request/response secrets are serialized.
func (a *App) handleV2AdminAPIRoutes(w http.ResponseWriter, _ *http.Request, _ Params) {
	items := make([]map[string]string, 0, len(a.routes))
	for _, route := range a.routes {
		items = append(items, map[string]string{
			"method":   route.Method,
			"path":     route.Pattern,
			"endpoint": route.Pattern,
			"auth":     authLevelName(route.Auth),
			"version":  routeVersion(route.Pattern),
		})
	}
	ok(w, "OK", map[string]any{"apis": items, "total": len(items)})
}

func authLevelName(level AuthLevel) string {
	switch level {
	case AuthUser:
		return "User"
	case AuthAdmin:
		return "Admin"
	case AuthAPIKey:
		return "API Key"
	default:
		return "Public"
	}
}

func routeVersion(pattern string) string {
	if strings.HasPrefix(pattern, "/api/v2/") || pattern == "/api/v2" {
		return "v2"
	}
	if strings.HasPrefix(pattern, "/api/v1/") || pattern == "/api/v1" {
		return "v1"
	}
	return "other"
}
