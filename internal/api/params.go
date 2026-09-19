package api

// 请求参数解析的统一入口。
//
// 这些 helper 原先散落在 business.go（queryInt / clamp / max / pages）与
// runtime_logs.go（minInt），其余 handler 则各写一遍 strconv.ParseInt +
// 忽略错误。散落的代价不是"重复代码"四个字，而是每个 call site 都要自己决定
// "解析失败怎么办"：多数写成 `_` 吞掉错误，于是 "abc" 静默变成 0，而这个 0
// 究竟表示"没传"（无害）还是"取第 0 条 / UID 0"（越界）只有读那个 handler
// 的人才知道。
//
// 收敛到这里之后，约定只有一条：解析失败或越界一律回退到调用方显式给出的
// fallback，并由 helper 负责夹取上下界。调用方不再需要自己写 if err == nil。
//
// 注意这里**不**把解析失败当 400 处理：这些参数大多是可选的展示类参数
// （limit / since / 过滤条件），把 "limit=abc" 变成一次请求失败会破坏既有
// 客户端。真正的必填路径参数走 int64Param（返回 error 由调用方决定 400）。

import (
	"net/http"
	"strconv"
)

// queryInt 取 int 型查询参数，缺失或非法时回退 fallback。
func queryInt(r *http.Request, key string, fallback int) int {
	if r == nil {
		return fallback
	}
	if value := r.URL.Query().Get(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

// queryIntClamped 取 int 型查询参数并夹到 [minValue, maxValue]。
//
// 写完 limit 再写 if parsed <= 0 || parsed > 1000 这种判断在仓库里出现过很多
// 次，且每次上限都不一样（100 / 500 / 1000）。夹取动作收进 helper，上限就变成
// call site 上一个看得见的数字，而不是埋在分支里的字面量。
func queryIntClamped(r *http.Request, key string, fallback, minValue, maxValue int) int {
	return clamp(queryInt(r, key, fallback), minValue, maxValue)
}

// queryInt64 取 int64 型查询参数（时间戳、UID、游标），缺失或非法时回退 fallback。
func queryInt64(r *http.Request, key string, fallback int64) int64 {
	if r == nil {
		return fallback
	}
	if value := r.URL.Query().Get(key); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed
		}
	}
	return fallback
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func pages(total, perPage int) int {
	if perPage <= 0 {
		return 1
	}
	if total == 0 {
		return 0
	}
	return (total + perPage - 1) / perPage
}
