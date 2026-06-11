package events

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
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

// filterEventsByComponent filters events to only those related to a component.
// It looks up each event's InvolvedObject and checks for the component label.
// Returns filtered events and any API error encountered (RBAC, timeout, etc.).
func (c *Command) filterEventsByComponent(ctx context.Context, events []clusterhealth.EventInfo) ([]clusterhealth.EventInfo, error) {
	targetLabel := resources.GetComponentLabelValue(c.Component)
	labelCache := make(map[string]bool)

	var filtered []clusterhealth.EventInfo

	for _, event := range events {
		cacheKey := fmt.Sprintf("%s/%s/%s/%s", targetLabel, event.Namespace, event.Kind, event.Name)

		hasLabel, found := labelCache[cacheKey]
		if !found {
			var err error
			hasLabel, err = c.checkObjectHasComponentLabel(ctx, event.Namespace, event.Kind, event.Name, targetLabel)
			if err != nil {
				return nil, fmt.Errorf("checking component label for %s/%s: %w", event.Kind, event.Name, err)
			}
			labelCache[cacheKey] = hasLabel
		}

		if hasLabel {
			filtered = append(filtered, event)
		}
	}

	return filtered, nil
}

// checkObjectHasComponentLabel checks if an object has the component label.
// Returns (false, nil) for unsupported kinds or not-found objects.
// Returns error for RBAC failures, timeouts, or other API errors.
func (c *Command) checkObjectHasComponentLabel(ctx context.Context, namespace, kind, name, labelValue string) (bool, error) {
	gvr := kindToGVR(kind)
	if gvr.Resource == "" {
		return false, nil
	}

	unstr, err := c.getObject(ctx, gvr, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}

		return false, fmt.Errorf("getting %s %s/%s: %w", gvr.Resource, namespace, name, err)
	}

	labels := unstr.GetLabels()

	return labels[resources.ComponentLabelKey] == labelValue, nil
}

// getObject fetches an object by GVR, namespace, and name.
// Returns raw error to allow caller to type-check before wrapping.
func (c *Command) getObject(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	if namespace != "" {
		//nolint:wrapcheck // Caller wraps after type-checking (e.g., IsNotFound)
		return c.Client.Dynamic().Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	}

	//nolint:wrapcheck // Caller wraps after type-checking (e.g., IsNotFound)
	return c.Client.Dynamic().Resource(gvr).Get(ctx, name, metav1.GetOptions{})
}

// kindToGVRMap maps Kubernetes Kind to ResourceType for event filtering.
// Note: Only core/apps kinds are mapped. Events referencing CRD objects
// (InferenceService, RayCluster, Notebook, etc.) are excluded from --component filtering.
//
//nolint:gochecknoglobals // Static mapping configuration
var kindToGVRMap = map[string]resources.ResourceType{
	"Pod":         resources.Pod,
	"Deployment":  resources.Deployment,
	"ReplicaSet":  resources.ReplicaSet,
	"StatefulSet": resources.StatefulSet,
	"DaemonSet":   resources.DaemonSet,
	"Service":     resources.Service,
	"ConfigMap":   resources.ConfigMap,
	"Secret":      resources.Secret,
	"Job":         resources.Job,
}

// kindToGVR maps Kubernetes Kind to GroupVersionResource.
func kindToGVR(kind string) schema.GroupVersionResource {
	if rt, ok := kindToGVRMap[kind]; ok {
		return rt.GVR()
	}

	return schema.GroupVersionResource{}
}
