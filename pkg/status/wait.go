package status

import (
	"context"
	"fmt"
	"time"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"

	"github.com/opendatahub-io/odh-cli/pkg/deps"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// runWaitFor polls clusterhealth.Run() until the specified condition is met.
func (c *Command) runWaitFor(ctx context.Context) error {
	hc, err := c.resolveHealthConfig(ctx)
	if err != nil {
		return err
	}

	var lastReport *clusterhealth.Report
	var lastDepStatuses []deps.DependencyStatus

	start := time.Now()

	err = c.RunWait(ctx, c.Timeout, func(ctx context.Context) (bool, error) {
		report, depStatuses, err := c.runHealthCheck(ctx, hc)
		if err != nil {
			if client.IsUnrecoverableError(err) {
				return false, err
			}

			_, _ = fmt.Fprintf(c.IO.ErrOut(), msgHealthCheckRetry, err)

			return false, nil
		}

		lastReport = report
		lastDepStatuses = depStatuses

		if report == nil {
			return false, nil
		}

		healthy := report.Healthy()

		if !healthy {
			elapsed := time.Since(start).Truncate(time.Second)
			_, _ = fmt.Fprintf(c.IO.ErrOut(), msgWaitProgress, c.WaitFor, elapsed)
		}

		return healthy, nil
	})

	if err != nil {
		return err //nolint:wrapcheck // structured wait errors propagate as-is
	}

	return c.output(context.WithoutCancel(ctx), lastReport, lastDepStatuses)
}
