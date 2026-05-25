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

func defaultErrorCode(status int, success bool) string {
	if success {
		return ""
	}
	switch status {
	case http.StatusBadRequest:
		return "BAD_REQUEST"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusMethodNotAllowed:
		return "METHOD_NOT_ALLOWED"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusGone:
		return "GONE"
	case http.StatusRequestEntityTooLarge:
		return "PAYLOAD_TOO_LARGE"
	case http.StatusTooManyRequests:
		return "RATE_LIMITED"
	case http.StatusBadGateway:
		return "UPSTREAM_ERROR"
	case http.StatusServiceUnavailable:
		return "SERVICE_UNAVAILABLE"
	default:
		if status >= 500 {
			return "INTERNAL_ERROR"
		}
		return "REQUEST_FAILED"
	}
}

func ok(w http.ResponseWriter, message string, data any) {
	writeJSON(w, http.StatusOK, true, message, data)
}

func created(w http.ResponseWriter, message string, data any) {
	writeJSON(w, http.StatusCreated, true, message, data)
}

func fail(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, false, message, nil)
}

// failWithCode 是 fail 的重载，附带业务级错误码（见 errcode.go）。
// 推荐所有领域错误（容量满 / 绑定冲突 / 弱密码等）使用本函数，
// 仅协议层错误（参数缺失 / 鉴权失败等通用类）才使用 fail。
func failWithCode(w http.ResponseWriter, status int, code ErrCode, message string) {
	writeJSONWithCode(w, status, false, code, message, nil)
}

// failWithCodeData 在 failWithCode 基础上允许返回 data（如 system_update 的
// 详细 results 列表）。其它路径请优先用 failWithCode；只有本身就需要把诊断
// 上下文一并下发的接口才使用本函数。
func failWithCodeData(w http.ResponseWriter, status int, code ErrCode, message string, data any) {
	writeJSONWithCode(w, status, false, code, message, data)
}
