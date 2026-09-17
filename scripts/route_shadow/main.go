// scripts/check_route_shadow.go 是 CI 一致性 lint：防止路由表里出现"永远匹配
// 不到"或"谁生效靠注册顺序"的路由对。
//
// 背景（真实事故）：路由按 (method, 段数, 分组) 分桶后按注册顺序线性匹配，
// 于是 `/api/v2/admin/users/:uid` 先注册就把后注册的 `/expiring` 与
// `/batch/{enable,disable,renew,delete,refresh-status}` 全部吃掉——请求以
// uid="expiring"/"batch" 落到单用户 handler 上，管理端批量操作与"即将到期"
// 列表长期 400/404，而编译和单测都没报错。`/admin/media-requests/batch` 同理
// 被 `:request_id` 吃掉。
//
// 路由器已改为"同桶内字面量段多者优先"（见 internal/api/app.go 的
// matchIndexedRoutes），注册顺序不再决定胜负；本 lint 负责阻止新的隐患：
//
//  1. 同方法同路径重复注册：第二条永不生效，属死代码（且让端点计数失真）。
//  2. 真歧义：两条路由段数相同、字面量数也相同，但参数位置不同，因此存在
//     同时匹配两者请求路径——胜负仍取决于注册顺序，换个注册位置行为就变。
//
// 字面量数不同的重叠（`/x/:id` 与 `/x/batch`）由路由器按最具体者优先解决，
// 这里只作提示、不算失败。
//
// 运行方式（cwd 必须是仓库根目录，脚本按相对路径找路由表）：
//
//	go run ./scripts/route_shadow
//
// 放在独立子目录是因为 scripts/ 下已有 check_docs_drift.go 这个 package main，
// 同目录再放一个 main 会让 `go build ./...` 直接报 "main redeclared"。
//
// 退出码：0 无问题；1 发现重复注册或真歧义；2 解析 / IO 故障。
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// routeAddPattern 抓 routes.go / routes_v2.go 里的 a.add(...) 单行注册。
var routeAddPattern = regexp.MustCompile(`a\.add\(http\.Method(\w+),\s*"([^"]+)",\s*(\w+),\s*([A-Za-z0-9_.()]+)\)`)

type route struct {
	method   string
	path     string
	handler  string
	file     string
	line     int
	parts    []string
	literals int
}

func (r route) String() string {
	return fmt.Sprintf("%-6s %s", r.method, r.path)
}

func main() {
	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "check_route_shadow: %v\n", err)
		os.Exit(2)
	}
	var routes []route
	for _, name := range []string{"routes.go", "routes_v2.go"} {
		parsed, err := parseRoutes(filepath.Join(repoRoot, "internal", "api", name))
		if err != nil {
			fmt.Fprintf(os.Stderr, "check_route_shadow: %v\n", err)
			os.Exit(2)
		}
		routes = append(routes, parsed...)
	}
	if len(routes) == 0 {
		fmt.Fprintln(os.Stderr, "check_route_shadow: 没解析到任何 a.add()，正则需要更新")
		os.Exit(2)
	}

	duplicates := findDuplicates(routes)
	ambiguous := findAmbiguous(routes)
	overlaps := findLiteralOverlaps(routes)

	fmt.Printf("扫描 %d 条路由\n", len(routes))

	if len(overlaps) > 0 {
		fmt.Printf("\n[提示] 以下 %d 条静态路由注册在同形参数路由之后，\n", len(overlaps))
		fmt.Println("       由路由器按\"字面量多者优先\"解决，无需调整注册顺序：")
		for _, pair := range overlaps {
			fmt.Printf("       %s (%s:%d)\n", pair.later.String(), pair.later.file, pair.later.line)
			fmt.Printf("         与 %s (%s:%d) 重叠\n", pair.earlier.path, pair.earlier.file, pair.earlier.line)
		}
	}

	failed := false
	if len(duplicates) > 0 {
		failed = true
		fmt.Printf("\n[FAIL] 同方法同路径重复注册 %d 组（后者永不生效）：\n", len(duplicates))
		for _, group := range duplicates {
			for _, r := range group {
				fmt.Printf("       %s -> %s (%s:%d)\n", r.String(), r.handler, r.file, r.line)
			}
		}
	}
	if len(ambiguous) > 0 {
		failed = true
		fmt.Printf("\n[FAIL] 真歧义路由 %d 组（胜负仍取决于注册顺序）：\n", len(ambiguous))
		for _, pair := range ambiguous {
			fmt.Printf("       %s (%s:%d)\n", pair.a.String(), pair.a.file, pair.a.line)
			fmt.Printf("       %s (%s:%d)\n", pair.b.String(), pair.b.file, pair.b.line)
			fmt.Println("         两者字面量段数相同，存在同时匹配两者的请求路径")
		}
	}

	if failed {
		fmt.Println("\ncheck_route_shadow: 发现需要处理的路由问题")
		os.Exit(1)
	}
	fmt.Println("check_route_shadow: OK")
}

func parseRoutes(path string) ([]route, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []route
	base := filepath.Base(path)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for line := 1; scanner.Scan(); line++ {
		m := routeAddPattern.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}
		parts := splitPath(m[2])
		literals := 0
		for _, part := range parts {
			if !strings.HasPrefix(part, ":") {
				literals++
			}
		}
		out = append(out, route{
			method:   strings.ToUpper(m[1]),
			path:     m[2],
			handler:  m[4],
			file:     base,
			line:     line,
			parts:    parts,
			literals: literals,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// findDuplicates 找出同方法同路径的重复注册。
func findDuplicates(routes []route) [][]route {
	groups := map[string][]route{}
	for _, r := range routes {
		groups[r.method+" "+r.path] = append(groups[r.method+" "+r.path], r)
	}
	var keys []string
	for key, group := range groups {
		if len(group) > 1 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := make([][]route, 0, len(keys))
	for _, key := range keys {
		out = append(out, groups[key])
	}
	return out
}

type pair struct{ a, b route }

type overlapPair struct{ earlier, later route }

// findAmbiguous 找出"真歧义"：段数相同、字面量数相同，但参数位置不同。
// 这类路由对一定存在同时匹配两者的请求路径，胜负只能靠注册顺序。
func findAmbiguous(routes []route) []pair {
	var out []pair
	for i := 0; i < len(routes); i++ {
		for j := i + 1; j < len(routes); j++ {
			a, b := routes[i], routes[j]
			if a.method != b.method || len(a.parts) != len(b.parts) || a.literals != b.literals {
				continue
			}
			if a.path == b.path {
				continue // 重复注册由 findDuplicates 报告
			}
			if shapesConflict(a.parts, b.parts) {
				out = append(out, pair{a, b})
			}
		}
	}
	return out
}

// shapesConflict 判断两段路由是否存在"同一位置一个是参数、另一个是字面量"，
// 且其余位置相等或同为参数。满足则两者会同时匹配某个具体路径。
func shapesConflict(a, b []string) bool {
	conflict := false
	for i := range a {
		aParam, bParam := strings.HasPrefix(a[i], ":"), strings.HasPrefix(b[i], ":")
		switch {
		case aParam && bParam:
			// 都是参数：任何取值都能同时匹配，仅在别处有冲突时才算歧义
		case aParam && !bParam, !aParam && bParam:
			conflict = true
		case a[i] != b[i]:
			return false // 字面量不同：不可能同时匹配
		}
	}
	return conflict
}

// findLiteralOverlaps 找出"先参数后字面量"的重叠（路由器已能正确处理，仅提示）。
func findLiteralOverlaps(routes []route) []overlapPair {
	var out []overlapPair
	for i := 0; i < len(routes); i++ {
		for j := i + 1; j < len(routes); j++ {
			earlier, later := routes[i], routes[j]
			if earlier.method != later.method || len(earlier.parts) != len(later.parts) {
				continue
			}
			if later.literals <= earlier.literals {
				continue
			}
			if shadows(earlier.parts, later.parts) {
				out = append(out, overlapPair{earlier, later})
			}
		}
	}
	return out
}

// shadows 判断 earlier 是否覆盖 later 的所有具体路径（同位置：earlier 是参数、
// later 是字面量，其余相等）。
func shadows(earlier, later []string) bool {
	diff := 0
	for i := range earlier {
		if earlier[i] == later[i] {
			continue
		}
		if strings.HasPrefix(earlier[i], ":") && !strings.HasPrefix(later[i], ":") {
			diff++
			continue
		}
		return false
	}
	return diff > 0
}

// splitPath 与 internal/api 的实现保持一致：去掉首尾斜杠后按 "/" 切分。
func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}
