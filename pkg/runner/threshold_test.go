package runner

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseThreshold_PercentAndAbsolute(t *testing.T) {
	cases := []struct {
		in      string
		metric  string
		op      string
		value   float64
		percent bool
	}{
		{"invalid > 5%", "invalid", ">", 5, true},
		{"valid >= 10", "valid", ">=", 10, false},
		{"error<1%", "error", "<", 1, true},
		{"rate_limit == 0", "rate_limit", "==", 0, false},
		{"  skipped   >=   3  ", "skipped", ">=", 3, false},
	}
	for _, c := range cases {
		got, err := ParseThreshold(c.in)
		require.NoError(t, err, c.in)
		assert.Equal(t, c.metric, got.Metric, c.in)
		assert.Equal(t, c.op, got.Op, c.in)
		assert.InDelta(t, c.value, got.Value, 1e-9, c.in)
		assert.Equal(t, c.percent, got.Percent, c.in)
	}
}

func TestParseThreshold_Errors(t *testing.T) {
	for _, in := range []string{"", "garbage", "invalid 5%", "bogus > 1"} {
		_, err := ParseThreshold(in)
		assert.Error(t, err, in)
	}
}

func TestParseThreshold_Synonyms(t *testing.T) {
	for _, in := range []string{"errors > 0", "rate-limit >= 1", "ratelimited < 5"} {
		t.Run(in, func(t *testing.T) {
			got, err := ParseThreshold(in)
			require.NoError(t, err)
			switch in {
			case "errors > 0":
				assert.Equal(t, "error", got.Metric)
			case "rate-limit >= 1":
				assert.Equal(t, "rate_limit", got.Metric)
			case "ratelimited < 5":
				assert.Equal(t, "rate_limit", got.Metric)
			}
		})
	}
}

func TestEvaluate_PercentageViolations(t *testing.T) {
	stats := RunStats{Total: 100, Valid: 80, Invalid: 15, RateLimit: 5}

	ts, err := ParseThresholds([]string{"invalid > 10%"})
	require.NoError(t, err)

	violated, ok := Evaluate(ts, stats)
	assert.False(t, ok)
	require.Len(t, violated, 1)
	assert.Equal(t, "invalid", violated[0].Metric)
}

func TestEvaluate_PassesWhenBelowThreshold(t *testing.T) {
	// "fail if invalid < 10%" means: the run fails when invalid is less
	// than 10. Here invalid=15 is NOT less than 10, so no violation.
	stats := RunStats{Total: 100, Valid: 80, Invalid: 15}
	ts, _ := ParseThresholds([]string{"invalid < 10%"})
	_, ok := Evaluate(ts, stats)
	assert.True(t, ok)
}

func TestEvaluate_ViolatesWhenBelowThreshold(t *testing.T) {
	// Companion to the above: invalid=5 IS less than 10, so the threshold
	// "fail if invalid < 10%" is violated.
	stats := RunStats{Total: 100, Valid: 95, Invalid: 5}
	ts, _ := ParseThresholds([]string{"invalid < 10%"})
	_, ok := Evaluate(ts, stats)
	assert.False(t, ok)
}

func TestEvaluate_AbsoluteValueComparison(t *testing.T) {
	// "valid >= 100" with valid=50: condition not held → no violation.
	stats := RunStats{Total: 1000, Valid: 50, Invalid: 950}
	ts, _ := ParseThresholds([]string{"valid >= 100"})
	_, ok := Evaluate(ts, stats)
	assert.True(t, ok, "valid=50 should NOT violate 'valid >= 100' (condition not held)")

	// Now the same threshold with valid=200: condition held → violation.
	stats2 := RunStats{Total: 1000, Valid: 200, Invalid: 800}
	ts2, _ := ParseThresholds([]string{"valid >= 100"})
	violated, ok := Evaluate(ts2, stats2)
	assert.False(t, ok, "valid=200 should violate 'valid >= 100'")
	require.Len(t, violated, 1)
}

func TestEvaluate_MultipleThresholds(t *testing.T) {
	stats := RunStats{Total: 100, Valid: 90, Invalid: 8, Errored: 2}

	ts, _ := ParseThresholds([]string{
		"valid > 80%",
		"invalid < 5%",
	})
	_, ok := Evaluate(ts, stats)
	assert.False(t, ok)
}

func TestFormatViolations(t *testing.T) {
	stats := RunStats{Total: 100, Valid: 80, Invalid: 15}
	ts, _ := ParseThresholds([]string{"invalid > 10%"})
	v, _ := Evaluate(ts, stats)
	out := FormatViolations(v, stats)
	assert.NotEmpty(t, out)
	assert.True(t, strings.Contains(out, "invalid"))
}
