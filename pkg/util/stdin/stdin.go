package stdin

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/pflag"
	"sigs.k8s.io/yaml"
)

// FlagDesc is the standard --from-stdin flag description used across all commands.
const FlagDesc = "read configuration from stdin (JSON/YAML); CLI flags override stdin values"

// PipeChecker is an optional interface for io.Reader implementations that wrap
// an underlying file descriptor. Implement it on any Reader that wraps os.Stdin
// so that CheckPiped remains effective even when stdin is layered behind a wrapper.
type PipeChecker interface {
	IsPiped() bool
}

// CheckPiped returns an error if r is connected to an interactive terminal.
// It handles bare *os.File directly and delegates to PipeChecker for wrappers.
// Returns nil for other reader types (e.g. *bytes.Buffer in tests).
// Call this at the start of any --from-stdin handler to fail fast instead of blocking
// on io.ReadAll when the user forgets to pipe input.
func CheckPiped(r io.Reader) error {
	switch v := r.(type) {
	case *os.File:
		if !IsPiped(v) {
			return errors.New("stdin is a terminal; pipe input or omit --from-stdin")
		}
	case PipeChecker:
		if !v.IsPiped() {
			return errors.New("stdin is a terminal; pipe input or omit --from-stdin")
		}
	}

	return nil
}

// FlagChanged reports whether the named flag was explicitly set on the command line.
// Returns false if fs is nil (e.g. in tests that bypass AddFlags).
func FlagChanged(fs *pflag.FlagSet, name string) bool {
	if fs == nil {
		return false
	}

	f := fs.Lookup(name)

	return f != nil && f.Changed
}

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
		// Stat failure is rare; assume terminal so CheckPiped returns an error.
		// Safer to reject than to block on io.ReadAll against an unknown fd.
		return false
	}

	// If it's a character device, it's a terminal (not piped)
	return (stat.Mode() & os.ModeCharDevice) == 0
}
