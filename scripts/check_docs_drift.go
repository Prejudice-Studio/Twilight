// scripts/check_docs_drift.go 是 CI 一致性 lint：保证 routes.go 与 routes_v2.go
// 注册的每条 (METHOD, PATH) 都在 docs/*.md 任一文档里出现至少一次，否则 fail。
// 触达背景：BACKEND_API.md 目前对 routes.go 已有大量历史漂移，新人接入时
// 只能从源码反推；这条 lint 阻止下次 PR 又少写一条。
//
// 两个文件都必须覆盖：前端默认 API 版本已经是 v2（DEFAULT_API_VERSION="v2"），
// 只 lint  routes.go 会让 `/api/v2/*` 的漂移完全逃过 CI——v1 路径与 v2 路径是
// 不同的字符串，文档里写了 v1 并不会让 v2 端点"被文档化"。
//
// 运行方式（本地或 CI）：
//
//	go run ./scripts/check_docs_drift.go
//
// 退出码：
//   - 0：所有 routes.go / routes_v2.go 注册的端点都在 docs 中至少出现一次，
//     或被 baseline 文件显式 grandfather。
//   - 1：检测到 drift，stdout 列出未在 docs 中出现且不在 baseline 中的
//     (METHOD, PATH)；或 baseline 中存在已经被消除（路由消失或文档已补全）
//     的"陈旧"条目，需要从 baseline 中删除。
//   - 2：解析或 IO 故障（lint 自身需要修）。
//
// 解析规则：
//
//   - 从 internal/api/routes.go 与 internal/api/routes_v2.go 提取
//     `a.add(http.Method<X>, "<PATH>", ...)`。
//   - 把 ":foo" 形式的路径参数归一化成 "{foo}"，再做包含匹配——docs 里两种
//     写法都允许，但内部规范化后只看路径形状。
//   - 跳过以 `// docs:skip` 结尾的行：极个别内部端点（diagnostic 探针 /
//     临时调试）可以显式标记不参加 lint。
//
// Baseline 机制：
//
//   - scripts/docs_drift_baseline.txt 保存"已知未文档化"清单，每行
//     "METHOD PATH"（# 开头的行视作注释）。新增路由必须文档化或写入 baseline；
//     baseline 是 PR 可见 diff，避免静悄悄绕过 lint。
//   - 如果 baseline 里某条已经被文档化、或路由表已经移除该路由，lint
//     报错要求清理 baseline——保证 baseline 只会变小不会僵化成永久噪音。
package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// addPattern 必须能命中 routes.go 里以下三种风格：
//
//	a.add(http.MethodGet, "/api/...", AuthUser, a.handleX)
//	a.add(http.MethodPost,
//	      "/api/...",
//	      AuthAdmin, a.handleY)
//	a.add(http.MethodDelete, "/api/.../:id", AuthAdmin, a.handleZ) // docs:skip
//
// 多行调用做不了真正的 AST 解析（避免引入额外依赖），但 routes.go 当前
// 100% 是单行 a.add，加 sanity check 兜底：解析后总数 != grep 计数 → 报错。
var addPattern = regexp.MustCompile(`a\.add\(http\.Method([A-Z][a-zA-Z]+),\s*"([^"]+)"`)

// pathParamPattern 把 routes.go 的 ":foo" 形参换成 docs 风格的 "{foo}"。
// 不在 path segment 边界外做替换：":foo/bar/:baz" 也都安全。
var pathParamPattern = regexp.MustCompile(`:([a-zA-Z_][a-zA-Z0-9_]*)`)

// braceParamPattern 是 pathParamPattern 的逆向：把已归一化的 "{foo}" 还原成
// routes.go 原生的 ":foo"。
//
// 曾经这里直接用 pathParamPattern 做"反归一化"，但它匹配的是冒号而不是花括号，
// 对已经是 "{foo}" 的字符串永远替换不出东西——结果是文档里按源码写法引用
// `/api/v2/media/tmdb/:tmdb_id` 时永远匹配不上，lint 把已文档化的端点误报成
// drift。两个方向必须是两条不同的正则。
var braceParamPattern = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

type endpoint struct {
	Method string
	Path   string
}

func (e endpoint) key() string { return e.Method + " " + e.Path }

func main() {
	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "check_docs_drift: %v\n", err)
		os.Exit(2)
	}
	// v1 与 v2 两套路由表都参与 lint：v2 是当前前端默认版本，只查 v1 会让
	// 三百多条 v2 端点完全逃过文档检查。
	routeFiles := []string{
		filepath.Join(repoRoot, "internal", "api", "routes.go"),
		filepath.Join(repoRoot, "internal", "api", "routes_v2.go"),
	}
	endpoints, totalAdds, err := parseRoutes(routeFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check_docs_drift: parse route tables: %v\n", err)
		os.Exit(2)
	}
	if len(endpoints) == 0 {
		fmt.Fprintf(os.Stderr, "check_docs_drift: route tables have 0 a.add() calls — pattern out of date?\n")
		os.Exit(2)
	}
	// sanity：parsed 与 grep 计数一致才相信解析覆盖完整。如果有人引入多行
	// a.add，这条断言会先 fail，提示 lint 自身需要升级。
	if totalAdds != len(endpoints) {
		fmt.Fprintf(os.Stderr, "check_docs_drift: parsed %d endpoints but found %d a.add( occurrences — multi-line a.add not supported, please collapse to single line\n", len(endpoints), totalAdds)
		os.Exit(2)
	}

	docsDir := filepath.Join(repoRoot, "docs")
	docsCorpus, err := loadDocsCorpus(docsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check_docs_drift: load docs: %v\n", err)
		os.Exit(2)
	}

	baselinePath := filepath.Join(repoRoot, "scripts", "docs_drift_baseline.txt")
	baseline, err := loadBaseline(baselinePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check_docs_drift: load baseline: %v\n", err)
		os.Exit(2)
	}

	endpointKeys := make(map[string]struct{}, len(endpoints))
	for _, ep := range endpoints {
		endpointKeys[ep.key()] = struct{}{}
	}

	var (
		missing      []endpoint // 未文档化且未在 baseline 中：必须修
		baselineUsed = map[string]struct{}{}
	)
	for _, ep := range endpoints {
		if endpointDocumented(docsCorpus, ep) {
			continue
		}
		if _, ok := baseline[ep.key()]; ok {
			baselineUsed[ep.key()] = struct{}{}
			continue
		}
		missing = append(missing, ep)
	}

	// stale baseline = 该条目要么已经被文档化（已不再 drift）、要么路由本身
	// 已被删除。两种情况都要求清理 baseline，保证 baseline 单调收敛。
	var stale []string
	for key := range baseline {
		if _, used := baselineUsed[key]; used {
			continue
		}
		stale = append(stale, key)
	}
	sort.Strings(stale)

	failed := false
	if len(missing) > 0 {
		failed = true
		sort.Slice(missing, func(i, j int) bool {
			if missing[i].Path != missing[j].Path {
				return missing[i].Path < missing[j].Path
			}
			return missing[i].Method < missing[j].Method
		})
		fmt.Printf("Found %d undocumented endpoints not in baseline (out of %d total, %d in baseline):\n\n", len(missing), len(endpoints), len(baseline))
		for _, ep := range missing {
			fmt.Printf("  %-7s %s\n", ep.Method, ep.Path)
		}
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Each missing endpoint must appear at least once in docs/*.md (BACKEND_API.md is the canonical surface),")
		fmt.Fprintln(os.Stderr, "or be added to scripts/docs_drift_baseline.txt as an explicitly accepted gap.")
		fmt.Fprintln(os.Stderr, "If an endpoint is intentionally undocumented, append `// docs:skip` to its a.add line in routes.go.")
	}
	if len(stale) > 0 {
		failed = true
		fmt.Printf("\nFound %d stale baseline entries (route gone or now documented — please remove from scripts/docs_drift_baseline.txt):\n\n", len(stale))
		for _, key := range stale {
			fmt.Printf("  %s\n", key)
		}
	}
	if failed {
		os.Exit(1)
	}

	if len(baseline) == 0 {
		fmt.Printf("OK: all %d endpoints documented in docs/*.md\n", len(endpoints))
	} else {
		fmt.Printf("OK: %d endpoints, %d documented, %d grandfathered via scripts/docs_drift_baseline.txt\n", len(endpoints), len(endpoints)-len(baseline), len(baseline))
	}
}

// loadBaseline 读取 scripts/docs_drift_baseline.txt：每行 "METHOD PATH"，
// # 开头是注释。文件不存在等同于空 baseline（首次启用 lint 时是常态）。
func loadBaseline(path string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s:%d: expected `METHOD PATH`, got %q", path, lineNo, line)
		}
		method := strings.ToUpper(fields[0])
		out[method+" "+fields[1]] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cur := wd
	for {
		if _, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("go.mod not found from %s up", wd)
		}
		cur = parent
	}
}

// parseRoutes 汇总多个路由表文件。跨文件按 (METHOD, PATH) 去重——v1/v2 路径
// 前缀不同所以天然不冲突，但同一个文件里同一路径注册两次（例如 v2 为兼容
// V1 方法补注册）只会算一条。
func parseRoutes(paths []string) ([]endpoint, int, error) {
	var (
		eps      []endpoint
		seen     = map[string]struct{}{}
		totalAdd int
	)
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return nil, 0, err
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.Contains(line, "a.add(http.Method") {
				continue
			}
			totalAdd++
			if strings.Contains(line, "// docs:skip") {
				continue
			}
			m := addPattern.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			method := strings.ToUpper(m[1])
			raw := m[2]
			normalized := pathParamPattern.ReplaceAllString(raw, "{$1}")
			key := method + " " + normalized
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			eps = append(eps, endpoint{Method: method, Path: normalized})
		}
		if err := scanner.Err(); err != nil {
			f.Close()
			return nil, 0, err
		}
		f.Close()
	}
	return eps, totalAdd, nil
}

func loadDocsCorpus(dir string) (string, error) {
	var sb strings.Builder
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sb.Write(data)
		sb.WriteByte('\n')
		return nil
	})
	if err != nil {
		return "", err
	}
	return sb.String(), nil
}

// endpointDocumented 用最宽松的"包含"语义：任意 docs/*.md 出现归一化后的
// "METHOD PATH" 字符串就算通过——不强求章节编排，只确保有人提过它。
// 也接受常见格式变体：
//
//	GET /api/v1/x/{id}
//	`GET` `/api/v1/x/{id}`
//	`GET /api/v1/x/{id}`
//	GET /api/v1/x/:id     # routes.go 风格也兼容（向后给运维空间）
func endpointDocumented(corpus string, ep endpoint) bool {
	rawPath := braceParamPattern.ReplaceAllString(ep.Path, ":$1") // 反归一化拿回 :foo 形式
	candidates := []string{
		ep.Method + " " + ep.Path,
		ep.Method + " " + rawPath,
	}
	// path 单独命中也接受——很多文档把 method 写在 chip / 表头列里，
	// 路径单独引用。这里用 path 的精确字符串搜索，避免太松导致误命中。
	candidates = append(candidates, ep.Path)
	if rawPath != ep.Path {
		candidates = append(candidates, rawPath)
	}
	for _, needle := range candidates {
		if strings.Contains(corpus, needle) {
			return true
		}
	}
	return false
}
