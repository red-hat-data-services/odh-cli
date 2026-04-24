package client

import (
	"context"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/mock"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// MockOLMReader is a mock implementation of client.OLMReader using testify/mock.
type MockOLMReader struct {
	mock.Mock
}

var _ client.OLMReader = (*MockOLMReader)(nil)

func (m *MockOLMReader) Available() bool {
	args := m.Called()

	return args.Bool(0)
}

func (m *MockOLMReader) Subscriptions(namespace string) client.SubscriptionReader {
	args := m.Called(namespace)
	if args.Get(0) == nil {
		return nil
	}

	result, _ := args.Get(0).(client.SubscriptionReader)

	return result
}

func (m *MockOLMReader) ClusterServiceVersions(namespace string) client.CSVReader {
	args := m.Called(namespace)
	if args.Get(0) == nil {
		return nil
	}

	result, _ := args.Get(0).(client.CSVReader)

	return result
}

// MockSubscriptionReader is a mock implementation of client.SubscriptionReader.
type MockSubscriptionReader struct {
	mock.Mock
}

var _ client.SubscriptionReader = (*MockSubscriptionReader)(nil)

func (m *MockSubscriptionReader) List(ctx context.Context, opts metav1.ListOptions) (*operatorsv1alpha1.SubscriptionList, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).(*operatorsv1alpha1.SubscriptionList)

	return result, args.Error(1)
}

func (m *MockSubscriptionReader) Get(ctx context.Context, name string, opts metav1.GetOptions) (*operatorsv1alpha1.Subscription, error) {
	args := m.Called(ctx, name, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).(*operatorsv1alpha1.Subscription)

	return result, args.Error(1)
}

// MockCSVReader is a mock implementation of client.CSVReader.
type MockCSVReader struct {
	mock.Mock
}

var _ client.CSVReader = (*MockCSVReader)(nil)

func (m *MockCSVReader) List(ctx context.Context, opts metav1.ListOptions) (*operatorsv1alpha1.ClusterServiceVersionList, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).(*operatorsv1alpha1.ClusterServiceVersionList)

	return result, args.Error(1)
}

func (m *MockCSVReader) Get(ctx context.Context, name string, opts metav1.GetOptions) (*operatorsv1alpha1.ClusterServiceVersion, error) {
	args := m.Called(ctx, name, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).(*operatorsv1alpha1.ClusterServiceVersion)

	return result, args.Error(1)
}
