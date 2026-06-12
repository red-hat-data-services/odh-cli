package training_test

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	training "github.com/opendatahub-io/odh-cli/pkg/migrate/actions/training"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

func newDynamicClient(objects ...runtime.Object) client.Client {
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}

// --- ExtractJobStatus tests ---

func TestExtractJobStatus_Succeeded(t *testing.T) {
	g := NewWithT(t)

	obj := newTrainingJob(resources.PyTorchJob, "my-job", "test-ns", succeededConditions())
	g.Expect(training.ExtractJobStatus(obj)).To(Equal("Succeeded"))
}

func TestExtractJobStatus_Running(t *testing.T) {
	g := NewWithT(t)

	obj := newTrainingJob(resources.PyTorchJob, "my-job", "test-ns", runningConditions())
	g.Expect(training.ExtractJobStatus(obj)).To(Equal("Running"))
}

func TestExtractJobStatus_Failed(t *testing.T) {
	g := NewWithT(t)

	obj := newTrainingJob(resources.PyTorchJob, "my-job", "test-ns", failedConditions())
	g.Expect(training.ExtractJobStatus(obj)).To(Equal("Failed"))
}

func TestExtractJobStatus_NoConditions(t *testing.T) {
	g := NewWithT(t)

	obj := newTrainingJob(resources.PyTorchJob, "my-job", "test-ns", nil)
	g.Expect(training.ExtractJobStatus(obj)).To(Equal("Unknown"))
}

func TestExtractJobStatus_EmptyConditions(t *testing.T) {
	g := NewWithT(t)

	obj := newTrainingJob(resources.PyTorchJob, "my-job", "test-ns", []any{})
	g.Expect(training.ExtractJobStatus(obj)).To(Equal("Unknown"))
}

func TestExtractJobStatus_AllConditionsFalse(t *testing.T) {
	g := NewWithT(t)

	conditions := []any{
		map[string]any{"type": "Created", "status": "False"},
		map[string]any{"type": "Running", "status": "False"},
	}
	obj := newTrainingJob(resources.PyTorchJob, "my-job", "test-ns", conditions)
	g.Expect(training.ExtractJobStatus(obj)).To(Equal("Unknown"))
}

// --- EnumerateWorkloads tests ---

func TestEnumerateWorkloads_MultipleJobTypes(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pytorch := newTrainingJob(resources.PyTorchJob, "pytorch-1", "test-ns", succeededConditions())
	tfjob := newTrainingJob(resources.TFJob, "tf-1", "test-ns", runningConditions())
	mpijob := newTrainingJob(resources.MPIJob, "mpi-1", "test-ns", failedConditions())
	xgboost := newTrainingJob(resources.XGBoostJob, "xgb-1", "test-ns", succeededConditions())

	entries, failures, err := training.EnumerateWorkloads(ctx, newDynamicClient(pytorch, tfjob, mpijob, xgboost))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(failures).To(BeEmpty())
	g.Expect(entries).To(HaveLen(4))
}

func TestEnumerateWorkloads_Empty(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	entries, failures, err := training.EnumerateWorkloads(ctx, newDynamicClient())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(failures).To(BeEmpty())
	g.Expect(entries).To(BeEmpty())
}

func TestEnumerateWorkloads_SortsResults(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	jobB := newTrainingJob(resources.PyTorchJob, "b-job", "test-ns", succeededConditions())
	jobA := newTrainingJob(resources.PyTorchJob, "a-job", "test-ns", succeededConditions())
	jobC := newTrainingJob(resources.TFJob, "c-job", "test-ns", runningConditions())

	entries, _, err := training.EnumerateWorkloads(ctx, newDynamicClient(jobB, jobA, jobC))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(entries).To(HaveLen(3))

	g.Expect(entries[0].Kind).To(Equal("PyTorchJob"))
	g.Expect(entries[0].Name).To(Equal("a-job"))
	g.Expect(entries[1].Kind).To(Equal("PyTorchJob"))
	g.Expect(entries[1].Name).To(Equal("b-job"))
	g.Expect(entries[2].Kind).To(Equal("TFJob"))
	g.Expect(entries[2].Name).To(Equal("c-job"))
}

func TestEnumerateWorkloads_AllListsFail(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	dynamicClient.PrependReactor("list", "*", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})

	k8sClient := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	_, _, err := training.EnumerateWorkloads(ctx, k8sClient)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("failed to list any training workload types"))
}

func TestEnumerateWorkloads_PartialFailure(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	tfjob := newTrainingJob(resources.TFJob, "tf-1", "test-ns", succeededConditions())
	mpijob := newTrainingJob(resources.MPIJob, "mpi-1", "test-ns", runningConditions())

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, tfjob, mpijob)
	dynamicClient.PrependReactor("list", "pytorchjobs", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})

	k8sClient := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	entries, failures, err := training.EnumerateWorkloads(ctx, k8sClient)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(failures).To(HaveLen(1))
	g.Expect(failures[0].Kind).To(Equal("PyTorchJob"))
	g.Expect(entries).To(HaveLen(2))
}

func TestEnumerateWorkloads_NoEntriesWithFailures(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	dynamicClient.PrependReactor("list", "pytorchjobs", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(
			schema.GroupResource{Group: resources.PyTorchJob.Group, Resource: resources.PyTorchJob.Resource}, "")
	})
	dynamicClient.PrependReactor("list", "tfjobs", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(
			schema.GroupResource{Group: resources.TFJob.Group, Resource: resources.TFJob.Resource}, "")
	})
	dynamicClient.PrependReactor("list", "mpijobs", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})

	k8sClient := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	_, failures, err := training.EnumerateWorkloads(ctx, k8sClient)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("failed to list any training workload types"))
	g.Expect(failures).To(HaveLen(1))
	g.Expect(failures[0].Kind).To(Equal("MPIJob"))
}

func TestEnumerateWorkloads_ContextCanceled(t *testing.T) {
	g := NewWithT(t)

	ctx, cancel := context.WithCancel(t.Context())

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	dynamicClient.PrependReactor("list", "pytorchjobs", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		cancel()

		return true, nil, context.Canceled
	})

	k8sClient := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	_, _, err := training.EnumerateWorkloads(ctx, k8sClient)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("context canceled"))
}

// --- BuildReport tests ---

func TestBuildReport(t *testing.T) {
	g := NewWithT(t)

	entries := []training.WorkloadEntry{
		{Kind: "PyTorchJob", Status: "Succeeded"},
		{Kind: "PyTorchJob", Status: "Running"},
		{Kind: "PyTorchJob", Status: "Failed"},
		{Kind: "TFJob", Status: "Succeeded"},
		{Kind: "TFJob", Status: "Succeeded"},
		{Kind: "MPIJob", Status: "Running"},
	}

	report := training.BuildReport(entries)
	g.Expect(report.Kind).To(Equal("TrainingWorkloadReport"))
	g.Expect(report.Summary.Total).To(Equal(6))
	g.Expect(report.Summary.ByKind["PyTorchJob"]).To(Equal(3))
	g.Expect(report.Summary.ByKind["TFJob"]).To(Equal(2))
	g.Expect(report.Summary.ByKind["MPIJob"]).To(Equal(1))
	g.Expect(report.Summary.ByStatus["Succeeded"]).To(Equal(3))
	g.Expect(report.Summary.ByStatus["Running"]).To(Equal(2))
	g.Expect(report.Summary.ByStatus["Failed"]).To(Equal(1))
}

func TestBuildReport_Empty(t *testing.T) {
	g := NewWithT(t)

	report := training.BuildReport(nil)
	g.Expect(report.Kind).To(Equal("TrainingWorkloadReport"))
	g.Expect(report.Summary.Total).To(Equal(0))
}

// --- ExtractWorkloadEntry tests ---

func TestExtractWorkloadEntry(t *testing.T) {
	g := NewWithT(t)

	obj := newTrainingJob(resources.PyTorchJob, "my-job", "my-ns", succeededConditions())
	entry := training.ExtractWorkloadEntry(obj, "PyTorchJob")

	g.Expect(entry.Namespace).To(Equal("my-ns"))
	g.Expect(entry.Kind).To(Equal("PyTorchJob"))
	g.Expect(entry.Name).To(Equal("my-job"))
	g.Expect(entry.Status).To(Equal("Succeeded"))
	g.Expect(entry.Age).ToNot(BeEmpty())
}
