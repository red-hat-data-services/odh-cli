package rhbok_test

import (
	"bytes"
	"testing"

	"github.com/blang/semver/v4"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	operatorfake "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned/fake"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

//nolint:gochecknoglobals // test-only GVR→ListKind mapping for fake dynamic client
var testListKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceClusterV1.GVR(): resources.DataScienceClusterV1.ListKind(),
	resources.ClusterQueue.GVR():         resources.ClusterQueue.ListKind(),
	resources.LocalQueue.GVR():           resources.LocalQueue.ListKind(),
	resources.Subscription.GVR():         resources.Subscription.ListKind(),
	resources.ConfigMap.GVR():            resources.ConfigMap.ListKind(),
}

type targetOpts struct {
	dryRun         bool
	skipConfirm    bool
	outputDir      string
	olmObjects     []runtime.Object
	rbacAllowed    bool
	dynamicReactor func(k8stesting.Action) (bool, runtime.Object, error)
}

func newTarget(t *testing.T, objects []*unstructured.Unstructured, opts targetOpts) action.Target {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicObjs := make([]runtime.Object, len(objects))
	for i, obj := range objects {
		dynamicObjs[i] = obj
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		testListKinds,
		dynamicObjs...,
	)

	if opts.dynamicReactor != nil {
		dynamicClient.PrependReactor("*", "*", opts.dynamicReactor)
	}

	olmClient := operatorfake.NewSimpleClientset(opts.olmObjects...) //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM

	kubeClient := kubefake.NewSimpleClientset() //nolint:staticcheck // Need PrependReactor for SelfSubjectAccessReview responses
	kubeClient.PrependReactor("create", "selfsubjectaccessreviews",
		func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, &authorizationv1.SelfSubjectAccessReview{
				Status: authorizationv1.SubjectAccessReviewStatus{Allowed: opts.rbacAllowed},
			}, nil
		},
	)

	testClient := client.NewForTesting(client.TestClientConfig{
		Dynamic:    dynamicClient,
		OLM:        olmClient,
		Kubernetes: kubeClient,
	})

	outputDir := opts.outputDir
	if outputDir == "" {
		outputDir = t.TempDir()
	}

	currentVersion := semver.MustParse("2.25.0")
	targetVersion := semver.MustParse("3.0.0")

	return action.Target{
		Client:         testClient,
		CurrentVersion: &currentVersion,
		TargetVersion:  &targetVersion,
		DryRun:         opts.dryRun,
		SkipConfirm:    opts.skipConfirm,
		OutputDir:      outputDir,
		Recorder:       action.NewRootRecorder(),
		IO:             iostreams.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}),
	}
}

type objOption func(*unstructured.Unstructured)

func withComponent(name, state string) objOption {
	return func(obj *unstructured.Unstructured) {
		components, _, _ := unstructured.NestedMap(obj.Object, "spec", "components")
		if components == nil {
			components = map[string]any{}
		}

		components[name] = map[string]any{"managementState": state}
		_ = unstructured.SetNestedField(obj.Object, components, "spec", "components")
	}
}

func inNamespace(ns string) objOption {
	return func(obj *unstructured.Unstructured) {
		obj.SetNamespace(ns)
	}
}

func withAnnotation(key, value string) objOption {
	return func(obj *unstructured.Unstructured) {
		annotations := obj.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}

		annotations[key] = value
		obj.SetAnnotations(annotations)
	}
}

func makeObj(rt resources.ResourceType, name string, opts ...objOption) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": rt.APIVersion(),
			"kind":       rt.Kind,
			"metadata": map[string]any{
				"name": name,
			},
		},
	}

	for _, opt := range opts {
		opt(obj)
	}

	return obj
}

func makeDSCV1(name string, opts ...objOption) *unstructured.Unstructured {
	return makeObj(resources.DataScienceClusterV1, name, opts...)
}

func makeConfigMap(name string, opts ...objOption) *unstructured.Unstructured {
	obj := makeObj(resources.ConfigMap, name, opts...)
	if _, ok, _ := unstructured.NestedMap(obj.Object, "data"); !ok {
		_ = unstructured.SetNestedField(obj.Object, map[string]any{"config.yaml": "test-config-data"}, "data")
	}

	return obj
}

func makeClusterQueue(name string, opts ...objOption) *unstructured.Unstructured {
	return makeObj(resources.ClusterQueue, name, opts...)
}

func makeLocalQueue(name string, opts ...objOption) *unstructured.Unstructured {
	return makeObj(resources.LocalQueue, name, opts...)
}

func makeSubscription(name string, opts ...objOption) *unstructured.Unstructured {
	return makeObj(resources.Subscription, name, opts...)
}

func newOLMSubscription(name, namespace string) *operatorsv1alpha1.Subscription {
	return &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func newOLMCSV(name, namespace string) *operatorsv1alpha1.ClusterServiceVersion {
	return &operatorsv1alpha1.ClusterServiceVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: operatorsv1alpha1.ClusterServiceVersionStatus{
			Phase: operatorsv1alpha1.CSVPhaseSucceeded,
		},
	}
}
