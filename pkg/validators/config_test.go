package validators

import (
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestLoadProviderConfigs(t *testing.T) {
	configs, err := LoadProviderConfigs()

	require.NoError(t, err)
	assert.Greater(t, len(configs), 0, "should load at least one provider config")

	seen := make(map[string]bool)
	for _, cfg := range configs {
		assert.NotEmpty(t, cfg.Name, "provider name should not be empty")
		assert.NotEmpty(t, cfg.Category, "provider category should not be empty")
		assert.True(t, len(cfg.KeyPrefixes) > 0 || len(cfg.KeyPatterns) > 0,
			"provider should have at least one key prefix or pattern")

		assert.False(t, seen[cfg.Name], "provider names should be unique")
		seen[cfg.Name] = true
	}
}

func TestLoadProviderConfigs_Validation(t *testing.T) {
	configs, err := LoadProviderConfigs()
	require.NoError(t, err)

	for _, cfg := range configs {
		t.Run(cfg.Name, func(t *testing.T) {
			assert.NotEmpty(t, cfg.Validation.URL, "validation URL should not be empty")
			assert.NotEmpty(t, cfg.Validation.Method, "validation method should not be empty")
			assert.Contains(t, []string{"GET", "POST", "PUT", "DELETE"}, cfg.Validation.Method,
				"validation method should be valid HTTP method")
		})
	}
}

func TestBuildDetectionIndex(t *testing.T) {
	configs := []ProviderConfig{
		{
			Name:        "test-provider-1",
			Category:    "test",
			KeyPrefixes: []string{"tp1-", "tp1-long-prefix-"},
			KeyPatterns: []string{"^tp1-[a-z0-9]+$"},
		},
		{
			Name:        "test-provider-2",
			Category:    "test",
			KeyPrefixes: []string{"tp2-"},
			KeyPatterns: []string{"^tp2-[a-z0-9]+$", "^tp2-[a-z]{10}$"},
		},
	}

	prefixes, patterns := BuildDetectionIndex(configs)

	assert.Greater(t, len(prefixes), 0, "should have prefix entries")
	assert.Greater(t, len(patterns), 0, "should have pattern entries")

	prefixMap := make(map[string]string)
	for _, p := range prefixes {
		prefixMap[p.Prefix] = p.Provider
	}

	assert.Equal(t, "test-provider-1", prefixMap["tp1-long-prefix-"],
		"longer prefix should be checked first")
	assert.Equal(t, "test-provider-2", prefixMap["tp2-"],
		"provider 2 should be detected")
}

func TestBuildDetectionIndex_InvalidRegex(t *testing.T) {
	configs := []ProviderConfig{
		{
			Name:        "invalid-regex",
			Category:    "test",
			KeyPatterns: []string{"[invalid", "^valid-[a-z]+$"},
		},
	}

	prefixes, patterns := BuildDetectionIndex(configs)

	assert.Empty(t, prefixes)
	assert.Len(t, patterns, 1, "should skip invalid regex and keep valid one")
	assert.Equal(t, "invalid-regex", patterns[0].Provider)
}

func TestSortPrefixesByLength(t *testing.T) {
	entries := []PrefixEntry{
		{Prefix: "a", Provider: "p1"},
		{Prefix: "abc", Provider: "p2"},
		{Prefix: "ab", Provider: "p3"},
	}

	sortPrefixesByLength(entries)

	assert.Equal(t, "abc", entries[0].Prefix, "longest first")
	assert.Equal(t, "ab", entries[1].Prefix)
	assert.Equal(t, "a", entries[2].Prefix, "shortest last")
}

func TestSortPatternsBySpecificity(t *testing.T) {
	entries := []PatternEntry{
		{Regex: regexp.MustCompile("^a$"), Provider: "p1"},
		{Regex: regexp.MustCompile("^[a-z]+$"), Provider: "p2"},
		{Regex: regexp.MustCompile("^test-[a-z0-9]+$"), Provider: "p3"},
	}

	sortPatternsBySpecificity(entries)

	// Longer/more specific regex patterns should come first
	assert.Equal(t, "p3", entries[0].Provider, "most specific first")
}

func TestProviderConfigEmbedded(t *testing.T) {
	tests := []struct {
		name     string
		filename string
	}{
		{"llm providers", "llm.yaml"},
		{"common services", "common-services.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := providersFS.ReadFile(filepath.Join("providers", tt.filename))
			require.NoError(t, err)
			assert.Greater(t, len(data), 0, "yaml file should not be empty")

			var configs []ProviderConfig
			err = yaml.Unmarshal(data, &configs)
			require.NoError(t, err)
			assert.Greater(t, len(configs), 0, "should parse at least one provider")
		})
	}
}
