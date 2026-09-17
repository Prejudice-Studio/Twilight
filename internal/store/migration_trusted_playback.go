package store

// These DTOs are deliberately separate from the runtime playback structs.
// Migration files are an external, versioned format and must not inherit
// internal fields or database scan details by accident.
type migrationTrustedPlaybackEvent struct {
	UID            int64  `json:"uid"`
	EventID        string `json:"event_id"`
	PlaybackID     string `json:"playback_id"`
	DeviceID       string `json:"device_id"`
	ItemID         string `json:"item_id"`
	Title          string `json:"title,omitempty"`
	SeriesName     string `json:"series_name,omitempty"`
	MediaType      string `json:"media_type,omitempty"`
	EventType      string `json:"event_type"`
	EventAt        int64  `json:"event_at"`
	ReceivedAt     int64  `json:"received_at"`
	Sequence       int64  `json:"sequence"`
	TimeZone       string `json:"time_zone"`
	Source         string `json:"source"`
	PayloadHash    string `json:"payload_hash,omitempty"`
	AppliedSeconds int64  `json:"applied_seconds"`
	Finalized      bool   `json:"finalized"`
	ProcessedAt    int64  `json:"processed_at"`
}

type migrationTrustedPlaybackSegment struct {
	UID          int64  `json:"uid"`
	PlaybackID   string `json:"playback_id"`
	DeviceID     string `json:"device_id"`
	ItemID       string `json:"item_id"`
	Title        string `json:"title,omitempty"`
	SeriesName   string `json:"series_name,omitempty"`
	MediaType    string `json:"media_type,omitempty"`
	StartedAt    int64  `json:"started_at"`
	LastAt       int64  `json:"last_at"`
	EndedAt      int64  `json:"ended_at"`
	Duration     int64  `json:"duration"`
	Status       string `json:"status"`
	LastSequence int64  `json:"last_sequence"`
	TimeZone     string `json:"time_zone"`
	UpdatedAt    int64  `json:"updated_at"`
}

type migrationTrustedPlaybackDaily struct {
	UID       int64  `json:"uid"`
	Day       string `json:"day"`
	TimeZone  string `json:"time_zone"`
	Seconds   int64  `json:"seconds"`
	UpdatedAt int64  `json:"updated_at"`
}
