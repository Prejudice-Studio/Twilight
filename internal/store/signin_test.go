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

func TestAddSigninTrimsExcessRecords(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	uid := int64(10)
	// 填充超出 maxSigninRecords 的记录，模拟旧数据
	oldRecords := make([]SigninRecord, 0, maxSigninRecords+10)
	for i := 0; i < maxSigninRecords+10; i++ {
		oldRecords = append(oldRecords, SigninRecord{
			Date:      "2000-01-01",
			Points:    1,
			Total:     1,
			Streak:    1,
			CreatedAt: 1000000000,
		})
	}
	st.mu.Lock()
	st.state.Signin[uid] = Signin{
		UID:        uid,
		Points:     100,
		Streak:     1,
		LastSignin: time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
		Records:    oldRecords,
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
	if len(si.Records) > maxSigninRecords {
		t.Fatalf("records not trimmed: got %d, want <= %d", len(si.Records), maxSigninRecords)
	}
	if len(si.Records) != maxSigninRecords {
		t.Fatalf("expected exactly %d records after trim, got %d", maxSigninRecords, len(si.Records))
	}
}

func TestSigninHistoryRecordsReturnsLatestWindow(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	uid := int64(11)
	st.mu.Lock()
	st.state.Signin[uid] = Signin{
		UID: uid,
		Records: []SigninRecord{
			{Date: "2026-01-01", CreatedAt: 1},
			{Date: "2026-01-02", CreatedAt: 2},
			{Date: "2026-01-03", CreatedAt: 3},
		},
	}
	st.mu.Unlock()
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	records := st.SigninHistoryRecords(uid, 2)
	if len(records) != 2 || records[0].Date != "2026-01-03" || records[1].Date != "2026-01-02" {
		t.Fatalf("unexpected history window: %#v", records)
	}
	records[0].Date = "mutated"
	again := st.SigninHistoryRecords(uid, 1)
	if len(again) != 1 || again[0].Date != "2026-01-03" {
		t.Fatalf("history records should be returned as copies, got %#v", again)
	}
	if empty := st.SigninHistoryRecords(999, 30); len(empty) != 0 {
		t.Fatalf("missing user history should be empty, got %#v", empty)
	}
}
