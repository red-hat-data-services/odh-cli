//nolint:testpackage // Tests internal implementation (namespace discovery functions)
package status

import (
	"errors"
	"testing"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/mock"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	mockclient "github.com/opendatahub-io/odh-cli/pkg/util/test/mocks/client"

	. "github.com/onsi/gomega"
)

// notFoundErr creates a proper Kubernetes NotFound error for deployment lookups.
func notFoundErr(name string) error {
	return apierrors.NewNotFound(schema.GroupResource{Resource: "deployments"}, name)
}

func TestDiscoverAppsNamespace(t *testing.T) {
	t.Run("override takes priority", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := discoverAppsNamespace(nil, "my-override-ns")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ns).To(Equal("my-override-ns"))
	})

	t.Run("returns error when DSCI is nil and no override", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := discoverAppsNamespace(nil, "")

		g.Expect(err).To(HaveOccurred())
		g.Expect(ns).To(BeEmpty())
	})

	t.Run("reads namespace from DSCI spec", func(t *testing.T) {
		g := NewWithT(t)

		dsci := &unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{
					"applicationsNamespace": "redhat-ods-applications",
				},
			},
		}

		ns, err := discoverAppsNamespace(dsci, "")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ns).To(Equal("redhat-ods-applications"))
	})

	t.Run("returns error when DSCI missing applicationsNamespace", func(t *testing.T) {
		g := NewWithT(t)

		dsci := &unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{},
			},
		}

		ns, err := discoverAppsNamespace(dsci, "")

		g.Expect(err).To(HaveOccurred())
		g.Expect(ns).To(BeEmpty())
	})
}

func TestDiscoverMonitoringNamespace(t *testing.T) {
	t.Run("returns apps namespace when DSCI is nil", func(t *testing.T) {
		g := NewWithT(t)

		ns := discoverMonitoringNamespace(nil, "redhat-ods-applications")

		g.Expect(ns).To(Equal("redhat-ods-applications"))
	})

	t.Run("returns apps namespace when monitoring not configured", func(t *testing.T) {
		g := NewWithT(t)

		dsci := &unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{},
			},
		}

		ns := discoverMonitoringNamespace(dsci, "redhat-ods-applications")

		g.Expect(ns).To(Equal("redhat-ods-applications"))
	})

	t.Run("reads monitoring namespace from DSCI spec", func(t *testing.T) {
		g := NewWithT(t)

		dsci := &unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{
					"monitoring": map[string]any{
						"namespace": "redhat-ods-monitoring",
					},
				},
			},
		}

		ns := discoverMonitoringNamespace(dsci, "redhat-ods-applications")

		g.Expect(ns).To(Equal("redhat-ods-monitoring"))
	})

	t.Run("returns apps namespace when monitoring namespace is empty", func(t *testing.T) {
		g := NewWithT(t)

		dsci := &unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{
					"monitoring": map[string]any{
						"namespace": "",
					},
				},
			},
		}

		ns := discoverMonitoringNamespace(dsci, "redhat-ods-applications")

		g.Expect(ns).To(Equal("redhat-ods-applications"))
	})
}

func TestDiscoverOperatorName(t *testing.T) {
	t.Run("override takes priority", func(t *testing.T) {
		g := NewWithT(t)

		name := discoverOperatorName(&client.OperatorInfo{DeploymentName: "from-olm"}, "my-override")

		g.Expect(name).To(Equal("my-override"))
	})

	t.Run("uses operator info deployment name", func(t *testing.T) {
		g := NewWithT(t)

		name := discoverOperatorName(&client.OperatorInfo{DeploymentName: "opendatahub-operator-controller-manager"}, "")

		g.Expect(name).To(Equal("opendatahub-operator-controller-manager"))
	})

	t.Run("returns default when operator info is nil", func(t *testing.T) {
		g := NewWithT(t)

		name := discoverOperatorName(nil, "")

		g.Expect(name).To(Equal(defaultRHOAIOperatorName))
	})

	t.Run("returns default when deployment name is empty", func(t *testing.T) {
		g := NewWithT(t)

		name := discoverOperatorName(&client.OperatorInfo{}, "")

		g.Expect(name).To(Equal(defaultRHOAIOperatorName))
	})
}

func TestDiscoverOperatorNamespace(t *testing.T) {
	t.Run("override takes priority", func(t *testing.T) {
		g := NewWithT(t)

		mockReader := &mockclient.MockReader{}

		ns, err := discoverOperatorNamespace(t.Context(), mockReader, nil, "my-override-ns")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ns).To(Equal("my-override-ns"))
	})

	t.Run("uses namespace from operator info", func(t *testing.T) {
		g := NewWithT(t)

		mockReader := &mockclient.MockReader{}

		ns, err := discoverOperatorNamespace(t.Context(), mockReader, &client.OperatorInfo{Namespace: "openshift-operators"}, "")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ns).To(Equal("openshift-operators"))
	})

	t.Run("falls back to defaults when operator info is nil", func(t *testing.T) {
		g := NewWithT(t)

		mockReader := &mockclient.MockReader{}
		mockReader.On("GetResource", mock.Anything, resources.Deployment, "rhods-operator", mock.Anything).
			Return(&unstructured.Unstructured{}, nil).Once()

		ns, err := discoverOperatorNamespace(t.Context(), mockReader, nil, "")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ns).To(Equal(client.DefaultRHOAIOperatorNamespace))
		mockReader.AssertExpectations(t)
	})
}

func TestDiscoverOperatorNamespaceWithInfoFallback(t *testing.T) {
	t.Run("finds rhods-operator in redhat-ods-operator namespace", func(t *testing.T) {
		g := NewWithT(t)

		mockReader := &mockclient.MockReader{}
		mockReader.On("GetResource", mock.Anything, resources.Deployment, "rhods-operator", mock.Anything).
			Return(&unstructured.Unstructured{}, nil).Once()

		ns, err := client.DiscoverOperatorNamespaceWithInfo(t.Context(), mockReader, nil)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ns).To(Equal(client.DefaultRHOAIOperatorNamespace))
		mockReader.AssertExpectations(t)
	})

	t.Run("finds odh operator in opendatahub namespace", func(t *testing.T) {
		g := NewWithT(t)

		mockReader := &mockclient.MockReader{}
		mockReader.On("GetResource", mock.Anything, resources.Deployment, "rhods-operator", mock.Anything).
			Return(nil, notFoundErr("rhods-operator")).Times(2)
		mockReader.On("GetResource", mock.Anything, resources.Deployment, "opendatahub-operator-controller-manager", mock.Anything).
			Return(nil, notFoundErr("opendatahub-operator-controller-manager")).Once()
		mockReader.On("GetResource", mock.Anything, resources.Deployment, "opendatahub-operator-controller-manager", mock.Anything).
			Return(&unstructured.Unstructured{}, nil).Once()

		ns, err := client.DiscoverOperatorNamespaceWithInfo(t.Context(), mockReader, nil)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ns).To(Equal(client.DefaultODHOperatorNamespace))
		mockReader.AssertExpectations(t)
	})

	t.Run("finds operator in openshift-operators namespace", func(t *testing.T) {
		g := NewWithT(t)

		mockReader := &mockclient.MockReader{}
		mockReader.On("GetResource", mock.Anything, resources.Deployment, "rhods-operator", mock.Anything).
			Return(nil, notFoundErr("rhods-operator")).Times(3)
		mockReader.On("GetResource", mock.Anything, resources.Deployment, "opendatahub-operator-controller-manager", mock.Anything).
			Return(nil, notFoundErr("opendatahub-operator-controller-manager")).Times(2)
		mockReader.On("GetResource", mock.Anything, resources.Deployment, "opendatahub-operator-controller-manager", mock.Anything).
			Return(&unstructured.Unstructured{}, nil).Once()

		ns, err := client.DiscoverOperatorNamespaceWithInfo(t.Context(), mockReader, nil)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ns).To(Equal(client.DefaultOpenShiftOperatorsNS))
		mockReader.AssertExpectations(t)
	})

	t.Run("returns error when no operator found in any namespace", func(t *testing.T) {
		g := NewWithT(t)

		mockReader := &mockclient.MockReader{}
		mockReader.On("GetResource", mock.Anything, resources.Deployment, mock.Anything, mock.Anything).
			Return(nil, notFoundErr("operator"))

		ns, err := client.DiscoverOperatorNamespaceWithInfo(t.Context(), mockReader, nil)

		g.Expect(err).To(HaveOccurred())
		g.Expect(ns).To(BeEmpty())
		mockReader.AssertExpectations(t)
	})
}

func TestDiscoverOperatorFromOLM(t *testing.T) {
	t.Run("returns nil when OLM not available", func(t *testing.T) {
		g := NewWithT(t)

		mockOLM := &mockclient.MockOLMReader{}
		mockOLM.On("Available").Return(false)

		mockReader := &mockclient.MockReader{}
		mockReader.On("OLM").Return(mockOLM)

		info, err := client.DiscoverOperatorFromOLM(t.Context(), mockReader)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(info).To(BeNil())
		mockOLM.AssertExpectations(t)
	})

	t.Run("returns error when CSV list fails", func(t *testing.T) {
		g := NewWithT(t)

		mockCSVReader := &mockclient.MockCSVReader{}
		mockCSVReader.On("List", mock.Anything, mock.Anything).
			Return(nil, errors.New("api error"))

		mockOLM := &mockclient.MockOLMReader{}
		mockOLM.On("Available").Return(true)
		mockOLM.On("ClusterServiceVersions", "").Return(mockCSVReader)

		mockReader := &mockclient.MockReader{}
		mockReader.On("OLM").Return(mockOLM)

		info, err := client.DiscoverOperatorFromOLM(t.Context(), mockReader)

		g.Expect(err).To(HaveOccurred())
		g.Expect(info).To(BeNil())
		mockReader.AssertExpectations(t)
		mockOLM.AssertExpectations(t)
		mockCSVReader.AssertExpectations(t)
	})

	t.Run("returns nil when no matching CSV found", func(t *testing.T) {
		g := NewWithT(t)

		mockCSVReader := &mockclient.MockCSVReader{}
		mockCSVReader.On("List", mock.Anything, mock.Anything).
			Return(&operatorsv1alpha1.ClusterServiceVersionList{
				Items: []operatorsv1alpha1.ClusterServiceVersion{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "some-other-operator.v1.0.0",
							Namespace: "openshift-operators",
						},
					},
				},
			}, nil)

		mockOLM := &mockclient.MockOLMReader{}
		mockOLM.On("Available").Return(true)
		mockOLM.On("ClusterServiceVersions", "").Return(mockCSVReader)

		mockReader := &mockclient.MockReader{}
		mockReader.On("OLM").Return(mockOLM)

		info, err := client.DiscoverOperatorFromOLM(t.Context(), mockReader)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(info).To(BeNil())
		mockReader.AssertExpectations(t)
		mockOLM.AssertExpectations(t)
		mockCSVReader.AssertExpectations(t)
	})

	t.Run("finds rhods-operator CSV", func(t *testing.T) {
		g := NewWithT(t)

		mockCSVReader := &mockclient.MockCSVReader{}
		mockCSVReader.On("List", mock.Anything, mock.Anything).
			Return(&operatorsv1alpha1.ClusterServiceVersionList{
				Items: []operatorsv1alpha1.ClusterServiceVersion{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "rhods-operator.v2.15.0",
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
			}, nil)

		mockOLM := &mockclient.MockOLMReader{}
		mockOLM.On("Available").Return(true)
		mockOLM.On("ClusterServiceVersions", "").Return(mockCSVReader)

		mockReader := &mockclient.MockReader{}
		mockReader.On("OLM").Return(mockOLM)

		info, err := client.DiscoverOperatorFromOLM(t.Context(), mockReader)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(info).ToNot(BeNil())
		g.Expect(info.Namespace).To(Equal("redhat-ods-operator"))
		g.Expect(info.DeploymentName).To(Equal("rhods-operator"))
		mockReader.AssertExpectations(t)
		mockOLM.AssertExpectations(t)
		mockCSVReader.AssertExpectations(t)
	})

	t.Run("finds opendatahub-operator CSV", func(t *testing.T) {
		g := NewWithT(t)

		mockCSVReader := &mockclient.MockCSVReader{}
		mockCSVReader.On("List", mock.Anything, mock.Anything).
			Return(&operatorsv1alpha1.ClusterServiceVersionList{
				Items: []operatorsv1alpha1.ClusterServiceVersion{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "opendatahub-operator.v2.10.0",
							Namespace: "openshift-operators",
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
			}, nil)

		mockOLM := &mockclient.MockOLMReader{}
		mockOLM.On("Available").Return(true)
		mockOLM.On("ClusterServiceVersions", "").Return(mockCSVReader)

		mockReader := &mockclient.MockReader{}
		mockReader.On("OLM").Return(mockOLM)

		info, err := client.DiscoverOperatorFromOLM(t.Context(), mockReader)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(info).ToNot(BeNil())
		g.Expect(info.Namespace).To(Equal("openshift-operators"))
		g.Expect(info.DeploymentName).To(Equal("opendatahub-operator-controller-manager"))
		mockReader.AssertExpectations(t)
		mockOLM.AssertExpectations(t)
		mockCSVReader.AssertExpectations(t)
	})

	t.Run("uses olm.copiedFrom label for original namespace", func(t *testing.T) {
		g := NewWithT(t)

		mockCSVReader := &mockclient.MockCSVReader{}
		mockCSVReader.On("List", mock.Anything, mock.Anything).
			Return(&operatorsv1alpha1.ClusterServiceVersionList{
				Items: []operatorsv1alpha1.ClusterServiceVersion{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "rhods-operator.v2.15.0",
							Namespace: "redhat-ods-applications",
							Labels: map[string]string{
								"olm.copiedFrom": "redhat-ods-operator",
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
			}, nil)

		mockOLM := &mockclient.MockOLMReader{}
		mockOLM.On("Available").Return(true)
		mockOLM.On("ClusterServiceVersions", "").Return(mockCSVReader)

		mockReader := &mockclient.MockReader{}
		mockReader.On("OLM").Return(mockOLM)

		info, err := client.DiscoverOperatorFromOLM(t.Context(), mockReader)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(info).ToNot(BeNil())
		g.Expect(info.Namespace).To(Equal("redhat-ods-operator"))
		g.Expect(info.DeploymentName).To(Equal("rhods-operator"))
		mockReader.AssertExpectations(t)
		mockOLM.AssertExpectations(t)
		mockCSVReader.AssertExpectations(t)
	})

	t.Run("handles CSV with no deployments", func(t *testing.T) {
		g := NewWithT(t)

		mockCSVReader := &mockclient.MockCSVReader{}
		mockCSVReader.On("List", mock.Anything, mock.Anything).
			Return(&operatorsv1alpha1.ClusterServiceVersionList{
				Items: []operatorsv1alpha1.ClusterServiceVersion{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "rhods-operator.v2.15.0",
							Namespace: "redhat-ods-operator",
						},
						Spec: operatorsv1alpha1.ClusterServiceVersionSpec{
							InstallStrategy: operatorsv1alpha1.NamedInstallStrategy{
								StrategySpec: operatorsv1alpha1.StrategyDetailsDeployment{
									DeploymentSpecs: []operatorsv1alpha1.StrategyDeploymentSpec{},
								},
							},
						},
					},
				},
			}, nil)

		mockOLM := &mockclient.MockOLMReader{}
		mockOLM.On("Available").Return(true)
		mockOLM.On("ClusterServiceVersions", "").Return(mockCSVReader)

		mockReader := &mockclient.MockReader{}
		mockReader.On("OLM").Return(mockOLM)

		info, err := client.DiscoverOperatorFromOLM(t.Context(), mockReader)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(info).ToNot(BeNil())
		g.Expect(info.Namespace).To(Equal("redhat-ods-operator"))
		g.Expect(info.DeploymentName).To(BeEmpty())
		mockReader.AssertExpectations(t)
		mockOLM.AssertExpectations(t)
		mockCSVReader.AssertExpectations(t)
	})
}
