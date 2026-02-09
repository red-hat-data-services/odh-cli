package kube

import (
	"k8s.io/apimachinery/pkg/types"
)

// ToNamespacedNames converts objects with metadata to a slice of NamespacedName.
func ToNamespacedNames[T NamespacedNamer](items []T) []types.NamespacedName {
	result := make([]types.NamespacedName, 0, len(items))

	for _, item := range items {
		result = append(result, types.NamespacedName{
			Namespace: item.GetNamespace(),
			Name:      item.GetName(),
		})
	}

	return result
}
