package api

import (
	"testing"

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
