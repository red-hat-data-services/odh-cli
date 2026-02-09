package kube

// NamespacedNamer is satisfied by both *unstructured.Unstructured and *metav1.PartialObjectMetadata.
type NamespacedNamer interface {
	GetName() string
	GetNamespace() string
}
