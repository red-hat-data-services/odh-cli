package training

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

const (
	statusUnknown = "Unknown"
	hoursPerDay   = 24
)

//nolint:gochecknoglobals // Immutable list of supported training job types
var TrainingJobTypes = []resources.ResourceType{
	resources.PyTorchJob,
	resources.TFJob,
	resources.MPIJob,
	resources.XGBoostJob,
}

type WorkloadEntry struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Age       string `json:"age"`
}

type ListFailure struct {
	Kind string
	Err  error
}

type WorkloadReport struct {
	Kind    string          `json:"kind"`
	Items   []WorkloadEntry `json:"items"`
	Summary ReportSummary   `json:"summary"`
}

type ReportSummary struct {
	Total    int            `json:"total"`
	ByKind   map[string]int `json:"byKind"`
	ByStatus map[string]int `json:"byStatus"`
}

// EnumerateWorkloads lists all v1 training workloads across all namespaces.
// It returns entries for successfully listed types and failures for types that
// could not be listed. An error is returned only if every type fails.
func EnumerateWorkloads(ctx context.Context, k8sClient client.Client) ([]WorkloadEntry, []ListFailure, error) {
	var entries []WorkloadEntry

	var failures []ListFailure

	for _, rt := range TrainingJobTypes {
		items, err := k8sClient.List(ctx, rt)
		if err != nil {
			if client.IsResourceTypeNotFound(err) {
				continue
			}

			if ctx.Err() != nil {
				return entries, failures, fmt.Errorf("listing %s: %w", rt.Kind, ctx.Err())
			}

			failures = append(failures, ListFailure{Kind: rt.Kind, Err: fmt.Errorf("listing %s: %w", rt.Kind, err)})

			continue
		}

		for _, item := range items {
			entries = append(entries, ExtractWorkloadEntry(item, rt.Kind))
		}
	}

	if len(failures) == len(TrainingJobTypes) {
		return nil, failures, errors.New("failed to list any training workload types")
	}

	if len(entries) == 0 && len(failures) > 0 {
		return nil, failures, errors.New("failed to list any training workload types")
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Namespace != entries[j].Namespace {
			return entries[i].Namespace < entries[j].Namespace
		}

		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}

		return entries[i].Name < entries[j].Name
	})

	return entries, failures, nil
}

func ExtractWorkloadEntry(obj *unstructured.Unstructured, kind string) WorkloadEntry {
	return WorkloadEntry{
		Namespace: obj.GetNamespace(),
		Kind:      kind,
		Name:      obj.GetName(),
		Status:    ExtractJobStatus(obj),
		Age:       formatAge(obj.GetCreationTimestamp().Time),
	}
}

func ExtractJobStatus(obj *unstructured.Unstructured) string {
	statusObj, ok := obj.Object["status"].(map[string]any)
	if !ok {
		return statusUnknown
	}

	conditions, ok := statusObj["conditions"].([]any)
	if !ok || len(conditions) == 0 {
		return statusUnknown
	}

	for i := len(conditions) - 1; i >= 0; i-- {
		cond, ok := conditions[i].(map[string]any)
		if !ok {
			continue
		}

		condStatus, _ := cond["status"].(string)
		if condStatus != "True" {
			continue
		}

		condType, _ := cond["type"].(string)
		if condType != "" {
			return condType
		}
	}

	return statusUnknown
}

func BuildReport(entries []WorkloadEntry) WorkloadReport {
	byKind := make(map[string]int)
	byStatus := make(map[string]int)

	for _, e := range entries {
		byKind[e.Kind]++
		byStatus[e.Status]++
	}

	return WorkloadReport{
		Kind:  "TrainingWorkloadReport",
		Items: entries,
		Summary: ReportSummary{
			Total:    len(entries),
			ByKind:   byKind,
			ByStatus: byStatus,
		},
	}
}

func formatAge(t time.Time) string {
	if t.IsZero() {
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
		return fmt.Sprintf("%dd", int(d.Hours()/hoursPerDay))
	}
}
