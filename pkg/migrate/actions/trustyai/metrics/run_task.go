package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
)

type runTask struct {
	action *MetricsAction
}

func (t *runTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("check-metrics-restore", "Check metrics restore readiness")

	if t.action.BackupFile == "" {
		step.Completef(result.StepSkipped, "No --metrics-file specified; nothing to restore")

		return action.BuildResult(target)
	}

	if _, err := os.Stat(t.action.BackupFile); err != nil {
		step.Completef(result.StepFailed, "Backup file %s not found: %v", t.action.BackupFile, err)

		return action.BuildResult(target)
	}

	namespaces, err := discoverTrustyAINamespaces(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to discover TrustyAI routes: %v", err)

		return action.BuildResult(target)
	}

	if len(namespaces) == 0 {
		step.Completef(result.StepFailed, "No TrustyAI routes found for restore")
	} else {
		step.Completef(result.StepCompleted, "Ready to restore: backup file exists, %d route(s) available", len(namespaces))
	}

	return action.BuildResult(target)
}

func (t *runTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("restore-trustyai-metrics", "Restore TrustyAI scheduled metrics")

	if t.action.BackupFile == "" {
		step.Completef(result.StepSkipped, "No --metrics-file specified; nothing to restore")

		return action.BuildResult(target)
	}

	entries, err := t.parseBackupEntries()
	if err != nil {
		step.Completef(result.StepFailed, "Failed to load backup: %v", err)

		return action.BuildResult(target)
	}

	if len(entries) == 0 {
		step.Completef(result.StepSkipped, "No metrics found in backup file")

		return action.BuildResult(target)
	}

	step.Recordf("parse", "Found %d metric(s) in backup file", result.StepCompleted, len(entries))

	namespaces, err := discoverTrustyAINamespaces(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to discover TrustyAI routes: %v", err)

		return action.BuildResult(target)
	}

	if len(namespaces) == 0 {
		step.Completef(result.StepFailed, "No TrustyAI routes found for restore")

		return action.BuildResult(target)
	}

	httpH, err := newHTTPHelper(target.RESTConfig)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to create HTTP client: %v", err)

		return action.BuildResult(target)
	}

	if !target.DryRun && !target.SkipConfirm {
		target.IO.Fprintln()
		target.IO.Errorf("About to restore %d metric(s) to %d namespace(s)", len(entries), len(namespaces))

		if !confirmation.Prompt(target.IO, "Proceed with metrics restore?") {
			step.Completef(result.StepSkipped, "User cancelled metrics restore")

			return action.BuildResult(target)
		}
	}

	for ns, routeHost := range namespaces {
		nsStep := step.Child("restore-"+ns, "Restore metrics to "+ns)
		t.restoreToNamespace(ctx, httpH, routeHost, ns, entries, target, nsStep)
	}

	step.Completef(result.StepCompleted, "Metrics restore complete")

	return action.BuildResult(target)
}

func (t *runTask) parseBackupEntries() ([]restoreEntry, error) {
	data, err := os.ReadFile(t.action.BackupFile)
	if err != nil {
		return nil, fmt.Errorf("reading backup file: %w", err)
	}

	var raw struct {
		Requests []json.RawMessage `json:"requests"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing backup file: %w", err)
	}

	entries := make([]restoreEntry, 0, len(raw.Requests))

	for _, rawReq := range raw.Requests {
		var entry restoreEntry
		if err := json.Unmarshal(rawReq, &entry); err != nil {
			return nil, fmt.Errorf("parsing metric entry: %w", err)
		}

		entry.Raw = rawReq
		entries = append(entries, entry)
	}

	return entries, nil
}

func (t *runTask) restoreToNamespace(
	ctx context.Context,
	httpH *httpHelper,
	routeHost, ns string,
	entries []restoreEntry,
	target action.Target,
	nsStep action.StepRecorder,
) {
	var existingMetrics map[string]bool

	if t.action.SkipExisting {
		var err error

		existingMetrics, err = fetchExistingMetrics(ctx, httpH, routeHost)
		if err != nil {
			target.IO.Errorf("WARNING: could not fetch existing metrics in %s: %v", ns, err)

			existingMetrics = map[string]bool{}
		}
	}

	restored, skipped, failed := 0, 0, 0

	for _, entry := range entries {
		status := t.restoreMetric(ctx, httpH, routeHost, entry, existingMetrics, target, nsStep)

		switch status {
		case restoreSuccess:
			restored++
		case restoreSkipped:
			skipped++
		case restoreFailed:
			failed++
		}
	}

	if target.DryRun {
		nsStep.Completef(result.StepSkipped, "Would restore %d metric(s) to %s", len(entries), ns)
	} else {
		nsStep.Completef(result.StepCompleted, "Restored %d, skipped %d, failed %d in %s", restored, skipped, failed, ns)
	}
}

type restoreStatus int

const (
	restoreSuccess restoreStatus = iota
	restoreSkipped
	restoreFailed
)

func (t *runTask) restoreMetric(
	ctx context.Context,
	httpH *httpHelper,
	routeHost string,
	entry restoreEntry,
	existingMetrics map[string]bool,
	target action.Target,
	step action.StepRecorder,
) restoreStatus {
	metricName := entry.Request.MetricName
	modelID := entry.Request.ModelID

	if t.action.SkipExisting && existingMetrics != nil {
		key := modelID + ":" + metricName
		if existingMetrics[key] {
			step.Recordf(fmt.Sprintf("skip-%s-%s", modelID, metricName),
				"Skipped %s for model %s (already exists)", result.StepSkipped, metricName, modelID)

			return restoreSkipped
		}
	}

	endpointPath, ok := metricEndpoints[strings.ToLower(metricName)]
	if !ok {
		step.Recordf(fmt.Sprintf("unknown-%s-%s", modelID, metricName),
			"Unknown metric type %s for model %s", result.StepFailed, metricName, modelID)

		return restoreFailed
	}

	endpoint := "https://" + routeHost + endpointPath

	if target.DryRun {
		step.Recordf(fmt.Sprintf("dryrun-%s-%s", modelID, metricName),
			"Would POST %s for model %s to %s", result.StepSkipped, metricName, modelID, endpointPath)

		return restoreSkipped
	}

	var full map[string]json.RawMessage
	if err := json.Unmarshal(entry.Raw, &full); err != nil {
		step.Recordf(fmt.Sprintf("parse-err-%s-%s", modelID, metricName),
			"Failed to parse entry for %s/%s: %v", result.StepFailed, metricName, modelID, err)

		return restoreFailed
	}

	requestPayload, ok := full["request"]
	if !ok {
		step.Recordf(fmt.Sprintf("missing-req-%s-%s", modelID, metricName),
			"Entry missing .request field for %s/%s", result.StepFailed, metricName, modelID)

		return restoreFailed
	}

	statusCode, err := httpH.post(ctx, endpoint, requestPayload)
	if err != nil {
		step.Recordf(fmt.Sprintf("http-err-%s-%s", modelID, metricName),
			"HTTP error for %s/%s: %v", result.StepFailed, metricName, modelID, err)

		return restoreFailed
	}

	if statusCode != http.StatusOK {
		step.Recordf(fmt.Sprintf("http-status-%s-%s", modelID, metricName),
			"HTTP %d for %s/%s", result.StepFailed, statusCode, metricName, modelID)

		return restoreFailed
	}

	step.Recordf(fmt.Sprintf("restored-%s-%s", modelID, metricName),
		"Restored %s for model %s", result.StepCompleted, metricName, modelID)

	return restoreSuccess
}

func fetchExistingMetrics(ctx context.Context, httpH *httpHelper, routeHost string) (map[string]bool, error) {
	apiURL := "https://" + routeHost + "/metrics/all/requests"

	body, err := httpH.get(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("fetching existing metrics: %w", err)
	}

	var resp struct {
		Requests []struct {
			Request struct {
				MetricName string `json:"metricName"`
				ModelID    string `json:"modelId"`
			} `json:"request"`
		} `json:"requests"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing existing metrics: %w", err)
	}

	existing := make(map[string]bool, len(resp.Requests))
	for _, r := range resp.Requests {
		key := r.Request.ModelID + ":" + r.Request.MetricName
		existing[key] = true
	}

	return existing, nil
}
