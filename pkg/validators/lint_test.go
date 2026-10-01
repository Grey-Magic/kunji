package validators

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLintProvider_MissingFields(t *testing.T) {
	cfg := ProviderConfig{}
	issues := LintProvider(cfg)

	severity := func(field string) (string, bool) {
		for _, i := range issues {
			if i.Field == field {
				return string(i.Severity), true
			}
		}
		return "", false
	}

	s, ok := severity("name")
	assert.True(t, ok)
	assert.Equal(t, "error", s)

	_, hasPrefixesOrPatterns := severity("key_patterns")
	assert.False(t, hasPrefixesOrPatterns,
		"missing both prefix and pattern should be a top-level error, not a field error")
}

func TestLintProvider_InvalidRegex(t *testing.T) {
	cfg := ProviderConfig{
		Name:        "bad",
		KeyPrefixes: []string{"x-"},
		KeyPatterns: []string{"("},
	}
	issues := LintProvider(cfg)
	require.NotEmpty(t, issues)

	found := false
	for _, i := range issues {
		if i.Field == "key_patterns[0]" && i.Severity == SeverityError {
			found = true
		}
	}
	assert.True(t, found)
}

func TestLintProvider_UnknownAuth(t *testing.T) {
	cfg := ProviderConfig{
		Name:        "x",
		KeyPrefixes: []string{"x-"},
		Validation: ValidationConfig{
			Method: "GET",
			URL:    "https://api.example.com/v1/me",
			Auth:   "token",
		},
	}
	issues := LintProvider(cfg)
	found := false
	for _, i := range issues {
		if i.Field == "validation.auth" {
			found = true
		}
	}
	assert.True(t, found, "unknown auth scheme should produce an error")
}

func TestLintProvider_HeaderAuthIsAccepted(t *testing.T) {
	cfg := ProviderConfig{
		Name:        "x",
		KeyPrefixes: []string{"x-"},
		Validation: ValidationConfig{
			Method: "GET",
			URL:    "https://api.example.com/v1/me",
			Auth:   "header:X-Api-Key",
		},
	}
	for _, i := range LintProvider(cfg) {
		assert.NotEqual(t, "validation.auth", i.Field, "header:X-Api-Key should be accepted")
	}
}

func TestLintProvider_URLMustBeHTTP(t *testing.T) {
	cfg := ProviderConfig{
		Name:        "x",
		KeyPrefixes: []string{"x-"},
		Validation: ValidationConfig{
			Method: "GET",
			URL:    "ftp://api.example.com/v1/me",
			Auth:   "bearer",
		},
	}
	issues := LintProvider(cfg)
	found := false
	for _, i := range issues {
		if i.Field == "validation.url" && i.Severity == SeverityError {
			found = true
		}
	}
	assert.True(t, found)
}

func TestLintProvider_MissingURL(t *testing.T) {
	cfg := ProviderConfig{
		Name:        "x",
		KeyPrefixes: []string{"x-"},
		Validation: ValidationConfig{
			Method: "POST",
			Auth:   "bearer",
		},
	}
	issues := LintProvider(cfg)
	found := false
	for _, i := range issues {
		if i.Field == "validation.url" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestLintProvider_RetryPolicyBounds(t *testing.T) {
	cfg := ProviderConfig{
		Name:        "x",
		KeyPrefixes: []string{"x-"},
		Validation: ValidationConfig{
			Method: "GET",
			URL:    "https://api.example.com",
			Auth:   "bearer",
		},
		RetryPolicy: &RetryPolicy{
			MaxAttempts:      0,
			InitialBackoffMs: 200,
			MaxBackoffMs:     100,
		},
	}
	issues := LintProvider(cfg)
	gotError := false
	gotWarn := false
	for _, i := range issues {
		if i.Field == "retry_policy.max_attempts" && i.Severity == SeverityError {
			gotError = true
		}
		if i.Field == "retry_policy.backoff_ms" && i.Severity == SeverityWarning {
			gotWarn = true
		}
	}
	assert.True(t, gotError, "max_attempts < 1 should be an error")
	assert.True(t, gotWarn, "initial > max should be a warning")
}

func TestLintProvider_NegativeCacheTTL(t *testing.T) {
	cfg := ProviderConfig{
		Name:        "x",
		KeyPrefixes: []string{"x-"},
		Validation: ValidationConfig{
			Method: "GET", URL: "https://api.example.com", Auth: "bearer",
		},
		CacheTTLSeconds: -5,
	}
	issues := LintProvider(cfg)
	found := false
	for _, i := range issues {
		if i.Field == "cache_ttl_seconds" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestLintAll_DuplicateNames(t *testing.T) {
	configs := []ProviderConfig{
		{Name: "openai", KeyPrefixes: []string{"sk-"}, Validation: ValidationConfig{Method: "GET", URL: "https://api.openai.com", Auth: "bearer"}},
		{Name: "openai", KeyPrefixes: []string{"x-"}, Validation: ValidationConfig{Method: "GET", URL: "https://api.example.com", Auth: "bearer"}},
	}
	issues, ok := LintAll(configs)
	assert.False(t, ok, "duplicate names should produce errors")
	foundDup := false
	for _, i := range issues {
		if i.Provider == "openai" && i.Severity == SeverityError {
			foundDup = true
		}
	}
	assert.True(t, foundDup)
}

func TestLintFile_ReadsAndLints(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.yaml")
	body := `
- name: my-provider
  category: llm
  key_prefixes: ["mp-"]
  validation:
    method: GET
    url: "https://api.example.com/v1/me"
    auth: bearer
`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	issues, ok, err := LintFile(path)
	require.NoError(t, err)
	assert.True(t, ok, "valid provider should lint clean, got: %v", issues)
}

func TestLintFile_ReportsBadYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte("not: valid: yaml: at: all:\n  - [\n"), 0o600))

	_, _, err := LintFile(path)
	assert.Error(t, err)
}
