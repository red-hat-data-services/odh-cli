package status

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"
	"golang.org/x/sync/errgroup"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/opendatahub-io/odh-cli/pkg/deps"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// resolveHealthConfig returns healthConfig if pre-set (testing), otherwise
// builds one from the live cluster.
func (c *Command) resolveHealthConfig(ctx context.Context) (*clusterhealth.Config, error) {
	if c.healthConfig != nil {
		return c.healthConfig, nil
	}

	return c.buildHealthConfig(ctx)
}

// buildHealthConfig performs one-time setup: DSCI fetch, namespace discovery,
// CR name discovery, and clusterhealth config construction.
func (c *Command) buildHealthConfig(ctx context.Context) (*clusterhealth.Config, error) {
	dsci, err := client.GetDSCInitialization(ctx, c.client)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("getting DSCInitialization: %w", err)
	}

	nsCfg, opInfo, err := discoverNamespaces(ctx, c.client, dsci, c.AppsNamespace, c.OperatorNamespace)
	if err != nil {
		return nil, fmt.Errorf("discovering namespaces: %w", err)
	}

	dsciName, dscName, err := discoverCRNames(ctx, c.client, dsci)
	if err != nil {
		return nil, err
	}

	crClient, err := client.NewControllerRuntimeClient(c.restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating controller-runtime client: %w", err)
	}

	operatorName := discoverOperatorName(opInfo, c.OperatorName)

	nsCfgHealth := clusterhealth.NamespaceConfig{
		Apps:       nsCfg.Apps,
		Monitoring: nsCfg.Monitoring,
	}

	if c.IncludeInfra {
		nsCfgHealth.Extra = []string{"kube-system"}
	}

	cfg := clusterhealth.Config{
		Client: crClient,
		Operator: clusterhealth.OperatorConfig{
			Namespace: nsCfg.Operator,
			Name:      operatorName,
		},
		Namespaces:   nsCfgHealth,
		DSCI:         dsciName,
		DSC:          dscName,
		OnlySections: c.Sections,
		Layers:       c.Layers,
	}

	return &cfg, nil
}

// runHealthCheck executes a single health check pass and optionally checks dependencies.
func (c *Command) runHealthCheck(ctx context.Context, hc *clusterhealth.Config) (*clusterhealth.Report, []deps.DependencyStatus, error) {
	var report *clusterhealth.Report
	var depStatuses []deps.DependencyStatus

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		report, err = clusterhealth.Run(gctx, *hc)

		return err //nolint:wrapcheck // only one goroutine can error; wrapping adds noise
	})

	if c.IncludeDeps {
		g.Go(func() error {
			depStatuses = c.checkDependencies(gctx)

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, nil, err //nolint:wrapcheck // single error source; pass through as-is
	}

	return report, depStatuses, nil
}
