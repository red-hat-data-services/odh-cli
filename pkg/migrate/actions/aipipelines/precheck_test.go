package aipipelines_test

import (
	"testing"

	"github.com/blang/semver/v4"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/aipipelines"

	. "github.com/onsi/gomega"
)

func TestPreUpgradeCheckAction(t *testing.T) {
	t.Run("metadata", func(t *testing.T) {
		g := NewWithT(t)

		a := &aipipelines.PreUpgradeCheckAction{}
		g.Expect(a.ID()).To(Equal("ai-pipelines.pre-upgrade-check"))
		g.Expect(a.Name()).To(Equal("AI Pipelines pre-upgrade check"))
		g.Expect(a.Group()).To(Equal(action.GroupMigration))
		g.Expect(a.Phase()).To(Equal(action.PhasePreUpgrade))
	})

	t.Run("CanApply returns true for version 2.x", func(t *testing.T) {
		g := NewWithT(t)

		a := &aipipelines.PreUpgradeCheckAction{}
		v2 := semver.MustParse("2.25.0")
		v3 := semver.MustParse("3.0.0")

		g.Expect(a.CanApply(action.Target{CurrentVersion: &v2})).To(BeTrue())
		g.Expect(a.CanApply(action.Target{CurrentVersion: &v3})).To(BeFalse())
	})

	t.Run("has both Prepare and Run tasks", func(t *testing.T) {
		g := NewWithT(t)

		a := &aipipelines.PreUpgradeCheckAction{}
		g.Expect(a.Prepare()).ToNot(BeNil())
		g.Expect(a.Run()).ToNot(BeNil())
	})
}
