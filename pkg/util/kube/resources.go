package kube

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

// ToUnstructured converts a typed Kubernetes object to *unstructured.Unstructured.
func ToUnstructured(obj runtime.Object) (*unstructured.Unstructured, error) {
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to unstructured: %w", err)
	}

	return &unstructured.Unstructured{Object: unstructuredObj}, nil
}

// StripFields removes specified fields from a resource using JQ.
func StripFields(obj *unstructured.Unstructured, fields []string) (*unstructured.Unstructured, error) {
	if len(fields) == 0 {
		return obj.DeepCopy(), nil
	}

	stripped := obj.DeepCopy()

	jqExpr := "del(" + strings.Join(fields, ", ") + ")"

	if err := jq.Transform(stripped, "%s", jqExpr); err != nil {
		return nil, fmt.Errorf("applying JQ transform: %w", err)
	}

	return stripped, nil
}
