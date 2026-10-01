package validators

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Grey-Magic/kunji/pkg/client"
)

type ValidatorFactory struct {
	configs       map[string]ProviderConfig
	sharedClient  *http.Client
	sharedLimiter *client.RateLimiterManager
	sharedCache   client.ResultCache
	validators    map[string]Validator
	mux           sync.RWMutex
}

// FactoryOptions configures the optional layers attached to a ValidatorFactory.
type FactoryOptions struct {
	// PersistentCachePath enables the cross-run positive cache when non-empty.
	// When empty, the factory uses only the in-memory ValidationCache.
	PersistentCachePath string
	// PersistentCacheTTL is the freshness window for persistent entries.
	// Zero means use the default (5 minutes).
	PersistentCacheTTL time.Duration
	// DisableCache disables both layers; validators will always hit the
	// network. Used when the user passes --no-cache.
	DisableCache bool
}

func NewValidatorFactory(proxy string, timeout int) (*ValidatorFactory, []ProviderConfig, *client.ProxyRotator, error) {
	return NewValidatorFactoryWithOptions(proxy, timeout, FactoryOptions{})
}

// NewValidatorFactoryWithOptions is the configurable constructor. opts.PersistentCachePath
// enables a cross-run positive cache; opts.DisableCache forces every request
// through the network regardless of either cache layer.
func NewValidatorFactoryWithOptions(proxy string, timeout int, opts FactoryOptions) (*ValidatorFactory, []ProviderConfig, *client.ProxyRotator, error) {
	configs, err := LoadProviderConfigs()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading provider configs: %w", err)
	}

	sharedClient, rotator, err := client.NewHTTPClient(proxy, timeout)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating shared HTTP client: %w", err)
	}

	sharedLimiter := client.NewRateLimiterManager(50, 50)

	var sharedCache client.ResultCache
	switch {
	case opts.DisableCache:
		sharedCache = nil
	case opts.PersistentCachePath != "":
		ttl := opts.PersistentCacheTTL
		if ttl <= 0 {
			ttl = 5 * time.Minute
		}
		disk, err := client.NewPersistentCache(opts.PersistentCachePath, ttl)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("creating persistent cache: %w", err)
		}
		mem := client.NewValidationCache(5*time.Minute, 50000)
		sharedCache = client.NewLayeredCache(mem, disk)
	default:
		sharedCache = client.NewValidationCache(5*time.Minute, 50000)
	}

	configMap := make(map[string]ProviderConfig)
	for _, cfg := range configs {
		configMap[cfg.Name] = cfg
	}

	return &ValidatorFactory{
		configs:       configMap,
		sharedClient:  sharedClient,
		sharedLimiter: sharedLimiter,
		sharedCache:   sharedCache,
		validators:    make(map[string]Validator),
	}, configs, rotator, nil
}

func (f *ValidatorFactory) GetValidator(name string) (Validator, bool) {
	f.mux.RLock()
	if v, ok := f.validators[name]; ok {
		f.mux.RUnlock()
		return v, true
	}
	f.mux.RUnlock()

	f.mux.Lock()
	defer f.mux.Unlock()

	if v, ok := f.validators[name]; ok {
		return v, true
	}

	cfg, ok := f.configs[name]
	if !ok {
		return nil, false
	}

	v := NewGenericValidatorWithClient(cfg, f.sharedClient, f.sharedLimiter)
	if f.sharedCache != nil {
		v.SetCache(f.sharedCache)
	}
	f.validators[name] = v
	return v, true
}

// RegisterConfig adds (or replaces) a provider config in the factory. Used
// by tests to inject stub providers without mutating the global cached
// configs. Production code should rely on LoadProviderConfigs and the
// --templates flag instead.
func (f *ValidatorFactory) RegisterConfig(cfg ProviderConfig) {
	f.mux.Lock()
	defer f.mux.Unlock()
	f.configs[cfg.Name] = cfg
	delete(f.validators, cfg.Name)
}

func (f *ValidatorFactory) Cache() client.ResultCache {
	return f.sharedCache
}

func (f *ValidatorFactory) SharedClient() *http.Client {
	return f.sharedClient
}

// SharedLimiter exposes the underlying RateLimiterManager so callers can
// install a global rate ceiling via SetGlobalLimit.
func (f *ValidatorFactory) SharedLimiter() *client.RateLimiterManager {
	return f.sharedLimiter
}

func InitValidatorsWithConfigs(proxy string, timeout int) (map[string]Validator, []ProviderConfig, *client.ProxyRotator, error) {
	factory, configs, rotator, err := NewValidatorFactory(proxy, timeout)
	if err != nil {
		return nil, nil, nil, err
	}

	validatorsMap := make(map[string]Validator)
	for _, cfg := range configs {
		v, _ := factory.GetValidator(cfg.Name)
		validatorsMap[cfg.Name] = v
	}

	return validatorsMap, configs, rotator, nil
}
