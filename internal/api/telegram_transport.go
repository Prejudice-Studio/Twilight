package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const telegramMaxResponseBytes = 4 << 20

type telegramEnvelope[T any] struct {
	OK          bool                       `json:"ok"`
	Result      T                          `json:"result"`
	Description string                     `json:"description"`
	ErrorCode   int                        `json:"error_code"`
	Parameters  telegramResponseParameters `json:"parameters"`
}

// telegramDiscardResult accepts any Telegram result without retaining it. It
// keeps write-only API calls on the same single-decode transport without
// allocating a RawMessage copy for an object or boolean the caller never uses.
type telegramDiscardResult struct{}

func (*telegramDiscardResult) UnmarshalJSON([]byte) error { return nil }

type telegramMessageResult struct {
	MessageID int64 `json:"message_id"`
}

type telegramSendMessageRequest struct {
	ChatID                any    `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode,omitempty"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
	ReplyMarkup           any    `json:"reply_markup,omitempty"`
}

type telegramEditMessageRequest struct {
	ChatID                int64  `json:"chat_id"`
	MessageID             int64  `json:"message_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode,omitempty"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
	ReplyMarkup           any    `json:"reply_markup,omitempty"`
}

type telegramDeleteMessageRequest struct {
	ChatID    int64 `json:"chat_id"`
	MessageID int64 `json:"message_id"`
}

type telegramAnswerCallbackRequest struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text"`
	ShowAlert       bool   `json:"show_alert"`
}

type telegramChatMemberRequest struct {
	ChatID string `json:"chat_id"`
	UserID int64  `json:"user_id"`
}

type telegramBanMemberRequest struct {
	ChatID         string `json:"chat_id"`
	UserID         int64  `json:"user_id"`
	RevokeMessages bool   `json:"revoke_messages"`
}

type telegramUnbanMemberRequest struct {
	ChatID       string `json:"chat_id"`
	UserID       int64  `json:"user_id"`
	OnlyIfBanned bool   `json:"only_if_banned"`
}

type telegramGetUpdatesRequest struct {
	Offset         int64     `json:"offset,omitempty"`
	Timeout        int       `json:"timeout"`
	AllowedUpdates [4]string `json:"allowed_updates"`
}

var telegramAllowedUpdates = [4]string{"message", "callback_query", "chat_member", "my_chat_member"}

func telegramPostResult[T any](a *App, ctx context.Context, method string, body any, timeout time.Duration) (T, error) {
	var zero T
	if !a.telegramAvailable() {
		return zero, fmt.Errorf("Telegram is not enabled or bot token is not configured")
	}
	endpoint, err := a.telegramEndpoint(method)
	if err != nil {
		return zero, fmt.Errorf("%s", a.telegramSanitizeError(err))
	}
	result, err := telegramPostResultToEndpoint[T](ctx, endpoint, method, body, timeout)
	if err != nil {
		return zero, fmt.Errorf("%s", a.telegramSanitizeError(err))
	}
	return result, nil
}

func telegramPostResultToEndpoint[T any](ctx context.Context, endpoint, method string, body any, timeout time.Duration) (T, error) {
	var zero T
	data, err := json.Marshal(body)
	if err != nil {
		return zero, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return zero, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if _, ok := req.Context().Deadline(); !ok {
		requestCtx, cancel := context.WithTimeout(req.Context(), timeout)
		defer cancel()
		req = req.WithContext(requestCtx)
	}
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		return zero, telegramDecodeErrorResponse(resp, method)
	}
	payload, err := decodeTelegramEnvelope[T](resp.Body)
	if err != nil {
		return zero, err
	}
	if !payload.OK {
		return zero, telegramEnvelopeError(method, payload.Description, payload.ErrorCode, payload.Parameters)
	}
	return payload.Result, nil
}

func decodeTelegramEnvelope[T any](reader io.Reader) (telegramEnvelope[T], error) {
	var payload telegramEnvelope[T]
	limited := &io.LimitedReader{R: reader, N: telegramMaxResponseBytes + 1}
	decoder := json.NewDecoder(limited)
	if err := decoder.Decode(&payload); err != nil {
		if limited.N <= 0 {
			return payload, fmt.Errorf("Telegram API response exceeds %d bytes", telegramMaxResponseBytes)
		}
		return payload, err
	}
	var trailing telegramDiscardResult
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return payload, fmt.Errorf("Telegram API returned multiple JSON values")
		}
		return payload, err
	}
	if limited.N <= 0 {
		return payload, fmt.Errorf("Telegram API response exceeds %d bytes", telegramMaxResponseBytes)
	}
	return payload, nil
}

func telegramDecodeErrorResponse(resp *http.Response, method string) error {
	if resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode < http.StatusBadRequest {
		location := strings.TrimSpace(resp.Header.Get("Location"))
		if location == "" {
			return fmt.Errorf("remote status %d: cross-host or excessive redirect refused", resp.StatusCode)
		}
		return fmt.Errorf("remote status %d: cross-host redirect refused (Location=%s)", resp.StatusCode, truncateString(location, 200))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var payload telegramResponse
	if len(bytes.TrimSpace(raw)) > 0 && json.Unmarshal(raw, &payload) == nil && !payload.OK {
		return telegramEnvelopeError(method, payload.Description, payload.ErrorCode, payload.Parameters)
	}
	detail := strings.TrimSpace(string(raw))
	if detail != "" {
		return fmt.Errorf("remote status %d: %s", resp.StatusCode, truncateString(detail, 300))
	}
	return fmt.Errorf("remote status %d", resp.StatusCode)
}

func telegramEnvelopeError(method, description string, errorCode int, parameters telegramResponseParameters) error {
	message := strings.TrimSpace(description)
	if message == "" {
		message = "Telegram API request failed"
	}
	if parameters.RetryAfter > 0 {
		if errorCode != 0 {
			return fmt.Errorf("telegram %s failed: %s (%d) [%s%d]", method, message, errorCode, telegramRetryAfterSentinel, parameters.RetryAfter)
		}
		return fmt.Errorf("telegram %s failed: %s [%s%d]", method, message, telegramRetryAfterSentinel, parameters.RetryAfter)
	}
	if errorCode != 0 {
		return fmt.Errorf("telegram %s failed: %s (%d)", method, message, errorCode)
	}
	return fmt.Errorf("telegram %s failed: %s", method, message)
}

func (a *App) telegramPostNoResult(ctx context.Context, method string, body any) error {
	_, err := telegramPostResult[telegramDiscardResult](a, ctx, method, body, 20*time.Second)
	return err
}
