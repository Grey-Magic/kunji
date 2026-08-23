package runner

import (
	"testing"
	"time"

	"github.com/Grey-Magic/kunji/pkg/validators"
	"github.com/stretchr/testify/assert"
)

func TestRetryBudget_DefaultsWhenNoProviderPolicy(t *testing.T) {
	r := &Runner{Retries: 3}
	cfg := validators.ProviderConfig{Name: "x"}

	// Build a GenericValidator purely to exercise retryBudget.
	v := validators.NewGenericValidatorWithClient(cfg, nil, nil)
	b := r.retryBudget(v)

	assert.Equal(t, 4, b.maxAttempts, "max attempts defaults to global Retries+1")
	assert.True(t, b.onStatus[429])
	assert.True(t, b.onStatus[500])
	assert.True(t, b.onStatus[503])
	assert.False(t, b.onStatus[200])
	assert.Equal(t, 1*time.Second, b.initialBackoff)
	assert.Equal(t, 8*time.Second, b.maxBackoff)
}

func TestRetryBudget_ProviderOverridesAllFields(t *testing.T) {
	r := &Runner{Retries: 3}
	cfg := validators.ProviderConfig{
		Name: "x",
		RetryPolicy: &validators.RetryPolicy{
			MaxAttempts:      5,
			OnStatus:         []int{429, 502},
			InitialBackoffMs: 250,
			MaxBackoffMs:     3000,
		},
	}
	v := validators.NewGenericValidatorWithClient(cfg, nil, nil)
	b := r.retryBudget(v)

	assert.Equal(t, 5, b.maxAttempts)
	assert.True(t, b.onStatus[429])
	assert.True(t, b.onStatus[502])
	assert.False(t, b.onStatus[500], "503 was not in the provider's on_status list")
	assert.Equal(t, 250*time.Millisecond, b.initialBackoff)
	assert.Equal(t, 3000*time.Millisecond, b.maxBackoff)
}

func TestRetryBudget_PartialOverrideKeepsDefaults(t *testing.T) {
	r := &Runner{Retries: 3}
	cfg := validators.ProviderConfig{
		Name:        "x",
		RetryPolicy: &validators.RetryPolicy{MaxAttempts: 1},
	}
	v := validators.NewGenericValidatorWithClient(cfg, nil, nil)
	b := r.retryBudget(v)

	assert.Equal(t, 1, b.maxAttempts)
	// on_status / backoff fall back to defaults.
	assert.True(t, b.onStatus[429])
	assert.Equal(t, 1*time.Second, b.initialBackoff)
}

func TestRetryBudget_InvalidMaxAttemptsFallsBackToGlobal(t *testing.T) {
	r := &Runner{Retries: 2}
	cfg := validators.ProviderConfig{
		Name:        "x",
		RetryPolicy: &validators.RetryPolicy{MaxAttempts: 0}, // invalid, ignored
	}
	v := validators.NewGenericValidatorWithClient(cfg, nil, nil)
	b := r.retryBudget(v)
	assert.Equal(t, 3, b.maxAttempts)
}

func TestBackoffDuration_DoublesUpToCap(t *testing.T) {
	b := retryBudget{
		initialBackoff: 100 * time.Millisecond,
		maxBackoff:     500 * time.Millisecond,
	}

	assert.Equal(t, 100*time.Millisecond, b.backoffDuration(0, 0))
	assert.Equal(t, 200*time.Millisecond, b.backoffDuration(1, 0))
	assert.Equal(t, 400*time.Millisecond, b.backoffDuration(2, 0))
	assert.Equal(t, 500*time.Millisecond, b.backoffDuration(3, 0), "should cap at maxBackoff")
	assert.Equal(t, 500*time.Millisecond, b.backoffDuration(10, 0), "very late attempts stay capped")
}

func TestBackoffDuration_RetryAfterOverridesSchedule(t *testing.T) {
	// Retry-After replaces the schedule-derived duration, but is still
	// capped at maxBackoff (see TestBackoffDuration_RetryAfterCappedAtMaxBackoff
	// for the cap boundary).
	b := retryBudget{
		initialBackoff: 100 * time.Millisecond,
		maxBackoff:     2 * time.Second,
	}

	// RetryAfter=1s is below maxBackoff → uses the RetryAfter value.
	assert.Equal(t, 1*time.Second, b.backoffDuration(0, 1),
		"small Retry-After replaces the schedule")

	// RetryAfter=2s exactly matches maxBackoff → 2s.
	assert.Equal(t, 2*time.Second, b.backoffDuration(0, 2),
		"Retry-After equal to maxBackoff stays at maxBackoff")
}

func TestBackoffDuration_RetryAfterCappedAtMaxBackoff(t *testing.T) {
	b := retryBudget{
		initialBackoff: 100 * time.Millisecond,
		maxBackoff:     2 * time.Second,
	}

	assert.Equal(t, 2*time.Second, b.backoffDuration(0, 600),
		"absurdly large Retry-After is capped at maxBackoff")
}

func TestDefaultRetryOnStatus_CoversAll5xx(t *testing.T) {
	m := defaultRetryOnStatus()
	for c := 500; c < 600; c++ {
		assert.True(t, m[c], "5xx code %d should be in default retry set", c)
	}
	assert.True(t, m[429])
	assert.False(t, m[200])
	assert.False(t, m[404])
}
