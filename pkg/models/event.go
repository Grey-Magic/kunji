package models

import (
	"encoding/json"
	"time"
)

// EventType discriminates stream events emitted in JSONL mode.
type EventType string

const (
	EventResult    EventType = "result"
	EventProgress  EventType = "progress"
	EventSummary   EventType = "summary"
	EventStartMeta EventType = "start"
)

// StreamEvent is the canonical envelope for `--format jsonl`.
//
// Exactly one of Result / Progress / Summary / StartMeta is non-nil based on Type.
// Fields outside the event payload carry a common envelope: event type, an
// ISO-8601 timestamp, and a stable schema version.
type StreamEvent struct {
	Event     EventType         `json:"event"`
	Timestamp string            `json:"timestamp"`
	Version   string            `json:"schema_version"`
	Result    *ValidationResult `json:"result,omitempty"`
	Progress  *ProgressEvent    `json:"progress,omitempty"`
	Summary   *SummaryEvent     `json:"summary,omitempty"`
	Start     *StartEvent       `json:"start,omitempty"`
}

const SchemaVersion = "1.0.0"

// ProgressEvent is emitted periodically while validation runs.
type ProgressEvent struct {
	Done       int     `json:"done"`
	Total      int     `json:"total"`
	Valid      int     `json:"valid"`
	Invalid    int     `json:"invalid"`
	RateLimit  int     `json:"rate_limited"`
	Skipped    int     `json:"skipped"`
	SpeedKeys  float64 `json:"keys_per_second"`
	ETASeconds float64 `json:"eta_seconds"`
}

// SummaryEvent is emitted exactly once at end of run.
type SummaryEvent struct {
	Total      int                       `json:"total"`
	Valid      int                       `json:"valid"`
	Invalid    int                       `json:"invalid"`
	RateLimit  int                       `json:"rate_limited"`
	Skipped    int                       `json:"skipped"`
	CacheHits  int                       `json:"cache_hits"`
	DurationMs int64                     `json:"duration_ms"`
	ByProvider map[string]ProviderTotals `json:"by_provider,omitempty"`
}

// ProviderTotals is a per-provider tally in the summary event.
type ProviderTotals struct {
	Valid     int `json:"valid"`
	Invalid   int `json:"invalid"`
	RateLimit int `json:"rate_limited"`
	Skipped   int `json:"skipped"`
}

// StartEvent announces total key count and run configuration at start.
type StartEvent struct {
	Total   int    `json:"total"`
	Threads int    `json:"threads"`
	Proxy   string `json:"proxy,omitempty"`
}

// NewResultEvent wraps a ValidationResult in the envelope.
func NewResultEvent(r *ValidationResult) *StreamEvent {
	return &StreamEvent{
		Event:     EventResult,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Version:   SchemaVersion,
		Result:    r,
	}
}

// NewProgressEvent wraps a progress snapshot.
func NewProgressEvent(p *ProgressEvent) *StreamEvent {
	return &StreamEvent{
		Event:     EventProgress,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Version:   SchemaVersion,
		Progress:  p,
	}
}

// NewSummaryEvent wraps the run summary.
func NewSummaryEvent(s *SummaryEvent) *StreamEvent {
	return &StreamEvent{
		Event:     EventSummary,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Version:   SchemaVersion,
		Summary:   s,
	}
}

// NewStartEvent wraps the run-start metadata.
func NewStartEvent(s *StartEvent) *StreamEvent {
	return &StreamEvent{
		Event:     EventStartMeta,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Version:   SchemaVersion,
		Start:     s,
	}
}

// Marshal returns one line of compact JSON with a trailing newline.
func (e *StreamEvent) Marshal() ([]byte, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(b)+1)
	out = append(out, b...)
	out = append(out, '\n')
	return out, nil
}
