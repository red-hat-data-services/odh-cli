package deps

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	msgParseManifest = "parse manifest: %w"
)

//nolint:gochecknoglobals // static lookup table for display names
var displayNames = map[string]string{
	// Dependencies
	"certManager":             "Cert Manager",
	"leaderWorkerSet":         "Leader Worker Set",
	"jobSet":                  "Job Set",
	"rhcl":                    "Red Hat Connectivity Link",
	"customMetricsAutoscaler": "Custom Metrics Autoscaler",
	"serviceMesh":             "Service Mesh",
	"serverless":              "Serverless",
	"authorino":               "Authorino",
	"kueue":                   "Kueue",
	"opentelemetry":           "OpenTelemetry",
	"tempo":                   "Tempo",
	"clusterObservability":    "Cluster Observability",
	"nfd":                     "Node Feature Discovery",
	"nvidiaGPUOperator":       "NVIDIA GPU Operator",
	// Components
	"aipipelines":        "AI Pipelines",
	"dashboard":          "Dashboard",
	"feastoperator":      "Feast Operator",
	"kserve":             "KServe",
	"modelregistry":      "Model Registry",
	"ray":                "Ray",
	"trainer":            "Trainer",
	"trainingoperator":   "Training Operator",
	"trustyai":           "TrustyAI",
	"workbenches":        "Workbenches",
	"mlflowoperator":     "MLflow Operator",
	"llamastackoperator": "LlamaStack Operator",
	"sparkoperator":      "Spark Operator",
}

// Manifest represents the parsed values.yaml structure from odh-gitops.
type Manifest struct {
	Dependencies map[string]Dependency `yaml:"dependencies"`
	Components   map[string]Component  `yaml:"components"`
}

// Dependency represents an operator dependency configuration.
type Dependency struct {
	Enabled      string         `yaml:"enabled"` // "auto", "true", "false"
	OLM          OLMConfig      `yaml:"olm"`
	Dependencies map[string]any `yaml:"dependencies"` // Transitive dependencies
}

// OLMConfig contains OLM subscription details.
type OLMConfig struct {
	Channel   string `yaml:"channel"`
	Name      string `yaml:"name"`      // Subscription name
	Namespace string `yaml:"namespace"` // Operator namespace
	Source    string `yaml:"source"`    // Catalog source (optional, defaults to redhat-operators)
}

// Component represents an ODH/RHOAI component configuration.
type Component struct {
	Dependencies map[string]any `yaml:"dependencies"`
}

// DependencyInfo is a flattened view of a dependency for display.
type DependencyInfo struct {
	Name         string   `json:"name"                 yaml:"name"`
	DisplayName  string   `json:"displayName"          yaml:"displayName"`
	Enabled      string   `json:"enabled"              yaml:"enabled"`
	Subscription string   `json:"subscription"         yaml:"subscription"`
	Namespace    string   `json:"namespace"            yaml:"namespace"`
	Channel      string   `json:"channel,omitempty"    yaml:"channel,omitempty"`
	RequiredBy   []string `json:"requiredBy,omitempty" yaml:"requiredBy,omitempty"`
}

// Parse parses values.yaml content into a Manifest.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf(msgParseManifest, err)
	}

	return &m, nil
}

// GetDependencies returns a flat list of dependencies with their metadata, sorted by name.
func (m *Manifest) GetDependencies() []DependencyInfo {
	requiredBy := m.buildRequiredByMap()

	deps := make([]DependencyInfo, 0, len(m.Dependencies))
	for name, dep := range m.Dependencies {
		info := DependencyInfo{
			Name:         name,
			DisplayName:  toDisplayName(name),
			Enabled:      dep.Enabled,
			Subscription: dep.OLM.Name,
			Namespace:    dep.OLM.Namespace,
			Channel:      dep.OLM.Channel,
			RequiredBy:   requiredBy[name],
		}
		deps = append(deps, info)
	}

	sort.Slice(deps, func(i, j int) bool {
		return deps[i].Name < deps[j].Name
	})

	return deps
}

// buildRequiredByMap builds a reverse map of dependency -> components that need it.
func (m *Manifest) buildRequiredByMap() map[string][]string {
	depSets := make(map[string]sets.Set[string])

	addEntry := func(depName, requiredBy string) {
		if depSets[depName] == nil {
			depSets[depName] = sets.New[string]()
		}

		depSets[depName].Insert(requiredBy)
	}

	// From components
	for compName, comp := range m.Components {
		for depName, val := range comp.Dependencies {
			if isEnabled(val) {
				addEntry(depName, toDisplayName(compName))
			}
		}
	}

	// From transitive dependencies
	for depName, dep := range m.Dependencies {
		for transDepName, val := range dep.Dependencies {
			if isEnabled(val) {
				addEntry(transDepName, toDisplayName(depName))
			}
		}
	}

	// Convert sets to sorted slices
	result := make(map[string][]string, len(depSets))
	for depName, set := range depSets {
		result[depName] = sets.List(set)
	}

	return result
}

// isEnabled checks if a dependency value indicates it's enabled.
// Only bool true or strings "true"/"auto" are considered enabled.
// All other types (int, map, slice, etc.) return false.
func isEnabled(val any) bool {
	switch v := val.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "auto"
	default:
		return false
	}
}

// toDisplayName converts camelCase to human-readable name.
func toDisplayName(name string) string {
	if display, ok := displayNames[name]; ok {
		return display
	}

	return name
}
