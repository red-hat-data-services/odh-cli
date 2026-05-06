package client

import (
	"fmt"

	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// NewControllerRuntimeClient creates a controller-runtime client from a REST config.
// Used by the status command to satisfy clusterhealth.Config.Client, which requires
// sigs.k8s.io/controller-runtime/pkg/client.Client rather than the CLI's dynamic client.
// The scheme registers core (Pods, Nodes, Events, ResourceQuotas, etc.) and apps
// (Deployments) API groups — sufficient for all clusterhealth section runners.
// DSCI/DSC sections use unstructured lookups internally, so no ODH types are needed.
func NewControllerRuntimeClient(restConfig *rest.Config) (crclient.Client, error) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add client-go scheme: %w", err)
	}

	c, err := crclient.New(restConfig, crclient.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create controller-runtime client: %w", err)
	}

	return c, nil
}
