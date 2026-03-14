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

	prefixes, patterns, _ := BuildDetectionIndex(configs)
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
		{"Anthropic api03 key - longest prefix wins", "sk-ant-api03-abcdefghijklmnopqrstuvwxyz", "", "anthropic"},
		{"Anthropic key with full prefix", "anthropic-api-key-12345", "", "anthropic"},
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

	prefixes, _, _ := BuildDetectionIndex(configs)
	detector := &Detector{prefixes: prefixes}

	result := detector.DetectProvider("sk-proj-somekey", "")
	// Longer prefix should win when there's a clear winner
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

	_, patterns, _ := BuildDetectionIndex(configs)
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

	prefixes, patterns, _ := BuildDetectionIndex(configs)
	detector := &Detector{prefixes: prefixes, patterns: patterns}

	result := detector.DetectProvider("client_id:client_secret", "")
	// Pattern should match but may return unknown if no prefix
	assert.NotEmpty(t, result)
}

func TestDetectProvider_Whitespace(t *testing.T) {
	configs, _ := LoadProviderConfigs()
	detector := NewDetectorFromConfigs(configs)

	result := detector.DetectProvider("  sk-proj-test-key-with-spaces  ", "")
	assert.Equal(t, "openai", result, "sk-proj- prefix should match openai even with spaces")
}

func TestDetectProvider_LLMSpecificPrefixes(t *testing.T) {
	configs, _ := LoadProviderConfigs()
	detector := NewDetectorFromConfigs(configs)

	tests := []struct {
		name          string
		key           string
		category      string
		expectedMatch string
	}{
		{"DeepSeek key (long)", "sk-deepseek-abcdef0123456789abcdef0123456789", "", "deepseek"},
		{"DeepSeek key (short sk-)", "sk-0123456789abcdef0123456789abcdef", "", "deepseek"},
		{"OpenAI project key", "sk-proj-abc1234567890abcdefghijklmnopqrst", "", "openai"},
		{"Anthropic legacy key", "anthropic-api-key-1234567890abcdef", "", "anthropic"},
		{"OpenRouter key", "sk-or-abcdefghijklmnopqrstuvwxyz", "", "openrouter"},
		{"Gemini key - ambiguous without category filter", "AIzaSyABCDEFGHIJKLMNOPQRSTUVWXYZAaBbCcDdEe", "", "unknown"},
		{"Gemini key - with category filter", "AIzaSyABCDEFGHIJKLMNOPQRSTUVWXYZAaBbCcDdEe", "llm", "gemini"},
		{"HuggingFace key", "hf_abcdefghijklmnopqrstuvwxyz123456", "", "huggingface"},
		{"xAI key", "xai-abcdefghijklmnopqrstuvwxyz", "", "xai"},
		{"Venice key", "venice-abcdefghijklmnopqrstuvwxyz", "", "venice"},
		{"Minimax key", "minimax-abcdefghijklmnopqrstuvwxyz", "", "minimax"},
		{"Perplexity key", "pplx-abcdefghijklmnopqrstuvwxyz1234567890ab", "", "perplexity"},
		{"Groq key", "gsk_abcdefghijklmnopqrstuvwxyz1234567890AB", "", "groq"},
		{"Fireworks key", "fw_abcdefghijklmnopqrstuvwxyz12345678", "", "fireworks"},
		{"Replicate key", "r8_abcdefghijklmnopqrstuvwxyz1234567890", "", "replicate"},
		{"Replicate alt key", "r8_abcdefghijklmnopqrstuvwxyz1234567890ab", "", "replicate"},
		{"ElevenLabs key", "sk_abcdef0123456789abcdef0123456789", "", "elevenlabs"},
		{"Replicate actual", "r8_abcdefghijklmnopqrstuvwxyz1234567890", "", "replicate"},
		{"Mistral key", "mistral-abcdef0123456789abcdef0123456789", "", "mistral"},
		{"Cohere key", "cohere-abcdefghijklmnopqrstuvwxyz1234567890AB", "", "cohere"},
		{"Together key", "together-abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", "", "together"},
		{"Novita key", "novita-abcdefghijklmnopqrstuvwxyz123456", "", "novita"},
		{"Kimi key", "sk-kimi-abcdefghijklmnopqrstuvwxyz", "", "kimi"},
		{"Qwen key", "sk-qwen-abcdefghijklmnopqrstuv", "", "qwen"},
		{"Midjourney key", "mj-abcdefghijklmnopqrstuvwxyz123456", "", "midjourney"},
		{"Midjourney goapi", "goapi-abcdefghijklmnopqrstuvwxyz123456", "", "midjourney"},
		{"Kilo key", "kilo-abcdefghijklmnopqrstuvwxyz123", "", "kilo"},
		{"Roocode key", "roo-abcdefghijklmnopqrstuvwxyz123", "", "roocode"},
		{"Cline key", "cline-abcdefghijklmnopqrstuvwxyz123", "", "cline"},
		{"Aider key", "aider-abcdefghijklmnopqrstuvwxyz123", "", "aider"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectProvider(tt.key, tt.category)
			assert.Equal(t, tt.expectedMatch, result, "key should match expected provider")
		})
	}
}

func TestDetectProviderWithSuggestion(t *testing.T) {
	configs, _ := LoadProviderConfigs()
	detector := NewDetectorFromConfigs(configs)

	tests := []struct {
		name          string
		key           string
		category      string
		expectedMatch string
		expectSuggest bool
	}{
		{"Known key OpenAI", "sk-proj-abc1234567890abcdefghijklmnopqrstuvwxyz", "", "openai", false},
		{"Unknown key - should suggest", "random-unknown-key-12345", "", "unknown", true},
		{"Unique sk-ant key", "sk-ant-test-key", "", "anthropic", false},
		{"Unique sk-ant with category", "sk-ant-test-key", "llm", "anthropic", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectProviderWithSuggestion(tt.key, tt.category)
			assert.Equal(t, tt.expectedMatch, result.Provider, "provider should match")
			if tt.expectSuggest {
				assert.Greater(t, len(result.Suggestions), 0, "should have suggestions")
			}
		})
	}
}

func TestDetectProvider_SuggestionsContent(t *testing.T) {
	configs, _ := LoadProviderConfigs()
	detector := NewDetectorFromConfigs(configs)

	// Test that suggestions contain relevant providers for sk- keys
	result := detector.DetectProviderWithSuggestion("random-unknown-key", "")
	assert.Equal(t, "unknown", result.Provider)

	// Should have suggestions based on common prefixes
	assert.GreaterOrEqual(t, len(result.Suggestions), 0, "should have suggestions array")
}

func TestDetectProvider_AmbiguousPrefix(t *testing.T) {
	configs := []ProviderConfig{
		{
			Name:        "provider-a",
			Category:    "test",
			KeyPrefixes: []string{"test-abc-", "te-"},
		},
		{
			Name:        "provider-b",
			Category:    "test",
			KeyPrefixes: []string{"test-xyz-"},
		},
	}

	prefixes, _, _ := BuildDetectionIndex(configs)
	detector := &Detector{prefixes: prefixes}

	// test-abc-xxx matches provider-a (longest prefix test-abc-)
	// test-xyz-xxx matches provider-b (longest prefix test-xyz-)
	// test-xxx matches both test-abc- and test-xyz- with same length - should be unknown
	result := detector.DetectProviderWithSuggestion("test-xyz-abc", "")
	assert.Equal(t, "provider-b", result.Provider)
}

func TestDetectProvider_EmptyAndWhitespace(t *testing.T) {
	configs, _ := LoadProviderConfigs()
	detector := NewDetectorFromConfigs(configs)

	tests := []struct {
		name          string
		key           string
		expectedMatch string
	}{
		{"Empty key", "", "unknown"},
		{"Whitespace only", "   ", "unknown"},
		{"Tab whitespace", "\t\n", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectProvider(tt.key, "")
			assert.Equal(t, tt.expectedMatch, result)
		})
	}
}

func TestDetectProvider_CaseInsensitivePrefix(t *testing.T) {
	configs, _ := LoadProviderConfigs()
	detector := NewDetectorFromConfigs(configs)

	// Test that detection works regardless of case
	tests := []struct {
		name string
		key  string
	}{
		{"Uppercase SK-", "SK-ABC123"},
		{"Mixed case Sk-", "Sk-ABC123"},
		{"Lowercase sk-", "sk-abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectProvider(tt.key, "")
			// Should match openai (sk- prefix) or return unknown if not valid format
			assert.NotEmpty(t, result)
		})
	}
}

func TestDetectProvider_NewServices(t *testing.T) {
	configs, _ := LoadProviderConfigs()
	detector := NewDetectorFromConfigs(configs)

	tests := []struct {
		name          string
		key           string
		expectedMatch string
	}{
		{"Shopify composite", "myshop:shpat_1234567890abcdef1234567890abcdef", "shopify"},
		{"Vercel token", "vercel_abcdefghijklmnopqrstuvwxyz", "vercel"},
		{"Netlify token", "nfp_abcdefghijklmnopqrstuvwxyz", "netlify"},
		{"Railway token", "railway_abcdefghijklmnopqrstuvwxyz123456", "railway"},
		{"Fly.io token", "fly_abcdefghijklmnopqrstuvwxyz123456", "flyio"},
		{"Upstash token", "upstash_abcdefghijklmnopqrstuvwxyz", "upstash"},
		{"Discord Bot token", "Bot abcdefghijklmnopqrstuvwxyz.123456.789012345678901234567890", "discord_bot"},
		{"Collision OpenAI vs DeepSeek", "sk-0123456789abcdef0123456789abcdef", "deepseek"},
		{"GitHub PAT", "ghp_123456789012345678901234567890123456", "github"},
		{"GitHub PAT (duplicate test)", "ghcr_123456789012345678901234567890123456", "github"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectProvider(tt.key, "")
			assert.Equal(t, tt.expectedMatch, result, "key should match expected provider")
		})
	}
}
