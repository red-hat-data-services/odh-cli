package diagnose

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"
	"github.com/opendatahub-io/opendatahub-operator/pkg/failureclassifier"
	"golang.org/x/sync/errgroup"
)

// DefaultEventsSince is the default look-back window for recent events.
// Exported so cmd/diagnose can use it as the --events-since flag default.
const DefaultEventsSince = 5 * time.Minute

// Run executes the 4-step diagnostic flow and returns a Report.
func Run(ctx context.Context, cfg Config) (*Report, error) {
	eventsSince := cfg.EventsSince
	if eventsSince <= 0 {
		eventsSince = DefaultEventsSince
	}

	// Step 1 — Triage: full health check.
	health, err := clusterhealth.Run(ctx, clusterhealth.Config{
		Client:     cfg.Client,
		Operator:   clusterhealth.OperatorConfig{Namespace: cfg.OperatorNS, Name: cfg.OperatorName},
		Namespaces: clusterhealth.NamespaceConfig{Apps: cfg.AppsNS, Monitoring: cfg.AppsNS},
	})
	if err != nil {
		return nil, fmt.Errorf("health check: %w", err)
	}

	if health.Healthy() {
		return &Report{Healthy: true, Health: health}, nil
	}

	// Step 2 — Investigate: classify failure + collect recent events (concurrent).
	var (
		classification failureclassifier.FailureClassification
		events         []clusterhealth.EventInfo
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		classification = failureclassifier.Classify(health)

		return nil
	})

	g.Go(func() error {
		var evtErr error
		events, evtErr = clusterhealth.RunRecentEvents(gctx, clusterhealth.RecentEventsConfig{
			Client:     cfg.Client,
			Namespaces: clusterhealth.NamespaceConfig{Apps: cfg.AppsNS, Monitoring: cfg.AppsNS}.List(),
			Since:      eventsSince,
			EventType:  "Warning",
		})
		if evtErr != nil {
			return fmt.Errorf("recent events: %w", evtErr)
		}

		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("investigate: %w", err)
	}

	// Step 3 — Correlate: per-component status.
	// Step 4 — Assemble report.
	return &Report{
		Health:         health,
		Classification: &classification,
		Components:     correlate(ctx, cfg),
		Events:         events,
	}, nil
}

// correlate fetches component status for each relevant component.
// Errors per component are embedded in the result rather than aborting the run.
func correlate(ctx context.Context, cfg Config) []*clusterhealth.ComponentStatusResult {
	names := make([]string, 0, len(clusterhealth.KnownComponents))
	if cfg.TargetComponent != "" {
		names = append(names, cfg.TargetComponent)
	} else {
		for name := range clusterhealth.KnownComponents {
			names = append(names, name)
		}
		sort.Strings(names)
	}

	results := make([]*clusterhealth.ComponentStatusResult, 0, len(names))

	for _, name := range names {
		r, err := clusterhealth.GetComponentStatus(ctx, cfg.Client, name, cfg.AppsNS)
		if err != nil {
			results = append(results, &clusterhealth.ComponentStatusResult{
				Component: name,
				Errors:    []string{err.Error()},
			})

			continue
		}
		results = append(results, r)
	}

	return results
}
