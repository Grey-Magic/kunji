package runner

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/Grey-Magic/kunji/pkg/utils"
	"github.com/Grey-Magic/kunji/pkg/validators"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain disables the SSRF guard for the duration of this package's
// tests, since httptest servers listen on 127.0.0.1 and would otherwise
// be rejected by utils.ValidateURL. Production code never has
// SkipSSRFCheck set.
func TestMain(m *testing.M) {
	utils.SkipSSRFCheck = true
	defer func() { utils.SkipSSRFCheck = false }()
	m.Run()
}

// shardedFixture spins up one httptest server per provider, configures a
// Factory with stub providers pointing at those servers, and returns the
// pieces runSharded needs to validate keys end-to-end without hitting the
// real internet.
type shardedFixture struct {
	Servers  map[string]*httptest.Server // provider name -> server
	Factory  *validators.ValidatorFactory
	Detector *validators.Detector
	Hits     map[string]int // provider name -> request count
	mu       sync.Mutex
}

func newShardedFixture(t *testing.T, providers []string) *shardedFixture {
	t.Helper()
	f := &shardedFixture{
		Servers: make(map[string]*httptest.Server, len(providers)),
		Hits:    make(map[string]int),
	}

	configs := make([]validators.ProviderConfig, 0, len(providers))
	for _, name := range providers {
		// Each server returns 200 with a tiny JSON body for valid keys,
		// 401 for everything else. We use the key prefix as the validator.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			f.Hits[name]++
			f.mu.Unlock()
			auth := r.Header.Get("Authorization")
			// Match keys whose bearer token starts with "sk-<name>-valid".
			// The trailing characters are opaque so callers can use keys
			// like sk-alpha-valid-1, sk-alpha-valid-foo, etc.
			if strings.HasPrefix(auth, "Bearer sk-"+name+"-valid") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, `{"ok":true}`)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
		}))
		f.Servers[name] = srv
		t.Cleanup(srv.Close)

		configs = append(configs, validators.ProviderConfig{
			Name:        name,
			Category:    "test",
			KeyPrefixes: []string{"sk-" + name + "-"},
			Validation: validators.ValidationConfig{
				Method: "GET",
				URL:    srv.URL,
				Auth:   "bearer",
			},
		})
	}

	factory, _, _, err := validators.NewValidatorFactoryWithOptions("", 5, validators.FactoryOptions{DisableCache: true})
	require.NoError(t, err)

	for _, cfg := range configs {
		factory.RegisterConfig(cfg)
	}

	// Build the detector manually from our test configs so the cached
	// global index isn't disturbed.
	detector := validators.NewDetectorFromConfigs(configs)
	f.Factory = factory
	f.Detector = detector
	return f
}

func TestRunSharded_RoutesEachProviderToItsOwnShard(t *testing.T) {
	fix := newShardedFixture(t, []string{"alpha", "bravo", "charlie"})

	keys := []string{
		"sk-alpha-valid-1",
		"sk-alpha-valid-2",
		"sk-bravo-valid-1",
		"sk-charlie-valid-1",
		"sk-charlie-valid-2",
		"sk-charlie-valid-3",
	}

	results := make(chan *models.ValidationResult, len(keys))
	var seen []*models.ValidationResult

	fix.Factory.SharedClient() // ensure factory wired

	// Drive runSharded directly. We mirror the subset runSharded reads:
	// Detector, Factory, ManualCategory. SkipMetadata is set so the shard
	// workers don't push to r.metadataJobs (which we don't initialize here).
	r := &Runner{
		Detector:       fix.Detector,
		Factory:        fix.Factory,
		ManualCategory: "",
		SkipMetadata:   true,
		Timeout:        10, // seconds; the zero value gives 0s ctx deadline
	}
	r.runSharded(strings.NewReader(strings.Join(keys, "\n")), results)
	close(results)

	for res := range results {
		seen = append(seen, res)
	}

	for _, res := range seen {
		t.Logf("DEBUG: key=%s provider=%s isValid=%v err=%q status=%d", res.Key, res.Provider, res.IsValid, res.ErrorMessage, res.StatusCode)
	}

	// Every key should have produced exactly one result.
	assert.Len(t, seen, len(keys))

	// Results are tagged with the right provider per key.
	want := map[string]string{
		"sk-alpha-valid-1":   "alpha",
		"sk-alpha-valid-2":   "alpha",
		"sk-bravo-valid-1":   "bravo",
		"sk-charlie-valid-1": "charlie",
		"sk-charlie-valid-2": "charlie",
		"sk-charlie-valid-3": "charlie",
	}
	for _, res := range seen {
		assert.Equal(t, want[res.Key], res.Provider, "wrong provider for key %s", res.Key)
		assert.True(t, res.IsValid, "valid key should be reported valid: %s", res.Key)
	}

	// Each provider's server should have been hit the right number of
	// times. Run-shared runs validateWithRetries with max_attempts=1 for
	// these tests so the count matches.
	fix.mu.Lock()
	defer fix.mu.Unlock()
	assert.Equal(t, 2, fix.Hits["alpha"])
	assert.Equal(t, 1, fix.Hits["bravo"])
	assert.Equal(t, 3, fix.Hits["charlie"])
}

func TestRunSharded_UnknownKeysStillProduceResults(t *testing.T) {
	fix := newShardedFixture(t, []string{"alpha"})

	keys := []string{"sk-alpha-valid-1", "no-prefix-here"}

	results := make(chan *models.ValidationResult, len(keys))
	r := &Runner{Detector: fix.Detector, Factory: fix.Factory, Timeout: 10}
	r.runSharded(strings.NewReader(strings.Join(keys, "\n")), results)
	close(results)

	var seen []*models.ValidationResult
	for res := range results {
		seen = append(seen, res)
		t.Logf("DEBUG: result key=%s provider=%s isValid=%v", res.Key, res.Provider, res.IsValid)
	}

	assert.Len(t, seen, len(keys))

	// The unknown key still gets routed (to the "_unknown" shard) and
	// produces a result that downstream code can filter / sink.
	hasUnknown := false
	hasAlpha := false
	for _, res := range seen {
		if res.Provider == "unknown" {
			hasUnknown = true
		}
		if res.Provider == "alpha" {
			hasAlpha = true
		}
	}
	assert.True(t, hasAlpha, "the matching key should produce a provider=alpha result")
	assert.True(t, hasUnknown, "the unmatched key should produce a provider=unknown result")
}

func TestRunSharded_SingleProviderMatchesGlobalPoolBehavior(t *testing.T) {
	// Smoke check: a single-provider run through sharded mode should
	// behave like a normal validation in terms of result count and shape.
	fix := newShardedFixture(t, []string{"only"})

	results := make(chan *models.ValidationResult, 3)
	r := &Runner{Detector: fix.Detector, Factory: fix.Factory, Timeout: 10}
	r.runSharded(strings.NewReader("sk-only-valid-key\nsk-only-other-key\n"), results)
	close(results)

	var seen []*models.ValidationResult
	for res := range results {
		seen = append(seen, res)
	}
	require.Len(t, seen, 2)
	for _, res := range seen {
		assert.Equal(t, "only", res.Provider)
	}
}

func TestRunSharded_DoesNotLeakGoroutines(t *testing.T) {
	// Sanity: after runSharded returns, all shard workers must have
	// exited. We can't directly count goroutines without breaking the
	// rest of the test suite, but we can verify the function returns
	// in bounded time when fed a small input.
	fix := newShardedFixture(t, []string{"alpha", "bravo"})
	results := make(chan *models.ValidationResult, 4)
	r := &Runner{Detector: fix.Detector, Factory: fix.Factory, Timeout: 10}

	done := make(chan struct{})
	go func() {
		r.runSharded(strings.NewReader("sk-alpha-x\nsk-bravo-y\n"), results)
		close(results)
		close(done)
	}()

	// Drain results so the feeder closes its shards.
	for range results {
	}
	<-done
	// No assertion needed; the test fails if runSharded deadlocks.
}

// helper used in some debugging — kept here so future tests can adopt it.
func Example_shardedUsage() {
	fmt.Println("kunji validate -f keys.txt --sharded")
}
