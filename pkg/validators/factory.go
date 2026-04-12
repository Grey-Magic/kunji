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
	sharedCache   *client.ValidationCache
	validators    map[string]Validator
	mux           sync.RWMutex
}

func NewValidatorFactory(proxy string, timeout int) (*ValidatorFactory, []ProviderConfig, *client.ProxyRotator, error) {
	configs, err := LoadProviderConfigs()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading provider configs: %w", err)
	}

	sharedClient, rotator, err := client.NewHTTPClient(proxy, timeout)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating shared HTTP client: %w", err)
	}

	sharedLimiter := client.NewRateLimiterManager(10, 10)
	sharedCache := client.NewValidationCache(5*time.Minute, 50000)

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

func (f *ValidatorFactory) Cache() *client.ValidationCache {
	return f.sharedCache
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
