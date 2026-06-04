package events

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"

	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const eventFetchTimeout = 30 * time.Second

// fetchEvents retrieves events using the clusterhealth library.
func (c *Command) fetchEvents(ctx context.Context) ([]clusterhealth.EventInfo, error) {
	namespaces, err := c.getTargetNamespaces()
	if err != nil {
		return nil, err
	}

	fetchCtx, cancel := context.WithTimeout(ctx, eventFetchTimeout)
	defer cancel()

	cfg := clusterhealth.RecentEventsConfig{
		Client:     c.crClient,
		Namespaces: namespaces,
		Since:      c.Since,
		EventType:  c.EventType,
	}

	events, err := clusterhealth.RunRecentEvents(fetchCtx, cfg)
	if err != nil {
		return nil, clierrors.ErrEventsFetchFailed(err)
	}

	return events, nil
}

// getTargetNamespaces returns the namespaces to query for events.
// For -n <namespace>, returns ONLY that namespace (exclusive scope like kubectl).
// For --all-namespaces or no flags, returns ODH namespaces (apps, operator, monitoring).
// Returns ErrNoNamespacesDiscovered if no namespaces could be determined.
func (c *Command) getTargetNamespaces() ([]string, error) {
	// If user explicitly passed -n <namespace>, return ONLY that namespace (exclusive)
	// Note: NamespaceExplicit distinguishes "odh events -n foo" from "odh events" (no flags)
	if c.NamespaceExplicit && c.Namespace != "" {
		return []string{c.Namespace}, nil
	}

	// Otherwise return ODH namespaces (for -A or no flags)
	seen := make(map[string]bool)
	var namespaces []string

	add := func(ns string) {
		if ns != "" && !seen[ns] {
			seen[ns] = true
			namespaces = append(namespaces, ns)
		}
	}

	add(c.ApplicationsNS)
	add(c.OperatorNS)
	add(c.MonitoringNS)

	if len(namespaces) == 0 {
		return nil, clierrors.ErrNoNamespacesDiscovered()
	}

	return namespaces, nil
}

// sortEventsByTime sorts events in place by timestamp, most recent first.
// Uses stable sort to preserve original order for events with identical timestamps.
func sortEventsByTime(events []clusterhealth.EventInfo) {
	slices.SortStableFunc(events, func(a, b clusterhealth.EventInfo) int {
		return cmp.Compare(b.LastTime.UnixNano(), a.LastTime.UnixNano())
	})
}
