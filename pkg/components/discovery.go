package components

import (
	"context"
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

// DiscoverComponents dynamically reads all components from the DSC singleton.
// It iterates over .spec.components and extracts the managementState for each.
func DiscoverComponents(ctx context.Context, r client.Reader) ([]ComponentInfo, error) {
	dsc, err := client.GetDataScienceCluster(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return ExtractComponents(dsc)
}

// ExtractComponents extracts component information from a DSC object.
func ExtractComponents(dsc *unstructured.Unstructured) ([]ComponentInfo, error) {
	componentsMap, err := jq.Query[map[string]any](dsc, ".spec.components")
	if err != nil {
		return nil, fmt.Errorf("querying spec.components: %w", err)
	}

	if componentsMap == nil {
		return []ComponentInfo{}, nil
	}

	components := make([]ComponentInfo, 0, len(componentsMap))

	for name := range componentsMap {
		state := getManagementState(dsc, name)

		components = append(components, ComponentInfo{
			Name:            name,
			ManagementState: state,
		})
	}

	sort.Slice(components, func(i, j int) bool {
		return components[i].Name < components[j].Name
	})

	return components, nil
}

// GetComponent returns information for a specific component from the DSC.
func GetComponent(ctx context.Context, r client.Reader, name string) (*ComponentInfo, error) {
	dsc, err := client.GetDataScienceCluster(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return GetComponentFromDSC(dsc, name)
}

// GetComponentFromDSC extracts component information from an already-fetched DSC.
func GetComponentFromDSC(dsc *unstructured.Unstructured, name string) (*ComponentInfo, error) {
	componentsMap, err := jq.Query[map[string]any](dsc, ".spec.components")
	if err != nil {
		return nil, fmt.Errorf("querying spec.components: %w", err)
	}

	if _, exists := componentsMap[name]; !exists {
		return nil, ErrComponentNotFound(name, sortedKeys(componentsMap))
	}

	state := getManagementState(dsc, name)

	return &ComponentInfo{
		Name:            name,
		ManagementState: state,
	}, nil
}

// sortedKeys returns sorted keys from a map.
func sortedKeys(m map[string]any) []string {
	if m == nil {
		return nil
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// getManagementState retrieves the managementState for a component.
// Returns "Removed" if the component or state is not configured.
func getManagementState(dsc *unstructured.Unstructured, componentName string) string {
	state, found, err := unstructured.NestedString(dsc.Object, "spec", "components", componentName, "managementState")
	if err != nil || !found || state == "" {
		return constants.ManagementStateRemoved
	}

	return state
}
