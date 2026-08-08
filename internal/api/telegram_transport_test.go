package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestDecodeTelegramEnvelopeDirectResult(t *testing.T) {
	raw := `{"result":[{"update_id":7,"message":{"text":"hello"}}],"ok":true,"unknown":{"ignored":true}}`
	payload, err := decodeTelegramEnvelope[[]map[string]any](strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !payload.OK || len(payload.Result) != 1 || numeric(payload.Result[0]["update_id"]) != 7 {
		t.Fatalf("unexpected payload: %#v", payload)
	}

	discarded, err := decodeTelegramEnvelope[telegramDiscardResult](strings.NewReader(`{"ok":true,"result":{"message_id":42,"text":"unused"}}`))
	if err != nil || !discarded.OK {
		t.Fatalf("discard result failed: payload=%#v err=%v", discarded, err)
	}
}

func TestDecodeTelegramTypedUpdates(t *testing.T) {
	raw := `{"ok":true,"result":[` +
		`{"update_id":1,"message":{"message_id":11,"text":"/ping","chat":{"id":-100,"type":"supergroup"},"from":{"id":9007199254740993,"is_bot":false,"username":"alice"},"sender_chat":{"id":-100},"reply_to_message":{"from":{"id":77}}}},` +
		`{"update_id":2,"callback_query":{"id":"cb","data":"gadm:act:close:token","from":{"id":8,"username":"admin"},"message":{"message_id":12,"chat":{"id":-100,"type":"supergroup"}}}},` +
		`{"update_id":3,"chat_member":{"chat":{"id":-200,"type":"supergroup"},"from":{"id":9},"new_chat_member":{"status":"left","user":{"id":10,"is_bot":true,"username":"helper"}}}},` +
		`{"update_id":4,"my_chat_member":{"chat":{"id":-300,"type":"supergroup"},"from":{"id":11},"new_chat_member":{"status":"administrator","user":{"id":12,"is_bot":true}}}}]}`
	payload, err := decodeTelegramEnvelope[[]telegramUpdate](strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Result) != 4 {
		t.Fatalf("typed update count=%d, want 4", len(payload.Result))
	}
	message := payload.Result[0].Message
	if message == nil || message.From.ID != 9007199254740993 || message.SenderChat.ID != -100 || message.ReplyToMessage == nil || message.ReplyToMessage.From.ID != 77 {
		t.Fatalf("typed message lost fields: %#v", message)
	}
	callback := payload.Result[1].CallbackQuery
	if callback == nil || callback.ID != "cb" || callback.From.ID != 8 || callback.Message == nil || callback.Message.MessageID != 12 {
		t.Fatalf("typed callback lost fields: %#v", callback)
	}
	membership := payload.Result[2].ChatMember
	if membership == nil || membership.Chat.ID != -200 || membership.From.ID != 9 || membership.NewChatMember.Status != "left" || membership.NewChatMember.User.ID != 10 || !membership.NewChatMember.User.IsBot {
		t.Fatalf("typed membership lost fields: %#v", membership)
	}
	if payload.Result[3].MyChatMember == nil || payload.Result[3].MyChatMember.NewChatMember.Status != "administrator" {
		t.Fatalf("typed my_chat_member lost fields: %#v", payload.Result[3].MyChatMember)
	}
}

func TestDecodeTelegramTypedChatMembers(t *testing.T) {
	raw := `{"ok":true,"result":[` +
		`{"status":"creator","user":{"id":9007199254740993,"is_bot":false,"username":"owner"},"custom_title":"ignored"},` +
		`{"status":"administrator","user":{"id":8,"is_bot":true,"username":"helper"}}]}`
	payload, err := decodeTelegramEnvelope[[]telegramChatMember](strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Result) != 2 || payload.Result[0].User.ID != 9007199254740993 || payload.Result[0].Status != "creator" {
		t.Fatalf("typed administrators lost fields: %#v", payload.Result)
	}
	if !payload.Result[1].User.IsBot || !telegramMemberIsAdminOrBot(payload.Result[1]) {
		t.Fatalf("typed bot administrator not recognized: %#v", payload.Result[1])
	}
	if !telegramMemberIsGone(telegramChatMember{Status: "LEFT"}) {
		t.Fatal("case-insensitive left status was not recognized")
	}
}

func TestDecodeTelegramTypedIdentityAndChat(t *testing.T) {
	identity, err := decodeTelegramEnvelope[telegramUser](strings.NewReader(`{"ok":true,"result":{"id":42,"is_bot":true,"username":"twilight_bot","first_name":"ignored"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if identity.Result.ID != 42 || !identity.Result.IsBot || identity.Result.Username != "twilight_bot" {
		t.Fatalf("typed identity lost fields: %#v", identity.Result)
	}
	chat, err := decodeTelegramEnvelope[telegramChat](strings.NewReader(`{"ok":true,"result":{"id":-1001,"type":"supergroup","title":"Twilight","username":"twilight_group","description":"ignored"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if chat.Result.ID != -1001 || chat.Result.Type != "supergroup" || chat.Result.Title != "Twilight" || chat.Result.Username != "twilight_group" {
		t.Fatalf("typed chat lost fields: %#v", chat.Result)
	}
}

func TestTelegramConfigAdminIndexRefreshesWithSnapshotSlices(t *testing.T) {
	app := newTestApp(t)
	app.cfg().TelegramAdminIDs = []int64{11, 22}
	if !app.telegramAdminID(11) || !app.telegramAdminID(22) || app.telegramAdminID(33) {
		t.Fatal("configured administrator index returned the wrong initial membership")
	}
	app.cfg().TelegramAdminIDs = []int64{33}
	if app.telegramAdminID(11) || !app.telegramAdminID(33) {
		t.Fatal("configured administrator index did not refresh after slice replacement")
	}
	user, err := app.store().CreateUser(store.User{Username: "dynamic-admin", TelegramID: 44, Role: store.RoleAdmin, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if !app.telegramAdminID(44) {
		t.Fatal("store-backed administrator role was not recognized")
	}
	if _, err := app.store().UpdateUser(user.UID, func(current *store.User) error {
		current.Role = store.RoleNormal
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if app.telegramAdminID(44) {
		t.Fatal("store-backed administrator role remained cached after demotion")
	}
}

func TestTelegramRenderPanelTemplateCompatibility(t *testing.T) {
	values := map[string]string{"username": "alice", "uid": "42"}
	tests := []string{
		"plain text",
		"{username}/{uid}/{username}",
		"unknown={future_key}; user={username}",
		"malformed={future_{username}}",
		"unterminated={username",
	}
	for _, template := range tests {
		pairs := make([]string, 0, len(values)*2)
		for key, value := range values {
			pairs = append(pairs, "{"+key+"}", value)
		}
		want := strings.NewReplacer(pairs...).Replace(template)
		if got := telegramRenderPanelTemplate(template, values); got != want {
			t.Fatalf("template %q rendered as %q, want %q", template, got, want)
		}
	}
}

func TestDecodeTelegramEnvelopeRejectsTrailingAndOversizedData(t *testing.T) {
	if _, err := decodeTelegramEnvelope[map[string]any](strings.NewReader(`{"ok":true,"result":{}} {"extra":true}`)); err == nil {
		t.Fatal("multiple top-level JSON values were accepted")
	}
	prefix := `{"ok":true,"result":"`
	suffix := `"}`
	exact := prefix + strings.Repeat("x", telegramMaxResponseBytes-len(prefix)-len(suffix)) + suffix
	if _, err := decodeTelegramEnvelope[string](strings.NewReader(exact)); err != nil {
		t.Fatalf("exact-size response was rejected: %v", err)
	}
	if _, err := decodeTelegramEnvelope[string](strings.NewReader(exact + " ")); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized response error=%v", err)
	}
}

func TestTelegramTypedRequestJSONCompatibility(t *testing.T) {
	tests := []struct {
		name string
		body any
		want map[string]any
	}{
		{
			name: "plain message omits optional fields",
			body: telegramSendMessageRequest{ChatID: int64(42), Text: "hello", DisableWebPagePreview: true},
			want: map[string]any{"chat_id": float64(42), "text": "hello", "disable_web_page_preview": true},
		},
		{
			name: "message markup",
			body: telegramSendMessageRequest{ChatID: "@channel", Text: "hello", ParseMode: "HTML", DisableWebPagePreview: true, ReplyMarkup: map[string]any{"inline_keyboard": []any{}}},
			want: map[string]any{"chat_id": "@channel", "text": "hello", "parse_mode": "HTML", "disable_web_page_preview": true, "reply_markup": map[string]any{"inline_keyboard": []any{}}},
		},
		{
			name: "edit message",
			body: telegramEditMessageRequest{ChatID: 7, MessageID: 9, Text: "updated", DisableWebPagePreview: true},
			want: map[string]any{"chat_id": float64(7), "message_id": float64(9), "text": "updated", "disable_web_page_preview": true},
		},
		{
			name: "callback keeps false alert",
			body: telegramAnswerCallbackRequest{CallbackQueryID: "cb", Text: "done", ShowAlert: false},
			want: map[string]any{"callback_query_id": "cb", "text": "done", "show_alert": false},
		},
		{
			name: "ban keeps false revoke",
			body: telegramBanMemberRequest{ChatID: "-1001", UserID: 8, RevokeMessages: false},
			want: map[string]any{"chat_id": "-1001", "user_id": float64(8), "revoke_messages": false},
		},
		{
			name: "unban",
			body: telegramUnbanMemberRequest{ChatID: "-1001", UserID: 8, OnlyIfBanned: true},
			want: map[string]any{"chat_id": "-1001", "user_id": float64(8), "only_if_banned": true},
		},
		{
			name: "initial update poll omits offset",
			body: telegramGetUpdatesRequest{Timeout: 30, AllowedUpdates: telegramAllowedUpdates},
			want: map[string]any{"timeout": float64(30), "allowed_updates": []any{"message", "callback_query", "chat_member", "my_chat_member"}},
		},
		{
			name: "subsequent update poll includes offset",
			body: telegramGetUpdatesRequest{Offset: 99, Timeout: 30, AllowedUpdates: telegramAllowedUpdates},
			want: map[string]any{"offset": float64(99), "timeout": float64(30), "allowed_updates": []any{"message", "callback_query", "chat_member", "my_chat_member"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("request JSON=%s\ngot:  %#v\nwant: %#v", raw, got, tt.want)
			}
		})
	}
}

func TestTelegramHTTPErrorPreservesRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":9}}`))
	}))
	defer server.Close()
	_, err := telegramPostResultToEndpoint[telegramDiscardResult](context.Background(), server.URL, "sendMessage", struct{}{}, time.Second)
	if err == nil {
		t.Fatal("expected Telegram HTTP error")
	}
	if retry, ok := telegramRetryAfterFromError(err); !ok || retry != 9*time.Second {
		t.Fatalf("retry_after=%v ok=%v err=%v", retry, ok, err)
	}
}

func TestTelegramTransportReportsNonJSONStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer server.Close()
	_, err := telegramPostResultToEndpoint[telegramDiscardResult](context.Background(), server.URL, "sendMessage", struct{}{}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "remote status 502") || !strings.Contains(err.Error(), "upstream unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTelegramTransportPreservesCallerDeadline(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := telegramPostResultToEndpoint[telegramDiscardResult](ctx, server.URL, "getUpdates", struct{}{}, time.Second)
	if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("expected caller deadline error, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("caller deadline was replaced by transport timeout: %v", elapsed)
	}
}

func TestTelegramTransportSanitizesTokenFromAllErrorSources(t *testing.T) {
	const token = "123:VERY_SECRET_TOKEN"
	tests := []struct {
		name    string
		handler http.Handler
		closed  bool
	}{
		{
			name: "error body",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "upstream echoed "+token, http.StatusBadGateway)
			}),
		},
		{
			name: "redirect location",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "https://example.com/bot"+token+"/getMe")
				w.WriteHeader(http.StatusFound)
			}),
		},
		{name: "network error", handler: http.NotFoundHandler(), closed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			baseURL := server.URL
			if tt.closed {
				server.Close()
			} else {
				defer server.Close()
			}

			app := newTestApp(t)
			app.cfg().TelegramMode = true
			app.cfg().TelegramBotToken = token
			app.cfg().TelegramAPIURL = baseURL
			_, err := app.telegramGetMe(context.Background())
			if err == nil {
				t.Fatal("expected Telegram transport error")
			}
			if strings.Contains(err.Error(), token) {
				t.Fatalf("Telegram token leaked in error: %v", err)
			}
			if !strings.Contains(err.Error(), "<redacted>") {
				t.Fatalf("sanitized error lacks redaction marker: %v", err)
			}
		})
	}
}

func TestTelegramSendMessageDiscardsUnusedMessageResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":"unused-invalid-type","text":"ignored"}}`))
	}))
	defer server.Close()

	app := newTestApp(t)
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	app.cfg().TelegramAPIURL = server.URL
	if err := app.telegramSendMessage(context.Background(), 42, "pong"); err != nil {
		t.Fatalf("ordinary message retained unused result: %v", err)
	}
	if _, err := app.telegramSendMessageWithMarkup(context.Background(), 42, "panel", nil); err == nil {
		t.Fatal("message-id path accepted an invalid message_id")
	}
}

func BenchmarkTelegramUpdateEnvelopeDecode(b *testing.B) {
	updates := make([]map[string]any, 100)
	for i := range updates {
		updates[i] = map[string]any{
			"update_id": i + 1,
			"message": map[string]any{
				"message_id": i + 10,
				"chat":       map[string]any{"id": -1001234567890, "type": "supergroup"},
				"from":       map[string]any{"id": i + 1000, "username": fmt.Sprintf("member_%d", i)},
				"text":       strings.Repeat("telegram update payload ", 12),
			},
		}
	}
	raw, err := json.Marshal(map[string]any{"ok": true, "result": updates})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(len(raw)), "response_bytes")

	b.Run("legacy_read_raw_decode", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			copied, readErr := io.ReadAll(bytes.NewReader(raw))
			if readErr != nil {
				b.Fatal(readErr)
			}
			var envelope telegramResponse
			if err := json.Unmarshal(copied, &envelope); err != nil {
				b.Fatal(err)
			}
			var result []map[string]any
			if err := json.Unmarshal(envelope.Result, &result); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("single_dynamic_decode", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			payload, err := decodeTelegramEnvelope[[]map[string]any](bytes.NewReader(raw))
			if err != nil {
				b.Fatal(err)
			}
			if len(payload.Result) != len(updates) {
				b.Fatal("decoded update count mismatch")
			}
		}
	})

	b.Run("typed_update_decode", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			payload, err := decodeTelegramEnvelope[[]telegramUpdate](bytes.NewReader(raw))
			if err != nil {
				b.Fatal(err)
			}
			if len(payload.Result) != len(updates) || payload.Result[0].Message == nil {
				b.Fatal("decoded typed update count mismatch")
			}
		}
	})

	b.Run("interleaved_compare", func(b *testing.B) {
		var dynamicDuration time.Duration
		var typedDuration time.Duration
		decodeDynamic := func() {
			started := time.Now()
			payload, decodeErr := decodeTelegramEnvelope[[]map[string]any](bytes.NewReader(raw))
			dynamicDuration += time.Since(started)
			if decodeErr != nil || len(payload.Result) != len(updates) {
				b.Fatal("dynamic update decode failed")
			}
		}
		decodeTyped := func() {
			started := time.Now()
			payload, decodeErr := decodeTelegramEnvelope[[]telegramUpdate](bytes.NewReader(raw))
			typedDuration += time.Since(started)
			if decodeErr != nil || len(payload.Result) != len(updates) || payload.Result[0].Message == nil {
				b.Fatal("typed update decode failed")
			}
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if i&1 == 0 {
				decodeDynamic()
				decodeTyped()
			} else {
				decodeTyped()
				decodeDynamic()
			}
		}
		b.StopTimer()
		dynamicNS := float64(dynamicDuration.Nanoseconds()) / float64(b.N)
		typedNS := float64(typedDuration.Nanoseconds()) / float64(b.N)
		b.ReportMetric(dynamicNS, "dynamic_ns/op")
		b.ReportMetric(typedNS, "typed_ns/op")
		b.ReportMetric(typedNS/dynamicNS, "typed/dynamic")
	})

}

func BenchmarkTelegramChatAdministratorsDecode(b *testing.B) {
	admins := make([]map[string]any, 100)
	for i := range admins {
		admins[i] = map[string]any{
			"status": "administrator",
			"user": map[string]any{
				"id":         i + 1000,
				"is_bot":     i%10 == 0,
				"username":   fmt.Sprintf("admin_%d", i),
				"first_name": strings.Repeat("administrator ", 4),
			},
			"can_manage_chat": true,
		}
	}
	raw, err := json.Marshal(map[string]any{"ok": true, "result": admins})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(len(raw)), "response_bytes")

	b.Run("dynamic", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			payload, decodeErr := decodeTelegramEnvelope[[]map[string]any](bytes.NewReader(raw))
			if decodeErr != nil || len(payload.Result) != len(admins) {
				b.Fatal("dynamic administrator decode failed")
			}
		}
	})
	b.Run("typed", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			payload, decodeErr := decodeTelegramEnvelope[[]telegramChatMember](bytes.NewReader(raw))
			if decodeErr != nil || len(payload.Result) != len(admins) {
				b.Fatal("typed administrator decode failed")
			}
		}
	})
	b.Run("interleaved_compare", func(b *testing.B) {
		var dynamicDuration time.Duration
		var typedDuration time.Duration
		decodeDynamic := func() {
			started := time.Now()
			payload, decodeErr := decodeTelegramEnvelope[[]map[string]any](bytes.NewReader(raw))
			dynamicDuration += time.Since(started)
			if decodeErr != nil || len(payload.Result) != len(admins) {
				b.Fatal("dynamic administrator decode failed")
			}
		}
		decodeTyped := func() {
			started := time.Now()
			payload, decodeErr := decodeTelegramEnvelope[[]telegramChatMember](bytes.NewReader(raw))
			typedDuration += time.Since(started)
			if decodeErr != nil || len(payload.Result) != len(admins) {
				b.Fatal("typed administrator decode failed")
			}
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if i&1 == 0 {
				decodeDynamic()
				decodeTyped()
			} else {
				decodeTyped()
				decodeDynamic()
			}
		}
		b.StopTimer()
		dynamicNS := float64(dynamicDuration.Nanoseconds()) / float64(b.N)
		typedNS := float64(typedDuration.Nanoseconds()) / float64(b.N)
		b.ReportMetric(dynamicNS, "dynamic_ns/op")
		b.ReportMetric(typedNS, "typed_ns/op")
		b.ReportMetric(typedNS/dynamicNS, "typed/dynamic")
	})
}

func BenchmarkTelegramPanelTemplateRender(b *testing.B) {
	template := "user={username} uid={uid} status={web_status} remote={emby_remote_status} unknown={future_key} again={username}"
	values := map[string]string{
		"username":           "alice",
		"uid":                "42",
		"web_status":         "启用",
		"emby_remote_status": "已找到",
		"unused_1":           "unused",
		"unused_2":           "unused",
		"unused_3":           "unused",
		"unused_4":           "unused",
	}
	legacy := func() string {
		pairs := make([]string, 0, len(values)*2)
		for key, value := range values {
			pairs = append(pairs, "{"+key+"}", value)
		}
		return strings.NewReplacer(pairs...).Replace(template)
	}
	want := legacy()
	if got := telegramRenderPanelTemplate(template, values); got != want {
		b.Fatalf("render mismatch: got %q want %q", got, want)
	}
	b.Run("legacy_replacer", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if legacy() != want {
				b.Fatal("legacy render changed")
			}
		}
	})
	b.Run("single_pass", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if telegramRenderPanelTemplate(template, values) != want {
				b.Fatal("single-pass render changed")
			}
		}
	})
}
