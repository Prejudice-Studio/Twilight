package api

import (
	"runtime"
	"sync"

	"github.com/prejudice-studio/twilight/internal/security"
)

// password_verify.go 集中登录路径上与密码校验相关的两项防护：
//
//  1. 并发上限信号量（pwVerifySem）：PBKDF2-SHA256 在 600k 迭代下单次约 ~150ms
//     单核 CPU。未鉴权的 /auth/login 对已知用户名每次都会触发一次完整校验，攻击者
//     用极低带宽即可把多核全部压在哈希上，饿死请求处理 goroutine。把并发校验数
//     限制在 GOMAXPROCS-1，保证总有核心服务正常请求；超出的校验排队等待而非
//     并行抢占 CPU。请求量本身由全局 / 登录 IP 限流兜底。
//
//  2. 常量代价占位哈希（dummyPasswordHash）：用户名不存在时若直接短路返回，
//     响应明显快于"存在但密码错"（后者跑完整 PBKDF2），形成用户名枚举时序旁路。
//     登录路径对不存在的用户名也跑一次等代价的 PBKDF2（对占位哈希），抹平时序差。

var pwVerifySem = make(chan struct{}, maxPasswordVerifyConcurrency())

func maxPasswordVerifyConcurrency() int {
	n := runtime.GOMAXPROCS(0) - 1
	if n < 1 {
		n = 1
	}
	return n
}

// verifyPasswordThrottled 在并发上限信号量下执行 security.VerifyPassword。
// 信号量满时阻塞等待槽位（而非拒绝），配合上游限流把 PBKDF2 的总 CPU 占用约束住。
func verifyPasswordThrottled(password, encoded string) bool {
	pwVerifySem <- struct{}{}
	defer func() { <-pwVerifySem }()
	return security.VerifyPassword(password, encoded)
}

var (
	dummyPasswordHashOnce sync.Once
	dummyPasswordHashVal  string
)

// dummyPasswordHash 返回一份用当前默认参数（PBKDF2-600k）生成的占位哈希，
// 用于用户名不存在时跑等代价校验。首次调用时惰性生成（~150ms 一次性成本），
// 之后复用。生成失败（crypto/rand 故障，极罕见）时返回空串——此时 VerifyPassword
// 会快速返回 false，时序均一性退化，但 rand 故障本身已是更严重的系统问题。
func dummyPasswordHash() string {
	dummyPasswordHashOnce.Do(func() {
		if h, err := security.HashPassword("twilight-nonexistent-account-timing-equalizer"); err == nil {
			dummyPasswordHashVal = h
		}
	})
	return dummyPasswordHashVal
}
