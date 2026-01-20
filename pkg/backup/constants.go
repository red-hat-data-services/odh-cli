package backup

// DefaultWorkloadTypes are the workload types to backup if no --includes specified.
//
//nolint:gochecknoglobals // Configuration constant for default workload types.
var DefaultWorkloadTypes = []string{
	"notebooks.kubeflow.org",
}

// DefaultStripFields are cluster-specific fields to strip from all resources.
//
//nolint:gochecknoglobals // Configuration constant for default field stripping.
var DefaultStripFields = []string{
	".status",
	".metadata.generation",
	".metadata.resourceVersion",
	".metadata.uid",
	".metadata.creationTimestamp",
	".metadata.managedFields",
	".metadata.selfLink",
	".metadata.annotations.\"kubectl.kubernetes.io/last-applied-configuration\"",
}
