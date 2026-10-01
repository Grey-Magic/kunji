package runner

import (
	"fmt"
	"strconv"
	"strings"
)

// RunStats is the aggregate of a finished run, used for CI threshold checks.
// All counts are absolute; percentages are computed inside Evaluate.
type RunStats struct {
	Total     int
	Valid     int
	Invalid   int
	RateLimit int
	Errored   int
	Skipped   int
}

// Threshold is one parsed "metric op value" clause from --fail-if.
//
//	invalid > 5%      (5 percent of total)
//	invalid > 100     (100 absolute invalid keys)
//	valid >= 50       (at least 50 valid keys)
type Threshold struct {
	Metric   string // "invalid" | "valid" | "error" | "rate_limit" | "skipped" | "total"
	Op       string // ">" | "<" | ">=" | "<="
	Value    float64
	Percent  bool
	RawInput string
}

func (t Threshold) String() string {
	unit := ""
	if t.Percent {
		unit = "%"
	}
	return fmt.Sprintf("%s%s%s%g%s", t.Metric, sep(t.Op), t.Op, t.Value, unit)
}

func sep(op string) string { return " " }

// ParseThreshold parses one --fail-if argument of the form:
//
//	metric op value[%]    e.g. invalid > 5%,  valid >= 10, error < 1%
//
// Whitespace around metric/op/value is optional.
func ParseThreshold(raw string) (Threshold, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Threshold{}, fmt.Errorf("empty threshold")
	}

	// Try operators in length order so ">=" is matched before ">".
	var op string
	var rest string
	for _, candidate := range []string{">=", "<=", ">", "<", "==", "!="} {
		if idx := strings.Index(s, candidate); idx >= 0 {
			op = candidate
			rest = strings.TrimSpace(s[idx+len(candidate):])
			s = strings.TrimSpace(s[:idx])
			break
		}
	}
	if op == "" {
		return Threshold{}, fmt.Errorf("threshold %q: no operator (expected >, <, >=, <=)", raw)
	}

	percent := strings.HasSuffix(rest, "%")
	if percent {
		rest = strings.TrimSpace(strings.TrimSuffix(rest, "%"))
	}
	val, err := strconv.ParseFloat(rest, 64)
	if err != nil {
		return Threshold{}, fmt.Errorf("threshold %q: value not numeric: %v", raw, err)
	}

	metric := strings.ToLower(s)
	switch metric {
	case "invalid", "valid", "error", "errors", "rate_limit", "rate-limit", "ratelimited", "skipped", "total":
		// Normalize synonyms.
		switch metric {
		case "errors":
			metric = "error"
		case "rate-limit", "ratelimited":
			metric = "rate_limit"
		}
	default:
		return Threshold{}, fmt.Errorf("threshold %q: unknown metric %q (allowed: invalid, valid, error, rate_limit, skipped, total)", raw, metric)
	}

	return Threshold{
		Metric:   metric,
		Op:       op,
		Value:    val,
		Percent:  percent,
		RawInput: raw,
	}, nil
}

// ParseThresholds parses every --fail-if argument.
func ParseThresholds(raw []string) ([]Threshold, error) {
	out := make([]Threshold, 0, len(raw))
	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		t, err := ParseThreshold(r)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// Evaluate checks every threshold against stats and returns the violations.
// ok is true when no thresholds were violated.
func Evaluate(thresholds []Threshold, stats RunStats) (violated []Threshold, ok bool) {
	if len(thresholds) == 0 {
		return nil, true
	}
	for _, t := range thresholds {
		got := metricValue(t.Metric, stats)
		var compare float64
		if t.Percent && stats.Total > 0 {
			compare = got / float64(stats.Total) * 100.0
		} else {
			compare = got
		}
		if compareOp(compare, t.Op, t.Value) {
			violated = append(violated, t)
		}
	}
	return violated, len(violated) == 0
}

func metricValue(name string, s RunStats) float64 {
	switch name {
	case "valid":
		return float64(s.Valid)
	case "invalid":
		return float64(s.Invalid)
	case "error":
		return float64(s.Errored)
	case "rate_limit":
		return float64(s.RateLimit)
	case "skipped":
		return float64(s.Skipped)
	case "total":
		return float64(s.Total)
	}
	return 0
}

func compareOp(got float64, op string, want float64) bool {
	switch op {
	case ">":
		return got > want
	case ">=":
		return got >= want
	case "<":
		return got < want
	case "<=":
		return got <= want
	case "==":
		return got == want
	case "!=":
		return got != want
	}
	return false
}

// FormatViolations renders a human-readable list of violated thresholds for
// CI logs.
func FormatViolations(vs []Threshold, stats RunStats) string {
	if len(vs) == 0 {
		return ""
	}
	var b strings.Builder
	for i, t := range vs {
		got := metricValue(t.Metric, stats)
		display := fmt.Sprintf("%g", got)
		if t.Percent && stats.Total > 0 {
			pct := got / float64(stats.Total) * 100.0
			display = fmt.Sprintf("%g (%.2f%%)", got, pct)
		}
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s %s %g%s (got %s)", t.Metric, t.Op, t.Value, percentSuffix(t.Percent), display)
	}
	return b.String()
}

func percentSuffix(p bool) string {
	if p {
		return "%"
	}
	return ""
}
