package status

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

// namespaceConfig holds the three namespaces required by clusterhealth.Config.
type namespaceConfig struct {
	Apps       string
	Operator   string
	Monitoring string
}

// discoverNamespaces resolves the apps, operator, and monitoring namespaces.
// Flag overrides take priority over auto-detection from cluster resources.
// If dsci is provided, it will be reused instead of fetching again.
// Also returns OperatorInfo for reuse in operator name discovery.
func discoverNamespaces(
	ctx context.Context,
	c client.Client,
	dsci *unstructured.Unstructured,
	appsNSOverride string,
	operatorNSOverride string,
) (*namespaceConfig, *client.OperatorInfo, error) {
	cfg := &namespaceConfig{}

	appsNS, err := discoverAppsNamespace(dsci, appsNSOverride)
	if err != nil {
		return nil, nil, err
	}

	cfg.Apps = appsNS

	// Fetch operator info from OLM once, reuse for namespace and name discovery.
	// When operatorNSOverride is set, make OLM lookup best-effort so users with
	// explicit --operator-namespace don't need cluster-wide CSV list RBAC.
	var opInfo *client.OperatorInfo

	if operatorNSOverride == "" {
		var olmErr error

		opInfo, olmErr = client.DiscoverOperatorFromOLM(ctx, c)
		if olmErr != nil {
			return nil, nil, fmt.Errorf("discovering operator from OLM: %w", olmErr)
		}
	} else {
		// Best-effort when override is set - ignore errors
		opInfo, _ = client.DiscoverOperatorFromOLM(ctx, c)
	}

	operatorNS, err := discoverOperatorNamespace(ctx, c, opInfo, operatorNSOverride)
	if err != nil {
		return nil, nil, err
	}

	cfg.Operator = operatorNS

	cfg.Monitoring = discoverMonitoringNamespace(dsci, appsNS)

	return cfg, opInfo, nil
}

// discoverAppsNamespace returns the applications namespace.
// Uses the flag override if set, otherwise reads from the provided DSCI.
func discoverAppsNamespace(dsci *unstructured.Unstructured, override string) (string, error) {
	if override != "" {
		return override, nil
	}

	if dsci == nil {
		return "", ErrNoDSCIFound()
	}

	ns, err := jq.Query[string](dsci, ".spec.applicationsNamespace")
	if err != nil {
		return "", fmt.Errorf("querying applicationsNamespace from DSCI: %w", err)
	}

	return ns, nil
}

// discoverOperatorNamespace returns the operator namespace.
// Uses the flag override if set, otherwise uses pre-fetched info from OLM,
// falling back to well-known defaults.
func discoverOperatorNamespace(ctx context.Context, c client.Reader, info *client.OperatorInfo, override string) (string, error) {
	if override != "" {
		return override, nil
	}

	ns, err := client.DiscoverOperatorNamespaceWithInfo(ctx, c, info)
	if err != nil {
		return "", fmt.Errorf("discovering operator namespace: %w", err)
	}

	return ns, nil
}

// discoverOperatorName returns the operator deployment name.
// Uses the override if set, otherwise uses pre-fetched info or falls back to defaults.
func discoverOperatorName(info *client.OperatorInfo, override string) string {
	if override != "" {
		return override
	}

	if info != nil && info.DeploymentName != "" {
		return info.DeploymentName
	}

	return defaultRHOAIOperatorName
}

// discoverMonitoringNamespace returns the monitoring namespace.
// Reads from DSCI spec.monitoring.namespace, defaults to the apps namespace.
func discoverMonitoringNamespace(dsci *unstructured.Unstructured, appsNS string) string {
	if dsci == nil {
		return appsNS
	}

	ns, err := jq.Query[string](dsci, ".spec.monitoring.namespace")
	if err != nil || ns == "" {
		return appsNS
	}

	return ns
}
