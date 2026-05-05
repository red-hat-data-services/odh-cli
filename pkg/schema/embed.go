package schema

import (
	"embed"
	"fmt"
	"io"
)

//go:embed data/*.json
var embeddedSchemas embed.FS

// Get returns the embedded JSON Schema for the given type.
func Get(schemaType SchemaType) ([]byte, error) {
	path := fmt.Sprintf("data/%s.json", schemaType)

	data, err := embeddedSchemas.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("schema %q not found: %w", schemaType, err)
	}

	return data, nil
}

// WriteTo writes the JSON Schema for the given type to the writer with a trailing newline.
func WriteTo(w io.Writer, schemaType SchemaType) error {
	data, err := Get(schemaType)
	if err != nil {
		return fmt.Errorf("retrieving schema: %w", err)
	}

	if _, err = w.Write(data); err != nil {
		return fmt.Errorf("writing schema: %w", err)
	}

	if _, err = w.Write([]byte("\n")); err != nil {
		return fmt.Errorf("writing newline: %w", err)
	}

	return nil
}
