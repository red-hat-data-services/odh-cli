package components

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sync/errgroup"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/conditions"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

// HealthInfo contains health status extracted from a component CR.
type HealthInfo struct {
	Ready      *bool
	Message    string
	Conditions []metav1.Condition
}

// EnrichWithHealth adds health information to components by querying component CRs.
// Health fetches run in parallel for better performance with many active components.
// If the component CR API is unavailable (older ODH), Ready will be nil.
func EnrichWithHealth(ctx context.Context, r client.Reader, components []ComponentInfo) []ComponentInfo {
	enriched := make([]ComponentInfo, len(components))
	copy(enriched, components)

	g, ctx := errgroup.WithContext(ctx)

	for i := range enriched {
		if !enriched[i].IsActive() {
			continue
		}

		g.Go(func() error {
			health, err := GetComponentHealth(ctx, r, enriched[i].Name)
			if err != nil {
				if client.IsResourceTypeNotFound(err) {
					return nil
				}

				enriched[i].Message = err.Error()

				return nil
			}

			enriched[i].Ready = health.Ready
			enriched[i].Message = health.Message

			return nil
		})
	}

	_ = g.Wait() // Errors are handled per-component, not propagated

	return enriched
}

// GetComponentHealth retrieves health information for a specific component.
func GetComponentHealth(ctx context.Context, r client.Reader, name string) (*HealthInfo, error) {
	rt := resources.GetComponentCR(name)
	if rt == nil {
		return nil, fmt.Errorf("unknown component CR for %q", name)
	}

	items, err := r.List(ctx, *rt)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return nil, fmt.Errorf("component CR API unavailable: %w", err)
		}

		return nil, fmt.Errorf("listing component CR: %w", err)
	}

	if len(items) == 0 {
		return &HealthInfo{}, nil
	}

	return extractHealthFromCR(items[0])
}

// extractHealthFromCR parses health info from a component CR's status.
func extractHealthFromCR(cr *unstructured.Unstructured) (*HealthInfo, error) {
	health := &HealthInfo{}

	conds, err := jq.Query[[]metav1.Condition](cr, ".status.conditions // []")
	if err != nil && !errors.Is(err, jq.ErrNotFound) {
		return nil, fmt.Errorf("querying conditions: %w", err)
	}

	health.Conditions = conds

	ready := conditions.FindReady(conds)
	if ready != nil {
		isReady := ready.Status == metav1.ConditionTrue
		health.Ready = &isReady

		if !isReady && ready.Message != "" {
			health.Message = ready.Message
		}
	}

	if health.Message == "" {
		health.Message = conditions.CollectDegradedMessages(conds)
	}

	return health, nil
}
