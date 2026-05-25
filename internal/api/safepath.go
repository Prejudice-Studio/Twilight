package api

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// safepath.go 集中实现路径穿越防护，避免在 handlers.go / config_admin.go /
// database_admin.go / admin_extra.go 中各自维护一套规则。修改本文件时务必
// 保持 "Abs+Clean+Rel" 三步顺序，并保留对符号链接和绝对路径的拒绝。

// ErrUnsafePath 表示请求的路径越出根目录、或类型不允许（符号链接、目录等）。
var ErrUnsafePath = errors.New("unsafe path")

// ResolveWithinRoot 把 candidate 拼接到 root 下并返回绝对路径，若越界返回 ErrUnsafePath。
// candidate 为相对路径或单文件名；绝对路径会先转换为相对 root 再校验。
// 不做 Lstat / 扩展名 / 符号链接判断，调用方可基于返回值再做。
func ResolveWithinRoot(root, candidate string) (string, error) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return "", ErrUnsafePath
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	joined := candidate
	if !filepath.IsAbs(joined) {
		joined = filepath.Join(base, joined)
	}
	abs, err := filepath.Abs(filepath.Clean(joined))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrUnsafePath
	}
	return abs, nil
}

// ResolveLeafFile 是 ResolveWithinRoot 的严格子集：要求 name 为单一文件名
// （没有目录分隔符 / 没有 ..），且最终路径必须是普通文件、非符号链接，
// 可选地校验扩展名（lowercase，不含点 "."；空字符串表示不校验）。
// 用于备份恢复、上传资源访问等"必须落在 root 直下"的场景。
func ResolveLeafFile(root, name, requiredExt string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || filepath.Base(name) != name || filepath.IsAbs(name) || strings.Contains(name, "..") {
		return "", ErrUnsafePath
	}
	if requiredExt != "" {
		if strings.ToLower(filepath.Ext(name)) != "."+strings.ToLower(strings.TrimPrefix(requiredExt, ".")) {
			return "", ErrUnsafePath
		}
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(base, name))
	if err != nil {
		return "", err
	}
	if filepath.Dir(target) != base {
		return "", ErrUnsafePath
	}
	info, err := os.Lstat(target)
	if err != nil {
		return "", ErrUnsafePath
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", ErrUnsafePath
	}
	return target, nil
}
