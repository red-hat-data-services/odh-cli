package schema_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/schema"

	. "github.com/onsi/gomega"
)

func TestGet_AllSchemas(t *testing.T) {
	tests := []struct {
		name       string
		schemaType schema.SchemaType
	}{
		{"diagnostic_result_list", schema.SchemaDiagnosticResultList},
		{"component_list", schema.SchemaComponentList},
		{"component_details", schema.SchemaComponentDetails},
		{"dependency_status_list", schema.SchemaDependencyStatusList},
		{"dependency_info_list", schema.SchemaDependencyInfoList},
		{"version_info", schema.SchemaVersionInfo},
		{"kubernetes_list", schema.SchemaKubernetesList},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			data, err := schema.Get(tt.schemaType)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(data).ToNot(BeEmpty())

			// Verify it's valid JSON
			var parsed map[string]any
			err = json.Unmarshal(data, &parsed)
			g.Expect(err).ToNot(HaveOccurred())

			// Verify it has JSON Schema structure
			g.Expect(parsed).To(HaveKey("type"))
			g.Expect(parsed).To(HaveKey("properties"))
		})
	}
}

func TestGet_InvalidSchema(t *testing.T) {
	g := NewWithT(t)

	_, err := schema.Get("nonexistent_schema")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("not found"))
}

func TestWriteTo_Success(t *testing.T) {
	g := NewWithT(t)

	var buf bytes.Buffer
	err := schema.WriteTo(&buf, schema.SchemaVersionInfo)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(buf.String()).To(HaveSuffix("\n"))

	// Verify content is valid JSON (minus the newline)
	content := buf.String()
	var parsed map[string]any
	err = json.Unmarshal([]byte(content), &parsed)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestWriteTo_InvalidSchema(t *testing.T) {
	g := NewWithT(t)

	var buf bytes.Buffer
	err := schema.WriteTo(&buf, "nonexistent_schema")

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("retrieving schema"))
}

// failingWriter is a writer that always fails.
type failingWriter struct {
	failOnCall int
	callCount  int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.callCount++
	if w.callCount >= w.failOnCall {
		return 0, errors.New("write failed")
	}

	return len(p), nil
}

func TestWriteTo_WriteError(t *testing.T) {
	g := NewWithT(t)

	// Fail on first write (schema data)
	w := &failingWriter{failOnCall: 1}
	err := schema.WriteTo(w, schema.SchemaVersionInfo)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("writing schema"))
}

func TestWriteTo_NewlineError(t *testing.T) {
	g := NewWithT(t)

	// Fail on second write (newline)
	w := &failingWriter{failOnCall: 2}
	err := schema.WriteTo(w, schema.SchemaVersionInfo)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("writing newline"))
}
