package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	// runtimeLogSidecarSuffix 是 JSON 后端 runtime log 旁路文件的后缀，与 state.json 同目录。
	runtimeLogSidecarSuffix = ".runtimelog"
	// runtimeLogLoadCap 是启动解析旁路文件时的内存上限：即便 compaction 漏掉、磁盘行数
	// 异常膨胀，也只保留末尾这么多条，避免 OOM。attach 后 configure 会 PruneRuntimeLogs 到真实 limit。
	runtimeLogLoadCap = 50000
	// runtimeLogScanBufMax 是单行 NDJSON 的最大字节：带 attrs 的日志可能较长，放宽到 4 MiB。
	runtimeLogScanBufMax = 4 << 20
)

// runtimeLogFile 是 JSON 后端的 runtime log 旁路存储：内存环形缓冲（读取来源）
// + 磁盘 NDJSON 追加文件（持久化）。它拥有独立于 Store.mu 的互斥锁，彻底避开
// 「saveLocked 持 s.mu 时全局 zap sink 回调 AddRuntimeLog 再抢 s.mu」的自旋死锁：
// 现在 AddRuntimeLog 的 JSON 分支只碰 rlf.mu，与 s.mu 无交集。
//
// 这样每条 zap 日志不再触发整份多 MB state.json 的 marshal + fsync（旧路径每条
// Info 日志都要 refreshLocked + append 到 state.RuntimeLogs + saveLocked），CPU 与
// 磁盘 I/O 从「随日志量线性放大整库写」降到「一行 append」。
//
// 落盘策略：每次 append 走 open→写一行→close（不留常驻句柄），规避 Windows 上
// 「文件仍打开时 rename 失败」的坑；不做 per-append fsync（诊断日志尽力而为，容忍
// 崩溃丢尾部若干条）；磁盘物理行数超过 ~2×limit 时原子重写（compaction）收敛体积。
type runtimeLogFile struct {
	mu        sync.Mutex
	path      string
	entries   []RuntimeLogEntry // 环形缓冲，按 ID 升序
	nextID    int64
	diskLines int // 旁路文件当前物理行数估计，用于触发 compaction
}

// newRuntimeLogFile 挂接旁路文件：
//   - 文件已存在：以它为准 seed 环形缓冲（权威来源），nextID = max(maxID+1, seedNextID)；
//   - 文件不存在：从 seed（历史 state.json 内嵌的 RuntimeLogs）迁移并写出旁路文件，
//     由调用方随后清空 state.RuntimeLogs 使其不再落进 state.json。
//
// seedNextID 传入 state.NextRuntimeLogID 作为下限，保证计数器不回退（即使旁路
// 文件被外部截断，也不会分配到与历史 ID 撞车的值）。
func newRuntimeLogFile(path string, seed []RuntimeLogEntry, seedNextID int64) (*runtimeLogFile, error) {
	rlf := &runtimeLogFile{path: path, nextID: 1}
	if seedNextID > rlf.nextID {
		rlf.nextID = seedNextID
	}
	data, err := os.ReadFile(path)
	if err == nil {
		entries, physical, maxID := parseRuntimeLogNDJSON(data)
		rlf.entries = entries
		rlf.diskLines = physical
		if maxID+1 > rlf.nextID {
			rlf.nextID = maxID + 1
		}
		return rlf, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(seed) > 0 {
		trimmed := compactTail(seed, runtimeLogLoadCap)
		out := make([]RuntimeLogEntry, len(trimmed))
		copy(out, trimmed)
		rlf.entries = out
		for i := range out {
			if out[i].ID+1 > rlf.nextID {
				rlf.nextID = out[i].ID + 1
			}
		}
		if err := rlf.rewriteLocked(); err != nil {
			return nil, err
		}
	}
	return rlf, nil
}

// parseRuntimeLogNDJSON 逐行解析旁路文件：坏行（截断 / 半截 JSON，通常来自崩溃时
// 的最后一次 append）直接跳过，不让整份文件解析失败。返回按 ID 升序的条目、物理
// 行数（含被跳过的坏行，用于 compaction 触发判断）以及最大 ID。超过 runtimeLogLoadCap
// 时只保留末尾部分（旁路文件本就近似有序追加，末尾即最新）。
func parseRuntimeLogNDJSON(data []byte) ([]RuntimeLogEntry, int, int64) {
	if len(data) == 0 {
		return nil, 0, 0
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), runtimeLogScanBufMax)
	entries := make([]RuntimeLogEntry, 0, 256)
	physical := 0
	var maxID int64
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		physical++
		var entry RuntimeLogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.ID > maxID {
			maxID = entry.ID
		}
		entries = append(entries, entry)
	}
	if len(entries) > runtimeLogLoadCap {
		entries = compactTail(entries, runtimeLogLoadCap)
	}
	return entries, physical, maxID
}

// add 追加一条日志：分配 ID（ID==0 时取 nextID 自增，与旧 JSON 语义一致；非零 ID
// 走 LoadSnapshot 之外基本不出现，保守保留其 ID 不推进计数器）、trim 环形缓冲到
// limit、尽力落盘。持久化失败**不返回 error**：条目已在环形缓冲里可被 RuntimeLogs
// 读到，返回 error 反而会让上层 sink 误判为「未落库」转投 fallback buffer 造成重复。
func (r *runtimeLogFile) add(entry RuntimeLogEntry, limit int) RuntimeLogEntry {
	limit = clampRuntimeLogLimit(limit)
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry.ID == 0 {
		entry.ID = r.nextID
		r.nextID++
	} else if entry.ID+1 > r.nextID {
		r.nextID = entry.ID + 1
	}
	if entry.Time == 0 {
		entry.Time = time.Now().Unix()
	}
	r.entries = append(r.entries, entry)
	if len(r.entries) > limit {
		r.entries = compactTail(r.entries, limit)
	}
	r.persistAppendLocked(entry, limit)
	return entry
}

// persistAppendLocked 必须持 r.mu：把单条 entry 以一行 NDJSON append 到旁路文件。
// 物理行数超过 2×limit 时改走整份原子重写（compaction），把磁盘收敛回环形缓冲当前内容。
// 任何 I/O 失败仅写 stderr（不能走 zap —— 会再次回调本 sink），日志本体已在内存环形缓冲里。
func (r *runtimeLogFile) persistAppendLocked(entry RuntimeLogEntry, limit int) {
	if r.diskLines+1 > limit*2 {
		if err := r.rewriteLocked(); err != nil {
			fmt.Fprintf(os.Stderr, "twilight: runtime log sidecar compaction failed path=%s err=%v\n", r.path, err)
		}
		return
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(r.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "twilight: runtime log sidecar open failed path=%s err=%v\n", r.path, err)
		return
	}
	encoded = append(encoded, '\n')
	if _, err := f.Write(encoded); err != nil {
		fmt.Fprintf(os.Stderr, "twilight: runtime log sidecar append failed path=%s err=%v\n", r.path, err)
		_ = f.Close()
		return
	}
	_ = f.Close()
	r.diskLines++
}

// rewriteLocked 必须持 r.mu：把当前环形缓冲整份原子重写到旁路文件（tmp+fsync+rename+fsync(dir)），
// 复用 writeFileAtomicSync。重写后 diskLines 收敛为 len(entries)。空缓冲也写空文件，
// 使「无日志」语义严格落盘（LoadSnapshot 恢复到无日志时点时不残留旧行）。
func (r *runtimeLogFile) rewriteLocked() error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for i := range r.entries {
		if err := enc.Encode(r.entries[i]); err != nil {
			return err
		}
	}
	if err := writeFileAtomicSync(r.path, buf.Bytes(), 0o600); err != nil {
		return err
	}
	r.diskLines = len(r.entries)
	return nil
}

// snapshotEntries 返回环形缓冲的深拷贝 + nextID，供 Store.Snapshot 把 runtime log
// 重新注入 State（JSON 后端内存 state.RuntimeLogs 恒空，备份要自洽必须现取）。
func (r *runtimeLogFile) snapshotEntries() ([]RuntimeLogEntry, int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]RuntimeLogEntry, len(r.entries))
	copy(out, r.entries)
	return out, r.nextID
}

// logs 复刻旧 JSON 分支的游标语义：after<=0 取末尾 limit 条；after>0 取 ID>after 的
// 前 limit 条。next 在空窗口时回落到 nextID-1（maxID），非空时取最后一条的 ID。
func (r *runtimeLogFile) logs(limit int, after int64) ([]RuntimeLogEntry, int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	maxLimit := len(r.entries)
	if limit <= 0 || limit > maxLimit {
		limit = maxLimit
	}
	next := after
	if r.nextID > 1 {
		next = r.nextID - 1
	}
	start, end := runtimeLogWindow(r.entries, limit, after)
	if start == end {
		return nil, next
	}
	filtered := r.entries[start:end]
	next = filtered[len(filtered)-1].ID
	out := make([]RuntimeLogEntry, len(filtered))
	copy(out, filtered)
	return out, next
}

// stats 返回 (maxID, count)，maxID = nextID-1（trim 后依旧保留最大 ID，与旧语义一致）。
func (r *runtimeLogFile) stats() (int64, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := int64(0)
	if r.nextID > 1 {
		next = r.nextID - 1
	}
	return next, len(r.entries)
}

// prune 把环形缓冲收敛到 limit，并整份重写旁路文件收敛磁盘体积。
func (r *runtimeLogFile) prune(limit int) error {
	limit = clampRuntimeLogLimit(limit)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) <= limit {
		return nil
	}
	r.entries = compactTail(r.entries, limit)
	return r.rewriteLocked()
}

// replace 用于 LoadSnapshot：用快照里的 runtime log 整份替换环形缓冲与旁路文件，
// nextID 取 max(maxID+1, 现有 nextID)（不回退），原子重写落盘。
func (r *runtimeLogFile) replace(entries []RuntimeLogEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	trimmed := compactTail(entries, runtimeLogLoadCap)
	out := make([]RuntimeLogEntry, len(trimmed))
	copy(out, trimmed)
	r.entries = out
	for i := range out {
		if out[i].ID+1 > r.nextID {
			r.nextID = out[i].ID + 1
		}
	}
	return r.rewriteLocked()
}

// sidecarPath 返回 state 文件对应的 runtime log 旁路文件路径。
func sidecarPath(statePath string) string {
	return statePath + runtimeLogSidecarSuffix
}

// RemoveRuntimeLogSidecar 删除 state 文件对应的旁路文件（best-effort）。JSON→JSON
// 迁移时目标 state.json 已内嵌 runtime log；必须清掉目标处的历史旁路文件，否则下次
// Open 会以旧旁路为准、遮蔽刚迁移进来的内嵌日志。供 api 层迁移路径调用。
func RemoveRuntimeLogSidecar(statePath string) {
	if err := os.Remove(sidecarPath(statePath)); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "twilight: remove runtime log sidecar failed path=%s err=%v\n", statePath, err)
	}
}
