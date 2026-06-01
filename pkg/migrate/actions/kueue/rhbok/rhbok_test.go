package rhbok_test

import (
	"testing"

	"github.com/blang/semver/v4"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"

	. "github.com/onsi/gomega"
)

func TestRHBOKMigrationAction_Metadata(t *testing.T) {
	g := NewWithT(t)
	a := &rhbok.RHBOKMigrationAction{}

	g.Expect(a.ID()).To(Equal("kueue.rhbok.migrate"))
	g.Expect(a.Name()).ToNot(BeEmpty())
	g.Expect(a.Description()).ToNot(BeEmpty())
	g.Expect(a.Group()).To(Equal(action.GroupMigration))
	g.Expect(a.Phase()).To(Equal(action.PhasePreUpgrade))
	g.Expect(a.Prepare()).ToNot(BeNil())
	g.Expect(a.Run()).ToNot(BeNil())
}

func TestRHBOKMigrationAction_CanApply(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected bool
	}{
		{name: "2.24 too old", version: "2.24.0", expected: false},
		{name: "2.25 minimum", version: "2.25.0", expected: true},
		{name: "2.26", version: "2.26.0", expected: true},
		{name: "2.99", version: "2.99.0", expected: true},
		{name: "3.0 wrong major", version: "3.0.0", expected: false},
		{name: "1.0 wrong major", version: "1.0.0", expected: false},
	}

	t.Run("nil CurrentVersion", func(t *testing.T) {
		g := NewWithT(t)
		a := &rhbok.RHBOKMigrationAction{}

		target := action.Target{CurrentVersion: nil}
		g.Expect(a.CanApply(target)).To(BeFalse())
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			a := &rhbok.RHBOKMigrationAction{}

			v := semver.MustParse(tt.version)
			target := action.Target{CurrentVersion: &v}

			g.Expect(a.CanApply(target)).To(Equal(tt.expected))
		})
	}
}
