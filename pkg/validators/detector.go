package validators

import (
	"strings"

	"github.com/pterm/pterm"
)

type Detector struct {
	index         *DetectorIndex
	minScoreByKey map[string]int // provider name -> configured MinScore; 0/absent = no threshold
}

func NewDetectorFromConfigs(configs []ProviderConfig) *Detector {
	d := &Detector{
		index:         BuildDetectionIndex(configs),
		minScoreByKey: make(map[string]int, len(configs)),
	}
	for _, c := range configs {
		if c.Detection != nil && c.Detection.MinScore > 0 {
			d.minScoreByKey[c.Name] = c.Detection.MinScore
		}
	}
	return d
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
