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

	nsDir := filepath.Join(outputDir, namespace)
	if err := os.MkdirAll(nsDir, dirPermissions); err != nil {
		return fmt.Errorf("creating namespace directory: %w", err)
	}

	return writeResourceToDir(nsDir, gvr, obj)
}

// WriteResourceFlat writes a resource to $dir/$GVR-$name.yaml without creating
// a namespace subdirectory. Use this when the directory structure already encodes
// the namespace (e.g. $outputDir/$namespace/$name/).
func WriteResourceFlat(
	dir string,
	gvr schema.GroupVersionResource,
	obj *unstructured.Unstructured,
) error {
	return writeResourceToDir(dir, gvr, obj)
}

// writeResourceToDir writes a resource YAML directly to the given directory.
func writeResourceToDir(
	dir string,
	gvr schema.GroupVersionResource,
	obj *unstructured.Unstructured,
) error {
	gvrStr := gvr.Resource
	if gvr.Group != "" {
		gvrStr = gvr.Resource + "." + gvr.Group
	}

	filename := fmt.Sprintf("%s-%s.yaml", gvrStr, obj.GetName())
	filePath := filepath.Join(dir, filename)

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

// WriteResourcesToDir writes multiple resources to a directory using WriteResourceToFile.
func WriteResourcesToDir(
	outputDir string,
	gvr schema.GroupVersionResource,
	resources []*unstructured.Unstructured,
) error {
	for _, resource := range resources {
		if err := WriteResourceToFile(outputDir, gvr, resource); err != nil {
			return fmt.Errorf("writing resource %s: %w", resource.GetName(), err)
		}
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
