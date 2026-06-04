//nolint:testpackage // Tests internal implementation
package client

import (
	"context"
	"errors"
	"testing"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util"

	. "github.com/onsi/gomega"
)

// --- Mock OLM Reader ---

type mockOLMReader struct {
	available bool
	csvReader *mockCSVReader
}

func (m *mockOLMReader) Available() bool {
	return m.available
}

func (m *mockOLMReader) Subscriptions(_ string) SubscriptionReader {
	return nil
}

func (m *mockOLMReader) ClusterServiceVersions(_ string) CSVReader {
	return m.csvReader
}

// mockCSVReader implements CSVReader for testing.
type mockCSVReader struct {
	csvList *operatorsv1alpha1.ClusterServiceVersionList
	listErr error
}

func (m *mockCSVReader) List(_ context.Context, _ metav1.ListOptions) (*operatorsv1alpha1.ClusterServiceVersionList, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}

	return m.csvList, nil
}

func (m *mockCSVReader) Get(_ context.Context, _ string, _ metav1.GetOptions) (*operatorsv1alpha1.ClusterServiceVersion, error) {
	return nil, nil
}

// --- Mock Reader for discovery tests ---

// namespaceName tracks a queried namespace/deployment pair.
type namespaceName struct {
	namespace string
	name      string
}

type mockDiscoveryReader struct {
	olmReader    OLMReader
	deployments  map[string]map[string]bool // namespace -> deployment name -> exists
	getErr       error                      // error to return for all Get calls
	forbiddenNS  map[string]bool            // namespaces that return Forbidden
	queriedPairs []namespaceName            // tracks namespace/name pairs queried
}

func (m *mockDiscoveryReader) OLM() OLMReader {
	return m.olmReader
}

func (m *mockDiscoveryReader) GetResource(
	_ context.Context,
	resourceType resources.ResourceType,
	name string,
	opts ...GetOption,
) (*unstructured.Unstructured, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}

	// Only handle Deployment resources
	if resourceType != resources.Deployment {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "unknown"}, name)
	}

	// Extract namespace from options
	cfg := &GetConfig{}
	util.ApplyOptions(cfg, opts...)

	ns := cfg.Namespace

	// Track the query
	m.queriedPairs = append(m.queriedPairs, namespaceName{namespace: ns, name: name})

	// Check if namespace returns Forbidden
	if m.forbiddenNS != nil && m.forbiddenNS[ns] {
		return nil, apierrors.NewForbidden(schema.GroupResource{Resource: "deployments"}, name, errors.New("forbidden"))
	}

	// Check if deployment exists
	if m.deployments != nil {
		if nsDeployments, ok := m.deployments[ns]; ok {
			if nsDeployments[name] {
				return &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]any{
							"name":      name,
							"namespace": ns,
						},
					},
				}, nil
			}
		}
	}

	return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "deployments"}, name)
}

// Implement remaining Reader interface methods (unused in discovery tests).
func (m *mockDiscoveryReader) List(_ context.Context, _ resources.ResourceType, _ ...ListResourcesOption) ([]*unstructured.Unstructured, error) {
	return nil, nil
}

func (m *mockDiscoveryReader) ListMetadata(_ context.Context, _ resources.ResourceType, _ ...ListResourcesOption) ([]*metav1.PartialObjectMetadata, error) {
	return nil, nil
}

func (m *mockDiscoveryReader) ListResources(_ context.Context, _ schema.GroupVersionResource, _ ...ListResourcesOption) ([]*unstructured.Unstructured, error) {
	return nil, nil
}

func (m *mockDiscoveryReader) Get(_ context.Context, _ schema.GroupVersionResource, _ string, _ ...GetOption) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (m *mockDiscoveryReader) GetResourceMetadata(_ context.Context, _ resources.ResourceType, _ string, _ ...GetOption) (*metav1.PartialObjectMetadata, error) {
	return nil, nil
}

// Compile-time check that mockDiscoveryReader implements Reader.
var _ Reader = (*mockDiscoveryReader)(nil)

// --- Tests for DiscoverOperatorFromOLM ---

func TestDiscoverOperatorFromOLM_OLMNotAvailable(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{available: false},
	}

	info, err := DiscoverOperatorFromOLM(ctx, reader)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(info).To(BeNil())
}

func TestDiscoverOperatorFromOLM_ListError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{
			available: true,
			csvReader: &mockCSVReader{
				listErr: errors.New("list failed"),
			},
		},
	}

	info, err := DiscoverOperatorFromOLM(ctx, reader)

	g.Expect(err).To(HaveOccurred())
	g.Expect(info).To(BeNil())
}

func TestDiscoverOperatorFromOLM_NoMatchingCSV(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{
			available: true,
			csvReader: &mockCSVReader{
				csvList: &operatorsv1alpha1.ClusterServiceVersionList{
					Items: []operatorsv1alpha1.ClusterServiceVersion{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "some-other-operator.v1.0.0",
								Namespace: "some-namespace",
							},
						},
					},
				},
			},
		},
	}

	info, err := DiscoverOperatorFromOLM(ctx, reader)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(info).To(BeNil())
}

func TestDiscoverOperatorFromOLM_RHODSOperatorFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{
			available: true,
			csvReader: &mockCSVReader{
				csvList: &operatorsv1alpha1.ClusterServiceVersionList{
					Items: []operatorsv1alpha1.ClusterServiceVersion{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "rhods-operator.v2.10.0",
								Namespace: "redhat-ods-operator",
							},
							Spec: operatorsv1alpha1.ClusterServiceVersionSpec{
								InstallStrategy: operatorsv1alpha1.NamedInstallStrategy{
									StrategySpec: operatorsv1alpha1.StrategyDetailsDeployment{
										DeploymentSpecs: []operatorsv1alpha1.StrategyDeploymentSpec{
											{Name: "rhods-operator"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	info, err := DiscoverOperatorFromOLM(ctx, reader)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(info).ToNot(BeNil())
	g.Expect(info.Namespace).To(Equal("redhat-ods-operator"))
	g.Expect(info.DeploymentName).To(Equal("rhods-operator"))
}

func TestDiscoverOperatorFromOLM_ODHOperatorFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{
			available: true,
			csvReader: &mockCSVReader{
				csvList: &operatorsv1alpha1.ClusterServiceVersionList{
					Items: []operatorsv1alpha1.ClusterServiceVersion{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "opendatahub-operator.v2.10.0",
								Namespace: "opendatahub",
							},
							Spec: operatorsv1alpha1.ClusterServiceVersionSpec{
								InstallStrategy: operatorsv1alpha1.NamedInstallStrategy{
									StrategySpec: operatorsv1alpha1.StrategyDetailsDeployment{
										DeploymentSpecs: []operatorsv1alpha1.StrategyDeploymentSpec{
											{Name: "opendatahub-operator-controller-manager"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	info, err := DiscoverOperatorFromOLM(ctx, reader)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(info).ToNot(BeNil())
	g.Expect(info.Namespace).To(Equal("opendatahub"))
	g.Expect(info.DeploymentName).To(Equal("opendatahub-operator-controller-manager"))
}

func TestDiscoverOperatorFromOLM_CopiedFromLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{
			available: true,
			csvReader: &mockCSVReader{
				csvList: &operatorsv1alpha1.ClusterServiceVersionList{
					Items: []operatorsv1alpha1.ClusterServiceVersion{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "rhods-operator.v2.10.0",
								Namespace: "redhat-ods-applications", // copied CSV
								Labels: map[string]string{
									"olm.copiedFrom": "redhat-ods-operator", // original namespace
								},
							},
							Spec: operatorsv1alpha1.ClusterServiceVersionSpec{
								InstallStrategy: operatorsv1alpha1.NamedInstallStrategy{
									StrategySpec: operatorsv1alpha1.StrategyDetailsDeployment{
										DeploymentSpecs: []operatorsv1alpha1.StrategyDeploymentSpec{
											{Name: "rhods-operator"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	info, err := DiscoverOperatorFromOLM(ctx, reader)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(info).ToNot(BeNil())
	g.Expect(info.Namespace).To(Equal("redhat-ods-operator")) // should use copiedFrom
	g.Expect(info.DeploymentName).To(Equal("rhods-operator"))
}

// --- Tests for DiscoverOperatorNamespace ---

func TestDiscoverOperatorNamespace_UsesOLMDiscovery(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{
			available: true,
			csvReader: &mockCSVReader{
				csvList: &operatorsv1alpha1.ClusterServiceVersionList{
					Items: []operatorsv1alpha1.ClusterServiceVersion{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "rhods-operator.v2.10.0",
								Namespace: "redhat-ods-operator",
							},
						},
					},
				},
			},
		},
	}

	ns, err := DiscoverOperatorNamespace(ctx, reader)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(Equal("redhat-ods-operator"))
}

func TestDiscoverOperatorNamespace_FallsBackToDefaults(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{available: false},
		deployments: map[string]map[string]bool{
			DefaultODHOperatorNamespace: {
				"opendatahub-operator-controller-manager": true,
			},
		},
	}

	ns, err := DiscoverOperatorNamespace(ctx, reader)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(Equal(DefaultODHOperatorNamespace))
}

// --- Tests for DiscoverOperatorNamespaceWithInfo ---

func TestDiscoverOperatorNamespaceWithInfo_UsesProvidedInfo(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{available: false},
	}

	info := &OperatorInfo{
		Namespace:      "custom-operator-ns",
		DeploymentName: "my-operator",
	}

	ns, err := DiscoverOperatorNamespaceWithInfo(ctx, reader, info)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(Equal("custom-operator-ns"))
}

func TestDiscoverOperatorNamespaceWithInfo_FallsBackWhenInfoNil(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{available: false},
		deployments: map[string]map[string]bool{
			DefaultRHOAIOperatorNamespace: {
				"rhods-operator": true,
			},
		},
	}

	ns, err := DiscoverOperatorNamespaceWithInfo(ctx, reader, nil)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(Equal(DefaultRHOAIOperatorNamespace))
}

func TestDiscoverOperatorNamespaceWithInfo_FallsBackWhenNamespaceEmpty(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{available: false},
		deployments: map[string]map[string]bool{
			DefaultRHOAIOperatorNamespace: {
				"rhods-operator": true,
			},
		},
	}

	info := &OperatorInfo{
		Namespace:      "", // empty
		DeploymentName: "some-deployment",
	}

	ns, err := DiscoverOperatorNamespaceWithInfo(ctx, reader, info)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(Equal(DefaultRHOAIOperatorNamespace))
}

// --- Tests for discoverOperatorNamespaceFromDefaults ---

func TestDiscoverOperatorNamespaceFromDefaults_FindsRHOAIOperator(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{available: false},
		deployments: map[string]map[string]bool{
			DefaultRHOAIOperatorNamespace: {
				"rhods-operator": true,
			},
		},
	}

	ns, err := discoverOperatorNamespaceFromDefaults(ctx, reader)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(Equal(DefaultRHOAIOperatorNamespace))
}

func TestDiscoverOperatorNamespaceFromDefaults_FindsODHOperator(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{available: false},
		deployments: map[string]map[string]bool{
			DefaultODHOperatorNamespace: {
				"opendatahub-operator-controller-manager": true,
			},
		},
	}

	ns, err := discoverOperatorNamespaceFromDefaults(ctx, reader)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(Equal(DefaultODHOperatorNamespace))
}

func TestDiscoverOperatorNamespaceFromDefaults_FindsInOpenShiftOperators(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{available: false},
		deployments: map[string]map[string]bool{
			DefaultOpenShiftOperatorsNS: {
				"rhods-operator": true,
			},
		},
	}

	ns, err := discoverOperatorNamespaceFromDefaults(ctx, reader)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(Equal(DefaultOpenShiftOperatorsNS))
}

func TestDiscoverOperatorNamespaceFromDefaults_NotFoundError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader:   &mockOLMReader{available: false},
		deployments: map[string]map[string]bool{}, // no deployments
	}

	ns, err := discoverOperatorNamespaceFromDefaults(ctx, reader)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("could not discover operator namespace"))
	g.Expect(ns).To(BeEmpty())
}

func TestDiscoverOperatorNamespaceFromDefaults_ReturnsForbiddenError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{available: false},
		forbiddenNS: map[string]bool{
			DefaultRHOAIOperatorNamespace: true,
		},
		deployments: map[string]map[string]bool{}, // no deployments found in other namespaces
	}

	ns, err := discoverOperatorNamespaceFromDefaults(ctx, reader)

	g.Expect(err).To(HaveOccurred())
	g.Expect(apierrors.IsForbidden(err) || err.Error() != "").To(BeTrue())
	g.Expect(ns).To(BeEmpty())
}

func TestDiscoverOperatorNamespaceFromDefaults_SkipsNotFoundContinuesSearch(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// First two namespaces return NotFound, third has the deployment
	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{available: false},
		deployments: map[string]map[string]bool{
			// DefaultRHOAIOperatorNamespace: not found
			// DefaultODHOperatorNamespace: not found
			DefaultOpenShiftOperatorsNS: {
				"opendatahub-operator-controller-manager": true,
			},
		},
	}

	ns, err := discoverOperatorNamespaceFromDefaults(ctx, reader)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(Equal(DefaultOpenShiftOperatorsNS))
}

func TestDiscoverOperatorNamespaceFromDefaults_FirstHitCancelsSearch(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Deployment exists in first namespace - should stop searching immediately
	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{available: false},
		deployments: map[string]map[string]bool{
			DefaultRHOAIOperatorNamespace: {
				"rhods-operator": true,
			},
			// Also has deployment in other namespaces - should NOT be queried
			DefaultODHOperatorNamespace: {
				"opendatahub-operator-controller-manager": true,
			},
			DefaultOpenShiftOperatorsNS: {
				"rhods-operator": true,
			},
		},
	}

	ns, err := discoverOperatorNamespaceFromDefaults(ctx, reader)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(Equal(DefaultRHOAIOperatorNamespace))

	// Verify only one query was made (first hit cancels)
	g.Expect(reader.queriedPairs).To(HaveLen(1))
	g.Expect(reader.queriedPairs[0].namespace).To(Equal(DefaultRHOAIOperatorNamespace))
	g.Expect(reader.queriedPairs[0].name).To(Equal("rhods-operator"))
}

func TestDiscoverOperatorNamespaceFromDefaults_AllNotFoundExhaustiveSearch(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// No deployments anywhere - should try all combinations
	reader := &mockDiscoveryReader{
		olmReader:   &mockOLMReader{available: false},
		deployments: map[string]map[string]bool{},
	}

	ns, err := discoverOperatorNamespaceFromDefaults(ctx, reader)

	g.Expect(err).To(HaveOccurred())
	g.Expect(ns).To(BeEmpty())

	// Verify all 6 combinations were tried (3 namespaces × 2 deployment names)
	// Order: for each namespace, try both deployment names
	expectedQueries := []namespaceName{
		{DefaultRHOAIOperatorNamespace, "rhods-operator"},
		{DefaultRHOAIOperatorNamespace, "opendatahub-operator-controller-manager"},
		{DefaultODHOperatorNamespace, "rhods-operator"},
		{DefaultODHOperatorNamespace, "opendatahub-operator-controller-manager"},
		{DefaultOpenShiftOperatorsNS, "rhods-operator"},
		{DefaultOpenShiftOperatorsNS, "opendatahub-operator-controller-manager"},
	}

	g.Expect(reader.queriedPairs).To(HaveLen(6))
	g.Expect(reader.queriedPairs).To(Equal(expectedQueries))
}

func TestDiscoverOperatorNamespaceFromDefaults_ForbiddenThenFoundReturnsSuccess(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// First namespace returns Forbidden, but deployment exists in second namespace
	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{available: false},
		forbiddenNS: map[string]bool{
			DefaultRHOAIOperatorNamespace: true, // Forbidden here
		},
		deployments: map[string]map[string]bool{
			DefaultODHOperatorNamespace: {
				"opendatahub-operator-controller-manager": true, // Found here
			},
		},
	}

	ns, err := discoverOperatorNamespaceFromDefaults(ctx, reader)

	// Should succeed despite Forbidden in first namespace
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(Equal(DefaultODHOperatorNamespace))

	// Verify search continued past Forbidden namespace
	g.Expect(len(reader.queriedPairs)).To(BeNumerically(">", 2))
}

func TestDiscoverOperatorNamespaceFromDefaults_ForbiddenCapturedWhenNothingFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// First namespace returns Forbidden, nothing found in other namespaces
	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{available: false},
		forbiddenNS: map[string]bool{
			DefaultRHOAIOperatorNamespace: true,
		},
		deployments: map[string]map[string]bool{}, // Nothing found anywhere
	}

	ns, err := discoverOperatorNamespaceFromDefaults(ctx, reader)

	// Should return the Forbidden error (not ErrOperatorNamespaceNotFound)
	g.Expect(err).To(HaveOccurred())
	g.Expect(ns).To(BeEmpty())

	// The error should be related to Forbidden, not "not found"
	g.Expect(err.Error()).ToNot(ContainSubstring("could not discover operator namespace"))
}

func TestDiscoverOperatorNamespaceFromDefaults_SecondDeploymentNameInSameNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// First deployment name not found, but second deployment name exists in first namespace
	reader := &mockDiscoveryReader{
		olmReader: &mockOLMReader{available: false},
		deployments: map[string]map[string]bool{
			DefaultRHOAIOperatorNamespace: {
				// "rhods-operator": not found
				"opendatahub-operator-controller-manager": true, // Found on second try
			},
		},
	}

	ns, err := discoverOperatorNamespaceFromDefaults(ctx, reader)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(Equal(DefaultRHOAIOperatorNamespace))

	// Should have tried both deployment names in first namespace
	g.Expect(reader.queriedPairs).To(HaveLen(2))
	g.Expect(reader.queriedPairs[0].name).To(Equal("rhods-operator"))
	g.Expect(reader.queriedPairs[1].name).To(Equal("opendatahub-operator-controller-manager"))
}
