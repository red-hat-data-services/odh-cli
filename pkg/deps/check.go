package deps

import (
	"context"
	"errors"
	"fmt"
	"sort"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/output"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

const (
	msgOLMNotAvailable = "OLM (Operator Lifecycle Manager) is not available in this cluster; cannot check operator dependencies"
)

// ErrOLMNotAvailable is returned when OLM is not installed in the cluster.
var ErrOLMNotAvailable = errors.New(msgOLMNotAvailable)

// Status represents the installation status of a dependency.
type Status string

const (
	StatusInstalled Status = "installed"
	StatusMissing   Status = "missing"
	StatusOptional  Status = "optional"
	StatusUnknown   Status = "unknown"
)

// DependencyStatus represents the checked status of a dependency on the cluster.
type DependencyStatus struct {
	Name         string   `json:"name"                 jsonschema:"description=Operator package name"                                                      yaml:"name"`
	DisplayName  string   `json:"displayName"          jsonschema:"description=Human-readable operator name"                                               yaml:"displayName"`
	Status       Status   `json:"status"               jsonschema:"description=Installation status,enum=installed,enum=missing,enum=optional,enum=unknown" yaml:"status"`
	Version      string   `json:"version,omitempty"    jsonschema:"description=Installed operator version"                                                 yaml:"version,omitempty"`
	Namespace    string   `json:"namespace"            jsonschema:"description=Operator namespace"                                                         yaml:"namespace"`
	Subscription string   `json:"subscription"         jsonschema:"description=OLM subscription name"                                                      yaml:"subscription"`
	RequiredBy   []string `json:"requiredBy,omitempty" jsonschema:"description=Components that require this dependency"                                    yaml:"requiredBy,omitempty"`
	Error        string   `json:"error,omitempty"      jsonschema:"description=Error message if status check failed"                                       yaml:"error,omitempty"`
}

// DependencyList wraps dependency statuses with a self-describing envelope.
type DependencyList struct {
	output.Envelope

	Dependencies []DependencyStatus `json:"dependencies" yaml:"dependencies"`
}

// NewDependencyList creates a new DependencyList with envelope fields populated.
func NewDependencyList(statuses []DependencyStatus) *DependencyList {
	list := &DependencyList{
		Envelope:     output.NewEnvelope("DependencyList", "deps"),
		Dependencies: statuses,
	}
	list.computeStatus()

	return list
}

// computeStatus calculates the Status based on Dependencies.
func (l *DependencyList) computeStatus() {
	var warnings, errs int
	for _, d := range l.Dependencies {
		// Check for per-dependency errors first
		if d.Error != "" {
			errs++

			continue
		}

		switch d.Status {
		case StatusMissing:
			errs++
		case StatusUnknown:
			warnings++
		case StatusInstalled, StatusOptional:
			// No action needed for installed or optional dependencies.
		}
	}
	l.SetStatus(warnings, errs)
}

// CheckDependencies queries the cluster for dependency installation status.
func CheckDependencies(ctx context.Context, olmReader client.OLMReader, manifest *Manifest) ([]DependencyStatus, error) {
	if !olmReader.Available() {
		return nil, ErrOLMNotAvailable
	}

	deps := manifest.GetDependencies()
	results := make([]DependencyStatus, 0, len(deps))

	for _, dep := range deps {
		status := checkSingleDependency(ctx, olmReader, dep)
		results = append(results, status)
	}

	// Sort by name for consistent output
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})

	return results, nil
}

func checkSingleDependency(ctx context.Context, olmReader client.OLMReader, dep DependencyInfo) DependencyStatus {
	status := DependencyStatus{
		Name:         dep.Name,
		DisplayName:  dep.DisplayName,
		Namespace:    dep.Namespace,
		Subscription: dep.Subscription,
		RequiredBy:   dep.RequiredBy,
	}

	sub, err := getSubscription(ctx, olmReader, dep.Namespace, dep.Subscription)
	if err != nil {
		status.Status = StatusUnknown
		status.Error = err.Error()

		return status
	}

	if sub == nil {
		// Not installed - check if optional or required
		if dep.Enabled == "auto" || dep.Enabled == "false" {
			status.Status = StatusOptional
		} else {
			status.Status = StatusMissing
		}

		return status
	}

	status.Status = StatusInstalled

	version, err := getVersionFromCSV(ctx, olmReader, dep.Namespace, sub.Status.InstalledCSV)
	if err != nil {
		status.Error = err.Error()
	}

	// Fallback: if InstalledCSV is empty (not error), search for matching CSV in namespace
	if err == nil && version == "" {
		var fallbackErr error

		version, fallbackErr = findMatchingCSVVersion(ctx, olmReader, dep.Namespace, dep.Subscription)
		if fallbackErr != nil && status.Error == "" {
			status.Error = fallbackErr.Error()
		}
	}

	status.Version = version

	return status
}

func getVersionFromCSV(ctx context.Context, olmReader client.OLMReader, namespace, csvName string) (string, error) {
	if csvName == "" {
		return "", nil
	}

	csv, err := getCSV(ctx, olmReader, namespace, csvName)
	if err != nil {
		return "", err
	}

	if csv == nil {
		return "", nil
	}

	return csv.Spec.Version.String(), nil
}

// getSubscription retrieves an OLM subscription by namespace and name.
// Returns (nil, nil) if not found, (nil, err) for other API errors.
func getSubscription(ctx context.Context, olm client.OLMReader, namespace, name string) (*operatorsv1alpha1.Subscription, error) {
	if namespace == "" || name == "" {
		return nil, nil
	}

	sub, err := olm.Subscriptions(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("get subscription %s/%s: %w", namespace, name, err)
	}

	return sub, nil
}

// getCSV retrieves a ClusterServiceVersion by namespace and name.
// Returns (nil, nil) if not found, (nil, err) for other API errors.
func getCSV(ctx context.Context, olm client.OLMReader, namespace, name string) (*operatorsv1alpha1.ClusterServiceVersion, error) {
	if namespace == "" || name == "" {
		return nil, nil
	}

	csv, err := olm.ClusterServiceVersions(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("get csv %s/%s: %w", namespace, name, err)
	}

	return csv, nil
}

// findMatchingCSVVersion searches for a Succeeded CSV matching the subscription name.
// Used as fallback when subscription's InstalledCSV is empty (e.g., OLM resolution issues).
func findMatchingCSVVersion(ctx context.Context, olm client.OLMReader, namespace, subName string) (string, error) {
	csvList, err := olm.ClusterServiceVersions(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("list CSVs in %s: %w", namespace, err)
	}

	for i := range csvList.Items {
		csv := &csvList.Items[i]

		if csv.Status.Phase != operatorsv1alpha1.CSVPhaseSucceeded {
			continue
		}

		if MatchesSubscription(csv.Name, subName) {
			return csv.Spec.Version.String(), nil
		}
	}

	return "", nil
}
