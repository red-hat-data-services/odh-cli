package events

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const (
	eventFetchTimeout  = 30 * time.Second
	eventChannelBuffer = 100
)

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

const maxLabelFetchWorkers = 10

// labelLookup represents an object to check for component labels.
type labelLookup struct {
	namespace, kind, name string
}

// filterEventsByComponent filters events to only those related to a component.
// It looks up each event's InvolvedObject and checks for the component label.
// Lookups are parallelized for performance, with up to 10 concurrent API calls.
// Returns filtered events and any API error encountered (RBAC, timeout, etc.).
func (c *Command) filterEventsByComponent(ctx context.Context, events []clusterhealth.EventInfo) ([]clusterhealth.EventInfo, error) {
	targetLabel := resources.GetComponentLabelValue(c.Component)

	// Collect unique objects to look up
	seen := make(map[labelLookup]struct{})
	var lookups []labelLookup

	for _, event := range events {
		key := labelLookup{event.Namespace, event.Kind, event.Name}
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			lookups = append(lookups, key)
		}
	}

	// Fetch labels in parallel
	labelCache, err := c.fetchComponentLabelsParallel(ctx, lookups, targetLabel)
	if err != nil {
		return nil, err
	}

	// Filter events using pre-populated cache
	var filtered []clusterhealth.EventInfo

	for _, event := range events {
		cacheKey := componentCacheKey(targetLabel, event.Namespace, event.Kind, event.Name)
		if labelCache[cacheKey] {
			filtered = append(filtered, event)
		}
	}

	return filtered, nil
}

// fetchComponentLabelsParallel fetches component labels for objects in parallel.
func (c *Command) fetchComponentLabelsParallel(ctx context.Context, lookups []labelLookup, targetLabel string) (map[string]bool, error) {
	if len(lookups) == 0 {
		return make(map[string]bool), nil
	}

	type result struct {
		cacheKey string
		hasLabel bool
		err      error
	}

	resultCh := make(chan result, len(lookups))
	workCh := make(chan labelLookup, len(lookups))

	// Start workers
	numWorkers := min(maxLabelFetchWorkers, len(lookups))

	var wg sync.WaitGroup

	for range numWorkers {
		wg.Go(func() {
			for item := range workCh {
				hasLabel, err := c.checkObjectHasComponentLabel(ctx, item.namespace, item.kind, item.name, targetLabel)
				cacheKey := componentCacheKey(targetLabel, item.namespace, item.kind, item.name)
				resultCh <- result{cacheKey, hasLabel, err}
			}
		})
	}

	// Send work
	for _, item := range lookups {
		workCh <- item
	}

	close(workCh)

	// Wait for completion
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results
	cache := make(map[string]bool, len(lookups))

	for res := range resultCh {
		if res.err != nil {
			return nil, fmt.Errorf("checking component label: %w", res.err)
		}

		cache[res.cacheKey] = res.hasLabel
	}

	return cache, nil
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

// componentCacheKey builds a cache key for component label lookups.
func componentCacheKey(targetLabel, namespace, kind, name string) string {
	return fmt.Sprintf("%s/%s/%s/%s", targetLabel, namespace, kind, name)
}

// labelCache provides thread-safe caching for component label lookups.
type labelCache struct {
	mu    sync.RWMutex
	cache map[string]bool
}

func newLabelCache() *labelCache {
	return &labelCache{cache: make(map[string]bool)}
}

func (lc *labelCache) lookup(key string) (bool, bool) {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	v, ok := lc.cache[key]

	return v, ok
}

func (lc *labelCache) store(key string, val bool) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.cache[key] = val
}

// watchEvents streams events in real-time using Kubernetes watch.
func (c *Command) watchEvents(ctx context.Context) error {
	namespaces, err := c.getTargetNamespaces()
	if err != nil {
		return err
	}

	showNamespace := c.AllNamespaces || !c.NamespaceExplicit
	if err := c.printStreamHeader(showNamespace); err != nil {
		return err
	}

	eventCh := make(chan corev1.Event, eventChannelBuffer)
	cache := newLabelCache()

	var wg sync.WaitGroup

	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, ns := range namespaces {
		wg.Add(1)

		go func(namespace string) {
			defer wg.Done()
			c.watchNamespaceEvents(watchCtx, namespace, eventCh, cache)
		}(ns)
	}

	go func() {
		wg.Wait()
		close(eventCh)
	}()

	return c.streamWatchedEvents(watchCtx, eventCh)
}

const (
	maxWatchRetries     = 5
	initialRetryBackoff = time.Second
	maxRetryBackoff     = 30 * time.Second
	backoffMultiplier   = 2
)

// watchNamespaceEvents watches events in a single namespace with automatic reconnection.
func (c *Command) watchNamespaceEvents(ctx context.Context, namespace string, eventCh chan<- corev1.Event, cache *labelCache) {
	listOpts := metav1.ListOptions{}
	if c.EventType != "" {
		listOpts.FieldSelector = "type=" + c.EventType
	}

	// sinceCutoff filters initial/existing events only; new events streaming in are always shown
	var sinceCutoff time.Time
	if c.Since > 0 {
		sinceCutoff = time.Now().Add(-c.Since)
	}

	var consecutiveErrors int

	backoff := initialRetryBackoff

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		watcher, err := c.Client.CoreV1().Events(namespace).Watch(ctx, listOpts)
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= maxWatchRetries {
				_, _ = fmt.Fprintf(c.IO.ErrOut(), "Error: failed to watch events in %s after %d retries: %v\n", namespace, maxWatchRetries, err)

				return
			}

			_, _ = fmt.Fprintf(c.IO.ErrOut(), "Warning: failed to watch events in %s: %v, retrying in %v...\n", namespace, err, backoff)
			time.Sleep(backoff)
			backoff = min(backoff*backoffMultiplier, maxRetryBackoff)

			continue
		}

		// Reset error state on successful connection
		consecutiveErrors = 0
		backoff = initialRetryBackoff

		lastRV := c.processWatchEvents(ctx, watcher, eventCh, cache, sinceCutoff)
		watcher.Stop()

		// Use last seen ResourceVersion on reconnect to avoid replaying events
		if lastRV != "" {
			listOpts.ResourceVersion = lastRV
		}

		select {
		case <-ctx.Done():
			return
		default:
			_, _ = fmt.Fprintf(c.IO.ErrOut(), "Warning: watch connection closed for %s, reconnecting...\n", namespace)
			time.Sleep(time.Second)
		}
	}
}

// processWatchEvents handles events from a watcher until the channel closes.
// Returns the last seen ResourceVersion for reconnection.
func (c *Command) processWatchEvents(ctx context.Context, watcher watch.Interface, eventCh chan<- corev1.Event, cache *labelCache, sinceCutoff time.Time) string {
	var lastRV string

	for {
		select {
		case <-ctx.Done():
			return lastRV
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return lastRV
			}

			// Advance ResourceVersion before filtering to avoid replaying filtered events on reconnect
			if e, ok := event.Object.(*corev1.Event); ok && e.ResourceVersion != "" {
				lastRV = e.ResourceVersion
			}

			if e := c.extractValidEvent(ctx, event, cache, sinceCutoff); e != nil {
				select {
				case eventCh <- *e:
				case <-ctx.Done():
					return lastRV
				}
			}
		}
	}
}

// extractValidEvent extracts a valid event from a watch event, applying filters.
func (c *Command) extractValidEvent(ctx context.Context, event watch.Event, cache *labelCache, sinceCutoff time.Time) *corev1.Event {
	if event.Type != watch.Added && event.Type != watch.Modified {
		return nil
	}

	e, ok := event.Object.(*corev1.Event)
	if !ok {
		return nil
	}

	if !sinceCutoff.IsZero() && isEventBeforeCutoff(e, sinceCutoff) {
		return nil
	}

	if c.Component != "" && !c.checkEventMatchesComponentCached(ctx, e, cache) {
		return nil
	}

	return e
}

// isEventBeforeCutoff checks if the event occurred before the cutoff time.
func isEventBeforeCutoff(e *corev1.Event, cutoff time.Time) bool {
	eventTime := e.LastTimestamp.Time
	if eventTime.IsZero() {
		eventTime = e.EventTime.Time
	}

	if eventTime.IsZero() {
		return false
	}

	return eventTime.Before(cutoff)
}

// streamWatchedEvents outputs events as they arrive.
func (c *Command) streamWatchedEvents(ctx context.Context, eventCh <-chan corev1.Event) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-eventCh:
			if !ok {
				return nil
			}

			info := eventToEventInfo(event)
			if err := c.printSingleEvent(info); err != nil {
				return err
			}
		}
	}
}

// eventToEventInfo converts a corev1.Event to clusterhealth.EventInfo.
func eventToEventInfo(e corev1.Event) clusterhealth.EventInfo {
	lastTime := e.LastTimestamp.Time
	if lastTime.IsZero() {
		lastTime = e.EventTime.Time
	}

	return clusterhealth.EventInfo{
		Namespace: e.Namespace,
		Name:      e.InvolvedObject.Name,
		Kind:      e.InvolvedObject.Kind,
		Reason:    e.Reason,
		Message:   e.Message,
		Count:     e.Count,
		Type:      e.Type,
		LastTime:  lastTime,
	}
}

// checkEventMatchesComponentCached checks if an event's InvolvedObject belongs to a component, using cache.
func (c *Command) checkEventMatchesComponentCached(ctx context.Context, event *corev1.Event, cache *labelCache) bool {
	targetLabel := resources.GetComponentLabelValue(c.Component)
	cacheKey := componentCacheKey(targetLabel, event.InvolvedObject.Namespace, event.InvolvedObject.Kind, event.InvolvedObject.Name)

	if hasLabel, found := cache.lookup(cacheKey); found {
		return hasLabel
	}

	hasLabel, err := c.checkObjectHasComponentLabel(ctx, event.InvolvedObject.Namespace,
		event.InvolvedObject.Kind, event.InvolvedObject.Name, targetLabel)
	if err != nil {
		_, _ = fmt.Fprintf(c.IO.ErrOut(), "Warning: failed to check component label for %s/%s: %v\n",
			event.InvolvedObject.Kind, event.InvolvedObject.Name, err)
		// Don't cache on error - allow retry on next event for this object
		return false
	}

	cache.store(cacheKey, hasLabel)

	return hasLabel
}
