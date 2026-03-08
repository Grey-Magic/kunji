package validators

import (
	"strings"

	"github.com/pterm/pterm"
)

type Detector struct {
	prefixes []PrefixEntry
	patterns []PatternEntry
}

func NewDetectorFromConfigs(configs []ProviderConfig) *Detector {
	prefixes, patterns := BuildDetectionIndex(configs)
	return &Detector{
		prefixes: prefixes,
		patterns: patterns,
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
	return DetectProviderFromIndex(strings.TrimSpace(apiKey), d.prefixes, d.patterns, manualCategory)
}

// DetectProviderWithSuggestion returns detection result with suggestions if detection fails
func (d *Detector) DetectProviderWithSuggestion(apiKey string, manualCategory string) DetectionResult {
	return DetectProviderWithSuggestion(strings.TrimSpace(apiKey), d.prefixes, d.patterns, manualCategory)
}
