package client

import (
	"context"

	"github.com/stretchr/testify/mock"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// MockReader is a mock implementation of client.Reader using testify/mock.
type MockReader struct {
	mock.Mock
}

var _ client.Reader = (*MockReader)(nil)

func (m *MockReader) List(
	ctx context.Context,
	resourceType resources.ResourceType,
	opts ...client.ListResourcesOption,
) ([]*unstructured.Unstructured, error) {
	args := m.Called(ctx, resourceType, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).([]*unstructured.Unstructured)

	return result, args.Error(1)
}

func (m *MockReader) ListMetadata(
	ctx context.Context,
	resourceType resources.ResourceType,
	opts ...client.ListResourcesOption,
) ([]*metav1.PartialObjectMetadata, error) {
	args := m.Called(ctx, resourceType, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).([]*metav1.PartialObjectMetadata)

	return result, args.Error(1)
}

func (m *MockReader) ListResources(
	ctx context.Context,
	gvr schema.GroupVersionResource,
	opts ...client.ListResourcesOption,
) ([]*unstructured.Unstructured, error) {
	args := m.Called(ctx, gvr, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).([]*unstructured.Unstructured)

	return result, args.Error(1)
}

func (m *MockReader) Get(
	ctx context.Context,
	gvr schema.GroupVersionResource,
	name string,
	opts ...client.GetOption,
) (*unstructured.Unstructured, error) {
	args := m.Called(ctx, gvr, name, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).(*unstructured.Unstructured)

	return result, args.Error(1)
}

func (m *MockReader) GetResource(
	ctx context.Context,
	resourceType resources.ResourceType,
	name string,
	opts ...client.GetOption,
) (*unstructured.Unstructured, error) {
	args := m.Called(ctx, resourceType, name, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).(*unstructured.Unstructured)

	return result, args.Error(1)
}

func (m *MockReader) GetResourceMetadata(
	ctx context.Context,
	resourceType resources.ResourceType,
	name string,
	opts ...client.GetOption,
) (*metav1.PartialObjectMetadata, error) {
	args := m.Called(ctx, resourceType, name, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).(*metav1.PartialObjectMetadata)

	return result, args.Error(1)
}

func (m *MockReader) OLM() client.OLMReader {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}

	result, _ := args.Get(0).(client.OLMReader)

	return result
}
