package events

import (
	"errors"
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
		showNamespace := c.AllNamespaces || !c.NamespaceExplicit

		return outputTable(c.IO.Out(), events, showNamespace)
	}
}

// outputTable renders events as a formatted table.
// When showNamespace is true, adds a NAMESPACE column (like kubectl -A).
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

// streamWriter writes events with fixed-width columns for streaming output.
type streamWriter struct {
	w             io.Writer
	showNamespace bool
}

func newStreamWriter(w io.Writer, showNamespace bool) *streamWriter {
	return &streamWriter{w: w, showNamespace: showNamespace}
}

const (
	colWidthNamespace = 20
	colWidthLastSeen  = 10
	colWidthType      = 8
	colWidthReason    = 20
	colWidthObject    = 40
	colWidthCount     = 6
)

func (sw *streamWriter) writeHeader() error {
	var format string
	if sw.showNamespace {
		format = fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%s\n",
			colWidthNamespace, colWidthLastSeen, colWidthType, colWidthReason, colWidthObject, colWidthCount)
		if _, err := fmt.Fprintf(sw.w, format, "NAMESPACE", "LAST SEEN", "TYPE", "REASON", "OBJECT", "COUNT", "MESSAGE"); err != nil {
			return fmt.Errorf("writing header: %w", err)
		}

		return nil
	}

	format = fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%s\n",
		colWidthLastSeen, colWidthType, colWidthReason, colWidthObject, colWidthCount)
	if _, err := fmt.Fprintf(sw.w, format, "LAST SEEN", "TYPE", "REASON", "OBJECT", "COUNT", "MESSAGE"); err != nil {
		return fmt.Errorf("writing header: %w", err)
	}

	return nil
}

func (sw *streamWriter) writeEvent(e clusterhealth.EventInfo) error {
	age := formatAge(e.LastTime)
	eventType := sanitizeForTerminal(e.Type)
	kind := sanitizeForTerminal(e.Kind)
	name := sanitizeForTerminal(e.Name)
	reason := sanitizeForTerminal(e.Reason)
	message := truncateMessage(sanitizeForTerminal(e.Message))
	object := truncateField(fmt.Sprintf("%s/%s", kind, name), colWidthObject)

	var format string
	if sw.showNamespace {
		namespace := truncateField(sanitizeForTerminal(e.Namespace), colWidthNamespace)
		format = fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%dd  %%s\n",
			colWidthNamespace, colWidthLastSeen, colWidthType, colWidthReason, colWidthObject, colWidthCount)
		if _, err := fmt.Fprintf(sw.w, format, namespace, age, eventType, reason, object, e.Count, message); err != nil {
			return fmt.Errorf("writing event: %w", err)
		}

		return nil
	}

	format = fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%dd  %%s\n",
		colWidthLastSeen, colWidthType, colWidthReason, colWidthObject, colWidthCount)
	if _, err := fmt.Fprintf(sw.w, format, age, eventType, reason, object, e.Count, message); err != nil {
		return fmt.Errorf("writing event: %w", err)
	}

	return nil
}

func truncateField(s string, maxWidth int) string {
	if maxWidth <= 3 {
		return s
	}

	r := []rune(s)
	if len(r) <= maxWidth {
		return s
	}

	return string(r[:maxWidth-3]) + "..."
}

func (c *Command) printStreamHeader(showNamespace bool) error {
	c.streamOut = newStreamWriter(c.IO.Out(), showNamespace)

	if err := c.streamOut.writeHeader(); err != nil {
		return clierrors.ErrRenderFailed("header", err)
	}

	return nil
}

func (c *Command) printSingleEvent(e clusterhealth.EventInfo) error {
	if c.streamOut == nil {
		return clierrors.ErrRenderFailed("stream", errors.New("stream writer not initialized"))
	}

	if err := c.streamOut.writeEvent(e); err != nil {
		return clierrors.ErrRenderFailed("stream", err)
	}

	return nil
}
