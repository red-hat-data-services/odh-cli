package events

import (
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"

	printerjson "github.com/opendatahub-io/odh-cli/pkg/printer/json"
	printeryaml "github.com/opendatahub-io/odh-cli/pkg/printer/yaml"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const (
	maxMessageLen    = 80
	hoursPerDay      = 24
	tabwriterPadding = 2

	msgNoEventsFound  = "No events found."
	tableHeader       = "LAST SEEN\tTYPE\tREASON\tOBJECT\tCOUNT\tMESSAGE\n"
	tableHeaderWithNS = "NAMESPACE\tLAST SEEN\tTYPE\tREASON\tOBJECT\tCOUNT\tMESSAGE\n"
)

// formatAge converts a time.Time to a human-readable relative time.
func formatAge(t time.Time) string {
	return formatAgeFrom(t, time.Now())
}

// formatAgeFrom formats a duration relative to a reference time.
// Extracted for deterministic testing without clock drift.
func formatAgeFrom(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "<unknown>"
	}

	d := now.Sub(t)
	if d < 0 {
		return "0s"
	}

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

// truncateMessage truncates long messages for table display.
// Uses rune-safe truncation to avoid splitting multibyte UTF-8 characters.
func truncateMessage(msg string) string {
	r := []rune(msg)
	if len(r) <= maxMessageLen {
		return msg
	}

	return string(r[:maxMessageLen-3]) + "..."
}

// ansiEscapeRegex matches ANSI escape sequences (CSI sequences like \x1b[31m).
var ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// sanitizeForTerminal removes ANSI escape sequences and control characters
// from untrusted input to prevent terminal escape sequence injection (CWE-150).
func sanitizeForTerminal(s string) string {
	// First strip full ANSI sequences (e.g., \x1b[31m) to avoid bracket remnants
	s = ansiEscapeRegex.ReplaceAllString(s, "")

	// Then strip any remaining control characters
	return strings.Map(func(r rune) rune {
		// Keep printable characters and tabs
		if r >= 32 || r == '\t' {
			return r
		}
		// Drop control characters including bare ESC (\x1b)
		return -1
	}, s)
}

// renderOutput dispatches to the appropriate output formatter.
func (c *Command) renderOutput(events []clusterhealth.EventInfo) error {
	switch c.OutputFormat {
	case outputFormatJSON:
		return outputJSON(c.IO.Out(), events)
	case outputFormatYAML:
		return outputYAML(c.IO.Out(), events)
	default:
		return outputTable(c.IO.Out(), events, c.AllNamespaces)
	}
}

// outputTable renders events as a formatted table.
// When showNamespace is true, adds a NAMESPACE column (like kubectl -A).
//
//nolint:revive // flag-parameter: showNamespace mirrors kubectl -A behavior
func outputTable(w io.Writer, events []clusterhealth.EventInfo, showNamespace bool) error {
	if len(events) == 0 {
		if _, err := fmt.Fprintln(w, msgNoEventsFound); err != nil {
			return clierrors.ErrRenderFailed("table", err)
		}

		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, tabwriterPadding, ' ', 0)

	if showNamespace {
		if _, err := fmt.Fprint(tw, tableHeaderWithNS); err != nil {
			return clierrors.ErrRenderFailed("table", err)
		}
	} else {
		if _, err := fmt.Fprint(tw, tableHeader); err != nil {
			return clierrors.ErrRenderFailed("table", err)
		}
	}

	for _, e := range events {
		age := formatAge(e.LastTime)
		// Sanitize all rendered fields for consistency (CWE-150)
		eventType := sanitizeForTerminal(e.Type)
		kind := sanitizeForTerminal(e.Kind)
		name := sanitizeForTerminal(e.Name)
		reason := sanitizeForTerminal(e.Reason)
		message := truncateMessage(sanitizeForTerminal(e.Message))
		object := fmt.Sprintf("%s/%s", kind, name)

		if showNamespace {
			namespace := sanitizeForTerminal(e.Namespace)
			if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\n", namespace, age, eventType, reason, object, e.Count, message); err != nil {
				return clierrors.ErrRenderFailed("table", err)
			}
		} else {
			if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n", age, eventType, reason, object, e.Count, message); err != nil {
				return clierrors.ErrRenderFailed("table", err)
			}
		}
	}

	if err := tw.Flush(); err != nil {
		return clierrors.ErrRenderFailed("table", err)
	}

	return nil
}

// outputJSON renders events as JSON.
func outputJSON(w io.Writer, events []clusterhealth.EventInfo) error {
	output := toEventOutputList(events)

	renderer := printerjson.NewRenderer[any](
		printerjson.WithWriter[any](w),
	)

	if err := renderer.Render(output); err != nil {
		return clierrors.ErrRenderFailed("JSON", err)
	}

	return nil
}

// outputYAML renders events as YAML.
func outputYAML(w io.Writer, events []clusterhealth.EventInfo) error {
	output := toEventOutputList(events)

	renderer := printeryaml.NewRenderer[any](
		printeryaml.WithWriter[any](w),
	)

	if err := renderer.Render(output); err != nil {
		return clierrors.ErrRenderFailed("YAML", err)
	}

	return nil
}

// toEventOutputList converts EventInfo slice to a structure suitable for JSON/YAML rendering.
// Always returns consistent EventList shape for stable machine consumption.
func toEventOutputList(events []clusterhealth.EventInfo) any {
	return map[string]any{
		"apiVersion": "v1",
		"kind":       "EventList",
		"items":      events,
	}
}
