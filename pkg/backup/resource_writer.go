package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// WriteResourceToFile writes a resource to $outputDir/$namespace/$GVR-$name.yaml.
func WriteResourceToFile(
	outputDir string,
	gvr schema.GroupVersionResource,
	obj *unstructured.Unstructured,
) error {
	namespace := obj.GetNamespace()
	if namespace == "" {
		namespace = "cluster-scoped"
	}
	name := obj.GetName()

	nsDir := filepath.Join(outputDir, namespace)
	if err := os.MkdirAll(nsDir, dirPermissions); err != nil {
		return fmt.Errorf("creating namespace directory: %w", err)
	}

	gvrStr := gvr.Resource
	if gvr.Group != "" {
		gvrStr = gvr.Resource + "." + gvr.Group
	}
	filename := fmt.Sprintf("%s-%s.yaml", gvrStr, name)
	filePath := filepath.Join(nsDir, filename)

	data, err := yaml.Marshal(obj.Object)
	if err != nil {
		return fmt.Errorf("marshaling to YAML: %w", err)
	}

	if err := os.WriteFile(filePath, data, filePermissions); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

// WriteResourceToStdout writes a resource to stdout as YAML with --- separator.
func WriteResourceToStdout(
	out io.Writer,
	_ schema.GroupVersionResource,
	obj *unstructured.Unstructured,
) error {
	data, err := yaml.Marshal(obj.Object)
	if err != nil {
		return fmt.Errorf("marshaling to YAML: %w", err)
	}

	if _, err := fmt.Fprintln(out, "---"); err != nil {
		return fmt.Errorf("writing separator: %w", err)
	}

	if _, err := out.Write(data); err != nil {
		return fmt.Errorf("writing YAML: %w", err)
	}

	return nil
}

const gvrSeparatorLimit = 2

// parseGVRString parses a GVR string like "notebooks.kubeflow.org" into a GVR.
// Format: resource.group or just resource (for core resources).
func parseGVRString(s string) schema.GroupVersionResource {
	parts := strings.SplitN(s, ".", gvrSeparatorLimit)

	if len(parts) == 1 {
		return schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: parts[0],
		}
	}

	return schema.GroupVersionResource{
		Group:    parts[1],
		Version:  "v1",
		Resource: parts[0],
	}
}
