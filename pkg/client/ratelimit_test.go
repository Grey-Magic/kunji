package client

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiterManager_GlobalLimitCapsAggregateRPS(t *testing.T) {
	// 5 rps global ceiling with bucket 5. With per-provider default 100 rps,
	// the global limiter must pace all providers collectively.
	rm := NewRateLimiterManager(100, 100)
	rm.SetGlobalLimit(5)

	const providers = 4
	const total = 20

	var wg = make(chan struct{}, total)
	start := time.Now()
	for i := 0; i < total; i++ {
		prov := []string{"a", "b", "c", "d"}[i%providers]
		go func(p string) {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			require.NoError(t, rm.Wait(ctx, p))
			wg <- struct{}{}
		}(prov)
	}

	done := 0
	timeout := time.After(6 * time.Second)
loop:
	for {
		select {
		case <-wg:
			done++
			if done == total {
				break loop
			}
		case <-timeout:
			goto finish
		}
	}
finish:

	elapsed := time.Since(start)
	require.Equal(t, total, done, "all requests must eventually complete")

	// First 5 should complete essentially immediately (bucket fill).
	// Beyond that, the global limiter paces at 5/s: (total-bucket)/rps seconds.
	minExpected := time.Duration((total-5)/5) * time.Second
	assert.GreaterOrEqual(t, elapsed, minExpected,
		"global limit should pace aggregate RPS, got elapsed=%v expected >= %v", elapsed, minExpected)
}

func TestRateLimiterManager_GlobalLimitZeroDisables(t *testing.T) {
	// rps=0 must disable the global ceiling so per-provider limits apply.
	// We use a generous per-provider rate so the waits complete under 100ms.
	rm := NewRateLimiterManager(1000, 1000)
	rm.SetGlobalLimit(0)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	for i := 0; i < 5; i++ {
		require.NoError(t, rm.Wait(ctx, "any"))
	}
}

func TestRateLimiterManager_RetryAfterExtendsBackoff(t *testing.T) {
	rm := NewRateLimiterManager(50, 50)

	// First warm up the per-provider state.
	rm.Wait(context.Background(), "p1")

	start := time.Now()
	rm.ReportResultWithRetry("p1", 429, 2) // 2s backoff

	// Next Wait must block until ~2s from start (or until backoff passes).
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	require.NoError(t, rm.Wait(ctx, "p1"))

	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 1900*time.Millisecond,
		"Retry-After of 2s should dominate default backoff; elapsed=%v", elapsed)
	assert.Less(t, elapsed, 3500*time.Millisecond, "should not wait significantly longer than advertised")
}

func TestRateLimiterManager_RetryAfterZeroUsesDefault(t *testing.T) {
	rm := NewRateLimiterManager(50, 50)
	rm.Wait(context.Background(), "p2")

	start := time.Now()
	rm.ReportResultWithRetry("p2", 429, 0) // 0 → use default schedule (3s for first 429)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, rm.Wait(ctx, "p2"))

	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 2900*time.Millisecond,
		"default first-429 backoff is 3s when Retry-After is absent")
}

func TestRateLimiterManager_GlobalLimitRespectedPerProvider(t *testing.T) {
	// The per-provider limiter must still apply even with a global ceiling,
	// proving the two layers compose rather than one overriding the other.
	rm := NewRateLimiterManager(2, 1)
	rm.SetGlobalLimit(100)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	// Provider "a" has rate 2/s with burst 1 — so waits 2..4 must each take
	// at least ~500ms. We expect ~1.5s for 4 calls.
	for i := 0; i < 4; i++ {
		require.NoError(t, rm.Wait(ctx, "a"))
	}
	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 1500*time.Millisecond,
		"per-provider limit should still throttle under global cap; elapsed=%v", elapsed)
}
