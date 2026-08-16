package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func setupTelegramMembershipTest(t *testing.T) *App {
	t.Helper()
	app := newTestApp(t)
	app.cfg().TelegramMode = true
	app.cfg().TelegramBotToken = "123:ABC"
	app.cfg().TelegramRequireMembership = true
	app.cfg().TelegramGroupIDs = []string{"-1001"}
	return app
}

func TestTelegramMembershipPendingRebindSkipsDisable(t *testing.T) {
	app := setupTelegramMembershipTest(t)
	user, err := app.store().CreateUser(store.User{
		Username:   "pending-rebind",
		Role:       store.RoleNormal,
		Active:     true,
		TelegramID: 42001,
		EmbyID:     "emby-pending-rebind",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().CreateRebindRequest(store.RebindRequest{UID: user.UID, Username: user.Username, OldTelegramID: user.TelegramID}); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"status":"left","user":{"id":42001,"is_bot":false}}}`))
	}))
	defer tg.Close()
	app.cfg().TelegramAPIURL = tg.URL

	summary, logs, err := app.enforceTelegramMembership(context.Background(), false)
	if err != nil {
		t.Fatalf("membership enforcement failed: %v logs=%v", err, logs)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("pending rebind should skip remote membership checks, got %d", got)
	}
	if int(numeric(summary["disabled"])) != 0 || int(numeric(summary["rebind_protected"])) != 1 || int(numeric(summary["rebind_pending"])) != 1 {
		t.Fatalf("unexpected pending-rebind summary: %#v", summary)
	}
	updated, found := app.store().User(user.UID)
	if !found || !updated.Active {
		t.Fatalf("pending rebind user was disabled: %#v", updated)
	}
}

func TestTelegramMembershipRechecksRebindBeforeDisable(t *testing.T) {
	app := setupTelegramMembershipTest(t)
	user, err := app.store().CreateUser(store.User{
		Username:   "racing-rebind",
		Role:       store.RoleNormal,
		Active:     true,
		TelegramID: 42002,
		EmbyID:     "emby-racing-rebind",
	})
	if err != nil {
		t.Fatal(err)
	}
	checked := make(chan struct{})
	release := make(chan struct{})
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-checked:
		default:
			close(checked)
		}
		<-release
		_, _ = w.Write([]byte(`{"ok":true,"result":{"status":"left","user":{"id":42002,"is_bot":false}}}`))
	}))
	defer tg.Close()
	app.cfg().TelegramAPIURL = tg.URL

	type outcome struct {
		summary map[string]any
		logs    []string
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		summary, logs, runErr := app.enforceTelegramMembership(context.Background(), false)
		done <- outcome{summary: summary, logs: logs, err: runErr}
	}()
	select {
	case <-checked:
	case <-time.After(3 * time.Second):
		t.Fatal("membership check did not start")
	}
	if _, err := app.store().CreateRebindRequest(store.RebindRequest{UID: user.UID, Username: user.Username, OldTelegramID: user.TelegramID}); err != nil {
		t.Fatal(err)
	}
	close(release)
	var result outcome
	select {
	case result = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("membership enforcement did not finish")
	}
	if result.err != nil {
		t.Fatalf("membership enforcement failed: %v logs=%v", result.err, result.logs)
	}
	if int(numeric(result.summary["disabled"])) != 0 || int(numeric(result.summary["rebind_protected"])) != 1 {
		t.Fatalf("rebind submit race was not protected: %#v", result.summary)
	}
	updated, found := app.store().User(user.UID)
	if !found || !updated.Active {
		t.Fatalf("user was disabled despite a concurrent rebind request: %#v", updated)
	}
}
