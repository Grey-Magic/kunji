package validators

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Grey-Magic/kunji/pkg/client"
	"github.com/Grey-Magic/kunji/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetadataExtraction_Regression(t *testing.T) {
	utils.SkipSSRFCheck = true
	defer func() { utils.SkipSSRFCheck = false }()

	configs, err := LoadProviderConfigs()
	require.NoError(t, err)

	for _, cfg := range configs {
		if cfg.MetadataFromValidation == nil {
			continue
		}

		t.Run(cfg.Name+"_extraction", func(t *testing.T) {

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")

				fmt.Fprint(w, `{"balance": 100.5, "amount": 100.5, "total": 100.5, "email": "test@example.com", "name": "Test User", "user": {"email": "test@example.com", "username": "testuser"}}`)
			}))
			defer server.Close()

			origURL := cfg.Validation.URL
			cfg.Validation.URL = server.URL
			defer func() { cfg.Validation.URL = origURL }()

			v := NewGenericValidatorWithClient(cfg, http.DefaultClient, client.NewRateLimiterManager(100, 100))
			res, err := v.Validate(context.Background(), "test-key")

			require.NoError(t, err)
			assert.True(t, res.IsValid)

			if cfg.MetadataFromValidation.BalancePath == "balance" {
				assert.Equal(t, 100.5, res.Balance)
			}
		})
	}
}
