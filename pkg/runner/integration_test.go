package runner

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Grey-Magic/kunji/pkg/validators"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_OpenAI_ValidKey(t *testing.T) {
	key := os.Getenv("KUNJI_TEST_OPENAI_KEY")
	if key == "" {
		t.Skip("Set KUNJI_TEST_OPENAI_KEY to run integration test")
	}

	v, _, _, err := validators.InitValidatorsWithConfigs("", 15)
	require.NoError(t, err)

	val, exists := v["openai"]
	require.True(t, exists, "openai validator should exist")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := val.Validate(ctx, key)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.IsValid, "OpenAI key should be valid")
	assert.Equal(t, "openai", result.Provider)
	assert.Greater(t, result.ResponseTime, 0.0)
}

func TestIntegration_OpenAI_InvalidKey(t *testing.T) {
	key := "sk-test-invalid-key-12345"

	v, _, _, err := validators.InitValidatorsWithConfigs("", 15)
	require.NoError(t, err)

	val, exists := v["openai"]
	require.True(t, exists)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := val.Validate(ctx, key)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.False(t, result.IsValid, "Invalid OpenAI key should not be valid")
}

func TestIntegration_GitHub_ValidKey(t *testing.T) {
	key := os.Getenv("KUNJI_TEST_GITHUB_KEY")
	if key == "" {
		t.Skip("Set KUNJI_TEST_GITHUB_KEY to run integration test")
	}

	v, _, _, err := validators.InitValidatorsWithConfigs("", 15)
	require.NoError(t, err)

	val, exists := v["github"]
	require.True(t, exists)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := val.Validate(ctx, key)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.IsValid, "GitHub key should be valid")
}

func TestIntegration_Stripe_ValidKey(t *testing.T) {
	key := os.Getenv("KUNJI_TEST_STRIPE_KEY")
	if key == "" {
		t.Skip("Set KUNJI_TEST_STRIPE_KEY to run integration test")
	}

	v, _, _, err := validators.InitValidatorsWithConfigs("", 15)
	require.NoError(t, err)

	val, exists := v["stripe"]
	require.True(t, exists)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := val.Validate(ctx, key)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.IsValid, "Stripe key should be valid")
}

func TestIntegration_Anthropic_ValidKey(t *testing.T) {
	key := os.Getenv("KUNJI_TEST_ANTHROPIC_KEY")
	if key == "" {
		t.Skip("Set KUNJI_TEST_ANTHROPIC_KEY to run integration test")
	}

	v, _, _, err := validators.InitValidatorsWithConfigs("", 15)
	require.NoError(t, err)

	val, exists := v["anthropic"]
	require.True(t, exists)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := val.Validate(ctx, key)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.IsValid, "Anthropic key should be valid")
}

func TestIntegration_ProviderDetection(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{"OpenAI sk-", "sk-abcdefghijklmnopqrstuvwxyz1234567890abcdefghijkl", "openai"},
		{"OpenAI sk1-", "sk1-abcdefghijklmnopqrstuvwxyz1234567890abcdefghijkl", "openai"},
		{"Anthropic sk-ant-", "sk-ant-api03-abcdefghijklmnopqrstuvwxyz1234567890", "anthropic"},
		{"Groq gsk_", "gsk_abcdefghijklmnopqrstuvwxyz1234567890abcdefghij", "groq"},
		{"GitHub ghp_", "ghp_123456789012345678901234567890123456", "github"},
		{"Stripe sk_live_", "sk_live_abcdefghijklmnopqrstuvwxyz1234567890", "stripe"},
		{"Slack xoxb-", "xoxb-12345678901-12345678901-abcdefghijklmnopqrstuvwx", "slack"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := validators.NewDetector()
			result := detector.DetectProvider(tt.key, "")
			assert.Equal(t, tt.expected, result, "Provider should be detected correctly")
		})
	}
}

func TestIntegration_CompositeKey_Twilio(t *testing.T) {
	accountSid := os.Getenv("KUNJI_TEST_TWILIO_SID")
	authToken := os.Getenv("KUNJI_TEST_TWILIO_TOKEN")

	if accountSid == "" || authToken == "" {
		t.Skip("Set KUNJI_TEST_TWILIO_SID and KUNJI_TEST_TWILIO_TOKEN to run integration test")
	}

	key := accountSid + ":" + authToken

	v, _, _, err := validators.InitValidatorsWithConfigs("", 15)
	require.NoError(t, err)

	val, exists := v["twilio"]
	require.True(t, exists)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := val.Validate(ctx, key)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.IsValid, "Twilio composite key should be valid")
}

func TestIntegration_LoadProviderConfigs(t *testing.T) {
	configs, err := validators.LoadProviderConfigs()
	require.NoError(t, err)
	assert.Greater(t, len(configs), 100, "Should have 100+ provider configs")
}

func TestIntegration_ListProviders(t *testing.T) {
	providers, err := validators.GetAllProviders()
	require.NoError(t, err)
	assert.Greater(t, len(providers), 100, "Should have 100+ providers")
}

func TestIntegration_Categories(t *testing.T) {
	categories, err := validators.GetCategories()
	require.NoError(t, err)
	assert.Greater(t, len(categories), 5, "Should have multiple categories")

	found := false
	for _, cat := range categories {
		if cat == "llm" {
			found = true
			break
		}
	}
	assert.True(t, found, "Should have 'llm' category")
}

func TestIntegration_GetProvidersByCategory(t *testing.T) {
	llmProviders, err := validators.GetProvidersByCategory("llm")
	require.NoError(t, err)
	assert.Greater(t, len(llmProviders), 10, "Should have 10+ LLM providers")
}
