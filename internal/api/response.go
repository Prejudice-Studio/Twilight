package api

import (
	"encoding/json"
	"net/http"
	"time"
)

type envelope struct {
	Success   bool   `json:"success"`
	Code      int    `json:"code"`
	ErrorCode string `json:"error_code,omitempty"`
	Message   string `json:"message"`
	Data      any    `json:"data,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

func writeJSON(w http.ResponseWriter, status int, success bool, message string, data any) {
	writeJSONWithCode(w, status, success, "", message, data)
}

// writeJSONWithCode 在 writeJSON 基础上允许传入业务级 error_code（见 errcode.go）。
// 当 errorCode 为空时回落到按 HTTP status 自动推导（defaultErrorCode）。
func writeJSONWithCode(w http.ResponseWriter, status int, success bool, errorCode, message string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-store, private")
	}
	if w.Header().Get("Pragma") == "" {
		w.Header().Set("Pragma", "no-cache")
	}
	w.WriteHeader(status)
	resolvedCode := errorCode
	if resolvedCode == "" {
		resolvedCode = defaultErrorCode(status, success)
	}
	_ = json.NewEncoder(w).Encode(envelope{
		Success:   success,
		Code:      status,
		ErrorCode: resolvedCode,
		Message:   message,
		Data:      data,
		Timestamp: time.Now().Unix(),
	})
}

// statusToErrorCode 把 HTTP status 直接映射到协议层 ErrCode。
// 历史实现是一连串 case 字面量，每加一个 status 就要改一处 switch；
// 用 map 改成纯数据后：
//
//  1. 增删 status 只改一行字面量，避免 case 顺序与漏写；
//  2. response_test.go 可以直接 range map 反向校验，确保 errcode.go
//     的常量没有被静默改动；
//  3. 5xx 不在 map 里，由 defaultErrorCode 兜底为 ErrInternal——这是
//     经验：未知 5xx 一律算服务端故障，不要随便回成 REQUEST_FAILED
//     让前端把它当用户输入错误重试。
var statusToErrorCode = map[int]string{
	http.StatusBadRequest:            ErrBadRequest,
	http.StatusUnauthorized:          ErrUnauthorized,
	http.StatusForbidden:             ErrForbidden,
	http.StatusNotFound:              ErrNotFound,
	http.StatusMethodNotAllowed:      ErrMethodNotAllowed,
	http.StatusConflict:              ErrConflict,
	http.StatusGone:                  ErrGone,
	http.StatusRequestEntityTooLarge: ErrPayloadTooLarge,
	http.StatusTooManyRequests:       ErrRateLimited,
	http.StatusBadGateway:            ErrUpstreamError,
	http.StatusServiceUnavailable:    ErrServiceUnavailable,
	http.StatusInternalServerError:   ErrInternal,
}

func defaultErrorCode(status int, success bool) string {
	if success {
		return ""
	}
	if code, ok := statusToErrorCode[status]; ok {
		return code
	}
	if status >= 500 {
		return ErrInternal
	}
	return ErrRequestFailed
}

func ok(w http.ResponseWriter, message string, data any) {
	writeJSON(w, http.StatusOK, true, message, data)
}

func created(w http.ResponseWriter, message string, data any) {
	writeJSON(w, http.StatusCreated, true, message, data)
}

// fail/failWithCode/failWithCodeData 是失败响应的统一出口。所有 message
// 在落到 envelope 前都会过 redactSensitiveText —— 不少 handler 习惯直接
// `failWithCode(..., err.Error())`，store / pgx / db driver 的错误链可能
// 携带 token、连接串、bearer 等敏感片段；handler 一处漏挡就直接随 5xx
// envelope 回到客户端 / 浏览器历史 / 网关日志。集中在此处兜底比逐 handler
// 包 redact 更可靠，匹配不到的普通文案保持原样，无副作用。
func fail(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, false, redactSensitiveText(message), nil)
}

// failWithCode 是 fail 的重载，附带业务级错误码（见 errcode.go）。
// 推荐所有领域错误（容量满 / 绑定冲突 / 弱密码等）使用本函数，
// 仅协议层错误（参数缺失 / 鉴权失败等通用类）才使用 fail。
func failWithCode(w http.ResponseWriter, status int, code ErrCode, message string) {
	writeJSONWithCode(w, status, false, code, redactSensitiveText(message), nil)
}

// failWithCodeData 在 failWithCode 基础上允许返回 data（如 system_update 的
// 详细 results 列表）。其它路径请优先用 failWithCode；只有本身就需要把诊断
// 上下文一并下发的接口才使用本函数。
func failWithCodeData(w http.ResponseWriter, status int, code ErrCode, message string, data any) {
	writeJSONWithCode(w, status, false, code, redactSensitiveText(message), data)
}
