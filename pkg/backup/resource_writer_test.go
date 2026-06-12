package backup_test

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/backup"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals
var coreGVR = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "configmaps",
}

//nolint:gochecknoglobals
var groupedGVR = schema.GroupVersionResource{
	Group:    "llamastack.io",
	Version:  "v1alpha1",
	Resource: "llamastackdistributions",
}

func TestWriteResourceFlat(t *testing.T) {
	t.Run("writes grouped resource without namespace subdirectory", func(t *testing.T) {
		g := NewWithT(t)
		dir := t.TempDir()

		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "llamastack.io/v1alpha1",
				"kind":       "LlamaStackDistribution",
				"metadata": map[string]any{
					"name":      "my-llsd",
					"namespace": "test-ns",
				},
			},
		}

		err := backup.WriteResourceFlat(dir, groupedGVR, obj)
		g.Expect(err).ToNot(HaveOccurred())

		expectedFile := filepath.Join(dir, "llamastackdistributions.llamastack.io-my-llsd.yaml")
		g.Expect(expectedFile).To(BeAnExistingFile())

		// Verify no namespace subdirectory was created
		entries, err := os.ReadDir(dir)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(entries).To(HaveLen(1))
		g.Expect(entries[0].Name()).To(Equal("llamastackdistributions.llamastack.io-my-llsd.yaml"))
	})

	t.Run("writes core resource with correct filename format", func(t *testing.T) {
		g := NewWithT(t)
		dir := t.TempDir()

		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "my-config",
					"namespace": "test-ns",
				},
			},
		}

		err := backup.WriteResourceFlat(dir, coreGVR, obj)
		g.Expect(err).ToNot(HaveOccurred())

		// Core resources have no group, so filename is resource-name.yaml
		expectedFile := filepath.Join(dir, "configmaps-my-config.yaml")
		g.Expect(expectedFile).To(BeAnExistingFile())
	})

	t.Run("file contains valid YAML content", func(t *testing.T) {
		g := NewWithT(t)
		dir := t.TempDir()

		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "my-config",
					"namespace": "test-ns",
				},
				"data": map[string]any{
					"key": "value",
				},
			},
		}

		err := backup.WriteResourceFlat(dir, coreGVR, obj)
		g.Expect(err).ToNot(HaveOccurred())

		filePath := filepath.Join(dir, "configmaps-my-config.yaml")
		content, err := os.ReadFile(filePath) //nolint:gosec // test file with controlled path
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(string(content)).To(ContainSubstring("kind: ConfigMap"))
		g.Expect(string(content)).To(ContainSubstring("name: my-config"))
	})
}
