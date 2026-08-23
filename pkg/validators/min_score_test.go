package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectProvider_MinScoreFiltersOutLowConfidenceMatches(t *testing.T) {
	configs := []ProviderConfig{
		{
			Name:        "noisy",
			Category:    "test",
			KeyPrefixes: []string{"sk-"},
			// Loose pattern that matches almost everything starting with sk-.
			KeyPatterns: []string{"^sk-.+$"},
			Detection:   &DetectionConfig{MinScore: 500},
		},
		{
			Name:        "precise",
			Category:    "test",
			KeyPrefixes: []string{"sk-proj-"},
			KeyPatterns: []string{"^sk-proj-[a-z0-9]{20,}$"},
		},
	}

	d := NewDetectorFromConfigs(configs)

	t.Run("noisy provider is filtered out by min_score", func(t *testing.T) {
		// sk-xyz123 would normally hit the noisy provider with a low score,
		// but with MinScore=500 it must be filtered. Only the precise
		// provider's prefix matches if any.
		res := d.DetectProviderWithSuggestion("sk-xyz123abc456def", "")
		assert.NotEqual(t, "noisy", res.Provider,
			"noisy provider should be filtered out below its min_score")
	})

	t.Run("precise provider still wins for its exact prefix", func(t *testing.T) {
		res := d.DetectProviderWithSuggestion("sk-proj-abcdef0123456789abcdef", "")
		assert.Equal(t, "precise", res.Provider)
		assert.Greater(t, res.Score, 0)
	})
}

func TestDetectProvider_MinScoreZeroBehavesLikeBefore(t *testing.T) {
	configs := []ProviderConfig{
		{
			Name:        "loose",
			Category:    "test",
			KeyPrefixes: []string{"sk-"},
		},
	}
	d := NewDetectorFromConfigs(configs)
	res := d.DetectProviderWithSuggestion("sk-anything", "")
	assert.Equal(t, "loose", res.Provider)
}
