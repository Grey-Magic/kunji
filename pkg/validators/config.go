package validators

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

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

func LoadProviderConfigs() ([]ProviderConfig, error) {
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
		return nil, err
	}

	return allConfigs, nil
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

	for _, p := range prefixes {
		if manualCategory != "" && p.Category != manualCategory {
			continue
		}
		if strings.HasPrefix(key, p.Prefix) {
			return p.Provider
		}
	}

	for _, p := range patterns {
		if manualCategory != "" && p.Category != manualCategory {
			continue
		}
		if p.Regex.MatchString(key) {
			return p.Provider
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
