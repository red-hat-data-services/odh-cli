//nolint:testpackage // Tests internal implementation
package otelexporter

import (
	"bytes"
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()

	if err := metav1.AddMetaToScheme(scheme); err != nil {
		panic("AddMetaToScheme failed: " + err.Error())
	}

	return scheme
}

func newFakeClient(scheme *runtime.Scheme, objects ...runtime.Object) client.Client {
	listKinds := map[schema.GroupVersionResource]string{
		resources.GuardrailsOrchestrator.GVR(): resources.GuardrailsOrchestrator.ListKind(),
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}

func newGORCH(name, namespace string, otelExporter map[string]any) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
		"kind":       resources.GuardrailsOrchestrator.Kind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
	}

	if otelExporter != nil {
		obj["spec"] = map[string]any{
			"otelExporter": otelExporter,
		}
	}

	return &unstructured.Unstructured{Object: obj}
}

func newTarget(k8sClient client.Client, opts ...func(*action.Target)) action.Target {
	currentVersion := semver.MustParse("2.25.0")
	targetVersion := semver.MustParse("3.0.0")

	io := iostreams.NewIOStreams(
		&bytes.Buffer{},
		&bytes.Buffer{},
		&bytes.Buffer{},
	)

	target := action.Target{
		Client:         k8sClient,
		CurrentVersion: &currentVersion,
		TargetVersion:  &targetVersion,
		DryRun:         false,
		SkipConfirm:    true,
		Recorder:       action.NewVerboseRootRecorder(io),
		IO:             io,
	}

	for _, opt := range opts {
		opt(&target)
	}

	return target
}

func withDryRun(t *action.Target) {
	t.DryRun = true
}

func TestMigrateOtelExporterAction_Metadata(t *testing.T) {
	g := NewWithT(t)

	a := &MigrateOtelExporterAction{}

	g.Expect(a.ID()).To(Equal("trustyai.migrate-gorch-otel-exporter"))
	g.Expect(a.Name()).To(ContainSubstring("otelExporter"))
	g.Expect(a.Description()).To(ContainSubstring("otelExporter"))
	g.Expect(a.Group()).To(Equal(action.GroupMigration))
	g.Expect(a.Phase()).To(Equal(action.PhasePreUpgrade))
	g.Expect(a.Prepare()).To(BeNil())
	g.Expect(a.Run()).ToNot(BeNil())
}

func TestMigrateOtelExporterAction_CanApply(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		target   string
		expected bool
	}{
		{"2.25 to 3.0", "2.25.0", "3.0.0", true},
		{"3.0 to 3.1", "3.0.0", "3.1.0", false},
		{"1.0 to 2.0", "1.0.0", "2.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			a := &MigrateOtelExporterAction{}
			cv := semver.MustParse(tt.current)
			tv := semver.MustParse(tt.target)

			g.Expect(a.CanApply(action.Target{CurrentVersion: &cv, TargetVersion: &tv})).To(Equal(tt.expected))
		})
	}
}

func TestMigrateOtelExporterAction_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	a := &MigrateOtelExporterAction{}
	g.Expect(a.CanApply(action.Target{})).To(BeFalse())
}

func TestRunTask_Validate_NoGORCHs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newFakeClient(newScheme()))

	a := &MigrateOtelExporterAction{}
	result, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_WithOldSchema(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch := newGORCH("my-gorch", "ns1", map[string]any{
		"protocol":     "grpc",
		"otlpEndpoint": "http://otel:4317",
	})

	target := newTarget(newFakeClient(newScheme(), gorch))

	a := &MigrateOtelExporterAction{}
	result, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_NoGORCHs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newFakeClient(newScheme()))

	a := &MigrateOtelExporterAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_MigratesOldSchema(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch := newGORCH("my-gorch", "ns1", map[string]any{
		"protocol":     "grpc",
		"otlpEndpoint": "http://otel:4317",
		"otlpExport":   "all",
	})

	scheme := newScheme()
	k8sClient := newFakeClient(scheme, gorch)
	target := newTarget(k8sClient)

	a := &MigrateOtelExporterAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())

	updated, err := k8sClient.Dynamic().Resource(resources.GuardrailsOrchestrator.GVR()).
		Namespace("ns1").Get(ctx, "my-gorch", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	otel, _, _ := unstructured.NestedMap(updated.Object, "spec", "otelExporter")
	g.Expect(otel).To(HaveKeyWithValue("otlpProtocol", "grpc"))
	g.Expect(otel).To(HaveKeyWithValue("enableTraces", true))
	g.Expect(otel).To(HaveKeyWithValue("enableMetrics", true))
}

func TestRunTask_Execute_SkipsNewSchema(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch := newGORCH("my-gorch", "ns1", map[string]any{
		"otlpProtocol": "grpc",
		"enableTraces": true,
	})

	target := newTarget(newFakeClient(newScheme(), gorch))

	a := &MigrateOtelExporterAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_SkipsNoOtelExporter(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch := newGORCH("my-gorch", "ns1", nil)
	target := newTarget(newFakeClient(newScheme(), gorch))

	a := &MigrateOtelExporterAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_DryRun(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch := newGORCH("my-gorch", "ns1", map[string]any{
		"protocol":     "grpc",
		"otlpEndpoint": "http://otel:4317",
	})

	scheme := newScheme()
	k8sClient := newFakeClient(scheme, gorch)
	target := newTarget(k8sClient, withDryRun)

	a := &MigrateOtelExporterAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())

	updated, err := k8sClient.Dynamic().Resource(resources.GuardrailsOrchestrator.GVR()).
		Namespace("ns1").Get(ctx, "my-gorch", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	otel, _, _ := unstructured.NestedMap(updated.Object, "spec", "otelExporter")
	g.Expect(otel).To(HaveKey("protocol"))
	g.Expect(otel).ToNot(HaveKey("otlpProtocol"))
}

func TestRunTask_Execute_MultiNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch1 := newGORCH("gorch-1", "ns1", map[string]any{
		"protocol": "grpc",
	})
	gorch2 := newGORCH("gorch-2", "ns2", map[string]any{
		"protocol": "http",
	})

	scheme := newScheme()
	k8sClient := newFakeClient(scheme, gorch1, gorch2)
	target := newTarget(k8sClient)

	a := &MigrateOtelExporterAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())

	updated1, err := k8sClient.Dynamic().Resource(resources.GuardrailsOrchestrator.GVR()).
		Namespace("ns1").Get(ctx, "gorch-1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	otel1, _, _ := unstructured.NestedMap(updated1.Object, "spec", "otelExporter")
	g.Expect(otel1).To(HaveKeyWithValue("otlpProtocol", "grpc"))

	updated2, err := k8sClient.Dynamic().Resource(resources.GuardrailsOrchestrator.GVR()).
		Namespace("ns2").Get(ctx, "gorch-2", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	otel2, _, _ := unstructured.NestedMap(updated2.Object, "spec", "otelExporter")
	g.Expect(otel2).To(HaveKeyWithValue("otlpProtocol", "http"))
}

func TestRunTask_Execute_UserCancels(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch := newGORCH("my-gorch", "ns1", map[string]any{
		"protocol": "grpc",
	})

	scheme := newScheme()
	k8sClient := newFakeClient(scheme, gorch)

	inBuf := bytes.NewBufferString("n\n")
	io := iostreams.NewIOStreams(inBuf, &bytes.Buffer{}, &bytes.Buffer{})

	target := newTarget(k8sClient, func(t *action.Target) {
		t.SkipConfirm = false
		t.IO = io
		t.Recorder = action.NewVerboseRootRecorder(io)
	})

	a := &MigrateOtelExporterAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_WithWarnings(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch := newGORCH("my-gorch", "ns1", map[string]any{
		"tracesProtocol":  "grpc",
		"metricsProtocol": "http/protobuf",
		"otlpEndpoint":    "http://otel:4317",
	})

	scheme := newScheme()
	k8sClient := newFakeClient(scheme, gorch)
	target := newTarget(k8sClient)

	a := &MigrateOtelExporterAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())

	errBuf, ok := target.IO.ErrOut().(*bytes.Buffer)
	g.Expect(ok).To(BeTrue())
	g.Expect(errBuf.String()).To(ContainSubstring("tracesProtocol and metricsProtocol differ"))
}

func TestClassifyCR(t *testing.T) {
	tests := []struct {
		name     string
		otel     map[string]any
		expected migrationStatus
	}{
		{"no otel", nil, statusSkipped},
		{"new schema", map[string]any{"otlpProtocol": "grpc"}, statusSkipped},
		{"old schema", map[string]any{"protocol": "grpc"}, statusNeedsMigration},
		{"unknown fields", map[string]any{"foo": "bar"}, statusSkipped},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			gorch := newGORCH("test", "ns1", tt.otel)
			info := classifyCR(gorch)
			g.Expect(info.status).To(Equal(tt.expected))
		})
	}

	t.Run("invalid otelExporter type", func(t *testing.T) {
		g := NewWithT(t)
		gorch := &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"name": "test", "namespace": "ns1"},
			"spec":     map[string]any{"otelExporter": "not-a-map"},
		}}
		info := classifyCR(gorch)
		g.Expect(info.status).To(Equal(statusInvalid))
		g.Expect(info.message).To(ContainSubstring("invalid otelExporter"))
	})
}
