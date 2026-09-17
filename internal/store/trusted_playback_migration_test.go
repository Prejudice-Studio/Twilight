package store

import (
	"context"
	"testing"

	"github.com/prejudice-studio/twilight/internal/migration"
	"github.com/prejudice-studio/twilight/internal/playback"
)

func TestTrustedPlaybackMigrationRoundTrip(t *testing.T) {
	st, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	user, err := st.CreateUser(User{Username: "trusted-migration", Active: true, PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := st.applyTrustedPlaybackEvent(ctx, playback.Event{
		UID: user.UID, EventID: "migration-start", PlaybackID: "migration-playback", DeviceID: "migration-device",
		ItemID: "migration-item", Type: playback.EventStarted, At: 1704067190, TimeZone: "UTC",
	}, "client", 1704067190); err != nil {
		t.Fatal(err)
	}
	if _, err := st.applyTrustedPlaybackEvent(ctx, playback.Event{
		UID: user.UID, EventID: "migration-stop", PlaybackID: "migration-playback", DeviceID: "migration-device",
		Type: playback.EventStopped, At: 1704067210, TimeZone: "UTC",
	}, "client", 1704067210); err != nil {
		t.Fatal(err)
	}

	files, err := st.ExportMigrationFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool, len(files))
	for _, file := range files {
		seen[file.Path] = true
	}
	for _, name := range []string{
		"data/trusted-playback-events.json",
		"data/trusted-playback-segments.json",
		"data/trusted-playback-daily.json",
	} {
		if !seen[name] {
			t.Fatalf("export is missing %s", name)
		}
	}

	archiveBytes, _, err := migration.Create(migration.Input{
		TwilightVersion:       "test",
		DatabaseSchemaVersion: "postgres-state-v1",
		Files:                 files,
	})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := migration.Open(archiveBytes, "", migration.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}

	restored := reopenTestStore(t)
	if _, err := restored.ImportMigrationArchive(ctx, archive); err != nil {
		t.Fatal(err)
	}
	summary, err := restored.TrustedPlaybackSummary(ctx, user.UID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Events != 2 || summary.Seconds != 20 || summary.FinalizedEvents != 1 || summary.ActiveSegments != 0 {
		t.Fatalf("trusted playback summary changed after migration: %+v", summary)
	}
	daily, err := restored.TrustedPlaybackDaily(ctx, user.UID, "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	// 这段播放跨了 UTC 午夜（23:59:50 → 次日 00:00:10），splitBuckets 会按本地
	// 日界切成两桶各 10 秒；迁移必须原样保留这个切分。
	if len(daily) != 2 || daily[0].Seconds != 10 || daily[1].Seconds != 10 {
		t.Fatalf("trusted playback daily buckets changed after migration: %+v", daily)
	}
}
