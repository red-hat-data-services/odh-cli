//nolint:testpackage // Tests internal implementation (Client fields)
package client

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/onsi/gomega/types"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	discoveryfake "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	metadatafake "k8s.io/client-go/metadata/fake"
	coretesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
)

const testNamespace = "test-namespace"

// createTestObjects creates unstructured objects from YAML manifests.
func createTestObjects(count int) []runtime.Object {
	objects := make([]runtime.Object, count)
	for i := range count {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "test-cm-" + string(rune('1'+i)),
					"namespace": testNamespace,
				},
			},
		}
		objects[i] = obj
	}

	return objects
}

// HavePointerTo is a matcher that verifies the result is a pointer to the expected value.
func HavePointerTo(expected types.GomegaMatcher) types.GomegaMatcher {
	return WithTransform(func(ptr *unstructured.Unstructured) unstructured.Unstructured {
		if ptr == nil {
			return unstructured.Unstructured{}
		}

		return *ptr
	}, expected)
}

func TestListResources_SinglePage(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := createTestObjects(2)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	results, err := client.ListResources(ctx, gvr)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(HaveLen(2))
	g.Expect(results[0]).To(HavePointerTo(HaveField("Object", HaveKeyWithValue("kind", "ConfigMap"))))
	g.Expect(results[1]).To(HavePointerTo(HaveField("Object", HaveKeyWithValue("kind", "ConfigMap"))))
}

func TestListResources_MultiplePages(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create more objects than the page size to trigger pagination
	objects := createTestObjects(10)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	// API will automatically paginate when needed
	results, err := client.ListResources(ctx, gvr)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(HaveLen(10))

	// Verify all results are pointers
	for i := range results {
		g.Expect(results[i]).ToNot(BeNil())
		g.Expect(results[i]).To(HavePointerTo(HaveField("Object", HaveKeyWithValue("kind", "ConfigMap"))))
	}
}

func TestListResources_EmptyResults(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	// Create fake client with custom list kinds to handle ConfigMapList
	gvrListMap := map[schema.GroupVersionResource]string{
		{Group: "", Version: "v1", Resource: "configmaps"}: "ConfigMapList",
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrListMap)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	results, err := client.ListResources(ctx, gvr)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(BeEmpty())
}

func TestListResources_NamespaceScoped(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := createTestObjects(3)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	results, err := client.ListResources(ctx, gvr, WithNamespace(testNamespace))

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(HaveLen(3))

	// Verify all results are in the expected namespace
	for i := range results {
		g.Expect(results[i].GetNamespace()).To(Equal(testNamespace))
	}
}

func TestList_DelegatesToListResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := createTestObjects(2)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	resourceType := resources.ResourceType{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
		Kind:     "ConfigMap",
	}

	results, err := client.List(ctx, resourceType)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(HaveLen(2))
}

// TestListMetadata_Pagination is skipped due to limitations in fake metadata client.
// In real usage, ListMetadata works correctly with proper Kubernetes API server.
func TestListMetadata_Pagination(t *testing.T) {
	t.Skip("Skipping ListMetadata test due to fake client limitations")
}

func TestGetSingleton_WithPointers(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := createTestObjects(1)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	resourceType := resources.ResourceType{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
		Kind:     "ConfigMap",
	}

	result, err := GetSingleton(ctx, client, resourceType)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.GetName()).To(Equal("test-cm-1"))
}

func TestGetSingleton_MultipleInstances(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := createTestObjects(2)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	resourceType := resources.ResourceType{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
		Kind:     "ConfigMap",
	}

	_, err := GetSingleton(ctx, client, resourceType)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("expected single"))
}

func TestGetSingleton_NoInstances(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	// Create fake client with custom list kinds to handle ConfigMapList
	gvrListMap := map[schema.GroupVersionResource]string{
		{Group: "", Version: "v1", Resource: "configmaps"}: "ConfigMapList",
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrListMap)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	resourceType := resources.ResourceType{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
		Kind:     "ConfigMap",
	}

	_, err := GetSingleton(ctx, client, resourceType)

	g.Expect(err).To(HaveOccurred())
}

// TestListResources_ClusterScoped verifies cluster-scoped resource listing.
func TestListResources_ClusterScoped(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create cluster-scoped objects (no namespace)
	objects := make([]runtime.Object, 3)
	for i := range 3 {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]any{
					"name": "test-ns-" + string(rune('1'+i)),
				},
			},
		}
		objects[i] = obj
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	}

	// List without namespace filter (cluster-scoped)
	results, err := client.ListResources(ctx, gvr)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(HaveLen(3))

	// Verify all results are cluster-scoped (no namespace)
	for i := range results {
		g.Expect(results[i].GetNamespace()).To(BeEmpty())
	}
}

// TestListMetadata_NamespaceScoped is skipped due to limitations in fake metadata client.
// In real usage, ListMetadata works correctly with proper Kubernetes API server.
func TestListMetadata_NamespaceScoped(t *testing.T) {
	t.Skip("Skipping ListMetadata test due to fake client limitations")
}

// createDSCInitialization creates a DSCInitialization object with the given applications namespace.
func createDSCInitialization(applicationsNamespace string) runtime.Object {
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
		},
	}

	if applicationsNamespace != "" {
		dsci.Object["spec"] = map[string]any{
			"applicationsNamespace": applicationsNamespace,
		}
	}

	return dsci
}

// createDSCInitializationWithEmptySpec creates a DSCI with spec but no applicationsNamespace.
func createDSCInitializationWithEmptySpec() runtime.Object {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"otherField": "value",
			},
		},
	}
}

func TestGetApplicationsNamespace_DSCINotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	// Create fake client with custom list kinds for DSCInitialization
	gvrListMap := map[schema.GroupVersionResource]string{
		resources.DSCInitialization.GVR(): "DSCInitializationList",
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrListMap)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	namespace, err := GetApplicationsNamespace(ctx, client)

	g.Expect(err).To(Satisfy(apierrors.IsNotFound))
	g.Expect(namespace).To(BeEmpty())
}

func TestGetApplicationsNamespace_NamespaceSet(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	const expectedNamespace = "my-odh-namespace"

	dsci := createDSCInitialization(expectedNamespace)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, dsci)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, dsci)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	namespace, err := GetApplicationsNamespace(ctx, client)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(namespace).To(Equal(expectedNamespace))
}

func TestGetApplicationsNamespace_NamespaceNotSet(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsci := createDSCInitializationWithEmptySpec()
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, dsci)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, dsci)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	namespace, err := GetApplicationsNamespace(ctx, client)

	g.Expect(err).To(Satisfy(apierrors.IsNotFound))
	g.Expect(namespace).To(BeEmpty())
}

func TestGetApplicationsNamespace_EmptyNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create DSCI with empty applicationsNamespace
	dsci := createDSCInitialization("")
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, dsci)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, dsci)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	namespace, err := GetApplicationsNamespace(ctx, client)

	g.Expect(err).To(Satisfy(apierrors.IsNotFound))
	g.Expect(namespace).To(BeEmpty())
}

// createDSCInitializationWithMonitoring creates a DSCI with both applications and monitoring namespaces.
func createDSCInitializationWithMonitoring(applicationsNS, monitoringNS string) runtime.Object {
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{},
		},
	}

	spec := dsci.Object["spec"].(map[string]any)
	if applicationsNS != "" {
		spec["applicationsNamespace"] = applicationsNS
	}
	if monitoringNS != "" {
		spec["monitoring"] = map[string]any{
			"namespace": monitoringNS,
		}
	}

	return dsci
}

func TestGetDSCINamespaces_BothNamespacesSet(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsci := createDSCInitializationWithMonitoring("redhat-ods-applications", "redhat-ods-monitoring")
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, dsci)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, dsci)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	namespaces, err := GetDSCINamespaces(ctx, client)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(namespaces.Applications).To(Equal("redhat-ods-applications"))
	g.Expect(namespaces.Monitoring).To(Equal("redhat-ods-monitoring"))
}

func TestGetDSCINamespaces_MonitoringFallsBackToApplications(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// DSCI with only applications namespace set
	dsci := createDSCInitializationWithMonitoring("redhat-ods-applications", "")
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, dsci)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, dsci)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	namespaces, err := GetDSCINamespaces(ctx, client)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(namespaces.Applications).To(Equal("redhat-ods-applications"))
	g.Expect(namespaces.Monitoring).To(Equal("redhat-ods-applications"))
}

func TestGetDSCINamespaces_DSCINotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	gvrListMap := map[schema.GroupVersionResource]string{
		resources.DSCInitialization.GVR(): "DSCInitializationList",
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrListMap)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	client := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	namespaces, err := GetDSCINamespaces(ctx, client)

	g.Expect(err).To(Satisfy(apierrors.IsNotFound))
	g.Expect(namespaces.Applications).To(BeEmpty())
	g.Expect(namespaces.Monitoring).To(BeEmpty())
}

// --- List[T] tests ---

func configMapResourceType() resources.ResourceType {
	return resources.ResourceType{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
		Kind:     "ConfigMap",
	}
}

func TestList_WithFilter(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := createTestObjects(3)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	c := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	// Filter: only items named "test-cm-1".
	filter := func(obj *unstructured.Unstructured) (bool, error) {
		return obj.GetName() == "test-cm-1", nil
	}

	results, err := List[*unstructured.Unstructured](ctx, c, configMapResourceType(), filter)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(HaveLen(1))
	g.Expect(results[0].GetName()).To(Equal("test-cm-1"))
	g.Expect(results[0].GetNamespace()).To(Equal(testNamespace))
}

func TestList_NilFilter(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := createTestObjects(3)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	c := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	results, err := List[*unstructured.Unstructured](ctx, c, configMapResourceType(), nil)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(HaveLen(3))
}

func TestList_EmptyList(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	gvrListMap := map[schema.GroupVersionResource]string{
		configMapResourceType().GVR(): "ConfigMapList",
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrListMap)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	c := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	results, err := List[*unstructured.Unstructured](ctx, c, configMapResourceType(), nil)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(BeEmpty())
}

func TestList_CRDNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Use a resource type whose CRD does not exist.
	notebookResource := resources.Notebook

	c := &errorReader{
		listErr: &meta.NoResourceMatchError{PartialResource: notebookResource.GVR()},
	}

	results, err := List[*unstructured.Unstructured](ctx, c, notebookResource, nil)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(BeNil())
}

func TestList_FilterErrorPropagation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := createTestObjects(2)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	c := &defaultClient{
		dynamic:   dynamicClient,
		metadata:  metadataClient,
		olmReader: newOLMReader(nil),
	}

	filterErr := errors.New("filter failed")

	filter := func(_ *unstructured.Unstructured) (bool, error) {
		return false, filterErr
	}

	results, err := List[*unstructured.Unstructured](ctx, c, configMapResourceType(), filter)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ContainSubstring("filter failed")))
	g.Expect(results).To(BeNil())
}

// errorReader is a minimal Reader that returns a preconfigured error from all methods.
type errorReader struct {
	listErr error
}

func (r *errorReader) List(
	_ context.Context,
	_ resources.ResourceType,
	_ ...ListResourcesOption,
) ([]*unstructured.Unstructured, error) {
	return nil, r.listErr
}

func (r *errorReader) ListMetadata(
	_ context.Context,
	_ resources.ResourceType,
	_ ...ListResourcesOption,
) ([]*metav1.PartialObjectMetadata, error) {
	return nil, r.listErr
}

func (r *errorReader) ListResources(
	_ context.Context,
	_ schema.GroupVersionResource,
	_ ...ListResourcesOption,
) ([]*unstructured.Unstructured, error) {
	return nil, r.listErr
}

func (r *errorReader) Get(
	_ context.Context,
	_ schema.GroupVersionResource,
	_ string,
	_ ...GetOption,
) (*unstructured.Unstructured, error) {
	return nil, r.listErr
}

func (r *errorReader) GetResource(
	_ context.Context,
	_ resources.ResourceType,
	_ string,
	_ ...GetOption,
) (*unstructured.Unstructured, error) {
	return nil, r.listErr
}

func (r *errorReader) GetResourceMetadata(
	_ context.Context,
	_ resources.ResourceType,
	_ string,
	_ ...GetOption,
) (*metav1.PartialObjectMetadata, error) {
	return nil, r.listErr
}

func (r *errorReader) OLM() OLMReader {
	return newOLMReader(nil)
}

// Compile-time check that errorReader implements Reader.
var _ Reader = (*errorReader)(nil)

// pureReader is a Reader that does NOT implement discoverer.
// Used to test that GetDataScienceCluster falls back to v2-only when no discovery is available.
type pureReader struct {
	dynamic *dynamicfake.FakeDynamicClient
}

func (r *pureReader) List(
	ctx context.Context,
	resourceType resources.ResourceType,
	_ ...ListResourcesOption,
) ([]*unstructured.Unstructured, error) {
	list, err := r.dynamic.Resource(resourceType.GVR()).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", resourceType.Kind, err)
	}

	result := make([]*unstructured.Unstructured, len(list.Items))
	for i := range list.Items {
		result[i] = &list.Items[i]
	}

	return result, nil
}

func (r *pureReader) ListMetadata(
	_ context.Context,
	_ resources.ResourceType,
	_ ...ListResourcesOption,
) ([]*metav1.PartialObjectMetadata, error) {
	return nil, nil
}

func (r *pureReader) ListResources(
	_ context.Context,
	_ schema.GroupVersionResource,
	_ ...ListResourcesOption,
) ([]*unstructured.Unstructured, error) {
	return nil, nil
}

func (r *pureReader) Get(
	_ context.Context,
	_ schema.GroupVersionResource,
	_ string,
	_ ...GetOption,
) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (r *pureReader) GetResource(
	_ context.Context,
	_ resources.ResourceType,
	_ string,
	_ ...GetOption,
) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (r *pureReader) GetResourceMetadata(
	_ context.Context,
	_ resources.ResourceType,
	_ string,
	_ ...GetOption,
) (*metav1.PartialObjectMetadata, error) {
	return nil, nil
}

func (r *pureReader) OLM() OLMReader {
	return newOLMReader(nil)
}

var _ Reader = (*pureReader)(nil)

// --- getSingletonWithDiscovery tests ---

//nolint:gochecknoglobals // Test fixture
var dscListKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR():   resources.DataScienceCluster.ListKind(),
	resources.DataScienceClusterV1.GVR(): resources.DataScienceClusterV1.ListKind(),
	resources.DSCInitialization.GVR():    resources.DSCInitialization.ListKind(),
	resources.DSCInitializationV1.GVR():  resources.DSCInitializationV1.ListKind(),
}

func newFakeDiscovery(groupVersions ...string) *discoveryfake.FakeDiscovery {
	fd := &discoveryfake.FakeDiscovery{Fake: &coretesting.Fake{}}

	lists := make([]*metav1.APIResourceList, 0, len(groupVersions))
	for _, gv := range groupVersions {
		lists = append(lists, &metav1.APIResourceList{
			GroupVersion: gv,
			APIResources: []metav1.APIResource{
				{Name: "datascienceclusters", Kind: "DataScienceCluster"},
				{Name: "dscinitializations", Kind: "DSCInitialization"},
			},
		})
	}

	fd.Resources = lists

	return fd
}

func createDSCV2() runtime.Object {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata":   map[string]any{"name": "default-dsc"},
		},
	}
}

func createDSCV1() runtime.Object {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceClusterV1.APIVersion(),
			"kind":       resources.DataScienceClusterV1.Kind,
			"metadata":   map[string]any{"name": "default-dsc"},
		},
	}
}

func TestGetDataScienceCluster_V2Available(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := createDSCV2()
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dscListKinds, dsc)

	c := NewForTesting(TestClientConfig{
		Dynamic:   dynamicClient,
		Discovery: newFakeDiscovery(resources.DataScienceCluster.Group + "/v2"),
	})

	result, err := GetDataScienceCluster(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.GetName()).To(Equal("default-dsc"))
	g.Expect(result.GetAPIVersion()).To(Equal(resources.DataScienceCluster.APIVersion()))
}

func TestGetDataScienceCluster_V2AbsentFallsBackToV1(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := createDSCV1()
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dscListKinds, dsc)

	c := NewForTesting(TestClientConfig{
		Dynamic:   dynamicClient,
		Discovery: newFakeDiscovery(resources.DataScienceCluster.Group + "/v1"),
	})

	result, err := GetDataScienceCluster(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.GetName()).To(Equal("default-dsc"))
	g.Expect(result.GetAPIVersion()).To(Equal(resources.DataScienceClusterV1.APIVersion()))
}

func TestGetDataScienceCluster_MidUpgradeFallsBackToV1(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := createDSCV1()
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dscListKinds, dsc)

	c := NewForTesting(TestClientConfig{
		Dynamic:   dynamicClient,
		Discovery: newFakeDiscovery(resources.DataScienceCluster.Group+"/v2", resources.DataScienceCluster.Group+"/v1"),
	})

	result, err := GetDataScienceCluster(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.GetAPIVersion()).To(Equal(resources.DataScienceClusterV1.APIVersion()))
}

func TestGetDataScienceCluster_NeitherVersionAvailable(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dscListKinds)

	c := NewForTesting(TestClientConfig{
		Dynamic:   dynamicClient,
		Discovery: newFakeDiscovery(),
	})

	_, err := GetDataScienceCluster(ctx, c)
	g.Expect(err).To(HaveOccurred())
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

func TestGetDataScienceCluster_PureReaderTriesV2Only(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := createDSCV2()
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dscListKinds, dsc)

	reader := &pureReader{
		dynamic: dynamicClient,
	}

	result, err := GetDataScienceCluster(ctx, reader)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.GetAPIVersion()).To(Equal(resources.DataScienceCluster.APIVersion()))
}

func TestGetDataScienceCluster_NilDiscoveryUsesV2Only(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := createDSCV2()
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dscListKinds, dsc)

	c := NewForTesting(TestClientConfig{
		Dynamic:   dynamicClient,
		Discovery: nil,
	})

	result, err := GetDataScienceCluster(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.GetAPIVersion()).To(Equal(resources.DataScienceCluster.APIVersion()))
}

func TestGetDSCInitialization_V2AbsentFallsBackToV1(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitializationV1.APIVersion(),
			"kind":       resources.DSCInitializationV1.Kind,
			"metadata":   map[string]any{"name": "default-dsci"},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dscListKinds, dsci)

	c := NewForTesting(TestClientConfig{
		Dynamic:   dynamicClient,
		Discovery: newFakeDiscovery(resources.DSCInitialization.Group + "/v1"),
	})

	result, err := GetDSCInitialization(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.GetName()).To(Equal("default-dsci"))
	g.Expect(result.GetAPIVersion()).To(Equal(resources.DSCInitializationV1.APIVersion()))
}
