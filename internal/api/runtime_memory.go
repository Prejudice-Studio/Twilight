package api

import (
	"math"
	"runtime/debug"

	"go.uber.org/zap"
)

func applyRuntimeMemoryLimit(limitMB int) {
	limitBytes := int64(math.MaxInt64)
	if limitMB > 0 {
		limitBytes = int64(limitMB) * 1024 * 1024
	}
	debug.SetMemoryLimit(limitBytes)
	if limitMB > 0 {
		zap.L().Info("runtime memory limit applied", zap.Int("limit_mb", limitMB))
		return
	}
	zap.L().Info("runtime memory limit disabled")
}

func runtimeMemoryLimitBytes() int64 {
	return debug.SetMemoryLimit(-1)
}
