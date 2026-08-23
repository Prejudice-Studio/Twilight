package api

import (
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

// TestRefreshTelegramUsername 覆盖被动刷新的三种情形：用户名变化写库、空用户名
// 保留旧值、未绑定 telegram id 安全 no-op。
func TestRefreshTelegramUsername(t *testing.T) {
	app := newTestApp(t)
	u, err := app.store().CreateUser(store.User{Username: "alice", TelegramID: 555, TelegramUsername: "old_name"})
	if err != nil {
		t.Fatal(err)
	}

	// 新用户名（带 @ 前缀）→ 去前缀后写库。
	app.refreshTelegramUsername(555, "@New_Name")
	if got, _ := app.store().User(u.UID); got.TelegramUsername != "New_Name" {
		t.Fatalf("expected username refreshed to New_Name, got %q", got.TelegramUsername)
	}

	// 空用户名（对方删了 @username）→ 保留旧值，不清空。
	app.refreshTelegramUsername(555, "")
	if got, _ := app.store().User(u.UID); got.TelegramUsername != "New_Name" {
		t.Fatalf("empty username must not clear stored username, got %q", got.TelegramUsername)
	}

	// 相同用户名 → 无变化（不报错）。
	app.refreshTelegramUsername(555, "New_Name")
	if got, _ := app.store().User(u.UID); got.TelegramUsername != "New_Name" {
		t.Fatalf("idempotent refresh changed username, got %q", got.TelegramUsername)
	}

	// 未知 telegram id / 非法 id → 安全 no-op，不 panic。
	app.refreshTelegramUsername(999, "ghost")
	app.refreshTelegramUsername(0, "ignored")
}

func TestTelegramGroupUserSearchSupportsFuzzyAndExplicitFields(t *testing.T) {
	for _, test := range []struct {
		input string
		query string
		field store.UserIdentitySearchField
	}{
		{input: "2345 uid", query: "2345", field: store.UserIdentitySearchUID},
		{input: "testuser username", query: "testuser", field: store.UserIdentitySearchUsername},
		{input: "2345 tgid", query: "2345", field: store.UserIdentitySearchTelegramID},
		{input: "@testuser tgname", query: "testuser", field: store.UserIdentitySearchTelegramUsername},
		{input: "testuser", query: "testuser", field: store.UserIdentitySearchAny},
	} {
		got, reason := telegramParseGroupUserSearch(test.input)
		if reason != "" || got.Query != test.query || got.Field != test.field {
			t.Fatalf("parse %q = %#v reason=%q", test.input, got, reason)
		}
	}
}

func TestTelegramGroupUserResolutionKeepsBoundedCandidateSet(t *testing.T) {
	app := newTestApp(t)
	first, err := app.store().CreateUser(store.User{Username: "match-one", Role: store.RoleNormal, TelegramUsername: "same-name"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.store().CreateUser(store.User{Username: "match-two", Role: store.RoleNormal, TelegramUsername: "same-name-two"})
	if err != nil {
		t.Fatal(err)
	}
	resolution := app.telegramResolveGroupUserTargetValues("same tgname", 0)
	if resolution.Reason != "" || len(resolution.Users) != 2 {
		t.Fatalf("unexpected resolution: %#v", resolution)
	}
	panel := telegramPanelContext{CandidateUIDs: telegramUserUIDs(resolution.Users)}
	if !telegramPanelCandidateAllowed(panel, first.UID) || !telegramPanelCandidateAllowed(panel, second.UID) {
		t.Fatalf("matched users missing from candidate set: %#v", panel)
	}
	if telegramPanelCandidateAllowed(panel, first.UID+second.UID+1) {
		t.Fatalf("unmatched user was accepted by candidate set: %#v", panel)
	}
}
