package stdin

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"sigs.k8s.io/yaml"
)

const maxInputBytes = 1 << 20 // 1 MiB

// Parse reads JSON or YAML from the reader and unmarshals it into the target struct.
// It uses strict unmarshaling to reject unknown fields, helping catch typos in input.
// Input is limited to 1 MiB to prevent memory exhaustion.
func Parse(r io.Reader, target any) error {
	limited := io.LimitReader(r, maxInputBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	if len(data) > maxInputBytes {
		return fmt.Errorf("input too large: max %d bytes", maxInputBytes)
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return errors.New("empty input: expected JSON or YAML")
	}

	// UnmarshalStrict rejects unknown fields (catches typos)
	// sigs.k8s.io/yaml handles both JSON and YAML
	if err := yaml.UnmarshalStrict(data, target); err != nil {
		return fmt.Errorf("parsing input: %w", err)
	}

	return nil
}

// IsPiped returns true if the file is not a terminal (i.e., has piped data).
// Use this to warn users who specify --from-stdin but forget to pipe input.
func IsPiped(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		// Stat failure is rare; return false to assume terminal and show warning.
		// This is safe: the warning is non-blocking and helps catch user mistakes.
		return false
	}

	// If it's a character device, it's a terminal (not piped)
	return (stat.Mode() & os.ModeCharDevice) == 0
}
