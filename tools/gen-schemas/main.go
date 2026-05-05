package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/invopop/jsonschema"

	"github.com/opendatahub-io/odh-cli/internal/version"
	"github.com/opendatahub-io/odh-cli/pkg/components"
	"github.com/opendatahub-io/odh-cli/pkg/deps"
	"github.com/opendatahub-io/odh-cli/pkg/get"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
)

const schemaFileMode = 0o644

type schemaTarget struct {
	name   string
	target any
}

func main() {
	outputDir := "pkg/schema/data"

	targets := []schemaTarget{
		{"diagnostic_result_list", result.DiagnosticResultList{}},
		{"component_list", components.ComponentList{}},
		{"component_details", components.ComponentDetails{}},
		{"dependency_status_list", deps.DependencyList{}},
		{"dependency_info_list", deps.DependencyManifestList{}},
		{"version_info", version.Info{}},
		{"version_info_verbose", version.VerboseInfo{}},
		{"kubernetes_list", get.KubernetesList{}},
	}

	reflector := &jsonschema.Reflector{
		DoNotReference: true,
	}

	for _, t := range targets {
		s := reflector.Reflect(t.target)

		// Remove unnecessary metadata fields
		s.Version = ""
		s.ID = ""

		data, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			_, _ = io.WriteString(os.Stderr, "error marshaling schema for "+t.name+": "+err.Error()+"\n")
			os.Exit(1)
		}

		outputPath := filepath.Join(outputDir, t.name+".json")

		if err := os.WriteFile(outputPath, data, schemaFileMode); err != nil {
			_, _ = io.WriteString(os.Stderr, "error writing schema to "+outputPath+": "+err.Error()+"\n")
			os.Exit(1)
		}

		_, _ = io.WriteString(os.Stdout, "Generated "+outputPath+"\n")
	}
}
