package validators

import (
	"strings"

	"github.com/pterm/pterm"
)

type Detector struct {
	index *DetectorIndex
}

func NewDetectorFromConfigs(configs []ProviderConfig) *Detector {
	return &Detector{
		index: BuildDetectionIndex(configs),
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
	res := d.DetectProviderWithSuggestion(apiKey, manualCategory)
	return res.Provider
}

func (d *Detector) DetectProviderBytes(apiKey []byte, manualCategory string) string {
	res := d.DetectProviderWithSuggestion(string(apiKey), manualCategory)
	return res.Provider
}

func (d *Detector) DetectProviderWithSuggestion(apiKey string, manualCategory string) DetectionResult {
	return DetectProviderWithSuggestion(strings.TrimSpace(apiKey), d.index, manualCategory)
}
