//nolint:testpackage // Tests internal implementation (unexported functions)
package events

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

func TestGetTargetNamespaces(t *testing.T) {
	tests := []struct {
		name          string
		cmd           *Command
		wantNamespace []string
		wantErr       bool
	}{
		{
			name: "all namespaces with ODH namespaces",
			cmd: &Command{
				AllNamespaces:  true,
				ApplicationsNS: "redhat-ods-applications",
				OperatorNS:     "redhat-ods-operator",
				MonitoringNS:   "redhat-ods-monitoring",
			},
			wantNamespace: []string{
				"redhat-ods-applications",
				"redhat-ods-operator",
				"redhat-ods-monitoring",
			},
		},
		{
			name: "explicit -n flag is exclusive",
			cmd: &Command{
				NamespaceExplicit: true,
				Namespace:         "my-project",
				ApplicationsNS:    "redhat-ods-applications",
				OperatorNS:        "redhat-ods-operator",
				MonitoringNS:      "redhat-ods-applications",
			},
			wantNamespace: []string{
				"my-project",
			},
		},
		{
			name: "no flags uses ODH namespaces (not kubeconfig namespace)",
			cmd: &Command{
				NamespaceExplicit: false,
				Namespace:         "default", // from kubeconfig, should be ignored
				ApplicationsNS:    "redhat-ods-applications",
				OperatorNS:        "redhat-ods-operator",
				MonitoringNS:      "redhat-ods-monitoring",
			},
			wantNamespace: []string{
				"redhat-ods-applications",
				"redhat-ods-operator",
				"redhat-ods-monitoring",
			},
		},
		{
			name: "deduplicates ODH namespaces",
			cmd: &Command{
				AllNamespaces:  true,
				ApplicationsNS: "redhat-ods-applications",
				OperatorNS:     "redhat-ods-operator",
				MonitoringNS:   "redhat-ods-applications",
			},
			wantNamespace: []string{
				"redhat-ods-applications",
				"redhat-ods-operator",
			},
		},
		{
			name: "empty namespaces returns error",
			cmd: &Command{
				AllNamespaces: true,
			},
			wantErr: true,
		},
		{
			name: "skips empty namespace strings",
			cmd: &Command{
				AllNamespaces:  true,
				ApplicationsNS: "",
				OperatorNS:     "redhat-ods-operator",
				MonitoringNS:   "",
			},
			wantNamespace: []string{"redhat-ods-operator"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.cmd.getTargetNamespaces()

			if tt.wantErr {
				if err == nil {
					t.Error("getTargetNamespaces() expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Errorf("getTargetNamespaces() unexpected error: %v", err)

				return
			}

			if len(result) != len(tt.wantNamespace) {
				t.Errorf("getTargetNamespaces() returned %d namespaces, expected %d\nGot: %v\nExpected: %v",
					len(result), len(tt.wantNamespace), result, tt.wantNamespace)

				return
			}

			resultSet := make(map[string]bool)
			for _, ns := range result {
				resultSet[ns] = true
			}

			for _, want := range tt.wantNamespace {
				if !resultSet[want] {
					t.Errorf("getTargetNamespaces() missing namespace %q\nGot: %v", want, result)
				}
			}
		})
	}
}

func TestSortEventsByTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name   string
		events []clusterhealth.EventInfo
		want   []string
	}{
		{
			name:   "empty events",
			events: []clusterhealth.EventInfo{},
			want:   []string{},
		},
		{
			name: "single event",
			events: []clusterhealth.EventInfo{
				{Name: "event1", LastTime: now},
			},
			want: []string{"event1"},
		},
		{
			name: "multiple events sorted by time descending",
			events: []clusterhealth.EventInfo{
				{Name: "old", LastTime: now.Add(-1 * time.Hour)},
				{Name: "newest", LastTime: now},
				{Name: "middle", LastTime: now.Add(-30 * time.Minute)},
			},
			want: []string{"newest", "middle", "old"},
		},
		{
			name: "events with zero time",
			events: []clusterhealth.EventInfo{
				{Name: "no-time", LastTime: time.Time{}},
				{Name: "has-time", LastTime: now},
			},
			want: []string{"has-time", "no-time"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortEventsByTime(tt.events)

			if len(tt.events) != len(tt.want) {
				t.Errorf("sortEventsByTime() resulted in %d events, expected %d",
					len(tt.events), len(tt.want))

				return
			}

			for i, event := range tt.events {
				if event.Name != tt.want[i] {
					t.Errorf("sortEventsByTime()[%d] = %q, expected %q",
						i, event.Name, tt.want[i])
				}
			}
		})
	}
}

func TestGetComponentLabelValue(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		component string
		want      string
	}{
		{"kserve", "kserve"},
		{"dashboard", "dashboard"},
		{"aipipelines", "data-science-pipelines-operator"},
		{"modelregistry", "model-registry-operator"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		got := resources.GetComponentLabelValue(tt.component)
		g.Expect(got).To(Equal(tt.want))
	}
}

func TestKindToGVR(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		kind     string
		wantRes  string
		wantNull bool
	}{
		{"Pod", "pods", false},
		{"Deployment", "deployments", false},
		{"ReplicaSet", "replicasets", false},
		{"Unknown", "", true},
	}

	for _, tt := range tests {
		got := kindToGVR(tt.kind)
		if tt.wantNull {
			g.Expect(got.Resource).To(BeEmpty())
		} else {
			g.Expect(got.Resource).To(Equal(tt.wantRes))
		}
	}
}

// TestKindToGVRMapCompleteness ensures kindToGVRMap includes all expected core Kubernetes
// resource types. When adding new core resource types to pkg/resources/types.go that should
// be supported by --component filtering, add them to this list.
func TestKindToGVRMapCompleteness(t *testing.T) {
	// expectedCoreKinds lists core/apps Kubernetes kinds that should be in kindToGVRMap.
	// This test catches drift when new resource types are added to pkg/resources/types.go
	// but not to kindToGVRMap, which would silently exclude those events from --component filtering.
	expectedCoreKinds := []string{
		"Pod",
		"Deployment",
		"ReplicaSet",
		"StatefulSet",
		"DaemonSet",
		"Service",
		"ConfigMap",
		"Secret",
		"Job",
	}

	for _, kind := range expectedCoreKinds {
		t.Run(kind, func(t *testing.T) {
			gvr := kindToGVR(kind)
			if gvr.Resource == "" {
				t.Errorf("kindToGVRMap missing expected kind %q - add it to support --component filtering for %s events", kind, kind)
			}
		})
	}
}

// createTestPod creates an unstructured Pod with the given labels.
func createTestPod(name, namespace string, labels map[string]string) *unstructured.Unstructured {
	pod := &unstructured.Unstructured{}
	pod.SetAPIVersion("v1")
	pod.SetKind("Pod")
	pod.SetName(name)
	pod.SetNamespace(namespace)
	pod.SetLabels(labels)

	return pod
}

// createFakeClient creates a fake dynamic client with the given objects.
func createFakeClient(t *testing.T, objs ...runtime.Object) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	listKinds := map[schema.GroupVersionResource]string{
		resources.Pod.GVR():        "PodList",
		resources.Deployment.GVR(): "DeploymentList",
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objs...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}

func TestCheckObjectHasComponentLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		objects    []runtime.Object
		namespace  string
		kind       string
		objName    string
		labelValue string
		want       bool
		wantErr    bool
	}{
		{
			name: "pod with matching label",
			objects: []runtime.Object{
				createTestPod("kserve-pod", "odh-apps", map[string]string{
					resources.ComponentLabelKey: "kserve",
				}),
			},
			namespace:  "odh-apps",
			kind:       "Pod",
			objName:    "kserve-pod",
			labelValue: "kserve",
			want:       true,
		},
		{
			name: "pod with non-matching label",
			objects: []runtime.Object{
				createTestPod("dashboard-pod", "odh-apps", map[string]string{
					resources.ComponentLabelKey: "dashboard",
				}),
			},
			namespace:  "odh-apps",
			kind:       "Pod",
			objName:    "dashboard-pod",
			labelValue: "kserve",
			want:       false,
		},
		{
			name:       "object not found returns false without error",
			objects:    []runtime.Object{},
			namespace:  "odh-apps",
			kind:       "Pod",
			objName:    "missing-pod",
			labelValue: "kserve",
			want:       false,
			wantErr:    false,
		},
		{
			name:       "unsupported kind returns false without error",
			objects:    []runtime.Object{},
			namespace:  "odh-apps",
			kind:       "Lease",
			objName:    "test-lease",
			labelValue: "kserve",
			want:       false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := createFakeClient(t, tt.objects...)
			cmd := &Command{Client: fakeClient}

			got, err := cmd.checkObjectHasComponentLabel(ctx, tt.namespace, tt.kind, tt.objName, tt.labelValue)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestFilterEventsByComponent(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	kservePod := createTestPod("kserve-pod", "odh-apps", map[string]string{
		resources.ComponentLabelKey: "kserve",
	})
	dashboardPod := createTestPod("dashboard-pod", "odh-apps", map[string]string{
		resources.ComponentLabelKey: "dashboard",
	})

	fakeClient := createFakeClient(t, kservePod, dashboardPod)

	events := []clusterhealth.EventInfo{
		{Namespace: "odh-apps", Kind: "Pod", Name: "kserve-pod", Reason: "Created"},
		{Namespace: "odh-apps", Kind: "Pod", Name: "dashboard-pod", Reason: "Created"},
		{Namespace: "odh-apps", Kind: "Pod", Name: "missing-pod", Reason: "Created"},
	}

	cmd := &Command{
		Client:    fakeClient,
		Component: "kserve",
	}

	filtered, err := cmd.filterEventsByComponent(ctx, events)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(filtered).To(HaveLen(1))
	g.Expect(filtered[0].Name).To(Equal("kserve-pod"))
}

func TestLabelCache(t *testing.T) {
	g := NewWithT(t)
	cache := newLabelCache()

	// Empty cache
	val, found := cache.lookup("key1")
	g.Expect(found).To(BeFalse())
	g.Expect(val).To(BeFalse())

	// Store and retrieve
	cache.store("key1", true)
	val, found = cache.lookup("key1")
	g.Expect(found).To(BeTrue())
	g.Expect(val).To(BeTrue())

	cache.store("key2", false)
	val, found = cache.lookup("key2")
	g.Expect(found).To(BeTrue())
	g.Expect(val).To(BeFalse())
}

func TestIsEventBeforeCutoff(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-1 * time.Hour)

	tests := []struct {
		name  string
		event *corev1.Event
		want  bool
	}{
		{
			name:  "event after cutoff",
			event: &corev1.Event{LastTimestamp: metav1.Time{Time: now.Add(-30 * time.Minute)}},
			want:  false,
		},
		{
			name:  "event before cutoff",
			event: &corev1.Event{LastTimestamp: metav1.Time{Time: now.Add(-2 * time.Hour)}},
			want:  true,
		},
		{
			name:  "zero timestamp",
			event: &corev1.Event{},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := isEventBeforeCutoff(tt.event, cutoff)
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestExtractValidEvent(t *testing.T) {
	now := time.Now()
	cmd := &Command{}
	cache := newLabelCache()

	tests := []struct {
		name       string
		watchEvent watch.Event
		wantNil    bool
	}{
		{
			name: "valid ADDED event",
			watchEvent: watch.Event{
				Type:   watch.Added,
				Object: &corev1.Event{LastTimestamp: metav1.Time{Time: now}},
			},
			wantNil: false,
		},
		{
			name: "valid MODIFIED event",
			watchEvent: watch.Event{
				Type:   watch.Modified,
				Object: &corev1.Event{LastTimestamp: metav1.Time{Time: now}},
			},
			wantNil: false,
		},
		{
			name: "DELETED event returns nil",
			watchEvent: watch.Event{
				Type:   watch.Deleted,
				Object: &corev1.Event{},
			},
			wantNil: true,
		},
		{
			name: "non-Event object returns nil",
			watchEvent: watch.Event{
				Type:   watch.Added,
				Object: &corev1.Pod{},
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := cmd.extractValidEvent(context.Background(), tt.watchEvent, cache, time.Time{})
			if tt.wantNil {
				g.Expect(got).To(BeNil())
			} else {
				g.Expect(got).ToNot(BeNil())
			}
		})
	}
}

func TestEventToEventInfo(t *testing.T) {
	g := NewWithT(t)
	now := time.Now()

	event := corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "test-pod",
		},
		Reason:        "Created",
		Message:       "Pod created",
		Count:         1,
		Type:          "Normal",
		LastTimestamp: metav1.Time{Time: now},
	}

	info := eventToEventInfo(event)

	g.Expect(info.Namespace).To(Equal("test-ns"))
	g.Expect(info.Name).To(Equal("test-pod"))
	g.Expect(info.Kind).To(Equal("Pod"))
	g.Expect(info.Reason).To(Equal("Created"))
	g.Expect(info.Type).To(Equal("Normal"))
}

func TestProcessWatchEvents(t *testing.T) {
	g := NewWithT(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	fakeWatcher := watch.NewFake()
	eventCh := make(chan corev1.Event, 10)
	cache := newLabelCache()
	cmd := &Command{}

	var wg sync.WaitGroup
	wg.Go(func() {
		cmd.processWatchEvents(ctx, fakeWatcher, eventCh, cache, time.Time{})
	})

	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "event1", Namespace: "test"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "pod1",
		},
		LastTimestamp: metav1.Time{Time: time.Now()},
	}

	fakeWatcher.Add(event)

	g.Eventually(func() int {
		select {
		case <-eventCh:
			return 1
		default:
			return 0
		}
	}, time.Second, 10*time.Millisecond).Should(Equal(1))

	fakeWatcher.Stop()
	wg.Wait()
}

func TestStreamWatchedEvents(t *testing.T) {
	g := NewWithT(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var buf bytes.Buffer
	io := iostreams.NewIOStreams(nil, &buf, &buf)

	cmd := &Command{
		IO:        io,
		streamOut: newStreamWriter(&buf, false),
	}

	eventCh := make(chan corev1.Event, 5)
	eventCh <- corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "test-pod",
		},
		Reason:        "Created",
		Type:          "Normal",
		LastTimestamp: metav1.Time{Time: time.Now()},
	}
	close(eventCh)

	err := cmd.streamWatchedEvents(ctx, eventCh)
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).To(ContainSubstring("Pod/test-pod"))
	g.Expect(output).To(ContainSubstring("Created"))
}

func TestComponentCacheKey(t *testing.T) {
	g := NewWithT(t)

	key := componentCacheKey("kserve", "odh-apps", "Pod", "my-pod")
	g.Expect(key).To(Equal("kserve/odh-apps/Pod/my-pod"))

	key2 := componentCacheKey("dashboard", "ns", "Deployment", "deploy")
	g.Expect(key2).To(Equal("dashboard/ns/Deployment/deploy"))
}

func TestCheckEventMatchesComponentCached_ErrorNotCached(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create client with no objects - will return not found (no error)
	fakeClient := createFakeClient(t)
	var errBuf bytes.Buffer

	cmd := &Command{
		Client:    fakeClient,
		Component: "kserve",
		IO:        iostreams.NewIOStreams(nil, nil, &errBuf),
	}

	cache := newLabelCache()
	event := &corev1.Event{
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "missing-pod",
			Namespace: "odh-apps",
		},
	}

	// First call - object not found returns false, caches result
	result := cmd.checkEventMatchesComponentCached(ctx, event, cache)
	g.Expect(result).To(BeFalse())

	// Verify it was cached (not found is not an error, so it's cached)
	cacheKey := componentCacheKey("kserve", "odh-apps", "Pod", "missing-pod")
	val, found := cache.lookup(cacheKey)
	g.Expect(found).To(BeTrue())
	g.Expect(val).To(BeFalse())
}
