package status

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	// Well-known operator namespace defaults.
	defaultRHOAIOperatorNamespace = "redhat-ods-operator"
	defaultODHOperatorNamespace   = "opendatahub"
	defaultOpenShiftOperatorsNS   = "openshift-operators"
)

// operatorInfo holds operator namespace and deployment name discovered from OLM.
type operatorInfo struct {
	Namespace      string
	DeploymentName string
}

// namespaceConfig holds the three namespaces required by clusterhealth.Config.
type namespaceConfig struct {
	Apps       string
	Operator   string
	Monitoring string
}

// discoverNamespaces resolves the apps, operator, and monitoring namespaces.
// Flag overrides take priority over auto-detection from cluster resources.
// If dsci is provided, it will be reused instead of fetching again.
// Also returns operatorInfo for reuse in operator name discovery.
func discoverNamespaces(
	ctx context.Context,
	c client.Client,
	dsci *unstructured.Unstructured,
	appsNSOverride string,
	operatorNSOverride string,
) (*namespaceConfig, *operatorInfo, error) {
	cfg := &namespaceConfig{}

	appsNS, err := discoverAppsNamespace(dsci, appsNSOverride)
	if err != nil {
		return nil, nil, err
	}

	cfg.Apps = appsNS

	// Fetch operator info from OLM once, reuse for namespace and name discovery
	opInfo := discoverOperatorFromOLM(ctx, c)

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
func discoverOperatorNamespace(ctx context.Context, c client.Reader, info *operatorInfo, override string) (string, error) {
	if override != "" {
		return override, nil
	}

	if info != nil && info.Namespace != "" {
		return info.Namespace, nil
	}

	return discoverOperatorNamespaceFromDefaults(ctx, c)
}

// discoverOperatorFromOLM searches for the operator CSV across all namespaces.
// Finds CSVs by name prefix (rhods-operator or opendatahub-operator) and returns
// both the namespace and deployment name from a single API call.
func discoverOperatorFromOLM(ctx context.Context, c client.Reader) *operatorInfo {
	if !c.OLM().Available() {
		return nil
	}

	csvList, err := c.OLM().ClusterServiceVersions("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}

	for _, csv := range csvList.Items {
		name := csv.GetName()
		if strings.HasPrefix(name, "rhods-operator.") || strings.HasPrefix(name, "opendatahub-operator.") {
			info := &operatorInfo{}

			// Get namespace - use original namespace from olm.copiedFrom if this is a copy
			if copiedFrom, ok := csv.GetLabels()["olm.copiedFrom"]; ok && copiedFrom != "" {
				info.Namespace = copiedFrom
			} else {
				info.Namespace = csv.GetNamespace()
			}

			// Get deployment name from install strategy
			deployments := csv.Spec.InstallStrategy.StrategySpec.DeploymentSpecs
			if len(deployments) > 0 {
				info.DeploymentName = deployments[0].Name
			}

			return info
		}
	}

	return nil
}

// discoverOperatorNamespaceFromDefaults tries well-known operator namespaces
// by checking if the specific operator deployment exists there.
func discoverOperatorNamespaceFromDefaults(ctx context.Context, c client.Reader) (string, error) {
	operatorDeploymentNames := []string{
		"rhods-operator",
		"opendatahub-operator-controller-manager",
	}

	for _, ns := range []string{defaultRHOAIOperatorNamespace, defaultODHOperatorNamespace, defaultOpenShiftOperatorsNS} {
		for _, name := range operatorDeploymentNames {
			_, err := c.GetResource(ctx, resources.Deployment, name, client.InNamespace(ns))
			if err == nil {
				return ns, nil
			}
		}
	}

	return "", ErrOperatorNamespaceNotFound()
}

// discoverOperatorName returns the operator deployment name.
// Uses the override if set, otherwise uses pre-fetched info or falls back to defaults.
func discoverOperatorName(info *operatorInfo, override string) string {
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
