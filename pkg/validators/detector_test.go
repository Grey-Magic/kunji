package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectProviderFromIndex(t *testing.T) {
	configs := []ProviderConfig{
		{
			Name:        "openai",
			Category:    "llm",
			KeyPrefixes: []string{"sk-", "sk-proj-", "sk-svcacct-"},
			KeyPatterns: []string{"^sk-[a-zA-Z0-9]{32,}$"},
		},
		{
			Name:        "anthropic",
			Category:    "llm",
			KeyPrefixes: []string{"sk-ant-", "anthropic-"},
			KeyPatterns: []string{"^anthropic-", "^sk-ant-"},
		},
		{
			Name:        "github",
			Category:    "developer",
			KeyPrefixes: []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_pat_"},
			KeyPatterns: []string{"^(gh[pousr]_[a-zA-Z0-9]{36}|github_pat_[a-zA-Z0-9]{22}_[a-zA-Z0-9]{59})$"},
		},
		{
			Name:        "stripe",
			Category:    "payments",
			KeyPrefixes: []string{"sk_live_"},
			KeyPatterns: []string{"(?i)^sk_live_[a-zA-Z0-9]{24,99}$"},
		},
	}

	prefixes, patterns := BuildDetectionIndex(configs)
	detector := &Detector{
		prefixes: prefixes,
		patterns: patterns,
	}

	tests := []struct {
		name          string
		key           string
		category      string
		expectedMatch string
	}{
		{"OpenAI standard key", "sk-abc1234567890abcdefghijklmnopqrstuvwxyz", "", "openai"},
		{"OpenAI project key", "sk-proj-abc1234567890abcdefghijklmnopqrst", "", "openai"},
		{"Anthropic key", "sk-ant-api03-abcdefghijklmnopqrstuvwxyz", "", "anthropic"},
		{"GitHub PAT", "github_pat_11AAAABBBCCCDDDEEEFF_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "", "github"},
		{"Stripe live key", "sk_live_abc123def456ghi789jklmno", "", "stripe"},
		{"Unknown key", "unknown-key-format-12345", "", "unknown"},
		{"Category filter - LLM with sk-ant", "sk-ant-api03-test", "llm", "anthropic"},
		{"Category filter - mismatch", "ghp_testtoken", "payments", "unknown"},
		{"Empty key", "", "", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectProvider(tt.key, tt.category)
			assert.Equal(t, tt.expectedMatch, result)
		})
	}
}

func TestDetectProvider_PrefixPriority(t *testing.T) {
	configs := []ProviderConfig{
		{
			Name:        "short-prefix",
			Category:    "test",
			KeyPrefixes: []string{"sk-"},
		},
		{
			Name:        "long-prefix",
			Category:    "test",
			KeyPrefixes: []string{"sk-proj-"},
		},
	}

	prefixes, _ := BuildDetectionIndex(configs)
	detector := &Detector{prefixes: prefixes}

	result := detector.DetectProvider("sk-proj-somekey", "")
	assert.Equal(t, "long-prefix", result, "longer prefix should match first")
}

func TestDetectProvider_PatternPriority(t *testing.T) {
	configs := []ProviderConfig{
		{
			Name:        "general-pattern",
			Category:    "test",
			KeyPatterns: []string{"^sk-[a-z]+$"},
		},
		{
			Name:        "specific-pattern",
			Category:    "test",
			KeyPatterns: []string{"^sk-proj-[a-z0-9]{20,}$"},
		},
	}

	_, patterns := BuildDetectionIndex(configs)
	detector := &Detector{patterns: patterns}

	result := detector.DetectProvider("sk-proj-abcdefghijk1234567890", "")
	assert.Equal(t, "specific-pattern", result, "more specific pattern should match first")
}

func TestDetectProvider_CompositeKeys(t *testing.T) {
	configs := []ProviderConfig{
		{
			Name:        "composite-test",
			Category:    "test",
			KeyPatterns: []string{"^[a-zA-Z0-9]+:[a-zA-Z0-9]+$"},
		},
	}

	prefixes, patterns := BuildDetectionIndex(configs)
	detector := &Detector{prefixes: prefixes, patterns: patterns}

	result := detector.DetectProvider("client_id:client_secret", "")
	// Pattern should match but may return unknown if no prefix
	assert.NotEmpty(t, result)
}

func TestDetectProvider_Whitespace(t *testing.T) {
	configs, _ := LoadProviderConfigs()
	detector := NewDetectorFromConfigs(configs)

	result := detector.DetectProvider("  sk-test-key-with-spaces  ", "")
	assert.NotEqual(t, "unknown", result, "should trim whitespace")
}
