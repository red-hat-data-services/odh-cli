package get

import (
	"fmt"
	"io"
	"math"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	printerjson "github.com/opendatahub-io/odh-cli/pkg/printer/json"
	"github.com/opendatahub-io/odh-cli/pkg/printer/table"
	printeryaml "github.com/opendatahub-io/odh-cli/pkg/printer/yaml"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	colName       = "NAME"
	colNamespace  = "NAMESPACE"
	colImage      = "IMAGE"
	colReady      = "READY"
	colAge        = "AGE"
	colURL        = "URL"
	colDisabled   = "DISABLED"
	colModelType  = "MODELTYPE"
	colContainers = "CONTAINERS"

	jqName      = ".metadata.name"
	jqNamespace = ".metadata.namespace"
	jqAge       = ".metadata.creationTimestamp"

	jqNotebookImage = ".spec.template.spec.containers[0].image"
	jqNotebookReady = ".status.readyReplicas // 0"

	jqInferenceServiceURL   = `.status.url // ""`
	jqInferenceServiceReady = `(.status.conditions // [])[] | select(.type=="Ready") | .status // "Unknown"`

	jqServingRuntimeDisabled   = ".spec.disabled // false"
	jqServingRuntimeModelType  = `(.spec.supportedModelFormats // [])[0].name // ""`
	jqServingRuntimeContainers = ".spec.containers // [] | length"

	jqPipelineReady = `(.status.conditions // [])[] | select(.type=="Ready") | .status // "Unknown"`
)

// ageFormatter converts a creationTimestamp string to a human-readable age.
func ageFormatter(value any) any {
	ts, ok := value.(string)
	if !ok {
		return "<unknown>"
	}

	return formatAge(ts)
}

const hoursPerDay = 24

// formatAge computes a human-readable duration string from an RFC 3339 timestamp.
func formatAge(timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return "<unknown>"
	}

	d := time.Since(t)

	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < hoursPerDay*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(math.Floor(d.Hours()/hoursPerDay)))
	}
}

// notebookColumns returns table columns for Notebook resources.
func notebookColumns(prefix []table.Column) []table.Column {
	return append(prefix,
		table.NewColumn(colImage).JQ(jqNotebookImage),
		table.NewColumn(colReady).JQ(jqNotebookReady),
		ageColumn(),
	)
}

// inferenceServiceColumns returns table columns for InferenceService resources.
func inferenceServiceColumns(prefix []table.Column) []table.Column {
	return append(prefix,
		table.NewColumn(colURL).JQ(jqInferenceServiceURL),
		table.NewColumn(colReady).JQ(jqInferenceServiceReady),
		ageColumn(),
	)
}

// servingRuntimeColumns returns table columns for ServingRuntime resources.
func servingRuntimeColumns(prefix []table.Column) []table.Column {
	return append(prefix,
		table.NewColumn(colDisabled).JQ(jqServingRuntimeDisabled),
		table.NewColumn(colModelType).JQ(jqServingRuntimeModelType),
		table.NewColumn(colContainers).JQ(jqServingRuntimeContainers),
		ageColumn(),
	)
}

// pipelineColumns returns table columns for DataSciencePipelinesApplication resources.
func pipelineColumns(prefix []table.Column) []table.Column {
	return append(prefix,
		table.NewColumn(colReady).JQ(jqPipelineReady),
		ageColumn(),
	)
}

// nameOnlyColumns returns just the NAME column.
func nameOnlyColumns() []table.Column {
	return []table.Column{
		table.NewColumn(colName).JQ(jqName),
	}
}

// namespacedNameColumns returns NAMESPACE and NAME columns for cross-namespace listing.
func namespacedNameColumns() []table.Column {
	return []table.Column{
		table.NewColumn(colNamespace).JQ(jqNamespace),
		table.NewColumn(colName).JQ(jqName),
	}
}

// ageColumn returns a column that renders human-readable age from creationTimestamp.
func ageColumn() table.Column {
	return table.NewColumn(colAge).JQ(jqAge).Fn(ageFormatter)
}

// columnConfig holds parameters for building table columns.
type columnConfig struct {
	resourceType  resources.ResourceType
	allNamespaces bool
}

// columns returns the full set of table columns for the configured resource type.
func (c columnConfig) columns() []table.Column {
	prefix := nameOnlyColumns()
	if c.allNamespaces {
		prefix = namespacedNameColumns()
	}

	switch c.resourceType {
	case resources.Notebook:
		return notebookColumns(prefix)
	case resources.InferenceService:
		return inferenceServiceColumns(prefix)
	case resources.ServingRuntime:
		return servingRuntimeColumns(prefix)
	case resources.DataSciencePipelinesApplicationV1:
		return pipelineColumns(prefix)
	default:
		return prefix
	}
}

// outputTable renders resources as a formatted table.
func outputTable(
	w io.Writer,
	items []*unstructured.Unstructured,
	rt resources.ResourceType,
	allNamespaces bool,
) error {
	cfg := columnConfig{resourceType: rt, allNamespaces: allNamespaces}
	columns := cfg.columns()

	renderer := table.NewWithColumns[*unstructured.Unstructured](w, columns...)

	for _, item := range items {
		if err := renderer.Append(item); err != nil {
			return fmt.Errorf("rendering row: %w", err)
		}
	}

	if err := renderer.Render(); err != nil {
		return fmt.Errorf("rendering table: %w", err)
	}

	return nil
}

// outputJSON renders resources as JSON.
func outputJSON(w io.Writer, items []*unstructured.Unstructured) error {
	output := toOutputList(items)

	renderer := printerjson.NewRenderer[any](
		printerjson.WithWriter[any](w),
	)

	if err := renderer.Render(output); err != nil {
		return fmt.Errorf("rendering JSON: %w", err)
	}

	return nil
}

// outputYAML renders resources as YAML.
func outputYAML(w io.Writer, items []*unstructured.Unstructured) error {
	output := toOutputList(items)

	renderer := printeryaml.NewRenderer[any](
		printeryaml.WithWriter[any](w),
	)

	if err := renderer.Render(output); err != nil {
		return fmt.Errorf("rendering YAML: %w", err)
	}

	return nil
}

// toOutputList converts items to a raw object or list suitable for JSON/YAML rendering.
// Returns the single item directly if there is exactly one, otherwise wraps in a List.
func toOutputList(items []*unstructured.Unstructured) any {
	if len(items) == 1 {
		return items[0].Object
	}

	rawItems := make([]map[string]any, 0, len(items))
	for _, item := range items {
		rawItems = append(rawItems, item.Object)
	}

	return map[string]any{
		"apiVersion": "v1",
		"kind":       "List",
		"items":      rawItems,
	}
}

// KubernetesList represents the standard Kubernetes v1.List output format.
type KubernetesList struct {
	APIVersion string           `json:"apiVersion" jsonschema:"description=API version (v1),const=v1"`
	Kind       string           `json:"kind"       jsonschema:"description=Resource kind (List),const=List"`
	Items      []map[string]any `json:"items"      jsonschema:"description=Array of Kubernetes resources"`
}
