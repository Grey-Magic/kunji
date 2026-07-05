package models

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamEvent_MarshalProducesSingleLineJSON(t *testing.T) {
	e := NewResultEvent(&ValidationResult{
		Key:      "sk-test",
		Provider: "openai",
		IsValid:  true,
	})
	out, err := e.Marshal()
	require.NoError(t, err)

	// Must be exactly one line, ending in a newline.
	assert.False(t, bytes.Contains(out[:len(out)-1], []byte("\n")), "payload must not contain embedded newline before trailing newline")
	assert.Equal(t, byte('\n'), out[len(out)-1], "must end with newline")

	// Round-trip: must decode as a single JSON object.
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1024), 1024*1024)
	require.True(t, sc.Scan(), "must produce at least one line")
	var decoded StreamEvent
	require.NoError(t, json.Unmarshal(sc.Bytes(), &decoded))
	assert.Equal(t, EventResult, decoded.Event)
	assert.Equal(t, SchemaVersion, decoded.Version)
	require.NotNil(t, decoded.Result)
	assert.Equal(t, "openai", decoded.Result.Provider)
}

func TestStreamEvent_EnvelopeDiscriminator(t *testing.T) {
	start := NewStartEvent(&StartEvent{Total: 10, Threads: 4})
	prog := NewProgressEvent(&ProgressEvent{Done: 5, Total: 10, Valid: 4, SpeedKeys: 12.5, ETASeconds: 0.5})
	sum := NewSummaryEvent(&SummaryEvent{Total: 10, Valid: 4, Invalid: 5, RateLimit: 1, DurationMs: 1234})

	cases := []struct {
		name string
		e    *StreamEvent
		typ  EventType
	}{
		{"start", start, EventStartMeta},
		{"progress", prog, EventProgress},
		{"summary", sum, EventSummary},
		{"result", NewResultEvent(&ValidationResult{Provider: "p"}), EventResult},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.e.Marshal()
			require.NoError(t, err)
			var d StreamEvent
			require.NoError(t, json.Unmarshal(out, &d))
			assert.Equal(t, tc.typ, d.Event)
		})
	}
}

func TestStreamEvent_TimestampAlwaysSet(t *testing.T) {
	e := NewResultEvent(&ValidationResult{Provider: "x"})
	out, err := e.Marshal()
	require.NoError(t, err)
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &raw))
	assert.NotEmpty(t, raw["timestamp"], "timestamp field must be present")
	assert.True(t, strings.Contains(raw["timestamp"].(string), "T"),
		"timestamp should be ISO-8601 / RFC3339")
}

func TestSummaryEvent_ByProviderEncoding(t *testing.T) {
	s := NewSummaryEvent(&SummaryEvent{
		Total: 10,
		Valid: 4,
		ByProvider: map[string]ProviderTotals{
			"openai": {Valid: 4, Invalid: 1, RateLimit: 0, Skipped: 0},
			"stripe": {Valid: 0, Invalid: 5, RateLimit: 1, Skipped: 0},
		},
	})
	out, err := s.Marshal()
	require.NoError(t, err)
	var d StreamEvent
	require.NoError(t, json.Unmarshal(out, &d))
	assert.Equal(t, EventSummary, d.Event)
	require.NotNil(t, d.Summary)
	require.NotNil(t, d.Summary.ByProvider)
	assert.Equal(t, 4, d.Summary.ByProvider["openai"].Valid)
	assert.Equal(t, 1, d.Summary.ByProvider["stripe"].RateLimit)
}
