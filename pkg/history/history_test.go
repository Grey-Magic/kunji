package history

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogger_AppendsOneLinePerEmit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	l, err := Open(path, "run-1")
	require.NoError(t, err)

	require.NoError(t, l.Emit(&models.ValidationResult{Provider: "openai", Key: "sk-a", IsValid: true, StatusCode: 200}))
	require.NoError(t, l.Emit(&models.ValidationResult{Provider: "openai", Key: "sk-a", IsValid: false, StatusCode: 401}))
	require.NoError(t, l.Close())

	records, err := ReadAll(path)
	require.NoError(t, err)
	assert.Len(t, records, 2)
	assert.Equal(t, "run-1", records[0].RunID)
	assert.Equal(t, "openai", records[0].Provider)
}

func TestLogger_EmptyPathIsNoop(t *testing.T) {
	l, err := Open("", "x")
	require.NoError(t, err)
	require.NoError(t, l.Emit(&models.ValidationResult{Provider: "p", Key: "k"}))
	require.NoError(t, l.Close())
	assert.Empty(t, l.Path())
}

func TestByKey_GroupsAndSortsNewestFirst(t *testing.T) {
	now := time.Now()
	records := []Record{
		{Provider: "openai", KeySHA256: "h1", Timestamp: now.Add(-2 * time.Hour).Format(time.RFC3339Nano), IsValid: true},
		{Provider: "openai", KeySHA256: "h1", Timestamp: now.Format(time.RFC3339Nano), IsValid: false},
		{Provider: "stripe", KeySHA256: "h2", Timestamp: now.Format(time.RFC3339Nano), IsValid: true},
	}
	groups := ByKey(records)
	assert.Len(t, groups["openai|h1"], 2)
	// Newest first.
	assert.True(t, groups["openai|h1"][0].Timestamp > groups["openai|h1"][1].Timestamp)
}

func TestSummarize_StreakAndTotals(t *testing.T) {
	now := time.Now()
	records := []Record{
		{Provider: "openai", KeySHA256: "h", Timestamp: now.Add(-3 * time.Hour).Format(time.RFC3339Nano), IsValid: true},
		{Provider: "openai", KeySHA256: "h", Timestamp: now.Add(-2 * time.Hour).Format(time.RFC3339Nano), IsValid: true},
		{Provider: "openai", KeySHA256: "h", Timestamp: now.Add(-1 * time.Hour).Format(time.RFC3339Nano), IsValid: false},
		{Provider: "openai", KeySHA256: "h", Timestamp: now.Format(time.RFC3339Nano), IsValid: true},
	}
	sums := Summarize(records)
	require.Len(t, sums, 1)
	s := sums[0]
	assert.Equal(t, 4, s.TotalRuns)
	assert.Equal(t, 3, s.ValidRuns)
	assert.Equal(t, 1, s.ValidStreak, "the streak should reset on the false record")
}

func TestNewRunID_UniquePerCall(t *testing.T) {
	a := NewRunID()
	b := NewRunID()
	assert.NotEqual(t, a, b)
	assert.NotEmpty(t, a)
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("KUNJI_HISTORY_FILE", "")
	p := DefaultPath()
	assert.Contains(t, p, ".kunji")
	assert.Contains(t, p, "history.jsonl")
}
