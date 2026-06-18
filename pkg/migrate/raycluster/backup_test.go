package raycluster_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/raycluster"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

func TestPreUpgrade_EmptyOutputDir(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	io := iostreams.NewIOStreams(nil, nil, nil)

	_, err := raycluster.PreUpgrade(ctx, nil, "", "", "", nil, io)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("output directory is required"))
}

func TestPreUpgrade_PreflightChecksFail(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	io := iostreams.NewIOStreams(nil, nil, nil)

	checks := []raycluster.PreflightCheck{
		{Name: "test-check", Passed: false, Message: "fail", Required: true},
	}

	dir := t.TempDir()
	_, err := raycluster.PreUpgrade(ctx, nil, dir, "", "", checks, io)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("pre-upgrade checks failed"))
}
