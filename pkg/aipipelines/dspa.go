package aipipelines

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// ListDSPAs returns DSPA objects, preferring v1 and falling back to v1alpha1 when v1 is unavailable.
// The returned ResourceType identifies which API version was used for the list.
func ListDSPAs(
	ctx context.Context,
	r client.Reader,
) ([]*unstructured.Unstructured, resources.ResourceType, error) {
	dspasV1, err := r.List(ctx, resources.DataSciencePipelinesApplicationV1)
	if err == nil {
		return dspasV1, resources.DataSciencePipelinesApplicationV1, nil
	}

	if !apierrors.IsNotFound(err) {
		return nil, resources.ResourceType{}, fmt.Errorf("listing DataSciencePipelinesApplications v1: %w", err)
	}

	dspasV1Alpha1, err := r.List(ctx, resources.DataSciencePipelinesApplicationV1Alpha1)
	if err != nil {
		return nil, resources.ResourceType{}, fmt.Errorf("listing DataSciencePipelinesApplications v1alpha1: %w", err)
	}

	return dspasV1Alpha1, resources.DataSciencePipelinesApplicationV1Alpha1, nil
}

// HasDSPAs reports whether any DSPA objects exist on the cluster.
func HasDSPAs(ctx context.Context, r client.Reader) (bool, error) {
	dspas, _, err := ListDSPAs(ctx, r)
	if err != nil {
		return false, err
	}

	return len(dspas) > 0, nil
}
