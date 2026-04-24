package deps_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/deps"

	. "github.com/onsi/gomega"
)

func TestEmbeddedManifest(t *testing.T) {
	g := NewWithT(t)

	data := deps.EmbeddedManifest()

	if len(data) == 0 {
		t.Skip("Skipping: embedded manifest not available (run 'make fetch-deps' first)")
	}

	manifest, err := deps.Parse(data)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(manifest).ToNot(BeNil())
}

func TestManifestVersion(t *testing.T) {
	g := NewWithT(t)

	version, err := deps.ManifestVersion()

	if err != nil {
		t.Skip("Skipping: embedded Chart.yaml not available (run 'make fetch-deps' first)")
	}

	g.Expect(len(version)).To(BeNumerically(">=", 3))
}
