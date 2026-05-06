package status

import (
	"fmt"
	"io"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"

	"github.com/opendatahub-io/odh-cli/pkg/printer/json"
)

// renderJSON writes the report as indented JSON to w.
func renderJSON(w io.Writer, report *clusterhealth.Report) error {
	renderer := json.NewRenderer[*clusterhealth.Report](
		json.WithWriter[*clusterhealth.Report](w),
	)

	if err := renderer.Render(report); err != nil {
		return fmt.Errorf("rendering JSON report: %w", err)
	}

	return nil
}
