package get_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/get"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

const (
	testNamespace = "test-ns"
	testNotebook1 = "notebook-alpha"
	testNotebook2 = "notebook-beta"
	testISVCName  = "my-isvc"
	testImage1    = "quay.io/image:v1"
	testImage2    = "quay.io/image:v2"
	testISVCURL   = "https://my-isvc.example.com"
	testTimestamp = "2025-01-01T00:00:00Z"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.Notebook.GVR():                          resources.Notebook.ListKind(),
	resources.InferenceService.GVR():                  resources.InferenceService.ListKind(),
	resources.ServingRuntime.GVR():                    resources.ServingRuntime.ListKind(),
	resources.DataSciencePipelinesApplicationV1.GVR(): resources.DataSciencePipelinesApplicationV1.ListKind(),
}

func newNotebook(name, namespace, image string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":              name,
				"namespace":         namespace,
				"creationTimestamp": testTimestamp,
			},
			"spec": map[string]any{
				"template": map[string]any{
					"spec": map[string]any{
						"containers": []any{
							map[string]any{"image": image},
						},
					},
				},
			},
			"status": map[string]any{
				"readyReplicas": int64(1),
			},
		},
	}
}

func newInferenceService(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":              name,
				"namespace":         namespace,
				"creationTimestamp": testTimestamp,
			},
			"status": map[string]any{
				"url": testISVCURL,
				"conditions": []any{
					map[string]any{
						"type":   "Ready",
						"status": "True",
					},
				},
			},
		},
	}
}

func newTestCommand(k8sClient client.Client) (*get.Command, *bytes.Buffer) {
	var out bytes.Buffer

	cmd := &get.Command{
		IO:           iostreams.NewIOStreams(nil, &out, &out),
		Client:       k8sClient,
		OutputFormat: "table",
		Namespace:    testNamespace,
	}

	return cmd, &out
}

func newDynamicClient(objects ...runtime.Object) client.Client {
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}

// --- Registry Tests ---

func TestResolve(t *testing.T) {
	t.Run("resolves canonical name", func(t *testing.T) {
		g := NewWithT(t)

		rt, err := get.Resolve("notebooks")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rt).To(Equal(resources.Notebook))
	})

	t.Run("resolves alias nb to Notebook", func(t *testing.T) {
		g := NewWithT(t)

		rt, err := get.Resolve("nb")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rt).To(Equal(resources.Notebook))
	})

	t.Run("resolves alias isvc to InferenceService", func(t *testing.T) {
		g := NewWithT(t)

		rt, err := get.Resolve("isvc")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rt).To(Equal(resources.InferenceService))
	})

	t.Run("resolves alias sr to ServingRuntime", func(t *testing.T) {
		g := NewWithT(t)

		rt, err := get.Resolve("sr")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rt).To(Equal(resources.ServingRuntime))
	})

	t.Run("resolves alias pipeline to DataSciencePipelinesApplicationV1", func(t *testing.T) {
		g := NewWithT(t)

		rt, err := get.Resolve("pipeline")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rt).To(Equal(resources.DataSciencePipelinesApplicationV1))
	})

	t.Run("is case-insensitive", func(t *testing.T) {
		g := NewWithT(t)

		rt, err := get.Resolve("NB")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rt).To(Equal(resources.Notebook))
	})

	t.Run("returns error for unknown resource with available list", func(t *testing.T) {
		g := NewWithT(t)

		_, err := get.Resolve("unknown")
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("unknown resource type"))
		g.Expect(err.Error()).To(ContainSubstring("nb"))
		g.Expect(err.Error()).To(ContainSubstring("notebooks"))
	})
}

func TestNames(t *testing.T) {
	g := NewWithT(t)

	names := get.Names()
	g.Expect(names).To(ContainElements("nb", "notebooks", "isvc", "inferenceservices", "sr", "servingruntimes", "pipeline", "datasciencepipelinesapplications"))
	// Verify sorted
	for i := 1; i < len(names); i++ {
		g.Expect(names[i] >= names[i-1]).To(BeTrue(), "Names() should return sorted list")
	}
}

// --- Command Tests ---

func TestListNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb1 := newNotebook(testNotebook1, testNamespace, testImage1)
	nb2 := newNotebook(testNotebook2, testNamespace, testImage2)

	k8sClient := newDynamicClient(nb1, nb2)
	cmd, out := newTestCommand(k8sClient)
	cmd.ResourceName = "nb"
	cmd.ResolvedType = resources.Notebook

	err := cmd.Run(ctx)
	g.Expect(err).ToNot(HaveOccurred())

	output := out.String()
	g.Expect(output).To(ContainSubstring("NAME"))
	g.Expect(output).To(ContainSubstring("IMAGE"))
	g.Expect(output).To(ContainSubstring("READY"))
	g.Expect(output).To(ContainSubstring("AGE"))
	g.Expect(output).To(ContainSubstring(testNotebook1))
	g.Expect(output).To(ContainSubstring(testNotebook2))
	g.Expect(output).To(ContainSubstring(testImage1))
	g.Expect(output).To(ContainSubstring(testImage2))
}

func TestGetSpecificResource(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	isvc := newInferenceService(testISVCName, testNamespace)

	k8sClient := newDynamicClient(isvc)
	cmd, out := newTestCommand(k8sClient)
	cmd.ResourceName = "isvc"
	cmd.ItemName = testISVCName
	cmd.ResolvedType = resources.InferenceService

	err := cmd.Run(ctx)
	g.Expect(err).ToNot(HaveOccurred())

	output := out.String()
	g.Expect(output).To(ContainSubstring(testISVCName))
	g.Expect(output).To(ContainSubstring(testISVCURL))
}

func TestListNotebooksAllNamespaces(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb1 := newNotebook(testNotebook1, testNamespace, testImage1)
	nb2 := newNotebook(testNotebook2, "other-ns", testImage2)

	k8sClient := newDynamicClient(nb1, nb2)
	cmd, out := newTestCommand(k8sClient)
	cmd.ResourceName = "nb"
	cmd.AllNamespaces = true
	cmd.Namespace = ""
	cmd.ResolvedType = resources.Notebook

	err := cmd.Run(ctx)
	g.Expect(err).ToNot(HaveOccurred())

	output := out.String()
	g.Expect(output).To(ContainSubstring("NAMESPACE"))
	g.Expect(output).To(ContainSubstring(testNamespace))
	g.Expect(output).To(ContainSubstring("other-ns"))
}

func TestJSONOutput(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook(testNotebook1, testNamespace, testImage1)

	k8sClient := newDynamicClient(nb)
	cmd, out := newTestCommand(k8sClient)
	cmd.ResourceName = "nb"
	cmd.OutputFormat = "json"
	cmd.ResolvedType = resources.Notebook

	err := cmd.Run(ctx)
	g.Expect(err).ToNot(HaveOccurred())

	output := out.String()
	g.Expect(output).To(ContainSubstring(`"kind"`))
	g.Expect(output).To(ContainSubstring(testNotebook1))
}

// --- Validation Tests ---

func TestValidate(t *testing.T) {
	t.Run("rejects all-namespaces with explicit namespace", func(t *testing.T) {
		g := NewWithT(t)

		ns := "my-ns"
		cmd := &get.Command{
			AllNamespaces: true,
			OutputFormat:  "table",
			ConfigFlags:   configFlagsWithNamespace(&ns),
		}

		err := cmd.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("mutually exclusive"))
	})

	t.Run("rejects invalid output format", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &get.Command{
			OutputFormat: "xml",
			ConfigFlags:  configFlagsWithNamespace(nil),
		}

		err := cmd.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid output format"))
	})

	t.Run("accepts valid output formats", func(t *testing.T) {
		g := NewWithT(t)

		for _, format := range []string{"table", "json", "yaml"} {
			cmd := &get.Command{
				OutputFormat: format,
				ConfigFlags:  configFlagsWithNamespace(nil),
			}

			err := cmd.Validate()
			g.Expect(err).ToNot(HaveOccurred(), "format %q should be valid", format)
		}
	})
}

// --- Empty Result Tests ---

func TestListReturnsEmptyTable(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newDynamicClient()
	cmd, out := newTestCommand(k8sClient)
	cmd.ResourceName = "nb"
	cmd.ResolvedType = resources.Notebook

	err := cmd.Run(ctx)
	g.Expect(err).ToNot(HaveOccurred())

	output := out.String()
	g.Expect(output).To(ContainSubstring("NAME"))
	g.Expect(output).ToNot(ContainSubstring(testNotebook1))
}

// --- JSON/YAML Output Tests ---

func TestJSONOutput_MultipleItems_ReturnsList(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb1 := newNotebook(testNotebook1, testNamespace, testImage1)
	nb2 := newNotebook(testNotebook2, testNamespace, testImage2)

	k8sClient := newDynamicClient(nb1, nb2)
	cmd, out := newTestCommand(k8sClient)
	cmd.ResourceName = "nb"
	cmd.OutputFormat = "json"
	cmd.ResolvedType = resources.Notebook

	err := cmd.Run(ctx)
	g.Expect(err).ToNot(HaveOccurred())

	// Parse the JSON output
	var result map[string]any
	err = json.Unmarshal(out.Bytes(), &result)
	g.Expect(err).ToNot(HaveOccurred())

	// Multiple items should be wrapped in List
	g.Expect(result).To(HaveKeyWithValue("apiVersion", "v1"))
	g.Expect(result).To(HaveKeyWithValue("kind", "List"))
	g.Expect(result).To(HaveKey("items"))

	items, ok := result["items"].([]any)
	g.Expect(ok).To(BeTrue(), "items should be an array")
	g.Expect(items).To(HaveLen(2))
}

func TestJSONOutput_SingleItem_ReturnsBareObject(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	isvc := newInferenceService(testISVCName, testNamespace)

	k8sClient := newDynamicClient(isvc)
	cmd, out := newTestCommand(k8sClient)
	cmd.ResourceName = "isvc"
	cmd.ItemName = testISVCName
	cmd.OutputFormat = "json"
	cmd.ResolvedType = resources.InferenceService

	err := cmd.Run(ctx)
	g.Expect(err).ToNot(HaveOccurred())

	// Parse the JSON output
	var result map[string]any
	err = json.Unmarshal(out.Bytes(), &result)
	g.Expect(err).ToNot(HaveOccurred())

	// Single item should be returned as bare object (not wrapped in List)
	g.Expect(result).To(HaveKeyWithValue("apiVersion", "serving.kserve.io/v1beta1"))
	g.Expect(result).To(HaveKeyWithValue("kind", "InferenceService"))
	g.Expect(result).To(HaveKey("metadata"))

	metadata, ok := result["metadata"].(map[string]any)
	g.Expect(ok).To(BeTrue(), "metadata should be a map")
	g.Expect(metadata["name"]).To(Equal(testISVCName))
}

func TestJSONOutput_EmptyList(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newDynamicClient()
	cmd, out := newTestCommand(k8sClient)
	cmd.ResourceName = "nb"
	cmd.OutputFormat = "json"
	cmd.ResolvedType = resources.Notebook

	err := cmd.Run(ctx)
	g.Expect(err).ToNot(HaveOccurred())

	// Parse the JSON output
	var result map[string]any
	err = json.Unmarshal(out.Bytes(), &result)
	g.Expect(err).ToNot(HaveOccurred())

	// Empty result should still have List structure
	g.Expect(result).To(HaveKeyWithValue("apiVersion", "v1"))
	g.Expect(result).To(HaveKeyWithValue("kind", "List"))
	g.Expect(result).To(HaveKey("items"))

	items, ok := result["items"].([]any)
	g.Expect(ok).To(BeTrue(), "items should be an array")
	g.Expect(items).To(BeEmpty())
}

// --- Helpers ---

func configFlagsWithNamespace(ns *string) *genericclioptions.ConfigFlags {
	return &genericclioptions.ConfigFlags{
		Namespace: ns,
	}
}
