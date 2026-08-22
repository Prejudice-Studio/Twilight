package api

import (
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestEmbyActivityUserKeysAndScopedMatching(t *testing.T) {
	events := []embyActivityPlaybackEvent{
		{UserID: "EMBY-1", UserName: "Alice", UserKey: "alice"},
		{UserID: "emby-1", UserName: "Bob"},
	}
	keys := embyActivityUserKeys(events)
	if len(keys) != 3 {
		t.Fatalf("keys=%v, want three normalized keys", keys)
	}
	users := []store.User{
		{UID: 1, EmbyID: "emby-1", Username: "unrelated"},
		{UID: 2, EmbyID: "emby-2", Username: "Alice"},
		{UID: 3, EmbyID: "emby-3", Username: "nobody"},
	}
	matched := make([]store.User, 0, len(users))
	for _, user := range users {
		for _, key := range []string{user.EmbyID, user.EmbyUsername, user.Username} {
			if _, ok := keys[normalizeEmbyActivityUserKey(key)]; ok {
				matched = append(matched, user)
				break
			}
		}
	}
	if len(matched) != 2 || matched[0].UID != 1 || matched[1].UID != 2 {
		t.Fatalf("matched=%v, want users 1 and 2", matched)
	}
}
