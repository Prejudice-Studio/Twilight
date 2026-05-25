package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAddSigninResetsStreakAfterMissedDay(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	uid := int64(7)
	st.mu.Lock()
	st.state.Signin[uid] = Signin{
		UID:        uid,
		Points:     10,
		Streak:     5,
		LastSignin: time.Now().AddDate(0, 0, -2).Format("2006-01-02"),
	}
	st.mu.Unlock()
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	si, created, err := st.AddSignin(uid, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected signin to be created")
	}
	if si.Streak != 1 {
		t.Fatalf("expected streak reset to 1 after missed day, got %d", si.Streak)
	}
	if si.LongestStreak != 5 {
		t.Fatalf("expected longest streak to preserve previous streak, got %d", si.LongestStreak)
	}
}

func TestAddSigninContinuesStreakFromYesterday(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	uid := int64(8)
	st.mu.Lock()
	st.state.Signin[uid] = Signin{
		UID:        uid,
		Points:     2,
		Streak:     2,
		LastSignin: time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
	}
	st.mu.Unlock()
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	si, created, err := st.AddSignin(uid, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected signin to be created")
	}
	if si.Streak != 3 {
		t.Fatalf("expected streak to continue to 3, got %d", si.Streak)
	}
	if si.LongestStreak != 3 {
		t.Fatalf("expected longest streak to update to 3, got %d", si.LongestStreak)
	}
}

func TestAddSigninDoesNotCreateDuplicateForSameDay(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	uid := int64(9)
	first, created, err := st.AddSigninWithOptions(uid, 2, func(streak int) int {
		if streak != 1 {
			t.Fatalf("bonus received streak %d", streak)
		}
		return 3
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !created || first.Points != 5 || len(first.Records) != 1 {
		t.Fatalf("unexpected first signin: created=%v signin=%#v", created, first)
	}

	second, created, err := st.AddSignin(uid, 2)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("expected duplicate same-day signin to be ignored")
	}
	if second.Points != first.Points || len(second.Records) != len(first.Records) {
		t.Fatalf("duplicate signin changed state: first=%#v second=%#v", first, second)
	}
}
