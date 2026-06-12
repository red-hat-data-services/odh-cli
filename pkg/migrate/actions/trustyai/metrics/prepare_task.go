package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	backupTimestampFmt   = "20060102-150405"
	backupFilePermission = 0o644
	backupDirPermission  = 0o755
	metricTypeFairness   = "fairness"
)

type prepareTask struct {
	action *MetricsAction
}

func (t *prepareTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("check-trustyai-metrics", "Check TrustyAI metrics routes")

	namespaces, err := discoverTrustyAINamespaces(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to discover TrustyAI routes: %v", err)

		return action.BuildResult(target)
	}

	if len(namespaces) == 0 {
		step.Completef(result.StepSkipped, "No TrustyAI routes found")
	} else {
		step.Completef(result.StepCompleted, "Found TrustyAI routes in %d namespace(s)", len(namespaces))
	}

	return action.BuildResult(target)
}

func (t *prepareTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("backup-trustyai-metrics", "Backup TrustyAI scheduled metrics")

	namespaces, err := discoverTrustyAINamespaces(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to discover TrustyAI routes: %v", err)

		return action.BuildResult(target)
	}

	if len(namespaces) == 0 {
		step.Completef(result.StepSkipped, "No TrustyAI routes found")

		return action.BuildResult(target)
	}

	http, err := newHTTPHelper(target.RESTConfig)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to create HTTP client: %v", err)

		return action.BuildResult(target)
	}

	outputDir := t.action.BackupDir
	if target.OutputDir != "" {
		outputDir = filepath.Join(target.OutputDir, "trustyai-metrics")
	}

	totalMetrics := 0

	for ns, routeHost := range namespaces {
		nsStep := step.Child(
			"backup-"+ns,
			"Backup metrics in "+ns,
		)

		count, err := t.backupNamespace(ctx, http, routeHost, ns, outputDir, target, nsStep)
		if err != nil {
			nsStep.Completef(result.StepFailed, "Failed to backup metrics in %s: %v", ns, err)

			continue
		}

		totalMetrics += count
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would backup %d metric(s) across %d namespace(s)", totalMetrics, len(namespaces))
	} else {
		step.Completef(result.StepCompleted, "Backed up %d metric(s) across %d namespace(s) to %s", totalMetrics, len(namespaces), outputDir)
	}

	return action.BuildResult(target)
}

func (t *prepareTask) backupNamespace(
	ctx context.Context,
	http *httpHelper,
	routeHost, namespace, outputDir string,
	target action.Target,
	step action.StepRecorder,
) (int, error) {
	apiURL := "https://" + routeHost + "/metrics/all/requests"
	if t.action.MetricType == metricTypeFairness {
		apiURL += "?type=fairness"
	}

	body, err := http.get(ctx, apiURL)
	if err != nil {
		return 0, fmt.Errorf("fetching metrics: %w", err)
	}

	var resp backupResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("parsing metrics response: %w", err)
	}

	metricCount := len(resp.Requests)

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would backup %d metric(s) from %s", metricCount, namespace)

		return metricCount, nil
	}

	if err := os.MkdirAll(outputDir, backupDirPermission); err != nil {
		return 0, fmt.Errorf("creating backup directory: %w", err)
	}

	typeSuffix := ""
	if t.action.MetricType == metricTypeFairness {
		typeSuffix = "-fairness"
	}

	timestamp := time.Now().Format(backupTimestampFmt)
	filename := fmt.Sprintf("trustyai-metrics-%s%s-%s.json", namespace, typeSuffix, timestamp)
	backupPath := filepath.Join(outputDir, filename)

	prettyJSON, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("formatting backup JSON: %w", err)
	}

	if err := os.WriteFile(backupPath, prettyJSON, backupFilePermission); err != nil {
		return 0, fmt.Errorf("writing backup file: %w", err)
	}

	step.Completef(result.StepCompleted, "Backed up %d metric(s) to %s", metricCount, backupPath)

	return metricCount, nil
}

func discoverTrustyAINamespaces(ctx context.Context, target action.Target) (map[string]string, error) {
	routes, err := target.Client.List(ctx, resources.Route)
	if err != nil {
		return nil, fmt.Errorf("listing routes: %w", err)
	}

	discovered := make(map[string]string)
	routeLabel := defaultRouteLabel

	if a, ok := target.Recorder.(interface{ Action() action.Action }); ok {
		if ma, ok := a.Action().(*MetricsAction); ok && ma.RouteLabel != "" {
			routeLabel = ma.RouteLabel
		}
	}

	for _, route := range routes {
		ns := route.GetNamespace()
		if _, exists := discovered[ns]; exists {
			continue
		}

		host, err := discoverRouteInNamespace(ctx, target.Client, ns, routeLabel)
		if err != nil {
			continue
		}

		if host != "" {
			discovered[ns] = host
		}
	}

	if len(discovered) == 0 {
		taiServices, err := target.Client.List(ctx, resources.TrustyAIService)
		if err != nil {
			return nil, fmt.Errorf("listing TrustyAIServices: %w", err)
		}

		for _, svc := range taiServices {
			ns := svc.GetNamespace()
			if _, exists := discovered[ns]; exists {
				continue
			}

			host, err := discoverRouteInNamespace(ctx, target.Client, ns, routeLabel)
			if err != nil || host == "" {
				continue
			}

			discovered[ns] = host
		}
	}

	return discovered, nil
}
