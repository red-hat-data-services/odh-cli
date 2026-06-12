package otelexporter

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"
)

const (
	oldFieldProtocol        = "protocol"
	oldFieldTracesProtocol  = "tracesProtocol"
	oldFieldMetricsProtocol = "metricsProtocol"
	oldFieldOtlpEndpoint    = "otlpEndpoint"
	oldFieldTracesEndpoint  = "tracesEndpoint"
	oldFieldMetricsEndpoint = "metricsEndpoint"
	oldFieldOtlpExport      = "otlpExport"

	newFieldOtlpProtocol        = "otlpProtocol"
	newFieldOtlpTracesEndpoint  = "otlpTracesEndpoint"
	newFieldOtlpMetricsEndpoint = "otlpMetricsEndpoint"
	newFieldEnableTraces        = "enableTraces"
	newFieldEnableMetrics       = "enableMetrics"
)

//nolint:gochecknoglobals // Static field name lists; Go has no const slices.
var oldSchemaFields = []string{
	oldFieldProtocol, oldFieldTracesProtocol, oldFieldMetricsProtocol,
	oldFieldOtlpEndpoint, oldFieldTracesEndpoint, oldFieldMetricsEndpoint,
	oldFieldOtlpExport,
}

//nolint:gochecknoglobals // Static field name lists; Go has no const slices.
var newSchemaFields = []string{
	newFieldOtlpProtocol, newFieldOtlpTracesEndpoint, newFieldOtlpMetricsEndpoint,
	newFieldEnableTraces, newFieldEnableMetrics,
}

type migrationStatus string

const (
	statusSkipped        migrationStatus = "SKIPPED"
	statusInvalid        migrationStatus = "INVALID"
	statusNeedsMigration migrationStatus = "NEEDS_MIGRATION"
)

type migrationInfo struct {
	gorchName string
	namespace string
	status    migrationStatus
	message   string
}

type migrationCandidate struct {
	name      string
	namespace string
	patchData []byte
}

func hasAnyOldFields(otel map[string]any) bool {
	for _, field := range oldSchemaFields {
		if _, ok := otel[field]; ok {
			return true
		}
	}

	return false
}

func hasOnlyNewFields(otel map[string]any) bool {
	hasNew := false

	for _, field := range newSchemaFields {
		if _, ok := otel[field]; ok {
			hasNew = true
		}
	}

	if !hasNew {
		return false
	}

	return !hasAnyOldFields(otel)
}

func mapOtelFields(otel map[string]any) (map[string]any, []string) {
	result := make(map[string]any)
	var warnings []string

	protocol := resolveProtocol(otel, &warnings)
	if protocol != "" {
		result[newFieldOtlpProtocol] = protocol
	}

	endpoints := resolveEndpoints(otel)
	if endpoints.traces != "" {
		result[newFieldOtlpTracesEndpoint] = endpoints.traces
	}

	if endpoints.metrics != "" {
		result[newFieldOtlpMetricsEndpoint] = endpoints.metrics
	}

	exports := resolveExportFlags(otel)
	if exports.traces {
		result[newFieldEnableTraces] = true
	}

	if exports.metrics {
		result[newFieldEnableMetrics] = true
	}

	return result, warnings
}

func resolveProtocol(otel map[string]any, warnings *[]string) string {
	if p, ok := getString(otel, oldFieldProtocol); ok {
		return p
	}

	tracesP, hasTraces := getString(otel, oldFieldTracesProtocol)
	metricsP, hasMetrics := getString(otel, oldFieldMetricsProtocol)

	if hasTraces && hasMetrics && tracesP != metricsP {
		*warnings = append(*warnings, "tracesProtocol and metricsProtocol differ; new schema supports a single protocol")
	}

	if hasTraces {
		return tracesP
	}

	if hasMetrics {
		return metricsP
	}

	return ""
}

type resolvedEndpoints struct {
	traces  string
	metrics string
}

func resolveEndpoints(otel map[string]any) resolvedEndpoints {
	sharedEndpoint, _ := getString(otel, oldFieldOtlpEndpoint)

	traces := sharedEndpoint
	if ep, ok := getString(otel, oldFieldTracesEndpoint); ok {
		traces = ep
	}

	metrics := sharedEndpoint
	if ep, ok := getString(otel, oldFieldMetricsEndpoint); ok {
		metrics = ep
	}

	return resolvedEndpoints{traces: traces, metrics: metrics}
}

type resolvedExportFlags struct {
	traces  bool
	metrics bool
}

func resolveExportFlags(otel map[string]any) resolvedExportFlags {
	exportVal, ok := otel[oldFieldOtlpExport]
	if !ok {
		return resolvedExportFlags{}
	}

	tokens := strings.FieldsFunc(strings.ToLower(fmt.Sprintf("%v", exportVal)), func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '|'
	})

	var flags resolvedExportFlags

	for _, tok := range tokens {
		switch tok {
		case "all":
			flags.traces = true
			flags.metrics = true
		case "trace", "traces":
			flags.traces = true
		case "metric", "metrics":
			flags.metrics = true
		}
	}

	return flags
}

func getString(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}

	s, ok := v.(string)

	return s, ok
}

func buildOtelMigrationPatch(newFields map[string]any) ([]byte, error) {
	otelPatch := make(map[string]any, len(newFields)+len(oldSchemaFields))

	for _, field := range oldSchemaFields {
		otelPatch[field] = nil
	}

	maps.Copy(otelPatch, newFields)

	patch := map[string]any{
		"spec": map[string]any{
			"otelExporter": otelPatch,
		},
	}

	data, err := json.MarshalIndent(patch, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling patch: %w", err)
	}

	return data, nil
}
