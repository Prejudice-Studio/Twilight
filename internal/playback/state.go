// Package playback contains the deterministic domain rules for trusted
// playback events. It does not know about HTTP, PostgreSQL, Emby, or users.
package playback

import (
	"errors"
	"fmt"
	"time"
)

const (
	EventStarted   EventType = "started"
	EventPlaying   EventType = "playing"
	EventPaused    EventType = "paused"
	EventResumed   EventType = "resumed"
	EventStopped   EventType = "stopped"
	EventCompleted EventType = "completed"
	EventExpired   EventType = "expired"

	// MaxSegmentSeconds prevents a missing stop/heartbeat from becoming
	// unlimited watch time. A scheduler can close stale sessions with expired.
	MaxSegmentSeconds    int64 = 12 * 60 * 60
	maxFutureSkewSeconds       = 5 * 60
)

var (
	ErrInvalidEvent      = errors.New("invalid playback event")
	ErrUnsupportedEvent  = errors.New("unsupported playback event type")
	ErrInvalidTimeZone   = errors.New("invalid playback time zone")
	ErrInvalidTransition = errors.New("invalid playback state transition")
)

type EventType string

type Event struct {
	EventID    string
	UID        int64
	PlaybackID string
	DeviceID   string
	ItemID     string
	Title      string
	SeriesName string
	MediaType  string
	Type       EventType
	At         int64
	ReceivedAt int64
	Sequence   int64
	TimeZone   string
}

type Segment struct {
	UID          int64
	PlaybackID   string
	DeviceID     string
	ItemID       string
	Title        string
	SeriesName   string
	MediaType    string
	StartedAt    int64
	LastAt       int64
	EndedAt      int64
	Duration     int64
	Status       EventType
	LastSequence int64
	TimeZone     string
}

type DailyBucket struct {
	UID      int64
	Day      string
	TimeZone string
	Seconds  int64
}

type Result struct {
	SegmentChanged bool
	Finalized      bool
	AddedSeconds   int64
	Buckets        []DailyBucket
}

// Apply advances one playback segment with one event. Event de-duplication is
// owned by the persistence layer through EventID; Sequence additionally
// protects the segment from older/out-of-order client retries.
func Apply(segment *Segment, event Event, now int64) (Result, error) {
	if segment == nil {
		return Result{}, fmt.Errorf("%w: nil segment", ErrInvalidEvent)
	}
	if err := validateEvent(event); err != nil {
		return Result{}, err
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	at := CanonicalEventTime(event, now)
	if segment.UID == 0 {
		segment.UID = event.UID
	}
	if segment.PlaybackID == "" {
		segment.PlaybackID = event.PlaybackID
	}
	if segment.UID != event.UID || segment.PlaybackID != event.PlaybackID {
		return Result{}, fmt.Errorf("%w: event does not belong to segment", ErrInvalidEvent)
	}
	if segment.LastSequence > 0 && event.Sequence > 0 && event.Sequence <= segment.LastSequence {
		return Result{}, nil
	}
	if segment.Status == EventCompleted || segment.Status == EventStopped || segment.Status == EventExpired {
		if event.Sequence > segment.LastSequence {
			segment.LastSequence = event.Sequence
		}
		return Result{}, nil
	}

	if segment.TimeZone == "" {
		segment.TimeZone = event.TimeZone
		if segment.TimeZone == "" {
			segment.TimeZone = "UTC"
		}
	}
	if _, err := time.LoadLocation(segment.TimeZone); err != nil {
		return Result{}, fmt.Errorf("%w: %s", ErrInvalidTimeZone, segment.TimeZone)
	}
	if segment.DeviceID == "" {
		segment.DeviceID = event.DeviceID
	}
	if segment.ItemID == "" {
		segment.ItemID, segment.Title, segment.SeriesName, segment.MediaType = event.ItemID, event.Title, event.SeriesName, event.MediaType
	}
	if segment.StartedAt == 0 {
		if event.Type != EventStarted && event.Type != EventPlaying && event.Type != EventResumed {
			return Result{}, fmt.Errorf("%w: %s without an active segment", ErrInvalidTransition, event.Type)
		}
		segment.StartedAt = at
		segment.LastAt = at
		segment.Status = EventPlaying
	}

	result := Result{}
	if at < segment.LastAt {
		at = segment.LastAt
	}
	if segment.Status == EventPlaying && at > segment.LastAt {
		seconds := at - segment.LastAt
		remaining := MaxSegmentSeconds - segment.Duration
		if remaining < 0 {
			remaining = 0
		}
		if seconds > remaining {
			seconds = remaining
		}
		if seconds > 0 {
			result.Buckets = splitBuckets(segment.UID, segment.TimeZone, segment.LastAt, segment.LastAt+seconds)
			segment.Duration += seconds
			result.AddedSeconds = seconds
		}
	}
	segment.LastAt = at
	result.SegmentChanged = true

	switch event.Type {
	case EventStarted, EventPlaying, EventResumed:
		segment.Status = EventPlaying
	case EventPaused:
		segment.Status = EventPaused
	case EventStopped, EventCompleted, EventExpired:
		segment.Status = event.Type
		segment.EndedAt = at
		result.Finalized = true
	default:
		return Result{}, fmt.Errorf("%w: %s", ErrUnsupportedEvent, event.Type)
	}
	if event.Sequence > segment.LastSequence {
		segment.LastSequence = event.Sequence
	}
	return result, nil
}

func validateEvent(event Event) error {
	if event.UID <= 0 || event.PlaybackID == "" || event.Type == "" {
		return fmt.Errorf("%w: missing identity or type", ErrInvalidEvent)
	}
	switch event.Type {
	case EventStarted, EventPlaying, EventPaused, EventResumed, EventStopped, EventCompleted, EventExpired:
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedEvent, event.Type)
	}
	if event.At <= 0 && event.ReceivedAt <= 0 {
		return fmt.Errorf("%w: missing event time", ErrInvalidEvent)
	}
	zone := event.TimeZone
	if zone == "" {
		zone = "UTC"
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidTimeZone, zone)
	}
	return nil
}

// CanonicalEventTime returns the event timestamp used by the state machine.
// Persistence adapters should use the same value so reporting cannot observe
// a future or otherwise different timestamp from the one used for duration.
func CanonicalEventTime(event Event, now int64) int64 {
	at := event.At
	if at <= 0 {
		at = event.ReceivedAt
	}
	if at <= 0 {
		at = now
	}
	received := event.ReceivedAt
	if received <= 0 {
		received = now
	}
	if at > received+maxFutureSkewSeconds {
		at = received + maxFutureSkewSeconds
	}
	return at
}

func splitBuckets(uid int64, zone string, start, end int64) []DailyBucket {
	if uid <= 0 || end <= start {
		return nil
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		location = time.UTC
	}
	buckets := make([]DailyBucket, 0, 2)
	for cursor := start; cursor < end; {
		local := time.Unix(cursor, 0).In(location)
		nextDay := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, location).Unix()
		boundary := end
		if nextDay > cursor && nextDay < boundary {
			boundary = nextDay
		}
		seconds := boundary - cursor
		if seconds > 0 {
			day := local.Format("2006-01-02")
			if len(buckets) > 0 && buckets[len(buckets)-1].Day == day {
				buckets[len(buckets)-1].Seconds += seconds
			} else {
				buckets = append(buckets, DailyBucket{UID: uid, Day: day, TimeZone: zone, Seconds: seconds})
			}
		}
		if boundary <= cursor {
			break
		}
		cursor = boundary
	}
	return buckets
}
