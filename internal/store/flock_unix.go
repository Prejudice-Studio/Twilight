//go:build unix

package store

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// fileLock 是一个基于 OS flock 的进程级排他锁，仅 JSON 后端使用。
// 多进程共用 state.json 时（API + scheduler + bot），靠它把读改写串行化，
// 避免 NextUserID 跳号 / 重号等丢更新问题。
// 持有方式：Store 在 Open() 时尝试 LOCK_EX|LOCK_NB；
// 第二个进程拿不到锁会立即得到 ErrLockBusy，由 Open() 转成可读的启动错误。
// Close() 时 unlock 并关闭 fd。
type fileLock struct {
	f *os.File
}

// ErrLockBusy 表示 state 文件已被其它 Twilight 进程持锁。
// 上层翻译成 "JSON state file already locked by another process" 一条启动错误。
var ErrLockBusy = errors.New("state file is locked by another process")

func acquireStateLock(path string) (*fileLock, error) {
	if path == "" {
		return nil, nil
	}
	// 用一个独立的 .lock 文件而不是 state.json 本身：
	//   - 我们后续 saveLocked 会做 rename(tmp → state.json)，
	//     rename 会让 state.json 的 inode 变；锁挂在 state.json fd 上会失效。
	//   - 而 .lock 文件是常驻 inode，flock 一直有效。
	lockPath := path + ".lock"
	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open state lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLockBusy
		}
		return nil, fmt.Errorf("flock state lock: %w", err)
	}
	return &fileLock{f: f}, nil
}

func (l *fileLock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	err := l.f.Close()
	l.f = nil
	return err
}
