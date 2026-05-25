package api

import (
	"net/http"
	"testing"
)

func TestDefaultErrorCodeCoversAPIStatuses(t *testing.T) {
	tests := map[int]string{
		http.StatusBadRequest:            "BAD_REQUEST",
		http.StatusUnauthorized:          "UNAUTHORIZED",
		http.StatusForbidden:             "FORBIDDEN",
		http.StatusNotFound:              "NOT_FOUND",
		http.StatusMethodNotAllowed:      "METHOD_NOT_ALLOWED",
		http.StatusConflict:              "CONFLICT",
		http.StatusGone:                  "GONE",
		http.StatusRequestEntityTooLarge: "PAYLOAD_TOO_LARGE",
		http.StatusTooManyRequests:       "RATE_LIMITED",
		http.StatusBadGateway:            "UPSTREAM_ERROR",
		http.StatusServiceUnavailable:    "SERVICE_UNAVAILABLE",
		http.StatusInternalServerError:   "INTERNAL_ERROR",
	}

	for status, want := range tests {
		if got := defaultErrorCode(status, false); got != want {
			t.Fatalf("status %d error code = %q, want %q", status, got, want)
		}
	}
	if got := defaultErrorCode(http.StatusOK, true); got != "" {
		t.Fatalf("success error code = %q", got)
	}
}
