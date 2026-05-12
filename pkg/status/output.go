package status

import (
	"fmt"
	"io"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"

	"github.com/opendatahub-io/odh-cli/pkg/deps"
	"github.com/opendatahub-io/odh-cli/pkg/printer/json"
	"github.com/opendatahub-io/odh-cli/pkg/printer/yaml"
)

// renderJSON writes the report as JSON with envelope to w.
func renderJSON(w io.Writer, report *clusterhealth.Report, depStatuses []deps.DependencyStatus) error {
	statusReport := NewStatusReport(report, depStatuses)
	renderer := json.NewRenderer[*StatusReport](
		json.WithWriter[*StatusReport](w),
	)

	if err := renderer.Render(statusReport); err != nil {
		return fmt.Errorf("rendering JSON report: %w", err)
	}

	return nil
}

// renderYAML writes the report as YAML with envelope to w.
func renderYAML(w io.Writer, report *clusterhealth.Report, depStatuses []deps.DependencyStatus) error {
	statusReport := NewStatusReport(report, depStatuses)
	renderer := yaml.NewRenderer[*StatusReport](
		yaml.WithWriter[*StatusReport](w),
	)

	if err := renderer.Render(statusReport); err != nil {
		return fmt.Errorf("rendering YAML report: %w", err)
	}

	return nil
}
