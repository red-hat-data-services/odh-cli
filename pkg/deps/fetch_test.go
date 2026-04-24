package deps_test

import (
	"errors"
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/deps"

	. "github.com/onsi/gomega"
)

func TestGetManifest_EmbeddedMode(t *testing.T) {
	g := NewWithT(t)

	result, err := deps.GetManifest(t.Context(), false)

	if err != nil {
		if errors.Is(err, deps.ErrEmbeddedEmpty) {
			t.Skip("Skipping: embedded manifest not available")
		}

		t.Fatalf("GetManifest(refresh=false) error = %v", err)
	}

	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Manifest).ToNot(BeNil())
}

func TestGetManifest_EmbeddedMode_HasDependencies(t *testing.T) {
	g := NewWithT(t)

	result, err := deps.GetManifest(t.Context(), false)

	if err != nil {
		if errors.Is(err, deps.ErrEmbeddedEmpty) {
			t.Skip("Skipping: embedded manifest not available")
		}

		t.Fatalf("GetManifest(refresh=false) error = %v", err)
	}

	g.Expect(result.Manifest.Dependencies).ToNot(BeEmpty())
}

func TestGetManifest_EmbeddedMode_HasVersion(t *testing.T) {
	g := NewWithT(t)

	result, err := deps.GetManifest(t.Context(), false)

	if err != nil {
		if errors.Is(err, deps.ErrEmbeddedEmpty) {
			t.Skip("Skipping: embedded manifest not available")
		}

		t.Fatalf("GetManifest(refresh=false) error = %v", err)
	}

	g.Expect(result.Version).ToNot(BeEmpty())
	g.Expect(len(result.Version)).To(BeNumerically(">=", 3))
}
