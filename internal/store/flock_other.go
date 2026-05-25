//go:build !unix

package store

import "errors"

// 非 Unix 平台（例如 Windows）暂不提供进程级锁。
// 上游调用者应在多进程部署时禁用 JSON 后端 / 切到 Postgres。
type fileLock struct{}

var ErrLockBusy = errors.New("state file is locked by another process")

func acquireStateLock(path string) (*fileLock, error) {
	// noop：Windows / 其它非 Unix 平台不上锁。
	// 多进程共用 state.json 仍可能丢更新，应改用 Postgres 后端。
	_ = path
	return &fileLock{}, nil
}

func (l *fileLock) Release() error { return nil }
