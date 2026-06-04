//nolint:testpackage // Tests internal implementation (discoverNamespaces)
package events

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	mockclient "github.com/opendatahub-io/odh-cli/pkg/util/test/mocks/client"

	. "github.com/onsi/gomega"
)

// dsciNotFoundErr creates a proper Kubernetes NotFound error for DSCI.
func dsciNotFoundErr() error {
	return apierrors.NewNotFound(schema.GroupResource{
		Group:    resources.DSCInitialization.Group,
		Resource: resources.DSCInitialization.Resource,
	}, "")
}

// forbiddenErr creates a proper Kubernetes Forbidden error.
func forbiddenErr(resource, name string) error {
	return apierrors.NewForbidden(schema.GroupResource{Resource: resource}, name, errors.New("forbidden"))
}

// createDSCI creates a test DSCInitialization object.
func createDSCI(appNS, monitoringNS string) *unstructured.Unstructured {
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

	if appNS != "" {
		spec := dsci.Object["spec"].(map[string]any)
		spec["applicationsNamespace"] = appNS
	}

	if monitoringNS != "" {
		spec := dsci.Object["spec"].(map[string]any)
		spec["monitoring"] = map[string]any{
			"namespace": monitoringNS,
		}
	}

	return dsci
}

// setupCommand creates a Command with mock client and IO streams.
func setupCommand(mockClient *mockclient.MockClient) (*Command, *bytes.Buffer) {
	var errBuf bytes.Buffer
	streams := iostreams.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &errBuf)

	cmd := &Command{
		IO:          streams,
		ConfigFlags: genericclioptions.NewConfigFlags(true),
		Client:      mockClient,
	}

	return cmd, &errBuf
}

func TestDiscoverNamespaces_DSCINotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	mockClient := &mockclient.MockClient{}

	// Both GetApplicationsNamespace and GetMonitoringNamespace call List for DSCI
	// Both return NotFound - this is handled gracefully
	mockClient.On("List", mock.Anything, resources.DSCInitialization, mock.Anything).
		Return(nil, dsciNotFoundErr())

	// Operator discovery - found in first namespace (no OLM needed)
	mockClient.On("GetResource", mock.Anything, resources.Deployment, "rhods-operator", mock.Anything).
		Return(&unstructured.Unstructured{}, nil).Once()

	cmd, _ := setupCommand(mockClient)
	err := cmd.discoverNamespaces(ctx)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cmd.ApplicationsNS).To(BeEmpty())
	g.Expect(cmd.MonitoringNS).To(BeEmpty())
	g.Expect(cmd.OperatorNS).To(Equal(client.DefaultRHOAIOperatorNamespace))
	mockClient.AssertExpectations(t)
}

func TestDiscoverNamespaces_DSCIForbidden(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	mockClient := &mockclient.MockClient{}

	// DSCI returns Forbidden error - warns but continues
	mockClient.On("List", mock.Anything, resources.DSCInitialization, mock.Anything).
		Return(nil, forbiddenErr("dscinitialization", ""))

	// Operator discovery still proceeds
	mockClient.On("GetResource", mock.Anything, resources.Deployment, "rhods-operator", mock.Anything).
		Return(&unstructured.Unstructured{}, nil).Once()

	cmd, errBuf := setupCommand(mockClient)
	err := cmd.discoverNamespaces(ctx)

	// Now warns instead of erroring - permissive error handling
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(errBuf.String()).To(ContainSubstring("Warning"))
	g.Expect(cmd.ApplicationsNS).To(BeEmpty())
	mockClient.AssertExpectations(t)
}

func TestDiscoverNamespaces_DSCIWithAllFields(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	mockClient := &mockclient.MockClient{}

	dsci := createDSCI("redhat-ods-applications", "redhat-ods-monitoring")
	// Both helpers call List for DSCI
	mockClient.On("List", mock.Anything, resources.DSCInitialization, mock.Anything).
		Return([]*unstructured.Unstructured{dsci}, nil)

	// Operator discovery - found in first namespace (no OLM needed)
	mockClient.On("GetResource", mock.Anything, resources.Deployment, "rhods-operator", mock.Anything).
		Return(&unstructured.Unstructured{}, nil).Once()

	cmd, _ := setupCommand(mockClient)
	err := cmd.discoverNamespaces(ctx)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cmd.ApplicationsNS).To(Equal("redhat-ods-applications"))
	g.Expect(cmd.MonitoringNS).To(Equal("redhat-ods-monitoring"))
	g.Expect(cmd.OperatorNS).To(Equal(client.DefaultRHOAIOperatorNamespace))
	mockClient.AssertExpectations(t)
}

func TestDiscoverNamespaces_DSCIWithoutMonitoring(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	mockClient := &mockclient.MockClient{}

	dsci := createDSCI("redhat-ods-applications", "")
	mockClient.On("List", mock.Anything, resources.DSCInitialization, mock.Anything).
		Return([]*unstructured.Unstructured{dsci}, nil)

	// Operator discovery - found in first namespace (no OLM needed)
	mockClient.On("GetResource", mock.Anything, resources.Deployment, "rhods-operator", mock.Anything).
		Return(&unstructured.Unstructured{}, nil).Once()

	cmd, _ := setupCommand(mockClient)
	err := cmd.discoverNamespaces(ctx)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cmd.ApplicationsNS).To(Equal("redhat-ods-applications"))
	g.Expect(cmd.MonitoringNS).To(Equal("redhat-ods-applications")) // Falls back to apps NS
	mockClient.AssertExpectations(t)
}

func TestDiscoverNamespaces_OperatorOverride(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	mockClient := &mockclient.MockClient{}

	// DSCI found
	dsci := createDSCI("redhat-ods-applications", "")
	mockClient.On("List", mock.Anything, resources.DSCInitialization, mock.Anything).
		Return([]*unstructured.Unstructured{dsci}, nil)

	// No operator discovery calls expected - override is set

	cmd, _ := setupCommand(mockClient)
	cmd.OperatorNSOverride = "custom-operator-ns"
	err := cmd.discoverNamespaces(ctx)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cmd.OperatorNS).To(Equal("custom-operator-ns"))
	mockClient.AssertExpectations(t)
}

func TestDiscoverNamespaces_OperatorNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	mockClient := &mockclient.MockClient{}
	mockOLM := &mockclient.MockOLMReader{}
	mockOLM.On("Available").Return(false)
	mockClient.On("OLM").Return(mockOLM)

	// DSCI found
	dsci := createDSCI("redhat-ods-applications", "")
	mockClient.On("List", mock.Anything, resources.DSCInitialization, mock.Anything).
		Return([]*unstructured.Unstructured{dsci}, nil)

	// Operator not found in any namespace - all GetResource calls return NotFound
	mockClient.On("GetResource", mock.Anything, resources.Deployment, mock.Anything, mock.Anything).
		Return(nil, apierrors.NewNotFound(schema.GroupResource{Resource: "deployments"}, ""))

	cmd, errBuf := setupCommand(mockClient)
	err := cmd.discoverNamespaces(ctx)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cmd.OperatorNS).To(Equal(client.DefaultRHOAIOperatorNamespace))
	g.Expect(errBuf.String()).To(ContainSubstring("Warning"))
	mockClient.AssertExpectations(t)
}

func TestDiscoverNamespaces_OperatorForbidden(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	mockClient := &mockclient.MockClient{}
	mockOLM := &mockclient.MockOLMReader{}
	mockOLM.On("Available").Return(false)
	mockClient.On("OLM").Return(mockOLM)

	// DSCI found
	dsci := createDSCI("redhat-ods-applications", "")
	mockClient.On("List", mock.Anything, resources.DSCInitialization, mock.Anything).
		Return([]*unstructured.Unstructured{dsci}, nil)

	// Operator discovery returns Forbidden
	mockClient.On("GetResource", mock.Anything, resources.Deployment, mock.Anything, mock.Anything).
		Return(nil, forbiddenErr("deployments", "rhods-operator"))

	cmd, _ := setupCommand(mockClient)
	err := cmd.discoverNamespaces(ctx)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("discovering operator namespace"))
	mockClient.AssertExpectations(t)
}
