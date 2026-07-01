package status

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	cmdpkg "github.com/opendatahub-io/odh-cli/pkg/cmd"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

// Test timing constants.
const (
	testPollInterval = 1 * time.Second
	testTimeout      = 10 * time.Second
	testShortTimeout = 3 * time.Second

	testAppNS      = "test-apps"
	testOperatorNS = "test-operator-ns"
	testOperator   = "test-operator"
)

//nolint:gochecknoglobals // Test data constants
var (
	testDSCIGVK = schema.GroupVersionKind{
		Group: "dscinitialization.opendatahub.io", Version: "v2", Kind: "DSCInitialization",
	}
	testDSCGVK = schema.GroupVersionKind{
		Group: "datasciencecluster.opendatahub.io", Version: "v2", Kind: "DataScienceCluster",
	}
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)

	for _, gvk := range []schema.GroupVersionKind{testDSCIGVK, testDSCGVK} {
		s.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		s.AddKnownTypeWithName(schema.GroupVersionKind{
			Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind + "List",
		}, &unstructured.UnstructuredList{})
	}

	return s
}

func newCRObject(gvk schema.GroupVersionKind, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(name)
	_ = unstructured.SetNestedField(obj.Object, []any{
		map[string]any{"type": "ReconcileComplete", "status": "True"},
	}, "status", "conditions")

	return obj
}

func testOperatorDeployment() *appsv1.Deployment {
	replicas := int32(1)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: testOperator, Namespace: testOperatorNS},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1, Replicas: 1, AvailableReplicas: 1},
	}
}

func baseObjects() []crclient.Object {
	return []crclient.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testAppNS}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testOperatorNS}},
		newCRObject(testDSCIGVK, "default-dsci"),
		newCRObject(testDSCGVK, "default-dsc"),
	}
}

func newTestConfig(fc crclient.Client) *clusterhealth.Config {
	return &clusterhealth.Config{
		Client:     fc,
		Operator:   clusterhealth.OperatorConfig{Namespace: testOperatorNS, Name: testOperator},
		Namespaces: clusterhealth.NamespaceConfig{Apps: testAppNS},
		DSCI:       types.NamespacedName{Name: "default-dsci"},
		DSC:        types.NamespacedName{Name: "default-dsc"},
	}
}

func healthyConfig(scheme *runtime.Scheme) *clusterhealth.Config {
	dep := testOperatorDeployment()
	objs := append(baseObjects(), dep)

	fc := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(dep).
		Build()

	return newTestConfig(fc)
}

func unhealthyConfig(scheme *runtime.Scheme) *clusterhealth.Config {
	fc := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(baseObjects()...).
		Build()

	return newTestConfig(fc)
}

func newTestWaitCommand(cfg *clusterhealth.Config) (*Command, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer

	cmd := &Command{
		IO:           iostreams.NewIOStreams(nil, &stdout, &stderr),
		OutputFormat: OutputFormatJSON,
		WaitOptions: cmdpkg.WaitOptions{
			WaitFor:      WaitConditionHealthy,
			PollInterval: testPollInterval,
		},
		Timeout:      testTimeout,
		healthConfig: cfg,
	}

	return cmd, &stdout, &stderr
}

func TestRunWaitFor(t *testing.T) {
	scheme := testScheme()

	t.Run("healthy on first poll exits immediately", func(t *testing.T) {
		g := NewWithT(t)

		cmd, _, _ := newTestWaitCommand(healthyConfig(scheme))

		err := cmd.runWaitFor(t.Context())
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("timeout returns structured error with WAIT_TIMEOUT code", func(t *testing.T) {
		g := NewWithT(t)

		cmd, _, _ := newTestWaitCommand(unhealthyConfig(scheme))
		cmd.Timeout = testShortTimeout

		err := cmd.runWaitFor(t.Context())
		g.Expect(err).To(HaveOccurred())

		var structured *clierrors.StructuredError
		g.Expect(errors.As(err, &structured)).To(BeTrue())
		g.Expect(structured).To(HaveField("Code", "WAIT_TIMEOUT"))
		g.Expect(structured).To(HaveField("Category", clierrors.CategoryTimeout))
	})

	t.Run("unhealthy logs waiting message to stderr", func(t *testing.T) {
		g := NewWithT(t)

		cmd, _, stderr := newTestWaitCommand(unhealthyConfig(scheme))
		cmd.Timeout = testShortTimeout

		_ = cmd.runWaitFor(t.Context())
		g.Expect(stderr.String()).To(ContainSubstring("Waiting for healthy status"))
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		g := NewWithT(t)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		cmd, _, _ := newTestWaitCommand(unhealthyConfig(scheme))
		cmd.Timeout = 0

		err := cmd.runWaitFor(ctx)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("becomes healthy after operator deployment is created", func(t *testing.T) {
		g := NewWithT(t)

		cfg := unhealthyConfig(scheme)
		cmd, _, _ := newTestWaitCommand(cfg)
		cmd.Timeout = 0

		dep := testOperatorDeployment()

		setupErr := make(chan error, 1)

		go func() {
			time.Sleep(1500 * time.Millisecond)

			if err := cfg.Client.Create(context.Background(), dep); err != nil {
				setupErr <- err

				return
			}

			dep.Status.ReadyReplicas = 1
			dep.Status.Replicas = 1
			dep.Status.AvailableReplicas = 1
			setupErr <- cfg.Client.Status().Update(context.Background(), dep)
		}()

		err := cmd.runWaitFor(t.Context())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(<-setupErr).ToNot(HaveOccurred())
	})
}
