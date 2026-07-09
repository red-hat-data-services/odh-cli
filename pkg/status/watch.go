package status

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/deps"
)

type watchData struct {
	report      *clusterhealth.Report
	depStatuses []deps.DependencyStatus
}

// runWatch polls clusterhealth.Run() and emits streaming output on each state change.
func (c *Command) runWatch(ctx context.Context) error {
	hc, err := c.resolveHealthConfig(ctx)
	if err != nil {
		return err
	}

	timeout := time.Duration(0)
	if c.TimeoutExplicit {
		timeout = c.Timeout
	}

	return c.RunWatch(ctx, cmd.WatchRunConfig{ //nolint:wrapcheck // structured watch errors propagate as-is
		Timeout:      timeout,
		PollInterval: c.PollInterval,
		ErrOut:       c.IO.ErrOut(),
		Poller: func(ctx context.Context) (any, []byte, error) {
			report, depStatuses, err := c.runHealthCheck(ctx, hc)
			if err != nil {
				return nil, nil, err
			}

			return &watchData{report, depStatuses}, snapshotReport(report), nil
		},
		Emitter: func(data any) error {
			d, ok := data.(*watchData)
			if !ok {
				return fmt.Errorf(msgUnexpectedWatchData, data)
			}

			return c.streamOutput(d.report, d.depStatuses)
		},
	})
}

// streamOutput writes a single report in the configured streaming format.
func (c *Command) streamOutput(report *clusterhealth.Report, depStatuses []deps.DependencyStatus) error {
	switch c.OutputFormat {
	case OutputFormatJSON:
		return streamJSON(c.IO.Out(), report, depStatuses)
	case OutputFormatYAML:
		return streamYAML(c.IO.Out(), report, depStatuses)
	case OutputFormatTable:
		return cmd.ErrWatchRequiresStructuredOutput()
	}

	return cmd.ErrWatchRequiresStructuredOutput()
}

// snapshotReport marshals the report to compact JSON for state-change comparison.
// The CollectedAt timestamp is zeroed to avoid false positives.
func snapshotReport(report *clusterhealth.Report) []byte {
	if report == nil {
		return nil
	}

	cpy := *report
	cpy.CollectedAt = time.Time{}

	data, err := json.Marshal(cpy)
	if err != nil {
		return []byte("marshal-error")
	}

	return data
}

// streamJSON writes a single StatusReport as a compact NDJSON line.
func streamJSON(w io.Writer, report *clusterhealth.Report, depStatuses []deps.DependencyStatus) error {
	statusReport := NewStatusReport(report, depStatuses)

	data, err := json.Marshal(statusReport)
	if err != nil {
		return fmt.Errorf("streaming JSON report: %w", err)
	}

	_, err = fmt.Fprintf(w, "%s\n", data)
	if err != nil {
		return fmt.Errorf("writing JSON line: %w", err)
	}

	return nil
}

// streamYAML writes a single StatusReport as a YAML document with a --- separator.
func streamYAML(w io.Writer, report *clusterhealth.Report, depStatuses []deps.DependencyStatus) error {
	if _, err := fmt.Fprint(w, "---\n"); err != nil {
		return fmt.Errorf("writing YAML separator: %w", err)
	}

	return renderYAML(w, report, depStatuses)
}
