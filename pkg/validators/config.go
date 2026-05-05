package validators

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
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

// ... (keep MetadataConfig and other existing types)

func isHex(s string) bool {
	match, _ := regexp.MatchString("^[a-f0-9]+$", strings.ToLower(s))
	return match
}

func isUUID(s string) bool {
	match, _ := regexp.MatchString("^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$", strings.ToLower(s))
	return match
}

func decodeJWT(s string) map[string]interface{} {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil
	}
	return data
}

//go:embed providers
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

type EndpointConfig struct {
	URL     string            `yaml:"url"`
	Region  string            `yaml:"region,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

type ErrorCheck struct {
	JSONPath string `yaml:"json_path,omitempty"`
}

type ValidationConfig struct {
	Method     string            `yaml:"method"`
	URL        string            `yaml:"url"`
	Auth       string            `yaml:"auth"`
	Headers    map[string]string `yaml:"headers"`
	Body       string            `yaml:"body"`
	Endpoints  []EndpointConfig  `yaml:"endpoints,omitempty"`
	ErrorCheck *ErrorCheck       `yaml:"error_check,omitempty"`
}

type ProviderConfig struct {
	Name                   string                  `yaml:"name"`
	Category               string                  `yaml:"category"`
	KeyPrefixes            []string                `yaml:"key_prefixes"`
	KeyPatterns            []string                `yaml:"key_patterns"`
	SyntaxCheck            string                  `yaml:"syntax_check,omitempty"`
	CanaryPatterns         []string                `yaml:"canary_patterns,omitempty"`
	Validation             ValidationConfig        `yaml:"validation"`
	Metadata               []MetadataConfig        `yaml:"metadata,omitempty"`
	MetadataFromValidation *MetadataFromValidation `yaml:"metadata_from_validation,omitempty"`
	IsCustom               bool                    `yaml:"-"`
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
			pterm.Warning.Printfln("Could not parse custom provider config %s: %v", path, err)
			continue
		}

		for i := range configs {
			configs[i].IsCustom = true
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

type TrieNode struct {
	children map[rune]*TrieNode
	output   []*PrefixEntry
	fail     *TrieNode
}

type PrefixTrie struct {
	root *TrieNode
}

func NewPrefixTrie() *PrefixTrie {
	return &PrefixTrie{root: &TrieNode{children: make(map[rune]*TrieNode)}}
}

func (t *PrefixTrie) Add(p *PrefixEntry) {
	node := t.root
	for _, r := range p.Prefix {
		if node.children[r] == nil {
			node.children[r] = &TrieNode{children: make(map[rune]*TrieNode)}
		}
		node = node.children[r]
	}
	node.output = append(node.output, p)
}

func (t *PrefixTrie) Build() {
	queue := make([]*TrieNode, 0)
	for _, child := range t.root.children {
		child.fail = t.root
		queue = append(queue, child)
	}

	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]

		for r, v := range u.children {
			f := u.fail
			for f != t.root && f.children[r] == nil {
				f = f.fail
			}
			if f.children[r] != nil {
				v.fail = f.children[r]
			} else {
				v.fail = t.root
			}
			v.output = append(v.output, v.fail.output...)
			queue = append(queue, v)
		}
	}
}

func (t *PrefixTrie) Search(key string) []*PrefixEntry {
	var results []*PrefixEntry
	node := t.root
	for _, r := range key {
		if node.children[r] == nil {
			break // Only match at the start
		}
		node = node.children[r]
		if len(node.output) > 0 {
			results = append(results, node.output...)
		}
	}
	return results
}

type DetectorIndex struct {
	prefixes         []PrefixEntry
	patterns         []PatternEntry
	prefixMap        PrefixIndex
	providerPatterns map[string][]*regexp.Regexp
	trie             *PrefixTrie
}

func BuildDetectionIndex(configs []ProviderConfig) *DetectorIndex {
	idx := &DetectorIndex{
		prefixMap:        make(PrefixIndex),
		providerPatterns: make(map[string][]*regexp.Regexp),
		trie:             NewPrefixTrie(),
	}

	for _, cfg := range configs {
		var providerRes []*regexp.Regexp
		for _, p := range cfg.KeyPatterns {
			re, err := regexp.Compile(p)
			if err != nil {
				pterm.Warning.Printf("Invalid regex pattern for provider %s: %s - %v\n", cfg.Name, p, err)
				continue
			}
			idx.patterns = append(idx.patterns, PatternEntry{Regex: re, Provider: cfg.Name, Category: cfg.Category})
			providerRes = append(providerRes, re)
		}
		idx.providerPatterns[cfg.Name] = providerRes

		for _, p := range cfg.KeyPrefixes {
			if p != "" {
				entry := PrefixEntry{Prefix: p, Provider: cfg.Name, Category: cfg.Category}
				idx.prefixes = append(idx.prefixes, entry)
				firstChar := string(p[0])
				idx.prefixMap[firstChar] = append(idx.prefixMap[firstChar], entry)
				idx.trie.Add(&entry)
			}
		}
	}

	idx.trie.Build()
	sortPrefixesByLength(idx.prefixes)
	sortPatternsBySpecificity(idx.patterns)

	for _, entries := range idx.prefixMap {
		sortPrefixesByLength(entries)
	}

	return idx
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

func DetectProviderWithSuggestion(key string, idx *DetectorIndex, manualCategory string) DetectionResult {
	key = strings.TrimSpace(key)
	if key == "" {
		return DetectionResult{Provider: "unknown", Message: "Empty key", Entropy: 0}
	}

	entropy := utils.CalculateShannonEntropy(key)
	keyLower := strings.ToLower(key)
	scores := make(map[string]int)

	// 1. Structural Analysis
	// JWT Decoding
	if strings.HasPrefix(key, "eyJ") {
		if payload := decodeJWT(key); payload != nil {
			// Heuristics for common JWT-based providers
			if iss, ok := payload["iss"].(string); ok {
				if strings.Contains(iss, "supabase") {
					scores["supabase"] += 300
				} else if strings.Contains(iss, "clerk") {
					scores["clerk"] += 300
				} else if strings.Contains(iss, "auth0") {
					scores["auth0"] += 300
				}
			}
			// Check other common claims
			if _, ok := payload["grafana_user"]; ok {
				scores["grafana"] += 300
			}
		}
	}

	// Fingerprinting
	isPureHex := isHex(key)
	isUuidFormat := isUUID(key)

	// 2. Prefix Scoring (Aho-Corasick Optimized)
	trieMatches := idx.trie.Search(key)
	for _, p := range trieMatches {
		if manualCategory != "" && p.Category != manualCategory {
			continue
		}
		// Since we matched via trie, we know it has the prefix
		scores[p.Provider] += 100 + len(p.Prefix)*2
	}

	// Fallback for case-insensitive prefixes not in trie (or simple search)
	if len(trieMatches) == 0 {
		for _, p := range idx.prefixes {
			if p.Prefix == "" {
				continue
			}
			if manualCategory != "" && p.Category != manualCategory {
				continue
			}
			if strings.HasPrefix(keyLower, strings.ToLower(p.Prefix)) {
				scores[p.Provider] += 50 + len(p.Prefix)*2
			}
		}
	}

	// 3. Pattern Scoring
	for _, p := range idx.patterns {
		if manualCategory != "" && p.Category != manualCategory {
			continue
		}

		if p.Regex.MatchString(key) {
			patternScore := 50
			reStr := p.Regex.String()

			// Specificity bonus: longer, more complex regexes are more likely to be correct
			patternScore += len(reStr)

			// Anchored patterns are much more reliable
			if strings.HasPrefix(reStr, "^") && strings.HasSuffix(reStr, "$") {
				patternScore += 30
			}

			// Character class complexity bonus
			if strings.Contains(reStr, "[a-zA-Z0-9]") || strings.Contains(reStr, "[a-f0-9]") {
				patternScore += 10
			}

			// Fingerprint alignment bonus
			if isPureHex && strings.Contains(reStr, "[a-f0-9]") {
				patternScore += 20
			}
			if isUuidFormat && (strings.Contains(reStr, "[a-f0-9]{8}") || strings.Contains(reStr, "0-9a-f")) {
				patternScore += 30
			}

			// Penalty for extremely loose patterns
			literalInfo := 0
			inClass := false
			for i := 0; i < len(reStr); i++ {
				if reStr[i] == '[' {
					inClass = true
					continue
				}
				if reStr[i] == ']' {
					inClass = false
					continue
				}
				if !inClass && !strings.ContainsAny(string(reStr[i]), "^$.*+?{}()|\\") {
					literalInfo++
				}
			}
			if literalInfo < 3 {
				patternScore -= 40
			}

			scores[p.Provider] += patternScore
		}
	}

	// 4. Heuristics & Weights
	// OpenAI internal marker - extremely high confidence
	if strings.Contains(key, "T3BlbkFJ") {
		scores["openai"] += 250
	}
	// Stripe internal markers
	if strings.Contains(keyLower, "live") && strings.Contains(keyLower, "sk_") {
		scores["stripe"] += 60
	}
	// DeepSeek specific (only sk- and 32 hex chars)
	if isPureHex && len(key) == 35 && strings.HasPrefix(key, "sk-") {
		scores["deepseek"] += 50
	}

	// Priority weights for common/major providers to break ties
	priorities := map[string]int{
		"openai":      10,
		"gemini":      9,
		"anthropic":   9,
		"stripe":      8,
		"aws":         8,
		"github":      8,
		"google_maps": 7,
		"deepseek":    7,
	}
	for p, weight := range priorities {
		if _, exists := scores[p]; exists {
			scores[p] += weight
		}
	}

	// Find best match
	bestProvider := "unknown"
	maxScore := 0
	var ties []string

	for p, s := range scores {
		if s > maxScore {
			maxScore = s
			bestProvider = p
			ties = []string{p}
		} else if s == maxScore && s > 0 {
			ties = append(ties, p)
		}
	}

	// Handle ties
	if len(ties) > 1 {
		// If all ties are Google services, pick one (e.g., google_maps or gemini)
		isAllGoogle := true
		for _, t := range ties {
			if !strings.HasPrefix(t, "google_") && t != "gemini" && t != "youtube" {
				isAllGoogle = false
				break
			}
		}
		if isAllGoogle {
			// Pick gemini or maps as a representative
			for _, t := range ties {
				if t == "gemini" {
					return DetectionResult{Provider: t, Entropy: entropy}
				}
			}
			return DetectionResult{Provider: ties[0], Entropy: entropy}
		}

		// Otherwise return unknown but with suggestions
		return DetectionResult{
			Provider:    "unknown",
			Suggestions: ties,
			Entropy:     entropy,
			Message:     fmt.Sprintf("Ambiguous key (Score: %d) - matches: %s", maxScore, strings.Join(ties, ", ")),
		}
	}

	if bestProvider != "unknown" {
		return DetectionResult{
			Provider: bestProvider,
			Entropy:  entropy,
		}
	}

	// Last resort: suggestions
	suggestions := generateSuggestions(key, idx.prefixes)
	msg := "Could not auto-detect provider"
	if entropy > 3.5 {
		msg = fmt.Sprintf("Unknown high-entropy key (%.2f)", entropy)
	}
	return DetectionResult{
		Provider:    "unknown",
		Suggestions: suggestions,
		Entropy:     entropy,
		Message:     msg,
	}
}

func GetCommonDomains() []string {
	configs, err := LoadProviderConfigs()
	if err != nil {
		return nil
	}

	domainMap := make(map[string]bool)
	for _, cfg := range configs {
		if u, err := url.Parse(cfg.Validation.URL); err == nil {
			host := u.Hostname()
			if host != "" {
				domainMap[host] = true
			}
		}
		for _, m := range cfg.Metadata {
			if u, err := url.Parse(m.URL); err == nil {
				host := u.Hostname()
				if host != "" {
					domainMap[host] = true
				}
			}
		}
	}

	domains := make([]string, 0, len(domainMap))
	for d := range domainMap {
		domains = append(domains, d)
	}
	return domains
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
