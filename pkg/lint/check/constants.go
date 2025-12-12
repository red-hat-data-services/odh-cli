package check

// Management state values for components and services.
const (
	ManagementStateManaged   = "Managed"
	ManagementStateUnmanaged = "Unmanaged"
	ManagementStateRemoved   = "Removed"
)

// Component names for diagnostic results.
const (
	ComponentCodeFlare = "codeflare"
	ComponentModelMesh = "modelmesh"
	ComponentKServe    = "kserve"
	ComponentKueue     = "kueue"
)

// Service names for diagnostic results.
const (
	ServiceServiceMesh = "servicemesh"
)

// Dependency names for diagnostic results.
const (
	DependencyCertManager           = "certmanager"
	DependencyKueueOperator         = "kueueoperator"
	DependencyServiceMeshOperatorV2 = "servicemesh-operator-v2"
)

// Check type names (third parameter to result.New).
const (
	CheckTypeRemoval           = "removal"
	CheckTypeInstalled         = "installed"
	CheckTypeManagedRemoval    = "managed-removal"
	CheckTypeServerlessRemoval = "serverless-removal"
)

// Annotation keys for diagnostic results.
const (
	// Component annotations
	AnnotationComponentManagementState = "component.opendatahub.io/management-state"
	AnnotationComponentKServeState     = "component.opendatahub.io/kserve-management-state"
	AnnotationComponentServingState    = "component.opendatahub.io/serving-management-state"

	// Service annotations
	AnnotationServiceManagementState = "service.opendatahub.io/management-state"

	// Check annotations
	AnnotationCheckTargetVersion = "check.opendatahub.io/target-version"

	// Operator annotations
	AnnotationOperatorInstalledVersion = "operator.opendatahub.io/installed-version"
)
