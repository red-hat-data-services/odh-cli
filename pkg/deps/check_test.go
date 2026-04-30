package deps_test

import (
	"testing"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/mock"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/deps"
	mockclient "github.com/opendatahub-io/odh-cli/pkg/util/test/mocks/client"

	. "github.com/onsi/gomega"
)

func TestCheckDependencies_OLMNotAvailable(t *testing.T) {
	g := NewWithT(t)

	mockOLM := &mockclient.MockOLMReader{}
	mockOLM.On("Available").Return(false)

	manifest := &deps.Manifest{
		Dependencies: map[string]deps.Dependency{
			"certManager": {Enabled: "true"},
		},
	}

	_, err := deps.CheckDependencies(t.Context(), mockOLM, manifest)

	g.Expect(err).To(MatchError(deps.ErrOLMNotAvailable))
	mockOLM.AssertExpectations(t)
}

func TestCheckDependencies_MissingDependency(t *testing.T) {
	g := NewWithT(t)

	mockSubReader := &mockclient.MockSubscriptionReader{}
	mockSubReader.On("Get", mock.Anything, "cert-manager", mock.Anything).
		Return(nil, apierrors.NewNotFound(schema.GroupResource{Group: "operators.coreos.com", Resource: "subscriptions"}, "cert-manager"))

	mockOLM := &mockclient.MockOLMReader{}
	mockOLM.On("Available").Return(true)
	mockOLM.On("Subscriptions", "cert-manager").Return(mockSubReader)

	manifest := &deps.Manifest{
		Dependencies: map[string]deps.Dependency{
			"certManager": {
				Enabled: "true",
				OLM: deps.OLMConfig{
					Name:      "cert-manager",
					Namespace: "cert-manager",
				},
			},
		},
	}

	statuses, err := deps.CheckDependencies(t.Context(), mockOLM, manifest)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(statuses).To(HaveLen(1))
	g.Expect(statuses[0].Status).To(Equal(deps.StatusMissing))

	mockOLM.AssertExpectations(t)
	mockSubReader.AssertExpectations(t)
}

func TestCheckDependencies_OptionalStatus(t *testing.T) {
	tests := []struct {
		name    string
		enabled string
		want    deps.Status
	}{
		{"auto is optional", "auto", deps.StatusOptional},
		{"false is optional", "false", deps.StatusOptional},
		{"true is missing", "true", deps.StatusMissing},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			mockSubReader := &mockclient.MockSubscriptionReader{}
			mockSubReader.On("Get", mock.Anything, "servicemeshoperator", mock.Anything).
				Return(nil, apierrors.NewNotFound(schema.GroupResource{Group: "operators.coreos.com", Resource: "subscriptions"}, "servicemeshoperator"))

			mockOLM := &mockclient.MockOLMReader{}
			mockOLM.On("Available").Return(true)
			mockOLM.On("Subscriptions", "openshift-operators").Return(mockSubReader)

			manifest := &deps.Manifest{
				Dependencies: map[string]deps.Dependency{
					"serviceMesh": {
						Enabled: tt.enabled,
						OLM: deps.OLMConfig{
							Name:      "servicemeshoperator",
							Namespace: "openshift-operators",
						},
					},
				},
			}

			statuses, err := deps.CheckDependencies(t.Context(), mockOLM, manifest)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(statuses[0].Status).To(Equal(tt.want))

			mockOLM.AssertExpectations(t)
		})
	}
}

func TestCheckDependencies_Installed(t *testing.T) {
	g := NewWithT(t)

	mockSubReader := &mockclient.MockSubscriptionReader{}
	mockSubReader.On("Get", mock.Anything, "cert-manager", mock.Anything).
		Return(&operatorsv1alpha1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cert-manager",
				Namespace: "cert-manager",
			},
			Status: operatorsv1alpha1.SubscriptionStatus{
				InstalledCSV: "cert-manager.v1.14.0",
			},
		}, nil)

	mockCSVReader := &mockclient.MockCSVReader{}
	mockCSVReader.On("Get", mock.Anything, "cert-manager.v1.14.0", mock.Anything).
		Return(nil, apierrors.NewNotFound(schema.GroupResource{Group: "operators.coreos.com", Resource: "clusterserviceversions"}, "cert-manager.v1.14.0"))

	mockOLM := &mockclient.MockOLMReader{}
	mockOLM.On("Available").Return(true)
	mockOLM.On("Subscriptions", "cert-manager").Return(mockSubReader)
	mockOLM.On("ClusterServiceVersions", "cert-manager").Return(mockCSVReader)

	manifest := &deps.Manifest{
		Dependencies: map[string]deps.Dependency{
			"certManager": {
				Enabled: "true",
				OLM: deps.OLMConfig{
					Name:      "cert-manager",
					Namespace: "cert-manager",
				},
			},
		},
	}

	statuses, err := deps.CheckDependencies(t.Context(), mockOLM, manifest)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(statuses).To(HaveLen(1))
	g.Expect(statuses[0].Status).To(Equal(deps.StatusInstalled))
	g.Expect(statuses[0].Name).To(Equal("certManager"))
	g.Expect(statuses[0].DisplayName).To(Equal("Cert Manager"))

	mockOLM.AssertExpectations(t)
}

func TestCheckDependencies_EmptyNamespace(t *testing.T) {
	g := NewWithT(t)

	mockOLM := &mockclient.MockOLMReader{}
	mockOLM.On("Available").Return(true)

	manifest := &deps.Manifest{
		Dependencies: map[string]deps.Dependency{
			"test": {
				Enabled: "true",
				OLM: deps.OLMConfig{
					Name:      "test-sub",
					Namespace: "",
				},
			},
		},
	}

	statuses, err := deps.CheckDependencies(t.Context(), mockOLM, manifest)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(statuses[0].Status).To(Equal(deps.StatusMissing))

	mockOLM.AssertExpectations(t)
}

func TestCheckDependencies_EmptySubscription(t *testing.T) {
	g := NewWithT(t)

	mockOLM := &mockclient.MockOLMReader{}
	mockOLM.On("Available").Return(true)

	manifest := &deps.Manifest{
		Dependencies: map[string]deps.Dependency{
			"test": {
				Enabled: "true",
				OLM: deps.OLMConfig{
					Name:      "",
					Namespace: "test-ns",
				},
			},
		},
	}

	statuses, err := deps.CheckDependencies(t.Context(), mockOLM, manifest)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(statuses[0].Status).To(Equal(deps.StatusMissing))

	mockOLM.AssertExpectations(t)
}

func TestNewDependencyList_EnvelopeFields(t *testing.T) {
	g := NewWithT(t)

	statuses := []deps.DependencyStatus{
		{Name: "certManager", DisplayName: "Cert Manager", Status: deps.StatusInstalled},
		{Name: "serviceMesh", DisplayName: "Service Mesh", Status: deps.StatusMissing},
	}

	list := deps.NewDependencyList(statuses)

	g.Expect(list.APIVersion).To(Equal("cli.opendatahub.io/v1"))
	g.Expect(list.Kind).To(Equal("DependencyList"))
	g.Expect(list.Metadata.Command).To(Equal("deps"))
	g.Expect(list.Metadata.CLIVersion).ToNot(BeEmpty())
	g.Expect(list.Metadata.GeneratedAt).ToNot(BeEmpty())
	g.Expect(list.Status).ToNot(BeNil())
	g.Expect(list.Dependencies).To(HaveLen(2))
}

func TestNewDependencyList_ComputesStatus(t *testing.T) {
	t.Run("success when all installed", func(t *testing.T) {
		g := NewWithT(t)

		statuses := []deps.DependencyStatus{
			{Name: "certManager", Status: deps.StatusInstalled},
			{Name: "serviceMesh", Status: deps.StatusOptional},
		}

		list := deps.NewDependencyList(statuses)

		g.Expect(list.Status.Result).To(Equal("success"))
		g.Expect(list.Status.Errors).To(Equal(0))
		g.Expect(list.Status.Warnings).To(Equal(0))
	})

	t.Run("failure when missing", func(t *testing.T) {
		g := NewWithT(t)

		statuses := []deps.DependencyStatus{
			{Name: "certManager", Status: deps.StatusMissing},
		}

		list := deps.NewDependencyList(statuses)

		g.Expect(list.Status.Result).To(Equal("failure"))
		g.Expect(list.Status.Errors).To(Equal(1))
	})

	t.Run("warning when unknown", func(t *testing.T) {
		g := NewWithT(t)

		statuses := []deps.DependencyStatus{
			{Name: "certManager", Status: deps.StatusUnknown},
		}

		list := deps.NewDependencyList(statuses)

		g.Expect(list.Status.Result).To(Equal("warning"))
		g.Expect(list.Status.Warnings).To(Equal(1))
	})
}
