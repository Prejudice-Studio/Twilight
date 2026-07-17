package api

import (
	"testing"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestAdminUserMatchesFilterSharedSemantics(t *testing.T) {
	user := store.User{
		UID:        42,
		Username:   "alice",
		Email:      "alice@example.com",
		Role:       store.RoleNormal,
		Active:     true,
		EmbyID:     "emby-alice",
		TelegramID: 9001,
	}

	cases := []struct {
		name         string
		uidSet       map[int64]bool
		roleFilter   any
		hasRole      bool
		activeFilter any
		hasActive    bool
		embyFilter   string
		search       string
		want         bool
	}{
		{name: "empty filter matches", want: true},
		{name: "uid include matches", uidSet: map[int64]bool{42: true}, want: true},
		{name: "uid include rejects", uidSet: map[int64]bool{7: true}, want: false},
		{name: "role matches", roleFilter: "1", hasRole: true, want: true},
		{name: "role rejects", roleFilter: "0", hasRole: true, want: false},
		{name: "active matches", activeFilter: true, hasActive: true, want: true},
		{name: "active rejects", activeFilter: false, hasActive: true, want: false},
		{name: "emby bound matches", embyFilter: "bound", want: true},
		{name: "emby unbound rejects", embyFilter: "unbound", want: false},
		{name: "search username matches", search: "ali", want: true},
		{name: "search email matches", search: "example.com", want: true},
		{name: "search emby matches", search: "emby-alice", want: true},
		{name: "search uid matches", search: "42", want: true},
		{name: "search telegram matches", search: "9001", want: true},
		{name: "search rejects", search: "missing", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := adminUserMatchesFilter(user, tc.uidSet, tc.roleFilter, tc.hasRole, tc.activeFilter, tc.hasActive, tc.embyFilter, tc.search)
			if got != tc.want {
				t.Fatalf("adminUserMatchesFilter() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAdminUserMatchesListFiltersSharedSemantics(t *testing.T) {
	now := time.Now().Unix()
	boundActive := store.User{
		UID:           101,
		Username:      "active-user",
		Email:         "active@example.com",
		EmailVerified: true,
		Role:          store.RoleNormal,
		Active:        true,
		EmbyID:        "emby-active",
		TelegramID:    5001,
	}
	boundExpired := store.User{
		UID:       102,
		Username:  "expired-user",
		Email:     "expired@example.com",
		Role:      store.RoleNormal,
		Active:    true,
		EmbyID:    "emby-expired",
		ExpiredAt: now - 60,
	}
	noEmail := store.User{
		UID:      103,
		Username: "no-email",
		Role:     store.RoleNormal,
		Active:   false,
	}

	cases := []struct {
		name   string
		user   store.User
		filter adminUserListFilter
		want   bool
	}{
		{name: "empty filter matches", user: boundActive, filter: adminUserListFilter{now: now}, want: true},
		{name: "query active true matches", user: boundActive, filter: adminUserListFilter{hasActive: true, strictQueryActive: true, activeFilter: "true", now: now}, want: true},
		{name: "query active false rejects", user: boundActive, filter: adminUserListFilter{hasActive: true, strictQueryActive: true, activeFilter: "false", now: now}, want: false},
		{name: "payload active false matches", user: noEmail, filter: adminUserListFilter{hasActive: true, activeFilter: false, now: now}, want: true},
		{name: "emby active matches", user: boundActive, filter: adminUserListFilter{embyStatusFilter: "active", now: now}, want: true},
		{name: "expired normal is emby disabled", user: boundExpired, filter: adminUserListFilter{embyStatusFilter: "disabled", now: now}, want: true},
		{name: "expired normal not emby active", user: boundExpired, filter: adminUserListFilter{embyStatusFilter: "active", now: now}, want: false},
		{name: "verified email matches", user: boundActive, filter: adminUserListFilter{emailFilter: "verified", now: now}, want: true},
		{name: "none email matches", user: noEmail, filter: adminUserListFilter{emailFilter: "none", now: now}, want: true},
		{name: "search includes telegram", user: boundActive, filter: adminUserListFilter{search: "5001", now: now}, want: true},
		{name: "search rejects", user: boundActive, filter: adminUserListFilter{search: "missing", now: now}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := adminUserMatchesListFilters(tc.user, tc.filter)
			if got != tc.want {
				t.Fatalf("adminUserMatchesListFilters() = %v, want %v", got, tc.want)
			}
		})
	}
}
