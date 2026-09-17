package store

import (
	"context"
	"errors"
	"testing"

	"github.com/prejudice-studio/twilight/internal/playback"
)

func TestTrustedPlaybackEventIsAtomicAndIdempotent(t *testing.T) {
	st, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	const now = int64(1000)
	events := []playback.Event{
		{UID: 1, EventID: "e-start", PlaybackID: "play-1", DeviceID: "phone", ItemID: "item-1", Type: playback.EventStarted, At: 100, TimeZone: "UTC"},
		{UID: 1, EventID: "e-play", PlaybackID: "play-1", DeviceID: "phone", Type: playback.EventPlaying, At: 160, Sequence: 2, TimeZone: "UTC"},
		{UID: 1, EventID: "e-pause", PlaybackID: "play-1", DeviceID: "phone", Type: playback.EventPaused, At: 220, Sequence: 3, TimeZone: "UTC"},
		{UID: 1, EventID: "e-resume", PlaybackID: "play-1", DeviceID: "phone", Type: playback.EventResumed, At: 300, Sequence: 4, TimeZone: "UTC"},
		{UID: 1, EventID: "e-stop", PlaybackID: "play-1", DeviceID: "phone", Type: playback.EventStopped, At: 360, Sequence: 5, TimeZone: "UTC"},
	}
	wantAdded := []int64{0, 60, 60, 0, 60}
	for i, event := range events {
		result, err := st.applyTrustedPlaybackEvent(ctx, event, "client", now)
		if err != nil {
			t.Fatalf("event %s: %v", event.EventID, err)
		}
		if result.Duplicate || result.AddedSeconds != wantAdded[i] {
			t.Fatalf("event %s result=%+v, want added=%d", event.EventID, result, wantAdded[i])
		}
	}

	duplicate, err := st.applyTrustedPlaybackEvent(ctx, events[1], "client", now+500)
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate.Duplicate || duplicate.AddedSeconds != 60 {
		t.Fatalf("duplicate result=%+v, want prior result", duplicate)
	}

	summary, err := st.TrustedPlaybackSummary(ctx, 1, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Events != 5 || summary.Seconds != 180 || summary.ActiveSegments != 0 || summary.FinalizedEvents != 1 {
		t.Fatalf("unexpected trusted playback summary: %+v", summary)
	}
	daily, err := st.TrustedPlaybackDaily(ctx, 1, "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(daily) != 1 || daily[0].Seconds != 180 || daily[0].Day != "1970-01-01" {
		t.Fatalf("unexpected trusted playback daily rows: %+v", daily)
	}
}

func TestTrustedPlaybackKeepsSamePlaybackSeparateByDevice(t *testing.T) {
	st, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	for _, device := range []string{"phone", "tablet"} {
		start, err := st.applyTrustedPlaybackEvent(ctx, playback.Event{
			UID: 2, EventID: "start-" + device, PlaybackID: "same-playback", DeviceID: device,
			ItemID: "item-2", Type: playback.EventStarted, At: 100, TimeZone: "UTC",
		}, "client", 100)
		if err != nil || start.Segment.DeviceID != device {
			t.Fatalf("start device=%s result=%+v err=%v", device, start, err)
		}
		if _, err := st.applyTrustedPlaybackEvent(ctx, playback.Event{
			UID: 2, EventID: "stop-" + device, PlaybackID: "same-playback", DeviceID: device,
			Type: playback.EventStopped, At: 130, TimeZone: "UTC",
		}, "client", 130); err != nil {
			t.Fatal(err)
		}
	}

	summary, err := st.TrustedPlaybackSummary(ctx, 2, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Events != 4 || summary.Seconds != 60 {
		t.Fatalf("same playback was not isolated by device: %+v", summary)
	}
}

func TestTrustedPlaybackRejectsInvalidEventWithoutLeavingRows(t *testing.T) {
	st, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	_, err = st.applyTrustedPlaybackEvent(context.Background(), playback.Event{
		UID: 3, EventID: "bad", PlaybackID: "play-3", DeviceID: "device-3",
		Type: playback.EventPaused, At: 100, TimeZone: "UTC",
	}, "client", 100)
	if err == nil {
		t.Fatal("pause without an active segment should fail")
	}
	summary, err := st.TrustedPlaybackSummary(context.Background(), 3, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Events != 0 || summary.Seconds != 0 {
		t.Fatalf("failed transaction left trusted playback rows: %+v", summary)
	}
}

func TestTrustedPlaybackRejectsSameEventIDWithDifferentPayload(t *testing.T) {
	st, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	first := playback.Event{
		UID: 4, EventID: "same-event", PlaybackID: "play-4", DeviceID: "phone",
		ItemID: "item-a", Type: playback.EventStarted, At: 100, TimeZone: "UTC",
	}
	if _, err := st.applyTrustedPlaybackEvent(context.Background(), first, "client", 100); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ItemID = "item-b"
	if _, err := st.applyTrustedPlaybackEvent(context.Background(), second, "client", 100); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected payload conflict, got %v", err)
	}
}

func TestTrustedPlaybackRejectsInvalidDayRangeAndTimezone(t *testing.T) {
	st, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.TrustedPlaybackDaily(context.Background(), 1, "2026-02-30", "", 10); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected invalid date, got %v", err)
	}
	if _, err := st.TrustedPlaybackDaily(context.Background(), 1, "2026-03-02", "2026-03-01", 10); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected reversed range error, got %v", err)
	}
	_, err = st.applyTrustedPlaybackEvent(context.Background(), playback.Event{
		UID: 5, EventID: "bad-zone", PlaybackID: "play-5", Type: playback.EventStarted,
		At: 100, TimeZone: "not/a-zone",
	}, "client", 100)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected invalid timezone, got %v", err)
	}
}
