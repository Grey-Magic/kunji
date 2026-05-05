package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectProvider_Advanced(t *testing.T) {
	configs, _ := LoadProviderConfigs()
	detector := NewDetectorFromConfigs(configs)

	tests := []struct {
		name          string
		key           string
		expectedMatch string
	}{
		// Case 1: JWT-based key (Supabase)
		// Header: {"alg":"HS256","typ":"JWT"}
		// Payload: {"role":"anon","iss":"supabase"}
		{"Supabase JWT", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJyb2xlIjoiYW5vbiIsImlzcyI6InN1cGFiYXNlIn0.signature", "supabase"},

		// Case 2: DeepSeek vs OpenAI collision
		// DeepSeek is often pure hex after sk-
		{"DeepSeek Hex", "sk-0123456789abcdef0123456789abcdef", "deepseek"},

		// Case 3: Stripe markers
		{"Stripe Live", "sk_live_abc1234567890abcdef1234567890", "stripe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectProvider(tt.key, "")
			assert.Equal(t, tt.expectedMatch, result, "Key: %s", tt.key)
		})
	}
}
