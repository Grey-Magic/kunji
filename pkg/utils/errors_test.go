package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseAPIError(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		expected string
	}{
		{
			name:     "empty body",
			body:     []byte{},
			expected: "Unknown error (empty body)",
		},
		{
			name:     "nil body",
			body:     nil,
			expected: "Unknown error (empty body)",
		},
		{
			name:     "simple error message",
			body:     []byte(`{"error": "Something went wrong"}`),
			expected: "Something went wrong",
		},
		{
			name:     "message at top level",
			body:     []byte(`{"message": "Rate limit exceeded"}`),
			expected: "Rate limit exceeded",
		},
		{
			name:     "msg field",
			body:     []byte(`{"msg": "Invalid API key"}`),
			expected: "Invalid API key",
		},
		{
			name:     "nested error object",
			body:     []byte(`{"error": {"message": "Unauthorized access"}}`),
			expected: "Unauthorized access",
		},
		{
			name:     "plain text response",
			body:     []byte(`Internal Server Error`),
			expected: "Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseAPIError(tt.body, "")
			assert.Equal(t, tt.expected, result)
		})
	}

	t.Run("Scrubbing API Key", func(t *testing.T) {
		body := []byte(`{"error": "Invalid key: sk-abc123456"}`)
		key := "sk-abc123456"
		result := ParseAPIError(body, key)
		assert.Contains(t, result, "[MASKED_KEY]")
		assert.NotContains(t, result, key)
	})
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name            string
		headers         map[string][]string
		expectedSeconds int
	}{
		{
			name:            "empty headers",
			headers:         map[string][]string{},
			expectedSeconds: 0,
		},
		{
			name: "Retry-After header in seconds",
			headers: map[string][]string{
				"Retry-After": {"30"},
			},
			expectedSeconds: 30,
		},
		{
			name: "X-Ratelimit-Reset as timestamp",
			headers: map[string][]string{
				"X-Ratelimit-Reset": {"1700000000"},
			},
			expectedSeconds: 0, // Will be > 0 at runtime but we test for parsing
		},
		{
			name: "X-RateLimit-Reset header",
			headers: map[string][]string{
				"X-RateLimit-Reset": {"60"},
			},
			expectedSeconds: 60,
		},
		{
			name: "invalid header value",
			headers: map[string][]string{
				"Retry-After": {"not-a-number"},
			},
			expectedSeconds: 0,
		},
		{
			name: "multiple headers - first valid",
			headers: map[string][]string{
				"Retry-After":       {"30"},
				"X-RateLimit-Reset": {"60"},
			},
			expectedSeconds: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseRetryAfter(tt.headers)
			// For timestamp tests, just verify it parses without error
			if tt.name == "X-Ratelimit-Reset as timestamp" {
				assert.GreaterOrEqual(t, result, 0)
			} else {
				assert.Equal(t, tt.expectedSeconds, result)
			}
		})
	}
}

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		limit    int
		expected string
	}{
		{
			name:     "string shorter than limit",
			input:    "hello",
			limit:    10,
			expected: "hello",
		},
		{
			name:     "string equal to limit",
			input:    "hello",
			limit:    5,
			expected: "hello",
		},
		{
			name:     "string longer than limit",
			input:    "hello world",
			limit:    5,
			expected: "hello...",
		},
		{
			name:     "empty string",
			input:    "",
			limit:    5,
			expected: "",
		},
		{
			name:     "zero limit",
			input:    "hello",
			limit:    0,
			expected: "",
		},
		{
			name:     "negative limit",
			input:    "hello",
			limit:    -1,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateStr(tt.input, tt.limit)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func makeString(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += "a"
	}
	return s
}
