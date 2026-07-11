package api

import (
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestEmbyPlaybackEventsFromLogsPairsStartAndStop(t *testing.T) {
	logs := []store.EmbyActivityLog{
		{EmbyLogID: 4, Type: "playback.stop", ItemID: "episode-1", UserID: "user-1", Date: 220},
		{EmbyLogID: 2, Type: "playback.start", ItemID: "episode-1", UserID: "user-1", Date: 100},
		{EmbyLogID: 1, Type: "playback.start", ItemID: "outside", UserID: "user-1", Date: 10},
		{EmbyLogID: 3, Type: "playback.stop", ItemID: "outside", UserID: "user-1", Date: 90},
	}

	events := embyPlaybackEventsFromLogs(logs, 80, 300)
	if len(events) != 2 {
		t.Fatalf("events=%d want 2: %#v", len(events), events)
	}
	if events[0].ItemID != "outside" || events[0].Duration != 10 {
		t.Fatalf("window-clipped event = %+v", events[0])
	}
	if events[1].ItemID != "episode-1" || events[1].Duration != 120 {
		t.Fatalf("paired event = %+v", events[1])
	}
}

func TestCountEmbyPlayingSessionsIgnoresIdleSessions(t *testing.T) {
	sessions := []map[string]any{
		{"Id": "playing", "NowPlayingItem": map[string]any{"Id": "item"}},
		{"Id": "idle", "IsActive": true},
		{"Id": "offline"},
	}
	if got := countEmbyPlayingSessions(sessions); got != 1 {
		t.Fatalf("playing sessions=%d want 1", got)
	}
}
