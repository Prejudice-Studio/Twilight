package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBangumiCollectionCacheCloneAndDelete(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now().Unix()
	entry := BangumiCollectionCacheEntry{
		UID:       42,
		Username:  "alice",
		Type:      3,
		Total:     1,
		UpdatedAt: now,
		ExpiresAt: now + 3600,
		Entries: []map[string]any{{
			"subject_id": float64(1001),
			"subject": map[string]any{
				"name": "demo",
			},
		}},
	}
	if err := st.UpsertBangumiCollectionCache(entry); err != nil {
		t.Fatal(err)
	}
	raw := st.state.BangumiCollectionCache[bangumiCollectionCacheKey(42, 3)]
	if _, ok := raw.Entries[0]["subject"]; ok {
		t.Fatal("user-scoped collection cache should not store duplicated subject payload")
	}
	if got := len(st.state.BangumiSubjectCache); got != 1 {
		t.Fatalf("expected one global subject cache entry, got %d", got)
	}

	cached, ok := st.BangumiCollectionCache(42, 3)
	if !ok {
		t.Fatal("expected bangumi collection cache")
	}
	cached.Entries[0]["subject_id"] = float64(9999)
	cached.Entries[0]["subject"].(map[string]any)["name"] = "mutated"

	cachedAgain, ok := st.BangumiCollectionCache(42, 3)
	if !ok {
		t.Fatal("expected bangumi collection cache after clone mutation")
	}
	if got := cachedAgain.Entries[0]["subject_id"]; got != float64(1001) {
		t.Fatalf("cache entry was not cloned: subject_id=%v", got)
	}
	if got := cachedAgain.Entries[0]["subject"].(map[string]any)["name"]; got != "demo" {
		t.Fatalf("nested cache entry was not cloned: name=%v", got)
	}

	if err := st.UpsertBangumiCollectionCache(BangumiCollectionCacheEntry{UID: 42, Type: 1, Entries: []map[string]any{{
		"subject_id": float64(1001),
		"subject": map[string]any{
			"name": "demo",
		},
	}}, Total: 1}); err != nil {
		t.Fatal(err)
	}
	if got := len(st.state.BangumiSubjectCache); got != 1 {
		t.Fatalf("same Bangumi subject should be cached globally once, got %d", got)
	}
	if err := st.UpsertBangumiCollectionCache(BangumiCollectionCacheEntry{UID: 7, Type: 3, Entries: []map[string]any{}, Total: 0}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteBangumiCollectionCache(42, 3); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.BangumiCollectionCache(42, 3); ok {
		t.Fatal("type-specific cache should be deleted")
	}
	if _, ok := st.BangumiCollectionCache(42, 1); !ok {
		t.Fatal("other type cache should remain")
	}
	if err := st.DeleteBangumiCollectionCache(42, 0); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.BangumiCollectionCache(42, 1); ok {
		t.Fatal("all caches for uid should be deleted")
	}
	if _, ok := st.BangumiCollectionCache(7, 3); !ok {
		t.Fatal("other user's cache should remain")
	}
}

func TestDeleteUserRemovesBangumiCollectionCache(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	user, err := st.CreateUser(User{Username: "target", Active: true, PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateUser(User{Username: "other", Active: true, PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertBangumiCollectionCache(BangumiCollectionCacheEntry{UID: user.UID, Type: 3, Entries: []map[string]any{{
		"subject_id": float64(1),
		"subject": map[string]any{
			"name": "shared",
		},
	}}, Total: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertBangumiCollectionCache(BangumiCollectionCacheEntry{UID: other.UID, Type: 3, Entries: []map[string]any{{"subject_id": float64(2)}}, Total: 1}); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteUser(user.UID); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.BangumiCollectionCache(user.UID, 3); ok {
		t.Fatal("deleted user's bangumi collection cache should be removed")
	}
	if _, ok := st.BangumiCollectionCache(other.UID, 3); !ok {
		t.Fatal("other user's bangumi collection cache should remain")
	}
	if _, ok := st.state.BangumiSubjectCache[bangumiSubjectCacheKey(1)]; !ok {
		t.Fatal("global subject cache should remain after deleting one user")
	}
}

func TestEnsureMigratesLegacyBangumiCollectionSubjects(t *testing.T) {
	state := State{
		BangumiCollectionCache: map[string]BangumiCollectionCacheEntry{
			bangumiCollectionCacheKey(9, 3): {
				UID:  9,
				Type: 3,
				Entries: []map[string]any{{
					"subject_id": float64(3001),
					"ep_status":  float64(4),
					"subject": map[string]any{
						"id":   float64(3001),
						"name": "legacy",
					},
				}},
			},
		},
	}

	state.ensure()

	entry := state.BangumiCollectionCache[bangumiCollectionCacheKey(9, 3)]
	if _, ok := entry.Entries[0]["subject"]; ok {
		t.Fatal("legacy user collection cache should be normalized without subject payload")
	}
	subject, ok := state.BangumiSubjectCache[bangumiSubjectCacheKey(3001)]
	if !ok {
		t.Fatal("legacy subject should be migrated into global subject cache")
	}
	if got := subject.Subject["name"]; got != "legacy" {
		t.Fatalf("unexpected migrated subject name: %v", got)
	}
}
