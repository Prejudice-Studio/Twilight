package api

import (
	"net/http"
	"testing"
)

func TestRouteIndexPreservesMatchingAndMethodAllowed(t *testing.T) {
	app := &App{}
	app.add(http.MethodGet, "/api/v1/users/me", AuthUser, nil)
	app.add(http.MethodGet, "/api/v1/users/:uid", AuthUser, nil)
	app.add(http.MethodPost, "/api/v1/users/:uid", AuthAdmin, nil)

	route, params, ok := app.match(http.MethodGet, "/api/v1/users/42")
	if !ok || route == nil || route.Pattern != "/api/v1/users/:uid" || params["uid"] != "42" {
		t.Fatalf("parameter route mismatch: route=%+v params=%v ok=%v", route, params, ok)
	}

	route, _, methodAllowed := app.match(http.MethodDelete, "/api/v1/users/42")
	if route != nil || !methodAllowed {
		t.Fatalf("method mismatch: route=%+v methodAllowed=%v", route, methodAllowed)
	}

	route, _, methodAllowed = app.match(http.MethodGet, "/api/v1/unknown/42")
	if route != nil || methodAllowed {
		t.Fatalf("unknown path mismatch: route=%+v methodAllowed=%v", route, methodAllowed)
	}
}

func TestRouteIndexPreservesWildcardGroupRegistrationOrder(t *testing.T) {
	app := &App{}
	app.add(http.MethodGet, "/api/v1/:domain/detail", AuthPublic, nil)
	app.add(http.MethodGet, "/api/v1/users/detail", AuthUser, nil)

	route, params, ok := app.match(http.MethodGet, "/api/v1/users/detail")
	if !ok || route == nil || route.Pattern != "/api/v1/:domain/detail" || params["domain"] != "users" {
		t.Fatalf("registration order changed: route=%+v params=%v ok=%v", route, params, ok)
	}
}

func TestSplitPathNormalizesEquivalentInputs(t *testing.T) {
	want := []string{"api", "v1", "users", "me"}
	for _, input := range []string{
		"api/v1/users/me",
		"/api/v1/users/me",
		"//api//v1/users/./me",
		"/api/v1/admin/../users/me/",
	} {
		got := splitPath(input)
		if len(got) != len(want) {
			t.Fatalf("splitPath(%q) = %v, want %v", input, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("splitPath(%q) = %v, want %v", input, got, want)
			}
		}
	}
	if got := splitPath(""); got != nil {
		t.Fatalf("splitPath(empty) = %v, want nil", got)
	}
	if got := splitPath("/"); got != nil {
		t.Fatalf("splitPath(root) = %v, want nil", got)
	}
}

func BenchmarkRouteIndexMatch(b *testing.B) {
	app := benchmarkRouteApp()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		route, params, ok := app.match(http.MethodGet, "/api/v1/admin/target/42/action")
		if !ok || route == nil || params["uid"] != "42" {
			b.Fatal("route did not match")
		}
	}
}

func BenchmarkRouteLinearMatch(b *testing.B) {
	app := benchmarkRouteApp()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		requestParts := splitPath("/api/v1/admin/target/42/action")
		var matched *Route
		var params Params
		for routeIndex := range app.routes {
			route := &app.routes[routeIndex]
			candidate, ok := matchPattern(route.Parts, requestParts)
			if ok && route.Method == http.MethodGet {
				matched, params = route, candidate
				break
			}
		}
		if matched == nil || params["uid"] != "42" {
			b.Fatal("route did not match")
		}
	}
}

func benchmarkRouteApp() *App {
	app := &App{}
	for i := 0; i < 100; i++ {
		app.add(http.MethodGet, "/api/v1/admin/domain"+string(rune('a'+i%26))+"/:uid/action", AuthAdmin, nil)
		app.add(http.MethodPost, "/api/v1/users/domain"+string(rune('a'+i%26))+"/:uid/action", AuthAdmin, nil)
		app.add(http.MethodDelete, "/api/v1/system/domain"+string(rune('a'+i%26))+"/:uid/action", AuthAdmin, nil)
	}
	app.add(http.MethodGet, "/api/v1/admin/target/:uid/action", AuthAdmin, nil)
	return app
}
