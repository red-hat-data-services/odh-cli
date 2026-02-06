package client

import (
	"fmt"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
)

const (
	// DefaultQPS is the default queries per second for CLI client.
	// This is significantly higher than kubectl's default (5) to support
	// parallel operations like backup with multiple concurrent workers.
	DefaultQPS = 50

	// DefaultBurst is the default burst capacity for CLI client.
	// This is significantly higher than kubectl's default (10) to handle
	// initial spikes when all workers start simultaneously.
	DefaultBurst = 100
)

// ConfigureThrottling configures QPS and Burst on a REST config.
// These settings control client-side rate limiting for Kubernetes API requests.
//
// QPS (Queries Per Second): Sustained rate of API requests allowed.
// Burst: Maximum number of requests that can be issued in a short burst.
//
// For CLI tools with parallel operations (like backup), higher values
// are recommended to avoid unnecessary throttling delays.
func ConfigureThrottling(config *rest.Config, qps float32, burst int) {
	config.QPS = qps
	config.Burst = burst
}

// NewRESTConfig creates a REST config with appropriate throttling for CLI usage.
// The QPS and Burst parameters allow callers to customize throttling settings.
// Use DefaultQPS and DefaultBurst for standard parallel operations.
func NewRESTConfig(
	configFlags *genericclioptions.ConfigFlags,
	qps float32,
	burst int,
) (*rest.Config, error) {
	restConfig, err := configFlags.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create REST config: %w", err)
	}

	ConfigureThrottling(restConfig, qps, burst)

	// Suppress Kubernetes API server deprecation warnings from cluttering CLI output.
	restConfig.WarningHandler = rest.NoWarnings{}

	return restConfig, nil
}
