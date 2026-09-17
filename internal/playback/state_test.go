package playback

import "testing"

func TestApplyPauseResumeOnlyCountsPlayingIntervals(t *testing.T) {
	segment := Segment{}
	apply := func(event Event) Result {
		result, err := Apply(&segment, event, 1000)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	apply(Event{UID: 1, PlaybackID: "p1", DeviceID: "device-1", ItemID: "item", Type: EventStarted, At: 100, ReceivedAt: 100, TimeZone: "UTC"})
	if got := apply(Event{UID: 1, PlaybackID: "p1", Type: EventPaused, At: 160, ReceivedAt: 160, TimeZone: "UTC"}).AddedSeconds; got != 60 {
		t.Fatalf("pause added %d seconds, want 60", got)
	}
	if got := apply(Event{UID: 1, PlaybackID: "p1", Type: EventResumed, At: 300, ReceivedAt: 300, TimeZone: "UTC"}).AddedSeconds; got != 0 {
		t.Fatalf("resume counted paused interval: %d", got)
	}
	if got := apply(Event{UID: 1, PlaybackID: "p1", Type: EventCompleted, At: 360, ReceivedAt: 360, TimeZone: "UTC"}).AddedSeconds; got != 60 {
		t.Fatalf("complete added %d seconds, want 60", got)
	}
	if segment.Duration != 120 || !segmentEnded(segment) {
		t.Fatalf("unexpected segment: %+v", segment)
	}
	if segment.DeviceID != "device-1" {
		t.Fatalf("segment device=%q, want device-1", segment.DeviceID)
	}
}

func TestApplyCapsSegmentAndClampsFutureEvent(t *testing.T) {
	segment := Segment{}
	if _, err := Apply(&segment, Event{UID: 2, PlaybackID: "p2", Type: EventStarted, At: 100, ReceivedAt: 100, TimeZone: "UTC"}, 100); err != nil {
		t.Fatal(err)
	}
	result, err := Apply(&segment, Event{UID: 2, PlaybackID: "p2", Type: EventStopped, At: 100000, ReceivedAt: 200, TimeZone: "UTC"}, 200)
	if err != nil {
		t.Fatal(err)
	}
	if result.AddedSeconds != 400 || segment.Duration != 400 {
		t.Fatalf("future timestamp was not bounded: result=%+v segment=%+v", result, segment)
	}

	segment = Segment{}
	if _, err := Apply(&segment, Event{UID: 3, PlaybackID: "p3", Type: EventStarted, At: 1, ReceivedAt: 1, TimeZone: "UTC"}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(&segment, Event{UID: 3, PlaybackID: "p3", Type: EventStopped, At: MaxSegmentSeconds + 10, ReceivedAt: MaxSegmentSeconds + 10, TimeZone: "UTC"}, MaxSegmentSeconds+10); err != nil {
		t.Fatal(err)
	}
	if segment.Duration != MaxSegmentSeconds {
		t.Fatalf("duration=%d, want cap %d", segment.Duration, MaxSegmentSeconds)
	}
}

func TestApplySplitsAcrossLocalDays(t *testing.T) {
	segment := Segment{}
	start := int64(1710028790) // 2024-03-09 23:59:50 UTC
	if _, err := Apply(&segment, Event{UID: 4, PlaybackID: "p4", Type: EventStarted, At: start, ReceivedAt: start, TimeZone: "UTC"}, start); err != nil {
		t.Fatal(err)
	}
	result, err := Apply(&segment, Event{UID: 4, PlaybackID: "p4", Type: EventStopped, At: start + 20, ReceivedAt: start + 20, TimeZone: "UTC"}, start+20)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Buckets) != 2 || result.Buckets[0].Seconds != 10 || result.Buckets[1].Seconds != 10 {
		t.Fatalf("unexpected daily buckets: %+v", result.Buckets)
	}
}

func TestApplyRejectsOutOfOrderSequenceAndInvalidZone(t *testing.T) {
	segment := Segment{}
	if _, err := Apply(&segment, Event{UID: 5, PlaybackID: "p5", Type: EventStarted, Sequence: 2, At: 100, ReceivedAt: 100, TimeZone: "UTC"}, 100); err != nil {
		t.Fatal(err)
	}
	result, err := Apply(&segment, Event{UID: 5, PlaybackID: "p5", Type: EventPlaying, Sequence: 1, At: 200, ReceivedAt: 200, TimeZone: "UTC"}, 200)
	if err != nil {
		t.Fatal(err)
	}
	if result.SegmentChanged || segment.Duration != 0 {
		t.Fatalf("out-of-order sequence changed segment: %+v", segment)
	}
	if _, err := Apply(&Segment{}, Event{UID: 5, PlaybackID: "p6", Type: EventStarted, At: 100, ReceivedAt: 100, TimeZone: "Invalid/Zone"}, 100); err == nil {
		t.Fatal("invalid time zone was accepted")
	}
}

func segmentEnded(segment Segment) bool {
	return segment.Status == EventCompleted && segment.EndedAt > 0
}
