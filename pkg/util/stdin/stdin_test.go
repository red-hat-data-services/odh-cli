package stdin_test

import (
	"os"
	"strings"
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/util/stdin"

	. "github.com/onsi/gomega"
)

// Test fixtures for stdin parsing.
const (
	fixtureJSONSimple     = `{"name": "test", "enabled": true}`
	fixtureJSONWithArrays = `{"name": "test", "items": ["a", "b", "c"]}`
	fixtureJSONMinimal    = `{"name": "minimal"}`
	fixtureJSONUnknown    = `{"name": "test", "unknownField": "value"}`
	fixtureJSONMalformed  = `{"name": "test"`
	fixtureYAMLMalformed  = "name: [unclosed bracket\n"

	fixtureYAMLSimple = `
name: test
enabled: true
count: 42
`
	fixtureYAMLWithArrays = `
name: test
items:
  - alpha
  - beta
  - gamma
`
)

type testInput struct {
	Name    string   `json:"name"`
	Items   []string `json:"items,omitempty"`
	Enabled bool     `json:"enabled,omitempty"`
	Count   int      `json:"count,omitempty"`
}

func TestParse_JSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantErr  bool
	}{
		{
			name:     "simple JSON",
			input:    fixtureJSONSimple,
			wantName: "test",
			wantErr:  false,
		},
		{
			name:     "JSON with arrays",
			input:    fixtureJSONWithArrays,
			wantName: "test",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			reader := strings.NewReader(tt.input)

			var result testInput
			err := stdin.Parse(reader, &result)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(result.Name).To(Equal(tt.wantName))
			}
		})
	}
}

func TestParse_YAML(t *testing.T) {
	t.Run("simple YAML", func(t *testing.T) {
		g := NewWithT(t)
		reader := strings.NewReader(fixtureYAMLSimple)

		var result testInput
		err := stdin.Parse(reader, &result)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.Name).To(Equal("test"))
		g.Expect(result.Enabled).To(BeTrue())
		g.Expect(result.Count).To(Equal(42))
	})

	t.Run("YAML with arrays", func(t *testing.T) {
		g := NewWithT(t)
		reader := strings.NewReader(fixtureYAMLWithArrays)

		var result testInput
		err := stdin.Parse(reader, &result)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.Name).To(Equal("test"))
		g.Expect(result.Items).To(Equal([]string{"alpha", "beta", "gamma"}))
	})
}

func TestParse_InvalidInput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		errContains string
	}{
		{
			name:        "empty input",
			input:       "",
			errContains: "empty input",
		},
		{
			name:        "unknown fields rejected",
			input:       fixtureJSONUnknown,
			errContains: "parsing input",
		},
		{
			name:        "malformed JSON",
			input:       fixtureJSONMalformed,
			errContains: "parsing input",
		},
		{
			name:        "malformed YAML",
			input:       fixtureYAMLMalformed,
			errContains: "parsing input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			reader := strings.NewReader(tt.input)

			var result testInput
			err := stdin.Parse(reader, &result)

			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring(tt.errContains))
		})
	}
}

func TestParse_PartialInput(t *testing.T) {
	g := NewWithT(t)

	reader := strings.NewReader(fixtureJSONMinimal)

	var result testInput
	err := stdin.Parse(reader, &result)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.Name).To(Equal("minimal"))
	g.Expect(result.Items).To(BeNil())
	g.Expect(result.Enabled).To(BeFalse())
	g.Expect(result.Count).To(Equal(0))
}

func TestParse_InputTooLarge(t *testing.T) {
	g := NewWithT(t)

	// Create input larger than 1 MiB limit
	largeInput := strings.Repeat("x", 1<<20+1)
	reader := strings.NewReader(largeInput)

	var result testInput
	err := stdin.Parse(reader, &result)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("input too large"))
}

func TestIsPiped(t *testing.T) {
	t.Run("pipe returns true", func(t *testing.T) {
		g := NewWithT(t)

		// Create a pipe - the read end is not a TTY
		r, w, err := os.Pipe()
		g.Expect(err).NotTo(HaveOccurred())
		t.Cleanup(func() {
			_ = r.Close()
			_ = w.Close()
		})

		g.Expect(stdin.IsPiped(r)).To(BeTrue())
	})

	t.Run("regular file returns true", func(t *testing.T) {
		g := NewWithT(t)

		// Create a temp file in t.TempDir() - auto-cleaned up
		f, err := os.CreateTemp(t.TempDir(), "stdin-test-*")
		g.Expect(err).NotTo(HaveOccurred())
		t.Cleanup(func() { _ = f.Close() })

		g.Expect(stdin.IsPiped(f)).To(BeTrue())
	})
}
