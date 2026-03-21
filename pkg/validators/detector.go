package validators

import (
	"regexp"
	"strings"

	"github.com/pterm/pterm"
)

type Detector struct {
	prefixes         []PrefixEntry
	patterns         []PatternEntry
	prefixMap        PrefixIndex
	providerPatterns map[string][]*regexp.Regexp
}

func NewDetectorFromConfigs(configs []ProviderConfig) *Detector {
	prefixes, patterns, prefixMap, providerPatterns := BuildDetectionIndex(configs)
	return &Detector{
		prefixes:         prefixes,
		patterns:         patterns,
		prefixMap:        prefixMap,
		providerPatterns: providerPatterns,
	}
}

func NewDetector() *Detector {
	configs, err := LoadProviderConfigs()
	if err != nil {
		pterm.Warning.Printfln("Failed to load provider configs: %v", err)
		return NewDetectorFromConfigs(nil)
	}
	return NewDetectorFromConfigs(configs)
}

func (d *Detector) DetectProvider(apiKey string, manualCategory string) string {
	return DetectProviderFromIndex(strings.TrimSpace(apiKey), d.prefixes, d.patterns, manualCategory, d.providerPatterns)
}

func (d *Detector) DetectProviderWithSuggestion(apiKey string, manualCategory string) DetectionResult {
	return DetectProviderWithSuggestion(strings.TrimSpace(apiKey), d.prefixes, d.patterns, manualCategory, d.providerPatterns)
}
