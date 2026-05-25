// Package validate 提供前后端共用的字段校验规则。
// 任何修改都必须同步到前端 webui/src/lib/password.ts 等镜像文件。
package validate

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// 用户名规则（与前端 register/login 提示对齐）：
//   - 长度 3-32
//   - 禁止 / \ @ : NUL < > " ' & 等用于路径/HTML/SQL 注入的字符
const (
	UsernameMinLen      = 3
	UsernameMaxLen      = 32
	usernameForbiddenCh = "/\\@:\x00<>\"'&"
)

// 密码强度规则（与前端 webui/src/lib/password.ts 一一对齐）：
//   - 长度 8-128
//   - 至少包含 1 个小写、1 个大写、1 个数字
const (
	PasswordMinLen = 8
	PasswordMaxLen = 128
)

// 错误集合：handler 可对照 errcode.go 映射 ErrCode。
var (
	ErrUsernameLen          = errors.New("用户名长度需为 3-32 位")
	ErrUsernameForbiddenCh  = errors.New("用户名包含禁用字符（/\\@:<>\"'&）")
	ErrPasswordTooShort     = fmt.Errorf("密码长度至少 %d 位", PasswordMinLen)
	ErrPasswordTooLong      = fmt.Errorf("密码长度不能超过 %d 位", PasswordMaxLen)
	ErrPasswordMissingLower = errors.New("密码需要至少 1 个小写字母")
	ErrPasswordMissingUpper = errors.New("密码需要至少 1 个大写字母")
	ErrPasswordMissingDigit = errors.New("密码需要至少 1 个数字")
)

// ValidateUsername 校验用户名。
func ValidateUsername(username string) error {
	if n := len(username); n < UsernameMinLen || n > UsernameMaxLen {
		return ErrUsernameLen
	}
	if strings.ContainsAny(username, usernameForbiddenCh) {
		return ErrUsernameForbiddenCh
	}
	return nil
}

// ValidatePasswordStrength 与前端 password.ts:validatePasswordStrength 等价。
// 前端可在用户输入时给出可视化提示，后端必须用本函数兜底，否则可被绕过。
func ValidatePasswordStrength(password string) error {
	if len(password) < PasswordMinLen {
		return ErrPasswordTooShort
	}
	if len(password) > PasswordMaxLen {
		return ErrPasswordTooLong
	}
	var hasLower, hasUpper, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLower {
		return ErrPasswordMissingLower
	}
	if !hasUpper {
		return ErrPasswordMissingUpper
	}
	if !hasDigit {
		return ErrPasswordMissingDigit
	}
	return nil
}

// ValidatePasswordLegacy 仅校验最小长度，用于不强制强度的旧路径
// （如系统首次初始化、忘记密码自动生成、向后兼容场景）。新代码请使用
// ValidatePasswordStrength。
func ValidatePasswordLegacy(password string) error {
	if len(password) < PasswordMinLen {
		return ErrPasswordTooShort
	}
	if len(password) > PasswordMaxLen {
		return ErrPasswordTooLong
	}
	return nil
}
