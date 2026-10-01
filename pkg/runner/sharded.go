package runner

import (
	"bufio"
	"context"
	"io"
	"strings"
	"sync"

	"github.com/Grey-Magic/kunji/pkg/models"
)

// shardedWorkersPerProvider caps concurrent validators per provider. Even
// when the global --threads is high, we keep this modest so a single noisy
// provider cannot saturate its host or its rate-limiter mutex.
const shardedWorkersPerProvider = 4

// runSharded routes keys through per-provider worker pools and feeds results
// into the same downstream channel that runInternal drains. Detection runs
// in the feeder goroutine so workers don't re-detect.
func (r *Runner) runSharded(keyReader io.Reader, results chan<- *models.ValidationResult) {
	type shard struct {
		ch     chan string
		wg     *sync.WaitGroup
		cancel context.CancelFunc
	}

	shards := make(map[string]*shard)
	var shardsMu sync.Mutex

	getShard := func(provider string) *shard {
		shardsMu.Lock()
		defer shardsMu.Unlock()
		if s, ok := shards[provider]; ok {
			return s
		}
		s := &shard{ch: make(chan string, 256)}
		s.wg = &sync.WaitGroup{}
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		for i := 0; i < shardedWorkersPerProvider; i++ {
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				for key := range s.ch {
					r.shardValidateOne(ctx, provider, key, results)
				}
			}()
		}
		shards[provider] = s
		return s
	}

	// Feeder: detect + dedupe + route.
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(keyReader)
	for scanner.Scan() {
		key := strings.TrimSpace(scanner.Text())
		if len(key) < r.MinKeyLength {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if !r.NoCache && r.negativeCache != nil && r.negativeCache.Test(key) {
			continue
		}

		provider := r.ManualProvider
		if provider == "" {
			res := r.Detector.DetectProviderWithSuggestion(key, r.ManualCategory)
			provider = res.Provider
			if provider == "" || provider == "unknown" {
				provider = "_unknown"
			}
		}
		s := getShard(provider)
		s.ch <- key
	}

	// Close every shard so its workers exit.
	shardsMu.Lock()
	for _, s := range shards {
		close(s.ch)
	}
	shardsMu.Unlock()

	// Wait for all shards to drain.
	shardsMu.Lock()
	for _, s := range shards {
		s.wg.Wait()
		s.cancel()
	}
	shardsMu.Unlock()
}

// shardValidateOne runs validation through the shared validator pipeline and
// emits the result through the same channel the non-sharded run uses. When
// metadata enrichment is enabled and the result is valid, the result is
// routed through r.metadataJobs so a metadata worker picks it up (mirrors
// the non-sharded worker() behaviour).
//
// If no validator exists for the provider (e.g. the shard is the catch-all
// "_unknown" shard fed by keys that the detector could not classify), we
// emit a synthetic failure result so downstream sinks / JSONL output see
// the key — same behaviour as worker() emits when len(providersToTry)==0.
func (r *Runner) shardValidateOne(ctx context.Context, provider, key string, results chan<- *models.ValidationResult) {
	val, exists := r.Factory.GetValidator(provider)
	if !exists {
		results <- &models.ValidationResult{
			Key:          key,
			Provider:     "unknown",
			IsValid:      false,
			ErrorMessage: "Could not auto-detect provider. Use -p flag to specify manually.",
		}
		return
	}
	val.SetSkipMetadata(true)
	val.SetCanaryCheck(r.CanaryCheck)

	res := r.validateWithRetries(val, key, provider)
	if res.IsValid && !r.SkipMetadata {
		r.metadataWg.Add(1)
		select {
		case r.metadataJobs <- res:
		default:
			r.metadataWg.Done()
			results <- res
		}
		return
	}
	results <- res
}
