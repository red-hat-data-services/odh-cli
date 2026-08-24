package aipipelines

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	// ComponentKeyV1 is the DSC v1 component key for DataSciencePipelines.
	ComponentKeyV1 = "datasciencepipelines"
	// ComponentKeyV2 is the DSC v2 component key for AI Pipelines.
	ComponentKeyV2 = "aipipelines"
)

// ShouldApplyChecks reports whether AI Pipelines upgrade checks should run.
// Checks apply when the component is Managed under either DSC key, or when DSPAs
// exist on the cluster (for example, component Removed but workloads remain).
func ShouldApplyChecks(ctx context.Context, dsc *unstructured.Unstructured, r client.Reader) (bool, error) {
	if components.HasManagementState(dsc, ComponentKeyV2, constants.ManagementStateManaged) ||
		components.HasManagementState(dsc, ComponentKeyV1, constants.ManagementStateManaged) {
		return true, nil
	}

	exists, err := HasDSPAs(ctx, r)
	if err != nil {
		return false, fmt.Errorf("checking for DataSciencePipelinesApplications: %w", err)
	}

	return exists, nil
}

// HasComponentKey reports whether componentKey is present under spec.components.
func HasComponentKey(dsc *unstructured.Unstructured, componentKey string) (bool, error) {
	path := ".spec.components." + componentKey

	_, err := jq.Query[any](dsc, path)
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			return false, nil
		}

		return false, fmt.Errorf("querying %s: %w", path, err)
	}

	return true, nil
}

// UsesComponentKeyV2 reports whether AI Pipelines is configured under the DSC v2 component key.
func UsesComponentKeyV2(dsc *unstructured.Unstructured) (bool, error) {
	return HasComponentKey(dsc, ComponentKeyV2)
}

// EffectiveManagementState returns the management state for AI Pipelines, preferring the
// DSC v2 component key when present and falling back to the v1 key.
func EffectiveManagementState(dsc *unstructured.Unstructured) (string, error) {
	usesV2, err := UsesComponentKeyV2(dsc)
	if err != nil {
		return "", fmt.Errorf("checking AI Pipelines v2 component key: %w", err)
	}

	if usesV2 {
		state, err := components.GetManagementState(dsc, ComponentKeyV2)
		if err != nil {
			return "", fmt.Errorf("querying %s management state: %w", ComponentKeyV2, err)
		}

		return state, nil
	}

	state, err := components.GetManagementState(dsc, ComponentKeyV1)
	if err != nil {
		return "", fmt.Errorf("querying %s management state: %w", ComponentKeyV1, err)
	}

	return state, nil
}
