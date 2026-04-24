package deps

import (
	_ "embed"
)

//go:embed data/values.yaml
var embeddedManifest []byte

//go:embed data/Chart.yaml
var embeddedChart []byte

// EmbeddedManifest returns the embedded values.yaml content.
func EmbeddedManifest() []byte {
	return embeddedManifest
}

// chartMetadata represents the relevant fields from Chart.yaml.
type chartMetadata struct {
	AppVersion string `yaml:"appVersion"`
}

// ManifestVersion returns the appVersion from the embedded Chart.yaml.
func ManifestVersion() (string, error) {
	return parseChartVersion(embeddedChart)
}
