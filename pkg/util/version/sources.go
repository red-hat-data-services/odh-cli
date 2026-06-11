package version

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube/discovery"
)

// getSingletonWithDiscovery dynamically detects the available API version and retrieves
// a singleton resource. Tries v2 first, falls back to v1.
// This avoids code duplication for resources that exist in both v1 and v2 API versions.
func getSingletonWithDiscovery(
	ctx context.Context,
	c client.Client,
	v2Resource, v1Resource resources.ResourceType,
) (*unstructured.Unstructured, error) {
	// Try v2 first if Discovery is available
	obj, fallbackToV1, err := tryGetV2Singleton(ctx, c, v2Resource)
	if err != nil {
		return nil, fmt.Errorf("getting %s (v2): %w", v2Resource.Kind, err)
	}

	if !fallbackToV1 {
		return obj, nil
	}

	// Fallback to v1 (ODH/RHOAI 2.x) - used when:
	// - Discovery is not available
	// - v2 API doesn't exist on cluster
	// - v2 API exists but no v2 instance found (mid-upgrade scenario)
	obj, err = client.GetSingleton(ctx, c, v1Resource)
	if err != nil {
		return nil, fmt.Errorf("getting %s (v1): %w", v1Resource.Kind, err)
	}

	return obj, nil
}

// tryGetV2Singleton attempts to get the v2 singleton if the v2 API exists.
// Returns:
//   - (obj, false, nil) if v2 singleton found
//   - (nil, true, nil) if should fall back to v1 (v2 API absent or v2 instance not found)
//   - (nil, false, err) if a real error occurred (403, timeout, unexpected count, etc.)
func tryGetV2Singleton(
	ctx context.Context,
	c client.Client,
	v2Resource resources.ResourceType,
) (*unstructured.Unstructured, bool, error) {
	if c.Discovery() == nil {
		return nil, true, nil // No discovery available, fall back to v1
	}

	v2Resources, err := discovery.GetGroupVersionResources(
		c.Discovery(),
		schema.GroupVersion{Group: v2Resource.Group, Version: "v2"},
	)
	if err != nil {
		// Discovery error is not fatal - fall back to v1
		return nil, true, nil
	}

	if len(v2Resources) == 0 {
		return nil, true, nil // v2 API doesn't exist, fall back to v1
	}

	obj, err := client.GetSingleton(ctx, c, v2Resource)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// v2 instance not found (mid-upgrade scenario) - fall back to v1
			return nil, true, nil
		}
		// Real error (403, timeout, "expected single, found 2", etc.) - propagate
		return nil, false, fmt.Errorf("getting v2 singleton: %w", err)
	}

	return obj, false, nil
}

// DetectFromDataScienceCluster attempts to detect version from DataScienceCluster resource.
// Dynamically detects whether to use v1 or v2 API based on cluster capabilities.
// Returns version string and true if found, empty string and false otherwise.
func DetectFromDataScienceCluster(ctx context.Context, c client.Client) (string, bool, error) {
	// Get the DataScienceCluster singleton using discovery-based version detection
	dsc, err := getSingletonWithDiscovery(ctx, c, resources.DataScienceCluster, resources.DataScienceClusterV1)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	// Query .status.release.version using JQ
	versionStr, err := jq.Query[string](dsc, ".status.release.version")
	if err != nil {
		return "", false, fmt.Errorf("querying .status.release.version: %w", err)
	}

	if versionStr == "" {
		return "", false, nil
	}

	return versionStr, true, nil
}

// DetectFromDSCInitialization attempts to detect version from DSCInitialization resource.
// Dynamically detects whether to use v1 or v2 API based on cluster capabilities.
// Returns version string and true if found, empty string and false otherwise.
func DetectFromDSCInitialization(ctx context.Context, c client.Client) (string, bool, error) {
	// Get the DSCInitialization singleton using discovery-based version detection
	dsci, err := getSingletonWithDiscovery(ctx, c, resources.DSCInitialization, resources.DSCInitializationV1)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("getting DSCInitialization: %w", err)
	}

	// Query .status.release.version using JQ
	versionStr, err := jq.Query[string](dsci, ".status.release.version")
	if err != nil {
		return "", false, fmt.Errorf("querying .status.release.version: %w", err)
	}

	if versionStr == "" {
		return "", false, nil
	}

	return versionStr, true, nil
}

// DetectFromOLM attempts to detect version from OLM ClusterServiceVersion
// Returns version string and true if found, empty string and false otherwise.
func DetectFromOLM(ctx context.Context, c client.Reader) (string, bool, error) {
	// Check if OLM client is available
	if !c.OLM().Available() {
		return "", false, nil
	}

	// List ClusterServiceVersions with label selector for OpenShift AI operator
	csvList, err := c.OLM().ClusterServiceVersions("").List(ctx, metav1.ListOptions{
		LabelSelector: "operators.coreos.com/rhods-operator.redhat-ods-operator",
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("listing ClusterServiceVersion: %w", err)
	}

	if len(csvList.Items) == 0 {
		return "", false, nil
	}

	// Use the first CSV found
	csv := &csvList.Items[0]

	// Access .spec.version directly
	versionStr := csv.Spec.Version.String()

	if versionStr == "" {
		return "", false, nil
	}

	return versionStr, true, nil
}
