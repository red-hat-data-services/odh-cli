//nolint:testpackage // Tests internal pure functions
package otelexporter

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestHasAnyOldFields(t *testing.T) {
	tests := []struct {
		name     string
		otel     map[string]any
		expected bool
	}{
		{"has protocol", map[string]any{"protocol": "grpc"}, true},
		{"has otlpEndpoint", map[string]any{"otlpEndpoint": "http://host"}, true},
		{"has otlpExport", map[string]any{"otlpExport": "all"}, true},
		{"new fields only", map[string]any{"otlpProtocol": "grpc"}, false},
		{"empty", map[string]any{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(hasAnyOldFields(tt.otel)).To(Equal(tt.expected))
		})
	}
}

func TestHasOnlyNewFields(t *testing.T) {
	tests := []struct {
		name     string
		otel     map[string]any
		expected bool
	}{
		{"new fields only", map[string]any{"otlpProtocol": "grpc", "enableTraces": true}, true},
		{"mixed old and new", map[string]any{"otlpProtocol": "grpc", "protocol": "grpc"}, false},
		{"old fields only", map[string]any{"protocol": "grpc"}, false},
		{"empty", map[string]any{}, false},
		{"unknown fields", map[string]any{"foo": "bar"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(hasOnlyNewFields(tt.otel)).To(Equal(tt.expected))
		})
	}
}

func TestMapOtelFields(t *testing.T) {
	t.Run("maps shared protocol", func(t *testing.T) {
		g := NewWithT(t)

		otel := map[string]any{"protocol": "grpc"}
		result, warnings := mapOtelFields(otel)

		g.Expect(warnings).To(BeEmpty())
		g.Expect(result).To(HaveKeyWithValue("otlpProtocol", "grpc"))
	})

	t.Run("maps per-signal endpoints", func(t *testing.T) {
		g := NewWithT(t)

		otel := map[string]any{
			"tracesEndpoint":  "http://traces:4317",
			"metricsEndpoint": "http://metrics:4317",
		}
		result, warnings := mapOtelFields(otel)

		g.Expect(warnings).To(BeEmpty())
		g.Expect(result).To(HaveKeyWithValue("otlpTracesEndpoint", "http://traces:4317"))
		g.Expect(result).To(HaveKeyWithValue("otlpMetricsEndpoint", "http://metrics:4317"))
	})

	t.Run("maps shared endpoint to both", func(t *testing.T) {
		g := NewWithT(t)

		otel := map[string]any{"otlpEndpoint": "http://shared:4317"}
		result, _ := mapOtelFields(otel)

		g.Expect(result).To(HaveKeyWithValue("otlpTracesEndpoint", "http://shared:4317"))
		g.Expect(result).To(HaveKeyWithValue("otlpMetricsEndpoint", "http://shared:4317"))
	})

	t.Run("maps export flags", func(t *testing.T) {
		g := NewWithT(t)

		otel := map[string]any{"otlpExport": "all"}
		result, _ := mapOtelFields(otel)

		g.Expect(result).To(HaveKeyWithValue("enableTraces", true))
		g.Expect(result).To(HaveKeyWithValue("enableMetrics", true))
	})

	t.Run("warns on conflicting protocols", func(t *testing.T) {
		g := NewWithT(t)

		otel := map[string]any{
			"tracesProtocol":  "grpc",
			"metricsProtocol": "http",
		}
		_, warnings := mapOtelFields(otel)

		g.Expect(warnings).To(HaveLen(1))
		g.Expect(warnings[0]).To(ContainSubstring("differ"))
	})
}

func TestResolveExportFlags(t *testing.T) {
	tests := []struct {
		name          string
		otel          map[string]any
		expectTraces  bool
		expectMetrics bool
	}{
		{"all", map[string]any{"otlpExport": "all"}, true, true},
		{"traces only", map[string]any{"otlpExport": "traces"}, true, false},
		{"metrics only", map[string]any{"otlpExport": "metrics"}, false, true},
		{"comma separated", map[string]any{"otlpExport": "traces,metrics"}, true, true},
		{"space separated", map[string]any{"otlpExport": "traces metrics"}, true, true},
		{"substring not matched", map[string]any{"otlpExport": "notrace"}, false, false},
		{"no export field", map[string]any{}, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			result := resolveExportFlags(tt.otel)
			g.Expect(result.traces).To(Equal(tt.expectTraces))
			g.Expect(result.metrics).To(Equal(tt.expectMetrics))
		})
	}
}

func TestBuildOtelMigrationPatch(t *testing.T) {
	g := NewWithT(t)

	newFields := map[string]any{
		"otlpProtocol": "grpc",
		"enableTraces": true,
	}

	patchBytes, err := buildOtelMigrationPatch(newFields)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(patchBytes)).To(ContainSubstring("otlpProtocol"))
	g.Expect(string(patchBytes)).To(ContainSubstring("enableTraces"))
	g.Expect(string(patchBytes)).To(ContainSubstring(`"protocol": null`))
}
