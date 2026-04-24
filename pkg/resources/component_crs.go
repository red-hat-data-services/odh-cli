package resources

const componentCRGroup = "components.platform.opendatahub.io"

// ComponentCRResourceTypes maps DSC component names to their corresponding
// component CR resource types from the components.platform.opendatahub.io API group.
// These CRs are v1alpha1 and may not exist on older ODH versions.
//
//nolint:gochecknoglobals // Component CR mapping is static configuration
var ComponentCRResourceTypes = map[string]ResourceType{
	// DSC v2 component names
	"aipipelines":        newComponentCR("datasciencepipelines", "DataSciencePipelines"),
	"dashboard":          newComponentCR("dashboards", "Dashboard"),
	"feastoperator":      newComponentCR("feastoperators", "FeastOperator"),
	"kserve":             newComponentCR("kserves", "Kserve"),
	"llamastackoperator": newComponentCR("llamastackoperators", "LlamaStackOperator"),
	"mlflowoperator":     newComponentCR("mlflowoperators", "MLflowOperator"),
	"modelregistry":      newComponentCR("modelregistries", "ModelRegistry"),
	"ray":                newComponentCR("rays", "Ray"),
	"sparkoperator":      newComponentCR("sparkoperators", "SparkOperator"),
	"trainer":            newComponentCR("trainers", "Trainer"),
	"trainingoperator":   newComponentCR("trainingoperators", "TrainingOperator"),
	"trustyai":           newComponentCR("trustyais", "TrustyAI"),
	"workbenches":        newComponentCR("workbenches", "Workbenches"),
}

func newComponentCR(resource, kind string) ResourceType {
	return ResourceType{
		Group:    componentCRGroup,
		Version:  "v1alpha1",
		Kind:     kind,
		Resource: resource,
	}
}

// GetComponentCR returns the ResourceType for a component CR by name.
// Returns nil if the component is not found.
func GetComponentCR(name string) *ResourceType {
	rt, ok := ComponentCRResourceTypes[name]
	if !ok {
		return nil
	}

	return &rt
}
