package diagnose

import (
	"fmt"
	"io"
	"strings"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"
)

// Format writes a human-readable diagnostic report to w.
func Format(w io.Writer, r *Report) {
	_, _ = fmt.Fprintln(w, "=== ODH Platform Diagnostic Report ===")

	if r.Healthy {
		_, _ = fmt.Fprintln(w, "Status: HEALTHY")

		return
	}

	_, _ = fmt.Fprintln(w, "Status: ISSUES FOUND")

	if r.Classification != nil {
		c := r.Classification
		_, _ = fmt.Fprintf(w, "\nClassification: %s/%s [code:%d] (%s)\n",
			c.Category, c.Subcategory, c.ErrorCode, c.Confidence)

		if len(c.Evidence) > 0 {
			_, _ = fmt.Fprintln(w, "Evidence:")

			for _, e := range c.Evidence {
				_, _ = fmt.Fprintf(w, "  - %s\n", e)
			}
		}
	}

	if len(r.Components) > 0 {
		_, _ = fmt.Fprintln(w, "\nComponent Status:")
		_, _ = fmt.Fprintf(w, "  %-22s  %-6s  %s\n", "NAME", "CR", "CONDITIONS")

		for _, comp := range r.Components {
			if len(comp.Errors) > 0 {
				_, _ = fmt.Fprintf(w, "  %-22s  ERROR   %s\n", comp.Component, strings.Join(comp.Errors, "; "))

				continue
			}

			cr := "absent"
			if comp.CRFound {
				cr = "found"
			}

			_, _ = fmt.Fprintf(w, "  %-22s  %-6s  %s\n", comp.Component, cr, summariseConditions(comp.Conditions))
		}
	}

	if len(r.Events) > 0 {
		_, _ = fmt.Fprintln(w, "\nRecent Warning Events:")
		_, _ = fmt.Fprintf(w, "  %-20s  %-22s  %s\n", "NAMESPACE", "REASON", "MESSAGE")

		for _, e := range r.Events {
			msg := e.Message
			if runes := []rune(msg); len(runes) > 80 {
				msg = string(runes[:77]) + "..."
			}

			_, _ = fmt.Fprintf(w, "  %-20s  %-22s  %s\n", e.Namespace, e.Reason, msg)
		}
	}
}

func summariseConditions(conds []clusterhealth.ConditionSummary) string {
	if len(conds) == 0 {
		return "none"
	}

	var degraded []string

	for _, c := range conds {
		if c.Status != "True" {
			degraded = append(degraded, c.Type)
		}
	}

	if len(degraded) == 0 {
		return fmt.Sprintf("all %d ok", len(conds))
	}

	return "degraded: " + strings.Join(degraded, ", ")
}
