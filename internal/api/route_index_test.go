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

// TestRouteIndexPrefersLiteralOverParameter 锁定"最具体者胜"而不是"先注册者胜"。
//
// 旧语义下这两条路由谁生效纯靠注册顺序：/api/v1/:domain/detail 先注册就会把
// /api/v1/users/detail 整个吃掉。真实路由表踩过同一个坑——/api/v2/admin/users/:uid
// 注册在 /expiring 与 /batch/* 之前，于是"即将到期用户"和全部批量操作都被单用户
// handler 以 uid="expiring"/"batch" 接走，请求永远失败。现在按字面量段数选最具体
// 的那条；同分（字面量数相同）才回退到注册顺序。
func TestRouteIndexPrefersLiteralOverParameter(t *testing.T) {
	app := &App{}
	app.add(http.MethodGet, "/api/v1/:domain/detail", AuthPublic, nil)
	app.add(http.MethodGet, "/api/v1/users/detail", AuthUser, nil)

	route, _, ok := app.match(http.MethodGet, "/api/v1/users/detail")
	if !ok || route == nil || route.Pattern != "/api/v1/users/detail" {
		t.Fatalf("literal route should win over a same-shape parameter route: route=%+v ok=%v", route, ok)
	}

	// 参数位仍然要能匹配非字面量的取值。
	route, params, ok := app.match(http.MethodGet, "/api/v1/other/detail")
	if !ok || route == nil || route.Pattern != "/api/v1/:domain/detail" || params["domain"] != "other" {
		t.Fatalf("parameter route still must match other values: route=%+v params=%v ok=%v", route, params, ok)
	}
}

// TestRouteIndexKeepsRegistrationOrderOnEqualSpecificity 同分时保留先注册者，
// 避免改成"按字面量数排序"后把既有路由的优先级悄悄换掉。
func TestRouteIndexKeepsRegistrationOrderOnEqualSpecificity(t *testing.T) {
	app := &App{}
	// 两条都是 3 个字面量段（api / v1 + 各自一个字面量），/api/v1/x/y 同时匹配
	// 两者，谁都不比谁更具体 —— 此时才回退到注册顺序。
	app.add(http.MethodGet, "/api/v1/:a/y", AuthPublic, nil)
	app.add(http.MethodGet, "/api/v1/x/:b", AuthUser, nil)

	route, params, ok := app.match(http.MethodGet, "/api/v1/x/y")
	if !ok || route == nil || route.Pattern != "/api/v1/:a/y" || params["a"] != "x" {
		t.Fatalf("equal specificity must keep registration order: route=%+v params=%v ok=%v", route, params, ok)
	}
}

// TestRealRouteTableResolvesStaticRoutesHiddenBehindParameters 用真实路由表
// 兜住那批"注册在参数路由之后、曾经永远匹配不到"的静态端点。它们一旦被遮蔽，
// 表现是管理端的批量操作与"即将到期"列表莫名 404/400，而不是编译报错——必须
// 有测试顶住。
func TestRealRouteTableResolvesStaticRoutesHiddenBehindParameters(t *testing.T) {
	app := &App{}
	app.registerRoutes()
	app.registerV2Routes()
	app.registerV2CompletionRoutes()

	cases := []struct {
		method  string
		path    string
		pattern string
	}{
		{http.MethodGet, "/api/v2/admin/users/expiring", "/api/v2/admin/users/expiring"},
		{http.MethodPost, "/api/v2/admin/users/batch/enable", "/api/v2/admin/users/batch/enable"},
		{http.MethodPost, "/api/v2/admin/users/batch/disable", "/api/v2/admin/users/batch/disable"},
		{http.MethodPost, "/api/v2/admin/users/batch/renew", "/api/v2/admin/users/batch/renew"},
		{http.MethodPost, "/api/v2/admin/users/batch/delete", "/api/v2/admin/users/batch/delete"},
		{http.MethodPost, "/api/v2/admin/users/batch/refresh-status", "/api/v2/admin/users/batch/refresh-status"},
		{http.MethodPut, "/api/v2/admin/media-requests/batch", "/api/v2/admin/media-requests/batch"},
	}
	for _, tc := range cases {
		route, _, ok := app.match(tc.method, tc.path)
		if !ok || route == nil {
			t.Fatalf("%s %s did not match any route", tc.method, tc.path)
		}
		if route.Pattern != tc.pattern {
			t.Fatalf("%s %s matched %q, want %q", tc.method, tc.path, route.Pattern, tc.pattern)
		}
	}

	// 参数路由本身不能被这次改动带偏。
	route, params, ok := app.match(http.MethodPost, "/api/v2/admin/users/42/disable")
	if !ok || route == nil || route.Pattern != "/api/v2/admin/users/:uid/disable" || params["uid"] != "42" {
		t.Fatalf("parameter route regressed: route=%+v params=%v ok=%v", route, params, ok)
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
