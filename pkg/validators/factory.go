package validators

import (
	"fmt"

	"github.com/Grey-Magic/kunji/pkg/client"
)

func InitValidatorsWithConfigs(proxy string, timeout int) (map[string]Validator, []ProviderConfig, error) {
	configs, err := LoadProviderConfigs()
	if err != nil {
		return nil, nil, fmt.Errorf("loading provider configs: %w", err)
	}

	sharedClient, err := client.NewHTTPClient(proxy, timeout)
	if err != nil {
		return nil, nil, fmt.Errorf("creating shared HTTP client: %w", err)
	}

	sharedLimiter := client.NewRateLimiterManager(10, 10)

	validatorsMap := make(map[string]Validator, len(configs))

	for _, cfg := range configs {
		validatorsMap[cfg.Name] = NewGenericValidatorWithClient(cfg, sharedClient, sharedLimiter)
	}

	return validatorsMap, configs, nil
}

func InitValidators(proxy string, timeout int) (map[string]Validator, error) {
	v, _, err := InitValidatorsWithConfigs(proxy, timeout)
	return v, err
}
