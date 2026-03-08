package validators

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/pterm/pterm"
	"gopkg.in/yaml.v3"
)

//go:embed providers/*.yaml
var providersFS embed.FS

type MetadataConfig struct {
	URL         string            `yaml:"url"`
	Method      string            `yaml:"method"`
	Auth        string            `yaml:"auth"`
	Headers     map[string]string `yaml:"headers"`
	BalancePath string            `yaml:"balance_path"`
	Extract     string            `yaml:"extract"`
	StoreAs     string            `yaml:"store_as"`
}

type MetadataFromValidation struct {
	BalancePath         string `yaml:"balance_path"`
	BalanceSubtractPath string `yaml:"balance_subtract_path"`
	NamePath            string `yaml:"name_path"`
	NameFallbackPath    string `yaml:"name_fallback_path"`
	EmailPath           string `yaml:"email_path"`
	QuotaPath           string `yaml:"quota_path"`
	CreditsPath         string `yaml:"credits_path"`
	VIPLevelPath        string `yaml:"vip_level_path"`
	TeamNamePath        string `yaml:"team_name_path"`
	UsernamePath        string `yaml:"username_path"`
}

type ValidationConfig struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Auth    string            `yaml:"auth"`
	Headers map[string]string `yaml:"headers"`
	Body    string            `yaml:"body"`
}

type ProviderConfig struct {
	Name                   string                  `yaml:"name"`
	Category               string                  `yaml:"category"`
	KeyPrefixes            []string                `yaml:"key_prefixes"`
	KeyPatterns            []string                `yaml:"key_patterns"`
	Validation             ValidationConfig        `yaml:"validation"`
	Metadata               []MetadataConfig        `yaml:"metadata"`
	MetadataFromValidation *MetadataFromValidation `yaml:"metadata_from_validation"`
}

var (
	configsCache      []ProviderConfig
	configsCacheOnce  sync.Once
	configsCacheError error
)

func LoadProviderConfigs() ([]ProviderConfig, error) {
	configsCacheOnce.Do(func() {
		var allConfigs []ProviderConfig

		err := fs.WalkDir(providersFS, "providers", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".yaml" {
				return nil
			}

			data, err := providersFS.ReadFile(path)
			if err != nil {
				return fmt.Errorf("reading %s: %w", path, err)
			}

			var configs []ProviderConfig
			if err := yaml.Unmarshal(data, &configs); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}

			allConfigs = append(allConfigs, configs...)
			return nil
		})

		if err != nil {
			configsCacheError = err
			return
		}

		configsCache = allConfigs
	})

	if configsCacheError != nil {
		return nil, configsCacheError
	}
	return configsCache, nil
}

type PrefixEntry struct {
	Prefix   string
	Provider string
	Category string
}

type PatternEntry struct {
	Regex    *regexp.Regexp
	Provider string
	Category string
}

func BuildDetectionIndex(configs []ProviderConfig) ([]PrefixEntry, []PatternEntry) {
	var prefixes []PrefixEntry
	var patterns []PatternEntry

	for _, cfg := range configs {
		for _, p := range cfg.KeyPrefixes {
			if p != "" {
				prefixes = append(prefixes, PrefixEntry{Prefix: p, Provider: cfg.Name, Category: cfg.Category})
			}
		}
		for _, p := range cfg.KeyPatterns {
			re, err := regexp.Compile(p)
			if err != nil {
				pterm.Warning.Printf("Invalid regex pattern for provider %s: %s - %v\n", cfg.Name, p, err)
				continue
			}
			patterns = append(patterns, PatternEntry{Regex: re, Provider: cfg.Name, Category: cfg.Category})
		}
	}

	sortPrefixesByLength(prefixes)
	sortPatternsBySpecificity(patterns)
	return prefixes, patterns
}

func sortPrefixesByLength(entries []PrefixEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return len(entries[i].Prefix) > len(entries[j].Prefix)
	})
}

func sortPatternsBySpecificity(patterns []PatternEntry) {
	sort.Slice(patterns, func(i, j int) bool {
		return len(patterns[i].Regex.String()) > len(patterns[j].Regex.String())
	})
}

func DetectProviderFromIndex(key string, prefixes []PrefixEntry, patterns []PatternEntry, manualCategory string) string {
	key = strings.TrimSpace(key)

	if key == "" {
		return "unknown"
	}

	type match struct {
		provider string
		prefix   string
		length   int
	}

	var matches []match
	for _, p := range prefixes {
		if manualCategory != "" && p.Category != manualCategory {
			continue
		}
		if strings.HasPrefix(key, p.Prefix) {
			matches = append(matches, match{provider: p.Provider, prefix: p.Prefix, length: len(p.Prefix)})
		}
	}

	if len(matches) == 1 {
		return matches[0].provider
	}

	if len(matches) > 1 {
		maxLen := 0
		for _, m := range matches {
			if m.length > maxLen {
				maxLen = m.length
			}
		}
		var longestMatches []match
		for _, m := range matches {
			if m.length == maxLen {
				longestMatches = append(longestMatches, m)
			}
		}
		if len(longestMatches) == 1 {
			return longestMatches[0].provider
		}
		return "unknown"
	}

	patternProviders := make(map[string]bool)

	for _, p := range patterns {
		if manualCategory != "" && p.Category != manualCategory {
			continue
		}
		if p.Regex.MatchString(key) {
			patternProviders[p.Provider] = true
		}
	}

	if len(patternProviders) == 1 {
		for provider := range patternProviders {
			return provider
		}
	}

	if len(patternProviders) > 1 {
		return "unknown"
	}

	return "unknown"
}

type ProviderInfo struct {
	Name        string
	Category    string
	KeyPrefixes []string
}

func GetAllProviders() ([]ProviderInfo, error) {
	configs, err := LoadProviderConfigs()
	if err != nil {
		return nil, err
	}

	providers := make([]ProviderInfo, len(configs))
	for i, cfg := range configs {
		providers[i] = ProviderInfo{
			Name:        cfg.Name,
			Category:    cfg.Category,
			KeyPrefixes: cfg.KeyPrefixes,
		}
	}

	return providers, nil
}

func GetCategories() ([]string, error) {
	configs, err := LoadProviderConfigs()
	if err != nil {
		return nil, err
	}

	categorySet := make(map[string]bool)
	for _, cfg := range configs {
		if cfg.Category != "" {
			categorySet[cfg.Category] = true
		}
	}

	categories := make([]string, 0, len(categorySet))
	for cat := range categorySet {
		categories = append(categories, cat)
	}
	sort.Strings(categories)
	return categories, nil
}

func GetProvidersByCategory(category string) ([]ProviderInfo, error) {
	configs, err := LoadProviderConfigs()
	if err != nil {
		return nil, err
	}

	var providers []ProviderInfo
	for _, cfg := range configs {
		if cfg.Category == category {
			providers = append(providers, ProviderInfo{
				Name:        cfg.Name,
				Category:    cfg.Category,
				KeyPrefixes: cfg.KeyPrefixes,
			})
		}
	}
	return providers, nil
}

type ProviderGroup struct {
	Prefix    string
	Providers []ProviderInfo
}

func GetProviderGroups() (map[string][]ProviderInfo, error) {
	configs, err := LoadProviderConfigs()
	if err != nil {
		return nil, err
	}

	groupMap := make(map[string][]ProviderInfo)
	for _, cfg := range configs {
		if len(cfg.KeyPrefixes) > 0 {
			prefix := cfg.KeyPrefixes[0]
			groupMap[prefix] = append(groupMap[prefix], ProviderInfo{
				Name:        cfg.Name,
				Category:    cfg.Category,
				KeyPrefixes: cfg.KeyPrefixes,
			})
		}
	}

	return groupMap, nil
}

func FindProviderByName(name string) ([]ProviderInfo, error) {
	configs, err := LoadProviderConfigs()
	if err != nil {
		return nil, err
	}

	nameLower := strings.ToLower(name)
	var providers []ProviderInfo
	for _, cfg := range configs {
		cfgNameLower := strings.ToLower(cfg.Name)
		if strings.HasPrefix(cfgNameLower, nameLower+"_") || cfgNameLower == nameLower {
			providers = append(providers, ProviderInfo{
				Name:        cfg.Name,
				Category:    cfg.Category,
				KeyPrefixes: cfg.KeyPrefixes,
			})
		}
	}
	return providers, nil
}

// DetectionResult contains detection result with optional suggestions
type DetectionResult struct {
	Provider    string
	Suggestions []string
	Message     string
}

// DetectProviderWithSuggestion detects provider and provides suggestions if detection fails
func DetectProviderWithSuggestion(key string, prefixes []PrefixEntry, patterns []PatternEntry, manualCategory string) DetectionResult {
	key = strings.TrimSpace(key)

	if key == "" {
		return DetectionResult{
			Provider: "unknown",
			Message:  "Empty key provided",
		}
	}

	type match struct {
		provider string
		prefix   string
		length   int
	}

	var matches []match
	for _, p := range prefixes {
		if manualCategory != "" && p.Category != manualCategory {
			continue
		}
		if strings.HasPrefix(key, p.Prefix) {
			matches = append(matches, match{provider: p.Provider, prefix: p.Prefix, length: len(p.Prefix)})
		}
	}

	if len(matches) == 1 {
		return DetectionResult{
			Provider: matches[0].provider,
		}
	}

	if len(matches) > 1 {
		maxLen := 0
		for _, m := range matches {
			if m.length > maxLen {
				maxLen = m.length
			}
		}
		var longestMatches []match
		for _, m := range matches {
			if m.length == maxLen {
				longestMatches = append(longestMatches, m)
			}
		}
		if len(longestMatches) == 1 {
			return DetectionResult{
				Provider: longestMatches[0].provider,
			}
		}
		// Multiple matches with same prefix length - return suggestions
		var providerNames []string
		seen := make(map[string]bool)
		for _, m := range matches {
			if !seen[m.provider] {
				seen[m.provider] = true
				providerNames = append(providerNames, m.provider)
			}
		}
		suggestions := generateSuggestions(key, prefixes)
		return DetectionResult{
			Provider:    "unknown",
			Suggestions: suggestions,
			Message:     fmt.Sprintf("Ambiguous key - matches: %s", strings.Join(providerNames, ", ")),
		}
	}

	// No prefix matches - try patterns
	patternProviders := make(map[string]bool)

	for _, p := range patterns {
		if manualCategory != "" && p.Category != manualCategory {
			continue
		}
		if p.Regex.MatchString(key) {
			patternProviders[p.Provider] = true
		}
	}

	if len(patternProviders) == 1 {
		for provider := range patternProviders {
			return DetectionResult{
				Provider: provider,
			}
		}
	}

	if len(patternProviders) > 1 {
		var providerNames []string
		for p := range patternProviders {
			providerNames = append(providerNames, p)
		}
		suggestions := generateSuggestions(key, prefixes)
		return DetectionResult{
			Provider:    "unknown",
			Suggestions: suggestions,
			Message:     fmt.Sprintf("Ambiguous key - matches: %s", strings.Join(providerNames, ", ")),
		}
	}

	// No matches at all - generate suggestions
	suggestions := generateSuggestions(key, prefixes)
	return DetectionResult{
		Provider:    "unknown",
		Suggestions: suggestions,
		Message:     "Could not auto-detect provider",
	}
}

// generateSuggestions finds providers with similar prefixes
func generateSuggestions(key string, prefixes []PrefixEntry) []string {
	keyLower := strings.ToLower(key)

	// Find prefixes that partially match
	var suggestions []string
	seen := make(map[string]bool)

	// First, check for any prefix match (even partial)
	for _, p := range prefixes {
		if len(p.Prefix) < 3 {
			continue
		}
		// Check if key starts with same characters as prefix
		if strings.HasPrefix(keyLower, p.Prefix[:min(len(p.Prefix), len(key))]) {
			if !seen[p.Provider] {
				seen[p.Provider] = true
				suggestions = append(suggestions, p.Provider)
			}
		}
	}

	// If no prefix matches, suggest providers with common prefixes
	if len(suggestions) == 0 {
		commonPrefixes := []string{"sk-", "sk_", "api_", "key-", "pk.", "token"}
		for _, cp := range commonPrefixes {
			if strings.HasPrefix(keyLower, cp) || (len(key) > 3 && strings.Contains(keyLower, cp)) {
				for _, p := range prefixes {
					if strings.HasPrefix(strings.ToLower(p.Prefix), cp) || p.Prefix == cp {
						if !seen[p.Provider] {
							seen[p.Provider] = true
							suggestions = append(suggestions, p.Provider)
						}
					}
				}
			}
		}
	}

	// Limit to top 5 suggestions
	if len(suggestions) > 5 {
		suggestions = suggestions[:5]
	}

	return suggestions
}
