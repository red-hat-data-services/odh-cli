package deps

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/opendatahub-io/odh-cli/pkg/output"
)

const (
	msgParseManifest = "parse manifest: %w"

	enabledTrue = "true"
	enabledAuto = "auto"
)

// allowedCatalogSources is the allow-list of trusted OLM catalog sources.
// This prevents supply-chain attacks via untrusted catalogs (CWE-829).
//
//nolint:gochecknoglobals // static security allow-list
var allowedCatalogSources = map[string]bool{
	"":                    true, // empty defaults to redhat-operators
	"redhat-operators":    true,
	"community-operators": true,
	"certified-operators": true,
	"redhat-marketplace":  true,
}

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
	Channel          string   `yaml:"channel"`
	Name             string   `yaml:"name"`             // Subscription name
	Namespace        string   `yaml:"namespace"`        // Operator namespace
	Source           string   `yaml:"source"`           // Catalog source (optional, defaults to redhat-operators)
	TargetNamespaces []string `yaml:"targetNamespaces"` // OperatorGroup target namespaces
}

// Component represents an ODH/RHOAI component configuration.
type Component struct {
	Dependencies map[string]any `yaml:"dependencies"`
}

// DependencyInfo is a flattened view of a dependency for display and installation.
type DependencyInfo struct {
	Name             string   `json:"name"                       jsonschema:"description=Dependency identifier"                yaml:"name"`
	DisplayName      string   `json:"displayName"                jsonschema:"description=Human-readable name"                  yaml:"displayName"`
	Enabled          string   `json:"enabled"                    jsonschema:"description=Enable state (auto/true/false)"       yaml:"enabled"`
	Subscription     string   `json:"subscription"               jsonschema:"description=OLM subscription name"                yaml:"subscription"`
	Namespace        string   `json:"namespace"                  jsonschema:"description=Operator namespace"                   yaml:"namespace"`
	Channel          string   `json:"channel,omitempty"          jsonschema:"description=OLM channel"                          yaml:"channel,omitempty"`
	Source           string   `json:"source,omitempty"           jsonschema:"description=Catalog source name"                  yaml:"source,omitempty"`
	TargetNamespaces []string `json:"targetNamespaces,omitempty" jsonschema:"description=Target namespaces for operator"       yaml:"targetNamespaces,omitempty"`
	RequiredBy       []string `json:"requiredBy,omitempty"       jsonschema:"description=Components requiring this dependency" yaml:"requiredBy,omitempty"`
}

// DependencyManifestList wraps dependency manifest info with a self-describing envelope.
// Used for dry-run output where cluster is not queried. Status is intentionally omitted
// because no cluster state is available to compute warnings/errors.
type DependencyManifestList struct {
	output.Envelope

	Dependencies []DependencyInfo `json:"dependencies" yaml:"dependencies"`
}

// NewDependencyManifestList creates a new DependencyManifestList with envelope fields populated.
func NewDependencyManifestList(deps []DependencyInfo) *DependencyManifestList {
	return &DependencyManifestList{
		Envelope:     output.NewEnvelope("DependencyManifestList", "deps"),
		Dependencies: deps,
	}
}

// Parse parses values.yaml content into a Manifest and validates security-sensitive fields.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf(msgParseManifest, err)
	}

	if err := m.validate(); err != nil {
		return nil, fmt.Errorf("validate manifest: %w", err)
	}

	return &m, nil
}

// validate checks security-sensitive fields in the manifest.
// This prevents supply-chain attacks via untrusted catalog sources (CWE-829)
// and privilege escalation via attacker-controlled target namespaces (CWE-269).
func (m *Manifest) validate() error {
	for name, dep := range m.Dependencies {
		if err := validateOLMConfig(name, dep.OLM); err != nil {
			return err
		}
	}

	return nil
}

// validateOLMConfig validates security-sensitive OLM configuration fields.
func validateOLMConfig(depName string, olm OLMConfig) error {
	// Validate catalog source against allow-list
	if !allowedCatalogSources[olm.Source] {
		return fmt.Errorf(
			"dependency %q: untrusted catalog source %q; allowed: redhat-operators, community-operators, certified-operators, redhat-marketplace",
			depName, olm.Source,
		)
	}

	// Validate target namespaces
	for _, ns := range olm.TargetNamespaces {
		if err := validateTargetNamespace(depName, ns, olm.Namespace); err != nil {
			return err
		}
	}

	return nil
}

// validateTargetNamespace validates a single target namespace entry.
func validateTargetNamespace(depName, ns, operatorNamespace string) error {
	// Validate as DNS-1123 label
	if errs := validation.IsDNS1123Label(ns); len(errs) > 0 {
		return fmt.Errorf(
			"dependency %q: invalid targetNamespace %q: %s",
			depName, ns, strings.Join(errs, ", "),
		)
	}

	// Reject system namespaces unless it's the operator's own namespace
	if ns == operatorNamespace {
		return nil
	}

	if strings.HasPrefix(ns, "kube-") || strings.HasPrefix(ns, "openshift-") {
		return fmt.Errorf(
			"dependency %q: targetNamespace %q is a system namespace; only the operator's own namespace %q is allowed",
			depName, ns, operatorNamespace,
		)
	}

	return nil
}

// GetDependencies returns a flat list of dependencies with their metadata, sorted by name.
func (m *Manifest) GetDependencies() []DependencyInfo {
	requiredBy := m.buildRequiredByMap()

	deps := make([]DependencyInfo, 0, len(m.Dependencies))
	for name, dep := range m.Dependencies {
		info := DependencyInfo{
			Name:             name,
			DisplayName:      toDisplayName(name),
			Enabled:          dep.Enabled,
			Subscription:     dep.OLM.Name,
			Namespace:        dep.OLM.Namespace,
			Channel:          dep.OLM.Channel,
			Source:           dep.OLM.Source,
			TargetNamespaces: dep.OLM.TargetNamespaces,
			RequiredBy:       requiredBy[name],
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
		return v == enabledTrue || v == enabledAuto
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
