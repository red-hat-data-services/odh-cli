package schema

// SchemaType identifies a schema by its output type.
type SchemaType string

const (
	// SchemaDiagnosticResultList is the schema for lint command JSON/YAML output.
	SchemaDiagnosticResultList SchemaType = "diagnostic_result_list"
	// SchemaComponentList is the schema for components list JSON/YAML output.
	SchemaComponentList SchemaType = "component_list"
	// SchemaComponentDetails is the schema for components describe JSON/YAML output.
	SchemaComponentDetails SchemaType = "component_details"
	// SchemaDependencyStatusList is the schema for deps command JSON/YAML output.
	SchemaDependencyStatusList SchemaType = "dependency_status_list"
	// SchemaDependencyInfoList is the schema for deps --dry-run JSON/YAML output.
	SchemaDependencyInfoList SchemaType = "dependency_info_list"
	// SchemaVersionInfo is the schema for version command JSON output.
	SchemaVersionInfo SchemaType = "version_info"
	// SchemaVersionInfoVerbose is the schema for version --verbose JSON output.
	SchemaVersionInfoVerbose SchemaType = "version_info_verbose"
	// SchemaKubernetesList is the schema for get command JSON/YAML output.
	SchemaKubernetesList SchemaType = "kubernetes_list"
)
