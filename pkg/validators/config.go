package validators

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Grey-Magic/kunji/pkg/utils"
	"github.com/pterm/pterm"
	"gopkg.in/yaml.v3"
)

//go:embed providers/*.yaml
var providersFS embed.FS

type MetadataConfig struct {
	URL               string            `yaml:"url"`
	Method            string            `yaml:"method"`
	Auth              string            `yaml:"auth"`
	Headers           map[string]string `yaml:"headers"`
	BalancePath       string            `yaml:"balance_path"`
	Extract           string            `yaml:"extract"`
	StoreAs           string            `yaml:"store_as"`
	RegexExtract      string            `yaml:"regex_extract"`
	RegexExtractMatch int               `yaml:"regex_extract_match"`
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
	RegexExtract        string `yaml:"regex_extract"`
	RegexExtractMatch   int    `yaml:"regex_extract_match"`
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
	configsCache       []ProviderConfig
	configsCacheOnce   sync.Once
	configsCacheError  error
	CustomProvidersDir string
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

		if CustomProvidersDir != "" {
			importCustomProviderConfigs(&allConfigs, CustomProvidersDir)
		}

		homeDir, err := os.UserHomeDir()
		if err == nil {
			defaultPath := filepath.Join(homeDir, ".kunji", "providers")
			if _, err := os.Stat(defaultPath); err == nil {
				importCustomProviderConfigs(&allConfigs, defaultPath)
			}
		}

		configsCache = allConfigs
	})

	if configsCacheError != nil {
		return nil, configsCacheError
	}
	return configsCache, nil
}

func importCustomProviderConfigs(allConfigs *[]ProviderConfig, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		pterm.Warning.Printfln("Could not read custom providers directory: %v", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			pterm.Warning.Printfln("Error reading custom provider file %s: %v", path, err)
			continue
		}

		var configs []ProviderConfig
		if err := yaml.Unmarshal(data, &configs); err != nil {
			pterm.Warning.Printfln("Error parsing custom provider file %s: %v", path, err)
			continue
		}

		*allConfigs = append(*allConfigs, configs...)
		pterm.Info.Printfln("Loaded custom provider file: %s", entry.Name())
	}
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

type PrefixIndex map[string][]PrefixEntry

type DetectorIndex struct {
	prefixes  []PrefixEntry
	patterns  []PatternEntry
	prefixMap PrefixIndex
	providerPatterns map[string][]*regexp.Regexp
}

func BuildDetectionIndex(configs []ProviderConfig) ([]PrefixEntry, []PatternEntry, PrefixIndex, map[string][]*regexp.Regexp) {
	var prefixes []PrefixEntry
	var patterns []PatternEntry
	prefixMap := make(PrefixIndex)
	providerPatterns := make(map[string][]*regexp.Regexp)

	for _, cfg := range configs {
		var providerRes []*regexp.Regexp
		for _, p := range cfg.KeyPatterns {
			re, err := regexp.Compile(p)
			if err != nil {
				pterm.Warning.Printf("Invalid regex pattern for provider %s: %s - %v\n", cfg.Name, p, err)
				continue
			}
			patterns = append(patterns, PatternEntry{Regex: re, Provider: cfg.Name, Category: cfg.Category})
			providerRes = append(providerRes, re)
		}
		providerPatterns[cfg.Name] = providerRes

		for _, p := range cfg.KeyPrefixes {
			if p != "" {
				entry := PrefixEntry{Prefix: p, Provider: cfg.Name, Category: cfg.Category}
				prefixes = append(prefixes, entry)
				firstChar := string(p[0])
				prefixMap[firstChar] = append(prefixMap[firstChar], entry)
			}
		}
	}

	sortPrefixesByLength(prefixes)
	sortPatternsBySpecificity(patterns)

	for _, entries := range prefixMap {
		sortPrefixesByLength(entries)
	}

	return prefixes, patterns, prefixMap, providerPatterns
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

func DetectProviderFromIndex(key string, prefixes []PrefixEntry, patterns []PatternEntry, manualCategory string, providerPatterns map[string][]*regexp.Regexp) string {
	key = strings.TrimSpace(key)

	if key == "" {
		return "unknown"
	}

	type match struct {
		provider string
		prefix   string
		length   int
	}

	var candidates []match
	if len(key) > 0 {
		for _, p := range prefixes {
			if len(p.Prefix) > 0 && p.Prefix[0] != key[0] {
				continue
			}
			if manualCategory != "" && p.Category != manualCategory {
				continue
			}
			if strings.HasPrefix(key, p.Prefix) {
				candidates = append(candidates, match{provider: p.Provider, prefix: p.Prefix, length: len(p.Prefix)})
			}
		}
	}

	if len(candidates) > 0 {
		// Filter candidates by their patterns if they have any
		var verifiedMatches []match
		for _, c := range candidates {
			res, hasPatterns := providerPatterns[c.provider]
			if !hasPatterns || len(res) == 0 {
				verifiedMatches = append(verifiedMatches, c)
				continue
			}

			matchedPattern := false
			for _, re := range res {
				if re.MatchString(key) {
					matchedPattern = true
					break
				}
			}
			if matchedPattern {
				verifiedMatches = append(verifiedMatches, c)
			}
		}

		if len(verifiedMatches) == 1 {
			return verifiedMatches[0].provider
		}

		if len(verifiedMatches) > 1 {
			// Pick the longest prefix match among verified ones
			maxLen := 0
			for _, m := range verifiedMatches {
				if m.length > maxLen {
					maxLen = m.length
				}
			}
			var longestMatches []match
			for _, m := range verifiedMatches {
				if m.length == maxLen {
					longestMatches = append(longestMatches, m)
				}
			}
			if len(longestMatches) == 1 {
				return longestMatches[0].provider
			}
			return "unknown"
		}
	}

	// Fallback to checking all patterns
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

type DetectionResult struct {
	Provider    string
	Suggestions []string
	Message     string
	Entropy     float64
}

func DetectProviderWithSuggestion(key string, prefixes []PrefixEntry, patterns []PatternEntry, manualCategory string, providerPatterns map[string][]*regexp.Regexp) DetectionResult {
	key = strings.TrimSpace(key)
	entropy := utils.CalculateShannonEntropy(key)

	if key == "" {
		return DetectionResult{
			Provider: "unknown",
			Message:  "Empty key provided",
			Entropy:  0,
		}
	}

	type match struct {
		provider string
		prefix   string
		length   int
	}

	var candidates []match
	if len(key) > 0 {
		for _, p := range prefixes {
			if len(p.Prefix) > 0 && p.Prefix[0] != key[0] {
				continue
			}
			if manualCategory != "" && p.Category != manualCategory {
				continue
			}
			if strings.HasPrefix(key, p.Prefix) {
				candidates = append(candidates, match{provider: p.Provider, prefix: p.Prefix, length: len(p.Prefix)})
			}
		}
	}

	if len(candidates) > 0 {
		var verifiedMatches []match
		for _, c := range candidates {
			res, hasPatterns := providerPatterns[c.provider]
			if !hasPatterns || len(res) == 0 {
				verifiedMatches = append(verifiedMatches, c)
				continue
			}

			matchedPattern := false
			for _, re := range res {
				if re.MatchString(key) {
					matchedPattern = true
					break
				}
			}
			if matchedPattern {
				verifiedMatches = append(verifiedMatches, c)
			}
		}

		if len(verifiedMatches) == 1 {
			return DetectionResult{
				Provider: verifiedMatches[0].provider,
				Entropy:  entropy,
			}
		}

		if len(verifiedMatches) > 1 {
			maxLen := 0
			for _, m := range verifiedMatches {
				if m.length > maxLen {
					maxLen = m.length
				}
			}
			var longestMatches []match
			for _, m := range verifiedMatches {
				if m.length == maxLen {
					longestMatches = append(longestMatches, m)
				}
			}
			if len(longestMatches) == 1 {
				return DetectionResult{
					Provider: longestMatches[0].provider,
					Entropy:  entropy,
				}
			}

			var providerNames []string
			seen := make(map[string]bool)
			for _, m := range verifiedMatches {
				if !seen[m.provider] {
					seen[m.provider] = true
					providerNames = append(providerNames, m.provider)
				}
			}
			suggestions := generateSuggestions(key, prefixes)
			return DetectionResult{
				Provider:    "unknown",
				Suggestions: suggestions,
				Entropy:     entropy,
				Message:     fmt.Sprintf("Ambiguous key (Entropy: %.2f) - matches: %s. Try --deep-scan", entropy, strings.Join(providerNames, ", ")),
			}
		}
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
			return DetectionResult{
				Provider: provider,
				Entropy:  entropy,
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
			Entropy:     entropy,
			Message:     fmt.Sprintf("Ambiguous key (Entropy: %.2f) - matches: %s. Try --deep-scan", entropy, strings.Join(providerNames, ", ")),
		}
	}

	suggestions := generateSuggestions(key, prefixes)
	msg := "Could not auto-detect provider"
	if entropy > 3.5 {
		msg = fmt.Sprintf("Unknown high-entropy key (%.2f). Use --deep-scan to brute-force likely providers.", entropy)
	}
	return DetectionResult{
		Provider:    "unknown",
		Suggestions: suggestions,
		Entropy:     entropy,
		Message:     msg,
	}
}

func generateSuggestions(key string, prefixes []PrefixEntry) []string {
	keyLower := strings.ToLower(key)

	var suggestions []string
	seen := make(map[string]bool)

	for _, p := range prefixes {
		if len(p.Prefix) < 3 {
			continue
		}

		if strings.HasPrefix(keyLower, p.Prefix[:min(len(p.Prefix), len(key))]) {
			if !seen[p.Provider] {
				seen[p.Provider] = true
				suggestions = append(suggestions, p.Provider)
			}
		}
	}

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

	if len(suggestions) > 5 {
		suggestions = suggestions[:5]
	}

	return suggestions
}
